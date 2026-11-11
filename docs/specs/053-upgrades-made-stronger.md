# Spec 053: upgrades made stronger

Wave O2, the second wave of spec 048. It builds the upgrade lines of the study of blueprint 07 (`docs/blueprint/07-operations/01-upgrades-and-schema-change.md`, OPS-070 to OPS-075) and the corrections the study made to wave O7 (`09-review-of-wave-o7.md`). The backup check that refuses by default was built in wave FU (ADR-040) and is not repeated here.

## Study

The study read the research on upgrade failures (Zhang et al., SOSP 2021: 60% come through stored data, 40% through messages) and the practice of GitLab, Stripe, Kubernetes and PostgreSQL. This wave checked the code against each point:

| Point | Found in the code | Consequence |
|---|---|---|
| Contract migrations wait | `contractGrace = 10 * time.Minute` in `internal/app/ops.go` | Too short for a problem found the next day (D1) |
| Long statements | Each migration has `lock_timeout = 5s`, no `statement_timeout` | A migration that gets its lock can hold it for as long as it runs |
| `CREATE INDEX CONCURRENTLY` | Every migration runs in a transaction, where `CONCURRENTLY` is refused | A large index can only be built while writes wait (OPS-043) |
| Rewrites | Nothing refuses `ALTER COLUMN ... TYPE` and the like | One careless line can lock a busy table for minutes |
| Upgrade test | Reads, logins and the audit chain; no pages, comments or files | The 40% through messages and most of the 60% through data are not exercised |
| Jobs across versions | A worker claims any queued job and fails one whose kind it does not know | During an upgrade an old worker can take and fail a job a new node queued |
| Events across versions | Every listener receives every event of the stream | Each must ignore a type or a field it does not know |
| Settings across versions | Stored sections are read leniently, but written whole | A node that does not know a field drops it when it saves the section |
| Formats with a version | Documents (`SchemaVersion`, upgrade steps on read), bundles (format numbers) | Already built; this wave adds the rule that keeps them |

## Decisions

1. **Contract grace.** `DIFFLUENCE_CONTRACT_GRACE`, 24 hours by default (D1). Zero is allowed for a test installation; a negative value is refused at start. The health check warns when a contract migration has waited a day longer than the grace, not a day in all.
2. **Statement timeout.** Every migration runs with `statement_timeout = 1min` unless its header says `-- statement_timeout: <duration>`, with the reason on the next line. A migration that needs longer belongs in a background job (OPS-035). One minute leaves every migration of today with a wide margin on an installation of the size of the stress test.
3. **Migrations outside a transaction.** A header `-- transaction: none` runs the file as one statement on its own, with the same lock, `lock_timeout`, `statement_timeout` and retries. The file must hold exactly one statement. Afterwards the run checks that no index is left invalid, because a failed concurrent build leaves one behind, and names it.
4. **The rewrite check.** A Go test reads every migration from 0048 on and refuses: `ALTER COLUMN ... TYPE`, `SET NOT NULL`, `ADD COLUMN` with a volatile default, `CREATE INDEX` without `CONCURRENTLY` on a table the file did not create, a `CHECK` or `FOREIGN KEY` without `NOT VALID` on such a table, `VACUUM FULL` and `CLUSTER`. An exception is a comment `-- rewrite-ok: <reason>` in or right above the statement. Earlier migrations ran already, or run on empty tables of a new installation.
5. **Jobs.** A worker claims only kinds it has a handler for. A job of a newer release waits for a node that knows it.
6. **Events.** Every listener must ignore an event type and a field it does not know. A test sends each listener an event of an unknown type.
7. **Settings.** Saving a section keeps the fields this release does not know, so an older node does not erase what a newer one stored.
8. **The release rule (D2).** A contract migration may only remove what the previous release already stopped reading and writing. It goes in the release checklist and the migration rules at the top of `migrate.go`.
9. **The version-skew policy (OPS-075)** is written in `docs/operations/version-skew.md`: which parts may differ by how many releases, and for how long.
10. **Six months (D3, OPS-076)** is written in the policy; its test in CI waits for 1.0, as the line says.

## Plan

Each step: failing test first, then the change, then a mutation.

1. Configuration and contract grace; health threshold relative to the grace.
2. `statement_timeout` and its header.
3. `-- transaction: none` with the validity check.
4. The rewrite check as a test over the embedded migrations, with table-driven cases.
5. Jobs claim only known kinds.
6. Events: unknown types are ignored by every listener.
7. Settings keep unknown fields on save.
8. The upgrade test writes: a space, a page, a draft, a comment and a file on the old node, read on the new one, and the reverse while both serve; after the rollback the old node reads both. At the end no job failed, no event failed and the outbox is empty.
9. Documents: the policy, `upgrade.md`, `release.md`, the OPS lines, the changelog.

## Load budget (ARC-238)

None at runtime. Claiming a job adds one array comparison to a query that already filters on kind. Saving a settings section reads its row once more inside the same transaction. CI time grows by about a minute for the writes in the upgrade test.

## Not proven on one machine

A migration that really runs for minutes on a large table, and `CREATE INDEX CONCURRENTLY` under load: both need the data of wave 7.
