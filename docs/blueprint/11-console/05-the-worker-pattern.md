# 11.5: The worker pattern

How to read this chapter: see [01-console-runs-the-installation.md](01-console-runs-the-installation.md).

The console controls and a worker does the work. Spec 058 built this for backups, spec 059 for a restore, and spec 070 made the worker the only holder of the owner of the tables and the backup key, and made a restore wait for a code from the worker's own log. This chapter makes that pattern general, so every new kind of work in chapter 07 uses it the same way.

## 5.1 What goes through a worker, and what does not

- **CSL-400** · new · **A worker carries out what needs a secret or a right the server does not hold**: the owner of the tables (migrations, prune), the backup key (backups, restores, the restore test, codes, checkpoints), or write access to files the server must not change (the key files). *Why:* ADR-044: the server holds only the application role.
- **CSL-401** · new · **The server changes directly what is plain database work with the application role**: settings (schedule, retention value, thresholds, the log level), ending sessions, the events switch. Each is one transaction with its audit event (`CLAUDE.md`, "Every state change"). *Why:* a round trip through a worker would only add a minute of delay and a dependency on a worker being alive, for work the server already does for itself.

## 5.2 Requests in the database

- **CSL-405** · new · **One table for the new kinds of work, `ops.tasks`**, in the schema `ops`, which a backup holds without rows and a restore never restores (spec 059), so a request can never come back from a backup:

```sql
CREATE TABLE ops.tasks (
  id            uuid PRIMARY KEY,                 -- UUIDv7
  kind          text NOT NULL,                    -- restore_test, audit_prune, contract_now, key_add, ...
  params        jsonb NOT NULL DEFAULT '{}',      -- small, validated per kind in Go
  tier          smallint NOT NULL,                -- 1, 2 or 3 (chapter 06)
  state         text NOT NULL,                    -- requested, awaiting_code, confirmed, running, done, failed, refused, cancelled
  requested_by  uuid NOT NULL, requested_at timestamptz NOT NULL DEFAULT now(),
  not_before    timestamptz,                      -- a delay, such as lowered audit retention
  code_entered  text, wrong_codes smallint NOT NULL DEFAULT 0, code_shown_at timestamptz,
  seal          bytea,                            -- MAC over id, kind and params, by the worker
  claimed_by    uuid, claimed_at timestamptz,
  step          text, progress jsonb, result jsonb,
  finished_at   timestamptz
);
CREATE UNIQUE INDEX ops_tasks_one_open ON ops.tasks (kind)
  WHERE state IN ('requested','awaiting_code','confirmed','running');
```

  The backup tables of migration 0048 and `ops.restores` stay as they are (CSL-1202). *Proof:* the migration test; a test that a backup's dump holds `ops.tasks` without rows.
- **CSL-406** · new · **One open task per kind**, held by the partial unique index, and **no task while a restore is open**: a restore excludes everything, and a task excludes a restore. *Why:* a prune during a restore, or a reseal during a key retire, would race. *Proof:* a test per pair.
- **CSL-407** · new · **A claim is taken in the transaction that hands it out**, with `FOR UPDATE SKIP LOCKED`, and a claim older than its kind's limit (6 hours for a restore test, 24 for a reseal, 1 for the rest) may be taken again, as spec 058 D4 does for backups. *Proof:* the claim test of two workers.
- **CSL-408** · new · **Limits per kind** against wearing the server down: the restore test once a day, a prune once a day, a reseal once at a time, a key step once a minute, contract migrations once an hour. A refusal says when the next is possible. *Proof:* a test per limit.
- **CSL-409** · new · **The server validates `params` per kind before it writes the row**, and the worker validates them again before it acts. Unknown kinds are refused by both. *Why:* input is validated at the boundary, and the worker's boundary is the table.

## 5.3 The worker and its capabilities

- **CSL-410** · new · **The worker says what it can do.** Each round it upserts a row of `ops.workers`: its id, platform (`compose`, `kubernetes`, `systemd`), version, highest migration it knows, `seen_at` from the database clock, the backup key's fingerprint (spec 058, D8), the **capabilities** it has (`backup`, `restore`, `restore_test`, `audit_prune`, `contract_now`, `key_files`, …) and the **targets** it can write to (name, kind, free space). The console offers an action only when a worker seen in the last two minutes has its capability. *Why:* platforms differ, and an older worker cannot do what a newer console asks (CSL-1206). *Proof:* a test per capability that the route answers `409 no_worker` without it; an e2e test that the button is absent.
- **CSL-411** · new · The table `backup_worker` of migration 0048 stays, written as before for the release before this one; the new row is in addition, so a console of either release sees the worker.
- **CSL-412** · new · **The worker's version shows on the upgrade page beside the nodes**, and a worker of another release than the newest node is a warning there. *Why:* in Compose the backup image is built apart (`DIFFLUENCE_BACKUP_IMAGE`), and an old worker cannot run the new migrations (spec 070).
- **CSL-413** · new · **The worker reports the settings it started with**, as `config.Inspect` gives them, secrets as `(set)`, into its row (chapter 08). *Why:* the console's Configuration page shows only the server's process today, and the worker holds the settings that matter most for recovery.

## 5.4 The loop

