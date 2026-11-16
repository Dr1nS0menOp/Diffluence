# 079: The page: header row, byline, More, Share and the notification panel

Status: built
Phase: the screens of ADR-045, second area (wave SC2)

## Problem

Blueprint 03 was completed on 2026-11-14 from a study of another product's screens (ADR-045). Its new lines describe the page of a space differently from how it is built: the header row, the byline, the menu More, the Share dialog and the panel of the bell. Some of those lines reverse built behaviour, and the product owner confirmed on 2026-11-15 that the reversals are wanted: the star and the lock leave the header row, the byline loses its date and version, and Watch becomes a sub-menu (PAG-141 to PAG-144, PAG-151). This spec rebuilds those parts as the lines say, keeps every built function reachable, and records where it had to choose.

The frame (top bar, sidebar, start screen) is wave SC1 and is not touched here. The bell itself sits in the top bar; this spec changes only its panel and the state the bell shows while the panel is open.

## The lines in scope

| Line | Status before | After | What changes |
|---|---|---|---|
| PAG-101 | built | built | The actions are Edit, Share joined to Copy link, and More; the star and the lock leave the row (narrowed by PAG-141). |
| PAG-102 | built | built | Share stays; Watch in More is a sub-menu (PAG-151). |
| PAG-103 | built | built | The byline shows the owner; the date moves to the header row and the version to the history (narrowed by PAG-142, PAG-144). |
| PAG-140 | new | built | The header row: breadcrumbs at the start; at the end, outside the reading column, the publishing state with its date, who else is reading, Edit, Share with Copy link, More. |
| PAG-141 | new | built | Share carries the lock's state; no star and no lock in the row. The favourite is in More, restrictions open from Share. |
| PAG-142 | new | built | The publishing state says in words that the page is published, with the date of the last update in the person's format. |
| PAG-143 | new | built | On the home page of a space the breadcrumbs hold only the space's name. |
| PAG-144 | new | built | The byline: owner's picture and name, view count, add a reaction, the document-control state, the status at the end; no date, no version. |
| PAG-145 | new | partly | Where document control is on, the byline names the page's control state, which opens the review panel for someone who may see it. Still open: saying that no workflow governs a page, which needs the space to say whether document control is on. |
| PAG-146 | new | built | The home page of a personal space has no byline and no publishing state. |
| PAG-148 | new | partly | More in four groups divided by lines. Still open: apps (PLT-041, not built) and report (COL-132, later). |
| PAG-149 | new | built | Archive and Delete share one entry with a sub-menu; no entry of More is in the danger colour. |
| PAG-150 | new | partly | More details opens a sub-menu: Page information (which lists the incoming links) and the files. Still open: incoming links as an entry of their own. |
| PAG-151 | new | built | Watch opens a sub-menu of three choices shown as on or off; the entry's mark is filled while any is on; `w` still toggles the page. |
| PAG-164 | new | built | On the home page of a space a thin line runs under the header row. |
| COL-099 | new | built | The byline offers to add a reaction, while reading, where reactions are on. |
| COL-102 | new | built | The Share dialog: a card near the top over a veil; title row with More and close; the field; a heading over one access row; a line; a footer of actions at the start side. |
| COL-103 | new | built | One field adds people, groups and e-mail addresses; no assistants. |
| COL-104 | new | new | Suggested people. Not built: the line asks for its own study of how suggestions avoid revealing who works with whom. |
| COL-105 | new | built | The access row shows the reach and what people may do, as plain choices that change the restrictions, read-only without *manage restrictions*. |
| COL-106 | new | partly | The footer: Copy link and the single shared page. Still open: sharing to a chat system (COL-048, later). |
| COL-107 | new | built | Without a recipient there is no primary button; once one is named, Send becomes primary. The access list shows only when it has entries. |
| COL-108 | new | partly | More in the title row holds the detailed restrictions. The rest of its contents needs its own study. |
| COL-113 | new | built | The panel hangs under the bell, end edges aligned, with a fixed height and its own scrolling; the bell shows a pressed state with its count hidden. |
| COL-114 | new | built | The panel's header row: the title, then the unread-only switch, a button to the full screen and More. |
| COL-115 | new | built | Two tabs, For me and Watched, in the panel and on the full screen; the switch and Mark all as read act on the open tab. |
| COL-116 | new | built | Items are grouped under a heading per day, newest first. |
| COL-117 | new | partly | The item's arrangement. Still open: the title on its own line (see D9). |
| COL-118 | new | built | The icon of the kind of content, the space, and for a comment or mention the start of the comment, made when shown. |
| COL-119 | new | new | Reacting and replying from the panel. Not built in this wave (D10). |
| COL-120 | new | new | Folding several updates of one person. Not built in this wave (D10). |

