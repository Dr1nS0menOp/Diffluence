# 009: Wave B, reading and getting around

Status: built, waiting for the product owner's acceptance session
Phase: wave B of `docs/blueprint/03-functional-requirements.md`. The product owner asked on 2026-11-05 to build the whole of blueprint 03, one commit per wave, and tests it when all waves are done.

## Problem

Wave A made writing good. Reading is still bare: a page is a title and text, the tree can only open and close, and the top bar knows nothing about where a person has been. A reader should see at a glance who wrote a page and how long it takes to read, jump inside a long page, find anything from the search box without leaving the screen, and reach everything from the keyboard.

## What the study found

**How readers expect to get around.** The wikis and documentation tools studied agree on the shape. A sidebar tree has a filter at the top that narrows by title. A row shows an actions menu on hover, with rename, duplicate, move and delete. Personal stars sit near the top, are private and can be reordered. Home shows recent work. Long pages have a contents list beside them that marks the section in view, and that can be hidden. `[` hides the sidebar, and `?` lists the shortcuts. Help pages only describe what a person sees, which is all we use.

**What accessibility demands.** These come from WCAG 2.2 and the WAI-ARIA Authoring Practices, and change the design:

| Source | Demand | What we do |
|---|---|---|
| Tree view pattern | Up and Down move, Right opens or goes to the first child, Left closes or goes to the parent, Home and End jump, typing a letter jumps to a match. | The tree follows these keys. |
| Tree view pattern | A `treeitem` is one widget: buttons inside it are not allowed by the pattern. | We keep plain links and buttons in a labelled `nav`, and add the keys on top. See D1. |
| Disclosure navigation example | Plain page links in a list are not a `menu`, and promising `menu` makes screen readers expect more. | Our `Menu` already has full menu keys, so it stays for lists of actions and of pages. |
| Combobox pattern | Focus stays in the input. `aria-activedescendant` points at the option, `aria-expanded` follows the popup, Escape closes, and options wrap. | Quick search is a combobox with a list popup. |
| Tabs pattern | Left and Right move. Automatic activation only when panels show without delay. | Recent has tabs with manual data already loaded per tab, using automatic activation only for loaded panels. |
| Dialog pattern | Long content: focus goes to a static element at the start. Focus returns to what opened it. | The shortcuts overview and the image viewer do this. |
| 2.1.4 Character key shortcuts | A shortcut made of a printable key needs a way to turn it off, or to remap it, or it may only work while a component has focus. Sequences such as `g h` count too. | A preference turns all of them off. |
| 1.4.13 Content on hover or focus | Hover cards can be dismissed with Escape, can be hovered, and stay until the pointer leaves. | The tree card does this, and a preference turns it off. |
| 2.5.7 Dragging | Everything dragged has another way. | Favourites have Move up and Move down, the sidebar edge takes arrow keys, the header image has a position field, shortcuts have buttons. |
| 2.2.1 Timing adjustable | A message with an action must not vanish while it is being used. | The Undo message stays while the pointer or focus is on it. |
| 2.4.11 Focus not obscured | A focused heading may not end up under the top bar. | `scroll-margin-block-start` on headings. |
| 4.1.3 Status messages | "Link copied" must reach screen readers without moving focus. | Messages use `role="status"`. |

**What the browser gives us.** An `IntersectionObserver` with a negative `rootMargin` makes a reading band near the top of the window, which is the usual way to mark the section in view. `beforeprint` and `afterprint` let us open every `details` for printing and put them back. CSS `light-dark()` holds both colours in one token, and only needs `color-scheme` to switch, so a person's choice of theme needs no second list of colours. It takes colours only, so shadows get a colour token of their own. A container query lets the contents list appear only when the room beside the column is there, whatever the sidebar's width. `Intl.RelativeTimeFormat` writes "3 hours ago" in every language with no dependency.

**Reading speed.** A review of 190 studies put adult silent reading of English non-fiction at 238 words per minute, with most people between 175 and 300. Blueprint 03 says 230, which rounds down a little for tables and code. We keep 230.

