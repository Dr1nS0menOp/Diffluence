# Changelog

Plain-language notes per release. Every entry has an "Upgrade impact" line.

## Unreleased
### Changed: limits of signing in, counted audit erasures and exports, and accounts from single sign-on (spec 071, wave RV4)
- The limits per address count an IPv6 address by its /64 network. The limiter forgets ended windows as it goes. Signing in has its own share of the database pool beside saving, and a request that ends while waiting for a password hash stops waiting
- An audit CSV ends with a line that says `#complete`, `#limit-reached` or `#incomplete` with the number of events. It is read in pages of 5,000, and a long export or export download is no longer cut off by the server's write timeout. The check of the audit log from the console may walk the chain for up to ten minutes
- A change to who may do what is counted once per transaction, at its commit, so a long import or sync no longer makes other permission changes wait
- A session that must set up a second factor reaches only the setup screen's routes and signing out; making a token there is refused
- Anonymising someone records how many personal parts of audit events it erased. The check of the audit log counts the parts that are gone, and fails when more are gone than erasures recorded (`erased_parts`, `recorded_erasures`)
- Forgot password and sign-up always answer after 400 ms, while their work goes on beside the answer
- Single sign-on makes no account at a first login unless `DIFFLUENCE_OIDC_AUTO_CREATE=true`, and then only for an address the provider marks as verified. New setting `DIFFLUENCE_OIDC_ALLOW_UNVERIFIED_EMAIL` accepts unverified addresses. The audit log names the reason `email_not_verified`
- The editing service compares the API's token in constant time, and runs in `compose.yaml` with a read-only root and no capabilities
- Upgrade impact: an installation whose people get their account at their first single sign-on sets `DIFFLUENCE_OIDC_AUTO_CREATE=true` before upgrading, and, with a provider that does not mark addresses as verified (Authentik by default, Entra ID), also `DIFFLUENCE_OIDC_ALLOW_UNVERIFIED_EMAIL=true` or marks them there. Migration 0057 changes the permission triggers and counts the audit parts erased before; finish upgrading every node before anonymising anyone. A script that reads the audit CSV skips its last line. Take the new `compose.yaml`. Migration 0056 only holds its number
### Changed: the backup service migrates, signs the audit checkpoints and confirms a restore (spec 070, wave RV3b)
- In `compose.yaml` the server gets only the application's database role. The backup service alone holds the owner of the tables: every round, at most a minute apart, it runs the migrations of a new release, and the contract migrations once their grace period is over. Until it has, the server shows "Waiting before an upgrade" with the kind `migrations` and looks again every 5 seconds
- The backup service signs the audit checkpoints every five minutes with a key derived from the backup key, and writes them to its own log. The change of key is a position in the audit chain, endorsed by the key derived from `master.key`, so older checkpoints keep verifying and `diffluence audit verify` needs nothing new. The database refuses a checkpoint of a key no longer in force
- A restore from the console waits for a one-time code the backup service writes to its log (`docker compose logs backup`), entered on the Backups page within 30 minutes, with three tries. The backup service checks its seal on the restore at every step, and the way back restores only a safety backup it made. `diffluence backup restore-request` run inside the backup service needs no code
- `diffluence worker migrate` and `diffluence worker checkpoint` are the backup service's new steps. `diffluence migrate`, `audit prune` and `user create-admin` in a container without the owner of the tables say where to run them instead of trying as the application role. `scripts/restore.sh` migrates a restored backup of an older release in a one-off container
- Upgrade impact: rebuild the backup service with the server (`docker compose up -d --build`), or pull its image of the same release. With the `compose.yaml` of the repository nothing else is needed. A `compose.yaml` of your own keeps working as before, with the server migrating; [upgrade.md](docs/operations/upgrade.md) says how to move. Migration 0055 adds the table of checkpoint keys, a trigger on `audit_checkpoints` and five columns to `ops.restores`. The code before a restore holds once the backup service runs this release, which is one more reason to rebuild it with the server
### Changed: experimental parts start off, a passkey after the password, and a smaller claim (spec 069, wave RV3a)
- Automation rules, databases, bundles between instances and restore from the console are experimental for 1.0 and off on a new installation (ADR-043). An administrator turns each on under Administration, Organising, Experimental parts. While a part is off, its routes answer 404 `experimental_off`, no screen offers it, a rule stays and does not run, and a database stays and can be edited. Public sites and analytics keep their own switches, which already start off, and are marked experimental
- The example space of a new installation leaves out its database while databases are off
- An account whose second factor is a passkey is asked for it after its password; before, the password alone was enough for such an account. An account with an authenticator app and a passkey may use either. New routes `POST /api/v1/auth/login/passkey/begin` and `/finish`; the login answer names the factors in `mfa_methods`
- A token's scope follows the route it calls, not words in the address: the pages of a label called "export" are read with the read scope
- The few queries that named a table from a list now are written out in full; a test keeps every query from being put together with `fmt.Sprintf`
- The README, the install guide and the known limits say what Diffluence is sized for: one node, a few hundred people, measured on your own hardware. `SECURITY.md` says alpha, as the README does. The risks page and the threat model are brought up to date. The built decisions ADR-005 to 007, 009, 011, 013 and 017 to 020 are marked accepted
- `compose.dev.yaml` pins `postgres:16` by digest, as `compose.yaml` does
- Upgrade impact: migration 0054 keeps on each experimental part the installation already used: a rule exists, a database exists, a bundle export or import is on record, or a restore was asked for in the console. Check Administration, Organising afterwards and turn on what you use that left no trace (`docs/operations/upgrade.md`). A script that logs in with a password for an account with a passkey now gets `mfa_required` and must finish with the passkey. A token with only the read scope can no longer be refused on a route that merely contains the word export
### Changed: rules for administration, editing sockets and logins (spec 066)
- Open editors no longer wait in the order of work or count as requests in the metrics. They have caps of their own: 512 per node and 8 per login.
- Only a system administrator acts on an account or group that holds system administration: its name, status, reset link, second factor, password stop, end date, anonymising, and who is in such a group. Only a system administrator starts a restore from the console
- An administrator now sends a reset link instead of typing a new password: mailed when mail is on, otherwise shown once in the console to pass on. The person chooses their own password. Accounts from an identity provider get no link
- A wrong code given for a restore is written to the audit log, and codes count against a limit per account
- Discovery of the OIDC provider stops after 10 seconds, runs once for every login waiting on it, and a failure is remembered for 30 seconds
- `compose.yaml` passes `DIFFLUENCE_TRUSTED_PROXIES` on, and trusts a proxy on the same machine or in the same Compose project by default. System health warns when every login of a day came from one private address
- Upgrade impact: take the new `compose.yaml`. With the port on `127.0.0.1`, as by default, there is nothing more to do. With `DIFFLUENCE_BIND=0.0.0.0`, set `DIFFLUENCE_TRUSTED_PROXIES` to the proxy's own address. Administrators without system administration lose the restore and the actions above on system administrators. A script that sent a password to `POST /api/v1/admin/users/{id}/reset-password` gets a link back instead. No migration
### Added: events through PostgreSQL, built beside NATS and still off (spec 068, wave EV1)
- A second path for events: a sequencer moves each saved event into an event log with a position that has no gaps, and every listener reads that log in four lanes, each held by one worker at a time. A failed event waits for its retries as before and then shows under Care. A worker listens for new events and also looks every second, so a lost wake-up costs a second, never an event
- This release keeps sending events through NATS: the new path starts only when a later release switches to it
- `diffluence events check` says whether the event log lost anything: no gap in its positions, no listener past an event it did not hand over, nothing waiting long while a worker runs
- New setting `DIFFLUENCE_LISTEN_DATABASE_URL`: where a worker listens for new events, for an installation behind PgBouncer in transaction mode. Empty means `DIFFLUENCE_DATABASE_URL`
- System health warns when an event waits more than a minute to be passed on. New metrics `diffluence_events_waiting`, and once the new path runs `diffluence_event_lag_seconds` and `diffluence_event_retries` per listener
- Upgrade impact: migration 0053 adds the tables, empty, and lets autovacuum clean the outbox by a count of dead rows. The relay now takes a lock of its own per batch, so on several workers one relays at a time. Migration 0052 only holds its number
### Changed: pinned and scanned builds, and a ticket for editing together (spec 067)
- The service for editing together now gets a signed ticket for one person and one page from the API, which checks the session and the right to edit when the socket opens. The browser's cookies stay with the API. The service asks again with the ticket every minute and gets a new one each time; these checks do not count as activity, so a session still ends at its idle limit while an editor is open
- Every GitHub action is pinned by commit, and the base images `postgres:16` and `node:22-slim` by digest. CI builds and scans the app, backup and editing-together images with Trivy, and runs `npm audit` for the editing-together service. A release signs and attests all three images
- The image of the editing-together service ships without npm and corepack, which it never runs. The backup image takes Debian's security updates when it is built, and leaves out `gosu`, which it never runs
- A test walks every route that takes an id as someone with no access, from one table that says whose object each route reaches
- Upgrade impact: deploy the API and the editing-together service of this release together; an older service is refused by the new API and the other way round, so open editors reconnect once. `compose.yaml` pins `postgres:16` by digest; update it with the release
### Fixed: the database settings after a restore, and two builds of one release (spec 065)
- A backup does not carry the defaults of the database itself, so a restore into a fresh database lost `plan_cache_mode` and `jit` while the migrations said they were set. Every run of migrations now sets them again: the node that starts after a restore, `diffluence migrate`, and the restore from the console. The restore drill checks both in a plain session
- The dry run of a release built the same release twice and found them different on CI: the first build was kept inside the checkout, and Go records an untracked folder in the binary. It is kept outside now, the checkout must be clean before the second build, and a difference fails the job again
- Upgrade impact: none. A node writes the two defaults at every start, which changes nothing when they are already there
### Added: ready for a NATS cluster and for PgBouncer (spec 063)
- `DIFFLUENCE_NATS_REPLICAS` sets how many NATS nodes keep a copy of each event: 1 (the default, and the only value for the embedded server), 3 or 5 for a cluster. Any other value is refused at start. A node raises the copies of an existing event stream to this setting, and never lowers them by itself
- The database now holds `plan_cache_mode = force_custom_plan` and `jit = off` as its own defaults, so every session has them, also through PgBouncer in transaction mode, which drops what a client sends at connect. The pool still sends both, for a direct connection
- Upgrade impact: migration 0050 sets those two defaults on the database. It needs the owner of the database; when the migration role is not the owner, as on some managed databases, it notes that and goes on, and only direct connections get both settings. A NATS cluster must run this release on every node before `DIFFLUENCE_NATS_REPLICAS` is raised, because an older release asks for one copy
### Added: document control, first part (spec 064, wave DC1)
- A space can have workflows: states with a kind (draft, in review, approved, rejected, obsolete) and named steps between them. A workflow that cannot reach approval from every open state is refused, and each saved change is a new version with a list of what changed
- A workflow governs a whole space (for the kinds of item chosen), one page, or a page and everything below it. The space workflow wins, and turning it on removes the page workflows for good. Setting one shows first how many pages change and where they land
- Readers only see approved versions of a controlled page. A page that was never approved, or was withdrawn, does not exist for them: not in the tree, search, the feed, links, public sites or the API. Every published change, also a silent one, a rename or a restore, waits for review while readers keep the approved version; a change back to the approved text returns the page to approved
- A band under the title says the state, the approved version and who approved it, which workflow governs the page and why, and offers its authors the next steps. Space settings list the workflows. Making and changing workflows is done through the API in this release; the editor comes later
- Two new permissions: `space.workflows` (split off "Run the space") and `page.workflow` (split off "Edit pages"). Whoever held the broader one holds the new one, so nobody gains or loses anything. Bundles are now version 6; an older bundle gives the new permissions to those its grants gave the broader ones
- Upgrade impact: migration 0051 adds the tables and the two permissions and changes the rule of view, which costs nothing while no page is controlled. Migration 0050 only holds its number
### Added: restoring over the live installation from the console (spec 059)
- Administration, System, Backups: choose a complete backup and restore it over the whole installation. The page says what is lost, asks for the installation's name typed out and for a second factor: a code from the authenticator app, a recovery code, or a passkey confirmed just before. At most one restore a day
- The backup service carries it out: it checks the backup while people still work, stops every node, makes a safety backup of the installation as it is, restores the database in one transaction and the files with their keys, brings the database up to this release, and checks the audit log and the keys. Every node shows a waiting page meanwhile, and serves again once it is done. The restore and who asked for it are written into the restored audit log
- If a restore stops halfway, every node keeps waiting, and the page names the way back: `docker compose run --rm backup way-back` restores the safety backup. An operator without the console asks with `docker compose run --rm diffluence backup restore-request --name <backup> --confirm <installation name>`
- Upgrade impact: migration 0049 adds the schema `ops`, which steers a restore and is never restored, and backups hold its tables without rows. In `compose.yaml` the backup service now writes the data volume, and gets the owner's database address and `DIFFLUENCE_DATA_DIR`: update `compose.yaml` with the release
### Changed: reads fast on real data (spec 061)
- The start screen's feed is fast on a large data set: on 60,000 pages "for you" answers in tens of milliseconds where it took minutes, "recent" in about 40 ms, and "popular" counts a week of activity in one pass
- The database connections plan every query with its values and do not JIT-compile plans; both made reads far slower on large data
- No read can hold the database for long: a query of a read stops after 15 seconds, and the person is told the server is busy and may try again
- `scripts/stress/real-data.sh` measures the feed on a dump of real data
- Upgrade impact: none to do. Each query is planned anew, which costs about a millisecond
### Added: traces with OpenTelemetry, off by default (spec 060)
- `DIFFLUENCE_TRACING=true` sends traces to an OpenTelemetry collector over OTLP, at the address in the standard `OTEL_EXPORTER_OTLP_ENDPOINT`. One new trace in ten is kept (`DIFFLUENCE_TRACING_SAMPLE`); a request that arrives inside a sampled trace stays in it
- Spans for every request by route pattern, every query by operation and table, every event sent and handled (the trace crosses the queue in its headers), every job and periodic task, the audit sealer and the backup worker's commands. They never hold a path, an id, a statement, a value, a name, an address, text or a token, and a failure records its kind, not its message
- While tracing is on, log lines written inside a request or a job carry `trace_id` and `span_id`
- The operations guide lists the log messages an operator may build on; they change only with a note here
- New dependency: the OpenTelemetry Go SDK 1.47.0 and its OTLP exporter (Apache-2.0), with the modules they bring (listed in `docs/dependencies.md`)
- Upgrade impact: none. Off by default; nothing leaves the machine until you switch it on
### Added: running the installation from the console (spec 058)
- Administration, System, Backups: set how often the backup service backs up and how many backups it keeps, ask for a backup now (at most 4 an hour), and see every backup with its size, the step that failed, and whether a test restore of it passed. The page says when the service last checked in and whether it holds a key, by a fingerprint only. It also spells out the command that restores a chosen backup over this installation
- The backup service asks the console what to do and reports each backup; `scripts/restore-check.sh` reports each weekly test restore. A failed backup or test restore shows on System health and is mailed to the administrators, at most twice a day. The service needs the release that has migration 0048; an older one keeps its own schedule
- Administration, System, Configuration: the settings the server starts with and where each came from, secrets hidden, and what a restart would refuse
- Administration, People, Sessions: log one person out everywhere, or everyone after an incident
- New commands for a backup worker on any platform: `diffluence backup next`, `backup report`, `backup restore-checked`
### Added: backups that run by themselves, and an upgrade that waits instead of looping (spec 054)
- **Upgrade impact first: run `sh scripts/setup-secrets.sh` once more after you update `compose.yaml`.** It makes the backup key `backup-key/backup.key` and the folder `./backups`, and overwrites nothing. Keep a copy of the key in a password safe, and copy `./backups` off the machine
- `compose.yaml` runs a backup service: every 4 hours a backup of the database, files, keys and secrets, encrypted, into `./backups`, keeping the newest 42, and noted for the checks before an upgrade. `docker compose run --rm backup once` makes one now. Set `DIFFLUENCE_BACKUP_INTERVAL`, `DIFFLUENCE_BACKUP_KEEP`, `DIFFLUENCE_BACKUP_DIR` and `DIFFLUENCE_BACKUP_KEY_DIR` to change it
- An upgrade that may not start no longer exits and restarts in a loop. The node waits: every page says why and how to go on, `/readyz` answers not ready, and it migrates by itself within a minute once the cause is fixed. `scripts/backup.sh` makes it check at once
- The first upgrade of an installation that never noted a backup goes on once, with a warning and an entry in the audit log, instead of stopping (ADR-042). From the next upgrade on, a backup of the last 24 hours is asked for
- `diffluence migrate` asks for a recent backup too. `diffluence migrate --without-backup` upgrades once without one and records that in the audit log
- `scripts/restore-check.sh` restores your newest backup into a throw-away project, checks it and removes it: run it every week from a timer. The backup guide now covers pgBackRest with point-in-time recovery, and why a standby is not a backup
- Upgrade impact: see the first line. No migration. The backup service needs nothing else; until the key exists, it logs why it makes no backup
### Added: releases with packages, signatures and an SBOM (spec 057)
- **Packages.** A release now comes as archives and as `.deb` and `.rpm` packages for Linux amd64 and arm64. They have the sandboxed systemd unit and the `diffluence` user, and never start the service on a first install. Removing a package keeps the data. `docs/operations/install.md` has the steps
- **Signatures and provenance.** Every file of a release is listed in `checksums.txt`, signed with Sigstore by the release workflow, without a long-lived key, and has SLSA provenance from GitHub. The image on `ghcr.io` is signed and attested the same way
- **SBOM.** Each release lists its parts in SPDX and CycloneDX: per binary, for the source with the npm packages, and per image
- **Reproducibility.** CI checks on every push to `main` that two builds of the binary, the web assets, the archives and the packages are the same byte for byte
- **Base images.** The build and runtime images in `Dockerfile` are pinned by digest
- **Support policy.** From 1.0 only the newest release line is supported, a security fix ships within days, and an installation up to six months old upgrades in one step (`SECURITY.md`)
- New release tools, only in the release workflow and CI, never in the product: GoReleaser 2.20.1 (MIT), cosign 3.1.4 and syft 1.57.1 (Apache-2.0)
- Upgrade impact: none

