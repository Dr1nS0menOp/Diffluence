# Spec 073: Draft and published spaces

Study: [blueprint 09, draft and published spaces](../blueprint/09-enterprise/publishing/01-draft-and-published-spaces.md), chapters 02 to 15, lines PUB-001 to PUB-1517. Basis: [08-roadmap/02-enterprise-document-needs.md](../blueprint/08-roadmap/02-enterprise-document-needs.md) (RMP-013) and DOC-431. This is the second of the three enterprise features before 1.0 (ADR-038, spec 048 D14). It is a clean-room build: the behaviour blueprint 08 names was read in public documentation for behaviour only, and every requirement is stated in Diffluence's own terms (sources in the study's [sources.md](../blueprint/09-enterprise/publishing/sources.md)).

Decisions made on the product owner's behalf on 2026-11-14 are PUB-1401 to PUB-1430 in the study's section 1.3, and D1 to D12 below. Each was decided on the product owner's behalf on 2026-11-14, and may be overruled before the wave that uses it.

## Core idea

Two spaces of one installation are linked as a **pair**: writers work in the **draft space**, readers have the **published space**. Content crosses only by a **release** (the word chosen so that "publish" keeps its meaning of making a version, PUB-1401): one page, a branch or the whole space, always through a preview, or by an **approval** under document control. The published space is **managed**: nobody can write its content by hand, which the permission rule decides and a database trigger holds (PUB-220, PUB-224). A twin in the published space is an ordinary page on every read path, so reading costs nothing extra and the existing visibility rules keep drafts from readers without a new rule (PUB-203). One row per released draft page, the **page pair**, holds the facts that say what is out of step, kept in the same transaction as every change (PUB-340). Links are rewritten from draft ids to twin ids; a link whose target has no twin becomes its own text (PUB-510).

The needs of chapter 02 (PUB-100 to PUB-109) are not built on their own: each is carried by the lines of chapters 03 to 14 that it names, and is proven by their tests.

## Is it experimental? Yes

ADR-043 froze new parts for 1.0 and made optional parts experimental and off by default, each with one switch for the installation. This feature is built before 1.0 because the product owner asked for it (ADR-038), and it is an **experimental part named `publishing`, off by default** on new and upgraded installations (PUB-1415, PUB-1303). The reasons:
- it is a **new part** after the freeze, and optional: most installations will never pair two spaces;
- it adds a **second write path** into spaces (the release path) and a rule that takes editing away from people, both of which should run on real installations before they are on by default;
- an off part keeps what exists (ADR-043), and for this part that is safe: published spaces stay read-only, and ending a pair stays open (PUB-1304), so an administrator can always undo it.

Document control itself is not experimental; this part depends on it only in PB3, and works without any workflow in PB1 and PB2.

## Rules for every wave

Every wave follows these rules:
- **Tests first.** Start from failing tests, failure paths before the happy path (chapter 11 first where it applies).
- **Mutations.** Add a mutation for each new check.
- **Permissions.** Keep the permission matrix green and extend it with the space kinds draft, published and ordinary (PUB-012).
- **Leaks.** Every new route or output is rendered for a reader of the published space and searched for the draft space's id, key and name (PUB-108, PUB-620).
- **Audit.** Write an audit entry for every new action (PUB-900).
- **Load.** Measure on wave PF's real-data dump with its largest space paired (PUB-1427), never only on generated data.
- **Accessibility and design.** Meet chapter 13 and the design standard; every new screen has its approved pictures.
- **Off by default.** Every route answers 404 `experimental_off` while the part is off, except ending a pair (PUB-1304).

## PB1: the pair, the managed space, and releasing one page

