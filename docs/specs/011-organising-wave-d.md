# 011: Wave D, organising

Status: planned in detail, not built yet
Phase: wave D of `docs/blueprint/03-functional-requirements.md`. The product owner asked on 2026-11-05 to build the whole of blueprint 03, one commit per wave, and tests it when all waves are done. This wave comes after wave C (spec 010) and uses its notifications and watching.

## Problem

A wiki that grows needs order. Today a page has no owner, no status and no labels, nothing can be archived, a page can only move inside its space, and a person starts every page from nothing. Space administrators have no way to clean up, search has three filters, and a space cannot carry news. This wave gives a space its structure: who is responsible, what state a page is in, how pages are grouped, how new pages start, and how old ones leave.

## What the study found

**How people expect to organise.** The behaviour comes from the study behind blueprint 03 (`03-functional/sources.md`), read again for this plan, plus the sources under Provenance.

- **Labels** are lower case without spaces, and typed spaces become dashes. The box suggests existing labels. They can be added while reading or editing, and removed with a control beside each label. A template passes its labels on to new pages.
- **Templates** come per space or for all spaces. A template with fields first shows a form, and the answers fill the page.
- **Blog posts** live outside the tree, and are otherwise pages. Treating them as a second kind of content doubles the work for no gain to the reader.
- **Archiving** takes a page out of the way without deleting it. People expect it to keep its address.
- **Clearing up** happens from one table with filters. Bulk actions must say what they will do first, because a space administrator cannot undo 400 moves by hand.

**What accessibility demands.**

| Source | Demand | What we do |
|---|---|---|
| Grid pattern | A table with mostly static content stays a table, and then every control in it is a Tab stop. A grid cuts the stops but needs its own focus handling. | The Content screen stays a plain `table`. Actions sit in one bar above it, so a row has two Tab stops: its checkbox and its link. |
| Grid pattern | Selected rows say so. | Checked rows have a visible tint and a real checkbox, so state never rests on colour. |
| Combobox pattern | Focus stays in the input, and the list is pointed at. | The label box, the owner picker and the approver picker are comboboxes, as quick search is. |
| 2.5.7 Dragging | Everything dragged has another way. | Moving several pages, moving to another space and sorting a level all work from a dialog. |
| 3.3.4 Error prevention | An action with a large effect can be checked first, or reversed. | Bulk actions show a summary first. Deleting a space asks for its key. |
| 3.3.2 Labels or instructions | Fields in a template form have names and help. | The form comes from the template's field names, and a required field says so. |
| 4.1.3 Status messages | Progress reaches screen readers without moving focus. | Bulk jobs, purges and background copies report through `role="status"`. |
| 1.4.1 Use of colour | Colour alone may not carry meaning. | A page status is always a word and a colour. Verified and Approved have an icon and a word. |
| 1.3.1 Info and relationships | Filters and results belong together. | Search filters are one labelled `form`, and the count is announced. |

**What PostgreSQL gives us.** `websearch_to_tsquery` handles quotes and `or` and never raises an error, which is close to what ORG-028 to 032 ask for. It has no brackets, no capital `NOT`, no wildcards and no field filters, so we parse the search language ourselves and keep the rule that nothing typed can act as query syntax. `<->` gives phrases, and `:*` gives prefixes. Wildcards inside a word have no index support, so they need a limit.

**What the code already has.** `page.archive` exists as a permission and is in the editor role, with nothing behind it. `permissions.Evaluate` knows archived spaces through `worksWhenArchived`. `jobs` runs queued work with a result for the person. `page_links` from wave B answers broken and orphaned pages. `pages.status` already means draft, published or trashed, so the word for what writers call a status must be another one in code.

**Safety.**

