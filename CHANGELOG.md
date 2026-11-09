# Changelog

Plain-language notes per release. Every entry has an "Upgrade impact" line.

## Unreleased
### Added: a note when a newer version runs (spec 041)
- A tab that was opened before an update says so under the top bar, and offers Reload. It never reloads by itself and takes no focus, so typing goes on
- Reload first saves the draft of an open editor, so the text is simply there afterwards
- Comments now ask the page whether they are closed, instead of reading it themselves. Nothing changes for people
- Upgrade impact: none
### Security: the rest of blueprint 06 (spec 042)
- Editing together: what a client sends past the editor's rules leaves the shared draft at once, and the name in a colleague's cursor always comes from their login
- A mail goes out when a password, second factor or passkey changes, also when an administrator makes the change
- Reset and sign-up mail to one address is limited, wherever the requests come from. Turning off an authenticator app shares the limit on password tries
- A warning at start and in the admin console when the public address is plain http to others. The login cookie for an identity provider carries the `__Secure-` prefix on https
- A SCIM filter on an attribute Diffluence does not read is refused instead of ignored
- More of the readers are fuzzed every night: HTML import, archives, SCIM filters and the search language
- Upgrade impact: **Docker Compose now publishes the app on `127.0.0.1` only.** If your reverse proxy runs on another machine, set `DIFFLUENCE_BIND=0.0.0.0` before you update. No migration

### Added: a setup screen for a new install (spec 038)
- A new install opens on a setup screen: the name of the company or person it is for, which the search box shows as "Search in" that name, the first administrator and the default time zone. Right after, it asks for a passkey
- The screen needs a one-time code from the server's log, or from `diffluence setup-code`. A code lasts an hour, works once and stops after 10 wrong tries. Once an administrator exists, the screen is gone for good
- Folded groups in the settings sidebar are kept per person, on every device
- The results page has no space list of its own any more: choose spaces in the search panel
- Settings and the administration start right beside the sidebar and stay inside the window on a laptop. A table that is too wide scrolls sideways in its own box, also with the keyboard
- Upgrade impact: one migration. An existing install has an administrator, so it never shows the setup screen

### Changed: a faster mutation check (spec 037)
- The check that breaks security rules on purpose runs several mutations at once, and the slowest ones run only the tests that catch them. A full run takes about a third of the time, and catches the same mutations with the same tests
- Builders may run only what their change can affect. Main and CI always run every mutation
- Upgrade impact: none

### Added: roles you make yourself (spec 036)
- Access is given as roles: named sets of permissions, for a space or for the whole instance. Administrators make and change them; people and groups hold them, and what they may do follows a role when it changes
- Viewer, Editor, Manager and Space admin are built-in roles with the meaning they always had, and cannot be changed or deleted
- A role in use cannot be deleted, cannot gain space administration, and changes only after a preview that says who it reaches. Every change to a role or to who holds it is audited, with before and after
- Who may give access does not change: in a space whoever manages access there, for the instance administrators, and only a space administrator makes another one
- The routes for single permissions keep working: a set becomes the role with exactly that set. Own permission sets became roles under their own names
- Four permissions of their own, taken out of space administration: publishing to the web (public sites and public links), automation (rules and webhooks), templates, and reading the audit of a space. Whoever administered a space holds them too, so a role can now give one without the rest
- Every permission has a plain name and one line on what it allows, in groups: pages, comments, files, people and access, space, administration. No key such as create_personal_space shows on a screen any more
- Administration > Roles makes and changes roles with those groups of checkboxes, and shows who holds each role and in which space. A space's settings give roles under People and roles. The list of who holds which instance permission stays, in words. The old box for named permission sets is gone: those sets are roles now
- Space settings show each part to whoever holds the permission it needs, so someone who only publishes or only runs automation still finds it
- Bundles are format 2. A bundle of format 1 still comes in, and its space administrators keep what the split took out of administering
- Upgrade impact: one migration, which gives the four new permissions to everyone who administered a space and turns every existing permission into roles, without changing what anyone may do. The application's database role can no longer write permissions itself; only the database's own role triggers can

