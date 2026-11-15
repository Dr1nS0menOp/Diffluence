# 03.7: Working together

A wiki becomes useful when people respond to each other. This chapter covers comments, reactions, mentions, tasks, sharing, watching, notifications and editing at the same time.

## 7.1 Page comments

- **COL-001** · built · Below the page is a comment box. Anyone with *add comment* writes a comment with simple formatting: bold, italic, lists, links, code, mentions and images.
- **COL-002** · built · A comment shows the author's picture and name, the time, and whether it was edited.
- **COL-003** · built · A person replies to a comment. Replies form one level below the comment, so long threads stay readable.
- **COL-004** · built · A person edits their own comment. A space administrator can edit any comment. The time of the last edit shows.
- **COL-005** · built · A person with *delete own* deletes their own comment, and with *delete any* any comment. A comment with replies leaves a short "removed" line, so the thread stays readable.
- **COL-006** · built · Deleted comments go to the trash of the space like other content, and can be restored there.
- **COL-007** · built · Every comment has its own address, available from its date. Opening it scrolls to the comment and marks it briefly.
- **COL-008** · built · Sorting is oldest first or newest first, kept per person.
- **COL-009** · built · Commenting can be turned off per space, and per page by an editor.
- **COL-010** · built · Long threads load in parts.
- **COL-011** · built · A draft of a comment is kept in the browser if the person leaves the page.
- **COL-012** · built · Posting a comment makes the author a watcher of the page, unless they turned that off.

## 7.2 Inline comments

- **COL-013** · built · A reader selects text, and a small button offers Comment. The selected words get a soft highlight.
- **COL-014** · built · Selecting an image or a table cell allows the same.
- **COL-015** · built · Selecting a highlight opens the thread beside the text, with replies below it.
- **COL-016** · built · **Resolve** closes a thread: the highlight disappears and the thread moves to the list of resolved comments. Anyone with *edit* can resolve.
- **COL-017** · built · **Reopen** brings it back, and the highlight returns if the text still exists.
- **COL-018** · built · If the commented text was removed, the thread stays in the list with the old words, marked as no longer in the page.
- **COL-019** · built · Comments made while editing are visible to editors only until the page is published.
- **COL-020** · built · A panel beside the page lists all comments of a page. It filters by open, resolved, unread and page comments, sorts by time or by position, and moves between comments with keys.
- **COL-021** · built · On narrow screens a thread opens below the text rather than beside it.
- **COL-022** · built · A person can navigate highlights by keyboard, without a mouse.
- **COL-098** · new · Beyond COL-013, hovering over or focusing a block of a page shows a small comment button in the end margin, level with the block, which starts an inline comment on that block. At rest the margin is empty. The button is reachable by keyboard, and absent for those who may not comment and where commenting is off (COL-009).

## 7.3 Reactions

- **COL-023** · built · A person adds a reaction to a page, blog post or comment, from a short list of emoji and a picker.
- **COL-024** · built · Reactions show as small counters. Selecting one adds or removes the person's own reaction.
- **COL-025** · built · Hovering over a counter shows who reacted.
- **COL-026** · built · The author gets a notification, bundled when many people react.
- **COL-027** · built · Reactions can be turned off for a space, or for the instance.
- **COL-028** · built · Reactions count towards the Popular tab (NAV-032).
- **COL-099** · new · Beyond COL-023, the byline of a page offers to add a reaction, as a mark and words after the view count (PAG-144). It opens the short list of emoji and the picker, and reactions given still show at the bottom of the page (PAG-119). It shows while reading only, not in the editor, and is absent where reactions are off (COL-027).

## 7.4 Mentions

