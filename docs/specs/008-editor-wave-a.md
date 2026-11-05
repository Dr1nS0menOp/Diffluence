# 008: Wave A, writing feels right

Status: built, waiting for the product owner's acceptance session
Phase: wave A of the proposal in `docs/blueprint/03-functional-requirements.md`. The product owner agreed on 2026-11-05 that the four waves come before phase 3, and that every phase starts with a large study and a detailed plan (ADR-024).

## Problem

The editor can only do the basics. The product owner said the product misses a lot, and most of that sits in the editor. A writer should reach everything from the keyboard with `/`, and shape a page without leaving the text. This spec is the plan for that wave: what the study found, what is decided, and how each part behaves down to its edge cases.

## What the study found

**How writers expect to work.** Every wiki and notes tool studied has the same core: a `/` menu at the cursor, a handle beside each block that drags it and opens a menu, *turn into* to change a block's type, coloured call-out boxes, collapsible sections, and columns. Recently used entries come first. Keyboard shortcuts for headings and lists have settled on one convention: Ctrl Alt with a number for headings, and Ctrl Shift with 7, 8 and 9 for lists. Our editor library follows that convention already, so we keep it.

**What accessibility demands.** These come from WCAG 2.2 and the WAI-ARIA Authoring Practices, and change the design:

| Source | Demand | What we do |
|---|---|---|
| Toolbar pattern | A toolbar is one Tab stop. Arrow keys move inside it, Home and End jump. | The tools get roving focus. Today every button is a Tab stop, which means 26 presses to get past. |
| Combobox pattern | The element being typed in keeps focus. The list is pointed at with `aria-activedescendant`. | The editor keeps focus while the `/` menu is open. |
| Menu pattern | Arrows, Home, End, a letter jumps to an item, Escape returns to the button. Choices are `menuitemradio` with `aria-checked`. | `Menu` gains Home, End and letters. Colour and alignment menus use radio items. |
| 2.5.7 Dragging | Anything done by dragging must also work without dragging. | The block menu has Move up and Move down. Column widths have presets and an Equal button. |
| 2.5.8 Target size | At least 24 by 24 CSS pixels. | The handle and swatches are at least `--size-twist` (24px). |
| 2.4.11 Focus not obscured | A focused item may not be hidden under a sticky bar. | `scroll-padding-block-start` keeps it clear of both bars. |
| 1.4.1 Use of colour | Colour alone may not carry meaning. | Panels have an icon and a name, status marks have words, and swatches have names. |
| 1.4.11 Non-text contrast | Controls need 3:1. | Swatches have a border, and contrast pairs join the design check. |
| 2.1.4 Character keys | Single-key shortcuts must be limited. | `/`, `:` and `//` only work while typing in the editor, where they are text input. |
| 4.1.3 Status messages | Messages must reach screen readers without focus. | Upload lines and the match count use `role="status"`. |

**What the libraries allow.** The `@tiptap/suggestion` utility positions its popup with Floating UI through `mount`, which `@tiptap/react` already brings in. TipTap's own drag handle package needs the whole collaboration stack (Yjs and more), so it is out: our handle is small and our own. ProseMirror's `dropPoint` and `replaceRangeWith` find a place for a block near a position, and may change more than the given range, which is what a block needs. `isolating` stops Backspace crossing a boundary, and `defining` keeps a block when its content is replaced. Content expressions can count children, so `column{2,5}` is enforced by the schema. Nodes without text need the gap cursor to be reachable, and it needs a visible style, which ours lacks.

