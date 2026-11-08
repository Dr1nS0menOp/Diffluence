# 05.12: Gaps and contradictions

Found by reading blueprint 03, specs 008 to 016 and the code on 2026-11-05. For waves C, D and F the branches of that day were read as well, so a line says what was seen there. These numbers never change either. A gap is closed by a change and a test, and the line then says so.

**Severity:** *High* means a person can see something they may not see, or a restriction can be lost, or a promise to the product owner or a data subject does not hold. *Medium* means the model is wrong or will break at a merge. *Low* means a small signal, or an unclear rule.

## 12.1 Can show what a person may not see

| Number | Severity | Where | What is wrong | Fix |
|---|---|---|---|---|
| **GAP-001** | High | Built. `trash.ListSpace`, `authorize` | The trash of a space lists the title of every deleted page and the name of every deleted file, also on pages hidden from that space administrator. They can also restore and purge those. Administrators of a space do not read content by default, and a purge destroys what they cannot see. Wave D's search in the trash makes it easier | List through `perm_hidden_pages`, and show such an item as "A restricted page" without its title. Restore stays. A manual purge of a hidden item is refused, and retention still works. Wave C's comment items follow the same rule |
| **GAP-003** | High | Built, and still so in wave D. `Copy`, `copyOne` | A copy takes direct restrictions only. A page under a restricted parent copied elsewhere, or to another space in wave D, loses what it inherited, also with Keep restrictions on. A person who can read the page could retype it, but this happens by accident, and says the opposite | Run the check of a move: lost restrictions need *manage restrictions*, and the dialog says so. Several levels cannot become one list (PRM-054), so an AND cannot be copied, only refused |
| **GAP-006** | High | Wave C. `ReleaseEditorsOnly`, C2 | A thread made while editing holds a quote from a draft. Publishing opens every such thread to all readers. When the text was removed before publishing, the words stay in the list as "no longer in the page". So a sentence that was never published reaches readers. It also shows words from one person's private draft to other editors, against PAG-013 | At publishing, open a thread only when its quote is in the published text. Otherwise keep it `editors_only`, or drop the quote. The product owner decides the second half (below) |
| **GAP-010** | High | Wave G, D1 and G15 | A public site has no person, and no rule for what refers elsewhere: a dynamic block, an include, a card, a tag, a task with a person. `G2` says blocks show what the exporting person sees, which is the wrong answer for the world. A mention prints a colleague's name, and the scrub only looks for addresses | PRM-097: build as a reader who may see only the pages on that site. Names only when the site chooses them |
| **GAP-015** | High | Wave G, D18 and G16 | A page leaves a site when it gets a restriction or is deleted. That misses a restriction on a page above it, a move below a restricted page, archiving the page or the space, deleting the space, and removing a version with a secret | PRM-098: one function, run on all those events. The test walks each event |
| **GAP-016** | High | Wave H, D11 and D13 | A rule fills `{page.title}` and `{found.title}` as its account sees them, and can send that to a group. The check at notification time looks at the triggering page, not at a page found by a lookup. So a person can read a title they may not see | Each recipient must pass the rule for every page a message refers to, or gets no message. A rule that writes into a page warns about this in its test run |
| **GAP-009** | Medium | Wave H, H6 | The run log holds what the rule did, as its account saw it. Any space administrator reads it, also one who is not on a restriction | Store ids, and resolve them at reading for the person looking |
| **GAP-023** | High when built | Waves C and D. ORG-024 | Searching comments is in scope of wave D, and wave D's search has no comments yet, because the table is wave C's. When it comes it must follow PRM-077, or `editors_only` and trashed comments show in search | One function for comment visibility, used by search. Part of the list walk (PRM-171) |
| **GAP-007** | Medium | Wave C, D18 | Allowing access adds the person to the restriction that blocks them. When that sits on a page above, it opens all of that branch. With two levels both must change. The answer must come from someone who may manage both | The dialog lists each level and what it opens. It is refused when the person cannot manage all of them |
| **GAP-024** | Low | Wave D, D12 | The filter "links to pages that are gone" can tell a hidden page from an absent one | Treat what the person cannot see as gone |
| **GAP-025** | Low | Built. `Restrictions` | Everyone who can see a page sees the names on its restriction, also guests without `view_profiles`, and those people need not share a space with them | Accept for people, because it explains why colleagues cannot find a page. Names of groups can stay. Say so in PRM-059 |
| **GAP-013** | Low | Built. `Tree`, `reposition` | `position` has gaps where a row is hidden, so it shows that something is there. `before_id` answers differently for a hidden sibling and an unknown id | Send a rank, not the stored number. Same answer for both |
| **GAP-014** | Low | Built. `Delete`, wave D `moveToSpace` | "This page has pages below it that you cannot delete" reveals hidden pages, while the impact dialog counted only what shows. NAV-005 says the wording never reveals that | Accept as a known exception: it gives no title or number, and the alternative is deleting what a person cannot see. Say so in NAV-005 |
| **GAP-030** | Low | Wave C, `PUT /users/{id}/follow` | A person without `view_profiles` can test whether an id exists | Follow PRM-083 |

