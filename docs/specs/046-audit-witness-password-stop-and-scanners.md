# Spec 046: a witness of the audit log, a stop on password guessing, and scanners in CI

Status: built on branch `wave-sx`.

Phase: the second wave of ADR-038 (security). Wave PM ran at the same time (permissions for blog posts and databases), so this wave stays out of permission code. Its migration is 0042, after wave PM's 0041. Until wave PM lands, this branch holds 0041 with an empty placeholder, because versions must be contiguous.

## The study

### The audit chain and its checkpoints (ADR-013)

| What | Found on `main` at `1a42053` |
|---|---|
| Chain | `audit_events` with a hash per row over the row before, an advisory lock for the order, triggers against update and delete, and no such rights for the application role (SEC-810, SEC-811) |
| Checkpoints | `audit.SignCheckpoint` signs `(seq, hash)` of the head with Ed25519, stores it in `audit_checkpoints` and records `audit.checkpoint_signed`. The job `audit-checkpoint` ran once a day |
| Checking | `audit.Check` walks the chain, then every checkpoint against the key and the chain. The daily job, the Verify button, `diffluence audit verify` and `restore.sh` all call it (SEC-814) |
| The gap | Whoever owns the tables can switch off the trigger, delete the newest rows and the checkpoints that cover them, and switch it on again. The shorter chain is whole, and every checkpoint left matches it. Nothing outside the database remembers how far the chain reached |
| The key | Derived from the root in `master.key`, outside the database. Whoever holds both can sign new checkpoints (SEC-815) |
| The application log | JSON on standard error through `slog`, at the level `DIFFLUENCE_LOG_LEVEL` sets. Operators send it to a log server or a SIEM |

### Password guessing

| What | Found |
|---|---|
| Limits | In memory, per node, in a window of 15 minutes: 50 tries per address, 10 per account and address, 100 per account from all addresses. A good login gives its attempt back (SEC-109) |
| Answers | One message for an unknown address, a wrong password and a disabled account, and a dummy hash so they take the same time (SEC-108) |
| What NIST asks | SP 800-63B: an authenticator stops after at most 100 failures in a row, kept across restarts (SEC-110) |
| What ASVS asks | 6.1.1: document how guessing is stopped without letting anyone lock a person out on purpose (SEC-111) |
| Other password checks | Changing the password, starting TOTP and turning it off check the password behind a session, with their own limit per account |
| Ways in without a password | Passkeys and OpenID Connect open a session through `OpenSession`, which never reads `local_credentials` |
| Mail | `notify.AccountMessage` holds the words of each account mail; mail goes through the queue when a mail server is set |
| Console | Administration, Users opens a dialog per person with disable, reset authenticator and a new password |

### Scanners

`09-way-of-working.md` named Semgrep, CodeQL and a ZAP baseline as "not in CI yet" (SEC-1023). ADR-038 asks for all three. Actions in `ci.yml` are pinned to commit SHAs where Renovate pins them, as Trivy is.

## Decisions

### D1. Checkpoints every five minutes, and only when something happened

The job `audit-checkpoint` runs every five minutes. It signs nothing when the newest event is the record of the last checkpoint, so a quiet instance does not add 288 events a day to its own log. This bounds what can be removed without a trace to the events of the last five minutes.

### D2. Each checkpoint goes to the application log, at every level

After the checkpoint is committed, the worker writes one line:

```json
{"level":"INFO","msg":"audit checkpoint","witness":"audit-checkpoint-1","seq":4711,"hash":"…","signature":"…","public_key":"…"}
```

It is written after the commit, because a line for a checkpoint that was rolled back would later look like a removed tail. It is written whatever `DIFFLUENCE_LOG_LEVEL` says: a handler wrapper answers "enabled" for every level, so an operator who quiets the log does not lose the witness. The line holds a position, two hashes and a public key, and no personal data. The format has a name, so a later shape can be told apart.

### D3. `diffluence audit verify --witness FILE`

It reads the log (`-` for standard input), takes every line with the witness name, also after a prefix from a container runtime, and refuses a witness line it cannot read: a log that was tampered with must not pass because a line was skipped. A log with no witness line is refused as the wrong file. It then runs the usual check, and compares each checkpoint, oldest first:

