# What 1.0 promises

Written for: operators deciding whether to rely on Diffluence, and contributors who must keep these promises in every change.

These promises start with release **1.0**. Before 1.0 nothing is promised and no version is supported; the [beta criteria](beta-criteria.md) say what comes first. This page answers RMP-002 of the [roadmap](blueprint/08-roadmap.md).

Diffluence has one maintainer, no budget and no paid staff. Every promise here is one that a single person can keep, and nothing here changes the [Apache License 2.0](../LICENSE): the software comes as it is, without warranty. Where this page says *aim*, it means an aim, not a guarantee.

## 1. What is frozen

From 1.0, these are the interface. They only change as section 2 says.

| What | Frozen | Where it is written |
|---|---|---|
| **The API** | `/api/v1`: its routes, the meaning of every field, the error codes, the token scopes, the webhook format, and the exit codes of `diffluence client` | [docs/api](api/README.md), `routes.txt`, `openapi.json` |
| **The data format** | A page document of any version since 1.0 opens in every later release. A backup of any release restores into that release or a newer one within the upgrade window of section 3 | `internal/document`, [backup-restore.md](operations/backup-restore.md) |
| **The bundle format** | A bundle of any version since 1.0 imports into every later release. A newer bundle is refused with a reason, never read wrongly | [version-skew.md](operations/version-skew.md) |
| **The configuration names** | Every `DIFFLUENCE_*` variable, its `_FILE` form and its key in the configuration file, with the meaning of its values | `.env.example`, `deploy/diffluence.example.yaml` |
| **The commands** | The `diffluence` commands, their flags and their exit codes | [README](../README.md) |
| **Metric and log names** | The metrics `/metrics` sends and the log messages listed as an interface | [monitoring.md](operations/monitoring.md) |

**Not frozen:** the database schema, its tables and SQL functions (only migrations touch them); the Go packages; the screens, the look and the wording; log messages not listed in `monitoring.md`; default values, which may change with a note in the changelog; and everything of an **experimental part** (automation rules, databases and sheets, bundles between instances, restore from the console, draft and published spaces, guided templates, public sites, analytics). An experimental part may change or go in any release. The data it holds is not deleted when it goes: it stays in the database and in every backup. A part that leaves the experimental list joins the frozen set from that release on.

## 2. How something frozen may still change

- **Adding is always allowed:** a new route, field, setting, command, metric or format version. Clients and scripts ignore what they do not know.
- **The API** never changes meaning or loses a route inside `/api/v1`. A removal only comes with a new version beside it, and is announced at least **12 months** ahead with a `Sunset` header (RFC 8594) on the route and in the changelog. This was already promised in [docs/api](api/README.md).
- **A configuration name, command, flag, metric or listed log message** is renamed or removed only after a **deprecation** of at least **six months and two releases**, whichever is longer. During that time the old name keeps working, the changelog lists it under *Deprecated*, and the server or `diffluence doctor` warns each time it is used. Six months matches the longest jump of section 3, so an installation that jumps still starts with its old configuration. After that, a start with the old name stops and names the new one.
- **A stored format** (page documents, bundles, backups, settings) changes in two releases: first one that reads both, then one that writes the new one. Older versions stay readable.
- **The one exception is security.** When keeping something frozen would leave people exposed, it may change in a patch release, without the notice above. The changelog says so first, in bold, with what to do.

## 3. The upgrade promise

- **One command.** With Compose: `git pull && docker compose up -d --build`, or `docker compose pull && docker compose up -d` with pulled images. The migrations run by themselves ([upgrade.md](operations/upgrade.md)).
- **From one release to the next without downtime**, when two or more nodes run behind a load balancer and are replaced one at a time. With one node, people wait a few seconds while it restarts.
- **Expand, then contract.** A release only *adds* to the database while it starts (expand). It removes only what the release before it already stopped using, and only once every running node knows that change and the newest node has run for the **grace period**: a day by default, set with `DIFFLUENCE_CONTRACT_GRACE`.
- **Rolling back** to the previous release works until those contract migrations ran. After that, the way back is the backup that the `system.upgrade_started` marker in the audit log names.
- **A jump of up to six months in one step**, with a short stop: stop the old nodes, start the new ones. Older installations upgrade in steps of at most six months. From 1.0, CI also upgrades from the oldest release that may jump to the newest (OPS-076).
- **Which releases:** every release from 1.0 on, and the last beta, which upgrades to 1.0 in one step. Builds before the last beta are not covered.
- **A recent backup is asked for.** An upgrade waits when no backup was noted in the last 24 hours (ADR-040, ADR-042). The backup service of `compose.yaml` makes one every 4 hours, so an installation that runs it never waits.
- **PostgreSQL:** a release drops a PostgreSQL major version only once the PostgreSQL project has stopped supporting it, and says so six months ahead. Today the lowest is PostgreSQL 16.
- **Proof:** CI runs the upgrade test on every change: the previous release and the new one run side by side, write and read each other's data, and roll back.

