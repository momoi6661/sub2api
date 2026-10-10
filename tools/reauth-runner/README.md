# One-click authorization runner

API code can return an OAuth file without a browser, using Node's HTTP fetch.
Email-code login uses a fresh Playwright Chromium context per account. No desktop
or preinstalled Chrome is required; the Dockerfile installs headless Chromium.
Password, CAPTCHA, passkey and unsupported MFA return `MANUAL_REQUIRED`.

Run this optional service separately from Sub2API. Set `REAUTH_RUNNER_TOKEN` to a
random value of at least 32 characters. Configure Sub2API with
`ONE_CLICK_AUTH_RUNNER_URL=http://reauth-runner:8189` and
`ONE_CLICK_AUTH_RUNNER_TOKEN` using the same token. Do not expose the runner port
publicly or mount Sub2API data/credentials into it. Apply container resource
limits, a read-only filesystem and a temporary directory for browser profiles.

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
browser authorization has a 120-second deadline. Errors and timeouts terminate
the task. `retryable` informs the operator; it does not trigger automatic retries.

For a native install, run `npm install` and `npx playwright install chromium` (Linux also needs
`npx playwright install-deps chromium`). Node 24 is recommended.

Authorization profiles can hold a proxy pool. The backend chooses one proxy per
account task and keeps it for browser login, script fetch, and token exchange.
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
panel compose command. The overlay builds the browser runner and connects it on
the compose network without publishing its port. Use the modified panel image,
not an upstream image without these endpoints. Example:

```sh
docker compose -f docker-compose.dev.yml -f docker-compose.reauth.yml up -d --build
```

The runner image installs Chromium and its system libraries during image build.
For code requiring a password, CAPTCHA, passkey or unsupported MFA, the task
returns `MANUAL_REQUIRED`; it does not attempt to bypass verification.
