# Backup and restore

Written for: operators. Goal from blueprint 07: lose at most 4 hours of work, and be back within 4 hours.

## What must be backed up

| What | Where | Why |
|---|---|---|
| Database | PostgreSQL | Pages, versions, users, permissions, audit log |
| Stored files | `objects/` in the data directory | Attachments, encrypted with keys that `master.key` protects (spec 032). `previews/` below it can be left out: it is made again |
| `master.key` | data directory | Opens every stored file, TOTP seeds, webhook secrets and queued account mail; signs audit checkpoints |
| `master.key.next`, `master.key.previous` | data directory, only while the key is being replaced | Open what the new or the old key sealed |
| `secrets/` | next to `compose.yaml` | Database passwords |

A backup holds all of these, each encrypted with AES-256. The queue data (`nats/`) needs no backup: the outbox in the database holds what was not delivered yet.

## The backup key

`sh scripts/setup-secrets.sh` makes it as `backup-key/backup.key`, beside `compose.yaml` but outside `secrets/`, which every backup holds. On an installation from before spec 054, run the script once more: it makes what is missing and overwrites nothing. To make a key by hand:

```sh
head -c 32 /dev/urandom | base64 >/root/diffluence-backup.key
chmod 600 /root/diffluence-backup.key
```

**Without this key a backup cannot be restored.** Keep a second copy in a password safe or on paper. Whoever has both the key and a backup can read everything, including TOTP seeds, so never store them together. The backup service and the script refuse a key inside the backup folder, and refuse to run without one.

The backup service also derives two more keys from it (spec 070): the key that signs the audit checkpoints, and the key of the codes that confirm a restore or fewer backups (spec 076). The server never reads this file. Replacing the backup key replaces the checkpoint key too; the backup service records the change in the audit chain on its next round, and older checkpoints keep verifying.

## Docker Compose: the backup service

`compose.yaml` runs a backup service by default (spec 054). Every 4 hours it backs up into `./backups`, keeps the newest 42 (a week), and notes each one, so an upgrade always finds a recent backup.

| Setting in `.env` | Default | Meaning |
|---|---|---|
| `DIFFLUENCE_BACKUP_INTERVAL` | `4h` | How often: `30m`, `4h`, `1d` |
| `DIFFLUENCE_BACKUP_KEEP` | `42` | How many backups stay in the folder |
| `DIFFLUENCE_BACKUP_DIR` | `./backups` | Where backups are written |
| `DIFFLUENCE_BACKUP_KEY_DIR` | `./backup-key` | The folder that holds `backup.key` |

```sh
docker compose run --rm backup once     # one backup now
docker compose logs backup              # what it did, and why it did not
```

**In the console** (Administration, System, Backups; spec 058): the schedule and the number kept set there win over these two variables, which stay the default. "Back up now" asks the service for a backup, which it starts within a minute (at most 4 an hour). The page lists every backup with its size, whether it was complete or which step failed, and whether a test restore of it passed; it shows when the service last checked in and whether it holds a key (only a fingerprint, never the key). A failed backup or test restore shows on System health and is mailed to the administrators, at most twice a day. The service talks to the console through the database, with the access it already has, so the same page works when the worker is a Kubernetes CronJob or a systemd timer that runs `diffluence backup next` and `diffluence backup report`.

**Keeping fewer backups, or backing up less often** (spec 076), is a system administrator's change, with a fresh login and a second factor. The service then writes a code to its log, valid 30 minutes, which a system administrator enters on the same page:

```
To keep the newest 7 backups and back up every 168 hours, enter this confirmation code in the console: 12345-67890 (valid 30 minutes)
```

Until the right code is entered, the service keeps what it kept, and the page says so; three wrong codes, or none within 30 minutes, end the ask. More backups, or more often, take effect at once. The service keeps what it holds to in `./backups/.diffluence-held.json`, a small file the server never reaches; it never removes more old backups than that file says, also for `docker compose run --rm backup once`. Leave the file in place. A new or emptied backup folder starts from the larger of the console's number and `DIFFLUENCE_BACKUP_KEEP`.

