# Installing Diffluence

Written for: operators who know Docker or Linux. Tested with PostgreSQL 16.

## Docker Compose (the reference installation)

Needs Docker with the Compose plugin.

```sh
git clone https://github.com/Dr1nS0menOp/Diffluence.git && cd Diffluence
sh scripts/setup-secrets.sh                 # creates ./secrets, never overwrites
export DIFFLUENCE_PUBLIC_URL=https://wiki.example.com
docker compose up -d --build --wait
printf '%s\n' 'a long password' | docker compose exec -T diffluence \
  diffluence user create-admin --email you@example.com --name "You"
```

`DIFFLUENCE_PUBLIC_URL` must be the address people type in their browser. It drives secure cookies, cross-site protection, passkeys and the OIDC redirect. It is also what decides whether the session cookie gets the `__Host-` prefix, so **https is required in production**.

Put a TLS reverse proxy in front (Traefik, nginx). If it sits on another machine or network, set `DIFFLUENCE_TRUSTED_PROXIES` to its CIDR range, or every user appears to come from the proxy, and the login rate limit counts them all as one.

A new instance gets an example space within seconds of its first administrator: it shows everything a page can do, and can be deleted like any other space. To start empty, set `DIFFLUENCE_EXAMPLE_SPACE=false`. An administrator can add it later under Administration, Settings.

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

Mail is not signed by Diffluence. Let your mail server sign with DKIM, because providers trust one-click unsubscribe only on signed mail. The link in every mail works either way.

Mailpit is a tool for developers and catches mail without delivering it, so it is not for production. See `docs/development.md`.

## The encryption key

Diffluence creates `master.key` in the data directory on first start. It encrypts TOTP seeds and derives the key that signs audit checkpoints. **Back it up separately from your data.** Without it, users must set up their authenticator app again.

With more than one node (`DIFFLUENCE_ROLE=api` or `worker`) set `DIFFLUENCE_MASTER_KEY_FILE` to the same key on every node. Diffluence refuses to start without it. HA itself is phase 3: attachments are on local disk until the S3 store exists.

## Health

- `/livez`: the process is alive
- `/readyz`: ready for traffic (database, migrations, queue, storage)
- Admin console, System health: nodes, versions, queues, failed jobs

## Passwords

New passwords need 15 characters, and are checked against about 5 million leaked passwords that ship with Diffluence. To refuse more, set `DIFFLUENCE_PASSWORD_BLOCKLIST_FILE` to a text file with one password per line. It works on top of the built-in list, and Diffluence reads it at startup. Existing passwords stay valid.

## Audit retention

The console stores how many days audit rows stay: a year by default, and longer only with a stated reason. The application cannot delete them. To remove older rows, an operator runs `diffluence audit prune` with `DIFFLUENCE_MIGRATE_DATABASE_URL` set to the owner of the tables. It verifies the chain, leaves an anchor signed with the instance key, and removes the rows. Verification then starts at the anchor.

## After restoring a backup

Anonymising does not reach backups. After a restore, anonymise the same people again.

## New releases

Diffluence never calls home. Follow the release feed of the project to hear about updates.
