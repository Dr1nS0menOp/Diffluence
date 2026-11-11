# Installing Diffluence

Written for: operators who know Docker or Linux. Tested with PostgreSQL 16.

**PostgreSQL needs the `pg_trgm` extension** (spec 047), which ships with PostgreSQL in its contrib modules and in the official `postgres` images. The role that migrates creates it, which needs the right to create objects in the database. A managed database must allow `pg_trgm`. Without it, a node refuses to migrate, says why, and changes nothing.

## Docker Compose (the reference installation)

Needs Docker with the Compose plugin.

```sh
git clone https://github.com/Dr1nS0menOp/Diffluence.git && cd Diffluence
sh scripts/setup-secrets.sh                 # creates ./secrets, never overwrites
export DIFFLUENCE_PUBLIC_URL=https://wiki.example.com
docker compose up -d --build --wait
docker compose logs diffluence | grep 'first setup'
```

Open the address in that log line, `/setup`, and enter its code. The setup screen makes the first administrator, takes the name of the company or person the instance is for, and sets the default time zone. Then it asks for a passkey.

**The setup screen is guarded.** It works only while no administrator exists, and only with the one-time code in the server's log, which only someone who runs the server can read. A code lasts an hour, works once, and stops after 10 wrong tries; one address gets 10 tries in 15 minutes. Every try is audited, never with the code. `diffluence setup-code` prints a new code and makes older ones useless. Once an administrator exists, the screen and its routes answer as if they do not exist, also when that administrator is later removed: the way back is the command line. Do not leave a new install reachable from the internet before it is set up.

The command line does the same without the screen:

```sh
printf '%s\n' 'a long password' | docker compose exec -T diffluence \
  diffluence user create-admin --email you@example.com --name "You" --organisation "Your team"
```

`DIFFLUENCE_PUBLIC_URL` must be the address people type in their browser. It drives secure cookies, cross-site protection, passkeys and the OIDC redirect. It is also what decides whether the session cookie gets the `__Host-` prefix, so **https is required in production**.

**It must use a host name, not an IP address.** Passkeys are bound to a name, and browsers do not accept an address for that, so Diffluence refuses to start with an address and says so. On the machine itself, use `http://localhost:8080`. A test machine on a network without DNS can use a name from the hosts file of each computer that opens it, such as `http://wiki.test:8080`.

Put a TLS reverse proxy in front (Traefik, nginx). The app speaks plain http, so Compose publishes its port on `127.0.0.1` only, where a proxy on the same machine reaches it. If the proxy sits on another machine or network, set `DIFFLUENCE_BIND=0.0.0.0` (or the address of one network card), and set `DIFFLUENCE_TRUSTED_PROXIES` to the proxy's CIDR range, or every user appears to come from the proxy, and the login rate limit counts them all as one. When the public address starts with `http` on any host but this machine, the log at start and the admin console say so.

A new instance gets an example space within seconds of its first administrator: it shows everything a page can do, and can be deleted like any other space. To start empty, set `DIFFLUENCE_EXAMPLE_SPACE=false`. An administrator can add it later under Administration, Settings.

Outgoing calls only reach public addresses over https, and only hosts an administrator allowed. If your outside systems live on your own network, set `DIFFLUENCE_OUTWARD_ALLOW_PRIVATE=true`: the list of hosts still applies, but private addresses and plain http then work, so only allow hosts you trust.

Drawing is optional. Run the jgraph/drawio image on its own address, and set DIFFLUENCE_DRAWING_URL to it, such as https://draw.example.com. It must be another origin than DIFFLUENCE_PUBLIC_URL, and the app refuses to start otherwise. Only that address may be framed. Serve it over HTTPS from the same kind of network as the wiki: a browser can block a public page from framing a private address.

What you get: the container runs as non-root on a read-only filesystem with no capabilities, and the application connects with a database role that does not own the tables, so it cannot change or delete audit events.

## LXC on Proxmox, or bare Linux

Copy `deploy/` and a built `diffluence` binary into the container, then:

```sh
sh deploy/install-lxc.sh https://wiki.example.com
```

It installs PostgreSQL, creates the two database roles, writes `/etc/diffluence/diffluence.env` and starts a sandboxed systemd service on `127.0.0.1:8080`. Status: written to the same design as the Compose setup, but **not yet run on a real LXC container**. Try it on a scratch container first.

## Packages (.deb and .rpm)

Each release has `.deb` and `.rpm` packages for amd64 and arm64, beside the archives (spec 057). Check them first (see "Checking what you downloaded" below), then:

```sh
sudo apt install ./diffluence_1.2.3_amd64.deb      # Debian, Ubuntu
sudo dnf install ./diffluence_1.2.3_amd64.rpm      # Fedora, RHEL, Rocky, Alma
```

The package puts the binary in `/usr/bin/diffluence` and the sandboxed systemd unit in `/usr/lib/systemd/system/`. The unit sets `DIFFLUENCE_DATA_DIR=/var/lib/diffluence`, the only folder the sandbox lets it write, and `DIFFLUENCE_HTTP_ADDR=127.0.0.1:8080` for a reverse proxy on the same machine. It creates the system user `diffluence`, `/var/lib/diffluence` (0700) and `/etc/diffluence` (0750, `root:diffluence`). It does **not** start the service, because it has no database yet:

1. Create the database and its two roles, as `deploy/install-lxc.sh` does, on this machine or elsewhere.
2. Write `/etc/diffluence/diffluence.env` with `DIFFLUENCE_PUBLIC_URL`, `DIFFLUENCE_DATABASE_URL` and `DIFFLUENCE_MIGRATE_DATABASE_URL`, or with only `DIFFLUENCE_CONFIG_FILE=/etc/diffluence/diffluence.yaml` and the settings in that file (`/usr/share/doc/diffluence/diffluence.example.yaml`). Make it `chmod 640` and `chgrp diffluence`.
3. `sudo -u diffluence sh -c 'set -a; . /etc/diffluence/diffluence.env; diffluence config check'`, then `sudo systemctl enable --now diffluence`.

An upgrade is the next package: `apt install ./diffluence_1.2.4_amd64.deb`. It restarts a service that runs, and the checks before an upgrade then decide whether it may migrate (spec 054). Removing the package stops the service but keeps `/var/lib/diffluence`, `/etc/diffluence` and the user: a wiki is never removed with its package. Remove them by hand when you mean to. There is no `apt` or `dnf` repository yet; a package is a file of the release.

## Checking what you downloaded