**Safety.** Quick search must never show more than the full search, so its parts all go through the same permission rules. Incoming links are listed through `perm_visible_pages`, so a hidden page cannot be discovered by linking to something. The count of pages under a delete only counts what the person can see, because the delete is refused anyway when they cannot see everything. A header image must be an image on that same page, or a page could show a file from a page its reader may not see.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | The tree stays a `nav` with links, with the keys of the tree pattern added | A `treeitem` may not hold buttons, and our rows have an add button and a menu. Roving focus keeps it one Tab stop plus the buttons of the focused row. |
| D2 | Quick search is one combobox. Groups are `role="group"` in the list | The pattern gives focus handling, and the groups keep the list readable. |
| D3 | Recent searches stay in the browser | They are private, and a server copy would be one more list of what people look for. |
| D4 | Page look (emoji, width, title alignment, header image, child list) is page data, not part of a version, and applies at once | It is how a page looks, not what it says. Restoring an old version must not change it. |
| D5 | Heading addresses come from the heading's text, on the client | No stored ids, so nothing changes in the document format. A renamed heading loses old links, and then the page simply opens at the top. |
| D6 | The contents list shows levels 1 to 3, from three headings up, and only when there is room | Below that it is noise. |
| D7 | Page information and the Details panel are one panel | Two ways to the same facts would drift apart. |
| D8 | Tokens hold both colours in `light-dark()` | One list, so a person's theme cannot get out of step. Values do not change, so this is not a change of look. |
| D9 | Shortcuts are on by default, and a preference turns them off | WCAG 2.1.4 allows this, and people expect them. |
| D10 | Presentation mode splits at the highest heading level in the page, at the top of the document | Headings inside panels are not slide titles. |
| D11 | Undo of a delete works for the person who deleted, for 60 seconds on the server, and the message shows for 10 | The margin covers a slow connection. Anything older belongs to the trash. |
| D12 | A filter on the tree works in the browser | The tree is loaded already, and it answers at once. |
| D13 | Sidebar width and the tree view are preferences, so they follow the person | SPC-024 asks for that. |
| D14 | Links to pages open in the same tab, through the router | Reading a wiki means following links. Outside addresses still open a new tab. |

## Scope

**In:** NAV-010, 011, 015 to 017, 019, 020, 022, 024, 040, 041, 043 to 045, 047 to 051, 053, 054, 061 to 064, 066 to 069, 074, 078, 081, 082. SPC-003, 017 to 019, 021, 024 to 026, 028 to 030, 038 to 042, 045, 069. PAG-078, 079, 085, 091, 094, 100, 103, 104, 106 to 111, 113 to 118, 121, 123 to 126. ORG-001 to 007.

**Out, and where it goes:**

| Item | Goes to |
|---|---|
| Feed tabs on the start screen (NAV-029 to 034), Watch, unread comments, bell | Wave C: they need watching, reactions and comments |
| Status and owner in the byline, statuses in the tree card | Wave D |
| Several rows at once, dragging to another space (SPC-043, 044) | Wave D, with bulk actions and moves between spaces |
| People directory, name cards, time zone and date format | Wave F, with people and profiles |
| Space categories, Personal filter | Wave F, with personal spaces |
| View count, most viewed pages | Wave H, with analytics |
| Loading a tree branch by branch (SPC-031) | Phase 3, measured under load first |
| Getting-started card and tour (NAV-036, 070) | Wave F |

## Feature plan

### B1. Preferences

A screen at `/me/preferences`, reached from the account menu. Values live on the server, so they follow the person.

| Preference | Values | Default |
|---|---|---|
| Theme | system, light, dark | system |
| Keyboard shortcuts | on, off | on |
| Contents beside the page | on, off | on |
| Cards on hover | on, off | on |
| Sidebar width | 14 to 32rem | 18rem |
| Tree view | tree, visited, updated, alphabet | tree |

- **Storage:** one row per person with a JSON value. Unknown keys are refused. Personal, so not audited.
- **Theme:** `data-theme` on the root element. The value is also kept in the browser, so the first paint is right before the person's data has arrived.
- **Edge cases:** a failed save keeps the screen as it was and says so.

### B2. The top bar

Order: logo, Home, Spaces, Recent, Favourites, Create, then search, help, account. On narrow screens the row wraps.