- **A leak through a label.** A label that only exists on hidden pages must not show up in suggestions or lists, and a count must not reveal how many hidden pages carry it.
- **A leak through spelling.** A "did you mean" taken from all text would reveal words on pages the person may not see. Suggestions come only from titles they may see.
- **Moving between spaces** changes who can read a page. A branch with pages the person cannot see must not move, for the same reason a delete is refused.
- **A restriction must survive.** Archiving, moving up a level and moving to another space can all drop an inherited view restriction, so they all go through the same check as a move.
- **Handing over a page** must not hand over a right. *Delete own* keeps meaning the creator.
- **Template answers** are plain text. They become text nodes and pass through `Normalize`, so an answer cannot carry a link or a block.
- **Bulk actions** check every page on their own. A selection is never a permission.
- **Scheduled publishing** runs later, under the rights of that moment.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | `pages` gets a `kind`: page, post, folder, link, template | Versions, drafts, restrictions, comments, files and search then work for all of them. A second table would copy every rule. |
| D2 | A page status is called `stage` in code, and Status in the interface | `status` is the life cycle of a page, and two meanings of one word cause bugs. |
| D3 | Archived is its own fact (`archived_at`), not a value of `status` | The status check stays, `perm_visible_pages` still lets a reader open an archived page by address, and no constraint changes during an upgrade. |
| D4 | A new function `perm_listed_pages` is what lists use: visible and not archived | One place decides what a list shows. A list that forgets the filter would bring archived pages back. |
| D5 | `perm_visible_pages` is replaced in the migration to leave out templates, and `Evaluate` refuses them | A template must never appear in a list or open as a page, whichever list is written next. |
| D6 | A template is a hidden page of `kind` template, and instance templates live in a system space with no grants | The same editor, versions, comparison and copied files come for free (ORG-047, 052, 060). The `templates` module holds the rule and uses unchecked calls, as `exchange` does. |
| D7 | Labels are text on a page, with no table of labels | A label nobody uses disappears by itself (ORG-041), and rename and merge are one statement. |
| D8 | Labels on files wait for wave E | They belong with the file list. |
| D9 | *Delete own* stays with the creator | Handing over a page gives no new right, and the permission rule does not change. |
| D10 | Verified belongs to the owner and space administrators, with *edit* | PAG-059 asks for "the right permission". This needs no new permission, and so no change to the matrix for every existing space. |
| D11 | An archived page stays in `page_tree` | It keeps the restrictions it inherits while it is away. |
| D12 | A page below an archived page cannot be restored on its own. The dialog offers to restore the parent as well | PAG-073 says "to the top", but that would drop inherited restrictions. It is the same reasoning as PAG-081. See "For the product owner to decide". |
| D13 | Moving a branch to another space is refused when it holds pages the person cannot see | Otherwise someone could change who reads a page they cannot see themselves. |
| D14 | The move impact names restrictions and spaces, never the people of a target space | Those lists are for its administrators. |
| D15 | A move locks both spaces in the order of their ids | Two moves in opposite directions would otherwise deadlock. |
| D16 | Scheduled publishing uses the draft at that moment, and the rights at that moment | PAG-026 says later edits are included. If the rights are gone, nothing is published and the person is told. |
| D17 | Approval works on a frozen copy of the draft | Approvers must see what they sign, and publishing only counts for that exact text. |
| D18 | The search language is our own small parser, and it never fails | Requirement ORG-032. Every term ends up as letters and digits, as today. |
| D19 | Wildcards inside a word use a prefix plus a check on the text, at most three per search | There is no index for them, so they need a limit. |
| D20 | Spelling suggestions use titles the person may see, compared in Go | No extension to install, and no leak. |
| D21 | The total of a search stops at 1,000 and then says "1,000 or more" | An exact count over a large instance costs more than it tells. |
| D22 | The Content screen shows and acts only on pages the person may see | Administrators do not read content by default, and that holds for space administrators too. |
| D23 | Tree selection has a Select mode | Ctrl and click already open a link in a new tab. In Select mode the row is a checkbox, and Ctrl and Shift work on it. |
| D24 | Dropping a row on a space opens the Move dialog, and does not move at once | A move between spaces always shows its impact first (PAG-044). |
| D25 | Features live in a small `features` package with its own table | Pages, comments and spaces all ask it. `pages` cannot import `spaces`. |
| D26 | Turning a feature off refuses new content with a clear error and hides what exists | SPC-048: nothing is deleted. |
| D27 | Space owners are a chosen list, which falls back to the administrators | SPC-013 asks for people to ask, and they need not be the people who hold the rights. |
| D28 | A space icon image is cropped by the style sheet, with no crop tool | A tool is a lot of code for a small picture. |
| D29 | Bulk actions run as jobs, one transaction per page | One failure must not stop the rest, and SPC-064 asks for a report. |
| D30 | Removing a version deletes the row and leaves a marker | Versions stay immutable: the trigger forbids changes, and a purge already deletes rows. |
| D31 | Starter templates are our own words, shipped in the binary | Clean-room rules apply to templates in particular. |
| D32 | A space template holds text only: pages, labels and templates | Files would make a bundle large for little gain. |

## Scope

**In:**

- Chapter 8: ORG-015 to 024, 026, 028 to 032, 034 to 041, 043 to 047, 048 (labels), 049 to 074.
- Chapter 3: PAG-003, 004, 008, 010, 016, 017, 019, 024 to 030, 033, 034 (the warning), 035 (the part without requests), 036, 040, 042, 044, 045, 048 to 050, 051 to 061, 062 to 069, 070 to 075, 084, 092, 093, 096, plus what spec 009 left open in PAG-103, 115, 116, 119, 126.
- Chapter 2: SPC-006, 008, 011 to 013, 015, 020, 026 (Blog), 038 (Archive), 039, 043, 044, 047 to 049, 060 to 067, 070, 073, 075.
- Chapter 1: NAV-019.

