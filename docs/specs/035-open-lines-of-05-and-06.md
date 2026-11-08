# Spec 035: the open lines of blueprints 05 and 06

Status: built on branch `wave-z`.

## The study

Blueprints 05 and 06 held 436 numbered lines that were not `built`. Most were marked with the wave that planned them (C to H), and those waves landed long ago without updating these two blueprints. So the first job was to find out what is really there.

Three kinds of evidence were used, in this order:

1. **The names in the line exist in the code.** A line that names tables, columns, functions or settings in backticks counts as built when every one of them exists in `internal/`, `cmd/`, `web/src`, `scripts`, `deploy` or `compose.yaml`. 95 lines passed. Names of the form `table.column` were checked per column: `comments_closed`, `silent`, `reauth_at`, `needs_second_factor`, `stage_text`, `review_on`, `kind` and `owner_id` on `pages`.
2. **A test proves the behaviour.** For lines without names, the test names of the Go tests and the browser tests were matched against the words of the line, and each pair was read by hand. Examples: `TestAnExpiredAccountHasNoAccessFromThatMoment` for DAT-171, `TestPollingDoesNotKeepASessionAlive` for SEC-180, `TestAnEndpointSwitchesOffAfterFiftyFailures` for SEC-444.
3. **This wave built it**, failing test first.

A few lines were built under another name: `block_templates` became `saved_blocks` (DAT-077, spec 027), and `pageTag` became `pageLink` (DAT-087, spec 023).

Lines with none of these three stay as they were, marked with their wave, so nobody reads them as proven. They are listed at the end.

## What this wave builds

| Line | What | Load budget (ARC-238) |
|---|---|---|
| SEC-503, SEC-511, SEC-502, SEC-514 | The PDF helper answers only with its password (`DIFFLUENCE_PDF_PASSWORD`), never fetches an address, runs no script in pages, has no webhook, runs read-only without capabilities, and stops a request after 120 seconds. The image is pinned by digest. Without a password set, an existing install keeps working | No query. One header per helper call |
| SEC-107 | Passwords are normalised to Unicode NFC before hashing and counting. A hash from before is still checked as typed, and is stored again in the new form after a match | No query. A second hash only for a password that is not already NFC |
| SEC-106 (partly) | A new password may not hold the name of the product. The organisation name follows when wave X gives an instance one | No query |
| SEC-319 (partly) | The characters that change reading order are removed from page titles, file names, display names (local, from the provider, from SCIM) and space names | No query |
| DAT-015, GAP-019, PRM-177 | The schema refuses deleting a parent page alone (migration 0035, `ON DELETE NO ACTION`). The purge deletes whole branches in one statement, so it keeps working | No query in normal use |
| SEC-178 | `diffluence session end --email ADDR` or `--all` logs one person or everyone out without disabling anyone, audited as `sessions.ended`. `--all` must be given on purpose | Saving class: one lookup, one delete, one audit row |
| SEC-150 | A step-up through the identity provider asks with `prompt=login` and `max_age=0`, and is refused unless the token's `auth_time` is after the step-up started, less one minute for clocks | No query beyond the existing flow |
| SEC-1025 | A test plants a wrong password, the old and new password, a token and the session cookie, and finds none of them in the log, the audit log or the outbox | Test only |
| SEC-149, SEC-152, SEC-433, SEC-507, SEC-657, SEC-723 | The identity provider guide says how session lifetimes relate and who gets an account. The install guide says what each helper reaches and which way out the app needs. `docs/operations/incidents.md` is the runbook, with how to replace every secret | Documents |

`golang.org/x/text` moves from an indirect to a direct requirement. It was already part of the build, so no new module comes in.

## What stays open, and why

- **SEC-136**, a passkey counter that falls: proving it needs a test authenticator that reports a falling counter, which the tests do not have yet.
- **SEC-243**, no GET route writes: not true by design. Opening a page records it in the recent list and, when analytics is on, in the daily counts.
- **SEC-153**, the `__Secure-` prefix on the cookie of the login flow: small, and it changes the cookie a running login uses; left for a wave that touches login.
- **SEC-106** for the organisation name: waits for wave X.
- **SEC-319** for group names, labels and the other short names: titles, files, people and spaces were the ones a person sees as links.
- **Roles people make themselves (PRM-036's own sets beyond the fixed ones)**: their own wave after wave X.
- **The audit tail**: waits for the product owner.
- **Real servers**, a lawyer, or money: SEC-1034, SEC-1035, SEC-1040 and the like.
- **Lines still marked with their wave** because no evidence was found: DAT-013, DAT-026, DAT-099, PRM-081, PRM-087, PRM-095, PRM-138, PRM-140, PRM-142, PRM-143, PRM-144, PRM-147, PRM-149, PRM-153, PRM-156, PRM-159, PRM-163, SEC-013, SEC-127, SEC-138, SEC-190, SEC-222, SEC-260, SEC-275, SEC-309, SEC-351, SEC-363, SEC-416, SEC-544, SEC-915, SEC-943. Most are probably built, but this study could not prove it. A next pass should write a test per line, or find one.

## Counts

| | Built | Partly | Marked with a wave | Phase | New | Later | Total |
|---|---|---|---|---|---|---|---|
| 05 before | 152 | 7 | 125 | | 10 | 1 | 295 |
| 05 after | 265 | 6 | 17 | | 9 | 1 | 298 |
| 06 before | 153 | 39 | 119 | 8 | 99 | 11 | 429 |
| 06 after | 284 | 43 | 14 | 5 | 68 | 15 | 429 |

The total of 05 is three lines higher than before: the earlier count was made by hand, this one by a script that reads every numbered line and table row outside chapter 12.
