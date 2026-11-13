# Threat model

Written for: reviewers and a future penetration tester. Updated every phase (blueprint 06). Version: phases 1 and 2, with the changes up to spec 070 (13 November 2026).

**Assets:** page content, attachments, credentials and sessions, the permission model, the audit trail.

**Attackers:** an outsider on the network; a logged-in user reaching beyond their rights (the most likely case); a guest; a malicious file or Markdown import; someone with database access rewriting history; a compromised identity provider account.

## STRIDE per component

| Component | Threat | Control | Proof |
|---|---|---|---|
| Permissions | Elevation: a list or export shows more than direct access | One rule; Go for decisions, SQL for lists, compared with each other and with a literal transcription | Matrix test, 102,960 cases; mutation check |
| API routes | Elevation: a new route forgets its check | Routes without a rule refuse to register; modules check permissions themselves | Route walker: stranger gets 404, viewer 403, on every route; `routes.txt` in review |
| Existence of pages | Information disclosure | No view means "not found", never "forbidden" | Route walker |
| Trash and moves | Elevation: a page loses an inherited restriction | Restoring waits for the parent; a purge takes everything below; moves that drop a restriction need the permission; deleting a group keeps the restriction closed | Tests plus mutation check |
| Documents | Tampering: stored XSS | Allow-list rebuild on the server; no HTML stored; links limited to http, https, mailto and relative paths; no external images | Unit tests and fuzzing; browser test |
| Web app | Tampering: XSS if everything else fails | CSP with no inline script, Trusted Types, no `dangerouslySetInnerHTML` | Every browser test fails on a violation; a test confirms an inline script cannot run |
| Attachments | Tampering: a file executes in our origin | Type from content; only PNG, JPEG, GIF and WebP inline; all else a download; `sandbox` policy and `nosniff` | Test with HTML posing as PNG, and SVG |
| Imports | Denial of service, path traversal | Size, count and unpacked limits, enforced while reading; paths never touch disk; runs in the worker | Tests with a zip bomb and `../../etc/passwd`; fuzzing |
| First setup | Elevation: a stranger reaches a fresh install first and becomes its administrator | One-time code only in the server's log; an hour, once, 10 wrong tries; 10 tries per address per 15 minutes; code checked first; one transaction under a lock; closed for good after use (spec 038) | Tests, mutations |
| Login | Spoofing: brute force | Argon2id; limits per account and per address, also on every route that checks a password; failed logins audited; a mail on every change to a password, factor or passkey | Tests, mutations |
| Account mail | Denial of service: flooding a person's inbox | Reset and sign-up mail limited per address and per recipient, with the same answer either way | Test, mutation |
| TOTP | Spoofing: replay, stolen database | One use per time step; challenge dies after 5 tries; seeds sealed with AES-256-GCM, recovery codes hashed | Tests plus mutation check |
| OIDC | Spoofing: account takeover, login CSRF | PKCE, state, nonce, flow tied to the browser; link by stable id, never e-mail alone; accounts with a password never link | Tests; real Authentik 2026.8.5 |
| SCIM | Spoofing: stolen token | Tokens hashed, shown once, revocable, audited per token; filters refused rather than ignored, also on attributes a route does not read | Tests, fuzzing; real Authentik 2026.8.5 |
| Sessions | Spoofing: theft | Hashed tokens, `HttpOnly`, `SameSite=Strict`, `__Host-` on https, `__Secure-` on the login flow; idle and absolute limits; ended on password change and disable; a loud warning when the public address is plain http | Tests, mutations |
| Editing together | Tampering, spoofing: a client that skips the editor | Stored state passes the gate; every browser removes what its schema and link rules refuse; the service sets the name in every cursor from the session; the service holds a signed ticket for one person and one page, never the session cookie (spec 067) | Hostile client through the real service; unit tests; mutations |
| Cross-site requests | Tampering | Go's cross-origin protection, strict cookies | Test |
| Audit log | Repudiation, tampering | Hash chain, sealed within a second of saving (ADR-041); the application role cannot update or delete; daily verification; Ed25519 checkpoints every five minutes with a key outside the database, checked against a witness outside it (ADR-039) | Tests plus mutation check |
| Audit log, against the server's own process | Tampering: code running in the server's container rewrites history | In Compose the server holds only the application role: no owner's address, so no migration, trigger or delete; the backup service signs the checkpoints with a key derived from the backup key, endorsed by the installation's key at a position of the chain; the database refuses a checkpoint of a key no longer in force (spec 070, ADR-044). Holds against the server's process alone, and against the owner's address alone. Whoever holds both `master.key` and the owner's address can endorse a key of their own; the witness lines show it | Tests across the change of key, a forged endorsement and anchor; mutation check |
| Administrators | Elevation: quiet access to content | No automatic content access; recover access needs a reason and is audited; system administrator can only be granted by one | Tests |
| Roles | Elevation: an administrator, or a role made in the console, reaches past its rights | Roles are sets of named permissions, checked by the same rule as grants; acting on an account or group that holds system administration needs that role; no password is set for an account of an identity provider; a reset goes by link (spec 066) | Tests, mutations |
| Login with a second factor | Spoofing: a password alone opens an account that has a second factor | After the password, every second factor of the account is asked for: a code, or a passkey bound to the challenge of that password (spec 069) | Tests; browser test with a virtual authenticator |
| API tokens | Elevation: a token does more than its scopes | The scope a request needs comes from the pattern of its route, never from words in the address; admin routes need the admin scope; a token never exceeds its owner (spec 069) | Tests over every export route, mutations |
| Backup worker | Tampering, denial of service: backups that silently stop or are read by others | A separate service makes a backup every 4 hours, encrypted with a key apart from the backups, and notes each one; an upgrade waits for a recent backup; failures are mailed to administrators (ADR-040, ADR-042) | Tests; upgrade test |
| Restore from the console | Elevation, tampering: one session rewinds every account, grant and page | Experimental and off by default (ADR-043); system administrators only; a fresh login plus a second factor or passkey; the name of the installation typed; codes limited and audited; one open restore at a time; a safety backup first; nodes wait until it ends (specs 059, 066). The backup service goes on only with a one-time code it wrote to its own log, typed into the console (three tries, 30 minutes), and checks its seal on the restore at every step and on the safety backup before the way back; every refusal is recorded, so a request the server's role writes straight into `ops.restores` is not an order (spec 070) | Tests, mutations; a request written by the application role is refused and recorded; the restore drill |
| Experimental parts | Elevation, information disclosure: a part nobody chose is reachable | Automation, databases, bundles and the console restore are off until an administrator turns each on; their routes answer 404 `experimental_off`; public sites and analytics have their own switches, off at the start | Tests on a fresh installation and after an upgrade, mutations |
| Supply chain | Tampering | Few dependencies; licence gate; `govulncheck`, `npm audit`, Trivy; distroless non-root image | CI |