### Security: open lines of blueprints 05 and 06 (spec 035)
- The PDF helper can ask for a password, `DIFFLUENCE_PDF_PASSWORD`, and then answers only Diffluence. In `compose.yaml` it also fetches no address, runs no script in pages, runs read-only without extra rights, and is pinned by digest
- Passwords are compared in one Unicode form, so the same password typed on two systems always works. A new password may not hold the product's name
- Characters that turn reading order around are removed from page titles, file names, names of people and names of spaces
- The database refuses to delete a parent page on its own, so a child can never lose a restriction it inherits
- `diffluence session end --email ADDR` or `--all` logs one person or everyone out, without disabling anyone. `docs/operations/incidents.md` says what to do when something went wrong
- Asking again through the identity provider now checks that the provider really asked
- Blueprints 05 and 06 show what is built, checked against the code and the tests
- Upgrade impact: one migration. To protect the PDF helper, set a long `DIFFLUENCE_PDF_PASSWORD` in `.env` and restart both. Without it, everything works as before
### Changed: a quieter look, and settings in the sidebar (spec 033, ADR-034)
- The look is calmer: fewer lines and boxes, tables as rows, labels as plain links, and no guide lines in the page tree
- The sidebar shows on the home page and the other personal screens too. Its parts fold and the space part can be made taller; both follow you to other devices
- The page tree follows the links of a space directly, starts folded from the first level, and keeps its tools and filter in a quiet row of icons
- The search box says Search in the name of your organisation, given with `diffluence user create-admin --organisation` or under Administration, Look
- The search box always searches every space. Clicking it unfolds filters for spaces, type, labels, people and date, which write the same words a query can hold, such as space:ENG; the results page shows and keeps them
- Space settings and the administration show their sections in the sidebar, in groups that fold, with a way to jump to one. Care and System health sit under System
- The People screen of a space shows pictures, guests, how someone got in, and a role that those who manage access change in the row
- The user list filters by role and by space
- A rule of the instance can act in chosen spaces only
- Upgrade impact: none. Links to space settings still work and open the first section
### Security: master.key can be replaced (spec 034)
- `diffluence key add`, `promote`, `reseal`, `check` and `retire` replace `master.key` while Diffluence keeps running. Every stored file and secret is sealed again with the new key, and the old key is removed only when a check proves nothing needs it
- `diffluence key check` proves that every stored file and secret opens, and names what does not, without showing its content. `--deep` reads every byte
- Backups take a key on its way in or out along, and a restore now ends with `key check`. The restore drill replaces the key halfway through
- The job that encrypts old files remembers where it stopped, so a store of millions of files gets through
- Upgrade impact: one migration. New secrets are sealed in a new layout that names their key; older ones still open. Nothing to do
### Changed: an order of work when the database is full (spec 031)
- Saving comes first, then reading, then search and reports. Each has its own share of the database connections, so a flood of searches or page views cannot take the room a save needs
- When a share is full and too many wait, the answer comes at once: 503 with `Retry-After`, and the screen says Diffluence is very busy. Text being written stays in the browser
- `/readyz` says `busy` when every connection is taken or a queue is long, so a load balancer can send people elsewhere. `/livez` does not change
- Each node hashes at most half as many passwords at once as it has cores, so a rush of logins leaves the rest alone
- A page view reuses the permission facts it loaded: opening a page costs 25 queries instead of 30, a space 11 instead of 36
- The main screens have a query budget that a test enforces, and `/metrics` counts queries as `diffluence_db_queries_total`
- Upgrade impact: none. A load balancer that checks `/readyz` now also takes a busy node out for a moment
### Security: stored files are encrypted (spec 032)
- Every stored file is encrypted with its own key, which `master.key` protects. A wrong key, a changed file or a file cut short is refused, and nothing of it is served
- Files from before this release are encrypted by a job in the background, a little at a time. Until it is done they are still read; after that, a plain file is refused
- Upgrade impact: one migration. Keep `master.key` safe and apart from the data: without it, no file can be read

