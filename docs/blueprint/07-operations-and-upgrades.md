# 07: Operations and upgrades

> **The study (2026-11-10).** Each part of this blueprint was studied against research and industry practice in [`07-operations/`](07-operations/): upgrades and schema change, backup and recovery, high availability, observability, release engineering, configuration, running it yourself, capacity and load testing, and a review of what wave O7 built. The study adds lines OPS-070 to OPS-114 below. The build order is in spec 048.

## Deployment targets

| Target | What we ship |
|---|---|
| **Docker Compose** | The reference installation. One `compose.yaml` and one `.env` file. |
| **Kubernetes** | A Helm chart (phase 3). An operator later, if there is demand. |
| **LXC on Proxmox** | The single binary as a `.deb` plus a systemd unit, an install script, and an Ansible role. No Docker needed. |
| Bare Linux | `.deb` and `.rpm` packages built by GoReleaser |

Configuration is one YAML file and/or environment variables. Secrets come from files (Docker/Kubernetes secrets), never from the command line.

## Topologies

**Small (up to a few hundred users):** one node with `role=all`, embedded NATS, local Postgres, local disk storage.

**HA (from phase 3):**
- 2 or more `api` nodes and 2 or more `worker` nodes behind a load balancer
- A NATS JetStream cluster of 3 nodes
- PostgreSQL with Patroni (3 nodes), PgBouncer and HAProxy. Reads can go to replicas.
- S3-compatible storage with versioning

**Rough sizing, worked out:** 5,000 users Ã— 10% active at the same time = 500 active users. Each makes about 1 request every 10 seconds: 500 Ã· 10 = **50 requests per second**. One API node handles well over 1,000 per second, so **3 nodes** is for redundancy, not capacity. The k6 load test in CI checks this.

## Monitoring

- `/livez` (process is alive) and `/readyz` (ready for traffic: database, NATS and storage reachable, migrations compatible)
- Prometheus metrics at `/metrics` (requests, latency, queue depth, job failures, migration progress). checkmk can read them with its Prometheus integration.
- OpenTelemetry traces and structured JSON logs (`slog`), ready for Graylog or Loki

## Upgrades: the most important promise

**Goal:** after 1.0, releases come monthly and older versions go out of support quickly. So an upgrade must be **one command** (`docker compose pull && docker compose up -d`, `helm upgrade`, or `apt upgrade`), and it must cause **no downtime** while people are working.

**Lessons from large vendors**
- **GitLab** splits migrations into *regular* ones (additive, before new code runs), *post-deployment* ones (removals, only after all nodes run the new version) and *batched background* ones (big data changes in small chunks). It keeps the number of required upgrade stops low.
- **Atlassian Data Center** rolling upgrades: nodes are upgraded one by one while the cluster temporarily runs **two versions side by side**.
- **Stripe** online migrations: dual-write to old and new, move reads over, move writes over, then remove the old. Never one big switch.
- **Kubernetes** version-skew policy: components must work with the version just before and after them.
- **PostgreSQL** practice: `CREATE INDEX CONCURRENTLY`, a short `lock_timeout` on every migration, and no full table rewrites during working hours.

**Our rules**
1. **Expand â†’ migrate â†’ contract.** A release only *adds* (new columns, tables, indexes). Data is moved by **background jobs via NATS**, with progress shown in the admin console. Removal of old structures happens in a **later** release.
2. **Version N and N+1 must run together.** Every release works with both the old and new schema, so rolling restarts are safe. CI tests this: the old version keeps serving traffic while the new one migrates.
3. **No manual steps, ever.** If an upgrade needs a human, it is a bug.
4. **Preflight checks** before migrating: disk space, Postgres version, NATS health, backup recency. If one fails, the upgrade refuses to start and says why.
5. **Feature flags** for risky features, so code can ship dark and be switched on later.
6. **Roll forward, plus a safety net.** Database changes are additive, so rolling back to version N keeps working. A pre-upgrade backup marker makes a full restore possible.
7. **Skipping versions:** jumps are allowed. The binary runs all migrations in order. Zero downtime is **guaranteed between consecutive releases**. A larger jump may need a short maintenance window, which the preflight check announces up front.

