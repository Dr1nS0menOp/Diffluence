# 11.2: Inventory

How to read this chapter: see [01-console-runs-the-installation.md](01-console-runs-the-installation.md).

Everything an operator uses today, read from the code and the docs of main `4b8a343` on 2026-11-14. **Who** says what a person needs to use it: a shell where the process runs, and which secret the command reads (the application role, the owner of the tables, `master.key`, the backup key). **Console today** says whether the console already does the same. The class it gets is in chapter 03.

Words used in the column Who:

- **App shell**: a shell in the server's container or machine, with the application role's address and `master.key`.
- **Worker shell**: a shell in the backup service, which holds the owner of the tables and the backup key (spec 070). Outside Compose today the server's process holds the owner address too.
- **Host**: the machine that runs Docker, systemd or `kubectl`, with the files beside `compose.yaml`.
- **Token**: an API token of a person, with that person's rights.

## 2.1 Commands of the binary

From `cmd/diffluence/main.go`, `key.go`, `client.go` and `worker.go`.

| Line | Command | What it does | Who | Console today |
|---|---|---|---|---|
| CSL-101 | `serve` | Runs the server in the role of `DIFFLUENCE_ROLE` | Host | No (it is the server) |
| CSL-102 | `migrate` | Applies expand migrations, after the checks before an upgrade (backup age, free space, PostgreSQL version) | Worker shell (owner) | System health shows waiting migrations |
| CSL-103 | `migrate --contract` | Also applies contract migrations at once, ending the window for a rollback | Worker shell (owner) | No; health shows when the grace ends |
| CSL-104 | `migrate --without-backup` | Upgrades once without a recent backup, recorded as `system.upgrade_without_backup` | Worker shell (owner) | No |
| CSL-105 | `user create-admin` | Makes a local administrator with every global permission, password on standard input | App shell | No; the console makes users, but needs someone logged in |
| CSL-106 | `setup-code` | Prints a new one-time code for the setup screen of a new installation | App shell | No |
| CSL-107 | `audit verify` | Seals what waits, checks the hash chain and the signed checkpoints, records `audit.verified` | App shell (needs `master.key` for the public key) | Yes: Audit, Check |
| CSL-108 | `audit verify --witness FILE` | Also compares the chain with the witness lines of an application log | App shell plus the log | No |
| CSL-109 | `audit prune` | Removes audit rows past the retention set in the console, leaving a signed anchor | Worker shell (owner, `master.key`) | Retention is set in the console; the removal is not |
| CSL-110 | `events check` | Checks the event log for gaps, cursors past lost events and long waits (L1 to L3) | App shell | Partly: health warns on waits and lag |
| CSL-111 | `events switch postgres\|nats` | Sends events through PostgreSQL or back through NATS (spec 068) | App shell | No |
| CSL-112 | `events status` | Shows the transport, the drain, older nodes and whether the release without NATS may be installed | App shell | No |
| CSL-113 | `backup note` | Records that a complete backup was made, for the checks before an upgrade | App shell or worker shell | No (the worker and the script call it) |
| CSL-114 | `backup next` | What the worker asks every minute: back up now, restore, show a code, or wait | Worker shell | Its answer is driven by the console |
| CSL-115 | `backup report` | The worker tells the console how a backup went | Worker shell | Its result is shown |
| CSL-116 | `backup restore-checked` | `restore-check.sh` tells the console how a test restore went | Host, through the server or a one-off container | Its result is shown |
| CSL-117 | `backup restore-request` | Asks for a restore over the live installation; inside the worker it fills in the code itself | Worker shell, or app shell (then a code is asked) | Yes: Backups, Restore (experimental) |
| CSL-118 | `backup restore-prepare` | Step of a restore: checks the backup, stops every node | Worker | Its step is shown |
| CSL-119 | `backup restore-run` | Step of a restore: restores, migrates, records, checks | Worker | Its step is shown |
| CSL-120 | `backup restore-abort` | Step of a restore: ends one that could not begin | Worker | Its step is shown |
| CSL-121 | `backup restore-way-back` | Restores the safety backup after a restore stopped halfway | Worker shell | No (the nodes wait on their page then) |
| CSL-122 | `worker migrate` | What the worker runs each round: the release's migrations, and contract migrations after the grace | Worker | Shown as health and upgrade facts |
| CSL-123 | `worker checkpoint --key` | Signs the head of the audit chain with the key derived from the backup key | Worker | No (checkpoints are listed by the check) |
| CSL-124 | `key add` | Writes `master.key.next` beside `master.key` | App shell, with write access to the key folder | No |
| CSL-125 | `key promote` | The new key seals from now on; the old one becomes `master.key.previous` | App shell, key folder | No |
| CSL-126 | `key reseal` | Seals every value and file again with the new key, in runs of two minutes | App shell | No |
| CSL-127 | `key check [--deep] [--require-current]` | Proves that everything opens, and with `--require-current` that nothing needs an older key | App shell | No |
| CSL-128 | `key retire` | Removes the old key once nothing needs it | App shell, key folder | No |
| CSL-129 | `session end --email ADDR` | Logs one person out everywhere without disabling them | App shell | Yes: People, Sessions |
| CSL-130 | `session end --all` | Logs everyone out, for an incident | App shell | Yes: People, Sessions |
| CSL-131 | `config check [--offline]` | Reads the environment and the file as a start would, prints each setting and its source with secrets hidden, and tries mail, the identity provider and the publish folder | Host (works with the server stopped) | Yes: System, Configuration (without the tries) |
| CSL-132 | `healthcheck` | Asks the local server whether it is ready; the container's probe | Host | No |
| CSL-133 | `doctor` | The checks of System health plus configuration and keys, as a report without secrets; exits 1 when a person is needed | App shell | Yes: System health |
| CSL-134 | `client spaces list \| pages list \| export \| import \| backup-content` | Uses the API with a token, for scripts | Token | The same actions exist in the app |
| CSL-135 | `version` | Prints the version | Anyone | Yes: System health |