- **Spaces menu:** favourite spaces first, then recently visited spaces, then a separator and **All spaces**, which opens the directory. An archived space is never listed.
- **Recent menu:** the last eight pages visited with space and time. A separator, then **All recent**, to the Recent screen.
- **Favourites menu:** favourite pages, or a sentence that explains the star when there are none. Then **All favourites**.
- **Create:** the button keeps its one click for the most used action. A small arrow beside it opens Page, Space, as far as the person may.
- **Help menu:** Keyboard shortcuts, Documentation (the address an administrator set, or our own), About.
- **Account menu:** gains Preferences.
- **Loading:** menus load when they open, so the bar costs nothing on each page.

### B3. Quick search

The box in the top bar becomes a combobox.

- **Opening:** focus or `/` opens the panel. With an empty box it shows recent pages and recent searches.
- **Typing:** after a pause of 200 ms the groups Pages, Spaces, Files and People appear, up to five each. An answer that arrives after a newer one is thrown away. A row has a title, the space, and the date of the last change.
- **Keys:** Down and Up move and wrap. Enter opens the row, or goes to the full results when none is marked. Escape closes, and a second Escape clears the box. Tab closes and moves on.
- **Last row:** always **All results for "…"**.
- **Inside a space:** a chip with the space name limits the search. Backspace in an empty box removes it. The chip comes back on the next page in a space.
- **Recent searches:** the last eight, stored in the browser when a search is carried out. A row clears them.
- **People:** only for those with *view profiles*. Others do not get the group, and the server does not send it.
- **Accessibility:** `role="combobox"`, `aria-expanded`, `aria-controls`, `aria-activedescendant`, `aria-autocomplete="list"`. The result count is announced.
- **Edge cases:** a failed search shows one line in the panel and keeps the text. A click outside closes it.

### B4. The page tree

- **Filter:** a field above the tree. Typing keeps matching rows and their parents, opens those branches, and marks the matching letters. Nothing found says so. Escape clears it. Dragging is off while filtering.
- **Views:** Tree, Last visited by me, Last updated, A to Z. The last three are flat lists without dragging, and say that the tree is where pages are arranged.
- **Row:** the page's emoji replaces the page icon. A lock shows on a page with its own restriction. A dot shows on a page where the person has a draft. Unpublished pages of the person are muted, with a word for screen readers.
- **Row menu:** Open in new tab, Copy link, Rename, Move, Copy, Favourite, Delete, each only for those who may. The menu leaves the sidebar's scroll area, so it is never cut off.
- **Card:** after 600 ms of hovering or focusing a row, a small card shows who changed the page and when. Escape closes it. It can be turned off.
- **Buttons:** Collapse all, and Show current page, beside the Pages heading.
- **Keys:** the tree is one Tab stop. Up, Down, Home and End move between rows. Right opens a branch or goes to its first child, Left closes it or goes to the parent. Enter follows the link, as links do. Typing jumps to the next row that starts with those letters. Tab goes to the row's own buttons, then out of the tree.
- **Long drags:** holding a row over a closed branch opens it after 700 ms. Moving near the top or bottom edge of the sidebar scrolls it.
- **Edge cases:** the current row takes the Tab stop when the tree is first reached. A filter that hides the focused row moves the stop to the first row.

### B5. The sidebar

- **Width:** a separator on the far edge can be dragged. Left and Right change it from the keyboard, with `aria-valuenow`. The width is a preference.
- **Above the tree:** Overview, then the space's shortcuts.
- **Shortcuts:** links to pages, other spaces or outside addresses, each with a label. Space administrators edit them in Space settings, with Move up and Move down, and drag. At most 20. An outside address opens in a new tab and shows an arrow. Only `http`, `https` and addresses inside the instance are allowed.
- **More by the space name:** Favourite, Copy link, Export, Space settings, as far as the person may.

### B6. The space overview

`/spaces/:key/overview`: name, description, administrators, number of pages, and the ten latest changes. Favourite is a star here too. An archived space shows its banner. The count covers only what the person can see.

### B7. The spaces directory

`/spaces`: a table of all spaces the person can see.

