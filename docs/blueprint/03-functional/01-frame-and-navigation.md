# 03.1: The frame and getting around

How to read this chapter: see `../03-functional-requirements.md`. Every line has a number and a status: **built**, **partly**, **phase N**, **new** or **later**.

## 1.1 What every screen has in common

- **NAV-001** · built · Every screen after login is one of four frames: Plain, Space, Admin or Focus (`docs/design/README.md`). No screen invents a fifth.
- **NAV-002** · built · The top bar is on every frame. It stays in view while scrolling, except on narrow screens.
- **NAV-003** · built · Everything a person can reach by mouse can be reached by keyboard, and focus is always visible.
- **NAV-004** · built · A person who cannot use something does not see it. A control is absent rather than greyed out, unless the reason helps them, and then the screen says why.
- **NAV-005** · built · A page or space a person may not see behaves as if it does not exist. The wording never reveals that something is hidden.
- **NAV-006** · built · Every list, card and search result shows only what the person may see at that moment. A page that was restricted a second ago disappears from their favourites, recent list and search.
- **NAV-007** · built · Every screen handles five states: loading, failed, empty, no permission and busy. An empty list says what will appear there.
- **NAV-008** · built · Addresses are stable. A page keeps its address when it is renamed or moved, so links in mail and chat keep working.
- **NAV-009** · built · The back and forward buttons work, and every screen has its own address that can be shared or reloaded.
- **NAV-010** · built · After a reload, a person returns to the same scroll position in a long page.
- **NAV-011** · built · The browser tab shows the page title and the instance name, so many open tabs stay readable.
- **NAV-012** · built · A failed action never loses typed text. The form stays open with the message beside it.
- **NAV-089** · new · Thin dividing lines mark the regions of the frame: one under the top bar across the full width, one between the sidebar and the main area, one between the global part of the sidebar and the block of the current space, and, inside a space, one across the sidebar above the links to other tools at its foot. The frame draws no other lines (changes the design standard, decided 2026-11-14).

## 1.2 The top bar

- **NAV-013** · built · Far side from the reading direction: logo and instance name, which go to the start screen.
- **NAV-014** · built · **Home** goes to the start screen.
- **NAV-015** · built · **Spaces** opens a menu of spaces.
- **NAV-016** · partly · **Recent** opens a menu with the last pages the person visited, with tabs for pages they visited, pages they worked on and their drafts. Each row shows the title, the space and when. Still open: the tabs live on the Recent screen, and the menu lists visited pages.
- **NAV-017** · built · **Favourites** opens a menu with favourite pages. It stays empty with one sentence that explains how to add one.
- **NAV-018** · built · **Create** is the one primary button of the top bar. Inside a space it adds a page below the page being read, if the person may. Outside a space it starts a new space.
- **NAV-019** · built · Create offers a small menu when there is more than one thing to make: page, page from a template, blog post, space. The most used choice stays one click away.
- **NAV-020** · built · The search box sits on the far side.
- **NAV-021** · built · A bell shows how many unread notifications there are, and opens the notification panel (chapter 7).
- **NAV-022** · built · A help menu offers the keyboard shortcuts, the documentation and the version number.
- **NAV-023** · built · The account menu shows the person, a link to their account, Administration when they are an administrator, and Log out.
- **NAV-024** · built · The account menu also offers their personal space, their profile and their preferences.
- **NAV-025** · built · An administrator can set the instance name and logo, and the logo replaces ours in the top bar (ADM-013).
- **NAV-090** · new · The top bar is one thin strip across the full width, in three groups. At the start: the button that shows or hides the sidebar, then the logo with the instance name (with the button for other tools of NAV-099 between them when there is one). In the middle: the search box with Create directly after it. At the end: the bell, help, the settings button and the account menu, in that order (changes the design standard, decided 2026-11-14).
- **NAV-091** · new · Home, Spaces, Recent and Favourites are not in the top bar. They are entries of the global part of the sidebar (section 1.10), where NAV-014 to NAV-017 keep their behaviour (changes the design standard, decided 2026-11-14).
- **NAV-092** · new · The button that shows or hides the sidebar is the first item of the top bar, so the sidebar can be brought back from the place where it was put away. It does what `[` does (NAV-064) and takes the place of the hide button inside the sidebar (SPC-022) (changes the design standard, decided 2026-11-14).
- **NAV-093** · new · The search box and Create stand together as one group in the middle of the bar, with a small gap between them. The search box is the widest item of the bar and takes the room there is up to its maximum width (ADR-035), so it no longer sits on the far side, as NAV-020 says (changes the design standard, decided 2026-11-14).
- **NAV-094** · new · The bell, help, the settings button and the account are round buttons with only an icon, the account with the person's picture or initials. Each has its name as accessible name and as tooltip, and the bell shows no number when nothing is unread. Create is the only filled button of the bar. A hovered or focused icon button gets a neutral tinted square behind it, and a button whose menu or panel is open keeps a framed look until it closes.
- **NAV-095** · new · The settings button, between help and the account, opens the administration (ADM-001). Only people who may open the administration see it (NAV-004), and Administration leaves the account menu, where NAV-023 puts it (changes the design standard, decided 2026-11-14).
- **NAV-096** · new · Beyond NAV-023 and NAV-024, the account menu holds, from top to bottom: an identity block with the person's picture, full name and e-mail address; Profile; Your account; **Theme**; a line; under a small heading with the instance name, their personal space, **Tasks** and Preferences; a line; Log out. The first entry has the focus when the menu opens.
- **NAV-097** · new · **Theme** in the account menu opens a sub-menu with light, dark and system, with a tick on the current choice. A choice applies at once and is the same preference as NAV-074, so it follows the person to other devices.
- **NAV-098** · new · The look of the account menu: the identity block is an inset tinted panel with a large picture, the name in large bold text and the e-mail address muted under it. Entries outside a headed group have an icon; entries under a group heading have none and line up with the heading's text. Group headings are small, bold and muted, thin lines separate the groups, and an entry with a sub-menu has an arrow at its row end.
- **NAV-099** · later · When an administrator lists other tools of the organisation (NAV-100), a button between the sidebar button and the logo opens that list, and each entry opens its tool in a new tab. Without entries the button is absent. An entry shows its name and an icon of ours or one uploaded and checked like any image, never one fetched from the tool (Where we deliberately differ 5).
- **NAV-100** · later · An administrator keeps the list of other tools in the administration, under a line that says where the list appears, as a table with the columns name, address, hide, shown only to groups, and actions. An add row comes first: an empty name field, a wider address field, the hide checkbox, a group picker that finds groups by name, and a small primary Add. Each existing row shows a drag handle, the name, the full address as plain text, the hide checkbox, the group picker and a settings button.
- **NAV-101** · later · Rules of the list of other tools: an address must be http or https (EDT-053); the entry for the instance itself is always there and cannot be removed; entries keep the order the administrator gives, by dragging or by Move up and Move down; a hidden entry shows to nobody; showing an entry to some groups only decides who sees the link, never who may use the tool. The same list shows at the foot of the sidebar (section 1.10). A row change applies at once, as in every such list (ADM-112); what the row's settings button holds needs its own study.

