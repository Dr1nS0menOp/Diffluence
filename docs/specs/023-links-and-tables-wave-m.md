# Spec 023: wave M, links and tables in the editor

Status: built. Closes most of the links and tables in chapter 4 of blueprint 03.

## Study

### What exists

- A link is a mark with `href`. A link to a page holds the id, so it survives a rename and a move. The words are stored in the document, as typed.
- `POST /pages/reachable` tells which linked pages the reader can open. The rest show as struck through, and a hidden page looks the same as a deleted one (EDT-059, EDT-060).
- `page_links` follows the current version, for links in and out and for the broken links filter.
- `anchor` is an inline node with a name, and `#name` jumps to it through the same code as a heading, because both end up as an element with that id.
- The table frame has one button with everything, an edge to stretch, and stays out of the row of tools (spec 017, D2).
- `@tiptap/pm/tables` 1.8 already has `moveTableRow`, `moveTableColumn`, `CellSelection.rowSelection`, `CellSelection.colSelection`, and pastes cells into a selection. No new dependency is needed for anything in this wave.

### What a stored title would do

A link that follows the title of its page cannot store that title. A version never changes, so the words would stay after a rename, and they would show to a reader who may not see the target. That is exactly what EDT-066 forbids. So the document holds the id and nothing else, and the words are looked up per reader while reading. The same answer serves a tag and a card.

### Public behaviour weighed

Public help pages of wikis describe links that show as plain text, an inline tag or a card, and links to pages that do not exist yet. Only that behaviour was used, in our own words. No text, icon or layout was taken.

## Decisions

**D1. Three new nodes, ids only.** `pageLink` (inline: `page_id`, optional `fragment`, optional `display: tag`), `pageCard` (block: `page_id`), `placeholderLink` (inline: `title`). Words typed by the writer stay a `link` mark, as before. That makes EDT-057 a simple rule: no words of your own means a `pageLink`.

**D2. One way to learn about a linked page.** `POST /pages/link-facts` takes ids, and answers title, emoji, space, excerpt and last change for the pages this person can see. Everything else is left out, so hidden and gone cannot be told apart. It reads through `perm_visible_pages`, as `/pages/reachable` does, so both agree on what is reachable.

**D3. What a reader who may not see the page gets.** The fixed words "A page you cannot open", struck through as a gone link is. No id, title, space or date. Someone not logged in gets the same: the server answers 401, and an error counts as nothing to show.

**D4. Everything that leaves the instance gets resolved first.** `document.ResolveLinks` turns the three nodes into plain text and link marks before a document goes to a writer. So HTML, PDF, Markdown, Word, ODT and public sites need no knowledge of them, and a new writer cannot forget them. A title is only used when the exporter may see the page, and on a public site only when the page is on that site. Otherwise the words are "Linked page", with no link.

**D5. Search does not index a linked title.** A `pageLink` adds no words, so a page is never found through the title of a page its reader may not see. A placeholder is different: its title is what the writer typed.

**D6. A heading or an anchor as the target.** `GET /pages/{id}/targets` lists both, with `page.view`. The address gets `#slug`. For the page being written the dialog reads the open document, because that is what will be published. Any other block is reached by placing an anchor in it, so EDT-058 stays partly.

**D7. A placeholder knows what it became.** `page_placeholders` holds source, title and target, and follows the current version in the transaction that publishes. Following a placeholder creates a draft below the page it stands on, with that title, and records it. Two people following at once get one page: the row is locked. After that the link leads to the page for whoever may see it, and stays a placeholder for others. Following needs `page.view` on the source and `page.create` on it as a parent, and the title has to be in the published page. A target the person cannot see gives a conflict that names no title beyond the one on the page.

**D8. The placeholder list** sits in Space settings for space administrators, and only shows when there is something to show. A source page they may not see is left out.

**D9. The link tools follow the selection.** They sit in the row, as all selection tools do (spec 008, D3): open, copy, edit, unlink, and display.

**D10. Handles on the current row and column only.** A row and a column are shown for the cell under the pointer, or else the cursor, so a table of a thousand rows does not get two thousand buttons, and the keyboard reaches both. A handle selects its line and opens its menu. A plus on the far edge of the same line inserts right there, apart from the handle so both stay large enough to hit (WCAG 2.5.8). Shift with a handle selects up to that line.

**D11. Dragging has a keyboard way.** Move up, down, left and right are in the menu of the handle, as WCAG 2.5.7 asks. Merged rows move as one block. Where the library refuses a move, the tools say so.

**D12. Numbered column.** `numbered` on the table draws the numbers in a margin inside the frame, with a CSS counter. They are no cells, so sorting, copying and exports of cells stay simple, and the number follows the row. Exports leave them out.

**D13. Flexible, not fixed, is the new word.** Tables today keep their layout and scroll. That stays the default, so nothing changes for existing tables, and `flexible` on the table makes it scale with the page instead.

**D14. A caption** is `caption` and `caption_below` on the table: a real `<caption>`, in exports too.

**D15. A table in a table.** The editor refuses it and says so. The server lifts one out of a cell, with its words kept, instead of refusing the document: a Word file with nested tables, or a draft from before this rule, must still save.

**D16. From a spreadsheet.** Copied cells carry a table in HTML, which already lands as cells. Text with tabs now does too, in a table and outside one. A row or column is copied by selecting it with its handle. Ctrl+Shift+V still pastes plain text.

**D17. The size picker.** `/table` asks for rows, columns and a header row, with 3 by 3 ready, so Enter is as fast as it was.

## Not in this wave

- Charts from tables, EDT-130 to EDT-136: a later wave.
- Comparison does not show a change in a `pageLink`, `pageCard` or table settings.
- Word and ODT get the caption as a line of text, with no numbers.

## Requirements

| Line | Outcome |
| --- | --- |
| EDT-056 | built: recent pages first |
| EDT-057 | built |
| EDT-058 | partly: headings and anchors, in this page and others |
| EDT-063 | built |
| EDT-064 | built |
| EDT-065, EDT-066, EDT-129 | built |
| EDT-068 | built |
| EDT-086, 087, 088, 091, 093, 098, 100, 101, 102 | built |

## Tests

The failure paths come first: a member without access to the target learns nothing through link facts, targets or placeholders, and a stranger gets a 401. Every permission rule has a mutation in `scripts/mutation-check.sh`.
