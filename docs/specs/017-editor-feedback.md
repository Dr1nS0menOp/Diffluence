# 017: The editor after the product owner's first test

Status: built, waiting for the product owner's acceptance session
Phase: feedback on waves A and B. The product owner tested the editor on 2026-11-05 and asked for six changes.

## Problem

The product owner's words, shortened. A table needs a button of its own, with a clear outline and a soft grid, and that button opens a rich menu, because the row of tools gets far too busy. Tables must stretch all the way, "extreme full width". The Page look button is not good: the page should scale by itself, with a Full width button in the row of tools. Header images stay, but not behind a button at the top right. Code blocks need many more languages. The other pages of the space may stay visible in the editor, because there is room.

## What the study found

**How writers expect it.** Other tools put table actions on the table, not in the main bar. Help pages and public questions show the same wish again and again: a table that can go wider than the text, snapping to the page edges, with a page width that is chosen separately. A table's width and its columns' widths are two different things, and people mix them up when a tool offers only one. Full width pages in other tools give tables no choice at all, which people complain about, so here the two stay independent.

**What accessibility demands.** From WCAG 2.2 and the WAI-ARIA Authoring Practices:

| Source | Demand | What we do |
|---|---|---|
| Menu button pattern | `aria-haspopup`, `aria-expanded`, Enter opens and moves to the first item | `Menu` does this. The table button is a `Menu` |
| 2.5.7 Dragging | Everything dragged has another way | Stretching has presets in the menu and arrow keys on the edge. The header image has a range field |
| Window splitter pattern | `role="separator"` with `aria-valuenow`, Left and Right move it, Home and End jump | The edge of a table is such a separator, from 0 to 100 |
| 1.4.10 Reflow | Tabular data is excepted, but a wide table sits in its own scrolling frame, and the page around it still reflows | Every table scrolls in its own frame, also when stretched |
| 1.4.8 Visual presentation (AAA) | Lines of at most 80 characters, by a mechanism that may be the browser | We aim for AA. The fluid column gets longer lines on large screens, which a person can undo by narrowing the window. Written down as a known choice in D3 |
| 2.5.8 Target size | At least 24 by 24 CSS pixels | Table button, swatches and the edge |
| 1.4.11 Non-text contrast | Controls need 3:1 | The outline uses `--color-guide` on the page, and the active outline uses the accent. Grid lines are decoration, the text keeps its own contrast |
| 1.4.1 Use of colour | Colour alone may not carry meaning | Swatches have names |

**What the libraries allow.** TipTap's table uses one node view for readers and writers, so a subclass can carry the width to both. A cell selection survives a command that does not ask for focus, so the menu can stay open while rows are added. `prosemirror-tables` gives fixed pixel columns a fixed table width, so a stretch has to scale those columns, or the table would not follow. `lowlight` ships 37 languages in `common` and about 190 in `all`, as separate modules, so all of them can load on demand in their own file. Container query units measure the frame around the page, so a table can reach that width from inside a narrow column with plain CSS.

**Safety.** The width is a number between 1 and 100, checked by `Normalize`, and turned into a style by our own code. A document still cannot carry a style. A language stays limited by the pattern in `document.go`, which all names of the library pass, and it only ever selects a grammar. The header image stays an image of the same page (spec 009).

**Leaving the editor.** The tree in the editor makes it easy to leave. Today a pending autosave is cancelled when the screen goes away, so up to 1.5 seconds of typing would be lost.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | One button on the table, at its top corner, opening one `Menu` with groups. The menu stays open while it is used | Adding three rows must not mean opening it three times |
| D2 | All table tools leave the row. Only inserting a table stays there | The product owner's request. The row stays calm |
| D3 | The page scales by itself: `clamp(47.5rem, 64cqi, 64rem)`. Full width removes the limit | The product owner's request, replacing the fixed column of ADR-021. Long lines on very large screens are the price, and the cap limits them |
| D4 | Table width is one number, `stretch`, from 0 (the text column) to 100 (the width of the frame). Presets are 0, 50 and 100 | A name list cannot do "fully stretchable", and pixels would mean nothing on another screen. The number stays relative to any window |
| D5 | Stretching is only for tables at the top of the document | A table in a column or panel has nothing to stretch into |
| D6 | A stretch scales fixed column widths by the same factor | Otherwise the frame grows and the table does not |
| D7 | A page with a stretched table hides the contents list | The table needs that room, and overlapping them would hide one of the two |
| D8 | Emoji, header image and page options sit at the title, quiet until hover or focus | The product owner's request. They belong to the title, and touch screens always show them |
| D9 | Title alignment and child pages are in a small Page options menu beside them | They are rarely used, and now have a home that is not a dialog |
| D10 | The header image is placed by dragging it, or with a range field | The field works with one click and with the arrow keys (2.5.7) |
| D11 | All languages load on demand, once a block needs one or the picker opens | About 190 grammars are 743 kB (248 kB gzip) in a file of their own, which would add 60 percent to the first load |
| D12 | The language picker is a dialog with a search field, and languages used on this page come first | A plain list of 190 cannot be used. No list of searches is kept in the browser, after SEC-265 |
| D13 | Automatic language detection stays limited to the common languages | With all of them, every keystroke in an unmarked block would try 190 grammars |
| D14 | The Focus frame keeps the sidebar, as far as there is room. It follows the same switch as everywhere else | The product owner's request, replacing "the sidebar goes" of ADR-021 |
| D15 | Leaving the editor saves first. If that fails, the text stays in the browser, in the backup of wave D, and a message leads back to it | The tree makes leaving easy, so text must never depend on timing |
| D16 | Only a change someone made marks a page as changed. The empty line after a last block does not count | That line arrives by itself on the first click, so saving on leaving left a draft on pages nobody touched |
| D17 | The table button stays in reach below the bars while a long table scrolls, and a menu that opens upwards stops at those bars | Found while building: the first items of the menu lay under the bars, and in a long table the button and Ctrl Shift . led nowhere |
| D18 | On a stretched table the block handle moves to the start corner, as the table button is on the other corner | Found in a picture: beside the text it sat on the cells |
| D19 | Bars wrap on a narrow window, so the table button measures how far they reach | A fixed size put the button under a wrapped row of tools |
| D20 | With Page look gone, labels sit below the text in the editor | The same place as for readers |