## 1.3 The start screen

The first screen after login. Its job is to answer "where was I, and what changed?".

- **NAV-026** · built · A greeting with the person's full display name. The first word of a name is never guessed to be a first name.
- **NAV-027** · built · A grid of spaces as cards with a badge, name and description.
- **NAV-028** · built · Their recent pages and favourite pages.
- **NAV-029** · built · The tabs **For you**, **Recent activity** and **Popular** sit above the content.
- **NAV-030** · built · **For you** lists new and changed pages from spaces and pages the person watches, and from people they follow, newest first, in one feed with the author, the action and the time.
- **NAV-031** · built · **Recent activity** lists what happened across all spaces the person can see. A filter narrows it to a space.
- **NAV-032** · built · **Popular** lists pages that got the most reactions and comments in the last 7 days. It counts only what the person may see.
- **NAV-033** · built · Each feed item shows the title, the space, a short excerpt, the author and the time. A reaction and the comment count show when they exist.
- **NAV-034** · built · The feed loads more when the person reaches its end. It never loads everything at once.
- **NAV-035** · built · A side column shows up to six favourite or recently visited spaces and the person's open tasks.
- **NAV-036** · built · A person can dismiss a getting-started card, which explains spaces and pages to a first-time user.
- **NAV-037** · built · A new installation with no spaces shows one clear sentence and the way to create the first space.
- **NAV-038** · built · An administrator can pick a page as the instance's welcome message. It shows above the feed for everyone who can see it.
- **NAV-102** · new · The start screen is one centred column. From the top: a heading with the cards of what the person last worked on or visited (NAV-104); a heading with the choice of feed as a row of buttons (NAV-106); then the feed. A round settings button sits in the top end corner of the main area when it exists (NAV-110) (changes the design standard, decided 2026-11-14).
- **NAV-103** · new · Replaces the arrangement of NAV-026, NAV-027 and NAV-035: the greeting, the grid of space cards and the side column leave the start screen. Favourite spaces are reached from the sidebar (section 1.10) and the Spaces panel, and open tasks from the Tasks entry. The getting-started card (NAV-036) stays at the top of the column, and the welcome message (NAV-038) directly above the heading of the feed (changes the design standard, decided 2026-11-14).
- **NAV-104** · new · Beyond NAV-028, under its own heading, a grid of up to six cards shows what the person last worked on or visited, newest first, three to a row. The whole card opens the item. It shows only what the person may see at that moment (NAV-006), and a draft only to its writer (PAG-013).
- **NAV-105** · new · A card of that grid shows the kind of item as an icon in a small outlined square, the title in bold, cut short with an ellipsis when it is long, a neutral **Draft** mark after the title for unpublished work, the space muted under the title, and, set apart below them, what the person last did with it, such as edited or visited, with a relative time.
- **NAV-106** · new · The three choices of the feed (NAV-029 to NAV-032) are buttons shaped as pills in one row under the feed's heading, each with an icon and words. The chosen one is marked, also for screen readers as pressed. They take the place of the tabs of NAV-029 (changes the design standard, decided 2026-11-14).
- **NAV-107** · new · Beyond NAV-033, a feed item is a card about one page. Its header line shows the person's picture and name, what they did (such as commented or updated) and when, and at its end a control that shows the recent actions on that page. Under it: the kind icon and the title as a link; the page owner (PAG-051) as a link to their profile, then the space; the first two lines of the page as plain words without formatting, ending in an ellipsis; and a row with a button to add a reaction at its start and the number of comments with its icon at its end. The reaction button shows no count while nobody has reacted, and follows the switches of COL-027. What the recent-actions control opens needs its own study.
- **NAV-108** · new · When the event is a comment, the card also shows that comment under a line: its author's picture and name, its text and its own reaction button indented under the name, and a link at the bottom end that opens the comment in the page. The excerpt and the comment show only to people who may see the page, checked when shown (NAV-006, Where we deliberately differ 8).
- **NAV-109** · new · The look of a feed card: a thin border; in the header the name in the darker text, the event word, a dot and the time muted, and the recent-actions control muted at the end; a line under the header; everything below starts where the header's text starts; the title is the largest text of the column; the owner is underlined and followed by a dot and the space. An older event shows a date instead of a relative time (section 1.9), and a title the person has visited shows, with its icon, in the visited link colour (section 1.13).
- **NAV-110** · later · A round settings button in the top end corner of the start screen lets a person choose what the start screen shows, such as which parts appear and which feed opens first. The choice is stored with the account (ACC-026). What it offers needs its own study.
- **NAV-111** · later · An administrator can make one space the landing space of the instance. When it is made, and again when an administrator resets its access to the default, it gets all members as viewers and the administrators of the instance as its space administrators. That is an ordinary grant shown on the space's access screen, never a silent bypass (Where we deliberately differ 2). What the landing space is for and how people reach it needs its own study.