1. The signature must hold for the instance key. A forged line proves nothing.
2. A checkpoint at or before the last prune anchor is skipped and counted: its rows are gone by design.
3. A checkpoint past the newest event means the tail was removed.
4. A checkpoint whose event is missing, or whose hash differs, means the chain was changed.

The first disagreement is named with its position and reason, the result is audited as `audit.verified` with `witness_compared`, and the command fails. Checkpoints are looked up a thousand at a time.

### D4. A stop after 100 wrong passwords in a row

Two columns on `local_credentials` (migration 0042, expand only): `failed_logins_in_row` and `password_locked_at`.

- Only a wrong password on a live account, on the login route, counts. An unknown address, a disabled account and an account already stopped do not. A password checked behind a session never counts, so nothing but guessing this account's password on the login page can stop it.
- One `UPDATE … RETURNING` adds one and sets the stop when the run reaches 100, so two guesses at the same moment cannot both miss it. An unknown address runs the same statement on no row, so it costs about the same.
- The hundredth wrong password records `auth.password_locked`, in the same transaction as the failed login, and the person gets a mail once. More guesses after that write nothing.
- While stopped, the password still runs through the hash, and every answer, also to the right password, is the one for a wrong password. The stop says nothing to the guesser that a wrong password does not.
- The right password ends a run (written only when there was one). A new password ends a stop, whoever set it: the person by a reset link or in their account, or an administrator. An administrator can also press **Allow password login again**, `POST /api/v1/admin/users/{id}/unlock-password`, audited as `auth.password_unlocked`.
- Passkeys and the identity provider keep working: they never read these columns.
- The memory limits stay as they were. One address gets at most 10 guesses on an account in 15 minutes, so reaching 100 needs ten addresses at once, or many hours. `docs/operations/install.md` lists the three layers and what ends each (ASVS 6.1.1).

### D5. Semgrep, CodeQL and ZAP

| Scanner | How | Fails on | Exceptions |
|---|---|---|---|
| Semgrep | Job `semgrep` in the official image, pinned to `1.183.0` and its digest; rules `p/golang`, `p/typescript`, `p/react`, `p/secrets`; built web files left out | Any finding (`--error`) | `nosemgrep: <rule>` on the line before, with the reason above it |
| CodeQL | Job `codeql` for Go (autobuild) and TypeScript (no build), `security-extended` queries plus the language's `AlertSuppression.ql`, `github/codeql-action` pinned to v4.39.3 | Any result in the SARIF file that is not suppressed; results go to code scanning, except from a fork | `codeql[<rule>]` on the line before, with the reason above it |
| ZAP | In the job `container`, after Trivy: the image just built runs with PostgreSQL, and `zaproxy/action-baseline` pinned to v0.15.0 scans it with `-a` | Any alert not set to IGNORE in `.zap/rules.tsv` | One line per rule in `.zap/rules.tsv`, with the reason |

## Findings of the scanners, and what was done

| Scanner | Finding | Done |
|---|---|---|
| Semgrep | `cookie-missing-secure` on four `http.SetCookie` calls (session cookie set and cleared, login-flow cookie set and cleared) | Not a fault: `Secure` follows the public address, on for https and off only on plain http, where a browser would drop a Secure cookie. Exception on each line |
| Semgrep | `filepath-clean-misuse` in `internal/webui/webui.go` | Not a fault: the name opens in an `fs.FS`, which refuses any name with `..`. Exception on the line |
| Semgrep | `wildcard-postmessage-configuration` in `web/e2e/blocks.spec.ts` | A test that forges a message from the page on purpose. Exception on the line |