Lines of 3.11 that belong to other areas stay as they are: page tools (PAG-147), widths (PAG-152, PAG-153), the pictures of readers per space (PAG-154, PAG-155), density (PAG-156 to PAG-158) and related pages (PAG-159).

## Superseded decisions

Recorded in ADR-047:

- **PAG-101 and NAV-039:** the star at the top of a page becomes the favourite entry in More. `s` still toggles it (NAV-063).
- **ACC-066:** the lock icon moves from its own button onto Share; what it shows is unchanged.
- **PAG-103:** the byline holds no date and no version.
- **PAG-102:** Watch is one entry with a sub-menu, in place of two toggles.
- **NAV-147, for More only:** no entry of More is in the danger colour; the dialog that asks first carries it.
- **The design standard:** "breadcrumbs with Edit, Share, star, lock and More on one line" (A page) and "On the home page the breadcrumbs end in Home" (The page tree), as `docs/design/README.md` already lists under ADR-045.

## Behaviour

### The header row

1. The row is the first row of the page, above the header image, and runs the full width of the area beside the sidebar, so its end is outside the reading column. Breadcrumbs at the start; at the end, in this order: the publishing state, the pictures of who else is reading, Edit, Share with Copy link, More.
2. The publishing state is words and a date: "Published" and the date of the last update for a published page, "Not published" without a date for a page that never was. Its accessible name says both.
3. Share is a button with the lock: closed when the page has restrictions of its own, open when it has none, and its name says which. Copy link is an icon button joined to it, with the same confirmation as before (COL-044).
4. The breadcrumbs of the home page of a space are the space's name alone, as the current place. The home page has a thin line under the row; no other page has.
5. A personal space's home page shows no publishing state and no byline (PAG-146).

### The byline

6. One line of small muted parts: the owner's picture and name (a button that opens Details, as PAG-106), the view count where the person may see it (a link to the page's insights), Add a reaction where reactions are on and the person may react, the document-control state where it is on, the marks Verified and Public, then the status at the end.
7. Add a reaction opens the short list of common reactions and the picker, the same as the reactions at the bottom; a reaction given counts there.
8. The document-control state is the name of the page's state. It is a button that opens the review panel for whoever may open it, and plain words otherwise. It never says that a working version exists.
9. Read time, the version, the count of comments and the Restricted mark leave the byline: read time and versions are in Details (PAG-115), the restriction is on Share, and comments are under the page and in More.

### More

10. Four groups divided by lines, without headings; each row an icon and words, a sub-menu arrow at the end of an entry that opens one. An entry a person cannot use is absent, and a group without entries is absent with its line.
    1. Favourite (on or off), Watch ▸, Present, Reader mode, Print.
    2. History, More details ▸ (Page information, Files), Comments, Approvals, Verify, Ask to edit, Close or open comments, Run a rule ▸.
    3. Rename, Copy, Move, Convert ▸ (folder, page, blog post: those the page allows), Save as template, Export ▸ (each offered format), the release entries of a paired space, Archive or delete ▸ (Archive, Delete).
    4. Report (COL-132): later, so absent.
11. Watch ▸ holds This page, This page and everything below it, and The whole space, each a menu checkbox. The entry's eye is filled while any is on. `w` still toggles the page.
12. A sub-menu opens with a click, Enter, Space or the arrow towards the end side (left in right-to-left), and focuses its first entry. The arrow towards the start side and Escape close it and return to its entry; Tab closes the whole menu. A sub-menu that does not fit beside the menu opens over it, a little lower, so it stays inside a 360-pixel screen.

### Share

