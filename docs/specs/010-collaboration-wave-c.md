# 010: Wave C, working together

Status: built. What is still open is in the status lines of chapter 7 and in `docs/handoff/2026-11-05-wave-c.md`
Phase: wave C of `docs/blueprint/03-functional-requirements.md`. The product owner asked on 2026-11-05 to build the whole of blueprint 03, one commit per wave, and tests it when all waves are done.

## Problem

A page can now be written and read well, but it is a monologue. Nobody can answer it, point at a sentence, say thanks, hand someone a task, or hear that something changed. This wave makes people respond to each other: comments on a page and on a piece of text, reactions, mentions, tasks with a person and a date, sharing, asking for access, watching, notifications in the app and by e-mail, and a start screen that tells what happened. Editing at the same moment is not part of this wave.

## What the study found

**How people expect to respond.** The tools studied agree on the shape of inline comments. Selecting text brings up a small button. The thread sits beside the text. Resolving ends it, and it can be reopened from a list of resolved comments. One product advises a new comment rather than a reopened one for new feedback on the same passage. None of the sources says what happens to a thread when its text is deleted, so that rule comes from COL-018 alone.

**How tasks work elsewhere.** A task is a line in a task list. The first person mentioned in it is the assignee, and a date typed with `//` is the due date. Someone without access to the page does not see the task, and a personal list filters on open or done, and on assigned to me or created by me. A date near or past its due date changes colour. Our wave A already has a `date` node and the `//` trigger, so this needs no new attributes.

**How watching and notifications behave.** A watchlist is private, and watching is one click on a star or a word. One wiki stops mailing about a page until the person has visited it. A code host explains why each notification came (a label with a reason), keeps them for three months, and adds headers to its mails so people can filter them. The advice on noise is consistent: group by the thing that changed, let people choose per type of event, bundle what is not urgent into a digest, and leave out a digest with nothing new. The grouping key that fits us is the page.

**How text keeps its place.** The W3C Web Annotation model has two ways to point at text. A Text Quote Selector copies the words (`exact`) with some text before and after (`prefix`, `suffix`), and finds them again after the document moved around them. A Text Position Selector counts characters, and the model itself calls that very brittle. It says that when several places match, all of them match. It gives no advice on the length of prefix and suffix.

**What accessibility demands.** These come from WCAG 2.2 and the WAI-ARIA Authoring Practices, and change the design:

| Source | Demand | What we do |
|---|---|---|
| Feed pattern | A list that loads more while scrolling is a `feed` of `article` elements. `aria-busy` while adding, `aria-posinset`, `aria-setsize` (-1 when unknown). Page Down and Page Up move between articles, Control End and Control Home leave the feed, and these keys must be documented. | The start screen tabs are a feed with these keys, listed in the shortcuts overview. |
| 4.1.3 Status messages | A count change is `role="status"`, and the whole phrase is the content, such as "4 unread notifications", with `aria-atomic`. A saved comment is `role="status"`. `alert` is for errors only, and overuse is a failure. | The bell has a hidden status line. "Comment added" uses the toast. |
| Dialog pattern | A long list opens with focus on a static start. Focus returns to what opened it. | Share and the panels do this. |
| Combobox pattern | Focus stays in the text, and the list is pointed at. | The `@` list is the same list as `/`, with `aria-activedescendant`. |
| Tabs pattern | Left and Right move. | The start screen tabs and the comment filters. |
| 2.1.4 Character key shortcuts | Single keys need a way to turn them off. | `w`, `m`, `n`, `p`, `g n` join the existing preference. |
| 1.4.13 Content on hover or focus | Dismissable, hoverable, persistent. | Who reacted is a card that follows these rules, and a focusable counter shows the same. |
| 2.1.1 Keyboard | Nothing may need a mouse. | A highlight is a button. `m` comments on a selection. The panel lists every thread. |
| 2.5.8 Target size | At least 24 by 24. | Reaction counters, the bell and the floating button. |
| 1.4.1 Use of colour | Colour alone may not carry meaning. | An overdue date also says "overdue". A highlight has an underline as well as a tint. Unread items have a dot with a word for screen readers. |
| 2.2.1 Timing | A message with an action stays while used. | The toast after mentioning someone without access keeps its Share button. |

**What the libraries allow.** `@tiptap/suggestion` is already there, and wave A built the popup, so a mention needs no new package. Go's `net/smtp` is frozen, not deprecated, and has no DKIM or MIME help. `SendMail` only moves to TLS "if possible", so encryption is not guaranteed that way: we must drive `Client.StartTLS` ourselves. `PlainAuth` refuses to send a password without TLS, except to localhost. `mime`, `mime/multipart`, `net/mail` and `html/template` from the standard library do the rest. `time/tzdata` gives time zones in a small image. The outbox relay takes a `Publisher`, so a test can deliver events without NATS. `keys.Keyring` already has `Seal` and `Open` with a context, which gives an opaque, tamper-proof token.

