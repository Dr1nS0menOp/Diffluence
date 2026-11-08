# 05.9: The rule

## 9.1 The rule itself

- **PRM-040** · built · A person may do action *A* on page *P* when all of these hold:

| # | Check | Failing it means |
|---|---|---|
| 1 | The account is active | Not found |
| 2 | The person holds `use_product` | Not found |
| 3 | The space is not in the trash | Not found |
| 4 | The person holds `space.view` | Not found |
| 5 | The person holds the permission for *A* | Forbidden |
| 6 | The space is not archived, unless *A* is view, export or administer | Forbidden |
| 7 | The page is not in the trash | Not found |
| 8 | The page is published, or the person created it | Not found |
| 9 | Wave D: the page is not a template | Not found |
| 10 | Wave D: the page is not archived, unless *A* reads, exports or deletes | Forbidden |
| 11 | On every page from the top down to *P*: no view restriction, or the person matches it | Not found |
| 12 | For edits: no edit restriction on *P*, or the person matches it | Forbidden |

- **PRM-041** · built · Every check always runs, so an explanation shows the whole picture and not only the first refusal.
- **PRM-042** · built · `Allowed` is all checks. `Visible` is the checks that say "not found". The answer follows from these two, and nothing else.
- **PRM-043** · built · Not visible gives 404. Visible but not allowed gives 403. Nothing reveals the difference between hidden and absent (NAV-005). Known exceptions are in GAP-014.
- **PRM-044** · built · Allow-only. There is no deny entry. To limit access, narrow it with a restriction.
- **PRM-045** · built · Deny by default. No grant means no access, also for administrators.
- **PRM-046** · built · One rule, in two forms: `permissions.Evaluate` for one decision, the `perm_*` SQL functions for lists. A test compares both with a third, literal copy of the rule (05.11).
- **PRM-047** · built · **No cache.** Facts are read for each decision. The old text promised a cache with invalidation, which does not exist. A cache may only come after a measurement in phase 3, with a test that a revoked right ends within one request.
- **PRM-048** · built · A decision holds for one request. Under Read Committed a restriction can land between the check and the read, and the person then gets what they could have had a moment earlier. Actions that write check inside their transaction. Long actions check again at the end, as an upload does.
- **PRM-049** · built · Every refusal through a module is audited as denied.

## 9.2 Who matches

- **PRM-050** · built · A person matches a restriction when listed directly, or through a group.
- **PRM-051** · built · All members means `type = member`, all guests means `type = guest`. These two exist only as grants, never on a restriction. A service account is neither.
- **PRM-052** · built · **A restriction only closes.** A listed group that no longer exists stays on the list and matches nobody, so the page stays closed. The same goes for a disabled person. An empty list is no restriction, so lifting is an act of its own and is audited.

## 9.3 Restrictions

- **PRM-053** · built · A view restriction applies to the page and everything below it. An edit restriction applies to that page only.
- **PRM-054** · built · Several view restrictions on one chain all count. A person needs to match each of them. They add up as an AND, and that cannot be written as one list.
- **PRM-055** · built · A restriction can only narrow. Being listed without space access gives nothing.
- **PRM-056** · built · "Edits" are: edit, archive, delete, add a file, delete a file. Creating a page below, commenting and managing restrictions are not edits. So a space administrator can lift an edit restriction they are not on, as long as they can see the page.
- **PRM-057** · built · Managing restrictions needs view on the page. A space administrator who is not on a view restriction cannot see that page, and has no bypass. Only recover access reaches it.
- **PRM-058** · built · Nobody locks themselves out: a view restriction that would hide the page from its setter is refused. Wave F adds the person to the list by themselves.
- **PRM-059** · built · Anyone who can view a page can see that it is restricted, and who is on the list.

## 9.4 States

