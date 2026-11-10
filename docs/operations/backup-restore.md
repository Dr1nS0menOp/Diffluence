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

The script needs `gpg`. Everything streams through it, so plain data never reaches the disk. It dumps the database first and the files second, so a backup can hold a file no page uses, but no page that points at a missing file. Every backup has a `SHA256SUMS` file, which can be checked without the key. Once every part is written, it notes the backup in the audit log as `system.backup_noted`, which the checks before an upgrade read ([upgrade.md](upgrade.md)).

A daily run gives you at most a day of loss. To reach the 4-hour goal, run it every few hours, or use pgBackRest with WAL archiving for a loss of minutes, which is the advice for larger installations. pgBackRest then has its own encryption, which you must turn on.

Backups from before this format were plain folders. Restore those with the `restore.sh` of that version.

## Restore

On a clean machine with Docker:

```sh
git clone https://github.com/Dr1nS0menOp/Diffluence.git && cd Diffluence
bash scripts/restore.sh /backups/diffluence-20261104T182225Z /root/diffluence-backup.key
```

The script checks the checksums, checks the key before it changes anything, restores the database and files, starts Diffluence, verifies the audit chain, and runs `diffluence key check` to prove that every stored file and secret opens with the restored `master.key`. Log in afterwards and open a few pages.

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

## Practise it

```sh
bash scripts/restore-drill.sh
```

This installs Diffluence, adds content, starts replacing `master.key`, proves that a backup is refused without a key and with the key beside it, makes a backup halfway through the key change and proves it was noted once, destroys everything, proves that a wrong key does not open it, restores on a clean installation, and proves that the page, attachment, search, login, audit chain and database role separation all came back. Then it finishes the key change on the restored installation, and proves the file still opens with only the new key. It uses its own compose projects and removes them afterwards. CI runs it every night.

Last run on 2026-11-06: passed in 29 seconds on a small data set. That says the procedure works, not how long 100 GB takes. Time it on your own data before relying on the 4-hour goal.
