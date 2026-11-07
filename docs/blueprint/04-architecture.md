# 04: Architecture

This is how Diffluence is built and how it must grow to carry all of blueprint 03 and specs 008 to 016: what runs where, who owns what, how parts talk, how they fail, and what was traded for what.

It came from a study on 2026-11-05, as the product owner asked for every blueprint. It reads the code as it is, not as the first version of this text hoped it would be. `04-architecture/sources.md` lists every source.

## How to read it

Every line has a number that never changes and is never reused, and one status:

| Status | Meaning |
|---|---|
| **built** | It works today, and the code shows it |
| **partly** | Part of it works. The line says what is open |
| **wave X** | Planned in that wave of blueprint 03 |
| **phase N** | Planned in that phase of the roadmap (08) |
| **later** | Worth keeping, not planned yet |

When something is built, its status changes in the same pull request.

## The picture

```
             Browser: React, TipTap
                  │ HTTPS: REST under /api/v1, polling
                  │ wss /collab, only with editing together
          ┌───────▼────────┐
          │ Reverse proxy  │  TLS, limits
          └───────┬────────┘
   ┌──────────────▼──────────────┐        ┌───────────────────┐
   │ diffluence, role api        │◄──────►│ Editing service   │ optional
   │ router, access rules, API   │        │ Node.js, Yjs      │
   └──┬───────────────┬──────────┘        └───────────────────┘
      │               │
      │         ┌─────▼──────┐
      │         │ Files      │  disk, or S3
      │         └─────▲──────┘
┌─────▼───────┐       │
│ PostgreSQL  │◄──┐   │
│ the truth   │   │   │
└─────────────┘   │   │
             ┌────┴───┴────────────────┐      ┌──────────────┐
             │ diffluence, role worker │◄────►│ NATS         │
             │ relay, jobs, listeners  │      │ events       │
             └────┬────────────────────┘      └──────────────┘
                  │ only outward, each optional
       ┌──────────┼──────────────┐
       ▼          ▼              ▼
  Mail server  Gotenberg     Collabora
```

In a small installation, `api`, `worker` and NATS are one process. draw.io and the PDF viewer are not in the picture because only browsers reach them, in a frame.

## The chapters

| Chapter | Covers | Lines |
|---|---|---|
| [1. The system and its roles](04-architecture/01-system-and-roles.md) | Roles, what runs where, the three shapes of an installation | 22 |
| [2. Modules](04-architecture/02-modules.md) | Who owns what, who may call whom, the frontend | 19 |
| [3. Events, jobs and periodic tasks](04-architecture/03-events-and-jobs.md) | The path of an event, every event with who sends and listens, fan-out, jobs, tasks | 27 |
| [4. Optional services](04-architecture/04-optional-services.md) | Mail, Gotenberg, office files, the PDF viewer, draw.io, frames, editing together | 58 |
| [5. The API and its edge](04-architecture/05-api-and-security-edge.md) | Conventions, access rules, middleware, headers, limits | 31 |
| [6. Live updates and caching](04-architecture/06-real-time-and-caching.md) | Polling or push, and what may be cached | 14 |
| [7. Data and storage](04-architecture/07-data-and-storage.md) | PostgreSQL, PgBouncer, what grows, search, files | 25 |
| [8. Scaling](04-architecture/08-scaling.md) | The load worked out, what hurts first, topology, the worst case and room for four times the database work | 31 |
| [9. Failures, upgrades and trade-offs](04-architecture/09-failures-and-trade-offs.md) | What breaks and what people see, secrets, every trade-off, risks | 14 |
| [Sources](04-architecture/sources.md) | What was read | |

In total 241 lines.

## The principles

1. **One binary, three roles,** from one small server to a cluster (chapter 1).
2. **PostgreSQL is the truth.** Everything else can be rebuilt, or can wait.
3. **A change, its audit and its event are one transaction.** Nothing is lost and nothing is invented.
4. **Permissions are never cached.** A restriction works at once, on every node.
5. **Nodes hold nothing.** Any node can die at any moment.
6. **Requests stay fast.** Slow work goes to the worker, and a helper is never on the path of a request.
7. **Helpers are optional, and a file is hostile,** also after a helper has touched it.
8. **Nothing leaves by default.** No outside address is called unless an administrator named it.