## 1.4 Personal lists

- **NAV-039** · built · Favourites: a person marks a page with the star at the top of the page. A second press removes it.
- **NAV-040** · built · A person can also mark a space as a favourite, on the space card and in the sidebar.
- **NAV-041** · built · A full **Favourites** screen lists pages and spaces, can be filtered by title, and lets a person remove several at once.
- **NAV-042** · built · Recent pages: the last pages a person viewed are remembered per person. This is a convenience list, not statistics.
- **NAV-043** · built · A full **Recent** screen with the tabs Visited, Worked on and Drafts, and a filter on the title. The filter looks at titles only, and says so.
- **NAV-044** · built · **Worked on** lists pages the person created, published or commented on, newest first.
- **NAV-045** · built · **Drafts** lists the person's unpublished pages and pages with unsaved changes.
- **NAV-046** · built · **Tasks** lists tasks assigned to the person from every page they may see, with a filter on open or done, and on due date (chapter 7).
- **NAV-047** · built · A person can reorder their favourites by dragging.

## 1.5 The spaces directory

- **NAV-048** · built · A screen lists all spaces the person may see.
- **NAV-049** · built · It can be filtered by title and by key, and by the kinds **All**, **Favourites**, **Team**, **Personal** and **Archived**.
- **NAV-050** · built · It can be sorted by name, by last change and by number of pages.
- **NAV-051** · built · Each row shows the badge, name, key, description, its administrators and the time of the last change.
- **NAV-052** · built · Space categories: an administrator of a space adds one or more categories, and the directory can filter on them.
- **NAV-053** · built · A person with *create spaces* sees a button to create a space here.
- **NAV-054** · built · Archived spaces only appear under the Archived filter.

## 1.6 The people directory

- **NAV-055** · built · People with *view profiles* can search for people by name.
- **NAV-056** · built · The directory lists people with their picture, name, job title and department, and can be filtered by group.
- **NAV-057** · built · Selecting a person opens their profile (ACC-020).
- **NAV-058** · partly · Hovering over or focusing a name anywhere in the app shows a small card with picture, name, job title and links to their profile and pages. It can be turned off in the preferences. Still open: the card on the byline, in history and in comments, which have no id yet.
- **NAV-059** · built · Guests see only people they share a space with, unless they have *view profiles*.
- **NAV-060** · built · Disabled people stay visible as authors of what they wrote, marked as no longer active. They do not appear in pickers.

## 1.7 Keyboard shortcuts outside the editor

