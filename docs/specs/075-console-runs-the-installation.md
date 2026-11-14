# Spec 075: The console runs the installation

Status: study and plan, 2026-11-14. Nothing is built yet.

Study: [blueprint 11](../blueprint/11-console/01-console-runs-the-installation.md), chapters 02 to 12, lines CSL-001 to CSL-1220. Basis: the roadmap item "Running the installation from the admin screen" (`08-roadmap.md`, "Wanted, later"), specs 058, 059, 068, 069 and 070, ADR-040 to ADR-044. The study sorts 132 commands, settings, fixed values and scripts: 25 change in the console (13 built, 12 new), 69 are shown only, and 38 stay commands, each with its reason (chapter 03).

Decisions made on the product owner's behalf on 2026-11-14 are CSL-1201 to CSL-1220 in blueprint 11.1, section 1.3, and the table at the end of this spec. Each may be overruled before the wave that uses it.

## Core idea

The console controls and a worker does the work, as spec 058 began. Every new kind of work is a row of one table, `ops.tasks`, that the server writes and a worker carries out (CSL-405). The worker says what it can do on this platform (CSL-410), so the console offers nothing it cannot. Every action has one of three tiers of guard (CSL-450 to CSL-452); the third, for whatever removes history or a way back, waits for a code that only the worker can make and writes to its own log, as the restore of spec 070 does (CSL-430). What the server needs to start is shown with its source and never changed (chapter 08).

## When

The feature freeze of ADR-043 holds: spec 075 is built after 1.0, except wave CS1, which only shows what exists and adds two bounded settings (CSL-1219). Every part that changes the installation through a new kind of work is an experimental part that starts off (CSL-1218):

| Part | Wave | What it turns on |
|---|---|---|
| `console_restore_test` | CS3 | The restore test into a scratch database, and Test now |
| `console_audit_prune` | CS4 | Pruning by the worker, and lowering retention in tier 3 |
| `console_contract_now` | CS4 | Running waiting contract migrations before the grace ends |
| `console_events_switch` | CS4 | The events switch in the console |
| `console_keys` | CS5 | The guided replacement of `master.key` |

Not experimental: everything of CS1, the worker in Go and its tasks page (CS2), the target and the tiers on the backup settings (CS3), and the workers of CS6. They show, or they harden what exists.

## Rules for every wave

- **Tests first.** Start from failing tests, failure paths before the happy path; chapter 11's edge cases first where they apply.
- **Mutations.** Add a mutation for every new check: each tier, each refusal of a task, each seal and code check, each bound of a setting.
- **Routes.** Every new route through `httpapi.Router` with an explicit rule: `SessionOnly` for tier 1, `Sensitive(system administrator)` for tier 2 and 3; the route walker lists them.
- **Audit.** An audit event for every state change, in the same transaction (CSL-440).
- **Load.** Measure what chapter 09 asks, on wave PF's real-data dump.
- **Design.** A picture in `web/e2e/0-design.spec.ts` for every new section, looked at in light, dark, narrow and right-to-left.
- **Words.** Through `en.json`; one word per concept (CSL-830).
- **Docs.** `backup-restore.md`, `upgrade.md`, `install.md`, `monitoring.md` and the threat model follow each wave.

## CS1: show what exists, and two small settings

**Scope:**
- the configuration page shows "changes at the next restart" (CSL-601), the OpenTelemetry variables (CSL-604), the reasons of class S (CSL-607) and the last configuration warnings (CSL-608);
- the upgrade page, read-only: nodes, migrations, the marker, how to upgrade (CSL-560 to CSL-562);
- the events area, read-only (CSL-590);
- the audit area: newest checkpoint and the signing keys (CSL-530), and the witness report with `audit verify --witness --report` (CSL-532, CSL-533);
- downloads of the monitoring rules and dashboard (CSL-585 to CSL-587);
- the log level for a while (CSL-570 to CSL-574);
- the disk thresholds as settings, and the backup line following the schedule (CSL-581, CSL-582);
- the count before logging everyone out, and the incident steps (CSL-541, CSL-542).

**Data:** migration `00xx_witness_checks.sql` (expand): `audit_witness_checks`; the application role may insert and read. The settings `log_override` and section `health` are keys of `settings`, no migration. `nodes` gains `log_level` and `key_ids` in the same migration (used from CS5).

