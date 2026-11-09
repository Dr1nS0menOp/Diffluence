# Spec 040: the rest of blueprint 05

Status: built on branch `wave-p5`.
Phase: 3 (blueprint 08).

## Problem

Blueprint 05 holds the model of data and permissions. After spec 035 and spec 036, 44 of its numbered lines were still not `built`: gaps in the permission matrix, nine finer permissions the product owner was promised after asking whether one screen of permissions was all there is, a few rules marked with waves that landed long ago, and the open gaps of chapter 12. Blueprint 04 adds three lines that belong here: faster lists of visible pages (ARC-205), counts per person (ARC-206), and a short memory for permission facts (ARC-231).

## The study

Every line of blueprint 05 that was not `built` was checked against the code and its tests, as spec 035 did: a line counts as built when a test proves its behaviour, or when this wave built it, failing test first. A line with neither stays as it was. The table holds the lines that were open; the counts are at the end.

### Lines found built, with their evidence

| Line | Evidence |
|---|---|
| DAT-005 | `TestAMoveTakesEveryRowOfTheBranchAlong`: a move rewrites the `space_id` of comments, tasks, notifications and access requests in one transaction |
| DAT-013 | `perm_listed_pages`, and the matrix compares it with the rule for every case |
| DAT-026 | `user_preferences.notifications`, `TestPreferencesDecideWhenMailGoes`, `TestDigestGroupsNewThingsAndStaysAwayWhenThereAreNone` |
| PRM-065 | `TestServiceAccountsWorkOnlyThroughTokens`; the matrix now holds a service account |
| PRM-070, PRM-131 | `TestTrashNeverNamesWhatIsHiddenFromThePerson`, `TestPurgingAPageIsRefusedWhenAPageBelowItIsHidden` |
| PRM-081 | `TestAReactionChangesNothingOnAnArchivedPageOrInAnArchivedSpace`; `reaction.set` is in the matrix |
| PRM-087 | `TestSharingOnlyNotifies` |
| PRM-094 | `TestExportLeavesOutWhatThePersonCannotSee`, `TestOnlyTheOwnerDownloadsAnExport`, and `export_run_pages` read again at download |
| PRM-095 | `TestABundleStopsOnAPageThePersonCannotSee`, `TestAnUnmatchedRestrictionStaysClosed`, `TestAGrantToAllMembersIsShownInTheReport`, and now `TestABundleStillNeedsExport` |
| PRM-138 | `TestWhoHasAccessShowsHowAndExportsSafely`, and the list walk |
| PRM-140, 142, 143, 144, 147 | The list and count walks of this wave, beside `TestCommentsForEditorsDoNotCountForReaders`, `TestTasksFollowTheDocument`, `TestTheFeedShowsOnlyWhatAPersonMaySee`, `TestLabelsAreCleanedAndNeverRevealHiddenPages` and `TestCommentSearchFollowsTheRuleOfAComment` |
| PRM-153 | `approval.read` is a named variant of the rule (`TestApprovalReadIsViewWithoutTheDraftRule`), which also closes GAP-028 |
| PRM-159 | The matrix holds Manager, and manage access without *administer* |

### What this wave builds

