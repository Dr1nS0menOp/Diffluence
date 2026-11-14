# 11.3: Sorting

How to read this chapter: see [01-console-runs-the-installation.md](01-console-runs-the-installation.md).

## 3.1 The criteria

Every item of chapter 02 is judged on three questions, in this order. The first answer that rules something out decides.

- **CSL-300** · new · **Recovery: must it work when the server does not?** A command that recovers an installation the console cannot reach stays a command: the first administrator, a restore onto a clean machine, the way back after a restore stopped halfway, the check of a configuration before a restart. A console that is down cannot help bring itself up. *Proof:* chapter 11 lists, for each class K item that recovers, the state in which the console is unreachable.
- **CSL-301** · new · **Security: what could a stolen administrator session do with it, against what the threat model holds?** The threat model (`docs/security/threat-model.md`) holds that an administrator has no automatic access to content, that the audit chain cannot be rewritten by the application role, that backups hold against a compromised app process, and that a restore needs a system administrator, a fresh login, a second factor and a code from the worker's log. An item may enter the console only behind a guard that keeps each of those true (chapter 06). If no guard can, it stays a command. *Proof:* chapter 06 gives the tier of every class C item.
- **CSL-302** · new · **Platform: can it work the same on Compose, Kubernetes and LXC?** An item that only one platform can carry out goes into the console only as a capability the worker reports (CSL-410), so the console never offers what this installation cannot do; on the other platforms it is shown with the command. *Proof:* 3.6.
- **CSL-303** · new · **What the server needs to start stays in the configuration**, shown with its source (roadmap: "what the server needs to start … stays in the configuration and shows read-only with its source"). A setting the server reads only at start is shown, not changed: a value stored in the database would be ignored until a restart, and a wrong one could stop the start that would correct it.
- **CSL-304** · new · **Secrets stay where they are.** A password or token never moves into the database, where every backup and copy would carry it (spec 058, 1.3). The console shows `(set)` and the source.

## 3.2 Commands

| Line | Item | Class | Reason |
|---|---|---|---|
| CSL-101 | `serve` | K | Because it is the server. |
| CSL-102 | `migrate` | S | Expand migrations run by themselves in the worker; the upgrade page shows what waits and why (CSL-560). The command stays for installations without a worker. |
| CSL-103 | `migrate --contract` | C | As a request "run the waiting contract migrations now", third tier, only when every live node knows them (CSL-563). Security: it ends the window for a rollback, so it needs the code from the worker's log. |
| CSL-104 | `migrate --without-backup` | K | Because it waives a safety rule. A stolen session must not upgrade without a backup; the console offers Back up now instead, which ends the wait within minutes. |
| CSL-105 | `user create-admin` | K | Because it is how an installation gets an administrator when nobody can log in (recovery). |
| CSL-106 | `setup-code` | K | Because it proves access to the machine before any account exists (spec 038). |
| CSL-107 | `audit verify` | C | Built: Audit, Check. Extended with the signing keys and checkpoints (CSL-530). |
| CSL-108 | `audit verify --witness` | K | Because the witness must stay where neither the console nor the database owner can change it (ADR-039). Its result is reported to the console and shown (CSL-532). |
| CSL-109 | `audit prune` | C | As the worker's daily task, with a preview; lowering the retention is third tier and waits seven days (CSL-535). |
| CSL-110 | `events check` | S | A worker runs it hourly and the health line shows a breach (CSL-591). |
| CSL-111 | `events switch` | C | Second tier, while the release has both transports (CSL-592). Reversible at any time. |
| CSL-112 | `events status` | S | The events area shows it (CSL-590). |
| CSL-113 | `backup note` | K | Because it is the worker's and the script's own call; the console sees its result as the last backup. |
| CSL-114 | `backup next` | K | Because it is the worker's side of the console (spec 058, D2). |
| CSL-115 | `backup report` | K | The same. |
| CSL-116 | `backup restore-checked` | K | The same; it stays for `restore-check.sh`. |
| CSL-117 | `backup restore-request` | C | Built (specs 059, 070), third tier, experimental. The command stays inside the worker. |
| CSL-118 | `backup restore-prepare` | K | Because it is a step the worker runs. |
| CSL-119 | `backup restore-run` | K | The same. |
| CSL-120 | `backup restore-abort` | K | The same. |
| CSL-121 | `backup restore-way-back` | K | Because the nodes wait on their page when it is needed, so the console is not there (recovery). |
| CSL-122 | `worker migrate` | K | Because it is the worker's own round. |
| CSL-123 | `worker checkpoint` | K | The same; the key it signs with must never reach the server (ADR-044). |
| CSL-124 | `key add` | C | Through the worker, second tier, where the key folder is writable (CSL-550). On a read-only key mount (Kubernetes Secret) it is shown as a command. |
| CSL-125 | `key promote` | C | The same. |
| CSL-126 | `key reseal` | C | As a worker task with progress (CSL-553). |
| CSL-127 | `key check` | C | As a worker task, first tier, with its result (CSL-554). |
| CSL-128 | `key retire` | C | Third tier: once retired, a file that still needs the old key is lost (CSL-555). |
| CSL-129 | `session end --email` | C | Built. |
| CSL-130 | `session end --all` | C | Built, with a fresh login. |
| CSL-131 | `config check` | S | Built as the Configuration page; the command stays for a change before a restart, when the server may not start (recovery). |
| CSL-132 | `healthcheck` | K | Because it is the container's own probe. |
| CSL-133 | `doctor` | S | System health is its console face. |
| CSL-134 | `client …` | K | Because it is a client of the API, for scripts. |
| CSL-135 | `version` | S | Shown on System health and the upgrade page. |
| CSL-136 | `diffluence-backup loop` | K | Because it is the worker. |
| CSL-137 | `diffluence-backup once` | C | Built as Back up now. |
| CSL-138 | `diffluence-backup way-back` | K | As CSL-121. |

