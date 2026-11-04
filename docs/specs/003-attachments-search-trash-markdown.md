# 003: Attachments, search, trash, Markdown

Status: built, waiting for the product owner's acceptance session (written by Claude from the blueprint)
Phase: 1

## Problem
A wiki needs files, needs to be searchable, must forgive mistakes, and must not lock people in.

## Behaviour

**Attachments.** Users with *add attachments* upload files to a page, up to a configurable size (default 100 MiB). Uploading a file with the same name adds a new version of it. Files can be downloaded and deleted (to the trash). Access always follows the page.

Files are never trusted. Only PNG, JPEG, GIF and WebP, detected from the content and not from the name, are shown inline. Everything else, including SVG and HTML, is sent as a download with a locked-down content policy.

**Search.** Full-text search over titles, published content and attachment names, with filters for space, author and date. Results show a snippet, and only ever include pages the user may view. It uses PostgreSQL full-text search without language-specific stemming, so Dutch and English behave alike. The last word of a query also matches as a prefix. Indexing happens in the same transaction as publishing, so a page is never findable with stale permissions or content.

**Trash.** Deleted pages, attachments and spaces stay in the trash for one universal retention period (default 30 days, set in the admin console). Space admins see their space's trash, restore items, or purge them early. A background job permanently removes expired items, including search entries and stored files.

**Markdown export.** Page as `.md`. Page tree or space as a `.zip` with folders and attachments. Exports contain only what the user may view, and need *export space*.

**Markdown import.** A `.md` file or a `.zip` of folders creates pages under a chosen parent. Imports run in the background and report their progress and problems. Limits: 200 MiB compressed, 1 GiB unpacked, 10,000 files, 20 MiB per Markdown file. Paths are never used on disk, so zip-slip cannot happen. Images found in the zip become attachments. External images become links.

## Permissions and audit
Attachments: *add*, *delete own*, *delete any*, and edit restrictions apply. Download: view. Trash: *administer* for a space's trash. Import: *create pages*. Events: `attachment.uploaded|downloaded|trashed|restored|purged`, `trash.purged`, `page.exported`, `space.exported`, `import.started|finished`. Searching is not audited (like page views).

## Acceptance criteria
- [x] An HTML file uploaded as `.png` is served as a download with `nosniff`
- [x] An attachment on a restricted page returns 404 for users who cannot view the page
- [x] Uploads over the limit are refused and leave no file behind
- [x] Search never returns a page the user cannot view; this is part of the permission matrix
- [x] After a purge, the page, its versions, its search entry and its stored files are gone
- [x] The purge job honours the retention setting
- [x] Markdown round trip: headings, lists, task lists, tables, code, quotes, links and images survive export and import
- [x] A zip bomb and a path like `../../etc/passwd` cannot harm the system
- [x] Parsers survive fuzzing

## How the product owner tests it
Upload an image and a PDF. Delete and restore a page. Search for a word from a restricted page as another user. Export a space and import it into a new one.

## Provenance
Own design from docs/blueprint/03, 05, 06. CommonMark and GFM are public specifications.
