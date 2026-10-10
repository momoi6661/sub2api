import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'

export const failure = (code, message, retryable = false) => ({ type: 'error', error: { code, message, retryable } })

const interrupted = signal => signal?.reason?.name === 'TimeoutError'
  ? failure('AUTH_TIMEOUT', 'Authorization did not complete within its deadline', true)
  : failure('TASK_CANCELLED', 'Task cancelled')

export function executeScript(task, { timeout = 30_000, signal } = {}) {
  return new Promise(resolve => {
    if (signal?.aborted) return resolve(interrupted(signal))
    const child = spawn(process.execPath, ['--max-old-space-size=128', fileURLToPath(new URL('./script-process.mjs', import.meta.url))], {
      stdio: ['pipe', 'pipe', 'ignore'], env: { PATH: process.env.PATH || '', SYSTEMROOT: process.env.SYSTEMROOT || '' }
    })
    let output = '', settled = false
    const finish = result => {
      if (settled) return
      settled = true
      clearTimeout(timer); signal?.removeEventListener('abort', abort)
      child.kill(); resolve(result)
    }
    const abort = () => finish(interrupted(signal))
    const timer = setTimeout(() => finish(failure('SCRIPT_TIMEOUT', 'Script exceeded 30 seconds', true)), timeout)
    signal?.addEventListener('abort', abort, { once: true })
    child.on('error', () => finish(failure('EXECUTOR_UNAVAILABLE', 'Code executor unavailable')))
    child.stdin.on('error', () => finish(failure('EXECUTOR_UNAVAILABLE', 'Code executor unavailable')))
    child.stdout.on('data', data => {
      output += data.toString()
      if (output.length > 131072) finish(failure('RESULT_TOO_LARGE', 'Script result exceeds size limit'))
    })
    child.on('close', () => {
      if (settled) return
      try { finish(JSON.parse(output)) } catch { finish(failure('INVALID_SCRIPT_RESULT', 'Code executor returned invalid output')) }
    })
    child.stdin.end(JSON.stringify(task))
  })
}

