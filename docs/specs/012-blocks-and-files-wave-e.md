# 012: Wave E, blocks that show things, and files

Status: planned in detail, not built yet
Phase: wave E of `docs/blueprint/03-functional-requirements.md`. The product owner asked on 2026-11-05 to build the whole of blueprint 03, one commit per wave. This wave comes after waves C (working together) and D (organising), and uses what they add. Each dependency is named per block below.

## Problem

Pages can hold text, but they cannot yet show anything that changes by itself. A team cannot list the pages below a page, collect facts from twenty pages into one table, reuse a paragraph in many places, or turn a table into a chart. Files are tiles with a name, and little more. Chapter 4 still has a long tail of editor work that wave A left out on purpose. This spec is the plan for all three: chapter 5, chapter 6, and what is still partly or new in chapter 4.

## What the study found

**How writers expect blocks to work.** Wikis agree on the shape. A block is inserted from the same menu as text, its settings sit beside it, and it shows real content while editing. A report pairs with a small table of facts on each page: a label and an optional id link the two, and leaving the id out takes all facts. Values in one filter field mean *or*, separate fields mean *and*, and a minus sign excludes. An empty report nearly always means the filters are too narrow, so an empty block must say what it looked for. Reports have a hard limit on rows, set by an administrator.

**What accessibility demands.** These come from WCAG 2.2 and the WAI-ARIA Authoring Practices, and change the design:

| Source | Demand | What we do |
|---|---|---|
| Complex images tutorial | A chart needs a short description and a long one with its values. A table belongs in structured content, not in `aria-describedby`, which reads as one paragraph. | A chart is a `figure` with a caption, a named picture and a real table beside it. |
| Sortable table example | Header text sits in a native `button` in the `th`, `aria-sort` sits on the sorted `th` only, arrows are hidden from screen readers, and instructions go in the caption. | Reader sorting and every report table work this way. No custom keys are needed. |
| 1.4.1 Use of colour | Series may not differ by colour alone. | Lines have their own marker shape and dash, and bars and slices have their own pattern. |
| 1.4.11 Non-text contrast | Bars, lines and points need 3:1. | Chart colours join the contrast pairs of the design check, in both themes. |
| 1.4.10 Reflow | A data table, diagram or map may scroll in two directions, but only inside its own frame. What is around it must reflow. | Wide tables, reports, charts and drawings each scroll in their own frame. |
| Carousel pattern | A native button stops and starts rotation and comes first. Focus inside stops rotation until the person restarts it. Hover stops it too. `aria-live` is off while rotating and polite when stopped. | Rotating cards do exactly this. |
| 2.2.2 Pause, stop, hide | Motion over five seconds needs a control, and pausing only while focus stays does not count. | The stop button is always there, and rotation is off by default. |
| 2.5.7 Dragging | Everything dragged has another way. | Image width has a field, rows and columns have Move in their menu, and a drawing has arrow keys. |
| 2.1.1 Keyboard | A drawing surface must work without a pointer. | Shapes are a list with buttons, and arrows move the selected shape. |
| 4.1.3 Status messages | Upload progress and sort changes reach screen readers without moving focus. | `role="status"`, updated in steps of ten percent. |

**What the web platform allows.** The policy of the web app is `default-src 'none'` with named exceptions, and Trusted Types. Media needs `media-src 'self'`. A PDF in a frame needs `frame-src`, and the MDN `iframe` page warns that a sandbox can block the browser's own PDF viewer, so it is no reliable way to show one. Giving a frame both `allow-scripts` and `allow-same-origin` for our own origin is the same as no sandbox. The sniffing standard detects MP4, WebM, Ogg, MP3 and WAV from content, and counts `application/pdf` as a scriptable type. So media can join the short list of what we show, and PDF does not.

**What images demand on the server.** Go's `image` package warns that decoding can use memory based on the header before the pixels are checked, and that untrusted files need `DecodeConfig` first. WebP is not in the standard library. Scaling needs code or `golang.org/x/image`. Not decoding at all is safer, and browsers scale images well.

**What libraries would bring.** See "For the product owner to decide". Mermaid has a `strict` level, which encodes HTML and turns off clicks, and a `sandbox` level. PDF.js ships a large worker file next to the page. Neither says how it behaves under Trusted Types and our style policy, so both need a proof before they are approved.

