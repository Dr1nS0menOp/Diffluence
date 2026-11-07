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
*Accepted (product owner, 2026-11-04).* Keep the total as low as possible, to avoid constant security advisories and abandoned code, but never hand-write an important or security-critical part when a widely used, maintained module does the job. Consequence: `docs/dependencies.md` lists every dependency with its reason. Not used from the blueprint's list: TanStack Router (it brought 9 packages and one outside our licence list; a small router replaces it), `bluemonday` and `sqlc`.

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
*Accepted by the product owner, 2026-11-05.* The product owner did not want an Upload button and an Attachments block under every page. Files are added while editing, by dragging them in or with the toolbar, and show in the text as a tile. A page has no upload, and the list of files moves to the More menu. Consequence: the document format gains a `file` block, and adding a file needs the editor, so a person with *add attachments* but without *edit* can no longer add files. Supersedes the page layout line of ADR-021.

**ADR-023: Blueprint 03 is the full functional description**
*Accepted by the product owner, 2026-11-05.* The product owner asked for a description of many pages with every detail. After a clean-room study of public help pages, 03 became 13 chapters with numbered requirements and a status each. Consequence: a build session finds the intended behaviour there, a status changes in the same pull request as the code, and numbers are never reused. Sources are in `03-functional/sources.md`. The proposed order of work in 03 does not change the roadmap until the product owner decides.

**ADR-024: Four product waves before phase 3, and a study before every phase**
*Accepted by the product owner, 2026-11-05.* Most of what makes a wiki feel complete sits in the editor and on the page, so waves A to D of `03-functional-requirements.md` come before production hardening. The product owner also decided that every phase starts like blueprint 03: a large study, then every detail planned, because that is how the product gets as complete as possible. Consequence: `09-way-of-working.md` holds the steps, and a phase has no code before its plan exists. `@tiptap/suggestion` is approved for wave A. Supersedes the order of phases in 08.

**ADR-025: How a page looks is page data, and one token holds both themes**
Status: decided (2026-11-05). Context: wave B adds an emoji, full width, title alignment and a header image, and a person's own theme. Decision: page look lives on the page, not in a version, and applies at once. Tokens use `light-dark()` instead of a second list of dark values, and `data-theme` on the root element beats the system. Consequences: restoring an old version does not change how a page looks. There is one list of colours, whose values did not change. See spec 009.

**ADR-026: The product owner's choices for the remaining waves**
Status: decided by the product owner (2026-11-05). Context: the plans for waves C to H left questions that were theirs to answer. Decision:
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

**ADR-030: The page scales, and the editor keeps the tree**
Status: decided by the product owner (2026-11-05), after their first test. Context: ADR-021 fixed the reading column at 47.5rem and took the sidebar away while editing. On a large screen that left the page small, and the row of tools was filling up. Decision: a page and the editor scale with the room, between 47.5rem and 64rem at 64 percent of the frame. Full width is a tool in the row and takes all the room. Tables get their own button and menu, and can stretch up to the frame. The title area holds emoji, header image and page options. The Focus frame keeps the sidebar. Consequences: lines get longer on very large screens, which the cap limits, and a stretched table hides the contents list. Supersedes the fixed column and "the sidebar goes" of ADR-021. See spec 017.

**ADR-031: Spaces have animals, and a comparison is the page itself**
Status: decided by the product owner (2026-11-05). Context: after their first test the product owner asked for a fun animal per space, with character and no standard emoji, and for changes in green and red. Decision: `SpaceBadge` shows a cartoon animal from Kenney's Animal Pack Remastered (CC0), picked from the key until someone chooses. A character stays possible. A comparison renders the real blocks with marks on top, and opening a version shows its changes. Consequences: every picture with a badge changed. The look of a badge follows from this ADR, not from the four tints. Word marks work in paragraphs and headings, and other blocks show as removed and added. See spec 018.

**ADR-032: Our own writers for Word and OpenDocument, no Pandoc**
*Accepted.* Blueprint 04 named Pandoc for this, as a sidecar. Our blocks are a short fixed list, and writing that subset takes only `archive/zip` and `encoding/xml`. Consequences: one process less to run and patch, and our reader is limited and fuzzed. Files stay plain, so the product owner checks one in Word. See spec 014, D2.