## 12.2 Promises that do not hold

| Number | Severity | Where | What is wrong | Fix |
|---|---|---|---|---|
| **GAP-004** | High | Waves C and F. `cleanMention`, F12 | A mention stores `label`, the name at that time, in `page_versions.document`. Versions cannot be changed. So after anonymising the name stays in every version, in the search text and in comments. Spec 013 says the name is left in no table but the audit log, and that its test walks every column. That test fails as soon as a mention exists, or passes only because it has none | Store the id only, and fill the name when a document is read, as D6 of spec 010 already does for showing it. This must land **before the first real mention is published**, because it cannot be undone in stored versions. Search then needs the name from `page_mentions` |
| **GAP-018** | Medium | Waves C and F. `Anonymise` | The list of tables is written by hand on a branch that does not know wave C: `follows`, `watches`, `comment_seen`, `notifications`, `mail_queue`, and from main `favourites`, `recent_pages`. The column walk only looks for text, so it misses rows that hold just an id | Add them at the merge. Extend the walk to ids (PRM-176) |
| **GAP-026** | Medium | Wave D, D30 | Removing a version for a leaked secret removes one row. The words can live on in comment quotes, drafts of others, a later version, mail, exports, bundles, site builds and backups | The dialog says so, and offers the list of later versions and quotes that hold the words. Scrubbing a quote is a new, audited action |
| **GAP-012** | Medium | Built. `keepsAnAdmin`, `DeleteGroup` | "A space keeps an administrator" counts a grant, not a person. A disabled person or an empty group passes. `DeleteGroup` removes grants with no check at all. Wave F adds a refusal for people and a list of spaces with no active administrator, and a provider's sync cannot be refused | Keep the list as the real answer, and change SPC-053 to say what is promised |
| **GAP-019** | Medium | Built. `pages.parent_id`, `page_tree` | Deleting a parent row sets the child's parent to null, and the closure rows go. A child would silently lose every inherited restriction. Only the purge deletes pages, and it takes the branch, so this is a promise of code | Make the delete fail in the schema (`RESTRICT`, with the purge deleting from the leaves up), or add the test PRM-177 |
| **GAP-011** | Medium | Wave G, G14 | Failing closed on an unmatched restriction is written for Confluence only. A bundle needs it too. A grant to all members means other people in another instance | Same rule, and the report shows such grants |
| **GAP-008** | Medium | Wave G, D5 | The check "again at download" cannot be done per page, because `export_runs` has no list of what the file holds | Add `export_run_pages`, and refuse the file when one is no longer visible |

## 12.3 Things that will collide