**Safety.** A dynamic block runs as the reader, so a saved block can never show more than a search by that reader. Settings are data from a short list, and a document never stores results. Reusing text is the risky part: a page that includes another page must check view rights at every level, and must stop on a loop. A link must never tell a reader whether a page they may not see exists.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | One document node `dynamic` with `kind` and `settings`. Settings pass an allow-list per kind in `internal/document/dynamic.go` | One gate, one renderer, one menu. A new kind is a table entry, not a new node. |
| D2 | A document stores settings, never results | Each reader gets their own answer, and no title can be frozen into a version. |
| D3 | One route answers every block, as the reader, and the module decides | Every kind goes through `perm_visible_pages`, so BLK-006 holds by construction. |
| D4 | Blocks get a short `block_id` only when something needs it | Copy link, charts and anchors need a stable name. Ids on every block would bloat documents and every comparison. |
| D5 | Tools for a selection stay in the row. No bars float at the selection | ADR-021 and the design standard say so. BLK-003, EDT-063 and EDT-080 are met by the row, and their words change. EDT-006 needs the product owner (see below). |
| D6 | A chart is its own block that points at a table by `block_id` | The table stays a normal table, and removing either one is simple. |
| D7 | Charts are our own drawing, with no chart library | Four kinds are a small job, and a library would bring its own colours and a large dependency. It does need one rule changed (see below). |
| D8 | A hidden link target and a deleted one look the same, unless the reader could have seen it | NAV-005 comes first. Only someone who could see the page gets the struck-through line. |
| D9 | `pageTag` and `pageCard` hold a page id and no title | EDT-066 asks for a per-reader title, and a stored title would leak through a version. |
| D10 | An include is expanded on the server, up to 5 levels, and rights are checked at each level | The client cannot skip a check, and a loop stops in one place. |
| D11 | Text in an include stays at its source for search | BLK-049. The include holds no text. |
| D12 | Page facts, decisions, tasks and includes are indexed at publish, in that transaction | Reports stay fast, and never run ahead of a published version. |
| D13 | Hiding page facts is for tidiness only, and the settings say so | The values are still in the document and in reports. |
| D14 | No server thumbnails. The large tile of an image uses the image, and other files keep the small tile | The server never decodes an untrusted file. |
| D15 | Text previews come through a JSON route as plain text, up to 1 MiB | No file is ever shown as a document in the browser. Colours come from `lowlight`, which is there already. |
| D16 | Video and audio show from a short list of detected types, with the same locked-down headers | EDT-083. PDF stays out, as the standard calls it scriptable. |
| D17 | Reader sorting and filtering change the reader's own view of the table, and nothing is sent | The read-only editor can take a local change. Nothing reaches a version. |
| D18 | Patterns in block settings use `*`, not regular expressions | A setting from a writer must not freeze a reader's browser. |
| D19 | Typographic replacement and text faces are ours, a few input rules behind one preference | Too small for a package. |
| D20 | A drawing and a timeline are stored as data in the document, never as SVG | The allow-list stays simple, and no document can carry markup. |
| D21 | Rotating cards do not rotate unless the writer turns it on | 2.2.2 is easiest to meet by not moving. |
| D22 | Multi-block selection works on top-level blocks only | The same limit as the handle (spec 008, D7). |

## Scope

**In, chapter 5:** BLK-001 to 011, 013 to 025, 027 to 065. **Chapter 6:** every line that is new, except those below. **Chapter 4:** EDT-007 to 011, 014, 017, 019, 020, 022 to 024, 026, 028 to 030, 042, 044, 047, 048, 050 to 052, 056 to 066, 068, 071 to 081, 083, 086 to 089, 091, 093, 094, 097 to 103, 105 to 107, 111 to 113, 115, 116, 118 to 120, 122, 123, 125, 127, 129 to 136, 138 to 140, 143, 145, 147, 148, 150, 151, 157, 158.

**Out, and where it goes:**

| Item | Goes to |
|---|---|
| Popular pages (BLK-026) | Wave H, with analytics |
| Calendar block (BLK-069) | Wave I, chapter 13 |
| Apps adding blocks (BLK-012), external embed (BLK-068) | Later, as the blueprint says. External embed needs a change to the policy of the app |
| Diagram (BLK-063), PDF viewer (BLK-066), PDF in previews | Waiting for the product owner: new dependencies |
| Office preview (BLK-067), office formats in previews (FIL-027) | Waiting for the product owner: a converter |
| Floating bar with hidden tools (EDT-006) | Waiting for the product owner: a change of look |
| Who else is editing (EDT-013), comment while editing (EDT-149) | Editing together, and wave C |
| Status above the title (EDT-012) | Wave D |
| Virus scanning (FIL-021), WebDAV (FIL-051), cropping (EDT-082), own emoji (EDT-128), outside link cards (EDT-067) | Later, as the blueprint says |

## Feature plan

### E1. How every dynamic block behaves

- **Inserting:** from the `/` menu and Insert, in new groups. A settings panel opens beside the sheet, in the look of the Details panel.
- **Settings:** a form built from a table per kind in `components/editor/dynamic/kinds.ts`. A change shows at once, with no save button. A change waits 300 ms for typing, and only the newest answer counts.
- **Selecting:** the block gets a selection outline, and Edit settings, Width, Duplicate and Delete join the row of tools (D5). The block handle works as for any block.
- **Width:** normal, wide or full, for blocks that have a table or cards. The width follows the rules of full-width pages.
- **While editing:** the block shows its real content, with links that do nothing. It is an atom, so the cursor goes around it.
- **Reading:** a block fetches when it comes near the screen, so the text never waits. While loading it holds a small `Loading` line.
- **Empty:** one muted sentence that names the filters, or nothing when the writer chose *Hide when empty*.
- **Failing:** a short message in place, with Try again. An error boundary keeps the rest of the page alive.
- **Limits:** every kind has a default and a maximum, enforced on the server. A list has Show more.
- **Keyboard:** Enter on a selected block opens its settings, Escape closes them and returns to the block.
- **Accessibility:** a block is a `section` with a name when it has a heading. Lists are lists, and tables have headers. Panel fields are `Field`s.
- **Export:** Markdown writes a note with the kind and a link to the page. PDF, Word and HTML wait for wave G, which shows the block as the exporting person sees it.
- **Print:** a block prints what it shows. Load more does not print.
- **Edge cases:** a kind this release does not know shows a placeholder and stays unchanged (EDT-158). Settings that point at a page the reader may not see give the same answer as a page that does not exist. A deleted page in a setting says so to editors only.

### E2. Finding your way on a page