## 4. Security fixes

- **Only the newest release line gets fixes.** A fix is never back-ported to an older line: the upgrade is the fix ([release.md](operations/release.md), OPS-098). Section 3 is what makes that fair: anything up to six months old reaches the newest release in one step.
- **How fast, as aims** after a report is confirmed: a Critical issue in a patch release within **7 days**, a High one within **30 days**, the rest in the next regular release. When a fix cannot be ready in time, an advisory with a workaround comes first.
- **How you hear of it:** a GitHub security advisory and a CVE where it applies, the changelog, and the release feed you follow. There is no mailing list and nothing calls home: watch the repository's releases and advisories.
- **How to report:** privately, as [SECURITY.md](../SECURITY.md) says. A report is acknowledged within 3 working days, as an aim.
- **No silent fixes.** Every security fix is named in the changelog once the release that holds it is out.

## 5. Support

Support is **community only**: issues and discussions on the repository, answered when the maintainer or another volunteer can. There is no support contract, no help desk, no answer time, and no help in private, except for security reports. Questions about your own installation are welcome as long as they leave out your data and secrets.

## 6. Releases

A release comes about **once a month** (`1.x.0`), with a security fix as a patch (`1.x.y`). A month may be skipped when there is nothing worth releasing; while the project is maintained, at least one release comes every three months. Each release has notes in plain language, with an "Upgrade impact" line for every change and what an operator must do first.

## 7. What is not promised

- That Diffluence has no flaws, or that a flaw is fixed in a set time. The times in section 4 are aims.
- Support for any release but the newest line.
- Any scale beyond the claim in [the known limits](operations/limits.md): **one node, a few hundred people, measured on your own hardware**. Several nodes, and the numbers measured for 5,000 people, are not a promise.
- Zero downtime with one node, or across a jump of more than one release.
- Anything about experimental parts beyond keeping their data.
- That an independent penetration test or audit was done: if none was done before 1.0, the release says so (ADR-029). One outside reader is planned for permissions, login and restore (ADR-043).
- Any certification, legal advice or compliance on your behalf: the organisation that runs Diffluence carries that ([the security risks](security/risks.md)).
- That the project goes on forever. If it stops, the README and `SECURITY.md` say so on the day it is decided, and the last release says which line was the last. The licence lets anyone carry it on, and the export formats let anyone take their pages elsewhere.

## Decisions

Each was decided on the product owner's behalf on 2026-11-14, may be overruled.

1. **Six months and two releases of deprecation**, not the one release RMP-002 suggested: one release would break an installation that jumps six months, which section 3 allows.
2. **The API keeps its 12 months**, as `docs/api/README.md` already promised; the API is the interface outside programs depend on most.
3. **Security aims of 7 days for Critical and 30 days for High.** `SECURITY.md` says a Critical fix within 7 days; "within days" for everything else is more than one maintainer can keep, so High gets 30 days and lower severities the next release.
4. **At least one release every three months**, beside the monthly aim, so the six-month jump always has releases to land on.
5. **The last beta is the only pre-release covered** by the upgrade promise; earlier builds also upgrade with the same machinery today, but are not promised.
6. **Experimental parts are outside the freeze**, but their data is kept: a part may go, its rows may not vanish with it.
7. **PostgreSQL follows the PostgreSQL project**: no major version is dropped while PostgreSQL still supports it.
8. **A notice if the project stops**, because a reader who relies on these promises should learn the same day that they ended.

## How others promise it

Read on 2026-11-14. Diffluence keeps the shape of these and promises less, because it has one maintainer.

| Project | What it promises | Source |
|---|---|---|
| PostgreSQL | Each major version supported for five years, with a minor release at least every three months that needs no dump and restore; skipping major versions is allowed | [Versioning policy](https://www.postgresql.org/support/versioning/) |
| Mattermost | Monthly releases; an extended support release every 9 months, supported for 12; high-severity security fixes back-ported to the previous three monthly releases; ESR to ESR upgrades tested | [Release policy](https://docs.mattermost.com/product-overview/release-policy.html) |
| Nextcloud | A major version every four months, maintained for a year by monthly releases; major versions cannot be skipped | [Maintenance and release schedule](https://github.com/nextcloud/server/wiki/Maintenance-and-Release-Schedule), [upgrade manual](https://docs.nextcloud.com/server/latest/admin_manual/maintenance/upgrade.html) |
| Discourse | A release cut about monthly gets critical security fixes for two release cycles; an extended support release about every six months gets them for longer | [Release channels](https://meta.discourse.org/t/understanding-discourse-release-channels/264400) |
| Gitea | Security fixes only for the newest major release and `main`; migrations run at start; no downgrade without a backup | [SECURITY.md](https://github.com/go-gitea/gitea/blob/main/SECURITY.md), [upgrade guide](https://docs.gitea.com/installation/upgrade-from-gitea) |

Diffluence is closest to Gitea: one supported line, and an upgrade path strong enough to make that fair.
