# Spec 068: events without NATS

Status: study and plan, 2026-11-13. Nothing in this spec is built yet. Blueprint lines: ADR-007,
ADR-018, OPS-012, OPS-083; spec 062 (F1, F3, T6) and spec 063 (F1).

Written for: the people who build and run Diffluence, and the product owner, who decides.

## Why

On 2026-11-13 the product owner decided that NATS may go, on one condition: PostgreSQL alone must
carry the events and their routing, and scale up and out, with several app nodes, to 5,000 people
with four times the database headroom of `docs/operations/limits.md`.

NATS came in with ADR-007 for durable streams and an embedded mode. Since then three things have
moved:

- ADR-018 put jobs and periodic tasks in PostgreSQL (`jobs` table, `SKIP LOCKED`, lease rows), and
  the mail queue and the audit sealer (ADR-041) followed the same way. Events are the only work
  that still leaves the database.
- Every listener of the event stream runs in the worker role of the same binary, and every one of
  them reads and writes PostgreSQL.
- Running on several nodes needs an external NATS cluster of three nodes (OPS-012, spec 062), its
  replica setting (spec 063), and a decision on the queue's storage settings with a proof of what
  a hard reset does to events (OPS-083). Those are still to be decided and proven.

This spec answers whether PostgreSQL can do the job, and how, in five parts: what flows today,
the design, the load budget, what is lost and gained, and a recommendation with waves.

Decisions are marked **Dn**. Each was decided on the product owner's behalf on 2026-11-13, and
may be overruled.

## 1. What flows today

### 1.1 The path of an event

1. A change calls `outbox.Enqueue(ctx, tx, type, payload)` inside its own transaction. The row
   goes into `outbox_events (id, type, payload jsonb, created_at, published_at)`, so the event
   commits or rolls back with the change. Most modules do this in one helper next to
   `audit.Record`, with the audit action as the event type (for example `pages.go:205-222`).
2. The relay in the worker (`outbox.Relay`) polls every 500 ms, claims up to 100 rows with
   `FOR UPDATE SKIP LOCKED` in `created_at, id` order, publishes each to the subject
   `diffluence.events.<type>` with the event id as the message id, and sets `published_at` in the
   same transaction. Events over 900 KiB are set aside as failures.
3. JetStream keeps the stream `DIFFLUENCE_EVENTS` (file storage, 7 days, duplicate window
   2 minutes, replicas from spec 063).
4. Four durable consumers, all in the worker role, get every event. Each acks on success, or
   asks again after 10 s, 1 min, 5 min, 15 min and 1 h; after that the event goes to
   `event_failures`, where an administrator can replay it (`failures.RunReplays`).
5. Published outbox rows are deleted after 7 days (`outbox.PurgePublished`).

API nodes publish nothing and consume nothing. They connect to NATS only so that readiness
(`needsQueueToBeReady`), the upgrade check (`ProblemQueue`) and the gauge
`diffluence_queue_healthy` can report it.

### 1.2 Producers

62 calls of `outbox.Enqueue` in 40 files (test files left out), by module. A type written as
`<action>` comes from a helper that uses the audit action, so every audited action of that
module is also an event.

| Module | Event types |
|---|---|
| pages | `<action>` for page changes (`page.created`, `page.published`, `page.moved`, `page.archived`, `page.trashed`, `page.labelled`, `page.stage_changed`, `page.owner_changed`, `page.withdrawn`, `page.controlled`, `page.version_removed`, `space.workflow_set`, `space.labels_fixed`, `space.label_renamed`, `space.stages_changed`, `space.review_changed`, `space.content_action`, `space.approval_rule_changed` and others), `page.approval_asked`, `page.approved`, `page.review_due`, `page.schedule_failed`, `page.draft_expiring`, `page.purged` |
| comments | `<action>` (`comment.created`, `comment.edited`, …), `reaction.added` and the other reaction actions, `comment.purged`, `comment.quote_blanked` |
| attachments | `attachment.uploaded`, `attachment.trashed`, `attachment.restored`, `attachment.purged`, version actions |
| spaces | `<action>` (`space.created`, `space.archived`, `space.trashed`, `space.purged`, …), personal space actions |
| permissions, sharing | `permission.*`, `restriction.*`, `page.shared`, `access.requested`, `access.allowed`, `access.refused`, `access.recovered`, invitation actions |
| identity | `user.created`, `auth.login_succeeded`, `auth.password_reset`, user actions (`user.disabled`, `user.expired`, `user.anonymised`, …), group actions (`group.member_added`, `group.member_removed`, `group.deleted`), factor actions |
| exchange | `export.created`, `export.downloaded`, `export.expired`, `export.failed`, `export.finished`, `export.policy_changed`, `import.started`, `import.mapped`, `import.undone`, import results, `bundle.imported`, `space.pdf_design_changed`, exchange actions |
| settings, console | `settings.updated`, `settings.logo_changed`, `settings.font_changed`, `maintenance.changed`, `storage.unused_removed` |
| automation | rule actions, `rule.limit_changed`, and the events a rule's own actions cause (`match.go:237`) |
| webhooks, watches, templates, sites, analytics, jobs, failures | their audited actions, `analytics.settings_changed`, `analytics.data_deleted`, `job.retried` and the other job changes, `event.replay_requested` |

