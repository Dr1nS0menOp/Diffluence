# 072: The module boundary as a check, and smaller fixes

Status: built
Phase: hardening before 1.0 (wave RV5)

## Problem

`CLAUDE.md` says that modules talk through their Go API and events, never through each other's tables. ARC-027 already loosened that for reading: a list may join a display column of another module's table. Neither was checked by anything, so nobody could tell which package reads which table, and a column could not change without searching the whole code base first.

Next to that, this wave takes four smaller items:

- the service for editing together checked every connection again for every change of rights, and runs could overlap;
- the owner's database password was part of the command line of `psql` and `pg_dump` in the backup service, and of `psql` in a restore;
- the placeholder migrations did not say why they stay, and the removal half of expand and contract had never run on the real schema;
- `master.key` sits in the data directory by default, and the guide did not say to keep it elsewhere.

## Behaviour

### 1. The module boundary is checked (ARC-027)

- `internal/db/boundary_list_test.go` names the one module that owns each table and view the migrations create. The table in blueprint 04, chapter 2, decides; a table of the installation as a whole that only the wiring writes belongs to `internal/app`.
- `internal/db/boundary_test.go` reads the string literals of every Go file outside tests, below `internal/` and `cmd/`, and finds each table they name after `FROM`, `JOIN`, `INTO`, `UPDATE`, `USING` or `TABLE`. Comments do not count.
- It fails `make check` when:
  - a migration creates a table that has no owner, or the list names a table no migration creates;
  - a module names another module's table more often than the list allows, or a table the list does not name for it. The message says which, and gives the line to add;
  - a module names it less often than listed: the number must come down with the code, so the list never hides room for a new mention.
- `internal/testutil` is left out: only tests run its SQL.
- The list starts as the state of 13 November, minus what moved behind the owner in this wave. Every group of lines says why the reads are there. Most are what ARC-027 allows, a list that joins a title or a name, because one join is one query and a call per row would be hundreds. The writes are those spec 032 recorded, which must happen in one transaction with the owner's change.

**Counted.** Before: 564 mentions in 201 pairs of module and table. After: 557 in 195. The review of 13 November counted about 340, for the core tables alone and by a looser search; this count covers every table and every module, including `app`, `console` and `cmd`.

**Moved behind the owner.** Each through a small exported function of the owning module, which takes a `db.Querier`, so the caller's transaction and the wiring stay as they were:

| Was | Now |
|---|---|
| `analytics` and `automation` read their key of `settings` | `settings.Load`, the read beside `settings.Store`; `ReadIn` uses it too |
| `webhooks` read `pages.title` for an endpoint that asks for titles | `pages.Title` |
| `sharing` read `spaces.home_page_id` for a guest invitation | `spaces.HomePage` |
| `restore` read `backup_runs` and `backup_requests` before a restore | `backups.ReadinessToRestore` |
| `cmd/diffluence` deleted its row of `ops.nodes` when a node had started | `restore.Leave` |

**Limits of the check.** It reads SQL written with upper-case keywords, as all SQL here is. A table named after a comma in `FROM a, b` is not seen; none is written that way today. SQL inside `perm_*` functions and other SQL functions of the migrations is not read: those are shared on purpose (ARC-027).

### 2. One check of the editing connections at a time

- The service for editing together checks every connection again when the API tells it rights changed, and every minute. Now one check runs at a time. Calls that arrive while it runs share one more run after it, because a change may have come after the running check passed that person.
- **Measured** in `collab/access.test.mjs`: ten calls during one run ask the API twice per connection, not ten times. A run that fails reports its failure to its own callers, and the queued run still happens.

### 3. The owner's password stays off the command line

- The backup service runs `psql` and `pg_dump` through `as_owner`: the address without its password goes on the command line, the password, percent-decoded, goes in `PGPASSWORD`. A command line is readable by every process in the container; the environment of a process only by the process itself and root. An address in the `key=value` form is passed whole, as before.
- A restore (`diffluence backup restore-run`) gives `psql` the same split (`passwordApart` in `cmd/diffluence/restore.go`).

### 4. Placeholder migrations and the removal half

- `docs/development.md` has a section on migrations: why placeholders such as `0013_reserved.sql` exist, why they can never be removed (each installation recorded its version and checksum, and the runner refuses a changed file or a gap), and how to write one. The old comments that say "remove when" are out of date and stay, because a merged migration is never edited.
- `TestAContractMigrationRemovesAColumnOnACopyOfAnInstallation` runs the removal half end to end on a copy of an installed database with data, as `make check` and CI's `go` job do on every change. It checks that the contract migration waits until allowed, runs, is recorded as `contract`, keeps the data around the removed column, that release N then refuses the copy, and that the original database is unchanged.

### 5. Where `master.key` lives

- The install guide says to keep `master.key` apart from the data on a real installation: with the packages, the LXC script or Kubernetes through `DIFFLUENCE_MASTER_KEY_FILE` on another disk, mount or secret store. In Compose the backup service reads the key from the data volume, so there the key stays and the volume is kept out of copies that would not get a key's care. The default itself is unchanged, so no installation has to move a file.

## Permissions and audit

Nothing changes in who may do what, or in what is audited. The moved reads decide nothing; each caller checks access before, as it did.

## Load budget

