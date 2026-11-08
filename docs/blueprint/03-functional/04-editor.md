# 03.4: The editor

The editor is where people spend their time. The look is settled (Rail, ADR-021): a strip with the page's fate at the top, the tools below it, and an open sheet with no boxes. This chapter describes what it can do.

## 4.1 The surface

- **EDT-001** · built · The strip holds Close with the save status on one side, and the summary, Discard draft and Publish on the other.
- **EDT-002** · built · The tools sit in one row below the strip. Both stay in view while scrolling, except on narrow screens.
- **EDT-003** · built · The title is a large plain input with no border. The body follows at most 50rem wide.
- **EDT-004** · built · An active tool is solid accent, so its state is clear without reading.
- **EDT-005** · built · Table tools appear only inside a table.
- **EDT-006** · new · The tools can be pinned to the top or hidden. When hidden, a small floating bar appears over selected text. The choice is kept per person.
- **EDT-007** · built · Tools that do not fit fold into a **More** menu at the end of the row, in a fixed order. Nothing is ever cut off.
- **EDT-008** · built · A person sets **Normal** or **Full width** for the page from the strip. The sheet shows the width readers will get.
- **EDT-009** · built · The title field gets the focus on a new page, and Enter moves to the body. In the body, the first Backspace at the very start moves back to the title.
- **EDT-010** · built · Hovering over the title reveals **Add emoji** and **Add header image** (PAG-123).
- **EDT-011** · built · A word count and read time show at the bottom of the sheet on request.
- **EDT-012** · built · The status of the page can be set above the title (PAG-054).
- **EDT-013** · built · The strip shows who else is editing, in phase 4 (COL-088).
- **EDT-014** · built · Clicking anywhere below the end of the text puts the cursor at the end, and adds a paragraph if the last block is not text.

## 4.2 Ways to add something

- **EDT-015** · built · **Slash menu:** typing `/` at the start of a line or after a space opens a menu of everything that can be added, right at the cursor.
- **EDT-016** · built · Typing after the `/` narrows the list by name and by synonyms. Arrow keys move, Enter inserts, Escape closes and leaves the typed text alone.
- **EDT-017** · built · The menu shows a group heading, an icon, a name and a one-line description. Recently used items come first.
- **EDT-018** · built · The menu does not open inside a code block, or when a person types an address or a date such as `1/2`.
- **EDT-019** · built · **Plus button:** hovering over an empty line shows a plus button that opens the same menu, for people who prefer the mouse.
- **EDT-020** · partly · **Insert menu:** a button in the tools opens the same list, grouped, with a search box. A **Browse all** entry shows every block with a preview. Still open: a preview of blocks that show data of a page or a space. Their hint says what they do.
- **EDT-021** · built · **Block handle:** hovering over any block shows a handle at its start. Dragging the handle moves the block, also into another column.
- **EDT-022** · built · **Block menu:** selecting the handle opens a menu for that block: turn into another type, copy link to this block, duplicate, move up, move down, delete.
- **EDT-023** · built · **Turn into:** text becomes a heading, list, quote, panel, code block or collapsible section, and back. The menu suggests the most likely changes first.
- **EDT-024** · built · Holding Shift and selecting a second block selects everything between them. Turn into, move and delete then work on all of them.
- **EDT-025** · built · Everything the block handle does also works from the keyboard: a shortcut opens the block menu for the block with the cursor, and Alt with the arrows moves it.
- **EDT-026** · built · **Block templates:** a person saves a selection as a named block and inserts it later from the slash menu. It is private unless shared.

## 4.3 Typing shortcuts

- **EDT-027** · built · These work as you type and stay editable after they apply:

| Type | Result | Status |
|---|---|---|
| `#` to `######` and a space | Heading 1 to 6 | built (the text style menu offers 1 to 3; see EDT-034) |
| `-`, `*` or `+` and a space | Bullet list | built |
| `1.` and a space | Numbered list | built |
| `[ ]` and a space | Task | built |
| `>` and a space | Quote | built |
| three backticks | Code block, with optional language | built |
| `---` | Horizontal rule | built |
| `**text**` | Bold | built |
| `*text*` or `_text_` | Italic | built |
| `~~text~~` | Strikethrough | built |
| `` `text` `` | Inline code | built |
| `[text](address)` | Link | new |
| `@` | Mention a person | phase 4 |
| `:` | Pick an emoji | new |
| `//` | Pick a date | new |
| `<>` and a space | Decision | new |
| `!!` and a space | Panel | new |