**Out, and where it goes:**

| Item | Goes to |
|---|---|
| Page facts in templates (ORG-048, BLK-039) | Wave E, with dynamic blocks |
| Labels on files (ORG-042) | Wave E, with the file list |
| Text inside attachments (ORG-025) | Wave E, and it needs a decision by the product owner |
| Word import when creating a page (PAG-005) | Wave G |
| Which searches found nothing (ORG-027), last viewed in the Content screen | Wave H, with analytics |
| Inviting someone into a draft (PAG-018), the full version of PAG-034 | Editing together |
| Space categories (SPC-014), Manager role, own role sets (SPC-054, 055), personal spaces | Wave F |
| Stemming per language (ORG-033), a feed per space (COL-081) | Later, as blueprint 03 says |

## Feature plan

### D1. Page kinds

`pages.kind` is `page` by default.

| Kind | In the tree | Has text | Notes |
|---|---|---|---|
| page | yes | yes | As today |
| post | no | yes | D5 |
| folder | yes | no | D6 |
| link | yes | no | D6 |
| template | never | yes | D4 |

- **Order:** new siblings and `reposition` only count kinds that sit in the tree, or posts and templates would take up positions.
- **Tree:** `Tree()` lists page, folder and link.
- **Search:** templates are never indexed. Folders and links are found by title.

### D2. Owner, status, verified and review

- **Owner:** `pages.owner_id`, at first the creator, filled in by the migration. The byline shows the owner, and the row card as well.
- **Hand over:** in Details, Change owner, with a person picker, for those with *edit*. The new owner gets a notification. Audited as `page.owner_changed`.
- **Disabled owner:** the Content screen filters on it, so a space administrator can pick a new one (PAG-053). The name stays visible, marked as no longer active.
- **Status:** a word and a colour name at the top of the page, in the byline, the row card, lists, search results and Content.
  - Suggested: *Early draft*, *Being written*, *Needs review*. A space administrator changes the list, up to six, or turns the feature off.
  - **Own word:** only when the space allows it, at most 30 characters.
  - **While reading:** a small menu by the byline, applied at once.
  - **While editing:** in the publish options, applied when publishing.
  - Not part of a version. Audited as `page.stage_changed`.
- **Verified:** the owner or a space administrator marks a page verified until a date. Readers see who and when, with an icon and a word. A later edit leaves the mark and adds "changed after verification". Past the date it shows as expired.
- **Review date:** `pages.review_on`. A space can set a default in months, used when a page is first published. A daily task reminds the owner once. A page past its date shows a quiet note to editors, and is a filter in Content.
- **Edge cases:** a removed status word in the space list stays on the pages that have it, until they change. A review reminder for a disabled owner goes to the space owners. A verified date in the past is refused.

### D3. Labels

- **Adding:** a combobox at the bottom of the page, in Details, and in Page look in the editor. Enter or a comma adds a label and keeps the box open. Applies at once, and makes no version.
- **Writing:** capitals become small letters and spaces become dashes while typing. Letters of any language, digits, dashes and underscores. At most 64 characters, and 50 labels on a page.
- **Suggestions:** from labels on pages the person may see, in that space first.
- **Removing:** each label has its own button, named "Remove label x". Removing the last use makes it disappear.
- **Screens:** `/labels/:label` lists pages with that label, can be limited to a space, and `+` combines labels: pages with all of them. `/labels` and `/spaces/:key/labels` list labels by name or by use, with the number of pages the person can see.
- **Rename and merge:** a space administrator does this in one action on the space's list. Renaming onto an existing label merges them. Audited.
- **Fixed list:** a space administrator can switch it on. Then only those labels can be added, and the box says so. Labels already on pages stay.
- **Edge cases:** an archived page refuses changes. A template passes labels on, and a label off the fixed list is left out and reported.
- **Audit:** `page.labelled` and `page.unlabelled`, with an outbox event each.

### D4. Templates

**Choosing.** Create offers Page from template. An empty new page shows a panel beside the editor, which closes when typing starts and can be opened from the strip (PAG-004).

- **Gallery:** a dialog with a search box, categories and a preview in the document style. Favourites and recently used come first, promoted ones next.
- **Keyboard:** the list is a `listbox`, with arrows moving and Enter choosing. Preview follows the marked row.

**Writing.** `/templates/:id/edit` uses `DocumentEditor`. A space administrator makes templates for a space, an administrator for all spaces.

