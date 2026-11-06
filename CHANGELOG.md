# Changelog

Plain-language notes per release. Every entry has an "Upgrade impact" line.

## Unreleased

### Added: the rest of daily writing, reader tables and moving files (wave J)

- A new page starts in the title. Enter moves to the text, and Backspace at the very start moves back.
- A click below the text goes on writing there. Word count, at the foot of the sheet, also counts a selection.
- Arrows, an ellipsis, curly quotes and smileys come from what people type anyway. A preference turns it off, and Backspace right after it brings back what was typed, also after a heading or list shortcut.
- The `/` menu has group headings, and what a person used lately comes first.
- Pasted Markdown turns into blocks, and Undo brings back the plain text. Ctrl Shift V keeps it plain.
- Lists change style by level, a list changes kind without losing its nesting, Find has Whole word, and the browser's spell checker is on.
- Readers can sort and filter a table for themselves, and a long table keeps its header in view. Nothing in the page changes.
- A link to a page the reader cannot reach is struck through and says why. A hidden page and a deleted page look the same.
- A file on the page can be shown again without a new upload. A tile shows the type and size, and a missing file or image says so.
- A file moves or copies to another page, and its tiles keep working. The list of a page sorts and filters.
- Administrators can block file types, by name and by content.
- With a helper connected, office files open as a PDF. A saved drawing can be changed.

**Upgrade impact:** no migration. To use Collabora for office files, set `DIFFLUENCE_COLLABORA_URL`. Without it, the LibreOffice of the PDF helper does the work.

### Security

- Moving a file needs delete on the source and add on the target, and from then on the rules of the new page decide who gets it.
- A blocked type looks at what a file contains too, and a rename cannot get around it.
- The office helper gets the content and the extension of a file, not its name. What comes back is a download with the same locked headers as every file.

### Added: databases (wave I)
- A database is a fourth kind of item, beside pages, blog posts and folders. It works like a simple spreadsheet: type, move with the arrows and Tab, paste from Excel, sums such as `=SUM(B1:B3)`, and undo
- Columns hold text, numbers, dates, ticks, choices or links. Readers can sort and filter. Permissions, history, search, trash and copying work as for a page
- CSV in and out, and Open in GridCraft for heavy work, for those who may export. Spreadsheet files on a page open in GridCraft too
- Upgrade impact: one migration that only widens what is allowed. Nothing to do

### Added: PDF in the browser
- A PDF opens in PdfCraft, from the file in the page, the Files list or its details, so people can look at it and change it there. The result is saved as a download. Download always stays next to it, because a screen reader cannot read the viewer
- Upgrade impact: the program grows by about 28 MiB. Nothing to do. The viewers run in a sandboxed frame with their own policy, and the rest of the app stays as strict as it was

### Added: insight, automation and the platform (wave H)
- Analytics with their own switch, off by default. Counts mode keeps nothing that points at a person once a day has closed. Name mode says what it does, and leaving it deletes the names for good. Your own views do not count, and page views stay out of the audit log
- A view count in the byline for those who may see it, insights for a page, a space and the instance, what people search for, most viewed pages on the overview, and sortable tables with a safe CSV
- Rules per space and for the instance: when, if, then. They run as their account through the same checks as a person, with a test run, a run log, ready-made rules, export and import, limits, and every action in the audit log with the rule's name
- Webhooks in the Standard Webhooks format, with a delivery log, five attempts over a day, and rotation with two signatures. Nothing leaves until an administrator allows a host
- The API has an OpenAPI description and a stability policy. Tokens gain the scopes read, write, export and admin, and a limit per minute
- `diffluence client` lists, exports, imports and backs up content with a token
- Who else is on a page, and a line in the editor when someone else is editing. Both can be turned off, by a person and for the instance
- Editing together, with the optional `collab` service: one shared draft per page, cursors with names, an undo per person, work kept offline, and versions that name everyone who took part. Without the service, every page keeps private drafts

### Security
- Outgoing calls go through one client: allowed hosts only, https, no private address at connect time, no redirects, 10 seconds and 64 KiB. Every CSV goes through one guard, which now also covers line feeds and full-width characters. The use of a token is in the audit log at most once a minute, and so is a refused scope