## 3.3 Settings the server reads

All 47 settings of 2.2 are **S**, with one exception, CSL-169. The reasons, by kind:

| Lines | Class | Reason |
|---|---|---|
| CSL-150 to CSL-163 (file, role, addresses, proxies, database and queue addresses, data folder, key file, migrating on start) | S | CSL-303: the server needs them to start or to reach what it serves; a wrong value would make the console unreachable to correct it, and the database addresses could not change the address they are read through. |
| CSL-164 `UPGRADE_BACKUP_MAX_AGE` | S | Security: `off` switches off the backup rule of ADR-040; a stolen session must not be able to. |
| CSL-165 `CONTRACT_GRACE` | S | The grace is the operator's promise of a rollback window; ending it early is the separate third-tier request CSL-103. |
| CSL-166, CSL-167 session limits | S | The ceiling the console's idle minutes stay within (spec 058). |
| CSL-168 `SHUTDOWN_DELAY` | S | Read at start; a matter of the load balancer. |
| CSL-169 `LOG_LEVEL` | **C** | As an override for at most 24 hours, returning to the configured level (CSL-570). The variable stays the default. |
| CSL-170, CSL-171 blocklist file, example space | S | Read at start. The example space can already be added from Settings. |
| CSL-172 `OUTWARD_ALLOW_PRIVATE` | S | Security: it opens private networks to outgoing calls; a stolen session could use the installation to reach inside the network. |
| CSL-173 to CSL-179 helpers and their tokens | S | Addresses and secrets of other services (CSL-304). |
| CSL-180 `PUBLISH_DIR` | S | Security: a person must not choose where the server writes (spec 033). |
| CSL-181, CSL-182 tracing | S | Read at start, with the standard OpenTelemetry variables beside them. |
| CSL-183 to CSL-189 OIDC | S | A secret, and `AUTO_CREATE` and `ALLOW_UNVERIFIED_EMAIL` decide who can get an account: a stolen session must not open the installation to everyone at the provider. |
| CSL-190 to CSL-196 SMTP | S | A password (CSL-304); the mail test of the console already tries them. |

## 3.4 Other settings