**ADR-033: Who may invite, and what waits**
Status: decided by the product owner (2026-11-06). Decision: inviting someone without an account needs a role that allows it, and an administrator approves every invitation. Searching inside attachments and S3 storage wait for later. PdfCraft stays at 18.7 MiB packed and 52.6 MiB unpacked, and databases are an own grid. Consequences: an ordinary member can never bring in an outsider alone.

**ADR-034: A quieter look, better settings, and roles you make yourself**
Status: decided by the product owner (2026-11-07), to build when blueprint 03 is done. Context: after testing, the function was right, but the look and the settings fell short. Decision: the direction Stil replaces the look of Helder: white and blue, hardly any lines or boxes, and the sidebar nearly always in view, also on the home page. Settings of a space and of the instance use the sidebar for their sections, with groups that fold and a search, and the large area on the right to change them. Lists of people filter by dropdowns. Permissions stop being a matrix of ticks: an administrator makes roles, gives each role fine-grained permissions, and gives roles to people and groups. Consequences: `docs/design/README.md` and its pictures change, and the Branches guide lines go. Roles touch `permissions.Evaluate`, the SQL and the matrix test, so they get their own study first, in blueprint 05. Existing grants must become roles without anyone gaining or losing access. Behaviour and structure may follow familiar wikis, but the look stays our own (02). Supersedes: the look in ADR-021, and the Branches tree.

**ADR-035: Search comes first**
Status: decided by the product owner (2026-11-07). Context: people spend about 40 percent of their time in search, and some search more than they write, such as during audits. The self-test found a common word in 50,000 pages to be the first ceiling. Decision: search must be very fast and find everything, also what is nine years old, with recent pages first. Ranking only the newest matches is refused, because it changes what people find. The search box moves to the middle of the top bar. A separate search service in its own container is allowed if that is what it takes. A long tree loads its branches as they open (SPC-031). Consequences: permissions stay the rule: a search engine outside PostgreSQL must never show what a reader may not see, also right after a change. An engine is a new dependency, so the study compares them and the product owner chooses.

**ADR-036: Room for four times the database work**
Status: decided by the product owner (2026-11-07). Context: one database carries reading, writing and search, and the self-test showed what happens when it is full: everything queues and the app still says it is ready. Features to come will ask more of the database. Decision: the architecture leaves room for four times today's database work. Writes stay on one primary, while reads can leave it. Under too much load, saving goes first, then reading, then search, and the app says it is busy instead of slowing down for everyone. Every feature states its load budget. Consequences: blueprint 04.8 section 8.5 holds the numbers, ARC-228 to ARC-241 and the signs for each step. Nothing is built ahead of need, but from now on a read says what it needs. The worst case is worked out, not measured, until real servers are available.

**ADR-037: Search stays in PostgreSQL, with room to grow**
Status: decided by the product owner (2026-11-07), after spec 028. Context: tuned PostgreSQL reached the target, and a separate engine keeps a copy that can show what a permission change has just hidden. Decision: search stays in PostgreSQL. The way to scale is fixed in this order: a PostgreSQL standby for search when search slows other work (ARC-229), and only then an engine, and only if measurements show the standby is not enough. Finding pages and checking permissions stay separate in the code, so the check stays on the primary whatever finds the pages. Word forms count: `plans` finds `plan`. File names match the same way everywhere, in the search box and on the Files screen: whole words and the start of a word, so `rep` finds `report.pdf`. The install guide names about 2.5 CPUs as the least for the database at the model of 5,000 people, because 1.5 was too tight. Consequences: a stress test on a large server (32 cores, 96 GB) follows when the product owner provides one, and replaces estimates with numbers in `docs/operations/limits.md`.

---

*Template for new ADRs:* `ADR-NNN: Title` · Status · Context (2 to 3 lines) · Decision (1 to 2 lines) · Consequences (1 to 3 lines) · Supersedes (if any).