**What the mail standards say.** RFC 8058 one-click unsubscribe needs one `List-Unsubscribe` header with exactly one HTTPS address, and `List-Unsubscribe-Post: List-Unsubscribe=One-Click`. The POST carries no cookies, so the address itself must identify the person, with a part that is hard to guess. The endpoint may not redirect. The message needs a DKIM signature covering both headers. RFC 3834 says mail from a periodic process carries `Auto-Submitted: auto-generated`, which stops auto-replies and loops.

**Safety.** The blueprint is firm: a notification is checked against the rule, and nobody learns a title or a sentence from a page they may not see (deliberate difference 8). So a notification row holds ids and never a title, and a person without access gets nothing at all. A comment is user content and goes through the same allow-list gate as a document. Names and titles go into mail, so they can carry line breaks into headers, and into HTML. Allowing access could widen rights quietly, so the dialog must say exactly what happens. A share or a group mention could become a way to send mass messages, so both have limits. The only new public route is the unsubscribe, which can only reduce mail.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | An inline comment holds a text quote (`exact`, `prefix`, `suffix`) and a character offset as a hint. No mark goes into the document | Versions are immutable and a reader cannot edit, so a mark would need a new version from someone without the right |
| D2 | The highlight is a decoration worked out in the browser. It only shows when one place fits. Otherwise the thread says its text is no longer in the page | Pointing at the wrong sentence is worse than pointing at none. The model allows all matches, and we choose not to guess |
| D3 | Prefix and suffix hold 32 characters each, and `exact` at most 1,000 | The model gives no length. 32 tells repeated phrases apart and stays small |
| D4 | A comment is a small document through the same gate, with a smaller allow-list | One gate, one rule. A second sanitiser would drift |
| D5 | Replies form one level. A reply to a reply joins the same thread | COL-003 asks for it, and long trees are hard to read and to hear |
| D6 | A mention is our own node on `@tiptap/suggestion`. It stores an id, and never a name, in documents, comments and search text. The name is filled in when a document is read, and search takes it from `page_mentions` | The mention follows a rename. A version cannot change, so a stored name could never be anonymised (GAP-004) |
| D7 | A task's person is the first mention in its own line, and its date the first date. `taskItem` only gains an `id` | People already expect this, wave A has the date, and the document stays the one truth. `page_tasks` is an index |
| D8 | Ticking a task from a list publishes a silent version | The document is the truth, so a tick must change it, and a tick is not news |
| D9 | Notifications fan out in the worker from the outbox events, and the handler can run twice without harm | Blueprint 04 says so. A slow fan-out never slows publishing |
| D10 | A notification row holds ids, a type and a group key. Titles are read when it is shown | A renamed page reads right, and a hidden page cannot leak through old text |
| D11 | Access is checked three times: at fan-out, when shown, and when mailed | COL-060 and COL-072. A person can lose access between any two of these |
| D12 | The count polls every 30 seconds, only in a visible tab. No WebSocket in this wave | Blueprint 04 plans a WebSocket for live updates. That belongs with editing together. One small indexed count at this rate is cheap, and it satisfies COL-070 |
| D13 | Mail goes through `net/smtp`, driven by hand: TLS is required unless the operator turns it off for a local relay, and the certificate of the server is checked against the system's trust store and the host name | No dependency, no quiet fall back to plain text, and no mail to whoever answers (SEC-416) |
| D14 | DKIM is left to the mail server | The standard library has no signing, and a key would be one more secret to run. The plain link in every mail works either way |
| D15 | An unsubscribe token is sealed with the keyring, and does not expire. GET never changes anything. Only POST does. The request log writes the route, not the token | Mail scanners follow links. The token only carries a person and what to stop, and the worst abuse is less mail. A log is read by more people than a mail (SEC-422) |
| D16 | By default mail holds titles and no excerpt. An administrator can allow excerpts | COL-076, for instances with sensitive content |
| D17 | The feed is worked out from versions and comments. No activity table | No backfill, no lag and no second copy. Phase 3 measures it under load |
| D18 | Allowing access adds the person to every restriction that blocks them. The dialog lists each level and what it opens, and it is refused for someone who cannot manage them all. If the space is what blocks them, only a space administrator can allow it, and the dialog says it gives Editor in the whole space | A restriction can only narrow, so a page cannot grant more than the space does. A restriction above opens a whole branch, and saying so prevents a quiet widening (GAP-007) |
| D19 | Sharing only notifies. A person without access gets no notification, and the dialog names them | COL-043. A notification would reveal the title |
| D20 | Group mentions are off by default, and stop at 50 members | COL-034 |
| D21 | Comments and reactions can be turned off per space in `spaces.features`. The full screen of SPC-047 waits for wave D | COL-009 and COL-027 need it now, and the column fits the later screen |
| D22 | The comments panel uses the place and pattern of the Details panel. Only one of the two is open at a time | One panel on the far side, no second way |
| D23 | Versions gain `silent`, written when they are made | Feed and notifications must leave them out, and a version cannot be changed later |
| D24 | Comment drafts stay in the browser, under the id of the person, for seven days, and logging out removes them | COL-011 says so, and a half-written comment is private. It is also text of a page on a device (SEC-266) |
| D25 | The default per type: mentions, tasks, shares and access requests in the app and by mail at once. All else in the app and the digest | COL-073 and COL-074 |
| D27 | Publishing opens a thread made while editing only when its quote is in the published text. Otherwise it stays with the editors | A sentence that was never published must not reach readers through a comment (GAP-006) |
| D28 | A reaction is its own action, `reaction.set`, in `Evaluate`: it needs view, and it stops in an archived space | It changes what others see, so it is not reading. Wave D adds archived pages to the same rule (GAP-021) |
| D30 | Removing a version blanks the quotes that hold its words, through `comments.BlankQuotes`. The thread stays. Wave D calls it from the removal of a version | An anchor copies text, so a secret would live on in it (SEC-407) |
| D29 | Following a person needs *view profiles*. Without it, every id gets the answer of an id that does not exist. Stopping always works | Nobody can test who is here (GAP-030, PRM-083) |
| D26 | For testing, Mailpit (MIT, image `axllent/mailpit`, pinned) is an optional service with its own profile in `compose.dev.yaml`. It shows all mail in a web screen, and nothing leaves the machine. Production takes any SMTP server | The product owner has no mail server and asked for an open-source one. It is a tool beside the product, so it adds no code dependency |

