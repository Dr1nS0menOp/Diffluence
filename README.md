# Diffluence

**An open-source, self-hosted knowledge platform. Secure by default, boring to operate, painless to upgrade.**

> Status: **alpha**. Phases 1 and 2 of the [roadmap](docs/blueprint/08-roadmap.md) are built and waiting for acceptance testing. Nothing is supported before 1.0.

Diffluence is a team wiki with spaces, pages and enterprise-grade permissions, licensed under Apache 2.0. The plan and every design decision live in [`docs/blueprint/`](docs/blueprint/00-README.md).

## What it does

Spaces and a page tree, a visual editor with Markdown shortcuts, drafts and version history with compare and restore, attachments, full-text search, a trash, Markdown import and export, a tamper-evident audit log with CSV export. Local accounts with an authenticator app, passkeys, OIDC login and SCIM provisioning. Roles, groups, guests, page restrictions and a "why can or can't they?" view. An admin console.

## Try it

Needs Docker with the Compose plugin.

```sh
sh scripts/setup-secrets.sh
docker compose up -d --build
printf '%s\n' 'a long password' | docker compose exec -T diffluence diffluence user create-admin --email you@example.com --name "You"
```

Then open http://localhost:8080. For a real installation, read [the install guide](docs/operations/install.md).

## Documentation

| | |
|---|---|
| [Install](docs/operations/install.md) | Docker Compose and LXC |
| [Upgrade](docs/operations/upgrade.md) | One command, no downtime |
| [Backup and restore](docs/operations/backup-restore.md) | With a drill that proves it works |
| [Identity providers](docs/operations/identity-providers.md) | Authentik, Entra ID, others |
| [Development](docs/development.md) | Everything runs in containers |
| [Threat model](docs/security/threat-model.md) | What we defend against, and how we know |
| [Routes](docs/api/routes.txt) | Every API route and its access rule |

## Commands

| Command | What it does |
|---|---|
| `diffluence serve` | Runs the server. `DIFFLUENCE_ROLE` = `all` (default), `api` or `worker`. |
| `diffluence migrate` | Applies pending *expand* migrations |
| `diffluence user create-admin` | Creates a local administrator account |
| `diffluence audit verify` | Verifies the audit log hash chain |
| `diffluence healthcheck` | Asks a running server whether it is ready |
| `diffluence version` | Prints the version |

## Configuration

All settings come from environment variables. Any variable can instead be read from a file by appending `_FILE`, for example `DIFFLUENCE_DATABASE_URL_FILE=/run/secrets/db_url`. See [`.env.example`](.env.example).

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md). Every commit needs a DCO sign-off (`git commit -s`), and the clean-room rules are mandatory.

## Security

Please report vulnerabilities privately. See [SECURITY.md](SECURITY.md).

## Licence

[Apache License 2.0](LICENSE)