- **PRM-060** · built · An unpublished page is reachable by its creator alone. Handing it to someone is not possible. Wave D has one exception: a person named in an approval round may read its frozen copy, when every other check passes (`mayReadRound`). That is a second path, so it joins the matrix (PRM-152).
- **PRM-061** · built · In an archived space only view, export and *administer* work. Personal marks still work, because they change nothing others see: favourites, watches, read marks. A reaction is seen by others, so it does not work there (GAP-021).
- **PRM-062** · built · An archived page can be read, exported and deleted. It cannot be edited, moved, labelled or commented on. Lists leave it out, and its address keeps working.
- **PRM-063** · built · A trashed page or space can only be reached through the trash, and PRM-070 applies.

## 9.5 Kinds of people

- **PRM-064** · built · **Guest:** a user type. Never in all members. Starts with nothing. Without `view_profiles` a guest cannot browse people.
- **PRM-065** · partly · **Service account:** a user type for automation. Built: the type, and the rule treats it as neither member nor guest. New: it cannot log in with a browser, and works through tokens.
- **PRM-066** · built · **Disabled:** nothing works, and every session ends. **Expired** (wave F) means the same from the date on, in `LoadUser` and in the SQL, before any job has run.
- **PRM-067** · built · **Personal space:** an ordinary space, with an ordinary grant for its owner. `Evaluate` has no branch for it. Others only see it through a grant. Recover access works here as elsewhere, audited, and the owner is told.

## 9.6 Moves and copies

- **PRM-068** · built · A move needs edit on the page, and create in the space. A move that would drop an inherited view restriction also needs *manage restrictions*.
- **PRM-069** · built · A move to another space also needs view and create in the target. It is refused when the branch holds anything the person cannot see. The home page cannot move.
- **PRM-071** · built · **A copy must follow the same check as a move.** A copy keeps own restrictions, and loses inherited ones without a word, which is not what "keep restrictions" promises (GAP-003).

## 9.7 Trash and recovery

- **PRM-070** · partly · The trash of a space belongs to its administrators. Built: list, restore, purge with *administer*. Missing: the rule of the object, so an item on a page they may not see must not show its title, and must not be purged by hand (GAP-001).
- **PRM-072** · built · The person who deleted a page can undo it for 60 seconds. Nobody else can, and after that it belongs to the trash.
- **PRM-073** · built · **Recover access:** an administrator gives themselves *space administrator* on a space, or adds themselves to every restriction on a page and above it. A reason is required. It is always audited. Wave F adds step-up, and tells the author.

## 9.8 Other objects

Each rule is "the rule of the page, and then this".

- **PRM-074** · built · **Template:** `Evaluate` refuses it to everyone, and the `templates` module holds its rule: a space template needs view on that space, using one needs create, an instance template is open to whoever can create somewhere, managing needs *administer* or *administrator*. Its files need a route of their own (GAP-022).
- **PRM-075** · built · **Post, folder, link entry:** the rule of a page. A folder can hold a restriction. A post has no parent, so it only has its own.
- **PRM-076** · built · **Label:** adding needs edit. A label is only seen through a page the person may see, and counts only count those.
- **PRM-077** · built · **Comment:** one function decides whether a person may see a comment: view on the page, not in the trash, and for `editors_only` also edit on the page. Every route, count, feed, notification and search uses it.
- **PRM-078** · built · Editing a comment: its author with `comment.add`, or a space administrator who can see the page.
- **PRM-079** · built · Resolving and reopening: `page.edit`.
- **PRM-080** · built · An `editors_only` thread opens at publishing only when its quote is in the published text. Otherwise it stays closed, or loses its quote (GAP-006).
- **PRM-081** · wave C · **Reaction:** needs view on the target, and that the target is not archived. Who reacted is shown to whoever can see the target.
- **PRM-082** · built · **Mention:** a person without access gets nothing, and no title. The writer is told.
- **PRM-083** · built · **Seeing a person.** `perm_visible_people`: with `view_profiles` everyone but service accounts. Without it, those they share a space with through a direct or group grant, because all members and all guests do not count. Names on content are always shown: it is the profile, the directory and the pickers that follow this rule. Until wave F lands, only direct grants count.
- **PRM-084** · built · **Task:** shown through the page. Ticking needs edit, and makes a silent version.
- **PRM-085** · built · **Notification:** checked at fan-out, when shown and when mailed. It holds ids. Nobody hears about what they cannot see, also when they are still a watcher.
- **PRM-086** · built · **Watch:** own watches are personal. Who watches a page is for a space administrator who can see that page.
- **PRM-087** · wave C · **Share:** never grants anything. A person without access gets no notification, and the sharer is told.
- **PRM-088** · built · **Asking for access:** a hidden page stays "not found". Only a space that turns the feature on offers a button, which reveals that something is there, and never what. Allowing changes a restriction or a grant, and only someone who may do that can allow it. When the blocking restriction sits above the page, allowing opens more than this page, and the dialog must say so (GAP-007).
- **PRM-089** · built · **Search:** only through the rule of the object. Comments in results follow PRM-077. A spelling suggestion comes only from titles the person may see.
- **PRM-090** · built · **Feed:** versions and comments on pages in `perm_listed_pages`, without silent versions, and with counts that follow PRM-077.
- **PRM-091** · built · **File:** the rule of its page, read from the file's own `page_id`, whatever document points at it. Inline or as a download makes no difference. Downloads are audited.
- **PRM-092** · built · **Dynamic block:** one route answers as the reader. A document stores settings, so a block can never hold a result. An include checks every level. Text in an include is indexed at its source only.
- **PRM-093** · built · **A link's state:** visible, deleted, or unavailable. Hidden and missing look the same. "Deleted" only shows to someone who could have seen the page, which needs the rule without its trash check, as a named variant of `Evaluate` (PRM-153).

