# Design standard

This is how Kenwerk looks and behaves, and how that stays true for every future change. It binds every build session: `CLAUDE.md` makes it a hard rule, and `make check` and the browser tests enforce most of it.

Status: agreed with the product owner, 2026-11-04 (ADR-021). The look changed to **Stil** on 2026-11-07 (ADR-034, spec 033); where this document and ADR-034 differ, ADR-034 wins.

## The decision

After ten designs, three iterations and a final choice, the product owner decided:

| Part | Decision |
| --- | --- |
| Style and layout | **Stil** (ADR-034), which replaced the look of Helder: white and blue, hardly any lines or boxes, a clear top bar, the sidebar nearly always in view (also on the personal screens), one centred reading column that scales with the window (ADR-030). |
| Editor | **Rail**: a strip with the page's fate at the top, the tools in a row below it, and an open sheet with no boxes. |
| Page tree | Indenting shows the structure, without guide lines (ADR-034, which replaced Branches). Every branch can be collapsed. |
| Font | The **system font**. A font can be imported in the settings. |

Rail kept the global navigation visible while editing. In Helder that means: **the top bar stays**. After their first test the product owner asked to keep the pages in view as well, so **the sidebar stays too**, as far as there is room (ADR-030).

The intuition of a familiar wiki is the aim, never a copy. The clean-room rules (`docs/blueprint/02`) apply to design as well: no logos, icons, copy or pixel-exact layouts from any other product. The logo and every icon are ours.

## The arrangement decided on 2026-11-14 (ADR-045)

**Built so far:** the frame (spec 078, ADR-046): the top bar, the sidebar outside a space, its panels, the space's block of the sidebar, the start screen, the current item and the line of the frame; the page (spec 079); and the administration (spec 081, ADR-048): its sidebar in two tiers, the head of a section, the centred column, notices, app tables, Save and Reset changes and empty sections; and the settings of a space (spec 083): their sidebar with a head, pinned entries and five groups, the screens of their own beside it, switches in a card and tiles for a choice. The rules below for those parts now describe the built screens, and the old rules they replaced are gone from this document.

On 2026-11-14 the product owner decided that, for the arrangement of a screen, a study of pictures of another wiki product wins over this document: which regions a screen has, where they sit, what each holds, their order and grouping, what stays in view and how a state is marked. The look does not change: colours, icons, fonts and every token value stay as this document and `tokens.css` set them. Each change below replaces the named rule of this document when its screen is rebuilt, in a wave with its own study and new design pictures; until then the existing rule describes the built screen, and the pictures in `web/e2e/design-baselines/` keep guarding it. The requirement lines of blueprint 03 that end with "(changes the design standard, decided 2026-11-14)" are the source of truth for the detail; this list only names what changes and which rule it touches.

### Lines, cards and outlines

- Thin lines mark the regions of the frame, and nothing else: under the top bar, beside the sidebar, between the global part of the sidebar and the block of a space, and above the links to other tools. Replaces "hardly any lines or boxes" (The decision) for the frame. NAV-089.
- Some screens hold their content in outlined cards or rows: the login policies, your own profile, the choice between the two permission exports, the security overview, the list of new features, System health and the compliance cards. Replaces "hardly any lines or boxes" (The decision) on those screens. ACC-097, ACC-114, ACC-159, ADM-128, ADM-156, ADM-187, ADM-206.

### The current item

- The current item, in the tree, a sidebar or the sections of the settings, keeps the accent tint across its row, now with rounded corners, normal weight and a short accent bar on its start edge. Replaces the tint alone, "Background of the current item" (Tokens). NAV-144.

### The frames

- The start screen is one centred column: what the person last worked on as cards, the choice of feed as a row of pill buttons, then the feed. The greeting, the grid of spaces and the side column leave it. Replaces the start screen as built (Home in the Personal frame), and `Tabs` for "Switching between lists" (Components) on this screen. NAV-102, NAV-103, NAV-106.
- Your account has a bar of its own in place of the top bar, with its sections as text tabs, and one narrow centred column under it. Replaces the Plain frame and the top bar for "your account" (The frame of a screen), and `AccountNav` (Components). ACC-102, ACC-104.

### The sidebar outside a space

- On the Personal, Space and Focus frames the sidebar starts with one global part in a fixed order; the Settings frame keeps its own sidebar of sections without it. Replaces "a sidebar with your own places and your spaces" (Personal frame) and the top of "The sidebar from top to bottom" (The page tree). NAV-125.
- Recent, Favourites and Spaces open a panel beside the sidebar: a card with a filter field, groups, and its actions at the foot. Replaces the menus they open from the top bar today ("A menu in something that scrolls", Components). NAV-131.
- The Spaces panel does the work of the strip of spaces, which leaves the sidebar. Replaces "The strip of spaces scrolls sideways on its own" (The page tree). NAV-135.

### The top bar

