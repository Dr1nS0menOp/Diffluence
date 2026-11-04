# 06: Security and compliance

## Biggest risks, and how we handle them

| Risk | Control |
|---|---|
| **Broken access control** (the most common serious bug in wikis) | One central permission function, the generated test matrix (05), and an `authz` annotation required on every endpoint, enforced by CI |
| **Cross-site scripting** via content, attachments, embeds | Server-side sanitising (allow-list), strict Content Security Policy with nonces, Trusted Types, attachments served as downloads from a **separate origin** with `nosniff` |
| Account takeover | Argon2id password hashing, TOTP and passkeys, rate limiting and lockout, breached-password check (offline list), OIDC with PKCE |
| Session theft / CSRF | Opaque session tokens stored hashed, `HttpOnly` + `Secure` + `SameSite=Strict` cookies, CSRF token on state changes, short idle timeout, revocation on password or role change |
| Injection | Parameterised SQL only (sqlc), no string-built queries, strict input validation at the API edge |
| Supply chain | Few dependencies, `govulncheck`, Trivy, licence allow-list, Renovate, pinned GitHub Actions, signed releases |
| Malicious imports | Imports run in the worker with size, time and memory limits. Parsers are fuzz-tested. Zip-slip and zip-bomb protection. |

A **threat model** (STRIDE per component) lives in `docs/security/threat-model.md` and is updated every phase.

## Encryption

**In transit**
- TLS 1.3 at the edge. Go's TLS stack uses hybrid post-quantum key exchange (X25519 + ML-KEM) by default.
- Optional mutual TLS between internal components (api ↔ NATS ↔ worker)

**At rest: three layers**
1. **Disk:** LUKS or ZFS native encryption on hosts (operator responsibility, documented)
2. **Application, envelope encryption:** every attachment and every secret field (TOTP seeds, OIDC client secrets, API tokens, webhook secrets) is encrypted with **AES-256-GCM** using its own data key. Data keys are wrapped by a key-encryption key (KEK) held in a **KMS**: OpenBao/Vault Transit, a PKCS#11 HSM, or for small installs a key file outside the database. KEK rotation runs as a background job without downtime.
3. **Backups:** encrypted with AES-256 before they leave the host

**Trade-off, stated honestly:** page *text* is protected by layers 1 and 3, not individually encrypted in the database. Otherwise Postgres full-text search is impossible. An optional "encrypt page content" mode for very sensitive spaces can come later, with search limited to titles.

## Audit log

**What is recorded:** every authentication event (login, logout, failure, MFA change), every content action (create, edit, move, delete, restore, export, download), every permission change, every admin and system action (settings, users, keys, backups, upgrades), and every API call made with a token. **Page views are not audit events.** They come later as a separate, privacy-aware statistics feature (03, Analytics).

**Each event contains:** time (UTC), actor (user, service or system), on-behalf-of (for apps), action, object type and id, space, result (success or denied), source IP, user agent, request id, and for changes a before/after summary.

**Integrity:** append-only (the application's database role cannot update or delete rows), each row includes a **hash of the previous row** (a hash chain), and a daily **Ed25519-signed checkpoint**. Any tampering breaks the chain and is detected by a scheduled verification job.

**Access and export:** filter in the admin console and export to CSV. Later: syslog (RFC 5424), CEF, GELF (Graylog), Microsoft Sentinel (Logs Ingestion API) and webhooks. Audit retention is **separate** from content retention, so deleting a page never deletes its audit trail.

## Privacy (GDPR / AVG)

- Data-subject export (everything about a user) and anonymisation of former users, while keeping audit integrity through pseudonymous ids
- Telemetry is off by default
- Data processing documentation for operators in `docs/compliance/`

## Supporting customers' frameworks

Features map to common controls (BIO, ISO 27001 Annex A, NIS2 duties): access control, logging, encryption, backup, vulnerability handling. A mapping table ships with 1.0 so operators can use it in their own audits.

## Secure development and disclosure

- Security checks are part of CI (see 09). An external **penetration test before 1.0**.
- `SECURITY.md`, a `security.txt` file, GitHub private vulnerability reporting, CVE assignment via GitHub, and fixes released within 7 days for critical issues
- Container images: distroless, non-root, read-only root filesystem, SBOM attached, signed with cosign, SLSA build provenance