Upgrade impact: migration 0018 adds tables and a column only, and gives View analytics to everyone who administers a space. A token that could write can still export, and now needs the admin scope for routes under `/api/v1/admin/`. New optional settings `DIFFLUENCE_OUTWARD_ALLOW_PRIVATE`, `DIFFLUENCE_COLLAB_URL` and `DIFFLUENCE_COLLAB_TOKEN`. Installing the service adds a Node.js container, and on those pages a draft is no longer private.

### Added: everything can leave and come in (wave G)
- One Export dialog on pages and spaces: PDF (with the optional Gotenberg helper), Word, OpenDocument, an HTML website that works offline, Markdown and plain text. Choose pages by hand. Files are made in the background, you get a notification, and only you can fetch the file for 24 hours
- An administrator can turn formats off, set sizes, and see what a helper needs
- Every import from the screen starts with a report and changes nothing until you say so. It can be undone in one action, and never removes someone's edit
- Markdown links, wiki links and front matter now carry over. Word files become pages, several at once wait under a restricted page, and one can go into a draft. HTML comes in through the allow-list
- A signed bundle moves a space to another instance with its history, comments, labels and permissions. One changed byte stops it, and a restriction that cannot be matched keeps a page closed
- Public sites and public links, as plain files for your own web server, off until an administrator turns them on. A preview comes first, and a page the world may no longer see leaves at once

**Upgrade impact:** migration 0017 adds tables and one column, with no downtime. To use PDF, start the `pdf` profile and set `DIFFLUENCE_PDF_URL`. To use public sites, set `DIFFLUENCE_PUBLISH_DIR` and turn them on under Administration, Export and publishing.

### Added: blocks and files (wave E)
- Blocks that fill themselves: contents, child pages, page tree, page index, spaces, recent changes, your favourites, drafts and tasks, page lists and reports, labels, facts, decisions, tasks, contributors, history, files, blog posts, link cards and a search box. Each reader gets their own answer
- Page facts with a report that sorts by real value, excerpts, showing text or a whole page from elsewhere, and anchors. A page that shows itself stops with a message, and text goes at most five levels deep. Details and Delete name the pages that show a page's text
- Charts from a table in the document, in four kinds, with the numbers one press away. Video and audio play in the page
- A Files screen per space, with storage for administrators. File versions, rename, a text preview and a zip of all files. The editor asks before it adds a version under a known name
- Drawing through an optional draw.io service on an address the administrator sets

### Security
- media-src 'self' joins the policy, and only a video or audio file that really is one plays. rame-src appears only with a drawing service, and that service must be on another origin. Its messages are checked on window and origin

Upgrade impact: migration 0016 adds tables and columns only. New optional setting DIFFLUENCE_DRAWING_URL. PDF stays a download.

### Added: people, access and administration (wave F)
- People and a profile for everyone, with a picture you crop by mouse or keyboard, and a choice of who sees what. Names lead to a small card. Guests and former colleagues are marked
- A forgotten password, invitations and self sign-up, by a link that works once. New passwords need 15 characters and are checked against 5 million leaked ones. Existing passwords stay valid
- A person with only a password is asked for a passkey or second factor. It can be put off, not switched off. An administrator can require a second factor, and an idle timeout with a warning
- Personal tokens for scripts, which never reach account security or the console. Sensitive actions ask for proof again after 10 minutes
- Personal spaces, a Manager role, who has access and how, a summary before someone loses access, and a safe CSV of it
- Restrictions in three clear states. Group owners manage members outside the console
- Dates follow your time zone and format. Your account has sections, and you can download your data or ask for removal
- The console opens on a summary with what needs attention, and warns while no identity provider or passkey is set up. It finds settings by name, filters users and audit through the address, and has bulk actions, import, service accounts, jobs, reports and unused files
- The look of an instance: name, logo, an accent for light and dark, login text and a banner. Maintenance mode stops every write and holds jobs
- Anonymising an account, with a reason, and `diffluence audit prune` with a signed anchor

