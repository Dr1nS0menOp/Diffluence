# 07.7: Running it yourself: what small teams struggle with

Diffluence is self-hosted, often by a team where one person looks after it next to other work. That person decides whether the product is trusted. This chapter collects what operators of self-hosted wikis and similar open-source products run into, from public documentation, issue trackers and post-mortems, and turns it into product work.

## 7.1 What goes wrong, again and again

| Problem | What it looks like elsewhere | Why it happens |
|---|---|---|
| **Upgrades that need a person** | Nextcloud runs upgrades in maintenance mode and cannot skip a major version; operators must step through each. GitLab publishes required upgrade stops and a tool to plan the path, because background migrations must finish before the next version. Sentry's installer refuses to jump over "hard stops" | Large data changes that cannot run while the old version works, and versions that are allowed to drift far apart |
| **Falling behind** | Installations that stop upgrading because one upgrade went badly, and then face many steps at once | Fear after a failure, and no sign in the product that it is old |
| **The disk fills** | Write-ahead log piling up when archiving fails, logs without rotation, uploads and previews without limits, old container images and build caches | Nothing watches the disk, and the product keeps saying it is healthy |
| **Backups nobody restored** | GitLab's 2017 outage, where most backup methods turned out not to work | A backup is checked by restoring it, and nobody had |
| **Certificates and outside services** | Mail stops because a password changed, login stops because an identity provider rotated its keys | Settings only used later are never checked (chapter 6) |
| **Settings nobody understands** | Long lists of options copied from a forum | Too many knobs (chapter 6) |
| **No idea what "normal" is** | Is 2 seconds for a search slow? Is 40 GB a big database? | No published numbers to compare with |

The common thread: each failure is quiet until it is large. The fixes the better projects use are the same: the product watches itself, says plainly what is wrong and what to do, and makes the safe path the easy one.

## 7.2 How the better projects answer it

- **A health page for the administrator** that shows version and age, pending migrations, background work, disk, last backup, mail and login status, each with a plain sentence and a link to the guide. Nextcloud's overview of setup warnings and GitLab's admin health checks are two forms of it.
- **A refusal with a reason** instead of a broken state: Sentry's hard stops, Diffluence's own checks before migrating.
- **Numbers to compare with**: published sizing and limits for typical installations.
- **A guide per failure**, written in the words an operator would search for.

## 7.3 Where Diffluence stands

| Practice | State | Where |
|---|---|---|
| Upgrade is one command, no manual steps, no maintenance mode | Built (a single node restarts for seconds) | OPS-030, OPS-037, `docs/operations/upgrade.md` |
| Checks before migrating that refuse and say why | Built | OPS-038 |
| System health and Care pages in the console | Built | `03-functional/10-administration.md` |
| Published limits and how things fail | Built | `docs/operations/limits.md` |
| Incident guide | Built | `docs/operations/incidents.md` |
| Disk, backup age and version age shown to the administrator | Not built | |
| A sign that the installation is behind the newest release | Not built | |

`limits.md` already tells operators that with a full data disk, uploads fail while `/readyz` still says ready, and that they must watch the disk themselves. That was an honest note; the recommendations below make the product do the watching.

## 7.4 Recommendations

- **OPS-103** · new · **High.** The System health page shows, each with a plain sentence and what to do: free space of the data folder, size of the database and how fast it grew in 30 days, age of the last noted backup, pending contract migrations and background work, failed jobs and events, mail and identity provider status (chapter 6), and the running version with its date. Each also becomes a metric (chapter 4).
- **OPS-104** · new · **High.** Low disk is visible before it is full: below 10% free in the data folder, administrators get a warning on every page of the console; below 2%, uploads, imports and exports are refused with a clear message instead of failing halfway, while reading and saving text go on. `/readyz` reports the state, so a load balancer with another node can prefer it.
- **OPS-105** · new · **Medium.** Everything that grows has a limit or a cleaner, and the guide lists them: logs (standard error, rotated by the platform), previews (made again when needed), the trash (retention), exports (removed after their download window), the outbox and failures (purged), and the queue's files. A test fills each one and checks it shrinks.
- **OPS-106** · new · **Medium.** A sign when the installation is behind: the binary knows its own release date, and the console says when it is more than 60 days old, without asking any outside server. An optional check of the public release feed can be switched on by the operator; it is off by default, so nothing phones home (decided in spec 048).
- **OPS-107** · new · **Medium.** `diffluence doctor`: one command that runs the checks of the health page from the command line, plus `config check` (chapter 6) and `key check`, and prints a short report an operator can paste in a support question, with no secrets and no personal data.
- **OPS-115** · built (spec 058) · **High.** Backups are run from the console on every platform: the console sets the schedule and how many are kept, asks for a backup now, lists every backup with its size, state and test restore, and shows whether the backup worker checked in and holds a key. The worker (the Compose service, a Kubernetes CronJob, a systemd timer) asks and reports through the database. A failed backup or test restore shows on System health and mails the administrators at most twice a day.
- **OPS-116** · built (spec 058) · **Medium.** The console shows the configuration the server starts with and where each value came from, with every secret hidden, and what a restart would refuse.
- **OPS-117** · built (spec 058) · **Medium.** The console logs one person out everywhere, or everyone after an incident, with a fresh login and an audit entry.
- **OPS-118** · built (spec 059; decided by the product owner on 2026-11-11) · **High.** A full restore over the live installation from the console: fresh second factor, typed confirmation, a safety backup first, the worker restores while every node shows the waiting page, then `audit verify` and `key check`, and the request and result are written into the restored audit log.
- **OPS-119** · new (spec 058, 3.3) · **Low.** The smaller console steps: a preview for audit retention, a log level for an hour, the disk thresholds as settings, downloading the monitoring files.
- **OPS-108** · new · **Low.** Sizing examples in the install guide: what a team of 50, 500 and 5,000 needs in CPU, memory and disk, from the measurements of chapter 8, each marked with the machine it was measured on.

## Sources

- Nextcloud: [How to upgrade](https://docs.nextcloud.com/server/latest/admin_manual/maintenance/upgrade.html).
- GitLab: [Plan your upgrade path](https://docs.gitlab.com/update/upgrade_paths/), [Upgrade path tool](https://gitlab-com.gitlab.io/support/toolbox/upgrade-path/).
- Sentry: [Self-hosted releases and upgrading](https://develop.sentry.dev/self-hosted/releases/).
- GitLab: [Postmortem of database outage of January 31](https://about.gitlab.com/blog/postmortem-of-database-outage-of-january-31/), 2017.
- PostgreSQL 16: [WAL configuration](https://www.postgresql.org/docs/16/wal-configuration.html), [Continuous archiving, when the archive command fails](https://www.postgresql.org/docs/16/continuous-archiving.html#BACKUP-ARCHIVING-WAL).
- Yin et al.: [An Empirical Study on Configuration Errors in Commercial and Open Source Systems](https://doi.org/10.1145/2043556.2043572), SOSP 2011.
- Raman, Joglekar, De Cristofaro, Sastry, Tyson: [Challenges in the Decentralised Web: The Mastodon Case](https://doi.org/10.1145/3355369.3355572), IMC 2019: how instances run by volunteers go offline, and what that does to their users.

All sources of this study are also in [sources.md](sources.md).