- **COL-029** · built · `@` in a page, comment or task opens a list of people. Typing narrows it. Arrow keys and Enter pick one.
- **COL-030** · built · The person shows as a highlighted name. Selecting it opens their card. If the name changes later, the mention follows.
- **COL-031** · built · A person who is mentioned is notified once per version, not on every later edit.
- **COL-032** · built · The list shows people who can see the page first, and marks people who cannot. Writing a mention of someone who cannot see the page warns the writer, and offers to share it.
- **COL-033** · built · A person cannot find out about a page through a mention unless they may see it: a mention never leaks a title or text in a notification.
- **COL-034** · built · Groups can be mentioned when an administrator allows that, up to a limit of 50 members, to prevent mass messages.
- **COL-035** · built · A person's profile lists pages where they were mentioned.
- **COL-100** · new · Beyond COL-030, a mention shows the person's small picture, or their initials on their tint, before the name, in a soft rounded box without an `@` sign, while reading and in the editor. The picture shows only where the reader may see profiles (NAV-088), otherwise the initials (changes the design standard, decided 2026-11-14).
- **COL-101** · new · A mention of the person who is reading is filled with the accent, with light text, so they find themselves on the page at once, and screen readers hear that the mention is them. Other mentions have a light neutral fill with dark text.

## 7.5 Tasks

- **COL-036** · built · A task in a page can have a person, by mention, and a due date.
- **COL-037** · built · The person is notified when they are assigned, and on the due date.
- **COL-038** · built · **My tasks** lists all tasks assigned to a person, grouped by due date. A person can tick them there, and open the page at the task.
- **COL-039** · built · A person also sees tasks they created for others, in a tab.
- **COL-040** · new · Tasks are searchable, and can be reported on (BLK-038).
- **COL-041** · built · A task on a page the assigned person cannot see is flagged to the writer, as with mentions.

## 7.6 Sharing

- **COL-042** · built · **Share** opens a dialog with the people who have access, a field to add people or groups, and an optional message.
- **COL-043** · partly · Sharing sends a notification with a link. It never gives access by itself. If a person lacks access, the dialog says so, and a person with the right permission can grant it there. Still open: Give access in the dialog. The lock is one step away.
- **COL-044** · built · **Copy link** is one click, beside Share, with a confirmation. It copies the stable address.
- **COL-045** · built · The dialog has a checkbox to send a notification or not.
- **COL-046** · built · Share also shows the page's restrictions and lets those who may change them, so one place covers "who can see this".
- **COL-047** · built · A person with the right role can share by e-mail to someone with no account. An administrator approves every invitation before it is sent. Nobody else can invite. The recipient gets an invitation, not the content. See spec 022.
- **COL-048** · later · Sharing to a chat system goes through webhooks or an app (chapter 12).
- **COL-102** · new · **The Share dialog** is a card near the top of the screen, centred over a veil. From the top it holds: a title row with the title at its start and More and close at its end; the field to add recipients over the full width; suggestions right under it; a small bold heading over one access row, which says who else can reach the page; a line over the full width; and a footer of actions at its start side (changes the design standard, decided 2026-11-14).
- **COL-103** · new · Beyond COL-042, Share has one field that adds people, groups and e-mail addresses (COL-047). It offers no assistants, because an assistant acts as the person (PLT-047).
- **COL-104** · new · Under the field, a row of a few suggested people the person often works with, each a chip as wide as the name with a plus, adds one of them with one click. It suggests only people the person may see (NAV-059), and how the suggestions are chosen must not reveal who works with whom (needs its own study).
- **COL-105** · new · The access row of Share shows who can reach the page without being named: a mark of the state on a soft square; the reach in bold as a choice, everyone in the space or only the people chosen, with a muted sentence under it that names the space as a link; and at the far end what they may do, view or view and edit. Together these give the three states of ACC-064. The choices are plain words with an arrow, not boxed fields; they change the page's restrictions (COL-046), and are read-only without *manage restrictions*.
- **COL-106** · new · The footer of Share holds, each as an icon and words: Copy link (COL-044); the single shared page where the space and the instance allow it (EXC-044), never a public link into the instance (ADR-010); and sharing to a chat system when one is connected (COL-048).
- **COL-107** · new · While nobody is named, Share has no primary button, and the list of people with access of COL-042 shows only when there are any. Once a recipient is in the field, the action that sends becomes the primary button (needs its own study).
- **COL-108** · new · A More button in the title row of Share holds its rarely used options (needs its own study).