- **Details:** name, description, category, labels, title pattern, default parent.
- **Fields:** a new inline block, `templateField`, with a name and a kind: one line, several lines, or a choice. `/` offers Field. Fields with the same name share one value. At most 20, with answers up to 2,000 characters.
- **Instruction text:** a new inline block, `instruction`. It shows in the editor in a muted style, disappears on typing, and is removed at publishing, so a reader never sees it.
- **Title pattern:** `{date}`, `{space}` and field names.
- **Guided page:** a template can group fields into steps. The form then shows a step at a time with Back and Next, and the last step sets the labels and parent.
- **Saving from a page:** More, Save as template, for those who may make one. Files are copied.
- **Versions:** each save is a version, and the history and comparison are the normal screens.

**Using.** The form comes first when there are fields. The new page is an unpublished page with a draft. Images and files are copied, and labels are added. Later changes to the template change nothing.

**Managing.** Space settings and Administration list templates: edit, hide, promote, delete, export, import. Export is one JSON file with its files, at most 20 MiB. Import goes through the same checks as a publish.

**Starter set.** Ten templates in our own words, shipped in the binary with a stable key: meeting notes, decision record, project plan, how-to guide, incident review, retrospective, requirements, onboarding plan, team home, weekly update. An administrator can hide them. An upgrade changes a starter only if nobody edited it.

**Space templates.** An administrator saves a space as a template, or uses one of two starters (team, project). Creating a space offers them, and it runs as a job past 50 pages. At most 200 pages. The form for a new space also says who will get access, and an administrator sets the default permission set (SPC-008).

**Edge cases:** a template's default parent that has gone is ignored. A field that was removed from the template drops its answer. A person without *create* in the space sees no gallery. An import with an unknown block is refused as a whole.

### D5. Blog posts

- **Feature:** Blog is a feature of a space, on by default for new spaces.
- **Creating:** Create offers Blog post. A post has a title, an author, a publication date and an optional header image.
- **Life:** the same editor, drafts, versions, comments, reactions, labels and restrictions. A post has no parent and no child pages. Its address is `/pages/:id`, and that id never changes.
- **Lists:** the Blog entry in the sidebar, at `/spaces/:key/blog`, newest first, with title, author, date and excerpt, grouped by year and month. `/blog` lists posts from all spaces. Both load in parts.
- **Publication date:** the first publish sets it. Later versions keep it. A scheduled post gets the scheduled time.
- **Converting:** an unpublished page can become a post and back. The dialog says what happens to its place in the tree.
- **Following:** Watch on the blog of a space, and on all posts, through wave C's watching.
- **Elsewhere:** quick search gets the group Blog posts, search has it as a content type, and the overview shows the newest three (SPC-020).
- **Edge cases:** turning Blog off hides the entry and refuses new posts, and addresses keep working. A page with children cannot become a post.

### D6. Folders, link entries and company home

- **Folder:** a tree entry that groups pages. Opening one shows its child pages. It can have restrictions, and these work as on a page. Row menu: Turn into folder, and Turn into page. Its old text stays in its versions and comes back. The home page cannot be a folder.
- **Link entry:** a tree entry with an address, added from the Add menu of the tree. It opens the address, with an icon that says it leaves. Addresses pass `SafeHref`. It has no children.
- **Company home:** an administrator picks a page in Administration. Home shows that page to everyone who may see it, with a link to the personal start screen. A preference makes the personal screen the default. Someone who may not see the page gets the personal screen.
- **Sort a level:** the Content screen and the row menu sort all children of a page by title, created or changed, after a question. Audited.

### D7. Publishing and drafts

The arrow beside Publish opens the publish options:

- **Location:** another parent or space, with the rules of a move. The dialog shows the impact.
- **Status:** see D2.
- **Notify watchers:** on by default. Off is a silent update: the outbox event says `silent`, and wave C sends nothing. The history still shows the version.
- **Schedule:** a date and time in the person's zone. The page shows a mark, and the person can change or cancel it. Editing goes on, and the draft of that moment is published. A task checks every minute, and publishes under the person's rights. If they are gone, it stops and tells them.
- **Preview:** shows the document as a reader will see it, in a dialog, without leaving the editor.

Other parts:

- **No title first:** Create can skip the dialog. The editor opens with focus in the title. A page without a title shows as Untitled in the person's lists, and cannot be published.
- **Same title:** while typing, a note links to a page with the same title in that space, only when the person can see it.
- **Page that does not exist yet:** a link to `/spaces/:key/new?title=…`. Someone with *create* who follows it starts that page, and others see a plain "does not exist".
- **Safe text:** if saving fails, the text stays in the browser and is offered back after a crash or a closed tab. Close waits for the save, or asks.
- **Old drafts:** a setting, one year by default. A task warns by notification 14 days before, then removes.
- **Marks:** Unpublished changes beside the title. **Revert to published** in the editor throws away the changes, after a question.
- **Someone else is editing:** when another person saved a draft of this page in the last 10 minutes, the editor says who.
- **After publishing:** a message offers Share (wave C).
- **No edit rights:** no Edit button, and if a request for access is open it says so (wave C).

