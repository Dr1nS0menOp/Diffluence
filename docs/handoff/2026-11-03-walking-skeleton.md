# Handoff 2026-11-03: walking skeleton (phase 0 + start of phase 1)

## Done
- Repo scaffold: licence (Apache 2.0), NOTICE, README, CONTRIBUTING (DCO + clean-room), SECURITY, CLAUDE.md, PR template, Renovate, CI workflow, DCO check script, Dockerfile (distroless, non-root), compose.yaml, Makefile, blueprint in `docs/blueprint/`
- Go backend skeleton (Go 1.26), about 2,900 lines of code plus 1,000 lines of tests:
  - `diffluence serve | migrate | user create-admin | audit verify | version`
  - Roles `all` / `api` / `worker`. Embedded NATS only with `all`.
  - Migration runner with expand/contract phases, checksums, advisory lock, lock_timeout + retry, and N/N+1 tolerance (an older node accepts a newer database)
  - Audit log: append-only via database triggers, SHA-256 hash chain, verification (detects changed payloads, changed columns and deleted rows), list with filters, CSV export with formula-injection protection
  - Transactional outbox, with the relay publishing to JetStream with de-duplication
  - Local accounts: Argon2id (OWASP parameters, automatic rehash), sessions stored as SHA-256 hashes, idle and absolute timeout, `__Host-` cookie on HTTPS
  - Router that **refuses routes without an access rule**. A test proves every non-public route returns 401 without a session.
  - Login rate limiting (per IP and per account), cross-origin protection (Go `CrossOriginProtection`), strict security headers, trusted-proxy aware client IP
  - `/livez` and `/readyz` (checks database, migrations and queue), graceful shutdown
- `make check` is green: gofmt, vet, staticcheck, govulncheck, 41 tests with the race detector against real PostgreSQL 16 and embedded NATS
- Ran the real binary: created an admin, logged in, read the audit log, verified the chain, and saw the outbox events published by the worker

## Not verified here
- `docker build` and `docker compose up`: no Docker daemon in Claude's workspace. CI builds and scans the image on the first push.
- The GitHub Actions workflow itself. It runs on the first push.

## Known limitations (deliberate, for later)
- The audit chain uses one global lock, so audited writes take turns. That is fine at wiki scale. The phase 3 load test will tell whether batching is needed.
- Daily signed (Ed25519) audit checkpoints are not built yet
- Contract migrations need `diffluence migrate --contract`. The blueprint says "no manual steps", so next is a node-version registry that runs them automatically once every node is on the new version.
- Rate limiting is in memory per node
- `user create-admin` shows the password while typing. Use `read -s PW; echo "$PW" | diffluence user create-admin ...`.
- In production the app must connect with a database role that does **not** own the tables, so it cannot disable the audit triggers. This goes into the install guide.
- No groups yet. Global permissions are per user only.
- No cleanup jobs yet (expired sessions, published outbox rows)
- Go module path `github.com/diffluence/diffluence` is a placeholder. Change it with one search-and-replace once the GitHub organisation exists.

## Next (in this order)
1. **Spec 001: spaces and pages** (with the product owner), then schema, API and permission checks
2. Frontend scaffold: React + TypeScript + Vite + TipTap, login page, translation setup
3. Worker jobs: purging sessions, outbox rows and the trash
4. Node registry, automatic contract migrations, and the N/N+1 upgrade test in CI
5. TOTP for local accounts

## For the product owner: putting it on GitHub
```sh
cd diffluence
git init -b main
git config user.name "Your Name"; git config user.email "you@example.com"
git add .
git commit -s -m "Walking skeleton: binary, migrations, audit log, outbox, local login"
gh repo create <your-org>/diffluence --private --source=. --push
```
Then in GitHub: Settings → Branches → protect `main` (require a pull request and the `ci` checks), enable private vulnerability reporting and secret scanning, and install the Renovate app.

## For the product owner: how to test (plain language)
Without Docker, on a machine with Go 1.26 and PostgreSQL:
1. `export DIFFLUENCE_DATABASE_URL=postgres://user:pass@localhost:5432/diffluence`
2. `make run` → the log says "diffluence started"
3. In a second terminal: `read -s PW; echo "$PW" | ./bin/diffluence user create-admin --email you@example.com --name "You"`
4. `curl -s localhost:8080/readyz` → `{"status":"ready"}`
5. Log in: `curl -s -c jar -H 'Content-Type: application/json' -d '{"email":"you@example.com","password":"…"}' localhost:8080/api/v1/auth/login` → your user is shown
6. `curl -s -b jar localhost:8080/api/v1/admin/audit-events` → your login is in the list
7. Try a wrong password 11 times → the 11th answer is "Too many login attempts"
8. `./bin/diffluence audit verify` → "audit chain OK"
9. Press Ctrl+C in the first terminal → the log says "shutting down" and then "stopped"
