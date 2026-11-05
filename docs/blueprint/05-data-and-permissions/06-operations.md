# 05.6: Operations, exchange and the platform

## 6.1 Running the instance

- **DAT-130** · built · `trash_items`: `id`, `space_id`, `object_type` (`page`, `attachment`, `space`, and `comment` in wave C), `object_id`, `title`, `deleted_by`, `deleted_at`, `purge_after`. `title` is a copy of the title or file name, so it needs the rule of the object (GAP-001).
- **DAT-131** · built · `jobs`: `id`, `kind`, `status` (`queued`, `running`, `done`, `failed`), `payload`, `result`, `created_by`, times. A job is shown to the person who started it, and to nobody else. A result can hold titles and file names.
- **DAT-132** · built · `job_leases`: periodic tasks take turns through a lease row, also through PgBouncer.
- **DAT-133** · built · `settings`: `key`, `value`, `updated_at`. Secrets stay in the environment. Wave F adds typed sections.
- **DAT-134** · built · `nodes`: which node runs which version, for upgrades.

## 6.2 Audit and events

- **DAT-135** · built · `audit_events`: `seq`, `id`, `occurred_at`, `actor_type`, `actor_id`, `action`, `object_type`, `object_id`, `space_id`, `result`, `ip`, `request_id`, `payload`, `prev_hash`, `hash`. `payload` is the exact JSON that was hashed. No foreign keys: rows outlive what they describe. Triggers refuse update, delete and truncate, and the application role holds no such right.
  - **Holds titles.** `record` in `pages` adds the title, and files add their name. An administrator therefore reads titles of pages they cannot open (For the product owner to decide, 1).
  - **Holds an e-mail address** in `user.created`. Anonymising cannot change it.
- **DAT-136** · built · `audit_checkpoints`: a daily signed anchor of the chain. Wave F adds `audit_anchors` for pruning.
- **DAT-137** · built · `outbox_events`: `id`, `type`, `payload`, `created_at`, `published_at`. Payloads hold ids. NATS keeps events for 7 days, and de-duplicates for 2 minutes.

## 6.3 Exchange

Wave G, spec 014.

- **DAT-138** · wave G · `export_runs`: job, space, format, options, object key, file name, size, created by, expiry. Only the person who started it can download it. It needs the list of pages it holds (GAP-008).
- **DAT-139** · wave G · `export_policy`, `space_pdf_design`.
- **DAT-140** · wave G · `import_runs`, `import_items`. An item ties a source to the page it made, so Undo can work. A former author has no user row, so the model needs a place for that name (GAP-029).
- **DAT-141** · wave G · A bundle is a zip with `manifest.json`, a SHA-256 per file, and a detached signature. It holds no passwords, sessions, tokens, audit events or view data.
- **DAT-142** · wave G · `public_sites`, `public_site_pages`, `public_site_builds`, `public_links` (with a token of 128 random bits). A built site is a copy outside every rule, so what goes in is decided by PRM-096 to PRM-099.

## 6.4 Analytics

Wave H, spec 015.

- **DAT-143** · wave H · `page_view_days`, `page_view_links`, `analytics_days`: counts without people.
- **DAT-144** · wave H · `page_view_seen`, `analytics_salts`: an HMAC of a person's id with a salt per day. Both go when the day closes, so nothing can be traced back.
- **DAT-145** · wave H · `page_view_people`: only in name mode, off by default (ADR-026). Leaving that mode deletes the rows in the same transaction.
- **DAT-146** · wave H · `search_terms`: without people, shown from three uses. A term can hold a name.

## 6.5 Automation and the outside

- **DAT-147** · wave H · `automation_rules`, `automation_runs` (with `UNIQUE (rule_id, event_id)`), `automation_hooks` (hashed token). A run's report must hold ids, not titles (GAP-009).
- **DAT-148** · wave H · `outgoing_hosts`: empty by default, so nothing leaves.
- **DAT-149** · wave H · `webhook_endpoints` (sealed secrets), `webhook_deliveries` (no bodies).
- **DAT-150** · wave H · `page_presence`: who is here, for 60 seconds, only when not turned off.
