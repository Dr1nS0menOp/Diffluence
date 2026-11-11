# Watching an installation

Written for: operators who run Diffluence and want to know before people notice.

Diffluence watches itself (spec 051). You see the same checks in three places:

- **System health** in the console: every check, with what it says and what to do. Every console page shows a band while the disk is running out, and the summary lists the checks that need a look.
- **`diffluence doctor`** on the command line: the same checks, plus the configuration and the keys, in a short report you can paste into a question. It holds no address, secret or name. It exits 1 when something needs a person now.
- **`/metrics`** for Prometheus, with the rules and the dashboard in [`deploy/monitoring/`](../../deploy/monitoring/).

## The checks

| Check | Fine | Needs a look | Needs a person now |
|---|---|---|---|
| Disk of the stored files | 10% or more free | Less than 10% free, or the growth of the last 30 days would fill it within a month | Less than 2% free: uploads, imports and exports are refused, text still saves |
| Database | Its size, and its growth over 30 days (measured once an hour) | | |
| Backup | Noted in the last 26 hours | None noted, or older than 26 hours | |
| Migrations | All applied, or contract migrations waiting during an upgrade | A contract migration waited a day longer than `DIFFLUENCE_CONTRACT_GRACE` | |
| Background work | Runs | A job failed in 7 days, or an event gave up | An event waited more than an hour to reach the queue |
| Mail | Set up | No mail server, or a mail failed in the last day | |
| Login | As set up | An account's password login stopped after many wrong guesses | |
| Release | Built less than 60 days ago, or a developer build | Built more than 60 days ago | |

The answer is kept for 30 seconds per node. Nothing is asked of any outside server: the age of a release comes from its own build date.

**A nearly full disk.** Below 2% free, every new file is refused with HTTP 507 and a message that says text still saves. `/readyz` stays `200` and adds `"notes": ["disk_low"]`: a single node keeps serving text, and a load balancer with another node to choose from can prefer that one.

## Metrics

`/metrics` answers a scraper that sends the token set in `DIFFLUENCE_METRICS_TOKEN`. Names change only with a note in the changelog.

| Metric | What |
|---|---|
| `diffluence_http_requests_total{method,status}` | Requests, by method and class of status (`2xx`, `5xx`, ...) |
| `diffluence_http_request_duration_seconds` | How long requests took; a bound at 0.3 seconds for the latency objective |
| `diffluence_database_up` | Whether the database answered the scrape |
| `diffluence_queue_healthy` | Whether this node reaches the queue |
| `diffluence_db_pool_acquired_connections`, `_idle_connections`, `_max_connections` | The connection pool |
| `diffluence_outbox_unpublished`, `diffluence_outbox_stuck` | Events waiting; those waiting more than an hour |
| `diffluence_jobs_queued`, `diffluence_jobs_failed_7d` | Jobs waiting; jobs failed in 7 days |
| `diffluence_event_failures` | Events that gave up and wait for a replay |
| `diffluence_mail_queued`, `diffluence_mail_failed_1d` | Mail waiting; mail failed in a day |
| `diffluence_storage_deletions_queued` | Stored files waiting to be removed |
| `diffluence_disk_free_bytes`, `diffluence_disk_total_bytes` | The disk of the stored files, when the store knows it |
| `diffluence_database_size_bytes` | The database at the last hourly measurement |
| `diffluence_backup_last_noted_timestamp_seconds` | When a backup was last noted; 0 = never |
| `diffluence_schema_migration`, `diffluence_migrations_pending_expand`, `diffluence_migrations_pending_contract` | How far migrations are |
| `diffluence_migration_applied_timestamp_seconds` | When the last migration was applied |
| `diffluence_build_timestamp_seconds` | The date of the commit the release was built from |
| `diffluence_health_problems`, `diffluence_health_warnings` | How many checks need a person now, and how many a look |

## Alerts

[`deploy/monitoring/diffluence-rules.yml`](../../deploy/monitoring/diffluence-rules.yml) holds two objectives, over 30 days: 99.5% of requests succeed, and 99.5% take less than 300 ms. A fast burn of the error budget pages within minutes and a slow one opens a ticket, as the Google SRE workbook describes. The other alerts are causes that need a person: Diffluence or its database down, the disk above 85% and above 98% full, no backup in 26 hours, jobs failing, an outbox that does not drain, events that gave up, a contract migration waiting a day, the queue unreachable, mail failing, a release older than 60 days. Change the numbers to your own objective.

## What grows, and what keeps it small

| What | Kept | Cleaner |
|---|---|---|
| Logs | Standard error | Your platform rotates them (Docker, journald) |
| The trash | The retention set in the console | `purge-trash`, hourly |
| Previews | Made again when needed | `purge-previews`, daily |
| Exports | Their download window | `purge-exports`, every 10 minutes |
| Imports | Until they are done | `purge-imports`, hourly |
| Published events in the outbox | 7 days | `purge-outbox`, hourly |
| Events that gave up | Their retention | `purge-event-failures`, daily |
| Rule runs, the webhook log, notifications | Their retention | `purge-rule-runs`, `purge-webhook-log`, `purge-notifications`, daily |
| Sessions and one-time codes | Until they expire | `purge-identity`, every 10 minutes |
| Finished jobs | 30 days; failed ones 90 days | `purge-finished-work`, daily |
| Sent and dropped mail | 30 days; failed mail 90 days | `purge-finished-work`, daily |
| Nodes that stopped without leaving | A day | `purge-finished-work`, daily |
| Size measurements of the database | 40 days | `purge-finished-work`, daily |
| The queue's files | 7 days per stream | NATS itself |
| Stored files nothing points at | Until an administrator removes them | Care in the console |
| The audit log | The retention set in the console | `diffluence audit prune`, by the owner of the tables |
| Pages, versions and files | For good: they are what people keep | People and the trash |
