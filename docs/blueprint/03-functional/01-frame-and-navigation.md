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
