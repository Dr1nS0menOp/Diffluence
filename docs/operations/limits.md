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
## An order of work under too much load (spec 031)

Measured on 8 November 2026, 50,000 pages, the same caps: the app 2 CPUs and 1.5 GB, PostgreSQL 1.5 CPUs and 2 GB, the load tool 0.5 CPU and 512 MB, a pool of 20. "Before" is main before spec 031, "after" is with it, on the same data, one after the other. Another build ran on the machine at the same time, the same for both. Each row is one run of 40 seconds, so take differences of a few percent as noise.

| What | Before | After | How it fails now |
|---|---|---|---|
| The model of spec 028 | Reading p95 146 ms, saving p95 278 ms, search p95 1.0 s, nothing failed | Reading p95 31 ms, saving p95 100 ms, search p95 100 ms, 1% of quick searches busy | |
| 600 page views a second, 2 saves | Served 378 a second, the load tool dropped 8,700 it could not send. Reading p95 2.7 s, saving p95 417 ms. `/readyz` ready the whole time | Served 599 a second. 30% of reads answered busy at once, the rest p95 303 ms. Saving p95 186 ms, none failed. `/readyz` busy in 34 of 38 checks | Reading past its share: 503 with `Retry-After: 2` |
| 400 page views, 150 quick searches, 40 results pages a second, 2 saves | Reading p95 3.3 s, 84 to 90% of searches busy after waiting a second, saving p95 500 ms | Reading p95 338 ms with 14% busy, 89% of searches busy at once, saving p95 196 ms, none failed. `/readyz` busy in 28 of 38 checks | Search first, then reading |

Saving never failed in any run, and got faster under every flood. The memo of permission facts (ARC-197) is in the "after" numbers as well, so the model got faster for that reason too.

**What to know when you run behind a load balancer.** A node says `busy` on `/readyz` while its queues are long. With several nodes the balancer sends people to a node with room. If every node is busy at once, a balancer that only uses ready nodes may have none left, and Kubernetes then sends nobody anywhere. Give the balancer a rule for that case, such as keeping all nodes when none is ready, or let your readiness probe tolerate a few failures in a row.

## What stops everything

| What | Result | What to do |
|---|---|---|
| Something holds the audit lock | Writes wait, which is known. With more waiting writes than the pool has connections, **reading stops too**: a page timed out after 25 s while `/readyz` still said ready | A long transaction never audits early (ARC-201). Watch for it. Set `pool_max_conns` well above your writers |
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
