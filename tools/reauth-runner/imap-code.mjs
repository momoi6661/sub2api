import { ImapFlow } from 'imapflow'
import { simpleParser } from 'mailparser'

const error = (code, message, retryable = false) => ({ type: 'error', error: { code, message, retryable } })
const addresses = value => (value?.value || []).flatMap(item => item.group || [item]).map(item => (item.address || '').toLowerCase())
// IMAP INTERNALDATE has second precision; the UID snapshot excludes older mail.
const deliveryStart = task => Math.floor(Date.parse(task.context.since) / 1000) * 1000

// Match actual delivery headers as well as To/Cc, including forwarded aliases.
export function matchMailboxMessage(parsed, internalDate, task) {
  // iCloud Hide My Email can forward a message to Gmail while rewriting the
  // visible recipient headers. Accept either the login alias or the mailbox
  // that actually receives the forwarded message.
  const recipients = new Set([
    task.context.loginEmail,
    task.context.mailboxAddress,
  ].filter(value => typeof value === 'string' && value.trim()).map(value => value.trim().toLowerCase()))
  const delivered = ['delivered-to', 'x-original-to', 'envelope-to'].flatMap(key => {
    const raw = parsed.headers.get(key)
    return typeof raw === 'string' ? raw.match(/[a-z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-z0-9.-]+/gi) || [] : addresses(raw)
  }).map(value => value.toLowerCase())
  if (![...addresses(parsed.to), ...addresses(parsed.cc), ...delivered].some(value => recipients.has(value))) return null
  if (!(internalDate instanceof Date) || !Number.isFinite(internalDate.getTime()) || !Number.isFinite(deliveryStart(task)) || internalDate.getTime() < deliveryStart(task)) return null
  if (task.mailbox.senderFilter && !addresses(parsed.from).includes(task.mailbox.senderFilter.toLowerCase())) return null
  const body = `${parsed.subject || ''}\n${parsed.text || ''}`.slice(0, 100000)
  const match = new RegExp(task.mailbox.codePattern).exec(body)
  return match?.[1]?.trim() || null
}

export async function executeMailbox(task) {
  const config = task.mailbox
  const client = new ImapFlow({
    host: config.host, port: config.port, secure: true,
    auth: { user: config.username || task.context.mailboxAddress, pass: task.context.secret },
    logger: false, connectionTimeout: 8000, greetingTimeout: 8000, socketTimeout: 8000
  })
  client.on('error', () => {})
  try {
    await client.connect()
    await client.mailboxOpen(config.folder || 'INBOX', { readOnly: true })
    if (task.operation === 'mailbox_snapshot') return { type: 'mailbox_snapshot', uidNext: client.mailbox.uidNext, uidValidity: String(client.mailbox.uidValidity) }
    if (String(client.mailbox.uidValidity) !== config.uidValidity) return error('MAILBOX_CHANGED', 'Mailbox was reset during this authorization task')
    const configuredSeconds = Number.parseInt(process.env.MAILBOX_WAIT_SECONDS || '60', 10)
    const waitSeconds = Number.isFinite(configuredSeconds) ? Math.min(180, Math.max(30, configuredSeconds)) : 60
    const deadline = Date.now() + waitSeconds * 1000
    while (Date.now() < deadline) {
      const uids = await client.search({ uid: `${config.minUID}:*`, since: new Date(task.context.since) }, { uid: true })
      const fresh = (uids || []).filter(uid => uid >= config.minUID).slice(-50).reverse()
      if (fresh.length) {
        const messages = await client.fetchAll(fresh, { internalDate: true, size: true, uid: true }, { uid: true })
        for (const message of messages.sort((a,b) => b.uid-a.uid)) {
          if (message.size > 1024*1024 || message.internalDate?.getTime() < deliveryStart(task)) continue
          const fetched = await client.fetchOne(message.uid, { source: true }, { uid: true })
          if (!fetched?.source || fetched.source.length > 1024*1024) continue
          const parsed = await simpleParser(fetched.source, { skipHtmlToText: false, skipTextToHtml: true })
          const code = matchMailboxMessage(parsed, message.internalDate, task)
          if (code) return { type: 'code', code }
        }
      }
      await new Promise(resolve => setTimeout(resolve, 1000))
      await client.noop()
    }
    return error('MAILBOX_TIMEOUT', 'No new verification email matching this account was received', true)
  } catch {
    return error('MAILBOX_CONNECTION_FAILED', 'Mailbox login or reading failed; check host, password and folder', true)
  } finally { client.close() }
}