### Added: a configuration file, and diffluence config check (spec 056)
- Settings can live in one YAML file named by `DIFFLUENCE_CONFIG_FILE`, beside the environment. The order is the default, then the file, then the environment, so nothing changes for an installation without a file. Unknown keys, a key written twice and a value of the wrong kind refuse the start and name the line; secrets come from paths, or from a file only its owner may read. `deploy/diffluence.example.yaml` is an example
- `diffluence config check` checks the configuration without starting, prints each setting with the value in effect and where it came from, secrets hidden, and exits 1 when a start would be refused. It also tries the mail server, the identity provider and the publish folder; a start does the same and logs a warning, and `diffluence doctor` prints it
- `compose.yaml` no longer repeats the defaults of `DIFFLUENCE_PUBLIC_URL`, `DIFFLUENCE_SMTP_PORT` and `DIFFLUENCE_SMTP_TLS`, so a file can set them; the values in effect are the same
- New dependency: `go.yaml.in/yaml/v3` 3.0.6 (MIT and Apache-2.0), the maintained successor of `gopkg.in/yaml.v3`
- Upgrade impact: none. An empty environment variable still means the default; it now also lets the file's value through

### Fixed: a line through a table while editing it (spec 055)
- While a table was in use, the marker that shows where a dragged row or column lands was always visible, as a line through the first row and down the first column. It now shows only during a drag. The handles of a column scrolled out of view hide again too
- Upgrade impact: none

