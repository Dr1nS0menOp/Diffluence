# Development

Written for: developers and build sessions. Everything runs in containers, so the host needs only Docker. On Windows, run these from WSL.

```sh
scripts/dev up                 # database and toolbox (Go, Node)
scripts/dev sh -c "cd web && npm ci"
scripts/dev make check         # what CI runs: format, vet, staticcheck, govulncheck, licences, frontend, tests
scripts/e2e                    # real binary, PostgreSQL and Chromium; add -g "passkey" for a selection
scripts/dev sh scripts/ci-local.sh   # the CI steps in a fresh clone of what is committed
scripts/dev make mutation      # breaks security rules on purpose and fails if the tests stay green
scripts/dev make fuzz          # fuzzes the parsers
scripts/dev down
```

Proof against real systems, each with its own containers:

```sh
bash scripts/restore-drill.sh                                    # back up, destroy, restore
scripts/dev bash scripts/upgrade-test.sh <old binary> <new binary>
docker build -t diffluence:local . && bash scripts/idp-test/run.sh  # real Authentik
```

## Conventions that tests enforce

- Every route has an access rule. `docs/api/routes.txt` lists them all, and a test fails when it differs from the code: regenerate with `UPDATE_ROUTES=1 go test ./internal/app/ -run RouteTable` and review the diff.
- All UI text goes through `web/src/locales/en.json`: `npm run strings` fails on hard-coded text and unknown keys.
- No `dangerouslySetInnerHTML`, enforced by ESLint.
- Every browser test fails on a Content Security Policy or Trusted Types violation.
- Licences are checked by `scripts/check-licenses.mjs`.

When you add a security rule, add a line to `scripts/mutation-check.sh` so we know a test really guards it.