### D8. Moving and copying

- **Move dialog:** tabs Tree and Search, and a Space field. It starts over from the top, and shows what will change before it runs.
- **Between spaces:** needs view and *create* in the target, and the rights to move in the source. The whole branch moves, with files, history, labels and watchers. The home page cannot move. Favourites stay for those who can still see the page, which holds by itself.
- **Impact:** the number of pages, inherited restrictions lost and gained, and that everyone who can see the target space can read it, unless restricted.
- **Lost restriction:** needs *manage restrictions*, as today.
- **Copy dialog:** target space and parent, keep labels, keep restrictions. Comments, versions, favourites and watchers are never copied.
- **Titles:** prefix, suffix, or find and replace, with a preview of the first five.
- **Large copies:** past 50 pages a copy is a job, which reports when it is done.
- **Several rows:** the Select button in the tree shows checkboxes. Ctrl, Shift and Space work on rows. A bar shows the count, with Move and Archive.
- **Dragging to a space:** while dragging a row, holding it over the Spaces menu opens that menu, and dropping on a space opens the Move dialog filled in.
- **Edge cases:** a target that is archived is refused. Pages a person cannot see in the branch refuse the move, and it says so. A move into a space with a fixed label list keeps labels.

### D9. Approvals

- **Asking:** a person with *edit* picks up to ten people, with a message and an optional date. The draft or the current version is frozen in a round. Approvers are notified, and a person who cannot see the page is pointed out before sending.
- **Answering:** Approve or Ask for changes, with an optional comment. A person can change their mind until the round ends.
- **Showing:** a banner under the byline, and a panel that lists who was asked, who answered what and who is still to answer. Everyone who can see the page can see it.
- **Result:** when all approve, the page gets Approved, with that version. A new version removes the mark and shows that the approved one is older, with a link to compare.
- **Requiring:** a space administrator can require approval before publishing, and name up to ten fixed approvers. Publishing is then refused unless the text equals an approved copy.
- **History:** past rounds stay in Details.
- **Edge cases:** a disabled approver can be removed from a round by the asker. A person cannot approve their own round when others are named. Any new round closes the open one.
- **Unpublished pages:** named approvers see the frozen copy when they hold view on the space and pass the restrictions of the parent.

### D10. Archiving

- **Page:** More, Archive, for those with `page.archive`. A question offers to include the pages below.
- **Pages left behind:** pages below that the person may not archive stay in the tree and move up a level. The dialog warns, and the move needs *manage restrictions* if a restriction would be lost.
- **Archived page:** keeps its address and shows a banner. It can be read, exported and deleted. It cannot be edited, moved, labelled or commented on. `Evaluate` holds this rule.
- **Lists:** gone from the tree, quick search, lists and feeds. Advanced search can include it.
- **Archive screen:** `/spaces/:key/archive` shows archived pages in their tree, with a filter and Restore.
- **Restore:** back to its place. Below a page that is still archived, the dialog offers to restore that one too (D12).
- **Bulk:** from Content.
- **Edge cases:** the home page cannot be archived. Deleting a page takes archived pages below it along.

### D11. Trash and versions

- **Trash:** can be searched and sorted, shows who deleted what, and has bulk restore and purge after a question.
- **Compare:** tables compare per cell, and images and files show as changed blocks with their names.
- **What changed:** `recent_pages` remembers the version a person last saw. If the page moved on, a link beside the byline opens the comparison. A first visit shows nothing.
- **Removing a version:** an administrator removes one version with a reason, for a leaked secret. The current version cannot be removed. The history shows the number as removed, and the audit log keeps who, when and why. The comparison refuses it clearly.

### D12. The Content screen

`/spaces/:key/content`, for space administrators, with a link in Space settings.

- **Table:** title, owner, status, labels, created, last changed, in pages of 50, sortable.
- **Filters:** owner, including disabled people; status; label; contributor; changed; created; not active for 1, 3, 6 or 12 months; has a restriction; links to pages that are gone; no incoming links and far from the top; past review date; owner disabled. Filters are in the address.
- **Not active:** leaves pages with children out, unless a box says otherwise.
- **Selection:** checkboxes. Selecting a parent selects its children, and each can be cleared. Select all takes the page of results, then offers all results.
- **Actions:** archive, delete, change owner, change status, add or remove a label, move.
- **Summary:** each action first shows what it will do, with the count. A dry run on the server gives the numbers.
- **Running:** a job, with one transaction per page. The screen shows progress, and at the end lists what worked and what did not, with reasons.
- **Edge cases:** a page the person lost access to in between is reported as failed. Closing the screen does not stop the job, and a notification says when it is done.

### D13. Space settings