And the entry points of the backup service image (`deploy/backup/backup-service.sh`):

| Line | Command | What it does | Who | Console today |
|---|---|---|---|---|
| CSL-136 | `diffluence-backup loop` | The worker itself: each minute it migrates, signs, asks the console and backs up | Host starts it | Its state is shown |
| CSL-137 | `diffluence-backup once` | One backup now | Host | Yes: Back up now |
| CSL-138 | `diffluence-backup way-back` | The way back after a restore stopped halfway | Host | No |

## 2.2 Settings the server reads

From `internal/config/config.go`: each is a variable `DIFFLUENCE_<NAME>`, a key of the configuration file, or a file named by `<NAME>_FILE` (spec 056). The Configuration page lists all of them today, with their source, secrets as `(set)`. **Who** is always the host: whoever starts the process. **Console today** is "shown" for all of them unless stated.

| Line | Setting | What it does |
|---|---|---|
| CSL-150 | `CONFIG_FILE` | The YAML file of settings |
| CSL-151 | `ROLE` | `all`, `api` or `worker` |
| CSL-152 | `HTTP_ADDR` | Where the process listens |
| CSL-153 | `PUBLIC_URL` | The address people type; cookies, origin checks, passkeys, OIDC redirect |
| CSL-154 | `INSTANCE_NAME` | The installation's name; typed to confirm a restore |
| CSL-155 | `TRUSTED_PROXIES` | Whose `X-Forwarded-For` is believed |
| CSL-156 | `DATABASE_URL` | The application role's address (secret part hidden) |
| CSL-157 | `MIGRATE_DATABASE_URL` | The owner's address, where this process migrates |
| CSL-158 | `LISTEN_DATABASE_URL` | Where a worker listens for new events (spec 068) |
| CSL-159 | `NATS_URL` | An outside NATS; empty is the embedded one |
| CSL-160 | `NATS_REPLICAS` | Copies per event in a NATS cluster |
| CSL-161 | `DATA_DIR` | The data folder: files, the embedded queue, by default the key |
| CSL-162 | `MASTER_KEY_FILE` | Where `master.key` lives |
| CSL-163 | `MIGRATE_ON_START` | Whether this process migrates when it starts |
| CSL-164 | `UPGRADE_BACKUP_MAX_AGE` | How old the last backup may be before an upgrade waits; `off` switches the rule off |
| CSL-165 | `CONTRACT_GRACE` | How long contract migrations wait after an upgrade |
| CSL-166 | `SESSION_IDLE_TIMEOUT` | The longest idle time of a session, the ceiling of the console's setting |
| CSL-167 | `SESSION_MAX_AGE` | The longest life of a session |
| CSL-168 | `SHUTDOWN_DELAY` | Time between "not ready" and closing connections |
| CSL-169 | `LOG_LEVEL` | `debug`, `info`, `warn` or `error` |
| CSL-170 | `PASSWORD_BLOCKLIST_FILE` | Extra passwords to refuse |
| CSL-171 | `EXAMPLE_SPACE` | Whether a new installation gets the example space |
| CSL-172 | `OUTWARD_ALLOW_PRIVATE` | Whether outgoing calls may reach private addresses and plain http |
| CSL-173 | `COLLAB_URL` | The service for editing together |
| CSL-174 | `COLLAB_TOKEN` | Its shared token (secret) |
| CSL-175 | `METRICS_TOKEN` | The token of `/metrics` (secret) |
| CSL-176 | `PDF_URL` | The PDF helper |
| CSL-177 | `PDF_PASSWORD` | Its password (secret) |
| CSL-178 | `COLLABORA_URL` | An office service |
| CSL-179 | `DRAWING_URL` | The draw.io service |
| CSL-180 | `PUBLISH_DIR` | Where public sites are written |
| CSL-181 | `TRACING` | Send traces to a collector |
| CSL-182 | `TRACING_SAMPLE` | Share of traces kept |
| CSL-183 | `OIDC_ISSUER` | The identity provider |
| CSL-184 | `OIDC_CLIENT_ID` | Its client |
| CSL-185 | `OIDC_CLIENT_SECRET` | Its secret (secret) |
| CSL-186 | `OIDC_DISPLAY_NAME` | The words on the login button |
| CSL-187 | `OIDC_AUTO_CREATE` | Make an account at a first login |
| CSL-188 | `OIDC_ALLOW_UNVERIFIED_EMAIL` | Accept an address the provider does not mark verified |
| CSL-189 | `OIDC_GROUPS_CLAIM` | The claim that lists groups |
| CSL-190 | `SMTP_HOST` | The mail server |
| CSL-191 | `SMTP_PORT` | Its port |
| CSL-192 | `SMTP_USERNAME` | Its user |
| CSL-193 | `SMTP_PASSWORD` | Its password (secret) |
| CSL-194 | `SMTP_FROM` | The sender |
| CSL-195 | `SMTP_REPLY_TO` | The reply address |
| CSL-196 | `SMTP_TLS` | `starttls`, `implicit` or `none` |

