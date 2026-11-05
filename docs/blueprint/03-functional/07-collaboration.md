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

## 7.3 Reactions

- **COL-023** · built · A person adds a reaction to a page, blog post or comment, from a short list of emoji and a picker.
- **COL-024** · built · Reactions show as small counters. Selecting one adds or removes the person's own reaction.
- **COL-025** · built · Hovering over a counter shows who reacted.
- **COL-026** · built · The author gets a notification, bundled when many people react.
- **COL-027** · built · Reactions can be turned off for a space, or for the instance.
- **COL-028** · built · Reactions count towards the Popular tab (NAV-032).

## 7.4 Mentions

- **COL-029** · built · `@` in a page, comment or task opens a list of people. Typing narrows it. Arrow keys and Enter pick one.
- **COL-030** · partly · The person shows as a highlighted name. Selecting it opens their card. If the name changes later, the mention follows. Still open: the card (wave F).
- **COL-031** · built · A person who is mentioned is notified once per version, not on every later edit.
- **COL-032** · built · The list shows people who can see the page first, and marks people who cannot. Writing a mention of someone who cannot see the page warns the writer, and offers to share it.
- **COL-033** · built · A person cannot find out about a page through a mention unless they may see it: a mention never leaks a title or text in a notification.
- **COL-034** · built · Groups can be mentioned when an administrator allows that, up to a limit of 50 members, to prevent mass messages.
- **COL-035** · new · A person's profile lists pages where they were mentioned.

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
- **COL-047** · new · A person can share by e-mail to someone with no account, only when an administrator allows invitations. The recipient gets an invitation, not the content.
- **COL-048** · later · Sharing to a chat system goes through webhooks or an app (chapter 12).

## 7.7 Asking for access

- **COL-049** · built · A person who opens a link to a page they may not see gets "not found", as the rule demands. An administrator can allow a request button there for spaces where the existence of pages is not secret.
- **COL-050** · built · A person who can read but not edit can ask for edit access from the page.
- **COL-051** · built · A request goes to the page owner, then the creator, then the last editor, and finally the space administrators, until someone is found who may grant it.
- **COL-052** · built · The person asked sees the request in their notifications and in Share, and can allow or refuse it.
- **COL-053** · built · Allowing adds the person to the page's access list and notifies them. Refusing closes the request. Every step is audited.

## 7.8 Watching

- **COL-054** · built · A person watches a page. They get a notification for new versions, comments and files.
- **COL-055** · built · A person watches a whole space, which covers all its pages and blog posts.
- **COL-056** · built · A person also watches a branch: a page and everything below it.
- **COL-057** · built · Creating or editing a page starts watching it by default. A preference turns that off.
- **COL-058** · built · A list in the preferences shows everything a person watches, with one click to stop.
- **COL-059** · built · A space administrator can see who watches a page, and add or remove people. A person who was removed may watch again.
- **COL-060** · built · A person who loses access stops getting notifications at once, also when they are still listed.
- **COL-061** · built · **Silent update** (PAG-025) sends no notification, and the history still shows the version.
- **COL-062** · new · Changes made by a dynamic block do not count as a change.

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

## 7.11 Notification preferences

- **COL-082** · built · A person sets per type of event: in the app, by e-mail at once, in the digest, or not at all.
- **COL-083** · built · A person sets the digest's day and time, and quiet hours.
- **COL-084** · built · A person can pause all e-mail for a period, for holidays.
- **COL-085** · built · Automatic watching can be turned off for pages a person creates, edits or comments on, separately.

## 7.12 Following people

- **COL-086** · built · A person can follow another person. Their pages and blog posts then appear in the For you feed.
- **COL-087** · new · A profile lists followers and who a person follows, and a person can turn this off for themselves.

## 7.13 Editing at the same time

- **COL-088** · phase 4 · Several people edit the same page at once, and see each other's changes and cursors with names.
- **COL-089** · phase 4 · Each person sees the others' pictures in the strip. A person who left turns grey.
- **COL-090** · phase 4 · Changes merge by themselves, with no conflicts and no lost text, also when two people type in the same line.
- **COL-091** · phase 4 · A person who is offline for a moment keeps typing. Their changes merge when the connection returns.
- **COL-092** · new · A draft is then shared by everyone who edits the page. Publishing publishes the shared draft, and the version names all who took part.
- **COL-093** · new · Clicking a colleague's picture jumps to their cursor.
- **COL-094** · new · The editor invites people from the strip, by sending a link or a notification.
- **COL-095** · new · If the page was published by another person while someone was away, they see that when they return.
- **COL-096** · new · At least 30 people can edit one page at once.
- **COL-097** · phase 4 · Comments, labels and status also update live for everyone on the page.
