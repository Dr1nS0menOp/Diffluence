# 05.10: How each list stays safe

A single object is guarded by `Require...`. A list is where hidden things slip through, because it reads many rows at once. This chapter names every list, what it reads, and what guards it. A new list adds a line here.

**The three questions for every list:**

1. Does each row come through `perm_visible_pages`, or through the function that replaces it?
2. Does a count, a sum or an order say something about rows that were left out?
3. Is there text in the row that was copied from somewhere else, so that it has its own rule?

## 10.1 Lists of pages

| Number | Status | List | Guard |
|---|---|---|---|
| **PRM-120** | built | Tree of a space | `perm_visible_pages`. `position` gaps show that a row is missing (GAP-013) |
| **PRM-121** | built | Recent, Worked on, Drafts | `perm_visible_pages`, own rows only |
| **PRM-122** | built | Favourites | `perm_visible_pages`, own rows only |
| **PRM-123** | built | Search and quick search | `perm_visible_pages`, published only, archived spaces left out of quick search. The snippet comes from a row that passed |
| **PRM-124** | built | Breadcrumbs | Ancestors of a page the person may see. Every view restriction above was passed, so their titles are allowed |
| **PRM-125** | built | Incoming and outgoing links, child pages | `refJoins`, which holds `perm_visible_pages` |
| **PRM-126** | built | Delete impact | Counts what the person can see. A refusal can still tell that more is there (GAP-014) |
| **PRM-127** | built | Overview of a space, directory counts | Count only visible pages |
| **PRM-128** | built | Recent spaces, favourite spaces | `perm_spaces`, active only |
| **PRM-129** | built | Export | `ForExport`, the same function, and files of those pages only |
| **PRM-130** | built | Files in quick search | `perm_visible_pages` on the file's page |
| **PRM-131** | partly | Trash of a space | *administer* only. No rule of the object (GAP-001) |
| **PRM-132** | built | Move dialog | The tree, so the same guard |

## 10.2 Lists of people

| Number | Status | List | Guard |
|---|---|---|---|
| **PRM-133** | built | Directory (pickers) | Route needs `view_profiles`. It returns the address, which wave F limits |
| **PRM-134** | built | People in quick search | `view_profiles`, and a name and id only |
| **PRM-135** | built | Administrators of a space | Whoever can see the space. Names only |
| **PRM-136** | built | Names on a restriction | Whoever can see the page. Names only (GAP-025) |
| **PRM-137** | built | Mention candidates | Without `view_profiles`, people with a direct grant in this space. `can_view` is only answered for a page the asker can see |
| **PRM-138** | wave C | Who has access | Whoever can see the page. All members is one line, at most 50 names |
| **PRM-139** | built | People directory, profiles, cards | `perm_visible_people` |

## 10.3 Lists from later waves

| Number | Status | List | Guard |
|---|---|---|---|
| **PRM-140** | wave C | Comments, threads, counts in the byline | PRM-077 |
| **PRM-141** | built | Notifications and the count | PRM-085. The count must follow the hidden rows, or it says that something exists |
| **PRM-142** | wave C | My tasks | Through the page. A task's text is a copy, so a hidden page takes its tasks along |
| **PRM-143** | wave C | Feed, Popular | PRM-090 |
| **PRM-144** | wave D | Labels, suggestions, counts, label pages | Through visible pages only. A rename by a space administrator also touches pages they cannot see, so its answer must not say how many |
| **PRM-145** | built | Content screen | Only pages the person may see. A link to a page they cannot see counts as "gone", so the filter cannot tell hidden from absent (GAP-024) |
| **PRM-146** | built | Archive screen, blog lists | `perm_visible_pages` with the right kind |
| **PRM-147** | wave D | Search filters and spelling | PRM-089 |
| **PRM-148** | built | Every dynamic block, reports, file lists | One route, one filter, and totals only for space administrators |
| **PRM-149** | wave H | Insights, rule logs, webhook logs | PRM-103, GAP-009 |

## 10.4 Side doors

Things that are no list, and still tell something.

| Door | What it tells | Answer |
|---|---|---|
| Unique key on `spaces.key` | That a key exists | Only for those with `create_space`, and never more than that |
| Unique file name on a page | Only within a page the person can see | Fine |
| `before_id` in a move | Whether an id is a sibling | Needs edit and create, and knowing the id. Same answer for hidden and absent would be better (GAP-013) |
| Conflict on delete | That something is below | Known exception (GAP-014) |
| UUIDv7 in a link | When the page was made | Accepted (DAT-001) |
| Timing | Hidden and absent take a different path | Not measured. Phase 3 |

PostgreSQL's own documentation names these as covert channels for row security too: unique and foreign key checks see every row, whatever a policy says. That is one reason not to rely on row security alone (05.11, section 11.6).
