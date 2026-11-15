# 03.13: Other kinds of content

The study also found content types beyond pages and blog posts. None of these is decided. They are listed so that the design leaves room, and so that the product owner can say which ones matter.

## 13.1 Always-live documents

- **OTH-001** · later · A document with no publish step: edits are visible at once to everyone with access.
- **OTH-002** · later · Versions are made automatically after a pause in editing, and can be compared and restored.
- **OTH-003** · later · A reader can switch to a read-only view to avoid accidental edits.
- **OTH-004** · later · A live document can become a normal page, and back, with its history kept.
- **OTH-005** · later · It needs editing at the same time (COL-088) first.

## 13.2 Whiteboards

- **OTH-006** · later · An endless canvas for several people at once, with notes, shapes, lines, text, images, sections, a timer and voting stamps.
- **OTH-007** · later · A whiteboard is an item in the page tree, with the same permissions, comments and version history.
- **OTH-008** · later · Notes can be turned into pages or tasks.
- **OTH-009** · later · A whiteboard can be shown in a page, as a picture that stays up to date.
- **OTH-025** · later · Beyond OTH-009, the slash menu and the insert menu start a whiteboard from a page: an empty one, or one from a starting layout. It shows in the page at once and stays up to date, and every whiteboard entry in the menus carries the whiteboard's icon. (Whether such a whiteboard is an item in the tree, OTH-007, or belongs to the page needs its own study.)
- **OTH-026** · later · The starting layouts of a whiteboard include a process flow, a look back on a team's work, gathering ideas, and the direction of a product, each offered once. They are called starting layouts, because a template is a page template here.
- **OTH-027** · later · A drawing (BLK-064) can also start as a free board without the shapes of a diagram, in the same optional diagram service (ADR-026), offered in the slash menu beside the drawing.

## 13.3 Tables as data

The product owner moved this up on 2026-11-05 (ADR-028): a database is literally a simple spreadsheet, and GridCraft may do the job better. Wave I starts with its own study, which also settles how these lines fit that choice.

- **OTH-010** · partly · A database is an item in the tree: rows are entries, columns are typed fields such as text, number, date, person, tag, link and page. Still open: person, tag and page columns. Tick and choice columns are built as well.
- **OTH-011** · partly · Views show it as a table, cards or a board, each with its own filter, sort and grouping. Still open: cards, board, grouping and saved views. The table sorts and filters.
- **OTH-012** · new · A view can be shown in a page.
- **OTH-013** · new · An entry can open as a page of its own.
- **OTH-014** · built · Data can be imported from and exported to CSV.
- **OTH-022** · built · A database works like a simple spreadsheet, so anyone who knows Excel can use it at once.
- **OTH-023** · partly · The study of wave I weighs GridCraft (its web build, MIT or Apache-2.0, alpha) against our own grid. If it fits, it runs in a sandboxed frame from our own origin, and a database can be opened and saved as an `.xlsx` file. Weighed in spec 019: our own grid is the core, and GridCraft opens a database or a spreadsheet file in a frame. Still open: what is changed there comes back only as a download.
- **OTH-028** · new · Beyond OTH-012, a page can show a whole database, one entry of it, or a single value of one entry, and stays up to date with it. It is always a view of the database item, never a sheet inside the page (spec 019 D6), and a reader sees only what they may see in the database (BLK-006). (How a single value looks in a line of text, and whether cells can be changed from the page, needs its own study.)
- **OTH-029** · new · The slash menu and the insert menu offer two database entries: one creates a new database as an item of its own in the tree and shows it in the page at once, and one shows an existing database, entry or value. (Where the new database lands in the tree needs its own study.)

## 13.3a Four kinds of item

- **OTH-024** · built · Create offers Page, Blog post, Database and Folder, made the same way. A page holds text, a blog post brings news, a database holds data, and a folder structures pages and databases.

## 13.4 Calendars

- **OTH-015** · later · A space can hold team calendars with events, leave and milestones.
- **OTH-016** · later · Calendars can be imported from and subscribed to with iCalendar.
- **OTH-017** · later · A calendar can be shown on a page.

## 13.5 Presentations

- **OTH-018** · built · A page can be presented full screen (PAG-114). A separate slide format is not needed yet.

## 13.6 Issues and boards

