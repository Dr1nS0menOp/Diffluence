# 04.3: Events, jobs and periodic tasks

Three kinds of background work exist, and each has its own job.

| Kind | Starts from | Holds | Runs |
|---|---|---|---|
| **Event** | A change that happened | Ids and a type | Every listener, at least once |
| **Job** | A person's request | Input and a result for them | Once, with a report |
| **Periodic task** | The clock | Nothing | Once per interval, in the whole cluster |

## 3.1 How an event travels

```
change + audit + outbox row   (one transaction)
        │
        ▼
relay in the worker           (SKIP LOCKED, batches of 100)
        │  Nats-Msg-Id = event id
        ▼
stream DIFFLUENCE_EVENTS      (7 days, file storage)
        │
        ▼
durable consumers             (one per listener, shared by all workers)
```

- **ARC-042** · built · An event is written in the same transaction as the change. It is never lost, and never sent for a change that was rolled back.
- **ARC-043** · built · The relay claims rows with `FOR UPDATE SKIP LOCKED`, which PostgreSQL describes as fit for queue-like tables, so several relays can run.
- **ARC-044** · built · Each message carries the event id as `Nats-Msg-Id`. JetStream drops a repeat within its duplicates window of two minutes, so a crash between publish and commit normally stores one copy.
- **ARC-045** · built · Published rows are removed after 7 days, the same as the stream.
- **ARC-046** · built · Delivery is **at least once and unordered**. Several relays and redeliveries can swap or repeat events. Every listener must allow for both, and must load current facts instead of trusting the order.
- **ARC-047** · built · An event holds ids, a type and a few flags, never titles or text. A listener reads what it needs at that moment, through permissions.
- **ARC-048** · built · Each listener is a durable pull consumer with explicit acknowledgement. Workers share one consumer, and the server spreads messages between them. A message is acknowledged only after the listener's transaction committed.
- **ARC-049** · built · Each listener is safe to run twice: a unique key on its own table, such as `(user, source_event)` for notifications and `(rule, event)` for rule runs.
- **ARC-050** · built · A consumer sets `MaxDeliver` and `BackOff`. By default JetStream redelivers for ever (`MaxDeliver` is -1), so one bad event would come back without end. After the last try the event is written to `event_failures`, shown in the console, and can be replayed.
- **ARC-051** · built · `MaxAckPending` defaults to 1000 for all workers together. A slow listener stops itself there, which is the right kind of back pressure. It does not hold up other consumers.
- **ARC-052** · phase 3 · In a cluster the stream and its consumers have three replicas. One copy loses the stream with its server, and a consumer with one copy is a single point of failure for its position. Two lost servers of three stop new messages, and the outbox simply waits.
- **ARC-053** · built · A stream with a limit of 7 days means a worker that is down longer loses events. The console warns from one day of lag, and the outbox keeps them for the same 7 days. Built: the console warns from one day, and the outbox keeps events (spec 021).

**What events are not for.** Audit and the search index are written in the same transaction as the change, not from events. The old text of this blueprint said otherwise. In-transaction is stricter: ORG-013 demands that a page is never found with stale content or permissions, and an audit gap would defeat the audit log.

## 3.2 Every event

Who listens: **N** notifications, **A** automation rules, **W** webhooks, **S** public site rebuild, **F** feed. Only N and F exist in wave C, and the feed reads tables rather than events (spec 010 D17). A, W and S come in waves H and G.

### Built

| Event | Sent by | Listens |
|---|---|---|
| `page.created` | `pages` | A, W |
| `page.published` | `pages` | N, A, W, S |
| `page.renamed`, `page.look_changed` | `pages` | W, S |
| `page.moved`, `page.copied` | `pages` | A, W, S |
| `page.version_restored` | `pages` | N, W, S |
| `page.trashed`, `page.restored`, `page.purged` | `pages` | A, W, S |
| `page.exported`, `space.exported` | `exchange` | W |
| `import.started`, `import.finished` | `exchange` | N, W |
| `space.created`, `space.updated`, `space.archived`, `space.unarchived`, `space.trashed`, `space.restored`, `space.purged`, `space.shortcuts_changed` | `spaces` | A, W |
| `attachment.uploaded`, `attachment.trashed`, `attachment.restored`, `attachment.purged` | `attachments` | N, A, W |
| `permission.space.granted`, `permission.space.revoked`, `permission.template.applied` | `permissions` | W |
| `restriction.added`, `restriction.removed` | `permissions` | S, W |
| `user.created`, `user.updated` | `identity` | W |
| `group.created`, `group.updated`, `group.deleted`, `group.member_added`, `group.member_removed` | `identity` | A, W |
| `permission.global.granted`, `permission.global.revoked` | `identity` | W |
| `auth.login_succeeded`, `auth.password_changed`, `auth.sessions_revoked`, `auth.totp_enabled`, `auth.totp_disabled`, `auth.passkey_added`, `auth.passkey_removed` | `identity` | W |
| `settings.updated`, `settings.font_changed` | `settings` | None yet |

### Planned

