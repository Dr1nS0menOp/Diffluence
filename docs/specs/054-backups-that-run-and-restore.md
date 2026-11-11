# Spec 054: Backups that run and restore

Wave O3, wave 3 of spec 048. Study: [07.2 Backup, restore and disaster recovery](../blueprint/07-operations/02-backup-and-recovery.md). Lines: OPS-079 to OPS-082, and the follow-up of ADR-040 that an installation stuck in the backup refusal showed on 2026-11-11.

## What happened

ADR-040 made an upgrade refuse without a backup noted in the last 24 hours. The first installation that met it had never run `scripts/backup.sh`. The node exited, Compose restarted it, it exited again: a loop, with the reason only in the log. `backup.sh` noted the backup through that node and could not reach it (fixed in `bad3e8a`: the note goes through a one-off container). Three lessons:

1. A rule that asks for backups is only fair when backups happen without anyone remembering them.
2. A refusal must be visible where people look, the browser and `docker compose ps`, and must not loop.
3. An installation that never knew the rule should not be stopped by it the first time.

## Plan

### Step 1: automatic backups by default (OPS-080)

- A `backup` service in `compose.yaml`, on by default. It runs the same steps as `scripts/backup.sh` every `DIFFLUENCE_BACKUP_INTERVAL` (default `4h`, the RPO of blueprint 07), keeps the newest `DIFFLUENCE_BACKUP_KEEP` backups (default 42, a week), and notes each complete backup with `diffluence backup note`.
- Its image is a stage of the main `Dockerfile` (`target: backup`): `postgres:16` for a `pg_dump` of the same major version, `gnupg` from Debian, and the `diffluence` binary of the same build. No Go or npm dependency is added.
- It reads the database as the owner (`migrate_database_url`), the data volume read-only, and `./secrets` read-only. It writes to `${DIFFLUENCE_BACKUP_DIR:-./backups}`. The key is `backup.key` in `${DIFFLUENCE_BACKUP_KEY_DIR:-./backup-key}`, mounted read-only.
- `scripts/setup-secrets.sh` makes the key on a new installation, and on an existing one when run again; it never overwrites one.
- Failure paths: no key, or an empty one, means a clear log line on every interval and no backup, never a crash loop. A key folder that is the backup folder is refused. A failed backup leaves no folder that looks complete: it is written under a temporary name and renamed only when every part and `SHA256SUMS` are written. The health page already warns after 26 hours without a noted backup (OPS-077).
- `docker compose run --rm backup once` makes one backup and exits with its result.
- The backup format is the one `scripts/restore.sh` reads; the restore drill restores a backup of the service to prove it.

### Step 2: a waiting node instead of a restart loop

- `app.New` returns an `*app.UpgradeRefused` with every problem and its kind (backup, disk, queue, extension) when the checks before an upgrade refuse.
- `diffluence serve` then does not exit. It serves on the same address:
  - `/livez`: 200, so the process is alive.
  - `/readyz`: 503 with `{"status":"waiting_for_upgrade","problems":[kinds]}`, so no load balancer sends people.
  - `/api/...`: 503 JSON with the code `upgrade_waiting` and `Retry-After: 60`.
  - Every other path: a plain HTML page, 503, that says the upgrade waits, names the kind of each problem and the commands that fix it. It shows no numbers, dates or paths, because anyone can open it. The full reasons are in the log.
- It touches nothing in the database. It tries again every minute, and at once on `SIGHUP` (`docker compose kill -s HUP diffluence`). `backup.sh` sends that signal after noting a backup, to a node that is unhealthy only, since a release before this one stops on SIGHUP; the backup service has no Docker socket, by design, so the node's minute retry picks its backup up.
- The Compose health check still reports not ready, and `docker compose ps` shows the container running. `diffluence healthcheck` prints the reason it got.

### Step 3: the first upgrade under the rule (ADR-042)