| CodeQL (TypeScript) | `js/regex/missing-regexp-anchor` in `web/src/components/file-preview.tsx` | **A real bug, fixed.** `/^text\/|json|xml|yaml/` also matched the type of Word and Excel files (`…openxmlformats…`), so a `.docx` got a text preview and the file page showed its bytes as text. The pattern now matches the whole type; `file-preview.test.ts` failed first, and a mutation guards it. No weakness: the text was shown as text, never run |
| CodeQL (TypeScript) | `js/incomplete-multi-character-sanitization` in `web/src/components/editor/paste.ts` | Not a fault: the marker of a Word list is only tested for a number and never put into the page; pasted HTML passes the editor's schema anyway. Exception on the line |
| CodeQL (Go) | `go/weak-sensitive-data-hashing` in `internal/audit/audit.go` | Not a fault: the chain hashes audit events, which hold no password (SEC-823); CodeQL follows a name with "password" in it. Exception on the line |
| CodeQL (Go) | `go/weak-sensitive-data-hashing` in `internal/identity/leaked/leaked.go` | Not a fault: the hash places a candidate in a filter of public leaked passwords and is never stored. Exception on the line |
| CodeQL (Go) | `go/log-injection` in `internal/httpapi/middleware.go` | Not a fault: the log is JSON, which escapes a line break in the path (SEC-840). Exception on the line |
| CodeQL (Go) | `go/allocation-size-overflow` in `internal/automation/definition.go` | Not a fault, as twice the size of a map in memory cannot overflow; the capacity hint is now the size itself, which removes the doubling |
| ZAP | 10049 non-storable content, 10094 base64 in the bundle, 10096 timestamps in the bundle, 90004 no COEP, 90005 Sec-Fetch-Dest | Low and informational, none a weakness; each is set to IGNORE in `.zap/rules.tsv` with its reason. 66 rules passed |

CodeQL ran here with the same bundle (2.27.4) and queries as the CI job; Semgrep and ZAP with the same images and rules.

## Tests, failure paths first

| Test | Proves |
|---|---|
| `TestTheWitnessLogShowsATailRemovedFromTheDatabase` | With the log at warning level, two checkpoints reach the log without personal data. After the owner removes the tail and its checkpoints, the chain still checks out alone, and the witness names the removed checkpoint |
| `TestAForgedWitnessLineIsRefused` | A changed witness line fails on its signature |
| `TestNothingNewMeansNoNewCheckpoint`, `TestTheHeadOfTheChainIsSignedEveryFiveMinutes` | D1 |
| `TestTheWitnessIsReadFromAMixedLog`, `TestABrokenWitnessLineIsAnError`, `TestWitnessCheckpointsBeforeAPruneAreSkipped`, `TestAChainThatDiffersFromTheWitnessIsNamed` | D3 in the audit package |
| `TestAuditVerifyComparesTheChainWithTheWitnessLog`, `TestAuditVerifyRefusesAWitnessLogWithoutCheckpoints` | The command, end to end on a real database |
| `TestTheHundredthWrongPasswordStopsPasswordLogin` | Through the API: the stop, the same answer to the right password, one audit event, the mail, and an open session that goes on |
| `TestAHundredWrongPasswordsStopPasswordLoginAcrossARestart` | 100 real wrong passwords, one mail, a new service on the same database still stopped, no second mail, and `OpenSession` (passkeys and OIDC) still opens a session |
| `TestOnlyWrongPasswordsOnALiveAccountCount` | Unknown address, disabled account, and password checks behind a session do not count |
| `TestTheRightPasswordEndsARunOfWrongOnes`, `TestAnAdministratorLiftsTheStopAndANewPasswordDoesToo` | What ends a run and a stop; only an administrator can lift it; the console shows it |

Twenty-one new mutations in `scripts/mutation-check.sh`, one for each check above and one for the file-preview fix (`file-preview.test.ts`).

## Load budget (ARC-238)

| Part | Cost |
|---|---|
| Checkpoints | Every five minutes: one indexed read of the newest event. When something happened: one Ed25519 signature, one insert, one audit event, one log line. At most 288 a day, about 100 bytes of log each |
| `audit verify --witness` | Run by an operator. One query per thousand checkpoints; a year of checkpoints is about 105,000 signatures to check, a few seconds |
| Login | The read that found the account already reads the two new columns. A wrong password adds one indexed `UPDATE` in the transaction of its audit event, which was already there. A right password writes only after a run of wrong ones. A stopped account writes nothing more |
| Console | One more `EXISTS` on the primary key per person in the user list |
| CI | Semgrep about two minutes, CodeQL about ten per language, ZAP about two after the image build |

## Not proven here

- The three scanner jobs have not run in GitHub Actions, because nothing is pushed. All three ran here with the same versions, rules and queries, and end clean. The Actions wiring itself (the ZAP action, the CodeQL upload and its permissions, Semgrep's image in a job container) is only proven by the first push.
- A real log server or SIEM keeping the witness lines: the format and the command are proven, the shipping is the operator's.
- A passkey login after the stop in a real browser: proven at `OpenSession`, which the passkey route calls, not with a virtual authenticator, because the end-to-end server has no way to reach 100 wrong passwords inside its own limits.