The service is also the only process that holds the owner of the tables (spec 070). Every round, at most a minute apart, it runs the migrations of a new release, then the contract migrations once their grace period is over, and signs the head of the audit chain every five minutes. It writes those checkpoints to its own log as witness lines.

The service starts its first backup once the tables exist, and then whenever the last try is older than the interval. Without a key it logs why on every interval and makes nothing; after adding the key, `docker compose restart backup` tries again at once. A backup is written under a name that starts with `.partial-` and renamed only when every part and `SHA256SUMS` are in, so a backup that died halfway is never picked for a restore. Each backup belongs to the owner of the backup folder, so that user can restore it.

**Copy `./backups` off this machine**, every day: to another disk, another building, or S3-compatible storage. A backup on the same disk is gone with the disk. Copy the key to a different place than the backups.

## Docker Compose: the script

Where backups are run from cron or another scheduler instead, the script does the same:

```sh
bash scripts/backup.sh /backups /root/diffluence-backup.key   # run it from cron, then copy the folder off this machine
```

The script needs `gpg`. Everything streams through it, so plain data never reaches the disk. It dumps the database first and the files second, so a backup can hold a file no page uses, but no page that points at a missing file. Every backup has a `SHA256SUMS` file, which can be checked without the key. Once every part is written, it notes the backup in the audit log as `system.backup_noted`, which the checks before an upgrade read ([upgrade.md](upgrade.md)). When the node is not running, it notes the backup through a one-off container. When the node waits before an upgrade, it asks it to check again at once.

A backup every 4 hours loses at most 4 hours of work. For a loss of minutes, use pgBackRest with WAL archiving, below.

## A standby is not a backup

A standby server, RAID or a snapshot on the same storage protects against a broken disk, not against a mistake: a deleted page is deleted on the standby too, within a second. Keep the **3-2-1 rule**: three copies of the data, on two kinds of storage, one of them away from the building. For a small installation that means: the database, the backups in `./backups` on a second disk, and a copy in another place, encrypted, which the backup files already are.

## pgBackRest and point-in-time recovery

For a loss of minutes, or to go back to a chosen moment before a mistake, PostgreSQL archives its write-ahead log continuously. pgBackRest (MIT) is the common tool for it: parallel, compressed, incremental, checked, encrypted, and able to send to S3-compatible storage. Diffluence itself needs nothing for it; it runs beside PostgreSQL. It is not part of `compose.yaml`, because it changes the PostgreSQL image, and has not been proven here yet on a real server (OPS-079).

The outline, from the pgBackRest user guide:

```ini
# /etc/pgbackrest/pgbackrest.conf
[global]
repo1-path=/var/lib/pgbackrest
repo1-cipher-type=aes-256-cbc
repo1-cipher-pass=<a long random secret, kept like the backup key>
repo1-retention-full=2
start-fast=y

[diffluence]
pg1-path=/var/lib/postgresql/16/main
```

```ini
# postgresql.conf
archive_mode = on
archive_command = 'pgbackrest --stanza=diffluence archive-push %p'
wal_level = replica
```

Then `pgbackrest --stanza=diffluence stanza-create`, a full backup weekly and a differential one daily from a timer, and `pgbackrest --stanza=diffluence --type=time "--target=2026-11-11 09:30:00" restore` to go back to a moment. Two alerts are not optional: one on failed archiving (`pg_stat_archiver.failed_count` rising), and one on the size of `pg_wal`. When archiving fails, PostgreSQL keeps every WAL file until the disk is full and the database stops. Keep `backup-key`, `master.key` and `secrets/` in the backups of the backup service as well: pgBackRest saves the database only.

Backups from before this format were plain folders. Restore those with the `restore.sh` of that version.

## Restore

On a clean machine with Docker:

```sh
git clone https://github.com/Dr1nS0menOp/Diffluence.git && cd Diffluence
bash scripts/restore.sh /backups/diffluence-20261104T182225Z /root/diffluence-backup.key
```

