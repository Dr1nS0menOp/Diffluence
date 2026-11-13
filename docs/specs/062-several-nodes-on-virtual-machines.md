# Spec 062: several nodes on virtual machines (wave 8 of spec 048)

Status: plan and scripts, ready for the sessions on the test host. Nothing here has run on
the host yet. Lines covered: OPS-084, which covers OPS-011 to OPS-013, and the queue's half
of OPS-083. Study: `docs/blueprint/07-operations/03-high-availability.md`; decision D13 of
spec 048 (Patroni with 3 etcd members and the watchdog on, NATS with 3 replicas per stream
and a sync policy chosen by measurement).

The aim is a small version of what Jepsen does, done with plain tools. Run the topology of
blueprint 07 under load, break each part on purpose, and prove two things:
- no acknowledged write is lost;
- no page shows two truths.

## 1. The topology

Eleven VMs on one test host with 12 cores, 66 GB of memory and about 3 TB of thin
storage, plus the load VM from the stress tests. The stress app VM (900, 32 GB) stays
stopped during these sessions.

| VM | Id | vCPU | Memory | Disk | Runs |
|---|---|---|---|---|---|
| db1, db2, db3 | 910–912 | 2 each | 6 GB each | 60 GB each | PostgreSQL 16, Patroni, one etcd member, the softdog watchdog |
| nats1, nats2, nats3 | 913–915 | 1 each | 2 GB each | 20 GB each | NATS JetStream cluster member |
| edge | 916 | 2 | 2 GB | 20 GB | HAProxy (leader on 5000, replicas on 5001, app on 80) and PgBouncer (6432, transaction mode) |
| api1, api2 | 917–918 | 2 each | 4 GB each | 20 GB each | `diffluence` with `DIFFLUENCE_ROLE=api`, from the `.deb` of wave O5 |
| work1, work2 | 919–920 | 1 each | 3 GB each | 20 GB each | `diffluence` with `DIFFLUENCE_ROLE=worker` |
| load | 901 | 4 | 8 GB | 30 GB | k6, the write ledger |

That is 21 vCPUs on 12 cores (1.75 times overcommitted, which suits a test where most
nodes idle) and 48 GB of the 66. The sizes, ids and roles live in
`scripts/ha/topology.env`.

**The paths a request takes:**

```
k6 / ledger ──► edge:80 HAProxy ──► api1 | api2  (readiness /readyz, no stickiness)
api, worker ──► edge:6432 PgBouncer ──► edge:5000 HAProxy ──► the Patroni leader
migrations  ──► edge:5000 HAProxy ──► the leader (a session lock: never through PgBouncer)
api, worker ──► nats1, nats2, nats3 (one URL list)
db1..db3    ──► etcd on db1..db3 (the leader key), the watchdog on each
```

## 2. Provisioning

Everything is in `scripts/ha/`, described in its README. One session builds it:
1. On the host, `host-create-vms.sh` imports the cloud image once into template 9000 and
   makes every VM a linked clone. A clone takes seconds; an import took about half an hour
   in the first stress test. Each clone gets a static address and the common cloud-init:
   chrony, nftables, the guest agent and softdog.
2. On db1 to db3, `roles/db.sh` sets up etcd, then Patroni from
   `conf/patroni.yml.tmpl`, with `--validate-config` before it starts.
3. On nats1 to nats3, `roles/nats.sh` installs the pinned NATS release, with its checksum
   checked, and `nats-server -t` on its config. The sync policy is a setting,
   `HA_NATS_SYNC`.
4. On the edge, `roles/edge.sh` sets up HAProxy (with `haproxy -c`) and PgBouncer, and
   creates the database and its two roles on the leader. Passwords reach `psql` through
   the environment, never through the command line.
5. On api1 first, then the rest, `roles/app.sh` installs the `.deb`, writes the
   environment file, runs `diffluence config check --offline`, and starts the service.

The addresses and secrets of one run live in `scripts/ha/ha.local.env`, which git ignores.
An example sits next to it.

`scripts/ha/validate.sh` checks all of it without a host:
- `bash -n` on every script, and the Python ledger;
- the validators of HAProxy, NATS and Patroni in throw-away containers. Patroni gets a
  stand-in etcd.

`shellcheck` passes on every script.

## 3. The tests