- **OTH-019** · phase 9 · The work module: issues, boards, sprints and workflows, linked with pages, using the same permission and event model. Its own blueprint comes later.
- **OTH-030** · later · **Tracker list:** a dynamic block shows the work items of a linked tracker that match a saved filter of that tracker, or a query in the tracker's own language, as a table with chosen columns or as a count. It works with every tracker connector an administrator set up (roadmap, "Connectors for work and code tools"), also for a tracker on the organisation's own servers, and never with an address a writer types (deliberate difference 5).
- **OTH-031** · later · A tracker block shows only what the reader's own account on the tracker, or the connector's account, may see, as the administrator chose for the connector; a reader without such an account sees one muted sentence (BLK-007). Results are limited and paged (BLK-010). (Refresh, caching and what an export shows, BLK-011, need their own study.)
- **OTH-032** · later · **My tracker items:** dynamic blocks show the reader's own work in a linked tracker: unresolved items assigned to them, items they watch, items they voted for, items in progress, and their favourite filters. Each reader sees their own.
- **OTH-033** · later · **Tracker charts:** a dynamic block draws a chart or a table of figures from a filter of a linked tracker, of a chosen kind: shares by a field as a pie, also by workload; created against resolved over time; recently created; the average age of open items; the time to resolve; the average time in each state; how often items enter each state; the time to a first response; the time since a chosen date; a heat map; figures by one field or by two fields; the labels in use; and a general chart. Charts follow EDT-133 and EDT-136, with colours by name (deliberate difference 10). (Whether one block with a choice of kind replaces fifteen menu entries needs its own study.)
- **OTH-034** · later · **Tracker planning views:** dynamic blocks show a board of a linked tracker as a wall display, the work left in a sprint, the days left in a sprint, how a sprint is doing, the coming versions of chosen projects with their dates, the expected release date of one version, a calendar of work items by date, and links to common tasks in the tracker. (Whether these wait for the work module, OTH-019, or come with the connectors needs its own study.)
- **OTH-035** · later · From the slash menu and the insert menu a person creates a work item in a linked tracker without leaving the page, and a card of the new item is left at the cursor. It acts with the person's own account on the tracker, never with more rights, and the creation is audited. (The fields of the short form, and what happens when the tracker cannot be reached, need their own study.)
- **OTH-036** · later · The work module brings its own triggers for automation rules, such as a work item created and a project created, and a rule step that runs once for each work item linked to the page of the trigger. A connector to an outside tracker can offer the same through that tool's events, called only through the one outward client to allowed hosts (spec 015 D15).

## 13.7 Mobile

- **OTH-020** · built · The web app works on phones.
- **OTH-021** · phase 10 · Apps for iOS and Android, with notifications and offline reading.
- **OTH-037** · later · When the apps exist, the Security group of the administration (ADM-072) has an **App rules** section: one default rule for everyone, and further rules for chosen groups. A rule decides how long a session in an app may last, at most the 7 days of ADR-029 and never without an end, whether pages may be kept for reading offline, and whether files may be opened in other apps on the device. (What else a rule holds needs its own study.)
- **OTH-038** · later · Pages kept for reading offline are removed from the device at logout, and when a rule stops allowing them (SEC-265).
- **OTH-039** · later · The **App rules** section shows from top to bottom: a title row with the one primary action, add a rule, and a More button at its end; three tiles with counts, everyone who uses an app (each person counted once) and the people on each platform, each tile with a mark that explains how it counts; then a table of rules with the platforms covered, the rule's name as a link, who it applies to, and a row menu. The default rule is always in the table, and the table has no sort, filter or paging.
- **OTH-040** · later · The figures about apps are counts without names (PLT-002), and insights (chapter 12) gain a topic for the use of the apps.
- **OTH-041** · later · The security overview of the administration (ADM-125) gains a step for app rules, and login policies carry the longest session in an app.

## 13.8 Linked asset registers

Some teams keep their devices, licences and systems in a register in another tool, and want pages to show it.

- **OTH-042** · later · **Asset list:** a dynamic block shows the objects of a linked asset register that match a filter, such as devices, licences or systems, with chosen fields. It works only through a connector an administrator set up, and shows only what the reader may see there (OTH-031). (Which open-source registers to connect, and what an object card shows, needs its own study.)

## 13.9 Questions and answers

A team can collect questions and their answers in a space, so people find an answer before they ask again.

- **OTH-043** · later · A space can hold questions that people answer, and a dynamic block lists the questions of chosen spaces or labels in a page; answers use comments, mentions and notifications as they are. (Everything else needs its own study: whether a question is a kind of item beside the four of OTH-024, how an answer is accepted, and whether this belongs in the product at all.)