**Scope:**
- PUB-010 to PUB-012;
- PUB-200 to PUB-203, PUB-210 to PUB-214, PUB-220 to PUB-224, PUB-230 to PUB-234, PUB-240;
- PUB-300 to PUB-304, PUB-310, PUB-311, PUB-320, PUB-321, PUB-330, PUB-332, PUB-340 to PUB-342, PUB-344;
- PUB-400, PUB-403, PUB-410, PUB-411, PUB-413, PUB-430 (for one page), PUB-431, PUB-433, PUB-450;
- PUB-500 to PUB-504, PUB-510, PUB-511, PUB-520, PUB-521, PUB-523, PUB-530 to PUB-534;
- PUB-600 to PUB-603, PUB-610, PUB-612, PUB-620 to PUB-635;
- PUB-900 to PUB-903, PUB-922, PUB-923;
- PUB-1000, PUB-1004, PUB-1006, PUB-1020 to PUB-1022, PUB-1030, PUB-1031, PUB-1041, PUB-1049, PUB-1050 to PUB-1053, PUB-1056, PUB-1060, PUB-1062, PUB-1064;
- PUB-1100 to PUB-1102;
- PUB-1200, PUB-1202, PUB-1221 to PUB-1223, PUB-1230, PUB-1231, PUB-1240;
- PUB-1300, PUB-1301, PUB-1303, PUB-1304, PUB-1306 to PUB-1308.

**Data,** migration `00xx_publishing.sql` (numbers are assigned at landing), expand only:
- `space_pairs`: the pair, its two spaces, its settings as JSON validated in Go, who made it and when, and when and by whom it ended; partial unique indexes so a space is in at most one active pair on either side;
- `spaces.pair_role`, nullable (`draft`, `published`), set and cleared with the pair, read by the write paths and the trigger;
- `page_pairs`: draft page, twin, the draft version last released, the twin version it became, when and by which run, and the facts of each aspect at that release (title, parent, position, look, labels, files, links that stayed text), plus the aspects changed since, kept in the transaction of each change;
- `file_pairs`: draft file, its copy, the version copied;
- `release_runs` and `release_run_pages`: each run with its preview, note, silence, actor and cause, status and counts; each page with its draft version, twin version, action, outcome, the content hash of the draft version and the link mapping (PUB-732);
- the trigger of PUB-224 on `pages`, `page_versions`, `attachments` and `attachment_versions`, refusing writes in a published space unless the transaction set `diffluence.release` locally;
- the split of `space.release` off `space.administer` (`rbac_split_off`, `rbac_words`, `rbac_split_roles_from`).

**Permissions:** `space.release`, with the migration proof of wave PM; the managed rule in `permissions.Evaluate` and in `perm_editable_pages` and the other SQL lists that decide writing, tested against each other (ADR-017).

**Tests:**
- the adoption test that splitting `space.release` changes no decision (PUB-1301);
- a second pair on either side, a chain, a self-pair and two drafts into one are refused with their reasons (PUB-200, PUB-1050 to PUB-1052);
- linking: the preview's counts, a non-empty published space refused with what is in it, a new published space with no reader, the draft space's site and links withdrawn (PUB-210, PUB-211, PUB-610);
- the managed rule: every write action against draft, published and ordinary spaces, for an instance administrator, a space administrator, an editor and a reader (PUB-220, PUB-221);
- the trigger: a write without the release mark fails with its message (PUB-224);
- the old binary of the previous release cannot edit a twin (PUB-1306);
- out of step: a table test of each aspect, of what never counts, and of undoing a change (PUB-300 to PUB-311, PUB-342);
- a release of one page: first, second, unchanged (no version), parent not released (placed higher), restricted from the releaser, rights missing (PUB-400, PUB-403, PUB-410, PUB-411, PUB-413, PUB-1426);
- files: unchanged not copied, new version copied, removed goes, a reader downloads only the copy (PUB-320, PUB-624);
- the run takes the previewed version when the page changed, and a preview older than an hour is refused (PUB-431, PUB-433);
- a one-transaction run read from another connection (PUB-450);
- links: rewritten with fragments, self-links, blocks naming pages or the space (with the guard over the block schema), placeholders as text, links without a twin as text, comment anchors and instruction removed, other spaces and outside addresses unchanged, database views as "not available here", the fuzz test that no draft id survives (PUB-500 to PUB-534);
- the leak tests of every door, as a reader of the published space, as a writer in the draft, and as a person in both (PUB-108, PUB-600 to PUB-635);
- audit entries per event, the published space's audit without draft names, and no page text in entries (PUB-900 to PUB-903, PUB-922, PUB-923);
- moves and renames of PB1's lines; a move within or into the published space refused; no cycle over random trees (PUB-1000, PUB-1004, PUB-1006, PUB-1020 to PUB-1022);
- version restores in the draft, none in the published space (PUB-1030, PUB-1031);
- two runs started together on two connections (PUB-1060), and a publish in the draft during a run (PUB-1062);
- query counts: a twin as an ordinary page, a draft page with at most one more, a publish in a draft space with one more statement (PUB-1100 to PUB-1102);
- every route answers 404 `experimental_off` while off, except ending a pair; turning off keeps published spaces read-only (PUB-1303, PUB-1304);
- e2e pictures and accessibility of the settings section, the band on a draft page and the byline of a twin (PUB-1200, PUB-1221 to PUB-1231).

