# Backup and restore

Written for: operators. Goal from blueprint 07: lose at most 4 hours of work, and be back within 4 hours.

## What must be backed up

| What | Where | Why |
|---|---|---|
| Database | PostgreSQL | Pages, versions, users, permissions, audit log |
| Stored files | `objects/` in the data directory | Attachments, encrypted with keys that `master.key` protects (spec 032). `previews/` below it can be left out: it is made again |
| `master.key` | data directory | Decrypts TOTP seeds, signs audit checkpoints |
| `secrets/` | next to `compose.yaml` | Database passwords |

A backup holds all of these, each encrypted with AES-256. The queue data (`nats/`) needs no backup: the outbox in the database holds what was not delivered yet.

## The backup key

Make it once, and keep it away from the backups:

```sh
head -c 32 /dev/urandom | base64 >/root/diffluence-backup.key
chmod 600 /root/diffluence-backup.key
```

**Without this key a backup cannot be restored.** Keep a second copy in a password safe or on paper. Whoever has both the key and a backup can read everything, including TOTP seeds, so never store them together. The script refuses a key inside the backup folder, and refuses to run without one.

## Docker Compose

```sh
bash scripts/backup.sh /backups /root/diffluence-backup.key   # run it from cron, then copy the folder off this machine
```

The script needs `gpg`. Everything streams through it, so plain data never reaches the disk. It dumps the database first and the files second, so a backup can hold a file no page uses, but no page that points at a missing file. Every backup has a `SHA256SUMS` file, which can be checked without the key.

A daily run gives you at most a day of loss. To reach the 4-hour goal, run it every few hours, or use pgBackRest with WAL archiving for a loss of minutes, which is the advice for larger installations. pgBackRest then has its own encryption, which you must turn on.

Backups from before this format were plain folders. Restore those with the `restore.sh` of that version.

## Restore

On a clean machine with Docker:

```sh
git clone https://github.com/Dr1nS0menOp/Diffluence.git && cd Diffluence
bash scripts/restore.sh /backups/diffluence-20261104T182225Z /root/diffluence-backup.key
```

The script checks the checksums, checks the key before it changes anything, restores the database and files, starts Diffluence and verifies the audit chain. Log in afterwards and open a few pages.

## Practise it

```sh
bash scripts/restore-drill.sh
```

This installs Diffluence, adds content, proves that a backup is refused without a key and with the key beside it, makes a backup, destroys everything, proves that a wrong key does not open it, restores on a clean installation, and proves that the page, attachment, search, login, audit chain and database role separation all came back. It uses its own compose projects and removes them afterwards. CI runs it every night.

Last run on 2026-11-06: passed in 29 seconds on a small data set. That says the procedure works, not how long 100 GB takes. Time it on your own data before relying on the 4-hour goal.
