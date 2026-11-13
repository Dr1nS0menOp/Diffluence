# Spec 069: a feature freeze for 1.0, a passkey after the password, and docs that say what is true

Wave RV3a. It builds ADR-043, the product owner's answers of 13 November 2026, and fixes found by the review of that day that are small and self-contained. How each answer is built was decided on the product owner's behalf on 2026-11-13, and may be overruled.

## 1. Experimental parts start off

**What is built.**

- **One switch per part, for the installation.** The settings key `experimental` holds the list of parts that are on: `automation`, `databases`, `bundles`, `console_restore`. No key means every part is off. `GET /api/v1/admin/experimental` reads it; `PUT` sets it, for administrators with a session, audited as `settings.updated`. An unknown name is refused, and a name given twice is stored once (`internal/features/experimental.go`).
- **Public sites and analytics keep their switches.** Both already start off (`exchange.public_sites_enabled`, `analytics.enabled`); the console marks them experimental.
- **Off means refused, hidden and kept.**
  - Every route of automation, the bundles and the console restore answers **404 `experimental_off`** with a message that says an administrator can turn it on. Access is checked first, so a stranger still gets 401 and a member without the right still gets what the rule says. Cancelling a restore stays open.
  - `GET /api/v1/pages/{id}/rules` answers an empty list instead: every page asks it, and no rule can run.
  - A new database is refused in `pages.Create` with the same 404; a database made before stays readable and editable.
  - A rule made before stays, and no rule runs: the matcher asks the switch before every run, also for a rule of the instance.
  - `GET /api/v1/system/capabilities` says `automation`, `databases`, `bundles`, `console_restore`, so no screen offers what is off. The screens show one line in place of the part.
  - The example space of a new installation leaves out its database while databases are off.
- **The console.** Administration, Organising has a section "Experimental parts" with one checkbox per part.
- **An upgrade keeps what was used.** Migration 0054 writes the key once, from the traces each part leaves: a row in `automation_rules`; a page of kind `database`, also in the trash; a `bundle_export` job, or `bundle.imported` in the audit log; a row in `ops.restores`. On a new installation nothing has left a trace, so the list is empty. `ON CONFLICT DO NOTHING` keeps a choice already made, also when a restore of an older backup runs the migration again.
- **The tests turn every part on.** `testutil.Database` turns all parts on, so each suite tests the part itself; `testutil.InstalledDatabase` is the database exactly as an installation starts. `scripts/e2e` does the same for the browser tests. A test keeps the list in `testutil` equal to the parts.

**Proof.**

- On a fresh installation: every part is off, the capabilities say so, analytics and public sites are off, and eleven routes answer 404 `experimental_off`; no database is made (`TestANewInstallationRefusesEveryExperimentalPart`, `TestANewInstallationStartsWithEveryExperimentalPartOff`).
- Only an administrator turns a part on, only a known part, and it is audited; the others stay off (`TestOnlyAnExperimentalPartCanBeTurnedOn`, `TestAnAdministratorTurnsOnOneExperimentalPart`).
- Off keeps what exists: a rule stays and does not run; a database stays editable (`TestARuleStopsRunningWhenAutomationIsTurnedOff`, `TestADatabaseOutlivesTheSwitch`).
- The upgrade: one rule turns on automation only; traces of all four turn on all four; a choice already made stays (`TestAnUpgrade*`).
- The example with databases off and on (`TestTheExample*Database*`).
- Browser: turning databases off in the console takes "New database" out of the Create menu, and turning it on brings it back (`create-kinds.spec.ts`).

**Not proven here:** the upgrade on a copy of a real installation; the detection was tested by running the migration again on prepared data. A bundle export whose job was cleaned up leaves no trace, and an administrator turns bundles on by hand (`docs/operations/upgrade.md` says so).

## 2. The claim

The README, `docs/operations/install.md` and `docs/operations/limits.md` say: one node, a few hundred people, measured on your own hardware, and link the stress tests of 10 and 12 November.

## 3. Docs that say what is true

