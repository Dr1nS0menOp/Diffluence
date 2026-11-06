# 03.3: Pages: life cycle and reading

A page goes through a simple life: a private draft, a published page, new versions, and at the end the archive or the trash. Every step is on record, and nearly every step can be undone.

## 3.1 Creating a page

- **PAG-001** · built · A person with *create* adds a page from the Create button, from the add button in the sidebar, or from the add button on a tree row.
- **PAG-002** · built · A dialog asks for a title. The page opens in the editor.
- **PAG-003** · built · The dialog can be skipped: the editor opens with an empty title field that has the focus. A page without a title keeps its draft, and cannot be published.
- **PAG-004** · partly · The editor offers a template panel beside an empty page. It closes as soon as the person starts typing, and can be opened again from the editor menu (ORG-045). Still open: the gallery opens as a dialog from a note above the editor, not as a panel beside it.
- **PAG-005** · new · Create also offers importing a Word or Markdown file into the new page (chapter 11).
- **PAG-006** · built · A new page starts unpublished: only its creator sees it.
- **PAG-007** · built · A child page cannot be created below an unpublished page.
- **PAG-008** · built · Titles need not be unique, but when a space already holds a page with the same title the editor gives a warning with a link to it.
- **PAG-009** · built · A title has at most 255 characters.
- **PAG-010** · built · A link to a page that does not exist yet can be written as a placeholder (EDT-064). A person with *create* who follows it starts that page.

## 3.2 Drafts and saving

- **PAG-011** · built · While the editor is open, the draft is saved after each pause in typing.
- **PAG-012** · built · A status beside Close says saving, saved, or that saving failed.
- **PAG-013** · built · Every person has at most one draft per page. It is private until the page is published.
- **PAG-014** · built · A person who leaves with a draft sees a note on the page, with a link back to the draft.
- **PAG-015** · built · Discarding a draft asks first, and cannot be undone.
- **PAG-016** Â· built Â· If saving fails, the text stays in the browser until it can be saved, and is offered back after a crash or a closed tab.
- **PAG-017** Â· built Â· Leaving the editor with unsaved changes never loses them. Close only leaves when the draft is safe, or asks. Leaving any other way saves first, and when that fails a message leads back to the text.- **PAG-018** · new · A person can invite someone to look at their draft by sending a link. The invited person needs edit rights on the page and sees the same draft.
- **PAG-019** · built · Drafts older than a configurable period (default: one year) are removed after a warning by notification.
- **PAG-020** · built · Saving a draft is not audited, because it is private work and autosave would flood the log.

## 3.3 Publishing

- **PAG-021** · built · Publish creates a new version and shows the page to everyone who may see it.
- **PAG-022** · built · An optional summary says what changed. It shows in the history.
- **PAG-023** · built · If someone else published in between, the person is told. They can compare, keep editing, or publish anyway.
- **PAG-024** · partly · A small arrow beside Publish opens the publish options: location, who can see it, notify watchers or not, and a schedule. Still open: who can see it.
- **PAG-025** · built · **Silent update:** the person can choose not to notify watchers, for small fixes.
- **PAG-026** · built · **Scheduled publishing:** a page or a change can be published at a chosen date and time. Until then a clear mark says so, the person can change or cancel it, and no notification is sent. Later edits are included.
- **PAG-027** · built · **Publish location:** a person can pick another parent or space at publishing time, with the same rules as a move.
- **PAG-028** · built · A page can be previewed before publishing, in the reading layout, without leaving the editor.
- **PAG-029** · built · Space settings can require approval before publishing (PAG-062).
- **PAG-030** · new · After publishing, a short message offers to share the page, with a link that undoes nothing but opens the share dialog.

## 3.4 Editing a published page

- **PAG-031** · built · Edit opens the editor with the published text, or with the person's draft if there is one.
- **PAG-032** Â· built Â· The top bar and the sidebar with the pages of the space stay while editing, as far as there is room.
- **PAG-033** Â· built Â· An **Unpublished changes** mark sits by the title for the person who has a draft on the page.
- **PAG-034** Â· partly Â· Several people can edit at once (COL-088). Until then, the editor warns when someone else is editing the same page. Still open: editing together. The warning is built.
- **PAG-035** Â· partly Â· A person without edit rights sees no Edit button. If they have asked for access before, it says so. Still open: saying that access was asked for.
- **PAG-036** Â· built Â· **Revert to published:** in the editor, a person can throw away all their changes since the last version.
## 3.5 Renaming, moving, copying

