# 08: Roadmap

> **The study (2026-11-10).** This roadmap was studied against research and the practice of comparable projects in [`08-roadmap/`](08-roadmap/): the road to 1.0, what enterprise teams need first, the risks of each phase after 1.0, and keeping the project going without a budget. It adds lines RMP-001 to RMP-033, listed at the end. The build order is in spec 048.

> The order changed on 2026-11-05 (ADR-024): The product owner chose to build the whole of `03-functional-requirements.md` first, in waves, with one commit per wave. Phase 3 follows that work. The phases below still say what each theme contains.

Every phase ends with an **exit check**: all its tests are green in CI, the product owner has done an acceptance session on the staging environment, and there are no open high or critical security findings. Durations are rough and depend on usage limits and review time.

## Phase 0: Foundation (about 1 to 2 weeks)
- GitHub organisation, repo, licence files, DCO check, branch protection
- `CLAUDE.md`, these blueprint documents, ADR template
- CI pipeline with every quality gate from 09 (initially on an empty "hello world")
- Development environment (Docker Compose with Postgres and NATS)
- Staging on a test server, deployed automatically from `main`
- First threat model
- **Exit:** a pull request with a deliberate bug is blocked by CI. The product owner can open the staging URL.

## Phase 1: MVP, the stable document wiki (about 4 to 6 weeks)
- Local accounts (Argon2id, TOTP), OIDC login, sessions
- Spaces, page tree, editor with Markdown shortcuts, drafts, publishing
- Versions with compare and restore, attachments, images
- Basic roles (viewer, editor, space admin)
- Postgres full-text search that respects permissions
- Trash with a universal retention period
- Audit log with hash chain and CSV export
- Markdown import and export
- Translation framework (English)
- Docker Compose and LXC installation, backup and restore guide
- **Upgrade machinery from day 1:** expand/contract migrations and the N/N+1 test
- **Exit:** the v1 success criteria from 01, except the 4-week soak, which runs during phase 2

## Phase 2: Permissions and identity parity (about 3 to 4 weeks)
- The full permission matrix, page restrictions, permission templates, "why can I?" view
- Groups, guests, SCIM 2.0, passkeys
- Admin console completed
- **Exit:** the permission test matrix (thousands of cases) is green, and SCIM works with Authentik and Entra ID

## Phase 3: Production hardening (about 3 to 4 weeks)
- HA topology: multiple api and worker nodes, NATS cluster, PgBouncer, Patroni-compatible setup
- Envelope encryption with KMS, key rotation
- Prometheus metrics, OpenTelemetry, Helm chart
- k6 load test with 5,000 simulated users in CI
- Automatic restore test
- **Exit:** a node can be killed under load without errors visible to users. An upgrade under load causes no interruption. A restore drill finishes within 4 hours.
- **→ First public "beta" release**

## Phase 4: Rich collaboration (about 6 to 8 weeks)
- Comments (page and inline), reactions, @mentions, notifications and e-mail digests, watching
- Labels, templates, blueprints, the macro framework and the first macros
- Real-time co-editing (collab service)
- Export formats: PDF, DOCX, ODT, HTML, Markdown, bundle

## Phase 5: Automation and publishing (about 6 to 8 weeks)
- Automation rules engine
- Static public publishing to a separate host
- **→ Release candidate**

## 1.0
- A penetration test and fixes, if a volunteer or donations make one possible. Otherwise 1.0 says plainly that none was done (ADR-029)
- Operator documentation, compliance mapping, `SECURITY.md` process live
- From here: monthly releases and the upgrade promise. Translations open on a translation platform (for example Weblate).

## After 1.0 (in this order)
| Phase | Content |
|---|---|
| 6 | **AI:** provider-agnostic (Claude, OpenAI-compatible, Ollama), Q&A that respects permissions (pgvector), summarise and rewrite, MCP server, admin controls and an audit trail of AI actions |
| 7 | **Platform:** public API with OAuth scopes and webhooks, then remote apps (sandboxed iframes on a separate origin), then WASM macros (wazero), then the marketplace (signed apps, consent screen, offline catalogue) |
| 8 | **Linked instances:** trust between instances, migrating and merging spaces, users and history |
| 9 | **Work module:** issues, boards, sprints, workflows, linked to pages, using the same permission and event model |
| 10 | **Mobile apps** (nice to have) |
| 11 | **Migration between products:** from Confluence to Diffluence, from Diffluence to Confluence and other apps, and from other large tools to Diffluence. A very large job, so it comes very late, when the product is far along (ADR-028) |

## Wanted, later