- **EDT-028** · built · Backspace straight after a shortcut applied undoes it and restores the typed characters.
- **EDT-029** · built · Common text faces such as `:)` become emoji. A person can turn that off in their preferences.
- **EDT-030** · built · `->`, `<-`, `...` and quotes become typographic characters, and the same setting turns that off.

## 4.4 Keyboard shortcuts in the editor

- **EDT-031** · built · Built: the table below.

| Keys (Ctrl or Cmd) | Does |
|---|---|
| B, I, U | Bold, italic, underline |
| Shift S | Strikethrough |
| E | Inline code |
| , and . | Subscript, superscript |
| Shift H | Highlight |
| \\ | Clear formatting |
| K | Add or edit a link |
| Alt 0 to 6 | Normal text, Heading 1 to 6 |
| Shift 7, 8, 9 | Numbered, bullet, task list |
| Shift B | Quote |
| Alt C | Code block |
| Shift L, E, R | Align to the start, centre, end |
| D | Duplicate the block |
| Shift . | The block menu; in a table, the table menu |
| Shift Backspace | Delete the block |
| F | Find |
| S | Save the draft now |
| Enter | Publish |
| Z, Shift Z | Undo, redo |
| Alt Up, Alt Down (no Ctrl) | Move the block |
| Alt F10 (no Ctrl) | Go to the tools |

- **EDT-032** · built · Escape leaves the current menu. A second Escape moves the focus from the body to the tools.
- **EDT-033** · built · Tab indents a list item and Shift Tab outdents it. Outside lists and tables, Tab leaves the editor, so a keyboard user is never trapped.

## 4.5 Text

- **EDT-034** · built · Text styles: Paragraph and Heading 1 to 6. Built: Heading 1 to 3 in the menu, 1 to 6 in the document. New: all six in the menu.
- **EDT-035** · built · Marks: bold, italic, underline, strikethrough, inline code.
- **EDT-036** · built · Marks: subscript and superscript.
- **EDT-037** · built · **Text colour:** a palette of 12 named colours and Default. Colours are stored as names, not values, so each has a light and a dark version that passes contrast.
- **EDT-038** · built · **Highlight:** a palette of 6 named background colours and None, with the same rule.
- **EDT-039** · built · **Clear formatting** removes all marks from the selection.
- **EDT-040** · built · **Alignment:** left, centre or right for paragraphs and headings. In right-to-left text the choices mirror.
- **EDT-041** · built · **Indent** moves a paragraph one step in, up to six steps. Buttons and keyboard both work.
- **EDT-042** · built · **Direction:** a block can be marked left-to-right or right-to-left when languages are mixed.
- **EDT-043** · built · A hard line break comes from Shift Enter.
- **EDT-044** · built · A **Heading anchor** is created for each heading from its text, stable across small edits, and used for links to a heading.

## 4.6 Lists and tasks

- **EDT-045** · built · Bullet, numbered and task lists can be nested to any depth.
- **EDT-046** · built · A numbered list can start at any number.
- **EDT-047** · built · Bullets and numbers change style by level, for easier reading.
- **EDT-048** · built · Dragging a list item moves it with everything nested below it.
- **EDT-049** · phase 4 · A task can have a person and a due date, typed with `@` and `//` (chapter 7).
- **EDT-050** · built · In reading mode, a person with *edit* can tick a task without opening the editor. The change is a small version with no notification.
- **EDT-051** · built · An overdue open task shows its date in the danger colour.
- **EDT-052** · built · Converting a list to a task list, and back, keeps its text and nesting.

## 4.7 Links