## Scope

**In:** COL-001 to 034, 036 to 039, 041 to 046, 049 to 080, 082 to 086. NAV-021, 029 to 035, 044, 046, 062, 063, 065. PAG-025, 095, 102, 112, 115 (watchers), 119 (reactions and comments). SPC-021, 028, 038 (Watch). ADM-029, and the part of ADM-028 that wave C cannot do without: settings from the environment and Send test.

**Out, and where it goes:**

| Item | Goes to |
|---|---|
| Editing at the same moment, and who else is on a page (COL-088 to 097, PAG-120) | Its own wave, with the WebSocket and a dependency question for the product owner |
| A profile that lists mentions, followers and following (COL-035, 087) | Wave F, with profiles |
| Inviting someone without an account (COL-047) | Wave F, with invitations |
| Mail in other languages (COL-080) | Wave F, with a person's language. Mail text already sits in one file |
| Task report block (BLK-038, the report half of COL-040) | Wave E. Task text is already found by search |
| Searching comments (ORG-024) | Wave D, with the search filters |
| The full features screen (SPC-047 to 049) | Wave D |
| The mail settings screen (the rest of ADM-028) | Wave F |
| Publish options other than silent (PAG-024, 026, 027) | Wave D |
| Sharing to chat, push, RSS (COL-048, 081) | Later |

COL-062 needs nothing yet: dynamic blocks do not exist, and wave E must keep it true.

## Feature plan

### C1. Page comments

The box sits below the page, after reactions, and is the end of the page.

- **Writing:** a small editor with bold, italic, inline code, links, lists, code blocks and mentions. Ctrl Enter posts. Images come in when the person has *add attachment*, and are attachments of the page.
- **Showing:** picture, name, relative time, and "edited" with the time on hover. Replies sit one level in. A reply to a reply joins the same thread and mentions the person.
- **Editing:** the author, or a space administrator. The last edit shows.
- **Deleting:** *delete own* or *delete any*. It goes to the trash of the space. A comment with replies leaves "This comment was removed", and the replies stay. Restoring brings the words back. One Undo in the toast, as for a page.
- **Address:** the date is a link to `#comment-<id>`. Opening it scrolls there, opens what is needed, and marks it briefly, with the code of B10.
- **Order:** oldest or newest first, a preference. Replies always run oldest first.
- **Loading:** 30 threads at a time, with Show more.
- **Turned off:** per space and per page. Existing comments stay readable, and the box says it is closed. Archived spaces and pages have no box.
- **Draft:** kept in the browser per person, page and reply, and offered back. Posting clears it, logging out removes it, and after seven days it goes by itself.
- **Watching:** posting starts watching the page, unless turned off.
- **Keyboard:** `m` moves to the box. Tab reaches every action. The thread is a list, and each comment an `article` with the author and time as its name.
- **Limits:** 100 KiB per comment, 30 comments a minute per person.
- **Edge cases:** a failed post keeps the text. A comment on a page that was deleted in between says so. An author who was disabled stays named. A version restore leaves comments alone (PAG-095). Copying a page copies none.

### C2. Inline comments

- **Starting:** selecting text in the document shows one floating button row, Comment and Copy link. `m` does the same with a selection. In the image viewer, a Comment button comments on that image.
- **Stored:** kind `text` with `exact`, `prefix`, `suffix`, `offset` and the version, or kind `image` with the attachment id. A selection in a table cell is text like any other. Longer than 1,000 characters is refused with a message.
- **Finding it again:** the text of the current version is built once, with blocks separated by a line break. Candidates are places where `exact` occurs. Those where prefix and suffix fit win. If several remain, the one at the stored offset wins only when it is exact. If there is no single answer, the thread has no highlight.
- **Highlight:** a soft tint with an underline. It is a `button` named "Comment by Anna: the quoted words". Selecting it opens the thread. Resolved threads have none.
- **Thread:** opens in the comments panel, with the quoted words on top, then replies.
- **Resolve and Reopen:** anyone with *edit*. Reopening brings back the highlight if the text is still there.
- **Text gone:** the thread stays in the list with the old words and a note "No longer in the page".
- **While editing:** selecting text in the editor offers Comment too. Such a thread is `editors_only`, and only people with *edit* see it. Publishing the page makes it visible to all when its quote is in the published text. Otherwise it stays with the editors, and it opens at the first publish that holds the words.
- **Narrow screens:** the panel is a sheet from the bottom, so the text stays above it.
- **Keyboard:** `n` and `p` go to the next and previous unread comment, or to the next highlight when nothing is unread. They wrap. Without shortcuts, Tab reaches each highlight, and the panel lists them all.
- **Edge cases:** a quote that spans a block that later moved is "no longer in the page". Overlapping highlights both work, and the inner one gets the click. A selection inside the title or the byline offers nothing. A person without *add comment* sees only Copy link.