## Backup and restore (RPO and RTO at most 4 hours)

- **PostgreSQL:** pgBackRest with continuous WAL archiving and a daily full or differential backup. In practice that gives an RPO of **minutes**.
- **Object storage:** versioning plus replication to a second location, or an encrypted nightly sync
- **Configuration and keys:** a documented export. The KEK is backed up **separately** from the data.
- **Automatic restore test:** a weekly job restores to a scratch environment and verifies hash-chain integrity and page counts, and alerts on failure
- **Runbook:** a step-by-step restore guide, practised at least every phase. RTO is measured in that practice and must stay under 4 hours for 100 GB of data.

## Releases

- Before 1.0: no support, breaking changes allowed, but the upgrade machinery is already used for every build
- From 1.0: monthly releases (`1.x.0`), with security fixes as patch releases (`1.x.y`). Only the latest release line is supported.
- Release notes in plain language, with an "upgrade impact" line (expected none)

## Lines

Spec 044 checked every promise above against the code, the guides in `docs/operations/` and the tests, on 9 November 2026. Each line has the form `OPS-012 · status · text`. A number never changes and is never reused. The statuses mean what they mean in blueprint 06: **built**, **partly** (the line says what is missing), **phase N**, **new** (in no plan yet) and **later** (left for later on purpose). Open lines carry **High**, **Medium** or **Low**. A line marked *servers* can only be proven on real servers, more than one machine or a real cluster.

### Deployment targets

- **OPS-001** · built · Docker Compose is the reference installation: one `compose.yaml` and one `.env`. The restore drill installs it from scratch every night (`scripts/restore-drill.sh`).
- **OPS-002** · partly · A Helm chart in `deploy/helm/diffluence`. Never installed on a real cluster. Phase 3, *servers*. Medium.
- **OPS-003** · later · A Kubernetes operator, only if there is demand.
- **OPS-004** · partly · LXC on Proxmox: `deploy/install-lxc.sh` and `deploy/systemd/diffluence.service`. No `.deb` and no Ansible role, and never run on a real host. *Servers*. Medium.
- **OPS-005** · new · `.deb` and `.rpm` packages built by GoReleaser. A new build tool, so a decision of the product owner. Low.
- **OPS-006** · built · Configuration from environment variables and one YAML file, `DIFFLUENCE_CONFIG_FILE` (spec 056, OPS-099).
- **OPS-007** · built · Secrets come from files: every variable has a `_FILE` variant, and setting both is refused (`TestFileVariant`, `TestBothVariantsIsAnError`). No command takes a secret as an argument: `user create-admin` reads the password from standard input.

### Topologies

- **OPS-010** · built · Small: one node with `role=all`, embedded NATS, local PostgreSQL and local disk. This is what Compose runs.
- **OPS-011** · partly · Roles `api` and `worker` exist, need one shared key (`DIFFLUENCE_MASTER_KEY_FILE`), and report themselves in the node list. Two machines behind a load balancer were never run. Phase 3, *servers*. Medium.
- **OPS-012** · phase 3 · A NATS JetStream cluster of 3 nodes. `DIFFLUENCE_NATS_URL` accepts one; never tested. *Servers*. Medium.
- **OPS-013** · phase 3 · PostgreSQL with Patroni, PgBouncer and HAProxy, reads to replicas. The queries that may read a copy that lags are counted already (ARC-228). *Servers*. Medium.
- **OPS-014** · phase 3 · S3-compatible storage with versioning. Storage has one interface and a disk store; no S3 store yet. Medium.
- **OPS-015** · partly · The sizing of 50 requests a second. A k6 script exists (`scripts/load/reading.js`, runs in `docs/operations/limits.md`), but it is run by hand, not in CI, and not with 5,000 users. Phase 3, *servers* (ARC-240). Medium.

### Monitoring

