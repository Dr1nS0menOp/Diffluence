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

## 13.3 Tables as data

The product owner moved this up on 2026-11-05 (ADR-028): a database is literally a simple spreadsheet, and GridCraft may do the job better. Wave I starts with its own study, which also settles how these lines fit that choice.

- **OTH-010** · new · A database is an item in the tree: rows are entries, columns are typed fields such as text, number, date, person, tag, link and page.
- **OTH-011** · new · Views show it as a table, cards or a board, each with its own filter, sort and grouping.
- **OTH-012** · new · A view can be shown in a page.
- **OTH-013** · new · An entry can open as a page of its own.
- **OTH-014** · new · Data can be imported from and exported to CSV.
- **OTH-022** · new · A database works like a simple spreadsheet, so anyone who knows Excel can use it at once.
- **OTH-023** · new · The study of wave I weighs GridCraft (its web build, MIT or Apache-2.0, alpha) against our own grid. If it fits, it runs in a sandboxed frame from our own origin, and a database can be opened and saved as an `.xlsx` file.

## 13.3a Four kinds of item

- **OTH-024** · new · Create offers Page, Blog post, Database and Folder, made the same way. A page holds text, a blog post brings news, a database holds data, and a folder structures pages and databases.

## 13.4 Calendars

- **OTH-015** · later · A space can hold team calendars with events, leave and milestones.
- **OTH-016** · later · Calendars can be imported from and subscribed to with iCalendar.
- **OTH-017** · later · A calendar can be shown on a page.

## 13.5 Presentations

- **OTH-018** · later · A page can be presented full screen (PAG-114). A separate slide format is not needed yet.

## 13.6 Issues and boards

- **OTH-019** · phase 9 · The work module: issues, boards, sprints and workflows, linked with pages, using the same permission and event model. Its own blueprint comes later.

## 13.7 Mobile

- **OTH-020** · built · The web app works on phones.
- **OTH-021** · phase 10 · Apps for iOS and Android, with notifications and offline reading.
