# 03.2: Spaces

A space is the unit of ownership: it holds a tree of pages, a blog, files and its own permissions. Every page lives in exactly one space.

## 2.1 Creating a space

- **SPC-001** · built · A person with *create spaces* creates a space with a name and a key. Description and icon are optional.
- **SPC-002** · built · A key has 2 to 32 letters, digits, dashes or underscores, and is unique ignoring case.
- **SPC-003** · built · The key is suggested from the name while typing, and can be changed before saving. The form says at once when a key is taken.
- **SPC-004** · built · The creator becomes space administrator, and a home page is created.
- **SPC-005** · built · A space can start from a permission set: *Team space* (all members can edit) or *Read-only knowledge base* (all members can view).
- **SPC-006** · built · A space can start from a space template, which brings a home page, a set of pages, templates and labels (ORG-055).
- **SPC-007** · built · When a space is created, the creating form shows who will have access, so nobody creates an open space by accident.
- **SPC-008** · built · An administrator can set the default permission set of new spaces.

## 2.2 Details of a space

- **SPC-009** · built · A space administrator edits the name, description and icon.
- **SPC-010** · built · An icon is an animal or one character. Without a choice, the key of the space picks an animal.
- **SPC-011** · built · An icon can be an uploaded image, cropped to a square.
- **SPC-012** · built · The key of a space can be changed by a space administrator. Old addresses that contain the key keep working through a redirect.
- **SPC-013** · built · A space has a list of owners shown to everyone, so people know who to ask.
- **SPC-014** · built · Categories can be set on a space (NAV-052).
- **SPC-015** · built · A space administrator picks which page is the home page.
- **SPC-016** · built · The home page cannot be deleted.

## 2.3 The overview of a space

- **SPC-017** · built · Opening a space shows its home page.
- **SPC-018** · built · The overview shows the space name, description, owners and the number of pages.
- **SPC-019** · built · It shows recent changes in the space and the most viewed pages, each of which can be turned off. Still open: most viewed pages (wave H), and turning parts off.
- **SPC-020** · built · It shows the space's newest blog posts when the space has a blog.
- **SPC-021** · built · A person can favourite and watch the space from here.

## 2.4 The sidebar of a space

- **SPC-022** · built · The sidebar shows the space badge and name, a button to hide it, and the page tree.
- **SPC-023** · built · The sidebar can be hidden and comes back at the same state after a reload.
- **SPC-024** · built · The sidebar can be made wider or narrower by dragging its edge. The width is remembered per person.
- **SPC-025** · partly · A section **Shortcuts** holds links chosen by space administrators: to pages, other spaces or outside addresses, each with its own label and an order that can be changed by dragging. Still open: dragging: Move up and Move down do the job.
- **SPC-026** · partly · Fixed entries sit above the tree: **Overview**, **Blog** when turned on, **Calendar** later, and **Files** (FIL-043). Still open: Calendar.
- **SPC-027** · built · Space administrators see links to Space settings and Trash at the bottom.
- **SPC-028** · built · A **More** button by the space name opens: Watch this space, Copy link, Export, Space settings.
- **SPC-029** · built · The sidebar has a filter on page titles. Typing narrows the tree to matching rows with their parents, and matching words are marked.
- **SPC-030** · built · A view switch offers **Tree**, **Last visited by me**, **Last updated** and **A to Z**. The chosen view stays. Only the tree can be rearranged, and the other views say so.
- **SPC-031** · new · A long tree loads branches as they open, so a space with 50,000 pages still opens at once.
- **SPC-082** · built · A fixed **Home** row sits above Overview and the tree. It is marked while the home page is open.
- **SPC-083** · built · The home page is not a row of the tree, the flat views or the filter. Its children are the top rows, followed by pages without a parent.
- **SPC-084** · built · On the home page the breadcrumbs end in **Home**.
- **SPC-085** · built · A strip of spaces at the top of the sidebar moves a person between spaces: favourites, then recent spaces, then the rest, as in the Spaces menu. It scrolls on its own, by wheel and by keyboard, and has a link to all spaces.
- **SPC-086** · built · The strip marks the current space and keeps it in view, also when the space is archived.
- **SPC-087** · built · A space administrator picks one of 30 cartoon animals in Space settings, and a person picks one when making a space.

## 2.5 The page tree

