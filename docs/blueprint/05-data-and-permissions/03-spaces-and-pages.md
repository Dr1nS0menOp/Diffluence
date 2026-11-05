# 05.3: Spaces and pages

Owners: `internal/spaces`, `internal/pages`. Grants and restrictions belong to `internal/permissions`.

## 3.1 Spaces

- **DAT-050** · built · `spaces`: `id`, `origin_instance_id`, `key` (unique ignoring case), `name`, `description`, `icon`, `status` (`active`, `archived`, `trashed`), `home_page_id`, `created_by`, times.
  - **Life:** active, archived (read-only for everyone), trashed, purged. A key stays taken while its space is in the trash.
  - **Purge:** the row goes, and every table with `space_id` cascades. Stored files are queued in `storage_deletions` in the same transaction.
  - **Export:** Markdown zip of what the person may see (built). Bundle of everything (wave G).
- **DAT-051** · wave C or D · The switches of a space. Wave C has `spaces.features`, a JSON column, and wave D has a table `space_features`. There can be one truth only (GAP-005).
- **DAT-052** · wave D · `spaces`: `system`, `free_stage`, `review_months`, `approval_required`, `fixed_labels`, `title_align`, `icon_object`. A system space has no grants, so nobody reaches it through the rule, and it holds instance templates.
- **DAT-053** · wave F · `spaces`: `kind` (`team`, `personal`), `owner_id`, with a unique index on `owner_id` for personal spaces. The key of a personal space is `~` and ten characters of the person's id, so it holds no name.
- **DAT-054** · wave D · `space_owners`, `space_stages`, `space_fixed_labels`, `space_approvers`, `space_key_aliases`. All cascade. An alias stays taken, and redirects.
- **DAT-055** · wave F · `space_categories`, `space_category_links`.
- **DAT-056** · built · `space_grants`: `space_id`, `subject_type` (`user`, `group`, `all_members`, `all_guests`), `subject_id` (nil for the last two), `permission`, `created_at`. Cascades with the space. Layer 2 of the rule.
- **DAT-057** · built · `space_favourites`, `space_shortcuts` (`label`, `target`, `position`, at most 20). A shortcut target passes `SafeHref`, without `mailto` and `#`.

## 3.2 Pages

- **DAT-060** · built · `pages`: `id`, `origin_instance_id`, `space_id`, `parent_id`, `position`, `title`, `status` (`draft`, `published`, `trashed`), `pre_trash_status`, `trash_item_id`, `current_version`, `created_by`, `updated_by`, times.
  - **Life:** see 05.7.
  - **`status`** is the life cycle. It must not be confused with `stage` (DAT-064).
  - **`created_by`** decides *delete own*, and who sees an unpublished page. It never changes.
- **DAT-061** · built · Page look, not part of a version (ADR-025): `emoji`, `full_width`, `title_align`, `hide_children`, `header_attachment_id` (`ON DELETE SET NULL`), `header_position`. A header must be an active image of the same page, checked on write and on read.
- **DAT-062** · built · `page_tree`: closure table, one row per ancestor and descendant, with each page as its own ancestor at depth 0. Rows stay for trashed pages, and for archived pages (wave D), so restrictions keep working while a page is away.
- **DAT-063** · wave D · `pages.kind`: `page`, `post`, `folder`, `link`, `template`. A post has no parent. A template is never a page to anyone. `link_target` passes `SafeHref`.
- **DAT-064** · wave D · `pages.owner_id` (who to ask, gives no right), `stage_text`, `stage_color`, `review_on`, `archived_at`, `archived_by`, `published_at`. The migration fills `owner_id` from `created_by`.
- **DAT-065** · wave C · `pages.comments_closed`.
- **DAT-066** · built · `page_versions`: `id`, `origin_instance_id`, `page_id`, `number`, `title`, `document`, `schema_version`, `author_id`, `summary`, `restored_from`, `created_at`. A trigger refuses every update. Delete is allowed, because a purge needs it.
  - **Keep:** as long as the page. Everyone who can view the page can read every version, so a sentence removed from a page stays readable in its history.
- **DAT-067** · wave C · `page_versions.silent`, set when written.
- **DAT-068** · wave D · `removed_versions`: number, who, when, why. An administrator removes one version that is not the current one, and its row goes. What that does not reach is GAP-026.
- **DAT-069** · built · `drafts`: `page_id`, `user_id`, `title`, `document`, `schema_version`, `base_version`, `updated_at`. One per person per page, private, not audited. Cascades. Reading one needs view on the page, so a person who lost access loses their draft from sight until it expires.
- **DAT-070** · wave H, part 5 · A shared draft replaces private drafts for pages in shared mode (ADR-026). Its store is not modelled yet (GAP-027).
- **DAT-071** · built · `page_restrictions`: `page_id`, `kind` (`view`, `edit`), `subject_type` (`user`, `group`), `subject_id`, `created_at`. Cascades with the page. Layer 3 of the rule. An empty list is no restriction.
- **DAT-072** · built · `favourites`: `user_id`, `page_id`, `position`, `created_at`. `recent_pages`: `user_id`, `page_id`, `viewed_at`, last 50. Both cascade from both sides, and neither is audited. Wave D adds `seen_version` to `recent_pages`.
- **DAT-073** · built · `page_links`: `source_id`, `target_id`, rewritten at each publish. The target has no foreign key, so a link to a page that is gone stays visible for clearing up.

## 3.3 Labels and what organises pages

- **DAT-074** · wave D · `page_labels`: page, label. Text, with no table of labels, so a label nobody uses disappears by itself. Copied only when the copy dialog says so.
- **DAT-075** · wave D · `page_verifications`, `scheduled_publications`, `approval_rounds`, `approval_answers`. A round holds a frozen copy of the text, which stays on record with the page and cascades on purge.
- **DAT-076** · wave D · `templates`, `template_favourites`, `template_uses`, `space_templates`. A template's text lives in a page of kind `template`.
- **DAT-077** · wave E · `block_templates`: own or shared with a space. Reading a shared one needs view on that space.