- **PAG-037** · built · Rename changes the title and creates a version.
- **PAG-038** · built · The address stays the same after a rename or a move.
- **PAG-039** · built · Move opens a dialog with the tree of the space, to choose a new parent and position.
- **PAG-040** · built · The Move dialog has a search tab to find a parent by title, and can start over.
- **PAG-041** · built · Moving a page takes its whole branch, its attachments and its history along.
- **PAG-042** · built · A page can be moved to another space. The person needs view and create in the target space, and the rights to move it in the current space. Everything below it moves too.
- **PAG-043** · built · A move that would remove an inherited view restriction needs *manage restrictions* and says so before it happens.
- **PAG-044** · built · A move shows what will change in who can see the page before it is carried out.
- **PAG-045** · built · A move to another space keeps old addresses working. Labels and watchers stay, and favourites stay when the person can still see the page.
- **PAG-046** · built · Copy creates an independent page with the same content, attachments and direct restrictions. The title gets a prefix.
- **PAG-047** · built · Copy can include the visible pages below it.
- **PAG-048** · built · The Copy dialog picks a target space and parent, and whether to keep labels and restrictions. Comments, versions, favourites and watchers are never copied.
- **PAG-049** · built · When several pages are copied, their titles can be changed together with a prefix, a suffix or find and replace.
- **PAG-050** · built · A copy of a large branch runs in the background and reports when it is finished.

## 3.6 Status, owner and review

- **PAG-051** · built · A page has an **owner**: at first its creator. The owner shows in the byline, and is who people ask.
- **PAG-052** · built · An editor can hand over ownership. The new owner is told, and the owner's name appears in the Content screen (SPC-060).
- **PAG-053** · built · When an owner is disabled, the page is listed so a space administrator can pick a new one.
- **PAG-054** · built · A page can have a **status**: a coloured mark and a short word, at the top of the page.
- **PAG-055** · built · The suggested statuses are *Early draft*, *Being written* and *Needs review*. A space administrator can change the list, up to six, or turn statuses off.
- **PAG-056** · built · A space administrator decides whether editors may type their own status.
- **PAG-057** · built · A status can be set while reading (it applies at once) or while editing (it applies when publishing).
- **PAG-058** · built · A status shows in the tree card, lists, search results and the Content screen.
- **PAG-059** · built · **Verified:** a person with the right permission marks a page as verified until a chosen date. Readers see who verified it and when. A later edit does not remove the mark, but the page shows that it changed after verification.
- **PAG-060** · built · **Review date:** a page can have a date on which the owner is reminded to check it. Spaces can set a default, such as every 12 months.
- **PAG-061** · built · A page past its review date shows a quiet note to editors and is listed in the Content screen.

## 3.7 Approvals

- **PAG-062** · built · A page editor can ask chosen people to approve a page, with a message and an optional date.
- **PAG-063** · built · The approvers are notified. A banner under the byline shows that approval is under way.
- **PAG-064** · built · Each approver chooses Approve or Ask for changes, with an optional comment, and can change their mind until the round ends.
- **PAG-065** · built · The panel shows who was asked, who answered what and who is still to answer. Everyone who can see the page can see it.
- **PAG-066** · built · When everybody approves, the page gets the mark Approved, with the version that was approved.
- **PAG-067** · built · A new version after approval removes the mark and shows that the approved version is an older one, with a link to compare.
- **PAG-068** · built · A space administrator can require approval before a page is published, and can name up to ten fixed approvers per space.
- **PAG-069** · built · Past approval rounds stay on record with people, choices, comments and times.

## 3.8 Archiving a page

- **PAG-070** · built · A person with *archive* moves a page out of the tree into the archive of the space. It can be restored.
- **PAG-071** · built · Archiving a page can include everything below it. Pages below that the person may not archive stay in the tree and move up a level, and the dialog warns about what that does to restrictions.
- **PAG-072** · built · An archived page keeps its address and shows a banner. It cannot be edited, moved or commented on. It can be read, exported and deleted.
- **PAG-073** · partly · The **Archive** screen of a space lists archived pages in their tree, can be filtered, and restores them. A restored page returns to its old parent, or to the top if that is gone. Still open: the list is flat, not a tree.
- **PAG-074** · built · Archived pages do not appear in quick search or the tree. Advanced search can include them.
- **PAG-075** · built · Bulk archive is possible from the Content screen (SPC-063).

## 3.9 Deleting and the trash

- **PAG-076** · built · A person with *delete own* or *delete any* deletes a page. It goes to the trash with everything below it.
- **PAG-077** · built · A delete is refused when the person cannot view or delete every page in that branch, so nobody deletes what they cannot see.
- **PAG-078** · built · Deleting asks first.
- **PAG-079** · built · The dialog lists pages that link to the page, because those links will break.
- **PAG-080** · built · A space administrator restores a page from the trash of the space. It returns to its old place with its restrictions.
- **PAG-081** · built · A page cannot be restored while its parent is in the trash, so a restriction can never be lost that way.
- **PAG-082** · built · Purging removes a page, everything below it, its versions, search entries and files for good.
- **PAG-083** · built · After the retention period (default 30 days) a job purges expired items.
- **PAG-084** · built · The trash can be searched and sorted, shows who deleted what, and offers bulk restore and bulk purge.
- **PAG-085** · built · A person gets a short **Undo** right after deleting a page, also without *administer*, for as long as the message shows.

## 3.10 Versions

