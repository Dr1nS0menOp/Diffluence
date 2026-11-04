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

## The encryption key

Diffluence creates `master.key` in the data directory on first start. It encrypts TOTP seeds and derives the key that signs audit checkpoints. **Back it up separately from your data.** Without it, users must set up their authenticator app again.

With more than one node (`DIFFLUENCE_ROLE=api` or `worker`) set `DIFFLUENCE_MASTER_KEY_FILE` to the same key on every node. Diffluence refuses to start without it. HA itself is phase 3: attachments are on local disk until the S3 store exists.

## Health

- `/livez`: the process is alive
- `/readyz`: ready for traffic (database, migrations, queue, storage)
- Admin console, System health: nodes, versions, queues, failed jobs
