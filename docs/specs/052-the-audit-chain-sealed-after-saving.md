# Spec 052: the audit chain sealed a moment after saving

Status: built in wave AC (2026-11-11). Decision: ADR-041, by the product owner on 2026-11-11. Blueprint lines: ARC-203.

## The study

Read for this wave:
- **Decisions:** ADR-013 (the hash chain), ADR-039 (checkpoints every five minutes and the witness in the application log), ADR-041.
- **Code:** `internal/audit` (record, verify, checkpoints, witness, prune), the jobs and worker loops of `internal/app`, the health checks of spec 051 (`internal/health`), and the pool rules of spec 050.
- **Measurements:** the stress test report (`docs/reports/2026-11-10-stress-test.md`) and `docs/operations/limits.md`.
- **Blueprint and study:** blueprint 04 on the audit (ARC-201, ARC-203), blueprint 06 on the audit log (SEC-811 to SEC-831), and the study of 07 on high availability (`docs/blueprint/07-operations/03-high-availability.md`).

### What the lock cost

Until now `audit.Record` took one advisory lock, read the newest hash, and inserted the event linked to it, all inside the transaction of the change. The lock is a transaction lock, so it was held until that transaction committed, and a commit waits until the disk has synced. Every audited write in the whole installation therefore went through one door, one commit at a time. With all 5,000 people at once on a disk that syncs about 160 times a second, the stress test measured saves of 3.7 seconds (p50) and 6.9 seconds (p95), most of it waiting at that door (ARC-201). The lock was correct; it was just held across the slowest step there is.

### What has to stay true

1. **No change without its event.** The event is written in the same transaction as the change, so both commit or neither does.
2. **One chain, tamper-evident.** Each event links to the one before by hash; changing, removing or reordering a sealed event breaks it, and `audit verify` names where.
3. **Checkpoints and the witness** of ADR-039 keep working: a signed head every five minutes, also in the application log.
4. **Personal parts** stay apart (SEC-821) and can be erased while the chain holds, also before an event is sealed.
5. **Several nodes:** however many workers run, the chain stays one line and every event goes in exactly once.
6. **An upgrade runs both releases side by side** on one database (blueprint 07): the previous release still appends straight into the chain.

### Ways considered

- **A shorter lock in the same transaction.** Taking the lock last, just before commit, still holds it across the commit's sync. It changes nothing measurable.
- **Hash in the row, filled in later** (`audit_events` with empty hash columns, a worker fills them). Every reader keeps working at once. But the order of `seq` is the order transactions began, not the order they committed: a row with a lower `seq` can commit after higher ones were chained. The chain would need a second position column, every checkpoint and witness line would change meaning, and the previous release, which reads the newest hash, would link to an empty one during an upgrade. Rejected.
- **A table of waiting events and one sealer** (chosen). A save inserts into `audit_pending`, no lock. A worker takes the same lock the previous release takes, reads what is visible in the order it was written, links it, inserts it into `audit_events` as every event was before, and removes it from `audit_pending`, all in one transaction. A row that commits late is simply sealed in a later step: the chain's order is the order in which events became visible, and nothing is skipped or put in between afterwards. Sealed rows look exactly as before, so checkpoints, the witness, pruning and the previous release need no change in meaning.

## What this wave builds

| # | What | Where |
|---|---|---|
| 1 | Migration 0047: `audit_pending`, its trigger and the view `audit_entries` | `internal/db/migrations/0047_audit_pending.sql` |
| 2 | `audit.Record` writes to `audit_pending`, without the lock | `internal/audit/audit.go` |
| 3 | `audit.SealWaiting` and `audit.SealIn`: take the chain lock, read up to 500 waiting events oldest first (locked for this transaction), link and insert them, move their personal parts, remove them from `audit_pending` | `internal/audit/seal.go` |
| 4 | `audit.RecordSealed`: the former path, straight into the chain, for the upgrade marker and the note of a backup, which run before migrations | `internal/audit/audit.go`, `internal/app/upgrade.go` |
| 5 | Every worker seals every second, and once more when it stops; `audit verify`, the console's check and each checkpoint seal first | `internal/app/ops.go`, `internal/app/app.go`, `cmd/diffluence/main.go` |
| 6 | Readers see waiting events at once: lists first without a seq, exports last with an empty seq; `audit_entries` for queries of what happened | `internal/audit/audit.go`, `internal/console/console.go` |
| 7 | Erasing a person also erases the personal part of a waiting event | `internal/audit/audit.go` |
| 8 | Pruning seals inside its own transaction, so the chain goes on from the anchor even when every event was removed | `internal/audit/prune.go`, `seal.go` |
| 9 | The health check "Audit log" and two metrics; an alert after 15 minutes | `internal/health`, `internal/app/health.go`, `gauges.go`, `deploy/monitoring/` |

### The trigger of `audit_pending`

