# One-click authorization runner

All one-click authorization uses protocols; no Playwright, Chromium or Xvfb is
installed. Existing IMAP and API code profiles read verification codes, and an
isolated HTTP cookie session performs email login, OAuth consent and PKCE token
exchange. The runner returns one Codex authorization JSON file. Custom `auth_file`
scripts continue to return files directly without starting a login session.

Only existing accounts are authorized. `/about-you`, signup and profile creation
return `ACCOUNT_NOT_REGISTERED` without submitting registration data. CAPTCHA and
Sentinel/security challenges return `PROVIDER_SECURITY_CHALLENGE`, preserving the
HTTP status when available. Phone verification, passkeys, MFA and ambiguous
workspace selection return `MANUAL_REQUIRED`. Password pages (including redirected
pages) use `/api/accounts/passwordless/send-otp`, then mailbox code reading and
`/api/accounts/email-otp/validate`. A return to the password page after sending
returns `EMAIL_CODE_UNAVAILABLE`. No account password is submitted. No challenge solving,
fingerprint impersonation or automatic retries are included.

The readable protocol reference is `Wenfxl-Codex-Manager-v18.2.0`:
`utils/auth_pipeline/register.py::run_oauth_only`, `oauth.py`, and
`utils/email_providers/mail_service.py::get_oai_code`. Its mailbox readers use
IMAP or HTTP APIs, not Playwright. Its `auth_core` is a compiled extension;
`init_auth`/`generate_payload` are not included in this client. This is not a complete
port of the reference's HTTP transport and compiled module. Provider security
requirements may prevent this client from reaching even the initial login step;
email-code support does not resolve an authorization-entry security challenge.

Run this optional service separately from Sub2API. Set `REAUTH_RUNNER_TOKEN` to a
random value of at least 32 characters. Configure Sub2API with
`ONE_CLICK_AUTH_RUNNER_URL=http://reauth-runner:8189` and
`ONE_CLICK_AUTH_RUNNER_TOKEN` using the same token. Do not expose the runner port
publicly or mount Sub2API data/credentials into it. Apply container resource
limits, a read-only filesystem and a temporary directory.

Scripts are trusted administrator code. A separate child process and sidecar
protect panel state; this is not a JavaScript security sandbox for hostile code.
Only install code you trust. The child receives no runner token environment.
Do not print or include tokens in error messages; results are not logged.

Return exactly one object:

```js
return { type: 'code', code: '123456' };
return { type: 'auth_file', auth: authJson };
return { type: 'error', error: {
  code: 'MAILBOX_TIMEOUT', message: 'No new code', retryable: true
}};
```

The `context` argument contains `accountId`, `loginEmail`, `since`, `apiURL`,
`mailboxAddress`, and `secret`. Filter a shared mailbox by login recipient and
task start time. Scripts execute once per task and have a 30-second deadline;
protocol authorization has a 120-second deadline. Errors and timeouts terminate
the task. `retryable` informs the operator; it does not trigger automatic retries.

For a native install, run `npm ci` and `npm start`. Node 24 is recommended.
Run `npm test` for local protocol-flow tests; they do not contact the provider.

Authorization profiles can hold a proxy pool. The backend chooses one proxy per
account task and keeps it for protocol login, script fetch, and token exchange.
Script fetch uses the selected HTTP, HTTPS, or SOCKS proxy; custom networking
outside fetch is the script author's responsibility. Empty pools use the account
proxy. A proxy failure ends this task instead of silently switching routes.

IMAP profiles use TLS and a read-only mailbox. Before login, the runner captures
UIDNEXT and UIDVALIDITY. It only reads new UIDs after that snapshot, checks the
server delivery time against task start, and matches To/Cc/Delivered-To/X-Original-To
against the account login email. Optional sender filtering is an exact email
address. The selected OAuth proxy does not route the IMAP mailbox connection.
Each read has a 30-second deadline and does not mark or delete messages.

Automatic execution is off by default. The panel checks authentication failures
once per minute and persists an attempt fingerprint before execution. Each failed
credential state is attempted once, including across restarts. Errors do not
create automatic retry loops. Changing the binding or toggling automatic execution
resets the attempt marker; the visible account button always permits manual retry.
Redis locks coordinate automatic and manual tasks across panel instances.

From `deploy/`, set a fixed `TOTP_ENCRYPTION_KEY` and a random `REAUTH_RUNNER_TOKEN`
in the deployment .env, then add `-f docker-compose.reauth.yml` to the selected
panel compose command. The overlay builds the protocol runner and connects it on
the compose network without publishing its port. Use the modified panel image,
not an upstream image without these endpoints. Example:

```sh
docker compose -f docker-compose.dev.yml -f docker-compose.reauth.yml up -d --build
```

Rebuild both the panel and runner when upgrading from the browser runner: code
profiles now send `mode=oauth_protocol` and receive `auth_file`, while direct
file scripts retain `mode=protocol`. Old browser history remains readable.
