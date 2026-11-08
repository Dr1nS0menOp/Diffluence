# 031: Carrying the load

Status: built on branch `wave-v`
Phase: blueprint 04, "Carrying the load" in the roadmap (ADR-036). Lines ARC-233 to ARC-236, ARC-238 and ARC-239.

## Problem

The self-test of spec 025 showed what happens when one database is full: requests queue until everything is slow, a save waits behind a flood of reading, and `/readyz` still says ready. Spec 028 gave search its own share of the connections. This spec gives every request a place in line, so that under too much load saving still works, the app says it is busy instead of slowing down for everyone, and a load balancer can tell a full node from a healthy one.

## What exists

- **The pool:** 20 connections by default (`internal/db`), tunable with `pool_max_conns`.
- **Search:** at most one connection in four (`internal/search`, `searchShare`). It waits one second for a turn and then answers `busy` (503, `Retry-After: 2`). The frontend shows the message of an error, so the person reads it.
- **Pasted Markdown:** two at a time per node (spec 025).
- **Login:** limits per account and address count failed tries, not how many hash at once (`internal/identity/password.go`). A rush of correct logins can still fill every core.
- **`/readyz`:** pings the database, checks migrations, the queue for workers, and storage. A ping on a full pool waits for a connection and then fails as `database`, which names the wrong cause.
- **Writing:** the editor keeps unsaved text in the browser when a save fails ("This browser kept text that was not saved"), so a refusal loses nothing.

## Study

**What a request costs.** Nearly every handler uses one connection at a time: queries run one after another, and a transaction holds one connection. So the number of requests that run at once is a good stand-in for the number of connections they hold. A request that holds two at once is rare, and the pool itself still caps the total, so the stand-in can only make the order softer, never break correctness.

**Where the order belongs.** It must sit before the first query of a request, which is the session lookup. So it is a middleware, before authentication. Health probes and `/metrics` never wait: a probe that waits on a full node reports too late.

**The classes.** Three, in the order they are served:

| Class | Requests | Why |
|---|---|---|
| Saving | Every request that is not GET, HEAD or OPTIONS | Losing a save is the worst outcome |
| Reading | Every other GET | What people do most |
| Search and reports | Search, exports, insights, analytics, the care report, the audit list | Heavy, and people can try again |

**Shares.** With a pool of `n` connections:

- Saving may use all `n`. Nothing caps it below the pool, because the point is that a save always finds room.
- Reading may use `n` minus a reserve of `n/4` (at least 1) that is kept for saving.
- Search and reports may use `n/4` (at least 1), and also count against reading, so together they can never eat the reserve. Search keeps its own share inside `internal/search` as well; folding that one away is left until wave T, which works in that package, has landed.

**Waiting.** A request that finds its class full waits a short time. A queue without end helps nobody, so each class has a limit on how many wait. Past it, or past the patience, the answer is 503 with `Retry-After` at once.

| Class | Patience | Most waiting |
|---|---|---|
| Saving | 15 s | 8 × `n` |
| Reading | 3 s | 4 × `n` |
| Search and reports | 1 s | 2 × its share |

Saving waits longest because a save is worth waiting for and is rare. The numbers are starting points; the measurement below says whether they hold.

**Readiness.** `/readyz` answers `busy` when saving or reading has more than half its limit waiting, or when every connection of the pool is taken. It checks the pool before it pings, so a full pool is reported as busy, not as a lost database. `/livez` does not change, so a busy node is not restarted (ARC-009). A node that says busy drops out of a load balancer until the queue is gone. When every node is busy at once, a balancer that only uses ready nodes has none left; that case needs a rule in the balancer, described in `docs/operations/limits.md`, and is a choice for the product owner (see the handoff).

**Password hashing.** Argon2id is meant to be expensive. At most half the cores, and at least one, hash at once per node. A login waits for its turn; the waiting is bounded by the saving class above.

**Load budget and query count.** The spec template gets a section *Load budget*: queries per request, writes per action, and the class. A test counts the queries of the main screens (opening a page, the tree, search, the bell) and fails when one goes past its budget. The count comes from a tracer on the pool that adds one per query; it is also a metric, `diffluence_db_queries_total`. Background work in a test can add queries, so the test takes the lowest of three runs. ARC-197 (load permission facts once in `Get`) follows only if the count shows the page is far over a sensible budget.

## Behaviour

- Under normal load nothing changes.
- When reading floods a node, saving still answers. Reading past its limit gets "Diffluence is very busy right now. Try again in a moment." A text being written stays in the browser, as it does today.
- When search floods a node, reading and saving keep working.
- A load balancer stops sending people to a node that says busy, and sends them back when it has caught up.

## Permissions and audit

None change. A refused request writes nothing: refusing must cost less than serving.

## Load budget (ARC-238)

The admission check is memory only: no query. Readiness reads the pool's numbers, also no query, before its ping.

## Acceptance criteria

- [ ] With reading full, a save still gets a connection and answers.
- [ ] Past its waiting limit or patience, a class answers 503 with `Retry-After` at once.
- [ ] Search and reports together never take the reserve of saving.
- [ ] Probes and `/metrics` never wait.
- [ ] `/readyz` says `busy` on a full pool or a long queue, and `/livez` stays ok.
- [ ] No more hashes run at once than the limit.
- [ ] The main screens stay within their query budget, and a change past it fails a test.
- [ ] Measured under the model of 5,000 people with search and reading flooded: saving keeps working.

## How the product owner tests it

Nothing to see on a normal day. Under a load test, saving a page still works while search answers "very busy".

## Provenance

Own design, from blueprint 04.8 section 8.5 and the measurements of specs 025 and 028.
