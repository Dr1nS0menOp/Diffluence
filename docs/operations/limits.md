# Known limits

What was tried to break Diffluence, where it gave way, how it fails, and what to set. Spec 025 holds the plan.

This is a self-test by the people who built the app. It is **not** an independent penetration test. A limit that was not reached is written as "not reached at N", which never means safe.

## How it was measured

- One node with `role=all`, in a read-only container without capabilities, and PostgreSQL 16 with its normal crash safety, on 6 November 2026.
- Limits on the containers: the app had 2 CPUs and 1.5 GB, PostgreSQL 1.5 CPUs and 2 GB, the load tool 0.5 CPU and 0.5 GB. The pool had 20 connections.
- One Windows machine with WSL, which also ran other builds. All connections stayed on that machine.
- One space of 50,000 pages, made through the API, 120 words each, 3 levels deep. Every page used the same 19 words, so a common word matches nearly every page. That is the worst case for search, and your text will be kinder.
- One account for all readers, an administrator. People with many groups and restrictions were not measured.

These numbers hold for this machine and these limits, not for yours. Use them to see what gives way first, and test your own.

## Load

| What | Tried | Result | How it fails | What to set |
|---|---|---|---|---|
| Reading a page, with the bell | 50, 200, 800 readers | 758 requests a second at 800 readers, p95 1.03 s, no errors. At 200 readers: 326 a second, p95 19 ms | Slower, no errors | The goal of 130 a second for 5,000 people is met for this kind of request |
| Search on a word in every page | 5, 20 people | **About 3.5 searches a second.** At 20 people each search takes 4.5 s | Slower, no errors. All other database work waits with it | Fixed in spec 028: see below |
| Search on a rare word | 5, 20 people | 16 a second, p95 52 ms. Ceiling not reached | | |
| Mixed readers with search and feed | 50 readers | p95 7.2 s, all from the search above | Slower | As above |
| The page tree of 50,000 pages | 5, 20, 60 people | **19 MB per load**, 4 loads a second, 13 s each at 60 people, 567 MB of memory at 20 | Slower, no errors. Not measured in a browser | Fixed in spec 028 (SPC-031): see below |
| Open page, feed, overview | One person, 50,000 pages | 9 ms, 88 ms, 119 ms | | |
| Publishing | 6 at once | 143 pages a second on an empty space, 70 at 50,000 pages, p95 121 ms | Slower | |
| Connections that say nothing | 500, 2,000, 8,000 held | All dropped after 10 seconds, others not disturbed, 154 MB. Ceiling not reached at 8,000 | | A proxy still comes first |
| Upload | 64 MB, and past the limit | 64 MB in 1.9 s, memory did not grow. Past the limit the connection is closed early | | Set the limit in the console |
| Export of 50,000 pages | Text, HTML | 5 and 6 seconds, 237 MB at most | | |

## Search, reading and saving at once (spec 028)

Measured on 7 November 2026, with the same caps, 50,000 pages, 5,000 people, 52,000 files and 20,000 comments. Reading is the mix of blueprint 04.8. The model is 130 reads, 2 saves, 40 searches while typing and 10 results pages a second: about 5,000 people, 10% active, 40% of the time in search.

| What | Result | How it fails | What to set |
|---|---|---|---|
| The model, PostgreSQL 1.5 CPUs | Reading p95 168 ms, nothing failed. Search p95 1.0 s, 1 to 3% told it is busy | Search waits for its share of the connections, then answers 503 with `Retry-After` | Give PostgreSQL at least 2.5 CPUs for this many people |
| The model, PostgreSQL 2.5 CPUs | Search p95 44 ms, typing p95 32 ms, reading p95 19 ms, nothing failed | | |
| Twice the search, PostgreSQL 2.5 CPUs | Reading p95 87 ms, nothing failed. 5 to 10% of searches busy | Search, not reading | A standby for search (ARC-229) |
| The newest pages hold none of the words | Each search 32 ms in the database. At 1.5 CPUs, 56% of searches busy | As above | As above |
| Opening the tree of 31,000 pages | 26 ms and 0.08 MB, against 275 ms and 13 MB | Show more past 200 rows a level | |
| Comments as the only filter, a common word | 678 ms | Slow, alone | |
| A wildcard inside a word | 219 ms | Slow, alone | |