// A fresh browser context per account avoids sharing cookies or login identity.
// Only the email-code flow is automatic; password/MFA/CAPTCHA returns an error.
export async function authorizeWithBrowser(task, chromium, signal, report = () => {}) {
  let browser
  let phase = 'BROWSER_START_FAILED'
  try {
    if (signal?.aborted) return interrupted(signal)
    if (task.mailbox) {
      report('mailbox_connect')
      const snapshot = await executeScript({ ...task, operation: 'mailbox_snapshot' }, { timeout: 10000, signal })
      if (snapshot.type === 'error') return snapshot
      if (snapshot.type !== 'mailbox_snapshot' || !snapshot.uidNext || !snapshot.uidValidity) return failure('MAILBOX_SNAPSHOT_FAILED', 'Cannot record mailbox state')
      task = { ...task, mailbox: { ...task.mailbox, minUID: snapshot.uidNext, uidValidity: snapshot.uidValidity } }
    }
    const proxy = task.proxyURL ? new URL(task.proxyURL) : null
    report('browser_start')
    // Xvfb provides a display for headful Chromium on servers without a desktop.
    browser = await chromium.launch({ headless: false })
    if (signal?.aborted) return interrupted(signal)
    const context = await browser.newContext({ locale: 'en-US', extraHTTPHeaders: {
      'Accept-Language': 'en-US,en;q=0.9,zh-CN;q=0.8,zh;q=0.7'
    }, ...(proxy ? { proxy: {
      server: `${proxy.protocol}//${proxy.host}`, username: decodeURIComponent(proxy.username), password: decodeURIComponent(proxy.password)
    } } : {}) })
    const page = await context.newPage()
    page.setDefaultTimeout(8000)
    let callback = null
    await context.route(`${task.redirectURI}*`, async route => {
      const url = new URL(route.request().url())
      callback = { type: 'oauth_callback', code: url.searchParams.get('code'), state: url.searchParams.get('state'), error: url.searchParams.get('error') }
      await route.fulfill({ contentType: 'text/plain', body: 'Authorization callback received.' })
    })
    const abort = () => { void browser.close().catch(() => {}) }
    signal?.addEventListener('abort', abort, { once: true })
    try {
      if (signal?.aborted) return interrupted(signal)
      phase = 'BROWSER_NAVIGATION_FAILED'
      const navigation = await page.goto(task.authURL, { waitUntil: 'domcontentloaded', timeout: 30000 })
      if (navigation && navigation.status() >= 400) {
        const status = navigation.status()
        const body = (await page.locator('body').innerText().catch(() => '')).slice(0, 12000)
        const challenge = /just a moment|checking your browser|verify you are human|验证您是人类|检查您的浏览器/i.test(body)
          || await page.locator('iframe[src*="captcha"], iframe[src*="challenges.cloudflare.com"]').first().isVisible()
        const result = failure(challenge ? 'MANUAL_REQUIRED' : 'AUTH_PAGE_HTTP_ERROR', `Authorization page returned HTTP ${status}`, !challenge && (status === 429 || status >= 500))
        result.error.httpStatus = status
        return result
      }
      report('email_login')
      let submittedEmail = false, submittedCode = false, requestedCode = false, consentSubmitted = false
      let lastActionAt = Date.now()
      const deadline = Date.now() + 100000
      while (Date.now() < deadline && !signal?.aborted) {
        if (callback) {
          if (callback.error || !callback.code || callback.state !== task.state) return failure('OAUTH_CALLBACK_INVALID', 'OAuth callback failed or state did not match')
          return { type: 'oauth_callback', code: callback.code, state: callback.state }
        }
        const challenge = page.locator('iframe[src*="captcha"], iframe[src*="challenges.cloudflare.com"], input[autocomplete="webauthn"]')
        if (await challenge.first().isVisible()) return failure('MANUAL_REQUIRED', 'Login requires CAPTCHA or passkey verification')
        const pageText = (await page.locator('body').innerText()).slice(0, 12000)
        if (Date.now() - lastActionAt > 10000 && /just a moment|checking your browser|verify you are human|验证您是人类|检查您的浏览器/i.test(pageText)) return failure('MANUAL_REQUIRED', 'Login is blocked by a browser or human-verification challenge')
        const alert = page.getByRole('alert').first()
        if (await alert.isVisible() && /invalid|incorrect|expired|could not|try again|无效|错误|已过期/i.test(await alert.innerText())) return failure('AUTH_REJECTED', 'Login rejected the verification code or authorization request')
        // Hidden duplicate inputs/buttons are common on the login landing page.
        const labelledInput = async (label, fallback) => {
          const matches = page.getByLabel(label)
          for (let i = 0; i < await matches.count(); i++) {
            const candidate = matches.nth(i)
            if (await candidate.isVisible() && await candidate.isEditable()) return candidate
          }
          return page.locator(fallback).first()
        }
        const email = await labelledInput(/^(email( address)?|电子邮件地址|电子邮箱|邮箱地址)$/i, 'input[type="email"]:visible, input[name="email"]:visible, input[id*="email" i]:visible, input[autocomplete="email"]:visible, input[name="username"]:visible, input[autocomplete="username"]:visible')
        const otp = await labelledInput(/^(verification code|email code|code|验证码)$/i, 'input[autocomplete="one-time-code"]:visible, input[name="code"]:visible, input[name="otp"]:visible, input[name="otp_code"]:visible, input[name="one-time-code"]:visible')
        const visibleControl = async (scope, roles, name) => {
          for (const role of roles) {
            const matches = scope.getByRole(role, { name })
            for (let i = 0; i < await matches.count(); i++) { const candidate = matches.nth(i); if (await candidate.isVisible() && await candidate.isEnabled()) return candidate }
          }
          return null
        }
        const submitInput = async input => {
          const form = input.locator('xpath=ancestor::form[1]')
          const scope = await form.count() ? form : page
          const named = await visibleControl(scope, ['button'], /^(continue|submit|verify|allow|authorize|log\s?in|sign\s?in|继续|提交|验证|授权|登录)(\s|$)/i)
          if (named) { await named.click(); return }
          const submit = scope.locator('button[type="submit"]:visible, input[type="submit"]:visible').first()
          if (await submit.isVisible() && await submit.isEnabled()) { await submit.click(); return }
          throw new Error('Supported form submit control not found')
        }
        if (!submittedEmail && await email.isVisible()) {
          phase = 'EMAIL_SUBMIT_FAILED'
          await email.fill(task.context.loginEmail)
          await submitInput(email); submittedEmail = true
          lastActionAt = Date.now()
          report('requesting_code')
        } else if (!submittedCode && await otp.isVisible()) {
          phase = 'CODE_SUBMIT_FAILED'
          report('waiting_code')
          // Execute the configured script exactly once for this account/task.
          const result = await executeScript({ ...task, resultType: 'code' }, { signal })
          if (result.type === 'error') return result
          report('submitting_code')
          await otp.fill(result.code); await submitInput(otp); submittedCode = true
          lastActionAt = Date.now()
          report('consent')
        } else {
          const codeLabel = /send.*code|use.*code|email.*code|log\s?in with.*code|sign\s?in with.*code|one.time code|发送.*验证码|使用.*验证码|通过.*验证码/i
          const sendCode = await visibleControl(page, ['button', 'link'], codeLabel)
          const login = !submittedEmail ? await visibleControl(page, ['button', 'link'], /^(log\s?in|sign\s?in|continue\s+with\s+(email|email\s+address)|use\s+email|email\s+login|登录|使用邮箱)(\s|$)/i) : null
          const consent = submittedCode && !consentSubmitted ? await visibleControl(page, ['button'], /^(continue|allow|authorize|继续|允许|授权)(\s|$)/i) : null
          if (login) { phase = 'LOGIN_ENTRY_FAILED'; report('email_login'); await login.click(); lastActionAt = Date.now() }
          else if (!requestedCode && submittedEmail && sendCode) { phase = 'CODE_REQUEST_FAILED'; report('requesting_code'); await sendCode.click(); requestedCode = true; lastActionAt = Date.now() }
          else if (await page.locator('input[type="password"]:visible').first().isVisible() && Date.now() - lastActionAt > 5000) return failure('MANUAL_REQUIRED', 'Login requires a password or unsupported verification method')
          else if (consent) { phase = 'CONSENT_FAILED'; await consent.click(); consentSubmitted = true; lastActionAt = Date.now() }
          else {
            if (Date.now() - lastActionAt > 20000 && !submittedCode) return failure(submittedEmail ? 'CODE_PAGE_NOT_REACHED' : 'LOGIN_PAGE_UNSUPPORTED', 'Login did not reach a supported email-code step; check the page or authorize manually', true)
            await page.waitForTimeout(250)
          }
        }
      }
      if (signal?.aborted) return interrupted(signal)
      return failure(!submittedEmail ? 'LOGIN_PAGE_UNSUPPORTED' : !submittedCode ? 'CODE_PAGE_NOT_REACHED' : 'AUTH_TIMEOUT', 'Authorization did not reach the next login step within its deadline', true)
    } finally { signal?.removeEventListener('abort', abort) }
  } catch {
    return signal?.aborted ? interrupted(signal) : failure(phase, 'Browser login failed at this step; check proxy, page changes or manual verification', true)
  } finally { await browser?.close().catch(() => {}) }
}
