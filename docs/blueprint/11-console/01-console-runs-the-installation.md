# 11.1: The console runs the installation

Blueprint 08 lists **Running the installation from the admin screen** under "Wanted, later": an administrator manages as much as possible from the console, whatever the installation runs on (Docker Compose, Kubernetes, an LXC container or a package on bare Linux). The console controls and a worker does the work. The roadmap asks for a study first, which sorts every command and setting into **change in the console**, **show only**, or **command only, because …**. This is that study. The plan to build it is [spec 075](../../specs/075-console-runs-the-installation.md).

Spec 058 made a first, short sorting on 2026-11-11 and built backups, the configuration page and logging people out. Specs 059, 065, 066, 069 and 070 added restoring over the live installation, the experimental switches and the backup service as the only holder of the owner of the tables. This study starts again from the code of main `4b8a343` and goes further: it lists every command, variable, setting, fixed value and script an operator meets, judges each on security, recovery and platform, and designs what the console still lacks.

## How to read this study

Every line has a number (CSL-NNN) and a status: **built** (exists today and is reused), **partly**, **new** (planned in spec 075), **later** (wanted, not in spec 075) or **not** (decided against, chapter 12). A line gives the behaviour, then *Why*, and where it helps *Edge cases* and *Proof* (how a test will show it). The prefix CSL is new; CON was left alone because a later roadmap may want it.

The three classes, written the same way everywhere:

| Class | Short | Means |
|---|---|---|
| **Change in the console** | C | An administrator changes it in the console. A worker carries it out where the server may not, and reports back |
| **Show only** | S | The console shows it, with its value and where the value came from, but cannot change it |
| **Command only, because …** | K | It stays on the command line or in a file, and the console at most explains it. The reason is always given |

This is a study of Diffluence's own operations. The five products of chapter 04 were read in their public documentation, for how they split the work, never for code (`02-clean-room-and-licensing.md`). The sources are in [sources.md](sources.md).

## Chapters

| Chapter | Lines | About |
|---|---|---|
| [02 Inventory](02-inventory.md) | CSL-100 to CSL-280 | Every command, variable, setting, fixed value and script: what it does, who may use it, whether it is in the console |
| [03 Sorting](03-sorting.md) | CSL-300 to CSL-322 | The criteria, the three classes with a reason per item, the counts, and the platforms |
| [04 What others do](04-what-others-do.md) | CSL-350 to CSL-362 | GitLab, Mattermost, Nextcloud, Discourse and Gitea, and what Diffluence takes from them |
| [05 The worker pattern](05-the-worker-pattern.md) | CSL-400 to CSL-448 | Requests in the database, a worker per platform, capabilities, codes from the log, reporting back |
| [06 A stolen session](06-a-stolen-session.md) | CSL-450 to CSL-472 | Three tiers of guard, judged against the threat model |
| [07 The console areas](07-console-areas.md) | CSL-500 to CSL-596 | Each area: backups, restore test, audit, sessions, `master.key`, upgrades, log level, maintenance and disk, monitoring files, events |
| [08 The start-up configuration](08-start-up-configuration.md) | CSL-600 to CSL-615 | What a process started with, read-only, with its source, for the server and the worker |
| [09 Load budget](09-load-budget.md) | CSL-700 to CSL-712 | What each part costs |
| [10 Screens](10-screens.md) | CSL-800 to CSL-836 | The screens, following the design standard |
| [11 Edge cases](11-edge-cases.md) | CSL-900 to CSL-948 | Absent and old workers, two workers, restores, clocks, logs, nodes |
| [12 What it will not do](12-what-it-will-not-do.md) | CSL-1000 to CSL-1016 | The limits, each with a reason |
| [sources.md](sources.md) | | Every source, with its address and the date it was read |

## 1.1 What exists today

Read in the code on 2026-11-14: `cmd/diffluence`, `internal/httpapi/handlers_running.go` and the other admin handlers, `web/src/views/admin-running.tsx` and `admin.tsx`, `internal/config`, `internal/settings/sections.go`, `internal/health/health.go`, `compose.yaml`, `.env.example`, `deploy/` and `scripts/`.

- **CSL-001** · built · **The meeting point is the database.** The console writes requests and settings; the backup service of `compose.yaml` asks `diffluence backup next` every minute, does the work and answers with `backup report` and `backup restore-checked` (spec 058, D1 to D3). The tables are `backup_requests`, `backup_runs`, `backup_restore_checks`, `backup_worker` and `backup_mail`; restores live in the schema `ops` (spec 059).
- **CSL-002** · built · **The worker holds what the server does not** (ADR-044, spec 070): the owner of the tables, the backup key, and the keys derived from it, which sign the audit checkpoints and make the codes that confirm a restore.
- **CSL-003** · built · **A dangerous request waits for a code from the worker's own log**: the restore of spec 070, ten digits, 30 minutes, three tries, sealed and checked at every step.
- **CSL-004** · built · **In the console under System:** System health, Backups (worker state, key fingerprint, back up now, schedule, the list with test restores, the restore), Configuration (the settings the server started with, with their source, secrets hidden), Care, Settings (including audit retention and maintenance mode), Mail and Audit (the list, the export and the check of the chain). Under People: Sessions (one person, everyone).
- **CSL-005** · built · **Experimental parts** each have one switch for the installation and answer `404 experimental_off` while off (ADR-043, spec 069).
- **CSL-006** · partly · **Platforms.** Only Compose has a separate worker. The Helm chart, the LXC script and the packages run one process that holds both database addresses and no backup worker at all (threat model, "Accepted for now"). The console's backup page then says that no worker has reported.

