# Spec 071: limits of signing in, counted audit erasures and exports, and accounts from single sign-on

Wave RV4. Small, self-contained changes to signing in, the audit log, permission counting, single sign-on and the editing service. Choices that change what an operator sees were decided on the product owner's behalf on 2026-11-13, and may be overruled.

## 1. Limits of signing in

**What is built.**

- **An IPv6 network counts as one address.** The limits per address (logins, second steps, passkey steps, account links, the first setup) count an IPv6 address by its /64, the block one network gets (RFC 6177). An IPv4 address, also written as IPv4-mapped IPv6, counts as itself (`addressKey`, `internal/httpapi/limiter.go`).
- **Windows end in order.** The limiter keeps its windows in the order they began, and each attempt forgets those that ended. No attempt walks every key, and memory follows the attempts of one window.
- **Signing in has a share of its own.** Requests that change something below `/api/v1/auth/` and `POST /api/v1/setup` form a class beside saving, reading and reports: a quarter of the pool at once, eight times that waiting, five seconds of patience. A rush of sign-ins no longer takes the room of saving, and saving no longer blocks a sign-in. Readiness does not count this class, so a rush of sign-ins does not take a node out of a load balancer.
- **A wait for a password hash ends with its request.** `HashPassword` and `VerifyPassword` take the context of their caller; a request that ends while it waits for a hash slot stops waiting. A login cut short that way is not counted as a wrong password.

**Proof.** `TestAnIPv6NetworkCountsAsOneAddress`, `TestTheLimiterForgetsExpiredWindowsAsItGoes`, `TestAResetKeyKeepsItsNewWindow`, `TestSignInsCannotFillTheRoomOfSaving`, `TestASignInGetsInWhileSavingIsFull`, `TestSignInRoutesHaveTheirOwnClass`, `TestAWaitForAHashEndsWithItsRequest`.

## 2. Audit exports and the check from the console

**What is built.**

- **An export says how it ended.** The last line of an audit CSV has `#complete`, `#limit-reached` or `#incomplete` in its first cell and the number of events in its second, as wide as the header. A file without such a line was cut off on the way.
- **An export reads in pages** of 5,000 sealed events, each a query of its own after the last seq it wrote, so the limit on one query of a read applies to each page. Events that wait for the sealer come last, as before.
- **Sending takes as long as it needs.** The audit export and the download of an export file move their write deadline as an attachment does.
- **The check from the console** walks the chain with a query limit of ten minutes instead of the fifteen seconds of a read, and its answer may take as long. The nightly check and `diffluence audit verify` have no limit, as before.

**Proof.** `TestAnExportEndsWithTheCountOfItsEvents` (pages of two over five sealed events and one waiting), `TestAnExportCutAtItsLimitSaysSo`, `TestTheConsoleCheckOfTheAuditLogIsNotCutAtTheLimitOfARead`.

## 3. Permission changes are counted at commit

**What is built.** Migration 0057 replaces the statement triggers that raise `permission_generation` with deferred constraint triggers on the same tables and columns. Each transaction that changes a fact of a permission check raises the count once, when it commits. The row of the count is locked for the moment of a commit only, so a long transaction that adds people (an import, a sync of people, a space made from a template) no longer makes every other such change wait for its end. The count still becomes visible together with the change. A change that rolls back counts nothing. `TRUNCATE`, which no code path runs, keeps counting at the statement.

Only requests with a memo read kept facts, and those change no permission, so no transaction keeps facts of its own uncommitted change under the old count.

The architecture principle "Permissions are never cached" now says what is built: a node keeps facts only while the count stays the same (ARC-231).

**Proof.** `TestALongPermissionChangeDoesNotHoldUpOthers`: a change commits within three seconds while a transaction that changed group members is still open; a rolled-back change counts nothing; a transaction of two statements counts once, at its commit. The existing `TestAChangeEndsKeptFactsAtOnce` still holds.

## 4. Setting up a second factor reaches only the setup

**What is built.** A session that must set up a second factor reaches an exact list of routes, by route pattern: reading itself and its security page, changing its password, setting up an authenticator app or a passkey, confirming who it is, and signing out. Everything else, also below `/api/v1/me` and including making a token, answers 403 `second_factor_required`. A new route is closed to such a session until it is listed.

**Proof.** `TestASessionThatMustSetUpASecondFactorReachesOnlyTheSetup`, with the existing `TestARequiredSecondFactorLimitsTheSession`.

## 5. Erasures of personal parts are counted

**What is built.**

