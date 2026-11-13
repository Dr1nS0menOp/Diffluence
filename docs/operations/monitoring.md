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
| Audit log | Events are sealed into the chain within seconds | An event waited more than a minute to be sealed: no worker seals | An event waited more than 15 minutes, so no checkpoint covers it |
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
| `diffluence_audit_unsealed_events`, `diffluence_audit_sealing_lag_seconds` | Audit events waiting for the sealer; how long the oldest has waited (ADR-041) |
| `diffluence_storage_deletions_queued` | Stored files waiting to be removed |
| `diffluence_disk_free_bytes`, `diffluence_disk_total_bytes` | The disk of the stored files, when the store knows it |
| `diffluence_database_size_bytes` | The database at the last hourly measurement |
| `diffluence_backup_last_noted_timestamp_seconds` | When a backup was last noted; 0 = never |
| `diffluence_schema_migration`, `diffluence_migrations_pending_expand`, `diffluence_migrations_pending_contract` | How far migrations are |
| `diffluence_migration_applied_timestamp_seconds` | When the last migration was applied |
| `diffluence_build_timestamp_seconds` | The date of the commit the release was built from |
| `diffluence_health_problems`, `diffluence_health_warnings` | How many checks need a person now, and how many a look |

## Traces

Diffluence can send traces to an OpenTelemetry collector (spec 060). It is **off** by default, and then nothing leaves the machine and nothing of it runs.

| Setting | Default | What |
|---|---|---|
| `DIFFLUENCE_TRACING` | `false` | Switch traces on |
| `DIFFLUENCE_TRACING_SAMPLE` | `0.1` | The share of new traces kept, from 0 to 1. A request that arrives inside a sampled trace (a `traceparent` from your proxy) is always kept |
| `OTEL_EXPORTER_OTLP_ENDPOINT` or `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | `http://localhost:4318` | Where the collector takes OTLP over HTTP |
| `OTEL_EXPORTER_OTLP_HEADERS`, `_CERTIFICATE`, `_TIMEOUT`, `_COMPRESSION` | | Authentication and transport, as OpenTelemetry documents them |
| `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES` | `diffluence` | The name and labels of this service in your tracing back end |

The first two are settings of Diffluence: they may sit in the configuration file and show in `diffluence config check`. The `OTEL_*` variables are the standard ones every collector documents; the exporter reads them itself, so put them in the environment.

What a trace holds:

| Span | Name | Attributes |
|---|---|---|
| A request | The route pattern, such as `GET /api/v1/pages/{id}`; `GET` alone for a request without a route | `http.request.method`, `http.route`, `http.response.status_code` |
| A query | The operation and the first table, such as `SELECT pages` | `db.system.name`, `db.operation.name`, `db.collection.name` |
| An event sent and handled | `queue publish diffluence.events.page.created`, `queue consume notify` | `messaging.system`, `messaging.destination.name`, `messaging.consumer.group.name` |
| A job and a periodic task | `job page.export`, `task purge-trash` | |
| The audit sealer | `audit seal` | `diffluence.audit.sealed`: how many events it sealed |
| The backup worker's commands | `backup next`, `backup report`, `backup restore-checked` | |

**Never in a span:** a path, a query string, an id, a statement or its values, a title, page text, a name, an email address, a token, a client address or a user agent. A failure sets the span's status to `error`, or for a database error to its SQLSTATE code, never to the error's message, which can quote a value. A test runs the whole application with tracing on and fails if any span holds such content.

While tracing is on, every log line written inside a request or a job carries `trace_id` and `span_id`, so you can go from a line to its trace and back. While it is off, the request line still carries `trace_id` when your proxy sent a `traceparent`.

What it costs is in [limits.md](limits.md). When the collector cannot be reached, spans are dropped after a queue of 2,048, a request never waits for it, and the log says so at most once a minute.

## Log events

Every line is JSON on standard error, with `time`, `level` and `msg`. The messages below are an interface: they change only with a note in the changelog, and a test checks that each one still exists in the code. Other messages may change without notice.

| `msg` | Level | When |
|---|---|---|
| `http request` | info (debug for `/livez`, `/readyz`, `/metrics`) | Every request: `method`, `path` without query or token, `status`, `duration_ms`, `ip`, `request_id`, `trace_id` |
| `audit checkpoint` | always written, whatever the level | Each signed checkpoint of the audit chain, for a witness outside the database (ADR-039) |
| `AUDIT LOG TAMPERING DETECTED` | error | The check of the audit chain found a change |
| `sealing the audit log failed` | error | The sealer could not put waiting events into the chain |
| `job failed` | error | A queued job ended in failure: `job`, `kind` |
| `periodic task failed` | error | A periodic task failed: `task` |
| `queue: gave up on an event` | warn | An event failed every try and waits for a replay in the console |
| `outbox relay failed, retrying` | error | Events cannot reach the queue |
| `nats disconnected` | warn | The node lost the queue |
| `sending a mail failed` | warn | One mail failed and will be tried again |
| `request found no database connection in time` | warn | The pool was full for 10 seconds; the request got 503 |
| `readiness check failed` | warn | `/readyz` answered not ready |
| `panic in handler` | error | A request crashed; it got 500 |
| `a setting does not work yet` | warn | A setting tried at start failed (OPS-101): `setting`, `reason` |
| `UPGRADING WITHOUT A RECENT BACKUP: make backups before the next upgrade, which will refuse without one` | warn | The first upgrade of an installation that never noted a backup (ADR-042) |
| `tracing: spans could not be sent` | warn | The collector cannot be reached, at most once a minute |

## Alerts

[`deploy/monitoring/diffluence-rules.yml`](../../deploy/monitoring/diffluence-rules.yml) holds two objectives, over 30 days: 99.5% of requests succeed, and 99.5% take less than 300 ms. A fast burn of the error budget pages within minutes and a slow one opens a ticket, as the Google SRE workbook describes. The other alerts are causes that need a person: Diffluence or its database down, the disk above 85% and above 98% full, no backup in 26 hours, jobs failing, an outbox that does not drain, audit events not sealed for 15 minutes, events that gave up, a contract migration waiting a day, the queue unreachable, mail failing, a release older than 60 days. Change the numbers to your own objective.

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
| Audit events waiting for the sealer | Seconds | The sealer of every worker, every second |
| Pages, versions and files | For good: they are what people keep | People and the trash |