- **EDT-053** · built · Links can use http, https, mailto or a relative path. Other schemes are refused, in any spelling.
- **EDT-054** · built · A link is added through our own dialog with the address. Removing a link is in the same dialog.
- **EDT-055** · built · The dialog also has a text field, for the words that show.
- **EDT-056** · built · The dialog searches pages while typing, shows recent pages first and offers them as links. The link then points to the page id, so it survives a rename or move.
- **EDT-057** · built · A link to a page shows the current title automatically, unless the writer typed their own words.
- **EDT-058** · partly · A link can point to a heading or a block in the same or another page. Still open: a block without any text, such as an image, which has nothing to hold an anchor.
- **EDT-059** · built · Links to pages the reader may not see stay as plain words with no title leak. The address shows nothing about the page.
- **EDT-060** · built · Links to deleted pages show as struck through, with an explanation on hover.
- **EDT-061** · built · Addresses that are typed or pasted become links on their own, and open in a new tab with `noopener`.
- **EDT-062** · built · Pasting an address over selected text makes that text a link.
- **EDT-063** · built · Selecting a link shows a small bar: open, copy, edit, unlink, and change display.
- **EDT-064** · built · **Placeholder link:** a link to a page that does not exist yet shows in a distinct colour. A person who may create pages follows it, and a new page starts with that title and the right parent. Space settings list all placeholders.
- **EDT-065** · built · **Link display:** a link to a page in this instance can show as plain text, as a short tag with an icon, or as a card with title, excerpt, space and date.
- **EDT-066** · built · A card or tag for an internal page is rendered per reader, so nobody sees a title they may not see.
- **EDT-067** · later · **Outside link cards:** the server never fetches outside addresses by default, because that leaks reading habits and invites request forgery. An administrator can allow a list of domains. Only those show a title and picture, fetched through a safe gateway.
- **EDT-068** · built · **Anchors:** a named anchor can be placed anywhere, for links from other pages (BLK-016).

## 4.8 Images

- **EDT-069** · built · An image is always an attachment of the page. External images are refused, because they would tell a third party who is reading.
- **EDT-070** · built · Only PNG, JPEG, GIF and WebP show as images, detected from the content.
- **EDT-071** · built · Images get in by the tools, drag and drop, or paste.
- **EDT-072** · built · **Size:** drag a corner, or type a width. Presets are small, medium, large and original. The reader never gets more than the width of the column.
- **EDT-073** · built · **Alignment and wrap:** left, centre or right, with or without text flowing around it.
- **EDT-074** · partly · **Caption:** a line of text under the image with simple formatting. Built: a plain caption. New: formatting in it.
- **EDT-075** · built · **Alt text:** a field for a description, offered when an image is added, because screen readers need it. A page with images without alt text gets a quiet hint in the editor.
- **EDT-076** · built · **Border:** none, thin or medium in a colour from the palette.
- **EDT-077** · built · **Link:** an image can link to a page or an address.
- **EDT-078** · built · **Inline image:** a small image can sit in a line of text, such as an icon.
- **EDT-079** · built · **Paste from a web page:** the image is downloaded by the browser where possible and stored as an attachment, or a clear note says it could not be taken along.
- **EDT-080** · partly · Selecting an image shows a small bar with these options, and Delete. Deleting it from the page does not delete the attachment. Built: the options, in the row of tools. New: a bar at the image.
- **EDT-081** · built · If the attachment no longer exists, the block shows a clear placeholder, not a broken picture.
- **EDT-082** · later · Basic cropping and rotating in the browser, stored as a new version of the attachment.
- **EDT-083** · built · **Video and audio** files that the browser can play show a player. They are served only from our own origin.

## 4.9 Tables

