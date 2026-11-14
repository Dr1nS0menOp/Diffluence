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
- A module names only its own tables in SQL. `internal/db/boundary_list_test.go` gives every table one owning module, and lists each place where a module still names another module's table, with how often and why (spec 072). A new table without an owner fails, and so does a new mention: call the owner's Go API instead, such as `pages.Title`, `spaces.HomePage` or `settings.Load`, or add a small function there. A list that joins a display column for every row may stay a join (ARC-027): then add it to the list with its reason. When a mention goes away, lower its number.

## Migrations

The rules are at the top of `internal/db/migrate.go`: numbered files in `internal/db/migrations/`, a first line `-- phase: expand` or `-- phase: contract`, never edited once merged.

**Placeholders.** Several files hold only `SELECT 1;`, such as `0009_reserved_wave_e.sql`, `0013_reserved.sql` and `0052_reserved.sql`. Builders working side by side are each given a number, and the runner wants versions without gaps from 0001, so a branch that needs 0059 while 0056 to 0058 are still open adds those three as placeholders. When the branches meet, the placeholder that arrived first stays. They cannot be removed afterwards:

- every installation that ran one recorded its version and checksum in `schema_migrations`;
- a removed or changed file makes the runner refuse that database (`an applied migration was modified`), and a gap makes it refuse the release;
- a placeholder costs nothing: it runs once, in a millisecond, and changes nothing.

So they stay, and a placeholder's comment saying "remove when" is out of date. A new placeholder is a file with LF line endings, `-- phase: expand`, the SPDX line, a line saying it holds the number, and `SELECT 1;`.

**Contract migrations.** None has shipped yet. `TestAContractMigrationRemovesAColumnOnACopyOfAnInstallation` in `internal/db` runs the whole removal path on a copy of an installed database that holds data: it adds a column in an expand migration, copies the database, drops the column in a contract migration on the copy, and checks that the run waits until it is allowed, that the data around it stays, that the previous release then refuses the copy, and that the original is unchanged. `scripts/upgrade-check.sh` covers the expand half between two real releases.

When you add a security rule, add a line to `scripts/mutation-check.sh` so we know a test really guards it.