## 7.7 Asking for access

- **COL-049** · built · A person who opens a link to a page they may not see gets "not found", as the rule demands. An administrator can allow a request button there for spaces where the existence of pages is not secret.
- **COL-050** · built · A person who can read but not edit can ask for edit access from the page.
- **COL-051** · built · A request goes to the page owner, then the creator, then the last editor, and finally the space administrators, until someone is found who may grant it.
- **COL-052** · built · The person asked sees the request in their notifications and in Share, and can allow or refuse it.
- **COL-053** · built · Allowing adds the person to the page's access list and notifies them. Refusing closes the request. Every step is audited.
- **COL-109** · later · An administrator decides for the whole instance whether spaces may offer access requests, in two parts, each with its own switch: requests to enter a space, and requests to see a single page (COL-049). A space offers requests only where the instance allows it, and a space that does not offer them keeps answering "not found" (decision 9).
- **COL-110** · later · A person may ask to enter a space that offers it. The request goes to the space's administrators (COL-051), reveals only that the space exists, and the answer never shows a name to someone who could not see it (PRM-088).
- **COL-111** · later · The section for access requests in the administration has one line of description with a help link, then two tabs, for spaces and for single pages. In the selected tab a card holds one row: an icon tile, a bold name, a muted line on what it allows, and a switch at the row's end that applies at once, without Save.
- **COL-112** · later · Under the card, a sub-heading and one line introduce a preview of what a person without access sees, in a bordered box marked as a preview in its top start corner: centred, a picture of our own or none, a short heading, one or two lines, and a primary button to ask for access with a second button to the start screen beside it. The buttons do nothing there, the text cannot be changed on this screen, and with requests off the preview shows the plain not-found screen (decision 9) without a request button.

## 7.8 Watching

- **COL-054** · built · A person watches a page. They get a notification for new versions, comments and files.
- **COL-055** · built · A person watches a whole space, which covers all its pages and blog posts.
- **COL-056** · built · A person also watches a branch: a page and everything below it.
- **COL-057** · built · Creating or editing a page starts watching it by default. A preference turns that off.
- **COL-058** · built · A list in the preferences shows everything a person watches, with one click to stop.
- **COL-059** · built · A space administrator can see who watches a page, and add or remove people. A person who was removed may watch again.
- **COL-060** · built · A person who loses access stops getting notifications at once, also when they are still listed.
- **COL-061** · built · **Silent update** (PAG-025) sends no notification, and the history still shows the version.
- **COL-062** · built · Changes made by a dynamic block do not count as a change. A dynamic block is worked out when a page is read, and is never stored, so it cannot make a version.

## 7.9 Notifications in the app