13. The dialog is a card near the top of the screen over a veil. The title row holds the title, More and Close. Under it the one field for recipients over the full width; chosen recipients as chips; a message and the choice to notify, shown once someone is named.
14. The field finds people and groups; an e-mail address is offered as an invitation for someone with the role to invite (spec 022). Sending notifies the people and groups and asks an administrator to invite each address, with the message as the note.
15. A small bold heading "General access" over one row: the state's lock on a soft square; the reach in bold as a choice (everyone in the space, or only the people chosen) with a muted sentence that names the space as a link; at the end what they may do, a choice between view and view and edit, shown while the reach is everyone. Together: open, view only for most, private (ACC-064). The choices change the restrictions as the restrictions dialog does, and are plain words without *manage restrictions*.
16. The list of who has access shows only when it has entries; the requests for access show to those who may answer them, as before.
17. A line, then the footer at the start side: Copy link, and the single shared page where the person may make one. Without a recipient there is no primary button; with one, Send is the primary button at the end of the footer.
18. More in the title row holds Restrictions in detail, which opens the restrictions dialog (read-only for those who may not change them), so the view the lock opened stays one step away for everyone.

### The notification panel

19. The panel hangs under the bell, its end edge on the bell's end edge, over the page, with a fixed height and its own scrolling. While open the bell shows a pressed state, hides its number, and says `aria-expanded="true"`.
20. The header row: the title, large and bold; at its end the switch Only unread with its label before it, a bare button that opens all notifications, and a bare More with Mark all as read. Under it the tabs For me and Watched over a thin line; the selected tab has accent words and a bar under them.
21. A notification belongs to Watched when it came from watching a page, branch or space, except a comment or a reaction on the person's own page, which is aimed at them; everything else is For me. The switch and Mark all as read act on the open tab. The full screen has the same tabs.
22. Items are grouped under a small bold heading per day (Today, Yesterday, or the date), newest first, without lines.
23. An item: the actor's picture at the start; then the sentence in bold with the age muted after it; the icon of the kind of content (page, blog post, comment, task) with the space muted; for a comment or a mention the start of the comment in a quiet framed box, two lines at most, made when the panel asks and only from what the person may see then. The unread mark sits at the inner end, level with the first line.

## Decisions on the product owner's behalf (2026-11-15, may be overruled)

- **D1, PAG-146:** the home page of a team space keeps its byline and its publishing state. A team home is shared content: who owns it, its status and its reactions matter to readers, while a personal home is one person's own place. Only the line under the header row (PAG-164) marks a home page.
- **D2, PAG-055 and PAG-133:** six suggested statuses stay the limit. Six is built, spaces may already hold six, and a smaller limit would take a status away from them for no gain; PAG-133's hint states the limit of PAG-055, so it says six.
- **D3, built entries of More that PAG-148 does not list** keep a place in the group they fit (behaviour 10): Present, Reader mode and Print with the favourite; Comments, Approvals, Verify, Ask to edit, closing comments and the manual rules with the history; Rename, Save as template and the release entries with copy and move. Removing them would take away built functions that no line removes. Comments stays until page tools (PAG-147) give it its own button.
- **D4, the single shared page** moves from More to the footer of Share (COL-106), where the line puts it.
- **D5, the publishing state** shows the date of the last update with the word Published, as PAG-142 asks; a page never published says Not published, without a date.
- **D6, the access row** shows what people may do only while the reach is everyone; with the reach set to the people chosen, those people decide alone, as the private state of ACC-064 has always meant.
- **D7, More in Share** holds Restrictions in detail for everyone who may see the page, read-only without the right to change them. That is the view the lock opened before.
- **D8, the header row runs above the header image**, because it is the first row of the page in the study and the image belongs to the document.
- **D9, COL-117:** the title stays inside the sentence, in bold, instead of on a line of its own. Splitting it needs a second sentence for each of about forty kinds of notification; the line with the kind's icon carries the space. The rest of the arrangement is built.
- **D10, COL-119 and COL-120** are not built in this wave. Replying and reacting from the panel is a new way to write a comment, with its own checks, and folding updates needs the server's grouping to change; both are larger than the arrangement this wave rebuilds, and both keep the status new.
- **D11, COL-104** (suggested people) is not built, as the line asks for its own study first.

