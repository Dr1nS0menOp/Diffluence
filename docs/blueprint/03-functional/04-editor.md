# 03.4: The editor

The editor is where people spend their time. The look is settled (Rail, ADR-021): a strip with the page's fate at the top, the tools below it, and an open sheet with no boxes. This chapter describes what it can do.

## 4.1 The surface

- **EDT-001** · built · The strip holds Close with the save status on one side, and the summary, Discard draft and Publish on the other.
- **EDT-002** · built · The tools sit in one row below the strip. Both stay in view while scrolling, except on narrow screens.
- **EDT-003** · built · The title is a large plain input with no border. The body follows at most 50rem wide.
- **EDT-004** · built · An active tool is solid accent, so its state is clear without reading.
- **EDT-005** · built · Table tools appear only inside a table.
- **EDT-006** · partly · The tools can be pinned to the top or hidden. When hidden, a small floating bar appears over selected text. The choice is kept per person. Built: a floating bar over selected text beside the row, which stays pinned (spec 047). Open: hiding the row, which the product owner put off on 2026-11-10.
- **EDT-007** · built · Tools that do not fit fold into a **More** menu at the end of the row, in a fixed order. Nothing is ever cut off.
- **EDT-008** · built · A person sets **Normal** or **Full width** for the page from the strip. The sheet shows the width readers will get.
- **EDT-009** · built · The title field gets the focus on a new page, and Enter moves to the body. In the body, the first Backspace at the very start moves back to the title.
- **EDT-010** · built · Hovering over the title reveals **Add emoji** and **Add header image** (PAG-123).
- **EDT-011** · built · A word count and read time show at the bottom of the sheet on request.
- **EDT-012** · built · The status of the page can be set above the title (PAG-054).
- **EDT-013** · built · The strip shows who else is editing, in phase 4 (COL-088).
- **EDT-014** · built · Clicking anywhere below the end of the text puts the cursor at the end, and adds a paragraph if the last block is not text.
- **EDT-165** · new · Beyond EDT-001, the strip holds at its start a small fold control, whose use needs a study of its own, the page's icon and its title in bold. At its end it holds, in this order, the save status in muted words (PAG-012), the pictures of the people editing (EDT-013), **Publish** as the one primary button, whose words say that a further step follows, and **Close** as quiet words at the very end (changes the design standard, decided 2026-11-14).
- **EDT-166** · new · The summary of a change (PAG-022) and **Discard draft** (PAG-015) leave the strip and move into the step that Publish opens, beside the publish options of PAG-024. Discard draft is a danger action there, apart from Publish, and asks first. Their place was decided on the product owner's behalf on 2026-11-15 and may be overruled (changes the design standard, decided 2026-11-14).
- **EDT-167** · new · The strip is a little taller than the row of tools, and a thin line under the row of tools divides both bars from the sheet.
- **EDT-168** · new · The row of tools holds, from the start: the text style, which shows the current style in words; bold, underline, italic and **More formatting**; bullet list, numbered list and more list choices; alignment; text colour; a divider; direct buttons for a task, an image or file, a mention (once mentions exist, EDT-126), an emoji, columns, a panel, a synced block (BLK-046), a table with an arrow for its size (EDT-086), and **Insert**; a divider; link; a divider; undo, redo and **History**. Only the text style shows words; every other tool is an icon with its name on hover and focus (changes the design standard, decided 2026-11-14).
- **EDT-169** · new · Tools of the row that EDT-168 does not name keep a place: highlight, strikethrough, inline code, subscript, superscript and clear formatting sit in More formatting; indent and outdent sit in the list choices; Find and Full width (EDT-164) sit in a last group after a divider at the very end. The tools for what is selected stay at the end of the row, before that last group (changes the design standard, decided 2026-11-14).
- **EDT-170** · new · Undo and redo are dimmed when there is nothing to undo or redo, and keep their place in the row.
- **EDT-171** · new · In place of the solid accent of EDT-004, an active tool shows its icon in the accent colour on the accent tint, so its state is still clear without reading it (changes the design standard, decided 2026-11-14).
- **EDT-172** · new · **History** in the row of tools opens the published versions of the page (PAG-087) in a panel at the end side of the editor, where a writer opens an earlier version and compares any two without leaving the draft. Restoring from there puts that version's text into the draft, which Undo reverses and Publish makes a new version (PAG-089); no version is ever changed (decided on the product owner's behalf on 2026-11-15, may be overruled).
- **EDT-173** · new · A small round help button sits in the bottom start corner of the editor, apart from the row of tools, and stays there while the sheet scrolls. It opens the overview of shortcuts for writing (EDT-197), with a link to the documentation the administrator set (NAV-068).
- **EDT-174** · new · Between the title and the body, the sheet shows a reduced byline: the author's small picture and name and, on a page under document control, the name of the control that applies as plain muted words that open its panel (DOC-901). It shows no views, reactions or status. A wide gap follows before the body, so the sheet looks like the page readers get (EDT-008) (changes the design standard, decided 2026-11-14).
- **EDT-223** · new · Beyond EDT-167, a thin line also runs across the full width between the strip and the row of tools, so the strip, the row and the sheet read as three bands, one above the other.

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
- **EDT-175** · new · **Insert dialog:** in place of the grouped list of EDT-020, the Insert tool, the plus button (EDT-019) and the last row of the slash menu open one large dialog with everything that can be added. It lies over the dimmed editor and fills nearly the whole window, and the editor cannot be used while it is open (changes the design standard, decided 2026-11-14).
- **EDT-176** · new · The insert dialog has these regions: a close button alone in the top end corner; under it the dialog's title at the start, on the same line as a search field that runs across the rest of the width; below the title a fixed column of categories on the start side, about a fifth of the width; beside it a grid of entries in three equal columns, which alone scrolls, with its own scrollbar, so rows are cut at its top and bottom edges; and a footer in the bottom end corner with the primary **Insert** first and a quiet **Close** at the very end. White space alone divides the regions, with no lines, and only the title and Insert are bold (changes the design standard, decided 2026-11-14).
- **EDT-177** · new · The category column lists, as words without icons, counts or a heading of its own: **Everything**, which is chosen when the dialog opens, then text and structure, the wiki's own content, media, pictures and diagrams, finding your way, outside content, reports, and software work. The chosen category shows accent words on the accent tint, and the first category is level with the first row of the grid. A category with nothing in it is not shown, such as outside content while no outside domain is allowed (EDT-067), or software work while no tracker is linked.
- **EDT-178** · new · The search field of the insert dialog narrows the entries of every category at once, by name, description and synonyms as the slash menu does (EDT-016). A quiet Enter hint sits at its end, and Enter adds the selected entry. A search that finds nothing says so in one muted sentence.
- **EDT-179** · new · Each entry of the grid is a cell with an icon tile at its start, its name on one line, and at most two lines of smaller muted description under the name. A shortcut box sits at the end of the name line and is never cut: a name that does not fit gives way and ends in an ellipsis, and so does a description after two lines. An entry without a description shows its name centred beside the icon. Cells have no borders and nothing between them, and everything in them is start-aligned.
- **EDT-180** · new · An entry under the pointer has a quiet fill over the whole cell, with no bar. The selected entry has the accent tint over the cell, an accent bar on its start edge, and its icon on a filled tile. Keyboard focus is an accent outline. The bar is the cue beyond colour, in place of the checkbox of selected rows, because only one entry is chosen at a time (changes the design standard, decided 2026-11-14).
- **EDT-181** · new · After a short pause on an entry, and when the entry has keyboard focus, a tooltip shows its whole description directly under it, as a dark box that may cover the start of the next row.
- **EDT-182** · new · The first entry of the list is selected when the insert dialog opens, and one entry is selected at a time. The arrow keys move the selection through the grid by row and column. The selection stays while the grid scrolls, and Insert keeps working when the selected entry is scrolled out of view. Enter or **Insert** adds the selected entry at the cursor; Close, the close button and Escape leave without adding anything.
- **EDT-183** · new · Focus starts in the search field when the insert dialog opens, so typing narrows the entries at once (decided on the product owner's behalf on 2026-11-15, may be overruled). Focus always shows: an accent outline around the control that has it, and around the whole grid while the grid has it. The list shows no count and simply ends.
- **EDT-184** · new · In place of the group headings of EDT-017, the slash menu is one flat list in a card with a soft shadow, with no title, categories, search field or buttons, and no lines between rows. Each row has an icon, a name, a description that wraps to a second line rather than being cut, the shortcut box at its end and the New mark where it applies. The rows are those of the dialog's Everything list, in the same order.
- **EDT-185** · new · The slash menu opens above the line when there is more room there, and then fills the room up to the row of tools. Its list scrolls inside the card with its own scrollbar. The first row is highlighted at once, by a quiet fill and an accent bar on its start edge. While the menu is open, the typed slash shows in the accent colour, so it reads as a command and not as text.
- **EDT-186** · new · The slash menu ends in a fixed row under a thin line, with its own quiet fill, three dots and words, which opens the insert dialog in place of the Browse all entry of EDT-020. The row stays in view while the list above it scrolls, the arrow keys reach it like any row, and the words typed after the slash carry over into the dialog's search field.
- **EDT-187** · new · Every entry of the slash menu and the insert dialog that has a typing shortcut (EDT-027) or a key combination (EDT-031) shows it in a small quiet box at the end of its name line, headings (`#`) and panels (`!!`) included, so people learn to add it without the menu. The keys show as the person's system names them (NAV-067), and a box appears only for a shortcut that works in this release. A screen reader hears the shortcut as part of the entry, and a touch screen without a keyboard shows no box.
- **EDT-188** · new · A kind of element that is new in this release carries the **New** mark of NAV-178 right after its name in the slash menu and the insert dialog, before any shortcut box.
- **EDT-189** · new · **Order of the list:** after the recently used entries of EDT-017, the slash menu and the dialog's Everything list run in this order of roles: a few entries that Kenwerk promotes, such as a new kind of content, which also keep their own place further down; items in a line of text and simple blocks (task, image or file, mention, synced block, emoji, collapsible section, table, code block, status mark, the panels, date, decision); text structure (bullet list, columns with two first and then one, three, four and five, link, numbered list, horizontal rule, quote, headings 1 to 6); Help; the block template gallery and the block templates; the dynamic blocks for reports, people and finding your way; display blocks and actions (contents, tabs, spotlight, link cards, rotating cards, a new page with a link to it); and entries of apps last, grouped per app.
- **EDT-190** · new · The slash menu and the insert dialog also hold the entries that chapters 12 and 13 add for their kinds of content, such as whiteboards, databases, views of a linked tracker and the automation button. Each shows in the category of its kind, and only once that kind exists in the instance.
- **EDT-191** · new · Every entry of the slash menu and the insert dialog has an icon tile. Entries of one family share one icon, so a family shows at a glance: every block template one, every starting layout of a whiteboard one, every view of a linked tracker one, every dynamic block without an icon of its own one. Icons of levels and counts show the number: heading icons shrink from level 1 to 6, and column icons show how many columns. A panel's icon carries its panel colour and sign, and entries about people use a person icon. One icon means one thing only, and no tile is solid accent, which belongs to the selected entry. An app's entry shows the app's own picture inside our tile frame, served from our own origin (phase 7, BLK-012) (changes the design standard, decided 2026-11-14).
- **EDT-192** · new · Beyond EDT-026, a **block template gallery** is an entry of the slash menu and the insert dialog. It lists the block templates a person may use, their own, those shared with them and the starter set, with a search box and a preview, as the template gallery of ORG-045 does. From the gallery a person adds one at the cursor, saves the selection as a new one, and renames, shares or deletes their own; deleting asks first.
- **EDT-193** · new · Kenwerk ships a starter set of block templates in our own words, to insert into an existing page: a defect record, a project at a glance, risks and assumptions, a decision log, who does what, a campaign plan, a launch checklist, a comparison with competitors, a meeting agenda, the schedule of an event, action items, coloured columns, a coloured information table, a week plan, a customer quotation, pros and cons, numbers and states in colour, then and now, a coloured project summary, a short project update, and options scored against criteria. The coloured ones use colour names (EDT-156).
- **EDT-194** · new · Administrators can hide starter block templates, and promote block templates so they come first, as for page templates (ORG-054).
- **EDT-195** · new · An administrator decides in the general settings of the instance whether people may share their block templates with others. When it is off, every block template stays private to its maker. It is on by default, so that sharing as it works today keeps working (decided on the product owner's behalf on 2026-11-15, may be overruled).
- **EDT-196** · new · An empty table cell with the cursor shows a quiet hint: a slash in a small key box and a few words that say it opens the insert menu. An empty paragraph shows no hint. A hint goes at the first character typed and is never stored in the document.

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
- **EDT-197** · new · In the editor, where `?` types a character (NAV-061), Ctrl / and a **Help** entry of the slash menu and the insert dialog open the overview of shortcuts (NAV-067). Opened from the editor, the overview also lists the typing shortcuts of EDT-027, so all shorthand is in one place.
- **EDT-198** · new · A key combination inserts a table of the default size (EDT-084), as other blocks have one, and the overview lists it. Which combination is free in common browsers, systems and screen readers needs its own study.
- **EDT-199** · new · In a table, Shift F10 and the menu key open the cell menu of the cell with the cursor (EDT-209), and Ctrl Shift . opens the More menu of the table bar (EDT-215). Ctrl Alt Right adds a column after the current one and Ctrl Alt Down adds a row below it, and the cell menu shows those keys beside its entries. Whether Ctrl Alt with the arrows clashes with keys of the system on some machines needs its own study.

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
- **EDT-200** · new · An entry of the slash menu and the insert dialog creates a new page at once, below this page or in a chosen place, and leaves a link to it at the cursor, while the writer stays in this editor. The new page is unpublished (PAG-006) until its creator publishes it, and readers who may not see it yet get the link as plain words (EDT-059). On a page that was never published, which cannot have children (PAG-007), the entry offers another place or a placeholder link (EDT-064).
- **EDT-201** · new · In the Link preferences section of Your account (ACC-105), a person chooses how a link they add or paste first shows: as the address, as a tag with the title (EDT-129), or as a card (EDT-065). New people start with the tag. The choice only sets the start: the link's bar still changes one link (EDT-063), links already in pages keep their form, and an outside address stays an address unless an administrator allowed its domain for cards (EDT-067, ADM-024).
- **EDT-202** · later · Under that default, a person can add exceptions: a whole address or the start of one, such as one space or one allowed outside domain, each with its own first form. The exceptions are listed under the default, each can be changed or removed, and the longest matching start wins.
- **EDT-203** · new · The Link preferences section shows, from the top: its title and one sentence saying that it concerns new links; a legend of the forms without a frame, each form on one line with a small icon, a bold term and a short explanation; the default under a bold heading with an information mark, in one framed row with the setting's name at the start and a borderless chooser with its value at the end; then the exceptions (EDT-202) under a second such heading, with their list and an add form in a framed card. The form has a small bold label marked as required in words as well as by a mark, a wide field without a placeholder with the form's chooser on the same line, a muted hint under the field, and then the primary add button and a secondary Cancel, aligned to the start under the hint.
- **EDT-204** · later · **Link shortcuts:** an administrator keeps a list of link shortcuts for the instance, each with a short key, an address pattern and an optional default link text. A new instance has none.
- **EDT-205** · later · In the link dialog (EDT-054), an address written as a key, a colon and a word, such as a ticket number, becomes the shortcut's address: Kenwerk puts the word where the pattern marks its place, or at the end. The default link text can hold the same mark, and becomes the words that show unless the writer typed their own. The link keeps the address it was given, so changing a shortcut later changes no existing link.
- **EDT-206** · later · A link shortcut's pattern must give an http or https address (EDT-053). Keys are unique without regard to case, and a key can never be a scheme such as `http` or `mailto`. The server fetches nothing: a shortcut only writes an address (deliberate difference 5).
- **EDT-207** · later · The screen of link shortcuts in the administration shows its title and one sentence; an add form with three one-line fields, key, address pattern and default link text, labelled at their start, and a small add button under them; then a table of the current shortcuts with the columns key, address pattern, default text and actions, in the order they were added, with Change and Delete in each row. Delete asks first. The table has no search, count or paging.
- **EDT-208** · later · At the end side of the link shortcuts screen, a framed help box with a bold title in a strip of its own holds worked examples, each indented with a line at its start and its variable part in bold. On narrow screens the box sits above the form.

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
- **EDT-080** · built · Selecting an image shows a small bar with these options, and Delete. Deleting it from the page does not delete the attachment. Built: the options, in the row of tools and in a bar at the image, with Delete (spec 047).
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
- **EDT-209** · new · **Cell button:** in the cell with the cursor, a small button sits in the top end corner and opens the cell menu beside and below it, over the next columns. Its tooltip names it and its keys on one line (EDT-199) (changes the design standard, decided 2026-11-14).
- **EDT-210** · new · The cell menu holds three groups divided by lines: cell colour, which shows the current colour in its row and opens the palette (EDT-095), and alignment, which opens a sub-menu; add a column after this one and add a row below, each with its keys in a box; clear the cell, delete the column and delete the row. Unlike the rule of NAV-147, the delete entries are plain, not in the danger colour, because Undo brings them back. When several cells are selected, the menu also offers merge and split (EDT-090), and every entry acts on all of them.
- **EDT-211** · new · **Empty the cell** clears the cell with the cursor, or every selected cell, and keeps the shape of the table and the colours of the cells. Undo brings the content back.
- **EDT-212** · new · **Cell alignment:** the sub-menu aligns the content of the cell, or of every selected cell, to the start, the centre or the end. In right-to-left text start and end mirror (EDT-040), and the choice is stored as a name (EDT-156). Whether it also offers top, middle and bottom needs its own study.
- **EDT-213** · new · **Table bar:** under the table with the cursor, centred on it, a small floating bar holds **Table settings**, a chooser for where the table sits and how wide it is (EDT-097), a button that makes the columns equal (EDT-096), a button that fits the table to the width of the text, a divider, and **More**. It lies over whatever follows the table, and hides while the cell menu is open. With the cell button it takes the place of the one button of EDT-160, and the row of tools still shows nothing for tables (EDT-161) (changes the design standard, decided 2026-11-14).
- **EDT-214** · new · Table settings opens a menu of entries that are on or off: fixed column widths (EDT-098), header row, header column (EDT-092) and numbered rows (EDT-093). An entry that is on has a tick, accent words on the accent tint and a mark on its start edge, and while the menu is open Table settings itself shows in the accent on the tint. The menu stays open while entries are switched.
- **EDT-215** · new · More on the table bar holds the rest of what the menu of EDT-160 offered: add a row above or a column before, sort (EDT-094), sticky header (EDT-099), caption (EDT-102), filter for readers (EDT-103), chart (EDT-130), copy the table, and delete the table. The order of these entries needs its own study.
- **EDT-216** · new · The controls around a table are faint and show only on the table with the cursor: a round button at its top start corner that selects the whole table, the row and column handles (EDT-087), small insert dots on the borders (EDT-088), a handle on the end edge for the width (EDT-097), and the block's plus and grip in the margin (EDT-019, EDT-021).
- **EDT-217** · new · A header row and a header column have a quiet fill, so they read as headers by more than bold text, and each row takes the height of its content.

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
- **EDT-218** · later · An administrator sets, for the instance, the language a new code block starts with, and whether its line numbers and the wrapping of long lines start on. A writer still changes each block (EDT-105, EDT-107, EDT-108), and existing blocks keep what they have. Colours of the highlighting stay tokens, so there is no colour theme to choose (deliberate difference 10).

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
- **EDT-219** · new · Beyond EDT-118, columns can also be a single column: a section of its own, which can be made wide or full (EDT-120). Each number of columns, one to five, is its own entry in the slash menu and the insert dialog, with an icon that shows that number. In Markdown a single column is just its content.
- **EDT-220** · new · **Tabs:** a block with several named parts, of which a reader sees one at a time and switches with a click or the arrow keys, as the tabs of the design standard do. Each part holds any block. It starts on the first part and remembers nothing, like a collapsible section (EDT-116), and following a link into a part opens that part.
- **EDT-221** · new · Printing and exporting show every part of a tabs block one after another under its name, and Markdown gives each part a heading. Search finds text in every part, and a comparison of versions shows every part. A limit on the number of parts, tabs inside tabs, and what narrow screens show need their own study.
- **EDT-222** · new · An empty decision (EDT-122) shows its mark and a short muted invitation to write it, across the full column. The invitation goes at the first character typed and is never stored in the document.

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
- **EDT-143** · partly · A copied block from another Kenwerk page keeps its blocks and marks. Attachments stay linked if the reader may see them, or are copied along. Still open: attachments of another page. Blocks and marks come along through the clipboard, and a file keeps pointing at its own page.

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