**Safety.** The clipboard API can refuse, so a failed copy must say so. A colour package would store free values, which our document rules forbid, so colours are own marks with names. Emoji are Unicode characters shown by the system font, so no images ship. Our short list has our own keywords, so no outside data set or licence is involved.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | One new dependency: `@tiptap/suggestion` | The product owner approved it. Everything else is small enough to write. |
| D2 | No TipTap colour, highlight, alignment or drag handle packages | They store free values, or pull in a large stack. |
| D3 | Tools that depend on the selection appear in the row, as table tools already do | It is what Rail chose, it needs no floating bars, and it is simpler to reach by keyboard. |
| D4 | Colours and sizes are names from short lists | No document can carry a style, and dark mode works everywhere. |
| D5 | Panels may hold any block, and the interface does not offer a panel inside a panel or columns inside columns | Server and editor stay simple, and a paste cannot lose content. |
| D6 | A collapsible section uses the native `details` element for readers | Keyboard, focus and find-in-page come for free. |
| D7 | A block handle moves top-level blocks only | Nested dragging is a large job and rarely needed. A block can still be dropped into a column. |
| D8 | Keyboard: Alt with arrows moves a block, Ctrl D duplicates it, Ctrl Shift . opens its menu | Alt with arrows is known from code editors, and it does not collide with text selection. Ctrl . is superscript. |
| D9 | Copy link to a block, multi-block selection and block templates wait for wave B or later | They need stable block anchors. |
| D10 | Image size uses presets, with no drag handles on corners yet | Presets satisfy 2.5.7 and keep documents consistent. |
| D11 | Sorting by readers, filtering, sticky headers and print wait for wave B | They belong to reading. |
| D12 | The status mark has six colours and a default word | Own palette, shared with highlights and panels. |

## Scope

In: EDT-015 to 018, 020 to 023 and 025 (slash menu, insert dialog, handle, block menu), the text chapter, panels, collapsible sections, columns, decisions, status marks, dates, emoji, the link dialog, image options, table tools, code options, find and replace, and the keyboard table.

Out, for later waves: plus button in the margin (EDT-019), recently used first until the menu has data worth sorting, drag on image corners, charts, block templates, synced blocks, reader sorting, placeholders for pages that do not exist, inline images, video.

## Feature plan

### A1. The row of tools

The row is the home of everything that is not typed. Order: text style, marks, More formatting, text colour, highlight, alignment, indent and outdent, lists and blocks, then link, add file, table, Insert, Find, then undo and redo, then a separator and the tools for the selection.

- **Keyboard:** one Tab stop. Left and Right move, wrapping at the ends. Home and End jump. A `select` keeps its own arrows, which the pattern allows, as it comes first. Alt F10 moves from the text to the row, and Escape returns to the text at the same spot.
- **Mouse:** pressing a tool never takes the selection away.
- **Menus in the row** (colour, highlight, alignment, more formatting) use `Menu`. Their choices have `aria-checked`, and the current one shows a mark. Default or None comes first.
- **Narrow screens:** the row wraps, as today. It lets go of its sticky place.
- **Edge cases:** a tool that cannot apply is disabled, not hidden, because its place should not jump. Selection tools are the exception: they are absent until they apply.

### A2. The slash menu

Typing `/` at the start of a line, or after a space, opens a list at the cursor.

- **Filter:** typing narrows by name and keywords, ignoring case. With no match the menu closes, and the typed text stays.
- **Keyboard:** Up and Down move and wrap, Enter inserts, Tab inserts, Escape closes and keeps the text. Typing goes on into the text, because the editor keeps focus.
- **Mouse:** clicking an entry inserts it. Hover marks it.
- **Order:** text, lists, blocks, media, in a line. Each entry has an icon, a name and a short hint.
- **Does not open:** inside code, after a letter or digit (so `1/2` and addresses are safe), or in the title.
- **What an entry does:** it removes the typed `/word`, then acts. On an empty line it replaces the line. On a line with text it turns that line into the block where that makes sense.
- **Accessibility:** the list has `role="listbox"`, the options `role="option"`, and the editor gets `aria-activedescendant` and `aria-controls` while it is open. It is a `textbox`, which does not allow `aria-expanded`.
- **Edge cases:** the popup flips above the line near the bottom of the screen. It closes when the cursor leaves the word. Undo restores the typed `/`. It stays inside the screen on narrow windows.

Entries: Text, Heading 1 to 6, Bullet list, Numbered list, Task list, Quote, Code block, Horizontal rule, Info panel, Note panel, Success panel, Warning panel, Error panel, Collapsible section, Two columns, Three columns, Decision, Table, Image or file, Link, Status, Date, Emoji.

### A3. The Insert dialog

