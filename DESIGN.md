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
proxy per task, retained for both headless browser login and token exchange.
An empty pool falls back to the account proxy. Distribution is not a guarantee
against provider risk checks. Authorization-file scripts use the protocol path;
mailbox-code login uses an isolated Playwright context. Script requests receive
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