### C3. The comments panel

A panel on the far side, opened by a count in the byline, or by a highlight.

- **Filters:** Open, Resolved, Unread, Page comments, as tabs. Sort by time or by position in the page.
- **Rows:** author, time, quote or first words, number of replies. Selecting a row scrolls to the text and marks it.
- **Unread:** a comment is unread when it is newer than the last time the person saw the comments of this page and is not their own. Seeing the panel or the end of the page moves that moment.
- **Accessibility:** `complementary`, focus to its title and back, Escape closes.

### C4. Reactions

- **Where:** a row at the end of a page, and under each comment.
- **Adding:** a button opens six common emoji and a search through our own list. The same person can add several, once each.
- **Counters:** a small `button` with the emoji and number, `aria-pressed` for your own. Selecting it adds or removes yours.
- **Who:** hover or focus shows up to 20 names and "and 4 more".
- **Limits:** 20 different emoji per item. An unknown value is refused.
- **Turned off:** per space and for the instance. Existing reactions stop showing, and are not deleted. In an archived space they stay, and cannot change.
- **Notification:** to the author, grouped, by digest by default. Never for your own reaction.

### C5. Mentions

- **Writing:** `@` opens the same list as `/`, in the editor and in comments. Typing narrows it. Up, Down, Enter and Tab pick, Escape closes and keeps the text.
- **Order:** people who can see the page first. Those who cannot are marked in words. Groups only when allowed, with their size.
- **Who is offered:** with *view profiles*, the directory. Without it, only people with a grant in this space (NAV-059).
- **Node:** `mention` with `id` and `kind`. The gate drops anything else. `label` only exists on the way out, filled in when a document is read. In text it reads `@Anna Visser`. An export writes the name of that moment.
- **Reading:** a soft box. It is a plain name in this wave, and the card comes with wave F. A disabled person stays named, marked as no longer active.
- **Writing someone without access:** a toast says so, with a Share button. The page is not shared by itself.
- **Notified:** once per version, for mentions that are new in that version (COL-031). A person without access gets nothing (COL-033). Mentions in a comment notify when it is posted, and an edit only notifies new ones. A silent update still notifies a new mention: a mention is addressed to a person.
- **Groups:** `@group` notifies each member, when allowed, up to 50. Above that the list does not offer it, and the server ignores it.
- **Edge cases:** a pasted mention from another page stays a mention. An account that is gone reads as "@someone". `@` after a letter, as in an address, does not open the list. Inside code it does not open.

### C6. Tasks

- **In a page:** a person and a date are written in the task's line, with `@` and `//`. The first of each counts. An overdue date is red and says "overdue", and one due within seven days is orange.
- **Index:** publishing rewrites `page_tasks` for that page in the same transaction, with the id, text, person, date and whether it is done. Each task gets an id when it is first published. A copied id gets a new one.
- **Ticking:** with *edit*, a tick in the reading view or in My tasks publishes a silent version. A person without *edit* sees the checkbox as text, with the reason in the row.
- **My tasks:** `/tasks`, with the tabs Assigned to me and Created by me, and a filter on open and done. Grouped into overdue, today, this week, later, no date. A row shows the task, the page and the date, and opens the page at the task.
- **Notified:** when assigned in a new version, and on the due date, once.
- **Writing for someone without access:** flagged as with mentions (COL-041).
- **Edge cases:** a task on a page in the trash or out of reach disappears from the list. A task with an empty line stays in the list as "Untitled task". Ticking while someone else has an open draft leads to the normal conflict question when they publish.

### C7. Sharing

- **Share** joins Edit, star and lock (PAG-102). **Copy link** sits beside it inside the dialog, and in the row menu it is already there.
- **Dialog:** who has access now, a field for people and groups, a message of at most 500 characters, and a checkbox to notify, on by default.
- **Who has access:** the people and groups in the restriction when there is one, or else in the space. "All members" is one line. At most 50 names, then a count.
- **No access:** a person who cannot see the page is marked before sending, gets no notification, and those with *manage restrictions* get Give access beside the name, which uses the existing rule.
- **Restrictions:** the dialog holds the same lists as the lock, so one place covers who can see this. The lock keeps working.
- **Limits:** 30 people per share, groups count by their members.
- **Audit:** `page.shared` with the number of people, without the message.

### C8. Asking for access