| Block | Needs | Notes |
|---|---|---|
| Table of contents (BLK-013) | Nothing | Runs in the browser from `headingsOf`. Settings: levels, list or line, markers, section numbers, indent, include and exclude patterns, leave out of print |
| Section contents (BLK-014) | Anchors | Between two anchors, or inside one collapsible section |
| Back to top (BLK-015) | Nothing | A link to the title, which takes focus |
| Anchor (BLK-016, EDT-068) | Nothing | An inline node. Lower case, spaces become dashes, at most 64 characters. Visible while editing only. Two with one name: the first wins, and the editor says so |
| Search box (BLK-017) | Wave D filters | Enter goes to the results screen with the filters in the address |
| Search in this branch (BLK-018) | Wave D's filter below a page | The same box, with a chosen page |

### E3. Showing what is in the tree

| Block | Needs | Notes |
|---|---|---|
| Child pages (BLK-019) | Nothing. Excerpt preview needs E7 | Parent, maximum, levels, sort, style, preview. The first lines come from the search text, never from a hidden page |
| Page tree (BLK-020) | Nothing | Loads a branch when it opens. The keys of the page tree, Collapse all, Expand all, an optional filter |
| Index of a space (BLK-021) | Nothing | A letter bar of links. Empty letters are not links. Other scripts collect under `#` |
| Spaces list (BLK-022) | Wave F for categories | Favourites and all, as cards or a list. The category filter arrives with categories |
| Breadcrumbs here (BLK-023) | Nothing | The path of a chosen page, for hub pages |

### E4. Recent and personal

| Block | Needs | Notes |
|---|---|---|
| Recent changes (BLK-024) | Comments from wave C, blog posts and labels from wave D | Spaces, types, number, people, labels, style. At most 200 |
| Activity tabs (BLK-025) | Following from wave C | Tabs by the pattern of the Recent screen |
| My favourites (BLK-027) | Nothing | The same list as the screen |
| My drafts and tasks (BLK-028) | Tasks from wave C | Personal, so every reader sees their own |

### E5. Lists by label and filter

| Block | Needs | Notes |
|---|---|---|
| Filter list (BLK-029 to 031) | Labels from wave D, mentions from wave C | Rows of filters. Values in one row mean or, rows mean and, a minus excludes. Each filter that lacks its data is left out of the form, not disabled |
| Report table (BLK-032) | Status from wave D, counts from wave C | Columns in the sortable pattern |
| Labels list, popular, related, recent (BLK-033 to 036) | Labels from wave D | The word cloud is a list with sizes, which reads in order for a screen reader |

The filter is one Go type, `blocks.Filter`, shared with the reports, so a later filter joins all of them.

### E6. Page facts and reports

- **Page facts (BLK-039):** a block holding a two-column table of keys and values. Values can be text, a person, a date or a status mark. The block has an optional id, so a page can hold several. *Hide on this page* is offered with a line that says this is not a secret (D13).
- **Index:** at publish each row becomes a row in `page_facts`, with the text and also a date or a number when the value is one.
- **Facts report (BLK-040, 041):** page filters as in E5, an id, a title heading, columns, a sort column, counts, rows per page. Columns follow the keys found, in order of first use. Sorting by a date or a number uses the real value. Text that is not a number sorts after numbers.
- **Decision report (BLK-037):** from `page_decisions`, which the decision block fills. The block gains a person and a date (EDT-122). The person needs wave C's mention.
- **Task report (BLK-038):** on wave C's task index. Readers with *edit* tick tasks in the report, which makes a small version without a notification (EDT-050).
- **Template (BLK-042):** wave D's templates hold a facts block as they hold any block.
- **Edge cases:** two rows with one key: both show, in one cell. A key longer than 200 characters is cut in the report. A fact in a page the reader may not see never counts. Removing a facts block removes its rows at the next publish.

### E7. Reusing text

- **Excerpt (BLK-043):** a wrapper with a name and *hide on this page*. A page can have several, and a name is unique per page. Showing one inline uses the first paragraph.
- **Show excerpt (BLK-044):** a page and an excerpt name, with a title or a frame. A link to the source shows on hover and focus.
- **Embedded page (BLK-045):** the whole current version of a page.
- **Synced block (BLK-046):** an excerpt marked as synced. Each place shows a small mark with the source, and Edit source for those who may. It opens the source page.
- **Rights:** at each level the reader needs view. Without it that part shows nothing, not even a gap with a message, unless the reader may see the writer's settings as editor.
- **Loops and depth (BLK-047, 048):** the server walks includes with a set of pages seen. A loop or a sixth level gives a short message in place.
- **Search (BLK-049):** only the source is indexed.
- **Where used (BLK-050):** `page_includes` is filled at publish. The Details panel lists it, and the Delete dialog warns about it, with what the person may see.
- **Variables (BLK-051):** an inline node `variable` with a name in an excerpt. Show excerpt fills it with plain text from its settings. An empty one shows its name in a muted box for editors, and nothing for readers.
- **Edge cases:** an excerpt that gets renamed breaks its uses. Those show a short message to editors, and the panel offers the names that exist. An attachment in an include loads through the source page's rights, so a reader who passes the check sees it.

### E8. People and files as blocks

| Block | Needs | Notes |
|---|---|---|
| Contributors (BLK-052) | Comments and watching from wave C, labels from wave D | Types, sort, maximum, list or inline, counts, last time. Starts with authors |
| Contribution table (BLK-053) | The same | By person or by page, in the sortable pattern |
| Person card (BLK-054) | Wave F for job title and picture | Needs *view profiles*. Without it, only a name. No contact details until wave F decides them |
| People list (BLK-055) | Groups exist | A group or chosen people, as a list or grid. Needs *view profiles* |
| Change history here (BLK-056) | Nothing | The last versions, with a link to Compare |
| Attachments here (BLK-057) | Labels on files, see E11 | Name pattern, old versions, sort, rows. Delete and versions for those who may. No upload, as files are added in the editor (ADR-022) |
| Space files (BLK-058) | E12 | The list of the Files screen as a block |