## 2.3 Settings of the worker, Compose and other programs

The server never reads these. **Who** is the host.

| Line | Setting | Read by | What it does | Console today |
|---|---|---|---|---|
| CSL-200 | `DIFFLUENCE_BACKUP_INTERVAL` | Backup service | How often to back up, the default under the console's schedule | Yes: the schedule wins (spec 058) |
| CSL-201 | `DIFFLUENCE_BACKUP_KEEP` | Backup service | How many backups to keep, the default | Yes: the number kept wins |
| CSL-202 | `DIFFLUENCE_BACKUP_DIR` | Compose | Where backups go on the host | No |
| CSL-203 | `DIFFLUENCE_BACKUP_KEY_DIR` | Compose | The folder of `backup.key` | Only the key's fingerprint |
| CSL-204 | `DIFFLUENCE_BIND` | Compose | Which address of the host the port listens on | No |
| CSL-205 | `DIFFLUENCE_PORT` | Compose | The port on the host | No |
| CSL-206 | `DIFFLUENCE_IMAGE` | Compose | The server's image | The version is shown |
| CSL-207 | `DIFFLUENCE_BACKUP_IMAGE` | Compose | The worker's image | No |
| CSL-208 | `DIFFLUENCE_RESTORE_CHECK_PORT` | `restore-check.sh` | The port of the throw-away project | No |
| CSL-209 | `DIFFLUENCE_URL`, `DIFFLUENCE_TOKEN`, `DIFFLUENCE_TOKEN_FILE` | `diffluence client` | Which server and which token | No |
| CSL-210 | `DIFFLUENCE_API_URL`, `DIFFLUENCE_COLLAB_PORT` | The editing service | Where it reaches the app, where it listens | No |
| CSL-211 | `OTEL_EXPORTER_OTLP_*`, `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES` | The trace exporter | The collector and labels | No |
| CSL-212 | The suffix `_FILE` | `internal/config` | Reads a setting from a file, for secrets | Yes: the source says "file" |
| CSL-213 | The values of the Helm chart (`publicUrl`, `mode`, `natsUrl`, replicas, `existingSecret`, persistence, ingress, `extraEnv`, resources) | Helm | The Kubernetes layout | Only through the settings they become |

`DIFFLUENCE_TEST_DATABASE_URL` and the other variables of `scripts/dev`, `scripts/e2e`, `scripts/stress` and `scripts/ha` are for building and testing Diffluence, not for running it, and are left out of the sorting.

## 2.4 Settings held in the database, and fixed values

The console already sets these, or they are constants in the code that an operator may want to see or change. **Who** for the sections is an administrator with a session; maintenance mode needs a system administrator with a fresh login.