### Changed: upgrades made stronger (spec 053)
- Contract migrations, which remove what an older release needs, now wait a day after every node runs the new release instead of 10 minutes, so a problem found the next working day can still be rolled back. Set `DIFFLUENCE_CONTRACT_GRACE` to change it
- Every migration has a time limit of a minute. One that runs longer stops and rolls back instead of holding a busy table; a migration that needs longer says so in its first lines. Indexes on large tables can now be built alongside the traffic, and a failed build is named at the next start
- During an upgrade, an older node no longer takes and fails a background job a newer node queued, and saving a section of settings on an older node keeps what a newer one stored there
- The upgrade test in CI now also writes on both releases and reads it on the other, and checks that no job failed and no event was set aside. Which releases may meet, and for how long, is in `docs/operations/version-skew.md`
- Upgrade impact: none to do. Contract migrations run a day later than before; `diffluence migrate --contract` still runs them at once
### Changed: saving no longer waits for the audit chain (spec 052)
- A save writes its audit event in the same transaction as the change, as before, but no longer links it into the hash chain there. A worker seals waiting events into the chain every second, so saves stop queueing behind each other when many people work at once (ADR-041)
- The audit log in the console shows an event at once, also before it is sealed. `diffluence audit verify` and the console's check seal what waits first, and say how many events still wait
- System health has a line "Audit log": it warns when an event waited more than a minute to be sealed, which means no worker seals, and calls for a person after fifteen minutes. Two new metrics, `diffluence_audit_unsealed_events` and `diffluence_audit_sealing_lag_seconds`, and an alert in `deploy/monitoring/`
- Upgrade impact: migration 0047 adds the table `audit_pending` and the view `audit_entries`, expand only. The previous release keeps writing straight into the chain during a rolling upgrade, and both kinds of event form one chain. Run at least one worker: without one, events wait unsealed and System health says so. If you go back to a release from before this one, events that still wait stay unsealed until you upgrade again; nothing is lost
### Added: the installation watches itself (spec 051)
- System health in the console now lists disk, database size and growth, backup, migrations, background work, mail, login and the age of the release, each with what it means and what to do. The summary repeats what needs a look, such as a backup older than 26 hours or none at all
- When the disk for files has less than 10% free, every console page says so. Below 2% free, uploads, imports and exports are refused with a clear message, while text still saves; `/readyz` stays ready and adds `"notes": ["disk_low"]`
- `diffluence doctor` runs the same checks on the command line, with the configuration and the keys, and prints a report without secrets that you can paste into a question
- Prometheus rules with objectives and alerts, and a Grafana dashboard, in `deploy/monitoring/`; every metric and everything that grows is listed in `docs/operations/monitoring.md`
- Finished jobs and sent mail are removed after 30 days, failed ones after 90, and nodes that stopped without leaving after a day
- Upgrade impact: migration 0046 adds the table `database_size_samples`, expand only. Build images with `--build-arg BUILD_DATE=$(git log -1 --format=%cs)` so the console knows the age of the release; without it, the age is unknown and never warns
### Changed: an upgrade asks for a recent backup, databases are edited apart, and people hold several roles (spec 049)
- **Upgrade impact first: an upgrade now refuses to start when no backup was noted in the last 24 hours.** Run `scripts/backup.sh` before you update. To change the age set `DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE`, such as `48h`; to upgrade without a recent backup set it to `off`. A new installation is never refused. The refusal names both ways on, and migrates nothing
- Editing a database has its own permission, "Edit databases". Everyone who could edit pages gets it in the update, also through the built-in Editor, Manager and Space admin, so nobody gains or loses anything. A role can now edit pages without changing databases, or the other way round
- A person or a group can hold several roles in a space. The Access screen shows every role they hold, adds one at a time, and takes one away at a time; taking the last one away still asks first
- A combination of permissions nobody named has a short name, such as "Custom combination 3 (12 permissions)", instead of a long list cut off at 90 characters. Open a role on the Access screen to see its permissions
- Upgrade impact: see the first line. Migration 0044 gives `database.edit` to every role that holds `page.edit` and renames combinations nobody named, expand only. Bundles are format 5; older bundles give `database.edit` with `page.edit`
### Changed: what a real server showed (spec 050)
- The start screen, the count on the bell and the search for file names ask who may see what only about the pages they could show, instead of about every page a person may see. On data of 5,000 people and 60,000 pages, the feed went from 379 to 20 ms, the bell from 55 to 6 ms, and file names from up to 1.4 s to under 80 ms. They show the same as before
- A request waits at most 10 seconds for a database connection, and is then told the server is busy, with a time to come back. Work that already holds a connection no longer asks for a second one, so many changes at once, such as creating many spaces, all finish
- `DIFFLUENCE_PUBLIC_URL` with an IP address is refused at start, with a message that asks for a host name. Passkeys need a name, so such an address never worked: the app stopped at start and restarted again and again
- The install guide gives PostgreSQL memory settings by machine size, how to turn on `pg_stat_statements`, and why writes go at the pace of the disk's sync. Known limits and a report hold the first test on a real server
- Upgrade impact: none for an installation that uses a host name. One that set an IP address as `DIFFLUENCE_PUBLIC_URL` could not start before, and now says why: use a name instead
### Security: a copy of the audit log outside the database, and a stop on password guessing (spec 046)
- The head of the audit log is signed every five minutes when something happened, instead of once a day, and each signature also goes to the application log, whatever its level. Keep that log on a log server: `diffluence audit verify --witness <log>` then shows if the newest audit rows were removed from the database
- After 100 wrong passwords in a row, password login for that account stops until the person chooses a new password or an administrator allows it again. The person gets a mail. Passkeys and single sign-on keep working, and the right password ends a run of wrong ones
- CI also runs Semgrep, CodeQL and a ZAP baseline against the real image
- Upgrade impact: migration 0042 adds two columns to `local_credentials`, expand only. Send the application log somewhere the database owner cannot change, and keep it as long as the audit log
### Added: floating bars, search that forgives a typo, and the storage limit everywhere (spec 047)
- Selected words get a small bar above them with bold, italic, link, code, colour, highlight, headings 1 to 3, lists and Comment. A selected image gets one with its tools and Delete. The row of tools stays as it is. Alt+F10 reaches the bar and Escape leaves it
- New dependency: `@tiptap/extension-bubble-menu` 3.32.2 (MIT), which was already installed with TipTap, and the Floating UI packages it uses (MIT)
- Search and the Files screen also find part of a word, such as "port" in "report", and from five letters a small typo, such as "handbok". Whole words and word starts still come first
- The storage limit of a space now counts files in the trash until they are purged, page and branch copies, pages made from a template, saving a page as a template, and moving pages to another space. A file made current again counts once. The copy and move dialogs say beforehand when the files will not fit
- Upgrade impact: **migration 0043 creates the PostgreSQL extension `pg_trgm`** and two indexes. A node refuses to migrate where `pg_trgm` is not available, and says why. Because files in the trash now count, a space may be over its limit after the update: nothing is deleted and everything keeps working, but adding files is refused until the trash is emptied or the limit raised. Check the Files screens of limited spaces before you update
### Added: checks before an upgrade, and a safe way back (spec 044)
- Before it migrates, a node checks that PostgreSQL is 16 or newer, that its data folder has 512 MiB free and that the queue answers. If not, it does not start, says every reason, and changes nothing
- Set `DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE`, such as `26h`, and an upgrade also refuses without a recent backup. `scripts/backup.sh` notes each backup it completes; without the setting, a missing backup only gives a warning
- Before migrating a database with data, a node writes `system.upgrade_started` to the audit log, naming the last backup: the one to restore if an upgrade must be undone
- A release refuses to start on a database a newer release has already contracted, and names the migration, instead of failing later on a missing column
- `/metrics` shows how far migrations are. `docs/operations/release.md` is the checklist for a release
- CI now runs the previous build and this one side by side on one database for every change, then rolls back to the previous one
- Upgrade impact: none. No migration. A restart with nothing to migrate checks nothing
### Added: a note when a newer version runs (spec 041)
- A tab that was opened before an update says so under the top bar, and offers Reload. It never reloads by itself and takes no focus, so typing goes on
- Reload first saves the draft of an open editor, so the text is simply there afterwards
- Comments now ask the page whether they are closed, instead of reading it themselves. Nothing changes for people
- Upgrade impact: none
### Security: the rest of blueprint 06 (spec 042)
- Editing together: what a client sends past the editor's rules leaves the shared draft at once, and the name in a colleague's cursor always comes from their login
- A mail goes out when a password, second factor or passkey changes, also when an administrator makes the change
- Reset and sign-up mail to one address is limited, wherever the requests come from. Turning off an authenticator app shares the limit on password tries
- A warning at start and in the admin console when the public address is plain http to others. The login cookie for an identity provider carries the `__Secure-` prefix on https
- A SCIM filter on an attribute Diffluence does not read is refused instead of ignored
- More of the readers are fuzzed every night: HTML import, archives, SCIM filters and the search language
- Upgrade impact: **Docker Compose now publishes the app on `127.0.0.1` only.** If your reverse proxy runs on another machine, set `DIFFLUENCE_BIND=0.0.0.0` before you update. No migration
### Changed: roles on the Access screen of a space (spec 043)
- The Access screen lists people and groups with one role each, chosen from a list of the built-in roles and the roles of your instance. The matrix of ticks is gone
- A combination of permissions that matches no named role shows as it is, "Custom: ...", and an administrator can give it a name without changing anyone's access
- Every change says what it gives and takes, and asks first when someone would lose the space. The settings part People and roles shows the same list
- "0 holders" on the Roles screen stays on one line
- A reply, a comment being changed and text in a settings form survive the Reload offered for a new version
- Upgrade impact: none. The routes for single permissions keep working for scripts
### Changed: blog posts and databases have their own permission (spec 045)
- Two new permissions: "Create blog posts" and "Create databases". A role can now make one kind of item without the others, such as a role that only writes blog posts
- Everyone who could create pages gets both in the update, also through the built-in Editor, Manager and Space admin, so nobody loses anything
- The Create button offers only what a person may make in the space, and makes a blog post at once for someone who may only write posts
- Turning an unpublished page into a blog post needs "Create blog posts"; copying or moving a database into a space needs "Create databases" there
- Export stays one permission
- Upgrade impact: migration 0041 gives the two new permissions to every role that creates pages. Bundles are now format 4; older bundles still import and give both with page creation