- **Filter:** text on name and key, and the kinds All, Favourites, Archived. The filter and the sort are part of the address.
- **Sort:** name, last change, number of pages.
- **Row:** badge, name, key, description, administrators, last change, a star.
- **Create:** a button for those with *create spaces*.
- **Paging:** 50 at a time, with Show more and a total.
- **Archived spaces** only show under Archived, and are absent from the Spaces menu, the start screen and quick search.

### B8. Personal lists

- **Recent screen:** `/recent`, tabs Visited, Worked on, Drafts, and a filter on titles that says it only looks at titles.
- **Worked on:** pages the person created or published, newest first.
- **Drafts:** pages they have not published and published pages they have a draft of, with the time and a Delete. Deleting asks first. An unpublished page's Delete sends it to the trash through the normal delete.
- **Favourites screen:** `/favourites`, pages and spaces, a filter, checkboxes with Remove selected, and reordering by drag, or Move up and Move down.
- **Star on spaces:** on the card, in the directory, on the overview and in More.
- **Edge cases:** a page that became restricted disappears from every list, because they all go through the same rule.

### B9. The byline and page header

- **Byline:** picture, author, last update, version, read time, restricted mark. Read time is 230 words a minute, rounded up, and left out below 100 words.
- **Contributors:** selecting the author opens the Details panel at Contributors.
- **Emoji:** beside the title, in the tree, breadcrumbs, lists, search results and the tab of the browser. Chosen in the editor with the emoji list of wave A. One emoji, or none.
- **Header image:** Add header image in the editor picks an image, which becomes an attachment of the page. Drag up or down, or use a position field from 0 to 100, to place it. Change and Remove are beside it. Its height is limited, and it is left out of print when it would take a page.
- **Title alignment:** start or centre, in the same place.
- **Full width:** a toggle in the same place. The column grows to the wide limit.
- **Show child pages:** a toggle in the same place, on by default.
- **Where:** a **Page look** button in the editor's top strip opens a dialog. Changes apply at once (D4).
- **Edge cases:** a removed attachment removes the header. A person who may not edit sees no button.

### B10. Inside the page

- **Heading links:** on hover or focus a link icon follows each heading. It copies an address with `#slug`. Two headings with the same text get `-2`, `-3`. A non-Latin heading keeps its letters.
- **Jumping:** opening such an address scrolls to the heading and marks it for two seconds. A slug that does not exist does nothing.
- **Contents:** beside the column, from three headings up, when the room is there. The current section is marked and the list follows the scroll. A link moves focus to the heading. It can be turned off in More and in the preferences.
- **Links between pages:** stay in the tab, through the router. A link to a page the person may not see gives the usual "not found".
- **Wide things:** code and tables scroll in their own frame, and the frame can be reached by keyboard.
- **Images:** selecting an image opens it on top of the page, with the caption, arrows and Escape. Left and Right move to the next image. One image has no arrows.
- **Child pages:** a list at the bottom, unless the page turned it off, in tree order. Only pages the person may see.
- **Long breadcrumbs:** past four steps the middle folds into a menu. Space and parent stay.
- **Details panel:** a button at the far edge of the page actions, and Page information in More. It holds Information (creator, created, last change, words, read time, address), Contributors with their number of versions, Incoming links, Outgoing links and Files. Escape closes it. It is a `complementary` region, and focus goes to its title and back to the button.
- **Reader mode:** `r` or More hides the sidebar and the top bar. A small button brings them back, and so do `r` and Escape.
- **Presentation:** More, Present. Full screen, one section per heading, Left, Right, Space, Home, End, Escape. The position shows as 3 of 12. A page without headings is one slide. It uses the browser's full screen when allowed, and works without it.
- **Print:** bars, sidebars, buttons, contents and panels disappear. Every collapsible section opens. Colours stay readable on paper. The address shows at the top.
- **Title of the tab:** `Page title · Instance name`.
- **Scroll position:** a reload returns to the same spot once the page is there. A new page starts at the top.

### B11. Deleting

- **The dialog** names the number of pages when there are pages below, and lists up to ten pages that link to it, with a count of the rest. Both only count what the person can see.
- **Undo:** after a delete, a message with **Undo** stays for 10 seconds, and longer while it has the pointer or focus. Undo brings the page back to its old place, and opens it.
- **Who:** the person who deleted, without *administer*, for 60 seconds. It is audited as a restore.
- **Edge cases:** Undo after the parent was deleted too says what happened.