### E9. Blog and content blocks

| Block | Needs | Notes |
|---|---|---|
| Blog posts (BLK-059) | Blog from wave D | Number, period, labels, authors, title or excerpt or full text. Full text goes through E7's check |
| Create from template (BLK-060) | Templates from wave D | A button. Only shown to those who may create in the target |
| Link cards (BLK-061) | Nothing. Filter needs E5 | Chosen pages, or a filter. The header image of the page, or none. Text comes from the search text |
| Rotating cards (BLK-062) | Link cards | Off by default (D21). The carousel pattern: stop button first, `aria-roledescription`, Previous and Next, and no motion with reduced motion |

### E10. Drawings and timelines

- **Drawing (BLK-064):** boxes, ellipses, text and arrows on a grid, in a dialog on top of the editor. Stored as a list of shapes with a position, size, words and a colour name. At most 200 shapes.
- **Mouse:** drag to move, drag a corner to resize, drag from a side to draw an arrow.
- **Keyboard:** a list of shapes beside the surface. Arrows move the selected shape by one grid step, Shift with arrows resizes, Delete removes it, Enter edits its words.
- **Accessibility:** a required description field, and the words of the shapes form a list for screen readers. For readers it is a `figure` with the picture and that text. It scrolls in its own frame.
- **Timeline (BLK-065):** lanes, bars and markers against a time scale, edited as rows in a table beside it, so nothing needs dragging. Dragging a bar works too. Dates are real dates. For readers it comes with the same rows as a table.
- **Edge cases:** words are text, never markup. A bar that ends before it starts is refused in the form.

### E11. Files: adding and details

- **Progress (FIL-011):** uploads use `XMLHttpRequest`, the only way to get progress. A bar with a percentage and Cancel. Cancel leaves nothing behind. `role="status"` announces steps of ten percent.
- **Same name (FIL-012):** the editor asks the server not to add a version silently. A 409 opens a dialog: New version, Keep both, Cancel. Keep both adds ` (2)` before the extension. Other clients keep today's behaviour.
- **On a tile (FIL-013):** dropping one file on a tile adds a new version of that file. The tile shows an outline first. Several files on a tile land after it as usual.
- **Archives (FIL-014):** already true, and a test proves it.
- **Picker (FIL-015, EDT-071):** a dialog with the files of the page, with a filter. It adds a tile or image without an upload.
- **Clearing up (FIL-017):** files that no version uses are listed in Attachments, with Delete. `attachment_refs` is filled at publish, and the migration fills it from all versions.
- **Types and limits (FIL-020):** administrators block extensions and detected types, and set a limit per space. The check runs before storing. The message names the rule, not the setting.
- **The tile (FIL-025, 026, 029):** a lookup gives each tile its live name, type, size and state, in one request per page. Small is one line. Large shows the image for images, and otherwise stays small (D14). A file in the trash says *This file was deleted* and is no link. A reader without access sees the same words.
- **Previews (FIL-027, 028):** one viewer on top of the page, with Download, Next and Previous, extending the image viewer of wave B. Images show as they do now. Text and code show from the text route, with colours from `lowlight`, up to 1 MiB, and a note when cut. Other types say that there is no preview, and offer Download.
- **Video and audio (EDT-083):** a `media` block with the browser's own controls and `preload="metadata"`, with a caption. MP4, WebM, Ogg, MP3 and WAV. Served with range requests under the same locked-down headers. A failing file offers Download.

### E12. Files: the lists

- **Attachments dialog (FIL-034, 036):** sort by column heading, a filter on the name, who added it and when, *Used on this page* or not.
- **Download all (FIL-035):** one zip, streamed, under the same rules as exports. One audit event.
- **Versions (FIL-037, 038):** a row opens a list with date, person and size. Old versions download. *Make current* adds a new version with that content. *delete any* removes an old version, never the current one.
- **Details (FIL-039 to 041):** a dialog to rename, add a comment and labels, move and copy. Moving needs *add* on the target and *delete* on the source. Tiles keep working, because they hold the id. Labels use wave D's labels.
- **Links (FIL-042):** Copy link gives the latest version, or one version with `?version=`.
- **Files screen (FIL-043 to 045):** `/spaces/:key/files`, with filters, paging and a link in the sidebar. Rows come through `perm_visible_pages`. Total storage and the largest files show to space administrators only, because a total would tell how much is hidden. Bulk delete takes up to 200 at once, and says what did and did not work.
- **History (FIL-050):** the page history shows files that came and went, from `attachment_events`.

### E13. The tail of the editor

**The surface.**
- **Tools (EDT-007):** what does not fit folds into a More menu at the end, in a fixed order, using a measurement after each resize. The tools for a selection fold last.
- **Width (EDT-008):** the sheet shows the page's width, and the strip gets a button for it. Page look stays the place for the rest.
- **Title (EDT-009, 010):** focus on a new page, Enter moves to the body, Backspace at the very start moves back. Hovering shows Add emoji and Add header image, which open Page look.
- **Counts (EDT-011, 151):** a word count and read time at the bottom, on request, which count the selection when there is one.
- **Below the text (EDT-014):** a click there puts the cursor at the end, and adds a paragraph after a block that is not text.