- `audit.ErasePersonal` returns how many personal parts it erased, waiting and sealed. Anonymising someone records that number in its `user.anonymised` event, under `personal_parts_erased`, in the chain.
- A check of the chain counts the events whose personal part is gone (`erased_parts`) and adds up the numbers erasures recorded (`recorded_erasures`). When more parts are gone than recorded, the check fails with the reason "personal parts are gone that no recorded erasure removed".
- Migration 0057 counts the parts erased before this release into `audit_erasure_baseline`, which the check adds to the recorded ones. The application role may only read that table.

**Proof.** `TestAPersonalPartRemovedWithoutARecordedErasureIsFound`, `TestAnErasureRecordedWithItsCountKeepsTheCheckWhole`, `TestErasingAPersonReachesAWaitingEvent`, and `TestPersonalDataInTheAuditLogCanBeErasedWhileTheChainHolds` through the API.

## 6. Forgot password and sign-up answer in a fixed time

**What is built.** The work of a forgot-password or sign-up request runs beside its answer, which always comes after 400 ms, however long the work takes. Only work for a known address, or a domain allowed to sign up, does much, so its time no longer shows in the answer. The work keeps going for up to 30 seconds after the caller leaves, so no link is lost to an impatient browser. Sixteen such pieces of work run at once per node; past that the answer is 503 "busy", whatever the address.

**Proof.** `TestForgotPasswordAndSignUpAnswerInTheSameTimeForAKnownAddress`: with a mailer that takes 1.5 seconds, both answer within a second, and both mails still go out.

## 7. Accounts from single sign-on

**What is built.**

- `DIFFLUENCE_OIDC_AUTO_CREATE` is off by default. A login through the provider then opens only an account that exists already, through SCIM or an invitation.
- When it is on, a new account is made only when the token says `email_verified: true`. Otherwise the login is refused as "no account", and the audit log names the reason `email_not_verified`.
- `DIFFLUENCE_OIDC_ALLOW_UNVERIFIED_EMAIL=true` lets an operator accept unverified addresses, for a provider that never sends the claim and where only the organisation can set an address.

**Decided on the product owner's behalf on 2026-11-13, may be overruled:** both defaults are the safe ones. An installation that relied on accounts at the first login sets the switches before upgrading (`docs/operations/upgrade.md`).

**Proof.** `TestSingleSignOnMakesNoAccountsByDefault`, `TestSingleSignOnMakesNoAccountFromAnAddressNobodyVerified` (false, missing and a string "true"), `TestAnOperatorMayAcceptUnverifiedAddresses`, and the existing SSO tests.

## 8. Guest invitations are decided in the sharing module

**What is built.** `sharing.InviteGuest` makes the whole decision for `POST /api/v1/spaces/{id}/guests`: the right to invite guests to the space, the switch for guests, and whether the caller invites at once as an administrator or files a request. The handler only reads the request and writes the answer.

**Proof.** `TestTheSharingModuleRefusesAGuestToSomeoneWhoDoesNotManageTheSpace` calls the module directly; the existing guest tests run through the API.

## 9. The editing service

**What is built.**

- The service compares the token of the API in constant time: both sides are hashed with SHA-256 and compared with `timingSafeEqual`. An empty token lets nobody in (`isTheAPI`, `collab/access.mjs`).
- In `compose.yaml` the service runs with a read-only root, `/tmp` in memory, no new privileges and every capability dropped, as the other helpers do.

**Proof.** `access.test.mjs`: the right token, a longer and a shorter one, a missing prefix, a missing header and an unset token. The compose settings are not run in a test here.

## Load

| Part | Extra database work |
| --- | --- |
| Limits of signing in | None: the limits live in memory. The sign-in class holds at most a quarter of the pool at once, which is less than before, when sign-ins could take all of saving |
| Counting at commit | Fewer writes: one `UPDATE permission_generation` per transaction instead of one per statement. Each changed row adds one deferred trigger call at commit that reads a transaction setting, without I/O. The row lock is held for the commit only |
| Erasure counts | One read of one row per check of the chain. The upgrade reads `audit_events` once, with `audit_personal`, to count the parts erased before |
| Exports | The same rows; a million rows take 200 queries instead of one, each by the primary key of `audit_events` |
| Forgot password and sign-up | The same work as before, beside the answer instead of in it |
| Second factor, single sign-on, guests, editing service | None |

## Not built here

- A job with a file for the audit export and for the check from the console.
- A measurement of the counting with a sync of people running, on the server.

## Provenance

Our own design, from the review of 13 November 2026. No outside code or text.