**Mutations:**
- the managed rule lets an administrator edit;
- the trigger is dropped;
- a release copies a comment;
- a link keeps the draft id;
- a twin version keeps the draft summary;
- a file route serves the draft file;
- a draft space builds a public site;
- out of step ignores the look;
- a run takes the newest version instead of the previewed one.

**Not in it:** branches and the whole space, leaving out, taking off, the release screen, batches, notifications beyond the existing ones for a version, document control, starting a draft space from an existing one. PB1 ships the pair, the band, single-page release from the page's More menu, and the API.

## PB2: the whole space, leaving out, taking off, and the release screen

**Scope:**
- PUB-222, PUB-223, PUB-241, PUB-243 to PUB-246;
- PUB-305 to PUB-307, PUB-331, PUB-343, PUB-345;
- PUB-401, PUB-402, PUB-412, PUB-420 to PUB-423, PUB-430 (all scopes), PUB-432, PUB-440 to PUB-442, PUB-451 to PUB-455;
- PUB-512 to PUB-514, PUB-522;
- PUB-611;
- PUB-800 to PUB-804, PUB-810 to PUB-818, PUB-821 to PUB-823, PUB-830 to PUB-833;
- PUB-904, PUB-910 to PUB-913, PUB-921;
- PUB-1001 to PUB-1003, PUB-1005, PUB-1010 to PUB-1016, PUB-1032, PUB-1034, PUB-1040, PUB-1042 to PUB-1048, PUB-1054, PUB-1055, PUB-1061, PUB-1063, PUB-1065, PUB-1066;
- PUB-1103 to PUB-1108;
- PUB-1201, PUB-1210 to PUB-1216, PUB-1220, PUB-1224, PUB-1232, PUB-1241, PUB-1242.

**Data,** migration `00xx_release_exclusions.sql`, expand only: `release_exclusions` (draft page, with children or not, reason, who and when); an index on `page_pairs` by pair and status for the release screen; runs gain batches and a resume point.

**Tests:**
- a branch and the whole space, first and later, with left-out pages and their children skipped and listed, and a second unchanged run making no version (PUB-401 to PUB-403);
- sibling order with released and unreleased siblings (PUB-412);
- leaving out one page, a branch, a new child, a move out; leaving out a released page with both answers; restricted draft pages left out of bulk runs unless named; including again (PUB-420 to PUB-423);
- taking off one page, a branch, children kept; never purged; release after take-off makes a new twin; a twin in the trash cannot be restored while paired (PUB-440 to PUB-442, PUB-1015);
- mirroring removals: deleted, archived, moved away, purged, restored from the trash, unarchived (PUB-331, PUB-1001, PUB-1010 to PUB-1014);
- links in one run resolve both ways; links waiting mark the linking page when their target is released and never change its twin silently; invisible twins between batches (PUB-512 to PUB-514);
- batches: a run killed after one batch and inside one, then resumed with the same versions; a slow run switching to batches; rights lost between batches; cancel; a third run waiting (PUB-451 to PUB-455, PUB-1061, PUB-1106);
- storage: a run refused before it starts, and one that meets the limit during a batch (PUB-1054);
- a missing file object stops its page and children only (PUB-1065);
- the consistency check finds and fixes a corrupted fact, and finds a twin version without a release (PUB-343, PUB-921);
- search marks, and search during a run (PUB-801, PUB-803);
- the feed shows one item per run of 500 pages; one notification per person per run; digests; mentions told once; tasks in a twin create no task rows; silent runs; the releaser told; webhooks per subscription (PUB-810 to PUB-833);
- the history, what readers had on a date, and the export of the release history (PUB-910 to PUB-913);
- ending a pair, relinking with edits on both sides, deleting either space, archiving either space (PUB-243 to PUB-246, PUB-1046 to PUB-1048);
- a full restore with a pair and a run going, as a drill step (PUB-1032);
- bundle round trips of both sides (PUB-1042, PUB-1043);
- the benchmarks of chapter 12 on the real-data dump (PUB-1103 to PUB-1108);
- e2e pictures, keyboard and accessibility of the release screen, the preview, progress, every state, narrow and RTL (PUB-1210 to PUB-1216, PUB-1241, PUB-1242).