### Added: a setup screen for a new install (spec 038)
- A new install opens on a setup screen: the name of the company or person it is for, which the search box shows as "Search in" that name, the first administrator and the default time zone. Right after, it asks for a passkey
- The screen needs a one-time code from the server's log, or from `diffluence setup-code`. A code lasts an hour, works once and stops after 10 wrong tries. Once an administrator exists, the screen is gone for good
- Folded groups in the settings sidebar are kept per person, on every device
- The results page has no space list of its own any more: choose spaces in the search panel
- Settings and the administration start right beside the sidebar and stay inside the window on a laptop. A table that is too wide scrolls sideways in its own box, also with the keyboard
- Upgrade impact: one migration. An existing install has an administrator, so it never shows the setup screen

### Changed: a faster mutation check (spec 037)
- The check that breaks security rules on purpose runs several mutations at once, and the slowest ones run only the tests that catch them. A full run takes about a third of the time, and catches the same mutations with the same tests
- Builders may run only what their change can affect. Main and CI always run every mutation
- Upgrade impact: none

### Added: roles you make yourself (spec 036)
- Access is given as roles: named sets of permissions, for a space or for the whole instance. Administrators make and change them; people and groups hold them, and what they may do follows a role when it changes
- Viewer, Editor, Manager and Space admin are built-in roles with the meaning they always had, and cannot be changed or deleted
- A role in use cannot be deleted, cannot gain space administration, and changes only after a preview that says who it reaches. Every change to a role or to who holds it is audited, with before and after
- Who may give access does not change: in a space whoever manages access there, for the instance administrators, and only a space administrator makes another one
- The routes for single permissions keep working: a set becomes the role with exactly that set. Own permission sets became roles under their own names
- Four permissions of their own, taken out of space administration: publishing to the web (public sites and public links), automation (rules and webhooks), templates, and reading the audit of a space. Whoever administered a space holds them too, so a role can now give one without the rest
- Every permission has a plain name and one line on what it allows, in groups: pages, comments, files, people and access, space, administration. No key such as create_personal_space shows on a screen any more
- Administration > Roles makes and changes roles with those groups of checkboxes, and shows who holds each role and in which space. A space's settings give roles under People and roles. The list of who holds which instance permission stays, in words. The old box for named permission sets is gone: those sets are roles now
- Space settings show each part to whoever holds the permission it needs, so someone who only publishes or only runs automation still finds it
- Bundles are format 2. A bundle of format 1 still comes in, and its space administrators keep what the split took out of administering
- Upgrade impact: one migration, which gives the four new permissions to everyone who administered a space and turns every existing permission into roles, without changing what anyone may do. The application's database role can no longer write permissions itself; only the database's own role triggers can