### Security
- A passkey now needs a PIN or fingerprint on the device, at login and when it is added. A key without one no longer works
- Adding or removing a passkey and turning off a factor ask for proof again. After removing one, Diffluence offers to log you out elsewhere, and an administrator's reset does that by itself
- A failed login no longer stores the address that was typed, which could hold a password by accident. Old rows stay until their retention ends
- The signatures on the audit log are now checked: by the daily job, by Verify and by `diffluence audit verify`, which `restore.sh` runs. A failure shows in the console
- New recovery codes hold 128 bits. Codes people already have keep working until they set up again
- SCIM tokens end after a year or on a date you choose, the console warns a month ahead, and Rotate gives a successor that overlaps for 7 days. Existing tokens get a year from the upgrade
- An administrator sees a page they cannot read in the audit log by its id, with the words (Censored title), on the screen and in exports. The id leads to a note on the page, without its title
- Administrators need a second factor 14 days after a new instance began. An operator can choose another rule in Rules
- What identifies a person in an audit event now sits apart from the chain, so anonymising erases it and the chain still holds. Events written before this stay as they are
- The audit log stays 12 months by default. Longer needs a stated reason
- Anonymising also removes follows, watches, what was seen, notifications, queued mail, favourites, recent pages and drafts

Upgrade impact: migration 0010 adds columns and tables, and gives members the right to a personal space. Expand only, so the previous release keeps working. New passwords are held to the new rules from now on. API tokens and the screens above need no setup. Reset links and invitations go by mail once `DIFFLUENCE_SMTP_HOST` is set. An instance older than 14 days asks its administrators for a second factor at their next login.

### Changed: the editor after the first test
- A table has one button on its outline, with one menu for rows, columns, cells, colours and width. The row of tools no longer changes in a table
- Tables have a clear outline and a soft grid
- Tables stretch up to the full width of the window, by dragging their end edge, from the menu or with the arrow keys
- A page scales with the window by itself, and Full width is a button in the row of tools
- Add emoji, Add header image and Page options sit at the title. Page look is gone. The header image can be dragged into place
- Code blocks know about 190 languages, with a search. They load when needed
- The tree stays beside the editor
- Leaving the editor saves first, and when that fails a message brings the text back
- Opening a page and clicking in it no longer leaves a draft behind when nothing was typed
- A long menu that opens upwards stops below the bars, so its first items can be reached

Upgrade impact: no database change. Documents can hold a new table width, so while two versions run side by side, a node on the older version drops it.

### Changed: animals, and changes you can see
- Every space has a cartoon animal, picked from its key. Pick another in Space settings, or use a character of your own
- A comparison now looks like the page: added on green, removed on red and struck through, with the word beside it
- Opening a version in History shows what changed in it, with a button for the clean page
- Fields for people show them as soon as you click in them, and work with the arrow keys

Upgrade impact: no database change. Existing spaces get an animal at once, unless they chose a character.
### Added: organising (wave D)
- A page has an owner, a status, a Verified mark and a review date. Handing a page over gives no right to delete it
- Labels at the bottom of a page, with their own screens. A space can keep to a fixed list
- Templates: a gallery, fields that a form fills in, instruction text for the writer, ten starters, and templates for a whole space
- Blog posts, folders, link entries in the tree, and a company home page
- Publish options: location, status, silent update, a schedule and a preview
- Archiving: a page leaves the tree, keeps its address and can come back
- Approvals: ask people to approve a page, and a space can require it
- Moves and copies between spaces say what changes first. Several rows move together, and a row can be dragged to a space
- The Content screen filters the pages of a space and works on many at once, with a summary first and a report after
- Space settings: key, owners, home page, features, statuses, labels, picture
- Search understands quotes, AND, OR, NOT, brackets, wildcards and fields such as `label:hr`, with filters in the address
- Text stays in the browser when saving fails, and Close waits for the save
- An administrator can remove one version with a reason
- The trash can be searched, sorted and worked off together. It no longer names what is hidden from the person, and such an item cannot be purged by hand
- A copy that would lose a restriction from a page above it now needs the right to manage restrictions
- Comments can be searched, and a result links to the comment. A thread for editors and a comment in the trash stay out
- Removing a version also takes its words out of comments that quote them
- A page that moves to another space takes its comments, tasks, notifications and requests along
- The example space shows labels, a status, a folder, a blog post and a template

