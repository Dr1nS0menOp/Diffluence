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

**ADR-021: One design standard, enforced by checks**
*Accepted by the product owner, 2026-11-04.* After ten designs and two rounds of iteration, the product owner chose the Helder style and layout, the Rail editor, and a page tree with collapsible branches, in the system font with an optional imported font. `docs/design/README.md` is the standard. Design values live only in `web/src/design/tokens.css`; `web/scripts/check-design.mjs` and approved pictures in the browser tests enforce it. Consequence: a change that drifts from the design fails, and every new screen needs a picture. Pictures are tied to one browser image, so updating Playwright means new pictures. No new dependency: icons, logo, menus and dialogs are our own. The look is the product owner's to change, through a new ADR.

**ADR-022: A file belongs in the text**
*Accepted by the product owner, 2026-11-05.* the product owner did not want an Upload button and an Attachments block under every page. Files are added while editing, by dragging them in or with the toolbar, and show in the text as a tile. A page has no upload, and the list of files moves to the More menu. Consequence: the document format gains a `file` block, and adding a file needs the editor, so a person with *add attachments* but without *edit* can no longer add files. Supersedes the page layout line of ADR-021.

**ADR-023: Blueprint 03 is the full functional description**
*Accepted by the product owner, 2026-11-05.* the product owner asked for a description of many pages with every detail. After a clean-room study of public help pages, 03 became 13 chapters with numbered requirements and a status each. Consequence: a build session finds the intended behaviour there, a status changes in the same pull request as the code, and numbers are never reused. Sources are in `03-functional/sources.md`. The proposed order of work in 03 does not change the roadmap until the product owner decides.

**ADR-024: Four product waves before phase 3, and a study before every phase**
*Accepted by the product owner, 2026-11-05.* Most of what makes a wiki feel complete sits in the editor and on the page, so waves A to D of `03-functional-requirements.md` come before production hardening. The product owner also decided that every phase starts like blueprint 03: a large study, then every detail planned, because that is how the product gets as complete as possible. Consequence: `09-way-of-working.md` holds the steps, and a phase has no code before its plan exists. `@tiptap/suggestion` is approved for wave A. Supersedes the order of phases in 08.

**ADR-025: How a page looks is page data, and one token holds both themes**
Status: decided (2026-11-05). Context: wave B adds an emoji, full width, title alignment and a header image, and a person's own theme. Decision: page look lives on the page, not in a version, and applies at once. Tokens use `light-dark()` instead of a second list of dark values, and `data-theme` on the root element beats the system. Consequences: restoring an old version does not change how a page looks. There is one list of colours, whose values did not change. See spec 009.

**ADR-026: The product owner's choices for the remaining waves**
Status: decided by the product owner (2026-11-05). Context: the plans for waves C to H left questions that were his to answer. Decision:
- **Editing together:** yes, with the service from blueprint 04.
- **PDF export:** Gotenberg as an optional container.
- **HTML import:** `golang.org/x/net/html`.
- **PDF in the browser:** PdfCraft (MIT or Apache-2.0), if its web build is not too large. The product owner wants people to be able to change a PDF themselves.
- **Diagrams:** draw.io (editor Apache-2.0) as a separate service in embed mode, not `mermaid`.
- **Office files:** the choice is made at installation. Default Collabora or Euro-Office, otherwise LibreOffice. These are separate programs, so they are not dependencies of ours.
- **Leaked passwords:** an embedded list of up to 50 MB, from SecLists (MIT).
- **Drawing:** `components/chart.tsx` and `components/drawing.tsx` may hold SVG beside `icons.tsx`.
- **EDT-006**, a floating bar, is dropped.
- **Passwords:** at least 15 characters. People are pushed towards 2FA and passkeys, administrators towards an identity provider and passkeys.
- **Login history:** no place. A SIEM can work that out.
- **Restoring below an archived page** is refused, with an offer to restore both.
- **Analytics with names** stays off by default.
- **OAuth for the API** stays in phase 7.
- **Public sites:** a folder and a zip.
- **Mail for testing:** Mailpit (MIT).
Consequences: a Confluence importer stays marked as tested on our own files until someone donates a sample. Everything that runs as a separate service stays optional, so a basic installation needs nothing extra.

**ADR-027: The home page sits above the tree, and a new instance starts with an example**
Status: decided by the product owner (2026-11-05). Context: the home page looked like any other row of the tree, and a new instance was empty. Decision: the sidebar gets a strip of spaces at the top, and a fixed Home row. The home page leaves the tree, and its children become the top rows. The worker makes an example space once, on an instance that has never had a space, and an administrator can add it later. Consequences: this changes the Space frame, so the design standard changes with it. The example must show everything the document format can hold, and a test fails when it does not. See spec 016.

**ADR-028: Four kinds of item, and migration comes last**
Status: decided by the product owner (2026-11-05). Context: The product owner set out what can be created, and how late migration comes. Decision: Create offers Page, Blog post, Database and Folder. Blog posts and folders stay in wave D. Databases become wave I, with GridCraft to be weighed against our own grid in its study. Moving from Confluence to Diffluence, from Diffluence to Confluence and other apps, and from other large tools becomes phase 11, after all other phases. Consequences: the Confluence importer leaves wave G and phase 5, so the first public releases have no way in from Confluence. Export of our own formats stays in wave G, so nobody is locked in.

**ADR-029: What blueprints 04, 05 and 06 asked of the product owner**
Status: decided by the product owner (2026-11-05). Context: the studies ended with questions. Decision: The product owner chose on four, and accepted the advice on the rest.
- **Titles in the audit log:** an administrator sees the id of a restricted page with the words "(Censored title)". They can look the page up, and cannot read its title directly. This holds for the screen and for exports.
- **Branch protection** goes on before the first outside contribution.
- **No budget:** Diffluence has no paying customers and no money. There is no paid outside reviewer and no lawyer. The product owner will ask for donations.
- **Accepted advice:** a High finding blocks a wave. Administrators need a second factor on a new instance, after a grace period. Editing together means one shared draft per page, shown clearly, and private drafts remain where the service is not installed. Live counts use polling. DOCX and ODT writers are our own. Files always go through the API. Sessions stay at 8 hours idle and 7 days, with stricter values in the guide. The audit format changes so personal data can be erased while the chain holds. The audit log stays 12 months by default. Malware scanning is an optional separate program. Operators follow a release feed, with no call home. A mention on a public site is neutral text, unless the site shows names.
Consequences: what would need a professional stays unchecked until a volunteer or a donation makes it possible, and the documents say so instead of assuming it. The penetration test before 1.0 depends on the same: see the roadmap.

---

*Template for new ADRs:* `ADR-NNN: Title` · Status · Context (2 to 3 lines) · Decision (1 to 2 lines) · Consequences (1 to 3 lines) · Supersedes (if any).