- **Tracked changes in the editor** (2026-11-08). A writer can switch on tracking in the editor. Every insertion and deletion then shows inline, live for everyone editing, marked with who made it and when. Each change can get a comment, and can be accepted or rejected one by one or all at once. It is off by default, because few teams need it, and switching it on or off is a choice in the editor. Accepting and rejecting need the right to edit; the marks never show what a reader may not see, and a published page shows only accepted text.
- **Automation as flows** (2026-11-08). Automation grows from rules into flows that can build almost anything: a trigger, then steps in a visual editor, with conditions, branches, loops, waits, variables, and steps that use DQL to find pages, people and files. A flow can be saved, versioned, switched on and off, and locked so others cannot change it. The aim is the reach of a general workflow tool, in the behaviour people know from wiki automation, with our own look (02). Every flow acts with the rights of a named owner, never more, writes every step to the audit log, has a load budget (ARC-238) and limits on loops, time and calls, and can be stopped at once.
- **Diffluence Query Language (DQL)** (2026-11-08). The search box always searches all spaces. Clicking in it unfolds a panel with filters as buttons. Later that panel also takes a query language, DQL, that means the same as the buttons, so a person can type what the buttons do and more. It builds on the existing search syntax, such as `title:`, and every result stays within what the reader may see.

- **Running the installation from the admin screen** (2026-11-11). An administrator manages as much as possible in the console, whatever the installation runs on (Compose, Kubernetes, LXC). The console controls and a worker does the work: settings and requests go into the database, and a backup worker (the Compose backup service, a Kubernetes CronJob or a systemd timer) carries them out and reports back. In the console: the backup schedule, retention, target (folder or S3) and "back up now"; a list of backups with size, state and whether a restore was tested; the weekly restore test into a throw-away copy; warnings on the health page and by mail. Also: checking the audit chain and its witness, audit retention, logging a person or everyone out, a guided replacement of `master.key`, the upgrade page with waiting contract migrations, the log level, maintenance mode and the disk thresholds, and downloading the monitoring rules. What the server needs to start (database and public address, where the key lives) stays in the configuration and shows read-only with its source. Restoring over the live installation stays a command the console spells out. It starts with a study that sorts every command and setting into "change in the console", "show only" or "command only, because".
- **Importing a space exported from Confluence** (2026-11-11). An administrator uploads a space export (the HTML or XML export a Confluence administrator can make) and gets a Diffluence space with the same page tree, page text, attachments, comments, labels and, where they can be matched, authors and dates. Macros without an equal become a visible note in the page, never lost silently. Only the documented export format is read; nothing of the product itself is used (`02-clean-room-and-licensing.md`). It comes before the wider migration of phase 11, which it starts. The import respects storage limits, runs as background work with progress, and can be tried on a copy first.
- **Connectors for work and code tools** (2026-11-11). Plane, OpenProject, GitLab CE, Forgejo and Gitea can each be added as a connector in the administration, with the address and a token. A space administrator then links projects or repositories of that tool to the space. A link to a work item, merge request, issue or repository becomes a small card in the text (title, state, assignee) or a large card that previews it (description, labels, recent activity). A card shows only what the reader's own account on that tool may see, or what the connector's account may share, as the administrator chooses; it refreshes in the background and says when the tool cannot be reached. Each connector is a separate module behind one interface, so more tools can follow.
- **Searching the text inside attachments** (ORG-025). People want it, and it needs a text extractor, a new dependency.
- **S3 storage.** It needs a client module, chosen when the time comes.

## Carrying the load (ADR-035, ADR-036)

Search comes first, then protection against too much load: the order of work, a limit on what waits, an honest `/readyz`, and a load budget per feature (ARC-233 to ARC-236, ARC-238, ARC-239). The rest of `04-architecture/08-scaling.md` section 8.5 follows the signs listed there.

## After blueprint 03: look, settings and roles (ADR-034)

In this order, each with its own study:

1. **Roles you make yourself:** an administrator makes a role, gives it fine-grained permissions, and gives it to people and groups, per space and for the instance. It replaces the matrix of ticks. Existing grants become roles without changing who can do what.
2. **Settings:** groups in the sidebar that fold, a search, and the large area on the right. Care and System health move under System. People lists filter by dropdowns. The People screen of a space is rebuilt.
3. **The look of Stil:** the sidebar also on the home page, parts of it that fold and can be made larger, no guide lines in the tree, tidier filters, labels without an outline.
4. **Automation for the instance:** rules that stay per space, plus rules for the instance where an administrator chooses which spaces they touch.

## Enterprise document features