- **EDT-084** · built · A table starts with three columns, three rows and a header row. Rows and columns are added and removed from the menu on the table.
- **EDT-085** · built · Cells can hold any block, and can be merged in the document format.
- **EDT-086** · built · A size picker chooses rows and columns when adding a table.
- **EDT-087** · built · **Handles:** a handle on each row and column selects it, and opens its menu.
- **EDT-088** · built · **Insert:** a small plus between rows or columns inserts one exactly there.
- **EDT-089** · built · Tab moves to the next cell, and in the last cell adds a row.
- **EDT-090** · built · **Merge and split** cells from the cell menu.
- **EDT-091** · built · **Drag rows and columns** by their handle to reorder them. Shift selects several.
- **EDT-092** · built · **Header row and header column** can each be turned on or off.
- **EDT-093** · built · **Numbered column:** a first column that counts the rows automatically.
- **EDT-094** · built · **Sort** by a column, ascending or descending. In the editor this changes the table. Readers can sort a table for themselves without changing it.
- **EDT-095** · built · **Cell colour:** from the same palette, with light and dark versions.
- **EDT-096** · built · **Column width:** drag the border between columns. A button makes selected columns equal.
- **EDT-097** · built · **Table width:** from the width of the text up to the full width of the window, by dragging its end edge, from the menu, or with the arrow keys on that edge. It stays centred on the text.
- **EDT-098** · built · **Fixed or flexible columns:** fixed tables keep their layout and scroll on narrow screens, flexible tables scale.
- **EDT-099** · built · **Sticky header:** the header row of a long table stays visible while scrolling.
- **EDT-100** · built · **Copy and paste** of rows and columns, and of cells from a spreadsheet, which land as cells.
- **EDT-101** · built · A table cannot hold another table, and the tools say so.
- **EDT-102** · built · **Caption:** a line above or below the table.
- **EDT-103** · built · **Filter for readers:** a table with a header row can get a filter box that hides rows without changing the page.

## 4.10 Code

- **EDT-104** · built · A code block has a language, syntax highlighting and always reads left-to-right.
- **EDT-105** · built · A language picker with search, with the languages of this page first. **Plain text** is one of them. About 190 languages, which load when needed.
- **EDT-106** · built · The language can be detected, and the writer can change it.
- **EDT-107** · built · **Line numbers** can be on or off, with a starting number.
- **EDT-108** · built · **Wrap long lines** can be on or off.
- **EDT-109** · built · **Copy button:** a reader copies a block with one click.
- **EDT-110** · built · **Title:** an optional line above the block, such as a file name.
- **EDT-111** · built · **Fold:** a long block can start folded with a control to open it.
- **EDT-112** · built · Tab inside a code block inserts spaces, and selecting several lines indents them all. A shortcut leaves the block, so nobody is trapped.
- **EDT-113** · built · Pasting code into a code block keeps it exactly as it is.

## 4.11 Blocks that structure a page

- **EDT-114** · built · Quote and horizontal rule.
- **EDT-115** · built · **Panel:** a coloured box with an icon, for info, note, success, warning and error, or a neutral one with a chosen emoji and colour. It holds any block except another panel.
- **EDT-116** · built · **Collapsible section:** a title and hidden content. Readers open it with a click or the keyboard. It remembers nothing, and starts closed unless the writer chose open. It opens by itself when printing, exporting or following a link into it.
- **EDT-117** · built · Collapsible sections can be nested.
- **EDT-118** · built · **Columns:** two to five columns, each holding any block. Preset splits include equal, wide left, wide right and a narrow sidebar column. Changing the number keeps the content.
- **EDT-119** · built · Columns are also made by dragging a block beside another. A dashed line shows where it will land.
- **EDT-120** · built · **Column block width:** normal, wide or full.
- **EDT-121** · built · On narrow screens columns stack in reading order.
- **EDT-122** · built · **Decision:** a block that records a decision with a clear mark. It can have a person and a date. A report lists decisions across pages (BLK-037).
- **EDT-123** · built · **Spotlight:** a wide coloured band for an important message, with a heading, text and an optional button link.

## 4.12 Small items in a line of text

- **EDT-124** · built · **Status mark:** a short coloured word in a line, with a palette of six colours. Typing changes the word.
- **EDT-125** · built · **Date:** a date in a soft box, chosen from a calendar. It shows in the reader's format. A date in a task sets its due date.
- **EDT-126** · phase 4 · **Mention:** `@` opens a list of people and groups the person may see (COL-029).
- **EDT-127** · built · **Emoji:** `:` opens a list with search, and a toolbar button shows a picker with groups and recent emoji. Emoji are standard Unicode characters, so no images are needed.
- **EDT-128** · later · **Own emoji:** administrators upload small pictures that everyone can use. They are served only to people who are logged in.
- **EDT-129** · built · **Page link tag:** the inline form of EDT-065.

## 4.13 Charts

