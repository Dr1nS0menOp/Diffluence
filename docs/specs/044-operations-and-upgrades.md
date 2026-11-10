# Spec 044: operations and upgrades

Status: built on branch `wave-o7`.

Phase: blueprint 07. No wave had worked on it directly. Wave AR ran at the same time (the Access screen of a space as roles), so this wave stays out of permission screens and role code.

## The study

Blueprint 07 had no numbered lines. This study read it in full, wrote down every promise it makes, and checked each against the code on `main` at `6d2caf2`, the guides in `docs/operations/` and the tests. The promises are now lines OPS-001 to OPS-062 at the end of the blueprint, in the style of blueprints 03 to 06, so later waves can track them.

What the study found before this wave built anything:

| Promise | Found |
|---|---|
| Compose as the reference install, secrets from files, `role=all` | In the code and proven every night by the restore drill |
| Helm, LXC, HA topology, NATS cluster, Patroni, S3, k6 at 5,000 users | Files or settings exist for some, but nothing ran on real servers |
| `/livez`, `/readyz`, JSON logs | In the code |
| Metrics, including "migration progress" | Requests, latency, queues and failed jobs: yes. Migration progress: no metric |
| OpenTelemetry traces | Not built; a `trace_id` from `traceparent` reaches the log |
| Expand, migrate, contract, contract after every node knows it, `lock_timeout` | In the code and tested |
| "N and N+1 run together. CI tests this" | `scripts/upgrade-test.sh` existed and passed once by hand on 2026-11-04. **CI did not run it** |
| Rolling back to N | Said in `upgrade.md`, never tested. A release older than a contract migration would start and fail on the first query that touches what was removed |
| Preflight checks that refuse and say why | Not built. A node migrated whatever the state of disk, queue or backups |
| Pre-upgrade backup marker | Not built |
| Skipping versions | Migrations run in order: yes. Announcing a maintenance window: no |
| Feature flags to ship dark | Features per space and per instance, not for code that ships dark |
| Weekly restore test, runbook, RTO | A nightly restore drill in CI on a fresh install; the runbook exists; 100 GB never timed |
| Release notes with an upgrade-impact line | Every `CHANGELOG.md` entry has one. No release checklist |

The worst risk was the promise the product calls its most important, the upgrade, resting on a test that nothing ran. Second came the unguarded rollback, then the missing checks before migrating.

## What this wave builds, worst risk first

| Line | What | Load budget (ARC-238) |
|---|---|---|
| OPS-036 (High) | A CI job `upgrade` on every pull request and push. `scripts/upgrade-check.sh <commit>` builds release N from that commit with `git archive` and N+1 from the checkout, then runs the upgrade test. N is the target branch of a pull request, main before a push, or the commit before HEAD | CI only: two builds and about 20 seconds of traffic |
| OPS-040 (High) | The upgrade test gains a rollback: after the old node stopped, the new one stops too and the old one starts again on the migrated database, logs in and reads. Both nodes now share one key, as a real installation does. `db.CheckCompatible` refuses a database in which a newer release ran a contract migration, inside `db.Migrate` and also at the start of a node that does not migrate | One query on `schema_migrations` per start, and one per migration run |
| OPS-038 (High) | Checks before migrating. `db.Migrate` refuses PostgreSQL older than 16. A node that has expand migrations to apply first checks 512 MiB free in its data folder, that the queue answers, and, when `DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE` is set, that a backup was noted within that time. It names every failed check at once and migrates nothing. The queue now opens before migrating, so this check can see it | Only on a start with something to migrate: one `statfs`, one query on `schema_migrations`, one indexed query on `audit_events (action, seq)` |
| OPS-041 | `diffluence backup note` writes `system.backup_noted`, and `scripts/backup.sh` runs it once every part is written. Before a node migrates a database that holds data, it writes `system.upgrade_started` with the migration before, the one after, its version and the time of the last noted backup. A new install writes neither | One audited write per backup and per upgrade |
| OPS-022 | Three gauges: `diffluence_schema_migration`, `diffluence_migrations_pending_expand`, `diffluence_migrations_pending_contract` | One query on `schema_migrations` (one row per migration, 40 today) per scrape |
| OPS-061, OPS-062 | `docs/operations/release.md`, a release checklist | Document |
| OPS-030, OPS-033 | `upgrade.md` matches the code: the checks, the marker, the refusal after a contract migration, the metrics, and that one Compose node restarts in seconds while zero downtime needs two nodes | Document |

