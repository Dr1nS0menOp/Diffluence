# 001: Spaces, pages, versions, drafts

Status: built, waiting for the product owner's acceptance session (written by Claude from the blueprint)
Phase: 1

## Problem
People need a place to write and organise documents: spaces that hold a tree of pages, an editor, and a history they can trust.

## Behaviour

**Spaces.** A user with *create spaces* makes a space with a key (2 to 32 letters, digits, dashes or underscores, unique ignoring case), a name, a description and an icon. The creator becomes space admin, and a home page is created automatically. Space admins can edit the details, archive the space (read-only for everyone, reversible) and delete it (to the trash).

**Pages.** A new page starts as an unpublished page: only its creator sees it. It becomes visible to others when it is published for the first time. Child pages cannot be created under an unpublished page.

**Tree.** Pages have one parent or none, any depth, and a manual order. Move and reorder are one action (new parent plus position). Moves stay within a space in this spec. A page cannot be moved under itself or its own descendants. Tree changes in one space take turns, so concurrent moves cannot create a loop. The home page cannot be deleted.

**URLs.** Every page has a permanent id. Its URL contains only that id, so renames and moves do not break links.

**Drafts.** Every user has at most one draft per page. Autosave overwrites it. Drafts are private to the user. A draft remembers the version it was based on.

**Publishing.** Publishing creates a new immutable version with the title, document, author, time and an optional summary. The client sends the version it was based on. If someone else published in between, the server refuses with a conflict, and the user chooses to compare, overwrite or keep editing. Renaming a page creates a version too.

**Versions.** Everyone who can view a page can list its versions, open any of them, and compare any two. Restoring version N publishes a new version with N's content and records "restored from N".

**Copy.** Copying a page creates an independent page with the same title (prefixed "Copy of") and content, attachments and direct restrictions, optionally with its visible children.

**Delete.** Deleting a page sends it and all pages below it to the trash. It is refused when the user cannot view or delete every page in that subtree, so nobody deletes content they cannot see.

**Personal lists.** Users can mark favourites and see their recently viewed pages. These lists only ever show what the user may still see. This is a per-user convenience list, not page-view statistics.

## Document format (ADR-004)
A JSON tree with a `schema_version`, based on the ProseMirror model. The server accepts only an allow-list of nodes, marks and attributes, and drops any attribute it does not know. Links must be http, https, mailto or a relative path. Images must point to an attachment: external images are refused, because they would leak who is reading. Documents are limited to 5 MiB and 100 levels of nesting.

Allowed nodes: paragraph, heading (1 to 6), bullet list, ordered list, list item, task list, task item, blockquote, code block (with language), horizontal rule, hard break, image, file (spec 007), table, table row, table cell, table header (with colspan and rowspan), text. Marks: bold, italic, underline, strike, code, link.

## Permissions and audit
Who may do what is defined in spec 002. Audit and outbox events, each in the same transaction as the change:
`space.created|updated|archived|unarchived|trashed|restored|purged`, `page.created|published|renamed|moved|copied|trashed|restored|purged|version_restored`, `page.exported`, `page.imported`. Saving a draft is not audited (private work in progress, and autosave would flood the log). Discarding one is not audited either.

## Acceptance criteria
- [x] A space key is unique ignoring case; invalid keys are refused
- [x] The creator of a space is space admin and a published home page exists
- [x] An unpublished page is invisible to everyone but its creator, in lists, tree and direct access
- [x] Publishing creates version 1, then 2, and versions cannot be changed afterwards
- [x] Publishing against an old base version returns a conflict; overwrite is explicit
- [x] Restoring version 1 creates a new version with its content
- [x] Comparing two versions shows added, removed and changed blocks
- [x] Moving a page under its own descendant is refused; concurrent moves never create a loop
- [x] After a rename or move, the same URL still opens the page
- [x] A move that would drop inherited view restrictions needs the restrictions permission (see 002)
- [x] Unknown node types are refused; `javascript:` links and external images are refused; unknown attributes are dropped
- [x] Every state change above appears in the audit log and the outbox in the same transaction

## How the product owner tests it
Create a space, write a page with a table, a task list and a code block, publish, edit and publish again, compare, restore. Drag a page to another parent. Log in as a second user in another browser and confirm the unpublished page cannot be seen.

## Provenance
Own design from docs/blueprint/01, 03, 05. General knowledge of wikis as a user. No product source code consulted.
