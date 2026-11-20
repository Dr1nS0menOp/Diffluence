# 095: The name Kenwerk

Status: built
Phase: hardening before 1.0 (wave KR1)

## Problem

On 17 November 2026 the product owner renamed the product from Diffluence to **Kenwerk**: the working name sat too close to the name of a commercial wiki, and people would confuse the two. The display name is "Kenwerk" and the identifier is `kenwerk`. The product is not at 1.0, so existing installs need no way across: they start fresh. Until 1.0 the API and every other part may change.

The name lives in far more places than the title bar. This spec lists each place, decides what becomes of it, and keeps the rename one mechanical step, so the branches that are open today can take the same step before they rebase.

## Study

On main at 21368c4 the word appeared on 4,123 lines of 866 of the 1,878 tracked files, in three spellings only: `diffluence`, `Diffluence` and `DIFFLUENCE`. No other spelling occurs, so a substitution that keeps the case covers every place. 34 tracked paths carry the name.

| Where | Before | After |
|---|---|---|
| Go module path | `github.com/diffluence/diffluence` | `github.com/kenwerk/kenwerk` (D2) |
| Command and binary | `cmd/diffluence`, `diffluence serve`, `/usr/local/bin/diffluence` | `cmd/kenwerk`, `kenwerk serve`, `/usr/local/bin/kenwerk` |
| Images built here | `diffluence:local`, `diffluence-backup:local`, `diffluence-collab`, `diffluence:ci` | `kenwerk:local`, `kenwerk-backup:local`, `kenwerk-collab`, `kenwerk:ci` |
| Released images | `ghcr.io/<owner>/diffluence`, `-backup`, `-collab` | `ghcr.io/<owner>/kenwerk`, `-backup`, `-collab` |
| Compose | project `diffluence-dev` in `compose.dev.yaml`; service `diffluence` in `compose.yaml` and `scripts/idp-test`, `scripts/stress` | `kenwerk-dev`; service `kenwerk` |
| Settings from the environment | `DIFFLUENCE_*` (72 names, such as `DIFFLUENCE_DATABASE_URL`) | `KENWERK_*`; the old names are not read (D3) |
| Configuration file | `/etc/diffluence/diffluence.yaml`, `deploy/diffluence.example.yaml` | `/etc/kenwerk/kenwerk.yaml`, `deploy/kenwerk.example.yaml` |
| Data directory | `/var/lib/diffluence` | `/var/lib/kenwerk` |
| Database and roles | database `diffluence`, roles `diffluence_owner` and `diffluence_app`, development role `diffluence` | `kenwerk`, `kenwerk_owner`, `kenwerk_app`, `kenwerk` |
| Database settings of a transaction | `diffluence.release`, `diffluence.permissions_counted` | `kenwerk.release`, `kenwerk.permissions_counted` |
| `application_name` of every connection | `diffluence` | `kenwerk` |
| Migrations | the files above name the settings, a backup's folder pattern and a notice | the same files with the new name (D4) |
| Backups | folder `diffluence-<time>Z`, service `diffluence-backup` | `kenwerk-<time>Z`, `kenwerk-backup` |
| Cookies | `diffluence_session`, `__Host-diffluence_session`, the sign-in flow cookie | `kenwerk_session`, `__Host-kenwerk_session`, the same with `kenwerk` |
| Tokens | personal tokens start `dfl_`, SCIM tokens `dfl_scim_` | `kenwerk_` and `kenwerk_scim_` (D5) |
| Two-step sign-in | issuer `Diffluence` in the authenticator app | `Kenwerk` |
| Password rules | a new password may not hold `diffluence` | may not hold `kenwerk` |
| PDF helper | user `diffluence` | `kenwerk` |
| Metrics | 41 names, such as `diffluence_http_requests_total` | `kenwerk_*` |
| Monitoring files | `deploy/monitoring/diffluence-dashboard.json`, `diffluence-rules.yml`, their embedded copies in `internal/monitoring` | `kenwerk-dashboard.json`, `kenwerk-rules.yml` |
| Tracing | service name and tracer `diffluence`, attribute `diffluence.role` | `kenwerk`, `kenwerk.role` |
| Audit | the label of a checkpoint key change, `diffluence checkpoint key v1` | `kenwerk checkpoint key v1` (D6) |
| API description | title `Diffluence API`, the session cookie | `Kenwerk API`, `kenwerk_session` |
| Browser event | `diffluence:before-reload` | `kenwerk:before-reload` |
| Screens | the name in the top bar, the title, About, setup and every string in `web/src/locales/en.json` | `Kenwerk`; the mark beside it stays (D7) |
| Packages | `.deb` and `.rpm` named `diffluence`, user and group `diffluence`, unit `deploy/systemd/diffluence.service` | `kenwerk`, `kenwerk.service` |
| Helm | chart `deploy/helm/diffluence` and its helpers | `deploy/helm/kenwerk` |
| LXC | `deploy/install-lxc.sh` makes the roles, the database and the directories | the new names |
| Scripts | `scripts/dev`, `scripts/e2e`, `scripts/mutation-check.sh`, the drills, `scripts/ha`, `scripts/stress` | the new names; the mutations name the new paths and strings |
| Links to the repository | `github.com/Dr1nS0menOp/Diffluence` | `github.com/Dr1nS0menOp/Kenwerk` (D8) |
| Documents | README, CLAUDE.md, CONTRIBUTING.md, blueprint, specs, ADRs, operations, API, design | the new name, except the records of D9 |

Left alone on purpose:

