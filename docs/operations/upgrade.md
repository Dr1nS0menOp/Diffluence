# Upgrading Diffluence

Written for: operators.

```sh
git pull && docker compose up -d --build     # or: docker compose pull && docker compose up -d
```

That is all. There are no manual steps.

## What happens

1. The new version starts and applies **expand** migrations. They only add things, so the running version keeps working.
2. With several nodes, you replace them one at a time. Old and new run side by side on the same database.
3. Every node reports its version and the migrations it knows, every 15 seconds.
4. When every live node knows a pending **contract** migration, and the newest node has been up for 10 minutes, the worker runs it automatically. It is recorded in the audit log as `system.contract_migrated`. The 10 minutes leave room to roll back right after an upgrade.

`diffluence migrate --contract` runs them by hand if you ever need to.

## Rolling back

Expand migrations leave the old structure in place, so the previous version runs on a database the new version has migrated. Roll back by starting the previous version. This stops being true once a contract migration has run, which is why they wait.

## Proof

`scripts/upgrade-test.sh <old binary> <new binary>` starts the old version with traffic, starts the new one on the same database, runs both, stops the old one, and fails on any failed request. Run on 2026-11-04 from the walking skeleton to this build: 507 requests, 0 failed, audit chain intact across both versions.
