# Spec 051: it watches itself

Status: built on branch `wave-o1`.

Phase: blueprint 07, wave 1 of [spec 048](048-study-of-07-and-08.md). Lines OPS-077, OPS-089, OPS-090 and OPS-103 to OPS-107. The study behind it is [`07-operations/`](../blueprint/07-operations/), chapters 2 (backup), 4 (observability) and 7 (running it yourself), with decisions D8 to D11 of spec 048. Wave LD runs at the same time (the pool, the feed, the bell count, file search, the audit lock, `limits.md` and the install guide), so this wave stays out of those files.

## What the study found, and what is there today

The failures that hurt self-hosted installations are quiet ones: a backup that stopped, a disk that filled, an installation that fell behind (07.7, GitLab 2017, the SRE book). The product must say so itself, on a page an administrator already opens, and as metrics an operator can alert on.

| Line | Today on `main` (`37423d7`) | Missing |
|---|---|---|
| OPS-077 backup age | `system.backup_noted` is written by `diffluence backup note`; only an upgrade reads it | A metric, a line on System health, a warning past 26 hours |
| OPS-103 health page | System health lists nodes, the version, queues, failed jobs and the trash | Disk, database size and growth, backup age, the age of pending contract migrations, mail, login, the age of the release, each with a sentence and what to do |
| OPS-104 low disk | The summary warns below 10% free | Every console page warns; below 2% uploads, imports and exports are refused while text saves; `/readyz` says so |
| OPS-105 what grows | Cleaners for trash, outbox, event failures, rule runs, webhook log, notifications, exports, previews, imports, analytics, sessions and challenges | Finished jobs, sent mail and nodes that crashed grow for ever; no guide lists the cleaners |
| OPS-106 behind | The binary knows its version, not its date | A build date and a warning past 60 days, without asking any server |
| OPS-107 `doctor` | `key check` exists | One command that runs the health checks, a configuration check and a key check, and prints a report fit to paste |
| OPS-089 rules | `/metrics` has requests, a latency histogram, queues, failed jobs and migrations | Recording and alert rules with SLOs and burn rates |
| OPS-090 dashboard | None | One Grafana dashboard as a file |

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | The checks are a pure function from a set of facts to a list of results (`internal/health`): each result has a name, a state (`ok`, `warn`, `problem`), a sentence and what to do. Reading the facts is separate | Every threshold can be tested and mutated without a database, and `doctor` and the console use the same rules |
| D2 | Thresholds: disk warns below 10% free and refuses large writes below 2% (spec 048 D9); a backup warns past 26 hours or when none was noted (D10); a contract migration that waits more than a day warns; the release warns past 60 days (D8); a failed job in 7 days, a failed mail in a day, an event that gave up, or an account whose password login stopped warns; the outbox holding an event for more than an hour is a problem | Each is a cause that needs a person soon. The outbox is a problem because events stop reaching anyone |
| D3 | The database grows a line once an hour in `database_size_samples`, kept 40 days; the growth shown is the last 30 days. When the growth of a month would fill the free disk, the disk check warns even above 10% | `pg_database_size` reads every file of the database, too much for every check (spec 048 load budget). A month ahead is how far an operator plans |
| D4 | Large writes are refused in one place: a store that wraps the real one and refuses `Put` below 2% free, with a new error kind `no_room` (HTTP 507). Text is in the database, so it goes on | Every upload, import, export, preview, picture, logo and font goes through `Put`. One guard cannot be forgotten by a new route |
| D5 | `/readyz` stays `200` when the disk is low and adds `"notes": ["disk_low"]` | A single node that says "not ready" stops text as well, which D9 of spec 048 forbids. A load balancer can still read the note |
| D6 | The build date is the date of the commit (`git log -1 --format=%cs`), set at link time; a build without it (a developer build) never warns | The commit date keeps builds reproducible; a clock date would not |
| D7 | Cleaners: finished jobs after 30 days and failed ones after 90; sent and dropped mail after 30 days and failed mail after 90; a node not seen for a day; size samples after 40 days. One daily task | Failed work is kept longer because a person may still need to look at it. A node that crashed never left the registry |
| D8 | `doctor` reads the configuration, opens the database and the files, runs the health checks and a shallow key check, and prints only states, counts, sizes and dates: no address, no secret, no name. It exits 1 when any check is a problem | An operator pastes it into a question; whoever reads it learns how the installation is doing and nothing else |
| D9 | The rules and the dashboard are files in `deploy/monitoring/`, built only on metrics `/metrics` sends. A test reads every metric name in them and checks that the server sends it | A renamed metric breaks the test, not an operator's alert at night |
| D10 | Every administrator page shows a band when the disk check is not ok | OPS-104 says "on every page of the console"; the band links to System health |

## Build steps, failure paths first

1. **Health rules** (`internal/health`): tests for every threshold on both sides, unknown facts (no disk size, no samples, no build date) never warn, then the code. Mutations of each threshold.
2. **The guard** (`storage.Guarded`): refused below 2%, allowed at 2%, unknown room allowed, the free space read at most every 30 seconds, `Space` passed on. Then `apperr.NoRoom` and its 507. A test through the API: an upload refused with 507 and a page still saved.
3. **Facts and cache** (`app`): one `statfs`, one query for counts, backup, size samples and the last migration, and the existing `db.Status`; kept 30 seconds per node. `GET /api/v1/admin/health` for the console; `/api/v1/admin/system` gains `checks`.
4. **Size samples and cleaners**: migration 0046 (`database_size_samples`), the hourly sample, the daily cleaner. Tests fill each table with old and new rows and check that only the old go.
5. **Metrics**: `diffluence_disk_free_bytes`, `diffluence_disk_total_bytes`, `diffluence_backup_last_noted_timestamp_seconds`, `diffluence_database_size_bytes`, `diffluence_migration_applied_timestamp_seconds`, `diffluence_build_timestamp_seconds`, `diffluence_event_failures`, `diffluence_health_problems` and `diffluence_health_warnings`.
6. **`/readyz` note**, the build date in `Makefile` and `Dockerfile`, and `diffluence doctor`.
7. **Console**: System health lists the checks first; the band on every administrator page; the summary links to them.
8. **Rules, dashboard and the guide**: `deploy/monitoring/`, `docs/operations/monitoring.md` with every metric, alert and cleaner, and the test of D9.

## Load budget (ARC-238)

| What | Cost |
|---|---|
| A health check (console, `doctor`, metrics) | One `statfs` and three indexed queries (counts, `db.Status`'s two), kept 30 seconds per node |
| The size sample | One `pg_database_size` an hour, by one worker |
| The guard | One `statfs` per 30 seconds per node, nothing per upload while the answer is fresh |
| The cleaner | One delete per table a day, by indexed age |
| The band | The console's health answer, already cached |

## What one machine cannot prove

The alert rules are checked for the names they use, not run in a Prometheus; the dashboard is not opened in a Grafana. A disk that really fills is simulated by the guard's rule with a fixed answer, not by filling a disk. Both are for the test machine of spec 048, wave 7.

## Sources

The study chapters [02](../blueprint/07-operations/02-backup-and-recovery.md), [04](../blueprint/07-operations/04-observability.md) and [07](../blueprint/07-operations/07-self-hosted-operations.md) and their sources.