### Changed: smaller parts of blueprint 04 (spec 032)
- The PDF of an office file is made once per version and kept, instead of on every view, and copies of the same file share it
- The feed and notifications page by the time and id of the last item, so two items with the same time are never skipped. `before` alone still works
- `GET /api/v1/system/capabilities` says which optional parts are on
- Upgrade impact: none
### Added: smaller things for the people who keep an instance tidy (spec 030)
- The lock beside a page title is open or closed, so a restricted page shows at a glance
- The spaces in the administration filter by name, pages, storage and months without a change
- The Content screen of a space filters by the day of the last change and of creation
- The user list names the spaces of each guest, and a person's screen shows their groups, spaces and how many pages they own, without titles
- The administration warns when the disk for files has less than a tenth free
- Upgrade impact: none
### Changed: word forms and file names in search (spec 029)
- A word also finds its common forms: `plans` finds `plan`, `policy` finds `policies`, in English and Dutch alike. Words in quotes are found as typed, and numbers have no other forms
- The Files screen finds a name the way the search box does: whole words and the start of a word, so `rep` finds `report.pdf` and `port` does not
- The install guide says how large the database must be: about 2.5 CPUs for 5,000 people
- A new test holds that a good login never buys extra password guesses from the same address
- Upgrade impact: none. A search may find more pages than before, because of the word forms

### Changed: search that carries the load, and a tree that loads by branch (spec 028)
- A search no longer reads every match. A common word in 50,000 pages went from seconds to tens of milliseconds, also with reading and publishing going on
- The order is new: the title as typed, every word in the title, then the rest, each with the newest first. Nothing is left out, however old
- The search box stands in the middle of the top bar
- Results go on past the total, which now says "More than" sooner. A plain word also finds labels, and a word that only a comment holds gets its own section
- A search that runs out of time says so, instead of showing nothing. When search is very busy it says that too, and reading and saving keep their connections
- In the box, files are found by the words of their name, so `port` no longer finds `report.pdf` there. The Files screen still looks inside names
- A large tree opens at once: it loads the start, the way to the open page, and branches as they open. The filter and the flat views ask the server
- Upgrade impact: one migration, about 16 seconds for 50,000 pages on a small machine, and no other step


### Added: the rest of the editor (wave Q)
- Tools that do not fit fold into More, so a narrow window no longer covers the first line of the page
- A plus on an empty line, Browse all with previews, and Turn into with the likely choices first. Panels and sections go back to text
- Copy link to any block with text. Shift and a grip select several blocks, which then move, duplicate and go together. A list item drags with what is nested in it, and dropping a block at the edge of another makes columns
- Saved blocks, private unless shared, in the `/` menu
- Images: a corner to drag, a width, text around them, a border, a link, and a small image in a line of text. The editor asks for a description and says how many are missing
- Images in pasted web pages are taken along when their bytes are in the clipboard. Otherwise a note says how many stayed behind
- Code blocks: a first line number, folded for readers, language detection, and Tab on several lines
- A panel with your own emoji, a spotlight band, a narrow sidebar split, and wider columns
- An emoji picker with groups and recent emoji
- Charts: axis and data columns, totals of repeated values, titles, legend, height, smooth lines and data points. Removing a table asks about its chart
- The language of a page and the direction of a line
- Lists from Word arrive as lists, and a block from a newer release is kept and shown as a note
- Upgrade impact: one migration. Nobody needs to do anything

### Added: the rest of dynamic blocks (wave O)
- Lists and reports filter by text, type, person, date and space. The report table now really shows its columns, and a writer chooses them
- New blocks: activity with tabs, popular pages, popular, related and recent labels, section contents, contribution table, person card, people list, space files, rotating cards, a button that starts a page from a template, a timeline, and a synced block
- Link cards can come from a filter and show a picture. The page tree opens and closes, and the index and labels list have a letter bar
- Reused text can have blanks that each page fills in, as `{{name}}`
- Settings of a block stand at the side, so the block stays in view
- Who watches a page shows in a block only to those who manage the space, and a comment for editors never counts
- Upgrade impact: one migration. Tasks that exist get their last change as their creation date

### Changed: small points of the UI test
- The public link dialog has a copy button. The administration shows types, sources and permissions in words, and the company home page is chosen by title
- A page with one version offers nothing to compare, the restore note shows a real time, and the import report counts correctly
- The title field grows with a long title
- Upgrade impact: none