### Decisions

| # | Decision | Why |
|---|---|---|
| D1 | The checks run only when the node has expand migrations to apply | A restart without a new release must never wait for a backup or fail on a full disk that has nothing to do with it. `/readyz` and the metrics cover the running node |
| D2 | The backup check refuses only when `DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE` is set; without it, the node warns | An install without `backup.sh`, such as a trial or CI, must keep upgrading with one command (rule 3). Whether the default should refuse is the product owner's decision |
| D3 | A backup is noted in the audit log, not in a table | No migration needed, and the note sits in the hash chain, so a changed or removed note shows when the chain is checked. The query uses the existing index on `(action, seq)` |
| D4 | The note is written after the backup, by `backup.sh`, and is not in the backup itself | A note before the dump would also claim backups that then fail |
| D5 | A release refuses a database with any contract migration it does not know | It cannot tell whether what was removed is something it reads, so it assumes it is. Expand migrations of a newer release stay fine: they only add |
| D6 | PostgreSQL 16 is the minimum, checked in `db.Migrate` | Blueprint 04 names it (PostgreSQL 16 or newer), and the migrations rely on it. `diffluence migrate` gets the check too |
| D7 | 512 MiB free in the data folder | Room for the queue's files and for uploads during the upgrade. The database disk belongs to PostgreSQL and is not visible from the node |
| D8 | Free space is read with `statfs` on Linux only; elsewhere the check cannot tell and is skipped | Diffluence ships for Linux. The other file only lets tests build elsewhere |
| D9 | `scripts/upgrade-check.sh` builds N from `git archive`, not a worktree | It leaves the repository as it was, also when it runs inside a container |
| D10 | No migration in this wave | Nothing needed a table, so 0044 stays free |
| D11 | The checks and the marker run as the owner of the tables, like the migrations | A separate application role gets its rights only after migrating, so before that it may not read `schema_migrations` |

### Tests, failure paths first

- `TestTheChecksBeforeAnUpgradeSayWhyTheyRefuse` and `TestEveryProblemIsNamedAtOnce`: a full data folder, a queue that does not answer, no backup, a backup one hour too old, and the boundaries that must pass.
- `TestAnUpgradeWithoutTheBackupTheOperatorAskedForIsRefusedBeforeMigrating`: a database one migration behind, a backup age set, no backup: the node refuses, nothing is migrated, no marker.
- `TestAnUpgradeLeavesAMarkerNamingTheLastBackup`, `TestARestartIsNoUpgrade`, `TestANewInstallNeedsNoBackup`.
- `TestOlderNodeRefusesADatabaseAfterAContractMigration`, `TestPendingContractOfThisReleaseIsCompatible`, `TestANodeRefusesADatabaseANewerReleaseContracted` (for a node that migrates and one that does not).
- `TestPostgresBelowTheMinimumIsRefused`, `TestStatusOfAnEmptyDatabase`, `TestTheBackupAgeForAnUpgradeIsOffByDefaultAndNeverNegative`.
- `TestMetricsCountRequestsAndShowWhatWaits` names the three new gauges.
- The restore drill now proves that a backup is noted once, and only the backup that was made.

Eleven new mutations in `scripts/mutation-check.sh` break each check and each guard, and the tests above must catch them.

## What needs real servers

These stay open until the product owner provides a machine to run them on: OPS-002 and OPS-031 (Helm on a cluster), OPS-004 (LXC on a real host), OPS-011 to OPS-013 (two or more nodes, a NATS cluster, Patroni), OPS-015 (the load test at size), OPS-033 for real machines behind a real load balancer, OPS-051 (pgBackRest), OPS-054 (a weekly restore of a real installation with an alert), and OPS-055 (the 4 hours for 100 GB).

## What the product owner decides

- Whether the backup check should refuse by default (D2), and with which age.
- GoReleaser for `.deb` and `.rpm` (OPS-005, OPS-032), a YAML reader for a configuration file (OPS-006) and an OpenTelemetry exporter (OPS-024): each is a new dependency or tool.
