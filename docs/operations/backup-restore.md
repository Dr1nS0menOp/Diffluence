# Backup and restore

Written for: operators. Goal from blueprint 07: lose at most 4 hours of work, and be back within 4 hours.

## What must be backed up

| What | Where | Why |
|---|---|---|
| Database | PostgreSQL | Pages, versions, users, permissions, audit log |
| Stored files | `objects/` in the data directory | Attachments |
| `master.key` | data directory | Decrypts TOTP seeds, signs audit checkpoints |
| `secrets/` | next to `compose.yaml` | Database passwords |

Keep `master.key` and `secrets/` **apart** from the rest. The queue data (`nats/`) needs no backup: the outbox in the database holds what was not delivered yet.

## Docker Compose

```sh
sh scripts/backup.sh /backups        # run it from cron, then copy the folder off this machine
```

The script dumps the database first and the files second, so a backup can hold a file no page uses, but no page that points at a missing file. Every backup has a `SHA256SUMS` file.

A daily run gives you at most a day of loss. To reach the 4-hour goal, run it every few hours, or use pgBackRest with WAL archiving for a loss of minutes, which is the advice for larger installations.

## Restore

On a clean machine with Docker:

```sh
git clone https://github.com/Dr1nS0menOp/Diffluence.git && cd Diffluence
sh scripts/restore.sh /backups/diffluence-20261104T182225Z
```

The script checks the checksums, restores the database and files, starts Diffluence and verifies the audit chain. Log in afterwards and open a few pages.

## Practise it

```sh
bash scripts/restore-drill.sh
```

This installs Diffluence, adds content, makes a backup, destroys everything, restores on a clean installation, and proves that the page, attachment, search, login, audit chain and database role separation all came back. It uses its own compose projects and removes them afterwards.

Last run on 2026-11-04: passed in 22 seconds on a small data set. That says the procedure works, not how long 100 GB takes. Time it on your own data before relying on the 4-hour goal.
