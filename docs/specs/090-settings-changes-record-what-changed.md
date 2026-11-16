# 090: Settings changes record what changed

Status: built
Phase: hardening before 1.0 (wave RV11)

## Problem

SEC-603 asks that a settings change is audited with what changed, and SEC-805 that the old value is added where it helps an investigation. This spec makes every change of the runtime settings record, in the same transaction, each changed field with its value before and after, while secrets and personal data stay out of the audit log. Two smaller points ride along: the public answer `GET /api/v1/appearance` is kept per node for a short time and limited per address, and the study of two addresses that could mislead a reader, the title of a link and the terms and help addresses.

## Options

### What a change records

| Option | For | Against |
|---|---|---|
| A. New values only | Smallest row | An investigation must find the row before to learn what a change undid; the first change after an upgrade has nothing before it |
| **B. Before and after of each changed field** | One row answers "what did this change": the permission changes already do so (SEC-805) | About twice the size of A, bounded below |

**Choice: B.** Fields that did not change are left out, so a row holds only what the change did.

### Which values may be written

| Option | For | Against |
|---|---|---|
| A. Every field, with a list of secrets to hide | Nothing to keep up for ordinary fields | A new field that holds a secret or a name is written until someone remembers the list |
| **B. Only fields listed as safe keep their values; every other field is recorded as changed, without values** | Deny by default: a field nobody decided on never reaches the log | Each new field needs a decision; a test fails until it has one |

**Choice: B.**

## Behaviour

1. **What a row holds.** The audit event `settings.updated` gains `changes`: for each field that changed, `{"before": …, "after": …}`, or `{"redacted": true}` for a field whose values are not written. A field absent before reads as `null`. Fields that did not change are left out; a save that changes nothing records `changes: {}`. The section or key, and `confirmed`, stay as before; a reset of the look keeps `reset: true`.
2. **Where.** Every writer of runtime settings that records `settings.updated`: a section (`SetSection`, `SetSectionConfirmed`, through them the first setup and the organisation name), a reset of the look, one stored value through `Write` (export limits, experimental parts, features for all spaces, the company home page, how long drafts stay, the default permission set of spaces), the console's general values (`Update`), and the backup schedule, which the backups module stores. The values before are read in the same transaction, from the row that transaction locks.
3. **Which fields keep their values.** Each section and each caller of `Write` lists its safe fields; anything else is redacted. The lists are in `internal/settings/changes.go` and at each caller. A test names every field of every section and fails for a field that is neither safe nor redacted on purpose.
   - Redacted on purpose: the organisation name of the look, which the first setup may fill with the name of a person (the instance it is for).
   - No runtime setting holds a secret. The SMTP password, the client secret of the identity provider, the collaboration, metrics and PDF tokens stay in the environment (`internal/config`); webhook secrets, tokens and SCIM tokens live sealed in their own tables, and their changes have their own events, which never held a secret. Deny by default keeps it so if one is added.
4. **How big.** A value whose text is longer than 200 characters is cut there, with `"truncated": true` on its field. An address with a user name or password in it is recorded without them.
5. **The audit screen** shows the whole event, `changes` included; it needs no change.

### The appearance answer

6. `GET /api/v1/appearance` takes the font, the look, the logo, the general section and the features from a copy each node keeps for 30 seconds, as it keeps the health facts. A change of a section, the look, the logo or the font on a node drops that node's copy at once; another node shows it within 30 seconds. Maintenance mode keeps its own copy of 5 seconds, and a banner past its end date is left out at the moment of the answer.
7. Without a login, one address (an IPv6 network counts as one) may ask it 600 times a minute, the way the login limits count addresses; past that the answer is 429 `rate_limited` with `Retry-After`. Someone logged in is not limited by it.

### Addresses that could mislead

8. **The title of a link.** A link in a document no longer keeps a `title`. It was shown as a tooltip and could name another address than the one the link opens; the editor offers no way to set one, and Markdown import took it from `[text](address "title")`. The document gate drops it from every saved document, and Markdown import no longer takes it. A document stored before keeps its title until it is next saved.
9. **The terms and help addresses** stay http or https. An internal help site or intranet often has no certificate, the address is chosen by an administrator, and the browser shows it before it opens. An address with a user name or password is refused: the appearance answer hands it to anyone, and the part before `@` can make it look like another site.

## Permissions and audit

Nothing changes in who may change a setting. Each change records one `settings.updated` as before, in the same transaction, with more in its details.

## Data

No migration. The audit rows of earlier changes keep what they held.

## Load budget

A change reads its row before it writes, inside the transaction that already locks it: one indexed read more per change, a few a day. The appearance answer, which every open tab polls, falls from six reads per call to six per node per 30 seconds; the limit and the copy are held in memory.

## Acceptance criteria

- [x] A change of a section records each changed field before and after, and leaves out the fields it did not change (`internal/app`, `TestASettingsChangeRecordsEachChangedFieldBeforeAndAfter`).
- [x] The organisation name is recorded as changed without its values (`TestTheOrganisationNameIsRecordedAsChangedWithoutItsValue`).
- [x] A field no list names is recorded as changed without values; a long value is cut and marked; an address loses its user name and password (`internal/settings`, `TestAFieldNotListedAsSafeIsRecordedWithoutItsValues`, `TestALongValueIsCutInTheAuditRow`, `TestAnAddressIsRecordedWithoutItsUserAndPassword`).
- [x] Every field of every section is safe or redacted on purpose (`TestEveryFieldOfEverySectionHasAnAuditDecision`).
- [x] A stored value, a reset of the look, the console's general values and the backup schedule record before and after (`TestAStoredSettingRecordsItsValueBeforeAndAfter`, `TestAResetOfTheLookRecordsWhatItReset`, `TestTheGeneralValuesRecordWhatChanged`, `internal/backups`).
- [x] The appearance answer comes from the node's copy, and a change on the node shows at once (`TestTheAppearanceAnswerIsKeptUntilASettingChanges`).
- [x] Without a login the appearance answer is limited per address; another address and a logged-in person are not (`TestTheAppearanceAnswerIsLimitedPerAddressWithoutALogin`).
- [x] A saved document and a Markdown import keep no link title (`internal/document`).
- [x] A terms or help address with a user name or password is refused; plain http stays allowed (`TestATermsOrHelpAddressMayNotCarryAUserOrPassword`).

## How the product owner tests it

1. Administration, Settings, General: change the default time zone and save. Administration, Audit: open the newest `settings.updated`. It shows `changes` with the zone before and after, and nothing else.
2. Look: change the organisation name and save. Its audit row says `organisation_name` with `redacted: true`, without either name.
3. Reset the look. Its row lists each field that went back to its default.
4. Open the login page in a private window: it shows the look as before. Change the instance name in another window and reload the login page: the new name shows at once.
5. Look: enter `https://name:password@help.example.com` as the help address. It is refused. `http://intranet/help` is accepted.

## Provenance

Own design. Go documentation of `encoding/json` and `net/url` (pkg.go.dev, consulted 2026-11-16). OWASP Logging Cheat Sheet on data to exclude from logs (owasp.org, consulted 2026-11-16).