The script checks the checksums, checks the key before it changes anything, restores the database and files, brings a backup of an older release up to this one as the owner of the tables in a one-off container, starts Diffluence, verifies the audit chain, and runs `diffluence key check` to prove that every stored file and secret opens with the restored `master.key`. Then start the backup service with `docker compose up -d backup`. Log in afterwards and open a few pages.

The dump does not carry the defaults of the database itself, the settings `plan_cache_mode` and `jit` that make every session plan its queries well (specs 061 and 063). A restore into a fresh database would lose them, so every run of migrations sets them again: the node that starts after the restore, `diffluence migrate`, and the restore from the console. Nothing needs doing by hand. To check, `SHOW plan_cache_mode` gives `force_custom_plan` and `SHOW jit` gives `off` in a plain `psql` session. On a managed database whose owner is not Diffluence's owner role, the settings cannot be stored in the database; the node still sends both when it connects, and only a connection through PgBouncer goes without them.

### Over the live installation, from the console

To put a running installation back to an earlier backup, an administrator uses Administration, System, Backups (spec 059). The page says what is lost, asks for the installation's name typed out, and asks for a second factor: a code from the authenticator app, a recovery code, or a passkey confirmed just before. At most one restore a day.

**The backup service asks for a code first** (spec 070). Within a minute of the request it writes one line to its own log, which the server cannot read or write:

```
To restore diffluence-20261104T182225Z, enter this confirmation code in the console: 12345-67890 (valid 30 minutes)
```

Read it with `docker compose logs backup`, or in the log collector that keeps that log, and enter it on the same page. The code is made from the backup key, so only the backup service can make it, and it fits this one request. Spaces and dashes do not matter. A wrong code is counted and recorded, and the page says how many tries are left; after the third, or without a code for 30 minutes, the restore ends and nothing changes. A request that did not come through the console, or whose backup changed after it was confirmed, ends the same way, recorded as `backup.restore_refused`.

The backup service does the work:
1. It checks the backup while people still work: the checksums, that the key opens it, and that this release knows its database. A backup that fails here changes nothing.
2. It stops every node. Each shows a waiting page, and `/readyz` answers `restoring`.
3. It makes a safety backup of the installation as it is now.
4. It restores the database in one transaction, then the files and keys.
5. It brings the database up to this release, writes the restore into the restored audit log, and checks the audit chain and the keys.
6. The nodes serve again within seconds.

If it stops halfway, every node keeps waiting and the page says so. Go back to the safety backup with:

```sh
docker compose run --rm backup way-back
```

The way back restores only a safety backup the backup service made itself for that restore. For anything else it refuses, records it, and leaves the nodes waiting: restore by hand with `scripts/restore.sh` once you know what happened.

Without the console, an operator on the machine asks for the same restore inside the backup service, which holds the key and so needs no code: `docker compose exec backup diffluence backup restore-request --name diffluence-20261104T182225Z --confirm "<installation name>"` (or `docker compose run --rm --entrypoint diffluence backup backup restore-request ...` while the service is stopped). The backup service picks it up within a minute. The same command in the server's container makes a request that waits for the code in the log, as one from the console does. The schema `ops` steers a restore and is never restored itself; backups hold its tables without rows, so a confirmation code never reaches a backup.

## master.key

`master.key` protects every stored file and every secret Diffluence keeps. **Without it, no file can be read, and nobody can log in with an authenticator app.** The backup keeps it, encrypted, in `master.key.gpg`. Keep a second copy of the backup key apart from the backups, as above.

### Check that it opens everything

```sh
docker compose exec diffluence diffluence key check           # the key, and the end of every file
docker compose exec diffluence diffluence key check --deep    # every byte of every file
```

It counts what each key protects and names anything that does not open, by table and row or by file name, never by its content. It exits with an error when something does not open.

### Replace it

Replace it on a schedule, when someone who knew it leaves, or when you suspect it leaked. Diffluence keeps running throughout.

```sh
docker compose exec diffluence diffluence key add      # writes master.key.next
docker compose restart diffluence                      # every node must load the new key
docker compose exec diffluence diffluence key promote  # the new key seals; the old one moves to master.key.previous
docker compose restart diffluence
docker compose exec diffluence diffluence key reseal   # seals every file and secret again
docker compose exec diffluence diffluence key check --require-current
docker compose exec diffluence diffluence key retire   # removes the old key, only when nothing needs it
```