Shortcuts are single keys, only when the focus is not in a text field. A person can turn them off in their preferences, because they can conflict with assistive technology (WCAG 2.1.4).

- **NAV-061** · built · `/` moves to the search box. `?` shows all shortcuts.
- **NAV-062** · built · `g` then `h` goes home, `g` then `s` opens the spaces directory, `g` then `n` opens notifications.
- **NAV-063** · built · `c` creates a page, `e` edits the current page, `s` toggles the favourite, `w` toggles watching.
- **NAV-064** · built · `[` shows or hides the sidebar.
- **NAV-065** · built · `m` jumps to the comment box, `n` and `p` go to the next and previous unread comment.
- **NAV-066** · built · In the page tree, the arrow keys move between rows, right opens a branch, left closes it, and Enter opens the page.
- **NAV-067** · built · The shortcuts overview lists all shortcuts by group, is searchable, and shows the keys for the person's system.
- **NAV-112** · new · A menu entry that has a keyboard shortcut shows its keys at the end of its row, and the tooltip of a button with a shortcut gives its name and then its keys, on one line. The keys follow the person's system, look the same everywhere, one small key cap per key, are also named to screen readers, and are left out when the person has turned shortcuts off.

## 1.8 Help, about and first use

- **NAV-068** · built · The help menu links to documentation that the administrator can point to their own address.
- **NAV-069** · built · An About dialog shows the version, the licence and a link to the third-party licences.
- **NAV-070** · built · A new user sees a short tour of three steps at most, which can be skipped and never returns.
- **NAV-071** · built · A new space starts with a useful home page, with sections to fill in, so it is never a blank screen.
- **NAV-083** · built · A new instance gets an example space shortly after its first administrator. It shows everything a page can hold, as a first introduction, and all members can edit it.
- **NAV-084** · built · The example space is made once. An instance that already has spaces gets none by itself, and a deleted example does not come back.
- **NAV-085** · built · An administrator can add the example space later, in Administration, Settings.
- **NAV-086** · built · A setting starts a new instance empty.
- **NAV-087** · built · The example always shows every block and mark of the document format: a test fails when it does not. Waves C, D and E add comments, labels, a blog post, templates and dynamic blocks to it.
- **NAV-088** · built · A field for people shows them as soon as it has focus, and works by keyboard. Without *view profiles* it shows none.
- **NAV-113** · new · Beyond NAV-068, every settings section, in a space and in the administration, starts under its heading with one or two short sentences that say what the section is for, in lines shorter than the column, ending in a link to its part of the documentation. The link follows the documentation address the administrator set, opens in a new tab, is marked as leaving the instance (section 1.13), and nothing is fetched to show it (SEC-900). No address of another product or of a community is built in. On an empty section the sentences and the link sit inside the empty block (NAV-173).
- **NAV-114** · new · A help link sits in one of four places: at the end of the description of a screen or section, inside the text of a section, on its own last line inside a note, or in a closing part of the screen with short help blocks, each a small heading, one sentence and one link. Which screens get a link, and how the documentation keeps a stable anchor for each part, needs its own study.
- **NAV-115** · new · Help can open in a panel at the far side of the screen, in the place that Details and Comments use, one of the three at a time. The panel is about a fifth of the window wide, runs from under the top bar to the bottom, and the screen beside it narrows. Its head strip, on a quiet ground, has Back at the start, the title muted in the middle and Close at the end; below it a short article scrolls on its own: a large heading with a mark that opens the full article in a new tab, paragraphs, sub-headings, and lists of examples in fixed-width type.
- **NAV-116** · new · The help panel shows help that ships with the instance, so nothing is loaded from outside (Where we deliberately differ 5). Its mark for the full article opens the same part at the documentation address of NAV-068.
- **NAV-117** · later · The account menu can offer **Send feedback**, without an icon, under a small heading of its own between two lines after Theme. It opens an address the administrator sets, such as the organisation's help desk; Diffluence itself sends nothing (ADR-029). Without an address the entry is absent.
- **NAV-118** · new · A person can contact the administrators of the instance on a page reached from a link on an error screen and from the help menu. The page shows the administrators' message and, when it is switched on, a contact form. Reaching it never tells whether a page exists (Where we deliberately differ 9).
- **NAV-119** · new · An administrator writes that message in the general settings of the administration, in a box of several lines with simple formatting, stored in our document format and never as markup or HTML (EDT-152, PAG-122), with hints under it on what the message is for and on what an empty box does. When the box is empty a standard message shows, which asks what the person needs and, for an error, what they were doing and when.
- **NAV-120** · new · Under the message box, a checkbox, on by default, switches the contact form on, with a hint that the administrators of the instance receive what is sent. The form can only be switched off when an own message exists, so people always learn how to get help. A message goes by mail through the queue, and only when mail is set up (COL-079). A limit against floods needs its own study.
- **NAV-121** · new · Beyond NAV-088, a field for people or groups finds them by the start of any word of their name, without the person typing a wildcard. Who appears is still limited by NAV-059 and ACC-036.

