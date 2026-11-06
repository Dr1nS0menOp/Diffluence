# 020: Wave J, what is still open in the editor, blocks and files

Status: built on branch wave-j
Phase: after waves A to H of `docs/blueprint/03-functional-requirements.md`. The product owner cannot answer while this is built, so what needs an answer goes on a list at the end.

## Problem

About 160 lines of chapters 4, 5 and 6 still say `new` or `partly`. Some of them are out of date: later waves built them and did not change the line. Others are small things a writer misses every day. The rest are large features that deserve their own wave. This spec sorts them, and builds the first two groups in the order a writer feels them.

## What the study found

**Lines that are out of date.** Full width sits in the row of tools (EDT-008), emoji and header image show at the title (EDT-010), people show in the strip (EDT-013), heading addresses exist (EDT-044), video and audio play (EDT-083), and overdue tasks show in the danger colour (EDT-051). They only need a status. Comments work in the editor, on a page that was published before (EDT-149).

**What the libraries already do.** ProseMirror keeps code as it is when pasted into a code block, and pastes plain text on Ctrl Shift V. TipTap's link mark turns an address pasted over a selection into a link, and Tab in the last cell adds a row. Browser tests prove these. Two proofs failed, and became code: Backspace after a shortcut forgot the shortcut as soon as the editor added its empty last line (`rule-undo.ts`), and changing a list to a task list lost the nesting (`setListKind`).

**What accessibility demands.**

| Source | Demand | What we do |
|---|---|---|
| Sortable table example, WAI-ARIA Authoring Practices | A sortable header holds a real `button`. `aria-sort` sits only on the sorted column. The direction icon is hidden from the name. | Reader sort follows this. |
| WCAG 2.2, 1.4.1 | Meaning not by colour alone. | A link that leads nowhere is struck through and says why. |
| WCAG 2.2, 4.1.3 | Status messages reach screen readers. | The word count and the number of filtered rows use `role="status"`. |
| WCAG 2.2, 2.5.7 | No dragging only. | Moving a file is a dialog. |

**Safety.** A link in a document names a page by id. Whether a reader may see it comes from the existing Details answer, which goes through `perm_visible_pages`. A page that is gone and a page that is hidden look the same, so nothing leaks (NAV-005). Moving a file needs *add* on the target page and *delete* on the source. Conversion to PDF is a download with the same locked headers as every file, and the helper runs without network. Blocked file types are checked on the name and on the detected type, because a name alone can lie.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | Status lines follow the code, in this wave too | The product owner reads them to see what is left |
| D2 | Typing help (`->`, `...`, quotes, `:)`) is one preference, on by default | EDT-029 and 030 ask for one setting |
| D3 | Recently used entries come first in the `/` menu, kept per person in the browser | It is a private convenience, and SEC-265 asks for keys per person |
| D4 | A link to what a reader cannot reach is struck through, with one sentence that is the same for hidden and gone | EDT-059, 060 and NAV-005 together |
| D5 | Sorting and filtering by readers change nothing stored | EDT-094, 103 |
| D6 | Pasted Markdown is converted by the server's own `FromMarkdown` | One converter, already behind the `Normalize` gate, and no client dependency |
| D7 | Office preview uses the LibreOffice route of the optional helper, and gives a PDF download | The product owner's choice at installation (ADR-026). Without a helper the file stays a download |
| D8 | EDT-006 is dropped | ADR-026 |

## Scope

**Build, most felt first.**

| Part | Lines |
|---|---|
| J1 Writing flow | EDT-009, 011, 014, 017, 029, 030, 047, 145, 147, 151 |
| J2 Paste and links | EDT-059, 060, 140 |
| J3 Images and files in the text | EDT-071, 081, FIL-015, 025, 029 |
| J4 Tables for readers | EDT-094, 099, 103 |
| J5 Files | FIL-020 (types), 034, 039 (moving), 040, BLK-067 (as a PDF) |
| J6 Drawings | BLK-064 |

**Prove, and only build if the proof fails.** EDT-028, 052, 062, 089, 113, 139.

**Only the status changes.** EDT-008, 010, 013, 044, 051, 083, and EDT-149 as partly.

**Left for a later wave, with the reason.**

| Lines | Why not now |
|---|---|
| EDT-012, 019, 024, 026, 042, 048, 050, 058, 063 to 066, 068, 078, 079, 119, 120, 123, 129, 143, 148, 150, 157, 158 | Each is a feature with its own format or interaction. None blocks daily writing. EDT-150 and 157 need measuring first |
| EDT-020, 022, 023, 031, 056, 057, 072 to 077, 080, 086 to 088, 091, 093, 098, 100 to 102, 106, 107, 111, 112, 115, 116, 118, 122, 125, 127, 138 | The main part works. What is missing is a further option, and each needs a document attribute and a test |
| EDT-130 to 136 | Charts were built in wave E. Options that remain are listed in chapter 5 |
| BLK-001, 014, 017 to 038, 042, 046, 051 to 062, 065, 069 | Further dynamic blocks. Lists that wait on nothing can follow in one small wave |
| BLK-066 | The PDF viewer belongs to wave I |
| FIL-011, 013, 014, 017, 026, 027, 028, 036, 041, 042, 043, 045, 050, and the limit per space of FIL-020 | Further options around files. FIL-027 waits for the viewer of wave I |

## Feature plan

### J1. Writing flow