**Tests:**
- the upgrade page's facts against fixtures of each waiting kind, and a contract migration waiting;
- the configuration page with a variable changed after the start;
- the witness report written by the command and shown; no report, an old report, a failed report;
- the downloaded files equal `deploy/monitoring/` (the embedded copy's test);
- the log override applied within five seconds, ended by its time on a fake clock, per node and for all; the checkpoint line and the code line written at level error; a request path at debug with no secret or title in the log;
- the thresholds' bounds as a table; uploads refused at the new refuse level; the backup line at interval plus two hours;
- the route walker: every new route has its rule, a token is refused, a member gets 404 or 403.

**Mutations:** the witness OK on a mismatch; the override never ending; the refuse threshold above warn accepted; the debug guard.

**Load:** CSL-701 to CSL-705, CSL-709 for the hourly events check, which arrives with the worker in CS2.

## CS2: the worker in Go and its tasks

**Scope:**
- `diffluence worker run` (CSL-420), the shell script reduced to `once` and `way-back`;
- `ops.tasks` and `ops.workers` (CSL-405 to CSL-413), claims, limits and validation;
- the tiers in one table (CSL-450 to CSL-455), and their mails;
- the code and seal for every tier-3 task (CSL-430 to CSL-434), and `diffluence task request` in the worker;
- reporting back, progress, the cleaner (CSL-440 to CSL-444);
- the Worker section (CSL-595, CSL-596) and the `WorkerCode` component (CSL-806);
- the configuration page's tab per process (CSL-602, CSL-603, CSL-605, CSL-606);
- the hourly `events check` by the worker (CSL-591).

**Data:** migration `00xx_worker_tasks.sql` (expand): `ops.tasks`, `ops.workers`, the column grants and the trigger of CSL-448.

**Tests:**
- the existing tests of backups, restores and codes pass against the Go loop; the restore drill passes on its own images;
- the trigger: as the application role, a task inserted in another state than `requested`, a move to anything but `cancelled`, an update of the seal: each refused;
- a claim by two workers; a claim taken again after its limit; one open task per kind; no task during a restore and no restore during a task;
- a task of an unknown kind refused by the route and by the worker;
- a tier-3 task without a code, with a wrong code three times, after 30 minutes, with changed params after the code, after the backup key changed: each refused and recorded;
- without a backup key: no tier-3 capability, and every tier-3 task refused at once;
- capabilities: the route answers `409 no_worker` without the capability; an older worker offers nothing new;
- a task killed halfway and run again, per kind as each wave adds kinds;
- the worker's clock an hour off;
- the mails: tier 2 and 3 to every system administrator at the request.

**Mutations:** the claim's `SKIP LOCKED` and age; the one-open index; the trigger's states; each tier in the table; the code comparison; the seal check before each step; the capability gate.

**Load:** CSL-700, CSL-709. The CI upgrade job runs an older worker against this server, and this worker against an older server.

## CS3: backups, their target, and the restore test

**Scope:**
- the target among reported targets, and changing it (CSL-503, CSL-504);
- tier 2 for a less frequent schedule, tier 3 for fewer backups kept (CSL-462, CSL-505);
- the list's target and key marks (CSL-506);
- the restore test into the scratch database: the scratch database at install, the capability, the schedule, Test now, room first, the result, priority (CSL-510 to CSL-516);
- the screens of CSL-815.

**Data:** migration `00xx_restore_test.sql` (expand): the test's settings key needs none; `backup_runs` gains `target` and `key_fingerprint`. `deploy/postgres-init.sh` makes `diffluence_check`.

**Tests:**
- the prune across two targets, and a target that disappears;
- lowering the number kept: nothing removed without the code, exactly the named backups removed with it;
- the restore test of the worker's own backup finds the counts; a damaged backup, one made with a previous backup key, a full disk, a test that died halfway; the scratch address pointing at the live database is refused (CSL-914);
- the due time across a change of daylight saving time; Test now once a day;
- the experimental part off: the routes answer `404 experimental_off` and the section is absent.

**Mutations:** the check that the scratch database is not the live one; skipping the audit check in the test; the room rule; the tier of lowering keep.

**Load:** CSL-706 measured on the real-data dump with the reading load; the result in `limits.md`.

## CS4: audit retention, contract migrations, and the events switch

**Scope:**
- retention: raising in tier 2, lowering in tier 3 with seven days' wait and Cancel (CSL-535);
- the prune by the worker, its preview and the witness rule (CSL-536, CSL-537);
- contract migrations now (CSL-563);
- the events switch (CSL-592);
- the experimental parts `console_audit_prune`, `console_contract_now`, `console_events_switch`.

**Data:** no new table: the lowering waits as a task with `not_before`. If the audit log lacks an index on its time, migration `00xx_audit_time_index.sql` adds one, built concurrently.

**Tests:**
- the prune uses the old retention before the date and the new after; nothing after a cancel; a waiting lowering across a restore (CSL-921);
- the preview equals the prune's count;
- the prune refused while the last witness report failed, and rows not covered by a witness kept (CSL-537);
- contract migrations now refused while an older node or worker lives, while a node of the previous release starts during the request (CSL-923), and only the named migrations run;
- the switch's refusals through the route, and switching back;
- each part off: `404 experimental_off` and the sections absent; the commands still work.

**Mutations:** the seven-day wait; the witness rule; the "every node knows them" check; the named-migrations filter.

**Load:** CSL-704 and CSL-708 on a generated audit log of 10 million rows.

## CS5: replacing `master.key`, guided

**Scope:**
- the page of five steps (CSL-550), nodes reporting key ids (CSL-551);
- add and promote by the worker, nodes loading new key files without a restart (CSL-552);
- reseal and check as tasks (CSL-553, CSL-554), retire in tier 3 (CSL-555), a backup after promote (CSL-558);
- read-only mounts and nodes on their own disks shown with commands (CSL-556, CSL-557);
- the experimental part `console_keys`.

**Tests:**
- two nodes, one slow to load the new key: the next step waits and names it; a node that cannot read the file;
- a node that starts during the change;
- retire refused after a value was sealed with the old key, and refused before a check newer than the last reseal;
- a reseal stopped and resumed;
- a backup made between add and retire restored by the drill, and the page offering the next step from there (CSL-932);
- a read-only key folder: no capability, and the commands shown;
- a request never reads the key folder (CSL-710).

**Mutations:** the wait for every node; the retire refusal; the key reload's generation check.

**Load:** CSL-707 measured on the dump with its files.

## CS6: a worker on Kubernetes and on LXC

**Scope:**
- the chart's `diffluence-worker` Deployment, the app's pods without the owner's address (CSL-421);
- `diffluence-worker.service`, its user and `worker.env` for the LXC script and the packages; the server's unit with only the application role (CSL-422);
- the platform's commands in every "no worker" sentence and code line (CSL-320, CSL-321);
- the threat model's "Accepted for now" line on Helm, the LXC script and the packages removed; `install.md` and `upgrade.md` with the move for existing installations.

**Tests:**
- `helm template` tests of both layouts; the chart's upgrade from the previous layout keeps a working installation;
- the install script and the packages in a container with systemd: the worker migrates, backs up, signs and shows a code in the journal;
- an existing LXC installation upgraded by the package keeps working with the old single unit until the operator moves (as spec 070's old `compose.yaml` does);
- on a k3s cluster when the cluster of ADR-040 exists; until then the chart is checked by template only, and `limits.md` says so.

**Mutations:** the server's unit carrying the owner's address fails a test.

## Load budget (ARC-238)

CSL-700 to CSL-712. In short:
- **Steady:** two statements a minute for the worker's round, two fields in the existing node heartbeat, one key in the settings read every five seconds. Nothing on a request path.
- **Pages while open:** each under 5 ms on the dump, polled every 15 seconds.
- **Heavy tasks, bounded and measured:** the restore test within 15 minutes on the dump with reads' p95 within 25%; a reseal at most 5% of the database's time; a prune at most 1% and no lock longer than one batch of 10,000 rows.
- Saving, reading and search do not change.

## What these waves do not cover

Chapter 12 in full. In short: no start, stop or upgrade of containers or packages; no secrets in the database; nothing that the server needs to start becomes changeable; no file on the host is written; no safety rule is waived; the witness is not compared in the console; backups are not downloaded or uploaded through the browser; no single-space restore; no management of PostgreSQL itself; no shell, SQL or script typed in the console; S3 waits for wave 9 of spec 048.

## Decisions

Each decided on the product owner's behalf on 2026-11-14; each may be overruled. The lines are explained in blueprint 11.1, section 1.3.

| | Line | Decision |
|---|---|---|
| D1 | CSL-1201 | Requirement lines use the prefix CSL |
| D2 | CSL-1202 | One table `ops.tasks` for every new kind of work; the backup tables and `ops.restores` stay |
| D3 | CSL-1203 | The worker's loop in Go, `diffluence worker run`; `pg_dump`, `pg_restore` and `gpg` stay outside tools |
| D4 | CSL-1204 | Three tiers: administrator; system administrator with a fresh login; plus a second factor and the code from the worker's log |
| D5 | CSL-1205 | Whatever removes history or a way back is tier 3: fewer backups kept, a target that drops backups, lower audit retention, contract migrations now, retiring a key, a restore |
| D6 | CSL-1206 | The worker reports its capabilities and targets; the console offers only those |
| D7 | CSL-1207 | Kubernetes: a Deployment of one worker pod, `Recreate`, not a CronJob |
| D8 | CSL-1208 | LXC and packages: a second unit, `diffluence-worker.service`, under its own user |
| D9 | CSL-1209 | The restore test uses a scratch database made at install, never a Compose project and never the Docker socket |
| D10 | CSL-1210 | The witness is compared outside, by command, which reports its result to the console |
| D11 | CSL-1211 | The worker prunes daily; lowering retention waits seven days after the code, cancellable |
| D12 | CSL-1212 | The log level changes for at most 24 hours, one by default, and returns by itself |
| D13 | CSL-1213 | Disk thresholds: warn 5 to 50 percent, refuse 1 to 10, refuse always below warn |
| D14 | CSL-1214 | The monitoring files are embedded from `deploy/monitoring/` with a test that they are equal |
| D15 | CSL-1215 | The events switch is in the console, tier 2, while the release has both transports |
| D16 | CSL-1216 | The worker writes the key files; nodes load a new key without a restart |
| D17 | CSL-1217 | Contract migrations before the grace ends: tier 3, only when every live node and worker knows them |
| D18 | CSL-1218 | Five experimental parts, off at the start; the show-only parts, the log level and the thresholds are not experimental |
| D19 | CSL-1219 | Built after 1.0, except CS1 |
| D20 | CSL-1220 | No backup download or upload through the browser |
| D21 | CSL-533 | A missing witness report warns only after one was ever reported, and then after eight days |
| D22 | CSL-537 | A prune keeps rows no witness has covered, once a witness was ever reported |
| D23 | CSL-582 | The backup line warns at the interval plus two hours, never sooner than 26 hours without a schedule |
| D24 | CSL-596 | Cancelling a task before it runs: tier 1 for tasks of tier 1 and 2, tier 2 for a task of tier 3 |
| D25 | CSL-946 | A waiting task of a person who is disabled is cancelled |

## Open for the product owner

1. **After 1.0, or partly before?** The freeze says after (D19). CS2 is not a new part for users, but it rewrites the worker every installation runs; building it before 1.0 would test it longer, at the cost of change during the freeze.
2. **Seven days for lowered audit retention** (D11): long enough for other administrators to notice, short enough for a team that must delete for a legal reason?
3. **The restore test on the same PostgreSQL** (D9) costs the database's room and time once a week. Is that acceptable on the single node that is claimed, or should it stay a script on the host?

## What one machine cannot prove

- A worker on a real Kubernetes cluster and on a real LXC container (CS6).
- The restore test's effect on reads on the stress server's disks (CS3 measures it on the development container first).
- Mail reaching every system administrator (the tests check what is queued).

## Provenance

Our own design, from the study of blueprint 11 and specs 058 and 070. The split of console and command line in GitLab, Mattermost, Nextcloud, Discourse and Gitea was read in their public documentation on 2026-11-14, for behaviour only ([sources](../blueprint/11-console/sources.md)). No outside code or text.