## 1.9 Narrow screens, dark mode, languages, accessibility

- **NAV-072** · built · Below 50rem the top bar wraps, sticky bars let go, and the sidebar starts hidden.
- **NAV-073** · built · Dark mode follows the operating system.
- **NAV-074** · built · A person can choose light, dark or system in their preferences. The choice follows them to other devices.
- **NAV-075** · built · All layout works right-to-left. No screen has a separate right-to-left style.
- **NAV-076** · built · All words come from translation files. English ships first.
- **NAV-077** · built · A person picks their language, time zone, date format and first day of the week. Dates and times everywhere follow these.
- **NAV-078** · built · Long dates show as relative time ("3 hours ago") with the exact time on hover or focus.
- **NAV-079** · built · WCAG 2.2 AA is the goal, checked by automated tests on screens in the browser tests.
- **NAV-080** · built · Screen reader users hear new notifications and save states through live regions, without being interrupted.
- **NAV-081** · built · Motion stops when the system asks for reduced motion. The tree and panels still work.
- **NAV-082** · built · A sensible browser print of any page: no bars, sidebars or buttons, and every collapsible section open.
- **NAV-122** · new · Beyond NAV-078, times of the last few days show as relative time, such as four hours ago, and older times as a date in the person's own date format (NAV-077), such as the day, the full month name and the year. Both show the exact time on hover or focus. Where the change from relative time to a date lies needs its own study.
- **NAV-123** · new · In the tables of the app, a moment of recent activity, such as a last login, a last use or a last change, shows as relative time with the exact time on hover and focus (NAV-122), and a fixed date, such as an end date or a creation date, shows as the date itself in the person's date format. A row without a date, such as a person who was never seen, shows a muted word instead.
- **NAV-124** · later · A language can come in regional variants where spelling or words differ, such as British and American English. The language list names each variant in its own language, and a word a variant lacks falls back to the main language. Date format and first day of the week stay preferences of their own (NAV-077).

## 1.10 The global part of the sidebar

The upper part of the sidebar holds a person's own places, the same on every screen outside the settings, and the foot holds links that leave the wiki.

- **NAV-125** · new · On the Personal, Space and Focus frames the sidebar starts with a global part, in this order: the start screen, Recent, Favourites, Spaces, Apps when apps are installed, Drafts, Tasks and Templates, then, after a gap, the entry that customises the sidebar (section 1.12). The settings of a space and the administration keep their own sidebar of sections, without the global part (changes the design standard, decided 2026-11-14).
- **NAV-126** · new · Each entry of the global part is one row with an icon and its name. The start screen entry goes to the start screen, as Home does (NAV-014). Recent, Favourites and Spaces carry an arrow at the row end and open a panel beside the sidebar (section 1.11). Drafts opens the list of NAV-045 as a screen of its own, Tasks the list of NAV-046, and Templates the templates a person may use (chapter 8).
- **NAV-127** · new · What follows the global part depends on the screen. Inside a space, a line across the sidebar and then the block of the current space (chapter 2). On the start screen and the other personal screens, after a gap, the person's favourite spaces under a small heading, each a row with its badge and name, ending in an entry that opens the spaces directory (NAV-048).
- **NAV-128** · new · The whole sidebar scrolls as one, so in a long sidebar the global part scrolls out of view with the rest; no part of it stays fixed.
- **NAV-129** · later · When apps are installed (PLT-039), the global part has an **Apps** entry. It unfolds in place into the apps that give people a screen of their own, under a small heading, each a row with its icon and name, and ends with an entry to find more apps for people who may install them. While it is unfolded, a More button on the entry offers administrators the ways to find and to manage apps. An app's icon is a file of the app, never fetched from outside (Where we deliberately differ 5). The entry has no arrow, because it opens no panel, and it is absent while the person may use no app (NAV-004).
- **NAV-130** · later · At the foot of the sidebar, the links to other tools of NAV-100 show as rows: the tool's icon on a small rounded tile, its name, and at the row end a mark that it opens elsewhere, in a new tab with `noopener` (EDT-061). Inside a space they come after the block of the space, below a line; on the personal screens they follow the favourite spaces after a gap. A person can hide and reorder them for themselves (section 1.12).

## 1.11 Panels beside the sidebar

Recent, Favourites and Spaces open a panel beside the sidebar instead of a menu, so a person chooses where to go without leaving the screen.

