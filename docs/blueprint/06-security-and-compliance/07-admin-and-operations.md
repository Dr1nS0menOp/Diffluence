# 06.7: The admin console and running the product

## 7.1 The admin console

**What can go wrong:** a stolen admin session does everything, an administrator reads content quietly, a setting is changed without a trace.

- **SEC-600** · built · Every admin route needs a global permission, and the router shows that in the route list.
- **SEC-601** · built · Administrators see metadata, not content. To read, they use Recover access, with a reason, and that is audited.
- **SEC-602** · built · *System administrator* is separate, and only one can grant it.
- **SEC-603** · built · Settings changes are audited with what changed.
- **SEC-604** · built · Reading and exporting the audit log is itself audited.
- **SEC-605** · wave F · Heavy actions need a recent proof of identity, through the `Sensitive` route wrapper: granting *administrator*, resets, anonymising, recover access, SCIM and service tokens, maintenance mode, audit retention and the rule for second factors (ASVS 7.5.1, 7.5.3).
- **SEC-606** · new · **Medium.** Titles and filenames are in the details of audit events, and the audit screen shows them to administrators. So an administrator can read the title of a restricted page, and "administrators do not read content by default" holds for text but not for titles. Spec 013 hides titles in the audit of a space, and the instance's audit screen still shows them. Decision for the product owner.
- **SEC-607** · built · There is no way to log in as someone else, and none is planned. Nobody can act as a person without leaving their own name.
- **SEC-608** · new · The console has no second layer of its own (ASVS 8.4.2, Level 3). Requiring a second factor for administrators is that layer (SEC-164). An address filter for the console can come later. Low.
- **SEC-609** · built · The command line runs on the host, which is full trust. Passwords are read from standard input, and never from an argument, so they stay out of the process list.

## 7.2 How it is deployed

**What can go wrong:** the app can change its own audit log, a container escapes, a secret is in the wrong place, debug tools are open.

- **SEC-615** · built · The image is distroless, runs as non-root, and holds no shell or package manager. The reference Compose file adds a read-only root file system, no capabilities and `no-new-privileges`.
- **SEC-616** · built · The systemd unit locks the process down: no new privileges, a read-only system, private temp and devices, no capabilities, and memory that cannot be both writable and executable.
- **SEC-617** · built · The application connects to PostgreSQL as a role that does not own the tables, so it cannot switch off the audit log's protection. Migrations use the owner. Both reference installs do this.
- **SEC-618** · partly · A single role still works, when `DIFFLUENCE_MIGRATE_DATABASE_URL` is empty. The audit log's triggers then protect only against mistakes. The console should show a warning. Low. Belongs in wave F.
- **SEC-619** · built · Secrets come from the environment or from `_FILE`, for Docker and Kubernetes secrets. `setup-secrets.sh` makes random ones, in a folder only the owner can open.
- **SEC-620** · built · There are no default passwords in the reference install. `.env.example` uses `change-me` as a visible placeholder.
- **SEC-621** · built · No debug routes, no profiling on the public port, no directory listings, no source control data in the image (ASVS 13.4.1 to 13.4.3).
- **SEC-622** · built · `/livez` and `/readyz` are public by design, and say no more than a short reason. The version number needs a login (ASVS 13.4.5, 13.4.6).
- **SEC-623** · built · The embedded queue has no network listener at all.
- **SEC-624** · phase 3 · An external NATS needs TLS and its own account, and so does anything else between nodes (ASVS 12.3.3).
- **SEC-625** · partly · The reference install talks to PostgreSQL with `sslmode=disable`, on a private Docker network or the local machine. A database on another machine needs `verify-full`, and the guide must say so (ASVS 12.3.1, 12.3.2).
- **SEC-626** · built · TLS ends at a reverse proxy, which the guide requires. The guide should give settings: TLS 1.2 or newer, and `X-Forwarded-For` set by the proxy only (ASVS 12.1.1, 4.1.3).
- **SEC-627** · new · `DIFFLUENCE_PORT` is published on all addresses in the reference Compose file, in plain http. A proxy on the same machine only needs `127.0.0.1`. Change the default. Low.
- **SEC-628** · built · A failure in the session lookup answers 503. It does not continue as if nobody were logged in, so errors fail closed (ASVS 16.5.3).

## 7.3 Backups

**What can go wrong:** a backup is a full copy of everything, outside all our rules, and it often holds the key as well.

- **SEC-635** · built · `scripts/backup.sh` runs with `umask 077`, writes checksums, and a restore drill exists. Restore verifies the audit chain.
- **SEC-636** · new · **High.** Backups are not encrypted, while this blueprint promised AES-256 before they leave the host. The script also copies `master.key` into the same folder as the database and files, and only prints a reminder. Whoever gets that folder gets everything, including TOTP seeds. Encrypt the backup with a key the operator holds, and write the key file somewhere else by default. Belongs in phase 3, and earlier for the script.
- **SEC-637** · built · The guide says to keep `master.key` and the secrets apart from the rest.
- **SEC-638** · wave F · After a restore, anonymising must be done again, which the operations guide will say.
- **SEC-639** · phase 3 · An automatic restore test, and RPO and RTO of at most 4 hours.

## 7.4 Upgrades and availability

- **SEC-645** · built · Migrations are expand and contract, and a contract only runs when every node knows it, so an upgrade cannot leave a node in a broken state.
- **SEC-646** · built · A migration file that changed after it ran stops the start, because its checksum differs.
- **SEC-647** · phase 3 · A node can be killed under load, rate limits are shared, and a load test measures the heavy functions.
- **SEC-648** · new · Security updates need a fast path: an operator must learn about them. The start screen for administrators could show that a newer release exists, but a call home conflicts with deliberate difference 5. A release feed an operator subscribes to is enough. Decision for later.

## 7.5 Noticing an attack

**What can go wrong:** the logs are perfect, and nobody reads them.

- **SEC-655** · partly · A daily job verifies the audit chain and writes the result as an audit event. A failure goes to the log, not to a person. Show it in the console, and notify administrators, once wave C has notifications. Medium.
- **SEC-656** · new · Alerts for what matters: many failed logins, a burst of refusals, a new administrator, a recovered access, a failed chain check. Metrics belong in phase 3, and events can go to a SIEM (SEC-832).
- **SEC-657** · new · An operator's runbook for an incident: end sessions, rotate keys, read the audit log, restore, tell people. Belongs in `docs/operations/` before 1.0.
