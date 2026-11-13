# Upgrading Diffluence

Written for: operators.

```sh
git pull && docker compose up -d --build     # or: docker compose pull && docker compose up -d
```

That is all. There are no manual steps. If you changed a setting or the configuration file, check it first: `docker compose run --rm --no-deps -T diffluence config check` refuses what a start would refuse, without starting ([install.md](install.md#checking-a-change-before-a-restart)).

With one node, Compose stops the old container and starts the new one, so people wait a few seconds. Zero downtime needs two or more nodes behind a load balancer, replaced one at a time.

## What happens

1. The new version starts and first checks that it may migrate (below). Then it applies **expand** migrations. They only add things, so the running version keeps working.
2. With several nodes, you replace them one at a time. Old and new run side by side on the same database.
3. Every node reports its version and the migrations it knows, every 15 seconds.
4. When every live node knows a pending **contract** migration, and the newest node has been up for a day, the worker runs it automatically. It is recorded in the audit log as `system.contract_migrated`. The day leaves room to notice a problem the next working day and still roll back. Set `DIFFLUENCE_CONTRACT_GRACE` to change it, such as `72h` for a longer window, or `0s` on a test installation.

`diffluence migrate --contract` runs them by hand if you ever need to.

Every migration runs with a time limit of a minute (`statement_timeout`), besides the five seconds it may wait for a lock. A migration that runs longer stops, rolls back and names itself in the log, so it cannot hold a busy table; nothing is half done. A migration that needs longer says so in its first lines, with the reason. Indexes on large tables are built alongside the traffic (`CREATE INDEX CONCURRENTLY`), outside a transaction; if such a build fails, it leaves an invalid index behind, and the next start names it and stops until it is dropped:

```
error: db: migration 0050_example failed: index pages_title_idx was left invalid by a failed concurrent build; drop it with DROP INDEX CONCURRENTLY and start again
```

Which releases may run side by side, and for how long, is in [version-skew.md](version-skew.md).

## The checks before migrating

A node only checks when it has something to migrate, so a plain restart never waits for them. If a check fails, nothing is migrated and the node **waits** (spec 054): it stays running, logs every reason at once, and checks again every minute, such as:

```
waiting before the upgrade: upgrade refused before migrating: the data folder has 300 MiB free and needs 512 MiB; the queue (NATS) does not answer
```

While it waits:

- every page in the browser shows "Waiting before an upgrade", with the kind of problem and the commands that fix it, but no figures or dates, since anyone can open it;
- `/readyz` answers 503 with the kinds of problem, so a load balancer sends nobody, and `/livez` answers 200;
- `docker compose ps` shows it running and unhealthy, and `docker inspect` shows the reason in its health log;
- it touches nothing in the database.

Once the cause is fixed it migrates by itself within a minute. `docker compose kill -s HUP diffluence` makes it check at once; `scripts/backup.sh` does that after a backup. Releases before spec 054 exited instead, and Compose restarted them in a loop.

| Check | Refuses when |
|---|---|
| PostgreSQL | The server is older than PostgreSQL 16. Also for `diffluence migrate` |
| Free space | The data folder has less than 512 MiB free |
| Queue | NATS does not answer |
| Backup | No backup was noted in the last 24 hours, or within `DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE` when that is set, such as `48h`. Also for `diffluence migrate`. The first upgrade of an installation that never noted a backup goes on once, below (ADR-040, ADR-042) |
| Newer database | A newer release has already run a contract migration. Also for a node with `DIFFLUENCE_MIGRATE_ON_START=false`. This one stops the node, as the database is beyond what it can read |

**An upgrade waits without a backup of the last 24 hours.** The backup service of `compose.yaml` makes one every 4 hours ([backup-restore.md](backup-restore.md)), so an installation that runs it never waits. The reason in the log looks like this:

```
waiting before the upgrade: upgrade refused before migrating: no backup was noted: run scripts/backup.sh or the backup service first (docker compose run --rm backup once), or upgrade once without one with diffluence migrate --without-backup, or set DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE=off to upgrade without one
```

The ways on, best first:

1. **Make a backup:** `docker compose run --rm backup once`, or `bash scripts/backup.sh <folder> <key>`. The node migrates within a minute.
2. **Upgrade once without one:** `docker compose run --rm diffluence migrate --without-backup`. It writes `system.upgrade_without_backup` with the reason `operator_override` to the audit log. The next upgrade asks for a backup again.
3. **Switch the check off:** `DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE=off`. Every later upgrade then goes on without a backup too, with only a warning in the log, so prefer the first two.

**The first upgrade under the rule** (ADR-042): an installation that holds data, never noted a backup, and never upgraded without one, is not stopped by the backup check once. The node logs `UPGRADING WITHOUT A RECENT BACKUP` as a warning, writes `system.upgrade_without_backup` with the reason `first_upgrade_under_the_backup_rule`, and migrates. The health page keeps warning that no backup was ever noted. From the next upgrade on, the rule holds in full. The other checks are never waived.

A new installation is never asked for a backup: it has nothing to back up.

The backup service and `scripts/backup.sh` note each complete backup in the audit log as `system.backup_noted`. A backup made another way can be noted with `docker compose exec diffluence diffluence backup note`, or `docker compose run --rm --no-deps diffluence backup note` when the node is not running, but only once it is complete.

## The marker before an upgrade

Before a node migrates a database that holds data, it writes `system.upgrade_started` to the audit log: the highest migration before, the one it migrates to, its version, and when the last backup was noted. If an upgrade goes wrong in a way a rollback cannot fix, restore the backup that this marker names, as in [backup-restore.md](backup-restore.md), and start the version you had.

## Experimental parts after an upgrade

Since migration 0054 (ADR-043, spec 069), automation rules, databases, bundles between instances and restore from the console are experimental and off on a new installation. The upgrade keeps on what the installation already used, once, from the traces each part leaves:

| Part | Kept on when |
|---|---|
| Automation rules | a rule exists, at the instance or in a space |
| Databases | a database exists, also one in the trash |
| Bundles between instances | a bundle export is still listed among the jobs, or the audit log holds a bundle import |
| Restore from the console | a restore was ever asked for in the console |

Afterwards, look under Administration, Organising, Experimental parts, and turn on what you use that left no trace: a bundle export whose job was already cleaned up, for example. Turning a part off deletes nothing: a rule stays and stops running, a database stays and can still be edited, only new ones wait. Public sites and analytics keep their own switches, unchanged by the upgrade.

## Rolling back

Expand migrations leave the old structure in place, so the previous version runs on a database the new version has migrated. Roll back by starting the previous version.

This stops being possible once a contract migration has run, which is why they wait. A release that is older than a contract migration in the database refuses to start, and says which migration it is:

```
error: db: a newer release already removed what this release may need: contract migration 0099_example ran; start that release or a newer one, or restore the backup made before the upgrade
```

## Skipping versions

A jump over several releases is allowed: the binary runs every migration in order. Zero downtime is only promised between consecutive releases. An installation up to six months old may jump straight to the newest release, with a short stop: stop the old nodes first, then start the new ones. Older than that, upgrade in steps of at most six months ([version-skew.md](version-skew.md)).

## Watching an upgrade

`/metrics` shows how far it is:

| Metric | Means |
|---|---|
| `diffluence_schema_migration` | The highest migration applied to the database |
| `diffluence_migrations_pending_expand` | Expand migrations of this node not applied yet. Above zero, the node says `migrations` on `/readyz` |
| `diffluence_migrations_pending_contract` | Contract migrations of this node that wait for every node to know them. If this stays above zero long after every node runs the new release, look at the node list in the admin console |

## Proof

`scripts/upgrade-check.sh <commit>` builds the release at that commit and the one in the checkout, and runs `scripts/upgrade-test.sh` between them. That test starts the old version with traffic, starts the new one on the same database, runs both, stops the old one, and fails on any failed request.

It also writes across versions: the old version publishes a page, saves a draft, comments and uploads a file; the new one reads all of it, writes its own, and the old one reads that while both run. Then it stops the new one and starts the old one again on the migrated database, as a rollback, which reads everything both wrote. At the end no job may have failed, no event may be set aside and the outbox must be empty, and the audit chain both wrote must hold.

CI runs it on every pull request and every push to main, with the target branch or the previous commit of main as release N.

Run on 2026-11-09 from the main branch of that day to this build: see [the report of wave O7](../reports/2026-11-09-wave-o7.md).
