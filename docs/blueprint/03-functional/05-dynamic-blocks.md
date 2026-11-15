# 03.5: Dynamic blocks

Some blocks do not hold text: they show something that changes by itself, such as a list of child pages. Other wikis call these macros. We call them dynamic blocks.

## 5.1 How all dynamic blocks behave

- **BLK-001** · built · A dynamic block is inserted from the slash menu or the insert menu, and opens a side panel with its settings.
- **BLK-002** · built · Changes in the settings show at once in the block, with no save button.
- **BLK-003** · phase 4 · Selecting a block shows a small bar: edit settings, width, duplicate, delete.
- **BLK-004** · phase 4 · Width is normal, wide or full for blocks where that makes sense.
- **BLK-005** · built · While editing, a block shows its real content, not a grey placeholder.
- **BLK-006** · built · **Permissions come first:** each reader sees only what they may see. A block never shows a title, name or count from a page the reader may not see. It filters through the same rule as search (blueprint 05).
- **BLK-007** · built · A block with nothing to show gives one muted sentence, or nothing at all when the writer chose that.
- **BLK-008** · built · A block that fails shows a short message in place, and the rest of the page still works.
- **BLK-009** · built · Blocks load after the page text, so a slow block never delays reading.
- **BLK-010** · built · Results are limited and paged. No block can make a page load the whole instance.
- **BLK-011** · phase 4 · Export to PDF, Word and HTML shows the state of a block as the exporting person sees it. Markdown shows a note and a link.
- **BLK-012** · later · Apps can add their own blocks (phase 7), run in a sandbox and never get more rights than the reader.
- **BLK-070** · new · Beyond BLK-008, a dynamic block that takes longer than a set time to work out stops and shows a short message in its place, while the text of the page and its other blocks stay readable. The time is a setting of the instance, shown with its default. The default, and whether the limit belongs to the block or to the whole request (SEC-271), need their own study.

## 5.2 Finding your way on a page

- **BLK-013** · built · **Table of contents.** Lists the headings of the page as links. Settings: from and to which heading level (default 1 to 6), list or one line, marker style (none, bullet, number), add section numbers such as 1.2, indent per level, include and exclude patterns on heading text, leave out of print. It follows every edit.
- **BLK-014** · built · **Section contents.** The same, limited to the part of the page between two markers, or to one collapsible section.
- **BLK-015** · built · **Back to top.** A small link that jumps to the title.
- **BLK-016** · built · **Anchor.** A named spot that other pages link to. The name appears only while editing. Names are matched without regard to case, and spaces become dashes.
- **BLK-017** · built · **Search box.** A search field inside a page. Settings: which spaces, which content types, which labels, placeholder text, small or large.
- **BLK-018** · built · **Search in this branch.** A search field that only looks below a chosen page.

## 5.3 Showing what is in the tree

- **BLK-019** · built · **Child pages.** Lists the pages below a page. Settings: which parent (default this page), all or a maximum (default 10), how many levels (1 to 9, or all), sort by tree order, title or created date in either direction, style as bullets or headings, and an optional preview of each page: none, the first lines, or its excerpt.
- **BLK-020** · built · **Page tree.** An expandable tree below a chosen page, with a Collapse all and Expand all button and an optional filter.
- **BLK-021** · built · **Index of a space.** All page titles in alphabetical order, with a letter bar.
- **BLK-022** · built · **Spaces list.** A list of spaces, filtered by category or favourite, as cards or a list.
- **BLK-023** · built · **Breadcrumbs here.** The path of a chosen page, for hub pages.
- **BLK-071** · later · When live documents exist (OTH-001), **Child pages** also lists the live documents below a page or below a live document, each marked with its kind.

## 5.4 Recent and popular content

- **BLK-024** · built · **Recent changes.** Lists what changed recently. Settings: which spaces (this one, chosen ones, favourites, personal, all), which types (pages, blog posts, comments, attachments), how many (default 15, at most 200), filter by people or labels, style (title only, title and name, with picture), hide the heading.
- **BLK-025** · built · **Activity tabs.** Recent changes with tabs for all spaces, favourite spaces and the person's own network.
- **BLK-026** · built · **Popular pages.** The pages most viewed in a chosen period, for spaces where analytics is on.
- **BLK-027** · built · **My favourites.** The reader's own favourite pages.
- **BLK-028** · built · **My drafts and tasks.** The reader's own drafts or open tasks, for a personal home page.

## 5.5 Lists by label and filter