- **Reading but not editing:** Ask to edit in More (COL-050).
- **Not found:** a page a person may not see stays "not found". Only when a space turns on `access_requests` does the answer also offer Ask for access. The status stays 404.
- **Who is asked:** the creator until wave D adds an owner, then the last editor, then the space administrators. The first one who may grant it gets the request, and all space administrators can see it.
- **Answering:** Allow or Refuse, from the notification and in Share. A note can go along.
- **Allowing:** see D18. The dialog lists each restriction and how many pages it opens, before it happens. Of a restriction the person cannot manage, they learn only that it is there.
- **Limits:** one open request per person, page and kind. Refused requests can be repeated after seven days.
- **Audit:** every step, with who and what.

### C9. Watching

- **Page:** Watch in More, and `w`. A second choice watches the page and everything below it.
- **Space:** in the space menu and the overview.
- **Automatic:** creating, editing and commenting start watching, each with its own preference.
- **List:** `/me/watching` lists everything with one Stop per row, and a filter.
- **Watchers:** a space administrator sees who watches a page, in Details, and can add or remove someone. This is audited. A person who was removed may watch again.
- **Lost access:** nobody hears about what they cannot see, also when they are still listed. The row stays, and works again if access returns.
- **Silent update:** the Publish button gains a small arrow with Publish without notifying. The history still shows the version.

### C10. The bell and the notification panel

- **Bell:** beside help, with a dot and a number, capped at 99+. Its name is "Notifications, 3 unread". A hidden `role="status"` line carries the same phrase and only changes when the number goes up.
- **Panel:** opens under the bell, and `g` `n` opens it. A list of links with a person, a sentence, the page, and the time, with an **Unread only** switch, Mark all as read, and a per-row Mark as read or unread.
- **Grouped:** the same type on the same page while unread is one row: "3 people commented on Onboarding".
- **Reason:** each row says why it came: mentioned, watching, assigned, shared.
- **Link:** goes to the exact spot, and marks the row as read.
- **Full screen:** `/notifications`, with filters on type and space, 50 at a time.
- **Kept:** 90 days.
- **Your own actions:** never, unless the person turned that on.
- **Lost access:** the row is hidden when shown, and the count follows.
- **Edge cases:** a page that was purged hides the row. A poll that fails keeps the old number and tries again. Two tabs agree after the next poll.

Types: `mention`, `task_assigned`, `task_due`, `share`, `comment`, `reply`, `reaction`, `page_changed`, `access_request`, `access_answer`. Review reminders join in wave D.

### C11. E-mail

- **Settings:** `DIFFLUENCE_SMTP_HOST`, `_PORT`, `_USERNAME`, `_PASSWORD` (and `_PASSWORD_FILE`), `_FROM`, `_REPLY_TO`, `_TLS` with `starttls` (default), `implicit` or `none`. Without a host, mail is off, and the app says so in the admin console.
- **Sending:** the worker works through `mail_queue`. A slow or broken server never touches a request. It tries five times with growing waits, then marks the mail as failed.
- **Rendered when sent:** the queue holds notification ids, not text. Access is checked and the text is made at that moment, so a person who lost access gets nothing.
- **Message:** plain text and HTML. `html/template` escapes everything. Names and titles lose line breaks, and non-ASCII headers are encoded.
- **Headers:** `Auto-Submitted: auto-generated`, `List-Id`, `List-Unsubscribe`, `List-Unsubscribe-Post`, and `X-Diffluence-Reason` for filters.
- **Content:** title and link by default. With the instance setting on, a short excerpt.
- **At once:** the types with that choice, in a batch per minute, so five mentions are one mail.
- **Digest:** daily or weekly, grouped by space, with comments, reactions, new and changed pages. Left out when there is nothing new, and things already read are left out too.
- **Footer:** a link to stop this kind of mail and a link to the preferences. A line that a watch brought has its own link to stop watching, which ends every watch that brings that page: on the page, a branch above it, and the space.
- **Admin:** a queue view with time, kind and status, and no content and no addresses. Send test mails the administrator.
- **Off:** an administrator can turn e-mail off for the instance, and set the defaults for new users.
- **Secrets:** the password is only read from the environment or a file, is never shown, and never reaches a log. Mail errors are logged without addresses.

### C12. Unsubscribe

- **Address:** `https://…/api/v1/mail/unsubscribe/<token>`. The token is sealed with the keyring, and holds the person, the scope and a target: all mail, digest, or one watch.
- **POST:** carries out the stop, with no session, and answers 200 without a redirect.
- **GET in a browser:** the link in the mail opens a small screen at `/unsubscribe/<token>`, which says what will stop and has one button. GET changes nothing.
- **Bad token:** the same plain answer, and nothing happens.

### C13. Notification preferences

`/me/notifications`:

- **Per type:** Off, In the app, In the app and by e-mail, In the app and in the digest.
- **Digest:** daily or weekly, day and hour. The time zone comes from the browser when saved, and is shown.
- **Quiet hours:** a start and end. Mail waits until they end.
- **Pause:** until a date. Nothing is mailed, and the app goes on.
- **Automatic watching:** three switches.
- **Your own actions:** one switch.
- **Comment order:** oldest or newest first.

### C14. The start screen