## What the study found

The most important findings, in order.

1. **All changes pass through one lock.** `audit.Record` holds a global lock until commit. Normal use is far below its limit, but a long transaction that audits early stops every login and every change in the instance. Copying a large branch does that today (ARC-201).
2. **The worker does one job at a time, and all periodic tasks in one loop.** That is fine for imports alone. Waves D, F, G and H all add long jobs, so this must change first (ARC-063, ARC-068).
3. **Fan-out needs a new question.** "Who may see this page" does not exist yet, and without it one publish in a watched space means thousands of permission checks (ARC-058, ARC-059).
4. **JetStream's defaults redeliver for ever.** Each consumer must set a limit and keep its failures (ARC-050).
5. **Every frame is blocked today.** `X-Frame-Options: DENY`, `frame-ancestors 'none'`, no `frame-src` and `Cross-Origin-Resource-Policy` together refuse the PDF viewer and draw.io. The content policy must be built from the configuration (ARC-096, ARC-107).
6. **The PDF viewer cannot have `allow-same-origin` on our origin,** or it could remove its own sandbox. It gets its file by `postMessage` instead (ARC-097).
7. **A preview needs no WOPI.** Blueprint 03 asks for previews, so one `Convert` call is enough, with far less attack surface (ARC-088, ARC-092).
8. **Two ways of drafting will live side by side,** because the editing service is optional (ARC-120).
9. **Migrations cannot run through PgBouncer in transaction mode** (ARC-178), and `/readyz` should not take API nodes down with NATS (ARC-216).
10. **Images are downloaded again on every view.** A small header change saves that, with no loss of safety (ARC-167).

## What changed from the first version of this text

| The first text said | Now | Why |
|---|---|---|
| WebSocket for live updates | Polling, and a WebSocket only for editing together | Chapter 6 |
| Jobs run through NATS | Jobs are in PostgreSQL | ADR-018 |
| Audit and search work from events | Both are written in the same transaction | Stricter, and ORG-013 asks for it |
| Spec-first OpenAPI, generated client | OpenAPI written from the routes and tested, client by hand | Spec 015 D19 |
| `sqlc`, `bluemonday`, TanStack Router, a Radix library | Not used | ADR-016 |
| Pandoc for DOCX and ODT | Our own writers | Spec 014 D2 |
| Headless Chromium helper of our own | Gotenberg | ADR-026 |
| Search is pluggable | It is built into the transaction | ARC-186 |
| `collab` reached by the proxy | The binary proxies `/collab` | ARC-118 |

The stack as it is stands in `docs/dependencies.md`.

## Public publishing

Public pages **never** come from the instance. The worker renders chosen pages to a static website, into a folder or a zip (ADR-010, ADR-026). The public side has no database, no accounts, no write API and no way back. A scan stops a build that holds any trace of the instance. See spec 014.

## Linked instances (later)

All ids are UUIDv7, and every content row records its `origin_instance_id`. Instances will trust each other through mutual TLS and exchange signed, versioned bundles, which spec 014 prepares. Merging instances after an acquisition then becomes a data operation, not a rewrite.

## For the product owner to decide

1. **Private drafts with editing together.** ADR-026 says yes to editing together, and spec 015 says that this changes a promise: on a page in shared mode, everyone with edit rights sees a draft while it is typed (PAG-013 against COL-092). Is that what you want? Advice: yes, with one clear line in the editor, and private drafts remain where the service is not installed.
2. **Polling instead of a WebSocket.** The first text promised live updates. The bell and presence can lag up to 30 seconds. Advice: accept it, and record it in an ADR. The code path to something faster stays open (ARC-164).
3. **Our own DOCX and ODT writers instead of Pandoc** (spec 014 D2). One sidecar less, and more code of ours. Advice: accept it, and record it in an ADR.
4. **Files through the API, always.** It is the safe choice, and in the large shape API nodes then carry every byte. Advice: keep it.
5. **The S3 client for phase 3.** It is a new dependency, and writing the signing by hand is against your rule. To be decided when phase 3 starts: its study compares the candidates.