- **Confluence.** Every mention stays as written: the clean-room rules in CLAUDE.md, CONTRIBUTING.md and `docs/blueprint/02-clean-room-and-licensing.md`, the sources of the blueprint, and the planned import of Confluence spaces. `git grep -i confluence` gives the same 45 lines as on main; on five of them the product's own name, which shares the line, changed and nothing else.
- **The short form `dfl` inside the code.** The names of throw-away test roles and databases (`dfl_app_`, `dfl_test_`, `dfl_template_`), the folders of `scripts/e2e`, the project names of the drills, and the advisory lock keys (`0x6466_6c75…`, "dflu" in ASCII) are never shown to anyone and never leave one process or test. Changing the lock keys would make nodes of two releases miss each other's lock during a rolling upgrade, for no gain.
- **SPDX headers.** Every file keeps `SPDX-License-Identifier: Apache-2.0`.

## Decision

Decided on the product owner's behalf on 20 November 2026, may be overruled:

- **D1. One mechanical step.** The rename is a script kept outside the repository, run once on main and once on each open branch before it rebases. It moves every tracked path that names the product with `git mv`, then rewrites every tracked text file with a substitution that keeps the case, and then formats the Go and TypeScript code, because a shorter name changes the order of imports and the alignment of columns. A second run changes nothing. Hand work is limited to this spec, the changelog, the report, the handoff and the two name notes of the blueprint.
- **D2. Module path `github.com/kenwerk/kenwerk`.** The old path, `github.com/diffluence/diffluence`, never pointed at the repository either: it is a name for the code, not an address, and `go install` from it never worked. Keeping that shape keeps the rename mechanical, and it does not tie every import in the code to the account that hosts the repository today. When the product gets an organisation of its own, the path can point there without another rename.
- **D3. No old names are read.** `DIFFLUENCE_*` settings, the old cookie, the old database and roles, the old token prefix, the old data directory and the old backup names are not read any more. A fallback would be code for installs that do not exist, so an install from before starts fresh, as the product owner decided on 17 November.
- **D4. Migrations change in place.** Six migrations name the product: in database settings, in a check on backup names, in a notice and in comments. Their checksums change, so a database made by an earlier build refuses to start with `an applied migration was modified`. That is the fresh start of D3 made visible, not a fault to work around.
- **D5. Token prefix `kenwerk_`.** `dfl_` was short for the old name. A prefix that names the product tells a secret scanner and a reader at once what leaked; `kenwerk_` does both, and a token stays well inside its 128 characters. The SCIM token becomes `kenwerk_scim_`. Tokens made before stop working, as D3 says.
- **D6. The audit label changes.** The label goes into the signature of each change of the checkpoint key, so a chain written by an earlier build no longer verifies with this one. No install has a chain to keep (D3), and a label that names a product other than the one that signed would mislead an auditor.
- **D7. The mark stays.** The logo is a river splitting in two, drawn with lines, with no letter in it, so nothing in it says "D". It was chosen for the old word's meaning; it stays because it is original, it passes the design check, and a new mark is a design choice for the product owner, not a step of a rename. The design standard and the comment on the mark say where it came from.
- **D8. Links name `Dr1nS0menOp/Kenwerk`.** GitHub sends the old address of a renamed repository to its new one, for the web, for `git clone` and for the API, so writing the new address now loses nothing once the product owner renames the repository. Until then these links answer 404: the guide link and the licences link in the app, the systemd unit, the install and backup guides, the example space, `.goreleaser.yaml` and the stress scripts. Released images get new names on GHCR; no release has been tagged, so none has to move.
- **D9. Records stay as written.** The entries of `CHANGELOG.md` before this one, every file in `docs/reports` and every file in `docs/handoff` describe what happened under the old name; rewriting them would change the record and break the paths they quote against the commits they describe. The two notes in the blueprint about where the working name came from (`00-README.md`, `02-clean-room-and-licensing.md`) stay too, with a note above each that names Kenwerk. So does this spec, which maps each old name to its new one. After the rename, `git grep -i diffluence` finds only these files.

## Upgrade impact

A rename; existing installs start fresh. Settings in `.env` or a configuration file take the `KENWERK_` names, and a database is made anew with the new names, because six migrations changed (D4). Personal and SCIM tokens, sessions, backups and audit chains from before are not read.

## Acceptance

- [x] `git grep -i diffluence` finds only `CHANGELOG.md`, `docs/reports/*`, `docs/handoff/*`, this spec, `docs/blueprint/00-README.md` and `docs/blueprint/02-clean-room-and-licensing.md`.
- [x] `git grep -i confluence` gives the same 45 lines as on main, the word Confluence untouched on each.
- [x] `make check` passes, apart from the known timeout of `internal/permissions` on the loaded build machine.
- [ ] The whole end-to-end suite passes, with every picture taken anew from an empty folder and then taken again without change. Pictures done and stable; one test (the example space) runs past its time limit on the build machine; see the report.
- [ ] The full mutation check passes. Every mutation finds its text, and the 23 that name the new name are caught; the full run is still to do; see the report.

## How the product owner tests it

1. Start the development containers and open the app: the top bar, the browser tab, About and the setup screen say Kenwerk, next to the same river mark.
2. Make a personal token in Your account: it starts with `kenwerk_`.
3. Look at `/metrics` with the metrics token: every name starts with `kenwerk_`.
4. Rename the repository on GitHub to Kenwerk; the guide link in the app then opens.

## Provenance

Own design. D8 relies on the redirects GitHub documents for a renamed repository (docs.github.com, "Renaming a repository"); the product owner sees them work after the rename.
