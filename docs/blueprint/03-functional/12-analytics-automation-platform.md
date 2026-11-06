# 03.12: Analytics, automation and the platform

## 12.1 Analytics: principles

Page views are personal data. They are not in the audit log (decided 2026-11-03), and they are a feature with its own switch.

- **PLT-001** · built · Analytics can be turned off for the instance or per space. When off, no views are recorded.
- **PLT-002** · built · **Privacy mode:** an administrator chooses between counts only (no names), or names for space administrators. The default is counts only.
- **PLT-003** · built · Moving to a stricter mode removes names from existing data. Going back never restores them.
- **PLT-004** · built · View data is kept for one year by default, and the period is a setting.
- **PLT-005** · built · A person's own views are left out of counts on their own pages.
- **PLT-006** · built · Reports never show anything about pages the viewer may not see.
- **PLT-007** · built · Who may see analytics is a permission per space.

## 12.2 Page insights

- **PLT-008** · built · Selecting the view count in the byline opens a summary: a chart of views and of unique readers over time.
- **PLT-009** · built · A period can be chosen: 7 days, 30 days, 3 months, this year, or a range.
- **PLT-010** · built · In name mode, a list shows who read it, when, how often, and which version they saw last.
- **PLT-011** · built · A reader counts as engaged after 20 seconds on the page, or as a brief visit.
- **PLT-012** · built · **Mentioned people:** whether people mentioned in the page have read it.
- **PLT-013** · built · **Links:** the most used links in the page, and the pages that bring the most readers here.

## 12.3 Space and instance insights

- **PLT-014** · built · **Instance:** active users, views, new pages, comments and searches over time.
- **PLT-015** · built · **Search insights:** the most used searches, and searches that found nothing, without names.
- **PLT-016** · partly · **Space:** popular pages, most active readers and contributors (name mode only), with full lists.
- **PLT-017** · built · A **content table** per space shows title, created, last changed, last viewed, comments, unique readers and views, and sorts on every column.
- **PLT-018** · built · A **people table** per space shows, in name mode, pages made, edits, comments and views.
- **PLT-019** · partly · Every table exports as CSV, and only active items are listed unless asked.
- **PLT-020** · built · **Stale content:** pages not viewed or changed for a chosen time, with a link to the Content screen (SPC-060).

## 12.4 Automation

- **PLT-021** · built · A rule is a trigger, conditions and actions, built in a screen with no code.
- **PLT-022** · built · Rules exist per space, or for the whole instance when made by an administrator.
- **PLT-023** · built · A rule acts as a chosen service account or as its owner, and can never do more than that account may do.
- **PLT-024** · built · Rules run in the background through the queue.
- **PLT-025** · built · **Triggers:** page published, edited, moved, copied, archived, deleted, commented on, labelled, status changed, owner changed; blog post published, commented on, labelled; attachment added or removed; task created or ticked; person mentioned; person added to or removed from a group; space created or archived; on a schedule; by hand from a page; incoming webhook.
- **PLT-026** · partly · **Conditions:** in a space or below a page, has a label, has a status, title or text contains, created by, older than, member of a group, plus and, or and not.
- **PLT-027** · partly · **Actions on pages:** publish a new page from a template, copy, move, archive, delete, restrict, add or remove a label, change status, change owner, add a comment, add or remove watchers.
- **PLT-028** · built · **Actions for people:** send a notification, send an e-mail, assign a task.
- **PLT-029** · built · **Actions outward:** call a webhook, post to a chat system through a webhook.
- **PLT-030** · built · **Lookups and values:** a rule can find pages, and use values such as the page title and author in later steps.
- **PLT-031** · built · **Run log:** each run lists what it did, what it skipped and why. A failing rule is reported to its owner.
- **PLT-032** · built · **Limits:** a loop cannot run forever. A rule cannot trigger itself, and each rule has a maximum number of runs per hour.
- **PLT-033** · built · **Templates for rules:** common rules ship ready to use, such as "remind the owner of pages not updated for a year".
- **PLT-034** · built · Rules can be turned off, copied, exported and imported.
- **PLT-035** · built · Every action of a rule is audited with the rule's name.

## 12.5 The platform

- **PLT-036** · built · Our own frontend uses only the public API.
- **PLT-037** · partly · The API is documented, versioned and stable, with OAuth scopes.
- **PLT-038** · built · Webhooks send events to outside systems, signed, with retries and a delivery log.
- **PLT-039** · phase 7 · Apps run as sandboxed frames on a separate address, then as WebAssembly blocks. A marketplace comes last.
- **PLT-040** · built · A **command line tool** uses the API, for scripted import, export and backups of content.
- **PLT-041** · new · The Create menu, the page menu, the sidebar and dynamic blocks all have defined places where apps can add entries.

## 12.6 AI

Later, and always optional (phase 6).

- **PLT-042** · phase 6 · An administrator chooses the provider, including a local model. Nothing leaves the instance without that choice.
- **PLT-043** · phase 6 · Summarise a page, a long comment thread or a changes since a last visit.
- **PLT-044** · phase 6 · Rewrite selected text: shorter, clearer, other tone, other language.
- **PLT-045** · phase 6 · Ask a question of the wiki, with answers that cite pages. Only pages the person may see are used.
- **PLT-046** · phase 6 · Suggest labels and titles, and extract tasks from meeting notes.
- **PLT-047** · phase 6 · An MCP server gives assistants access under the person's own permissions.
- **PLT-048** · phase 6 · Every AI action is audited, and can be turned off per space.

## 12.7 Linked instances

- **PLT-049** · phase 8 · An administrator links two instances, with mutual trust.
- **PLT-050** · phase 8 · Spaces, users and history can be moved or merged between them, for example after a merger. Stable ids make that possible.