| Line | What | Load budget (ARC-238) |
|---|---|---|
| PRM-155, 158, 161, 166, GAP-020 | The matrix gains a service account, `use_product` through a group, `view_profiles`, two groups; two subjects at once, delete own only, delete any only, comment rights, manage restrictions or manage access without *administer*, a second group; view restrictions on the middle level, two or three on one list, three levels at once, a group that no longer exists, a disabled person; `space.view` with no page; a file or comment whose creator differs from the page's; folders, links and databases | Test only |
| PRM-173 | The SQL half pairs each value added here with the plain values of the other dimensions, in fewer states, and writes its rows in one batch per person. `Evaluate` keeps the full product: 10,046,790 cases in about 10 seconds, one person per parallel subtest. The SQL half compares 93,636 pages in about 90 seconds; before this wave a smaller set took more than six minutes | Test only |
| PRM-171, PRM-172 | The list walk and the count walk (`TestNoListShowsAHiddenPageOrWhatHangsOnIt`): a hidden page with a label, comment, task, file, link, mention, reaction and an include; every GET of the API, every server block and the searches are called as a reader who may not see it. No answer holds a planted word or its id, and every number is the same after the page is purged. A GET route with a part the walk cannot fill fails the test until it is listed with a reason | Test only |
| PRM-174 | `TestNoPlaceChangeOpensAPage`: move within and out of the space, copy, archive and back, delete and undo, delete and restore from the trash. Import and bundle were already proven by `TestAnUnmatchedRestrictionStaysClosed` and `TestABundleStopsOnAPageThePersonCannotSee` | Test only |
| PRM-176 | `TestAPurgedPageLeavesNothingBehind` walks every table through `information_schema` after a purge: the id in every `uuid` column, the planted word in every text and JSON column. The anonymising half was built in wave F (`TestAnonymisingLeavesTheIdOnlyWhereThatIsDecided`) | Test only |
| PRM-207 to PRM-214 | Eight permissions split off broader ones (below) | No query more: the same check with another name |
| ARC-231, PRM-047 | Facts of a person and a space kept between reads (below) | One query per read request instead of four or more for the facts; one row update per statement that changes permissions |
| PRM-204 | Bundles of format 3. Older bundles give the permissions split off after them | No query |
| PRM-176 (found by it) | Reactions go with their page or comment, through a trigger, also when a space is purged. The walk found the reaction of a purged page left behind | One delete per deleted page or comment |
| ARC-205, ARC-206 | `perm_hidden_pages` hands the restricted pages to `page_tree` as an array, so their branches come through the index; `perm_listed_pages` reads pages once instead of joining them to `perm_visible_pages`. The matrix judges both | Fewer rows read; see the measurements |

### The finer permissions

The product owner asked for permissions that can be given apart. Each split follows spec 036: whoever held the broader permission gets the finer one in the migration, so nobody gains or loses anything; a role made afterwards can hold one without the other.

| Line | Permission | Name on screen | Split from | What it guards |
|---|---|---|---|---|
| PRM-207 | `space.labels` | Labels | `space.administer` | The fixed labels of a space, renaming a label |
| PRM-208 | `space.statuses` | Statuses and reviews | `space.administer` | The page statuses and the review period |
| PRM-209 | `space.approvals` | Approval rule | `space.administer` | Reading and setting the approval rule and its approvers |
| PRM-210 | `page.move_out` | Move pages to another space | `page.edit` | Moving a branch to another space |
| PRM-211 | `space.bundle` | Bundle the space | `space.administer` | A bundle of the whole space; it still needs export |
| PRM-212 | `page.watchers` | Manage who watches | `space.administer` | Who watches a page, and adding or removing someone else |
| PRM-213 | `space.invite_guests` | Ask for guests | `space.manage_access` | Asking for a guest in the space; *administer* implies it, as it implies managing access |
| PRM-214 | `space.look` | Look and shortcuts | `space.administer` | Shortcuts, picture, title alignment, home page |

Choices:

| # | Question | Choice | Why |
|---|---|---|---|
| D1 | Where does each new permission work? | Those split off *administer* work where it worked: in an archived space and on an archived page. `space.invite_guests` works in an archived space, as managing access does. `page.move_out` is an edit: an edit restriction stops it, and it does not work in the archive | The split changes no decision. `splitOffAdminister` reads `SplitFrom`, so a new split off *administer* follows by itself |
| D2 | Copying out of a space | Stays as it was: view where the page is, create where it goes | A copy takes nothing from the space. Asking `page.move_out` would take copying from people who may do it today |
| D3 | Name and description of a space | Stay with *administer* | They say what the space is, not how it looks |
| D4 | One list of pairs | `permissions.SplitFrom` in Go, `rbac_split_off` in SQL, `exchange.heldSince` for bundles. `TestTheSplitIsTheSameInGoAndSQL` and `TestEverySplitPermissionKnowsTheBundleVersionThatHoldsIt` keep them equal | A pair known to one side only would move access in an upgrade or an import |
| D5 | `post.create` (PRM-206) | Not built | It waits for the product owner: turning a page into a post would need it too |
| D6 | Export per format (rest of PRM-211) | Only the bundle is apart | PDF and office files apart from Markdown would take formats from everyone who exports today. The product owner decides |
| D7 | `space.databases` (PRM-215) | Not built | A database is a page of its own kind, guarded by `page.create` and `page.edit`. A permission of its own needs the kind of a page in `Evaluate` first |