### 1.3 Consumers

| Durable | Handler | Types it acts on | What it does | Effect of a delay |
|---|---|---|---|---|
| `notifications` | `notify.Handle`, then `sites.Handle` (`app.go:523-532`) | notify: `page.published`, `comment.created`, `comment.edited`, `reaction.added`, `access.requested`, `access.allowed`, `access.refused`, `access.recovered`, `export.finished`, `page.draft_expiring`. Sites: the 12 types of `eventsThatCanHide` and `page.published` | Writes notification rows for watchers and mentioned people; withdraws public pages that may no longer be public, and asks for a site build | A notification comes late. A public page that should be withdrawn stays up longer |
| `automation` | `automation` matcher (`match.go:111`) | the types of `eventsToTriggers` plus `page.published` and `comment.created` | One rule query per trigger and space; a match queues an `automation.run` job | A rule runs late |
| `webhooks` | `webhooks.Handle` (`webhooks.go:272`) | any type an endpoint subscribes to | `INSERT … ON CONFLICT (endpoint_id, event_id) DO NOTHING` into `webhook_deliveries`; the mail-style queue sends them | A delivery goes out late |
| `collab` | `collab.Handle` (`collab.go:244`) | `permission.*`, `restriction.*` and the 9 types of `accessEvents` | `POST /recheck` to the editing service | An editor whose access ended keeps the socket until the recheck or the next minute |

What each consumer needs:

- **At least once.** All four must be safe to run twice (`queue.go` says so, and webhooks enforce
  it with the unique key). None needs exactly once.
- **Order.** None depends on order today. JetStream delivers a durable's events in stream order
  to one worker, but a retried event comes back after later ones, and with several workers on one
  durable the order between them is not kept. Every handler reads the current state from
  PostgreSQL (the sites rule, the notification recipients, the rule matcher), so an older event
  handled late does not undo a newer one.
- **Latency.** Today a committed event waits up to 500 ms for the relay and then milliseconds.
  Nothing has a written target; the strictest is collab and the sites withdrawal, where a delay
  keeps access open. This spec sets one (D9).
- **Fan-out to nodes.** None. Each event goes to one worker per durable. Per-node caches (the
  permission memo, ARC-197) are checked against a generation in PostgreSQL, and collab is reached
  over HTTP from whichever worker handles the event. No node needs a broadcast.

### 1.4 Volume at 5,000 people

No run measured events a second: the stress tests counted requests, and the outbox was not
watched. The volume below is derived from the load model of spec 028 (`limits.md`: about 5,000
people, 10% active, 130 reads, 2 saves, 40 searches while typing and 10 results pages a second),
plus assumptions that are marked.

| Source | Events a second at the model | How |
|---|---|---|
| Saves (publish) | 2 | The model; one `page.published` each |
| Comments and reactions | 1 | Assumed: half a comment or reaction per save |
| Logins | 1.4, up to 8 in the busiest 10 minutes | Assumed: every person logs in once in the first working hour |
| Everything else (moves, labels, uploads, approvals, admin) | 0.5 | Assumed |
| **Total** | **about 5**, peaks of about 12 | |
| **Four times the model (`limits.md` headroom)** | **20**, peaks of about 50 | |
| All 5,000 at once (stress test 2) | about 85 for a minute | 5,000 logins in a ramp of one minute; most other requests were answered busy |

A day at the model is about 5 events a second for 8 hours, so about 150,000 events; at four
times, about 600,000. Payloads hold ids; assume 600 bytes a row with its indexes. This volume must
be measured (section 3.4).

## 2. The design

### 2.1 Overview

The outbox table stays as it is, so the saving side does not change. What NATS did is replaced by
three tables and two loops in the worker:

```
change tx ──INSERT──▶ outbox_events          (as today: commits with the change)
                           │
            sequencer (one at a time, under an advisory xact lock)
                           │  moves rows in order, gives each a gapless position
                           ▼
                      event_log (pos, lane, id, type, payload, …)   ── NOTIFY diffluence_events
                           │
      ┌────────────┬───────┴─────┬────────────┐
 notifications  automation   webhooks       collab        one cursor per (consumer, lane),
   × lanes        × lanes     × lanes       × lanes       held by a lease, advanced in batches
      │
 failure ─▶ event_retries (consumer, pos, tries, next_at) ─▶ event_failures (as today)
```

### 2.2 Positions in visibility order: the sequencer

A `bigserial` given at insert does not work as a cursor: a transaction that took number 10 and
commits after number 11 becomes visible after a reader has passed 11, and is skipped. The audit
chain had the same problem and solved it in ADR-041: a single sealer takes what is visible, in
order, under a lock. This design does the same.

**D1. A sequencer moves events from `outbox_events` into `event_log` and gives them positions.**
One transaction per batch of up to 500:

1. `SELECT pg_advisory_xact_lock(<events lock>)`: one sequencer at a time on any number of nodes.
   A transaction-level lock works through PgBouncer in transaction mode.
2. Read the oldest waiting rows, `ORDER BY created_at, id LIMIT 500 FOR UPDATE`.
3. `UPDATE event_head SET pos = pos + n RETURNING pos`: a single row, so positions are gapless and
   transactional. A PostgreSQL sequence is not used: it is not transactional, and after a crash it
   can hand out a value twice if its log record was not on disk.
4. Insert the rows into `event_log` with positions `head-n+1 … head` and their lane (2.4).
5. Delete them from `outbox_events`.
6. `SELECT pg_notify('diffluence_events', '<head>')`.
7. Commit, with `synchronous_commit` as the server has it (on).

Because the lock is held until commit, the next batch sees this one committed, and every row it
takes became visible later. So positions follow visibility, there are no gaps, and a consumer at
position p has seen everything at or below p. A gap is a fault, and the ledger check (2.10) looks
for one.

A long transaction elsewhere does not hold the sequencer up: it only sees what is committed. The
alternative of reading `outbox_events` directly by transaction id, below
`pg_snapshot_xmin(pg_current_snapshot())`, needs no sequencer but makes every event wait for the
oldest open transaction on the server, such as a long import. **D2. Not used**, for that reason.

The sequencer runs in the worker every 250 ms when idle, and at once again after a full batch. An
idle tick costs one `SELECT EXISTS (SELECT 1 FROM outbox_events)` (D7 explains why saves do not
wake it).

### 2.3 Consumers, cursors and leases

```sql
CREATE TABLE event_head    (one boolean PRIMARY KEY DEFAULT true CHECK (one), pos bigint NOT NULL);
CREATE TABLE event_log     (pos bigint PRIMARY KEY, lane smallint NOT NULL, id uuid NOT NULL UNIQUE,
                            type text NOT NULL, payload jsonb NOT NULL, created_at timestamptz NOT NULL,
                            sequenced_at timestamptz NOT NULL DEFAULT now(), trace text);
CREATE INDEX event_log_lane_pos ON event_log (lane, pos);
CREATE TABLE event_cursors (consumer text, lane smallint, pos bigint NOT NULL,
                            lease_owner uuid, lease_until timestamptz,
                            PRIMARY KEY (consumer, lane)) WITH (fillfactor = 50);
CREATE TABLE event_retries (consumer text, pos bigint, event_id uuid NOT NULL, tries int NOT NULL,
                            next_at timestamptz NOT NULL, last_error text NOT NULL,
                            PRIMARY KEY (consumer, pos));
CREATE INDEX event_retries_due ON event_retries (next_at);
```