Search can never take more than one connection in four, so a flood of searches leaves the rest for reading and saving. It shares the CPU of PostgreSQL, though: under a flood, reading slows. A standby takes that away.

### Word forms (spec 029)

Since spec 029 each word also asks for its common forms, such as `plans` for `plan`. Measured on 7 November 2026 in PostgreSQL alone, at 1.5 CPUs, on 50,000 synthetic documents of 120 words each with a GIN index. This was not the full setup of spec 028: the application and the load model were not part of it.

| Query | As typed | With its forms |
|---|---|---|
| A word in every document, as a prefix | about 10 ms | about 10 ms |
| A rare word | 0.2 to 0.4 ms | 0.1 to 0.2 ms |
| Two words, the newest ten | 0.3 ms | 0.6 ms |

The forms cost the index little. A full run with the setup of spec 028 is still open.

### Part of a word and small typos (spec 047)

Titles and file names are also matched by trigrams (`pg_trgm`), after the words and only when the words leave room: a name that holds the typed text from three letters, a typo from five. Measured on 10 November 2026 in PostgreSQL 16 alone, in the development container, on 50,000 titles and 52,000 file names made of 30 common words, with the two GIN indexes of migration 0043. The application and the load model of spec 028 were not part of it.

| Query | Result |
|---|---|
| Size of the indexes | 2.9 MB for titles, 2.4 MB for file names |
| A title that holds the typed text ("udget"), 5,000 matches | 4 ms, through the index |
| A typo of a word in a tenth of all titles ("handbok"), 5,000 near matches sorted by closeness | 48 ms, through the index |
| A typo that matches nothing | 0.08 ms |
| A file name that holds the text or is near it ("forcast") | 47 ms, through the index |

A typo of a very common word is the costly case, because every near title is ranked. It runs inside the search share of one connection in four and the 3 second statement time. The target of spec 047, the quick box at p95 under 50 ms at the model on 2.5 CPUs, is still to be measured with the full setup and on the stress server.
## An order of work under too much load (spec 031)

Measured on 8 November 2026, 50,000 pages, the same caps: the app 2 CPUs and 1.5 GB, PostgreSQL 1.5 CPUs and 2 GB, the load tool 0.5 CPU and 512 MB, a pool of 20. "Before" is main before spec 031, "after" is with it, on the same data, one after the other. Another build ran on the machine at the same time, the same for both. Each row is one run of 40 seconds, so take differences of a few percent as noise.

| What | Before | After | How it fails now |
|---|---|---|---|
| The model of spec 028 | Reading p95 146 ms, saving p95 278 ms, search p95 1.0 s, nothing failed | Reading p95 31 ms, saving p95 100 ms, search p95 100 ms, 1% of quick searches busy | |
| 600 page views a second, 2 saves | Served 378 a second, the load tool dropped 8,700 it could not send. Reading p95 2.7 s, saving p95 417 ms. `/readyz` ready the whole time | Served 599 a second. 30% of reads answered busy at once, the rest p95 303 ms. Saving p95 186 ms, none failed. `/readyz` busy in 34 of 38 checks | Reading past its share: 503 with `Retry-After: 2` |
| 400 page views, 150 quick searches, 40 results pages a second, 2 saves | Reading p95 3.3 s, 84 to 90% of searches busy after waiting a second, saving p95 500 ms | Reading p95 338 ms with 14% busy, 89% of searches busy at once, saving p95 196 ms, none failed. `/readyz` busy in 28 of 38 checks | Search first, then reading |

Saving never failed in any run, and got faster under every flood. The memo of permission facts (ARC-197) is in the "after" numbers as well, so the model got faster for that reason too.

**What to know when you run behind a load balancer.** A node says `busy` on `/readyz` while its queues are long. With several nodes the balancer sends people to a node with room. If every node is busy at once, a balancer that only uses ready nodes may have none left, and Kubernetes then sends nobody anywhere. Give the balancer a rule for that case, such as keeping all nodes when none is ready, or let your readiness probe tolerate a few failures in a row.

## A real server (10 November 2026)

The first run on a real server, with 5,000 seeded people, 300 spaces with their own groups and permissions, and 60,000 pages: [the stress test](../reports/2026-11-10-stress-test.md). It corrects the picture above, which used one space and one account: with a real permission model, **PostgreSQL ran out of CPU from about 50 active people** on 8 cores of a CPU from 2012, and turned the extra work away as busy, searches first. Reading a page stayed at about 30 ms and saving at about 60 ms at every level, and no answer was wrong.

