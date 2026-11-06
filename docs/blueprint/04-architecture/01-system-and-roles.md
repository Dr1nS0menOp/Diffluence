# 04.1: The system and its roles

How to read this chapter: see `../04-architecture.md`. Every line has a number and a status: **built**, **partly**, **wave X**, **phase N** or **later**.

## 1.1 The shape

- **ARC-001** · built · Diffluence is a modular monolith: one Go codebase, one binary, strict modules inside (chapter 2).
- **ARC-002** · built · The binary has three roles. `api` serves HTTP. `worker` does background work. `all` does both, for small installations.
- **ARC-003** · built · The frontend is built into the binary (`internal/webui`), so one file is an installation.
- **ARC-004** · built · The frontend talks only to the public API, like any other client. There is no private back door.
- **ARC-005** · built · PostgreSQL is the only required outside component.
- **ARC-006** · built · NATS JetStream runs embedded in `role=all`, with no network listener. Roles `api` and `worker` need an external NATS, and the configuration refuses to start otherwise.
- **ARC-007** · partly · Any API node can be killed at any time. Sessions live in PostgreSQL and keys come from one file on every node. Still open: the file store is a local disk until S3 arrives (phase 3), and the login limiter counts per node (ARC-210).
- **ARC-008** · built · Every process registers in `nodes` and sends a heartbeat every 15 seconds, so the console can show the cluster, and contract migrations can wait for old nodes.
- **ARC-009** · built · `/livez` says the process runs. `/readyz` says it may get traffic: database, migrations, queue and storage. A worker serves only these two.
- **ARC-010** · built · Stopping is graceful. A node first reports "not ready", waits `SHUTDOWN_DELAY`, finishes open requests within 30 seconds, then stops its background work.
- **ARC-011** · built · Configuration comes from `DIFFLUENCE_*` variables. Each can be read from a file with `_FILE`, for secrets. All mistakes are reported at once, not one at a time.
- **ARC-012** · built · The command line offers `serve`, `migrate`, `user`, `audit` and `version`.
- **ARC-013** · built · The command line gains `client`, which only uses the public API with a token (PLT-040, D22 of spec 015).

## 1.2 What runs where

| Part | `api` | `worker` | `all` |
|---|---|---|---|
| HTTP API and frontend | yes | health only | yes |
| Outbox relay | no | yes | yes |
| Job runner | no | yes | yes |
| Periodic tasks | no | yes | yes |
| Event consumers | no | yes | yes |
| Heartbeat | yes | yes | yes |
| Embedded NATS | never | never | when no address is set |

- **ARC-014** · built · Requests never do slow work. Imports, purges and anything that can take long go to the worker.
- **ARC-015** · built · Event consumers run in the worker role only. An API node stays a pure request handler.
- **ARC-016** · built · Several workers can run at once. Relay rows, jobs and periodic tasks are claimed through PostgreSQL, so each piece of work runs once (ADR-018).

## 1.3 The three shapes of an installation

**Small: one process.** One `diffluence` with `role=all`, PostgreSQL, a local disk. Up to a few hundred people. `compose.yaml` is this shape.

**Complete: one server with helpers.** The same, plus the optional containers an administrator chose in chapter 4. Each is separate, and each can be left out.

**Large: high availability.** Two or more `api` nodes, two or more `worker` nodes, a NATS cluster of three, PostgreSQL with replication behind PgBouncer, an S3 store. See chapter 8.

- **ARC-017** · built · The small shape needs nothing else, and stays possible for ever. A feature that needs a helper is absent without it, and never blocks the rest (ARC-070).
- **ARC-018** · phase 3 · The large shape is tested in CI with a load test, and a node can be killed under load without errors people can see.
- **ARC-019** · built · The same binary and the same migrations serve all three shapes. There is no enterprise build.

## 1.4 Outside the binary

| Component | Required | Who talks to it | Chapter |
|---|---|---|---|
| PostgreSQL 16 or newer | Yes | api, worker | 7 |
| NATS | Embedded, or external in the large shape | api (health), worker | 3 |
| Reverse proxy with TLS | Yes in production | browsers | 5 |
| S3-compatible store | Large shape | api, worker | 7 |
| Mail server | No | worker | 4 |
| Identity provider | No, and advised (ADR-026) | browsers, api | 4 |
| Gotenberg | No | worker | 4 |
| Collabora Online | No | worker | 4 |
| draw.io | No | browsers | 4 |
| Service for editing together | No | browsers, api | 4 |

- **ARC-020** · built · TLS ends at a reverse proxy. The binary speaks plain HTTP on the inside, and trusts `X-Forwarded-For` only from `TRUSTED_PROXIES`.
- **ARC-021** · built · The container runs read-only, without capabilities and with no new privileges (`compose.yaml`).
- **ARC-022** · built · Two database roles: migrations run as the owner, and the application role owns nothing and cannot change audit rows (ADR-019).