- **Key:** can be changed. The old key becomes an alias that redirects, and stays taken. Audited.
- **Icon:** an uploaded PNG, JPEG, GIF or WebP up to 1 MiB, or an emoji.
- **Owners:** a list of people shown on the overview and in the directory.
- **Home page:** a published page of the space.
- **Title alignment:** the default for new pages.
- **Features:** Blog, Comments, Reactions, Page statuses, Analytics, Automation, Calendar. Administrators set the defaults for new spaces, and can turn one off for the instance.
- **Deleting:** the dialog asks for the key, and offers Archive and Export first.
- **Permanent removal:** a job with progress, because a large space takes time.
- **Search in archived spaces:** a filter on the results screen.

### D14. Search

**Filters on the results screen,** in one form, all in the address:

- Content type: pages, blog posts, comments.
- Label and contributor, several meaning **or**. Contributor means wrote, edited or commented.
- Status, below a page, titles only.
- Date: last day, week, month, year, or a range.
- Space, author, and include archived.

**Results:** sorted by relevance or date. Breadcrumbs under each result are links. Results load 25 at a time, with a total. No results suggests removing filters, and shows a similar spelling. Comments link to the comment.

**Other tabs:** People and Spaces, in tabs on the same screen. People follows *view profiles*.

**Search language.**

| Typed | Means |
|---|---|
| `"open plan"` | That exact phrase |
| `plan AND budget`, `plan OR budget`, `NOT draft` | Capitals only. Small letters are words |
| `(plan OR budget) AND 2026` | Brackets group |
| `pla?`, `pl*n`, `plan*` | One character, any number. Not at the start of a word |
| `title:plan`, `label:hr`, `space:ENG`, `author:anna` | Field filters, the same as the filters |

Anything the parser cannot read falls back to plain words. It never gives an error. At most 20 terms and 3 wildcards.

**Edge cases:** an unknown field such as `foo:bar` is two plain words. `author:` matches the names of people only for those with *view profiles*, and otherwise is ignored. A label the person cannot see finds nothing.

## API

All new routes, with their access rule.

| Route | Rule | What |
|---|---|---|
| `PUT /pages/{id}/owner` | `page.edit` | Hand over |
| `PUT /pages/{id}/stage` | `page.edit` | Status |
| `PUT, DELETE /pages/{id}/verification` | `page.edit` | The module asks for owner or administrator |
| `PUT /pages/{id}/review` | `page.edit` | Review date |
| `GET, POST /pages/{id}/labels`, `DELETE /pages/{id}/labels/{label}` | `page.view`, `page.edit` | Labels |
| `GET /labels`, `GET /labels/{label}/pages` | list | Only what the caller may see |
| `GET, PUT /spaces/{id}/labels` | `space.view`, `space.administer` | List, fixed list |
| `POST /spaces/{id}/labels/rename` | `space.administer` | Rename and merge |
| `GET /spaces/{id}/templates`, `GET /templates` | `page.create`, list | Gallery |
| `POST, PATCH, DELETE /spaces/{id}/templates…` | `space.administer` | Space templates |
| `POST, PATCH, DELETE /admin/templates…` | permission `administrator` | Instance templates |
| `GET /templates/{id}`, `POST /templates/{id}/use` | list | Module checks *create* |
| `GET /templates/{id}/export`, `POST /…/templates/import` | as above | Bundle |
| `POST /pages/{id}/save-as-template` | `space.administer` | From a page |
| `GET, POST /admin/space-templates` | permission `administrator` | Space templates |
| `POST /pages/{id}/kind` | `page.edit` | Folder, page, post |
| `GET /spaces/{id}/posts`, `GET /posts` | `space.view`, list | Blog |
| `PUT, DELETE /pages/{id}/schedule` | `page.edit` | Scheduled publishing |
| `GET /pages/{id}/move-impact` | `page.edit` | Before a move |
| `POST /pages/{id}/move`, `/copy` | as today | Gain a space and options |
| `POST /pages/bulk-move` | list | Module checks each page |
| `POST /pages/{id}/sort-children` | `page.edit` | Sort a level |
| `POST, GET /pages/{id}/approvals`, `PUT /approvals/{id}/answer`, `POST /approvals/{id}/close` | `page.edit`, `page.view`, list | Rounds |
| `POST /pages/{id}/archive`, `/unarchive` | `page.archive` | Archiving |
| `GET /spaces/{id}/archive` | `space.view` | Archive screen |
| `GET /spaces/{id}/content` | `space.administer` | Content screen |
| `POST /spaces/{id}/content/preview`, `/content/run` | `space.administer` | Dry run, job |
| `POST /spaces/{id}/trash/bulk` | `space.administer` | Trash |
| `DELETE /admin/pages/{id}/versions/{number}` | permission `administrator` | With a reason |
| `PUT /spaces/{id}/key`, `/owners`, `/home`, `/features`, `/statuses` | `space.administer` | Settings |
| `POST /spaces/{id}/icon`, `GET /spaces/{id}/icon` | `space.administer`, `space.view` | Icon |
| `GET, PUT /admin/features`, `PUT /admin/company-home` | permission `administrator` | Defaults |