The application role owns no table, so it cannot switch a trigger off. The trigger allows:
- removing a waiting event only once a row with its id is in `audit_events`;
- changing a waiting event only by setting both parts of its personal part to empty, with every other column as it was;
- emptying the table never.

## Decided during this wave

| # | Decision | Why |
|---|---|---|
| D1 | The chain's order is the order events became visible to the sealer, not the order transactions began | It is the only order a sealer can know without waiting for transactions that may never commit, and it never needs to put an event between two sealed ones |
| D2 | One lock, the one the previous release already takes, rather than a lease or leader election | It works through PgBouncer (a transaction lock), across any number of nodes, and keeps the previous release in the same line during an upgrade |
| D3 | Seal every second, in steps of 500 | One cheap read per second when nothing waits; a step short enough that the chain is never held long |
| D4 | Warn after one minute of waiting, call for a person after fifteen | A worker seals every second, so a minute means none seals; fifteen minutes is three checkpoints that do not cover the events |
| D5 | The "Audit log" line reads the same whether nothing or a few events wait | The console would otherwise change every second, and its pictures with it |
| D6 | Seqs stay from the identity column; a seq may skip a number when a sealing transaction rolls back | A seq picked by the sealer would collide with the previous release's identity values during an upgrade. Seqs could skip numbers before too; the chain has no gap |
| D7 | The upgrade marker and the note of a backup stay sealed at once | They are written before migrations, when the table of waiting events may not exist yet, and the check before an upgrade must read the note at once |

## How it is proven

Failure paths first.

| Test | What it proves |
|---|---|
| `TestASaveDoesNotWaitForTheChain` | With the chain lock held elsewhere, a save still commits within 3 seconds |
| `TestAWaitingEventCannotBeRemovedOrChanged` | Removing, emptying or rewriting a waiting event is refused; erasing its personal part is allowed |
| `TestApplicationRoleCannotTamperWithTheAuditLog` (extended) | The application role cannot remove, change, empty, unprotect or drop waiting events, and can seal them |
| `TestEventsAreSealedInTheOrderTheyBecameVisible` | A transaction that began first and committed last is sealed last; the chain verifies with all three |
| `TestTheSealerWaitsWhileTheChainIsHeld` | While someone else holds the chain, as a release from before ADR-041 does while it appends, the sealer seals nothing |
| `TestTwoSealersNeverSealAnEventTwice` | Eight writers and two sealers at once: 200 events, one chain, nothing left |
| `TestABacklogLongerThanOneStepIsSealedWhole` | 503 waiting events are sealed in steps, all of them |
| `TestVerifyCountsWaitingEvents` | A check counts waiting events, which are not part of the chain yet |
| `TestReadersSeeWaitingEventsAtOnce` | Lists show them first, exports last, both with their personal part |
| `TestErasingAPersonReachesAWaitingEvent` | The personal part of a waiting event is erased too |
| `TestTheChainGoesOnFromTheAnchorAfterEverythingWasPruned` | A prune that removes every event leaves a chain that goes on and verifies |
| `TestAnEventSealedDirectlyJoinsTheSameChain` | An event written straight into the chain, as the previous release does, joins the sealed ones |
| `TestTheWorkerSealsWhatWaits`, `TestAStoppingWorkerSealsWhatIsLeft`, `TestAWorkerSealsEverySecond` | The worker's loop |
| `TestEventsLeftUnsealedShowOnTheHealthPage` and three rule tests in `internal/health` | The check, its thresholds and the metrics |

Every new check has a mutation in `scripts/mutation-check.sh`.

### Measured

`TestMeasureSavesUnderTheChainLock`, run only with `DIFFLUENCE_MEASURE=1`: 32 writers save 1,280 times at once, each save one transaction with one event, first holding the lock until commit as before, then without it, with a sealer running. In the development container, three runs: from about 2,000 to about 17,000 saves a second, p50 from 15 ms to 1.4 ms, p95 from 18 ms to 2.8 ms. That database does not sync to disk on commit, which is the very step the lock was held across, so the gain on a server is larger. The numbers are in `docs/operations/limits.md`.

## Load budget (ARC-238)

| Path | Cost |
|---|---|
| A save | One insert into `audit_pending` instead of a lock, a read of the newest hash and an insert: one statement fewer, and no lock |
| The sealer, nothing waiting | One `EXISTS` read a second per worker |
| The sealer, n waiting | One transaction: the lock, one read, n inserts and one delete, sent as one batch; per 500 events |
| Health facts | Two cheap reads of `audit_pending`, inside the existing statement, every 30 seconds |
| Lists and exports | One more indexed read of `audit_pending`, which holds seconds of events |

## What one machine cannot prove

- The save latency at 5,000 people on the server, and how far the sealer lags under that load. This needs the stress test again.
- A worker killed in the middle of sealing on another node while a second takes over. The transaction makes this all or nothing, and the two-sealer test covers the lock, but not a real second machine.

## Open for the product owner

Nothing.