- **OPS-020** · built · `/livez` says the process is alive.
- **OPS-021** · built · `/readyz` checks the database, the queue for roles that run jobs (an API node keeps serving without it, ARC-216), storage, and that no expand migration of this node is pending. It also says `busy` and `draining`, and a ready node adds the note `disk_low` while new files are refused (spec 051).
- **OPS-022** · built · Prometheus metrics at `/metrics`: requests, latency, the database pool, what waits in the outbox, jobs and mail, failed jobs, and the progress of migrations: `diffluence_schema_migration`, `diffluence_migrations_pending_expand`, `diffluence_migrations_pending_contract` (spec 044, `TestMetricsCountRequestsAndShowWhatWaits`), and since spec 051 the facts of the health checks: disk, database size, backup, build date and how many checks need a look. The full list is in `docs/operations/monitoring.md`.
- **OPS-023** · partly · checkmk reads them through its Prometheus integration. The format is the standard text format, but nobody tried it with checkmk. Low.
- **OPS-024** · new · OpenTelemetry traces. Requests carry a `trace_id` from `traceparent` into the log, but nothing exports traces: that needs a dependency, a decision of the product owner. Low.
- **OPS-025** · built · Structured JSON logs through `slog` on standard error, ready for Graylog or Loki.

### Upgrades

- **OPS-030** · built · One command for Compose: `docker compose pull && docker compose up -d`, or with `--build` from a checkout (`docs/operations/upgrade.md`).
- **OPS-031** · partly · `helm upgrade`: the chart exists, never run on a cluster. Phase 3, *servers*. Medium.
- **OPS-032** · new · `apt upgrade`: needs the package of OPS-005. Low.
- **OPS-033** · partly · No downtime while people work. Proven for two nodes on one database by the upgrade test in CI (OPS-036). A single Compose node restarts, which takes seconds: `upgrade.md` says so. Zero downtime on real machines behind a real load balancer: *servers*. Medium.
- **OPS-034** · built · Expand, migrate, contract. A release only adds. Contract migrations wait until every live node knows them and the newest node has run for `DIFFLUENCE_CONTRACT_GRACE`, a day by default (spec 053), then run by themselves and are audited as `system.contract_migrated` (`TestContract…`). Every migration runs with a `lock_timeout` of 5 seconds and retries.
- **OPS-035** · later · Big data changes moved by background jobs, with progress in the admin console. No migration has needed one yet; build it with the first that does. Medium.
- **OPS-036** · built · Version N and N+1 run together, and CI tests this: the job `upgrade` builds the target branch or the previous commit of main and the change itself, and `scripts/upgrade-test.sh` keeps the old one serving reads and logins while the new one migrates the same database. Any failed request fails the job (spec 044).
- **OPS-037** · built · No manual steps: a node migrates when it starts, and contract migrations run by themselves.
- **OPS-038** · built · Checks before migrating: PostgreSQL 16 or newer, 512 MiB free in the data folder, the queue answers, and a backup noted in the last 24 hours, or within `DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE`; `off` switches that check off on purpose (ADR-040, spec 049). A failed check stops the node before anything is migrated, and names every reason and the ways on (`TestTheChecksBeforeAnUpgradeSayWhyTheyRefuse`, `TestAnUpgradeWithoutARecentBackupIsRefusedByDefault`, `TestPostgresBelowTheMinimumIsRefused`).
- **OPS-039** · partly · Features can be switched off per space and for the whole instance (spec 011), which covers features people see. There is no switch for code that ships dark before anyone may turn it on. Low.
- **OPS-040** · built · Rolling back to version N works until a contract migration ran: the upgrade test starts the old version again on the migrated database. After a contract migration, a release that does not know it refuses to start and names it, also when it does not migrate (`TestOlderNodeRefusesADatabaseAfterAContractMigration`, `TestANodeRefusesADatabaseANewerReleaseContracted`).
- **OPS-041** · built · A pre-upgrade backup marker: `scripts/backup.sh` notes each complete backup as `system.backup_noted`, and a node that migrates a database with data first writes `system.upgrade_started` with the migrations before and after and the time of that backup (`TestAnUpgradeLeavesAMarkerNamingTheLastBackup`).
- **OPS-042** · partly · Skipping versions: the binary runs every migration in order, also from an empty database. Announcing a maintenance window for a larger jump needs to know which migrations belong to which release, which only exists from 1.0. Low.
- **OPS-043** · built · `CREATE INDEX CONCURRENTLY` and no full table rewrites during working hours: a migration with `-- transaction: none` runs outside a transaction, and the rewrite check refuses the rest (OPS-072, OPS-073, spec 053).