### Security: open lines of blueprints 05 and 06 (spec 035)
- The PDF helper can ask for a password, `DIFFLUENCE_PDF_PASSWORD`, and then answers only Diffluence. In `compose.yaml` it also fetches no address, runs no script in pages, runs read-only without extra rights, and is pinned by digest
- Passwords are compared in one Unicode form, so the same password typed on two systems always works. A new password may not hold the product's name
- Characters that turn reading order around are removed from page titles, file names, names of people and names of spaces
- The database refuses to delete a parent page on its own, so a child can never lose a restriction it inherits
- `diffluence session end --email ADDR` or `--all` logs one person or everyone out, without disabling anyone. `docs/operations/incidents.md` says what to do when something went wrong
- Asking again through the identity provider now checks that the provider really asked
- Blueprints 05 and 06 show what is built, checked against the code and the tests
- Upgrade impact: one migration. To protect the PDF helper, set a long `DIFFLUENCE_PDF_PASSWORD` in `.env` and restart both. Without it, everything works as before
### Changed: a quieter look, and settings in the sidebar (spec 033, ADR-034)
- The look is calmer: fewer lines and boxes, tables as rows, labels as plain links, and no guide lines in the page tree
- The sidebar shows on the home page and the other personal screens too. Its parts fold and the space part can be made taller; both follow you to other devices
- The page tree follows the links of a space directly, starts folded from the first level, and keeps its tools and filter in a quiet row of icons
- The search box says Search in the name of your organisation, given with `diffluence user create-admin --organisation` or under Administration, Look
- The search box always searches every space. Clicking it unfolds filters for spaces, type, labels, people and date, which write the same words a query can hold, such as space:ENG; the results page shows and keeps them
- Space settings and the administration show their sections in the sidebar, in groups that fold, with a way to jump to one. Care and System health sit under System
- The People screen of a space shows pictures, guests, how someone got in, and a role that those who manage access change in the row
- The user list filters by role and by space
- A rule of the instance can act in chosen spaces only
- Upgrade impact: none. Links to space settings still work and open the first section
### Security: master.key can be replaced (spec 034)
- `diffluence key add`, `promote`, `reseal`, `check` and `retire` replace `master.key` while Diffluence keeps running. Every stored file and secret is sealed again with the new key, and the old key is removed only when a check proves nothing needs it
- `diffluence key check` proves that every stored file and secret opens, and names what does not, without showing its content. `--deep` reads every byte
- Backups take a key on its way in or out along, and a restore now ends with `key check`. The restore drill replaces the key halfway through
- The job that encrypts old files remembers where it stopped, so a store of millions of files gets through
- Upgrade impact: one migration. New secrets are sealed in a new layout that names their key; older ones still open. Nothing to do
### Changed: an order of work when the database is full (spec 031)
- Saving comes first, then reading, then search and reports. Each has its own share of the database connections, so a flood of searches or page views cannot take the room a save needs
- When a share is full and too many wait, the answer comes at once: 503 with `Retry-After`, and the screen says Diffluence is very busy. Text being written stays in the browser
- `/readyz` says `busy` when every connection is taken or a queue is long, so a load balancer can send people elsewhere. `/livez` does not change
- Each node hashes at most half as many passwords at once as it has cores, so a rush of logins leaves the rest alone
- A page view reuses the permission facts it loaded: opening a page costs 25 queries instead of 30, a space 11 instead of 36
- The main screens have a query budget that a test enforces, and `/metrics` counts queries as `diffluence_db_queries_total`
- Upgrade impact: none. A load balancer that checks `/readyz` now also takes a busy node out for a moment
### Security: stored files are encrypted (spec 032)
- Every stored file is encrypted with its own key, which `master.key` protects. A wrong key, a changed file or a file cut short is refused, and nothing of it is served
- Files from before this release are encrypted by a job in the background, a little at a time. Until it is done they are still read; after that, a plain file is refused
- Upgrade impact: one migration. Keep `master.key` safe and apart from the data: without it, no file can be read

### Changed: smaller parts of blueprint 04 (spec 032)
- The PDF of an office file is made once per version and kept, instead of on every view, and copies of the same file share it
- The feed and notifications page by the time and id of the last item, so two items with the same time are never skipped. `before` alone still works
- `GET /api/v1/system/capabilities` says which optional parts are on
- Upgrade impact: none
### Added: smaller things for the people who keep an instance tidy (spec 030)
- The lock beside a page title is open or closed, so a restricted page shows at a glance
- The spaces in the administration filter by name, pages, storage and months without a change
- The Content screen of a space filters by the day of the last change and of creation
- The user list names the spaces of each guest, and a person's screen shows their groups, spaces and how many pages they own, without titles
- The administration warns when the disk for files has less than a tenth free
- Upgrade impact: none
### Changed: word forms and file names in search (spec 029)
- A word also finds its common forms: `plans` finds `plan`, `policy` finds `policies`, in English and Dutch alike. Words in quotes are found as typed, and numbers have no other forms
- The Files screen finds a name the way the search box does: whole words and the start of a word, so `rep` finds `report.pdf` and `port` does not
- The install guide says how large the database must be: about 2.5 CPUs for 5,000 people
- A new test holds that a good login never buys extra password guesses from the same address
- Upgrade impact: none. A search may find more pages than before, because of the word forms

### Changed: search that carries the load, and a tree that loads by branch (spec 028)
- A search no longer reads every match. A common word in 50,000 pages went from seconds to tens of milliseconds, also with reading and publishing going on
- The order is new: the title as typed, every word in the title, then the rest, each with the newest first. Nothing is left out, however old
- The search box stands in the middle of the top bar
- Results go on past the total, which now says "More than" sooner. A plain word also finds labels, and a word that only a comment holds gets its own section
- A search that runs out of time says so, instead of showing nothing. When search is very busy it says that too, and reading and saving keep their connections
- In the box, files are found by the words of their name, so `port` no longer finds `report.pdf` there. The Files screen still looks inside names
- A large tree opens at once: it loads the start, the way to the open page, and branches as they open. The filter and the flat views ask the server
- Upgrade impact: one migration, about 16 seconds for 50,000 pages on a small machine, and no other step


### Added: the rest of the editor (wave Q)
- Tools that do not fit fold into More, so a narrow window no longer covers the first line of the page
- A plus on an empty line, Browse all with previews, and Turn into with the likely choices first. Panels and sections go back to text
- Copy link to any block with text. Shift and a grip select several blocks, which then move, duplicate and go together. A list item drags with what is nested in it, and dropping a block at the edge of another makes columns
- Saved blocks, private unless shared, in the `/` menu
- Images: a corner to drag, a width, text around them, a border, a link, and a small image in a line of text. The editor asks for a description and says how many are missing
- Images in pasted web pages are taken along when their bytes are in the clipboard. Otherwise a note says how many stayed behind
- Code blocks: a first line number, folded for readers, language detection, and Tab on several lines
- A panel with your own emoji, a spotlight band, a narrow sidebar split, and wider columns
- An emoji picker with groups and recent emoji
- Charts: axis and data columns, totals of repeated values, titles, legend, height, smooth lines and data points. Removing a table asks about its chart
- The language of a page and the direction of a line
- Lists from Word arrive as lists, and a block from a newer release is kept and shown as a note
- Upgrade impact: one migration. Nobody needs to do anything

### Added: the rest of dynamic blocks (wave O)
- Lists and reports filter by text, type, person, date and space. The report table now really shows its columns, and a writer chooses them
- New blocks: activity with tabs, popular pages, popular, related and recent labels, section contents, contribution table, person card, people list, space files, rotating cards, a button that starts a page from a template, a timeline, and a synced block
- Link cards can come from a filter and show a picture. The page tree opens and closes, and the index and labels list have a letter bar
- Reused text can have blanks that each page fills in, as `{{name}}`
- Settings of a block stand at the side, so the block stays in view
- Who watches a page shows in a block only to those who manage the space, and a comment for editors never counts
- Upgrade impact: one migration. Tasks that exist get their last change as their creation date

### Changed: small points of the UI test
- The public link dialog has a copy button. The administration shows types, sources and permissions in words, and the company home page is chosen by title
- A page with one version offers nothing to compare, the restore note shows a real time, and the import report counts correctly
- The title field grows with a long title
- Upgrade impact: none