- **BLK-029** · built · **Filter list.** A list of pages built from filters: label, title contains, content type, text contains, date created, last changed, creator, contributor, mentioned person, space, children of a page, all pages below a page.
- **BLK-030** · phase 4 · Several values in one filter mean **or**. Several filters mean **and**. A minus sign before a label excludes it.
- **BLK-031** · phase 4 · The result can show a heading, sort by relevance, title, created or changed in either direction, show a preview, show labels, show the space, and a maximum (default 15, at most 500).
- **BLK-032** · built · **Report table.** The same selection as a table with title, creator, last changed, labels, status and optional comment and reaction counts.
- **BLK-033** · built · **Labels list.** All labels of a space with their pages, or alphabetical with a letter bar.
- **BLK-034** · built · **Popular labels.** The most used labels as a list or word cloud.
- **BLK-035** · built · **Related labels.** Labels on pages that share a label with this page.
- **BLK-036** · built · **Recently used labels.** The newest labels of a space or the instance.

## 5.6 Page facts and reports

This pair turns pages into a small database: a block of facts on each page, and a report that collects them.

- **BLK-037** · built · **Decision report.** Lists decisions across pages, filtered by space, page, label and person, with the date.
- **BLK-038** · built · **Task report.** Lists tasks across pages. Settings: spaces and pages, labels, assigned to, created by, created after, status (default open), rows per page (10, 20, 40), which columns (description, due date, assigned to, location, done date, labels), sort by due date, person or title. Readers with *edit* can tick tasks in the report.
- **BLK-039** · built · **Page facts.** A two-column table of keys and values on a page, optionally hidden from readers of that page. It can have an id, so one page can hold several sets. Values can be text, people, dates and status marks.
- **BLK-040** · built · **Facts report.** Collects page facts from pages with chosen labels into one table, with a column per key. Settings: the page filters of BLK-029, id, title column heading, which columns, sort by a column, comment and reaction count columns, rows per page (default 30).
- **BLK-041** · built · Sorting a facts report by a date or a number uses its real value, not its text.
- **BLK-042** · built · A template can include page facts, so similar pages report together (ORG-048).
- **BLK-072** · new · **Fact value:** an item in a line of text that shows the value of one key from the page facts of this page, or of another page the reader may see. It follows every change to those facts.
- **BLK-073** · new · A block form of the fact value shows several keys of one named set of page facts (BLK-039) at once, as a small table of keys and values, and follows every change to them.
- **BLK-074** · new · A fact value shows nothing from a page the reader may not see, and facts hidden from readers (BLK-039) stay hidden in it. When the key is missing, readers see nothing and writers see a muted note while editing. How dates, people and status marks look in a line of text needs its own study.

## 5.7 Reusing text

- **BLK-043** · built · **Excerpt.** Marks part of a page as reusable. It has a name, can be hidden on its own page, and can be inline or a block. A page can have several.
- **BLK-044** · built · **Show excerpt.** Shows an excerpt from another page. Settings: which page, which excerpt, show a title or a frame around it. A link to the source appears on hover.
- **BLK-045** · built · **Embedded page.** Shows the whole content of another page. A reader needs view rights on both pages, and otherwise sees nothing of it.
- **BLK-046** · built · **Synced block.** A block edited in one place that shows in many. A small mark shows the source, and editing is one click away for those who may.
- **BLK-047** · built · A page cannot include itself, directly or through others. A loop shows a short message.
- **BLK-048** · built · Embedding goes at most 5 levels deep.
- **BLK-049** · built · Search finds text at its source, not on every page that shows it.
- **BLK-050** · built · The source page lists where it is used, and its Delete dialog warns about that.
- **BLK-051** · built · **Snippet variables:** a reused block can have blanks that each page fills in, for names and versions.
- **BLK-075** · later · When live documents exist (OTH-001), **Embedded page** also shows a live document, with the same need for view rights on both sides (BLK-045). Whether it shows the live state or the last automatic version (OTH-002) needs its own study.

## 5.8 People and the page

- **BLK-052** · built · **Contributors.** Lists people who wrote, commented on, labelled or watch a page. Settings: which contribution types, sort by count, name or last contribution, a maximum, inline or list, show the count, show the last time, which page, which spaces, include children or all below.
- **BLK-053** · built · **Contribution table.** A table of contribution numbers, by person or by page.
- **BLK-054** · built · **Person card.** A card with a person's picture, name, job title and contact details, within what the reader may see.
- **BLK-055** · built · **People list.** People in a group or chosen by hand, as a list or grid of pictures.
- **BLK-056** · built · **Change history here.** The last versions of this page with author and summary.
- **BLK-057** · built · **Attachments here.** Lists files of this or another page. Settings: name pattern, labels, include old versions, sort by date, size, name or created in either direction, how many per page. Readers can sort by column heading, and with permission delete and see versions. Files are still added in the editor.
- **BLK-058** · built · **Space files.** Lists all files in a space, filtered by label and type.
- **BLK-076** · new · **Person picture:** a compact form of the person card (BLK-054) shows only a chosen person's picture, in a line of text or as a block, with the name on hover and focus. Without the right to view profiles, or when the person keeps the picture to themselves (ACC-023), only the name shows.

