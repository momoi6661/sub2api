import { randomBytes, createHash } from 'node:crypto'
import { CookieJar } from 'tough-cookie'
import { createCurlCffiFetch } from './curl-cffi-fetch.mjs'

const origin = 'https://auth.openai.com'
const redirectURI = 'http://localhost:1455/auth/callback'
const clientID = 'app_EMoamEEZ73f0CkXaXp7hrann'
const failure = (code, message, retryable = false) => ({ type: 'error', error: { code, message, retryable } })
const reject = (code, message, retryable = false) => { throw Object.assign(new Error(message), { result: failure(code, message, retryable) }) }
const claims = token => {
  try { return JSON.parse(Buffer.from(token.split('.')[1], 'base64url').toString()) } catch { return {} }
}

// Only existing-account login is supported. Never enter profile creation/signup.
function checkStep(url, pageType = '') {
  if (/about-you|sign-up|signup|register|create_account/.test(url.pathname + ' ' + pageType)) {
    reject('ACCOUNT_NOT_REGISTERED', 'The provider requires account registration or profile setup; this task only authorizes existing accounts')
  }
  if (/captcha|challenge|sentinel/.test(url.pathname + ' ' + pageType)) {
    reject('PROVIDER_SECURITY_CHALLENGE', 'The authorization provider requires a security challenge')
  }
  if (/add-phone|phone[_/-]|select-channel|mfa|totp|passkey/.test(url.pathname + ' ' + pageType)) {
    reject('MANUAL_REQUIRED', 'The provider requires additional verification; authorize this account manually')
  }
}

function nextURL(data) {
  if (typeof data.continue_url === 'string' && data.continue_url) return data.continue_url
  return {
    email_otp_verification: '/email-verification',
    sign_in_with_chatgpt_codex_consent: '/sign-in-with-chatgpt/codex/consent',
    workspace: '/workspace',
    password: '/log-in/password'
  }[data.page?.type] || ''
}

