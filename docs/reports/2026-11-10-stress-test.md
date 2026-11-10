# Stress test on a real server, 10 November 2026

Written for: the people who build and run Diffluence. What was run on a real server, what held, where it gave way, and what this run could not prove.

This is a self-test by the people who built the app, like [known limits](../operations/limits.md). A limit that was not reached is written as "not reached at N", which never means safe.

## The machine

- One server with a 6-core, 12-thread Xeon from 2012 (E5-2620), 66 GB of memory, and two hardware RAID volumes of spinning disks without a write cache, under Proxmox VE 9.
- Two virtual machines on it, Ubuntu 24.04:
  - **app**: 8 vCPUs, 32 GB, Diffluence and PostgreSQL 16 by `compose.yaml`;
  - **load**: 4 vCPUs, 8 GB, k6 and the seeding script.
- Both machines share the 12 hardware threads, so under full load the load tool takes CPU from the app. Numbers near the top of the step test are lower than the app alone would give.

### The disks set the pace

Measured inside the app machine with `dd`:

| Disk | 1 GB written, direct | 4 KB writes, each synced | 1 GB read, direct |
|---|---|---|---|
| First volume | 32 MB/s | about 50 a second (20 ms each) | 365 MB/s |
| Second volume | 25 MB/s | about 160 a second (6 ms each) | |

PostgreSQL waits for a synced write at every commit. On the first volume that allows about 50 commits a second for one writer, so PostgreSQL's data went on the second. A server with SSDs or a controller with a battery-backed cache syncs in well under a millisecond: there, the writing numbers below would be many times higher. The reading and search numbers depend much less on the disk, because the data fits in memory.

## Installing

The machine was installed as `docs/operations/install.md` says: clone, `scripts/setup-secrets.sh`, `docker compose up -d --build --wait`, then `diffluence user create-admin`. Two operator choices were added in a `compose.override.yaml` (`scripts/stress/compose.stress.yaml`):

- PostgreSQL settings for a 32 GB machine: `shared_buffers=4GB`, `effective_cache_size=16GB`, `work_mem=16MB`, `max_connections=200`. PostgreSQL's own defaults are sized for a small machine.
- `pg_stat_statements`, to see which queries cost the most.

What the install guide should say and does not yet:

1. **`DIFFLUENCE_PUBLIC_URL` must use a host name, not an IP address.** With `http://<address>:8080` the app stops at start ("field 'RPID' is not a valid domain string") and Compose restarts it forever, because passkeys need a domain. A test machine without DNS can use a name in `/etc/hosts`.
2. **The PostgreSQL settings.** The guide gives the CPUs PostgreSQL needs, but not its memory settings, and the defaults use 128 MB of shared buffers on any machine.
3. **`pg_stat_statements` needs `CREATE EXTENSION`** after `shared_preload_libraries` is set, which an operator only learns from PostgreSQL's own documentation.

## The data

`scripts/stress/seed.py` filled the instance through the public API only, so every row has its audit event and every file is encrypted, as in use. The sizes follow `docs/operations/limits.md` and blueprint 04.8: an organisation of 5,000 people.

| What | How many | Why |
|---|---|---|
| People | 5,000 | The size the product is built for |
| Groups | 120 teams and "All staff" | People are in 1 to 3 teams, everyone in All staff |
| Spaces | 300 | A few large handbooks and many small team spaces: sizes fall off as 1/rank^0.8 |
| Pages | 60,000, three levels deep | 12 pages a person, a common size for a wiki of this many people |
| Versions | about 30% of pages edited 1 to 5 more times | |
| Words | 150 to 900 a page, from a made-up language of 20,000 words with a Zipf spread | Some words are in nearly every page, most are rare, as in real text |
| Comments | 30,000 | By 300 different people: one person may only write so many a minute |
| Files | 12,000, 200 bytes to 2 MB | |
| Labels | 25,000, from 300 words | |