Upgrade impact: migration 0008 adds columns, tables, `perm_listed_pages` and `perm_visible_comments`, and replaces `perm_visible_pages`. It copies `spaces.features` into `space_features`, and that column stays until a contract migration. A change to those switches on the previous release during an upgrade is not carried over. Expand only. While both releases run, the old one shows posts in the tree, and nobody can make a post on it.

### Added: working together (wave C)
- Comments under a page, with replies, edits, removal with Undo, and a link to each comment. A draft stays in your browser
- Comments on chosen words or on an image. The words stay marked while the page changes around them, and a thread says so when they are gone. Threads can be resolved and reopened, and a panel beside the page lists them all
- Reactions on pages and comments, with who reacted
- `@` mentions in pages and comments, with a warning for someone who cannot see the page
- Tasks can have a person and a date, and Your tasks lists them, grouped by date. Ticking a task there also ticks it on the page
- Share tells people about a page and says who can see it. It never gives access
- Asking for access, and asking to edit. The answer says what will change. A space chooses whether hidden pages offer it
- Watching a page, a branch or a space, with a list to stop each. Publish without notifying for a small fix
- The bell and a notifications screen, which group, link to the exact spot, and follow what you may see at that moment
- Mail through any SMTP server: at once or as a digest, quiet hours, a pause, and one-click stop. A mail about something you watch has a link to stop watching. For testing, `docs/development.md` explains Mailpit
- The start screen has For you, Recent activity and Popular. You can follow someone from the Details panel
- New keys: `g` `n`, `w`, `m`, `n` and `p`

Upgrade impact: migration 0007 adds tables and columns. Expand only, so the previous release keeps working. Mail stays off until `DIFFLUENCE_SMTP_HOST` is set.

### Fixed: what stays behind
- Recent searches and the state of the tree are kept per person, and logging out removes them. Older ones without a person are removed once
- A token in an address no longer reaches the request log

### Added: the home of a space, and an example space
- The home page is no longer a row of the tree. It has a fixed Home row above it, which is marked while you are there, and the breadcrumbs say Home
- A strip at the top of the sidebar moves you between spaces. It scrolls on its own, and marks where you are
- A new instance starts with an example space that shows everything a page can do, also comments, reactions, mentions and tasks. An administrator can add it later in Administration, Settings

Upgrade impact: no database change. An instance that already has spaces gets no example by itself. To start a new instance empty, set `DIFFLUENCE_EXAMPLE_SPACE=false`.

### Added: reading and getting around (wave B)
- The search box finds pages, spaces, files and people while you type, works with the arrow keys, remembers recent searches, and can stay inside a space
- The top bar gains Recent and Favourites menus, a help menu, and a small menu on Create
- All spaces has its own screen with a filter, sorting and archived spaces. Spaces can get a star
- Recent has tabs for visited pages, pages worked on and drafts. Favourites can be reordered and removed together
- The tree has a filter, three flat views, a menu on each row, a card on hover, and keys: arrows, Home, End and letters. Rows show the emoji, a lock and a draft mark
- The sidebar can be made wider, has an Overview and shortcuts chosen by space administrators
- A page shows its read time, a list of headings beside it, links to each heading, its child pages, and a Details panel with contributors and links in and out
- Page look in the editor: emoji, full width, title position and a header image
- Images open large. Reader mode, presentation and print are there
- Deleting names what it touches, and Undo takes it back
- Preferences: light, dark or system, shortcuts, contents, cards. They follow you to other devices
- Keyboard shortcuts everywhere, with `?` for the list

Upgrade impact: migration 0006 adds columns and tables, and fills the list of links between pages once. Expand only, so the previous release keeps working.

### Added: the editor grows up (wave A)
- Type `/` to add anything at the cursor, or use Insert in the tools
- Every block has a grip to drag it and a menu: move, duplicate, delete, turn into another block. Alt with the arrows moves it
- Headings 1 to 6, subscript, superscript, 12 text colours, 6 highlights, clear formatting, alignment and indent
- Panels in five kinds, collapsible sections, 2 to 5 columns, decisions
- Status marks, dates and emoji in a line of text. Type `//` for today, and `:` and a word for an emoji
- The link dialog finds pages, and such a link survives a rename
- Images get a size, alignment, a caption and alt text
- Tables get merged cells, header rows and columns, cell colours, column widths by dragging, and sorting
- Code blocks get a language list, line numbers, wrapping and a title, and readers get a Copy button
- Find and replace with Ctrl F
- The row of tools is now one Tab stop, with arrow keys. Menus follow Home, End and letters