- **Tabs:** For you, Recent activity, Popular, above the feed. The Spaces cards stay.
- **For you:** pages changed or created in what the person watches, and by people they follow, without silent versions and without their own actions.
- **Recent activity:** everything in spaces they can see, with a filter on a space.
- **Popular:** pages with the most reactions and comments in seven days, counting only what they may see.
- **Item:** title, space, a short excerpt, author, time, and reactions and comments when there are any.
- **Loading:** more arrive near the end, and a Load more button always works. `aria-busy` while loading.
- **Side column:** up to six favourite or recent spaces, and open tasks. It moves below on narrow screens.
- **Empty:** a sentence that says what will appear, and how to watch something.

### C15. Following

A Follow button beside a name in Contributors, in the Details panel, for those with *view profiles*. It only feeds For you. The lists on a profile wait for wave F.

### C16. Keyboard shortcuts

| Keys | Action |
|---|---|
| `g` `n` | Open notifications |
| `w` | Watch or stop watching this page |
| `m` | Comment on the selection, or go to the comment box |
| `n`, `p` | Next and previous unread comment |
| Page Down, Page Up | Next and previous item in a feed |

All of them join the overview, and the preference turns off all but the Page keys, which only work with focus in the feed.

## API

All new routes, with their access rule.

| Route | Rule | What |
|---|---|---|
| `GET /pages/{id}/comments` | `page.view` | Threads, filter, paging |
| `POST /pages/{id}/comments` | `comment.add` | New comment or reply, with a quote or not |
| `PATCH /comments/{id}` | `comment.add` | Edit: author or space administrator |
| `DELETE /comments/{id}` | `comment.delete` | To the trash |
| `POST /comments/{id}/resolve`, `/reopen` | `page.edit` | End or reopen a thread |
| `PUT /pages/{id}/comments/seen` | `page.view` | Move the unread moment |
| `PUT, DELETE /pages/{id}/reactions/{emoji}` | `page.view` | Own reaction, needs the feature on |
| `PUT, DELETE /comments/{id}/reactions/{emoji}` | `page.view` | Own reaction |
| `GET /pages/{id}/reactions`, `/comments/{id}/reactions` | `page.view` | Counts, and who |
| `GET /pages/{id}/mention-candidates?q=` | `page.view` | People and groups, with `can_view` |
| `GET /me/tasks?tab=&status=` | authenticated | My tasks |
| `POST /tasks/{id}/done` | `page.edit` | Tick or untick |
| `GET /pages/{id}/access` | `page.view` | Who has access |
| `POST /pages/{id}/share` | `page.view` | Notify people |
| `POST /pages/{id}/access-requests` | authenticated | Ask. The module decides, and answers 404 unless allowed |
| `POST /access-requests/{id}/allow`, `/refuse` | `restriction.manage` | Answer |
| `PUT, DELETE /pages/{id}/watch` | `page.view` | Own watch, with `scope` |
| `PUT, DELETE /spaces/{id}/watch` | `space.view` | Own watch |
| `GET /me/watches` | authenticated | The list |
| `GET /pages/{id}/watchers` | `space.administer` | Who watches |
| `PUT, DELETE /pages/{id}/watchers/{user}` | `space.administer` | Add or remove |
| `GET /me/notifications`, `/me/notifications/count` | authenticated | List and count |
| `PUT /me/notifications/{id}/read`, `/unread` | authenticated | Own rows only |
| `POST /me/notifications/read-all` | authenticated | Own rows only |
| `GET, PUT /me/notification-settings` | authenticated | Preferences |
| `GET /feed?tab=&space_id=&before=` | authenticated | List: only what the caller may see |
| `PUT, DELETE /users/{id}/follow` | authenticated | Own follow |
| `PATCH /spaces/{id}/features` | `space.administer` | Comments, reactions, access requests |
| `PATCH /pages/{id}/comments-closed` | `page.edit` | Comments off for one page |
| `GET /admin/mail` | permission `administrator` | Queue, no content |
| `POST /admin/mail/test` | permission `administrator` | Send test |
| `POST /mail/unsubscribe/{token}` | public | RFC 8058 sends no session. The token is the proof, and it can only reduce mail |

`POST /pages/{id}/publish` gains `silent`. `permissions` gains the action `comment.delete` (own or any), which changes `Evaluate`, the SQL and the literal rule in the matrix test.

**Events** (audit and outbox in the same transaction): `comment.created`, `comment.edited`, `comment.trashed`, `comment.restored`, `comment.resolved`, `comment.reopened`, `reaction.added`, `reaction.removed`, `page.shared`, `access.requested`, `access.allowed`, `access.refused`, `watcher.added`, `watcher.removed`, `space.features_changed`, `task.done`, `task.reopened`. `record` in `pages` adds `actor_id`, `version` and `silent` to its payload. Payloads hold ids only. A person's own watch, follow, read mark and preferences are personal, so they are not audited.

## Data

Migration 0007, expand only:

