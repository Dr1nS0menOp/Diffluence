# 06.8: Cryptography, keys and secrets

## 8.1 What is in use

ASVS 11.1.2 asks for an inventory. Primitives come from the Go standard library and `golang.org/x/crypto`. We write none ourselves.

| Purpose | What | Key or secret | Where it lives | Status |
|---|---|---|---|---|
| Passwords | Argon2id, 19 MiB, 2 passes | Salt per password | `local_credentials` | built |
| Session tokens | 256 random bits, SHA-256 stored | None | `sessions` | built |
| Challenges for login, OIDC, passkeys | 256 random bits, SHA-256 stored | None | `auth_challenges` | built |
| TOTP | HMAC-SHA-1, 6 digits, 30 seconds (RFC 6238) | Seed, sealed | `totp_credentials` | built |
| Recovery codes | 80 random bits, SHA-256 stored | None | `recovery_codes` | built, see SEC-124 |
| Secret fields | AES-256-GCM, own data key per value, wrapped by the KEK, purpose bound as extra data | KEK | `master.key` | built |
| SCIM tokens | 256 random bits, SHA-256 stored | None | `scim_tokens` | built |
| Audit chain | SHA-256 over the previous hash and the event | None | `audit_events` | built |
| Audit checkpoints | Ed25519, key derived from the KEK with HKDF-SHA-256 | Derived | `audit_checkpoints` | built |
| Passkeys | What the authenticator offers, through `go-webauthn` | Public keys | `webauthn_credentials` | built |
| ID tokens | Signature checked through `go-oidc` | The provider's public keys | Memory | built |
| Unsubscribe tokens | Sealed with the keyring | KEK | In the link | wave C |
| Webhook signatures | HMAC-SHA-256 | Secret per endpoint, sealed | `webhook_endpoints` | wave H |
| Bundles | SHA-256 per file, Ed25519 signature | To be defined | With the bundle | wave G |
| Analytics in counts mode | HMAC with a salt per day | Salt, deleted daily | Memory or a table | wave H |

- **SEC-700** · built · All random secrets come from `crypto/rand`, with at least 256 bits, except recovery codes (ASVS 11.5.1, SEC-124).
- **SEC-701** · built · Comparisons of codes and hashes are constant time where a guess could be measured (ASVS 11.2.4). Tokens are looked up by their hash, so a timing difference shows nothing useful.
- **SEC-702** · built · A sealed value starts with a format byte, Argon2id hashes carry their parameters, and passkeys carry their algorithm, so each can change later (ASVS 11.2.2).
- **SEC-703** · built · AES-GCM uses a fresh random nonce and a fresh data key for every value, so nonces do not repeat under one key (ASVS 11.3.2 to 11.3.4).
- **SEC-704** · built · SHA-1 appears only inside HMAC for TOTP, which is what RFC 6238 and every authenticator app use. It is not used for anything that needs collision resistance (ASVS 11.4.1).
- **SEC-705** · new · ASVS 11.1.1 asks for a written key management policy, and this table is the start. Name for each key who holds it, how it is rotated, and what happens when it is lost. Belongs here, before 1.0.

## 8.2 Keys

**What can go wrong:** the key lies next to the data, a key leaks and cannot be replaced, one key does too many jobs.

- **SEC-710** · built · The KEK is a 32-byte file with mode 0600, outside the database. When several nodes run, `DIFFLUENCE_MASTER_KEY_FILE` is required, and the same key is shared.
- **SEC-711** · built · By default `master.key` sits in the data directory, next to the stored files. The guide says to back it up apart. See SEC-636. A backup now keeps `master.key` encrypted, with the key apart.
- **SEC-712** · phase 3 · A KMS can hold the KEK: OpenBao or Vault Transit, or an HSM (ADR-011, ASVS 13.3.3).
- **SEC-713** · new · **Medium.** The KEK cannot be rotated. `format` allows it, but there is no key id in a sealed value, and no job to re-wrap values. After a leak the only way out is new TOTP for everyone. Add a key id, and a background job that re-wraps (ASVS 13.3.4). Belongs in phase 3, with the KMS.
- **SEC-714** · new · The signing key for audit checkpoints is derived from the KEK. It is one secret doing two jobs. Whoever has the KEK can forge checkpoints as well as read seeds, so rotating the KEK also changes that key. `VerifyCheckpoints` checks against the current key only, although each row stores its public key. Verify each checkpoint against a list of known keys. Belongs in phase 3.
- **SEC-715** · later · An external anchor for the checkpoints: the signed head goes to a system the instance cannot change. See SEC-833.

## 8.3 Secrets of the installation

**What can go wrong:** a secret in a log, in Git, or in the process list.

- **SEC-720** · built · Secrets stay out of the database on purpose: database addresses, the OIDC client secret and the mail password come from the environment or a file. The blueprint said OIDC secrets would be sealed in the database, and this simpler way replaced it.
- **SEC-721** · built · The access log has no query strings or bodies, and no secret is written to the audit log. Tests and review watch for this.
- **SEC-722** · new · `.gitignore` covers `secrets/` and `.env`, and GitHub secret scanning should be on. A scan in CI would make that a check instead of a setting (ASVS 13.3.1). Low.
- **SEC-723** · new · Write down how each secret is rotated and how often (ASVS 13.1.4, 13.3.4): database roles, OIDC client secret, mail password, service secrets.

## 8.4 Encryption at rest and on the way

**What can go wrong:** a stolen disk or backup, a sniffed connection.

The blueprint names three layers at rest. **Disk:** LUKS or ZFS on the host, which is the operator's job. **Application:** secrets in the database are sealed with AES-256-GCM, and attachments are not yet (SEC-725). **Backups:** AES-256 before they leave the host, which is not built (SEC-636).

- **SEC-725** · built · Attachments and derived files are encrypted with their own key (ADR-011, spec 032). The key sits in the file's own header, wrapped by `master.key`, so `encryption_key_id` stays empty. A wrong or missing key, a changed file or a file cut short fails clearly and serves nothing. Without `master.key`, no file can be read: back it up apart from the data, as for TOTP seeds.
- **SEC-726** · built · The trade-off stays honest: page text is not encrypted in the database, because full-text search needs it. Disk encryption protects it today, and backup encryption will once SEC-636 is done.
- **SEC-727** · later · An option to encrypt the content of very sensitive spaces, with search limited to titles.
- **SEC-728** · partly · TLS is the job of the proxy, so the blueprint's sentence about Go's hybrid post-quantum exchange only holds if that proxy offers it. Go does it where Go is the TLS client: the identity provider and, later, mail and webhooks. The guide should advise TLS 1.3 at the proxy (ASVS 12.1.1, 12.1.2).
- **SEC-729** · phase 3 · Mutual TLS between nodes (ASVS 12.3.5, Level 3).
- **SEC-730** · later · A plan for post-quantum signatures on checkpoints and bundles (ASVS 11.1.4, Level 3). Nothing needs it yet.