### B12. Versions

- **Old version:** the version dialog shows a banner with the number, author, date, and a link to the current version.
- **Compare:** runs of unchanged blocks fold down to a line with a Show button. The two blocks around a change stay.

### B13. Times and small things

- **Relative time:** lists show "3 hours ago", in the person's language. The exact time is in `title` and in a `time` element. Older than 30 days shows the date.
- **Key suggestion:** a new space suggests its key from the name while the key is untouched. A taken key shows at once.
- **Motion:** smooth scrolling and marks stop with `prefers-reduced-motion`.

### B14. Keyboard shortcuts

Single keys, only when focus is not in a text field or the editor, and with no Ctrl, Alt or Meta held.

| Keys | Action |
|---|---|
| `/` | Search |
| `?` | Shortcuts overview |
| `g` `h` | Home |
| `g` `s` | Spaces directory |
| `g` `r` | Recent |
| `g` `f` | Favourites |
| `c` | Create |
| `e` | Edit this page |
| `s` | Favourite this page |
| `[` | Hide or show the sidebar |
| `r` | Reader mode |
| `i` | Details panel |

The overview lists them by group, has a search field, and tells where to turn them off. A second key of a pair must follow within a second.

## API

All new routes, with their access rule.

| Route | Rule | What |
|---|---|---|
| `GET, PUT /me/preferences` | authenticated | Own preferences |
| `GET /me/recent?kind=visited,worked,drafts` | authenticated | Extended |
| `GET /me/favourites`, `PUT /me/favourites/order` | authenticated | Extended, order |
| `GET /me/spaces` | authenticated | Favourite and recent spaces |
| `GET /space-directory` | list | Directory, filter, sort, paging |
| `PUT, DELETE /spaces/{id}/favourite` | `space.view` | Star |
| `GET /spaces/{id}/overview` | `space.view` | Overview |
| `GET /spaces/{id}/shortcuts` | `space.view` | Shortcuts |
| `PUT /spaces/{id}/shortcuts` | `space.administer` | Replace them |
| `GET /space-key-check?key=` | permission `create_space` | Is a key free |
| `PATCH /pages/{id}/look` | `page.edit` | Page look |
| `GET /pages/{id}/details` | `page.view` | Details panel and child pages |
| `GET /pages/{id}/impact` | `page.delete` | What a delete touches |
| `POST /pages/{id}/undo-delete` | authenticated | Own delete, 60 seconds |
| `GET /search/quick` | list | Groups for quick search |

## Data

Migration 0006, expand only:

- `pages`: `emoji`, `full_width`, `title_align`, `hide_children`, `header_attachment_id`, `header_position`.
- `favourites`: `position`.
- `space_favourites`, `user_preferences`, `space_shortcuts`.
- `page_links`: one row per link from a page's current version to another page, kept up to date in the transaction that publishes. The migration fills it from the documents that exist. The target has no foreign key, so a link to a removed page stays visible for wave D's clearing up.

## Build order

1. Go: migration, preferences, favourites, directory, overview, shortcuts, look, details, impact, undo, links, quick search, with tests first.
2. Tokens to `light-dark()`, and the design check with it.
3. Top bar, menus, quick search, shortcuts.
4. Tree, sidebar.
5. Page: byline, contents, heading links, Details, images, reader, presentation, print.
6. Directory, Recent, Favourites, Overview, Preferences, pictures.
7. Docs, statuses in `03-functional`, handoff.

## Test plan

- **Go integration:** every route, permissions and hidden pages in each list, the link table through publish, restore and purge, undo inside and outside the window and by someone else, look validation, a header image from another page.
- **Route walk:** the new object routes join `TestEveryObjectRouteIsProtected` by themselves.
- **Mutation:** a hidden page in incoming links, a header image from another page, undo by another person, a preference that is not on the list.
- **Vitest:** slugs, read time, relative time, tree filter, tree keys, sections for presentation, folding of comparisons.
- **Browser:** each section above, with the keyboard first.
- **Pictures:** Directory, Recent, Favourites, Overview, Preferences, a page with contents and details, the quick search panel, in light, dark and narrow where it matters. Short content as well as long.
- **Accessibility:** axe on each new screen, and with the panels open.