| Number | Severity | Where | What is wrong | Fix |
|---|---|---|---|---|
| **GAP-005** | Medium | Waves C and D | Wave C keeps switches in `spaces.features`. Wave D keeps them in the table `space_features`. Both are built. Comments and reactions then have two truths | Decide at the merge. Advice: the table of wave D, with wave C's readers moved to it, and the column left to a contract migration |
| **GAP-002** | Medium | Waves C and D. `moveToSpace` | A move rewrites `pages` and `attachments`. `comments`, `page_tasks`, `notifications` and `access_requests` have their own `space_id`, which wave D cannot know yet | Add them at the merge, with a test that compares every `space_id` column with the page |
| **GAP-021** | Low | Wave C. `SetReaction` | The route needs view, which works in an archived space, so a reaction changes something others see | Refuse in an archived space and on an archived page |
| **GAP-022** | Medium | Wave D. Templates | `Evaluate` refuses a template page, so `GET /attachments/{id}/content` refuses its files too. An image in a template cannot show in the gallery or the template editor, unless someone adds a bypass | A route of the `templates` module with its own rule, and a test |
| **GAP-028** | Low | Wave D. `mayReadRound` | The exception finds its rule by the name `published_or_own_draft`. It fails closed when that name changes, which is safe, and quietly breaks approvals | A named variant of the rule (PRM-153) |
| **GAP-027** | Medium | Wave H, part 5 | The collab service stores a shared document, with no table, retention, removal or export in the model. Removing a version cannot reach it. Private drafts end for those pages (ADR-026) | Closed by section 5.9 of `05-working-together.md` |
| **GAP-029** | Low | Wave G, G8 | "Keep the name as a former author" has no place: `author_id` is an id, and `type` has no such person | A person row of a new type, or a text column. Decide in that wave |

## 12.4 Where the old text and the code differ

- **GAP-017** · The old blueprint 05 and the code:

| Old text | Code | Now |
|---|---|---|
| `permissions.Can(user, action, object)` | `Evaluate`, `RequirePage`, `RequireSpace` | Fixed in 05.9 |
| Permissions are cached and invalidated by events | No cache | PRM-047 |
| One `grant` table with a scope | `global_grants` and `space_grants` | Fixed in 05.3 and 05.2 |
| `attachment` holds object key, hash and key id | These sit in `attachment_versions` | Fixed in 05.4 |
| Every row with content has `origin_instance_id` | Not all do | DAT-002 |
| Files are encrypted (blueprint 06) | Built in spec 032; the key sits in each file's header, so `encryption_key_id` stays empty | Closed |
| Files come from a separate origin (blueprint 06) | Same origin, locked down with `sandbox` and `nosniff` | For the study of blueprint 06 |

- **GAP-020** · Medium · The matrix does not hold a service account, `use_product` through a group, a restriction on the middle level, two subjects on a list, a space-level action with no page, or a creator who differs from the page's creator. See PRM-155, 158, 161, 166.

## 12.5 For the product owner to decide

1. **Titles in the audit log.** Decided (ADR-029): the id of the page with "(Censored title)", on the screen and in exports. The question was: an administrator cannot open a restricted page, but reads its title in the audit log and in exports of it. Advice: keep this, because an audit that cannot say what happened is of little use, and write it down: deliberate difference 2 is about documents, not about titles. The alternative is storing only ids, and then a purged page can never be named.
2. **The trash and hidden pages (GAP-001).** Advice: a space administrator sees "A restricted page", can restore it, and cannot purge it by hand.
3. **Copying a restricted page (GAP-003).** Advice: treat it like a move, so losing an inherited restriction needs *manage restrictions*.
4. **Threads made while editing (GAP-006).** Should other editors see a thread on words from someone's private draft at all? Advice: yes for now, because COL-019 asks for it, but a quote that never reached a published version stays with the editors.
5. **Names in stored documents (GAP-004).** Advice: store only the id. This is cheap now and impossible later.
6. **Names on public sites (GAP-010).** Advice: a mention prints as neutral text, unless the site shows names.
