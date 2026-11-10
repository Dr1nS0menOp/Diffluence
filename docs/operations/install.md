# Installing Diffluence

Written for: operators who know Docker or Linux. Tested with PostgreSQL 16.

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

## Settings

Every setting is in `.env.example`. Any variable can come from a file by adding `_FILE`, for Docker and Kubernetes secrets. Runtime options (trash retention, upload limit) are in the admin console.

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

Diffluence never calls home. Follow the release feed of the project to hear about updates.

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

For about 5,000 people, give PostgreSQL at least **2.5 CPUs**. With 1.5 CPUs, search had to turn away up to 3 percent of searches as busy at that size, and became slow (ADR-037, `docs/operations/limits.md`). This was measured on one machine with 50,000 pages, not on a real server. Below a few hundred people, 1 CPU is enough. When search starts to slow other work, add a PostgreSQL standby for search before anything else.

## Load test

[Known limits](limits.md) says what was measured, and what gives way first.

`scripts/load/reading.js` is a [k6](https://k6.io) script with the pattern of a real reader, including the polling. Its header says how to run it. Run it against a copy, not against your live instance.