**Under every test:**
- **Load:** k6 runs the realistic mix of `scripts/stress/load.js` at 100 active people.
- **Writes:** `scripts/ha/tests/ledger.py write` adds comments with a unique mark, one
  thread per token of a different person, paced below the per-person and per-token limits.
  It records every acknowledged mark.
- **Faults:** `scripts/ha/tests/faults.sh` makes the fault and prints UTC times, so the
  error window can be read from k6's output.

**After every test**, `ledger.py verify` reads the page through api1 and api2 separately.
It fails on:
- an acknowledged mark that is missing (a lost write);
- a mark found twice (a doubled write);
- a different set on the two nodes (two truths).

| Id | Test | Fault | Passes when |
|---|---|---|---|
| T0 | The baseline | none | Both api nodes ready. `patronictl list` shows a leader, one synchronous and one asynchronous standby. NATS shows 3 routes. The stream holds 3 replicas (after F1, section 4). Ledger clean. |
| T1 | An api node dies | `kill api1 diffluence` | HAProxy takes it out within 4 s (fall 2, inter 2 s). Failed requests are only those in flight on that node. Ledger clean. |
| T2 | A worker dies | `kill work1 diffluence` | The other worker takes over the relay and the jobs once the dead node's leases end. No job is lost or done twice. The outbox drains. Ledger clean. |
| T3 | The leader dies | `reset-vm` of the leader's VM | A new leader within 30 s (the ttl). Writes fail only within that window and then go on. Ledger clean: no acknowledged write lost, because of synchronous replication. The old node rejoins as a standby with `pg_rewind`. |
| T4 | The leader freezes | `stop-primary 60`, then `pause-vm` of the leader for 60 s | A new leader within 30 s. The old one, when it wakes, accepts no write: Patroni demotes it, or the watchdog resets it. Ledger shows no two truths. |
| T5 | The network splits | `cut db1` from db2 and from db3, with db1 the leader, for 60 s | db1 loses the leader key and stops taking writes within the ttl. db2 or db3 becomes the leader. When the network heals, db1 rejoins as a standby. Ledger clean. |
| T6 | A queue node dies, then all of them | `kill nats2 nats`, then `reset-vm` of all three at once, once per sync policy | One node down: publishing goes on and nothing is lost. All three reset: count the events the outbox marked published against what reached a consumer. The result decides the policy (D7). |
| T7 | A rolling upgrade under load | `tests/rolling-upgrade.sh` with the next build | Workers first, then api nodes, one at a time. No error other than 503 with `Retry-After` while a node drains. Both releases serve side by side. Ledger clean. |
| T8 | The edge dies | `kill edge haproxy`, then `pgbouncer` | The service stops: this is the known single point of D2. It comes back when the process restarts. No write that was acknowledged before is lost. |

**OPS-083 in T6.** The measure for each policy (`default`, `1s`, `always`):
- events a second published through the cluster, with a NATS benchmark from the load VM
  and from the outbox relay's own rate;
- events lost after the reset of all three.

Published outbox rows are kept 7 days already (`outbox.PurgePublished`), so lost events
can be sent again. F3 in section 4 says how.

## 4. What the app needs

Checked by reading the code at `503220b`.

| | Finding | Needed |
|---|---|---|
| F1 | `queue.Open` creates the stream without `Replicas`, so in a cluster it has 1 replica, and losing that node loses it | **Yes:** a setting `DIFFLUENCE_NATS_REPLICAS` (default 1, 3 for a cluster). `CreateOrUpdateStream` also raises it on an existing stream. With a test. |
| F2 | `db.Open` sends `plan_cache_mode` and `jit` as startup parameters (spec 061). PgBouncer refuses unknown startup parameters, and with `ignore_startup_parameters` it drops them. | **Yes:** a migration sets both for the database with `ALTER DATABASE … SET`, so they hold behind PgBouncer too. Until then, the Patroni template sets them server-wide and PgBouncer ignores them (D6). |
| F3 | After a hard reset of the queue, published events may be gone (T6 shows how many) | **If T6 loses events under the chosen policy:** `diffluence outbox resend --since <time>`, sending published rows again with the event id as the message id, so the queue's two-minute duplicate window and idempotent consumers drop repeats. Check first that the relay sets the message id. |
| F4 | `DIFFLUENCE_NATS_URL` with a comma-separated list | **No:** `nats.Connect` takes a list as it is. |
| F5 | Several database hosts or `target_session_attrs` | **No:** HAProxy routes to the leader. pgx can also take a host list, if an operator wants that without HAProxy. |
| F6 | Prepared statements behind PgBouncer | **No change:** pgx prepares statements, and PgBouncer 1.21 or later keeps them with `max_prepared_statements`. Ubuntu 24.04 ships 1.22. OPS-086 still wants the whole suite run behind PgBouncer once, which needs no server and can be a local test. |
| F7 | The migration lock is a session advisory lock | **No change:** migrations use `DIFFLUENCE_MIGRATE_DATABASE_URL` to HAProxy's port 5000, around PgBouncer. |
| F8 | Sessions | **No stickiness:** sessions live in PostgreSQL, so any api node serves anyone. The editing service is one node and stays out of this topology (`install.md`: more than one needs routing by page, not built). |

