# 05.1: Conventions of the data model

How to read this chapter: see `../05-data-and-permissions.md`. Every line has a number that never changes, and one status.

## 1.1 Rules that hold for every table

- **DAT-001** · built · Every id is a UUIDv7 from `ids.New()`. Ids are globally unique and carry their creation time. That time is not secret, and a link that holds an id reveals when the object was made.
- **DAT-002** · partly · `origin_instance_id` marks where a row was made, for merging instances later. Built on `users`, `groups`, `spaces`, `pages`, `page_versions`, `attachments`. Rows that only exist as part of another row do not need it: `attachment_versions`, `drafts`, `page_restrictions`, `favourites`. New objects that can travel on their own must carry it. The old text said "every row that holds content", which never matched the code (GAP-017).
- **DAT-003** · built · Times are `timestamptz`, stored in UTC. Dates without a time, such as a due date, are `date`.
- **DAT-004** · built · Content columns that point at a person (`created_by`, `updated_by`, `author_id`, `deleted_by`) have no foreign key. A user row is never deleted, only disabled or anonymised, so these ids always resolve. Names are read when shown.
- **DAT-005** · partly · `pages.space_id` is the truth about where a page lives. Other `space_id` columns are copies for fast lists: built on `attachments` and `trash_items`, planned on `comments`, `page_tasks`, `notifications`, `access_requests`. A move between spaces rewrites every copy in one transaction (GAP-002).
- **DAT-006** · built · A module owns its tables. Other modules use its Go API, never its tables. The exception that exists: `pages` and `spaces` read `perm_*` functions, which belong to `permissions`.
- **DAT-007** · built · Migrations are numbered, and start with `-- phase: expand` or `-- phase: contract`. A merged migration is never edited. Expand keeps the previous release working, so N and N+1 run side by side.
- **DAT-008** · built · A status word is a `CHECK` constraint, with few values. A meaning that can combine with a status is its own column. So archived is `archived_at` and not a status (wave D), and what writers call a status is `stage`.
- **DAT-009** · built · Text from people has a limit in the module that takes it, in characters, not bytes. No text reaches a query by string building.
- **DAT-010** · built · A secret is stored hashed when it only has to be compared (`token_hash`, `password_hash`, `code_hash`), and sealed with the keyring when it must be read back (`secret_sealed`). A secret never reaches a log, an error or an audit event.
- **DAT-011** · built · A derived table can always be rebuilt from `page_versions`: `search_documents`, `page_links`, and in later waves `page_mentions`, `page_tasks`, `page_facts`, `page_decisions`, `page_includes`, `attachment_refs`. It is rewritten in the transaction that publishes, so it never runs ahead of a version or behind it.
- **DAT-012** · built · A derived table holds no rule. Whoever reads it joins through `perm_visible_pages`, or through the function that replaces it for lists (DAT-013).
- **DAT-013** · wave D · `perm_listed_pages(u)` is what lists use: visible and not archived. `perm_visible_pages(u)` stays what a direct address uses.
- **DAT-014** · built · Foreign keys that delete have one of three meanings, and each is chosen on purpose. `CASCADE`: the child has no meaning without the parent. `SET NULL`: the link is optional. No key: the row must outlive its target, as in `audit_events` and `page_links.target_id`.
- **DAT-015** · built · **Nothing may fall open through a delete.** `pages.parent_id` is `ON DELETE SET NULL` and `page_tree` cascades. So deleting a parent row without its branch would lift an inherited view restriction. Today only `Purge` deletes pages, and it takes the whole branch. That is a promise in code, not in the schema (GAP-019).

## 1.2 What can happen to a row

Each sheet in the next chapters says which of these apply.

| Word | Means |
|---|---|
| **Trash** | The row stays, marked. A `trash_items` row holds the end date. It can be restored |
| **Purge** | The row and what hangs below it go for good, by a person or at the end of retention |
| **Cascade** | The row goes because its parent went |
| **Expire** | A periodic task removes it after a set time |
| **Anonymise** | The row stays, and what identifies a person is replaced |
| **Keep** | The row stays for as long as the instance, or for the audit retention |
| **Rewrite** | A derived row, replaced at each publish |

## 1.3 Retention at a glance

| Data | How long | Set by | Status |
|---|---|---|---|
| Published versions | As long as the page | Nobody | built |
| Trash | 30 days by default, 1 to 3,650 | Administrator (`trash_retention_days`) | built |
| Drafts | 1 year by default, warned 14 days before | Administrator (`draft_retention_days`) | wave D |
| Recent pages | Last 50 per person | Code | built |
| Sessions | Idle and maximum age from the configuration | Operator | built |
| Short-lived login steps | Their own `expires_at`, cleared every 10 minutes | Code | built |
| Outbox events | 7 days after publishing | Code | built |
| NATS stream | 7 days | Code | built |
| Audit log | For ever, until pruned with the owner's role | Administrator policy, operator action | built, pruning wave F |
| Notifications | 90 days | Code | wave C |
| Exports | 24 hours | Code | wave G |
| Analytics days | 1 year by default | Administrator | wave H |
| Analytics hashes and salt | Until the day closes | Code | wave H |
| Rule runs, webhook deliveries | 30 days | Code | wave H |
| Search terms | 90 days, shown from three uses | Code | wave H |
| Presence | 60 seconds | Code | wave H |

GDPR says personal data stays identifiable only as long as the purpose needs it, and only what is needed is kept (Article 5). Every line above is an answer to that, and a new table must add one.

## 1.4 Where personal data sits

| Kind | Where | Reached by anonymising |
|---|---|---|
| Account facts | `users`, `user_profiles`, `user_identities` | Yes |
| Secrets and factors | `local_credentials`, `totp_credentials`, `recovery_codes`, `webauthn_credentials`, `api_tokens` | Yes, deleted |
| Traces of use | `sessions`, `login_history`, `known_devices`, `recent_pages`, `favourites`, `user_preferences` | Yes, deleted (`recent_pages` and `favourites` must join the list, GAP-018) |
| Links to others | `follows`, `watches`, `comment_seen`, `notifications`, `mail_queue`, `access_requests`, `page_view_people`, `page_presence` | Must join the list after waves C and H (GAP-018) |
| Authorship by id | `created_by`, `author_id` and the like | Resolves to the neutral name |
| Names inside documents | `mention` labels, and text people typed | No, unless the label leaves the stored document (GAP-004) |
| Audit | `audit_events` with IP, user agent, and for `user.created` the e-mail address | No: the chain must stay whole. Limited by the audit retention |
| Backups | Outside the database | No: erased again after a restore |
