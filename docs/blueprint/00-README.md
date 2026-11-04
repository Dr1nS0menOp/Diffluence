# Diffluence: Blueprint

> **Working name: Diffluence.** In hydrology, a *diffluence* is the point where a river splits into branches. It is the opposite of a *confluence*. That suits an open-source wiki that anyone may fork. Before going public, check the name with EUIPO (TMview) and USPTO, and check the domain (see 02).

| | |
|---|---|
| Status | Draft v0.2, 2026-11-04 |
| Owner | The product owner (product owner, tester, operator) |
| Developer | Claude (via Claude Code) |
| License | Apache 2.0 |
| Location in repo | `docs/blueprint/` |

## Documents

| # | Document | What it answers |
|---|---|---|
| 01 | Vision and scope | What we build, for whom, and what we don't build |
| 02 | Clean-room and licensing | How we stay legally clean; CLA vs DCO; name |
| 03 | Functional requirements | What v1 does, and what full parity means later |
| 04 | Architecture | Components, stack, how they talk to each other |
| 05 | Data and permission model | Core entities, IDs, and the permission rules |
| 06 | Security and compliance | Threats, controls, crypto, audit log |
| 07 | Operations and upgrades | Deployment, HA, zero-downtime upgrades, backup |
| 08 | Roadmap | Phases with clear exit criteria |
| 09 | Way of working | How the product owner and Claude build, test and review |
| 10 | Decision log | Short ADRs: what we decided and why |

## Key decisions at a glance

- **Single-tenant** per installation. Instances can be **linked and merged** later (globally unique IDs from day 1).
- **Go** backend as a modular monolith in **one binary** with several roles (api, worker). **TypeScript/React** frontend.
- **PostgreSQL** is the only required database. **NATS JetStream** is the message queue (embedded for small installs, a cluster for HA).
- **Structured document model** (JSON) as the source of truth, with **Markdown** as a first-class editing, import and export format.
- **Public pages** are published as a **static site** to a fully separate host. There is no connection back into the internal instance.
- **Upgrades are one command**, and rolling upgrades run without downtime between consecutive releases.
- **Audit log** of every user and admin action, made tamper-evident with a hash chain.
- **Allow-only permissions** with no explicit "deny", inheriting down the page tree.
- **DCO** for contributions (no CLA).

## Resolved with the product owner (2026-11-03)

1. **Name:** *Diffluence*, pending a trademark check before going public.
2. **Guests:** guests *can be granted* access to any space, but see nothing until someone grants it.
3. **Document format:** structured JSON is the source of truth. Markdown is for typing, import and export (ADR-004).
4. **Page views:** *not* in the audit log. They come later as a separate statistics/analytics feature.
5. **Trash retention:** 30 days by default, configurable.
6. **Public pages:** a static site on a separate host, with no path back (ADR-010).

## How these documents are used

- They are the **single source of truth** for Claude Code. `CLAUDE.md` in the repo root points here.
- Changes go through a pull request, like code. Decisions are added to **10-decision-log.md** and never silently changed.
- Every phase in 08 refers back to the requirements in 03, so you can always check "is it done?".