## 5.9 Blog and content blocks

- **BLK-059** · built · **Blog posts.** Lists posts of chosen spaces. Settings: how many, period, labels, authors, title only, excerpt or full text, sort.
- **BLK-060** · built · **Create from template.** A button that makes a new page from a chosen template in a chosen location.
- **BLK-061** · built · **Link cards.** A grid of cards made from chosen pages, or from a filter, with a title, image and short text.
- **BLK-062** · built · **Rotating cards.** Cards that show one at a time, with controls. Moving stops on hover and focus, and when reduced motion is asked for.

## 5.10 Diagrams and embedded media

- **BLK-063** · phase 4 · **Diagram.** A diagram written as text with Mermaid, shown as a picture, with an editor beside a live preview.
- **BLK-064** · built · **Drawing.** A simple drawing surface for boxes and arrows, stored as a document and an image. It runs in the optional draw.io service (ADR-026), and a saved drawing can be changed.
- **BLK-065** · built · **Timeline.** A simple plan with lanes, bars and markers on a time scale, edited on the page.
- **BLK-066** · partly · **PDF viewer.** Shows a PDF attachment in the page, with paging, when the instance allows it. Still open: it opens in PdfCraft in a window over the page, not in the page itself, and no setting turns it off.
- **BLK-067** · partly · **Office preview.** Shows Word, Excel and PowerPoint attachments as pages, when a converter is installed. Built: with a helper, an office file opens as a PDF download. New: showing it as pages inside the page.
- **BLK-068** · later · **External embed.** Shows content of another site in a frame. The Content Security Policy allows only domains on a list set by an administrator, and the frame is sandboxed. Nothing is embedded by default.
- **BLK-069** · new · **Calendar block** when calendars are built (chapter 13).
- **BLK-077** · new · **Drawing shown elsewhere:** a drawing made on one page (BLK-064) can be shown on another without copying it. The block names the source, follows its changes, links to the source on hover, and shows nothing to a reader who may not see the source (as BLK-045). Editing happens at the source, one click away for those who may (as BLK-046), the source lists where it is used (BLK-050), and loops and depth follow BLK-047 and BLK-048.
- **BLK-078** · later · **Schema view:** an XML schema attached to the page shows as a diagram of its elements, or as a table of its elements, types and rules.
- **BLK-079** · later · Our own code reads an XML schema as data, with limits on size and depth and no outside entities, because files are never trusted (deliberate difference 6). It never fetches a schema that the file imports from an outside address (deliberate difference 5). The parser, the limits, and how schemas imported from the same page are found need their own study.
- **BLK-080** · later · **Process diagram:** a writer draws a business process in the BPMN 2.0 notation, with events, tasks, gateways, flows and lanes, in an editor over the page, and readers see it in the page as a picture that opens larger. It is stored as a document of the page and as an image, like a drawing (BLK-064), and a saved diagram can be changed.
- **BLK-081** · later · A process diagram can be exported and imported as its standard BPMN file. An imported file is never trusted (deliberate difference 6): it goes through a parser that keeps only the notation, and never into the page as markup.
- **BLK-082** · later · Process diagrams run in the optional drawing service (ADR-026) as far as its shapes for this notation are enough, and fetch nothing from outside. Whether a real modeller is needed, as a new optional service under the dependency policy, needs its own study.
- **BLK-083** · later · **Process overview:** a block shows all process diagrams of a space and how they connect.
- **BLK-084** · later · **Process profile:** a block shows the facts of one process on the page that describes it. Which facts it holds needs its own study.
- **BLK-085** · later · Process diagrams have settings in the administration and in the settings of a space. What those settings hold needs its own study.

## 5.11 Document control on the page

Three blocks bring the state of a controlled page into the text, within what document control lets each person see (09-enterprise).

- **BLK-086** · new · **State:** shows the current state of this page or another page as a word and a colour, in the style of a status mark: its status (PAG-054), or its state in document control when a workflow applies.
- **BLK-087** · new · **Approvals:** shows the approval rounds of a page, with who was asked, who answered what, and when.
- **BLK-088** · new · **Life cycle:** lists the steps of a page's history in its workflow, with dates and people.
- **BLK-089** · new · These blocks show a reader only what document control lets them see: the state, version, date and approvers of the approved version (DOC-302), never a working version, an open round or a review comment (DOC-300, DOC-501). Authors and reviewers also see the open round, and only people with `space.audit` see the full control history in the life cycle (DOC-1010).
- **BLK-090** · new · A block that names a controlled page that was never approved shows a reader nothing, as if the page did not exist (DOC-301). Whether showing another page's state can reveal that a working version exists needs its own study.