- Three groups: at the start the button that shows or hides the sidebar, then the logo and name; in the middle search with Create directly after it; at the end the bell, help, a settings button and the account menu. Replaces "logo and name, Home, Spaces, Recent, Favourites and Create, then search in the middle" (Top bar). NAV-090, NAV-093.
- Home, Spaces, Recent and Favourites leave the bar for the global part of the sidebar. Same rule. NAV-091.
- The button that puts the sidebar away moves from the sidebar to the start of the bar. Replaces the place of the button behind "The sidebar itself can be put away" (The page tree). NAV-092.
- The settings button opens the administration, which leaves the account menu; only people who may use the administration see it. Adds to "the bell, help and the account menu on the far side" (Top bar). NAV-095, ADM-059.

### The page header

- The header row has the breadcrumbs at its start and, at its end outside the reading column, the publishing state, who else is reading, Edit, Share joined to Copy link, and More. There is no star and no lock: Share shows the lock's state, and the favourite moves into More. Replaces "breadcrumbs with Edit, Share, star, lock and More on one line" (A page). PAG-140, PAG-141. Built in spec 079.
- On the home page of a space the breadcrumbs hold only the space's name, and a thin line runs under the header row. Replaces "On the home page the breadcrumbs end in Home" (The page tree) and, for that line, "hardly any lines or boxes" (The decision). PAG-143, PAG-164. Built in spec 079.
- Contents, comments and Details open from a small vertical group of buttons in the bottom end corner, one panel at a time, and the contents no longer stand beside the column. Replaces "The contents list stands beside the column when there is room" (A page). PAG-147.
- Archive and Delete share one entry of More, and no entry of More is in the danger colour: the dialog that asks first carries it. Narrows "Destructive action" (Components) for this menu. PAG-149. Built in spec 079.

### The Settings frame and the administration

- Space settings: a sidebar of sections about a quarter wide, with a thin line beside it, in the place of the space's sidebar; a head that stays in place with a way back, the space's badge and name, and the jump field; four sections on their own, a divider, then five groups that fold. Replaces "the sections in the sidebar in groups that fold, with a way to jump to one" (Settings frame) and "The sidebar holds the sections in groups that fold" (Settings and lists). SPC-134, SPC-136, SPC-138. Built in spec 083: the head holds the way back and **Space settings** in bold, then the badge and name, then the jump field with a search mark; it has no line under it at any time. Group headings read like entries, with a thin chevron in a column of its own; entries of a group are indented one step past their heading. The screens of their own, such as Content and Trash, open beside this sidebar for whoever may open space settings (SPC-135); its entries and their places are one list, `views/space-sections.ts`.
- Within those groups: sections indented as plain text, a chevron in its own column, every group open at first, the scroll position kept, and a set reach for the band of the current section. Same rules. SPC-146, SPC-147.
- The administration's sidebar: a header row with a way back that stays in place, tool sections with icons that are always open, then a settings heading with the jump field and the groups that fold, their state kept per person; a thin line beside it. Same rules. ADM-060, ADM-061, ADM-062. Built in spec 081.
- The administration's sections regroup into three tool sections and five settings groups, with each section in exactly one. Replaces "The administration has these groups: Overview; People and access; Content; Automation; Look; System" (Settings and lists). ADM-068 to ADM-075, ADM-077. Built in spec 081.
- The large area holds forms and short lists in one column of reading width, centred between the sidebar and the far edge, and wide lists and builders at full width beside the sidebar. Replaces "It starts beside the sidebar, not in the middle of the room" (Settings and lists) and the Look item of ADR-038. ADM-078, SPC-150. Built for the administration in spec 081 and for space settings in spec 083.
- A section from top to bottom: its name as the largest heading with More or its primary action on that line, the description with its help link, notes, a row of tools, then the content, with no breadcrumb and no save bar. Its parts get visible sub-headings, set apart by space. Replaces "The large area shows one section, with its name as the page title" (Settings and lists) and spec 033 D7. SPC-148, SPC-149. Built for the administration in spec 081 (`SettingsSection`) and for the sections of a space in spec 083, without the row of tools; a screen of its own keeps its heading until its own wave.

### Settings forms

- One column of reading width, small bold labels above narrow fields of one width, and facts that cannot change below the fields; tiles for a choice among a few, a slider for a number on a scale, and a preview beside the form. Replaces "A labelled input" with `Field` (Components) as the whole rule for a form. NAV-160, SPC-151.
- Save and Reset changes as one unit at the start side, directly under its fields and inactive until something changes, with one Save per group that saves apart. Replaces "Main action, one per view" (Components), which now counts per group, and adds an inactive state beside "Busy" (States). NAV-162, NAV-163. Built for the administration in spec 081 (`SaveReset`) and for space settings in spec 083.
- A list of switches is one bordered card with headed groups and lines between rows, and settings that belong together may be rows in one outlined card with their controls in one column. Replaces "hardly any lines or boxes" (The decision) for these lists. NAV-167, ADM-096. Built for the features of a space in spec 083 (`.settings-card`).
- A section that starts an action: heading, explanation, choices, then one primary button at the foot and no Cancel. Sets where the "Main action, one per view" (Components) goes in such a section. SPC-153.

