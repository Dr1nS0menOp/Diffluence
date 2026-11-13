# Spec 058: running the installation from the console

Wave AG, 2026-11-11. The product owner asked to manage as much as possible from the admin screen, backups first, on whatever the installation runs on (`08-roadmap.md`, "Running the installation from the admin screen"). This spec starts with a study: every command and every setting, sorted into what the console may change, what it only shows, and what stays a command and why. Then it plans the build, and the part that is too large for this wave: restoring over the live installation from the console.

## 1. The study

### 1.1 What the console already offers

Read in `web/src/views/admin*.tsx` and `internal/httpapi/handlers_*.go` on main `9522a83`:

| Already in the console | Where |
|---|---|
| Health checks (disk, database, backup age, migrations, work, audit sealing, mail, login, release) | System health (spec 051) |
| Checking the audit chain and the signed checkpoints | Audit (`GET /admin/audit-events/verify`) |
| Audit retention, with a reason past a year | Settings, section `retention` |
| Maintenance mode with a message | Settings (`PUT /admin/maintenance`) |
| Mail server test, mail queue | Mail |
| Failed jobs and events, retry and replay | Care |
| Lifting a password stop, ending a person's access by disabling them | Users |

So maintenance mode, the audit check and audit retention exist; this wave does not build them twice.

### 1.2 Every command

