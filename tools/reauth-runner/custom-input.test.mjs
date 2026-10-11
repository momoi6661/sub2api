import test from 'node:test'
import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
for (const input of ['', '42', '{"param":"example"}', '  plain text  ']) {
  test('custom input is passed unchanged: ' + JSON.stringify(input), () => {
    const child = spawnSync(process.execPath, [new URL('./script-process.mjs', import.meta.url).pathname.replace(/^\/(\w:)/, '$1')], {
      input: JSON.stringify({ mode: 'protocol', resultType: 'auth_file', input, context: {}, script: 'return {type:"auth_file",auth:{received:input}}' }), encoding:'utf8'
    })
    assert.equal(child.status, 0, child.stderr)
    assert.deepEqual(JSON.parse(JSON.parse(child.stdout).auth), {received:input})
  })
}