## 9.9 Copies that leave

- **PRM-094** · partly · **Export:** needs `space.export`, and holds published versions of pages the person may see at that moment. Built: Markdown, as a stream. Wave G: a file that lives 24 hours for the person who made it, checked per page again at download (GAP-008).
- **PRM-095** · wave G · **Bundle:** needs *administer* and *export*. It holds everything, so a page the person cannot see stops it. On import a restriction that cannot be matched stays closed, as for Confluence, and grants to all members are shown in the report, because the people differ (GAP-011).
- **PRM-096** · built · **Public site:** no person's rights decide what the world sees. A page is on a site when it was chosen by hand, is published, and has no view restriction on or above it.
- **PRM-097** · built · What a chosen page refers to must meet the same test. Dynamic blocks, includes, cards and tags are built as a reader who may see only the pages on that site. A person's name appears only when the site says so, and a mention is otherwise left as neutral text (GAP-010).
- **PRM-098** · built · One function says whether a page may be public. It runs on every event that can change that: a restriction on the page or above it, a move, an archive, a delete, a version removal, the space archived or deleted. A page that fails it leaves at once (GAP-015).
- **PRM-099** · built · A public link follows PRM-096 to PRM-098, for one page and one version.

## 9.10 Other callers

- **PRM-100** · built · **API token:** acts as its owner, and the rule runs for that person. Scope limits it further. A disabled or expired owner stops every token, because `LoadUser` decides.
- **PRM-101** · built · **Rule (automation):** acts as an account, through the same module methods. Whatever it writes or sends must be allowed for whoever reads it, not only for the account (GAP-016).
- **PRM-102** · built · **Webhook:** ids, type and time. Titles only on request. An event only goes out when the endpoint's owner may see the page at delivery.
- **PRM-103** · built · **Analytics:** `analytics.view`, and every list through `perm_visible_pages` for the person looking.
- **PRM-104** · built · **Presence:** only to people who can see the page, and not for those who turned it off.
- **PRM-105** · built · **Editing together:** the service asks the API for each connection, again every minute, and on a change of rights it closes the connection. Publishing stays in Go, through the gate. A person with view only cannot connect.
- **PRM-106** · built · **Administrators and content:** no route gives an administrator a document. They see spaces as metadata. Titles do reach them through the audit log (For the product owner to decide, 1).
- **PRM-107** · built · **Audit of a space:** a space administrator sees their space. A row about a page they may not see shows the action and the time, no title and no details.
- **PRM-108** · built · **Why can or can't I:** space administrators who can see the page, and administrators. It runs the real evaluation for the other person.