// The state and verifier are task-local; cookies never survive this function.
export async function authorizeWithProtocol(task, executeScript, signal, report = () => {}, fetchOverride) {
  let transport, phase = 'PROTOCOL_CONNECTION_FAILED'
  try {
    signal?.throwIfAborted()
    if (task.mailbox) {
      report('mailbox_connect')
      const snapshot = await executeScript({ ...task, operation: 'mailbox_snapshot' }, { timeout: 10000, signal })
      if (snapshot.type === 'error') return snapshot
      if (snapshot.type !== 'mailbox_snapshot' || !snapshot.uidNext || !snapshot.uidValidity) {
        return failure('MAILBOX_SNAPSHOT_FAILED', 'Cannot record mailbox state')
      }
      task = { ...task, mailbox: { ...task.mailbox, minUID: snapshot.uidNext, uidValidity: snapshot.uidValidity } }
    }
    // The reference GPT flow uses curl_cffi's Chrome TLS profile for the
    // provider session. Keep the Node proxy transport for scripts, but use the
    // protocol transport for OAuth/OTP requests.
    transport = fetchOverride ? null : createCurlCffiFetch(task.proxyURL)
    const fetch = fetchOverride || transport.fetch
    const jar = new CookieJar()
    const state = randomBytes(24).toString('base64url')
    const verifier = randomBytes(48).toString('base64url')
    const authorize = new URL('/oauth/authorize', origin)
    authorize.search = new URLSearchParams({
      client_id: clientID, response_type: 'code', redirect_uri: redirectURI,
      scope: 'openid profile email offline_access', state,
      code_challenge: createHash('sha256').update(verifier).digest('base64url'),
      code_challenge_method: 'S256', id_token_add_organizations: 'true', codex_cli_simplified_flow: 'true'
    }).toString()

    const request = async (target, options = {}) => {
      signal?.throwIfAborted()
      const url = new URL(target, origin)
      if (url.origin !== origin || url.username || url.password) reject('OAUTH_REDIRECT_INVALID', 'Authorization redirected outside the supported provider')
      checkStep(url)
      const cookies = await jar.getCookieString(url.href)
      const deviceID = (await jar.getCookies(url.href)).find(cookie => cookie.key === 'oai-did')?.value
      const response = await fetch(url.href, {
        ...options, redirect: 'manual', size: 1024 * 1024,
        signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(30000)]) : AbortSignal.timeout(30000),
        headers: { Accept: options.method === 'POST' ? 'application/json' : 'text/html,application/xhtml+xml', 'Accept-Language': 'en-US,en;q=0.9',
            'Sec-Fetch-Dest': options.method === 'POST' ? 'empty' : 'document',
            'Sec-Fetch-Mode': options.method === 'POST' ? 'cors' : 'navigate',
            'Sec-Fetch-Site': 'same-origin',
            ...(options.method === 'GET' ? { 'Upgrade-Insecure-Requests': '1' } : {}),
          ...(options.method === 'POST' && deviceID ? { 'oai-device-id': deviceID } : {}),
          ...(cookies ? { Cookie: cookies } : {}), ...options.headers }
      })
      for (const cookie of response.headers.raw()['set-cookie'] || []) await jar.setCookie(cookie, url.href)
      const text = await response.text()
      if (response.headers.get('cf-mitigated') === 'challenge' || /just a moment|checking your browser|verify you are human|challenges\.cloudflare\.com/i.test(text)) {
        const result = failure('PROVIDER_SECURITY_CHALLENGE', 'The authorization provider returned a security challenge; email-code login has not completed')
        result.error.httpStatus = response.status
        throw Object.assign(new Error(result.error.message), { result })
      }
      let data = {}
      try { data = JSON.parse(text) } catch { /* Login landing pages are HTML. */ }
      if (!data || typeof data !== 'object' || Array.isArray(data)) reject('PROTOCOL_STEP_UNSUPPORTED', 'The provider returned an invalid authorization response')
      if (/captcha|challenge|sentinel/i.test(String(data.error?.code || ''))) {
        const result = failure('PROVIDER_SECURITY_CHALLENGE', 'The authorization provider requires a security challenge')
        result.error.httpStatus = response.status
        throw Object.assign(new Error(result.error.message), { result })
      }
      if (/mfa_required|phone_verification/i.test(String(data.error?.code || ''))) {
        reject('MANUAL_REQUIRED', 'Authorization requires a provider security challenge; authorize manually')
      }
      const pageType = typeof data.page?.type === 'string' ? data.page.type : ''
      checkStep(url, pageType)
      if (!response.ok && ![301, 302, 303, 307, 308].includes(response.status)) {
        const result = failure(phase, `Authorization request failed (HTTP ${response.status})`, response.status === 429 || response.status >= 500)
        result.error.httpStatus = response.status
        throw Object.assign(new Error(result.error.message), { result })
      }
      return { url, response, data }
    }
    const follow = async target => {
      let url = new URL(target, origin)
      for (let count = 0; count < 12; count++) {
        signal?.throwIfAborted()
        if (url.origin === new URL(redirectURI).origin && url.pathname === new URL(redirectURI).pathname && !url.username && !url.password) {
          if (url.searchParams.get('error') || !url.searchParams.get('code') || url.searchParams.get('state') !== state) {
            reject('OAUTH_CALLBACK_INVALID', 'OAuth callback code or state is invalid')
          }
          return { url, code: url.searchParams.get('code') }
        }
        const step = await request(url.href)
        if (![301, 302, 303, 307, 308].includes(step.response.status)) return step
        const location = step.response.headers.get('location')
        if (!location) reject('OAUTH_REDIRECT_INVALID', 'Authorization redirect is missing its destination')
        url = new URL(location, url)
      }
      reject('OAUTH_REDIRECT_INVALID', 'Authorization exceeded the redirect limit')
    }
    const post = async (path, data, referer) => request(path, {
      method: 'POST', headers: { 'Content-Type': 'application/json', Origin: origin, Referer: referer.href }, body: JSON.stringify(data)
    })

    report('protocol_start')
    let step = await follow(authorize)
    if (!step.code) {
      phase = 'EMAIL_SUBMIT_FAILED'
      report('email_login')
      step = await post('/api/accounts/authorize/continue', { username: { value: task.context.loginEmail, kind: 'email' }, screen_hint: 'login' }, step.url)
      let target = nextURL(step.data)
      if (!target) reject('PROTOCOL_STEP_UNSUPPORTED', 'The provider did not return a supported login step')
      checkStep(new URL(target, origin), step.data.page?.type || '')
      phase = 'CODE_REQUEST_FAILED'
      report('requesting_code')
      // Open the returned step first so redirect destinations and cookies apply.
      step = await follow(target)
      // Existing password accounts may offer email-code login instead. The
      // destination can be a password page even when the API returned /log-in.
      if (!step.code && /password/.test(step.url.pathname)) {
        step = await post('/api/accounts/passwordless/send-otp', {}, step.url)
        target = nextURL(step.data)
        // Like the reference, accept a successful send with no response payload.
        if (!target && Object.keys(step.data).length === 0) target = '/email-verification'
        if (!target) reject('PROTOCOL_STEP_UNSUPPORTED', 'The provider did not confirm email-code login')
        step = await follow(target)
        if (!step.code && /password/.test(step.url.pathname)) reject('EMAIL_CODE_UNAVAILABLE', 'The provider did not offer email-code login for this account')
      }
      if (!step.code && step.url.pathname === '/email-verification') {
        report('waiting_code')
        const readerController = new AbortController()
        const readerSignal = signal ? AbortSignal.any([signal, readerController.signal]) : readerController.signal
        let resendTimer
        let result
        try {
          // Keep the original mailbox snapshot and reader alive during resend.
          const reading = executeScript({ ...task, resultType: 'code' }, {
            signal: readerSignal, ...(task.mailbox ? { timeout: 190000 } : {})
          })
          const resendDue = new Promise(resolve => { resendTimer = setTimeout(() => resolve(null), 30000) })
          result = await Promise.race([reading, resendDue])
          if (result === null) {
            signal?.throwIfAborted()
            phase = 'CODE_REQUEST_FAILED'
            report('requesting_code')
            const resent = await post('/api/accounts/email-otp/resend', {}, step.url)
            const destination = nextURL(resent.data) || (resent.data.success === true || Object.keys(resent.data).length === 0 ? '/email-verification' : '')
            if (!destination) reject('EMAIL_CODE_UNAVAILABLE', 'The provider did not confirm the resend request')
            step = await follow(destination)
            if (step.url.pathname !== '/email-verification') reject('EMAIL_CODE_UNAVAILABLE', 'The provider did not return to email verification after resend')
            report('waiting_code')
            result = await reading
          }
        } finally {
          clearTimeout(resendTimer)
          readerController.abort()
        }
        if (result.type === 'error') return result
        if (result.type !== 'code' || typeof result.code !== 'string' || !result.code.trim()) reject('INVALID_SCRIPT_RESULT', 'Mailbox reader must return a verification code')
        signal?.throwIfAborted()
        phase = 'CODE_SUBMIT_FAILED'
        report('submitting_code')
        const validated = await post('/api/accounts/email-otp/validate', { code: result.code }, step.url)
        target = nextURL(validated.data)
        if (!target) reject('PROTOCOL_STEP_UNSUPPORTED', 'The provider did not return a step after code verification')
        step = await follow(target)
      }
    }

    if (!step.code) {
      report('consent')
      phase = 'CONSENT_FAILED'
      if (!/\/(consent|workspace)$/.test(step.url.pathname)) reject('PROTOCOL_STEP_UNSUPPORTED', 'The provider returned an unsupported authorization step')
      const cookie = (await jar.getCookies(origin)).find(item => item.key === 'oai-client-auth-session')?.value || ''
      const parts = cookie.split('.')
      let workspaces = []
      for (const part of [parts[1], parts[0]]) {
        try {
          const value = JSON.parse(Buffer.from(part || '', 'base64url').toString())
          if (Array.isArray(value.workspaces) && value.workspaces.length) { workspaces = value.workspaces; break }
        } catch { /* Cookie format is provider-owned. */ }
      }
      const expectedID = task.context.expectedAccountID
      const workspace = expectedID ? workspaces.find(item => item.id === expectedID)
        : workspaces.length === 1 ? workspaces[0] : workspaces.find(item => item.is_personal === true)
      if (!workspace?.id) reject('MANUAL_REQUIRED', 'Workspace selection requires manual authorization')
      const selected = await post('/api/accounts/workspace/select', { workspace_id: workspace.id }, step.url)
      const target = nextURL(selected.data)
      if (!target) reject('PROTOCOL_STEP_UNSUPPORTED', 'The provider did not return an authorization callback')
      step = await follow(target)
      if (!step.code) reject('PROTOCOL_STEP_UNSUPPORTED', 'The provider did not complete authorization after workspace selection')
    }

    report('token_exchange')
    phase = 'TOKEN_EXCHANGE_FAILED'
    const { data } = await request('/oauth/token', {
      method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ grant_type: 'authorization_code', client_id: clientID, code: step.code, redirect_uri: redirectURI, code_verifier: verifier }).toString()
    })
    if (typeof data.access_token !== 'string' || !data.access_token || typeof data.refresh_token !== 'string' || !data.refresh_token || typeof data.id_token !== 'string' || !data.id_token || !Number.isFinite(Number(data.expires_in)) || Number(data.expires_in) <= 0) {
      reject('TOKEN_EXCHANGE_FAILED', 'The provider returned incomplete OAuth credentials')
    }
    const identity = claims(data.id_token)
    const accountID = identity['https://api.openai.com/auth']?.chatgpt_account_id
    if (typeof identity.email !== 'string' || identity.email.toLowerCase() !== task.context.loginEmail.toLowerCase() || !accountID || (task.context.expectedAccountID && accountID !== task.context.expectedAccountID)) {
      reject('IDENTITY_MISMATCH', 'Authorized identity does not match the requested account')
    }
    signal?.throwIfAborted()
    const now = Date.now()
    return { type: 'auth_file', auth: JSON.stringify({ type: 'codex', client_id: clientID,
      access_token: data.access_token, refresh_token: data.refresh_token, id_token: data.id_token,
      email: identity.email, account_id: accountID, last_refresh: new Date(now).toISOString(),
      expires_at: new Date(now + Number(data.expires_in) * 1000).toISOString(),
      expired: new Date(now + Number(data.expires_in) * 1000).toISOString() }) }
  } catch (error) {
    if (signal?.aborted || error?.name === 'TimeoutError') {
      return signal?.reason?.name === 'TimeoutError' || error?.name === 'TimeoutError'
        ? failure('AUTH_TIMEOUT', 'Authorization exceeded its deadline', true) : failure('TASK_CANCELLED', 'Task cancelled')
    }
    // Never include provider response bodies, callback URLs or cookies in errors.
    return error.result || failure(phase, 'Protocol authorization failed at this step; check the proxy and provider response', true)
  } finally { transport?.close() }
}