### Lists and tables

- Tables of the app have a quiet heading row with a line under it, rows set apart by space without lines between them, a line after the last row, a hover tint and one sort mark; rows are compact in the administration and up to two lines for tokens and sessions. Replaces "a line under each row" (Settings and lists). NAV-157, ADM-107, ACC-085, SPC-154. Built for every table in a `TableBox` in spec 081.
- Several rows of one item form a group under one spanning cell, and a two-level list shows its levels by weight. Same rule. SPC-155.
- A long list has one row of tools above it: search first, a button per filter that shows its value in words, more filters, and the screen's own action at the end; a filter applies at once. Replaces "Long lists filter with dropdowns above them" (Settings and lists). ADM-103.
- Lists of people have a checkbox column, the picture and name with a second line, the role chosen in the row with a button that explains it and how the holder got it, and More at the end; the list of users shows status and last login. Replaces "Lists of people show a picture, the name, how they got in and the role" (Settings and lists). SPC-118, ADM-131.
- The roles screen is three headed sections, with no search, sort, filter or heading row. Replaces "Tables of the app show rows, not boxes" for that screen. ACC-152.
- Every list and report shows an icon of the item's kind before its title, or the page's emoji. Replaces "A page in a list" with `PageMark` (Components). NAV-156.

### The sidebar of a space and the page tree

- Inside a space the block of the space follows the global part after a line: the space's header row, then groups for shortcuts, content with its filter and the tree, blogs, calendars and space apps, each under a heading of an icon and a word with its buttons in one shared column. Replaces "The sidebar from top to bottom: the strip of spaces, the name of the space, the part This space" and "The part folds shut from its heading" (The page tree). SPC-097, SPC-099.
- The space's header row leads to the home page, is marked while it is open, and always shows its More. Replaces "It has the fixed Home row" (The page tree). SPC-098.
- The fixed entries, the links at the bottom, the Home row and the strip of spaces leave the sidebar, and Overview, People, Files, archived pages and the settings move to the space menu. Replaces the part This space and the strip of spaces (The page tree). SPC-106.
- The heading of the content carries add and a menu of views and tools, the title filter is always shown under it, and the row of icons above the tree goes. Replaces "are a quiet row of icons above it" (The page tree). SPC-101.
- A Create row and that add button open a menu of the kinds of item, a line, then folder. Replaces "new page, link" among the tools of the tree (The page tree). SPC-112.
- Every row of the tree has a lead column with a chevron or a dot, the icon of the item's kind or its emoji, then the title and its marks, at one even height. Replaces "a page without children has a page icon" (The page tree). SPC-109.
- No row shows its buttons at rest, the current one included. Replaces "The current page has the tint, and shows the add button" (The page tree). SPC-110.

### The editor

- The top strip has the page's icon and bold title at the start, and the save status, the people editing, Publish and Close at the end; the summary and Discard draft move into the step that Publish opens. Replaces "Close on the left with the save status beside it; on the right the summary, Discard draft and Publish" (The editor). EDT-165, EDT-166.
- The row of tools gets a new order, with direct buttons for common blocks, History after undo and redo, and Find and Full width in a last group; highlight moves into More formatting and indent into the list choices. Replaces the order of "Below it one row of tools" (The editor). EDT-168, EDT-169.
- An active tool is an accent icon on the accent tint. Replaces "An active tool is solid accent" (The editor). EDT-171.
- A reduced byline sits between the title and the body, followed by a wide gap. Adds to "The title is a large plain input" (The editor). EDT-174.
- Insert opens one large dialog over the dimmed editor: a column of categories, a grid of entries in three columns that alone scrolls, the title and search on one line, and the actions at the end. Replaces "Insert opens the same list in a dialog" (The editor). EDT-175, EDT-176.
- The chosen entry in that dialog has the tint and a bar on its start edge, without a checkbox, because only one is chosen. Replaces "Selected rows have the accent tint and a real checkbox" (What organises a page) for this one choice. EDT-180.
- Entries of the slash menu and of that dialog have icon tiles, one icon per family, and an app's entry shows the app's own picture inside our tile. Adds to "Icons are our own line drawings" (Icons and logo). EDT-191.
- A table gets a button in the cell with the cursor and a small floating bar under it, in place of the one button on its outline; the row of tools still shows nothing for tables. Replaces "A table has one button on its outline" (The editor), and the table bar is an exception to "never a tool the row lacks". EDT-209, EDT-213.

### Insight and automation

