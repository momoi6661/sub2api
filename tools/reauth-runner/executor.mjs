import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'

export const failure = (code, message, retryable = false) => ({ type: 'error', error: { code, message, retryable } })

export function executeScript(task, { timeout = 30_000, signal } = {}) {
  return new Promise(resolve => {
    if (signal?.aborted) return resolve(failure('TASK_CANCELLED', 'Task cancelled'))
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
    const abort = () => finish(failure('TASK_CANCELLED', 'Task cancelled'))
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
export async function authorizeWithBrowser(task, chromium, signal) {
  let browser
  try {
    if (signal?.aborted) return failure('TASK_CANCELLED', 'Task cancelled')
    if (task.mailbox) {
      const snapshot = await executeScript({ ...task, operation: 'mailbox_snapshot' }, { timeout: 10000, signal })
      if (snapshot.type === 'error') return snapshot
      if (snapshot.type !== 'mailbox_snapshot' || !snapshot.uidNext || !snapshot.uidValidity) return failure('MAILBOX_SNAPSHOT_FAILED', 'Cannot record mailbox state')
      task = { ...task, mailbox: { ...task.mailbox, minUID: snapshot.uidNext, uidValidity: snapshot.uidValidity } }
    }
    const proxy = task.proxyURL ? new URL(task.proxyURL) : null
    browser = await chromium.launch({ headless: true })
    if (signal?.aborted) return failure('TASK_CANCELLED', 'Task cancelled')
    const context = await browser.newContext(proxy ? { proxy: {
      server: `${proxy.protocol}//${proxy.host}`, username: decodeURIComponent(proxy.username), password: decodeURIComponent(proxy.password)
    } } : {})
    const page = await context.newPage()
    page.setDefaultTimeout(8000)
    let callback = null
    await page.route(`${task.redirectURI}*`, async route => {
      const url = new URL(route.request().url())
      callback = { type: 'oauth_callback', code: url.searchParams.get('code'), state: url.searchParams.get('state'), error: url.searchParams.get('error') }
      await route.fulfill({ contentType: 'text/plain', body: 'Authorization callback received.' })
    })
    const abort = () => { void browser.close().catch(() => {}) }
    signal?.addEventListener('abort', abort, { once: true })
    try {
      await page.goto(task.authURL, { waitUntil: 'domcontentloaded', timeout: 30000 })
      let submittedEmail = false, submittedCode = false, requestedCode = false, consentSubmitted = false
      const deadline = Date.now() + 100000
      while (Date.now() < deadline && !signal?.aborted) {
        if (callback) {
          if (callback.error || !callback.code || callback.state !== task.state) return failure('OAUTH_CALLBACK_INVALID', 'OAuth callback failed or state did not match')
          return { type: 'oauth_callback', code: callback.code, state: callback.state }
        }
        const challenge = page.locator('iframe[src*="captcha"], iframe[src*="challenges.cloudflare.com"], input[autocomplete="webauthn"]')
        if (await challenge.first().isVisible()) return failure('MANUAL_REQUIRED', 'Login requires CAPTCHA or passkey verification')
        const alert = page.getByRole('alert').first()
        if (await alert.isVisible() && /invalid|incorrect|expired|could not|try again|无效|错误|已过期/i.test(await alert.innerText())) return failure('AUTH_REJECTED', 'Login rejected the verification code or authorization request')
        const email = page.locator('input[type="email"], input[name="email"], input[name="username"]').first()
        const otp = page.locator('input[autocomplete="one-time-code"], input[name="code"], input[name="otp"]').first()
        const submit = page.getByRole('button', { name: /^(continue|submit|verify|allow|authorize|继续|提交|验证|授权)$/i }).first()
        if (!submittedEmail && await email.isVisible()) {
          await email.fill(task.context.loginEmail)
          await submit.click(); submittedEmail = true
        } else if (!submittedCode && await otp.isVisible()) {
          // Execute the configured script exactly once for this account/task.
          const result = await executeScript({ ...task, resultType: 'code' }, { signal })
          if (result.type === 'error') return result
          await otp.fill(result.code); await submit.click(); submittedCode = true
        } else {
          const sendCode = page.getByRole('button', { name: /send.*code|use.*code|email.*code|log in with.*code|发送.*验证码|使用.*验证码/i }).or(page.getByRole('link', { name: /send.*code|use.*code|email.*code|log in with.*code|发送.*验证码|使用.*验证码/i })).first()
          if (!requestedCode && await sendCode.isVisible()) { await sendCode.click(); requestedCode = true }
          else if (await page.locator('input[type="password"]').first().isVisible()) return failure('MANUAL_REQUIRED', 'Login requires a password or unsupported verification method')
          else if (submittedCode && !consentSubmitted && await submit.isVisible()) { await submit.click(); consentSubmitted = true }
          else await page.waitForTimeout(250)
        }
      }
      return failure(signal?.aborted ? 'TASK_CANCELLED' : 'AUTH_TIMEOUT', 'Authorization did not complete within its deadline', !signal?.aborted)
    } finally { signal?.removeEventListener('abort', abort) }
  } catch {
    return failure(signal?.aborted ? 'TASK_CANCELLED' : 'BROWSER_AUTH_FAILED', 'Browser login failed; check proxy, page changes or manual verification', !signal?.aborted)
  } finally { await browser?.close().catch(() => {}) }
}