- **COL-063** · built · The bell shows the number of unread notifications, and opens a panel.
- **COL-064** · partly · A notification appears for a mention, a task, a share, a reply, a comment or new version on something watched, a reaction, an access request and a review reminder. Still open: review reminders (wave D).
- **COL-065** · built · A notification has the person's picture, a sentence, the page, the time, and a link to the exact spot.
- **COL-066** · built · Related notifications are grouped: "3 people commented on Onboarding".
- **COL-067** · built · The panel has an **unread only** switch, **Mark all as read**, and each item can be marked read or unread.
- **COL-068** · built · A full screen lists older notifications, filtered by type and space.
- **COL-069** · built · Notifications stay for 90 days.
- **COL-070** · built · New notifications appear without a reload, and the count updates live.
- **COL-071** · built · A person never gets a notification about their own action, unless they ask for it.
- **COL-072** · built · A notification is checked against the rule when it is shown. If the person lost access, it disappears.
- **COL-113** · new · **The notification panel** is a card hanging under the bell, its end edge on the bell's end edge, lying over the page with a fixed height and its own scrolling; help and settings beyond the bell stay uncovered. While it is open the bell shows a pressed state with its count hidden, and tells screen readers that it is expanded (changes the design standard, decided 2026-11-14).
- **COL-114** · new · The header row of the panel has the title, large and bold, at its start, and at its end, centred on the title: the unread-only switch with its label before it (COL-067), a bare button that opens the full screen (COL-068), and a bare More button. The tabs of COL-115 sit under it, over a thin line.
- **COL-115** · new · Beyond COL-064, the panel and the full screen have two tabs: **For me**, with what is aimed at the person (mentions, tasks, replies, shares, access requests, and reactions and comments on their own pages), and **Watched**, with what comes from the pages, branches and spaces they watch. The reason of each notification decides its tab, the unread-only switch and Mark all as read act on the open tab, and the selected tab has accent words with a bar under them.
- **COL-116** · new · In the panel, items are grouped under a small bold heading per day, newest first, and parted by space only, without lines.
- **COL-117** · new · An item of the panel has a column of pictures at its start; then, on one inner edge with a hanging indent: the actor and the action in bold with the age muted after them; the icon of the kind of content with the title; a muted line with the space; the excerpt of COL-118; the row of COL-119; and folded updates (COL-120) indented under it. The unread mark sits in one column at the inner end, level with the first line, and status names are plain words, not coloured marks.
- **COL-118** · new · Beyond COL-065, a notification shows a small icon for the kind of content (page, blog post, comment, task) before the title and the space under the title, and for a comment or a mention the first two lines of the comment in a quiet framed box over the width of the text, cut with an ellipsis. The excerpt is made when the notification is shown, from what the person may see at that moment, so lost access shows nothing (COL-072).
- **COL-119** · new · Under the excerpt of a comment, a row of small framed buttons offers the common reactions of the design standard and the picker, then a quiet Reply, without leaving the panel. A reply opens a small comment box in place and joins the thread as a normal reply (COL-003), a reaction counts on the comment itself, and a link at the far end of the row, in the accent, opens the whole thread at its place. The checks of the page apply at the moment of sending, and an item without a comment has no such row.
- **COL-120** · new · Beyond COL-066, when one person makes several changes to the same page, comment thread or task, the later notifications fold under the first as one line, indented and in the accent, with that person's small picture, naming them and the number of further updates. Selecting the line shows them in place.

## 7.10 E-mail

- **COL-073** · built · Mentions, tasks, shares and access requests are sent at once by e-mail, when the person wants that.
- **COL-074** · built · Other changes arrive as a **daily digest**, or a weekly digest. A person can choose single e-mails instead.
- **COL-075** · built · A digest groups changes by space, and can include comments, reactions, new pages, changed pages and blog posts. It is left out when there is nothing to tell.
- **COL-076** · built · An e-mail shows a short excerpt only when the instance allows content in e-mail. Otherwise it shows the title and a link, for instances with sensitive content.
- **COL-077** · built · Every e-mail has a one-click unsubscribe, a link to preferences, and a link to stop watching this page.
- **COL-078** · partly · An administrator can turn e-mail off for the instance, or set the defaults for new users. Still open: the defaults.
- **COL-079** · built · E-mail is sent in the background through the queue. A failing mail server never slows the app, and the admin console shows failures.
- **COL-080** · new · Mails are in the language of the person.
- **COL-081** · later · Push notifications for mobile apps (phase 10), and an RSS or Atom feed per space.
- **COL-121** · new · Beyond COL-078, an administrator turns each kind of e-mail on or off for the whole instance, each with one line that says what it sends. A kind that is off disappears from every person's notification preferences (COL-082), so nobody can turn it on for themselves, and the setting says so.
- **COL-122** · new · The notification settings of the instance have one section per channel, e-mail first and then each connected chat system (7.14), separated by lines. Each has a heading, the same short explanation that these settings hold for everyone and override personal choices, a checkbox per kind of message with a muted hint under it, and its own Save and Reset changes (NAV-162, NAV-163).
- **COL-123** · later · Once push notifications exist (COL-081), an administrator can turn them off for the whole instance, as a checkbox with a hint. Then nobody receives them, and the personal choice is not offered (NAV-004).
- **COL-124** · later · A person can get a weekly e-mail with popular pages from the spaces they can see and from people they follow, chosen from counts that carry no names, such as reactions and comments (NAV-032), never from what anyone read (decision 7). It names only pages the person may see when it is sent, shows excerpts only as COL-076 allows, and an administrator can turn the kind off (COL-121).
- **COL-125** · later · A person can get a weekly summary of the places where they were mentioned in the past week, with a link to each. Places they can no longer see are left out (COL-033).
- **COL-126** · later · Beyond COL-081, each space offers feeds of new and changed items, one per kind: pages, blog posts, comments, files, and one with everything. A section of space settings lists them under one line that says what feeds are for and that a feed reader is needed, each with a button that copies its address.
- **COL-127** · later · A feed address carries a personal key that the person can revoke, as a personal token (ACC-019), so no feed works without a person (deliberate difference 1). A feed holds only what that person may see at the moment it is read, and item text only where the instance allows text in messages (ADM-027); otherwise titles and links.
- **COL-128** · later · One feed holds at most a set number of items, 200 by default, and building one stops after a set time, 60 seconds by default, delivering what was ready by then.
- **COL-129** · later · A space can keep mail sent to an address of its own as a read-only archive that its members can search and follow. Mail and its attachments are never trusted (FIL-019), and receiving mail needs an inbound mail setting that does not exist today (needs its own study).

