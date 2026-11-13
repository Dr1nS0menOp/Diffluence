# Stress test 2: after waves LD, AC, O1 to O5 and AG

Date: 2026-11-12, one hour, on the same two virtual machines as the first test (`2026-11-10-stress-test.md`): an app machine of 8 vCPUs and 32 GB, and a load machine of 4 vCPUs, on a 12-core server. Both were rolled back to the snapshot with the seeded data (5,000 people, 300 spaces, 60,000 pages, 12,000 files), and the app machine was updated to main `ee13ee8`.

The time limit of one hour left out the break tests of the first run (none of the code they test changed) and everything of wave 7 that takes long.

## The upgrade

`docker compose up -d --build --wait` took 4 minutes, of which the image build was most. Migrations 0042 to 0048 ran in about 5 seconds on this data. No backup had ever been noted on this instance, so the first upgrade under the backup rule went on with the warning of ADR-042 instead of a refusal, as designed. The backup service was left out of this test (only `postgres` and `diffluence` were started), so a backup at the start would not weigh on the measurements.

**Planner statistics.** Rolling a running machine back to a snapshot is a crash for PostgreSQL, which then starts with empty table statistics (`pages` counted 10 live rows). The first series ran like that; `ANALYZE` (78 s) was run before the second series. The numbers below are from the second series; the first series was close to it, so the statistics were not the cause of what follows.

## Results, 2-minute levels

"Busy" is the share of requests answered 503 by the order of work of spec 031.

| Active people | Requests a second | Busy | Page p50 / p95 | Bell (poll) p50 / p95 | Publish p50 / p95 | PostgreSQL CPU, peak |
|---|---|---|---|---|---|---|
| 50, first test | 24 | 21% | 28 / 69 ms | not measured apart | 51 / 104 ms | 7.6 of 8 cores |
| 50, this test | 12 | 30% | 484 ms / 3.0 s | 434 ms / 3.0 s | 107 ms / 2.0 s | 8.0 |
| 350, first test | 210 | 71% | 30 / 293 ms | | 68 / 125 ms | 7.7 |
| 350, this test | 139 | 87% | 1 ms / 3.0 s (busy) | 1 ms / 3.0 s (busy) | 106 / 196 ms | 8.0 |
| 800, this test | 387 | 87% | 1 ms / 3.0 s (busy) | 1 ms / 3.0 s (busy) | 150 / 262 ms | 8.0 |

A p50 of 1 ms is an immediate busy answer; 3.0 s is the wait in line before a busy answer.

**The feed** did not answer within the request time limit of 60 s at any level, against 1.1 to 2.4 s in the first test. Its rewrite in wave LD was measured on generated data in the development container, where it took 20 ms; on this data set it is far slower. This needs a fix and its own measurement on this data set before anything else in this report is read as a gain.

**PostgreSQL** used all 8 cores from 50 people on, as in the first test, but each request now costs more, so fewer requests a second were served at the same level and more were answered busy. The costliest statements, by total time, in the second series:

| Statement | Mean | Calls |
|---|---|---|
| The page tree of a space | 260 ms | 247 |
| The count on the bell | 49 ms | 1,144 |
| Quick search, titles | 61 ms | 891 |
| The spaces a person may see (`perm_spaces`) | 56 ms | 891 |
| File names in quick search (two forms) | 112 and 45 ms | 404 and 757 |
| The storage still free in a space | 38 ms | 628 |

Every one of these is several times what it cost in the development container; PostgreSQL was full, so each waited for CPU. The storage query (wave ES) is new on this path. The feed is missing from the list because none of its statements finished.

**Saving held up:** publish stayed between 100 and 300 ms (p50 to p95) at every level but the first, and comments and drafts stayed fast where they were not turned away.

## All 5,000 at once

Ramp of 1 minute, 2 minutes held (the first test used 6 and 6).

| | First test | This test |
|---|---|---|
| Requests a second | 4,550 | 4,150 |
| Busy | 98.9% | 99.6% |
| Publish p50 / p95 | 3.7 s / 6.9 s | 0.48 s / 5.5 s |
| Logins answered busy | 1,331 | 4,033 |
| App and PostgreSQL stayed up | yes | yes |

The typical save went from 3.7 s to 0.48 s, which is the audit chain sealed after saving (wave AC). The audit sealer was never more than 1.2 s behind, under every load.

## Many spaces made at once

The first test froze the instance with 24 parallel creations. Now: all 24 answered 201 in 2.7 s (slowest 2.7 s, median 1.5 s), and a plain list of spaces read meanwhile took 50 ms (median) and 200 ms at most. The freeze is gone (wave LD).

## What this means

- Fixed and proven on the server: parallel creations no longer freeze the instance, and saving no longer waits for the audit chain.
- Not proven, and worse here than in the development measurements: reading. The feed must be fixed first, and the other statements in the table measured on this data set, before the server is used again for the load model of `docs/operations/limits.md`.
- The numbers of `limits.md` for the server stay those of the first test until then.

## Scripts

`scripts/stress/load.js` takes `HOLD` and `RAMP` to shorten a run, `monitor.sh` also samples the audit sealing lag, and `freeze.py` makes many spaces at once while it times a read.
