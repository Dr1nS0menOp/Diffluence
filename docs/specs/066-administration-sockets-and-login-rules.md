# Spec 066: rules for administration, editing sockets and logins

Wave RV1 tightens five places where an installation must hold up under load and under people with less than full rights. Each part says what is built and which test proves it.

## 1. Editing sockets have their own room

The WebSocket of the editor (`GET /collab`, ARC-118) stays open as long as a person edits. It holds no database connection.

**What is built.**

- The socket does not pass the admission gates of spec 061, so open editors never take a place that a read or a save needs.
- It is not timed as a request: a socket open for an hour would otherwise fill the slowest bucket of `diffluence_http_request_duration_seconds`.
- It has caps of its own: 512 open sockets per node and 8 per login, counted by the id of the session. Past a cap the answer is `503 busy` with `Retry-After: 5`, and the editor tries again. The cap is taken after the checks of spec 067 (a session, the right to edit the page, the ticket), so a refused socket holds no place.
- The origin check stays. It stops a page on another site in a browser. A client that is not a browser can send any origin, but it still needs a session and the right to edit, and the caps bound what it can hold.

**Proof.**

- `TestOpenEditorSocketsLeaveReadingFree`: 50 open sockets on a node with a pool of 20, then a page read answers at once;
- `TestEditorSocketsAreNotTimedAsRequests`;
- `TestOnePersonCannotTakeEverySocketOfANode`;
- `TestASocketWithoutALoginIsRefused`;
- four mutations.

## 2. Only a system administrator acts on a system administrator

An administrator runs accounts and groups. An account or group that holds `system_administrator` is acted on only by someone who holds it too, through one helper, `mayActOn`, and its twin for groups, `mayActOnGroup`.

**What is built.**

- `mayActOn` applies to:
  - changing the name or type of an account;
  - disabling or enabling it;
  - sending a reset link;
  - resetting the second factor;
  - lifting a password stop;
  - setting an end date;
  - anonymising.
- `mayActOnGroup` applies to adding a member to a group that holds system administration and to deleting that group. It sits in the one function every addition passes through, so the owner of a group is bound by it too.
- Taking someone out of any group counts as acting on their account, since it can take access away. A member of a group that holds system administration holds it through the group, so taking them out needs it too. Adding someone to an ordinary group only gives, and stays open to every administrator.
- Identity providers (SCIM and the OIDC groups claim) act as the system and are not bound: they are leading.
- The refusal is `409 needs_system_administrator`.

**An administrator sends a reset link.** An administrator no longer types a new password for someone else.

- `POST /api/v1/admin/users/{id}/reset-password` takes no body. It makes a reset link, valid once for an hour, and ends any earlier one.
- With mail on, the link is mailed and the answer is `{"mailed": true}`.
- With mail off, the answer is `{"mailed": false, "link": "…"}`. The console shows it once to pass on; it is not stored in a readable form.
- Both are audited as `auth.reset_link_sent`.
- The person chooses the password through the existing reset page, and the change is told to them by mail as any other.
- An account from an identity provider, a service account or a disabled account gets no link (`409 not_local` or `not_active`).
- No route or service function sets a password for someone else any more: the one that did is gone.

**Proof.**

- `TestOnlyASystemAdministratorActsOnASystemAdministrator`;
- `TestOnlyASystemAdministratorChangesASystemAdministrationGroup`;
- `TestAnAdministratorSendsAResetLinkAndNeverChoosesThePassword`;
- `TestAResetLinkIsOnlyForLocalAccounts`;
- `TestAPersonIsToldOfEveryChangeToTheirSignIn`, which now goes through a mailed link;
- sixteen mutations.

## 3. The address of the reverse proxy in Compose

`DIFFLUENCE_TRUSTED_PROXIES` says whose `X-Forwarded-For` is believed (SEC-012).

**What is built.**