Half the spaces are open to all staff to read, one in ten to write, and every space has three teams that read and one that writes. The load test uses those open spaces, so every simulated person has pages to read and change.

**What the database held** after seeding: 5,001 people, 300 spaces, 60,000 pages with 114,800 versions, 29,400 comments, 12,000 files and 24,900 labels, 295,000 audit events, 1.8 GB.

**How long it took** on this machine: 5,000 people in 3 minutes, 60,000 pages with their versions in about 63 minutes, files and labels in 8 minutes each. Comments came from 300 people, because one person may only write so many a minute.

## Load

`scripts/stress/load.js` (k6) makes each simulated person log in as one of the seeded people and then do what a person does: read a page with its files and the bell, open the tree and the feed now and then, search about 40% of the time (typing in the quick box, now and then a full search), and sometimes save a page, comment or upload. "People" below means people active at the same moment.

### How many people at once

Each level ran 4 minutes on its own (`SCENARIO=level`). "Busy" is the share of requests answered 503 with `Retry-After`, as the order of work of spec 031 does when PostgreSQL is full; all other answers were correct.

| Active people | Requests a second | Busy | Page p50 / p95 | Tree p50 / p95 | Feed p50 / p95 | Publish p50 / p95 | PostgreSQL CPU, peak |
|---|---|---|---|---|---|---|---|
| 20 (trial) | 11 | 0% | 38 / 109 ms | 80 / 134 ms | 1.1 / 1.4 s | 95 / 119 ms | |
| 50 | 24 | 21% | 28 / 69 ms | 115 / 255 ms | 1.2 / 1.7 s | 51 / 104 ms | 7.6 of 8 cores |
| 100 | 53 | 34% | 29 / 89 ms | 128 / 302 ms | 1.3 / 2.1 s | 58 / 101 ms | 7.6 |
| 200 | 115 | 48% | 35 / 340 ms | 178 / 504 ms | 1.7 / 3.0 s | 63 / 142 ms | 7.8 |
| 350 | 210 | 71% | 30 / 293 ms | 113 / 436 ms | 0.5 / 2.7 s | 68 / 125 ms | 7.7 |
| 500, 10 minutes (`mix`) | 228 | 63% | 659 ms / 1.4 s | 772 ms / 1.5 s | 2.4 / 3.8 s | 95 / 247 ms | 7.6 |

The busy answers were nearly all searches: from 50 people on, most searches waited their second and were told to try again, which is how search is kept to one connection in four.

**What gives way first, in this order:**

1. **Search on file names.** The quick box also looks for files by name, and that query cost 0.3 to 0.9 s each at every level, the most database time of any query below 100 people.
2. **The feed.** It cost 1.1 s with 20 people and 2 s at 200, from the first level on. It works out every page the person may see (tens of thousands here) and their versions before it keeps the newest 20. Above 100 people it takes the most database time of all queries.
3. **The count on the bell.** Each poll works out the visible pages again: 80 ms each, and the third largest share of database time.

**What held at every level:** reading a page stayed at about 30 ms (p50), saving at about 60 ms, comments at about 30 ms, and no answer was wrong. `/readyz` said busy only at the top. Memory of the app stayed below 160 MB.

**Room for four times the database work:** not on this machine. PostgreSQL used all 8 cores from 50 active people on, and the extra work was turned away as busy rather than queued. The limit here is CPU, not the disk: iowait stayed at 0 to 6%.

### Search-heavy

200 people who search 90% of the time, 6 minutes: 89% of searches were answered busy at once, as designed. Reading went on beside it at 32 ms (p50) and 112 ms (p95) for a page, and saving at 63 ms. Search shares the CPU of PostgreSQL with everything else, so reading slowed a little, which is what a standby for search (ARC-229) would take away.

### The worst case: all 5,000 at once

5,000 people logged in over 6 minutes and kept working with short pauses for 6 more: 4,550 requests a second reached the app.