- **EDT-130** · built · A table can get a chart below it: bar, line, area or pie.
- **EDT-131** · built · The chart stays linked to its table and follows every change.
- **EDT-132** · built · Options: which column is the axis, which columns are the data, how to total repeated values, title, axis titles, legend on or off and its position, height, smooth lines, data points.
- **EDT-133** · built · Colours come from the chart palette with light and dark versions. Series are also told apart by shape or pattern, not by colour alone.
- **EDT-134** · built · Removing a chart leaves the table. Removing a table asks what to do with its chart.
- **EDT-135** · built · A table can be hidden by placing it in a collapsible section, and the chart stays.
- **EDT-136** · built · Every chart can also be read as a table by screen readers.

## 4.14 Paste and drop

- **EDT-137** · built · Pasted content goes through the same allow-list as everything else. Nothing unknown gets in.
- **EDT-138** · partly · Formatting from Word and web pages is cleaned up. Still open: a real Word and Google Docs file: this was tried on samples of their HTML, and nested lists arrive flat.
- **EDT-139** · built · **Paste as plain text** with Ctrl Shift V.
- **EDT-140** · built · Pasted Markdown is recognised and turned into blocks. An undo restores the plain text.
- **EDT-141** · built · Files dropped anywhere in the editor, also beside the page, become part of the document. Several at once are kept in order (chapter 6).
- **EDT-142** · built · A pasted image becomes an attachment.
- **EDT-143** · partly · A copied block from another Diffluence page keeps its blocks and marks. Attachments stay linked if the reader may see them, or are copied along. Still open: attachments of another page. Blocks and marks come along through the clipboard, and a file keeps pointing at its own page.

## 4.15 Find, undo and the rest

- **EDT-144** · built · **Find** opens a bar at the top of the sheet. It marks all matches, shows how many, and moves with Enter.
- **EDT-145** · built · **Replace** replaces one or all matches. Options: match case, whole word.
- **EDT-146** · built · Undo and redo work across all edits in the session.
- **EDT-147** · built · The browser's spell checker is on, and respects the language of the page.
- **EDT-148** · built · **Page language:** a page can state its language, which sets the spell checker, the hyphenation and what screen readers pronounce.
- **EDT-149** · partly · **Comment while editing:** selecting text in the editor allows an inline comment. It stays with the draft until publishing. Built: on a page that was published before. New: on a page that never was.
- **EDT-150** · partly · A long page keeps typing smooth: 100,000 words stay usable, and the tools do not freeze. Measured in spec 030: a page of 100,000 words opens to read in 0.4 s and to edit in 0.5 s, and typing takes about 10 ms a key, in headless Chromium on one machine. Still open: the tools were not measured on their own, and nor was a slow device.
- **EDT-151** · built · **Word count** counts the selection when there is one.

## 4.16 Safety of the document

- **EDT-152** · built · The source of truth is a JSON document with a schema version. Markdown is a way to type, import and export, not the stored format.
- **EDT-153** · built · The server accepts only known nodes, marks and attributes, drops other attributes, and refuses what it does not know.
- **EDT-154** · built · A document is at most 5 MiB and 100 levels deep.
- **EDT-155** · built · Every new block in this chapter is added to that allow-list with tests, and to Markdown export, comparison and search.
- **EDT-156** · built · Colours, sizes and alignment are stored as names from a short list, never as free values, so no document can carry styles.
- **EDT-157** · built · An older document always opens in a newer release, upgraded on read.
- **EDT-158** · built · Blocks that a release does not know, for example from a newer linked instance, show as a clear placeholder and are kept unchanged.

## 4.x After the product owner's first test (spec 017)

- **EDT-159** · built · A table has a clear outline and a soft grid, for readers and for writers, and the table with the cursor gets an accent outline.
- **EDT-160** · built · One button on the table opens a menu with everything for tables: rows, columns, cells, colours and width. It stays open while it is used, and Ctrl Shift . opens it from the keyboard. The button stays in reach while a long table scrolls.
- **EDT-161** · built · The row of tools does not change when the cursor enters a table.
- **EDT-162** · built · A whole row or column gets a colour in one step.
- **EDT-163** · built · Emoji, header image and page options sit at the title, quiet until hover or focus.
- **EDT-164** · built · The row of tools has a Full width button, which applies at once.