## Acceptance criteria

- [x] AC-1 A person's theme choice beats the system, follows them to another browser, and is right on the first paint.
- [x] AC-2 The Spaces, Recent and Favourites menus show the right pages, and nothing a person may not see.
- [x] AC-3 Quick search works by keyboard alone, keeps the newest answer, shows recent pages when empty, and limits to a space with a chip that Backspace removes.
- [x] AC-4 A filter narrows the tree with parents and marks the letters, and the views list the pages flat.
- [x] AC-5 The tree works with arrows, Home, End and letters, and is one Tab stop.
- [x] AC-6 The row menu works, and only offers what the person may do.
- [x] AC-7 The sidebar width can be changed by mouse and by keyboard, and comes back.
- [x] AC-8 Shortcuts in the sidebar can be added, ordered and removed by an administrator, and outside addresses open safely.
- [x] AC-9 The overview and the directory show the right numbers, and hidden pages do not count.
- [x] AC-10 Archived spaces only show under Archived.
- [x] AC-11 Recent has Visited, Worked on and Drafts, and a draft can be deleted from there.
- [x] AC-12 Favourites hold pages and spaces, and can be reordered without dragging.
- [x] AC-13 Read time, contributors, emoji, full width, title alignment and the header image work and survive a reload.
- [x] AC-14 A heading link copies an address that jumps there and marks it.
- [x] AC-15 The contents list follows the scroll, and can be turned off.
- [x] AC-16 The Details panel lists incoming and outgoing links, without pages the person may not see.
- [x] AC-17 Images open large, with arrows and Escape.
- [x] AC-18 Reader mode and presentation work by keyboard alone.
- [x] AC-19 Printing shows only the page, with every section open.
- [x] AC-20 Deleting names what it touches, and Undo brings the page back for the person who deleted it, and only them.
- [x] AC-21 Old versions have a banner, and comparisons fold what did not change.
- [x] AC-22 All shortcuts work, can be listed, and can be turned off. They never fire while typing.
- [x] AC-23 Everything passes axe, and the new pictures have been looked at.

## Risks

| Risk | Answer |
|---|---|
| The tree's keys collide with drag and with links | Keys only act on the row links, and browser tests cover both |
| `light-dark()` is new | All browsers we support have it since 2024. The picture tests prove nothing changed |
| A `position: fixed` menu in the sidebar drifts while scrolling | It closes on scroll |
| Slugs differ between screen and link after an edit | The page opens at the top, which is the honest answer |
| Migration backfill on a large instance | It is one pass over current versions, and runs before the new release starts |

## How the product owner tests it

1. Open the account menu, Preferences, and pick Dark. Reload. Open another browser and log in.
2. Press `/`, type a few letters, use the arrows and Enter.
3. In a space, type in the tree's filter. Try the views. Tab into the tree and use the arrows.
4. Open a row's `…` menu and rename a page.
5. Drag the edge of the sidebar. Reload.
6. Open a long page with headings. Watch the list beside it. Hover over a heading and use its link.
7. Press `i`, then `r`, then pick Present in More.
8. Print a page with collapsible sections.
9. Delete a page, and press Undo.
10. Press `?`.

## Provenance

Own text from these public sources, read for behaviour only, plus the study behind blueprint 03 (`docs/blueprint/03-functional/sources.md`):

- W3C WAI-ARIA Authoring Practices: tree view, combobox with list autocomplete, disclosure navigation, tabs, modal dialog.
- W3C WCAG 2.2 Understanding 2.1.4 Character Key Shortcuts.
- MDN: Intersection Observer API, printing.
- Brysbaert, M. (2019), *How many words do we read per minute? A review and meta-analysis of reading rate*, Journal of Memory and Language.
- Public help pages: TYPO3 editor tutorial (page tree filter), GitBook documentation (page actions, contents), BookStack documentation (favourites), Outline guide (starred documents), the space sidebar help of a commercial wiki (the `[` key).
