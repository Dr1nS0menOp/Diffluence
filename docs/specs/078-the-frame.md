# 078: The frame

Status: built
Phase: the screens of blueprint 03, first wave (SC1), after the study of 2026-11-14 (ADR-045)

## Problem

The study of 2026-11-14 rewrote how the frame of every screen is arranged: the top bar, the part of the sidebar that is the same on every screen, the panels it opens, the block of a space in the sidebar and the start screen. Those lines describe built screens differently from how they are built, and some reverse earlier decisions of the product owner (ADR-027, spec 016, spec 033). On 2026-11-15 the product owner confirmed that the frame is to be rebuilt as the lines say, including the parts that reverse those decisions. This spec is the first of the order in `docs/handoff/2026-11-15-screens-study.md`: chapter 1, sections 1.2, 1.3 and 1.10 to 1.15, and chapter 2, section 2.4.

The look stays ours: colours, icons, fonts and every value come from `tokens.css` (ADR-021, 02 "One exception"). Only the arrangement follows the lines.

## Study

What is built today, and where each line touches it:

| Part | Built | Lines |
|---|---|---|
| Top bar (`app.tsx`, `components/topbar.tsx`) | Logo and name, Home, Spaces, People, Recent, Favourites and Create at the start; search in the middle; bell, help and the account menu at the end. Spaces, Recent and Favourites are menus. Administration sits in the account menu | NAV-090 to NAV-098 |
| Hiding the sidebar (`components/frame.tsx`) | A button in the sidebar's head; while hidden, a button at the start of the main area, or a rail in the editor. The personal screens cannot hide their sidebar | NAV-092, SPC-022 |
| Personal sidebar (`components/home-frame.tsx`) | Two folding parts, **For you** (Home, Recent, Favourites, Tasks, Watching) and **Spaces** (up to twelve spaces in the order of the strip, and All spaces) | NAV-125 to NAV-128, NAV-137 to NAV-142 |
| Sidebar of a space (`components/frame.tsx`, `space-strip.tsx`, `sidebar-part.tsx`, `tree.tsx`) | The strip of spaces; the space's name with More and the hide button; the folding part **This space** with a height handle (Home, Overview, People, Blog, Files, Archive, release, shortcuts); a quiet row of tree tools (filter, collapse, show current, new page, link, select); the tree; links to settings, content, access and trash at the foot | SPC-097 to SPC-108, SPC-160, NAV-089 |
| Start screen (`views/home.tsx`) | A greeting; the grid of every space; the feed under tabs beside a side column of recent pages, favourites and open tasks | NAV-102 to NAV-109 |
| Current row (`layout.css`) | Accent tint and bold text, square corners | NAV-144, NAV-145 |

The preferences of a person are a JSON document checked by `identity.Preferences.validate`, which refuses unknown fields. A choice of which entries of the sidebar show needs two fields there; no migration.

## Lines in scope

Every line of the named sections, with its status after this wave. "Not this wave" names a line of those sections that does not belong to the frame, or that a later wave of the order owns.

### 1.1 (the lines of the frame)

| Line | Was | Now | What changed |
|---|---|---|---|
| NAV-089 | new | built | A thin line under the top bar, one beside the sidebar, one between the global part and the space's block. The line above the links to other tools waits for that feature (NAV-130, later) |

### 1.2 The top bar

| Line | Was | Now | What changed |
|---|---|---|---|
| NAV-013 | built | built | Logo and name now come after the sidebar button |
| NAV-014 to NAV-017 | built/partly | built/partly | Their behaviour moves to the global part of the sidebar (NAV-091), as panels instead of menus |
| NAV-018, NAV-019 | built | built | Create stays, now directly after search |
| NAV-020 | built | replaced by NAV-093 | Search no longer on the far side |
| NAV-021, NAV-022 | built | built | Bell and help become round icon buttons (NAV-094) |
| NAV-023 | built | built, without Administration | Administration leaves the account menu (NAV-095) |
| NAV-024, NAV-025 | built | built | Unchanged in function |
| NAV-090 | new | built | Three groups: sidebar button, logo and name; search with Create; bell, help, settings, account. The button for other tools waits for NAV-099 (later) |
| NAV-091 | new | built | Home, Spaces, Recent, Favourites and People leave the bar |
| NAV-092 | new | built | The sidebar button is the first item of the bar, on every frame that has a sidebar that hides |
| NAV-093 | new | built | Search and Create are one group in the middle |
| NAV-094 | new | built | Round icon buttons with name and tooltip, a neutral square on hover and focus, a framed look while open |
| NAV-095 | new | built | The settings button opens the administration, only for administrators |
| NAV-096 | new | built | The account menu: identity block, Profile, Your account, Theme, a line, a group headed with the instance name (personal space, Tasks, Preferences), a line, Log out; the first entry has the focus |
| NAV-097 | new | built | Theme opens a sub-menu of light, dark and system with a tick; the choice is the preference of NAV-074 |
| NAV-098 | new | built | The look of the account menu |
| NAV-099 to NAV-101 | later | later | Links to other tools: see D10 |

