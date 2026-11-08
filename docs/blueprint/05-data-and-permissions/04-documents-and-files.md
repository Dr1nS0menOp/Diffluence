# 05.4: Documents, files and search

Owners: `internal/document`, `internal/attachments`, `internal/storage`, `internal/search`.

## 4.1 The document format

- **DAT-080** · built · A document is a JSON tree based on the ProseMirror model, with `type`, `attrs`, `content`, `marks`, `text` (ADR-004). `SchemaVersion` is 1. Everything added since stays valid, so it has not changed.
- **DAT-081** · built · `document.Normalize` is the only way in. It keeps what is on the allow-list, cleans attributes through `attrCleaners`, and refuses the rest. Limits: 5 MiB, depth 100. No stored document can carry a script, an event handler or an outside resource.
- **DAT-082** · built · Blocks: `doc`, `paragraph`, `heading`, `bulletList`, `orderedList`, `listItem`, `taskList`, `taskItem`, `blockquote`, `codeBlock`, `horizontalRule`, `image`, `file`, `panel`, `expand`, `columns`, `column`, `decision`, `table`, `tableRow`, `tableCell`, `tableHeader`. Inline: `text`, `hardBreak`, `status`, `date`.
- **DAT-083** · built · Marks: `bold`, `italic`, `underline`, `strike`, `code`, `link`, `subscript`, `superscript`, `textColor`, `highlight`. A link passes `SafeHref`: `http`, `https`, `mailto` and relative paths. Colours are names, never values.
- **DAT-084** · built · `image` and `file` hold an `attachment_id`, never an address. A document can name a file of another page. The file route then decides by that page's rights, so such a file simply does not show for someone who may not see it.
- **DAT-085** · built · `mention` (inline): `id`, `kind`, `label`. `taskItem` gains `id`. See GAP-004 on `label`.
- **DAT-086** · built · `templateField` and `instruction`, only in templates and drafts. Publishing removes `instruction`, and refuses `templateField`.
- **DAT-087** · built · `dynamic` with `kind` and `settings` from an allow-list per kind, `pageFacts`, `excerpt`, `variable`, `anchor`, `pageTag`, `pageCard`, `chart`, `drawing`, `timeline`, `media`, `spotlight`, and new attributes in spec 012. A document stores settings, never results, so no title is frozen into a version.
- **DAT-088** · built · `document.Text` gives the plain text for search, one line per text block. `Compare` works per block. `ToMarkdown` and `FromMarkdown` give the Markdown form.
- **DAT-089** · built · `document.ToHTML` is the one renderer for HTML, public sites and PDF. It escapes everything, and leaves the instance only as a file.
- **DAT-090** · later · An old schema is upgraded on read. No such upgrade has been needed yet.

## 4.2 Files

- **DAT-091** · built · `attachments`: `id`, `origin_instance_id`, `space_id`, `page_id`, `filename` (unique per page ignoring case, among active files), `status` (`active`, `trashed`), `current_version`, `trash_item_id`, `created_by`, times.
  - **Life:** a same-named upload adds a version. Delete sends the whole file to the trash. Purge queues every stored object.
  - **Showing:** only `image/png`, `image/jpeg`, `image/gif` and `image/webp` show inline. All else is a download. Headers lock it down.
  - **A document does not point at a version.** An old page version shows the newest file, and a purged file shows as gone. Versions of a page are immutable, and the files in them are not.
- **DAT-092** · built · `attachment_versions`: `id`, `attachment_id`, `number`, `object_key`, `sha256`, `size`, `content_type`, `encryption_key_id`, `uploaded_by`, `created_at`. `content_type` is detected from the file, never taken from the client. `encryption_key_id` is empty: files are not sealed yet, although blueprint 06 says so (GAP-017).
- **DAT-093** · built · `storage_deletions`: objects to remove, written in the transaction of the purge, worked off by the worker. A crash can never leave a file behind for good.
- **DAT-094** · built · `attachments.comment`, `attachment_refs` (which version uses which file, filled from every version), `attachment_events`, `attachment_labels`, `spaces.storage_limit_mib` (spec 024).
- **DAT-095** · built · The file itself lives under `storage.Store`, today on disk. The store knows no rules, and every read goes through `attachments`. `OpenUnchecked` and `ListForPage` exist for callers that checked already, and only `exchange` uses them.

## 4.3 Search and other derived tables

- **DAT-096** · built · `search_documents`: `page_id`, `title`, `body`, `attachment_names`, generated `tsv`, `updated_at`. `simple` configuration, so no stemming. Body is cut at 400,000 bytes to stay under the limit of 1 MB for a `tsvector`. Only the current version is indexed.
- **DAT-097** · built · `page_mentions`, `page_tasks`. `page_tasks` holds a copy of the task's text, and is an index only: the document stays the truth.
- **DAT-098** · built · `page_facts`, `page_decisions`, `page_includes`, `page_placeholders`. These copy text out of documents, so a reader only reaches them through the page.
- **DAT-098a** · built · `page_placeholders` has `source_id`, `title`, `target_id` (spec 023, D7). Source and title can be rebuilt from `page_versions`. `target_id` cannot: it records what a placeholder became, so publishing keeps it for a title that stays.
- **DAT-098b** · built · `pageLink`, `pageCard` and `placeholderLink` hold an id or the words of the writer, never a stored title (spec 023, D1). `document.ResolveLinks` turns them into words and links before a document leaves the instance.
- **DAT-099** · wave D or E · Searchable comments (ORG-024) and text inside files (ORG-025). Both need their own index with the rule of their object, not of the page alone (PRM-089).