## Scope

**In:** EDT-096, EDT-097, EDT-105 and the table rows of chapter 4 that move into the menu, PAG-032, PAG-107, PAG-124, NAV-001, and new requirements EDT-159 to EDT-164.

**Out, and where it goes:**

| Item | Goes to |
|---|---|
| Handles on each row and column (EDT-087), plus between rows (EDT-088) | Wave E, with the rest of chapter 4 |
| Readers sorting a table | Wave E |
| Text alignment per cell | Not asked. The alignment tool of the row already works in a cell |

## Feature plan

### F1. The table button and its menu

- **Showing:** the button sits on the outline at the top corner of the table the cursor is in, or the one under the pointer. On touch screens it shows when the cursor is in a table.
- **Before acting:** with the pointer on another table, opening the menu first puts the cursor in that table.
- **Menu:** groups Rows, Columns, Cells, Colour, Table, with these items:

| Group | Items |
|---|---|
| Rows | Add row above, Add row below, Delete row, Header row |
| Columns | Add column before, Add column after, Delete column, Header column, Sort ascending, Sort descending |
| Cells | Merge or split cells |
| Colour | Swatches for the cell, the row and the column, each with None |
| Table | Width: text, wide, full. Equal columns. Duplicate table. Delete table |

- **Stays open:** actions leave the menu open. Escape and a click outside close it. Delete table closes it.
- **Keyboard:** Ctrl Shift . in a table opens this menu, and elsewhere the block menu. Arrows, Home, End and letters move. Every swatch has a name. Those of the cell are `menuitemradio`, because a cell has one colour, and those of a row or a column are plain actions. Escape returns to the text.
- **Edge cases:** Sort is left out when cells are merged. Width is left out for a table inside another block.

### F2. The look of a table

An outline in `--color-guide` with rounded corners, and a grid in `--color-line`. The table the cursor is in gets an accent outline. This holds for readers too, without the accent.

### F3. Stretching

- **Attribute:** `stretch` on `table`, a whole number from 1 to 100, left out at 0. Anything else is dropped by `Normalize`.
- **Width:** the column plus that share of the room up to the frame, centred on the column, on both sides.
- **Dragging:** the end edge of the table, in steps of 5, shown while dragging. It is a separator from 0 to 100 with Left, Right, Home and End.
- **Menu:** Text width, Wide (50), Full width (100).
- **Narrow screens:** the column already fills the screen, so the number changes nothing, and the table scrolls in its frame.
- **Export:** Markdown has no width, so it leaves it out, as it does with colours.
- **Edge cases:** one Undo reverses a stretch. A full width page has no room left, so the number changes nothing there. Fixed columns scale with it (D6).

### F4. Width of the page

- **Scaling:** a page and the editor use `--size-page`: at least the old reading width, 64 percent of the frame, at most 64rem.
- **Full width:** a Full width tool in the row, pressed when on. It applies at once, as all page look does (ADR-025), and removes the limit. The text then runs to the padding of the frame.
- **Other screens:** settings, history and lists keep their fixed column.

### F5. The title area