- **SPC-032** · built · Pages form a tree of any depth with a manual order.
- **SPC-033** · built · A branch has a chevron and can be collapsed. Guide lines show the structure.
- **SPC-034** · built · The first two levels start open. What a reader collapses stays collapsed, per space, across pages and reloads.
- **SPC-035** · built · Going to a page reveals it, also inside a collapsed branch.
- **SPC-036** · built · The current page has a tint and shows the add button. Other rows show it on hover or focus.
- **SPC-037** · built · A person who may move pages can drag a row onto another page to make it a child, or between rows to reorder it.
- **SPC-038** · built · A **More** button on each row offers: Open in new tab, Copy link, Rename, Move, Copy, Favourite, Watch, Archive, Delete. Each is shown only to those who may.
- **SPC-039** · built · Hovering over or focusing a row shows a small card with the owner, the last change and the status. It can be turned off in the preferences.
- **SPC-040** · built · The row shows the page's emoji (PAG-123), a lock when it has a restriction, and a draft mark when the person has an unpublished change.
- **SPC-041** · built · The current person's unpublished pages appear in the tree in a muted style, only for them.
- **SPC-042** · built · A button collapses all branches at once, and another reveals the current page.
- **SPC-043** · built · Several rows can be selected with Shift or Ctrl and moved together.
- **SPC-044** · built · A row can be dragged to another space in the Spaces menu, with the same rules as a move (PAG-042).
- **SPC-045** · built · A long drag scrolls the tree and opens a collapsed branch after a short wait.
- **SPC-046** · built · A page cannot be moved below itself, and two people moving pages at once cannot create a loop.

## 2.6 Features of a space

- **SPC-047** · built · A space administrator turns features on or off per space: Blog, Comments, Reactions, Page statuses, Analytics, Automation, Calendar and later content types.
- **SPC-048** · built · Turning a feature off hides it. It never deletes content. Turning it on brings everything back.
- **SPC-049** · built · An administrator can set which features are on by default, and can turn a feature off for the whole instance.

## 2.7 Permissions of a space

- **SPC-050** · built · A space has a matrix of permissions per person or group, and for "all members" and "all guests".
- **SPC-051** · built · The permissions are: view, export, administer; create, edit, archive, delete own and delete any page; add, delete own and delete any comment; add, delete own and delete any attachment; manage restrictions.
- **SPC-052** · built · Viewer, Editor and Space administrator are named sets that fill in the boxes.
- **SPC-053** · built · A space always keeps at least one direct person or group with *administer*.
- **SPC-054** · built · A fourth set, **Manager**, can manage people and content but not the settings of the space.
- **SPC-055** · built · Administrators can define their own named sets for the whole instance.
- **SPC-056** · built · The screen can be searched by name, and shows how each person gets their access: directly, or through which group.
- **SPC-057** · built · A change shows a summary of who gains and who loses what before it is saved.
- **SPC-058** · built · A person can see who has access to a space, without seeing the matrix, when they are allowed to view it.
- **SPC-059** · built · A space administrator can export the list of who has access as CSV.

## 2.8 Clearing up a space

Space administrators need a way to keep a growing space clean.

- **SPC-060** · partly · A **Content** screen in space settings lists all published pages in a table with title, owner, status, labels, created, last changed and last viewed. Still open: last viewed (wave H).
- **SPC-061** · partly · It filters by owner (including disabled people), status, label, contributor, last changed, created, and "not active for 1, 3, 6 or 12 months". Pages with children are left out of the "not active" filter by default, so a parent is not removed by mistake. Built: filter on owner, status, label, contributor and inactivity. Still open: last changed and created.
- **SPC-062** · built · Rows are selected with checkboxes. Selecting a parent selects its children, and they can be deselected one by one.
- **SPC-063** · built · Actions on a selection: archive, delete, change owner, change status, add or remove a label, move.
- **SPC-064** · built · Each action shows what it will do, including how many pages, before it runs. Bulk actions run in the background and report what succeeded and what did not.
- **SPC-065** · built · A filter lists pages with a restriction, so space administrators can review them.
- **SPC-066** · built · A filter lists pages with links to pages that no longer exist.
- **SPC-067** · built · A filter lists pages with no incoming links and that are far from the top of the tree, as a hint that they may be lost.

## 2.9 Archiving and deleting a space

- **SPC-068** · built · A space administrator archives a space. It becomes read-only for everyone and can be restored.
- **SPC-069** · built · An archived space disappears from the Spaces menu, the directory and quick search. Direct addresses still work, with a banner that says the space is archived.
- **SPC-070** · built · Search can include archived spaces through a filter.
- **SPC-071** · built · A space administrator deletes a space. It goes to the trash for the universal retention period.
- **SPC-072** · built · Administrators restore or permanently remove spaces from the trash in the admin console.
- **SPC-073** · built · Deleting a space asks the person to type the key, and offers archiving and an export first.
- **SPC-074** · built · A key stays taken while its space is in the trash.
- **SPC-075** · new · The progress of a permanent removal is shown, because a large space takes time.

## 2.10 Personal spaces

- **SPC-076** · built · A person with *create personal space* has one personal space, created on request from the account menu.
- **SPC-077** · built · Its key comes from the person and cannot clash with team spaces. It is listed under Personal in the directory.
- **SPC-078** · built · The owner is its administrator, and it starts private. They choose whether others can see it.
- **SPC-079** · partly · A personal space is a safe place to prepare a page and later move it to a team space. Still open: moving a page out, with move between spaces (wave D).
- **SPC-080** · built · When a person is disabled, their personal space stays. An administrator can hand it over or archive it.
- **SPC-081** · built · Space settings are the same as for team spaces, except that the owner can never be removed by a group change.