The product owner counts these as basic enterprise features (2026-11-06). Each is a clean-room build, as `02-clean-room-and-licensing.md` describes: the products below are named only to say which behaviour is meant. We read their public pages for behaviour, write our own spec, and copy no text, workflow, icon or layout. Each starts with its own study. They are built before 1.0 (ADR-038). The sources are in `03-functional/sources.md`.

| Feature | What people get | What exists already |
|---|---|---|
| **Document control**, the behaviour of Comala Document Management | A page or a whole space follows a workflow of states, such as in review, rejected and approved, with assigned reviewers. A change to approved content sends it back to review. Readers see the last approved version, and only authors and reviewers see the draft. A space workflow overrides page workflows, and a page has one workflow at a time. Regulated work can ask for an electronic signature. | Approvals, fixed approvers per space, the record of past rounds and page status (PAG-062 to PAG-069). Missing: workflows of our own states, readers held on the approved version, signatures. |
| **Draft and published spaces**, the behaviour of Comala Publishing | Two linked spaces: people write in the draft space, and readers only have the published one, so a draft cannot leak through a page setting. It shows what is out of step, publishes one page or the whole space, can leave pages out, and can follow a workflow, so approval publishes. | Public sites on a separate host, and copying pages. Missing: the link between two spaces inside an instance, and tracking what is out of step. |
| **Guided templates**, the behaviour of Templating.app | One action creates a whole set of linked items from a template, with variables. Forms with many field types guide the person who creates, without scripting. Content is filled in by conditions. Rights are set per template, and an administrator approves templates that others may use. | Templates with labels, a title pattern and guided pages (ORG-045 to ORG-061). Missing: a set of pages at once, more field types, conditions, rights per template. |

The public pages of Templating.app describe a Jira app, for issues and projects. Here it means the same behaviour for pages.

## Kinds of item

Create offers four kinds of item, made the same way, each with its own job (ADR-028):

| Kind | Job | When |
|---|---|---|
| Page | Text that people write and keep up to date | Built |
| Blog post | News, published with a date | Wave D |
| Folder | Structure: groups pages and databases | Wave D |
| Database | Data in rows and columns: a simple spreadsheet, or GridCraft if it fits | Wave I, after wave H, with its own study |

## Things decided now so later phases don't need a rewrite
UUIDv7 IDs and `origin_instance_id` (phase 8), the outbox and event system (6, 7, automation), API-first with scopes (7), no hard-coded strings and RTL-ready CSS (translations), the structured document model (co-editing, macros), the `space_id` on everything (scaling and the work module).

## Lines from the study of 2026-11-10

Each line is explained in the chapter named after it, in [`08-roadmap/`](08-roadmap/). All are **new**; spec 048 places them in the build order.

| Line | Priority | What | Chapter |
|---|---|---|---|
| RMP-001 | High | Beta criteria written before the beta | 1 |
| RMP-002 | High | The 1.0 promises written as a page: what is frozen, how it may change, support, upgrades | 1 |
| RMP-003 | Medium | The order before 1.0: safe to run, enterprise document features, server lines, beta | 1 |
| RMP-004 | Medium | Every wave leaves the product maintainable if work pauses | 1 |
| RMP-005 | Low | Each phase after 1.0 starts with a study and a list of what it will not do | 1 |
| RMP-010 | High | Document control first of the three enterprise features | 2 |
| RMP-011 | High | The minimum of document control, derived from ISO 9001, 21 CFR Part 11 and Annex 11 | 2 |
| RMP-012 | Medium | Read and understood, with a record | 2 |
| RMP-013 | Medium | Then draft and published spaces | 2 |
| RMP-014 | Medium | Then guided templates | 2 |
| RMP-015 | Medium | The guide says which controls the software gives and which stay the customer's | 2 |
| RMP-020 | High | AI: no text reaches a model that the asker may not read; off by default | 3 |
| RMP-021 | High | Platform: narrow OAuth scopes, apps on their own origin, a marketplace last | 3 |
| RMP-022 | Medium | Linked instances start as one-way, revocable copies | 3 |
| RMP-023 | Medium | The work module starts as integration with existing trackers | 3 |
| RMP-024 | Medium | Mobile starts as a progressive web app | 3 |
| RMP-025 | Low | Migration starts in one direction, with a dry run and a report | 3 |
| RMP-030 | Medium | Donations only through a payout route that keeps the author anonymous | 4 |
| RMP-031 | Medium | Grant proposals ready per theme, written from the specs | 4 |
| RMP-032 | Medium | The project's own infrastructure costs nothing, and each service it depends on is listed | 4 |
| RMP-033 | Low | Good first issues from the Low lines of the blueprints | 4 |