`GET /search` gains the filters of D14 and returns `total`, `capped`, `suggestion` and breadcrumbs. Preferences gain `start_screen`.

## Data

Migration 0008, expand only.

- `pages`: `kind`, `owner_id`, `stage_text`, `stage_color`, `review_on`, `archived_at`, `archived_by`, `published_at`, `link_target`.
- `spaces`: `system`, `free_stage`, `review_months`, `approval_required`, `fixed_labels`, `title_align`, `icon_object`.
- `recent_pages`: `seen_version`.
- New tables: `page_labels`, `space_fixed_labels`, `space_stages`, `space_owners`, `space_approvers`, `space_key_aliases`, `space_features`, `page_verifications`, `scheduled_publications`, `approval_rounds`, `approval_answers`, `removed_versions`, `templates`, `template_favourites`, `template_uses`, `space_templates`.
- `perm_listed_pages(u)`: new. `perm_visible_pages(u)`: replaced, to leave out kind `template`.
- The migration sets `owner_id` to `created_by`, and `published_at` to the first version's time. Existing spaces get every feature on.
- `settings`: `draft_retention_days`, `company_home_page_id`, `hidden_starters`, feature defaults.

Document format, `SchemaVersion` unchanged: `templateField` and `instruction`. `Normalize` only lets them through in templates and drafts, and publishing a page removes `instruction` and refuses `templateField`.

## Build order

The wave is large, so it goes in steps that each stay green. One commit at the end, as the product owner asked.

1. Go: migration, kinds, listing functions, `Evaluate` for archived pages and templates, matrix test.
2. Owner, status, verified, review, labels, with their tests.
3. Archive, moves and copies between spaces, tree selection.
4. Templates, starter set, blog, folders, link entries, company home.
5. Publish options, scheduling, drafts, approvals.
6. Search language and filters.
7. Content screen, jobs, space settings, features, trash, versions.
8. Screens, pictures, docs, statuses in `03-functional`, handoff.

## Test plan

- **Go unit:** search parser with a table of cases, and a fuzz target `FuzzParseQuery`, because the nightly job fuzzes parsers. `FillFields`, instruction removal, label cleaning, title patterns.
- **Permission matrix:** archived pages and templates become dimensions of `Evaluate`, and the SQL must agree.
- **Go integration, failure paths first:**
  - A hidden page in labels, suggestions, counts, spelling, Content, the move impact and every search filter.
  - Moves between spaces: a hidden page in the branch, a lost restriction, an archived target, two moves in opposite directions at once.
  - Restore below an archived parent. Archiving with pages left behind.
  - Scheduled publishing after losing rights. Approval with changed text.
  - Bulk jobs with one failing page, and with a page that became hidden.
  - An answer with markup in a template. An import with an unknown block.
  - Removing the current version. Key aliases and a taken alias.
- **Route walk:** new object routes join `TestEveryObjectRouteIsProtected` by themselves.
- **Mutation:** archived pages stay writable; lists show archived pages; templates open as pages; a label reveals hidden pages; a spelling suggestion uses hidden titles; a move skips hidden pages; a restore drops a restriction; publishing ignores required approval; a scheduled publish ignores lost rights; handing over gives *delete own*; a bulk job skips the check.
- **Vitest:** label cleaning, title patterns, selection of parents with children, filters in the address.
- **Browser:** each section above, with the keyboard first.
- **Pictures:** `ContentView`, `LabelView`, `LabelsView`, `TemplateView`, `TemplatesView`, `BlogView`, `ArchiveView`, the gallery, the publish options, a page with owner, status, Verified, labels and approval, a folder, a post, the search filters. Empty and full, and dark and narrow where it matters.
- **Accessibility:** axe on each new screen, and with the dialogs open.

## Acceptance criteria