- The moved reads are the same queries, now in the owning package: no extra round trip.
- The editing service asks the API at most twice per connection for any burst of changes of rights, instead of once per change.
- The boundary check is a test only: it parses every Go file outside tests in about two seconds.

## Acceptance criteria

- [x] A table without an owner, and a new mention of another module's table, fail `make check` (`TestEveryTableHasAnOwningModule`, `TestModulesNameOnlyTheirOwnTablesOrListedOnes`); the reader of the check finds every way a query names a table, and only in string literals outside tests (`TestTablesNamed…`, `TestOnlyStringLiteralsCountAsSQL`, `TestCountCrossings…`).
- [x] The five moved reads answer as before (`TestLoadFillsAStoredValueAndLeavesAMissingOneAlone`, `TestTitleNamesAPageAndFailsForOneThatIsGone`, `TestHomePageIsNilUntilASpaceHasOne`, `TestReadinessSaysWhetherABackupIsWholeAndWhetherOneIsOpen`, `TestALeavingNodeNoLongerCountsAsBusy`), and the suites of their callers stay green.
- [x] A storm of rechecks runs one check at a time and one more after it.
- [x] The owner's password is taken out of the address for `psql` in a restore, decoded.
- [x] A contract migration runs end to end on a copy of an installed database.
- [x] Each rule above has a mutation in `scripts/mutation-check.sh`.

## How the product owner tests it

1. Nothing in the product looks different. Edit a page together in two browsers, and change a person's rights on the page: the other browser is closed within seconds, as before.
2. On a test installation, `docker compose exec backup ps -eo args` while a backup runs shows `pg_dump` with an address that holds no password.
3. Back up and restore from the console as before.

## Decided on the product owner's behalf on 2026-11-13, may be overruled

- **D1: enforce the rule, and keep today's reads in a list, rather than drop the rule.** The rule stays as ARC-027 words it: no module writes another's tables except the cases spec 032 records, and a read of another module's table is allowed for a display column of a list. The check makes every such read visible and stops new ones without a reason. Rewriting 557 mentions behind APIs would have touched most of the code base for no change in behaviour.
- **D2: the count may only fall, and must be exact.** A lower count fails until the number in the list is lowered too, so the list never leaves room for a new mention.
- **D3: count every module, `app`, `console` and `cmd` too.** The wiring and the console do the most crossing, and a rule with exceptions nobody sees is the problem this spec solves. Only `internal/testutil` is left out, because only tests run its SQL.
- **D4: a module owns a table when blueprint 04 says so, or else when it writes it.** Document control's tables belong to `pages`, which writes them today; `nodes`, `storage_state`, `helper_health` and `database_size_samples` belong to `internal/app`, which alone writes them. When a module of its own takes them over, the list changes with it.
- **D5: move only what an owner can answer with one small function and no change of wiring.** Five reads moved. A read that would need a new import across the direction of ARC-026 (`labels`, and `comments` asking `pages`), a list join, or a write spec 032 records, stays listed with its reason.
- **D6: modules that builders change at the same time keep their SQL listed.** The event path (`events`), identity and logins (`identity`, `sso`), and document control are being changed by other waves now; moving their SQL waits until those are merged, and a merge that changes their counts updates the list.
- **D7: of the slow work done one at a time, only the editing service's checks are changed now.** It is small and its effect is measured in a test. Webhook deliveries and uploads are left for later, each with its own measurement.
- **D8: the shared document's state is not reset in this wave** (section "Left"). A reset is only safe when no browser still holds the old state, and that needs a generation the service and the browser agree on: more than a small fix.
- **D9: the owner's password goes through `PGPASSWORD`, not a password file.** The environment of a process is closed to other users; a file would need writing, a mode and cleaning up for every call. The address without the password stays on the command line, so the logs of the tools still name host and database.
- **D10: `master.key` keeps its default place; the guide changes.** Moving the default would move a file on every installation and break the backup service in Compose, which reads the key from the data volume.
- **D11: the end-to-end contract test is a Go test, not a step of `scripts/upgrade-test.sh`.** A release binary embeds its migrations, so a contract migration cannot be added to one for a test; the Go test drives the same runner on a real, copied database.
- **D12: no migration in this wave.** None was needed, so no number was taken.

## Left

- **Refused reads and the audit log.** Every refused read still writes one audit row, as the threat model accepts until phase 3. Writing one row per person, object and minute belongs with the audit work of another wave that changes the same code now.
- **A shared document's state.** Nothing compacts or resets the state of a page edited together, so a page edited heavily for a very long time could reach the limit of 16 MiB, after which the service stops editing it together. The plan: a generation number in the name of the shared document, raised at publish when nobody is connected, so the state starts again from the published version and a browser that still holds the old one is told to reload instead of merging it back. First measure how fast the state grows under real editing.
- **The default place of `master.key` in Compose.** A separate volume for keys, shared read-only with the backup service, would take the key out of the data volume; it needs a step for every existing installation.

## Provenance

Own design. Go standard library documentation for `go/parser`, `go/ast` and `net/url` (pkg.go.dev, 2026-11-13). PostgreSQL 16 documentation on `CREATE DATABASE … TEMPLATE` and the libpq environment variables, `PGPASSWORD` among them (postgresql.org, 2026-11-13).