### Added: links that follow their page, and tables with handles (wave M)
- A link to a page without your own words shows the current title, as text, a tag or a card. The document holds only the id, so a reader who may not see the page gets neutral words
- A link can lead to a heading or an anchor, here or on another page, and the link dialog starts with recent pages
- A selected link has its tools in the row: open, copy, edit, remove, and how it shows
- A link to a page that does not exist yet starts that page below it. Space settings list the ones nobody has started
- A new table asks for its size. The row and column in use have a handle with a menu and a plus, and can be dragged or moved from the menu
- Tables can number their rows, scale with the page, and have a caption. Text with tabs lands as cells
- A table cannot hold another table any more. On saving, a table inside a table loses its inner grid and keeps its words
- Upgrade impact: one migration
### Added: the rest of files (wave N)
- A large upload shows how far it is and can be stopped. A file dropped on a tile becomes its next version
- A preview opens on top of the page, with colours for code and next and previous. Tiles can be small or large
- The list of a page says who added a file and whether the page shows it, and offers to clear up what nothing uses
- Files can have labels, and search and the Files screen filter on them. The Files screen also filters by type and person, and space administrators can delete many files at once
- Each version of a file has its own address, which also makes old versions downloadable for the first time. The address gives no access
- The page history shows when files came and went, from this release on
- An administrator can give a space a storage limit
- Fixed: renaming a file no longer wipes its comment
- Upgrade impact: one migration, which reads all page versions once, so it takes longer on a large instance. Nothing else to do. No space gets a limit by itself
### Security: what a test against the architecture found (spec 025)
- Pasted Markdown can no longer keep the server busy for minutes. A paste is 256 KiB at most, and a block too large to read stays plain text with a warning
- Wrong passwords no longer lock a person out from everywhere: the limit counts per account and address, with a wider limit per account
- One event too large for the queue no longer stops all others
- The editing service takes messages of 16 MiB at most, and goes back to the last good state past that
- In `compose.yaml` the PDF helper and the editing service sit on an internal network, without a way to the database or the internet
- `docs/operations/limits.md` lists what was measured, and what was not
- Upgrade impact: if you added your own helper to `compose.yaml`, put it on the `helpers` network

### Added: inviting someone without an account (COL-047)
- A person with the new global permission `invite_people` can ask for an invitation from the Share dialog. An administrator approves or refuses each one under Invitations, with a reason, and the person who asked hears the answer
- The recipient gets an invitation and never the content, and becomes a guest with view on that one space
### Changed: space managers no longer invite guests by themselves
- Inviting a guest from a space now needs `invite_people` and waits for an administrator, unless you are an administrator (ADR-033)
- Upgrade impact: one migration. Nobody holds `invite_people` yet, so give it to the people or groups who invite guests today

### Changed: backups are encrypted (wave K)
- `scripts/backup.sh` and `scripts/restore.sh` now take a key file, and all of a backup is encrypted with AES-256. The script refuses to run without a key, or with the key inside the backup folder
- Upgrade impact: make a key as `docs/operations/backup-restore.md` says, and change your cron line. A backup is run with `bash`, and needs `gpg`. Older backups are restored with the older script

### Added: the rest of the chapters (wave K)
- A profile lists where a person was mentioned, their spaces, and their followers, which they can hide in Preferences
- The owner of a page hears when an administrator recovers it, and a draft gets a notification two weeks before it goes
- The console lists restricted pages nobody can open, totals per space, the largest pages, and events that could not be handled, with Try again
- After a first publish, a message offers to share the page
- Upgrade impact: one migration. Presence restarts empty, which nobody notices

### Added: watching an instance (wave K)
- `/metrics` for Prometheus, behind `DIFFLUENCE_METRICS_TOKEN`. `trace_id` in the logs from a W3C `traceparent` header
- A node with role `api` stays ready when the queue is down
- An event that keeps failing stops after six tries instead of coming back for ever
- A worker runs several jobs at once, so a large export no longer holds up a small copy
- Pictures and files are no longer downloaded again on each view, while access is still checked each time
- Reading a page writes less to the database. The pool holds up to 20 connections unless the address says otherwise
- Upgrade impact: nothing to do. Check that all nodes together stay below `max_connections`
### Added: the rest of daily writing, reader tables and moving files (wave J)

- A new page starts in the title. Enter moves to the text, and Backspace at the very start moves back.
- A click below the text goes on writing there. Word count, at the foot of the sheet, also counts a selection.
- Arrows, an ellipsis, curly quotes and smileys come from what people type anyway. A preference turns it off, and Backspace right after it brings back what was typed, also after a heading or list shortcut.
- The `/` menu has group headings, and what a person used lately comes first.
- Pasted Markdown turns into blocks, and Undo brings back the plain text. Ctrl Shift V keeps it plain.
- Lists change style by level, a list changes kind without losing its nesting, Find has Whole word, and the browser's spell checker is on.
- Readers can sort and filter a table for themselves, and a long table keeps its header in view. Nothing in the page changes.
- A link to a page the reader cannot reach is struck through and says why. A hidden page and a deleted page look the same.
- A file on the page can be shown again without a new upload. A tile shows the type and size, and a missing file or image says so.
- A file moves or copies to another page, and its tiles keep working. The list of a page sorts and filters.
- Administrators can block file types, by name and by content.
- With a helper connected, office files open as a PDF. A saved drawing can be changed.

**Upgrade impact:** no migration. To use Collabora for office files, set `DIFFLUENCE_COLLABORA_URL`. Without it, the LibreOffice of the PDF helper does the work.

### Security

- Moving a file needs delete on the source and add on the target, and from then on the rules of the new page decide who gets it.
- A blocked type looks at what a file contains too, and a rename cannot get around it.
- The office helper gets the content and the extension of a file, not its name. What comes back is a download with the same locked headers as every file.

### Added: databases (wave I)
- A database is a fourth kind of item, beside pages, blog posts and folders. It works like a simple spreadsheet: type, move with the arrows and Tab, paste from Excel, sums such as `=SUM(B1:B3)`, and undo
- Columns hold text, numbers, dates, ticks, choices or links. Readers can sort and filter. Permissions, history, search, trash and copying work as for a page
- CSV in and out, and Open in GridCraft for heavy work, for those who may export. Spreadsheet files on a page open in GridCraft too
- Upgrade impact: one migration that only widens what is allowed. Nothing to do

### Added: PDF in the browser
- A PDF opens in PdfCraft, from the file in the page, the Files list or its details, so people can look at it and change it there. The result is saved as a download. Download always stays next to it, because a screen reader cannot read the viewer
- Upgrade impact: the program grows by about 28 MiB. Nothing to do. The viewers run in a sandboxed frame with their own policy, and the rest of the app stays as strict as it was

### Added: insight, automation and the platform (wave H)
- Analytics with their own switch, off by default. Counts mode keeps nothing that points at a person once a day has closed. Name mode says what it does, and leaving it deletes the names for good. Your own views do not count, and page views stay out of the audit log
- A view count in the byline for those who may see it, insights for a page, a space and the instance, what people search for, most viewed pages on the overview, and sortable tables with a safe CSV
- Rules per space and for the instance: when, if, then. They run as their account through the same checks as a person, with a test run, a run log, ready-made rules, export and import, limits, and every action in the audit log with the rule's name
- Webhooks in the Standard Webhooks format, with a delivery log, five attempts over a day, and rotation with two signatures. Nothing leaves until an administrator allows a host
- The API has an OpenAPI description and a stability policy. Tokens gain the scopes read, write, export and admin, and a limit per minute
- `diffluence client` lists, exports, imports and backs up content with a token
- Who else is on a page, and a line in the editor when someone else is editing. Both can be turned off, by a person and for the instance
- Editing together, with the optional `collab` service: one shared draft per page, cursors with names, an undo per person, work kept offline, and versions that name everyone who took part. Without the service, every page keeps private drafts

### Security
- Outgoing calls go through one client: allowed hosts only, https, no private address at connect time, no redirects, 10 seconds and 64 KiB. Every CSV goes through one guard, which now also covers line feeds and full-width characters. The use of a token is in the audit log at most once a minute, and so is a refused scope