**D3. A cursor per (consumer, lane), held by a lease, not by a transaction.** A worker takes a
lease with `UPDATE event_cursors SET lease_owner = $node, lease_until = now() + 1 min WHERE
consumer = $1 AND lane = $2 AND (lease_until IS NULL OR lease_until < now() OR lease_owner =
$node)`, the pattern of the periodic tasks (ADR-018). No transaction stays open while a handler
runs, so a slow handler (collab's HTTP call) holds no connection and no lock, and the lease works
through PgBouncer. A worker renews its leases every 20 s while it lives, and gives them back when
it stops.

The loop of one (consumer, lane):

1. Read `SELECT pos, id, type, payload, created_at, trace FROM event_log WHERE lane = $1 AND pos > $cursor
   ORDER BY pos LIMIT 100`.
2. Run the handler on each event, with a timeout of 1 minute (today's `consumeWait`).
3. In one transaction: for each event that failed, insert an `event_retries` row (tries 1, next
   try after 10 s); then `UPDATE event_cursors SET pos = <last> WHERE consumer, lane AND lease_owner
   = $node`. If the lease was lost meanwhile, the update touches nothing and the transaction rolls
   back, so the new holder runs the batch again: a duplicate, never a loss.
4. This commit may use `SET LOCAL synchronous_commit = off` (**D4**). A lost cursor commit after a
   crash moves the cursor back and repeats events; it can never move it forward, because it only
   names positions the sequencer committed durably before they were read.

A retry loop per consumer takes due rows with `FOR UPDATE SKIP LOCKED` (as the mail queue does),
runs the handler, and deletes the row on success, or sets the next wait (1 min, 5 min, 15 min,
1 h). After the last wait it calls `failures.Record` and deletes the row in one transaction. So a
failing event never holds its lane, which is what `NakWithDelay` gave, and the five waits and the
failure table stay as they are (ARC-050). Replay from the console stays as it is.

### 2.4 Lanes and order

**D5. Four lanes per consumer, by object.** The sequencer puts an event in lane
`abs(hashtext(key)) % 4`, where the key is the first of `page_id`, `space_id`, `user_id`,
`group_id` in the payload, else the event id. Events about one page therefore reach a consumer in
the order they became visible, while four lanes let up to four workers share one consumer. The
number is fixed by the migration; changing it later needs every cursor at the head first, which a
release can do while the sequencer is held.

Order holds per object and per consumer, except for an event that failed: its retry comes after
later events of the same object, as today. Handlers read current state (1.3), so that is safe;
any handler that ever needs strict order must say so and check the position of what it last
applied.

### 2.5 Wake-ups: LISTEN/NOTIFY, and polling as the floor

**D6. NOTIFY is a hint, polling is the guarantee.**

- Each worker holds one dedicated connection that runs `LISTEN diffluence_events`. A notification
  wakes all its consumer loops at once.
- Every loop also polls every second without a wake-up, so a lost notification, a dropped
  connection or a database failover costs at most one second of latency, never an event.
- The payload is the head position as text, under 20 bytes, far under PostgreSQL's 8,000-byte
  limit. Events are never sent in a notification.
- Notifications reach only sessions connected at commit; a worker that reconnects polls at once.
- `LISTEN` needs a session. Behind PgBouncer in transaction mode it does not work, so the
  listening connection uses `DIFFLUENCE_LISTEN_DATABASE_URL` (**D8**) when set, pointing at the
  database directly or at HAProxy (as `DIFFLUENCE_MIGRATE_DATABASE_URL` does for migrations, but
  with the application role, not the owner). Unset, it uses `DIFFLUENCE_DATABASE_URL`. If LISTEN
  does not work there, the worker logs it once and runs on polling.
- PostgreSQL's notification queue is 8 GB by default and is drained by every listener; one
  listening connection per worker, which only reads, keeps it empty.

**D7. Only the sequencer sends NOTIFY, never a save.** A transaction that ran NOTIFY takes a
lock shared by the whole database while it commits, so commits that notify are serialised. If every
save notified, saves would queue on that lock, as they did on the audit lock before ADR-041. The
sequencer commits a few times a second, so its notifications cost nothing that matters. The price
is up to 250 ms before the sequencer sees an event.

### 2.6 Several nodes, collab and caches

- **Any number of workers.** The sequencer runs on whichever worker takes the advisory lock;
  cursors spread by lease; retries by `SKIP LOCKED`. A worker that dies loses its leases after a
  minute, and another takes them over.
- **API nodes** need only PostgreSQL. They no longer connect to a queue, so readiness, the
  upgrade check and the gauge drop that part.
- **Collab** stays as it is: the `collab` consumer posts `/recheck` from whichever worker holds
  the lane. The editing service needs no database connection for this.
- **Per-node caches** keep using the permission generation. Nothing needs a broadcast now. If a
  later need arises, every node can LISTEN on the same channel: that is a reason to keep the
  wake-up channel general, not code to write now.

### 2.7 Retention and cleanup

**D10. `event_log` keeps 7 days, as the stream and the published outbox rows do today.** A
periodic task deletes rows older than 7 days whose position is below every cursor and that no
retry row names, in batches of 10,000. At the model that is about one million rows and 0.6 GB
(section 1.4), roughly what the published outbox rows already take today; the NATS store, which
held a second copy, goes away.

- `event_log` is append-only and never updated, so it creates dead rows only through the daily
  delete.
- `outbox_events` becomes a pure waiting room: every row is inserted and deleted within about a
  second. It is small, but each row leaves a dead tuple. Give it its own autovacuum setting
  (`autovacuum_vacuum_scale_factor = 0`, `autovacuum_vacuum_threshold = 5000`) and keep the
  partial index on waiting rows.
- `event_cursors` and `event_head` change on every batch. No indexed column changes, and the
  fill factor of 50 leaves room on the page, so the updates are HOT and need no index work.
- **Time partitioning is not built first.** At this volume a batched delete and autovacuum keep
  up. If the benchmark shows the daily delete or vacuum of `event_log` costing more than 1% of
  database CPU at four times the model, switch to daily partitions dropped whole. The primary key
  then becomes `(sequenced_at, pos)`, which consumers do not notice.

### 2.8 Crash and hard reset

The core property: **an event and the change it describes share one fate.** The change and its
outbox row commit together; the sequencer moves a row and records its position together; a
cursor moves only past events it handled or put in `event_retries`.

| What happens | Effect |
|---|---|
| A worker dies during a handler | Its lease ends within a minute; another worker runs the batch again. Duplicates only |
| A worker dies in the sequencer | The transaction rolls back; the rows wait in `outbox_events` |
| A sudden reset of the database machine | Every commit that was acknowledged is on disk (normal crash safety). Cursor commits of up to about 600 ms (three times `wal_writer_delay`) may be lost (D4): those events run again |
| A failover to a synchronous standby (Patroni) | As a reset |
| A failover to a standby that was behind | Changes and their events are lost together; no event outlives its change, and none is lost while its change stays |
| The listening connection drops | Polling every second, and an immediate poll on reconnect |
| A handler fails for ever | Five waits, then `event_failures`, as today |
| A worker is down for days | Events wait in `event_log` and the backlog shows as lag (2.9). After 7 days the cleanup would remove events a cursor has not passed; it must not: the cleanup only deletes below every cursor, so an idle consumer keeps rows longer, and System health says so |

The queue's crash behaviour is then the database's crash behaviour, which the install guide,
Patroni's synchronous replication (spec 062, T3) and the weekly restore test already cover. There
is no second store with its own sync policy to decide.

### 2.9 Seeing it run

- Gauges: `diffluence_events_waiting` (rows in `outbox_events`), `diffluence_event_lag_seconds`
  per consumer (age of the oldest event past its cursors), `diffluence_event_retries` per
  consumer. They replace `diffluence_queue_healthy`.
- System health warns when an event waits more than a minute for the sequencer, or a consumer
  lags more than 5 minutes, as it does for the audit sealer today.

### 2.10 How "nothing is lost" becomes provable

The state is all in one database, so loss is a query, not an estimate:

- **L1.** Positions in `event_log` have no gaps between the oldest kept row and `event_head`.
- **L2.** For each consumer and lane, every event at or below the cursor is either done or has a
  row in `event_retries` or `event_failures` for that consumer.
- **L3.** No row in `outbox_events` is older than one minute while a worker runs.
- **L4** (witness). In tests and on the server, a webhook endpoint subscribed to every type is a
  ledger: `webhook_deliveries` must hold one row per event in `event_log`, by event id.

`diffluence events check` runs L1 to L3 and prints the first breach. The hard-reset test of
section 5 runs it after recovery, together with L4 against the load tool's own list of changes it
saw acknowledged.

### 2.11 From NATS to PostgreSQL with no lost event

Two releases. During a rolling upgrade, release N (NATS only) and N+1 (both) run side by side.

**Release N+1 (A)** adds the tables and a row `event_transport` with the value `nats`. Its relay
and the sequencer are one loop under the events lock:

- While the value is `nats`, it does what the relay does today: publish to NATS and set
  `published_at`. Release N relays do the same and claim with `SKIP LOCKED`, so the two coexist.
- **The switch.** A worker of release A sets the value to `postgres`, under the events lock, once
  every node in `nodes` seen in the last 5 minutes runs release A or later. From that transaction
  on, the loop moves rows into `event_log` instead. Each event goes one way: whatever was claimed
  before the switch went to NATS, everything after goes to `event_log`, and the lock makes the
  boundary exact. Cursors start at the head at the switch.
- Release A keeps its NATS consumers running after the switch, so everything already in the
  stream, and anything an old node published late (a release N node that comes back), is still
  handled. When all four durables have nothing pending and nothing waiting for an ack, the worker
  writes `nats_drained_at`. A retry waiting in NATS can make this take up to about 1.5 hours.

**Release N+2 (B)** removes NATS. Its upgrade check refuses, as it does for a missing backup,
unless `event_transport = 'postgres'` and `nats_drained_at` is set, and names what to do.
`DIFFLUENCE_NATS_URL` and `DIFFLUENCE_NATS_REPLICAS` are accepted and ignored with a warning in B,
and refused in the release after it.

Published rows with `published_at` set stay in `outbox_events` until the existing 7-day purge,
which B keeps for one release. The `outbox resend` tool of spec 062 F3 is not needed.

## 3. The load budget

### 3.1 Work per event

| Step | Statements | Rows written |
|---|---|---|
| The save (unchanged) | 1 insert | 1 row, 3 index entries |
| Sequencer, per batch | lock, select, head update, insert, delete, notify, commit | per event: 1 insert in `event_log` (3 index entries), 1 delete in `outbox_events`; per batch: 1 HOT update |
| Each consumer, per batch per lane | 1 select, the handler's own work, 1 cursor update, commit | 1 HOT update per batch |
| Cleanup | 1 delete per 10,000 rows, daily | 1 dead row per event |

Against today: the relay does a select and an update of `published_at` per event (a non-HOT
update, since it changes the partial index), plus the publish over the network. The new path
writes about one more row per event and one HOT update per batch per lane.

### 3.2 At the model and at four times

Estimates, to be measured (3.4). Batches assume the sequencer runs four times a second when busy.

| | The model (5 events/s) | Four times (20/s) | Peak (85/s, one minute) |
|---|---|---|---|
| Events written | 5 rows/s | 20 rows/s | 85 rows/s |
| Sequencer | 4 batches/s, about 25 statements/s | the same, larger batches | the same, batches of about 20 |
| Consumer batches | up to 16 lanes × 4 = 64/s, about 130 statements/s | the same | the same |
| Idle polling | 3 sequencer checks a second per worker, 16 lane polls a second | | |
| Rows written for the queue | about 20/s (incl. HOT updates) | about 60/s | about 250/s |
| Commits | about 70/s, of which 64 asynchronous | the same | the same |
| Statements in all | about 200/s, each an index lookup under 1 ms | about 220/s | about 300/s |

For scale: the model's 130 reads a second are each about ten queries, so above 1,300 statements a
second without search. The queue adds about 10% of the statements and, at well under a
millisecond each, an estimated 1 to 2% of the database CPU of one core. The handlers' own work
(writing notifications, the rule queries) is the same in both designs.

The synchronous commits matter most on slow disks. The stress server's disk syncs about 160
times a second (`2026-11-10-stress-test.md`). The sequencer adds at most 4 synchronous commits a
second; the cursor commits are asynchronous (D4) and do not wait for the disk.

### 3.3 Index, vacuum and wake-ups

- `event_log`: two B-tree indexes and the unique id; append-only, so its indexes grow at the
  right edge and need no vacuum except after the daily delete.
- `outbox_events`: about 5 to 20 dead tuples a second; with D10's setting autovacuum visits it
  every 5,000. A transaction open for long on the server keeps those dead tuples, and the
  sequencer's select then reads past them. Measure with a transaction held open for 15 minutes
  (3.4, B5).
- `event_cursors`, `event_head`: HOT updates on 17 rows.
- Wake-ups: one notification per sequencer batch, at most 4 a second, to one connection per
  worker; idle cost about 30 single-row queries a second across the installation.

### 3.4 Benchmark plan

The numbers above are derived, not measured. Before NATS is removed:

- **B1, in the development container, on the real-data dump** (`scripts/stress/real-data.sh`,
  5,001 people, 60,385 pages): a measuring test in the style of
  `TestMeasureSavesUnderTheChainLock`. 32 writers save pages and comments, each with its event,
  at 5, 20, 85 and then rising until lag grows: sequencer and the four real consumers running.
  Record events a second sustained, lag p50 and p95 per consumer, statements and rows a second
  from `pg_stat_statements` and `pg_stat_user_tables`, WAL bytes a second, and dead tuples.
- **B2, the notifications consumer alone on the dump**: time per `page.published` in a space
  with many watchers, to know whether four lanes are enough at four times the model.
- **B3, on the stress server** (spinning disks, the second stress-test layout): B1 at 5 and 20
  events a second during the normal stress series, so events compete with reads for a database
  that is already full; record lag and the reads' p95 with and without events.
- **B4, wake-ups**: lag with LISTEN, and with LISTEN off (polling only).
- **B5, the bad cases**: a transaction held open for 15 minutes during B1; a worker stopped for
  an hour and started again (the backlog of `limits.md` "Not run"); a PostgreSQL restart.

Pass marks: at four times the model, consumer lag p95 under 1 s; sustained capacity at least ten
times the model (50 events a second) on the stress server; the queue's statements under 2% of
database CPU at the model. `limits.md` gets the results.

## 4. What is lost and what is gained

### 4.1 NATS features in use, and what replaces them

| Feature | Replaced by |
|---|---|
| Durable consumers that keep their place | `event_cursors` |
| Ack, redelivery after `AckWait` | Leases of 1 minute and the batch run again |
| `NakWithDelay` with five waits, `MaxDeliver`, give up | `event_retries`, then `event_failures`, unchanged waits |
| De-duplication by message id within 2 minutes | Not needed: the sequencer moves each row once, in one transaction |
| Stream retention of 7 days | `event_log` cleanup after 7 days |
| Replicas on a cluster (spec 063) | PostgreSQL's replication (Patroni, synchronous standby) |
| Trace headers on the message (spec 060 D8) | A `trace` column filled at `Enqueue` from the request's context, so the listener's work shows under the request that caused it, not under the relay |
| Embedded server for one node | Nothing to embed |
| Subjects | Unused: every consumer takes every event and filters by type |

Not in use and given up: very high throughput (millions of messages a second), pub/sub to clients
outside the binary, and NATS as a future bus between installations. None is in the blueprint for
1.0.

### 4.2 Removed

- Modules: `github.com/nats-io/nats-server/v2`, `github.com/nats-io/nats.go`, and the indirect
  modules only they bring, likely `nats-io/jwt/v2`, `nats-io/nkeys`, `nats-io/nuid`,
  `minio/highwayhash`, `google/go-tpm`, and possibly `klauspost/compress`, `golang-jwt/jwt/v5`
  and `golang.org/x/time` (to confirm with `go mod tidy` and `go mod why`). Fewer advisories to
  follow (ADR-016).
- Code: `internal/queue`, the relay's publish path, the 900 KiB limit's reason, the NATS store
  under the data folder.
- Configuration: `DIFFLUENCE_NATS_URL`, `DIFFLUENCE_NATS_REPLICAS`, the rule that only role `all`
  may embed, and their Helm, Compose and HA script lines.
- Operations: three NATS machines in the HA layout of spec 062, their install and upgrade, test
  T6, and "A NATS cluster" in "Not run".

### 4.3 Blueprint lines this closes or changes

- **OPS-012** (a NATS cluster of three nodes): withdrawn.
- **OPS-083** (the queue's storage settings chosen on purpose, events proven after a hard reset):
  closed by design, since events are PostgreSQL rows; the proof moves into the hard-reset test of
  the database leader (spec 062 T3), with the ledger check of 2.10.
- **Spec 062**: F1 (replicas) and F3 (resend) no longer apply; T6 is removed; D7 (sync policy)
  is moot.
- **ADR-007** is superseded by a new ADR; ADR-018's last sentence ("Events still go through the
  outbox and NATS") changes.

### 4.4 New risks

| Risk | Mitigation |
|---|---|
| PostgreSQL carries one more load, and the stress tests saw it run out of CPU from about 50 active people | The queue's share is small (3.2); B3 measures it on a full database. The worker's queries are not behind the admission order of spec 031, so lag under saturation must be watched (2.9) |
| Queue code written here, not taken from a widely used module (ADR-016) | The pattern is already in the repository three times (jobs, the mail queue, the audit sealer) and is a few hundred lines on pgx. A job library such as River would add a second job system beside `jobs` and its own migrations. **D11. Own code**, with mutation tests on every condition; the product owner may prefer a library |
| Queue-table bloat when a long transaction holds back vacuum | D10's autovacuum setting; B5 measures it; the waiting room is emptied within a second, so it stays small |
| LISTEN behind PgBouncer | D8; polling keeps correctness without it |
| A bug in the cursors loses events silently | The ledger checks L1 to L4, run in CI and after the hard-reset test |
| Events that were in NATS at the switch | The drain of 2.11; release B refuses to start until it is done |

## 5. Recommendation and waves

### 5.1 Recommendation

**Go**, on the product owner's condition, with evidence first.

- **Events and routing in PostgreSQL alone:** yes. All four consumers are single-logical
  consumers in the worker role that read and write PostgreSQL, nothing needs a broadcast, and
  the design keeps at-least-once, the retry waits and the failure table as they are.
- **Scaling up:** by estimate, the queue needs about 200 statements a second and about 2% of a
  core at four times the model, against a capacity that spec 052 suggests is in the thousands of
  single-row writes a second. Not yet measured.
- **Scaling out:** any number of workers through leases and the advisory lock; API nodes need
  only PostgreSQL. The HA layout loses three machines.

NATS stays until the gates below pass. If B1 or B3 fails its pass marks, or the hard-reset test
finds a loss, release A stays on `nats` and this spec is revisited; nothing is lost by building
release A first.

**D12. The order is: build both paths, measure, switch, and only then remove.**

### 5.2 What must be shown before removal

- **G1.** B1 and B2 pass on the real-data dump.
- **G2.** B3 and B5 pass on the stress server.
- **G3.** The hard-reset test: under load, reset the database VM; after recovery, L1 to L4 hold and
  every change the load tool saw acknowledged has its event handled by every consumer.
- **G4.** The rolling upgrade test (CI `upgrade` job and spec 062 T7) with release N and A side by
  side across the switch: L4 holds over both transports.
- **G5.** The mutation check covers the new package with no surviving mutant.

### 5.3 Waves

| Wave | What | Proof |
|---|---|---|
| **EV1** | Release A, part 1: migration (tables, `event_head`, `event_transport = 'nats'`); the sequencer in the relay loop behind the transport value; consumer loops with lanes, leases, retries, the LISTEN connection and polling; `diffluence events check`; gauges and System health. Default stays `nats` | Tests written first: `TestALateCommitIsNotSkipped` (a transaction that started first and committed last gets the later position), `TestPositionsHaveNoGaps`, `TestEveryEventReachesEveryConsumer`, `TestAFailedEventDoesNotHoldItsLane`, `TestARetryEndsInFailures`, `TestAnEndedLeaseIsTakenOver`, `TestALostCursorCommitRepeatsAndNeverSkips`, `TestPollingDeliversWithoutNotify`, `TestASaveDoesNotNotify`, `TestEventsOfOnePageKeepTheirOrder`. Mutations on every SQL condition (`pos >`, lane filter, lease check, retry insert, delete in the sequencer, the cleanup bound) |
| **EV2** | Release A, part 2: the switch and the drain; the trace column; the upgrade check of release B written and tested against fixtures | `TestEachEventGoesOneWayAcrossTheSwitch`, `TestAnOldRelayAfterTheSwitchLosesNothing` (an N relay still running), `TestTheSwitchWaitsForEveryNode`, `TestDrainedIsSetOnlyWhenNothingWaits`; the CI upgrade job across the switch (G4) |
| **Measure** | B1, B2 in the development container; B3, B5 and G3 in one session on the stress server | Results in `limits.md` and a report in `docs/reports`. Gate for EV3 |
| **EV3** | Release B: remove `internal/queue`, the NATS modules, the settings (warned, then refused), the readiness and upgrade parts, Helm, Compose, HA scripts; a new ADR superseding ADR-007; update ADR-018, OPS-012, OPS-083, spec 062, `limits.md`, `dependencies.md`, the install guide | `go mod tidy` diff; the upgrade check refuses without the drain (test); the whole suite and the mutation check; the rolling upgrade A to B |

### 5.4 Still to measure

- The real rate of events: count `outbox_events` inserts a second during the next stress series,
  by type, to replace the assumptions of 1.4.
- The cost of each consumer per event on the real data (B2), which decides whether four lanes
  are enough.
- The queue's share of a database that is already at full CPU (B3).
- Synchronous commits on the server's disks with the sequencer running (B3).
- LISTEN across a Patroni failover, and through HAProxy (B4, in wave 8).
- Autovacuum of `outbox_events` with a long transaction open (B5).

## Decisions

Each decided on the product owner's behalf on 2026-11-13; each may be overruled.

| | Decision |
|---|---|
| D1 | A single sequencer under an advisory transaction lock moves events into `event_log` with gapless positions in visibility order (as ADR-041 does for the audit chain) |
| D2 | No ordering by transaction id below the snapshot's xmin: it makes events wait for the longest open transaction |
| D3 | A cursor per consumer and lane, held by a lease of 1 minute, not by an open transaction |
| D4 | Cursor commits are asynchronous; the sequencer's are synchronous |
| D5 | Four lanes per consumer, by page, space, user or group, else event id |
| D6 | NOTIFY only wakes; every loop polls every second, so a lost notification costs latency, never an event |
| D7 | Only the sequencer sends NOTIFY; saves never do |
| D8 | `DIFFLUENCE_LISTEN_DATABASE_URL` for the listening connection behind PgBouncer, defaulting to the database URL, with the application role |
| D9 | Target: an event handled by every consumer within 1 s (p95) at four times the model |
| D10 | `event_log` kept 7 days and never below a cursor; batched delete, no partitioning unless B1 or B3 asks for it; own autovacuum settings for `outbox_events` |
| D11 | The queue is code of our own on pgx, following the jobs, mail queue and audit sealer patterns, rather than a job library |
| D12 | Build both paths, measure, switch, then remove NATS in the release after, gated on G1 to G5 |
