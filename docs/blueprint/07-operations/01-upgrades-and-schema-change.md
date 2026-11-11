# 07.1: Upgrades and changing the schema while people work

Blueprint 07 calls the upgrade "the most important promise". This chapter checks that promise against research and against how large projects do it, and says what Diffluence should add.

## 1.1 What the research says

**Upgrade failures are common, severe and mostly about data.** Zhang et al. studied 123 real upgrade failures in eight widely used distributed systems (SOSP 2021). Every one came from two versions that did not understand each other. In 60% the two versions met through **data on disk**, in 40% through **messages on the network**. Two thirds were catastrophic: they hit all or most users. Most could have been found by a test that runs the old and the new version against the same data. Their tool, DUPTester, did that and found 20 new failures in four mature systems. The lesson for Diffluence: the dangerous places are not only tables, but every format that one version writes and another reads.

An older study, Dumitraș and Narasimhan (Middleware 2009), found that errors in the upgrade *procedure*, such as broken dependencies, were the leading cause of failed upgrades in enterprise systems; a survey of administrators they cite put the average share of failed upgrades at 8.6%, and some reported up to half. They also warned that rolling upgrades are not atomic, so a mix of versions can break hidden dependencies, which is exactly what an N/N+1 test must look for. Gunawi et al. read the public reports of 597 outages of 32 large services (SoCC 2016) and found that redundancy alone did not prevent them, and that most reports did not even say what caused the outage or how it was fixed.

**Changing a schema without downtime is solved in theory and hard in practice.** De Jong, van Deursen and Cleve showed with QuantumDB (ICSE 2017) that a layer between application and database can make a schema change without locking, at an acceptable cost, across 81 industrial scenarios. The price is a layer that every query passes through. Curino, Moon and Zaniolo followed how Wikipedia's schema changed over years while building their PRISM workbench (PVLDB 2008): change never stops, and most changes add rather than remove. That is the reason the industry settled on **expand, migrate, contract**, which Fowler calls *parallel change*: first add the new shape, then move readers and writers, then remove the old shape, each step its own release.

## 1.2 How the industry does it

| Who | What they do | What Diffluence takes from it |
|---|---|---|
| **GitLab** | Regular migrations before the new code runs; post-deployment migrations after all nodes run it; batched background migrations for large data, which must finish before the next upgrade. Required upgrade stops at fixed minor versions (x.2, x.5, x.8, x.11), and a tool that plans the path | Already the model of OPS-034 and OPS-035. The stops show what happens when background migrations are allowed to pile up |
| **Stripe** | Dual writing: write to old and new, move reads, move writes, remove the old. Never one switch | The pattern for any change to a column people use |
| **Kubernetes** | A written version-skew policy: which components may differ by how many versions | Diffluence's N/N+1 rule is the same idea, and needs to be written as a policy with a test |
| **Nextcloud** | Upgrades run in maintenance mode, so users are locked out; major versions cannot be skipped | What Diffluence promises not to do |
| **Sentry (self-hosted)** | "Hard stops": the installer refuses to jump over versions with large database changes | The honest version of skipping: a refusal with a reason, not a broken install |
| **PostgreSQL practice** | A short `lock_timeout`, `CREATE INDEX CONCURRENTLY`, `NOT VALID` then `VALIDATE` for constraints, no table rewrite on a busy table | Partly in Diffluence (below) |

**The lock queue.** PostgreSQL grants locks first come, first served. A migration that wants an `ACCESS EXCLUSIVE` lock waits behind a long read, and **every query after it waits behind the migration**, even short reads. A migration does not need to be slow to stop a site: it only needs to wait. The cure everyone uses is a short `lock_timeout` with retries, which Diffluence has. The second danger is a statement that *gets* the lock and then holds it while it rewrites a table: changing a column type, adding a column with a volatile default, adding `NOT NULL` without a prepared check. Those need a rule that refuses them, not a timeout.

## 1.3 Where Diffluence stands

| Practice | State | Where |
|---|---|---|
| Expand and contract, contract only when every live node knows it | Built | `internal/db/migrate.go`, OPS-034 |
| `lock_timeout` of 5 s on every migration, retried with growing waits | Built | `applyWithRetry` in `internal/db/migrate.go` |
| A migration lock so nodes take turns | Built | `pg_advisory_lock` in `Migrate` |
| Old and new version on one database, in CI | Built for HTTP reads and logins | `scripts/upgrade-test.sh`, OPS-036 |
| Refusal of a database a newer release contracted | Built | `internal/db/compat.go`, OPS-040 |
| Checks before migrating | Built | spec 044, OPS-038 |
| Each migration in one transaction | Built; `-- transaction: none` for `CONCURRENTLY` (spec 053) | OPS-043, OPS-073 |
| A rule that refuses table rewrites | Built (spec 053) | OPS-072 |
| Writes during the upgrade test, and formats other than tables | Built for pages, drafts, comments, files, jobs, events and settings (spec 053) | OPS-070, OPS-071 |
| A map from release to migrations, for skipping versions | Not built (needs 1.0) | OPS-042 |
| Large data moves as background jobs | Later, with the first that needs it | OPS-035 |

