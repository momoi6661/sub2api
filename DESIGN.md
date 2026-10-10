# Sub2API authorization configuration

This feature follows the existing Sub2API admin UI. Keep its primary colors,
system Chinese font stack, input/button classes, dark mode, and BaseDialog.
No new visual brand or ornamental animation is introduced.

The profile list manages reusable shared or dedicated receiving mailboxes.
Account settings bind a profile ID and store the account's own login email.
Users must distinguish profile availability, manual authorization, and automatic
recovery. Automatic recovery is opt-in and remains off by default. A persistent
attempt marker prevents repeated execution for the same authentication failure.

Profiles can be created from the sidebar or inline from account settings. Keep
form errors and input visible after failed saves. List responses omit credentials;
an explicit administrator action reveals a saved password through a non-cached
endpoint. Hiding it or closing the view clears the revealed value from memory.
An empty replacement during editing preserves the saved encrypted credential.

Profile management reuses TablePageLayout, DataTable, SearchInput, Pagination,
and Select. Search matches profile names and receiving addresses, with read-method
and availability filters retained in the URL. Account and batch binding share an
always-searchable Select with two-line name/address options and disabled states.
Keep the existing rounded table surface, compact badges, brand focus ring, and
mobile cards. Distinguish an empty library from a search with no matches.

The existing list shortcut remains visible for supported accounts during testing.
It executes the bound profile on the server and reports structured errors.
A proxy pool permits multiple choices. Each account independently selects one
proxy per task, retained for protocol login, script requests and token exchange.
An empty pool falls back to the account proxy. Distribution is not a guarantee
against provider risk checks. Authorization-file scripts use the protocol path;
mailbox-code login uses an isolated HTTP cookie session and task-local PKCE.
Only existing accounts are authorized; registration/profile creation and manual
verification stop the task. Protocol login returns an identity-checked auth file. Script requests receive
context.proxyURL so custom authorization logic can use the chosen route.

Forms collapse to one column on narrow screens; tables use local horizontal
scrolling. Reuse dialog focus management and confirm profile deletion. Include
loading, empty, error, retry and success states, with localized Chinese/English
text. Verify account identity before replacing tokens.

Account creation offers one-click authorization beside the existing OAuth methods.
Select a reusable profile and enter the login email. A bounded draft task returns
identity-checked credentials without creating a placeholder. Successful authorization
proceeds to normal creation with the chosen account options, profile binding and
automatic recovery off. Canceling aborts the draft request; failed creation retains
credentials for retry while the dialog is open. Draft scripts receive accountId 0.
Refreshing the profile list hides any revealed passwords.

The sidebar Authorization tasks page owns all task history. Reuse TablePageLayout,
DataTable, SearchInput, Select, Pagination and BaseDialog, with compact status badges
and existing brand colors. Keep the information density moderate and animation
limited to meaningful busy indicators. First glance answers which account is being
authorized, its status and last real stage. Search/status/page/detail selection live
in the URL. Details show a timestamped stage history, elapsed time and safe error
codes with actionable hints. Existing-account failures can be manually resubmitted.

Manual and batch account actions submit persisted queue entries in one request.
A single worker drains the queue without idle polling; navigation does not cancel
execution. Queued entries survive server restarts; interrupted running tasks become
failed after a bounded deadline rather than retrying automatically. Profile/email
bindings are checked again before execution. Automatic recovery and draft creation
also write history; draft history records credentials obtained, not account created.
No scripts, mailbox passwords or OAuth credentials enter the task-history table.
Draft creation remains cancellable and preserves its result only in the open dialog.

Account management has only shortcuts and busy states, not a task panel. Active
history updates every four seconds while the page is visible; hidden/unmounted or
idle pages do not poll. Runner events distinguish mailbox connection, protocol start,
email submission, code request, code reading/submission, consent, token exchange, verification and
saving. Never show an invented percentage or claim mail delivery from a button click.

Profile forms expose one receiving-mailbox address; IMAP login uses that address.
The task/account login email alone determines the OAuth identity and recipient filter.
Gmail password-based IMAP uses an app password; show an official help link next to
the password field. Preserve existing backend username compatibility for older clients.

Custom-code profiles have no API-address input or required endpoint. Scripts own
request addresses/protocols and return the documented code, auth_file or error
object. Existing apiURL context values remain available for legacy scripts.

OneClickAuthForm is shared by account creation and the existing-account shortcut.
Existing accounts prefill the bound profile and account login email, merge only
the binding keys at submission and navigate to the task details. New accounts
require manual email input and retain the established creation flow.