- `SECURITY.md` says alpha, as the README does.
- `docs/security/risks.md`: the stop after a long run of wrong passwords holds after a restart and on every node (ADR-038); a line per experimental part; the measures added since the page was first written.
- `docs/security/threat-model.md`: rows for roles, login with a second factor, API tokens, the backup worker, the console restore and the experimental parts; the audit chain as ADR-039 and 041 built it; a table of what each control holds against a member, an administrator, SQL injection, a compromised app process and the database owner; the owner password in the serving process as a property to change in a coming wave.
- The decision log says *Accepted* for the built ADR-005 to 007, 009, 011, 013 and 017 to 020. Its own rule forbids editing a decision, not its status; ADR-043 records the change, and the decisions are unchanged.

## 4. The scope of a token follows its route

`scopeFor` reads the pattern of the route (`r.Pattern`) instead of searching the address for "/export". The export routes are listed by pattern; admin routes need the admin scope; a request without a pattern needs the admin scope.

**Proof:** a read token reads the pages of labels called `export` and `x-export`, and is still refused on a page export, the export formats and a space export (`TestTheScopeOfATokenFollowsTheRouteNotTheAddress`, written first and seen failing). Every route whose pattern holds "export" refuses a read and write token (`TestEveryExportRouteNeedsMoreThanTheReadScope`), so a new export route missing from the list fails. A unit test covers each branch of `scopeFor`.

## 5. No query is put together at run time

The key rotation (`internal/app/keyrotation.go`) and the backup mail (`internal/backups/backups.go`) built queries with `fmt.Sprintf` from quoted names out of a list in code. Each entry of those lists now holds its queries written out in full, and the page size of the rotation is a parameter. The cursor of the file jobs goes the same way. `TestEachSealedColumnHasItsOwnQueries` checks each written query against its table, column and id; `TestNoQueryIsPutTogetherWithSprintf` fails on any query built with `fmt.Sprintf` outside one listed exception (`SET ... statement_timeout`, a number).

## 6. A pinned database image for development

`compose.dev.yaml` pins `postgres:16` by the same digest as `compose.yaml`, so a moved tag no longer recreates the test database in the middle of a run.

## 7. An account with only a passkey is asked for it

**What is built.** After a right password, `Login` asks for every second factor the account has: a code from an authenticator app, a passkey, or either when it has both. The answer names them in `mfa_methods`. Two public routes finish the login with a passkey:

- `POST /api/v1/auth/login/passkey/begin` takes the `mfa_token` of the password step and returns WebAuthn options for the passkeys of that account only, with a token of its own.
- `POST /api/v1/auth/login/passkey/finish` takes both tokens and the assertion. The passkey step must belong to the account of the password; the shared second step counts tries, ends the challenge after five, and audits a failure as `auth.login_mfa` with the reason `bad_passkey`. The session is noted as "password and passkey".

The code route still accepts a recovery code for such an account. The login page shows the code field only when the account has a code, and a button "Confirm with your passkey" when it has a passkey. Both routes are rate limited per address and stay open during maintenance.

**Proof.** Written first and seen failing: the password alone no longer opens a session for an account with a passkey, and a code cannot finish it (`TestAPasswordLoginAsksForThePasskeyOfAnAccountThatHasOnlyThat`); a made-up challenge cannot begin a passkey step, an empty assertion is refused and audited (`TestThePasskeyStepOfALoginNeedsItsOwnChallengeAndAnAssertion`); a passkey step of one account does not finish another's (`TestThePasskeyStepIsBoundToTheAccountOfThePassword`); the factors offered match what the account has (`TestALoginOffersTheSecondFactorsTheAccountHas`). A full login with a real assertion runs in the browser with a virtual authenticator (`access.spec.ts`).

## Load

Each check of a switch is one read of one row of `settings` by its key. The hot paths do not read it: the space features, comments and reactions are unchanged. It is read on the rule routes, at the start of each rule run, on `GET /pages/{id}/rules` once per page view, on the capabilities once a minute per browser, and when a database is made.

## Provenance

Our own design, from the review of 13 November 2026 and the product owner's answers. No outside code or text.
