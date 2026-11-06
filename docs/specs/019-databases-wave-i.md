# 019: Wave I, databases, and PDF in the browser

Status: built, waiting for the product owner's acceptance session
Phase: wave I of `docs/blueprint/03-functional-requirements.md`, with the PdfCraft follow-up of wave E. Decisions: ADR-026 and ADR-028.

## Problem

The product owner: a database is literally a simple Excel, or better GridCraft. Today a table lives inside a page, and it has no types, no sums, no sort and no way out to a spreadsheet. People who keep a list of suppliers or a small budget need a sheet that anyone can use at once, that sits in the tree beside pages, and that obeys the same permissions, history and search.

Separately, wave E left PDF as a download because it took PdfCraft for AGPL. That was wrong.

## What the study found

**Licences, read from the files.** `storytold/pdfcraft` and `storytold/gridcraft` both hold LICENSE-MIT and LICENSE-APACHE, and both READMEs say dual-licensed under MIT or Apache-2.0. Both are allowed. The brand rule: the ArtCraft name and logos may be used only unmodified, as part of the product, and a fork must remove them. The NOTICE and ATTRIBUTION files must be kept. We ship the builds unchanged, so all of this holds.

**Size, measured on the release files.**

| Build | Compressed release | Unpacked | Largest part |
|---|---|---|---|
| PdfCraft 0.18.2 | 18.7 MiB | 52.6 MiB | WebAssembly, 52.34 MiB |
| GridCraft 0.16.1 | 8.9 MiB | 24.4 MiB | WebAssembly |

The line was 30 MB. Both pass. PdfCraft's unpacked size is above it, and we still count it as under, for a stated reason: the 18.7 MiB is what we store, ship and send, because the WebAssembly file stays gzipped from the repository to the browser. The 52 MB exists only in the memory of a person who opened a PDF. Nobody pays for either build unless they open a file in it. The program grows by about 28 MiB, within the product owner's 50 MB.

**Can they be embedded?** Yes, tried in a real browser in a new empty folder, with the builds treated as untrusted. Both start in `<iframe sandbox="allow-scripts allow-downloads">` with an opaque origin, with a policy of their own. Findings:

- Neither has a way to be handed a file. Both accept a dropped file, so our own host page takes the bytes by `postMessage` and fires `dragenter`, `dragover` and `drop` on the canvas.
- A sandboxed page has no origin, so `'self'` matches nothing: its policy must name our address, and its files need `Access-Control-Allow-Origin: *`. They are public code, so that gives nothing away.
- Browser storage is refused in the sandbox. The apps only warn.
- Saving gives a download. There is no way to get the changed file back, and GridCraft has no data interface.
- Both paint everything on a canvas (egui), so a screen reader gets nothing. Both are alpha.

**How a grid must behave.** From the WAI-ARIA grid pattern and Excel's public shortcut list:

| Source | Demand | What we do |
|---|---|---|
| APG grid | One Tab stop for the whole grid, and code moves focus inside it | Roving `tabindex` on the cells |
| APG grid | Arrows move and stop at the edges. Home, End, Ctrl+Home, Ctrl+End, Page Up and Page Down jump | All of these |
| APG grid | Enter or F2 starts editing, typing starts editing, Escape goes back and may undo | All of these. Escape throws the edit away |
| APG grid | `grid`, `row`, `columnheader`, `rowheader`, `gridcell`, `aria-sort` on the sorted header, `aria-readonly` when nothing can be edited | These roles, with a label on the grid |
| Excel | Tab goes right, Enter finishes and goes down, Shift goes the other way, Delete clears, Ctrl+Z undoes | The same, so Excel habits work |
| WCAG 2.5.7 | Nothing depends on dragging | Rows and columns move and resize through buttons and menus, not by dragging |

**CSV is not harmless.** OWASP: a cell starting with `=`, `+`, `-`, `@`, tab, carriage return or line feed, or a full-width lookalike, may run as a formula when the file opens in a spreadsheet. Quote every field and double inner quotes. OWASP warns that no way works everywhere. We quote every field and put `'` before a dangerous first character, in export. A real number such as `-5` stays as it is, or sums would break.

**What a database is elsewhere.** Public help pages of the tools studied describe rows as entries, typed columns and several views. The Atlassian page we tried to read no longer exists, so only the blueprint's own requirements stand here.

