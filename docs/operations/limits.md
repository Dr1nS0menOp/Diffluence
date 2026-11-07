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
| Search on a word in every page | 5, 20 people | **About 3.5 searches a second.** At 20 people each search takes 4.5 s | Slower, no errors. All other database work waits with it | Give PostgreSQL more CPU. This is the first thing that gives way |
| Search on a rare word | 5, 20 people | 16 a second, p95 52 ms. Ceiling not reached | | |
| Mixed readers with search and feed | 50 readers | p95 7.2 s, all from the search above | Slower | As above |
| The page tree of 50,000 pages | 5, 20, 60 people | **19 MB per load**, 4 loads a second, 13 s each at 60 people, 567 MB of memory at 20 | Slower, no errors. Not measured in a browser | Keep spaces far below this. Turn on compression at the proxy. SPC-031 is the real answer |
| Open page, feed, overview | One person, 50,000 pages | 9 ms, 88 ms, 119 ms | | |
| Publishing | 6 at once | 143 pages a second on an empty space, 70 at 50,000 pages, p95 121 ms | Slower | |
| Connections that say nothing | 500, 2,000, 8,000 held | All dropped after 10 seconds, others not disturbed, 154 MB. Ceiling not reached at 8,000 | | A proxy still comes first |
| Upload | 64 MB, and past the limit | 64 MB in 1.9 s, memory did not grow. Past the limit the connection is closed early | | Set the limit in the console |
| Export of 50,000 pages | Text, HTML | 5 and 6 seconds, 237 MB at most | | |

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