What is missing is the rest of the roadmap paragraph: the restore test from the console, the audit witness and pruning, the guided replacement of `master.key`, the upgrade page, the log level, the disk thresholds, the monitoring files, the events switch, and a worker on Kubernetes and LXC.

## 1.2 The rule in one sentence

**What changes the installation goes through a request that a worker carries out; what only the server needs is shown, never changed; what must work when the server does not, or what would let a stolen session do lasting harm beyond the guards of chapter 06, stays a command.**

## 1.3 Decided on the product owner's behalf on 2026-11-14

Each was decided on the product owner's behalf on 2026-11-14, and may be overruled before the wave that uses it.

| Line | Decision | Reason |
|---|---|---|
| CSL-1201 | The prefix of this study is CSL | CON may be wanted by a later roadmap; CSL is unused in the repository |
| CSL-1202 | One request table for every new kind of work, `ops.tasks`, beside the existing backup and restore tables, which stay as they are | The backup tables already work on two releases at once; moving them gains nothing and risks the upgrade |
| CSL-1203 | The worker's loop moves from the shell script into Go, `diffluence worker run`; `pg_dump`, `pg_restore` and `gpg` stay outside tools | The loop is past 300 lines of bash with tricky control flow; Go makes it the same on every platform and testable. No new dependency |
| CSL-1204 | Three tiers of guard: administrator; system administrator with a fresh login; system administrator with a fresh login, a second factor and a code from the worker's log (chapter 06) | Matches what exists for maintenance and the restore, and states it once |
| CSL-1205 | Everything that shortens or removes history (backups kept, the backup target, audit retention, a contract migration, retiring a key) is in the third tier | A stolen session must not be able to erase the evidence of what it did |
| CSL-1206 | The worker offers what it can do (its capabilities), and the console offers only that | Platforms differ: a key from a Kubernetes Secret cannot be written by anyone in the installation |
| CSL-1207 | On Kubernetes the worker is a Deployment of one pod running `diffluence worker run`, not a CronJob | A CronJob every minute starts a pod each minute and loses the log line with the code between runs |
| CSL-1208 | On LXC and the packages the worker is a second systemd unit, `diffluence-worker.service`, under its own user | The server's unit then loses the owner address, as in Compose |
| CSL-1209 | The restore test restores into a scratch database on the same PostgreSQL, made once by the install, never into a new Compose project | The worker must never hold the Docker socket; a scratch database needs no new right at run time |
| CSL-1210 | The witness comparison stays a command, run from the operator's log pipeline, which reports its result to the console | The witness exists to be outside what the database owner and the console can touch (ADR-039) |
| CSL-1211 | Audit retention is applied by the worker once a day; lowering it takes effect only after a code from the worker's log and a waiting time of seven days, during which it can be cancelled | Pruning is irreversible; seven days give the other administrators, who are mailed, time to see it |
| CSL-1212 | The log level can be raised or lowered for at most 24 hours, one hour by default, and then returns to the configured level by itself | A debug level left on fills disks; a time limit needs nobody to remember |
| CSL-1213 | The disk thresholds become settings: warn between 5 and 50 percent free, refuse between 1 and 10 percent, refuse always below warn | Bounds keep a mistake from letting the disk fill |
| CSL-1214 | The monitoring rules and dashboard are embedded in the binary from `deploy/monitoring/`, with a test that the copy equals the source | A download from the console must be the file of the release that runs |
| CSL-1215 | The events switch of spec 068 is offered in the console, as a system administrator with a fresh login, while the release still has both transports | It is reversible at any time; the command stays |
| CSL-1216 | The guided replacement of `master.key` has the worker write the key files; nodes load a new key without a restart, when the database says a step was taken | The console cannot restart containers, and a restart per step is what makes the command road slow |
| CSL-1217 | Running waiting contract migrations before the grace ends is offered in the console, in the third tier, and only when every live node knows them | It ends the window in which a rollback works, so it must be on purpose |
| CSL-1218 | The parts that change the installation through new kinds of work are experimental and start off (ADR-043): `console_restore_test`, `console_audit_prune`, `console_contract_now`, `console_events_switch`, `console_keys`. What only shows, the log level and the disk thresholds are not | The feature freeze allows no new part on by default before 1.0; showing more changes nothing |
| CSL-1219 | Spec 075 is built after 1.0, except wave CS1, which only shows and adds two small bounded settings | The roadmap puts this item under "Wanted, later"; the freeze holds |
| CSL-1220 | Backups are never downloaded or uploaded through the browser | A stolen session must not carry the whole installation out, even encrypted, and an upload is a way in |