- **CSL-420** · new · **`diffluence worker run`** is the worker's loop in Go, replacing the loop of `deploy/backup/backup-service.sh` (CSL-1203). Each round, at most a minute apart, and at once after a task ends:
  1. `worker migrate` (as today);
  2. `worker checkpoint` (as today);
  3. the heartbeat with capabilities (CSL-410);
  4. `backup next` as today: a backup, a restore, or a code to show;
  5. the next task of `ops.tasks`: show its code, or run it.
  A backup always comes before a task in the same round, and a task never starts while the backup it may need is due. `pg_dump`, `pg_restore` and `gpg` stay outside tools called by the loop; the shell script keeps only `once` and `way-back`, as thin calls. *Why:* the same loop on every platform, tested in Go. *Proof:* the existing backup and restore tests pass against the Go loop; the restore drill passes on its own images.
- **CSL-421** · new · **Kubernetes:** the chart gains a Deployment `diffluence-worker` with one replica and the `Recreate` strategy, from the backup image, with the owner's address, the backup key and the backup volume; the app's pods lose the owner's address. The upgrade order is the worker first: a pre-upgrade hook is not needed, because the app's pods wait on their page for migrations (spec 070). *Why:* CSL-1207. *Proof:* `helm template` tests; a k3s run when the cluster of ADR-040 exists.
- **CSL-422** · new · **LXC and packages:** a second unit `diffluence-worker.service` runs `diffluence worker run` as the user `diffluence-worker`, which owns the backup folder and reads `/etc/diffluence/worker.env` (the owner's address, the backup key's path); the server's unit keeps only the application role. Both units share the group that may read `master.key`. *Why:* CSL-1208. *Proof:* a test of the install script in a container with systemd.
- **CSL-423** · new · **A task runs with a context that ends with its limit** (CSL-407), and the worker records `failed` with the step that was running when it ended. *Proof:* a test with a task that does not finish.

## 5.5 Confirmation with a code from the worker's log

- **CSL-430** · new · **The code of spec 070 serves every third-tier task**: an HMAC over the task's id, kind and the hash of its `params`, with a key derived from the backup key under its own label (`diffluence task code v1`), ten digits shown as `12345-67890`, valid 30 minutes, three tries. The worker writes one line to its own log:
  `To <what, in words>, enter this confirmation code in the console: 12345-67890 (valid 30 minutes)`
  and sets `code_shown_at`. *Why:* one mechanism, already reviewed, instead of five. *Proof:* the tests of spec 070, run per kind.
- **CSL-431** · new · **The worker seals a confirmed task** with a MAC over id, kind and params under another derived label, and checks the seal before every step. A task whose params changed after it was confirmed ends `refused`, recorded. *Why:* the application role may write `ops.tasks`, but nothing it writes makes the worker act without the code (spec 070, 3). *Proof:* a test that changes params after the code.
- **CSL-432** · new · **The line names what will happen in words**, such as "retire the old master.key (id 3)" or "run 2 contract migrations now", never a path, address or secret. *Why:* the person reading the log must know what they confirm; the log may go to a collector.
- **CSL-433** · new · **Without a backup key the worker makes no codes**, refuses every third-tier task at once and says why in the task's step, as it does for a restore. Its capabilities then leave the third-tier kinds out, so the console does not offer them. *Proof:* a test.
- **CSL-434** · new · **The command road stays as strong:** inside the worker, `diffluence task request <kind> …` holds the key and fills in the code, as `backup restore-request` does. *Why:* an operator on the machine needs no console, and a console that is down must not block an incident.

## 5.6 Reporting back

- **CSL-440** · new · **Every change of a task's state is an audit event**, written by whoever changes it in the same transaction: `task.requested` and `task.cancelled` by the server, `task.code_entered` by the server without the code, `task.confirmed`, `task.code_wrong`, `task.refused`, `task.started`, `task.done` and `task.failed` by the worker. The kind is in the details; params only where they hold no secret, and they never do by design (CSL-409). *Proof:* a test per transition; the audit chain verifies afterwards.
- **CSL-441** · new · **Progress** is a small JSON object the worker updates at most every five seconds (`{"done": 1200, "of": 5000}`), which the console polls while the area is open. *Why:* a reseal or a restore test can take long, and the page must say how far it is.
- **CSL-442** · new · **Failures are mailed** to system administrators as backup failures are (spec 058, D6: every 5 minutes by a server node, at most once per kind in 12 hours). A third-tier request is mailed to every system administrator **when it is made**, not only when it fails. *Why:* the other administrators are the check against one stolen session. *Proof:* a test of what is queued.
- **CSL-443** · new · **A task that failed shows its step and the way back** in words on its area, as the restore does today.
- **CSL-444** · new · **Finished tasks are kept 400 days**, as backup runs are (spec 058, D12), by the daily cleaner.

## 5.7 Across releases

- **CSL-447** · new · **A worker of the release before ignores `ops.tasks`**: it has no such table in its code, and its capabilities (none in the new table) leave every new kind unoffered. **A newer worker against an older server** offers capabilities the server does not know, which the server ignores. *Proof:* the CI upgrade job runs both orders.
- **CSL-448** · new · **The migration is expand only**: `ops.tasks` and `ops.workers` are new tables; nothing changes in the backup tables. The application role gets `SELECT` and `INSERT` on `ops.tasks`, `UPDATE` only of the columns `code_entered` and `state`, and `SELECT` on `ops.workers`; a trigger lets the application role insert a task only in state `requested`, and move one only to `cancelled`, and only from a state before `running`. The seal is a MAC, so a row written by hand cannot carry a valid one either way. *Proof:* a test of the grants and the trigger, as the application role.