**Mutations:**
- a left-out page goes in a whole-space run;
- a restricted draft page goes in a bulk run unnamed;
- take-off purges;
- a batch commits half;
- a run ignores lost rights;
- watchers get one notification per page;
- the feed shows one item per page;
- the consistency check reports nothing.

## PB3: approval releases

Needs DC1 (built) and DC2 (the workflow editor form, to set the mark on a state). PUB-731 needs DC4 and is built when DC4 lands, in whichever wave comes second.

**Built** on 2026-11-14, except PUB-731 (waits for DC4; its band came with DC4 on 2026-11-15, its export block comes with PB4, DC4-21 of spec 064); decisions D39 to D51, report `docs/reports/2026-11-14-wave-pb3.md`.

**Scope:**
- PUB-700 to PUB-703;
- PUB-710 to PUB-714;
- PUB-720, PUB-721;
- PUB-730, PUB-731 (after DC4), PUB-732;
- PUB-820, PUB-902;
- PUB-1225.

**Data:** the mark "release when approved" in the workflow definition (a new optional field of an approved state, validated in `internal/workflow`, no migration); the pair settings "only approval releases" and "show approvers to readers".

**Tests:**
- a release of a page in review sends the approved version; a never-approved page cannot be released; Waiting for approval as its own status (PUB-700 to PUB-702);
- workflows refused in the published space (PUB-703);
- the mark: validator, ignored and explained in an unpaired space, versioned and audited (PUB-710);
- approval makes a run with actor "approval" and the approver as cause; a failing release leaves the page approved; returning to the approved text does not release again (PUB-711, PUB-712);
- a whole-space release in a controlled space takes approved versions only (PUB-713);
- "only approval releases" refuses manual release of controlled pages and allows it for ungoverned kinds (PUB-714);
- withdrawal takes the twin off at once (PUB-720);
- the approved-on line with and without approvers shown (PUB-730);
- verification: a twin verifies against its draft version, and a twin changed in the database fails (PUB-732);
- signature manifestations in the band and the exports' control block (PUB-731, after DC4);
- the approver and authors told (PUB-820).

**Mutations:**
- a release takes the head version of a controlled page;
- withdrawal leaves the twin;
- an approval release runs with the approver's rights instead of the release path;
- verification ignores the link mapping.

## PB4: existing spaces and the last measurements

**Scope:**
- PUB-1302, PUB-1305, PUB-1309 to PUB-1312;
- PUB-920;
- PUB-1033 (when restoring one space from a backup exists);
- PUB-1108 on the server's next stress test.

**Tests:**
- starting a draft space from a populated space: every page paired in step, the draft space's grants only those that write, a reader of the old space cannot see the draft space, restricted pages restricted on both sides, posts and databases listed and left (PUB-1302, PUB-1309);
- the draft copy's links lead to draft pages, and a release maps them back (PUB-1310);
- undo before the first release, and refusal after it (PUB-1311);
- turning the part off and on again with pairs (PUB-1305);
- space-wide verification with each outcome (PUB-920);
- the operator and upgrade guides (PUB-1312).

**Mutations:**
- the draft space receives a view-only grant;
- a page is paired out of step after the start.

## Load budget (ARC-238)

PUB-1100 to PUB-1108:
- **Reads:** a twin is an ordinary page, no extra query (class reading). A draft page adds at most one query by primary key.
- **Writes in a draft space:** one UPDATE of the page pair per change of a released page, in the same transaction (class saving); nothing in an ordinary space.
- **Release screen:** one query, paged by 200, within 100 ms at 5,000 pages and 300 ms at 50,000 (class reading).
- **Preview:** within 2 s for 1,000 changed pages, background work past that (class search and reports).
- **Runs:** background work in the worker, one connection each, at most two per node; at least 50 pages a second; one transaction up to 1,000 pages and 30 s, batches of 250 past that; under 64 MB of memory whatever the files.
- **Measured** on the real-data dump with its largest space paired; the server's next stress test re-measures.