| Line | Item | Class | Reason |
|---|---|---|---|
| CSL-200 | `DIFFLUENCE_BACKUP_INTERVAL` | C | Built: the console's schedule wins. |
| CSL-201 | `DIFFLUENCE_BACKUP_KEEP` | C | Built; lowering it becomes third tier (CSL-505). |
| CSL-202 | `DIFFLUENCE_BACKUP_DIR` | S | The worker reports its targets; the folder is on the worker's machine (CSL-503). |
| CSL-203 | `DIFFLUENCE_BACKUP_KEY_DIR` | S | Only the fingerprint, as today. |
| CSL-204 | `DIFFLUENCE_BIND` | K | Because the server never sees it: Compose reads it to publish a port. |
| CSL-205 | `DIFFLUENCE_PORT` | K | The same. |
| CSL-206 | `DIFFLUENCE_IMAGE` | K | Because changing the image is an upgrade, which the console does not do (CSL-1001). The version that runs is shown. |
| CSL-207 | `DIFFLUENCE_BACKUP_IMAGE` | K | The same; the worker's version is shown (CSL-412). |
| CSL-208 | `DIFFLUENCE_RESTORE_CHECK_PORT` | K | Because only the script reads it; the console's restore test does not need a port (CSL-510). |
| CSL-209 | Client variables | K | Because they belong to a client, not to the installation. |
| CSL-210 | Variables of the editing service | K | Because another program reads them. |
| CSL-211 | `OTEL_*` | S | Shown with values that may hold credentials hidden (CSL-604). |
| CSL-212 | The `_FILE` suffix | S | Built: the source says so. |
| CSL-213 | Helm values | K | Because Helm owns them; they reach the console only as the settings they become. |

## 3.5 Database settings, fixed values, scripts and files

| Line | Item | Class | Reason |
|---|---|---|---|
| CSL-230 | Section `backups` | C | Built; gains the target and the third tier for what removes backups (CSL-500 to CSL-506). |
| CSL-231 | Section `retention` | C | Built; applied by the worker, lowering is third tier (CSL-535). |
| CSL-232 | Maintenance mode | C | Built, second tier. |
| CSL-233 | Experimental parts | C | Built. |
| CSL-234 | Section `access` | C | Built. Out of the scope of this study. |
| CSL-235 | Other sections | C | Built. Out of the scope of this study. |
| CSL-236 | Disk warn 10% | C | A setting, within bounds (CSL-581). |
| CSL-237 | Disk refuse 2% | C | The same. |
| CSL-238 | Backup warn after 26 hours | S | Derived from the schedule from now on: the interval plus two hours (CSL-582). |
| CSL-239 | Release behind after 60 days | S | It follows the support policy of `release.md`, not a preference. |
| CSL-240 | Contract waiting a day past the grace | S | Follows the grace. |
| CSL-241 | Audit sealing thresholds | S | Follow ADR-041. |
| CSL-242 | Event thresholds | S | Follow spec 068, D9. |
| CSL-243 | Restore and code limits | S | Security: they bound what a stolen session can try. |
| CSL-244 | Back up now, 4 an hour | S | The same, against wearing the server down. |
| CSL-245 | 512 MiB before an upgrade | S | A safety margin, not a preference. |
| CSL-246 | History kept 400 and 90 days | S | Shown on the backup page; small tables. |
| CSL-260 | `scripts/backup.sh` | K | Because it is the road without a worker, and runs where the server cannot. Back up now covers the need where a worker runs. |
| CSL-261 | `scripts/restore.sh` | K | Because it restores onto a clean machine, where no server runs (recovery). |
| CSL-262 | `scripts/restore-check.sh` | C | The restore test moves into the worker, into a scratch database (CSL-510); the script stays for installations without one. |
| CSL-263 | `scripts/restore-drill.sh` | K | Because it builds and destroys whole Compose projects; it is a drill, not an operation. |
| CSL-264 | `scripts/setup-secrets.sh` | K | Because it makes the secrets before anything runs. |
| CSL-265 | `deploy/install-lxc.sh` | K | Because it installs the server. |
| CSL-266 | `deploy/postgres-init.sh` | K | Because it runs once when the database is made, as its superuser. |
| CSL-267 | `deploy/diffluence.example.yaml` | S | The file's values and lines are shown as sources (CSL-601). |
| CSL-268 | `deploy/systemd/diffluence.service` | K | Because systemd owns it. |
| CSL-269 | Packaging scripts | K | Because the package manager runs them. |
| CSL-270 | `deploy/backup/backup-service.sh` | K | Because it is the worker. |
| CSL-271 | Helm chart | K | Because Helm owns it. |
| CSL-272 | Monitoring rules | S | A download of the release's copy (CSL-585). |
| CSL-273 | Monitoring dashboard | S | The same. |
| CSL-274 | `compose.yaml` | K | Because Compose owns it; the console never edits files on the host (CSL-1004). |
| CSL-275 | `.env` | S | Its values reach the console as the environment, with their source. |