- **PAG-086** · built · Every publish creates an immutable version with number, title, document, author, time and summary.
- **PAG-087** · built · Everyone who can view a page can list its versions, open any of them and compare any two.
- **PAG-088** · built · The history lists the number, author, date, summary and whether it was restored from another version.
- **PAG-089** · built · Restoring a version publishes a new version with that content. Nothing is overwritten.
- **PAG-090** · built · Compare shows the page itself: added on green, removed on red and struck through, each with a word beside it, and changed words marked in place.
- **PAG-091** · built · Compare can collapse parts that did not change, with a control to show them.
- **PAG-092** · partly · Compare also shows changes in tables per cell, in images and in files. Still open: tables per cell. Images and files are built.
- **PAG-127** · built · Opening a version shows its changes against the version before it, with a button for the clean page.
- **PAG-128** · built · A mail about a change to a watched page shows those changes, when the instance allows content in mail and the person may still see the page. Several changes in one mail show everything since the first one.
- **PAG-093** · built · A reader sees **What changed** beside the byline after an update, with the differences since their last visit.
- **PAG-094** · built · An old version shows a banner: which version it is, who made it, and a link to the current one.
- **PAG-095** · built · Comments are not part of a version, and stay with the page when a version is restored.
- **PAG-096** · built · Deleting one version is not offered: versions are immutable, because the audit trail relies on them. If a secret ended up in a version, an administrator removes that one version with a reason, the audit log records it, and the version number stays visible as "removed".

## 3.11 Reading a page

- **PAG-097** · built · A page has breadcrumbs, the actions, the title, the byline and the document.
- **PAG-098** · built · The reading column is at most 47.5rem wide and centred in the space beside the sidebar. It keeps that width however short the text is.
- **PAG-099** · built · The breadcrumbs start with the space, and leave out the home page.
- **PAG-100** · built · Long breadcrumbs fold the middle into a menu.
- **PAG-101** · built · The actions on the page are Edit, the star, the lock and More. Rarely used actions live in More, never as a row of buttons.
- **PAG-102** · built · A **Share** button joins them (COL-042), and a **Watch** toggle joins More.
- **PAG-103** · built · The byline shows the owner, the date of the last update and the version.
- **PAG-104** · built · **Read time** is estimated at 230 words per minute, with a minimum of one minute. It is left out for very short pages.
- **PAG-105** · new · **View count** shows after the byline for those allowed to see analytics, and opens page insights (PLT-008).
- **PAG-106** · built · **Contributors:** selecting the byline shows who worked on the page, with their share of versions.
- **PAG-107** · built · **Page width:** a page scales with the window, between a comfortable minimum and maximum. Full width is chosen in the row of tools of the editor, stays with the page, and takes all the room.
- **PAG-108** · built · **Contents beside the page:** on wide screens, a page with three or more headings shows a floating list of headings that follows the scroll position. It can be turned off per person.
- **PAG-109** · built · Every heading has a link icon on hover or focus, which copies an address that jumps straight to that heading.
- **PAG-110** · built · An address that jumps to a heading scrolls to it and marks it briefly.
- **PAG-111** · built · Long code blocks and wide tables scroll inside their own frame, so the page itself never scrolls sideways.
- **PAG-112** · built · Selecting text shows a small floating button to comment on it (COL-013) and to copy a link to the selection.
- **PAG-113** · built · Selecting an image opens it large on top of the page, with arrows to the next image.
- **PAG-114** · built · A page can be shown in a presentation mode: full screen, one section per heading, moved with the arrow keys.
- **PAG-115** · built · **Page information** in More shows the owner, created, last changed, length, read time, labels, incoming links, outgoing links, watchers and the page's address.
- **PAG-116** · built · A **Details** panel opens beside the page, from a small button at the far edge, with labels, attachments, status and information on one screen. It replaces the need for many menu items.
- **PAG-117** · built · **Incoming links** lists all pages the person may see that link to this page.
- **PAG-118** · built · The page lists its child pages at the bottom when the document does not do so already, and this can be turned off per page.
- **PAG-119** · partly · At the bottom of a page: labels, then reactions, then comments (chapter 7). Still open: labels (wave D).
- **PAG-120** · phase 4 · Readers can see who else is on the page right now, as small pictures by the actions. Administrators can turn it off.
- **PAG-121** · built · **Reader mode:** a person can hide the sidebar and top bar with one key, to read without distraction.
- **PAG-122** · built · Stored documents are only ever rendered by the editor, so no HTML from a document reaches the browser.

## 3.12 The page header

- **PAG-123** · built · A page can have an emoji beside its title. It shows in the tree, breadcrumbs, cards, search results and the browser tab.
- **PAG-124** · built · A page can have a **header image** above the title. It is chosen by upload, repositioned by dragging up or down or with a range field, and can be changed or removed.
- **PAG-125** · built · The header image is an attachment of the page, and follows its rules. External image libraries are not used, because they would tell a third party who is reading.
- **PAG-126** · built · The title can be left-aligned or centred, and a space administrator can set a default.
