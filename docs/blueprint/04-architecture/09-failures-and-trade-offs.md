# 04.9: Failures, upgrades and trade-offs

## 9.1 When something breaks

| What fails | What people see | What happens to work | How it recovers |
|---|---|---|---|
| An API node | Nothing, or one failed request | Typed text stays in the browser | The load balancer leaves it out on `/readyz` |
| A worker | Background work waits | Nothing is lost: rows wait in tables | Another worker claims them. A job left running fails, with a message |
| PostgreSQL | "Please try again shortly", and `/readyz` fails | Autosave retries from the browser | A standby takes over, and nodes reconnect |
| NATS | Pages work. Notifications wait | Events wait in the outbox | The relay publishes them when NATS returns |
| The store | Files fail, and `/readyz` fails | Text still saves on the node that can reach PostgreSQL | Service restored |
| Mail server | No mail | Mail waits, five tries, then failed and visible | Retry from the console |
| Gotenberg, Collabora | PDF or preview says it failed | Nothing | Try again |
| draw.io | The editor does not open. Stored diagrams still show | Nothing | Service restored |
| Editing service | "Changes are not reaching others" | Typing stays safe in the browser | Reconnect merges |
| Identity provider | No new logins that way | Open sessions keep working | Service restored. A local administrator account stays possible |
| The reverse proxy | Everything | Nothing lost | Operator |

- **ARC-214** · built · A failed read of the session answers 503, and never treats a person as logged out.
- **ARC-215** · built · NATS being down does not stop changes: the outbox is in PostgreSQL. `/readyz` still reports it, so a node with a broken connection leaves the pool.
- **ARC-216** · built · That last point makes a NATS outage take down all API nodes, although they could serve people. An API node only needs NATS for health, so `/readyz` on role `api` should warn, and not fail. Only a worker needs it.
- **ARC-217** · built · A failed save keeps text in the browser until it can be saved (PAG-016).
- **ARC-218** · phase 3 · Every row above is drilled at least once, and the kill test of phase 3 covers the first four.

## 9.2 Upgrades

Blueprint 07 holds the rules. What the architecture adds:

- **ARC-219** · built · Two versions run side by side, so a migration only adds. CI starts the old version with traffic, and upgrades under it.
- **ARC-220** · built · A new document block is refused by a node that does not know it yet, so a person may see a save fail during a rolling upgrade of a few minutes. The draft stays in the browser.
- **ARC-221** · built · A consumer ignores an event type or a field it does not know. Old and new workers then share a stream safely.
- **ARC-222** · built · A handler that changes meaning gets a new event type. Old types stay until the stream has aged out, which is 7 days.
- **ARC-223** · built · `index.html` is never cached, and assets have a hash, so people get a new frontend on their next load. An open tab keeps its old code and talks to a new API, which only grows.
- **ARC-224** · built · An open tab can meet a node that lacks a new route during a rolling upgrade. A screen treats that as a failed action, and the capability list keeps new features hidden until all nodes know them.
- **ARC-225** · built · The service for editing together is upgraded with the release. Its clients reconnect, and a client with old code is asked to reload.

## 9.3 Who holds which secret

| Place | Secret | Comes from |
|---|---|---|
| API and worker | Database address, master key, OIDC secret | File or environment |
| Worker | SMTP password | File or environment |
| API | Service token for the editing service | File or environment |
| Editing service | Its service token, nothing else | File or environment |
| Gotenberg, Collabora, draw.io | Nothing | Nothing |
| The database | Sealed: TOTP keys, webhook secrets. Hashed: sessions, tokens | `internal/keys` |

- **ARC-226** · built · A secret never reaches a log, an error or an audit detail. `_FILE` variables keep them out of the process list.
- **ARC-227** · built · The editing service holds no database password and no master key. If it is taken over, it can see the documents people have open, and nothing else.

## 9.4 The trade-offs

Every line says what we chose, what it costs, and what would change our mind.

| # | Choice | Why | The cost | Would change it |
|---|---|---|---|---|
| T1 | One binary, not services | One thing to install, test and upgrade | All of it scales together, and a bug can reach all of it | A part that needs another runtime or very different scale |
| T2 | PostgreSQL for everything | One backup, one consistent moment, permissions in queries | One primary is the ceiling | Never for content. Search maybe |
| T3 | NATS for events | Embedded for small, cluster for large, and the relay gives commit order for free | In the large shape, three more servers to run | See below |
| T4 | Jobs in PostgreSQL | Same transaction as the request, one moving part fewer | Polling, and a queue table to keep small | Thousands of jobs a minute |
| T5 | Full-text search in PostgreSQL | Never stale, and permissions in the query | Ranking on common words, no stemming | The product owner, after a measurement (ARC-186) |
| T6 | No cache of permissions | A restriction works at once, on every node | About 20 small queries per page view | Never for decisions |
| T7 | Polling, not push | No state on nodes, works through any proxy | Up to 30 seconds of delay, and steady small traffic | People asking for faster |
| T8 | Files through the API | Every read is checked, audited and safe | Nodes carry the bytes | Nothing soon: safety comes first |
| T9 | A whole document per version | Simple, restore is trivial, comparison is cheap | The largest table | Storage cost at real scale |
| T10 | Hash chain with a global lock | Tamper-evident, in order, at once | One door for all changes | A measurement (ARC-203) |
| T11 | Helpers are optional | A small install stays small | Every feature has two states, and the test matrix grows | Nothing. It is the product owner's rule |
| T12 | A second runtime for editing together | Yjs is the mature choice, and Go ports are young | Node.js updates next to Go, and one more service | A mature Go port |
| T13 | Text anchors for inline comments | Works with immutable versions, and for readers | A moved sentence can lose its place | Nothing better is known |
| T14 | One instance per organisation | No tenant bugs, simple permissions | Hosting many customers means many instances | Not planned (ADR-008) |
| T15 | Previews, not office editing | No WOPI surface | People download to edit | A request from the product owner, with a new study |

**About T3.** Jobs left NATS for PostgreSQL (ADR-018), and until wave C nothing read the stream at all. It is fair to ask whether events could follow, with each listener keeping its own place in `outbox_events`. That would remove a component from the large shape. Against it: rows do not commit in the order of their ids, so a reader by position can miss a row that commits late. The relay solves that, as it publishes what is unpublished. We would also write our own redelivery, back-off and dead letters, which JetStream gives. So NATS stays. Revisit it if the cluster turns out to be the part operators struggle with.

## 9.5 Risks that remain

| Risk | Answer |
|---|---|
| PdfCraft is alpha, and embedding it is not described | A proof comes first. If it fails, PDF stays a download |
| draw.io sources disagree on self-hosted embedding | A proof comes first |
| Collabora's documentation could not be read for this study | Confirm the conversion path and image terms at build time |
| Two ways of drafting double a delicate path | ARC-120: tests for both, and a nightly run with the service on |
| The worker carries more and more | ARC-063 and ARC-068 first. Then `worker` scales by adding nodes |
| Optional helpers make combinations that nobody tests | One run with none, in every pull request, and one with all, every night |