The Insert tool opens a dialog with a search field and the same list, for people who prefer not to type `/`. It uses `Dialog`, so focus and Escape are handled. Enter on the search field inserts the first match.

### A4. The block handle and menu

- **Showing:** moving the pointer over a top-level block shows two small controls in the gutter at its first line: a grip and a menu button. They disappear with the pointer. They are at least 24px, and hidden on touch screens, where the menu is reached through the shortcut or the tools.
- **Dragging the grip:** the block moves. The drop cursor shows where it will land. Dropping into a column or a panel works. Dropping a block on itself does nothing.
- **The menu:** Move up, Move down, Duplicate, Delete, then Turn into: Text, Heading 1, Heading 2, Heading 3, Bullet list, Numbered list, Task list, Quote, Code block, Info panel, Collapsible section.
- **Keyboard:** Alt Up and Alt Down move the block with the cursor. Ctrl D duplicates it. Ctrl Shift . opens the menu for it.
- **Place:** the controls sit in the margin, outside the text, so they never cover the first letters. They stay still while the mouse button is down.
- **Edge cases:** Move up on the first block is disabled. A table moves as one block. Moving keeps the cursor in the block. Turning a list into text keeps each item as a line. Turn into is left out for blocks that cannot change, such as images, files, rules, tables and columns. One Undo reverses one move.

### A5. Text

- **Styles:** Paragraph and Heading 1 to 6, in the text style list.
- **Marks:** bold, italic, underline, strikethrough and inline code stay. Subscript and superscript are in More formatting, and exclude each other.
- **Text colour:** Default and 12 names: grey, slate, red, orange, amber, green, teal, blue, indigo, purple, pink, brown. Each has a light and a dark value with at least 4.5:1 on the page.
- **Highlight:** None and 6 names: grey, yellow, green, blue, purple, red. Normal text on each has at least 4.5:1, in both themes.
- **Clear formatting** removes all marks from the selection, but leaves links alone.
- **Alignment:** Start, Centre and End on paragraphs and headings. Logical names mean right-to-left mirrors by itself. Justify is left out: it hurts reading and accessibility.
- **Indent:** in a list it nests the item. In a paragraph or heading it adds one step, up to six. Outdent reverses it.
- **Keyboard:** see A18.
- **Edge cases:** inline code cannot take colours. A colour tool with an empty selection applies to what is typed next. Colours and alignment do not reach Markdown. Pasted colours are dropped by the allow-list unless they come from Diffluence.

### A6. Panels

A box with an icon, a tint and any blocks inside. Five kinds: Info (blue), Note (purple), Success (green), Warning (yellow), Error (red).

- **Making one:** from `/`, the block menu, or by typing `!!` and a space at the start of a line.
- **Changing kind:** tools appear in the row when the cursor is inside it. One button removes the panel and keeps its content.
- **Keyboard:** Enter stays in the panel. Backspace in an empty first line lifts the content out. Arrow keys leave it naturally.
- **Accessibility:** `role="note"` with the kind as its name, and the icon is hidden from screen readers.
- **Markdown:** exports as an alert (`> [!NOTE]`), and imports back. Info is NOTE, Note is IMPORTANT, Success is TIP, Warning is WARNING, Error is CAUTION.
- **Edge cases:** a panel inside a panel is not offered, but survives a paste. An empty panel keeps one line to type in.

### A7. Collapsible sections

A title and hidden content.

- **Writer:** the section is always open while editing, so nothing gets lost. The title is a field in the block. Enter in the title moves into the content. A tool in the row sets **Start open**. One button removes the section and keeps its content.
- **Reader:** a native `details` with a `summary`. Mouse, Enter and Space all toggle it. Find in the browser opens it.
- **Default title:** an empty title shows the word "Details" for readers, so the control always has a name.
- **Nesting:** sections can hold sections.
- **Search and comparison:** the title and the content count as text.
- **Markdown:** the title in bold, then the content. It does not come back as a section on import, because Markdown has no such thing, apart from raw HTML, which we refuse.

### A8. Columns

