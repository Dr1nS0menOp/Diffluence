# 07.9: Review of what wave O7 built

Wave O7 (spec 044) built part of blueprint 07 after a short study, before this one. This chapter checks each part against the best practice of chapters 1 to 8, and says what to keep and what to correct. Nothing here was found to be wrong in a way that harms an installation today; the corrections make the promises stronger.

| What O7 built | Best practice | Verdict | Correction |
|---|---|---|---|
| **Upgrade test in CI** (OPS-036): old and new release on one database, old one serves reads and logins while the new one migrates, then a rollback | DUPTester (chapter 1): run both versions on the same data; 60% of upgrade failures come through stored data, 40% through messages | **Keep.** The right test, and rare in projects of this size | Add writes during the migration (OPS-071) and tests for every format that crosses versions: documents, bundles, events, jobs, settings, the editing service (OPS-070) |
| **Rollback** (OPS-040): the old release runs on the migrated database until a contract migration ran; afterwards it refuses and names the migration | Expand and contract with removal in a **later** release (Fowler, GitLab's post-deployment migrations); blueprint 07 rule 1 says the same | **Keep the guard.** Refusing is far better than failing on the first query | The grace before a contract migration runs is 10 minutes (`contractGrace` in `internal/app/ops.go`). A bad upgrade is often noticed the next working day. Raise it to 24 hours by default and make it a setting (decided in spec 048). Add to the release checklist that a contract migration may only remove what the **previous** release already stopped reading, so a rollback by one release stays safe even after it ran |
| **Checks before migrating** (OPS-038): PostgreSQL 16, 512 MiB free in the data folder, queue answers, backup age when set | Refuse with a reason (Sentry's hard stops); check latent settings at start (Xu et al., OSDI 2016) | **Keep.** Matches the research closely; naming every reason at once is good practice | Backup check refuses by default with 24 hours, as the product owner decided on 2026-11-10. The database disk is invisible from the node: show the database size and growth on the health page instead (OPS-103) |
| **Backup marker** (OPS-041): `system.backup_noted` and `system.upgrade_started` in the audit chain | Backups are checked by restoring them (GitLab 2017, SRE book) | **Keep.** Tamper-evident and cheap | Nobody sees the backup age unless they upgrade: show it all the time (OPS-077) |
| **Migration metrics** (OPS-022) | Golden signals and saturation (chapter 4) | **Keep** | Add the backup age, disk and contract-pending age metrics, and alert rules (OPS-089) |
| **Release checklist** (`release.md`) | Time-based releases with a freeze; signed, reproducible, with an SBOM (chapter 5) | **Keep** as the base | Add signing, SBOM, provenance, the reproducibility check and `config check` to the steps (OPS-093 to OPS-096, OPS-100) |
| **Lines OPS-001 to OPS-062** | The study style of blueprints 03 to 06 | **Keep the numbering**; the statuses were checked again by this study and still hold | New lines OPS-070 and up come from this study |
| **`lock_timeout` of 5 s with retries** (from before O7, OPS-034) | The standard cure for the lock queue | **Keep** | Add `statement_timeout`, a check that refuses table rewrites, and migrations outside a transaction for concurrent indexes (OPS-072 to OPS-074) |

## What O7 left open, and where it now goes

| Line | Where in this study |
|---|---|
| OPS-002, OPS-031 (Helm on a cluster) | Spec 048, wave 10 |
| OPS-004 (LXC on a real host) | Spec 048, wave 10 |
| OPS-005, OPS-032 (packages) | Chapter 5, OPS-093 |
| OPS-006 (configuration file) | Chapter 6, OPS-099 |
| OPS-011 to OPS-013 (several nodes, NATS cluster, Patroni) | Chapter 3, OPS-084 |
| OPS-014, OPS-052 (S3) | Spec 048, wave 9 |
| OPS-015 (load test) | Chapter 8, OPS-109 to OPS-112 |
| OPS-023 (checkmk) | Chapter 4, OPS-091 |
| OPS-024 (traces) | Chapter 4, OPS-088 |
| OPS-033 (zero downtime on real machines) | Chapter 8, OPS-114 |
| OPS-035 (background data moves) | Chapter 1, with the first migration that needs it |
| OPS-039 (dark features) | Chapter 5, OPS-097 |
| OPS-042 (skipping versions) | Chapter 1, OPS-076 |
| OPS-043 (concurrent indexes) | Chapter 1, OPS-073 |
| OPS-050, OPS-051, OPS-054, OPS-055 (backup and restore) | Chapter 2, OPS-077 to OPS-081 |

## Sources

- Zhang et al.: [Understanding and Detecting Software Upgrade Failures in Distributed Systems](https://doi.org/10.1145/3477132.3483577), SOSP 2021.
- Fowler: [Parallel Change](https://martinfowler.com/bliki/ParallelChange.html).
- GitLab: [Migration style guide, post-deployment migrations](https://docs.gitlab.com/development/database/post_deployment_migrations/).
- Xu et al.: [Early Detection of Configuration Errors to Reduce Failure Damage](https://www.usenix.org/conference/osdi16/technical-sessions/presentation/xu), OSDI 2016.
- Sentry: [Self-hosted releases and upgrading](https://develop.sentry.dev/self-hosted/releases/).
- GitLab: [Postmortem of database outage of January 31](https://about.gitlab.com/blog/postmortem-of-database-outage-of-january-31/), 2017.

All sources of this study are also in [sources.md](sources.md).