**Ways to add.**
- **Menu (EDT-017, 019, 020):** group headings, recently used first (kept in the browser, at most five), a plus on empty lines, and Browse all with small previews drawn from our own blocks.
- **Block menu (EDT-022, 023):** Copy link to this block gives the block a `block_id`, and Turn into suggests the likely choices first, and works back from panels and sections.
- **Several blocks (EDT-024):** Shift with the grip, or Shift with arrows from the handle, selects a range. Move, Delete and Turn into then act on all of it. One Undo reverses it.
- **Block templates (EDT-026):** Save as block template in the block menu. They are private, stored per person, and appear in the `/` menu. Sharing them with a space comes with a checkbox. Content passes the same gate.

**Typing.** Backspace after a shortcut restores the typed characters (EDT-028). `:)` becomes an emoji, and `->`, `<-`, `...` and quotes become typographic characters, behind one preference (EDT-029, 030). Nothing changes inside code.

**Text and lists.**
- **Direction (EDT-042):** a block can be marked left-to-right or right-to-left, as a named attribute that sets `dir`.
- **Lists:** markers change by level in the style sheet (EDT-047). Dragging an item takes what is nested below it (EDT-048). A list turns into a task list and back with its nesting (EDT-052). An open overdue task shows its date in the danger colour with a word beside it (EDT-051).
- **Heading anchors (EDT-044):** built in wave B from the text. Small edits do break them. A `block_id` on a heading, given when a link to it is copied, keeps such a link working.

**Links.**
- **Recent pages first (EDT-056).**
- **Tags and cards (EDT-057, 065, 066, 129):** a link to a page can show as text that follows the title, as a tag, or as a card. `pageTag` and `pageCard` hold the id (D9). One lookup per page gives the titles.
- **States (EDT-059, 060):** *visible* shows the title. *deleted* shows struck through, only to those who could have seen the page. *unavailable* shows the writer's words, or the word *Page* for a tag, with no address and no title (D8).
- **Headings and blocks (EDT-058):** the link dialog can pick a heading or block in a page.
- **Pasting (EDT-062):** an address pasted over selected text makes that text a link.
- **Bar (EDT-063):** Open, Copy, Edit, Unlink and Change display sit in the row of tools (D5).
- **Placeholder (EDT-064):** the dialog offers *Create this page later*. The link shows in a distinct colour with a dashed underline. Following it opens the New page dialog with the title and parent. Space settings list placeholders. Without *create* it shows as plain words.

**Images.**
- **Size (EDT-072):** a corner handle and a width field, from 10 to 100 percent of the column, in steps of one. Stored as a number, which is the one exception to named sizes and is checked as a range.
- **Wrap (EDT-073):** text can flow around an image on the start or end side. Narrow screens stack.
- **Caption (EDT-074):** bold, italic and links, as a small inline document.
- **Alt text (EDT-075):** a field is offered when an image arrives. A quiet line in the editor counts images without one.
- **Border and link (EDT-076, 077):** a border from a short list, and a link to a page or address. A linked image opens the link, not the viewer.
- **Inline image (EDT-078):** a small image in a line, limited to the height of the line.
- **Pasting from a page (EDT-079):** the browser fetches what it is allowed to, and stores it as an attachment. What it may not fetch leaves a clear note. The server never fetches it.
- **Missing (EDT-081):** a placeholder with the alt text.

**Tables.**
- **Adding (EDT-086):** a size picker with a grid, and number fields for the keyboard.
- **Handles (EDT-087, 088, 091):** a handle on each row and column selects it and opens its menu, with Insert, Move, Delete. A plus between them inserts one. Dragging reorders, and Shift selects several.
- **Tab (EDT-089):** a test proves the last cell adds a row.
- **Numbered column (EDT-093):** counts by style sheet, so it follows sorting.
- **Width and layout (EDT-097, 098):** normal, wide or full, fixed or flexible.
- **Sticky header (EDT-099):** `position: sticky` inside the table's frame.
- **Paste (EDT-100):** cells from a spreadsheet land as cells in the table at the cursor, and add rows when needed.
- **No tables in tables (EDT-101):** the schema refuses it, and the tool says why. Old documents stay readable.
- **Caption (EDT-102):** a real `caption`.
- **For readers (EDT-094, 103):** sort buttons in header cells and a filter box above, for tables that turned them on (D17). The filter says how many rows show.

**Code.** Search and recently used languages (EDT-105). A detect button that uses `lowlight`, which the writer can change (EDT-106). A start number (EDT-107). A long block can start folded, as a `details` (EDT-111). Several lines indent together (EDT-112). Pasting keeps the text exactly (EDT-113).

**Structure.**
- **Panel (EDT-115):** a neutral kind with a chosen emoji and colour name.
- **Sections (EDT-116):** opening for print is built. Exports wave G. Links into a section open it, as `jumpToHeading` does.
- **Columns (EDT-118 to 120):** a narrow sidebar split, dropping a block beside another with a dashed line, and a width.
- **Spotlight (EDT-123):** a wide band with a heading, text and a button link. The colour is a name.

**Small items.** A date in a task sets its due date, on wave C's index (EDT-125). The emoji tool opens a picker with groups and recent emoji, and groups join `emoji.json` (EDT-127).

**Charts (EDT-130 to 136).**
- **Adding:** in the table tools, Add chart. A chart block lands below the table, and the table gets a `block_id`.
- **Data:** read from the table in the document, so it follows every edit without a save. Numbers are parsed in the reader's locale from the cell text. A cell that is not a number counts as empty, and the chart says how many.
- **Options:** kind, axis column, data columns, totals, title, axis titles, legend, height, smooth lines, points. In the settings panel of E1.
- **Drawing:** our own SVG in `components/chart.tsx`, which needs the rule change below. Colours are names, from six new tokens.
- **Telling series apart:** line markers and dashes, bar and slice patterns (1.4.1).
- **Reading:** a `figure role="group"`, a `figcaption`, a picture named by its title, and a real table below it behind a Show data toggle. Screen readers always reach that table, and the picture says it is there.
- **Removing:** removing a chart leaves the table. Removing a table asks: remove the chart too, or keep it with a note that its table is gone.
- **Hidden table (EDT-135):** works by itself, because the chart reads the document, not the screen.
- **Limits:** 500 rows and 12 series. Past that the chart draws the first ones and says so.