Upgrade impact: migration 0018 adds tables and a column only, and gives View analytics to everyone who administers a space. A token that could write can still export, and now needs the admin scope for routes under `/api/v1/admin/`. New optional settings `DIFFLUENCE_OUTWARD_ALLOW_PRIVATE`, `DIFFLUENCE_COLLAB_URL` and `DIFFLUENCE_COLLAB_TOKEN`. Installing the service adds a Node.js container, and on those pages a draft is no longer private.

### Added: everything can leave and come in (wave G)
- One Export dialog on pages and spaces: PDF (with the optional Gotenberg helper), Word, OpenDocument, an HTML website that works offline, Markdown and plain text. Choose pages by hand. Files are made in the background, you get a notification, and only you can fetch the file for 24 hours
- An administrator can turn formats off, set sizes, and see what a helper needs
- Every import from the screen starts with a report and changes nothing until you say so. It can be undone in one action, and never removes someone's edit
- Markdown links, wiki links and front matter now carry over. Word files become pages, several at once wait under a restricted page, and one can go into a draft. HTML comes in through the allow-list
- A signed bundle moves a space to another instance with its history, comments, labels and permissions. One changed byte stops it, and a restriction that cannot be matched keeps a page closed
- Public sites and public links, as plain files for your own web server, off until an administrator turns them on. A preview comes first, and a page the world may no longer see leaves at once

**Upgrade impact:** migration 0017 adds tables and one column, with no downtime. To use PDF, start the `pdf` profile and set `DIFFLUENCE_PDF_URL`. To use public sites, set `DIFFLUENCE_PUBLISH_DIR` and turn them on under Administration, Export and publishing.

### Added: blocks and files (wave E)
- Blocks that fill themselves: contents, child pages, page tree, page index, spaces, recent changes, your favourites, drafts and tasks, page lists and reports, labels, facts, decisions, tasks, contributors, history, files, blog posts, link cards and a search box. Each reader gets their own answer
- Page facts with a report that sorts by real value, excerpts, showing text or a whole page from elsewhere, and anchors. A page that shows itself stops with a message, and text goes at most five levels deep. Details and Delete name the pages that show a page's text
- Charts from a table in the document, in four kinds, with the numbers one press away. Video and audio play in the page
- A Files screen per space, with storage for administrators. File versions, rename, a text preview and a zip of all files. The editor asks before it adds a version under a known name
- Drawing through an optional draw.io service on an address the administrator sets

### Security
- media-src 'self' joins the policy, and only a video or audio file that really is one plays. rame-src appears only with a drawing service, and that service must be on another origin. Its messages are checked on window and origin

Upgrade impact: migration 0016 adds tables and columns only. New optional setting DIFFLUENCE_DRAWING_URL. PDF stays a download.

### Added: people, access and administration (wave F)
- People and a profile for everyone, with a picture you crop by mouse or keyboard, and a choice of who sees what. Names lead to a small card. Guests and former colleagues are marked
- A forgotten password, invitations and self sign-up, by a link that works once. New passwords need 15 characters and are checked against 5 million leaked ones. Existing passwords stay valid
- A person with only a password is asked for a passkey or second factor. It can be put off, not switched off. An administrator can require a second factor, and an idle timeout with a warning
- Personal tokens for scripts, which never reach account security or the console. Sensitive actions ask for proof again after 10 minutes
- Personal spaces, a Manager role, who has access and how, a summary before someone loses access, and a safe CSV of it
- Restrictions in three clear states. Group owners manage members outside the console
- Dates follow your time zone and format. Your account has sections, and you can download your data or ask for removal
- The console opens on a summary with what needs attention, and warns while no identity provider or passkey is set up. It finds settings by name, filters users and audit through the address, and has bulk actions, import, service accounts, jobs, reports and unused files
- The look of an instance: name, logo, an accent for light and dark, login text and a banner. Maintenance mode stops every write and holds jobs
- Anonymising an account, with a reason, and `diffluence audit prune` with a signed anchor

### Security
- A passkey now needs a PIN or fingerprint on the device, at login and when it is added. A key without one no longer works
- Adding or removing a passkey and turning off a factor ask for proof again. After removing one, Diffluence offers to log you out elsewhere, and an administrator's reset does that by itself
- A failed login no longer stores the address that was typed, which could hold a password by accident. Old rows stay until their retention ends
- The signatures on the audit log are now checked: by the daily job, by Verify and by `diffluence audit verify`, which `restore.sh` runs. A failure shows in the console
- New recovery codes hold 128 bits. Codes people already have keep working until they set up again
- SCIM tokens end after a year or on a date you choose, the console warns a month ahead, and Rotate gives a successor that overlaps for 7 days. Existing tokens get a year from the upgrade
- An administrator sees a page they cannot read in the audit log by its id, with the words (Censored title), on the screen and in exports. The id leads to a note on the page, without its title
- Administrators need a second factor 14 days after a new instance began. An operator can choose another rule in Rules
- What identifies a person in an audit event now sits apart from the chain, so anonymising erases it and the chain still holds. Events written before this stay as they are
- The audit log stays 12 months by default. Longer needs a stated reason
- Anonymising also removes follows, watches, what was seen, notifications, queued mail, favourites, recent pages and drafts

Upgrade impact: migration 0010 adds columns and tables, and gives members the right to a personal space. Expand only, so the previous release keeps working. New passwords are held to the new rules from now on. API tokens and the screens above need no setup. Reset links and invitations go by mail once `DIFFLUENCE_SMTP_HOST` is set. An instance older than 14 days asks its administrators for a second factor at their next login.

### Changed: the editor after the first test
- A table has one button on its outline, with one menu for rows, columns, cells, colours and width. The row of tools no longer changes in a table
- Tables have a clear outline and a soft grid
- Tables stretch up to the full width of the window, by dragging their end edge, from the menu or with the arrow keys
- A page scales with the window by itself, and Full width is a button in the row of tools
- Add emoji, Add header image and Page options sit at the title. Page look is gone. The header image can be dragged into place
- Code blocks know about 190 languages, with a search. They load when needed
- The tree stays beside the editor
- Leaving the editor saves first, and when that fails a message brings the text back
- Opening a page and clicking in it no longer leaves a draft behind when nothing was typed
- A long menu that opens upwards stops below the bars, so its first items can be reached

Upgrade impact: no database change. Documents can hold a new table width, so while two versions run side by side, a node on the older version drops it.

### Changed: animals, and changes you can see
- Every space has a cartoon animal, picked from its key. Pick another in Space settings, or use a character of your own
- A comparison now looks like the page: added on green, removed on red and struck through, with the word beside it
- Opening a version in History shows what changed in it, with a button for the clean page
- Fields for people show them as soon as you click in them, and work with the arrow keys

Upgrade impact: no database change. Existing spaces get an animal at once, unless they chose a character.
### Added: organising (wave D)
- A page has an owner, a status, a Verified mark and a review date. Handing a page over gives no right to delete it
- Labels at the bottom of a page, with their own screens. A space can keep to a fixed list
- Templates: a gallery, fields that a form fills in, instruction text for the writer, ten starters, and templates for a whole space
- Blog posts, folders, link entries in the tree, and a company home page
- Publish options: location, status, silent update, a schedule and a preview
- Archiving: a page leaves the tree, keeps its address and can come back
- Approvals: ask people to approve a page, and a space can require it
- Moves and copies between spaces say what changes first. Several rows move together, and a row can be dragged to a space
- The Content screen filters the pages of a space and works on many at once, with a summary first and a report after
- Space settings: key, owners, home page, features, statuses, labels, picture
- Search understands quotes, AND, OR, NOT, brackets, wildcards and fields such as `label:hr`, with filters in the address
- Text stays in the browser when saving fails, and Close waits for the save
- An administrator can remove one version with a reason
- The trash can be searched, sorted and worked off together. It no longer names what is hidden from the person, and such an item cannot be purged by hand
- A copy that would lose a restriction from a page above it now needs the right to manage restrictions
- Comments can be searched, and a result links to the comment. A thread for editors and a comment in the trash stay out
- Removing a version also takes its words out of comments that quote them
- A page that moves to another space takes its comments, tasks, notifications and requests along
- The example space shows labels, a status, a folder, a blog post and a template

