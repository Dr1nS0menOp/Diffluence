# 11.7: The console areas

How to read this chapter: see [01-console-runs-the-installation.md](01-console-runs-the-installation.md).

Each area of the roadmap paragraph, with what exists, what is new, its tier (chapter 06) and its proof. Where an area needs a worker, it follows chapter 05; where it only shows, it reads what the server and the worker already report.

## 7.1 Backups: schedule, number kept, target

- **CSL-500** · built · **The schedule**: every 1 to 168 hours, or the worker's default (spec 058). Tier 1 to make it more frequent or keep it; tier 2 to make it less frequent (CSL-462).
- **CSL-501** · built · **The number kept**: 1 to 1000, or the worker's default. Tier 1 to raise it.
- **CSL-502** · later · Keeping by age as well as by count, such as "one a day for 30 days, one a week for a year". Wanted by larger installations; it changes the worker's prune and needs its own tests. Not in spec 075.
- **CSL-503** · new · **The target** is chosen among the targets the worker reports (CSL-410): today one folder, `local`, with its free space; S3 when wave 9 of spec 048 adds it. The console never holds a target's address or credentials; it stores only the target's name. *Why:* CSL-304. *Proof:* a test that an unknown target name is refused; an e2e picture with one and with two targets.
- **CSL-504** · new · **Changing the target** keeps the backups on the old one, and the worker keeps pruning both by the number kept until the new target holds that many. Tier 2. A change that would drop the old target's backups at once is not offered; it is the tier-3 lowering of CSL-505 on the old target. *Edge cases:* the old target is gone (the worker no longer reports it): its backups show as "not reachable" and are not counted. *Proof:* a test of the prune across two targets.
- **CSL-505** · new · **Lowering the number kept is tier 3** (CSL-464): the request names how many backups will be removed and which, the worker shows the code, and only a confirmed task changes the setting. Until then the old number holds. *Proof:* a test that the setting does not change without the code, and that the right backups go once it is entered.
- **CSL-506** · built · **The list of backups**, with size, state, the step that failed, and whether a restore test of it passed. New: the target it is on, and the key fingerprint it was made with, so a backup made before a change of the backup key is marked "needs the previous backup key". *Proof:* an e2e picture.
- **CSL-507** · built · **Health and mail**: a failed backup or test restore shows on System health and is mailed, at most twice a day (spec 058, D6, D7).
- **CSL-508** · built · **Back up now**: within a minute, at most 4 an hour (spec 058).

## 7.2 The restore test