Every file of a release is listed in `checksums.txt`. That file is signed by the release workflow of this repository with Sigstore, without a key the project could lose (spec 057). With [cosign](https://docs.sigstore.dev/cosign/system_config/installation/):

```sh
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity "https://github.com/Dr1nS0menOp/Diffluence/.github/workflows/release.yml@refs/tags/v1.2.3"
sha256sum --ignore-missing -c checksums.txt
```

The first command says `Verified OK` only for a signature made by that workflow on that tag. The second then checks each file you downloaded against the list.

GitHub also records where each file was built, as SLSA provenance. With the GitHub CLI: `gh attestation verify diffluence_1.2.3_amd64.deb --repo Dr1nS0menOp/Diffluence`. The image is signed and attested the same way:

```sh
cosign verify ghcr.io/dr1ns0menop/diffluence:v1.2.3 \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity "https://github.com/Dr1nS0menOp/Diffluence/.github/workflows/release.yml@refs/tags/v1.2.3"
```

Each release also lists its parts as an SBOM, in SPDX and CycloneDX: one per binary (the Go modules), one for the source (the Go modules and the npm packages of the web app and the editing service), and one per image.

## Settings

Every setting is in `.env.example`. Any variable can come from a file by adding `_FILE`, for Docker and Kubernetes secrets. Runtime options (trash retention, upload limit) are in the admin console.

**Every installation sets** `DIFFLUENCE_PUBLIC_URL`, the database addresses, and the mail server if mail should go out. The other settings have defaults that fit most installations; change one only for a reason you can name.

### A configuration file

Settings can also live in one YAML file. Set `DIFFLUENCE_CONFIG_FILE` to its path; `deploy/diffluence.example.yaml` is a commented example.

- **Keys:** each key is the name of a variable in lower case, without `DIFFLUENCE_`. A group may nest them: `smtp: {host: mail.example.com}` is `smtp_host`.
- **Order:** the built-in default, then the file, then the environment. A variable that is set and not empty wins over the file, so a container platform can still override any value.
- **Strict:** an unknown key, a key written twice, and a value of the wrong kind refuse the start and name the line. A true/false setting takes `true` or `false`, never `yes`, `no`, `on` or `off`, and a number is read as written, never as octal.
- **Secrets:** give a path with a `_file` key (`smtp: {password_file: /run/secrets/smtp}`), so the file holds none. A secret written in the file itself is only accepted when the file can be read by its owner alone (`chmod 600`), and that owner must be the user Diffluence runs as.

With Docker Compose, put the file next to `compose.yaml`, uncomment its line under `volumes:` and set `DIFFLUENCE_CONFIG_FILE=/etc/diffluence/diffluence.yaml` in `.env`.

### Checking a change before a restart

```
docker compose run --rm --no-deps -T diffluence config check
```

It reads the environment and the file as a start would, starts nothing, and prints every setting with the value in effect and where it came from (default, the file and its line, or the environment), with each secret shown as `(set)`. It exits 1 when a start would be refused. It then tries the settings that are otherwise only used later: whether the mail server's name resolves and its port answers, whether the identity provider's discovery document loads, and whether the publish folder can be written. Those are warnings, because the other system may be down for its own reasons; `--offline` skips them. A start tries them too and logs a warning, and `diffluence doctor` prints them.

Identity providers: see `identity-providers.md`.

## The database connection

The reference install reaches PostgreSQL with `sslmode=disable`, on a private Docker network or on the same machine, where nothing passes over a wire. **A database on another machine needs `sslmode=verify-full`** in both database addresses, with the server's CA certificate (`sslrootcert=`), so the connection is encrypted and the server is the one you meant (SEC-625).

## The reverse proxy

TLS ends at your proxy, so its settings decide how strong it is. Offer TLS 1.3, and TLS 1.2 only if old clients need it. A recent proxy then also offers a key exchange that resists quantum computers, as Go does where Diffluence itself is the client (SEC-728).

Your proxy keeps access logs, and they hold the address of every person. Keep them as short as you need them: two weeks is plenty to look into a problem. Leave query strings out of them, because SCIM puts e-mail addresses there (SEC-843).

## What people put in pages

Any page can hold special categories of personal data: health, union membership, religion (GDPR article 9). Diffluence cannot know. Think about it before you open a space for HR or for people's own matters, and use page restrictions there: they are the tool for this (SEC-905).

## Mail

Diffluence works without mail: notifications show in the app, and mail waits in the queue. Any SMTP server works, whether your own or one from a provider. Set these:

| Setting | Means |
|---|---|
| `DIFFLUENCE_SMTP_HOST`, `DIFFLUENCE_SMTP_PORT` | The server. The port is 587 unless you set another |
| `DIFFLUENCE_SMTP_USERNAME`, `DIFFLUENCE_SMTP_PASSWORD` | Optional. Use `DIFFLUENCE_SMTP_PASSWORD_FILE` to read the password from a file |
| `DIFFLUENCE_SMTP_FROM` | The sender, required with a host |
| `DIFFLUENCE_SMTP_REPLY_TO` | Optional |
| `DIFFLUENCE_SMTP_TLS` | `starttls` (default), `implicit` for port 465, or `none` for a relay on the same machine |

With `starttls`, a server that does not offer encryption gets nothing: there is no quiet fall back to plain text. Administration, Mail shows the queue and has a Send test.

Mail is not signed by Diffluence. Let your mail server sign with DKIM, and publish SPF and DMARC records for the sender's domain: providers trust one-click unsubscribe only on mail whose DKIM signature covers those headers (RFC 8058), and unsigned mail tends to land in spam. The link in every mail works either way (SEC-423).

Mailpit is a tool for developers and catches mail without delivering it, so it is not for production. See `docs/development.md`.

## The encryption key

Diffluence creates `master.key` in the data directory on first start. It protects every stored file and secret, and derives the key that signs audit checkpoints. **Back it up separately from your data.** Without it, no file can be read and users must set up their authenticator app again. To replace it, see [backup and restore](backup-restore.md#replace-it).

With more than one node (`DIFFLUENCE_ROLE=api` or `worker`) set `DIFFLUENCE_MASTER_KEY_FILE` to the same key on every node. Diffluence refuses to start without it. HA itself is phase 3: attachments are on local disk until the S3 store exists.

## Backups before an upgrade

Backups run from the first day: the backup service of `compose.yaml` makes one every 4 hours into `./backups`, encrypted with `backup-key/backup.key`, which `scripts/setup-secrets.sh` made. **Copy `./backups` off this machine, and keep a copy of the key in a password safe** ([backup and restore](backup-restore.md)). Check once a week that the newest backup restores, with `scripts/restore-check.sh`. **An upgrade waits when no backup was noted in the last 24 hours** (ADR-040), so a forgotten backup never meets a failed migration; the browser then shows why and how to go on. `DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE` changes the age, such as `48h`. A new installation is never asked for a backup. See [upgrade.md](upgrade.md).

## Helpers and what they may reach

Each optional helper runs in its own container on the `helpers` network, which has no way out (SEC-501, SEC-507). What each one gets:

| Helper | Reaches | Sees | Notes |
|---|---|---|---|
| `pdf` (Gotenberg) | Nothing; only the app reaches it | The HTML of an export, and office files a reader opens as a preview | Set a long `DIFFLUENCE_PDF_PASSWORD` in `.env`: the helper then answers only the app. It fetches no address, runs no script in pages, keeps nothing on disk, and runs read-only without extra rights |
| `collab` | The app, to check each connection | The shared draft of a page for people who may edit it | Needs `DIFFLUENCE_COLLAB_TOKEN` on both sides |
| Collabora, draw.io | What you set up | Office files or drawings a person opens | Run them on the same closed network when you can |

**The app itself** needs to reach only PostgreSQL, the mail server, the identity provider, and the addresses an administrator allowed under Outgoing calls. A firewall rule that allows only those, and nothing else on the way out, stops a mistake from becoming a leak (SEC-433).

## Health

- `/livez`: the process is alive
- `/readyz`: ready for traffic (database, migrations, queue, storage)
- Admin console, System health: nodes, versions, queues, failed jobs

## Passwords

New passwords need 15 characters, and are checked against about 5 million leaked passwords that ship with Diffluence. To refuse more, set `DIFFLUENCE_PASSWORD_BLOCKLIST_FILE` to a text file with one password per line. It works on top of the built-in list, and Diffluence reads it at startup. Existing passwords stay valid.

Guessing is slowed in three layers:

| Limit | Counts | Kept |
|---|---|---|
| 50 tries per address in 15 minutes | Every login from one address | In memory, per node |
| 10 tries per account and address in 15 minutes, and 100 per account from all addresses | Logins on one account | In memory, per node |
| 100 wrong passwords in a row stop password login for the account | Only a wrong password on a live account, on the login page | In the database, across restarts and nodes |

The first two end by themselves after 15 minutes. The third does not: the person gets a mail, and password login for that account answers as if the password were wrong, until the person chooses a new password through "Forgot your password?", or an administrator presses **Allow password login again** in Administration, Users. A passkey and single sign-on keep working, so someone who guesses on purpose cannot keep a person out of their account. The right password ends a run of wrong ones. One address can make at most 10 wrong guesses on an account in 15 minutes, so the stop needs many addresses or many hours.

## Audit retention

The console stores how many days audit rows stay: a year by default, and longer only with a stated reason. The application cannot delete them. To remove older rows, an operator runs `diffluence audit prune` with `DIFFLUENCE_MIGRATE_DATABASE_URL` set to the owner of the tables. It verifies the chain, leaves an anchor signed with the instance key, and removes the rows. Verification then starts at the anchor.

## A copy of the audit log outside the database

Every five minutes, when something happened, a worker signs the head of the audit chain. It also writes that checkpoint to the application log, as a line with `"msg":"audit checkpoint"` and `"witness":"audit-checkpoint-1"`. The line holds a position, two hashes and a public key, and no personal data. It is written at every log level.

Someone who owns the database can remove the newest rows of the audit log together with their checkpoints, and the chain then still checks out on its own. The copies in the log show it. So send the application log to a place the database owner cannot change, such as a SIEM or a log server, and keep it as long as the audit log. To compare:

```sh
diffluence audit verify --witness /path/to/application.log
```

It reads every witness line, also with a prefix from the container runtime, checks each signature, and names the first checkpoint the chain no longer matches. `-` reads the log from standard input, for example `docker compose logs --no-log-prefix diffluence | diffluence audit verify --witness -`. Checkpoints older than the last prune are skipped. A log without any checkpoint is refused, because that is the wrong file or a collector that kept nothing. At most the last five minutes of events have no copy yet.

Whoever holds both the database and `master.key` can still sign new checkpoints, but cannot change the lines the log server already has.

## After restoring a backup

Anonymising does not reach backups. After a restore, anonymise the same people again.

## New releases

Diffluence never calls home. Follow the release feed of the project to hear about updates. The console warns when the release that runs is more than 60 days old, from the date of its commit. From 1.0, only the newest release line is supported, and an installation up to six months old upgrades to the newest release in one step ([SECURITY.md](../../SECURITY.md)).

## Editing together

Editing together is optional. Without it, every page keeps private drafts. With it, each page has one shared draft, which everyone who can edit the page sees while it is typed.

1. Choose a long random token, and set it as `DIFFLUENCE_COLLAB_TOKEN` for both containers.
2. Set `DIFFLUENCE_COLLAB_URL=http://collab:1234` for Diffluence.
3. Start it with `docker compose --profile collab up -d`.

Diffluence passes `/collab` on to the service, so your reverse proxy must allow WebSocket upgrades on that path. The service needs Node.js security updates from then on. One service node serves an instance: more than one needs sticky routing by page, which is not built yet.

## Watching it

- `/livez` says the process runs. `/readyz` says this node may get traffic: it checks the database, migrations and storage. A node with role `worker` or `all` also needs the queue. A node with role `api` does not, because events wait in the database, so people can go on working through a queue outage.
- `/metrics` gives numbers for Prometheus: requests by method and class of status, how long they took, the database pool, what waits in the outbox, jobs and mail, and how far the migrations of an upgrade are ([upgrade.md](upgrade.md)). It is off until you set `DIFFLUENCE_METRICS_TOKEN` to a long random token. Prometheus sends it as `authorization: { credentials_file: ... }`. Numbers about the queues are for the whole instance, so scrape one node for those. Request numbers are per node.
- Logs are JSON with a `request_id`. A proxy that sends a W3C `traceparent` header also gets its `trace_id` in the log line, so one request can be followed through both. Diffluence does not export spans.
- The pool opens at most 20 database connections per node. Add `pool_max_conns` to the database address to change that, and keep the total of all nodes below `max_connections`.

## Kubernetes

`deploy/helm/diffluence` is a chart for an existing PostgreSQL. It holds no secret values: make a Secret with the keys listed in `values.yaml`, then:

```sh
helm install wiki deploy/helm/diffluence --set publicUrl=https://wiki.example.com
```

`mode: single` is one pod with the embedded queue. `mode: split` gives API and worker pods, and needs an external NATS, an odd number of servers with three replicas per stream, and a volume all pods can write to, because files stay on a volume until an S3 store exists.

The chart has been checked with `helm lint` and `helm template`, not yet on a cluster. PostgreSQL scales up, not out, and reads stay on the primary, because a person who was just restricted must not read from a stale copy. If you use PgBouncer in transaction mode, it must be 1.21 or newer, and migrations must use a direct connection, because they hold a session lock. Put a limit per address in your proxy: the limits inside Diffluence count per node.

## How large the database must be

For about 5,000 people, give PostgreSQL at least **2.5 CPUs**. With 1.5 CPUs, search had to turn away up to 3 percent of searches as busy at that size, and became slow (ADR-037, `docs/operations/limits.md`). This was measured on one machine with 50,000 pages, not on a real server. Below a few hundred people, 1 CPU is enough. When search starts to slow other work, add a PostgreSQL standby for search before anything else. CPU is what runs out first: on a test server with 8 cores for PostgreSQL, it was CPU, not memory or the disk ([the stress test](../reports/2026-11-10-stress-test.md)).

**Give PostgreSQL memory settings for its machine.** Its defaults suit a very small machine, with 128 MB of shared buffers whatever the machine has. Set them in `postgresql.conf`, or as `command:` flags of the `postgres` service in a `compose.override.yaml` (`scripts/stress/compose.stress.yaml` is an example). Common starting points, from PostgreSQL's own guidance, for a machine that runs PostgreSQL and little else:

| Memory for PostgreSQL | `shared_buffers` | `effective_cache_size` | `work_mem` |
|---|---|---|---|
| 2 GB | 512MB | 1536MB | 8MB |
| 8 GB | 2GB | 6GB | 16MB |
| 32 GB | 8GB | 24GB | 32MB |

`shared_buffers` is about a quarter of the memory and `effective_cache_size` about three quarters. `work_mem` counts for each sort of each query, so keep it modest: the pool holds up to 20 connections a node. Keep `max_connections` above the pools of all nodes together, with room for backups and you.

**To see which queries cost the most**, turn on `pg_stat_statements`: set `shared_preload_libraries = 'pg_stat_statements'`, restart PostgreSQL, and then run `CREATE EXTENSION pg_stat_statements;` once, as the owner of the database. The setting alone loads the module, but the view only exists after `CREATE EXTENSION`. `SELECT calls, mean_exec_time, left(query, 80) FROM pg_stat_statements ORDER BY total_exec_time DESC LIMIT 10;` then lists the heaviest.

**Writes go at the pace of the disk's sync.** Every change that is audited takes the audit chain's lock in turn, and holds it until its commit is on disk (ADR-013). A disk that syncs 160 times a second therefore allows about 160 audited changes a second, however many CPUs there are. Put PostgreSQL on an SSD, or behind a controller with a battery-backed write cache, and leave `synchronous_commit` and `fsync` on.

## Load test

[Known limits](limits.md) says what was measured, and what gives way first.

`scripts/load/reading.js` is a [k6](https://k6.io) script with the pattern of a real reader, including the polling. Its header says how to run it. Run it against a copy, not against your live instance.