Upgrade impact: migration 0008 adds columns, tables, `perm_listed_pages` and `perm_visible_comments`, and replaces `perm_visible_pages`. It copies `spaces.features` into `space_features`, and that column stays until a contract migration. A change to those switches on the previous release during an upgrade is not carried over. Expand only. While both releases run, the old one shows posts in the tree, and nobody can make a post on it.

### Added: working together (wave C)
- Comments under a page, with replies, edits, removal with Undo, and a link to each comment. A draft stays in your browser
- Comments on chosen words or on an image. The words stay marked while the page changes around them, and a thread says so when they are gone. Threads can be resolved and reopened, and a panel beside the page lists them all
- Reactions on pages and comments, with who reacted
- `@` mentions in pages and comments, with a warning for someone who cannot see the page
- Tasks can have a person and a date, and Your tasks lists them, grouped by date. Ticking a task there also ticks it on the page
- Share tells people about a page and says who can see it. It never gives access
- Asking for access, and asking to edit. The answer says what will change. A space chooses whether hidden pages offer it
- Watching a page, a branch or a space, with a list to stop each. Publish without notifying for a small fix
- The bell and a notifications screen, which group, link to the exact spot, and follow what you may see at that moment
- Mail through any SMTP server: at once or as a digest, quiet hours, a pause, and one-click stop. A mail about something you watch has a link to stop watching. For testing, `docs/development.md` explains Mailpit
- The start screen has For you, Recent activity and Popular. You can follow someone from the Details panel
- New keys: `g` `n`, `w`, `m`, `n` and `p`

Upgrade impact: migration 0007 adds tables and columns. Expand only, so the previous release keeps working. Mail stays off until `DIFFLUENCE_SMTP_HOST` is set.

### Fixed: what stays behind
- Recent searches and the state of the tree are kept per person, and logging out removes them. Older ones without a person are removed once
- A token in an address no longer reaches the request log

### Added: the home of a space, and an example space
- The home page is no longer a row of the tree. It has a fixed Home row above it, which is marked while you are there, and the breadcrumbs say Home
- A strip at the top of the sidebar moves you between spaces. It scrolls on its own, and marks where you are
- A new instance starts with an example space that shows everything a page can do, also comments, reactions, mentions and tasks. An administrator can add it later in Administration, Settings

Upgrade impact: no database change. An instance that already has spaces gets no example by itself. To start a new instance empty, set `DIFFLUENCE_EXAMPLE_SPACE=false`.

### Added: reading and getting around (wave B)
- The search box finds pages, spaces, files and people while you type, works with the arrow keys, remembers recent searches, and can stay inside a space
- The top bar gains Recent and Favourites menus, a help menu, and a small menu on Create
- All spaces has its own screen with a filter, sorting and archived spaces. Spaces can get a star
- Recent has tabs for visited pages, pages worked on and drafts. Favourites can be reordered and removed together
- The tree has a filter, three flat views, a menu on each row, a card on hover, and keys: arrows, Home, End and letters. Rows show the emoji, a lock and a draft mark
- The sidebar can be made wider, has an Overview and shortcuts chosen by space administrators
- A page shows its read time, a list of headings beside it, links to each heading, its child pages, and a Details panel with contributors and links in and out
- Page look in the editor: emoji, full width, title position and a header image
- Images open large. Reader mode, presentation and print are there
- Deleting names what it touches, and Undo takes it back
- Preferences: light, dark or system, shortcuts, contents, cards. They follow you to other devices
- Keyboard shortcuts everywhere, with `?` for the list

Upgrade impact: migration 0006 adds columns and tables, and fills the list of links between pages once. Expand only, so the previous release keeps working.

### Added: the editor grows up (wave A)
- Type `/` to add anything at the cursor, or use Insert in the tools
- Every block has a grip to drag it and a menu: move, duplicate, delete, turn into another block. Alt with the arrows moves it
- Headings 1 to 6, subscript, superscript, 12 text colours, 6 highlights, clear formatting, alignment and indent
- Panels in five kinds, collapsible sections, 2 to 5 columns, decisions
- Status marks, dates and emoji in a line of text. Type `//` for today, and `:` and a word for an emoji
- The link dialog finds pages, and such a link survives a rename
- Images get a size, alignment, a caption and alt text
- Tables get merged cells, header rows and columns, cell colours, column widths by dragging, and sorting
- Code blocks get a language list, line numbers, wrapping and a title, and readers get a Copy button
- Find and replace with Ctrl F
- The row of tools is now one Tab stop, with arrow keys. Menus follow Home, End and letters

### Fixed
- Comparing versions no longer glues list items together

Upgrade impact: no database change. Documents gain new blocks, so while two versions run side by side, a node on the older version refuses them.

### Changed: files live in the text
- Drag files into the editor, anywhere on or beside the page, or use Add image or file. Several at once land in order
- A file shows in the text as a tile that downloads it. A page has no Upload button or Attachments block any more
- More, Attachments lists every file of a page
- Tiles survive copying a page, and export and import as Markdown
- Search finds the name of a file in a page, and comparing versions shows when a file came or went

### Fixed
- A page with little text was shown in a narrow box instead of the full reading column

### Added: the functional description
- `docs/blueprint/03-functional-requirements.md` is now a full description of the product in 13 chapters and 986 numbered requirements

Upgrade impact: no database change. Documents gain a new block, so while two versions run side by side, a node on the older version refuses a document with a file tile.

### Added: the design
- A new look: a calm white and blue design with a top bar, a sidebar per space and one reading column, in light and dark
- A new editor: a strip with Close, the save status, the summary, Discard draft and Publish, one row of tools, and an open sheet with a large title
- A page tree with visible branches. A branch can be collapsed and stays collapsed while you read on. The sidebar can be put away
- A start screen with your spaces, recent pages and favourites, a login card, and a vertical menu in the admin console
- Our own logo and icons, a More menu for less used page actions, and an account menu
- Our own confirmation and link dialogs. The browser's dialogs are gone
- The page shows who wrote it, when it was updated and which version it is
- Diffluence now uses the system font. An administrator can import a font under Administration, Settings
- The design standard in `docs/design/README.md`, enforced by `npm run design` and by approved pictures of every screen

### Fixed
- `user create-admin` failed with "permission denied" while a new server was still migrating
- Many nodes starting at once could fail while granting database rights

Upgrade impact: none. New routes only, no database change.

### Added: phases 1 and 2
- Spaces, a page tree with drag and drop, stable page addresses, breadcrumbs, favourites and recently viewed pages
- A visual editor with Markdown shortcuts, tables, task lists, highlighted code blocks and images. Autosaved private drafts, publishing with conflict protection
- Version history: view, compare with word-level differences, restore
- Attachments with versions. Uploaded files are never trusted: only real images display inline
- Full-text search over titles, content and attachment names, with filters
- A trash with one retention period, set in the admin console
- Markdown import and export for a page, a page tree or a whole space
- The full permission model: global, space and page layers, restrictions, roles and quick setups, groups, guests, "why can or can't they?", recover access for administrators
- Authenticator apps with recovery codes, passkeys, OIDC login, SCIM 2.0 provisioning
- An admin console: users, groups, spaces, audit log, settings, provisioning, system health
- Background jobs: cleanup, trash purge, daily audit verification, daily signed audit checkpoints
- Automatic contract migrations through a node registry: no manual upgrade step is left
- Separate database roles: the application cannot change or delete audit events
- Docker image with the frontend inside (44 MB, non-root, read-only), LXC install script, backup and restore scripts with a drill
- Translation framework (English)

### Added: walking skeleton
- Single `diffluence` binary with roles `all`, `api` and `worker`
- Configuration from environment variables, with `_FILE` support for secrets
- Migration runner: numbered, checksummed, expand/contract phases, safe with many nodes starting at once
- Tamper-evident audit log: append-only (enforced by the database), hash chain, verification, filtering and CSV export
- Transactional outbox, published by the worker to NATS JetStream (embedded for single-node installs)
- Local accounts with Argon2id passwords, sessions stored as hashes, idle and absolute timeouts
- Login rate limiting, cross-origin (CSRF) protection, strict security headers
- Health endpoints `/livez` and `/readyz` with graceful shutdown

Upgrade impact: none. Tested from the walking skeleton to this build with traffic: 507 requests, none failed.
