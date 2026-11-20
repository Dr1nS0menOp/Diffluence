# 077: Witness reports are recorded by the owner of the tables

Status: built
Phase: hardening before 1.0 (wave RV7)

## Problem

Spec 075 (CSL-532) lets an operator record the result of `kenwerk audit verify --witness FILE --report`, so Administration, Audit and System health can show when the audit chain was last compared with its witness in the log store, and what was found. The report went in with the application role, the role the server uses. Since spec 070 the backup service in Compose holds a role and a key the server lacks: the owner of the tables, and `backup.key`. From this spec a witness report is recorded with that owner role, so what the console shows comes from where the comparison ran, not from the server.

## Options

| Option | For | Against |
|---|---|---|
| **A. Only the owner of the tables records a report** | The grant already separates the two roles, as for the signing keys (spec 070); the command needs only the address it has in the backup service; nothing new to keep or derive | Where the server holds the owner's address too (an install without Compose's split), the console shows what that role recorded, as for every table the owner writes |
| B. Each report signed with a key derived from `backup.key`, unsigned rows shown as not verified | Holds wherever `backup.key` is kept away from the server | The server must learn the public key and which key may sign; the command needs `backup.key` where it runs; installs without a backup service show every report as not verified; more code and one more key derivation |

**Choice: A.** It is the simplest form, and it is as strong as the split of roles that already protects the audit tables and the signing keys.

## Behaviour

1. **The grant.** The application role may read `audit_witness_checks` and may no longer add to it (`db.GrantApp`, which runs right after the migrations of this release). Changing or removing a report stays refused for it, as before. The stamp below makes the grant and the migration independent of each other's timing: a report the application role adds before the grant is in place is stamped with that role and not shown.
2. **The stamp.** Migration 0067 adds `recorded_by` and a trigger before each insert that sets it to the role that inserts, and `checked_at` to the database's own time, whatever the insert names.
3. **What is shown.** `audit.LatestWitnessCheck`, which the audit area and System health read, takes the newest report whose `recorded_by` is the owner of the table. A report recorded otherwise, including every report from before migration 0067 (its `recorded_by` is empty), stays in the table and is not shown. So after the upgrade the audit area says no comparison was reported, and System health does not warn about the witness, until the first report of the owner.
4. **The command.** `kenwerk audit verify --witness FILE --report` compares as before with the application role, and records the report with the owner's address: `KENWERK_MIGRATE_DATABASE_URL`, or the one address of an install that has only one. A process that has neither, such as the server of `compose.yaml`, refuses `--report` before it compares, and names the backup service.
5. **Compose.** The backup service holds the owner's address, the data folder with `master.key`, and the `kenwerk` command, so the timer runs the command there:
   `docker compose logs --no-log-prefix backup | docker compose run --rm -T --entrypoint kenwerk backup audit verify --witness - --report`.
   A log server that keeps the log elsewhere pipes its copy into the same command.

## Permissions and audit

- Recording a report needs the owner of the tables. Reading it stays as in spec 075: the audit area and System health, for administrators.
- `audit.verified` is recorded as before, by the role that runs the comparison.

## Data

- `0066_reserved.sql`: holds the number for wave GT2.
- `0067_witness_reports_by_owner.sql` (expand): `audit_witness_checks.recorded_by text NOT NULL DEFAULT ''`, added only when missing so a second run keeps the reports already there empty; the function `audit_witness_check_stamp()` and the trigger `audit_witness_checks_stamp`. It may run twice. The previous release reads every report and ignores the column; its `--report` with the application role is refused once the grants of this release are in place.

## Load budget

The read of the audit area and System health stays one indexed query on `audit_witness_checks` by `checked_at`, now with a filter on `recorded_by` and one row of `pg_class`; the rows it skips are the few from before migration 0067. A report is one insert, at most a few a day. No change to the budgets of CSL-703.

## Acceptance criteria

- [x] The application role cannot insert a witness report; the owner can, and the application role reads it (`internal/audit`, `TestOnlyTheOwnerOfTheTablesRecordsAWitnessReport`).
- [x] A report another role recorded, as before 0067, is kept and not shown, and does not count as reported; a newer one of another role does not replace the owner's (`TestAWitnessReportOfAnotherRoleIsNotShown`).
- [x] The database sets the role and the time of a report whatever the insert names (`TestTheDatabaseStampsTheRoleAndTimeOfAWitnessReport`).
- [x] With only the application role, `--report` is refused, names the backup service and records nothing; with the owner's address it records the report, which is shown (`cmd/kenwerk`, `TestAuditVerifyReportsAsTheOwnerOfTheTables`).
- [x] The comparison and its report work as in spec 075 on a single-role install (`TestAuditVerifyReportsTheWitnessComparison`).

## How the product owner tests it

1. Upgrade a Compose install. Administration, Audit, Signatures says no comparison was reported.
2. Run `docker compose logs --no-log-prefix backup | docker compose run --rm -T --entrypoint kenwerk backup audit verify --witness - --report`. It prints `witness result reported to the console`; the page shows the comparison.
3. Run `docker compose exec kenwerk kenwerk audit verify --witness - --report` with the same log. It refuses and names the backup service; the page is unchanged.

## Provenance

Own design. PostgreSQL documentation on `GRANT`, triggers and `current_user` (postgresql.org/docs/16, consulted 2026-11-14).
