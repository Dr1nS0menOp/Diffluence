# Upgrading Diffluence

Written for: operators.

```sh
git pull && docker compose up -d --build     # or: docker compose pull && docker compose up -d
```

That is all. There are no manual steps.

With one node, Compose stops the old container and starts the new one, so people wait a few seconds. Zero downtime needs two or more nodes behind a load balancer, replaced one at a time.

## What happens

1. The new version starts and first checks that it may migrate (below). Then it applies **expand** migrations. They only add things, so the running version keeps working.
2. With several nodes, you replace them one at a time. Old and new run side by side on the same database.
3. Every node reports its version and the migrations it knows, every 15 seconds.
4. When every live node knows a pending **contract** migration, and the newest node has been up for 10 minutes, the worker runs it automatically. It is recorded in the audit log as `system.contract_migrated`. The 10 minutes leave room to roll back right after an upgrade.

`diffluence migrate --contract` runs them by hand if you ever need to.

## The checks before migrating

A node only checks when it has something to migrate, so a plain restart never waits for them. If a check fails, the node does not start, and the log names every reason at once, such as:

```
error: upgrade refused before migrating: the data folder has 300 MiB free and needs 512 MiB; the queue (NATS) does not answer
```

| Check | Refuses when |
|---|---|
| PostgreSQL | The server is older than PostgreSQL 16. Also for `diffluence migrate` |
| Free space | The data folder has less than 512 MiB free |
| Queue | NATS does not answer |
| Backup | No backup was noted in the last 24 hours, or within `DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE` when that is set, such as `48h`. Set it to `off` to upgrade without a recent backup: the node then only warns in the log (ADR-040) |
| Newer database | A newer release has already run a contract migration. Also for a node with `DIFFLUENCE_MIGRATE_ON_START=false` |

**Since this release an upgrade refuses to start without a backup of the last 24 hours.** Before you upgrade, run `scripts/backup.sh`. The refusal looks like this, and nothing is migrated:

```
error: upgrade refused before migrating: no backup was noted: run scripts/backup.sh first, or set DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE=off to upgrade without one
```

A new installation is never refused: it has nothing to back up.

`scripts/backup.sh` notes each complete backup in the audit log as `system.backup_noted`. A backup made another way can be noted with `docker compose exec diffluence diffluence backup note`, but only once it is complete.

Fix the cause and start the node again. Nothing was migrated.

## The marker before an upgrade

Before a node migrates a database that holds data, it writes `system.upgrade_started` to the audit log: the highest migration before, the one it migrates to, its version, and when the last backup was noted. If an upgrade goes wrong in a way a rollback cannot fix, restore the backup that this marker names, as in [backup-restore.md](backup-restore.md), and start the version you had.

## Rolling back

Expand migrations leave the old structure in place, so the previous version runs on a database the new version has migrated. Roll back by starting the previous version.

This stops being possible once a contract migration has run, which is why they wait. A release that is older than a contract migration in the database refuses to start, and says which migration it is:

```
error: db: a newer release already removed what this release may need: contract migration 0099_example ran; start that release or a newer one, or restore the backup made before the upgrade
```

## Skipping versions

A jump over several releases is allowed: the binary runs every migration in order. Zero downtime is only promised between consecutive releases. For a larger jump, stop the old nodes first.

## Watching an upgrade

`/metrics` shows how far it is:

| Metric | Means |
|---|---|
| `diffluence_schema_migration` | The highest migration applied to the database |
| `diffluence_migrations_pending_expand` | Expand migrations of this node not applied yet. Above zero, the node says `migrations` on `/readyz` |
| `diffluence_migrations_pending_contract` | Contract migrations of this node that wait for every node to know them. If this stays above zero long after every node runs the new release, look at the node list in the admin console |

## Proof

`scripts/upgrade-check.sh <commit>` builds the release at that commit and the one in the checkout, and runs `scripts/upgrade-test.sh` between them. That test starts the old version with traffic, starts the new one on the same database, runs both, stops the old one, and fails on any failed request. Then it stops the new one and starts the old one again on the migrated database, as a rollback, and checks the audit chain both wrote.

CI runs it on every pull request and every push to main, with the target branch or the previous commit of main as release N.

Run on 2026-11-09 from the main branch of that day to this build: see [the report of wave O7](../reports/2026-11-09-wave-o7.md).