**Safety.** A cell is data from a person, and the browser shows it as text only. A link cell goes through the same `SafeHref` gate as every link. Formulas are parsed by our own small parser in the browser, with no `eval`, and can only reach other cells. The server treats a formula as text.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | PdfCraft is embedded, loaded only when someone opens a PDF | It fits and it works. It is what the product owner chose |
| D2 | The core of a database is our own grid, not GridCraft | GridCraft has no way to give its data back, so no search, versions, permissions or comparison. A canvas fails WCAG 2.2 AA (NAV-079). It is alpha. The product owner's need is a simple sheet anyone can use |
| D3 | GridCraft still joins, as Open in GridCraft, for a database and for `.xlsx` and `.csv` files | It fits at 8.9 MiB, it costs nothing unless opened, and heavy work gets a real spreadsheet. OTH-023's `.xlsx` comes from there: Save gives the file |
| D4 | A database is a page of kind `database`, with a document of exactly one `sheet` block | Tree, folders, permissions, restrictions, audit, outbox, versions, drafts, trash, move, copy and search all come for free. No second system to keep safe |
| D5 | The sheet lives in one node with `columns` and `rows` in its attributes | One gate, and version comparison and restore work as they do. Limits: 100 columns, 5000 rows, 2000 characters a cell |
| D6 | A sheet is allowed only at the top of a document, and only in a database | A page stays a page. The gate and the pages service both refuse anything else |
| D7 | Column types: text, number, date, tick, choice, link | The types a simple spreadsheet needs. Person, tag and page wait: they need pickers and a stored reference, and text covers them today |
| D8 | Formulas start with `=`: cell references, ranges, + - * / brackets, SUM, AVERAGE, MIN, MAX, COUNT, ROUND | Enough for a budget. A loop shows `#CYCLE`, a mistake `#ERROR`. Anything larger is what GridCraft is for |
| D9 | Sort and filter are a reader's view and change nothing stored | A reader may sort without edit rights |
| D10 | CSV in and out in the browser. Export needs `space.export`, and writes the computed values | The server already holds the rights and the data, and this adds no route. Formulas in a CSV would be the injection risk |
| D11 | A database cannot become another kind, or the other way round | Its document would not fit |
| D12 | Only the viewers get `'wasm-unsafe-eval'` and only `/vendor/` may be framed | The app's own policy gets nothing looser than one path in `frame-src` |

## Scope

**In:** OTH-010 (without person, tag and page columns), OTH-014, OTH-022, OTH-023, OTH-024, and the table view of OTH-011. BLK-066, in a window over the page.

**Out, and where it goes:** cards, board and saved views of OTH-011, a view inside a page (OTH-012) and an entry as a page of its own (OTH-013) stay open in the status lines. They need the product owner's first test of the sheet before they are worth their weight.

## Feature plan

- **Create.** The Create menu gets Database. It opens the sheet editor with two columns and three rows. The title works as it does for a page.
- **Reading.** A database shows its grid, read-only for those who may not edit, with sort, filter, Export CSV and Open in GridCraft.
- **Editing.** Edit opens the same grid with cells that can change, autosaved as a draft like any page, and Publish makes a version. History and compare work on the text of the sheet.
- **Columns.** Add, rename, change type, move left or right, remove. Rows: add, remove, move.
- **Paste.** A block copied from Excel fills the grid from the cell in focus, and adds rows when needed.
- **Search.** Cell text is in the search index through the document's text, and the kind filter knows Database.
- **Example space.** A small budget with formulas, a choice column and a link column.

## API

No new routes. `POST /api/v1/pages` takes `kind: "database"`. Publish and draft refuse a document that is not one sheet. `docs/api/routes.txt` does not change.

## Data

`0019_databases.sql` widens the check on `pages.kind`. It only expands, so it can run before the new code.

## Test plan

Go: every refusal of the gate first (unknown column type, double ids, unsafe link, too many rows, a sheet in a page, a page document in a database, kind changes), then search and Markdown text. Vitest: formulas, loops, CSV quoting and injection, paste. Browser: create, type, formula, publish, reload, keyboard only, read-only reader, CSV round trip, PDF and GridCraft in the sandbox. Mutations: a sheet in a normal page, an unsafe link cell, a viewer with `allow-same-origin`, WebAssembly for the whole app, a CSV without the guard.

## Acceptance criteria

1. A database can be created from Create, and sits in the tree and in folders.
2. Someone who knows Excel can type, move with the arrows and Tab, write `=SUM(B1:B3)`, and paste from Excel without help.
3. A reader without edit rights can sort and filter and cannot change a cell.
4. Export needs `space.export`, and a cell `=1+1` typed as text in a text column cannot run when the file is opened.
5. A PDF opens in PdfCraft, and code in that frame cannot read the session.

## Risks

| Risk | Answer |
|---|---|
| The builds are alpha | They are optional windows. Download and our own grid never depend on them |
| Dropping a file is not a promised way in | A pinned version does not change. A browser test notices it at each upgrade |
| A viewer parses a hostile PDF | It runs in a frame with no origin, no session and no way out but a download |
| 5000 rows in one document are slow | The document limit of 5 MiB stops it first. The grid draws rows in pages of 200 |
| Formulas in a large sheet | Each cell is worked out once per change, with a cache |

## How the product owner tests it

1. Create, Database. Type in it like Excel, with a sum at the bottom. Publish.
2. Open the example space, and look at the budget. Sort a column, filter it, export it and open the file in Excel.
3. Press Open in GridCraft.
4. Open a PDF in a page and press Open in PdfCraft.

## Provenance

Clean-room rules hold: no source of any wiki or database product was read. Only public documents, each read on 2026-11-06:

- <https://www.w3.org/WAI/ARIA/apg/patterns/grid/>: keys, roles and properties.
- <https://community.owasp.org/attacks/CSV_Injection>: dangerous characters and mitigations.
- <https://support.microsoft.com/en-us/office/keyboard-shortcuts-in-excel-1798d9d5-842a-42b8-9c99-9b7213f0040f>: keys people know, behaviour only.
- <https://github.com/storytold/pdfcraft>, <https://github.com/storytold/gridcraft>: README, licence files, NOTICE, ATTRIBUTION and the release files. These builds are used, not studied for their design.
- <https://support.atlassian.com/confluence-cloud/docs/what-are-databases/>: gave 404, so nothing from it is used.