- 98.9% were answered busy; the app and PostgreSQL stayed up the whole time and came back to normal by themselves when the load stopped.
- Writes waited for the audit chain: every audited write takes one lock in turn (ARC-201), and under this load that lock cost 379 ms on average, so saving took 3.7 s (p50) and 6.9 s (p95). None was lost.
- 1,331 of 5,000 logins were answered busy and had to be tried again.
- The app used up to 4.3 cores and 330 MB.

### Ramping up

The step test (`SCENARIO=step`, 50 to 500 people over 21 minutes) gave the same picture as the levels above; its summary mixes the levels, so the table above uses the separate runs.

## Breaking it

Each test ran under load from `SCENARIO=level` (`scripts/stress/break.sh`, `slow-clients.py`, `huge-page.py`).

| What | Under | Result |
|---|---|---|
| PostgreSQL restarted | 100 people | Back in 12 seconds. The app stayed ready, 99.94% of checks passed, the rest were answered "try again" while it was down |
| The app's process killed with SIGKILL | 100 people | Compose started it again; ready after 3 seconds. 98.3% of checks passed |
| 50 people saving one page at the same moment, 3 minutes | | Publishing p50 34 ms, p95 79 ms. A third of the saves were told someone else saved first (409), as they should be. Nothing was lost or mixed |
| 3,000 connections that send their headers slowly | 50 people | All 3,000 were closed by the server within 16 seconds. The others did not notice: a page p50 31 ms |
| A page of 5.5 MB | | Refused with 413, "larger than the limit of 5 MiB", in 0.2 s |
| A page of 4.9 MB, just under the limit | | Published in 1.2 s. 20 people read it at once in 0.7 to 1.1 s each. Search found its words in 0.35 s |
| The disk of PostgreSQL and the files left with 4 MB free, 100 seconds | 50 people | Not reached: nothing failed, as PostgreSQL reuses its existing log files and the people wrote little in that time. [Known limits](../operations/limits.md) has what a full disk does to uploads |
| The server itself stopped four times during the day (see below) | Up to 500 people | Every time, PostgreSQL recovered by itself at the next start, and no data was lost |
| `diffluence audit verify` after all of the above | | "audit chain OK (324,108 events and 1 signed checkpoints checked)" |

## What to do with this

For the people who build Diffluence, in order of what it would give:

1. **The feed** should not work out every visible page before it keeps 20. Starting from the newest changes and checking visibility for those until 20 are found would make its cost depend on the 20, not on the size of the wiki.
2. **The count on the bell** should not work out the visible pages on every poll. It is asked on every page view, so a cheaper count, or one kept up to date when notifications are made, saves the third largest share of database time.
3. **Search on file names** needs an index that helps a search inside a name. Wave ES plans trigram indexes for titles and file names (spec 047); this run is a reason to measure that query before and after.
4. **Known limits** (`docs/operations/limits.md`) should say that the earlier single-space measurement with one account overstated what a real permission model allows, and give these numbers beside it.

For operators: on a server like this one, plan on PostgreSQL CPU, not memory or disk, as what runs out first. A PostgreSQL that is full turns away searches first and keeps reading and saving going.

## What this run could not prove

- **Fast disks.** Writing was measured on disks that sync about 160 times a second. The numbers say what such a disk allows, not what Diffluence allows on SSDs.
- **The server itself.** The server stopped and started again four times during the day, without a message in its log, each time while it was busy. A run of hours on this server cannot be trusted to finish, so each run here is short, and some had to be repeated. It did show that a hard stop of the whole machine loses nothing.
- **A modern CPU.** The CPU is from 2012. PostgreSQL's limit here is CPU, so a current server will carry more people at every level; how many more was not measured.
- **More than one node**, PgBouncer, a NATS cluster and S3, as in [known limits](../operations/limits.md).
- **Real browsers.** k6 sends the requests a browser would, but runs no JavaScript and draws nothing.