With several nodes, copy the key files to every node after `add` and after `promote`, and restart each node before the next step. Where the key comes from a read-only mount, such as a Kubernetes secret, place `master.key.next` beside it yourself, then the new `master.key` with `master.key.previous`. Make a backup after `promote`, and keep backups from before the change until the change is done: they need the old key.

Each step can be run again after a crash, and refuses when an earlier change is not finished. `retire` refuses while anything still needs the old key. Every step is in the audit log, naming the key by its id only.

What this does not do yet: it keeps the root from which the audit signing key and login fingerprints are derived, so if the key file itself was stolen, a thief can still forge audit checkpoints. Links to unsubscribe in mails sent before the change stop working once the old key is retired.

### If it is lost or damaged

1. **Stop.** Do not start Diffluence with a new, empty key: it would write new files that no backup can open together with the old ones.
2. **Restore `master.key` from the newest backup** with the backup key: `gpg --decrypt master.key.gpg >master.key`, mode 600, owned by the user Diffluence runs as. If the key was being replaced when the backup was made, restore `master.key.next.gpg` and `master.key.previous.gpg` beside it as well.
3. **Run `diffluence key check --deep`.** If everything opens, start Diffluence. If files written after that backup do not open, restore the whole backup as above: the key and the data must belong together.
4. **If no copy of the key exists, the stored files and authenticator seeds are lost.** Pages and their history in the database stay readable. Restore the files from a backup whose key you do have, ask everyone to set up their authenticator app again, and make new secrets for webhooks.

## Check every week that your backups restore

A backup that was never restored is a hope. The best-known outages of the last decade lost data because backups had silently stopped working. Restore your own newest backup every week, on the same machine, without touching the live installation:

```sh
bash scripts/restore-check.sh ./backups ./backup-key/backup.key
```

It restores the newest backup into a throw-away Compose project on port 18097 (`DIFFLUENCE_RESTORE_CHECK_PORT`), verifies the audit chain and that every key opens, counts pages, files and people, prints how long it took, and removes the project. It tells the console the result, which shows it under Backups and mails the administrators when it failed. It exits with an error when anything fails. Run it from a timer that tells a person when it fails, for example with systemd:

```ini
# /etc/systemd/system/diffluence-restore-check.service
[Unit]
Description=Restore the newest Diffluence backup into a throw-away project
OnFailure=notify-admin@%n.service

[Service]
Type=oneshot
WorkingDirectory=/opt/diffluence
ExecStart=/usr/bin/bash scripts/restore-check.sh ./backups ./backup-key/backup.key

# /etc/systemd/system/diffluence-restore-check.timer
[Timer]
OnCalendar=Sun 03:00
Persistent=true

[Install]
WantedBy=timers.target
```

With cron, `MAILTO` sends the output of a failed run: `0 3 * * 0 cd /opt/diffluence && bash scripts/restore-check.sh ./backups ./backup-key/backup.key >/dev/null`. Leave room on the disk for a second copy of the database and files while it runs.

## Practise it

```sh
bash scripts/restore-drill.sh
```

This installs Diffluence, adds content, starts replacing `master.key`, proves that a backup is refused without a key and with the key beside it, makes a backup halfway through the key change and proves it was noted once, and makes one more while the node is stopped and proves it was noted too. It runs the backup service once, proves it refuses without a key, and that its backup is complete and encrypted. It destroys everything, proves that a wrong key does not open the service's backup, restores it on a clean installation, and proves that the page, attachment, search, login, audit chain and database role separation all came back. Then it finishes the key change on the restored installation, proves the file still opens with only the new key, and runs the weekly restore check on the service's backups. It uses its own compose projects and removes them afterwards. CI runs it every night.

Last run on 2026-11-06: passed in 29 seconds on a small data set. That says the procedure works, not how long 100 GB takes. Time it on your own data before relying on the 4-hour goal.
