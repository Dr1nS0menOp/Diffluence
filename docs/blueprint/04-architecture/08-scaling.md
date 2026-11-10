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
| Search: the box while typing, and results (spec 028) | 200 people searching, a search each 20 seconds | about 50 |
| **Total** | | **about 180** |

Without search that is about 130, more than twice the old figure, and still not much: one node handles over 1,000 simple requests a second. So **three nodes remain a matter of redundancy, not capacity**. Requests are not the limit. What each request costs in PostgreSQL is.

**A page view is about 20 queries.** The session, the person and their groups, the page, its space with all grants, its restrictions, the version, breadcrumbs, marks, names, then the same permission facts again for the buttons, and two writes for the recent list. At 85 views-related requests a second that is well over 1,000 small queries a second on one primary. PostgreSQL does that, on indexes, with a warm cache. It is also the first thing to trim.

- **ARC-197** · built · `Get` loads permission facts once and reuses them for the buttons, which removes a third of its queries. Built as a memo per read request: opening a page went from 30 queries to 25, a space from 36 to 11 (spec 031). A write gets no memo, so a grant it makes counts at once.
- **ARC-198** · built · The recent list writes at most once a minute per person and page, and trims in a task instead of on each view. Going back to another page still writes at once, so the order stays right (spec 021 D8).
- **ARC-199** · built · The k6 test uses the real pattern of the frontend, with polling, and real spaces of 50,000 pages, not one small page in a loop. The run is in `docs/operations/limits.md` (spec 025).
- **ARC-200** · built · Targets per query go in the test: tree, search, feed, directory. At full size the page and the bell meet them, and the tree and search on a common word do not (`docs/operations/limits.md`).

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

- **ARC-201** · built · Rule: in a transaction, audit comes last, and long work runs as one transaction per page (ARC-064). `pages.Copy` of a large branch moves to a job in wave D, which also removes today's worst case.
- **ARC-202** · built · A test holds a long transaction and measures what waits (spec 025). With more waiting writes than connections in the pool, reading waits too, while `/readyz` says ready.
- **ARC-203** · later · If the lock ever limits real use, events can be written without a hash in the transaction, and a single worker chains them seconds later. The log stays complete and tamper-evident, with a short tail that is not chained yet. That changes ADR-013, so it only happens with a measurement and the product owner's agreement.

### The tree, the lists, the directory

- **ARC-204** · built · The tree loads per branch (SPC-031), and the filter and flat views run on the server (spec 028). Dialogs that list all pages still use the whole tree, only when they open.
- **ARC-205** · built · `perm_visible_pages` is inlined by the planner, so a query can start from its own rows. `perm_hidden_pages` scans all view restrictions for each query. Restrictions are few in normal use, and the matrix test must stay the judge of any faster form. A table of who sees what, kept up to date, is the known answer, and costs a rewrite on each group change. It is the last resort. Built in spec 040, judged by the matrix: `perm_hidden_pages` finds the branches of restricted pages through the index, and `perm_listed_pages` reads pages once. Measured at 100,000 pages with 1,000 restrictions: all visible pages 62 to 41 ms, a page through the function 17 to 3 ms, a space joined to the list 53 to 36 ms. Spec 050 makes the feed, the bell and file search start from their own few rows and ask the rule only about those: on 60,000 pages the feed went from 379 to 20 ms, the bell from 55 to 6 ms, and file names from up to 1.4 s to under 80 ms.
- **ARC-206** · built · Counts in the directory and the overview are worked out per person, because a hidden page must not count. If that gets slow, the fallback is to show a count only up to a limit. Spec 040 made the count of the space directory faster through ARC-205: 215 to 95 ms at 100,000 pages in 50 spaces, still per person. The count walk (PRM-172) proves no hidden page counts.

## 8.3 Topology at each size

| Size | Nodes | Database | Queue | Files |
|---|---|---|---|---|
| Up to a few hundred people | One, `role=all` | One PostgreSQL | Embedded | Disk |
| Thousands | 2 or more `api`, 2 `worker` | Primary and standbys, PgBouncer | NATS, 3 servers | S3 |
| More | Add `api` first, `worker` with job load | Larger primary, before anything clever | Same | Same |

- **ARC-207** · built · An odd number of NATS servers, normally three. An even number costs more and survives no more. Built as advice in `docs/operations/install.md`.
- **ARC-208** · built · API nodes scale by adding more, as they hold nothing. Workers scale by adding more, as they claim work through the database. Built: `deploy/helm/diffluence`, with API and worker pods.
- **ARC-209** · built · PostgreSQL scales up, not out. One instance serves one organisation (ADR-008), so a single primary for writes is the design, and sharding is not. Reads can move out (ARC-228). Built as advice in `docs/operations/install.md`.
- **ARC-210** · phase 3 · The login limiter counts per node, so an attacker gets as many tries as there are nodes. With three nodes that is still slow, and Argon2 makes each try expensive. A shared counter in PostgreSQL is the next step if a test shows a need.

## 8.4 What memory and CPU go to

- **ARC-211** · built · Password hashing with Argon2id is the heaviest thing a request does, by design, and is limited per login.
- **ARC-212** · built · Exports and imports stream, so memory does not grow with the size of a space. Keep that for every new format.
- **ARC-213** · built · A worker that builds a PDF or a site must not hold a whole space in memory. Build per page, and write to a file.

## 8.5 The worst case, and room to grow

Written after the self-test of spec 025 and ADR-036. Section 8.1 assumed 10 percent of the people active, and left out search. These numbers are worked out from that table and from `docs/operations/limits.md`. None of them was measured at this size.