- **NAV-131** · new · The panel is a card with rounded corners and a shadow, beside the sidebar, over the start of the main area, about a quarter of the window wide and reaching about two thirds down; nothing behind it is dimmed. At its top a title and a Close button, then a filter field that has the focus at once, then groups under small muted headings, and at the bottom, under a line, its actions, which have the weight of rows rather than of buttons (changes the design standard, decided 2026-11-14).
- **NAV-132** · new · While its panel is open, the entry that opened it has a neutral tint, not the mark of the current screen. Escape and Close shut the panel and return the focus to that entry, and choosing a row goes there and shuts it.
- **NAV-133** · new · Beyond NAV-015, the **Spaces** panel lists, each group under a small heading: the current space, the favourite spaces and the recently visited spaces, in that order, without archived spaces (SPC-069). The current space shows again among the favourites when it is one. Every row shows the badge and name of the space and a star at its end: filled on a favourite, where it removes the favourite, and on other rows shown on hover or focus, where it adds one. The filter narrows the rows as the person types. How many recent spaces it lists, and whether the filter also finds other spaces, needs its own study.
- **NAV-134** · new · Under a line, the Spaces panel ends with three actions, each with an icon: **All spaces**, which opens the spaces directory (NAV-048); **Create a space**, for people with *create spaces* (SPC-001); and **Import**, for people who may import, which until phase 11 offers our own formats (EXC-020, EXC-027, EXC-037; ADR-028).
- **NAV-135** · new · The Spaces panel takes over the job of the strip of spaces (SPC-085, SPC-086), which leaves the sidebar. Favourites come before recent spaces, as SPC-085 orders them (changes the design standard, decided 2026-11-14).
- **NAV-136** · new · The **Recent** and **Favourites** panels have the same form and hold what the menus of NAV-016 and NAV-017 hold: Recent the pages the person visited and worked on, and their drafts; Favourites their favourite pages and spaces. Each ends with an action that opens its full screen (NAV-041, NAV-043). How their groups are headed needs its own study.

## 1.12 Customising one's own sidebar

A person chooses which entries of the global part show in their own sidebar, and in which order.

- **NAV-137** · new · The last entry of the global part, set apart by a gap, opens a dialog in which a person chooses which entries of their own sidebar show and in which order. The choice touches nobody else, is stored with the account (ACC-026) and takes effect when they save; Cancel leaves everything as it was.
- **NAV-138** · new · The dialog has a title and a Close button, two short explanations (hidden entries stay reachable under More; the change touches only this person), then, under a heading, one row for each entry of the global part (NAV-125). A row holds a drag handle, a checkbox, the icon, the name and a More button at its end. The favourite spaces, the entry to all spaces and the customise entry itself have no row. At the bottom end come Cancel as quiet words and Save changes as the primary button.
- **NAV-139** · new · The start screen entry is always shown and always first: its row is ticked and drawn pale as locked (section 1.14), with no handle and no More button.
- **NAV-140** · new · Dragging a row by its handle moves it, and the More button of a row offers Move up and Move down, so the order also changes by keyboard.
- **NAV-141** · new · An unticked entry leaves the global part and moves under a **More** entry, which then takes the place of the customise entry at the end of the global part. More opens a list of the hidden entries that ends with the customise entry, so nothing becomes unreachable.
- **NAV-142** · new · **Watching**, the screen of what a person watches (COL-058), is also an entry of the global part, after Templates. It starts unticked, so it sits under More until the person ticks it.
- **NAV-143** · later · A second list in the dialog, under its own heading, holds the links to other tools (NAV-130), each with the tool's icon and with the same handle, checkbox and More button, so a person can hide or reorder them for themselves.

## 1.13 How states, menus, messages and marks look

One look for each state and each kind of message, the same on every screen, so a person learns it once.