| Event | Sent by | Listens | Wave |
|---|---|---|---|
| `comment.created`, `comment.edited`, `comment.resolved`, `comment.reopened` | `comments` | N, A, W | C |
| `comment.trashed`, `comment.restored` | `comments` | W | C |
| `reaction.added`, `reaction.removed` | `comments` | N, W | C |
| `page.shared`, `access.requested`, `access.allowed`, `access.refused` | `pages`, `permissions` | N, W | C |
| `watcher.added`, `watcher.removed` | `watches` | W | C |
| `task.done`, `task.reopened` | `tasks` | A, W | C |
| `space.features_changed` | `features` | W | C, D |
| `page.owner_changed`, `page.stage_changed`, `page.labelled`, `page.unlabelled` | `pages` | N, A, W, S | D |
| Approvals, review reminders, archive, scheduled publishing | `pages` | N, A, W | D |
| `export.created`, `export.downloaded` | `exchange` | W | G |
| `site.created`, `site.changed`, `site.published`, `site.withdrawn` | `exchange` | W | G |
| Rule runs and webhook failures | `automation`, `webhooks` | N | H |

- **ARC-054** · built · `pages` adds `actor_id`, `version` and `silent` to its events (spec 010). A silent update reaches listeners, and notifications leave it out.
- **ARC-055** · built · The chain of rules travels with the event, so a rule cannot trigger itself, and a chain stops at five (spec 015 D12). This field belongs in the envelope from the first rule on.
- **ARC-056** · built · A webhook sends ids, the type and the time, and gets a title only if it opted in. The event table above is the public list of what can be subscribed to, so renaming an event becomes a breaking change at that moment.
- **ARC-057** · built · A page that gets a view restriction or is deleted leaves a public site in a build that starts at once (spec 014 D18). This is the one listener where delay matters for safety, so it gets its own consumer. It must not queue behind notifications.

## 3.3 Fan-out at scale

- **ARC-058** · built · One event can mean thousands of people: a space with 5,000 watchers. Fan-out asks the database once who may see the page, and writes all rows in one statement. It does not check people one by one in Go.
- **ARC-059** · built · That needs the opposite of `perm_visible_pages`, which answers "what may this person see". A new SQL function `perm_page_viewers(page)` answers "who may see this page", joins the matrix test, and is compared with `Evaluate` as the others are.
- **ARC-060** · built · Access is checked at fan-out, when shown and when mailed (spec 010 D11). The first check keeps the table small. The others keep the promise.

## 3.4 Jobs

- **ARC-061** · built · A job is a row in `jobs`, claimed with `SKIP LOCKED`, and its result stays for the person who started it (ADR-018). Earlier text said jobs ran through NATS. That was changed on purpose: one moving part fewer.
- **ARC-062** · built · A job has a limit of one hour. A job left running by a dead worker is failed by a periodic task, so nothing hangs for ever.
- **ARC-063** · built · **A worker runs one job at a time today.** One large import blocks every other job on that worker. Bulk actions, exports and site builds need several slots: a small pool per worker, with a separate slot for short jobs, so a large export cannot delay a small copy. This must land before wave D's bulk actions.
- **ARC-064** · built · A job that touches many pages uses one transaction per page (spec 011 D29). It never holds the audit lock or the tree lock for a whole run (ARC-201).
- **ARC-065** · built · The console can retry and cancel jobs that are queued or failed. A running job cannot be cancelled yet (spec 013).
- **ARC-066** · built · A job reports progress, in rows of its own kind, so the screen can show it.

| Job | Why it is a job | Wave |
|---|---|---|
| Import | Long, and reads a hostile file | built |
| Bulk actions, copies past 50 pages, space templates, permanent removal | Many pages | D |
| CSV import of people, personal data download | Many rows, or a file for later | F |
| Export of any kind | Can be large, and the file lives 24 hours | G |
| Site build | Many pages, and a safety scan at the end | G |
| A rule that matched | Limits and retries (spec 015 D9) | H |
| Conversion of office files | Slow, and runs in a helper | E |

## 3.5 Periodic tasks

- **ARC-067** · built · A task has a name and an interval. A lease row in PostgreSQL makes it run once in the cluster, also through PgBouncer.
- **ARC-068** · built · **All tasks run in one loop today**, one after the other. A task that takes minutes holds up the others, and `audit-verify` reads the whole chain. Digests and reminders must not wait for that. Long tasks move to their own goroutine, or become jobs.

| Task | Interval | Status |
|---|---|---|
| `purge-trash` | Hour | built |
| `drain-storage` | Minute | built |
| `fail-stale-jobs` | 10 minutes | built |
| `purge-identity` | 10 minutes | built |
| `purge-outbox` | Hour | built |
| `audit-verify` | Day | built |
| `audit-checkpoint`, with a copy in the application log (ADR-039) | 5 minutes, when something happened | built |
| `contract-migrations` | Minute | built |
| Example space on a new instance | Minute, until done | Being built |
| Send mail queue, batch mail at once | Minute | wave C |
| Digests | Hour, per person's chosen moment | wave C |
| Due tasks, remove old notifications | Day | wave C |
| Scheduled publishing | Minute | wave D |
| Review dates, stale draft warnings | Day | wave D |
| Guest expiry | Hour | wave F |
| Remove expired exports, site schedules | Hour | wave G |
| Previews no longer used | Day | wave E |
| Close the day for analytics | Day | wave H |
| Rule schedules | Minute | wave H |
| Restore test | Week | phase 3 |