- A card on the administration's summary has a bold title, a small action, a large figure, a caption, then its chart; a chart of parts may hold several named parts, and the chart of spaces has two series that differ by shape. Replaces "has one series, so nothing rests on colour" (Insight and automation). ADM-122, ADM-123.
- Templates of rules are a grid of equal cards, and the figures of use are two framed tiles. A new kind of card and a framed form of `.tiles`, in place of "Reuse beats a new variant" and "Numbers on a summary" (Components). PLT-123, PLT-126.
- A rule opens in the large area of the Settings frame: a header strip, a canvas with the steps as a chain of cards joined by lines, and a settings panel over the end side of the canvas. Replaces "A rule is a form of native fields in When, If and Then" (Insight and automation). PLT-129, PLT-134.
- A step's card holds an icon tile, a title and a second line, with its kind shown by icon and colour and in words; a small add button after every step asks for the kind first and shows a placeholder card. Same rule. PLT-135, PLT-137.
- The panel shows the rule's summary and a card of details while no step is selected, and a step's settings with Back and Next while one is; saving happens only in the header, by a split button that stays unavailable until a trigger is chosen. Replaces "a summary at the top" (Insight and automation) and "Busy" (States) for that button. PLT-131, PLT-144, PLT-148.
- A step that may not fit starts with a tinted warning without a close button; matching all or any conditions is a pair of toggle buttons, and an empty list of conditions shows only its add button. Replaces "with each message beside its field" (Insight and automation) and the muted sentence of "Empty" (States) there. PLT-143, PLT-150.

### Working together

- A mention shows the person's small picture or initials before the name, without `@`. Replaces "A person mentioned is a soft box with `@`" (Working together). COL-100.
- The panel of the bell hangs under it with its own scrolling, and the bell looks pressed with its count hidden while it is open. Adds to "The bell carries a number capped at 99+" (Working together). COL-113. Built in spec 079.
- The share dialog is a card near the top: a title row with More and close, the field for recipients, suggestions, one row about access, a line, and its actions at the start side. Replaces "`Dialog`, with buttons in `.dialog-actions`" (Components) for sharing. COL-102. Built in spec 079.

### Notices and states

- An empty section that the person may fill shows a centred block with a bold heading, a few lines, the primary button and a guide link, and no create button in its heading. Replaces "A muted sentence that says what will appear here" (States, Empty) for such a section. NAV-173. Built in spec 081 (`EmptyState`) for Groups, Service accounts and Webhooks.
- A part that is still loading shows a framed box with a sentence and a placeholder in the shape of the result, and a figure that loads is a pale bar. Replaces `Loading` (States, Loading). NAV-177, PLT-128.
- Three states join Busy and No permission: a button that starts something stays inactive until its form is complete, with a line that says what is missing; a read-only setting keeps its control in view, muted; a section whose optional service is missing says what it needs. Adds to "Every screen handles all of these" (States). ADM-094, ADM-102, ADM-115.
- Before an export, a warning panel with a bold title names what the chosen format cannot hold. Adds a kind to "A message in the flow" (Components). EXC-052.

## How the standard holds

Words alone do not keep a design together, so the rules are checks. A failing check blocks the change.

| Rule | Enforced by |
| --- | --- |
| Colours, font sizes, families, weights, line heights, corners, shadows, layers and spacing come from `web/src/design/tokens.css` | `npm run design` (in `make check`) |
| Every token exists, and every token is used | `npm run design` |
| Text and controls have enough contrast, in light and in dark (WCAG AA) | `npm run design`, on the pairs in `check-design.mjs` |
| Layout is right-to-left ready: logical properties only | `npm run design` |
| No inline styles, no `!important`, no style sheets outside `web/src/design/` | `npm run design` |
| Every class in the code has a style | `npm run design` |
| No emoji or glyphs as icons; `<svg>` only in `components/icons.tsx` | `npm run design` |
| No `window.confirm`, `prompt` or `alert` | `npm run design`, and a browser test |
| Every screen has an approved picture | `npm run design`: each `…View` needs a test named `…View:` in `web/e2e/0-design.spec.ts` |
| Screens still look as approved, in light, dark, narrow and right-to-left | `web/e2e/0-design.spec.ts` against `web/e2e/design-baselines/` |
| Accessibility | axe in the browser tests |
| All words come from the translation files | `npm run strings` |

`web/scripts/check-design.test.mjs` proves that each rule fails on a violation.

**What no check can see:** whether the right component was chosen, whether a new screen feels like the others, and whether the words are good. That is what the checklist below is for. A picture only guards the state it shows: the first reading column looked fine with long text and collapsed with a short one. Look at a new screen with very little content as well as with a lot.

## Tokens

`web/src/design/tokens.css` is the only place with design values. The groups:

| Group | Prefix | Use |
| --- | --- | --- |
| Type | `--font-*`, `--text-*`, `--weight-*`, `--leading-*` | Interface text is `--text-ui`, documents `--text-prose` |
| Space | `--space-*` | All margins, paddings and gaps |
| Fixed sizes | `--size-*`, `--tree-*` | Controls, bars, columns |
| Corners, depth | `--radius*`, `--shadow-*`, `--layer-*`, `--motion-*` | 4px corners, 6px on cards and dialogs |
| Colour | `--color-*` | Both themes define the same names |

The colours that carry the look:

