# Spec 063: ready for a NATS cluster and for PgBouncer

Status: built 2026-11-13. Covers F1 and F2 of spec 062, section 4, which must be in place
before session S2 of wave 8. Blueprint line: OPS-012.

## Why

Spec 062 found two things the app needs before it can run on several nodes:

- **F1.** `queue.Open` created the event stream without a replica count, so in a NATS
  cluster the stream lives on one node, and losing that node loses the stream.
- **F2.** Wave PF (spec 061) asks every session for `plan_cache_mode = force_custom_plan` and
  `jit = off` as startup parameters. PgBouncer in transaction mode refuses unknown startup
  parameters, or drops them when told to ignore them, so behind PgBouncer the settings that
  made the feed fast would be gone.

## F1: copies of the event stream

- `DIFFLUENCE_NATS_REPLICAS` (and `nats_replicas` in the configuration file) is read through
  the one validator of spec 056. Default 1.
- Allowed: 1, 3 or 5. JetStream keeps an odd number so a majority can agree; more than five
  gains nothing for one stream. Anything else is refused at start, naming the setting.
- Above 1 needs `DIFFLUENCE_NATS_URL`: the embedded server is one node. Refused at start.
- `queue.Open` creates or updates the stream with the copies it gets from
  `streamReplicas(existing, configured)`, which is the larger of the two.
- A start that asks for more copies than the server can hold fails with the count in the
  message, instead of running with fewer copies than it was told to keep.

## F2: the settings in the database

- Migration 0050 sets both as defaults of the current database, with
  `ALTER DATABASE %I SET …` built by `format` from `current_database()`: a name cannot be a
  parameter. A new session then has them however it arrives.
- Only the owner of a database may set its defaults. The migration runs as the owner role of
  the tables, which owns the database in every install this project describes. Where it
  does not, as on some managed databases, the migration notes it and goes on.
- `db.Open` still sends both as startup parameters, so a direct connection has them even
  where the database could not take them. Behind PgBouncer, `ignore_startup_parameters`
  (spec 062, D6) drops them, and the database's own defaults apply.
- `ALTER DATABASE` rewrites no table, so the rewrite check of spec 053 has nothing to refuse.
- A backup by `pg_dump` does not carry database defaults. A restore into the same database
  keeps them; a restore into a new database gets them from the next migration run only if
  0050 runs again, which it does not. That is noted under "not proven".

## Decided on the product owner's behalf on 2026-11-13, may be overruled

| # | Decision | Why |
|---|---|---|
| D1 | An existing stream with fewer copies is raised; one with more is left as it is, with a warning | Raising adds safety and JetStream does it online. Lowering by itself would let one node started with the setting left out take copies from the whole cluster. Lowering stays an operator's step. |
| D2 | 1, 3 or 5 only | An even count cannot break a tie, and above five costs writes without gain. |
| D3 | The pool keeps sending both startup parameters | A direct connection keeps them even on a database the migration could not change. Through PgBouncer they are ignored there and the database defaults apply. |
| D4 | A migration role that does not own the database is a notice, not a failure | An install on a managed database must not be stopped by a speed setting; it keeps working, only slower behind PgBouncer. |

## Tests

- Config: the default is 1; 3 and 5 are taken with a cluster; 0, 2, 4, 7 and -1 are refused
  naming "1, 3 or 5"; 3 without `NATS_URL` is refused naming `NATS_URL`.
- Queue: `streamReplicas` raises and never lowers; the embedded stream has 1 copy; one server
  asked for 3 copies refuses at start, naming the count.
- Database: a plain connection without startup parameters has `force_custom_plan` and
  `jit = off`; the migration runs without error for a role that does not own the database;
  the pool still sends both.
- Seven mutations, one per check.

## Not proven

- A real 3-node NATS cluster taking 3 copies, and raising an existing stream from 1 to 3
  online: wave 8, session S2.
- PgBouncer in front: wave 8, session S2.
- A restore into a fresh database keeps the speed settings only if the operator runs 0050's
  statements again; the restore guide does not say so yet.

## Load budget (ARC-238)

One extra `Stream` info call at start. No cost per request.
