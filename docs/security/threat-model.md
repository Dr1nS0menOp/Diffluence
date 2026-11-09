# Threat model

Written for: reviewers and a future penetration tester. Updated every phase (blueprint 06). Version: phases 1 and 2, with the changes of spec 042.

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
| Editing together | Tampering, spoofing: a client that skips the editor | Stored state passes the gate; every browser removes what its schema and link rules refuse; the service sets the name in every cursor from the session | Hostile client through the real service; unit tests; mutations |
| Cross-site requests | Tampering | Go's cross-origin protection, strict cookies | Test |
| Audit log | Repudiation, tampering | Hash chain; the application role cannot update or delete; daily verification; daily Ed25519 checkpoint with a key outside the database | Tests plus mutation check |
| Administrators | Elevation: quiet access to content | No automatic content access; recover access needs a reason and is audited; system administrator can only be granted by one | Tests |
| Supply chain | Tampering | Few dependencies; licence gate; `govulncheck`, `npm audit`, Trivy; distroless non-root image | CI |

## Accepted for now, with a date

| Risk | Why accepted | Planned |
|---|---|---|
| Page text in the database is not encrypted at rest | Search needs it; files are encrypted since spec 032 | Disk encryption on the host |
| Attachments are served from the same origin as the app | `sandbox`, `nosniff` and forced download cover the known attacks; a separate origin is defence in depth | Phase 3 |
| Rate limits are per node | One node until phase 3 | Phase 3 |
| No breached-password check | Length rule only | Before 1.0 |
| The audit chain uses one global lock | Fine at wiki scale | Measured in the phase 3 load test |
| Every denied request writes an audit event | A flood of denied requests can fill the log and slow writes down | Rate limit per user, phase 3 |
| Not yet reviewed by anyone but the author | Automated checks stand in | External penetration test before 1.0 |