| What gave way | On the server | Cause |
|---|---|---|
| Search on file names | 0.3 to 0.9 s a search | The permission check ran again for every candidate file |
| The feed | 1.1 s at 20 people, 2 s at 200 | It worked out every visible page and its versions before keeping 20 |
| The count on the bell | 80 ms each poll | It worked out every visible page on every poll |
| Saving, all 5,000 at once | 3.7 s (p50) | The audit lock, held until a commit is on a disk that syncs about 160 times a second |

### What spec 050 changed

The three queries now ask the permission rule only about the few pages they could show. Measured on 10 November 2026 in PostgreSQL 16 alone, in the development container, on data of the same size made by `scripts/stress/bench-data.sql` (5,000 people, 300 spaces of which 250 open to all, 60,000 pages in trees seven levels deep, 120,000 versions, 300 restricted branches, 12,000 files, 250,000 notifications), for one member who sees 250 spaces. One run each, warm.

| Query | Before | After |
|---|---|---|
| The feed, recent | 379 ms | 20 ms |
| The count on the bell | 55 ms | 5.8 ms |
| File names, a common word | 268 ms | 41 ms |
| File names, a rare word | 277 ms | 18 ms |
| File names, near a typo | 1,363 ms | 78 ms |

The answers are the same: a test compares the new feed with the former query, row by row, for several people and every tab, and the permission tests stay as they were. **The full load test has not been repeated on the server with these changes.** Until it is, the table of the server above is the last measured state.

The audit lock is not changed. A shorter lock would need another design of the chain (ADR-013, ADR-039), which this wave did not prove safe. On a disk that syncs fast it costs little: see the install guide.

### What spec 052 changed

A save no longer takes the audit lock: it writes its event to `audit_pending`, and a worker seals waiting events into the chain every second (ADR-041). Measured on 11 November 2026 in the development container (16 CPUs, a pool of 20), with `TestMeasureSavesUnderTheChainLock`: 32 writers save 1,280 times at once, each save one transaction with one audit event, three runs.

| Saves | Before (lock held to commit) | After |
|---|---|---|
| Saves a second | 1,940 to 2,060 | 16,100 to 18,200 |
| p50 | 15.1 to 15.8 ms | 1.4 to 1.5 ms |
| p95 | 17.4 to 18.4 ms | 2.7 to 3.0 ms |
| p99 | 27.0 to 29.0 ms | 9.1 to 12.9 ms |

The database of the development container runs with `fsync` and `synchronous_commit` off, so no commit waits for the disk there. That is the step the lock was held across, so on a real disk the old way is slower still and these numbers understate the gain. **The stress test has not been repeated on the server with this change.** Until it is, the 3.7 seconds above is the last measured state for all 5,000 at once.

### Measured on the real data set (spec 061)

The second stress test (12 November 2026) found the generated data of spec 050 misleading: on the seeded data of the server the feed did not answer within a minute. Every hot read is now also measured on a dump of that data (5,001 people, 384 spaces, 60,385 pages, 115,022 versions, 1.7 GB), restored into PostgreSQL 16 with `scripts/stress/real-data.sh`, with nothing else running. "Member" is an ordinary member in the most groups; "watcher" is the person who watches the most pages, here every page, and who sees everything.

| Query | Before | After |
|---|---|---|
| Feed, recent, member of 201 spaces | 233 ms; over 2 minutes once pgx used a generic plan | 43 ms |
| Feed, recent, watcher | 79 ms | 35 ms |
| Feed, for you, member | over 2 minutes | 73 ms |
| Feed, for you, watcher of 60,384 pages | over 2 minutes | 233 ms |
| Feed, popular, 32,000 comments in the week | about 320 ms | 187 ms (member), 107 ms (watcher) |
| The page tree of the largest space (3,978 pages) | 36 ms | not changed |
| The storage still free in a space | 3 ms | not changed |
| The count on the bell | 0.2 ms | not changed |
| The spaces a person may see | 6.6 ms | not changed |

The feed is the median of five runs through `feed.Service`; the other rows are single runs of `EXPLAIN ANALYZE`. The last four were 38 to 260 ms in the stress test only because the feed held the database's cores: alone they are fast.

