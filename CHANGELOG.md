# Changelog

Plain-language notes per release. Every entry has an "Upgrade impact" line.

## Unreleased

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