## 7.11 Notification preferences

- **COL-082** · built · A person sets per type of event: in the app, by e-mail at once, in the digest, or not at all.
- **COL-083** · built · A person sets the digest's day and time, and quiet hours.
- **COL-084** · built · A person can pause all e-mail for a period, for holidays.
- **COL-085** · built · Automatic watching can be turned off for pages a person creates, edits or comments on, separately.

## 7.12 Following people

- **COL-086** · built · A person can follow another person. Their pages and blog posts then appear in the For you feed.
- **COL-087** · built · A profile lists followers and who a person follows, and a person can turn this off for themselves.

## 7.13 Editing at the same time

- **COL-088** · built · Several people edit the same page at once, and see each other's changes and cursors with names.
- **COL-089** · built · Each person sees the others' pictures in the strip. A person who left turns grey.
- **COL-090** · built · Changes merge by themselves, with no conflicts and no lost text, also when two people type in the same line.
- **COL-091** · built · A person who is offline for a moment keeps typing. Their changes merge when the connection returns.
- **COL-092** · built · A draft is then shared by everyone who edits the page. Publishing publishes the shared draft, and the version names all who took part.
- **COL-093** · built · Clicking a colleague's picture jumps to their cursor.
- **COL-094** · built · The editor invites people from the strip, by sending a link or a notification.
- **COL-095** · built · If the page was published by another person while someone was away, they see that when they return.
- **COL-096** · partly · At least 30 people can edit one page at once.
- **COL-097** · built · Comments, labels and status also update live for everyone on the page.

## 7.14 Messages in a chat system

Some people live in a chat system more than in their mail, so notifications can reach them there too.

- **COL-130** · later · When an operator connects the instance to a chat system, through the outward client and its allow-list (deliberate difference 5), a person can receive notifications and digests there as direct messages, besides or instead of e-mail.
- **COL-131** · later · An administrator turns each kind of chat message on or off for the instance, as for e-mail (COL-121): weekly popular pages (COL-124), the weekly summary of mentions (COL-125), and a daily digest of new pages, updates, comments and reactions. A message never names a page the person cannot see, and shows excerpts only as COL-076 allows.

## 7.15 Reporting content

A reader can tell the people who look after a space that something does not belong there.

- **COL-132** · later · A reader can report a page or a comment as unsuitable, from More (PAG-148), with a short reason. A report always has a person, because the instance has no anonymous access.
- **COL-133** · later · A report goes to the space administrators, not to the administrators of the instance, and they see who reported what. It never hides or changes the content by itself, and it is audited.