| | Normal: 500 active | Worst: 5,000 active | Worst, with 4 times the database work |
|---|---|---|---|
| Requests a second | about 130 | about 1,300 | about 1,300 |
| Small queries a second | about 1,000 | 10,000 or more | 40,000 or more |
| Searches a second, while typing | 50 to 80 | 500 to 800 | 500 to 800 |
| Writes a second | about 5 | about 50 | about 200 |

**The aim:** future features will ask more of the database, so the design leaves room for **four times today's database work**, with the same people. A feature must fit in that room, or bring its own way of carrying its load.

**What the worst case teaches:**

1. Search is the heaviest thing people do, and it was missing from the load table.
2. One primary cannot carry every read. Reads have to be able to leave it.
3. A full pool of connections stops everything, also what needed one quick query.
4. When the app is overloaded it queues until all is slow, and it still says it is ready.
5. A burst of logins costs far more than a burst of page views, by design.
6. At 200 writes a second the audit lock (ARC-201) comes near what it can pass.

### Keeping load off the primary

- **ARC-228** · partly · Reads can go to a standby. The code names each read as *must be current* or *may lag*. Permission checks, sessions and a person's own just-saved work stay on the primary. Lists, reports, feeds and analytics may lag, by a limit that is set and watched. Past that limit they go back to the primary. Built: the mark (`db.WithMayLag`, `db.MustBeCurrent`), set on search, reports, exports, the feed, the people list and the recent list. Permission facts mark themselves current inside it, and the gauge `diffluence_db_queries_may_lag_total` counts what could move (spec 039). Still open: the standby, the limit and the way back, which need real servers.
- **ARC-229** · phase 3 · Search leaves the primary, to a standby or to its own engine (ADR-035). Whichever it is, a permission change counts at once: results are checked against current permissions before they are shown. This changes ARC-186.
- **ARC-230** · phase 3 · Heavy reads never run on the primary at a large size: analytics, reports, exports and the care report go to a standby, or to tables a job fills.
- **ARC-231** · partly · What many requests ask for is kept in memory for a short time: a person's groups, a space and its grants, the first level of a tree. An event ends the entry at once, and a short maximum age covers a lost event. A cached permission may never outlive its change by more than that age, which is stated and tested. Built in spec 040 for a person's facts and a space with its grants: kept while `permission_generation` is unchanged and at most 30 seconds, so a change ends them at once on every node (PRM-047). A check of a space went from 1.03 ms to 0.19 ms. The first level of a tree is not kept: it changes with every page that is made, moved or saved.
- **ARC-232** · later · Tables that only grow are split by month: audit, page views, notifications, the outbox. Old parts can move to cheaper storage and be dropped by a retention rule. Do this before such a table slows its own queries, not after. The audit chain must stay verifiable across the parts.

### Staying up under too much load

- **ARC-233** · built · Work has an order: **saving comes first, then reading, then search and reports.** Each class has its own share of the connections, so a flood of searches can never take the connection a save needs. Built: three classes, sorted before the first query of a request. Saving may use the whole pool, reading leaves a quarter for saving, and search and reports use a quarter that also counts as reading (spec 031). The classes count requests, which stand in for connections because nearly every handler holds one at a time.
- **ARC-234** · built · Each class has a limit on what waits. Past it the app answers at once with 503 and `Retry-After`, and the screen says the system is busy. Text being written stays in the browser, as it does today, so nothing is lost. Built for every class: saving waits up to 15 s, reading 3 s, search and reports 1 s, each with a limit on how many wait (spec 031). The screen shows the message of the refusal.
- **ARC-235** · built · `/readyz` tells the truth under load: a node with a full pool or a long queue says so, so a load balancer can send people elsewhere. It stays separate from `/livez`, so a busy node is not restarted (ARC-009). Built: the answer is `busy` when every connection is taken, checked before the ping, or when saving or reading has more than half its limit waiting (spec 031).
- **ARC-236** · built · Logins have their own limit on how many hash at once per node, so a morning rush waits briefly and leaves the rest of the node alone. Built: half the cores, at least one (spec 031).
- **ARC-237** · later · If writes come near what the audit lock passes, events are chained after the commit (ARC-203). That needs a measurement and the product owner's agreement, because it changes ADR-013.

### Keeping the room

- **ARC-238** · built · Every feature has a **load budget**. Its spec states the queries per request, the writes per action, and which class of ARC-233 it belongs to. A feature that goes past its budget gets a cache, a job or a table of its own before it lands. Built: a section in the spec template, `docs/specs/README.md` (spec 031).
- **ARC-239** · built · The query count of the main screens is a test: opening a page, the tree, search and the bell. A change that adds queries fails, unless the budget is changed on purpose. Built: `internal/app/query_budget_test.go`, which also counts the summary of a page and a space, through a counter on the pool that is the metric `diffluence_db_queries_total` as well (spec 031).
- **ARC-240** · phase 3 · Each release runs the load test at a fixed size, and `docs/operations/limits.md` keeps the numbers, so a slow decline shows up. A run of the worst case needs real servers, so it is done when those are available. Until then, the documents do not claim it.
- **ARC-241** · phase 3 · The install guide gives sizes for each number of people, and says which were measured and which are estimates.

### When to do what

Nothing here is built ahead of need, but the code keeps the door open from now on: a read says what it needs, and a feature states its budget.

| Sign | Step |
|---|---|
| Any size | Load budget, query test, order of work, honest `/readyz` (ARC-233 to 235, 238, 239) |
| Search slows other work | Search leaves the primary (ARC-229) |
| The primary's CPU stays above half at busy hours | Reads and heavy reads to standbys (ARC-228, ARC-230) |
| The same reads dominate | Short cache (ARC-231) |
| A growing table slows its own queries | Split by month (ARC-232) |
| Writes near the audit limit | Chain after commit (ARC-237) |