| Line | Setting or value | Where | What it does | Console today |
|---|---|---|---|---|
| CSL-230 | Section `backups`: interval in hours, number kept | `internal/settings` | The schedule of the worker | Yes |
| CSL-231 | Section `retention`: audit days, with a reason past a year | `internal/settings` | How long audit rows are kept; only `audit prune` acts on it | Yes |
| CSL-232 | Maintenance mode with a message | `internal/settings` | Refuses every write | Yes |
| CSL-233 | Experimental parts | `internal/features` | One switch per part | Yes |
| CSL-234 | Section `access`: second factor, idle minutes, sign-up domains, guest invitations | `internal/settings` | Rules for accounts | Yes |
| CSL-235 | Sections `general`, `features`, `look`; mail switches, outgoing hosts, export policy, automation limit, analytics | various | The product's behaviour | Yes |
| CSL-236 | Disk: warn below 10% free | `internal/health` | System health and the band | No (constant) |
| CSL-237 | Disk: refuse files below 2% free | `internal/health` | Uploads, imports and exports refused | No (constant) |
| CSL-238 | Backup warns after 26 hours | `internal/health` | The backup line of health | No (constant) |
| CSL-239 | Release behind after 60 days | `internal/health` | The release line | No (constant) |
| CSL-240 | Contract migration waiting a day past the grace | `internal/health` | The migrations line | No (constant) |
| CSL-241 | Audit sealing: warn after a minute, a person after 15 | `internal/health` | The audit line | No (constant) |
| CSL-242 | Events: waiting a minute, lag five minutes | `internal/health` | The work line | No (constant) |
| CSL-243 | Restores: one a day; the code: three tries, 30 minutes | `internal/restore` | Limits of the restore | Shown in the restore's text |
| CSL-244 | Back up now: at most 4 an hour | `internal/backups` | Limit of a request | Shown beside the button |
| CSL-245 | Free space before an upgrade: 512 MiB | `internal/app` | A check before migrating | Shown on the waiting page when it fails |
| CSL-246 | Backup history 400 days, finished requests 90 days | `internal/backups` | The daily cleaner | No |

## 2.5 Scripts and files of the installation

From `scripts/` and `deploy/`. **Who** is the host, unless stated.

| Line | File | What it does | Console today |
|---|---|---|---|
| CSL-260 | `scripts/backup.sh` | One encrypted backup of a Compose installation, noted; for cron | Back up now does the same where a worker runs |
| CSL-261 | `scripts/restore.sh` | Restores a backup onto a clean Compose installation | No |
| CSL-262 | `scripts/restore-check.sh` | The weekly test restore into a throw-away Compose project, reported to the console | The result is shown |
| CSL-263 | `scripts/restore-drill.sh` | The full drill of blueprint 07, in its own projects | No |
| CSL-264 | `scripts/setup-secrets.sh` | Makes the secret files and the backup key, never overwrites | No |
| CSL-265 | `deploy/install-lxc.sh` | Installs on Debian or Ubuntu with a local PostgreSQL, as root | No |
| CSL-266 | `deploy/postgres-init.sh` | Makes the application role when the database is created | No |
| CSL-267 | `deploy/diffluence.example.yaml` | The commented configuration file | Its values are shown |
| CSL-268 | `deploy/systemd/diffluence.service` | The sandboxed unit for LXC and bare Linux | No |
| CSL-269 | `deploy/packaging/postinstall.sh`, `preremove.sh`, `postremove.sh` | What the `.deb` and `.rpm` do on install, upgrade and removal | No |
| CSL-270 | `deploy/backup/backup-service.sh` | The worker of Compose | Its state is shown |
| CSL-271 | `deploy/helm/diffluence` | The Kubernetes chart | No |
| CSL-272 | `deploy/monitoring/diffluence-rules.yml` | Prometheus alerts and objectives | No |
| CSL-273 | `deploy/monitoring/diffluence-dashboard.json` | A dashboard | No |
| CSL-274 | `compose.yaml` | The reference installation | No |
| CSL-275 | `.env` (from `.env.example`) | The settings of the Compose installation | Its values are shown |

- **CSL-280** · not sorted · The tools for building and testing Diffluence are left out, because an operator never runs them against an installation: `upgrade-check.sh`, `upgrade-test.sh`, `mutation-check.sh`, `reproducible.sh`, `ci-local.sh`, `check-dco.sh`, `check-licenses.mjs`, `fetch-craft.py`, `scripts/dev`, `scripts/e2e`, `scripts/ha/`, `scripts/stress/`, `scripts/load/` and `scripts/idp-test/`. `scripts/load/reading.js` is named in `install.md` for a load test against a copy; it stays a tool.

## 2.6 Totals

| Group | Items |
|---|---|
| Commands (2.1) | 38 |
| Settings the server reads (2.2) | 47 |
| Other settings (2.3) | 14 |
| Database settings and fixed values (2.4) | 17 |
| Scripts and files (2.5) | 16 |
| **All** | **132** |

Every setting of 2.2 is shown on the Configuration page today. Of the commands, nine have a console face that does or shows the same thing: CSL-107, CSL-110 (partly), CSL-117, CSL-129 to CSL-131, CSL-133, CSL-135 and CSL-137. Of the database settings, the six sections of CSL-230 to CSL-235 are set in the console.