- **Making them:** `/` offers two and three columns. A cursor inside columns gets tools for 2, 3, 4 or 5 columns, and for Equal, Wide first and Wide last.
- **Changing the number:** adding gives an empty column. Removing moves the content of the removed columns into the last one that stays, so nothing is lost.
- **Removing:** one button dissolves the columns, and their content follows one below the other.
- **Keyboard:** Tab does not leave a column by itself, arrows move through the text. Backspace cannot cross a column edge, because columns are isolating.
- **Narrow screens:** columns stack in reading order.
- **While editing:** a thin dashed outline shows each column. Readers see no outline.
- **Edge cases:** columns inside columns are not offered. A table in a narrow column scrolls inside its frame.

### A9. Decisions

A line with a clear mark that records a decision, made by `/` or by typing `<>` and a space. Enter makes another decision, and Enter on an empty one returns to normal text. Comparison and search treat it as text. Markdown writes `**Decision:** text`. A report across pages comes with dynamic blocks in wave D.

### A10. Status marks

A short coloured word in a line.

- **Making one:** `/status` inserts a mark with the word "Status" and selects it.
- **Changing:** with a mark selected, the row shows a text field and six colours. The field keeps its last valid value, because a mark without words is refused.
- **Limits:** at most 40 bytes.
- **Accessibility:** the words are the content, so colour is never the only meaning.
- **Markdown:** the word as plain text.

### A11. Dates

A date in a soft box, in the reader's own format.

- **Making one:** `//` or `/date` inserts today.
- **Changing:** with a date selected, the row shows a native date field, so keyboard and screen readers work.
- **Storing:** `YYYY-MM-DD`, with no time zone, so a date never shifts by a day. Shown with `toLocaleDateString`, and read in local time on purpose.
- **Markup:** a `time` element with `datetime`.
- **Markdown:** the ISO date.

### A12. Emoji

Typing `:` after a space and at least one letter opens a list, filtered on names and keywords. Enter inserts the character. `:` alone does not open it, so a sentence with a colon is left alone. `/emoji` types the `:` for you. The list has about 150 common emoji in our own data file. It is stored as text, so search, Markdown and copy all work with nothing extra.

### A13. Links

- **Dialog:** three fields: **Find a page**, **Link address**, **Text**.
- **Find a page:** typing shows up to 8 pages from search, each with its space. Choosing one fills the address with `/pages/<id>` and, if empty, the text.
- **Text:** starts with the selection. Changing it replaces the words. With no selection, the words default to the address.
- **Keyboard:** Ctrl K opens it. Enter in Find a page picks the first result. Enter elsewhere saves.
- **Typing:** `[words](address)` becomes a link, for allowed addresses.
- **Edge cases:** an address with a scheme we refuse shows a clear error in the dialog, instead of failing at publish. Search only returns what the person may see. A relative address stays relative. A link to a page keeps working after a rename, because it holds the id. The link keeps its words, which may go stale: live titles are in wave B.

### A14. Images

With an image selected, the row shows Size (Small, Medium, Large, Original), alignment (Start, Centre, End) and **Describe**, which opens a dialog with **Alt text** and **Caption**. Images show in a `figure` with a `figcaption`. The caption and alt text are searchable. Markdown writes `![alt](file "caption")` and reads it back. On a narrow screen an image never exceeds the column.

### A15. Tables

Inside a table the row offers: add and delete row and column, delete table (today), and new: **Merge or split cells**, **Header row**, **Header column**, **Cell colour** (the six highlight colours), **Equal columns**, **Sort ascending** and **Sort descending**.

- **Column width:** drag the border between columns. Equal columns gives the alternative that WCAG asks for.
- **Sorting:** by the column with the cursor, using natural order so 2 comes before 10. Header rows stay on top. A table with merged cells cannot be sorted, and then the tools are absent.
- **Colours:** stored as names, and cleared with None.
- **Comparison:** a table is one block in the comparison, with spaces between cells.
- **Markdown:** a table with merged cells exports as far as Markdown can, colours are left out.

### A16. Code blocks

Inside a code block the row offers a **Language** list (with Plain text), **Line numbers**, **Wrap** and **Title**. Readers get a header with the title and language, and a **Copy** button that says when it worked or failed.