- **CSL-510** · new · **The worker restores a backup into a scratch database**, `diffluence_check`, on the same PostgreSQL, and its files into a temporary folder of its own: the checksums, the key opens it, `pg_restore` into the scratch database, the migrations of this release as the owner, `audit verify` and `key check --deep` against the scratch database and the restored `master.key`, and counts of pages, files and people. Then it empties the scratch database and removes the folder. *Why:* the weekly test of `restore-check.sh` needs a throw-away Compose project, which a worker could only make with the Docker socket, and that socket would hand it the host (CSL-1209). A scratch database needs no new right at run time. *Edge cases:* CSL-912 to CSL-915. *Proof:* a test that restores the worker's own backup into the scratch database and finds the counts; a mutation that skips the audit check fails it.
- **CSL-511** · new · **The scratch database is made once, at install**: `deploy/postgres-init.sh` makes `diffluence_check` owned by the owner role; on an existing installation the worker reports the capability `restore_test` only when the database exists and it may connect to it, and the area says how to make it (one `CREATE DATABASE` as the superuser). *Why:* the owner role must not need `CREATEDB`.
- **CSL-512** · new · **Weekly by default**, at a time the area sets (a day and an hour, default Sunday 03:00 by the database's time zone), and **Test now**, tier 1, at most once a day. *Proof:* a test of the due time across a change of daylight saving time.
- **CSL-513** · new · **Room first**: the worker refuses to start when the free space of the database's disk is less than twice the backup's database part, or of its own temporary folder less than twice the files part, and says so as the test's result. It cannot see the database's disk directly, so it reads `pg_database_size` and the size the server measured (spec 051) and asks the operator to leave room (`backup-restore.md`). *Edge cases:* a database on another machine. *Proof:* a test with a small limit.
- **CSL-514** · new · **The result is a row of `backup_restore_checks`**, as `restore-check.sh` reports today, with how long it took. A failed test is on System health and mailed. *Proof:* the existing health and mail tests.
- **CSL-515** · new · **Load**: `pg_restore` with one job, the worker's process at the lowest CPU and I/O priority it may set, and the daytime hours refused for **Test now** unless the area's setting allows it. Chapter 09 has the numbers.
- **CSL-516** · new · Experimental part `console_restore_test`, off at the start (CSL-1218). `restore-check.sh` stays for installations without a worker.

## 7.3 Restoring over the live installation

- **CSL-520** · built · Unchanged: specs 059, 066 and 070. It shows in the list of the worker's work (CSL-595) beside the new tasks.

## 7.4 The audit chain, the witness and retention

- **CSL-530** · built · **Checking the chain** (Audit, Check). New beside it: the newest checkpoint and its time, and the **signing keys** of `audit_signing_keys`: from which position each signs, its public key's start, and whether its endorsement verifies (spec 070). *Proof:* an e2e picture after a change point.
- **CSL-531** · new · **The worker's checkpoint state**: the last checkpoint it signed and when, from its heartbeat. A worker that holds a backup key but has signed nothing for an hour while events were sealed is a warning on the audit line of System health. *Why:* a stopped signer is otherwise seen only by `audit verify`.
- **CSL-532** · new · **The witness status, reported from outside.** `diffluence audit verify --witness FILE --report` compares as today and then records the result in a new row (`audit_witness_checks`: when, how many lines compared, the first position that did not match, or none). The operator runs it from the log pipeline or a timer next to the log store. The console shows the last result and its age; it never reads the log. *Why:* CSL-1210. *Proof:* a test that a report is written and shown, and a mutation that writes OK on a mismatch fails.
- **CSL-533** · new · **Once a witness was ever reported**, no report for eight days is a warning on the audit line. Before the first report the area explains how to run it, and nothing warns. *Why:* an installation that never set it up should not see a warning it cannot act on from the console.
- **CSL-534** · built · **Retention in days**, with a reason past a year (SEC-831).
- **CSL-535** · new · **Lowering retention** is tier 3 and takes effect **seven days after** the code is entered; until then the area shows "Lowered to 90 days from 21 November" with **Cancel**, and the mail of CSL-454 goes to every system administrator. Raising it is tier 2 and takes effect at once. *Why:* CSL-1211. *Proof:* tests that the prune uses the old value before the date, the new one after, and nothing after a cancel.
- **CSL-536** · new · **The worker prunes once a day**, as `audit prune` does: it verifies the chain, leaves the anchor, removes rows past the retention in force, in batches of 10,000, and records `audit.pruned` with the count and the anchor. The area shows a **preview**: how many rows would go at the next prune, the oldest row that stays, and when it runs. *Proof:* a test of the preview against the prune's own count.
- **CSL-537** · new · The prune never removes a row a witness check has not covered when a witness was ever reported, unless the last report is OK and newer than the row. *Why:* a row pruned before anyone compared it is a row whose tampering nobody can see any more. *Edge cases:* a witness never reported: the rule does not apply, as today. *Proof:* a test.
- **CSL-538** · new · Experimental part `console_audit_prune`. While it is off, `audit prune` stays the only way, as today.

## 7.5 Logging people out

- **CSL-540** · built · One person, or everyone after a fresh login (spec 058, D10).
- **CSL-541** · new · **Before ending everyone's sessions**, the dialog says how many sessions of how many people will end. *Proof:* an e2e test.
- **CSL-542** · new · The area links to the incident steps of `docs/operations/incidents.md` in words ("After a suspected break-in, also: …"), which name the key, the backup key and the tokens. *Why:* logging everyone out is rarely the only step.

## 7.6 Replacing `master.key`, guided

- **CSL-550** · new · **A page of five steps**, add, promote, reseal, check, retire, with the state of each read from the database and from the nodes: which key ids each live node holds (CSL-551), which key seals, how many values and files still use an older key, and when each step ran. Only the next step has a button. *Why:* the five commands of `backup-restore.md` with a restart between each are the slowest and most error-prone operation an operator has.
- **CSL-551** · new · **Every node reports the key ids it holds** in its row of `nodes`, every 15 seconds as it reports its version. *Proof:* a test of the heartbeat.
- **CSL-552** · new · **Add and promote are worker tasks** that write `master.key.next`, and then rename the files, in the key folder, with mode 600, under a lock file, as `keys.AddNext` and the promote do today. **Nodes load the new key files without a restart** when the database's key generation changes (a row the task writes), and report it (CSL-551). The next step waits until every live node reports the new key. Tier 2. *Why:* CSL-1216. *Edge cases:* a node that cannot read the new file reports the error by its id; the page names the node. *Proof:* a test with two nodes, one of them slow to load.
- **CSL-553** · new · **Reseal** is a task with progress, using the existing loop in runs of two minutes, paced so it takes at most 5% of the database's time (chapter 09). Tier 2. *Proof:* a test of the progress and of a reseal stopped and resumed.
- **CSL-554** · new · **Check** is a task, tier 1, with the result of `key check --require-current` and **Check every file** for `--deep`. *Proof:* the existing check's tests through the task.
- **CSL-555** · new · **Retire** is tier 3, offered only when the last check found nothing that needs the old key and was made after the last reseal. The worker makes a backup first, then removes the old key. *Proof:* a test that retire is refused after a new value was sealed with the old key.
- **CSL-556** · new · **Where the key comes from a read-only mount** (a Kubernetes Secret, a read-only bind), the worker does not report `key_files`, and the page shows the same steps as **commands**, with the files to place, and still reads the state from the nodes. *Why:* only the cluster's operator may write a Secret (CSL-320).
- **CSL-557** · new · **With several nodes on their own disks**, the page lists each node with the key ids it holds, so the operator sees which still lacks the file after copying it. *Why:* `backup-restore.md` asks to copy the key to every node; the page shows where it is missing.
- **CSL-558** · new · After promote, the worker makes a backup at once, so a backup with the new key exists before reseal starts. *Why:* `backup-restore.md` asks for it by hand today.
- **CSL-559** · new · Experimental part `console_keys`. The commands stay.

## 7.7 The upgrade page

- **CSL-560** · new · **System, Upgrades** shows, read-only: each live node with its version, role, the highest migration it knows and when it was last seen; each worker with the same and its platform; the version of this release and its build date; the expand migrations that wait, and why (the waiting kinds of the page of spec 054: backup, free space, queue, migrations elsewhere); the last upgrade marker (`system.upgrade_started`) and the backup it names. *Why:* the facts exist in `nodes`, `schema_migrations`, `ops.workers` and the audit log, but nowhere together.
- **CSL-561** · new · **Contract migrations that wait**: their names, since when, which live node does not know them yet, and when the grace ends (`CONTRACT_GRACE`, shown with its source). *Proof:* an e2e picture with one waiting.
- **CSL-562** · new · **How to upgrade on this platform**, as commands, from the worker's platform: `git pull && docker compose up -d --build`, `helm upgrade …`, or the package. *Why:* the console cannot upgrade (CSL-1001), but it can say how.
- **CSL-563** · new · **Run the waiting contract migrations now**, tier 3, offered only when every live node and worker knows them. The worker runs `worker migrate` with the grace set to zero for this run only, and records `system.contract_migrated` as today. The request names the migrations; a migration added by a newer release after the request is not run. *Why:* CSL-1217. *Proof:* tests that it is refused while an older node is live, and that only the named migrations run.
- **CSL-564** · new · Experimental part `console_contract_now`. The display of CSL-560 to CSL-562 is not experimental.

## 7.8 The log level

- **CSL-570** · new · **Raise or lower the log level for a while**: debug, info, warn or error, for 1, 4 or 24 hours (default 1), for every node and the worker, or for one node. A setting `log_override` holds the level, the nodes it applies to and the end. Tier 1. *Why:* CSL-1212.
- **CSL-571** · new · **Each node applies it within five seconds**, as it rereads maintenance mode, through a `slog.LevelVar`, and returns to its configured level at the end without being asked. *Proof:* a test with a fake clock.
- **CSL-572** · new · **Each node reports the level it runs at** in its row of `nodes`, so the area shows where the change took. *Proof:* a test of the heartbeat.
- **CSL-573** · new · **Debug never logs a secret or page text**: the existing rule (`CLAUDE.md`, "Secrets never in logs") holds at every level, and a test runs a request path at debug and fails on a secret or a title in the log, as the trace test does for spans (spec 060). *Why:* a level an administrator can raise must not open a leak. *Proof:* that test.
- **CSL-574** · new · **The lines an operator builds on are written at every level**: the audit checkpoint (ADR-039) and the worker's code line (CSL-430). *Why:* raising the level to error must not hide a code or a witness line. *Proof:* a test at level error.

## 7.9 Maintenance mode and the disk thresholds

- **CSL-580** · built · **Maintenance mode** with a message, tier 2.
- **CSL-581** · new · **The disk thresholds as settings** in a section `health`: warn below 5 to 50 percent free (default 10), refuse files below 1 to 10 percent (default 2), refuse always below warn. Tier 1. *Why:* CSL-1213. *Proof:* table tests of the bounds; the health rules read the setting.
- **CSL-582** · new · **The backup line's age follows the schedule**: it warns when the newest backup is older than the interval plus two hours, and never sooner than 26 hours when no schedule is set. *Why:* a backup every 4 hours that stopped should warn after 6 hours, not 26.
- **CSL-583** · built · The band on every console page while the disk runs out (spec 051).

## 7.10 Monitoring files

- **CSL-585** · new · **Download the monitoring rules** of this release (`diffluence-rules.yml`) from System health, as a file, read from a copy embedded in the binary. *Why:* CSL-1214.
- **CSL-586** · new · **Download the dashboard** (`diffluence-dashboard.json`) the same way.
- **CSL-587** · new · Beside the downloads, whether `/metrics` is on (`METRICS_TOKEN` set or not, never its value) and how a scraper sends the token, from `install.md`. *Proof:* an e2e test that the downloaded file equals `deploy/monitoring/`.

## 7.11 The events transport

- **CSL-590** · new · **System, Events** shows what `events status` prints: the transport and since when, whether NATS is drained, nodes of an older release, and whether the release without NATS may be installed, with the problem in words. Not experimental.
- **CSL-591** · new · **The worker runs `events check` hourly** and the work line of System health shows a breach with the rule it broke (L1 to L3). *Proof:* a test with a gap made by hand.
- **CSL-592** · new · **Switch to PostgreSQL, or back to NATS**, tier 2, with the same refusals as the command (an older node seen in five minutes). *Why:* CSL-1215. *Proof:* the switch's tests through the route.
- **CSL-593** · new · Experimental part `console_events_switch`. When the release without NATS (EV3) lands, the switch and its part go; the area keeps the event log's state.

## 7.12 The worker's work

- **CSL-595** · new · **System, Worker** lists the workers with their platform, version, last seen, capabilities and targets, and the tasks newest first, with kind, state, step, who asked and when, and the result. *Proof:* an e2e picture.
- **CSL-596** · new · **Cancel** on a task that has not started running, tier 1 for a task of tier 1 and 2, tier 2 for a task of tier 3. *Why:* cancelling is always safe before the work starts, and the other administrators must be able to stop a tier-3 request they were mailed about.
