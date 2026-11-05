# 05.5: Working together

Everything here is wave C (spec 010), unless a line says otherwise. Owners: `internal/comments`, `internal/notify`, `internal/mail`, `internal/watches`, `internal/tasks`, `internal/sharing`.

## 5.1 Comments and reactions

- **DAT-110** · wave C · `comments`: `id`, `origin_instance_id`, `space_id`, `page_id`, `parent_id`, `author_id`, `body`, `anchor`, `status` (`open`, `resolved`), `editors_only`, `resolved_by`, `resolved_at`, `trash_item_id`, `created_at`, `edited_at`, `edited_by`.
  - **Body:** a small document through the same gate, with a smaller allow-list.
  - **Anchor:** `exact`, `prefix`, `suffix`, `offset`, `version`, following the W3C Text Quote Selector. `exact` is a copy of page text of at most 1,000 characters. The W3C model itself warns that copying text into an annotation can be a risk for restricted content.
  - **Life:** a comment goes to the trash of the space. With replies it leaves a line, and its words must not reach a reader while it is there. Purge removes it.
  - **Not part of a version.** A restore of a version leaves comments alone. A copy of a page copies none.
  - **Export:** an option in wave G, and always in a bundle.
- **DAT-111** · wave C · `comment_seen`: user, page, moment. Personal.
- **DAT-112** · wave C · `reactions`: `target_type`, `target_id`, `user_id`, `emoji`, `created_at`. At most 20 different emoji per item. Removed by the person. Follows its target when that is purged. Turning reactions off hides them and deletes nothing.

## 5.2 Watching, notifications and mail

- **DAT-113** · wave C · `watches`: `user_id`, `target_type` (`page`, `branch`, `space`), `target_id`, `created_at`. A row stays when access goes, and works again if access returns.
- **DAT-114** · wave C · `notifications`: `id`, `user_id`, `type`, `reason`, `actor_id`, `page_id`, `space_id`, `comment_id`, `group_key`, `source_event`, `data`, `read_at`, `hidden_at`, `digested_at`, `created_at`. Unique on person and source event, so a handler can run twice. It holds ids and no title. A share message or an answer to a request lives in `data` as written by a person, so it has the same retention and needs the same check. Expires after 90 days.
- **DAT-115** · wave C · `mail_queue`: `id`, `user_id`, `kind`, `notification_ids`, `status`, `attempts`, `next_attempt_at`, `sent_at`, `last_error`, `created_at`. It holds ids, not text, and the mail is made when it is sent. Errors hold no addresses.
- **DAT-116** · wave C · An unsubscribe token is sealed with the keyring and stored nowhere. It holds the person and what to stop. Its only power is to reduce mail.

## 5.3 Sharing and asking

- **DAT-117** · wave C · `access_requests`: `id`, `page_id`, `space_id`, `requester_id`, `kind`, `asked_id`, `status`, `answered_by`, `note`, `created_at`, `answered_at`. One open request per person, page and kind. Every step is audited. Sharing itself writes no row beyond notifications, and `page.shared` holds a number, not the message.

## 5.4 Editing together

- **DAT-118** · wave H, part 5 · `page_version_authors`: a version names everyone who took part.
- **DAT-119** · wave H, part 5 · The state of a shared document lives in the collab service. Its table, retention and what happens at a version removal are not designed yet (GAP-027).
