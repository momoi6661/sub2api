import http from 'node:http'
import { timingSafeEqual } from 'node:crypto'
import { executeScript, authorizeWithBrowser, failure } from './executor.mjs'

const token = process.env.REAUTH_RUNNER_TOKEN || ''
if (token.length < 32) throw new Error('REAUTH_RUNNER_TOKEN must contain at least 32 characters')
let active = 0
const server = http.createServer(async (request, response) => {
  const supplied = Buffer.from((request.headers.authorization || '').replace(/^Bearer /, ''))
  const expected = Buffer.from(token)
  if (supplied.length !== expected.length || !timingSafeEqual(supplied, expected)) { response.writeHead(401); response.end(); return }
  response.setHeader('Content-Type', 'application/json'); response.setHeader('Cache-Control', 'no-store')
  if (request.url === '/health') { response.end(JSON.stringify({ ready: true })); return }
  if (request.url !== '/execute' || request.method !== 'POST') { response.writeHead(404); response.end(); return }
  if (active >= 2) { response.writeHead(429); response.end(JSON.stringify(failure('RUNNER_BUSY', 'Runner is busy; retry later', true))); return }
  const controller = new AbortController()
  response.on('close', () => { if (!response.writableEnded) controller.abort() })
  const timer = setTimeout(() => controller.abort(), 120000)
  active++
  try {
    let body = ''
    for await (const chunk of request) { body += chunk; if (body.length > 65536) throw new Error('size') }
    const task = JSON.parse(body)
    if (typeof task.script !== 'string' || task.script.length > 16384 || !['code', 'auth_file'].includes(task.resultType) || !task.context?.loginEmail) throw new Error('task')
    if (task.operation) throw new Error('operation')
    if (task.mailbox) {
      const m = task.mailbox
      if (task.mode !== 'browser' || task.resultType !== 'code' || typeof m.host !== 'string' || !m.host || /[\s/:@]/.test(m.host) || !Number.isInteger(m.port) || m.port < 1 || m.port > 65535 || typeof m.codePattern !== 'string' || m.codePattern.length > 500 || !task.context.secret) throw new Error('mailbox')
    } else {
      const api = new URL(task.context.apiURL)
      if (api.protocol !== 'https:') throw new Error('api')
    }
    let result
    if (task.mode === 'browser') {
      const auth = new URL(task.authURL)
      if (auth.protocol !== 'https:' || auth.hostname !== 'auth.openai.com') throw new Error('auth')
      if (task.redirectURI !== 'http://localhost:1455/auth/callback') throw new Error('callback')
      const { chromium } = await import('playwright')
      result = await authorizeWithBrowser(task, chromium, controller.signal)
    } else if (task.mode === 'protocol') result = await executeScript(task, { signal: controller.signal })
    else throw new Error('mode')
    response.end(JSON.stringify(result))
  } catch { response.writeHead(400); response.end(JSON.stringify(failure('INVALID_RUNNER_TASK', 'Invalid execution task or unavailable Playwright installation'))) }
  finally { active--; clearTimeout(timer) }
})
server.requestTimeout = 125000
server.listen(Number(process.env.PORT || 8189), process.env.BIND_HOST || '127.0.0.1')