| F9 | Files live in each node's own data folder | **Yes, for files across nodes:** S3 storage (wave 9), or one shared folder for every app node as a stopgap. The tests of this wave use comments and pages, which live in PostgreSQL. |

F1 and F2 should be built before session S2, as one small wave with tests. They need no
server.

## 5. The sessions on the host

Each session is at most one hour, the limit the product owner set. The Sunday session of
2026-11-15 is the re-measure of wave PF, not this wave. Wave 8 needs four later sessions:

| Session | Within one hour | Leaves behind |
|---|---|---|
| S1 | Template and 11 clones (about 10 min); `roles/db.sh` on three nodes and a check with `patronictl list`; `roles/nats.sh` on three; `roles/edge.sh`; a snapshot `ha-base` of every VM | A running database and queue cluster, and a snapshot to return to |
| S2 | F1 and F2 deployed through the `.deb`; `roles/app.sh` on four nodes; T0, T1, T2 | Baseline numbers, and the two app-node tests |
| S3 | T3, T4, T5, each followed by `ledger.py verify` and a return to `ha-base` if a node is left broken | The fail-over results |
| S4 | T6 with the three sync policies, the NATS benchmark, T7 and T8, then the report | The policy decision, and the upgrade and edge results |

Every session starts with `bash scripts/ha/validate.sh` on a development machine. It stops
at the hour, even when a test is unfinished; the next session goes on from the snapshot.
If the host freezes, as it did in the first stress test, the session stops at once.

## 6. Decided on the product owner's behalf on 2026-11-13, may be overruled

| | Decision | Why |
|---|---|---|
| D1 | The VMs use the host's existing bridge; partitions are made inside the guests with nftables | No change to the host's network. A partition between two guests is what the tests need, and nftables makes it exactly. |
| D2 | One edge VM for HAProxy and PgBouncer: a known single point in this wave | A pair with a floating address (keepalived) is a separate proof. T8 shows what one edge costs. |
| D3 | etcd runs on the three database nodes | Three members are the minimum for a majority. Separate etcd VMs would cost three more VMs on a host that is already full. |
| D4 | No sticky sessions for the api nodes | Sessions are in PostgreSQL (F8). |
| D5 | `synchronous_mode: true`, not strict | An acknowledged write is on two nodes. With one node left, the cluster keeps taking writes instead of stopping all of them. The strict mode is a later choice for installations that would rather stop. |
| D6 | `plan_cache_mode` and `jit` are server settings in this topology, and PgBouncer ignores the startup parameters | Without it, PgBouncer refuses every connection of the app. F2 makes it right in the app. |
| D7 | The NATS sync policy for clusters is `always` if T6 shows it keeps at least twice the highest event rate of the stress tests; otherwise `1s` with F3 built | Losing acknowledged events is worse than a slower queue, as long as it keeps up with real load. |
| D8 | The app nodes run the `.deb` of wave O5, not containers | It tests the packages on real machines too. The units and folders are the ones operators get. |
| D9 | The load VM of the stress tests is reused, and the stress app VM stays stopped | Its 32 GB would not fit beside the topology. |

## 7. What this plan does not prove

- **Hardware:** a second physical host or a real network switch. All VMs share one host,
  one disk array and one power supply.
- **Clusters at the scale of the 5,000-person load test:** that needs server-class
  hardware (OPS-109).
- **S3 storage for files (wave 9):** the app nodes in this topology keep files on their
  own disk. A file uploaded through api1 is not on api2. Tests that read files across
  nodes therefore wait for wave 9. The ledger uses comments, which live in PostgreSQL.
