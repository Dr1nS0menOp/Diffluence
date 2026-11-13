# Spec 061: reads fast on real data

Status: built (wave PF, 12 November 2026).

## Why

The second stress test (`docs/reports/2026-11-12-stress-test-2.md`) ran on the seeded data of the server: 5,000 people, 384 spaces, 60,000 pages. The feed did not answer within the request time limit of a minute at any level of load, although spec 050 had measured 20 ms on generated data. Reads that cost a few milliseconds in development cost 40 to 260 ms there, and PostgreSQL used every core from 50 active people.

Generated data was too small and too even to show what the planner does. This wave works from a dump of the real data set and measures every change on it.

## How it was measured

A `pg_dump` of the seeded database, made on the server after the second stress test and copied off it, restored into a separate PostgreSQL 16 with `shared_buffers` 2 GB, then `ANALYZE`: 5,001 people, 384 spaces, 60,385 pages, 115,022 versions, 1.7 GB. The data holds no guest, so the users measured are:

- **member**: an ordinary member who sees 201 spaces and 38,706 pages, watches 2 pages and follows nobody;
- **member in the most groups**: used by the repeatable check, watches 98 pages;
- **watcher**: the administrator, who made the data, sees every page and watches all 60,384.

Plans were read with `EXPLAIN (ANALYZE, BUFFERS)`, one at a time, warm.

## What the plans showed

| Finding | Effect | Cause |
|---|---|---|
| F1. The feed's "for you" tab never finished | over 120 s, for every person | It read every version newest first and asked of each whether the person watches or follows it. With two watched pages, almost all of 115,022 versions were read and dropped, each with its checks. |
| F2. A generic plan of "recent" never finished | over 120 s, against 233 ms with the values | pgx prepares each statement. After five runs PostgreSQL may keep a generic plan, which cannot see that the space, the cursor and the "for you" filter are empty. The stress test ran each statement thousands of times, so it always met the generic plan. |
| F3. "Recent" with its values took 233 ms | 45 candidates × 6.4 ms | The rule `perm_visible_pages` was inlined and the person's spaces worked out again for every candidate version. |
| F4. JIT added more than half a second | "for you" 700 ms with JIT, 47 ms without | PostgreSQL compiles a plan whose estimate passes `jit_above_cost`. The estimates of these queries are large and the work is small. |
| F5. "Popular" took about 320 ms | 20,000 active pages | It counted reactions and comments page by page, two counts per page, then read the excerpt of every scored page before keeping 20. |
| F6. The other reads are fast alone | tree 36 ms, storage 3 ms, bell 0.2 ms, spaces 6.6 ms | They were slow in the stress test because the feed held the cores. |

## What changed

1. **Pool settings** (`internal/db/db.go`):
   - `plan_cache_mode = force_custom_plan` (F2);
   - `jit = off` (F4).

   Planning with the values costs about a millisecond a query. Search already set the first per transaction (spec 028).
2. **A limit on every query of a read.** `db.WithQueryLimit` puts a limit in the context, and the pool's tracer runs each query under a deadline. pgx cancels the statement on the server when it passes. Reads (`GET`, `HEAD`) get 15 seconds through `withQueryLimit`. A query that runs out answers 503 "busy" with `Retry-After`, as a full pool does (spec 050). Writes get no limit, so a change is never cut off halfway; background work gets none either, because a long export or check is expected there.
3. **"For you" gathers its pages first** (F1). It collects:
   - the pages a person watches;
   - the pages of spaces they watch;
   - the pages below branches they watch;
   - the pages of people they follow.

   Each gives its newest version that was not silent. The final filter is the same as before.
4. **The visibility rule is built once per query** (F3). `visible()` wraps the `IN` in an expression, so PostgreSQL builds the set of visible pages once, as a hash (about 30 ms), and keeps reading the newest versions first. A plain `IN` or `EXISTS` either repeats the rule per candidate or, as a `MATERIALIZED` set, gives up the newest-first walk (360 ms for the watcher). `perm_visible_pages` stays the only rule.
5. **"Popular" counts in one pass** (F5) and reads excerpts for the top pages alone.

The permission rule is unchanged. `TestTheFeedGivesWhatTheFormerQueryGave` compares every tab, row by row, with the former query. It now covers every way into "for you": a followed person, a watched space, and a watched branch seen by someone who may view the space.

## Results on the real data

| Query | Before | After | Budget |
|---|---|---|---|
| Recent, member | 233 ms (generic plan: over 120 s) | 43 ms | 150 ms |
| Recent, watcher | 79 ms | 35 ms | 150 ms |
| For you, member | over 120 s | 73 ms | 500 ms |
| For you, watcher of every page | over 120 s | 233 ms | 500 ms |
| Popular | about 320 ms | 107 to 187 ms | 250 ms |

The feed numbers are the median of five runs through `feed.Service` and pgx, with the pool settings above.

"Popular" gets a larger budget because it counts a week of activity: 32,000 comments on that data. A person who watches every page is the worst case of "for you", and only the administrator who made the data comes close.

## The repeatable check

`scripts/stress/real-data.sh <dump>` restores a dump into a throw-away PostgreSQL 16 on the toolbox's network and runs `TestTheFeedIsFastOnRealData`, which fails when a tab passes its budget. Without `DIFFLUENCE_REALDATA_URL` the test skips, so `make check` is unchanged. The dump holds generated people and pages only, but it stays out of the repository. A change to a hot read is measured this way as well as on generated data.

## Load budget (ARC-238)

On the real data, a feed costs at most one build of the visible set (about 30 ms) plus a short walk. Every other read of the table above stays as it was. The pool settings add about a millisecond of planning a query and remove JIT's half second. No read query can run past 15 seconds.

## Not proven

- **The load test on the server with these changes.** Every number here is one query at a time.
- **A guest.** The seeded data has none.
- **The admission and limit together at full load.** The 15-second limit is proven on one query (`TestAQueryOfARequestStopsAtItsLimit`); the step from the middleware to a 503 has no test of its own.
- **Index on `page_versions(author_id)`.** "For you" reads the versions of followed people with a scan, which is cheap on this data (nobody follows anyone in it). An index would need a migration; it is left until following is measured on real data.

## Decided in this wave

- **D1, custom plans for the whole pool rather than per query.** Many reads have optional filters, and the generic plan is wrong for all of them the same way. A millisecond of planning is far below what a wrong plan costs.
- **D2, JIT off for the whole pool.** No query of this application runs long enough for compilation to pay.
- **D3, a 15-second limit for reads only.** A write cut off halfway would roll back work a person thinks is saved, and background work has nobody waiting.
- **D4, the popular budget is 250 ms**, because it reads a week of activity, as said above.
