# 07.3: High availability: PostgreSQL, the queue and several nodes

Blueprint 07 plans an HA topology for phase 3: two or more `api` and `worker` nodes, a NATS JetStream cluster of three, and PostgreSQL with Patroni, PgBouncer and HAProxy. This chapter asks what goes wrong with each, and what must be proven before anyone relies on it.

## 3.1 The theory in short

- **Consensus needs a majority.** Raft (Ongaro and Ousterhout, USENIX ATC 2014) is the algorithm behind etcd, which Patroni uses, and behind NATS JetStream. A cluster of 3 survives the loss of 1, a cluster of 5 the loss of 2. An even number adds cost and no safety.
- **During a network split you choose.** Gilbert and Lynch proved Brewer's conjecture (2002): when the network splits, a system either refuses some requests or risks serving stale or conflicting data. Kleppmann's *Designing Data-Intensive Applications* explains the practical forms, and why "consistent" means different things in different products.
- **Fail-over without fencing is a split brain waiting to happen.** A primary that is only slow, or paused, may wake up and keep accepting writes while a new primary exists. Fencing means making sure the old one can no longer write before the new one starts.

## 3.2 What went wrong elsewhere

**GitHub, 21 October 2018.** A network link was lost for 43 seconds. The automatic fail-over moved the primary MySQL databases to another data centre. Both sides then held writes the other did not have, and they could not be merged safely. GitHub chose data integrity over speed, and the service was degraded for 24 hours and 11 minutes. Their post-incident analysis names the lesson: the fail-over tool could build a topology the application did not support.

**NATS JetStream under Jepsen, December 2025.** Jepsen tested NATS 2.12.1. With the default settings, JetStream acknowledges a write before it is flushed to disk (by default about every two minutes), so a power failure across nodes lost acknowledged writes, about 30 seconds of them in the test. Corrupted files on a minority of nodes could lose data or delete a stream, and one crashed operating system together with pauses or a partition left replicas permanently different. Some of these were fixed, others were still open when the report appeared. NATS now documents the risk of the lazy `fsync`. The lesson for anyone running a JetStream cluster: choose the sync policy on purpose, and treat the queue as a place where recent messages can be lost unless proven otherwise.

**Patroni.** Patroni keeps a leader key in a store such as etcd. It demotes a primary that loses the key, and it can use a hardware or software **watchdog** that resets the machine if Patroni itself hangs, so a stuck primary cannot keep writing. Without the watchdog, a paused machine can outlive its key.

## 3.3 How the industry builds it

| Part | Common choice | Why |
|---|---|---|
| PostgreSQL | Patroni with etcd (3 nodes), the watchdog on, synchronous replication where no loss is acceptable | Automatic fail-over with fencing |
| Connections | PgBouncer in transaction mode, HAProxy routing writes to the leader by Patroni's health endpoint | Many app connections over few database connections |
| Queue | NATS JetStream with 3 replicas per stream | A majority survives one loss |
| Application | Stateless nodes behind a load balancer, readiness checks, graceful drain | Any node can die |
| Files | S3-compatible storage with versioning and replication | Files outside any one machine |

## 3.4 Where Diffluence stands

| Part | State | Where |
|---|---|---|
| Roles `api` and `worker`, one shared key, node list | Built, never run on two machines | OPS-011 |
| Graceful drain: not ready first, then close | Built | `App.Serve` in `internal/app/app.go`, `ShutdownDelay` in `internal/config/config.go` |
| `/readyz` with `busy` and `draining` | Built | OPS-021 |
| PgBouncer-safe queries | Considered in blueprint 04 (pgx query modes) | `04-architecture/sources.md` |
| Events kept in an outbox in PostgreSQL until published | Built | `internal/outbox/outbox.go` |
| Jobs and periodic tasks coordinated through PostgreSQL | Built | ADR-018 |
| Embedded NATS for one node, external NATS by URL | Built | `internal/queue/queue.go` |
| NATS cluster, Patroni, HAProxy, S3 | Not run | OPS-012 to OPS-014 |

Diffluence has one design choice that limits the damage of a lost queue: jobs live in PostgreSQL, and events stay in the outbox until the queue accepted them. What the queue holds after that, such as notifications, webhooks, automation and the signal to the editing service, is only as safe as the queue's own storage.

## 3.5 Recommendations

- **OPS-083** · new · **High.** Decide the storage settings of the queue on purpose, for the embedded server and for a cluster, write them down, and prove what happens to events after a sudden power cut of the machine (a reset of the virtual machine, not a clean stop). Measure what a stricter sync policy costs in events a second. If acknowledged events can be lost, keep published rows in the outbox long enough to send them again, and give that a test.
- **OPS-084** · new · **High.** Before the HA topology is offered, run it on virtual machines: 2 `api`, 2 `worker`, 3 NATS, 3 PostgreSQL with Patroni and etcd, PgBouncer and HAProxy. Kill each kind of node under load, cut the network between them, pause a primary (`SIGSTOP` or a paused VM) and check that no acknowledged write is lost and no page shows two truths. This is a small version of what Jepsen does, done with plain tools. It covers OPS-011 to OPS-013.
- **OPS-085** · new · **Medium.** The reference Patroni set-up turns the watchdog on, uses 3 etcd members, and sets `maximum_lag_on_failover`. The install guide states what the operator gives up without them.
- **OPS-086** · new · **Medium.** Under PgBouncer in transaction mode, a test runs the whole suite with the pgx mode that PgBouncer supports, so no feature quietly depends on session state (advisory locks across statements, prepared statements, `SET` without `LOCAL`). The migration lock is a session lock, so migrations must connect around PgBouncer; the guide says so.
- **OPS-087** · new · **Low.** Write a short page "what HA protects and what it does not": it does not protect against a deleted space, a bad upgrade or a broken key. Those need backups (chapter 2).

## Sources

- Ongaro, Ousterhout: [In Search of an Understandable Consensus Algorithm](https://www.usenix.org/conference/atc14/technical-sessions/presentation/ongaro), USENIX ATC 2014.
- Gilbert, Lynch: [Brewer's Conjecture and the Feasibility of Consistent, Available, Partition-Tolerant Web Services](https://doi.org/10.1145/564585.564601), SIGACT News 2002.
- Kleppmann: [Designing Data-Intensive Applications](https://dataintensive.net/), O'Reilly 2017, chapters 5, 8 and 9.
- GitHub: [October 21 post-incident analysis](https://github.blog/2018-10-30-oct21-post-incident-analysis/), 2018.
- Jepsen: [NATS 2.12.1](https://jepsen.io/analyses/nats-2.12.1), 8 December 2025.
- NATS: [JetStream clustering](https://docs.nats.io/running-a-nats-service/configuration/clustering/jetstream_clustering), [JetStream concepts](https://docs.nats.io/nats-concepts/jetstream).
- Patroni: [Documentation](https://patroni.readthedocs.io/en/latest/), [Watchdog support](https://patroni.readthedocs.io/en/latest/watchdog.html).
- PgBouncer: [Features, what transaction pooling does not support](https://www.pgbouncer.org/features.html).
- PostgreSQL 16: [High availability, load balancing and replication](https://www.postgresql.org/docs/16/high-availability.html).

All sources of this study are also in [sources.md](sources.md).