The screens follow: the role editor shows the eight under pages, people and space with their names and one line each; a space's settings show a section to whoever holds what guards it; the rules section shows the review period, title and picture, and approvals each to its own holder; the watchers on a page show to `page.watchers`; the bundle button shows to holders of bundle and export. The route table declares the new permissions. Spec 036 had sent the section on rules to `space.automation`; it now asks for what the section holds.

### The migration proof

Migration 0040 gives each finer permission to every space role and old permission set that held the broader one, and the role trigger brings the grants along. `rbac_adopt` now adds every split-off permission when single grants become roles. `TestAdoptingGrantsChangesNoDecision` builds single grants of every shape (users, groups, all members, all guests; restrictions; drafts; archived spaces) without any split-off permission, adopts them, and compares every decision of `Evaluate` and the SQL lists: 949,740 decisions, 65,268 of them allowed, 3,506 of those through a permission split off a broader one. None changed: what was allowed through the broader permission is allowed through the finer one, and nothing else appeared. `TestBuiltInRolesMatchTheTemplates` holds the built-in roles to the templates after the migration.

### Every way to space administration still gives the eight

Nobody may lose anything through this wave, also not someone given administration after the migration. `TestEveryWayToAdministerASpaceGivesTheFinerPermissions` and the bundle tests check each way the product gives it:

| Way | What happens |
|---|---|
| The built-in Space admin role: a new space, a personal space, handing a personal space over, importing a bundle as its new owner | The role holds every space permission, the eight included; `TestBuiltInRolesMatchTheTemplates` holds the role to the template |
| The older grants route (`PUT /spaces/{id}/grants`) and the access screen that uses it | `permissions.WithSplitOff` completes a list written before a split: a broader permission without any of a spec's finer ones gets all of that spec's, as its migration gave them. A list that names one of them is taken as written. Found by this check and fixed here |
| Bundles of format 1 and 2 | `permissionsOfGrant` adds what was split off after their format |
| Roles that held administration, and own sets, at the migration | `rbac_split_roles` adds the eight, and the role trigger brings every holder's grants along |
| A role made or changed after this wave, in Administration > Roles | Holds exactly what it is given, the eight shown beside administration with their names. This is the one way to give administration without them, and it is what the split is for |

The instance tools and the imports of Markdown and Confluence give no space permissions.

### Facts kept between reads (ARC-231)

A read request already kept the facts of a person and a space for its own length (ARC-197). Now a node keeps them between requests too, under the number in `permission_generation`:

1. Statement triggers on `users` (type, status, end date), `group_members`, `global_grants`, `space_grants` and `spaces` (status) add one to the number. Roles reach grants through their own triggers, so a change to a role counts too.
2. A read reads the number once, first, then the facts. Facts are kept under the number they were read after, so they are never older than it.
3. A newer number empties the memory. A request that still holds an older number neither reads nor keeps.
4. Nothing is kept longer than 30 seconds. An account that passes its end date stops at that moment, also while kept.
5. A write has no memo, so it never reads kept facts: a grant it makes counts for its own next check.

Restrictions are not kept: they are read with the page, every time. The first level of a tree (the rest of ARC-231) is not kept either: it changes with every page that is made, moved or saved, and counting those would empty the memory on every edit.

