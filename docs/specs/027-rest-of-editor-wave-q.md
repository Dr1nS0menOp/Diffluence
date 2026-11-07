# Spec 027: wave Q, the rest of the editor

Status: built on branch `wave-q`, waiting for the product owner's acceptance session. Closes most of the 51 open lines of chapter 4 of blueprint 03. The handoff says which lines stay open, and why.

## Study

### What exists

- The tools sit in one row (`toolbar.tsx`), which wraps on a narrow screen. Tools for what is selected appear in that row, not in floating bars (spec 008, D3).
- The handle beside a block has a grip and a menu, for top-level blocks only (`block-handle.tsx`). Moving, duplicating and deleting are one Undo each (`block-commands.ts`).
- Images are attachments, never outside addresses. Their attributes are names, so a document cannot carry a style (`attrs.go`).
- `document.Normalize` is the security gate: every node, mark and attribute is on a list, and unknown nodes are refused. New attributes go through it first.
- A chart already points at a table by its block id and reads it from the document, so it follows every edit (spec 012, D7). It has a real table beside the picture, and series differ by shape.
- A date in a task already sets its due date (`document/people.go`, spec 010, D7), and a tick while reading already goes through `POST /tasks/{id}/done`.
- `page_versions.schema_version` exists, and `document.SchemaVersion` is 1, but nothing upgrades an older document yet.
- The content security policy allows `style` attributes and no scripts. It has `connect-src 'self'`, so the browser cannot fetch an image from another site.

### Public behaviour weighed

Public help pages of wikis describe a plus button on an empty line, a block menu, a browser of blocks, and charts under tables. Only that behaviour was used, in our own words. No text, icon or layout was taken.

## Decisions

**D1. The row folds, and keeps its order.** Tools that do not fit go into a **More** panel at the end of the row, in the same order, from the end backwards. The panel is a group of the same tools and not a menu, because selects and fields sit in it. Nothing wraps anymore, which also closes the known problem of a narrow bar that covers the first row of the page.

**D2. No floating bars.** EDT-006 (tools hidden, with a bar over selected text) and EDT-080 (a bar at the image) go against spec 008, D3, which says the tools stay in one row. The look is the product owner's decision, so both stay open and are named in the handoff.

**D3. A link to a block places an anchor.** "Copy link to this block" puts an anchor at the start of the first line of text in the block, and copies its address. One way to link to a place stays one way (spec 023), and the link works as soon as the page is published. A block without any text, such as an image, has nothing to hold an anchor, so the entry is not offered there.

**D4. Several blocks are a range of top-level blocks.** Shift and click on a grip selects from the block at the cursor to that block. The block menu, Alt and the arrows, Duplicate and Delete then work on the range, as one Undo. Turn into runs on the selection.

**D5. Turn into starts with what is likely.** The first three entries depend on the block. A panel and a section can go back to text. Going back keeps everything inside.

**D6. Dragging a list item.** A list item gets a grip of its own, which carries the item with everything nested below it. ProseMirror's own drop handling places it, so the list stays valid.

**D7. Images keep to values that mean the same everywhere.** `width` is a share of the column from 5 to 100, so the reader never gets more than the column. `wrap` lets text flow around an image aligned to the start or the end. `border` and `border_color` are names. `href` passes `SafeHref`, like any link. An image inside a line is a node of its own, `inlineImage`, and is an attachment like every other image.

**D8. Pasted images from a web page.** `connect-src 'self'` forbids fetching them, and that stays. An image with a `data:` address has its bytes in the clipboard, and is uploaded. For every other image the note says how many could not be taken along. The policy is not weakened for a convenience.

**D9. A neutral panel holds one emoji and a colour.** The emoji passes the same rule as the emoji of a page: no letters, digits or markup. The colour is a name on the list.

**D10. A spotlight is a block with a colour and an optional button.** Its content is ordinary blocks, so a heading and text need no new fields. The button is a label and an address that passes `SafeHref`.

**D11. A decision needs no new fields.** It already holds inline items, so a person is a mention and a date is a date. The row of tools offers both. The report of wave O already reads them.

**D12. Charts stay our own SVG.** No library. New options are stored in `chart` and pass `cleanChart`. Totals of repeated values happen in `chartData`, where they can be tested without a browser. Line and area series get a marker shape of their own as well as a dash.

**D13. Removing a table asks about its chart.** A chart without a table shows that its table is gone, which is honest but rarely wanted. So the table menu asks whether the chart goes too. Removing a chart leaves the table, because the chart only reads it.

**D14. Page language is page data, like the look.** It is not part of a version. It sets `lang` on the text, which drives the spell checker, hyphenation and what a screen reader pronounces. A value must look like a language tag. Empty means the language of the instance. Exports carry it too.

**D15. Direction is a name on a text block.** `dir` is `ltr` or `rtl`, on paragraphs and headings.

**D16. A saved block belongs to a person.** `saved_blocks` holds a name, an owner, a document that passed `Normalize`, and whether it is shared. Only the owner sees a private block, and only the owner changes or deletes one, also a shared one. Files in a saved block stay under the rights of their page: a person who may not see the file gets the same note as for any file that is gone.

**D17. Older documents upgrade on read.** `document.Upgrade` runs numbered steps from the stored version to the current one, on reading only. A stored version never changes. A document from a newer version is left alone. There is no step yet, because version 1 is the only one, so the test gives the function steps of its own.

**D18. An unknown block is kept, not trusted.** The server still refuses unknown nodes: that is the security gate. A browser that meets a node its schema does not know turns it into `unknownBlock`, which shows a clear placeholder and carries the original as an opaque string. `Normalize` accepts that string and nothing ever renders it. Turning it back into a real block is left for phase 8, when a newer linked instance exists, and has to go through `Normalize`, never through the browser alone.

**D19. A long block folds in the document, not in the data.** `folded` starts a code block closed behind a control for readers. While editing it stays open, because hidden text cannot be edited.

**D20. Word and web pages.** Word writes lists as paragraphs with a class and a marker. A paste step turns those into real lists before the schema reads them. The step works on a parsed document and writes nothing as markup of its own.

## What stays open

See the handoff of this wave. Open lines keep their status, with what is still missing.