### Added: links that follow their page, and tables with handles (wave M)
- A link to a page without your own words shows the current title, as text, a tag or a card. The document holds only the id, so a reader who may not see the page gets neutral words
- A link can lead to a heading or an anchor, here or on another page, and the link dialog starts with recent pages
- A selected link has its tools in the row: open, copy, edit, remove, and how it shows
- A link to a page that does not exist yet starts that page below it. Space settings list the ones nobody has started
- A new table asks for its size. The row and column in use have a handle with a menu and a plus, and can be dragged or moved from the menu
- Tables can number their rows, scale with the page, and have a caption. Text with tabs lands as cells
- A table cannot hold another table any more. On saving, a table inside a table loses its inner grid and keeps its words
- Upgrade impact: one migration
### Added: the rest of files (wave N)
- A large upload shows how far it is and can be stopped. A file dropped on a tile becomes its next version
- A preview opens on top of the page, with colours for code and next and previous. Tiles can be small or large
- The list of a page says who added a file and whether the page shows it, and offers to clear up what nothing uses
- Files can have labels, and search and the Files screen filter on them. The Files screen also filters by type and person, and space administrators can delete many files at once
- Each version of a file has its own address, which also makes old versions downloadable for the first time. The address gives no access
- The page history shows when files came and went, from this release on
- An administrator can give a space a storage limit
- Fixed: renaming a file no longer wipes its comment
- Upgrade impact: one migration, which reads all page versions once, so it takes longer on a large instance. Nothing else to do. No space gets a limit by itself
### Security: what a test against the architecture found (spec 025)
- Pasted Markdown can no longer keep the server busy for minutes. A paste is 256 KiB at most, and a block too large to read stays plain text with a warning
- Wrong passwords no longer lock a person out from everywhere: the limit counts per account and address, with a wider limit per account
- One event too large for the queue no longer stops all others
- The editing service takes messages of 16 MiB at most, and goes back to the last good state past that
- In `compose.yaml` the PDF helper and the editing service sit on an internal network, without a way to the database or the internet
- `docs/operations/limits.md` lists what was measured, and what was not
- Upgrade impact: if you added your own helper to `compose.yaml`, put it on the `helpers` network

### Added: inviting someone without an account (COL-047)
- A person with the new global permission `invite_people` can ask for an invitation from the Share dialog. An administrator approves or refuses each one under Invitations, with a reason, and the person who asked hears the answer
- The recipient gets an invitation and never the content, and becomes a guest with view on that one space
### Changed: space managers no longer invite guests by themselves
- Inviting a guest from a space now needs `invite_people` and waits for an administrator, unless you are an administrator (ADR-033)
- Upgrade impact: one migration. Nobody holds `invite_people` yet, so give it to the people or groups who invite guests today

### Changed: backups are encrypted (wave K)
- `scripts/backup.sh` and `scripts/restore.sh` now take a key file, and all of a backup is encrypted with AES-256. The script refuses to run without a key, or with the key inside the backup folder
- Upgrade impact: make a key as `docs/operations/backup-restore.md` says, and change your cron line. A backup is run with `bash`, and needs `gpg`. Older backups are restored with the older script

### Added: the rest of the chapters (wave K)
- A profile lists where a person was mentioned, their spaces, and their followers, which they can hide in Preferences
- The owner of a page hears when an administrator recovers it, and a draft gets a notification two weeks before it goes
- The console lists restricted pages nobody can open, totals per space, the largest pages, and events that could not be handled, with Try again
- After a first publish, a message offers to share the page
- Upgrade impact: one migration. Presence restarts empty, which nobody notices

### Added: watching an instance (wave K)
- `/metrics` for Prometheus, behind `DIFFLUENCE_METRICS_TOKEN`. `trace_id` in the logs from a W3C `traceparent` header
- A node with role `api` stays ready when the queue is down
- An event that keeps failing stops after six tries instead of coming back for ever
- A worker runs several jobs at once, so a large export no longer holds up a small copy
- Pictures and files are no longer downloaded again on each view, while access is still checked each time
- Reading a page writes less to the database. The pool holds up to 20 connections unless the address says otherwise
- Upgrade impact: nothing to do. Check that all nodes together stay below `max_connections`
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
