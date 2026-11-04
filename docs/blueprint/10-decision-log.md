# 10: Decision log (ADRs)

Each decision is short: **context → decision → consequences**. Status is *Accepted* (agreed with the product owner) or *Proposed* (waiting for the product owner). A decision is never edited afterwards. A new ADR supersedes it.

---

**ADR-001: Licence Apache 2.0 with DCO**
*Accepted.* We want maximum adoption, including by companies, and a patent grant. A DCO keeps contributing easy. Consequence: no relicensing later, and no copyleft code or dependencies.

**ADR-002: Clean-room development**
*Accepted.* Behaviour is based on public descriptions only, with code, text and design our own (see 02). Consequence: no competitor source code, a provenance log in the specs, and licence scanning in CI.

**ADR-003: Go backend, TypeScript/React frontend**
*Accepted.* Go is simple, memory-safe, has a strong standard library, few dependencies and strict compatibility, and builds to a single binary. The best editor and collaboration libraries are in TypeScript. Consequence: the real-time collab service (phase 4) is a small Node service without business logic.

**ADR-004: Structured JSON document model, Markdown as import/export**
*Accepted (2026-11-03).* Markdown alone cannot represent macros, mentions, merged table cells or co-editing state. Consequence: Markdown round-trips work for standard content. Rich elements export to Markdown as close as possible, with a note.

**ADR-005: Modular monolith in one binary with roles**
*Proposed.* One codebase and binary (`api`, `worker`, `all`) with strict internal modules. Consequence: easy to deploy from LXC to Kubernetes. Services can be split out later if measurements ever require it.

**ADR-006: PostgreSQL as the only required database**
*Proposed.* Proven, and fits the existing Patroni/HAProxy setup. Full-text search covers v1. Consequence: no separate search cluster to begin with. Search is behind an interface for later.

**ADR-007: NATS JetStream as the message queue, with a transactional outbox**
*Proposed.* Written in Go, Apache 2.0, can run **embedded** for small installs and as a cluster for HA, and has persistent streams. The outbox guarantees that events are not lost. Consequence: one extra component in HA setups, none in small setups.

**ADR-008: Single-tenant, ready for linking**
*Accepted.* One organisation per installation. UUIDv7 IDs and `origin_instance_id` make linking and merging possible later. Consequence: no multi-tenant SaaS in the core design.

**ADR-009: Allow-only permissions with inheriting view restrictions**
*Proposed.* Three layers (global, space, page) and no explicit deny. View restrictions inherit, edit restrictions don't. One central evaluation function. Consequence: effective rights are easy to explain and test.

**ADR-010: Public pages as a static site on a separate host**
*Accepted (2026-11-03).* This meets "fully cut off from the rest" most strictly: there is no server code, no database and no path back. Consequence: public pages update when published, not live. Public search is client-side.

**ADR-011: Envelope encryption (AES-256-GCM) with a pluggable KMS**
*Proposed.* Attachments and secrets are encrypted at application level. Page text is protected by disk and backup encryption so search keeps working. Consequence: key management must be documented and backed up separately.

**ADR-012: Zero-downtime upgrades through expand/contract and N/N+1 compatibility**
*Accepted (requirement), method proposed.* Additive migrations, background data migrations via the queue, removal in a later release, and a CI test with mixed versions. Consequence: some changes take two releases to complete. That is the price of upgrades without downtime.

**ADR-013: Tamper-evident audit log**
*Proposed.* Append-only, hash chain, daily signed checkpoint, separate retention. Consequence: anonymising users uses pseudonymous ids instead of deleting audit rows.

**ADR-014: One universal retention period for deleted items**
*Accepted.* Default 30 days, configurable. Per-space retention comes later. Consequence: a purge job also removes data from object storage and the search index.

**ADR-015: Deployment on Docker Compose, Kubernetes (Helm) and LXC (deb + systemd)**
*Accepted.* The same binary everywhere. Consequence: once each target exists (Helm from phase 3), every release is tested on it in CI.

**ADR-016: Dependency policy**
*Accepted (product owner,2026-11-04).* Keep the total as low as possible, to avoid constant security advisories and abandoned code, but never hand-write an important or security-critical part when a widely used, maintained module does the job. Consequence: `docs/dependencies.md` lists every dependency with its reason. Not used from the blueprint's list: TanStack Router (it brought 9 packages and one outside our licence list; a small router replaces it), `bluemonday` and `sqlc`.

**ADR-017: The permission rule lives in Go and in SQL, tested against each other**
*Proposed.* Single decisions use a pure Go function, which also feeds the "why can or can't I?" view. Lists use SQL functions, so filtering happens in the database. Consequence: two expressions of one rule, which is exactly why the matrix compares both with a third, literal one. A change touches all three.

**ADR-018: Jobs and periodic tasks are coordinated through PostgreSQL**
*Proposed.* Imports use a `jobs` table with `SKIP LOCKED`. Periodic tasks use a lease row, which also works through PgBouncer, unlike session advisory locks. Events still go through the outbox and NATS (ADR-007). Consequence: one moving part fewer. Revisit if job volume ever calls for it.

**ADR-019: Two database roles**
*Proposed.* Migrations run as the owner of the tables. The application uses a role that owns nothing, with UPDATE and DELETE on `audit_events` revoked. Consequence: the blueprint's "the application's role cannot change audit rows" is enforced by PostgreSQL. Compose sets this up; a single-role setup still works for development.

**ADR-020: Accounts link across SCIM and OIDC on a shared stable id**
*Proposed.* Found by testing against real Authentik: it does not mark e-mail addresses as verified, but its SCIM `externalId` equals the OIDC subject. Linking on that id needs no trust in an address users may change. Consequence: Entra ID needs `externalId` mapped to `objectId`. Local accounts with a password never link.

---

*Template for new ADRs:* `ADR-NNN: Title` · Status · Context (2 to 3 lines) · Decision (1 to 2 lines) · Consequences (1 to 3 lines) · Supersedes (if any).