- `compose.yaml` passes it on. Left empty, it is `127.0.0.0/8,::1/128,172.16.0.0/12`.
- That default fits the port Compose publishes on `127.0.0.1`. A proxy on the same machine reaches the app through Docker's gateway, and a proxy in the same Compose project from its own network. Docker takes both from `172.16.0.0/12`. Nothing else can reach a port on `127.0.0.1`, so nobody else can claim an address.
- With `DIFFLUENCE_BIND=0.0.0.0` the operator sets the proxy's own address: other machines in that range could then reach the port.
- `.env.example` and the install guide say so.
- System health warns (`proxy_not_trusted`) when every login of the last day came from one private or loopback address, and more than one person logged in. That address is the proxy, not yet trusted. A shared public address, such as an office line, does not warn.

**Proof.**

- `TestLoginsThatAllComeFromOnePrivateAddressWarn`;
- `TestLoginsThatAllComeFromOnePrivateAddressAreSeen`;
- three mutations.

## 4. Discovery of the identity provider is bounded

**What is built.**

- Discovery of the OIDC provider uses its own HTTP client with a 10-second timeout, passed through `oidc.ClientContext`. The same client fetches the signing keys later, so a key refresh is bounded too.
- The long-lived context, without the cancel of the request, is kept only so a key refresh can outlive the login that started it.
- Discovery runs outside the lock: one login asks, the others wait for that answer or for their own request to end.
- A failure is remembered for 30 seconds, so a provider that is down is not asked again by every login.

**Proof.**

- `TestAProviderThatNeverAnswersDoesNotHoldLoginsForEver`;
- `TestOneSlowDiscoveryDoesNotQueueEveryLoginBehindIt`;
- `TestAFailedDiscoveryIsRememberedForAWhile`;
- three mutations.

## 5. The restore from the console

**What is built.**

- `POST /api/v1/admin/backups/restore` needs `system_administrator`. A restore brings back every account and permission of the backup. Others see the state of a restore and a line that says who can start one.
- A code given for it counts against a limit per account: the same 10 tries in 15 minutes as a password, under its own key. Past it the answer is `429`.
- Every wrong code is written to the audit log as `auth.second_factor_failed`, in a transaction of its own, since the refusal rolls back its own.
- `ConfirmSecondFactor` has one caller, the restore.

**Proof.**

- `TestOnlyASystemAdministratorRestores`;
- `TestWrongCodesForARestoreAreAuditedAndLimited`;
- three mutations.

## Decided on the product owner's behalf on 2026-11-13, may be overruled

- **D1:** caps of 512 editing sockets per node and 8 per login. A person edits a few pages in a few tabs. 512 bounds the file handles and memory of proxied sockets, well above the number of people who edit at once on one node at the sizes of the load budget.
- **D2:** count sockets per session, after the checks of spec 067, rather than per person. A person in two browsers has two logins and gets two allowances, and a socket that is refused anyway never holds a place.
- **D3:** taking a system administrator out of a group counts as acting on them; adding them to an ordinary group does not. Adding only gives, and refusing it would stop administrators from building ordinary groups.
- **D4:** the reset link with mail off is shown once in the answer and in the console, not stored readably and not shown again. A new link ends the old one.
- **D5:** the Compose default trusts `172.16.0.0/12` and not `192.168.0.0/16`. Docker takes its networks from the first range unless that runs out, and a local network more often uses the second. An installation on another range sets its own.
- **D6:** the wrong-code limit reuses the per-account login limiter (10 in 15 minutes) under its own key, rather than a new limiter with its own numbers.
- **D7:** the restore page does not list which administrators and grants the backup brings back. The backup is encrypted, and only the backup worker holds the key. Reading it would mean decrypting and restoring it into a scratch database first, which is not cheap. The page says that a restore brings back every account and permission.

## Upgrade

- Nothing to do for an installation with its proxy on the same machine or in the same Compose project: the new default trusts it, and login limits and the audit log get the address of each person. Take the new `compose.yaml` with the release.
- With `DIFFLUENCE_BIND=0.0.0.0`, set `DIFFLUENCE_TRUSTED_PROXIES` to the proxy's address. A value set in `.env` before this release was not passed on by `compose.yaml`, and now is.
- Administrators without `system_administrator` can no longer start a restore, nor act on accounts and groups that hold it.
- Scripts that called the reset-password route with a password must read the link from the answer instead.