- `comments`: id, origin_instance_id, space_id, page_id, parent_id, author_id, body, anchor, status (`open`, `resolved`), editors_only, resolved_by, resolved_at, trash_item_id, created_at, edited_at, edited_by.
- `comment_seen` (user, page, seen_at).
- `reactions` (target_type, target_id, user_id, emoji, created_at), with the first four as key.
- `watches` (user_id, target_type `page`, `branch`, `space`, target_id, created_at).
- `follows` (user_id, followed_id, created_at).
- `page_mentions` (page_id, user_id), rewritten like `page_links`.
- `page_tasks`: id, page_id, space_id, text, assignee_id, creator_id, due_on, done, done_at, position, due_notified, updated_at.
- `notifications`: id, user_id, type, reason, actor_id, page_id, space_id, comment_id, group_key, source_event, data, read_at, hidden_at, digested_at, created_at. Unique on user and source event. A partial index on unread rows.
- `mail_queue`: id, user_id, kind, notification_ids, status, attempts, next_attempt_at, sent_at, last_error, created_at.
- `access_requests`: id, page_id, space_id, requester_id, kind, asked_id, status, answered_by, note, created_at, answered_at.
- `user_preferences`: gains `notifications jsonb`.
- `page_versions`: gains `silent boolean NOT NULL DEFAULT false`, set on insert. The trigger on updates stays.
- `pages`: gains `comments_closed`.
- `spaces`: gains `features jsonb NOT NULL DEFAULT '{}'`.
- `trash_items`: the check on `object_type` also allows `comment`.
- An index on `page_versions (created_at DESC)` for the feed.

`settings.Values` gains `mail_enabled`, `mail_excerpts`, `reactions_enabled`, `group_mentions`. Document format: `mention` node, `id` on `taskItem`. `SchemaVersion` stays 1, because old documents stay valid.

New packages: `internal/comments`, `internal/notify` (fan-out, list, preferences, digests), `internal/mail` (queue, rendering, SMTP), `internal/watches`, and `internal/tasks`. Each talks to the others through its exported API.

## Build order

Parts in one step touch different files, so they can be built side by side.

| Step | Parts | Touches |
|---|---|---|
| 1 | Migration, permission action with the matrix, document format (`mention`, task ids, comment allow-list, name filling) | `internal/db`, `internal/permissions`, `internal/document` |
| 2a | Comments and reactions on the server | `internal/comments` |
| 2b | Watches, follows, tasks | `internal/watches`, `internal/tasks`, `internal/pages` |
| 2c | Mail: queue, rendering, SMTP, unsubscribe | `internal/mail`, `internal/config` |
| 3 | Fan-out, notification list, preferences, digests, then the worker wiring | `internal/notify`, `internal/app` |
| 4 | Sharing, access requests, feed | `internal/pages`, `internal/httpapi` |
| 5a | Comment box, panel, highlights, floating button | `web/src/components/comments/` |
| 5b | Mention and task marks in the editor | `web/src/components/editor/` |
| 5c | Bell, panel, notification and settings screens | `web/src/components/notifications/`, `web/src/views/` |
| 5d | Share, access requests, Watch, feed, My tasks, Watching | `web/src/views/` |
| 6 | Shortcuts, pictures, mutations, docs, statuses, handoff | All |

## Test plan

- **Go integration, failure paths first:** every route with a stranger, a viewer and an editor. A comment on a hidden page. A body with a script or a free colour. A reply to a reply. Delete with replies, restore, purge. `editors_only` hidden from a viewer and open after publishing. Reaction limits. A mention of someone without access gets no row. A mention notifies once per version. Silent versions. Task index through publish, restore, copy and trash. Ticking without *edit*. Fan-out delivered twice gives one row. Own actions give none. Lost access hides the row and stops the mail. Digest with nothing new sends nothing. Access requests: all allow paths, and the 404 when the feature is off. Unsubscribe: tampered token, GET changes nothing, POST without a session.
- **Delivery:** tests run the real relay with a `Publisher` that calls the handler, and one test goes through real JetStream.
- **Mail:** a small fake SMTP server in the test proves STARTTLS is required, that no plain password leaves, retries, headers, and a title with a line break that cannot add a header.
- **Route walk:** `objectRoute` and `validRequest` learn comments, tasks and requests, so new object routes join `TestEveryObjectRouteIsProtected`.
- **Permission matrix:** `comment.delete` in `Evaluate`, in the SQL if a list needs it, and in the literal rule.
- **Mutation:** a hidden page in a notification, a notification without a check at show time, a mail without a check at send time, the actor notified about themselves, a tampered unsubscribe token accepted, GET unsubscribing, STARTTLS skipped, a share granting access, allow widening a space without an administrator, an `editors_only` comment shown to all, the group limit lifted, `silent` ignored.
- **Vitest:** finding a quote with one, several and no matches, across blocks, and after an edit. Grouping of My tasks by date. The `n` and `p` order. The count phrase.
- **Browser:** each part above with the keyboard first. Two people in two contexts for comments, mentions and notifications.
- **Pictures:** comments, a thread with a highlight, the bell panel, NotificationsView, NotificationSettingsView, TasksView, WatchingView, the start screen tabs, Share, in light, dark and narrow where it matters, with little and with much.
- **Accessibility:** axe on every new screen and with panels open.

## Acceptance criteria