Tests, failing paths first: `TestAChangeEndsKeptFactsAtOnce` (two services as two nodes: a revoked grant, a left group, an archived space, an end date, a disabled account and a lost `use_product` each count at the next request), `TestAWriteNeverReadsKeptFacts`, `TestAKeptAccountEndsAtItsEndDate`, `TestKeptFactsEndWithTheirGenerationAndTheirAge`.

### Measurements

Measured in the development container on a test database: 50 spaces of 2,000 pages, one page in 100 with a view restriction, half of them with the person on it through a group. Median of seven runs.

| Query | Before | After |
|---|---|---|
| All pages the person may see | 62 ms | 41 ms |
| Page counts of the space directory (ARC-206) | 215 ms | 95 ms |
| One space, joined to the list (56 callers use this form) | 53 ms | 36 ms |
| One page through the function | 17 ms | 3 ms |
| One space, as `IN (SELECT ... perm_listed_pages)` (5 callers) | 31 ms | 58 ms |

The last form got slower: the planner now reads the whole list before it narrows. It is the rarest form, and the others gained more; a caller that needs it fast can join instead.

Facts kept (ARC-231): 1,000 checks of a space, each its own request, took 1.03 s reading the facts every time and 0.19 s with kept facts (1,998 hits, 2 misses). A read request does one query for the generation instead of four or more for the facts.

### What stays open, and why

- **PRM-206** `post.create`: waits for the product owner.
- **PRM-211**, PDF and office files apart: waits for the product owner.
- **PRM-215** `space.databases`: needs the kind of a page in the rule, and a decision that a database should differ from a page.
- **PRM-197**: no code of this release writes `permission_sets`, but the release before spec 036 does, and starting it again is still the way back from an upgrade. The table goes in the first contract migration after a release with spec 036 has shipped.
- **PRM-156**, anonymised people in the matrix: not added, because anonymising leaves a disabled account, which the matrix holds. A page under an archived parent (PRM-163) is not a state of its own either: archiving takes the pages below along.
- **PRM-149**: insights, rule logs and webhook logs are for administrators and are not part of the walk of a reader.
- **DAT-099**: comments are searchable by their own rule; text inside files is not searched yet.
- **ARC-231**, the first level of a tree: see above.
- The gaps of chapter 12 that this study could not prove closed keep their text.

## Permissions and audit

The eight permissions above. The migration and the role trigger write no audit line, as in spec 036: nobody gains or loses anything. Giving a role with a new permission is audited as every role change is (PRM-188).

## Load budget

See the table of what this wave builds. The checks are the same in number; a read request does one query for the generation and reads the facts of a person and a space only when they changed or are older than 30 seconds.

## Acceptance criteria

- [x] The matrix holds every value of PRM-155, 158, 161 and 166, and the SQL half stays below two minutes.
- [x] No list shows a hidden page or what hangs on it, and no number counts it.
- [x] A place change opens no page to anyone who could not see it.
- [x] A purge leaves the id and the planted words in no table but the audit log.
- [x] Each of the eight permissions works alone, and *administer* or edit alone no longer holds it.
- [x] Nobody gains or loses anything in the migration.
- [x] A revoked grant stops at the next request, on every node.

## How the product owner tests it

1. Administration > Roles: make a role with only "Labels" and give it to someone in a space. They can fix the labels of that space, and see no other settings.
2. Make a role with "Edit pages" but without "Move pages to another space". Its holder can move a page within the space, not to another one.
3. Take a role away from someone who has a page open, and reload: the page is gone for them at once.

## Provenance

Own design, on blueprints 04 and 05 and specs 035 and 036.

## Counts of blueprint 05

Counted by a script over every numbered line and table row outside chapter 12, as in spec 035.

| | Built | Partly | Marked with a wave | New | Later | Total |
|---|---|---|---|---|---|---|
| Before | 292 | 6 | 17 | 20 | 1 | 336 |
| After | 327 | 4 | 1 | 3 | 1 | 336 |

Chapter 12: GAP-001, 002, 003, 004, 005, 011, 018, 020, 021, 022, 023, 024 and 028 now say which test closes them.