| Token | Light | Use |
| --- | --- | --- |
| `--color-ink` | `#1E2A3B` | Text |
| `--color-strong` | `#131C2B` | Headings |
| `--color-muted` | `#566278` | Secondary text |
| `--color-accent` | `#1D5FBF` | Links, primary buttons, the current item |
| `--color-accent-tint` | `#E4EDFA` | Background of the current item, which also has rounded corners, accent text in normal weight and a short accent bar on its start edge (NAV-144) |
| `--color-side` | `#F5F7FA` | Sidebar, toolbar |
| `--color-line` | `#DDE2EA` | Dividers |
| `--color-guide` | `#B9C6D8` | Quiet outlines, such as the edge of a table in a page. No longer the tree |
| `--color-lozenge` | `#FBEFD5` | Status marks such as Restricted |
| `--color-text-*` | 12 names | What a writer can choose for text. Each passes 4.5:1 on the page, in both themes |
| `--color-mark-*` | 6 names | Highlights, panels, status marks and table cells. Normal text on each passes 4.5:1 |

Need a value that is not there? Use the nearest token. If none fits, add one to `tokens.css` for both themes, and add a contrast pair if it carries text.

## The frame of a screen

Every screen is one of these. Do not invent another.

| Frame | Used for | Built from |
| --- | --- | --- |
| **Plain** | Search, your account, people, labels | Top bar, `Plain` in `app.tsx` (wide, or `reading`) |
| **Space** | Everything inside a space | Top bar, `Frame` or `SpaceShell` in `components/frame.tsx`: sidebar and a centred `.column` |
| **Settings** | The administration, and the settings of a space | Top bar, `SettingsFrame` in `components/settings-frame.tsx`: the sections in the sidebar in groups that fold, with a way to jump to one, and the large area that changes them. One section at a time, at its own address (`?section=` in a space, `/admin/<id>` in the administration). The administration adds a header row with a way back, tool sections with icons before the groups, and the jump field under a Settings heading; its sections and their places are one list, `views/admin-sections.ts`. A space has a head that stays (way back, badge and name, jump field), four pinned entries with icons, a divider and five groups; its entries are one list, `views/space-sections.ts`, and a screen of its own frames itself with `SpaceToolFrame`. The settings of a template keep the plain list |
| **Personal** | The start screen, Recent, Favourites, Drafts, Tasks, Watching, all spaces | Top bar, `HomeFrame` in `components/home-frame.tsx`: the sidebar with the global part (`GlobalPart`) and the favourite spaces, beside a `.column` |
| **Focus** | The editor | Top bar, sidebar, `main.focus`: `SpaceShell` with `focus`, and no reading column around it |

The login page stands alone: one card on the sidebar colour.

**Top bar (spec 078):** three groups. At the start the button that shows or hides the sidebar (`SidebarToggle`, which a frame with such a sidebar places into the bar, so the Plain and Settings frames have none), then the logo and name. In the middle search with Create directly after it. At the end the bell, help, the settings button to the administration (administrators only) and the account menu: round buttons with only an icon, a neutral square on hover and focus, framed while their menu is open. The account menu starts with who you are, then Profile, Your account and Theme (a sub-menu), then a group headed with the instance name, then Log out. Search is the main element of the bar, because people spend much of their time there (ADR-035): it takes the room there is, up to `--size-search-main`, and is never pushed to a corner. Create adds a page in the current space, or a space when there is none, and a small menu offers both. The search box is a combobox, and it always searches every space: there is no chip that keeps it inside one (spec 033). Clicking it unfolds one panel: first the filters as buttons (Spaces, Type, Labels, People, Date), each with a small chooser below the row, then the results; the space chooser has a field that finds any space by name. Chosen spaces, labels and people become words of the search syntax (`space:`, `label:`, `author:`), so a typed query and the buttons mean the same thing; type and date travel as the parameters the results page reads. The results page shows those words as chips that take a word out again.

**A page:** a header row with the breadcrumbs at its start and, at its end outside the reading column, the publishing state, who else is reading, Edit, Share with the lock on it joined to Copy link, and More (spec 079); the title; the byline; the document; child pages; reactions; comments. It fills the reading column, however short the text is. Files sit in the document as tiles, and a page has no upload (ADR-022). Rarely used actions live in More, never as a row of buttons. A header image sits above all of it. The contents list stands beside the column when there is room, and the column keeps its width. The column scales: `--size-page` is at least `--size-reading`, 64 percent of the room, and at most `--size-reading-max`. Full width takes all the room. Details is one panel on the far side, on top of the page, and does not block reading. Comments use the same place, and only one of the two is open at a time. On a narrow screen comments become a sheet from the bottom, so the words stay above it.

## Components

Use these. Reuse beats a new variant.

