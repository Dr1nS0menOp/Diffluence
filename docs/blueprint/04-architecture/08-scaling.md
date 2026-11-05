# 04.8: Scaling to thousands of people

The goal from blueprint 01: thousands of users per instance, and pages under 300 ms (p95) at 5,000 simulated users. Blueprint 07 worked out 50 requests a second. This chapter redoes that for what the product does now.

## 8.1 The load, worked out

5,000 people, 10% active at once, is 500 active. A page view today is not one request.

| Source | Per active person | Requests a second |
|---|---|---|
| Opening a page: page, space, tree, shortcuts, children | 5 requests, every 30 seconds or so | about 85 |
| Count of notifications | 1 per 30 seconds | about 17 |
| Presence on an open page | 1 per 20 seconds | about 25 |
| Writing: autosave, publish, comments | Small part of the people | about 5 |
| **Total** | | **about 130** |

That is more than twice the old figure, and still not much: one node handles over 1,000 simple requests a second. So **three nodes remain a matter of redundancy, not capacity**. Requests are not the limit. What each request costs in PostgreSQL is.

**A page view is about 20 queries.** The session, the person and their groups, the page, its space with all grants, its restrictions, the version, breadcrumbs, marks, names, then the same permission facts again for the buttons, and two writes for the recent list. At 85 views-related requests a second that is well over 1,000 small queries a second on one primary. PostgreSQL does that, on indexes, with a warm cache. It is also the first thing to trim.

- **ARC-197** · phase 3 · `Get` loads permission facts once and reuses them for the buttons, which removes a third of its queries.
- **ARC-198** · phase 3 · The recent list writes at most once a minute per person and page, and trims in a task instead of on each view.
- **ARC-199** · phase 3 · The k6 test uses the real pattern of the frontend, with polling, and real spaces of 50,000 pages, not one small page in a loop.
- **ARC-200** · phase 3 · Targets per query go in the test: tree, search, feed, directory.

## 8.2 What will hurt first

In order of how much it worries this study.

| # | What | Why | Answer |
|---|---|---|---|
| 1 | **The audit chain** | Every change takes one global lock, until commit | ARC-201 to 203 |
| 2 | **Fan-out without care** | One publish in a watched space can cost thousands of permission checks | ARC-058, ARC-059 |
| 3 | **One job at a time** | A long import holds up all jobs on a worker | ARC-063 |
| 4 | **The full tree** | A space of 50,000 pages sends everything on each load | ARC-204 |
| 5 | **Visible pages in every list** | `perm_visible_pages` works per person, and every list uses it | ARC-205 |
| 6 | **The directory and the feed** | They are worked out on each request from large tables | ARC-206 |
| 7 | **Search on common words** | Ranking reads every match | ARC-187 |
| 8 | **Files through the API** | Every byte passes a node | ARC-167, ARC-193 |

### The audit chain

`audit.Record` takes `pg_advisory_xact_lock` and reads the last hash. That lock is held **until the transaction commits**, so all changes in the whole instance pass through one door, including the commit itself. Logins and refused requests write audit events as well.

With a commit of a few milliseconds that door passes a few hundred changes a second, and we expect five. So normal use is fine. The danger is a **long transaction that audits early**: until it commits, nobody can log in, publish or be refused. A copy of a large branch does this today: it records per page, in one transaction.

- **ARC-201** · wave D · Rule: in a transaction, audit comes last, and long work runs as one transaction per page (ARC-064). `pages.Copy` of a large branch moves to a job in wave D, which also removes today's worst case.
- **ARC-202** · phase 3 · A test holds a long transaction and measures what waits, and the load test watches the wait on this lock.
- **ARC-203** · later · If the lock ever limits real use, events can be written without a hash in the transaction, and a single worker chains them seconds later. The log stays complete and tamper-evident, with a short tail that is not chained yet. That changes ADR-013, so it only happens with a measurement and the product owner's agreement.

### The tree, the lists, the directory

- **ARC-204** · phase 3 · The tree loads per branch (SPC-031). It changes the filter, which then needs the server, and `reveal`. Measure first: 50,000 rows of about 200 bytes is 10 MB before compression.
- **ARC-205** · phase 3 · `perm_visible_pages` is inlined by the planner, so a query can start from its own rows. `perm_hidden_pages` scans all view restrictions for each query. Restrictions are few in normal use, and the matrix test must stay the judge of any faster form. A table of who sees what, kept up to date, is the known answer, and costs a rewrite on each group change. It is the last resort.
- **ARC-206** · phase 3 · Counts in the directory and the overview are worked out per person, because a hidden page must not count. If that gets slow, the fallback is to show a count only up to a limit.

## 8.3 Topology at each size

| Size | Nodes | Database | Queue | Files |
|---|---|---|---|---|
| Up to a few hundred people | One, `role=all` | One PostgreSQL | Embedded | Disk |
| Thousands | 2 or more `api`, 2 `worker` | Primary and standbys, PgBouncer | NATS, 3 servers | S3 |
| More | Add `api` first, `worker` with job load | Larger primary, before anything clever | Same | Same |

- **ARC-207** · phase 3 · An odd number of NATS servers, normally three. An even number costs more and survives no more.
- **ARC-208** · phase 3 · API nodes scale by adding more, as they hold nothing. Workers scale by adding more, as they claim work through the database.
- **ARC-209** · phase 3 · PostgreSQL scales up, not out. One instance serves one organisation (ADR-008), so a single primary is the design, and sharding is not.
- **ARC-210** · phase 3 · The login limiter counts per node, so an attacker gets as many tries as there are nodes. With three nodes that is still slow, and Argon2 makes each try expensive. A shared counter in PostgreSQL is the next step if a test shows a need.

## 8.4 What memory and CPU go to

- **ARC-211** · built · Password hashing with Argon2id is the heaviest thing a request does, by design, and is limited per login.
- **ARC-212** · built · Exports and imports stream, so memory does not grow with the size of a space. Keep that for every new format.
- **ARC-213** · wave G · A worker that builds a PDF or a site must not hold a whole space in memory. Build per page, and write to a file.
