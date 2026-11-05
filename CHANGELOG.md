# Changelog

Plain-language notes per release. Every entry has an "Upgrade impact" line.

## Unreleased

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
