// Runs in a separate sidecar child process, never in the Sub2API process.
let input = ''
for await (const chunk of process.stdin) input += chunk
const failure = (code, message, retryable = false) => ({ type: 'error', error: { code: String(code).slice(0, 80), message: String(message).slice(0, 1000), retryable: retryable === true } })
let result
try {
  const task = JSON.parse(input)
  if (task.proxyURL) {
    const { default: fetch } = await import('node-fetch')
    const proxy = new URL(task.proxyURL)
    let agent
    if (['socks:', 'socks4:', 'socks5:', 'socks4a:', 'socks5h:'].includes(proxy.protocol)) {
      const { SocksProxyAgent } = await import('socks-proxy-agent')
      agent = new SocksProxyAgent(task.proxyURL)
    } else if (['http:', 'https:'].includes(proxy.protocol)) {
      const { HttpProxyAgent } = await import('http-proxy-agent')
      const { HttpsProxyAgent } = await import('https-proxy-agent')
      const http = new HttpProxyAgent(task.proxyURL), https = new HttpsProxyAgent(task.proxyURL)
      agent = target => target.protocol === 'https:' ? https : http
    } else throw new Error('Unsupported authorization proxy protocol')
    globalThis.fetch = (url, options = {}) => fetch(url, { ...options, agent })
  }
  if (task.mailbox) {
    const { executeMailbox } = await import('./imap-code.mjs')
    result = await executeMailbox(task)
  } else {
  const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor
  // No console output may become a credential-bearing process log.
  console.log = console.error = console.warn = () => {}
  const value = await new AsyncFunction('context', task.script)(task.context)
  if (value?.type === 'error' && typeof value.error?.code === 'string' && typeof value.error?.message === 'string') {
    result = failure(value.error.code, value.error.message, value.error.retryable)
  } else if (value?.type !== task.resultType) {
    result = failure('RESULT_TYPE_MISMATCH', 'Return an object whose type matches the configured result type')
  } else if (task.resultType === 'code' && typeof value.code === 'string' && value.code.trim() && value.code.length <= 128) {
    result = { type: 'code', code: value.code.trim() }
  } else if (task.resultType === 'auth_file' && value.auth && (typeof value.auth === 'string' || (typeof value.auth === 'object' && !Array.isArray(value.auth)))) {
    const content = typeof value.auth === 'string' ? value.auth : JSON.stringify(value.auth)
    if (content.length > 65536) result = failure('RESULT_TOO_LARGE', 'Authorization file exceeds 64 KB')
    else result = { type: 'auth_file', auth: content }
  } else result = failure('INVALID_SCRIPT_RESULT', 'Return a code string or one OAuth authorization JSON object')
  }
} catch (error) {
  result = failure('SCRIPT_EXCEPTION', error?.message || 'Script failed')
}
process.stdout.write(JSON.stringify(result))
