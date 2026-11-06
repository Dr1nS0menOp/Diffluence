# 021: Wave K, the rest of the chapters and the architecture

Status: built on branch `wave-k`, waiting for the product owner's acceptance session
Phase: wave K of `docs/blueprint/03-functional-requirements.md`, and the part of blueprint 04 that phase 3 asks for. The product owner asked for every ARC status line to say what is really built, and for what it still needs, as long as no new dependency is involved.

## Problem

Chapters 1, 2, 3, 7, 8, 9, 10, 11, 12 and 13 of blueprint 03 still hold about 90 lines marked `new` or `partly`. Many are stale: the code moved on and the line did not. Blueprint 04 is worse: around 150 lines still name a wave that has finished. An operator has no metrics, no way to follow a request across logs, and a backup that holds the master key in the clear next to the database. Blueprint 06 calls that last one High.

## What the study found

**Metrics.** Prometheus reads a plain text format, which is short and stable: `# HELP`, `# TYPE`, then `name{label="value"} number`, with `\`, `"` and newlines escaped in label values, and for a histogram the cumulative `_bucket{le="..."}` lines, ending in `le="+Inf"`, then `_sum` and `_count`. The official Go client would be one more dependency tree. We need counters, gauges and one histogram, so a small package of our own is less code than the wiring. Labels must stay few: a path as a label grows without limit and holds page ids, so requests are counted by method and status class only.

**Who may read metrics.** Numbers about an instance tell an outsider its size, so `/metrics` needs a secret. A person's session is the wrong tool, because Prometheus has none. Prometheus can send `Authorization: Bearer` from a file. The route answers 404 when no token is configured, so an instance that does not use it shows nothing.

**Tracing.** The W3C Trace Context standard defines `traceparent`: `version-traceid-parentid-flags`, lower-case hex, 32 and 16 digits, where all zeros is invalid. A proxy that sends it wants the trace id in our logs. Reading it and writing `trace_id` beside `request_id` needs no module. Exporting spans would need OpenTelemetry, which is a dependency, so that is left.

**Health.** `/readyz` fails on a NATS outage on every node, which takes down all API nodes although they could serve people. An API node only publishes through the outbox, which waits in the database (ARC-216). Worker roles still need the queue.

**Backups.** `scripts/backup.sh` writes the database, files, `master.key` and `secrets/` as plain files into one folder. `gpg --symmetric --cipher-algo AES256 --passphrase-file` is in every distribution, needs no code of ours, and gives authenticated encryption with its integrity packet. The passphrase file must be kept somewhere else than the backups. A script that finds no key must refuse, because a silent plain backup is what SEC-636 is about.

**Caching files.** A file's bytes are known by their stored SHA-256, so `ETag` is free. `private, no-cache` means the browser asks every time, so permissions are still checked on each view, and a 304 saves the bytes. A shared cache may never hold it, because of `private`.

**Helm.** A chart is YAML, and needs no dependency in the product. `helm template` checks it. It starts at the shapes of blueprint 04: one image with roles `api` and `worker`, and an external PostgreSQL.

**The status lines.** Each stale line was read against the code. The High lines of blueprint 06 are mostly closed: SEC-134 and 135 (passkeys), SEC-265 (browser data at logout), SEC-521 (drawing origin), SEC-542 (the collab recheck), SEC-555 (PDF frame), SEC-814 and SEC-820 (audit). Only SEC-636 was still really open, and SEC-546 needed checking.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | `internal/metrics` is our own small registry, in the Prometheus text format | Counters, gauges and one histogram do not justify a dependency tree. |
| D2 | `/metrics` is a `Public` route with its own Bearer token from `DIFFLUENCE_METRICS_TOKEN`, compared in constant time, and 404 without one | Prometheus has no session. Off by default. |
| D3 | Labels on requests are method and status class only | A path is unbounded and holds ids. |
| D4 | Metrics that come from the database are read when scraped, with a short timeout | No background loop to keep alive. |
| D5 | `traceparent` is read, validated and logged as `trace_id`; an invalid one is ignored, never an error | Input from outside must not break a request. No spans are exported, which would need a dependency. |
| D6 | Role `api` ignores the queue in `/readyz`, and the console still shows it | ARC-216. Roles that consume events still need it. |
| D7 | Files answer `private, no-cache` with a strong `ETag` from the stored hash, and 304 on a match, after the permission check | ARC-167. The check stays on every view. |
| D8 | Viewing the page a person viewed last writes `recent_pages` at most once a minute, going back to another page writes at once, and a periodic task trims the list | ARC-198. Reading a page should not be two writes, and the order of the list must stay what people did. |
| D9 | The pool defaults to 20 connections unless the URL sets `pool_max_conns` | ARC-175. The driver's default is the larger of 4 and the number of CPUs. |
| D10 | A backup is one archive, encrypted with AES-256 by `gpg`, with a passphrase file given by the operator. Without the key the script refuses | SEC-636. `master.key` no longer sits in the clear beside the data. |
| D11 | The passphrase file may not sit in the backup folder | A key next to its lock protects nothing. |
| D12 | The restore drill joins the nightly CI job | An automatic restore test, as blueprint 07 promises. |
| D13 | The Helm chart lives in `deploy/helm/diffluence`, and takes an existing PostgreSQL and existing secrets | It holds no secret values. |