- [x] AC-1 A person comments, replies, edits and deletes, and a deleted comment with replies leaves a line and comes back from the trash.
- [x] AC-2 A selected sentence gets a thread with a highlight that survives edits elsewhere in the page.
- [x] AC-3 When the text is gone or no longer unique, the thread stays in the list with its quote, and no wrong text is marked.
- [x] AC-4 Resolve and Reopen work, and only for those with *edit*.
- [x] AC-5 A thread made while editing is hidden from readers until the page is published.
- [x] AC-6 Everything about comments works without a mouse.
- [x] AC-7 Reactions can be added and removed on pages and comments, show who reacted, and can be turned off.
- [x] AC-8 `@` works in pages and comments, the name follows a rename, and someone without access is marked and gets nothing.
- [x] AC-9 A task with a person and a date shows in My tasks, can be ticked there, and warns on the due date.
- [x] AC-10 Share notifies only people with access, names the others, and never grants anything by itself.
- [x] AC-11 A request for access reaches someone who may grant it, every step is audited, and the dialog says what will change.
- [x] AC-12 A page, a branch and a space can be watched, and the list can stop each.
- [x] AC-13 The bell counts, groups and links to the exact spot, and updates without a reload.
- [x] AC-14 A person who loses access stops hearing at once, in the app and by mail.
- [x] AC-15 Nobody gets a notification about their own action, unless they asked.
- [x] AC-16 A silent update reaches nobody, and the history shows it.
- [x] AC-17 Mails go out over TLS with the right headers, and a broken mail server slows nothing.
- [x] AC-18 A digest groups by space, and does not come when there is nothing new.
- [x] AC-19 One-click unsubscribe works without a session, and a GET changes nothing.
- [x] AC-20 Preferences per type, quiet hours and pause are obeyed.
- [x] AC-21 The start screen has For you, Recent activity and Popular, loads more by itself, and shows nothing a person may not see.
- [x] AC-22 All new shortcuts work, are listed, and turn off with the rest.
- [x] AC-23 Everything passes axe, and the new pictures have been looked at.

## Risks

| Risk | Answer |
|---|---|
| A quote matches the wrong place | Only one fit counts (D2), and the thread keeps its words |
| Fan-out to many watchers is slow | It runs in the worker. A batch insert per event, and phase 3 measures it |
| The feed query is slow on a large instance | One index now, measured in phase 3, and a table can follow without a change for readers |
| The count shows a row the person lost access to | It only shows a number. The list hides the row, and the count follows |
| Ticking a task bumps the version while someone has a draft | The normal conflict question answers it, and the history shows why |
| A mail provider ignores one-click without DKIM | The operator's mail server signs. The link in the mail works either way, and the operations guide says so |
| `net/smtp` is frozen | It only needs to keep working, and a `Sender` interface keeps it replaceable |
| Mail is not set | The app works without it, the queue waits or is off, and the admin console says so |
| A share or mention becomes a mass message | Limits per share, per group and per minute |
| A time zone gets due dates wrong by a day | Due dates are dates, and the job checks them in the person's digest zone |
| This wave is large | The build order keeps parts apart, and each step ends with green tests |

## For the product owner to decide

- **Which mail server to test with.** Decided: Mailpit (D26). `docs/development.md` says how to start it. Everything works without one, and mail then waits in the queue.

No new dependency is needed.

## How the product owner tests it

Use two browsers, with two people.

1. As the first person, comment under a page. As the second, reply. Check the bell in the first browser, without reloading.
2. Select a sentence and press Comment. Edit another paragraph and publish. The sentence is still marked. Now delete that sentence, and see the thread say so.
3. Resolve the thread, and reopen it.
4. Add a reaction, and hover over the number.
5. In the editor, type `@`, pick the other person, and publish. Then make a person who cannot see the page, and mention them.
6. Add a task with `@` and `//`, publish, and open My tasks in the other browser. Tick it there.
7. Press Share and send the page. Try someone without access.
8. Watch a space with `w` on a page or from its menu, change a page as the other person, then again with Publish without notifying.
9. Take away access for someone with a notification, and see it disappear.
10. Set the mail values, send a test, and press Unsubscribe in a mail.
11. Open the start screen and try the three tabs.

## Provenance

Own text from these public sources, read for behaviour only, plus the study behind blueprint 03 (`docs/blueprint/03-functional/sources.md`). Help pages marked as summaries were read through a search summary.

- W3C Web Annotation Data Model: Text Quote Selector and Text Position Selector.
- W3C WAI-ARIA Authoring Practices: feed, combobox, tabs, modal dialog.
- W3C WCAG 2.2 Understanding 4.1.3 Status Messages and 2.1.4 Character Key Shortcuts.
- RFC 8058 (one-click unsubscribe) and RFC 3834 (automatic responses).
- Go documentation of `net/smtp`.
- Mailpit: release page and licence file.
- MediaWiki Help:Watchlist, and GitHub's documentation on configuring notifications.
- Summaries: inline comment help of Plane, Document360 and a commercial wiki. Task help of a commercial wiki, Loomio and Kinopio. Guidance on batching and digests from SuprSend, Courier and ServiceNow documentation.