| Need | Use |
| --- | --- |
| Main action, one per view | `<button className="primary">` |
| Normal action | `<button>` or `<a className="button">` |
| Destructive action | `className="danger"`, and ask first |
| Quiet action next to a primary one | `className="quiet"` |
| Action inside a table or list | `className="ghost"` |
| Icon only | `className="icon"` with `aria-label` and `title` |
| Asking before something hard to undo | `ConfirmButton`, or `ConfirmDialog` from a menu |
| A form or detail on top of a screen | `Dialog`, with buttons in `.dialog-actions` |
| A list of less used actions | `Menu`, `MenuItem`, `MenuLink`, `MenuDownload`, `MenuSeparator`, and `MenuSub` for an entry with choices of its own, which opens beside its menu or over it on a phone |
| A setting that is on or off at once | `Switch` in `components/switch.tsx`, a checkbox with `role="switch"` drawn by `.switch`, its name before it; it applies at once and `useSaved` says so |
| A choice among a few | `ChoiceTiles` in `components/choice-tiles.tsx`: tiles with a bold title, a muted line and a radio |
| A labelled input | `Field` |
| Choosing a file outside the editor | `FilePicker` |
| A file in a document | `.file-tile`, through the `file` block in `components/editor.tsx` |
| A person | `Avatar` |
| A space | `SpaceBadge`, which shows its animal |
| Choosing the icon of a space | `AnimalChoice` |
| A person or group in a form | `SubjectPicker`, which shows people on focus |
| A comparison of versions | `DiffDocument` |
| A status | `.lozenge` |
| A person | `PersonName` for a name on someone's work, `PersonPicture`, and `PersonMark` for the words Guest and No longer active |
| Something the whole instance says | `.site-note` under the top bar, a static region. Never a dialog that opens by itself |
| A new password | `NewPasswordField`, with Show password. Paste always works |
| Tabs inside a screen | `Tabs`, with arrow keys |
| Sections of Your account | `AccountNav`, which holds real addresses |
| Numbers on a summary | `.tiles` |
| A colleague in the same document | `TogetherPeople`, and a cursor with a name in words, from `renderCaret` |
| A table people sort | `SortTable` in `components/sort-table.tsx`, which keeps `aria-sort` right |
| Any other table of the app | `TableBox` in `components/table-box.tsx` around the `<table>`: a wide table scrolls there, and only then is the box a named Tab stop |
| Who else is on a page | `PresencePeople`: pictures up to five, then a number, and a quiet status line |
| A block that fills itself | `.dynamic`, with `.dynamic-head` only while editing. Empty is one muted sentence |
| A chart | `Chart` in `components/chart.tsx`. Series differ by shape as well as colour, and the numbers are one press away |
| Something drawn | Only `components/icons.tsx`, `components/chart.tsx` and `components/drawing.tsx` may hold SVG (ADR-026) |
| A message in the flow | `Notice` in `components/notice.tsx`: information, warning (always with a title) or error, with its icon in a column of its own; `ErrorNote` is its error kind. `.note` and `.note-error` remain outside the administration until each screen adopts `Notice` |
| The head of a section of the settings | `SettingsSection` in `components/settings-section.tsx`: title, introduction with its guide link, notices; the content puts its primary action in the title row with `SectionActions` |
| Save and Reset changes of a unit | `SaveReset` in `components/save-reset.tsx`, inactive until the unit changed |
| A section with nothing in it yet | `EmptyState` in `components/empty-state.tsx`, and `TableEmpty` for the one row of an empty table |
| A short message, maybe with Undo | `useToast`, and `useCopy` for copying |
| A time | `RelativeTime` |
| Switching between lists | `Tabs`, which brings the arrow keys; on the start screen, pills (`.pill` with `aria-pressed`) |
| A panel beside the sidebar | The panels of `components/sidebar-panels.tsx` |
| Writing a comment | `CommentBox` |
| Counting responses | `Reactions`, for a page or a comment |
| A menu in something that scrolls | `Menu` with `floating` |
| A page in a list | `PageMark`, which shows its emoji |

## Settings and lists (ADR-034)

- Settings of a space and of the instance use the **Settings** frame. The sidebar holds the sections in groups that fold, and **Jump to a setting** narrows them; the administration's own search finds its sections by name and the settings inside them. Nothing in the large area runs past the window: a wide table scrolls in its own `TableBox`, and the page itself never scrolls sideways. The settings of a space (spec 083) have four entries on their own (Automation while on, People, Content, Details) and five groups, General, Access, Pages and files, Look and Connections; a new entry goes into exactly one place in `views/space-sections.ts`, with the permissions that open it and its width, and every entry starts with `SettingsSection`.
- The administration (spec 081) has three tool sections, Overview, Tools and People and access, and five settings groups, Instance, Security, Permissions, Look and Data. A new section goes into exactly one group, in `views/admin-sections.ts`, with its width: `column` for forms and short lists, `full` for wide lists. Every section starts with `SettingsSection`.
- Tables of the app show rows, not boxes: a quiet heading row with a line under it, rows set apart by space, a line after the last row and a tint under the pointer, no cell borders. Row actions sit in the last column, whose heading only screen readers hear. Tables inside a page keep their own rules.
- Lists of people show a picture, the name, how they got in and the role. Someone who may change a role picks it in the row. Long lists filter with dropdowns above them.
- A rule of the instance says where it acts: in every space, or only in the chosen spaces, shown as chips.
## The page tree

