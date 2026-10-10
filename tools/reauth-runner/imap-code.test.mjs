import test from 'node:test'
import assert from 'node:assert/strict'
import { simpleParser } from 'mailparser'
import { matchMailboxMessage } from './imap-code.mjs'

const task = {
  context: { loginEmail: 'owner@example.com', since: '2026-10-10T12:00:00.650Z' },
  mailbox: { codePattern: '\\b(\\d{6})\\b', senderFilter: 'login@example.com' }
}
const email = ({ to = 'owner@example.com', from = 'login@example.com', delivered = '' } = {}) =>
  `From: Login <${from}>\r\nTo: <${to}>\r\n${delivered ? `Delivered-To: ${delivered}\r\n` : ''}Subject: Login verification\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Your code is <b>123456</b></p>`

test('ordinary HTML email yields a code when delivered in the task start second', async () => {
  const parsed = await simpleParser(email(), { skipHtmlToText: false, skipTextToHtml: true })
  assert.equal(matchMailboxMessage(parsed, new Date('2026-10-10T12:00:00Z'), task), '123456')
  assert.equal(matchMailboxMessage(parsed, new Date('2026-10-10T11:59:59Z'), task), null)
})

test('parsed delivery headers support forwarded login addresses', async () => {
  const parsed = await simpleParser(email({ to: 'shared@example.com', delivered: 'owner@example.com' }))
  assert.equal(matchMailboxMessage(parsed, new Date('2026-10-10T12:00:01Z'), task), '123456')
})

test('another recipient or sender never yields a code', async () => {
  for (const options of [{ to: 'other@example.com' }, { from: 'other@example.com' }]) {
    assert.equal(matchMailboxMessage(await simpleParser(email(options)), new Date('2026-10-10T12:00:01Z'), task), null)
  }
})

test('invalid delivery or task timestamps never yield a code', async () => {
  const parsed = await simpleParser(email())
  assert.equal(matchMailboxMessage(parsed, new Date('invalid'), task), null)
  assert.equal(matchMailboxMessage(parsed, new Date(), { ...task, context: { ...task.context, since: 'invalid' } }), null)
})