## 3.6 The counts

- **CSL-310** · new · The sorting of the 132 items of chapter 02:

| Group | Change in the console | Show only | Command only | All |
|---|---|---|---|---|
| Commands | 13 | 6 | 19 | 38 |
| Settings the server reads | 1 | 46 | 0 | 47 |
| Other settings | 2 | 4 | 8 | 14 |
| Database settings and fixed values | 8 | 9 | 0 | 17 |
| Scripts and files | 1 | 4 | 11 | 16 |
| **All** | **25** | **69** | **38** | **132** |

Of the 25 in class C, 13 are built (CSL-107, CSL-117, CSL-129, CSL-130, CSL-137, CSL-200, CSL-201, CSL-230 to CSL-235) and 12 are new: the contract migrations now, the prune, the events switch, the five key steps, the log level, the two disk thresholds and the restore test. The 38 in class K, by their main reason, item by item so the count can be checked:

| Reason | Items | Count |
|---|---|---|
| The worker's own side of the console | CSL-113 to CSL-116, CSL-118 to CSL-120, CSL-122, CSL-123, CSL-136, CSL-270 | 11 |
| Recovery: the server is down, not set up, or waiting | CSL-105, CSL-106, CSL-121, CSL-138, CSL-261, CSL-264, CSL-266 | 7 |
| The platform's own program, tool or file | CSL-101, CSL-132, CSL-134, CSL-204 to CSL-210, CSL-213, CSL-260, CSL-263, CSL-265, CSL-268, CSL-269, CSL-271, CSL-274 | 18 |
| Security: no guard keeps the threat model true | CSL-104, CSL-108 | 2 |

Security decides far more than those two: it is why the backup rule, outgoing calls, the publish folder, the identity provider and every secret of 3.3 are shown and not changed, and why the 12 new class C items sit behind the tiers of chapter 06.

## 3.7 Platforms

- **CSL-320** · new · What each class C and S area needs from the platform, and what it does where it cannot:

| Area | Compose | Kubernetes | LXC and packages |
|---|---|---|---|
| Worker | The backup service (built) | A Deployment of one pod, `diffluence worker run` (CSL-421) | A second unit, `diffluence-worker.service` (CSL-422) |
| Owner of the tables | Worker only (built) | Worker only, once the chart has the worker; until then the server holds it | Worker only, once the unit exists; until then the server holds it |
| Backup target | Host folder (built); S3 later | A volume claim of the worker; S3 later | A folder of the worker's user; S3 later |
| Back up now, schedule, keep | Worker | Worker | Worker |
| Restore over the live installation | Worker (built) | Worker; the app's pods stay up and wait, no right to scale (spec 058, 3.2) | Worker |
| Restore test | Scratch database (CSL-510) | The same | The same |
| Audit prune | Worker | Worker | Worker |
| Key steps | Worker writes in the data volume | **Shown with commands**: the key comes from a Secret, which only the cluster's operator may write (CSL-556) | Worker writes in the key's folder, when it is the worker's group's to write |
| Contract migrations now | Worker | Worker (or a pre-upgrade Job, when the chart has one) | Worker |
| Log level, disk thresholds, events switch | Server, through the database | The same | The same |
| Codes in the worker's log | `docker compose logs backup` | `kubectl logs deploy/diffluence-worker` | `journalctl -u diffluence-worker` |
| Start-up configuration | Server and worker each report theirs | The same | The same |

- **CSL-321** · new · Without any worker, the console offers no class C item that needs one: each area says that no worker has reported, names the platform's way to add one, and shows the command. *Why:* a button that does nothing for ever is worse than none. *Proof:* an e2e test with no worker row.
- **CSL-322** · later · Where a platform has an outside service that does the work better, such as a managed database's own backups or a cluster's backup operator, the worker may report that the target is managed elsewhere, and the console shows only its last result. Not in spec 075.