**Paste.** Lists, tables, headings and links from Word and Google Docs arrive as our blocks (EDT-138), through rules on their markup, with test samples of our own. Ctrl Shift V pastes plain text (EDT-139). Markdown is recognised by its markers, and one Undo restores the plain text (EDT-140). A block copied from another page keeps its blocks, and its files stay linked when the reader may see them, or are copied along (EDT-143).

**The rest.**
- **Whole word (EDT-145).**
- **Language (EDT-147, 148):** a page language in Page look sets `lang` and the browser's spell checker.
- **Long pages (EDT-150):** a measured test with 100,000 words, and a fix for the `setOptions` call on each render noted in wave A.
- **Older documents (EDT-157):** `document.Upgrade` runs on read, with a test per schema version.
- **Unknown blocks (EDT-158):** the gate keeps a block it does not know only in a read of an older stored document. Publishing such a page keeps it unchanged.

## API

| Route | Rule | What |
|---|---|---|
| `POST /blocks/{kind}` | authenticated | The answer of one block for this reader. List only: the module decides, through `perm_visible_pages` |
| `GET /block-kinds` | authenticated | Which kinds this instance has, and their limits |
| `POST /pages/resolve` | authenticated | Titles and states for links, tags and cards. At most 500 ids |
| `POST /attachments/lookup` | authenticated | Live facts for tiles. Unknown and hidden ids answer the same |
| `GET /pages/{id}/includes` | `page.view` | Where this page is used, as far as the reader may see |
| `GET /spaces/{id}/placeholders` | `space.view` | Links to pages that do not exist yet |
| `GET /spaces/{id}/files` | `space.view` | The Files screen, with filters and paging |
| `GET /spaces/{id}/files/usage` | `space.administer` | Storage and the largest files |
| `POST /spaces/{id}/files/delete` | `space.administer` | Bulk delete, at most 200 |
| `GET /pages/{id}/attachments.zip` | `page.view` | Download all |
| `GET /pages/{id}/file-history` | `page.view` | Files that came and went |
| `GET /attachments/{id}/versions` | `page.view` | Versions of a file |
| `POST /attachments/{id}/versions/{number}/current` | `attachment.add` | Make an older version current |
| `DELETE /attachments/{id}/versions/{number}` | `attachment.delete_any` | Remove an old version |
| `PATCH /attachments/{id}` | `attachment.add` | Rename, comment |
| `POST /attachments/{id}/move`, `/copy` | `attachment.add` | The module also checks delete on the source and add on the target |
| `PUT /attachments/{id}/content` | `attachment.add` | A new version by dropping on a tile |
| `GET /attachments/{id}/text` | `page.view` | Plain text for a preview |
| `GET, PUT, DELETE /me/block-templates` | authenticated | Own block templates |

Changed: `POST /pages/{id}/attachments` takes `on_conflict=ask`. `GET /attachments/{id}/content` takes `version=`, and shows video and audio on the short list.

## Data

Migration 0016, expand only:

- `page_facts`: page, facts id, key, position, value text, date, number.
- `page_decisions`: page, block id, text, person, date.
- `page_includes`: source page, target page, excerpt name.
- `page_placeholders`: source page, title, parent.
- `attachment_refs`: attachment, page, version. The migration fills it from every version, the way 0006 filled `page_links`.
- `attachment_events`: page, attachment, file name, action, person, time.
- `block_templates`: id, owner, name, document, shared space or none.
- `attachments`: `comment`.
- `spaces`: `storage_limit_mib`.
- Settings: `blocked_file_types`, and the row limit of reports.

Facts and decisions in pages that exist before this release are empty until their next publish, which matters little, as those blocks are new.

## Document format

`SchemaVersion` stays 1: everything is added, and old documents stay valid.

| Node or mark | Attributes |
|---|---|
| `dynamic` | `kind` from a list, `settings` by the allow-list of that kind, `width` |
| `pageFacts` | `facts_id`, `hidden`. Holds one table |
| `excerpt` | `name`, `hidden`, `synced` |
| `variable` (inline) | `name` |
| `anchor` (inline) | `name` |
| `pageTag` (inline) | `page_id`, `target`, `display` |
| `pageCard` | `page_id` |
| `chart` | `table`, `kind`, `axis`, `series`, `total`, `title`, `axis_titles`, `legend`, `height`, `smooth`, `points` |
| `drawing` | `shapes`, at most 200, `description` |
| `timeline` | `lanes`, `items`, `description` |
| `media` | `attachment_id`, `kind`, `caption` |
| `spotlight` | `color`, `link`, holds a heading and text |
| Added to many | `block_id`: 8 to 12 lowercase letters and digits. `dir`: `ltr`, `rtl` |
| `image` | `width` 10 to 100, `wrap`, `border`, `link`, `inline`, and `caption` as inline content |
| `table` | `caption`, `width`, `layout`, `numbered`, `sticky`, `reader_sort`, `reader_filter` |
| `codeBlock` | `start`, `folded` |
| `panel` | `emoji`, `color` for the kind `neutral` |
| `decision` | `person`, `date` |
| `columns` | `width`, and the split `sidebar` |