## What is built, line by line

Operations, in this order:

| Line | What |
|---|---|
| ARC-216 | `/readyz` for role `api` |
| ARC-167 | `ETag` and `no-cache` for files |
| ARC-198 | recent list limits and a trim task |
| ARC-175 | default pool size |
| ARC-218 and phase 3 metrics | `/metrics`, `traceparent`, request metrics |
| SEC-636, SEC-711 | encrypted backups, with restore and drill changed |
| Blueprint 07 restore test | drill in the nightly job |
| ARC-208, ARC-207 | Helm chart |
| ARC-199, ARC-200 | a k6 script under `scripts/load/`, without a run at full size |

Chapter lines that can be closed without the product owner:

| Line | What |
|---|---|
| PAG-030 | Offer to share after the first publish |
| COL-035, COL-087, ACC-021 | Mentions, followers and spaces on a profile, and the choice to hide followers |
| ACC-071, ACC-073 | Notice to the owner of a recovered page, and restricted pages with nobody left in the console |
| ADM-033, ADM-055, SPC-061 | Totals per space, largest pages, filter on contributor |
| ADM-026, ADM-027, ACC-067, COL-062, EXC-019, NAV-012, NAV-080, PAG-119, COL-030 | Status only, after checking the code |

## What changed while building

Reading every ARC line against the code found gaps the status lines had hidden, and these were built first: `MaxDeliver` with `event_failures` (ARC-050), job slots (ARC-063), a loop per task (ARC-068), the spread of polls (ARC-161) and an unlogged presence table (ARC-162). `Consume` had no limit at all, so one bad event came back for ever.

| # | Decision | Why |
|---|---|---|
| D14 | The queue counts tries itself, with `NakWithDelay`, and sets `MaxDeliver` 20 higher | `BackOff` only applies to a missed acknowledgement. A listener mostly fails when the database is down, and then the failure cannot be stored either, so it needs room to try again. |
| D15 | A replay runs in a task on the worker | Only a worker has the listeners. The console only asks. |
| D16 | A restricted page with nobody left shows in the console without its title | An administrator has not recovered it yet, so they may not read it. |
| D17 | Publishing offers to share only the first time | A message on every edit would be noise. |

ACC-073 stays `partly`: a notification about a page its reader cannot see does not show. Lines ADM-026, ADM-027, ACC-067, COL-062 and EXC-019 were already built, and only their status changed.

The small lines that remain, and need no decision, are listed in `docs/handoff/2026-11-06-wave-k.md`.

## What is left, and why

| Line | Reason |
|---|---|
| ARC-192, ARC-193 | S3 needs a client library. On the list for the product owner in `docs/dependencies.md`. |
| ORG-025, ARC-126 | Text in attachments needs an extractor, which is a new program or module. |
| ARC-125 | Virus scanning through ICAP, later by plan. |
| ARC-194 | Encrypting files changes storage, and waits for ADR work. |
| SPC-031, ARC-204 | Blueprint 04 says measure first. It needs a load test at full size. |
| COL-096, ARC-018, ARC-199, ARC-200, ARC-202, ARC-218 | The full runs need hardware: the script is built, the run is not. |
| ARC-052, ARC-053, ARC-123, ARC-124 | Cluster shapes, which need a cluster to prove. |
| ARC-176, ARC-178, ARC-179, ARC-182, ARC-207, ARC-209 | Advice for operators, not code. They become guidance. |
| PLT-037, PLT-041, ARC-143 | OAuth and apps belong to phase 7 (ADR-026). |
| COL-080 | The product has one language so far. |
| OTH-012, OTH-013, OTH-011, OTH-023 | Follow-ups of wave I, which need a study of their own. |
| COL-047 | Invitations by e-mail open a way in for people without an account, and need the product owner's decision. |
