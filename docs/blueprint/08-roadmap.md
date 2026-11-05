# 08: Roadmap

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
- External penetration test and fixes
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