Every line joins `Normalize`, `ToMarkdown`, `FromMarkdown` where it has a Markdown form, `Text`, `Compare` and the Go tests (EDT-155). A `pageTag` and `pageCard` export as a link, with the title as the exporter may see it. `Compare` shows a change of a block's settings as one line.

## Build order

1. Go: migration, document format, `internal/blocks`, indexes at publish, tests first.
2. Files on the server: versions, details, lookup, text, media, zip, Files screen, limits.
3. E1: the block node, panel, route, error handling. Then the kinds that need nothing: E2, E3.
4. Reuse: E7, then facts and reports: E6, then E4, E5, E8, E9.
5. Files in the browser: E11, E12.
6. Links and images of E13.
7. Tables, then charts.
8. The rest of E13, then E10.
9. Docs, statuses in `03-functional`, handoff.

The wave is large. Commits can follow these steps, with one commit note each, when the product owner prefers that to one commit.

## Test plan

- **Go unit:** every new node and attribute through the gate, with bad values. Facts values to dates and numbers. Patterns with `*`.
- **Go integration:** per kind: a hidden page never shows, counts leave it out, limits hold. Includes: rights at each level, a loop, level six, a deleted source. Link states: a hidden and a missing page answer the same. File moves, versions, block lists, per-space limits, totals for non-administrators.
- **Route walk:** the new object routes join `TestEveryObjectRouteIsProtected`. The list routes get their own test for a stranger.
- **Mutation:** a block that skips `perm_visible_pages`, an include that skips a level's check, the depth limit, a loop, link states that differ, the media list that lets HTML through, the usage route without administer, a blocked type that passes, an old version that can be deleted without *delete any*.
- **Vitest:** chart maths and number parsing, sections of a table of contents, patterns, slug and `block_id` rules, Word samples, settings forms.
- **Browser:** each section above, keyboard first. Upload progress and Cancel. A chart that follows an edit. A failing block that leaves the page working. Reader sort and filter that leave the version alone.
- **Policy:** every test already fails on a policy violation, which covers media and our own drawings.
- **Pictures:** the settings panel, one of each block family, a report, a chart in light and dark, the file viewer, the Files screen, a drawing, narrow and right-to-left. Short content as well as long.
- **Accessibility:** axe on each, with panels open. A chart's table is checked by name.
- **Speed:** a page with 100,000 words, and a page with 20 blocks that load near the screen only.

## Acceptance criteria

- [ ] AC-1 A block inserted from `/` opens its settings, shows real content at once, and needs no save.
- [ ] AC-2 No block shows a title, name, count or file from a page the reader may not see.
- [ ] AC-3 An empty block says so, and a failing block leaves the page working.
- [ ] AC-4 A slow block does not delay the text, and no block returns more than its limit.
- [ ] AC-5 Table of contents, child pages, page tree and index work with their settings.
- [ ] AC-6 Recent changes and the filter list follow the *or*, *and* and minus rules.
- [ ] AC-7 A facts report collects page facts, and sorts dates and numbers by their value.
- [ ] AC-8 Decision and task reports list what they should, and a task can be ticked there by those who may.
- [ ] AC-9 An excerpt shows on another page, and a change at the source shows there too.
- [ ] AC-10 A loop and a sixth level give a short message, and a page without rights shows nothing of its text.
- [ ] AC-11 Search finds included text only at its source, and a source warns where it is used.
- [ ] AC-12 A drawing and a timeline can be made by keyboard alone, and readers get a text version.
- [ ] AC-13 Upload progress shows, and Cancel leaves nothing behind.
- [ ] AC-14 A file with the same name asks first, and dropping on a tile adds a version.
- [ ] AC-15 A tile shows type and size, and a deleted file says so.
- [ ] AC-16 A text file previews with colours, and no preview runs anything from a file.
- [ ] AC-17 Video and audio play, and an HTML file named `.mp4` is a download.
- [ ] AC-18 File versions can be listed, downloaded, made current and removed by those who may.
- [ ] AC-19 A file can be renamed, moved and copied, and tiles keep working.
- [ ] AC-20 The Files screen respects restrictions, and only space administrators see totals.
- [ ] AC-21 An administrator can block a file type and limit a space.
- [ ] AC-22 Links show the current title, tags and cards are per reader, and hidden and missing pages cannot be told apart.
- [ ] AC-23 A placeholder link starts the right page.
- [ ] AC-24 Images can be sized, wrapped, bordered and linked, and a missing one shows a placeholder.
- [ ] AC-25 Tables have handles, a size picker, a caption, a sticky header, and reader sort and filter that change nothing.
- [ ] AC-26 A chart follows its table, tells series apart without colour, and can be read as a table.
- [ ] AC-27 Several blocks can be selected, moved and deleted, with one Undo.
- [ ] AC-28 Word, Markdown and spreadsheet pastes arrive as blocks, and Ctrl Shift V gives plain text.
- [ ] AC-29 Typing stays smooth at 100,000 words.
- [ ] AC-30 Everything passes axe, and the new pictures have been looked at.

## Risks

| Risk | Answer |
|---|---|
| A new kind forgets the permission rule | One route, one `Filter`, and a test that walks every kind with a hidden page |
| Reports are slow | Indexes at publish, a hard limit per kind, a row limit an administrator sets, and loading near the screen |
| Includes make a page heavy | At most 5 levels, a size limit on the expanded answer, and a message past it |
| `block_id` makes every comparison noisy | Only added on request, and left out of `Compare` |
| A chart misreads numbers across languages | Parsing has its own tests, and the chart says how many cells it skipped |
| Word paste differs per version | Test samples of our own, and a plain fallback through the allow-list |
| Replacing a file by a drop surprises someone | The tile shows an outline with words first, and versions keep the old file |
| The wave is too large for one go | The build order has points where it can stop, and every step leaves the app working |
| 100,000 words stays slow in the browser | It is measured, and a miss is reported, not hidden |

