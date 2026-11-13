# Spec 059: restoring over the live installation from the console

Decided by the product owner on 2026-11-11: a full restore over the live installation must be possible from the admin screen. Spec 058, section 3.2, planned it; this spec is the detailed plan, and wave RS builds it.

## 1. What it does

An administrator chooses a complete backup on Administration, System, Backups, reads what is lost, types the installation's name, gives a second factor, and presses **Restore over this installation**. Then:

1. The console writes a restore (`ops.restores`, state `requested`) and an audit entry `backup.restore_requested`.
2. The backup worker picks it up within a minute (`diffluence backup next` answers `restore <id> <backup>`), and checks the backup while the nodes still serve (`preparing`): every part against `SHA256SUMS`, the key opens `master.key.gpg`, and the newest migration in the dump is one this release knows.
3. The worker stops every node (`stopping`). Each node looks every 5 seconds, stops serving, and shows a waiting page. The worker waits until no node has said it is starting or serving for 20 seconds.
4. The worker makes a **safety backup** of the installation as it is (`backing_up`), with the same steps as every backup.
5. The worker restores (`restoring`, from here `changed` is true):
   - it ends every other session on the database;
   - it drops the schema `public`, makes it again and fills it from the dump, all in one transaction;
   - it empties the data folder and fills it from the backup, with its keys.
6. The worker checks (`checking`):
   - it brings the database up to this release, as the owner, and gives the application role its rights;
   - it writes the restore into the restored audit log, sealed;
   - it notes the safety backup for the checks before the next upgrade;
   - it checks the audit chain and that every secret and file opens with the restored keys.
7. Only then is the restore `done`. Every node leaves the waiting page within 5 seconds and starts as after an upgrade.

## 2. Decisions

| # | Decision | Reason |
|---|---|---|
| D1 | What steers a restore lives in a schema of its own, `ops`, with `ops.restores` and `ops.nodes` (migration 0049). The worker leaves `ops` out of the restore, and backups hold its tables without rows (`pg_dump --exclude-table-data='ops.*'`). | A restore rewinds every table in `public`. The state of the restore, and what each node does, must survive it, or the nodes could not be told the restore is over, nor that it failed. No foreign key reaches into `public`. |
| D2 | Migration 0049 may run again (`IF NOT EXISTS` throughout). | A restore of a backup from before 0049 brings back a `schema_migrations` without it, while `ops` survived. |
| D3 | The second factor is a code from the authenticator app or a recovery code, given with the request, or a passkey confirmed in the same session within 10 minutes (`sessions.reauth_method`). The password alone is not enough; the route is also sensitive, so a fresh login is asked first. | Spec 058, 3.2. A stolen session plus a guessed password must not be able to rewind the installation. An administrator who logs in only through the identity provider adds a passkey to restore. |
| D4 | The name typed is checked before the second factor. | A typing error costs no code. |
| D5 | At most one restore a day, none while another restore or a "back up now" is open (`restores_one_open`, a unique index). The limit is a fixed constant, not a console setting. | A restore stops the whole installation. A setting would be one more thing that a stolen session could raise; one a day is enough for its purpose, and an operator with the machine can still restore with `scripts/restore.sh`. |
| D6 | Nodes wait from `stopping` to `done`. A restore asked for or being checked (`requested`, `preparing`) leaves them serving, so a worker that never comes cannot take the site down. A restore that stood still for 6 hours before it changed anything is not waited for, and the next `backup next` ends it as refused. | The site is never held hostage by a worker that died before it changed anything. Once it changed something, only the way back may lift the wait, because the database may be half restored. |
| D7 | The worker restores only when no node has said it is starting or serving for 20 seconds; nodes say so every 5 seconds, also while they migrate or start (`starting`). A node that starts while a restore runs never serves: it looks before its first request, and before it opens anything. | A node that keeps serving could write into tables being made again. The worker refuses to change anything then. |
| D8 | Before it restores, the worker ends the sessions of every other database role. A role that may not do so only logs it. | A node that missed the waiting page, such as one paused for a long time, loses its connections, so it cannot write. |
| D9 | The database is restored in one transaction: `DROP SCHEMA public CASCADE`, `CREATE SCHEMA public`, then the dump without its entry for `public` and without `ops`, through `psql --single-transaction`. | A failure leaves the database exactly as it was. A table a newer release added cannot linger and break the migrations of the older backup. |
| D10 | The data folder is emptied only when it holds a `master.key`, and only at an absolute path. | A wrong `DIFFLUENCE_DATA_DIR` must never empty another folder. |
| D11 | A backup made by a newer release is refused while the nodes still serve. | This release cannot know the shape of a newer database; restoring it would break the installation. |
| D12 | The worker writes the request and the result again into the restored audit log, sealed, as the system with the name of who asked. | The restore rewound the audit log with everything else, so without this it would leave no trace. The witness log outside the database (ADR-039) keeps the checkpoints from before. |
| D13 | The way back is `docker compose run --rm backup way-back`, which restores the safety backup of a failed restore with every node still waiting. The waiting page names the command. | When a restore stopped halfway, the console is down too, because every node waits. The way back must work without it. |
| D14 | The public waiting page names the step, never the backup, a date or a person. | Anyone can open it (spec 054). The details are in the log and in `ops.restores`. |
| D15 | An operator with the machine can ask for a restore with `diffluence backup restore-request --name … --confirm …`, without a second factor; the audit log says it came from the command line. | Shell access to the installation is more than the console's second factor. The drill uses this route. |
| D16 | Restoring a single space or page from a backup is not part of this. It goes on the roadmap as a later item. | It is a different feature, closer to the trash than to a restore. Open question 1 of spec 058, decided for now as later. |

