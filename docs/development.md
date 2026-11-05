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

## Mail while developing

[Mailpit](https://mailpit.axllent.org) (MIT) catches all mail in a web screen, so nothing leaves the machine. It is optional and only starts with its own profile:

```sh
docker compose -f compose.dev.yaml --profile mail up -d mailpit
```

The screen is at <http://localhost:18025>. Set `DEV_MAIL_PORT` to use another port.

To send to it from a Diffluence started with `compose.yaml`, put it on the same network, and set these in `.env`. Recreate the container with `docker compose up -d` afterwards:

```sh
docker network connect diffluence_default diffluence-dev-mailpit-1
```

```sh
DIFFLUENCE_SMTP_HOST=diffluence-dev-mailpit-1
DIFFLUENCE_SMTP_PORT=1025
DIFFLUENCE_SMTP_TLS=none
DIFFLUENCE_SMTP_FROM=diffluence@example.test
```

Then Administration, Mail, Send test should show up in Mailpit. `none` means no encryption, which is only fine because this mail never leaves the machine. Automated tests use a fake server of their own, and need none of this.

## Conventions that tests enforce

- Every route has an access rule. `docs/api/routes.txt` lists them all, and a test fails when it differs from the code: regenerate with `UPDATE_ROUTES=1 go test ./internal/app/ -run RouteTable` and review the diff.
- All UI text goes through `web/src/locales/en.json`: `npm run strings` fails on hard-coded text and unknown keys.
- Everything visible follows `docs/design/README.md`: `npm run design` checks tokens, contrast, icons and more, and `web/e2e/0-design.spec.ts` compares every screen with its approved picture. After an intended change of look, run `scripts/e2e design.spec.ts --update-snapshots`, and look at the pictures in `web/e2e/design-baselines/` before committing. Pictures depend on the browser image, so updating Playwright means new pictures.
- No `dangerouslySetInnerHTML`, enforced by ESLint.
- Every browser test fails on a Content Security Policy or Trusted Types violation.
- Licences are checked by `scripts/check-licenses.mjs`.

When you add a security rule, add a line to `scripts/mutation-check.sh` so we know a test really guards it.