## Permissions and audit

Nothing new is allowed. Every entry keeps the check it had: the favourite and watching need *view*; the access row changes restrictions only with *manage restrictions*, through the same calls as the restrictions dialog, which audit as before; an invitation from Share is the invitation request of spec 022, with its approval. The panel's excerpt is read through the same rule as the mail excerpt: a thread kept for editors gives nothing to someone who may not edit, and a removed comment gives nothing.

## API

- `GET /api/v1/me/notifications` takes `tab=for_me|watched` and `excerpts=true`. With `excerpts`, an item about a comment carries `excerpt`, the start of the comment as plain text, at most 200 characters. Each item carries `page_kind`.
- `POST /api/v1/me/notifications/read-all` takes `tab=for_me|watched`; without it, everything is marked as before.
- An unknown `tab` is refused with 400.

## Data

No migration.

## Load budget

The panel asks for 20 items as before; with `excerpts` it adds one query for all their comments at once, through the editable pages of the person for threads kept for editors, the same narrowing as `visibleRows`. The tab is a condition on `reason` in the existing query, with a lookup of the person's own pages for comments and reactions. Opening More asks for the export formats (cached) and, as before, the manual rules and the watch state. No change on the path of reading a page.

## Acceptance criteria

- [x] The header row holds the publishing state, Edit, Share with its lock and Copy link, and More, its end outside the reading column, with no star and no lock button; the byline has no date or version; the home page of a space has the space alone in its path and a line under the row (`reading.spec.ts`, "the header row holds the publishing state...").
- [x] The home page of a personal space has no byline and no publishing state (`reading.spec.ts`).
- [x] Watch is a sub-menu of three choices: Enter, the arrows and Escape open, move inside, close and return focus; a choice shows as on; `w` still toggles the page (`reading.spec.ts`, "Watch is a sub-menu...").
- [x] More has at most four groups, nothing in the danger colour, and Archive and Delete in one sub-menu whose Delete dialog carries the danger action (`reading.spec.ts`).
- [x] The favourite stays reachable from More and with `s`, with a short message (`reading.spec.ts`); the restrictions open from Share's More (`access.spec.ts`, `feedback.spec.ts`).
- [x] A sub-menu stays on the screen: beside its entry, on the other side, or over the menu on a phone, mirrored in right-to-left (`the-page.test.ts`; `0-design.spec.ts` at 360 pixels, right to left).
- [x] Share: no Send without a recipient; the access row turns the page private and back, with the closed lock on Share; view only for most; a reader sees the row as words; an address becomes an invitation request only for someone with the role (`together.spec.ts`, `people.spec.ts`; `the-page.test.ts` for the states).
- [x] The panel: For me and Watched, a heading per day, Only unread and Mark all as read per tab, the bell pressed with its number hidden, Escape back to the bell (`together.spec.ts`; `the-page.test.ts` for the days and kinds).
- [x] The server: an unknown tab is refused; the tabs split the list and Mark all as read; a comment on one's own page is For me; the excerpt is given only when asked, never from a thread kept for editors to a reader, never from a removed comment (`internal/app/notification_panel_test.go`), guarded by four mutations.
- [x] axe finds nothing on the page, the sub-menu, Share and the panel.
## How the product owner tests it

1. Open a page. The row says Published and a date, then Edit, Share with a lock, a link button and More. There is no star.
2. Open More: four groups. Open Watch, choose the whole space, open it again: it is ticked and the eye is filled.
3. Open Share. Without a name there is no Send button. Change General access to the people chosen: the lock on Share closes.
4. Open the bell: the bell looks pressed, the panel has For me and Watched. Mark all as read on Watched leaves For me unread.

## Provenance

The lines of blueprint 03 named above, written from the study of ADR-045 under the conditions of `docs/blueprint/02-clean-room-and-licensing.md`, "One exception". The menu keys follow the WAI-ARIA Authoring Practices menu pattern (w3.org/WAI/ARIA/apg/patterns/menubar, consulted 2026-11-15). No picture, inventory or text of the other product was used in this wave.