- An installation with data, no backup ever noted, and no earlier waiver, is not refused for the backup on its next upgrade. The node logs a warning at `WARN`, writes `system.upgrade_without_backup` with `{"reason":"first_upgrade_under_the_backup_rule"}` to the audit log, and goes on. The health page keeps warning that no backup was ever noted.
- From the next upgrade on the rule holds in full, because the waiver is in the audit log.
- The other checks (disk, queue, extension) are never waived.

### Step 4: `diffluence migrate --without-backup`

- `diffluence migrate` now runs the backup part of the checks too. Before, it ran no check at all, which made it a silent way around ADR-040.
- `--without-backup` upgrades once without a backup and writes `system.upgrade_without_backup` with `{"reason":"operator_override"}`. It is preferred over leaving `DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE=off` set, which also waives every later upgrade.
- `migrate` also writes the upgrade marker (`system.upgrade_started`), as a migrating node does.
- `--contract` alone, with nothing to expand, needs no backup check: it runs only after the grace of a finished upgrade.

### Step 5: the drill covers the note while the node is down

- The restore drill stops the node and runs `backup.sh` again: the note must arrive through the one-off container.
- It runs the backup service once and restores that backup, so the default path is the one proved every night.

### Step 6: the weekly restore of a real installation (OPS-081)

- `scripts/restore-check.sh <backup folder> <key file>` restores the newest backup into a throw-away Compose project on its own port, runs `audit verify`, `key check`, counts pages, files and people, prints the time each part took, removes the project, and exits non-zero on any failure. It never touches the live installation.
- The guide gives a systemd timer with `OnFailure=` and a cron line, so a failure reaches a person.

### Step 7: pgBackRest and point-in-time recovery (OPS-079), the 3-2-1 note (OPS-082)

- `backup-restore.md` gets a tested-by-reading pgBackRest configuration for continuous WAL archiving, encrypted, to a second disk or S3-compatible storage, with the alert on archive failures and WAL size the study asks for. pgBackRest is not shipped: it changes the PostgreSQL image, and the drill against it needs the test server (wave 7). OPS-079 stays partly.
- It says plainly that a standby or RAID is not a backup, and what 3-2-1 asks of a small installation.

## Decisions

| | Decision | Why |
|---|---|---|
| D1 | The backup service is on by default, not a profile | ADR-040 refuses an upgrade without a backup; a rule that needs a backup must come with one |
| D2 | The service image is a stage of the main Dockerfile | One build, the same binary for the note, no image from elsewhere |
| D3 | The waiting page names kinds, not values | It is public; free space and backup dates are an administrator's business |
| D4 | The first waiver is per installation, kept in the audit log | The log already holds the backup notes; a second store could disagree |
| D5 | `migrate` checks the backup | A command that skips the rule silently is a hole in it |
| D6 | The service runs as root with only `DAC_OVERRIDE` and `CHOWN` | It must read files the app owns (`master.key`, mode 600), and hand each backup to the owner of the backup folder so that user can restore it |

## Tests, failure paths first

- Go: the backup part of the checks with every case (none, old, inside the age, off, first waiver, waiver used); the refusal carries kinds; the waiting handler for each path, with the HTML escaped and no reason text in the page; the waiting loop returns on a retry and on shutdown and frees the address; `migrate` refuses, waives with the flag, and leaves both audit entries; a node waives once and refuses the second time.
- Shell: the restore drill (steps 5 and 6) every night; the upgrade test unchanged.
- Mutations for every check: the waiver condition, the flag, the kinds on the page, the retry, the key checks in the service.

## Load budget (ARC-238)

| What | Cost |
|---|---|
| The backup service | One `pg_dump` and one `tar` every 4 hours, outside the request path; a database read of the whole data set |
| A waiting node | No database work; one retry a minute |

## What one machine cannot prove

- pgBackRest with WAL archiving, and point-in-time recovery to a chosen second (wave 7).
- The time a 100 GB restore takes (OPS-078, wave 7).
- Copying backups off the machine: the operator's choice of place; the guide says how.
