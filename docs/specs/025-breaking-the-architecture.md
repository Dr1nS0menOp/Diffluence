# Spec 025: Breaking the architecture

Status: run on 6 November 2026.
Phase: asked for by the product owner on 2026-11-06. The goal, in their words: break the app, so its limits are known.

This is a self-test by the people who built the app. It is not an independent penetration test, and it does not replace one (ADR-029).

## Rules of the test

- The target is one local instance in its own containers. Nothing on the internet is touched. Outbound features are attacked with stand-ins on the same machine.
- Every load test runs with limits on the target and the load tool together: at most 4 CPUs and 4 GB of memory. A disk test uses a volume of at most 20 GB. The numbers hold for this machine and these limits, not for yours.
- A vulnerability stays out of this repository until it is fixed. This file and the handoff hold fixed findings only.
- A limit that was not reached is written as "not reached at N". It never means safe.
- A finding gets a failing test first, then a fix. Each fix keeps its test.

## What counts as a result

| Result | Meaning |
|---|---|
| **Broken** | A rule of blueprint 04 does not hold, or a person gets past a limit |
| **Limit** | A number where the app slows, refuses or stops, with how it fails |
| **Held** | Attacked as described, and it did not break |
| **Not run** | Only reasoned about, with the reason |

## The plan, per chapter of blueprint 04

| Chapter | What can break | How it is attacked |
|---|---|---|
| 1 Roles | A worker that serves more than health, a start with a wrong shape, a stop that loses work, trust in forwarded addresses | Start each role with a wrong configuration, ask a worker for the API, send forged forwarding headers, stop and kill during work |
| 2 Modules | A route or module that skips a check | Walk every route in `docs/api/routes.txt` as a stranger, a guest, a viewer and a person of another space, with ids of objects they may not see, and compare with the rule |
| 3 Events and jobs | Lost, repeated or endless work | Poison events and jobs, repeats, a backlog, a worker killed in a job, a rule that triggers itself |
| 4 Helpers | Reaching inside addresses, hostile files, frames that escape | Outgoing addresses to private and tricky targets, redirects, hostile office and PDF files, the headers and sandbox of each frame, service tokens |
| 5 API edge | Login, sessions, cross-site writes, parsers, limits | Guessing and lock-out, session and token abuse, forged origins, deep and oversized bodies, hostile uploads and archives, injection, headers on every kind of answer |
| 6 Live updates and caching | Stale access, hostile collaboration clients, cached answers | A person who lost access, forged shared documents, malformed WebSocket traffic, cache headers after a restriction |
| 7 Data and storage | A log that can be changed, keys in the wrong place, a full disk, a database lost in a write | Tamper with the chain, read what the application role may do, fill the disk, stop the database during writes, check backups |
| 8 Scaling | The real ceilings | Step up people, pages, tree size, search, file size and editors until something gives, watch the audit lock |
| 9 Failures and upgrades | What people really see | Kill each process under load, two versions side by side, a restore |

## Results

Measured limits, attacks that held and what was not run are in `docs/operations/limits.md`. What broke and how it was fixed is in `docs/handoff/2026-11-07-breaking-the-architecture.md`.

| Chapter | Result |
|---|---|
| 1 Roles | Held |
| 2 Modules | Held, on all routes |
| 3 Events and jobs | Broken: one event too large stopped the relay. Fixed. A job of a dead worker waits up to 70 minutes |
| 4 Helpers | Broken: the network of `compose.yaml` did not match ARC-074. Fixed. Hostile office files held. Collabora and draw.io not run |
| 5 API edge | Broken: Markdown cost and account lock-out. Fixed. Everything else held |
| 6 Live updates | Broken: size of messages to the editing service. Fixed. Stale access held |
| 7 Data and storage | Held |
| 8 Scaling | Limits measured. Search and the tree give way first |
| 9 Failures | Held on one node. The large shape was not run |