### 1.3 The start screen

| Line | Was | Now | What changed |
|---|---|---|---|
| NAV-026 | built | replaced by NAV-103 | No greeting |
| NAV-027 | built | replaced by NAV-103 | No grid of spaces |
| NAV-028 | built | built | Now the cards of NAV-104 |
| NAV-029 | built | built | Now the pill buttons of NAV-106 |
| NAV-030 to NAV-034, NAV-036 to NAV-038 | built | built | Unchanged in function |
| NAV-035 | built | replaced by NAV-103 | No side column |
| NAV-102 | new | built | One centred column: getting started, the cards, the welcome message, the feed with its pills. The settings corner waits for NAV-110 (later) |
| NAV-103 | new | built | Greeting, grid and side column leave |
| NAV-104 | new | built | Up to six cards, three to a row, newest first, only what the person may see; a draft only to its writer (the server's lists already do this) |
| NAV-105 | new | built | Kind icon in an outlined square, bold title cut short, a Draft mark, the space, and what the person last did with a relative time |
| NAV-106 | new | built | Pills with icon and words, the chosen one pressed |
| NAV-107 | new | partly | The card: the person's initials and name, what they did, when; the kind icon and title; the space; two lines of the page as plain words; the count of comments, and of reactions when there are any. Still open: the page owner (the feed does not carry it), the reaction button and the recent-actions control (needs its own study) |
| NAV-108 | new | new | A comment under the card needs the feed to carry it: not this wave |
| NAV-109 | new | partly | The border, the line under the header, the title as the largest text. Still open: the owner, and the visited colour (NAV-159) |
| NAV-110, NAV-111 | later | later | |

### 1.10 The global part of the sidebar

| Line | Was | Now | What changed |
|---|---|---|---|
| NAV-125 | new | partly | Start screen, Recent, Favourites, Spaces, Drafts, Tasks, People (D1), Watching, then Customise. Still open: Apps (no apps exist) and Templates (no screen of the templates a person may use exists yet, D2) |
| NAV-126 | new | built | One row per entry; Recent, Favourites and Spaces carry an arrow and open a panel; Drafts opens a screen of its own (D3) |
| NAV-127 | new | built | A line and the space's block inside a space; the favourite spaces and All spaces on the personal screens |
| NAV-128 | new | built | The whole sidebar scrolls as one |
| NAV-129, NAV-130 | later | later | |

### 1.11 Panels beside the sidebar

| Line | Was | Now | What changed |
|---|---|---|---|
| NAV-131 | new | built | A card beside the sidebar: title and Close, a filter field with the focus, groups, actions at the foot under a line |
| NAV-132 | new | built | The entry is tinted while its panel is open; Escape and Close return the focus to it; choosing a row shuts it |
| NAV-133 | new | built | Spaces: the current space, favourites and recent spaces, with a star on each row. The count of recent spaces is what the server lists; the filter narrows only the rows shown (D8) |
| NAV-134 | new | built | All spaces, Create a space, Import (D8) |
| NAV-135 | new | built | The panel replaces the strip of spaces, which leaves the sidebar; a page dragged onto a space in the panel moves there, as the menu did (SPC-044) |
| NAV-136 | new | built | Recent: visited, worked on and drafts; Favourites: pages and spaces; each ends with its full screen (D7) |

### 1.12 Customising one's own sidebar

| Line | Was | Now | What changed |
|---|---|---|---|
| NAV-137 to NAV-141 | new | built | The dialog, the locked start entry, dragging and Move up and Move down, hidden entries under More. Stored as `sidebar_order` and `sidebar_hidden` in the preferences (D9) |
| NAV-142 | new | built | Watching starts hidden, under More |
| NAV-143 | later | later | |

### 1.13 States, menus, messages and marks

| Line | Now | What changed |
|---|---|---|
| NAV-144 | built | The current row of a sidebar, the tree and the settings sections: accent text in normal weight, the tint with rounded corners, a short bar on its start edge, `aria-current` |
| NAV-145 | built for the sidebar | The neutral hover tint of a row, the rounded focus outline. Lists and menus elsewhere keep theirs until their wave |
| NAV-146 | built for the Theme sub-menu | A tick at the start of the chosen option |
| NAV-147 | partly | The account menu has the rows, the headed group and the sub-menu arrow. Other menus keep their look until their wave |
| NAV-148 to NAV-159, NAV-178 | new, not this wave | Messages, notes, explanation buttons, lozenges, tiles, kind icons, tables, links and the New mark belong to the screens that show them: the page, settings and administration waves |

### 1.14 Forms and settings, 1.15 Empty and loading

NAV-160 to NAV-177 stay **new**: none is part of the frame. The settings waves (order item 4) build them.

### 2.4 The sidebar of a space

| Line | Was | Now | What changed |
|---|---|---|---|
| SPC-022 | built | built, without the hide button | The hide button moves to the top bar (NAV-092) |
| SPC-023, SPC-024 | built | built | Hidden state and width kept as before |
| SPC-025 | partly | partly | Shortcuts are a group of their own (SPC-100) |
| SPC-026 | partly | replaced by SPC-106 | No fixed entries |
| SPC-027 | built | replaced by SPC-106 | No links at the foot |
| SPC-028 | built | built | Now the menu of SPC-107 |
| SPC-029 to SPC-031 | built | built | The filter is always shown; the views move into a menu (SPC-101) |
| SPC-082 | built | replaced by SPC-098 | The header row leads to the home page |
| SPC-083, SPC-084 | built | built | Unchanged |
| SPC-085, SPC-086 | built | replaced by NAV-135 and SPC-106 | No strip of spaces |
| SPC-087 | built | built | Unchanged |
| SPC-097 | new | partly | Line, header row, Shortcuts, Content, Blogs, Space apps. Calendars wait for calendars (SPC-104, later) |
| SPC-098 | new | built | Badge, name, More always shown; the row leads to the home page and is marked current while it is open |
| SPC-099 | new | built | Group headings of an icon and a word, buttons in one column, rows indented, an empty group says so |
| SPC-100 | new | built | Add opens the Shortcuts form in a dialog, for those who may change shortcuts (D11) |
| SPC-101 | new | built | Content has add (D13) and order and view; the filter is always shown; the row of tool icons goes |
| SPC-102 | new | built | The control that brings the sidebar back to its start |
| SPC-103 | new | built | Blogs: heading to the list, add for `post.create`, the five newest posts (D12) |
| SPC-104, SPC-105 | later | later | |
| SPC-106 | new | built | Fixed entries, foot links, Home row and strip leave; Overview, People, Files, Archived pages and Space settings are in the space menu; trash and content through space settings |
| SPC-107 | new | built | The space menu in four blocks (D14) |
| SPC-108 | new | built | Icons, no arrows or key hints, a filled star on a favourite; People leads by role; Archive and Delete ask first |
| SPC-160 | new | partly | Space apps shows release to a published space while publishing is on. Still open: drawings and document control have no screen of a space yet, and choosing which entries show (needs storage per space) |

## Behaviour

1. **The top bar.** `header.top` holds three groups. At the start, on a screen whose frame has the sidebar (the Personal, Space and Focus frames), the sidebar button, named **Hide sidebar** or **Show sidebar** after what it does, then the logo and name. In the middle, search, then Create. At the end, the bell, help, settings (administrators only) and the account. The sidebar button is placed into the bar by the frame through a portal, so a screen without a hideable sidebar has none and nothing else needs to know which frame a route uses. `[` does the same as the button.
2. **One sidebar state.** Whether the sidebar is shown is one state for all frames, kept in the browser as before (`diffluence.sidebar`), starting hidden on a narrow screen. Hiding it on a personal screen hides it in a space too.
3. **The account menu.** As NAV-096. **Profile** opens the person's own profile (`/people/{id}`), **Your account** opens `/me`. Notification settings and Watching leave the menu: the first is a section of Your account, the second an entry of the sidebar. The Theme entry opens a sub-menu by Enter, Space, the arrow toward the end side or a click; Escape or the arrow toward the start side closes it and returns to Theme. Each choice is a `menuitemradio`, checked on the current one.
4. **The global part.** A `nav` named **Your places**, first in the sidebar: the start screen, then the person's own order of the other entries, then **More** while something is hidden, else **Customise sidebar**. More is a menu of the hidden entries that ends with Customise sidebar.
5. **Panels.** Recent, Favourites and Spaces are buttons with `aria-expanded` and `aria-controls`. A panel is a non-modal `role="dialog"` named after its entry; only one is open; the filter field takes the focus. Escape, Close or a click outside shuts it and gives the focus back to the entry (a click outside leaves the focus where it went). Choosing a row navigates and shuts it. Each list loads when its panel opens.
6. **Customise sidebar.** A `Dialog` with two explanations and one row per entry except the start screen, which is shown first, ticked, pale and locked. A row has a drag handle, a checkbox, the icon, the name and More (Move up, Move down). Save changes stores `sidebar_order` and `sidebar_hidden`; Cancel and Close change nothing.
7. **After the global part.** On the personal screens, after a gap, a heading **Favourite spaces**, the favourite spaces and **All spaces**. Inside a space, a line and the space's block.
8. **The space's block.** A header row (badge and name, leading to the home page, `aria-current` while it is open) with More; then **Shortcuts** when there are any or the person may change them; **Content** with add, order and view, the filter and the tree; **Blogs** while the blog is on; **Space apps** while one of its entries shows. Each group is a `section` named by its heading. Once the sidebar has scrolled, **Back to top** floats over its top; it scrolls back without motion when reduced motion is asked, and puts the focus on the header row.
9. **The space menu.** Favourite space or Remove from favourites; Watch space or Stop watching; the approvals digest (DOC-607, kept); Copy link; a line; under **Space tools**: Overview, People (the access screen for those who manage access, the people list for others), Files, Archived pages; a line; Export, Automation (while that part is on, for `space.automation`), Space settings; a line; Archive space (asks) or Restore space, and Delete space (asks for the key, danger style), for `space.administer`.
10. **The start screen.** One centred column: the getting-started card, **Pick up where you left off** with the cards, the welcome message, **Activity** with the pills, the space filter of Recent activity, and the feed. The cards merge what the person visited and worked on, newest first per page, labelled Edited or Visited; a page with only a draft goes to its editor.
11. **Drafts.** `/drafts` is a personal screen with the list of the Drafts tab of Recent.

## Permissions and audit

Nothing new is decided: every list comes from an existing route that already filters by what the person may see (NAV-006). The settings button shows only for `administrator`, as the account menu entry did; the server keeps refusing the administration to anyone else. The space menu shows each entry by the permission its screen needs. Preferences are private and not audited (as before).

## Data

`identity.Preferences` gains `sidebar_order` and `sidebar_hidden`, lists of entry ids (`recent`, `favourites`, `spaces`, `drafts`, `tasks`, `people`, `watching`). Each may be null (the default) and holds known ids only, each once. Null `sidebar_hidden` means the default, Watching hidden. No migration: preferences are one JSON document. A client of the previous release sends neither field and keeps the default; one of this release sending them to a node of the previous release is refused as unknown (400), which only stops saving the sidebar until every node is upgraded.

`sidebar_shut` and `sidebar_links_height` stay in the document and the check, because the previous release still sends them, but this release reads neither. They can go in a later contract step.

## Load budget

No route of the server changes, so `internal/app/query_budget_test.go` keeps its budgets. On the client, the frame adds one request a screen of a space did not make: the Blogs group asks for the five newest posts of the space (`GET /spaces/{id}/posts?limit=5`), an indexed read of the class reading, while the blog is on. The panels load their lists only when opened, as the menus did. The start screen drops the open tasks and the favourite pages; it reads the visited and worked-on lists (the side column read the visited list), and keeps the list of spaces for the first-space sentence (NAV-037) and the filter of Recent activity.

## Decided on the product owner's behalf on 2026-11-15, may be overruled

- **D1.** The people directory, which leaves the top bar and has no line of its own in the global part, becomes the entry **People** after Tasks, so NAV-055 keeps a way in.
- **D2.** Templates waits: no screen lists the templates a person may use across spaces, and building one is a new feature (chapter 8).
- **D3.** Drafts opens `/drafts`, a screen of its own with the list of NAV-045, as NAV-126 says.
- **D4.** The sidebar button exists only on a frame with a sidebar that hides; the Plain and Settings frames have none, because a control that does nothing is absent (NAV-004). Its name says what it does.
- **D5.** On a narrow screen the sidebar keeps opening above the content, as built; a panel then covers the width of the screen less the gutters.
- **D6.** A panel shuts on a click outside it, as a menu does.
- **D7.** The Recent panel has three groups, **Visited**, **Worked on** and **Drafts**, five rows each; the Favourites panel has **Pages** and **Spaces**. The filter narrows all groups by title.
- **D8.** The Spaces panel shows the recent spaces the server keeps, and its filter narrows only those rows. **Import** shows to administrators while bundles are on, and opens the administration's import and export section, because importing into a new space is a bundle today; importing into an existing space stays in its settings.
- **D9.** The choice of entries is stored as an order and a list of hidden entries. An entry that a later release adds goes at the end and shows, unless it starts hidden as Watching does.
- **D10.** Links to other tools in one place, not two: **the top bar** (NAV-099). It is on every frame, also the Plain and Settings frames and a narrow screen where the sidebar is hidden, while the sidebar's foot is not. NAV-130 and NAV-143 then fall away when that feature is built.
- **D11.** Shortcuts shows when the space has shortcuts, or when the person may change them (`space.look`, as the Shortcuts section of the space settings asks), with a muted sentence while it is empty.
- **D12.** Blogs lists the five newest posts, without grouping by year and month.
- **D13.** Until the create menu of SPC-112 is built (section 2.5), the add button of Content opens a menu of what can be made at the top of the tree today: New page, New database (while that part is on), Add a link.
- **D14.** The approvals digest of DOC-607 stays in the space menu, after Watch space. Archive space asks first; Restore space does not.
- **D15.** The account menu keeps Tasks, as NAV-096 lists, also though Tasks is an entry of the sidebar.
- **D16.** The current-row look of NAV-144 is one rule for every sidebar, so the sections of the settings sidebars get it too.

## Supersedes

ADR-046 records that this spec supersedes, as built: the strip of spaces and the fixed Home row of ADR-027 and spec 016 (H1); the part **This space**, its handle and the folding parts of the personal sidebar of spec 033; the quiet row of tree tools of spec 033; the top bar of ADR-035's "Top bar" rule apart from search in the middle; Administration in the account menu; the greeting, grid and side column of the start screen.

## Tests

Written before the code, failure paths first:

- `internal/app/reading_test.go`: an unknown entry, a repeated entry and a list that is too long are refused; a stored order and hidden list come back.
- `web/src/components/sidebar-entries.test.ts`: the order of entries with nothing stored, with an order that misses a new entry, with unknown ids; moving up and down at the edges; merging the cards of the start screen.
- `web/e2e/frame.spec.ts`: the landmarks (one banner, one main, the navigation of the sidebar); the top bar's order and that the settings button shows only to an administrator; the sidebar button and `[` hide and show the sidebar and keep it across a reload; each panel opens, takes the focus, filters, shuts by Escape and by Close with the focus back on its entry, and goes where a row says; the account menu's Theme sub-menu by keyboard; the customise dialog hides an entry under More and reorders by keyboard, and Cancel changes nothing; the space's header row, Back to top and the space menu; axe on each state; right-to-left and 360 px.
- `web/e2e/0-design.spec.ts`: new pictures for the panels, the account menu, the customise dialog, the space block and Drafts; every picture that changed is taken again.

## Acceptance criteria

- [x] Every line marked built above behaves as written, proved by the tests above.
- [ ] `make check`, the targeted mutations of the changed Go, the whole e2e suite once, and `0-design.spec.ts` twice in a row are green: still open, see the report for the runs and what waits for the build VM.
- [x] The product owner sees the before and after pictures in the report.

## How the product owner tests it

1. Log in. The top bar shows the sidebar button and the name at the start, search with Create in the middle, and the bell, help, the settings wheel and your picture at the end. Press the sidebar button, and then `[`: the sidebar goes and comes back, also after a reload.
2. In the sidebar press Recent, Favourites and Spaces in turn: a panel opens beside it with the cursor in its filter. Type a few letters, then press Escape: the panel shuts and the focus is back on the entry. In Spaces, star a space and find it under Favourites.
3. Press More, then Customise sidebar. Untick Tasks, move People up with its More button, and Save changes. Tasks is now under More. Open the dialog again and press Cancel after a change: nothing changes.
4. Open a space. Its name at the top of its block leads to the home page and is marked there. Its More holds Overview, People, Files, Archived pages, Space settings, Archive and Delete. Scroll the sidebar of a space with many pages: Back to top appears and brings you back.
5. Open your picture, then Theme, and choose Dark: the screen turns dark at once.
6. Go to the start screen: the cards show what you last read or edited, and the feed has its three buttons. No greeting and no grid of spaces.
7. Log in as someone who is not an administrator: there is no settings wheel.

## Provenance

Own words, from blueprint 03 chapters 1 and 2 as written in the study of 2026-11-14 (ADR-045, 02 "One exception"). No pictures, inventories or other product were consulted in this wave.