What changed, besides the feed itself:

- **Every query is planned with its values** (`plan_cache_mode = force_custom_plan` on the pool). pgx prepares statements, and after five runs PostgreSQL may keep a generic plan, which cannot see that an optional filter is empty. Planning costs about a millisecond a query.
- **No JIT** (`jit = off` on the pool). PostgreSQL compiled any plan whose estimate looked large, which cost half a second or more each time, on queries that take milliseconds.
- **No read holds the database for long.** Each query of a read stops after 15 seconds and the person is told the server is busy, as when no connection is free. Writes are not limited, so a change is never cut off halfway.

Budgets on that data, checked by `TestTheFeedIsFastOnRealData` when it runs against the dump: recent 150 ms, for you 500 ms, popular 250 ms. **The stress test has not been repeated on the server with these changes.**

## What stops everything

| What | Result | What to do |
|---|---|---|
| Something holds the audit lock | Since spec 052 no save takes it: only the sealer, the upgrade marker, a backup note, a prune and a release from before ADR-041 do. While it is held, events wait unsealed, and saves go on | Nothing for saves. If events wait more than a minute, System health says so: check that a worker runs |
| Many changes at once that each need a second connection | Since spec 050, work that holds a connection reads through it instead of asking the pool for another, so creating many spaces at once finishes, and any wait that is left ends as busy within 10 seconds | Nothing to set |
| The disk of the data folder is full | Uploads answer 500 "Something went wrong", no half file stays, text still saves, and `/readyz` still says ready | Watch the disk yourself |
| An account that gets 100 wrong passwords from many addresses in 15 minutes | That person cannot log in until the window ends | This limit stops a botnet from guessing. Passkeys and an identity provider do not depend on it |
| The address of the proxy is not in `DIFFLUENCE_TRUSTED_PROXIES` | Everyone counts as one address, so 50 failed logins in 15 minutes stop all logins | Set it |

## Failures under load

Six people reading and writing the whole time.

| What was killed | What people saw | Recovery |
|---|---|---|
| The app, restarted after 2 s | Failed requests for 3 seconds, as nothing else was there | At once. Nothing lost |
| The app, stopped normally | Failed requests for 1 second, as there was one node | At once |
| PostgreSQL, restarted after 5 s | "Please try again shortly" for 7.6 seconds | By itself. No event stayed unsent |
| The app during a job | The job stays "running" for up to 70 minutes, and blocks the next export of that person | `fail-stale-jobs`, or Retry in the console |
| PostgreSQL out of memory, during attacks | Errors, then back by itself | By itself |

## Attacks that held

- Every route, as a stranger, a guest, a person of another space and a reader, also on a hidden page: nothing passed and nothing leaked. Lists, blocks and search gave nothing of a hidden page.
- Cross-site writes, forged origins, method override, request smuggling, addresses and headers of 2 MB.
- JSON nested 200,000 deep, a document nested 3,000 deep, links with `javascript:` in many spellings, null bytes, broken UTF-8.
- Path traversal in addresses, file names and archives.
- HTML, SVG and script in uploads, also disguised as images. CRLF and quotes in file names.
- Archive bombs, archives with false sizes, 200,000 entries, and Word files with entities or deep nesting.
- SQL in every search and sort field.
- 19 inside addresses for webhooks, in decimal, hex, IPv6 and with credentials.
- Forged sessions, sessions after logout, after being disabled, and files after a restriction.
- Formulas in CSV exports.
- Changing or removing a row in the audit log, as database owner, is detected.
- A wrong shape of an installation: each mistake is named at start, and passwords stay out of errors.
- A hostile client against the service for editing together: other origins, no session, broken messages.
- Office files that ask for outside images, through the real Gotenberg 8.40: it asked for nothing.

## Not run

- The large shape: several nodes, a NATS cluster, PgBouncer, S3, Helm on a cluster.
- 30 people editing one page, and 5,000 simulated people.
- Two versions side by side and a restore: CI and the weekly restore test cover those. They were not repeated here.
- Collabora and draw.io, which only stand-ins have answered so far.
- A real browser against the frames. The sandbox and its policy were read and tested as headers.
- A backlog of events while the worker is down for days.
- DNS rebinding, which needs a name server of our own. The check on every resolved address was read, not attacked.
