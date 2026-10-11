import test from 'node:test'
import assert from 'node:assert/strict'
import net from 'node:net'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { setTimeout } from 'node:timers/promises'

test('protocol sidecar starts, authenticates, streams script results and rejects removed browser mode', async () => {
  const reservation = net.createServer()
  reservation.listen(0, '127.0.0.1')
  await once(reservation, 'listening')
  const port = reservation.address().port
  await new Promise(resolve => reservation.close(resolve))
  const token = '01234567890123456789012345678901'
  const child = spawn(process.execPath, ['server.mjs'], { cwd: new URL('.', import.meta.url),
    env: { ...process.env, BIND_HOST: '127.0.0.1', PORT: String(port), REAUTH_RUNNER_TOKEN: token }, stdio: ['ignore', 'ignore', 'pipe'] })
  let stderr = ''
  child.stderr.on('data', chunk => { stderr += chunk })
  const endpoint = `http://127.0.0.1:${port}`
  const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json', Accept: 'application/x-ndjson' }
  try {
    let healthy = false
    for (let attempt = 0; attempt < 60; attempt++) {
      if (child.exitCode !== null) assert.fail(`Runner exited: ${stderr}`)
      try {
        const response = await fetch(endpoint + '/health', { headers, signal: AbortSignal.timeout(1000) })
        healthy = response.ok && (await response.json()).ready
        if (healthy) break
      } catch { /* Wait for the isolated service to bind its port. */ }
      await setTimeout(100)
    }
    assert.equal(healthy, true, 'sidecar must become ready')
    assert.equal((await fetch(endpoint + '/health')).status, 401)
    const task = { mode: 'protocol', resultType: 'auth_file', context: { loginEmail: 'owner@example.com' },
      script: "return { type: 'auth_file', auth: { type: 'codex', access_token: 'test-access' } }" }
    const result = await fetch(endpoint + '/execute', { method: 'POST', headers, body: JSON.stringify(task) })
    assert.equal(result.status, 200)
    assert.equal(result.headers.get('content-type'), 'application/x-ndjson')
    const line = JSON.parse((await result.text()).trim())
    assert.equal(line.type, 'auth_file')
    assert.equal(JSON.parse(line.auth).access_token, 'test-access')
    assert.deepEqual(line.script_result, { type: 'auth_file', auth: { type: 'codex', access_token: 'test-access' } })
    const custom = { ...task, script: "return { type: 'auth_file', auth: JSON.stringify({email:'owner@example.com',access_token:'test'}), extra: { nested: [1,2] } }" }
    const customResponse = await fetch(endpoint + '/execute', { method: 'POST', headers, body: JSON.stringify(custom) })
    const customResult = JSON.parse((await customResponse.text()).trim())
    assert.deepEqual(customResult.script_result, { type: 'auth_file', auth: JSON.stringify({email:'owner@example.com',access_token:'test'}), extra: {nested:[1,2]} })
    const failureResponse = await fetch(endpoint + '/execute', { method: 'POST', headers, body: JSON.stringify({ ...task, script: "return { type:'error', error:{code:'CUSTOM_ERROR',message:'Test failure',retryable:false}, detail:'custom detail' }" }) })
    const failedResult = JSON.parse((await failureResponse.text()).trim())
    assert.equal(failedResult.script_result.detail, 'custom detail')
    const invalid = await fetch(endpoint + '/execute', { method: 'POST', headers, body: JSON.stringify({ ...task, mode: 'browser', resultType: 'code' }) })
    assert.equal(invalid.status, 400)
    assert.equal((await invalid.json()).error.code, 'INVALID_RUNNER_TASK')
  } finally {
    const exited = once(child, 'exit')
    child.kill()
    if (child.exitCode === null && child.signalCode === null) await exited
  }
})
