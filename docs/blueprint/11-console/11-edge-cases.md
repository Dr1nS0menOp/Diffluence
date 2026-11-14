# 11.11: Edge cases

How to read this chapter: see [01-console-runs-the-installation.md](01-console-runs-the-installation.md).

## 11.1 The worker

- **CSL-900** · new · **No worker at all** (Helm, LXC and packages until wave CS6, or a Compose installation that removed the backup service): every area that needs one says so and shows the command (CSL-321); the read-only areas work. *Proof:* an e2e test with no worker row.
- **CSL-901** · new · **The worker stops while a task runs**: its claim ends after the kind's limit, another worker or the same one after a restart takes it again, and the task's kind must be safe to run twice. Add and promote check the files' state first; reseal resumes; the prune verifies and resumes; the restore test starts over. *Proof:* a test per kind that kills the task halfway and runs it again.
- **CSL-902** · new · **Two workers against one database** (a second backup service by mistake, two pods during a Kubernetes rollout): claims keep each task to one; both report capabilities; the console shows both and warns. *Why:* the `Recreate` strategy avoids it on Kubernetes, but a mistake elsewhere must not double the work. *Proof:* the claim test with two workers.
- **CSL-903** · new · **A worker of an older release** (Compose with a pulled server image and an old `DIFFLUENCE_BACKUP_IMAGE`): it reports no capability for the new kinds, so they are not offered, and the upgrade page warns (CSL-412). *Proof:* the CI upgrade job.
- **CSL-904** · new · **A worker without the backup key**: no backups, no codes, no tier-3 kinds (CSL-433); its row says the key is missing, as today.
- **CSL-905** · new · **The worker's clock is off**: every time that matters (claims, code expiry, `seen_at`, `not_before`) comes from the database's clock, never the worker's. *Proof:* a test with a worker clock an hour off.

## 11.2 Codes and logs

- **CSL-910** · new · **The worker's log is lost before anyone reads the code** (a pod restarted, a log rotated): the code expires after 30 minutes and the task ends `refused`; asking again makes a new code. A worker that restarts while a code waits writes the line again, with the same code, while it is valid. *Proof:* a test of the rewrite after a restart.
- **CSL-911** · new · **The backup key is replaced between the code and its entry**: the code was made with the old key and no longer matches; the task ends `refused` with the step "the backup key changed", and asking again works. *Proof:* a test.
- **CSL-912** · new · **The restore test while a backup runs**: the backup comes first in the round (CSL-420); the test waits for the next round.
- **CSL-913** · new · **The restore test of a backup made with a previous backup key**: the worker cannot open it, and the result says "made with a previous backup key", not "failed". *Why:* an operator who replaced the key must not read that every old backup is broken.
- **CSL-914** · new · **The scratch database holds a test that died**: the next test empties it first (`DROP SCHEMA … CASCADE` of every schema the dump made, as the owner). The scratch database is never the live one: the worker refuses when the scratch database's name equals the live database's, or when its `instance` row is present before the restore. *Why:* the one mistake that must be impossible. *Proof:* a test with the scratch address pointing at the live database.
- **CSL-915** · new · **The scratch database is on a full disk**: the test fails at its step with "no room", nothing of the live installation is touched, and the room rule of CSL-513 is tightened in the result's text.
- **CSL-916** · new · **A log collector keeps the codes**: a code is useless after its task ended or after 30 minutes, and fits one task (spec 070). Nothing to do.

## 11.3 Restores and tasks

- **CSL-920** · new · **A restore rewinds the database while tasks exist**: `ops` is never restored (spec 059), so tasks keep their state; a task open when a restore is asked blocks the restore (CSL-406), and a restore blocks new tasks.
- **CSL-921** · new · **Settings after a restore**: the console's settings (schedule, retention, thresholds, log override) come back as they were in the backup (spec 065 handles settings that must survive, such as the experimental parts). A lowered retention waiting for its date (CSL-535) is kept in `ops.tasks`, outside the restore, and still applies on its date; the area shows it. *Proof:* a test of a waiting lowering across a restore.
- **CSL-922** · new · **A prune and a witness**: a prune leaves its anchor; the next witness comparison skips lines before it, as today. A prune does not run while the last witness report failed (CSL-537).
- **CSL-923** · new · **Contract migrations now, while a node of the previous release starts**: the request checks every live node at the moment the worker runs it, not at the moment it was asked; a node seen since then that does not know them ends the task `refused`. *Proof:* a test.

## 11.4 Keys

- **CSL-930** · new · **A node that cannot load the new key file** (wrong owner, a node on another disk): it keeps serving with the keys it has, reports the error by its id, and the next step waits (CSL-552). *Proof:* a test.
- **CSL-931** · new · **A node that starts during a key change** loads every key file present (as today) and reports its key ids at once.
- **CSL-932** · new · **A backup made between add and retire** holds `master.key.next` and `master.key.previous` as today; the list of backups marks which key set a backup holds. A restore of it brings the installation back into the middle of the change, and the page reads the state from the restored database and the files, and offers the next step from there. *Proof:* a step of the restore drill, as it does today halfway through a key change.
- **CSL-933** · new · **Retire while a value was just sealed with the old key** by a node that had not loaded the new one: retire is refused because its check finds it (CSL-555). *Proof:* a test.

## 11.5 Settings and nodes

- **CSL-940** · new · **The log level for one node that stops** before the end: the override ends by its own time; a node that starts later during the override applies it if it is named or if the override is for every node.
- **CSL-941** · new · **Thresholds that cross**: refuse at or above warn is refused by the setting's validation (CSL-581).
- **CSL-942** · new · **Maintenance mode while a tier-3 task waits for its code**: entering the code is a write; it stays open during maintenance, like the maintenance route itself. *Why:* an incident under maintenance must not block its own repair.
- **CSL-943** · new · **The waiting page of an upgrade**: the console is not reachable then; everything stays a command, as the page already says. The worker keeps running its tasks. *Why:* CSL-300.
- **CSL-944** · new · **Several nodes with different configuration**: CSL-603 marks it; the console still works per node.
- **CSL-945** · new · **An administrator who loses system administration** while their tier-3 request waits: the request stays, and only a system administrator may enter the code (the route checks at entry). *Proof:* a test.
- **CSL-946** · new · **The person who asked is disabled** while their task waits: the task is cancelled and recorded as `task.cancelled` with the reason. *Why:* a request of someone no longer trusted must not run on.
- **CSL-947** · new · **A managed database** (no `CREATE DATABASE` for anyone in the installation): the restore test is not offered, and the area says why. Migrations, prune and the other kinds work as long as the owner role exists.
- **CSL-948** · new · **PgBouncer in transaction mode**: tasks hold no session state between statements; the prune and the reseal take only transaction-level locks, as the migrations' direct connection rule asks (`install.md`).