- [ ] AC-1 A page has an owner, and handing over gives no right to delete.
- [ ] AC-2 Status can be set while reading and while editing, follows the list of the space, and shows in the byline, row card, lists, search and Content.
- [ ] AC-3 Verified and the review date work, including a change after verification and a reminder.
- [ ] AC-4 Labels are cleaned while typing, suggest, combine, and can be renamed and merged. Hidden pages never show through a label.
- [ ] AC-5 The gallery works by keyboard, and a page from a template gets its text, labels and files.
- [ ] AC-6 Fields, instruction text, title patterns and steps work. An answer can carry no markup. A reader never sees instruction text.
- [ ] AC-7 Templates have versions, and changing one changes no page.
- [ ] AC-8 The ten starter templates are there, and can be hidden.
- [ ] AC-9 A space template creates a working space.
- [ ] AC-10 Blog posts can be written, listed, scheduled and followed, and stay out of the tree.
- [ ] AC-11 Folders, link entries and company home work.
- [ ] AC-12 Publish options work: location, silent update, status, schedule, preview.
- [ ] AC-13 A scheduled page publishes the latest draft, and stops when the person lost their rights.
- [ ] AC-14 Text survives a failed save and a closed tab, and old drafts expire after a warning.
- [ ] AC-15 A page moves to another space with everything on it, after showing the impact, and old addresses work.
- [ ] AC-16 A move is refused when it would touch hidden pages or drop a restriction without the right.
- [ ] AC-17 Copy works across spaces, with titles, and a large copy runs in the background.
- [ ] AC-18 Several rows move together, and a row can be dragged to a space.
- [ ] AC-19 An approval round runs from asking to Approved, and a new version removes the mark.
- [ ] AC-20 A space can require approval, and then only approved text can be published.
- [ ] AC-21 Archived pages are read-only, keep their address, stay out of lists, and can be restored without losing a restriction.
- [ ] AC-22 The trash can be searched, and bulk restore works.
- [ ] AC-23 What changed shows the differences since a person's last visit.
- [ ] AC-24 An administrator can remove one version with a reason, and it shows as removed.
- [ ] AC-25 The Content screen filters, shows a summary, and reports what worked and what did not.
- [ ] AC-26 A key can change, and old addresses redirect.
- [ ] AC-27 Features turn on and off without losing anything.
- [ ] AC-28 Deleting a space asks for its key, and a permanent removal shows progress.
- [ ] AC-29 All search filters work and live in the address, and a shared address gives the same results.
- [ ] AC-30 The search language works, and nothing typed gives an error.
- [ ] AC-31 Everything passes axe, and the new pictures have been looked at.

## Risks

| Risk | Answer |
|---|---|
| A list forgets archived pages or kinds | `perm_listed_pages`, and a test that walks each list |
| Posts or templates take positions among siblings | `insert` and `reposition` filter on kind, with a test |
| During an upgrade the old release shows posts in the tree | Only for the minutes both run, and nobody can make a post on the old release. Noted in the changelog |
| Replacing `perm_visible_pages` slows lists | One more condition on a row already read. The load test covers it in phase 3 |
| Wave C is not finished when this starts | Silent updates, Share and reminders need it. The build order starts with what does not |
| A bulk job holds locks on a large space | One transaction per page, and the tree lock is held only for that page |
| Wildcards are expensive | At most three, and a local statement timeout of 3 seconds |
| Template fields leak into a published page | Publishing refuses them, with a test |
| A key alias blocks a wanted key | It stays taken on purpose. A space administrator of that space can release it |

## How the product owner tests it

1. Open a page, set a status, add two labels, and follow a label.
2. Hand the page to someone else, and mark it verified until next month.
3. Create a page from Meeting notes. Fill in the form, and publish with Notify off.
4. Write a template with a field and instruction text, and use it.
5. Write a blog post, schedule it for two minutes later, and wait.
6. Turn a page into a folder and back. Add a link entry to the tree.
7. Move a branch to another space, and read what the dialog says first.
8. In the tree press Select, pick three pages, and move them.
9. Ask a colleague to approve a page, then change the page.
10. Archive a page with children, open its old address, and restore it.
11. Open Space settings, Content. Filter on "not active for 12 months", and change the owner of a few pages.
12. Search for `"exact words" AND label:hr`, then something full of odd signs.
13. Change a space's key, and open an old link.

## For the product owner to decide

1. **Restoring below an archived page.** PAG-073 says a page returns to the top when its parent is gone. We keep that for a parent that is really gone, and refuse while the parent is archived, offering to restore both. Otherwise an inherited restriction could be lost. Is that fine?
2. **Text inside attachments (ORG-025, wave E).** Reading PDF and Word files needs a new dependency, and the requirement asks for a separate process. Wave D needs no new dependency, so this can wait until the wave E study.

## Provenance

Own text from these public sources, read for behaviour only, plus the study behind blueprint 03 (`docs/blueprint/03-functional/sources.md`), which covers labels, templates, statuses, approvals, archiving, content management and search syntax.

- W3C WAI-ARIA Authoring Practices: grid pattern, combobox pattern (through spec 009).
- W3C WCAG 2.2: criteria named in the table above.
- PostgreSQL documentation: text search functions and operators.
- RFC 4287, the Atom format: a post's id must never change, which ours does not, if a feed comes later.
- Public help pages of a commercial wiki on labels, and older versions on creating a page from a template, reached through a search summary.