- **Copy** uses the clipboard API, needs a secure address, and shows a message when the browser refuses.
- **Line numbers** sit in a gutter that screen readers skip, and are not copied.
- **Always left-to-right**, as today.
- **Keyboard:** Tab inserts two spaces inside a code block in the editor, and Escape then Tab leaves it, so nobody is trapped.
- **Edge cases:** an unknown language shows plain text. An empty block still shows its frame.

### A17. Find and replace

Ctrl F, or the Find tool, opens a bar under the row.

- **Find:** marks all matches, shows "3 of 12", and scrolls to the current one. Enter goes to the next, Shift Enter to the previous.
- **Options:** Match case. Whole word is left out until someone asks.
- **Replace:** one field, **Replace** and **Replace all**. Replace all is one Undo.
- **Closing:** Escape closes the bar, removes the marks, and returns to the text.
- **Limits:** text in the document only. Inline blocks such as status marks break a match, and titles of sections are not searched.
- **Accessibility:** the count uses `role="status"`. Matches also have an outline, so colour is not the only sign.
- **Edge cases:** an edit while the bar is open recounts. An empty field shows nothing. Find uses the browser's own search outside the editor.

### A18. Keyboard table

| Keys (Ctrl or Cmd) | Does | From |
|---|---|---|
| B, I, U | Bold, italic, underline | library |
| Shift S | Strikethrough | library |
| E | Inline code | library |
| , and . | Subscript, superscript | new |
| Shift H | Highlight in yellow | new |
| \ | Clear formatting | new |
| K | Link | new |
| Alt 0 to 6 | Text, Heading 1 to 6 | library |
| Shift 7, 8, 9 | Numbered, bullet, task list | library |
| Shift B | Quote | library |
| Alt C | Code block | library |
| Shift L, E, R | Align start, centre, end | new |
| D | Duplicate the block | new |
| . (with Shift) | The block menu | new |
| F | Find | new |
| Enter | Publish | new |
| S | Save the draft now | new |
| Alt with Up, Down (no Ctrl) | Move the block | new |
| Alt F10 (no Ctrl) | Go to the tools | new |

The list of shortcuts itself (`?` and Ctrl /) comes with wave B.

## Document format

| Name | Kind | Attributes | Rules |
|---|---|---|---|
| `panel` | block | `kind` | Required, one of five. Any blocks inside. |
| `expand` | block | `title`, `open` | Title at most 500 bytes. |
| `columns` | block | `layout` | 2 to 5 `column` children. `wide-start` or `wide-end`. |
| `column` | block | none | Only inside `columns`. |
| `decision` | block | none | Text and inline items. |
| `status` | inline | `text`, `color` | Both required. |
| `date` | inline | `value` | A real date, `YYYY-MM-DD`. |
| `subscript`, `superscript` | mark | none | |
| `textColor` | mark | `color` | One of twelve. Refused otherwise. |
| `highlight` | mark | `color` | One of six. Refused otherwise. |
| paragraph, heading | attributes | `align`, `indent` | `center` or `end`, and 1 to 6. Others dropped. |
| image | attributes | `size`, `align`, `caption` | From lists, caption at most 500 bytes. |
| cell, header | attribute | `background` | One of six. |
| code block | attributes | `title`, `line_numbers`, `wrap` | |

No schema version change: everything is additive, and old documents stay valid. A node on an older release refuses new blocks while two versions run side by side, as with file tiles.

## Design additions

- **Tokens:** `--color-text-*` (12), `--color-mark-*` (6), `--size-image-*` (3), in both themes, with contrast pairs in `check-design.mjs`.
- **Style sheet:** `web/src/design/blocks.css`.
- **Icons:** own drawings for alignment, indent, panels, colour, highlight, sort, grip and more.
- **Pictures:** `editor` shows the new row. A new document with every block, in light, dark and narrow.
- The look of Helder and Rail stays. This adds to the standard, and needs no change to it.

## Build order

1. Document format, Markdown, search and comparison. **Done first, tests green.**
2. Split `editor.tsx` into a folder with one job per file, with no change in behaviour.
3. Tokens, style sheet and icons.
4. Blocks and marks: format, then reading, then writing.
5. Selection tools in the row, and roving focus.
6. Slash menu and Insert dialog.
7. Block handle and menu.
8. Link dialog, then find and replace, then emoji.
9. Pictures, accessibility pass, full gates, update 03.