## The product owner's decisions (ADR-026)

These replace the options below, which stay as the record of what was asked.

**Correction, 2026-11-06:** PdfCraft is AGPL-3.0, by its LICENSE file and the GitHub API, not MIT or Apache-2.0. That is outside the licence list, and a site served from our own origin is no separate program, so it is not embedded and PDF stays a download. Its size was not measured. The product owner decides what happens next.

- **PDF:** PdfCraft instead of `pdfjs-dist`, if its web build is not too large. It is MIT or Apache-2.0, and alpha. Its WebAssembly static site is served from our own origin in a sandboxed frame, so the policy for WebAssembly applies to that frame only. Measure the size first, and pin the version. The product owner wants people to be able to change a PDF themselves: pages, comments and forms work today, and changing text is thin. Printing is missing from its web build. If it is too large, or cannot be embedded, say so and leave PDF as a download.
- **Diagrams:** draw.io instead of `mermaid`, as an optional separate service in embed mode. A person draws in a frame, and the result is stored as a PNG with the diagram inside it (`xmlpng`), as a normal attachment. Only an address the administrator set is used: `embed.diagrams.net` would tell an outside party who is drawing what. The editor is Apache-2.0, its Docker wrapper is GPL-3.0, and that is a separate program. Stencils have terms of their own, to check.
- **Office files:** chosen at installation. Collabora by its conversion route, LibreOffice through the Gotenberg container of wave G, or none. Euro-Office joins when its way of connecting is known: it is AGPLv3, a separate program, and no source read for this plan confirms how it connects.
- **Drawing:** `components/chart.tsx` and `components/drawing.tsx` may hold SVG, by name, in the design check.
- **EDT-006** is dropped.
- **`golang.org/x/image`** is not needed.

## For the product owner to decide

These block parts of the wave. Everything else can be built without them.

| Question | What it unlocks | Option | Licence | Without it |
|---|---|---|---|---|
| Add `mermaid`? | Diagram (BLK-063) | The usual library for diagrams as text. Large, with its own dependencies. A proof under our Trusted Types and style policy comes first | MIT, per its repository (to check at approval) | Text stays in a code block with the language `mermaid`, and drawings cover simple cases |
| Add `pdfjs-dist`? | PDF viewer (BLK-066), PDF in previews and tiles | The usual viewer. Runs in a worker, so the policy needs `worker-src 'self'`. Large files ship with the app | Apache-2.0 | PDF stays a download. A frame with the browser's viewer is not safe enough under our rules |
| Add a converter for office files? | Office preview (BLK-067) | A separate service an administrator installs, such as LibreOffice behind a small HTTP service. It is no Go or npm dependency, but LibreOffice is MPL-2.0, which is outside our licence list for dependencies. A separate program is a different case, and yours to judge | MPL-2.0, as a separate program | Office files stay a download |
| Add `golang.org/x/image`? | Server thumbnails, WebP | Only needed if D14 changes | BSD-3 | Not needed: browsers scale |
| May a second file draw? | Charts, drawings, timelines | The design check allows `<svg>` only in `components/icons.tsx`. Charts need SVG of their own. The advice is to allow `components/chart.tsx` and `components/drawing.tsx` by name, with an ADR | Own code | Charts cannot pass the check. Bars alone could be done in HTML |
| A floating bar when the tools are hidden? | EDT-006 | The design standard says no floating bars. It would be a change of look | Own code | The tools stay in the row, pinned, and EDT-006 is dropped |

## How the product owner tests it

1. Edit a page, type `/child` and insert Child pages. Change the settings and watch the block.
2. Log in as a guest, and see that a restricted page is gone from the block and from its count.
3. On two pages add Page facts with the same keys and one label, then add a Facts report on a third. Sort by a date.
4. Mark an excerpt on one page, show it on another, change the source and publish.
5. Make page A show page B, and B show A. Read the message.
6. Add a table with numbers, then Add chart. Change a number. Press Show data.
7. Drag a large file in, and press Cancel. Drag a file with the same name as an existing one.
8. Drag a file onto a tile. Open More, Attachments, and open its versions.
9. Open a text file from its tile. Open a video.
10. Open Files in the sidebar of a space, as a space administrator and as a reader.
11. Delete a file, and look at its tile.
12. Link to a page as a tag, rename that page, and look at the tag. Look at it as someone who may not see the page.
13. In a table, turn on reader sorting and the filter. Publish, sort, reload.
14. Paste a table from a spreadsheet, and a list from Word.
15. Hold Shift and select three blocks with their grips. Delete them, then press Undo.

## Provenance

Own text from these public sources, read for behaviour only, plus the study behind blueprint 03 (`docs/blueprint/03-functional/sources.md`):

- W3C WAI: Complex images tutorial. WAI-ARIA Authoring Practices: sortable table example, carousel pattern.
- W3C WCAG 2.2 Understanding: 1.4.10 Reflow, 2.2.2 Pause, Stop, Hide.
- WHATWG MIME Sniffing Standard.
- MDN: `frame-src`, the `iframe` element.
- Go documentation: package `image`.
- Mermaid documentation: usage and security levels. PDF.js repository: licence and what ships.
- Public help pages of a commercial wiki on its page properties report: the pairing with a label, or and and in filters, the row limit, empty reports. Read for behaviour only.
- General articles on accessible charts, used as pointers only. The rules above rest on WCAG and the WAI tutorial.