### Backup and restore

- **OPS-050** · partly · RPO and RTO of at most 4 hours. `scripts/backup.sh` makes an encrypted backup when the operator runs it; `backup-restore.md` says how often for 4 hours. Low.
- **OPS-051** · new · pgBackRest with WAL archiving. Advised in `backup-restore.md`, not set up by anything. *Servers*. Medium.
- **OPS-052** · partly · Stored files are in every backup, encrypted. Versioning and replication to a second location need the S3 store of OPS-014. Medium.
- **OPS-053** · partly · Configuration and keys: `secrets/` and `master.key` are in every backup, each encrypted, and the backup key must live apart from the backups (the script refuses otherwise). Keeping `master.key` in another place than the data is left to the operator, as `backup-restore.md` explains. Low.
- **OPS-054** · partly · The restore test runs every night in CI on a fresh installation and checks the page, the attachment, search, login, the audit chain and role separation; a failure fails the run. A weekly restore of a real installation into a scratch environment, with an alert, is not built. *Servers*. Medium.
- **OPS-055** · partly · The runbook is `backup-restore.md`, and the drill times itself (29 seconds on a small set). The 4 hours for 100 GB were never measured. *Servers*. Medium.

### Releases

- **OPS-060** · built · Before 1.0, every build goes through the upgrade machinery: migrations at start, and the upgrade test in CI on every change.
- **OPS-061** · later · From 1.0, monthly releases, security fixes as patch releases, only the latest line supported. The checklist is ready in `docs/operations/release.md` (spec 044).
- **OPS-062** · built · Release notes in plain language with an "upgrade impact" line: every entry in `CHANGELOG.md` has one, and the release checklist asks for it.

### From the study of 2026-11-10

Each line is explained in the chapter named after it, in [`07-operations/`](07-operations/). All are **new** unless the list under the table says otherwise; spec 048 orders them into waves.

| Line | Priority | What | Chapter |
|---|---|---|---|
| OPS-070 | High | Test every format that crosses versions: documents, bundles, events, jobs, settings, the editing service | 1 |
| OPS-071 | High | The upgrade test also writes while the new version migrates | 1 |
| OPS-072 | Medium | CI refuses migrations that rewrite or lock a table for long | 1 |
| OPS-073 | Medium | Migrations outside a transaction, for concurrent indexes, checked afterwards | 1 |
| OPS-074 | Medium | A `statement_timeout` on every migration | 1 |
| OPS-075 | Medium | A written version-skew policy, proven by the upgrade test | 1 |
| OPS-076 | Low | From 1.0, CI upgrades from the oldest release that may jump straight to this one | 1 |
| OPS-077 | High | The age of the last backup as a metric, on the health page and as a warning | 2 |
| OPS-078 | High | Restore time measured for 10 GB and 100 GB on real disks | 2 |
| OPS-079 | Medium | A tested pgBackRest set-up with WAL archiving | 2 |
| OPS-080 | Medium | A scheduled backup service in Compose, every 4 hours by default | 2 |
| OPS-081 | Medium | A weekly restore of a real installation into a throw-away project | 2 |
| OPS-082 | Low | The guide says a standby or RAID is not a backup, and what 3-2-1 asks | 2 |
| OPS-083 | High | The queue's storage settings chosen on purpose, and events proven after a hard reset | 3 |
| OPS-084 | High | The HA topology run on virtual machines, with failures and partitions under load | 3 |
| OPS-085 | Medium | The reference Patroni set-up with the watchdog and 3 etcd members | 3 |
| OPS-086 | Medium | The whole suite under PgBouncer in transaction mode | 3 |
| OPS-087 | Low | A page on what HA protects and what it does not | 3 |
| OPS-088 | Medium | Trace export with OpenTelemetry over OTLP, off by default | 4 |
| OPS-089 | Medium | Prometheus rules: SLOs, burn-rate alerts and alerts for causes | 4 |
| OPS-090 | Low | One dashboard as a file | 4 |
| OPS-091 | Low | checkmk tried and written down | 4 |
| OPS-092 | Low | Metric and log names listed as an interface | 4 |
| OPS-093 | High | A release workflow with GoReleaser: archives, `.deb`, `.rpm`, image | 5 |
| OPS-094 | High | SBOM, keyless signature and provenance for each release | 5 |
| OPS-095 | Medium | CI checks that a build reproduces byte for byte | 5 |
| OPS-096 | Medium | Base images pinned by digest | 5 |
| OPS-097 | Low | Named toggles for dark features, each with a removal issue | 5 |
| OPS-098 | Low | The support policy written down | 5 |
| OPS-099 | Medium | A strict YAML configuration file | 6 |
| OPS-100 | Medium | `diffluence config check` | 6 |
| OPS-101 | Medium | Settings for later use checked at start, as warnings | 6 |
| OPS-102 | Low | The list of settings reviewed and shortened | 6 |
| OPS-103 | High | The health page shows disk, database growth, backup, migrations, work, mail, login and version | 7 |
| OPS-104 | High | Low disk is visible and handled before it is full | 7 |
| OPS-105 | Medium | Everything that grows has a limit or a cleaner, tested | 7 |
| OPS-106 | Medium | A sign when the installation is behind, without phoning home | 7 |
| OPS-107 | Medium | `diffluence doctor` | 7 |
| OPS-108 | Low | Sizing examples for 50, 500 and 5,000 people | 7 |
| OPS-109 | High | The load test series on server hardware, to four times the model and beyond | 8 |
| OPS-110 | High | A soak of at least 24 hours | 8 |
| OPS-111 | Medium | k6 scripts for each kind of test, open model only | 8 |
| OPS-112 | Medium | A small load test in CI that catches slow queries | 8 |
| OPS-113 | Medium | CPU of PostgreSQL per active person, turned into sizing | 8 |
| OPS-114 | Low | A rolling upgrade under load on several VMs | 8 |