## 3. Failure paths and their proof

| Failure | What happens | Proof |
|---|---|---|
| The name typed is wrong | 400, nothing recorded, no code used | `TestARestoreAsksForTheNameOfTheInstallation`; the e2e test of the form; the drill (command line) |
| No fresh second factor | 403 `second_factor_required`; with no authenticator app, it says to set one up | `TestARestoreAsksForASecondFactor`; e2e |
| A backup that failed or is unknown, or a name outside the folder | 400 | `TestOnlyAnAdministratorRestoresACompleteBackup`, `TestABackupNameStaysInTheBackupFolder` |
| A second request while one is open; a "back up now" is open | 409 `restore_open`, `backup_requested` | `TestOneRestoreAtATimeAndOneADay` |
| More than one a day | 409 `restore_limit` | same |
| The backup is not whole, or does not open | Refused before any node stops | `TestABackupThatIsNotWholeIsRefused`; the drill tampers with a backup and checks the nodes kept serving |
| A backup made by a newer release | Refused before any node stops | `TestABackupOfANewerReleaseIsRefused` |
| A node keeps serving | Refused, nothing changed | `TestANodeThatKeepsServingStopsTheRestore` |
| The safety backup fails | Refused, nothing changed, nodes serve again | `TestASafetyBackupThatFailsChangesNothing` |
| The restore fails halfway | Failed and open; every node waits with the way back; the way back restores the safety backup and ends the wait | `TestARestoreThatFailsHalfwayKeepsTheNodesWaitingUntilTheWayBack` |
| The checks after the restore fail | Failed and open, nodes wait | `TestARestoreWhoseChecksFailKeepsTheNodesWaiting` |
| A node serves when a restore begins, or starts during one | It stops within 5 seconds; a starting one never serves | `TestAServingNodeStopsWhenARestoreBegins`, `TestServeRefusesToServeDuringARestore` |
| The worker died before it changed anything | After 6 hours, the nodes serve again and the restore ends | `TestARestoreThatStoodStillLongBeforeAnyChangeIsNotWaitedFor` |

The whole path runs for real in the restore drill (`scripts/restore-drill.sh`, step 6), in a throw-away Compose project with its own images. A backup by the service, a page made after it, a restore from the command line, the nodes on the waiting page, the restored page gone, the safety backup present, and the audit chain whole.

## 4. Kubernetes

The worker logic is the `diffluence` binary and the backup script, so a Job runs it:
- **How it starts:** a CronJob every minute, or an operator's controller, that runs `backup next` and acts on the answer.
- **What it needs:** the backup volume, the key secret, the data volume (read and write), and the owner's database address.
- **The app:** the Deployment stays as it is and shows the waiting page. Nodes are not stopped or scaled, so the Job needs no Kubernetes permission beyond its own Pod.

## 5. Load budget (ARC-238)

| Who | What |
|---|---|
| Every node, every 5 seconds | One insert-or-update on `ops.nodes`, one indexed read on `ops.restores`; both tables hold a few rows |
| The worker, every minute | One update that finds no request |
| The backup page | One more read of `ops.restores`, when polled |

## 6. What one machine cannot prove

- A Kubernetes Job as the worker.
- Several app nodes on several machines stopping together, beyond the test with one node and a stubborn node row.
- A paused node that wakes up during the restore: it is covered by D8, but not tried for real.
- A restore of a large installation: how long the nodes wait (wave 7 on the server).

## 7. Open for the product owner

- Whether one restore a day is right (spec 058, open question 2). Decided on the product owner's behalf on 2026-11-12, may be overruled: it stays a constant of one a day (D5), because a restore stops the whole installation and a raisable limit is one more thing a stolen session could change; an operator with the machine can always restore with `scripts/restore.sh`.
- Restoring one space from a backup (D16). Decided on the product owner's behalf on 2026-11-12, may be overruled: later, on the roadmap.