## Decisions

Decided on the product owner's behalf on 2026-11-14, may be overruled. The decisions of the study (PUB-1401 to PUB-1430) stand as listed there; these are the ones this spec adds or sharpens for building.

| # | Decision | Reason |
|---|---|---|
| D1 | The part is experimental and off by default, named `publishing` | ADR-043; see "Is it experimental?" above |
| D2 | Four waves: the pair and one page; the whole space and the screen; approval; existing spaces | Each wave leaves the product usable and maintainable if work pauses (RMP-004); approval waits for DC2 |
| D3 | The facts of each aspect are kept in `page_pairs` at write time, not computed from documents when listed | The release screen must fit its budget at 50,000 pages (PUB-1103) |
| D4 | Twins are ordinary pages and nothing is added to the read path | The read path is the most used and the most tested; adding nothing adds no leak (PUB-203) |
| D5 | The managed rule is decided in Go and SQL and held by a trigger | An older node, or a later write path, cannot write into a published space (PUB-224, PUB-1306) |
| D6 | The trigger's mark is a transaction-local setting, `diffluence.release` | It protects against mistakes and older nodes; a compromised application is covered by the permission rule and the audit, as stated (PUB-224) |
| D7 | Block attributes that hold page or space references are one list, guarded by a test over the block schema | A new block type cannot forget to be rewritten (PUB-502) |
| D8 | An approval release runs after the transition commits, in the run queue | A failing copy never undoes an approval (PUB-711) |
| D9 | The link mapping and the draft content hash are stored per released page | Verification of a twin needs them (PUB-732) |
| D10 | Withdrawal always takes the twin off, without a setting | A withdrawn procedure must not stay in front of readers (PUB-720) |
| D11 | Turning the part on first is refused during an upgrade's grace period when that can be read; otherwise the trigger alone carries it | Diffluence keeps no list of nodes and their releases (PUB-1306) |
| D12 | When this lands, the space settings wording "release" an old key becomes "free" the old key | One meaning per word (PUB-1401) |
| D13 | The facts of out of step are kept by a database trigger on `pages` and `page_labels` (`page_pair_refresh`), in the same statement as the change, instead of an UPDATE sent from Go | It adds no statement to any write path (measured: a publish in a draft space sends exactly as many as one in an ordinary space), and a node of the previous release keeps the facts too. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D14 | In PB1 the aspect "place" compares the parent only; the order among siblings comes with PUB-412 in PB2 | A reorder rewrites the positions of every sibling in one statement, so comparing positions would mark every sibling, which PUB-302 forbids. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D15 | A single-page release runs in the request, in one transaction, and not as background work | One page is well within the budget of 150 ms; the worker and batches come with bulk runs in PB2. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D16 | The answer `managed_space` never names the draft space: "This space is changed only by releases." | Naming it only to those who may view it means one more check on every refused write; the settings section and the line on a released page already name it to those who may. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D17 | What the managed rule refuses: creating any kind, editing, archiving, deleting, files, moving out, workflows of a page and of the space, templates, automation, labels and statuses of the space, approval rules and releasing from it. The trigger guards the content columns of pages and files only, so upkeep such as review reminders keeps running in a published space | The list of PUB-220 in the permissions of Diffluence; a trigger on every column would stop the daily tasks that touch every page. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D18 | The warning that drafts are readable (PUB-611) counts all members, all guests and a group that reads both spaces; a single person who reads both does not count | The person who links always reads both, so counting people would always warn. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D19 | A preview belongs to the person who made it, and is confirmed once | A person approves what they saw; someone else makes their own preview. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D20 | In PB1 an archived draft page and a controlled page with no approved version cannot be released, with the reason | Mirroring the archive is PUB-1013 (PB2) and approval releases are PB3; refusing until then never shows readers what they should not get. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D21 | Files of the draft space that a document shows are copied, whichever draft page holds them; files of other spaces stay as they are, like links to other spaces; a draft file that is gone shows "This part is not available here." | No draft file id reaches a reader, and other spaces keep their own rules. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D22 | The open questions of this spec (the word, audiences, on by default at 1.0) stay with the product owner; PB1 builds with "release", one published space per draft space, and the part off by default | They change the model or the words, which PB1 does not need to settle. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D23 | The preview of a run of any size is worked out in the request; a preview in the background (PUB-1104 past 1,000 pages) is not built | Measured: 150 to 190 ms for 2,001 pages and 25 ms with nothing to do, well inside 2 s, so background work would add a moving part without need. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D24 | The order among siblings (PUB-302, PUB-412) is compared by neighbours: among the released siblings that sit together on both sides, a page is out of place when the sibling before it and the sibling after it both differ from its twin's. A move marks the page that moved alone; swapping two neighbours marks both | A reorder rewrites every sibling's position, so comparing positions would mark them all (D14); neighbours need no stored fact and cost four look-ups by index. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D25 | A release of one page, or the end of a pair, asked while a larger run of the pair waits or goes, is refused with `run_going` and the screen says to try again when it has finished; it is not queued | A request that waits minutes for a run would time out; the person sees the run's progress on the release screen. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D26 | In a branch or whole-space run, the pages below a page left out are skipped when that page has no twin (they would land where readers do not expect them, PUB-401); below a page left out that is still published they go | The twin they go under exists, so readers find them where the draft has them. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D27 | Anyone with `space.release` may resume a stopped run; the rest of it runs with the resumer's rights and as theirs | A run stopped because its releaser lost a right could otherwise never finish. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D28 | A draft home page whose title is the draft space's name gives its twin the published space's name instead | A home page carries its space's name by default, so releasing it as it is would show readers the draft space's name (PUB-108); a home page renamed by its writers is released with their title. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D29 | A run of more than one page tells each watcher once, hung on the published home page, with how many of the pages they watch and may see it changed or took off; a silent run of many pages tells no watcher and no mentioned person (PUB-821), while a silent release of one page keeps the rule of a silent publish, which still tells a new mention | A notification needs a page its reader may see; counting keeps titles of pages they cannot see out. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D30 | A draft page deleted or archived keeps its page pair, so restoring or unarchiving it brings it back (PUB-1011, PUB-1013); one moved away or purged loses it, and its twin is kept in `release_removals` until taken off. Deleting either space ends the pair in the database, in the same statement as the delete, which is audited as `space.trashed` | The trash and the archive can be undone, a move or a purge cannot; a trigger cannot write the hash-chained audit, and the delete of the space is the act. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D31 | Linking the same two spaces again restores their page pairs; a twin changed by hand while the pair was ended is marked by an empty released hash, so it is Changed since release and the next release overwrites the edit; the consistency check leaves that mark alone | PUB-245 asks for exactly this, and the mark needs no new column. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D32 | A twin keeps no task rows, also after a release of one page (PUB-817) | A task belongs where it can be ticked. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D33 | The tile "Waiting for approval" shows 0 until PB3 builds approval releases | The screen keeps its shape when PB3 lands. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D34 | Webhooks gain `page.released` and `release.finished`, both with the published space only; a subscription of the draft space with both sides' ids (PUB-830, second half) is not built | Never a draft id to a subscription of the published space is the part that protects readers. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D35 | The estimate of a run counts 30 ms a page, measured on the loaded development machine of this wave, so a run estimated past 30 s goes in batches even under 1,000 pages | PUB-1106 asks for the switch; the measure is honest for this machine and is corrected by the server's next stress test. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D36 | Open question 1, the word: keep "release" for the act | It reads plainly beside "publish" and is used on every screen now; strings change easily until translations open at 1.0. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D37 | Open question 2, audiences: no, a draft space releases into one published space; different audiences are served by restrictions in the published space | Several published spaces would change the model, the screen and every leak test; restrictions already decide who reads (PUB-1405). Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D38 | Open question 3, on by default at 1.0: no; the part stays experimental and off at 1.0 unless it has run on a real installation through at least one full release cycle by then | ADR-043: an optional part earns its default by use, and its run speed is not measured on real data yet. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D39 | An approval release follows the events of document control (`page.version_approved` of wave DC3, sent on every path that approves with the approver as its actor, and `page.withdrawn`), which the worker hands to the publishing module after the notifications, as it does for public sites; the approval code is not touched | The event is written in the transaction of the step, so it is never lost and never seen before the step committed; a release that fails or is slow cannot reach the approval (D8, PUB-711); a second delivery of the event queues nothing. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D40 | The mark is read on the state the approved version was approved into; an approval of text the twin already has, such as a return to the approved text, queues nothing | A page that moved on before the worker read the event still releases what was approved; PUB-712 asks that nothing differing means nothing released. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D41 | A run of the system has no person as its author: the twin version of an approval release has no author, the audit names the actor `approval` or `workflow` and the person as `caused_by`, on the draft side only; a twin such a run makes is owned by the person who made the pair, who answers for the published space | The approver's name must not reach readers through the byline or the history while "show approvers to readers" is off (PUB-730); Part 11 §11.10(e) asks that acts are not attributed to people who did not take them (PUB-902). Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D42 | A run of the system leaves out a draft page with a view restriction, as skipped with the reason "restricted"; a person who sees it releases it by naming it | The system sees no one's restrictions, and a restriction in the draft is a reason the team chose; PUB-422 already leaves such pages to a person in bulk runs. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D43 | Approval releases wait while the part is off (nothing is queued); withdrawal takes a twin off even while it is off | Off means the part does nothing new (PUB-1303); a withdrawn procedure must not stay in front of readers (PUB-720, D10), as ending a pair stays open (PUB-1304). Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D44 | Under "only approval releases" a run skips every controlled page ("left to approval"); a person may still release by hand an approved version whose approval release did not go | Otherwise a failed approval release could only be repaired by approving again; releasing that version by hand is still that approval's release. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D45 | Waiting for approval is counted on its tile, and no tab lists those pages | The release screen lists only what a release would change (PUB-702); the tile keeps the screen's shape (D33). Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D46 | In a run, the pages below a page that cannot be released for itself (never approved, archived, a database, left to approval) go, under the nearest twin above; only a page left out or restricted keeps the pages below it out | A never-approved home page or section would otherwise keep every approved page of a controlled space out of a whole-space release (PUB-713); D26 is about pages left out. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D47 | Verification does the rewrite of the release again from its link map and compares the hash with the twin's document; the map now also keeps the words of links that became text, the files, and the keys of both spaces. It runs on one draft page; twins released before this wave answer "cannot be verified" until their next release; the title is not compared | The comparison is exact without reading anything that changed since (a renamed target, a new space key); the space-wide check is PUB-920 in PB4; PUB-732 binds the text, and the twin's title follows D28. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D48 | The approver and the authors (of the versions since the last release, at most 50) are told by one notification type, "approval_release", hung on the draft page, with what became of the page; the page says why in words | One choice in a person's settings; the band of the page is where the reason is read (PUB-820). Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D49 | The mark needs `space.administer` besides `space.workflows` whenever a save or a copy changes it; instance templates may carry it, and it does nothing until a copy is in a draft space | PUB-710: the person who turns approval releases on could have set up the pair. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D50 | The approved-on line takes the date of the round that approved the released version; the approver is named only while that version is still the approved one | No migration is needed, and the line never names someone for a version they did not approve. Decided on the product owner's behalf on 2026-11-14, may be overruled |
| D51 | A release of a controlled page copies each file at the version and name its released version pinned (DC3), not as the file is now | A new version of a file uploaded for a change in review is part of that change; releasing the approved text with it would show readers what nobody approved (PUB-700, DOC-510). Decided on the product owner's behalf on 2026-11-14, may be overruled |

## What these waves do not cover

Chapter 15 of the study lists them with reasons. In short:
- two drafts into one, several audiences from one draft, and chains (PUB-1500 to PUB-1502);
- releasing into another installation (phase 8, PUB-1503);
- editing in the published space and merging back (PUB-1504);
- blog posts, databases and templates in a pair (PUB-1506 to PUB-1508);
- releasing part of a page, scheduled releases, undo of a run (PUB-1509 to PUB-1511);
- read and understood on twins and review dates for readers (after DC5, PUB-1514);
- folding search results (PUB-1516).

## Open for the product owner

Answered on the product owner's behalf on 2026-11-14 in D36 to D38, and open to be overruled:

1. **The word:** "release" for the act (PUB-1401), or another word. Only strings change, until translations open at 1.0.
2. **Audiences:** should a draft space ever release into several published spaces, one per audience (PUB-1501)? It changes the model.
3. **On by default at 1.0:** the part starts experimental (D1); whether it should be on by default at 1.0 is best decided after it has run on a real installation.