Built since the study (spec 051, wave 1 of spec 048):

- **OPS-077** · built · The age of the last noted backup is the metric `diffluence_backup_last_noted_timestamp_seconds`, a line on System health, a line on the summary when it needs a look (none noted, or older than 26 hours), an alert in the shipped rules, and a line of `diffluence doctor` (`TestNoBackupNotedWarns`, `TestABackupOlderThanTwentySixHoursWarns`, `TestTheHealthPageShowsTheBackupAndTheDisk`).
- **OPS-089** · built · `deploy/monitoring/diffluence-rules.yml`: availability and latency objectives of 99.5% over 30 days, burn-rate alerts over two windows, and alerts for causes, each saying what to do. A test checks that every metric they use is one `/metrics` sends (`TestTheMonitoringFilesUseOnlyMetricsTheServerSends`). Never loaded into a running Prometheus: wave 7.
- **OPS-090** · built · `deploy/monitoring/diffluence-dashboard.json`, on the same metrics and checked by the same test. Never opened in a running Grafana: wave 7.
- **OPS-103** · built · System health lists disk, database size and growth over 30 days, backup, migrations, background work, mail, login and the age of the release, each with a sentence and what to do, and each is a metric (`internal/health`, `GET /api/v1/admin/health`, `web/e2e/health.spec.ts`). The identity provider is not asked whether it answers: that would be a call to an outside server on every look. Login shows accounts whose password login stopped.
- **OPS-104** · built · Below 10% free, a band on every page of the console; below 2%, every new file is refused with HTTP 507 while text saves, and `/readyz` adds `"notes": ["disk_low"]` (`TestANearlyFullDiskRefusesFilesButTextStillSaves`). A disk that really fills was not tried: the test stands in for one.
- **OPS-105** · built · Finished jobs, sent mail, nodes that crashed and size samples now have a daily cleaner, and `docs/operations/monitoring.md` lists everything that grows with what keeps it small (`TestTheCleanerRemovesOnlyOldFinishedWork`).
- **OPS-106** · built · The binary carries the date of its commit (`version.Date`, set by `make build` and by the image's `BUILD_DATE`); the console warns past 60 days, and nothing asks an outside server. The opt-in check of the public release feed is not built. A developer build has no date and never warns.
- **OPS-107** · built · `diffluence doctor` runs the health checks, a configuration check and a shallow key check, prints states, counts and dates only, and exits 1 on a problem (`TestDoctorReportsEveryCheckAndNoSecret`). Since spec 056 it also prints the warnings of the settings tried at start (OPS-101); `config check` (OPS-100) prints the whole configuration.

Built since the study (spec 053, wave 2 of spec 048):

- **OPS-070** · partly · Formats that cross versions: a worker claims only job kinds it knows (`TestAJobOfANewerReleaseWaitsForANodeThatKnowsIt`); every listener lets an event of an unknown type pass (`TestEveryListenerLetsAnEventOfANewerReleasePass`); saving settings keeps fields of a newer release (`TestSavingSettingsKeepsFieldsOfANewerRelease`); documents and bundles already carry versions. The upgrade test checks that no job failed and no event was set aside after both releases worked. Missing: the editing service across two releases.
- **OPS-071** · built · The upgrade test publishes, saves a draft, comments and uploads on the old release, reads it on the new one, writes there and reads it on the old one while both run, and again after the rollback (`scripts/upgrade-test.sh`).
- **OPS-072** · built · `TestNoMigrationRewritesATable` refuses, from migration 0048 on, a new column type, `SET NOT NULL`, a volatile default, a stored generated column, an index without `CONCURRENTLY` or a `CHECK` or `FOREIGN KEY` without `NOT VALID` on a table the file did not create, `VACUUM FULL` and `CLUSTER`, unless `-- rewrite-ok: <reason>` allows the statement.
- **OPS-073** · built · `-- transaction: none` runs a file of one statement outside a transaction, with the same lock, limits and retries, and stops naming any index it left invalid (`TestAConcurrentIndexIsBuiltOutsideATransaction`, `TestAnIndexLeftInvalidIsNamed`). This closes OPS-043.
- **OPS-074** · built · Every migration runs with `statement_timeout` of a minute, or the duration its header gives (`TestAMigrationThatRunsTooLongIsStoppedAndRolledBack`, `TestTheHeaderGivesAMigrationMoreTime`).
- **OPS-075** · built · [`docs/operations/version-skew.md`](../operations/version-skew.md): what may differ by how much and for how long, and the test that proves each line. Contract migrations wait a day by default (`DIFFLUENCE_CONTRACT_GRACE`, spec 048 D1), and a contract migration only removes what the previous release already stopped using (D2, in `migrate.go` and the release checklist).
- **OPS-076** · later · The six-month jump (D3) is written in the policy; its test waits for a release that old, from 1.0.

Built since the study (spec 056, wave 4 of spec 048):

- **OPS-099** · built · `DIFFLUENCE_CONFIG_FILE` names one YAML file, read with `go.yaml.in/yaml/v3`. Keys are the variable names in lower case without the prefix, nested or flat. The order is default, file, environment; an empty variable lets the file through. Unknown keys, a key written twice, a list or mapping where one value belongs, and a value of the wrong kind refuse the start with the line; values are read as written, so `no` and `012` are never `false` or octal. A secret comes from a `_file` key, or is written in a file its owner alone may read (`internal/config/file_test.go`, `TestTheExampleFileLoads`). This closes OPS-006.
- **OPS-100** · built · `diffluence config check [--offline]` loads the configuration through the same validator as a start, prints every setting with its value in effect and its source (default, file and line, environment, or environment over the file), each secret as `(set)` and a database address without its password, and exits 1 when a start would be refused (`TestConfigCheckPrintsEverySettingWithItsSourceAndNoSecret`, `TestConfigCheckExitsOneOnABadFile`, `TestTheReportNamesEverySourceAndHidesSecrets`). The release checklist runs it.
- **OPS-101** · partly · At start, in the background, and in `config check` and `doctor`: the mail server's name and port, the identity provider's discovery document, and a test file in the publish folder, each within 5 seconds, a failure being a warning (`internal/config/probe_test.go`). Missing: a line on System health; a start only logs the warning.
- **OPS-102** · partly · No setting removed or renamed, because installations set them. The guide names the few every installation sets, and the example file marks the rest as rarely needed; fixing settings nobody changes is left for a later review.