- **NAV-144** · new · The current item, such as the open page in the tree, the current entry of a sidebar or the current section of the settings, has accent text and icon in normal weight, the accent tint across the whole row with rounded corners, and a short accent bar on its start edge inside the tint, which mirrors in right-to-left. It is marked as current for screen readers, and the group that holds it gets no mark of its own (changes the design standard, decided 2026-11-14).
- **NAV-145** · new · Hovering a row of a sidebar, list or menu gives the whole row a neutral tint of the same shape as the current item's, with no bar, and leaves its text as it is. Keyboard focus is an outline with rounded corners around the row or button, with any bar inside it, in the accent colour on entries and buttons and in the text colour on the headers of folding groups, and it also shows on an entry just chosen until the focus moves on (NAV-003).
- **NAV-146** · new · A menu item that switches an option on or off has the look of NAV-144 while on, and also a tick at its start; while off, that place stays empty, so the state never rests on colour alone.
- **NAV-147** · new · A menu is a card with rounded corners and a soft shadow, opening below its button toward the end side, and it may overlap the edge of the sidebar. Its items are tall rows with the icon in one aligned column and the words in a second (the headed groups of the account menu go without icons, NAV-098). Lines split the items into blocks, and a block may open with a small bold heading, which screen readers hear as the name of the group. A destructive item comes last, its icon and words in the danger colour; an item with a sub-menu has an arrow at its row end; the focused item has an outline across the row.
- **NAV-148** · new · A short message after an action floats in the bottom start corner of the window, over the sidebar, about as wide as the sidebar and a little above the lower edge, mirrored in right-to-left. It is a card with a soft shadow: a mark of its kind and a bold heading on the first line, with Close at the end; the sentence under it; and its action, such as Copy link, on a line of its own. Screen readers hear it through a live region (NAV-080).
- **NAV-149** · new · A note in the flow of a screen is a tinted block with rounded corners across the content column, without a border. Its icon stands in a column of its own, level with the first line; an optional bold title, then the text, start after the icon; under the text may come a row of actions, a primary button first and a link after it, and a guide link as the last line. A note without a title sets its first sentence in bold. Links in a note are underlined.
- **NAV-150** · new · Notes come in three kinds, information, warning and error, each with its own icon and tint, and a warning always has a bold title, so the kind never rests on colour alone. A note about a setting or a lasting cause has no Close button and stays until its cause is gone; a note of news, such as what an upgrade changed, is dismissed by a text link under it and stays dismissed for that person on every device (ACC-026).
- **NAV-151** · new · Where a control is absent and the reason helps the person (NAV-004), an information note stands in its place, as wide as the control would have been, saying why, with a guide link last where there is one.
- **NAV-152** · new · An explanation button, a small round information mark, may follow a label, a heading, a column header, the title of a tile or a value where a short explanation helps. It has one look everywhere and a name that says what it explains. It opens a small tip on hover, focus or press; the tip can itself be hovered, stays until dismissed, and closes with Escape (WCAG 1.4.13).
- **NAV-153** · new · A status in a list of the app is a small tinted lozenge with a thin border and a word, in a role: success (such as active or verified), neutral and outlined (such as not invited), bold for a fixed kind (such as primary or system), a tint of its own for **New** (NAV-178), and outlined with a number for a count. The words are in sentence case. A value that is not a state, such as who manages something, is plain text.
- **NAV-178** · new · The **New** mark follows the name of something that came with the running release, wherever it is offered, such as an entry of the slash menu and the insert dialog (EDT-188) or a section in the sidebar of the administration (ADM-067). It is a short word in a tinted box with an outline, so it never rests on colour, and keeps its look in a selected row. It stays during the release that brought the thing, also after it was opened, and goes with the next feature release; it is the same for everyone, and the release the binary knows decides it, so nothing asks an outside server (OPS-106) (decided on the product owner's behalf on 2026-11-15, may be overruled).
- **NAV-154** · new · A group shows as a rounded square tile in a tint chosen from its name, and a person as a round picture, so the two never look alike.
- **NAV-155** · new · A list of what is allowed and what is not starts each line with a round mark, a tick for allowed and a cross for not allowed, which also say so in words to screen readers. A cross for not granted is neutral; only a rule that cannot be changed uses the danger colour.
- **NAV-156** · new · Every list and report shows the kind of each item as an icon before its title (page, home page, blog post, live document, whiteboard, database, folder), and the kind of each file before its name, such as picture, PDF or slides. A page's own emoji (PAG-123) shows where it has one, and colour is never the only sign of a kind (changes the design standard, decided 2026-11-14).
- **NAV-157** · new · A table of the app, in a space, in the administration or in Your account, has a quiet heading row of small bold words with a line under it, rows set apart by space without lines between them, a line after the last row, no cell borders, no outer frame and no grey header band, in place of the line under each row of the design standard's rule for tables. The first column starts a little in from the row's start; the row under the pointer gets a tint; every column has a heading, also the column of a switch or of a More button, at least for screen readers; and a sorted column shows one arrow right after its heading's text, with that heading in the accent colour (changes the design standard, decided 2026-11-14).
- **NAV-158** · new · A link in the text of a screen, such as in a description, a hint or a note, is underlined, so it never rests on colour alone. A link that leaves the instance or opens a new tab also has a small mark after it and says so in its accessible name; a link to another screen of the instance has neither.
- **NAV-159** · new · A link or title the person has already followed, in the feed (NAV-109) and in the text of the app's screens, shows in a visited colour of its own.

## 1.14 Forms and settings screens

How a form is laid out, when a change is saved, and how a control shows that it cannot act.

- **NAV-160** · new · A settings form is one column of reading width, placed in the large area as ADM-078 and SPC-150 say, under the section's name as the largest heading. Labels are small and bold, close above their fields; fields are narrow, never as wide as the column, and of one width, except a short field such as a number, which is as wide as its content needs; they have a thin border and slightly rounded corners, and about one field's height lies between a field and the next label. An explanation button stands just outside its field, a field for a person shows the picture before the name, and facts that cannot be changed, such as who made something and when, come below the fields (changes the design standard, decided 2026-11-14).
- **NAV-161** · new · A switch that changes one setting on its own applies at once, without Save, and so does a chooser that stands on its own outside a form, such as the role in a row of a list (ADM-092) or the mode of a section (PAG-132); a short message says that it was saved, also to screen readers (NAV-080). A screen made only of such controls has no Save button. A choice inside a form waits for the form's Save, and settings that wait for Save use checkboxes, never switches.
- **NAV-162** · new · A settings form with typed fields, or a change that needs several fields together, saves as one unit, in a space, in the administration and in every other settings screen. It ends with **Save** as the primary button and **Reset changes** as a quiet button after it, small and close together at the start side, directly under the unit's fields. Both stay inactive until something in the unit changed, and Save's description then says that nothing has changed yet; Reset changes puts back the stored value of every field of the unit, and a failed save keeps what was typed (NAV-012). Every settings form uses this one name for undoing edits and has no other Cancel or reset (decided on the product owner's behalf on 2026-11-14, may be overruled) (changes the design standard, decided 2026-11-14).
- **NAV-163** · new · A settings screen with groups of fields that are saved apart, such as the language of search beside the general settings, gives each group its own Save and Reset changes under its fields, with a thin rule and extra space between the groups, so the one primary action counts per group. A screen of one group has one Save at its end, and the page ends after it (changes the design standard, decided 2026-11-14).
- **NAV-164** · new · The main action of a list, such as making a new item, sits at the end of the list's heading line, and a second action of the list follows right after it as an outlined button.
- **NAV-165** · new · In every form, a required field has, after its label, a marker and the word for required in muted text in brackets, as TPL-315 does with the word in template forms, and it is announced as required to screen readers; an optional field has no mark. A form with required fields says in one line at its top how they are marked. The marking never rests on colour alone.
- **NAV-166** · new · A switch is a small pill, announced as a switch with its state. On shows a filled track with the knob at its end and a tick at its start; off shows a darker track with the knob at its start and a cross. A single switch in the flow of a section comes before its label; a switch at the end edge of a row takes the row's name as its label, and its label is always its accessible name.
- **NAV-167** · new · A list of switches is one bordered card with rounded corners. Each group has a light, muted heading band across the card; thin lines separate the rows; each row has a small icon in a fixed column, the name in small bold text, an optional hint under the name, and the switch at the row's end, clear of the border. A group without icons starts its names at the icon column (changes the design standard, decided 2026-11-14).
- **NAV-168** · new · The main choice of a section between a few options is a set of cards, each holding a radio and its label: a stack of cards as wide as the column, or cards side by side when each needs a description of its own (ADM-099). The whole card can be clicked, the set is one radio group for the keyboard, and the choice shows in the radio while the cards stay alike. A secondary choice is a tight stack of plain radios.
- **NAV-169** · new · Less used options of a form or section fold away behind one fold control in every screen: words in the text colour, start-aligned with the labels, with a chevron before them that points to the side while shut and down while open.
- **NAV-170** · new · A control whose value is fixed, by the product or by the operator's configuration file, stays in view, drawn pale and locked, keeps its value, and has a short line beside it that says why. NAV-004 still holds for what a person may not do.
- **NAV-171** · new · A button that has nothing to act on yet, such as one that applies a choice while there is nothing to choose, is shown inactive, with the reason in words next to it, such as the sentence of the empty list above it.

## 1.15 Empty and loading states

Beyond NAV-007: where the sentence of an empty or loading part sits, and what comes with it.

- **NAV-172** · new · An empty table keeps its header row, and its muted sentence, which says what will appear there, stands centred in the table's body, about a row below the header, with no picture. An empty list without columns shows its sentence inside the bordered region where the list will be, so the screen keeps its shape.
- **NAV-173** · new · An empty section that the person may fill shows no create button in its heading line while it is empty. Instead one centred block in its upper part holds a short bold heading, one to three short lines that say what will appear there, the primary button that makes the first item, and a guide link close under it; a longer explanation, when one is needed, follows under the block, aligned at the start. A table in such a section keeps its header row above the block. A picture appears only when it is our own drawing (changes the design standard, decided 2026-11-14).
- **NAV-174** · new · A part that must be set up before its first use shows a centred start block: its heading, one or two sentences, the main action with at most one other action beside it, a guide link, and at most two short cards side by side, such as what the part brings and how its data is handled, each a few rows of an icon and two lines of text.
- **NAV-175** · new · An empty result that no button can fill, such as a search that found nothing, shows in the upper third of the area a heading and one or two short paragraphs whose ways forward are links inside the text, then a guide link apart, and no button.
- **NAV-176** · new · An empty part of a form that can be started, such as the header of a PDF layout not yet written, shows a muted bar with an italic invitation in place of the field, and choosing the bar starts it.
- **NAV-177** · new · A part of a screen that is still loading shows, in its own place, a framed box with a bold sentence that says what it waits for and a placeholder in the shape of the result, without a spinner, while the rest of the screen can be used (changes the design standard, decided 2026-11-14).
