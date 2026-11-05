# 04.7: Data and storage

Tables and the permission model are in blueprint 05. This chapter is about how they behave as a system.

## 7.1 PostgreSQL

- **ARC-172** · built · One database holds content, permissions, audit, outbox, jobs and sessions. One backup then covers one consistent moment.
- **ARC-173** · built · Migrations are numbered files, expand or contract, with a five-second `lock_timeout` and a retry. A contract migration waits until every live node knows it.
- **ARC-174** · built · All SQL is parameterised, by hand, without an ORM or generator (ADR-016).
- **ARC-175** · partly · The connection pool is `pgxpool`, tuned by the URL. Still open: its default is small, the larger of four and the number of CPUs, so a node under load queues in the pool. The install guide must set `pool_max_conns`, and phase 3 measures it.
- **ARC-176** · phase 3 · **PgBouncer in transaction mode** needs care. pgx uses named prepared statements by default, so PgBouncer must be 1.21 or newer with `max_prepared_statements` above zero, or the URL sets `default_query_exec_mode=exec`.
- **ARC-177** · built · Nothing in the application uses `LISTEN` or session settings, which transaction pooling never supports. `SET LOCAL` and transaction advisory locks are fine. Keep it that way.
- **ARC-178** · phase 3 · **Migrations must not go through PgBouncer in transaction mode.** The runner holds a session advisory lock (`pg_advisory_lock`), which that mode never supports. `DIFFLUENCE_MIGRATE_DATABASE_URL` points straight at PostgreSQL, and the install guide says so. A preflight check can detect a wrong setup.
- **ARC-179** · phase 3 · Reads stay on the primary. A replica lags, and a person who was just restricted must not read from a stale copy. Analytics and exports may use one later, with care.
- **ARC-180** · built · Long work in a transaction blocks others, so there is one rule: a transaction does one thing for one object.

## 7.2 What grows

Rough figures for 5,000 people over five years. They are to plan with, not measurements.

| Table | Grows with | Rows | Kept | Plan |
|---|---|---|---|---|
| `page_versions` | Publishing | Millions, tens to hundreds of GB | For ever | The largest table. Stored compressed by PostgreSQL, and only the current one is read often |
| `audit_events` | Every change | Tens of millions | By policy (spec 013 D11) | Prune with a signed anchor. Partition by month if it passes memory |
| `search_documents` | Pages | One per page | Follows pages | GIN index |
| `notifications` | Fan-out | Millions at any time | 90 days | Partition by month when deleting hurts, and drop a month at once |
| `outbox_events` | Every change | Days of changes | 7 days | Deleted in the hour task |
| `recent_pages` | Reading | 50 per person | Capped | Fine |
| `page_view_*` | Reading | Small per day | By setting | Days are summed, and details are dropped |
| `jobs`, `mail_queue` | Work | Moderate | 90 days | A periodic task must remove old rows. None does yet |
| `sessions` | Logins | Small | Until expiry | Removed by `purge-identity` |

- **ARC-181** · wave C · Every table that grows without end has a named keeper: a periodic task or a retention setting. A new table without one fails review.
- **ARC-182** · phase 3 · Partitioning waits for a table larger than memory, which is the rule of thumb in the PostgreSQL manual. It pays when old rows go: dropping a partition is far cheaper than a bulk delete with its vacuum.
- **ARC-183** · later · Partitions stay few. PostgreSQL handles a few thousand well when queries prune them, and months give us tens.

## 7.3 Search

- **ARC-184** · built · Search is PostgreSQL full text with a GIN index, the type the manual prefers, and the `simple` configuration, so every language behaves alike.
- **ARC-185** · built · The index is written in the transaction that publishes. A page is never found with old content or old permissions (ORG-013).
- **ARC-186** · partly · Blueprint 04 calls search pluggable. It is not: `search.Index` runs in the caller's transaction, and queries join `perm_visible_pages`. That is what gives ORG-013. An outside engine could not offer the same promise, and would lag, and would need its own permission filter. Still open: nothing, unless phase 3 measures a need. If so, the choice is between that promise and speed, and it is the product owner's.
- **ARC-187** · phase 3 · A common word can match most pages, and `ts_rank` must read them all before `LIMIT` helps. Phase 3 measures this. The cheap answer is to rank only the newest matches past a limit.

## 7.4 Files

- **ARC-188** · built · `storage.Store` has four methods: `Put`, `Open`, `Delete`, `Healthy`. Disk is the first store.
- **ARC-189** · built · A file on disk is written to a temporary name first, so a failed upload leaves nothing.
- **ARC-190** · built · Removing a file is queued in the same transaction as the delete (`storage_deletions`), and worked off later. A crash can never leave a file behind for good.
- **ARC-191** · wave F · `Store` gains `List`, to find files nothing points to.
- **ARC-192** · phase 3 · The S3 store implements the same interface. `Open` needs a reader that seeks, so range reads replace the local file. S3 reads its own writes at once, so an upload can be read straight after.
- **ARC-193** · phase 3 · Files always stream through the API, with no presigned addresses. A presigned address would skip the permission check, the audit of a download, and our locked-down headers. API nodes carry the bytes, and that is the price. See the trade-offs in chapter 9.
- **ARC-194** · partly · The column for encryption keys exists and `internal/keys` can seal small values. Still open: files are not encrypted yet (ADR-011, phase 3).
- **ARC-195** · wave E · Derived files have their own prefix: `previews/`, `exports/`, later `collab/`. They can always be made again, so a backup may leave them out.
- **ARC-196** · wave E · Keys of derived files hold a hash of the source, so one preview serves all copies of the same file.