- **Controls:** above the title: Add emoji, Add header image, and Page options. They show on hover and on focus, and always on touch screens.
- **Emoji:** beside the title as a button once chosen. A small dialog holds the search, the grid and No emoji.
- **Header image:** shows above the controls. Over it: Change, Remove, and the range field. Dragging it up or down moves it. A value is saved a moment after the last change, so one drag is one audit event.
- **Page options:** Centre the title, Show child pages, as `menuitemcheckbox`.
- **Page look** as a button and dialog goes away.

### F6. Languages

- **Picker:** a tool that shows the language and opens a dialog with a search field. Enter takes the first match. Plain text is always first, then languages used on this page.
- **Aliases:** typing `js` or `c++` finds the language too.
- **Reading:** a block in a language that is not yet loaded shows plain, then gets its colours when the grammars arrive.
- **Failure:** when the file cannot load, the block stays readable, and the picker says so.

### F7. The tree in the editor

- **Frame:** the sidebar of the space beside the editor, with the edit strip and the tools in the same column as the sheet.
- **Hidden sidebar:** a thin rail holds the button that brings it back, and `[` works here too.
- **Leaving:** Close, a row of the tree, or any link saves a pending change first (D15).

## API

No new routes. `PATCH /pages/{id}/look` already takes everything.

## Data

No migration. `stretch` lives in the document.

## Build order

1. Go: `cleanTable`, with tests first.
2. Table extension, width maths, tests.
3. Table button and menu, row of tools, keys.
4. Tokens, page width, Full width tool.
5. Title area.
6. Languages.
7. Focus frame, saving on leaving.
8. Example space, docs, pictures.

## Test plan

- **Go:** `stretch` accepted from 1 to 100, and dropped at 0, 101, text and fractions. Language names of the library pass.
- **Vitest:** the width maths, scaling columns, stretch from a pointer, sorting of languages.
- **Browser:** each part above, with the keyboard first. Leaving through a row within a second of typing keeps the text. A failed save keeps the text.
- **Mutation:** a table with any width gets through.
- **Pictures:** the editor with its tree, the table menu, the title area, a stretched table for readers, a large screen.
- **Accessibility:** axe with the menu and the dialogs open.

## Acceptance criteria

- [x] AC-1 A table shows a clear outline and a soft grid, for readers and for writers.
- [x] AC-2 The button on a table opens one menu, by mouse and by Ctrl Shift ., and every table action is in it.
- [x] AC-3 The row of tools no longer changes when the cursor enters a table.
- [x] AC-4 The menu stays open while rows are added.
- [x] AC-5 A table stretches to the full width by dragging, by the menu and by arrow keys, and readers see the same.
- [x] AC-6 A stretched table scrolls in its own frame on a narrow screen.
- [x] AC-7 The page gets wider on a large screen by itself, and Full width in the row of tools takes all the room.
- [x] AC-8 Page look is gone as a button. Emoji, header image and page options work from the title.
- [x] AC-9 The header image moves by dragging and by the range field, and that survives a reload.
- [x] AC-10 A code block can be set to Rust through a search, gets colours, and keeps them for readers.
- [x] AC-11 The tree shows in the editor, and can be hidden.
- [x] AC-12 Leaving through a row at once keeps what was typed.
- [x] AC-13 Everything passes axe, and the new pictures have been looked at.

## Risks

| Risk | Answer |
|---|---|
| Container units behave differently inside a frame with containment | Tested in the browser on a real page, for readers and writers |
| A menu that stays open fights with focus | Its commands do not take focus, and tests walk it by keyboard |
| 190 grammars in the first load | Checked in the build output |
| The block handle sits on a stretched table | It did, in a picture, so it moved to the corner (D18), with a browser test |
| Lines get long on very large screens | The cap of D3, and it can be changed in one token |

## How the product owner tests it

1. Open a page with a table in the editor, and click in the table. Press the button at its corner.
2. Add three rows without reopening the menu. Colour a column. Pick Full width.
3. Drag the end edge of the table. Publish, and read the page.
4. Make your window wider and narrower. Press Full width in the row of tools.
5. Hover over the title: add an emoji and a header image, and drag the image.
6. In a code block, pick the language and search for rust.
7. Type a word and click another page in the tree at once. Go back: your word is there.

## Provenance

Own text from these public sources, read for behaviour only:

- W3C WCAG 2.2 Understanding 1.4.8 Visual Presentation, and 1.4.10 Reflow.
- W3C WAI-ARIA Authoring Practices: menu button, window splitter.
- Public help and community pages on table width: `community.atlassian.com` articles on resizing tables and on full width pages, `jira.atlassian.com/browse/CONFCLOUD-79811`, the ClickHelp user manual on resizing columns, the KnowledgeOwl help on Make Full Width. Read for what people ask for, and no text, layout or code was taken.
- TipTap and `lowlight` as installed in `node_modules`: their own code and types, which are our dependencies.

Not confirmed from a source: how container units combine with containment in every browser. The browser tests decide that for the browser we test in.
