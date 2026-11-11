# 07.2: Backup, restore and disaster recovery

Blueprint 07 promises to lose at most 4 hours of work (RPO) and to be back within 4 hours (RTO). This chapter checks how the industry reaches such numbers, what goes wrong in practice, and what Diffluence still needs.

## 2.1 The terms

- **RPO**, recovery point objective: how much recent work may be lost. It is set by how often a copy leaves the machine.
- **RTO**, recovery time objective: how long until people work again. It is set by how fast a copy can be found, moved and restored, and by how practised the people are.
- NIST SP 800-34 defines both and ties them to a business impact analysis. The numbers are a promise to the people who use the system, so they must be measured, not assumed.

## 2.2 What goes wrong in practice

**Backups that were never restored.** The best-known case is GitLab's outage of 31 January 2017. A person removed the data directory of the primary database by mistake. Of the backup methods in place, several had silently not worked, and the copy that saved the day was a manual snapshot made six hours earlier. Six hours of changes were lost. GitLab's own post-mortem lists the cause as backups nobody checked by restoring them. The Google SRE book makes the same point in its chapter on data integrity: nobody wants a backup, everybody wants a restore, so the restore is what you test.

**The 3-2-1 rule.** Three copies, on two kinds of media, one away from the site. It comes from practice, not research, and US-CERT published it as advice for everyone in 2012. For a self-hosted wiki it means: the database server, a backup on another disk, and a copy that leaves the building, encrypted.

**Keys kept with the data.** An encrypted backup is only useful with its key, and a key stored next to the backup protects nothing. Diffluence already refuses a backup key inside the backup folder.

## 2.3 How PostgreSQL installations reach minutes of RPO

| Method | RPO | RTO | Notes |
|---|---|---|---|
| `pg_dump` on a schedule | The interval | Restore time grows with size, and indexes are rebuilt | Simple, portable between major versions. What `scripts/backup.sh` does |
| Base backup plus continuous **WAL archiving** | Seconds to minutes | Copy the base backup, replay WAL | Point-in-time recovery to any moment. The PostgreSQL manual's chapter on continuous archiving is the reference |
| **pgBackRest** | As WAL archiving | Fast: parallel, compressed, incremental and differential, checks every file | The common tool for this; encrypts and sends to S3-compatible storage. MIT licence |
| PostgreSQL 17 incremental `pg_basebackup` | As WAL archiving | Incremental chains with `pg_combinebackup` | Newer, in core; Diffluence supports 16 today |
| A standby replica | Near zero for hardware loss | Fail over in seconds | **Not a backup**: a deleted page is deleted on the standby too |

A full disk is a known way for WAL archiving to fail: if `archive_command` keeps failing, WAL piles up on the primary until the disk fills and PostgreSQL stops. Every guide therefore asks for an alert on archive failures and on WAL size.

## 2.4 Where Diffluence stands

| Promise | State | Where |
|---|---|---|
| Encrypted backup of database, files, keys and secrets, with checksums | Built | `scripts/backup.sh`, `docs/operations/backup-restore.md` |
| Backup key refused inside the backup folder | Built | `scripts/backup.sh` |
| Restore on a clean machine, with audit chain and key check | Built | `scripts/restore.sh` |
| Restore drill every night in CI | Built, on a small set | `scripts/restore-drill.sh`, OPS-054 |
| Backup noted in the audit log, read before an upgrade | Built | OPS-041 |
| A schedule that runs backups | Left to the operator (cron) | `backup-restore.md` |
| WAL archiving, pgBackRest | Advised, not set up | OPS-051 |
| RTO for 100 GB measured | Not measured | OPS-055 |
| A warning when the last backup is old | Not built | |

## 2.5 Recommendations

- **OPS-077** · new · **High.** Show the age of the last noted backup: a metric (`diffluence_backup_last_noted_timestamp_seconds`), a line in the System health page, and a warning to administrators when it is older than 26 hours. The note already exists (OPS-041); nobody sees it today unless they upgrade. An unnoticed stopped backup is the GitLab failure.
- **OPS-078** · new · **High.** Measure RTO on real disks: restore 10 GB and 100 GB of generated data on the test machine, with the time of each step (copy, database, indexes, files, checks). Write the numbers in `limits.md`. If 100 GB takes longer than 4 hours with `pg_dump`, the guide must say pgBackRest is required above a stated size. This closes OPS-055.
- **OPS-079** · partly · **Medium.** A tested pgBackRest set-up for the Compose and LXC targets: a documented configuration, WAL archiving to a second disk or S3-compatible storage, encryption on, an alert when archiving fails, and the restore drill run against it on the test machine. pgBackRest runs beside PostgreSQL; Diffluence itself gains no dependency. This closes OPS-051.
- **OPS-080** · built · **Medium.** A scheduled backup without cron for Compose: an optional service in `compose.yaml` that runs `backup.sh` on an interval (default every 4 hours, which matches the RPO). Operators who keep cron lose nothing.
- **OPS-081** · built · **Medium.** The weekly restore of a **real** installation (OPS-054): a script an operator runs from a timer that restores the newest backup into a throw-away Compose project, runs the same checks as the drill, reports the time and the counts, and exits with an error an alert can read. It never touches the live installation.
- **OPS-082** · built · **Low.** Say plainly in `backup-restore.md` that a standby or RAID is not a backup, and what the 3-2-1 rule asks of a small installation.

## Sources

- NIST: [SP 800-34 Rev. 1, Contingency Planning Guide for Federal Information Systems](https://csrc.nist.gov/pubs/sp/800/34/r1/upd1/final).
- GitLab: [Postmortem of database outage of January 31](https://about.gitlab.com/blog/postmortem-of-database-outage-of-january-31/), 2017.
- Beyer, Jones, Petoff, Murphy (eds.): [Site Reliability Engineering, chapter 26, Data Integrity](https://sre.google/sre-book/data-integrity/), O'Reilly 2016.
- US-CERT: [Data Backup Options](https://www.cisa.gov/sites/default/files/publications/data_backup_options.pdf), 2012.
- PostgreSQL 16: [Continuous archiving and point-in-time recovery](https://www.postgresql.org/docs/16/continuous-archiving.html), [Backup and restore](https://www.postgresql.org/docs/16/backup.html).
- PostgreSQL 17: [pg_basebackup, incremental backups](https://www.postgresql.org/docs/17/app-pgbasebackup.html), [pg_combinebackup](https://www.postgresql.org/docs/17/app-pgcombinebackup.html).
- pgBackRest: [User guide](https://pgbackrest.org/user-guide.html), [Configuration reference](https://pgbackrest.org/configuration.html).

All sources of this study are also in [sources.md](sources.md).