- A branch has a chevron and can be collapsed; a page without children has a page icon.
- Indenting shows the branches; there are no guide lines (ADR-034).
- The tree starts folded from the first level: the top pages show, with their branches shut, so the shape of a space shows at once (spec 033).
- What a reader collapses stays collapsed, per space, across pages and reloads.
- Going to a page reveals it, also when it sits in a collapsed branch.
- The current page is marked as the current item (NAV-144), and shows the add button. Other rows show it on hover or focus.
- The sidebar itself can be put away from the button at the start of the top bar, or with `[`, and starts hidden on narrow screens. Its width is a preference. A thin line sets it apart from the main area.
- The sidebar from top to bottom (spec 078): the global part (`GlobalPart`: the start screen, then the person's own entries in their order, then More or Customise sidebar); inside a space, after a line, the space's block (`SpaceBlock`): the header row with the badge, the name and More, then Shortcuts, Content, Blogs and Space apps, each under a heading of an icon and a word with its buttons at the end. The heading of Content carries add and order and view; the filter is always shown under it, then the tree. On the personal screens the favourite spaces follow the global part instead. The whole sidebar scrolls as one; Back to top floats over it once it has scrolled. The flat views say that the tree is where pages are arranged.
- Recent, Favourites and Spaces open a panel beside the sidebar (`components/sidebar-panels.tsx`): a card with a title and Close, a filter field with the focus, groups under small muted headings, and its actions at the foot under a line. Escape and Close give the focus back to the entry.
- The home page is not a row of the tree. The space's header row leads to it and is marked while it is open, and its children are the top rows. On the home page the breadcrumbs hold only the space, as the current place, and a thin line runs under the header row (spec 079).
- The space menu holds the space's tools (Overview, People, Files, Archived pages), Export, Automation and Space settings, then Archive and Delete.
- The tree is a `nav` with links and the keys of the tree pattern, so it is one Tab stop plus the buttons of the focused row.

## The editor

- Top strip: Close on the left with the save status beside it; on the right the summary, Discard draft and Publish.
- Below it one row of tools: text style, marks, More formatting, colour, highlight; alignment and indent; lists and blocks; link, image or file, table, Insert, Find; undo and redo. It is one Tab stop: arrow keys move inside it, Escape returns to the text.
- The tools for what is selected appear at the end of the same row: image, panel, section, columns, status mark, date, code block, table. Text fields come last, because they need the arrow keys themselves. Selected words and a selected image also get a small floating bar with the tools most used on them (ADR-038, spec 047): never a tool the row lacks, above the selection or below it on a touch screen, never over the row, Alt+F10 to reach it and Escape to leave.
- `/` opens a list at the cursor, and Insert opens the same list in a dialog.
- A block has a grip and a menu in the margin, outside the text. Everything the grip does is also in the menu and on the keyboard.
- A file can be dropped anywhere below the strip. The sheet shows a dashed outline while it is above the editor.
- An active tool is solid accent, so its state is clear without reading it.
- The sheet has no border, and scales like a page. The title is a large plain input.
- Above the title: Add emoji, Add header image and Page options, quiet until hover or focus, and always there on touch screens.
- A table has one button on its outline, with one menu for everything about tables, and nothing for tables appears in the row. It has an outline in `--color-guide` and a grid in `--color-line`, and the table with the cursor has an accent outline. Its end edge stretches it.
- Both bars stay visible while scrolling, except on narrow screens.

## The blocks of a document

`web/src/design/blocks.css` holds them. A writer picks names, never values: a document can only say `red`, and each name has a light and a dark value in the tokens. Meaning never rests on colour alone: a panel has an icon and a name, a status mark has words, a match in Find has an outline.

| Block | Look |
|---|---|
| Panel | A tinted box with an icon: info blue, note purple, success green, warning yellow, error red |
| Collapsible section | A thin frame. Readers get the browser's own `details`. Writers see it open, with its title as a field |
| Columns | No frame for readers, a dashed outline for writers. They stack on narrow screens |
| Decision | A green line on the start side, with an icon |
| Status mark, date | A soft box in the line of text |
| Code block | A header with title, language and Copy for readers. Always left-to-right |
| Image | A `figure` with a caption, in three sizes or its own |

## Changes between versions

A comparison looks like the page. What was added sits on green, what was removed on red and struck through, and new words are underlined. Each changed block has the word Added, Removed or Changed beside it, so colour never stands alone. Words can be marked in paragraphs and headings. Other blocks show as they were and as they are.
## What organises a page

- **The byline** is one line of small muted parts: the owner, the view count, Add a reaction, the state of a controlled page, the marks, and the status at its end. It has no date and no version (spec 079).
- **Status** is always a word and a colour, in the style of a status mark in a document. It sits at the end of the byline.
- **Verified and Approved** have an icon and a word.
- **Labels** sit at the bottom of a page as plain links with a quiet `#`, without a box (ADR-034). Each has its own remove button, of at least 24 pixels. Adding one reads as quiet words until hovered or used, and then it is a field.
- **Notes on a page**, such as archived, scheduled and approval, use `.note` under the byline. Past review is a muted line, for editors only.
- **The publish arrow** sits beside Publish, in the same colour, with a small gap.
- **Fields and instruction text** in a template: a field has a dashed border, instruction text is muted and italic.
- **Selected rows** have the accent tint and a real checkbox, so state never rests on colour.
- A restricted item in the trash shows the words "A restricted page", with no title.

## Insight and automation

Every chart stands beside a sentence with its total and trend, and has one series, so nothing rests on colour. Empty says why: analytics off, or no reader yet. A rule is a form of native fields in When, If and Then, with each message beside its field, a summary at the top, and Move up and Move down instead of dragging.

## Working together

`web/src/design/together.css` holds comments, reactions, mentions, the bell and the feed.

- A comment on words is a soft tint with an underline, so it does not rest on colour alone. It is a `button`, and its name says who wrote what.
- A selection in a page shows one small floating row, because a reader has no row of tools. The floating bars of the editor share its look.
- A person mentioned is a soft box with `@`. An overdue task date is red and says "overdue" in words.
- The bell carries a number capped at 99+. A hidden `role="status"` line only changes when the number goes up. While its panel is open the bell looks pressed and hides the number; the panel has a fixed height, the tabs For me and Watched, and a heading per day (spec 079).
- Six common reactions come from `components/comments/common-reactions.json`. They are content, not icons.

## States

Every screen handles all of these:

| State | How |
| --- | --- |
| Loading | `Loading` |
| Failed | `ErrorNote`, or `.note-error` with a clear sentence |
| Empty | A muted sentence that says what will appear here, not a blank |
| No permission | The control is absent, not disabled. Pages say "does not exist, or you cannot see it" |
| Busy | The button is disabled while the action runs |

## Icons and logo

`web/src/components/icons.tsx` is the only file that draws. Icons are our own line drawings on a 24 by 24 grid, stroke 1.8, round ends, `currentColor`, hidden from screen readers. To add one, add a line to that file. An icon needs words beside it, or an `aria-label` and `title` on its button.

The logo is a river that splits in two, a mark kept from the former working name of the product (spec 095). User marks use four tints, chosen by the name, so they never change by themselves.

A space has a cartoon animal (ADR-031), picked from its key until someone chooses, and it can choose a character instead, which uses the tints. The animals are pictures from `web/public/animals/`, the one exception to our own line drawings, because they carry character and not meaning. They are always next to the name of the space.

## Words

UI text is English, in sentence case, through `web/src/locales/en.json`. Name things plainly: "Discard draft", not "Abort". Errors say what happened and what to do. Buttons say what they do; a confirmation repeats the action on its button instead of saying OK.

## Accessibility, dark mode, right-to-left, small screens

- WCAG 2.2 AA is the goal. Everything works from the keyboard, focus is always visible, and menus follow arrow keys and Escape.
- Dark mode follows the operating system, or a person's choice in Preferences. A token holds both colours in `light-dark()`, so nothing needs a dark variant of its own.
- Only logical properties, so a right-to-left language needs no new styles. Icons with a direction are mirrored in `layout.css`. Code stays left-to-right.
- Below 50rem the top bar wraps, the sticky bars let go, and the sidebar starts hidden.
- Motion is short and stops with `prefers-reduced-motion`.

## Fonts

The default is the system font: nothing to ship, license or download, and it fits each person's device. An administrator can import a font under **Administration, Look, Font** (spec 006). `web/src/appearance.ts` is the only code that applies it, by replacing `--font-sans`. The system stack stays behind it, so a font that fails to load changes nothing. The licence of an imported font is the responsibility of whoever imports it.

## Adding or changing a screen

1. Pick one of the four frames, and look at the nearest existing screen.
2. Use the components above. Need something new? Add it once in `components/` and to the table in this document.
3. Style with classes in `web/src/design/`, using tokens only.
4. Handle every state in the table above. All words go through `en.json`.
5. Add a test named `YourView: …` to `web/e2e/0-design.spec.ts`, with stable data, and mask what changes by itself such as dates.
6. Run `scripts/e2e design.spec.ts --update-snapshots`, then **look at each new picture**: compare it with the screens around it, also in dark and narrow if it matters.
7. Run `scripts/dev make check` and `scripts/e2e`.
8. Commit the pictures with the change. In the pull request, say which pictures changed and why.

Never update a picture just to get past a failure. A failing picture means the screen changed: either that was intended and you can say why, or it is a bug.

## Changing the standard

The look is the product owner's decision. Changing a token value, a frame or a rule in this document needs their agreement and a new ADR. Ask them with the before and after pictures. ADR-045 is that agreement for the changes listed under "The arrangement decided on 2026-11-14 (ADR-045)", and each wave that builds one still shows the product owner the before and after pictures.