### Fixed
- Comparing versions no longer glues list items together

Upgrade impact: no database change. Documents gain new blocks, so while two versions run side by side, a node on the older version refuses them.

### Changed: files live in the text
- Drag files into the editor, anywhere on or beside the page, or use Add image or file. Several at once land in order
- A file shows in the text as a tile that downloads it. A page has no Upload button or Attachments block any more
- More, Attachments lists every file of a page
- Tiles survive copying a page, and export and import as Markdown
- Search finds the name of a file in a page, and comparing versions shows when a file came or went

### Fixed
- A page with little text was shown in a narrow box instead of the full reading column

### Added: the functional description
- `docs/blueprint/03-functional-requirements.md` is now a full description of the product in 13 chapters and 986 numbered requirements

Upgrade impact: no database change. Documents gain a new block, so while two versions run side by side, a node on the older version refuses a document with a file tile.

### Added: the design
- A new look: a calm white and blue design with a top bar, a sidebar per space and one reading column, in light and dark
- A new editor: a strip with Close, the save status, the summary, Discard draft and Publish, one row of tools, and an open sheet with a large title
- A page tree with visible branches. A branch can be collapsed and stays collapsed while you read on. The sidebar can be put away
- A start screen with your spaces, recent pages and favourites, a login card, and a vertical menu in the admin console
- Our own logo and icons, a More menu for less used page actions, and an account menu
- Our own confirmation and link dialogs. The browser's dialogs are gone
- The page shows who wrote it, when it was updated and which version it is
- Diffluence now uses the system font. An administrator can import a font under Administration, Settings
- The design standard in `docs/design/README.md`, enforced by `npm run design` and by approved pictures of every screen

### Fixed
- `user create-admin` failed with "permission denied" while a new server was still migrating
- Many nodes starting at once could fail while granting database rights

Upgrade impact: none. New routes only, no database change.

### Added: phases 1 and 2
- Spaces, a page tree with drag and drop, stable page addresses, breadcrumbs, favourites and recently viewed pages
- A visual editor with Markdown shortcuts, tables, task lists, highlighted code blocks and images. Autosaved private drafts, publishing with conflict protection
- Version history: view, compare with word-level differences, restore
- Attachments with versions. Uploaded files are never trusted: only real images display inline
- Full-text search over titles, content and attachment names, with filters
- A trash with one retention period, set in the admin console
- Markdown import and export for a page, a page tree or a whole space
- The full permission model: global, space and page layers, restrictions, roles and quick setups, groups, guests, "why can or can't they?", recover access for administrators
- Authenticator apps with recovery codes, passkeys, OIDC login, SCIM 2.0 provisioning
- An admin console: users, groups, spaces, audit log, settings, provisioning, system health
- Background jobs: cleanup, trash purge, daily audit verification, daily signed audit checkpoints
- Automatic contract migrations through a node registry: no manual upgrade step is left
- Separate database roles: the application cannot change or delete audit events
- Docker image with the frontend inside (44 MB, non-root, read-only), LXC install script, backup and restore scripts with a drill
- Translation framework (English)

### Added: walking skeleton
- Single `diffluence` binary with roles `all`, `api` and `worker`
- Configuration from environment variables, with `_FILE` support for secrets
- Migration runner: numbered, checksummed, expand/contract phases, safe with many nodes starting at once
- Tamper-evident audit log: append-only (enforced by the database), hash chain, verification, filtering and CSV export
- Transactional outbox, published by the worker to NATS JetStream (embedded for single-node installs)
- Local accounts with Argon2id passwords, sessions stored as hashes, idle and absolute timeouts
- Login rate limiting, cross-origin (CSRF) protection, strict security headers
- Health endpoints `/livez` and `/readyz` with graceful shutdown

Upgrade impact: none. Tested from the walking skeleton to this build with traffic: 507 requests, none failed.