| Command | Sorting | Why |
|---|---|---|
| `serve` | command only | It is the server. |
| `migrate [--contract] [--without-backup]` | show only: the health page shows waiting contract migrations and when their grace ends (spec 053) | Migrations need the owner role, which the server does not hold while it serves (ADR-013's split of roles). A worker running contract migrations after the grace already exists (`contract-migrations` task). |
| `user create-admin` | command only | It is how the first administrator exists when nobody can log in. The console already creates users. |
| `setup-code` | command only | It proves access to the machine before anyone has an account (spec 038). |
| `audit verify [--witness FILE]` | console: check exists; the witness comparison stays a command | The witness is an application log outside the database (ADR-039). Handing it to the server to compare would make the server the judge of its own log. |
| `audit prune` | command only, with retention set in the console | It runs as the owner of the tables, which may remove audit rows; the server role may not (SEC-831). A "what would be removed" preview is planned in 3.3. |
| `backup note` | console, through the worker (this wave) | The worker notes each backup itself. |
| `key add/promote/reseal/check/retire` | command only for now; a guided page is its own later wave | Replacing `master.key` means writing key files on every node's disk and resealing everything. A page would need a worker on every node with write access to the key folder. Too large for this wave, and a mistake loses every file. |
| `session end --email / --all` | console (this wave) | Pure database work the server already does for itself. |
| `config check` | console shows its result (this wave) | Read-only; secrets show as `(set)`. |
| `healthcheck` | command only | It is the container's own probe. |
| `doctor` | the health page is its console face | Same checks (spec 051). |
| `client` | command only | It is a client of the API. |
| `version` | shown on System health | |

### 1.3 Every setting

The settings the server reads, from `internal/config/config.go` and `compose.yaml`:

| Setting | Sorting | Why |
|---|---|---|
| `DATABASE_URL`, `MIGRATE_DATABASE_URL`, `NATS_URL` | show only, password hidden | The server cannot connect to change where it connects. |
| `PUBLIC_URL`, `HTTP_ADDR`, `TRUSTED_PROXIES` | show only | A wrong value makes the console unreachable to correct it. |
| `ROLE`, `DATA_DIR`, `MASTER_KEY_FILE`, `PUBLISH_DIR` | show only | Per node and per disk; the server cannot move its own data. |
| `MIGRATE_ON_START`, `CONTRACT_GRACE`, `UPGRADE_BACKUP_MAX_AGE` | show only | They decide what a start does before the console exists. |
| `SESSION_IDLE_TIMEOUT`, `SESSION_MAX_AGE` | show only; the console's Access section already sets idle minutes within them | The configured value is the ceiling the console cannot raise. |
| `SHUTDOWN_DELAY` | show only | Read at start. |
| `SMTP_*`, `OIDC_*` | show only, secrets hidden | They hold passwords; storing them in the database would put them in every backup and every database copy. Kept in the environment or a file. |
| `COLLAB_*`, `PDF_*`, `COLLABORA_URL`, `DRAWING_URL`, `METRICS_TOKEN` | show only, secrets hidden | Addresses and tokens of other services. |
| `PASSWORD_BLOCKLIST_FILE`, `EXAMPLE_SPACE`, `OUTWARD_ALLOW_PRIVATE`, `INSTANCE_NAME` | show only | Read at start. |
| `LOG_LEVEL` | show only now; "log level for an hour" is planned in 3.3 | Needs every node to pick up a change; the settings cache can carry it. |
| `DIFFLUENCE_BACKUP_INTERVAL`, `DIFFLUENCE_BACKUP_KEEP` | **console** (this wave): the console's choice wins, the variable is the default | The worker asks the console each minute. |
| `DIFFLUENCE_BACKUP_DIR`, `DIFFLUENCE_BACKUP_KEY_DIR` | show only, through the worker | Folders of the worker's machine. |
| Disk thresholds (10% and 2%, spec 051) | command only for now; planned in 3.3 | Constants today; making them settings is small but is its own change with its own tests. |

## 2. Decisions

| # | Decision | Reason |
|---|---|---|
| D1 | The console controls, a worker does the work, and they meet in the database: `backup_requests`, `backup_runs`, `backup_restore_checks`, `backup_worker`, `backup_mail` (migration 0048). | The same tables work for the Compose service, a Kubernetes CronJob and a systemd timer. The key that opens backups never reaches the server. |
| D2 | The worker talks to the console through the `diffluence` binary in its image (`backup next`, `backup report`, `backup restore-checked`), with the **application role's database access it already has** for `backup note`. Not an API token. | The worker needs the database anyway (the dump reads it as the owner). An API token would add a secret, a network path to the app, and would fail exactly when the app is in its waiting page during an upgrade, when a backup matters most. The application role can do nothing here a token with one scope could not. |
| D3 | Every minute the worker asks `backup next`. It answers "now" for a console request or when the interval since the last try has passed, otherwise "wait" for at most 60 seconds. | A request is picked up within a minute, and the worker is seen as alive. |
| D4 | A request is claimed in the same transaction that hands it out. A claim older than 6 hours may be taken again. | Two workers never take the same request; a worker that died does not block "back up now" for ever. |
| D5 | One open request at a time, at most 4 an hour. | A backup reads the whole database and every file; pressing it in a loop must not wear the server down. |
| D6 | Failures are mailed by a server node every 5 minutes, at most once per kind in 12 hours; failures in between are folded in. The worker only writes rows. | Only the server holds the key that seals mail. A backup that fails every 4 hours must not fill inboxes. |
| D7 | The health page's backup line says "the latest backup failed" or "the latest test restore failed", ahead of the age. No new line. | A failure is the news even while an older backup is recent, and the console's pictures stay as they are on a healthy install. |
| D8 | The key shows as "present" and a 16-character start of a SHA-256 of it. | It shows a key exists and when it changed, and says nothing that helps to guess it. |
| D9 | The configuration page reads the environment and file again on each look, as `config check` does, and lists problems by key and line. | It shows what a restart would take; the problems never carry a value (spec 056). |
| D10 | Logging everyone out asks for the password again (a sensitive route) and a confirmation, and logs out the administrator too. Logging one person out needs their address. | It is an incident action; a stolen session must not use it as a weapon. Leaving the address empty must never log everyone out. |
| D11 | A replacement of `master.key` from the console is its own later wave. | See 1.2. |
| D12 | Backup history is kept 400 days, finished requests 90 days. | Long enough to see a pattern; the backups themselves are pruned by the worker. |

## 3. The plan

### 3.1 Built in this wave

1. Migration 0048: the five tables of D1.
2. `internal/backups`: `Next`, `ReportRun`, `ReportRestore`, `RequestNow`, `Overview`, `MailFailures`.
3. The settings section `backups` (interval in hours, number kept; zero is the worker's default).
4. `diffluence backup next | report | restore-checked`.
5. The backup service of Compose asks the console and reports each backup with the step that failed; a release without `backup next` falls back to the folder's own schedule.
6. `scripts/restore-check.sh` reports each weekly test restore to the console, through the running node or a one-off container.
7. The console: **Backups** (state of the worker, key, back up now, schedule, list with test restores, the restore command for a chosen backup), **Configuration** (show only), **Sessions** (one person, everyone).
8. The health line for a failed backup or test restore; mails to administrators.

Load budget (ARC-238): `backup next` is one upsert, one settings read and two indexed lookups a minute. The backup page is five small queries on tables the daily cleaner keeps small, polled every 15 seconds while open. The health facts gain two `LIMIT 1` lookups on indexed columns. Mail is one query every 5 minutes when nothing failed.

### 3.2 Restoring over the live installation from the console

Decided by the product owner on 2026-11-11: a full restore over the live installation must be possible from the admin screen. It is too large to build safely beside the rest of this wave, so it is its own step, planned here. Built in wave RS; spec 059 is the detailed plan and records where the build went further.

**Who and how.** Only an instance administrator. The route is sensitive and asks for a fresh second factor or passkey, not only the password. The administrator types the installation's name to confirm. Before the button, the page says in plain words what is lost: everything since that backup, for everyone, including pages, files, comments and accounts made since then.

**First a backup.** The request asks the worker for a backup of the current state first. Only when that backup is complete and noted does the restore go on, so the restore itself can be undone with the same page.

**Who does the work.** The worker, never the app:
1. The console writes a restore request (backup name, who, when) and an audit entry.
2. Every node sees the request in the database and switches to the waiting page of spec 054, the same one an upgrade without a backup uses; `/readyz` says `restoring`. No node writes to the database or the files while it waits.
3. The worker makes the safety backup, then restores the chosen backup into the database and the data folder, with the steps of `scripts/restore.sh`.
4. The worker runs `diffluence audit verify` and `diffluence key check` against the restored installation.
5. Only when both pass does it clear the request; the nodes leave the waiting page within a minute.

**Failure.** When any step fails, the nodes stay on the waiting page, which names the step and the safety backup. Restoring that safety backup is the way back, by the same page or by `scripts/restore.sh`.

**Audit.** A restore rewinds the database, and with it the audit log. So the worker writes the request and the result again into the restored installation's audit log, sealed (`audit.RecordSealed`), naming the backup, the safety backup, who asked and when. The restore leaves a trace even though it removed the history after the backup. The witness log of ADR-039, outside the database, keeps the checkpoints from before.

**Abuse.** At most one restore a day, and none while a backup or another restore is open.

**Kubernetes.** The request is picked up by the same worker logic run as a Job that an operator's controller or a CronJob starting every minute launches; it mounts the backup volume and the key secret, and connects with the owner role. The app's Deployment stays running and shows the waiting page; nodes are not stopped, so no Kubernetes permission to scale is needed.

**Not in that step:** restoring a single space or page from a backup; that is a different feature, closer to the trash.

### 3.3 Later, smaller steps

- A "what would be removed" preview for audit retention.
- Log level for an hour, carried to every node through the settings cache.
- The disk thresholds as settings.
- Downloading the monitoring rules and dashboard from the console (the files live in `deploy/`, outside what the binary embeds today).
- The configuration warnings of spec 056 (mail server, identity provider, publish folder) on System health (OPS-101).
- S3 as a backup target, with `minio-go`, in wave 9 of spec 048.

## 4. Open for the product owner

- Should a restore from the console also be possible for a single space, from a backup, without rewinding the rest?
- Is one restore a day the right limit?

## 5. What one machine cannot prove

- A Kubernetes CronJob or Job as the worker.
- Two workers at once against one database beyond the claim test.
- A real mail reaching an administrator (the test checks what is queued).