## Test plan

| Level | What |
|---|---|
| Go unit | Allow-list per block, refused values, dropped values, text for search and comparison, Markdown both ways, fuzz seeds with new blocks |
| Integration | Publishing every block through the API. Export and import of a panel. |
| Mutation | Free colours accepted, one column accepted, a bad date accepted |
| Vitest | Filtering of the slash list, sorting a table, match finding, moving a block, changing the column count |
| Browser | Slash menu by keyboard and mouse, every block survives publish and reload, block moves by menu and keys, find and replace with undo, reader opens a section by keyboard, copy button, roving focus through the tools, page link after a rename |
| Accessibility | axe on the editor with every block and in reading mode, in both themes |
| Pictures | Editor, all blocks, dark, narrow, right-to-left |

## Permissions and audit

No new rule. Everything happens in the editor, which needs *edit*. The document passes the same gate, and saving and publishing write the same events. Page search in the link dialog uses the normal search, so it only shows what the person may see.

## Acceptance criteria

- [x] AC-01 `/` opens the menu, filters, inserts with Enter and with a click, and Escape keeps the text
- [x] AC-02 `/` does not open in a code block, after a digit, or in an address
- [x] AC-03 Every new block survives publishing and reloading, and shows in reading mode
- [x] AC-04 A free colour, an unknown panel kind, an invalid date and one column are refused
- [x] AC-05 Text in panels, sections, columns, decisions and status marks is searchable, and shows in comparison with spaces between blocks
- [x] AC-06 A panel survives Markdown export and import
- [x] AC-07 Every text colour and highlight passes contrast in light and dark
- [x] AC-08 A block moves by dragging, by the menu and with Alt and the arrows, and Undo reverses it
- [x] AC-09 Find marks all matches, Replace all replaces them, and one Undo restores them
- [x] AC-10 A link to a page keeps working after a rename
- [x] AC-11 A reader opens a section with the keyboard, and copies a code block
- [x] AC-12 Merge, split, header toggles and cell colour survive publishing
- [x] AC-13 Sorting keeps header rows on top, and uses natural order
- [x] AC-14 The row of tools is one Tab stop with arrow keys
- [x] AC-15 Changing the number of columns loses no content
- [x] AC-16 axe finds nothing in the editor and in reading mode with every block

## How the product owner tests it

Open a page, choose Edit, and type `/`. Try each entry. Colour some text and highlight it. Make a panel, change its kind, and type `!!` at a new line. Make a section and two columns. Hover over a block and drag it by its handle, then use its menu. Press Ctrl F, replace a word everywhere, and press Ctrl Z once. Add a link to another page, publish, and rename that page. Read the page in dark mode, open the section with the keyboard, and copy a code block. Press Tab from the title, and walk through the tools with the arrow keys.

## Risks

| Risk | Answer |
|---|---|
| The node views clash with the strict Content Security Policy | Every browser test fails on a violation, and styles use classes |
| The size of the row on small windows | It wraps, and a picture guards a narrow window |
| Our own drag handle misbehaves with tables or columns | Top-level only, and the menu always works |
| Many new marks make Markdown lose detail | Stated in the spec, and export never loses words |
| The bundle grows | One small package, and the emoji list is about 150 entries |

## Provenance

`docs/blueprint/03-functional/04-editor.md` and its sources. For this plan, read on 2026-11-05 for behaviour only: Docmost's public editor documentation (`docmost.com/docs/user-guide/editor`), Notion's help pages on writing, columns, styling and shortcuts (`notion.com/help/...`), and Google's help on find and replace. Standards and library documentation: WAI-ARIA Authoring Practices (toolbar, combobox, menu button, menu and menubar, disclosure), WCAG 2.2, MDN on `details` and the clipboard, the ProseMirror reference, TipTap's documentation (Suggestion, tables, React node views, drag handle), GitHub's documentation on alerts, and the Unicode CLDR pages. No source code of any other product was read. Own design.
