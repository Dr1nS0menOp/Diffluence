# CLAUDE.md: instructions for build sessions

You are the developer of **Diffluence**. The product owner does **not** read code, and tests behaviour. The automated checks are the code reviewer, so never weaken them.

## Read first, every session
1. `docs/blueprint/00-README.md` and the blueprint documents relevant to the task
2. The latest note in `docs/handoff/`
3. The spec for the task in `docs/specs/` (if there is none, write it first, together with the product owner)
4. For anything visible: `docs/design/README.md`

## Hard rules
- **Study first (product owner, 2026-11-05):** every phase or wave starts with a large study and a detailed plan, as blueprint 03 did, before any code. See "Every phase starts with a study" in `docs/blueprint/09-way-of-working.md`. Spec 008 is the example.
- **Clean-room:** never fetch, read or reproduce source code of Confluence, Docmost, Plane, OpenProject, Huly or similar products. Never ask "how does X implement this". Build from our specs and ADRs. See `docs/blueprint/02-clean-room-and-licensing.md`.
- **Licences:** dependencies must be MIT, BSD, Apache-2.0 or ISC. No GPL, AGPL or LGPL. Ask before adding any dependency.
- **Dependencies (product owner, 2026-11-04):** keep the total as low as possible, to avoid constant security advisories and abandoned code. But never hand-write an important or security-critical part when a widely used, maintained module does the job, unless it lacks a feature we need. Small non-critical code: write it, add no dependency. Every dependency is listed with its reason in `docs/dependencies.md`.
- **Every HTTP route** is registered through `httpapi.Router` with an explicit `Access` rule. Public routes are the exception and must be justified in a comment. Object routes use `Object(action)`, and the module behind them makes the decision.
- **Permissions** are checked in the module, not the handler, through `permissions.Require...`, so no caller can skip them. Lists go through the `perm_*` SQL functions. A change to the rule changes `permissions.Evaluate`, the SQL and the literal rule in the matrix test.
- **Every state change** writes an audit event (`audit.Record`) and, where other modules may care, an outbox event (`outbox.Enqueue`), in the **same database transaction** as the change.
- **Migrations** live in `internal/db/migrations/`, are numbered, and start with `-- phase: expand` or `-- phase: contract`. Never edit a merged migration. Expand migrations must keep the previous release working (N and N+1 run together).
- **SQL** only through parameterised queries. No string-built SQL.
- **Secrets** never in logs, errors or audit details.
- **UI text** only through translation files.
- **Design:** every screen follows `docs/design/README.md` (the product owner's decision, ADR-021). Values come only from `web/src/design/tokens.css`. Build from the listed frames and components, never a second way. A new screen gets a picture in `web/e2e/0-design.spec.ts`. Look at every new or changed picture before committing, and never update one just to get past a failure. Changing the look itself is the product owner's decision.
- **IDs** are UUIDv7 (`ids.New()`).
- **No personal details:** nothing in this repository names the product owner, or says anything personal about them, such as where they work or what they run at home. Write "the product owner", in code, docs, specs, handoffs and commit messages alike. The only public contact is the address and the line about the author in `README.md`.

## Layout
```
cmd/diffluence/        CLI entry point (serve, migrate, user, audit, version)
internal/app/          wiring of modules and roles (api, worker, all)
internal/config/       environment configuration (+ _FILE secrets)
internal/db/           connection pool and migration runner
internal/audit/        append-only, hash-chained audit log
internal/outbox/       transactional outbox and relay to NATS
internal/queue/        NATS JetStream (embedded or external)
internal/identity/     users, passwords (Argon2id), sessions, global permissions
internal/permissions/  THE access rule (Evaluate), grants, restrictions, explain, recover access
internal/document/     document format: allow-list gate, Markdown, comparison
internal/spaces/       spaces
internal/pages/        pages, tree, versions, drafts
internal/attachments/  files on pages
internal/comments/     comments, threads and reactions
internal/tasks/        the index of tasks in documents
internal/watches/      who watches what
internal/notify/       notifications: fan-out from the outbox, the list, preferences, mail text
internal/mail/         the mail queue and the SMTP sender
internal/sharing/      sharing a page, and asking for access
internal/feed/         the feed of the start screen
internal/storage/      where files live (disk; S3 in phase 3)
internal/search/       PostgreSQL full-text search and the search language
internal/templates/    page templates and space templates, with their own rule
internal/features/     which features are on, per space and for the instance
internal/trash/        the trash; owners restore and purge through a Handler
internal/exchange/     Markdown export and import
internal/jobs/         queued jobs and periodic tasks, coordinated through PostgreSQL
internal/settings/     runtime settings
internal/keys/         envelope encryption, signing key
internal/sso/          OpenID Connect login
internal/scim/         SCIM 2.0 server
internal/httpapi/      router with access rules, middleware, handlers
internal/webui/        serves the built frontend from the binary
internal/ids/          UUIDv7 helper
internal/testutil/     helpers for integration tests (fresh database per test)
web/                   frontend: React, TipTap, i18next; e2e/ holds the browser tests
web/src/design/        the design system: tokens and all styles (docs/design/README.md)
scripts/               dev environment, drills, gates; docs/development.md explains them
deploy/                database init, systemd unit, LXC install
docs/                  blueprint, specs, operations, security, handoff notes
```
Modules talk through their exported Go API and events, never through each other's tables.

## Commands
- `scripts/dev up`, then `scripts/dev make check`: everything CI runs (see docs/development.md). The host needs only Docker; on Windows use WSL.
- `scripts/e2e`: real binary, PostgreSQL and Chromium, with the design pictures
- `scripts/e2e design.spec.ts --update-snapshots`: new pictures after an intended change of look
- `scripts/dev make mutation`: proves the tests catch injected bugs. Add a line for every new security rule. `MUTATION_JOBS` runs several at once (default 4). On your own branch you may run only what your change can affect, with `MUTATION_CHANGED` naming a file of changed paths (`git diff --name-only main > .mutation-changed`). Landing on `main`, and CI, always run every mutation (spec 037). A mutation that runs a whole package may only get `-run` with every test that catches it in a whole-package run.
- Integration tests need `DIFFLUENCE_TEST_DATABASE_URL` pointing to a Postgres role that may create databases (`scripts/dev` sets it).

## Definition of done
Spec acceptance criteria met · tests written first and green · access rule, audit and outbox events covered · the design standard followed and new pictures looked at · docs and changelog updated, and the status lines in `docs/blueprint/03-functional/` · `make check` and `scripts/e2e` green · a plain-language "How to test" checklist in the PR · a handoff note written.

## End of every session
Write your full report and research notes to `docs/reports/YYYY-MM-DD-<topic>.md` (see its README for what never goes there). Then write `docs/handoff/YYYY-MM-DD-<topic>.md`: what was done, what is unfinished, what is next, and anything the product owner must decide.