## 1.4 Recommendations

- **OPS-070** · partly · **High.** Test every format that crosses versions, not only tables. DUPTester's finding is that the persisted formats fail. For Diffluence those are: the JSON document of a page and its versions, export bundles (format numbers already exist), events in the outbox and the queue, jobs in the jobs table, settings stored as JSON, and the shared document of the editing service. Each gets a test that reads what release N wrote with release N+1, and the reverse where a rollback is promised.
- **OPS-071** · built · **High.** The upgrade test also **writes** while the new version migrates: autosave, publish, comment and upload on the old node, then read them on the new one, and the reverse. Reads and logins alone miss the 40% of failures that come through messages.
- **OPS-072** · built · **Medium.** A check in CI refuses migrations that rewrite or lock a table for long: `ALTER COLUMN ... TYPE`, `ADD COLUMN` with a volatile default, `SET NOT NULL` without a `CHECK ... NOT VALID` first, `CREATE INDEX` without `CONCURRENTLY` on a table that holds user data, and `VACUUM FULL` or `CLUSTER`. An exception is a comment on the line with the reason, the same rule as for scanners.
- **OPS-073** · built · **Medium.** Migrations that cannot run in a transaction, such as `CREATE INDEX CONCURRENTLY`, get a header (`-- transaction: none`) and run on their own, with the same lock, a retry, and a check afterwards that the index is valid (a failed concurrent build leaves an invalid index behind). This closes OPS-043.
- **OPS-074** · built · **Medium.** Every migration also runs with a `statement_timeout`, so one that holds its lock longer than planned fails and rolls back instead of holding the site. Background migrations (OPS-035) are the place for long work.
- **OPS-075** · built · **Medium.** Write the version-skew policy down, as Kubernetes does: which versions of the application, the editing service, the queue messages and the export bundle may meet, and for how long. The upgrade test proves exactly that policy.
- **OPS-076** · later · **Low.** Once 1.0 exists, CI also upgrades from the **oldest release that may still jump straight to this one** (decided below: six months), with a short stop allowed. That makes OPS-042 a tested promise instead of a sentence.

## Sources

- Zhang, Yang, Jin, Sethi, Rodrigues, Lu, Yuan: [Understanding and Detecting Software Upgrade Failures in Distributed Systems](https://www.cs.purdue.edu/homes/yonglezh/pub/upgrade-sosp21.pdf), SOSP 2021, [doi:10.1145/3477132.3483577](https://doi.org/10.1145/3477132.3483577).
- Dumitraș, Narasimhan: [Why Do Upgrades Fail and What Can We Do about It?](https://doi.org/10.1007/978-3-642-10445-9_18), Middleware 2009.
- Gunawi et al.: [Why Does the Cloud Stop Computing? Lessons from Hundreds of Service Outages](https://doi.org/10.1145/2987550.2987583), SoCC 2016.
- De Jong, van Deursen, Cleve: [Zero-Downtime SQL Database Schema Evolution for Continuous Deployment](https://pure.tudelft.nl/ws/files/11926973/TUD_SERG_2017_005.pdf), ICSE-SEIP 2017, [doi:10.1109/ICSE-SEIP.2017.5](https://doi.org/10.1109/ICSE-SEIP.2017.5).
- Curino, Moon, Zaniolo: [Graceful Database Schema Evolution: the PRISM Workbench](https://www.vldb.org/pvldb/vol1/1453939.pdf), PVLDB 1(1), 2008.
- Fowler: [Parallel Change](https://martinfowler.com/bliki/ParallelChange.html).
- Stripe: [Online migrations at scale](https://stripe.com/blog/online-migrations).
- GitLab: [Migration style guide](https://docs.gitlab.com/development/migration_style_guide/), [Plan your upgrade path](https://docs.gitlab.com/update/upgrade_paths/).
- Kubernetes: [Version skew policy](https://kubernetes.io/releases/version-skew-policy/).
- Nextcloud: [How to upgrade](https://docs.nextcloud.com/server/latest/admin_manual/maintenance/upgrade.html).
- Sentry: [Self-hosted releases and upgrading](https://develop.sentry.dev/self-hosted/releases/).
- PostgreSQL 16: [Explicit locking](https://www.postgresql.org/docs/16/explicit-locking.html), [lock_timeout and statement_timeout](https://www.postgresql.org/docs/16/runtime-config-client.html), [CREATE INDEX, building indexes concurrently](https://www.postgresql.org/docs/16/sql-createindex.html#SQL-CREATEINDEX-CONCURRENTLY), [ALTER TABLE, notes on rewrites](https://www.postgresql.org/docs/16/sql-altertable.html).
- Xata: [Schema changes and the Postgres lock queue](https://xata.io/blog/migrations-and-exclusive-locks).

All sources of this study are also in [sources.md](sources.md).