- **Title:** a new page opens with the focus in the title. Enter moves to the body. Backspace at the very start of the body moves back to the title. Works with the keyboard alone.
- **Below the text:** clicking the empty sheet below the last block puts the cursor at the end, and adds a paragraph when the last block is not text.
- **Word count:** a quiet line at the foot of the sheet, on request from a tool, with words and read time. With a selection it counts the selection.
- **The `/` menu:** group headings, and what a person used lately comes first.
- **Typing help:** `->` to an arrow, `<-` to an arrow, `...` to an ellipsis, straight quotes to curly ones, and `:)` `:(` `;)` `:D` to emoji. Never in code. Backspace right after it restores what was typed. A preference turns all of it off.
- **Lists:** bullets and numbers change by level.
- **Find:** a Whole word option.
- **Spell check:** the browser's own, on in the body.

### J2. Paste and links

- **Markdown:** plain text that clearly is Markdown (a heading, a list, a code fence, a link) turns into blocks, through the server. A failed call keeps the plain text. Undo restores it.
- **Links that lead nowhere:** for a reader, a link to a page that is gone or hidden is struck through, with the sentence "This page does not exist, or you cannot see it."

### J3. Images and files in the text

- **Picker:** the Insert list has **File on this page**, which lists what the page already holds, and adds it without a new upload.
- **Missing image:** a clear placeholder instead of a broken picture.
- **Tile:** shows the type and size. A deleted file says so, and is no link.

### J4. Tables for readers

- A table with a header row gets sort buttons in its header cells, and a filter box above it when it has more than five rows.
- A long table keeps its header in view.
- Nothing is stored.

### J5. Files

- **Move and copy:** a file moves or copies to another page, in the Files screen and the file dialog, by a page search. Tiles keep working after a move, because the id stays.
- **Blocked types:** an administrator lists file extensions that are refused, checked at upload.
- **The list of a page:** sort by name, date, size and type, and filter by name.
- **Office preview:** with a helper connected, Word, Excel and PowerPoint files offer **Open as PDF**.

### J6. Drawings

A saved drawing opens again in draw.io from its tools. Saving makes a new version of the same file, so the page keeps showing it.

## API

| Route | Rule | What |
|---|---|---|
| `POST /markdown/document` | authenticated | Markdown to a checked document |
| `POST /attachments/{id}/move` | `attachment.delete` | Moves a file to another page |
| `POST /attachments/{id}/copy` | `page.view` | Copies a file to another page |
| `GET /attachments/{id}/preview.pdf` | `page.view` | An office file as PDF |
| `POST /pages/reachable` | authenticated | Which of these pages the caller can open |

## Data

No migration: blocked types live in `settings`, under `blocked_file_types`, and preferences gain `typing_help`. Number 0020 stays free.

## Build order

J1, J2, J3, J4, J5, J6, committing each when green.

## Test plan

Go integration tests for each route, with people who may and may not. Vitest for the typing rules, Markdown detection and table sort. Browser tests per part, keyboard first. Mutation lines for move, copy, blocked types and preview. Pictures of what changes.

## Acceptance criteria

- [x] AC-1 Enter in the title moves to the body, and Backspace at the start moves back.
- [x] AC-2 Clicking below the text places the cursor at the end.
- [x] AC-3 The word count follows the selection.
- [x] AC-4 Typing help works, stays out of code, and a preference turns it off.
- [x] AC-5 Recently used entries come first in the `/` menu.
- [x] AC-6 Pasted Markdown becomes blocks, and Undo restores the text.
- [x] AC-7 A link to a hidden or deleted page looks the same, and leaks nothing.
- [x] AC-8 A file on the page can be added again without an upload.
- [x] AC-9 A missing image and a missing file say so.
- [x] AC-10 Readers can sort and filter a table, and nothing changes in the page.
- [x] AC-11 A file moves and copies, only for those who may, and tiles keep working.
- [x] AC-12 A blocked type is refused, whatever its name says in capitals.
- [x] AC-13 An office file opens as a PDF when a helper is there.
- [x] AC-14 A saved drawing can be changed.

## Risks

| Risk | Answer |
|---|---|
| Typing help surprises a writer | Backspace restores it, and it can be turned off |
| Markdown detection takes plain text for Markdown | It asks for clear signs, and Undo restores the text |
| Sorting a read-only document | The change stays in that view of the page and is never sent |

## How the product owner tests it

1. Make a page, type a title, press Enter, type, press Backspace at the start of the body.
2. Type `->` and `:)`. Turn it off in Preferences.
3. Paste a few lines of Markdown.
4. Link to a page, delete that page, and read the first page.
5. Sort and filter a long table as a reader.
6. Move a file to another page, and look at its tile.
7. Set a blocked type in Administration, and try to add such a file.

## For the product owner to decide

- No dependency was added, and none is asked for.
- Which office service to run. Collabora needs `DIFFLUENCE_COLLABORA_URL`, the other choice is the `pdf` profile.
- Whether the rest of chapter 5, the further dynamic blocks, is worth a small wave of its own.

## Provenance

Own text. Read for this spec: the sortable table example of the WAI-ARIA Authoring Practices, WCAG 2.2. Everything else comes from the study behind blueprint 03 and specs 008, 009 and 012. The address of Gotenberg's documentation that was tried did not answer. The routes for office files, `/forms/libreoffice/convert` and Collabora's `/cool/convert-to/pdf`, and the `xmlpng` field of draw.io, come from what I know of those products, and were only tested against fakes. They need one try against the real services.