## Accepted for now, with a date

| Risk | Why accepted | Planned |
|---|---|---|
| Page text in the database is not encrypted at rest | Search needs it; files are encrypted since spec 032 | Disk encryption on the host |
| Attachments are served from the same origin as the app | `sandbox`, `nosniff` and forced download cover the known attacks; a separate origin is defence in depth | Phase 3 |
| Rate limits per minute are per node | One node is what is claimed (ADR-043); the stop after a long run of wrong passwords is stored with the account (ADR-038) | When several nodes are proven |
| No breached-password check | Length rule only | Before 1.0 |
| Every denied request writes an audit event | A flood of denied requests can fill the log and slow writes down | Rate limit per user, phase 3 |
| Outside Compose (Helm, the LXC script, the packages) the serving process still holds the owner password of the database, for migrations, and signs the audit checkpoints. In Compose the backup service holds both since spec 070 (ADR-044) | Those layouts run one process; they keep working as before | The same split for each of them, in its own wave |
| Not yet reviewed by anyone but the author | Automated checks stand in | One outside reader for permissions, login and restore at 1.0 (ADR-043) |

## What each control holds against

Five kinds of attacker, from the most likely to the most powerful. "Holds" means the control stops that attacker; "no" means that attacker can get around it, and something else must stop them.

| Control | A member | An administrator | SQL injection | A compromised app process | The database owner |
|---|---|---|---|---|---|
| The permission rule (Go and SQL) | Holds | Holds for content: no automatic access, recover access is audited | Holds: every value is a parameter, and no query is put together at run time | No: the process can read the tables | No |
| Roles and system administration | Holds | Holds: system administration is needed to act on it | Holds | No | No |
| Second factor and passkeys | Holds for their own account | Holds: resets go by link, and a factor is never shown | Holds: seeds are sealed, recovery codes hashed | No: it could open a session itself | No |
| Experimental switches | Holds: only an administrator turns a part on | No: an administrator decides them | Holds | No | No |
| Restore from the console | Holds | Holds unless they are system administrator, with a fresh login and a second factor | Holds | Holds in Compose: the backup service goes on only with the code in its own log and its own seal (spec 070) | No |
| Audit chain and checkpoints | Holds | Holds: the application role cannot change or delete events | Holds | Partly: it can add events, not rewrite sealed ones; in Compose it holds no owner's address and no key that signs a checkpoint after the change point (spec 070) | Partly: a change after a checkpoint shows against the witness outside the database; without `master.key` or the backup key it cannot bring in a key that verifies |
| Encryption of files and secrets | Holds | Holds | Holds | No: it holds the keys | Holds without `master.key` |
| Backups | Holds | Holds: the backup key is apart from the backups | Holds | Holds: backups are made by a separate service | No: it can change what is backed up |
