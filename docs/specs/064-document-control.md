# Spec 064: Document control

Study: [blueprint 09](../blueprint/09-enterprise/01-document-control.md), chapters 02 to 14, lines DOC-001 to DOC-1306. Basis: [08-roadmap/02-enterprise-document-needs.md](../blueprint/08-roadmap/02-enterprise-document-needs.md) (RMP-010 to RMP-012, RMP-015). This is the first of the three enterprise features before 1.0 (ADR-038, spec 048). It is a clean-room build: the requirements come from ISO 9001, 21 CFR Part 11, EU GMP Annex 11 and eIDAS, never from a product.

Decisions made on the product owner's behalf on 2026-11-13 are DOC-1401 to DOC-1424 in blueprint 09.1, section 1.3. They may be overruled before the wave that uses them.

## Core idea

One new row per controlled page, `page_control`, holds the workflow, its version, the current state and kind, the **approved version** and the **working version** (DOC-1300). Every read path asks the permission rule one question: may this person see the working version, as an author or a reviewer of the round? Pages, the tree, search, the feed, exports, sites, notifications, the API and webhooks all take the version from that one answer (DOC-300). Draft and published spaces, the next enterprise feature, reuse this core.

## Rules for every wave

Every wave follows these rules:
- **Tests first.** Start from failing tests, failure paths before the happy path (chapter 13 first where it applies).
- **Mutations.** Add a mutation for each new check.
- **Permissions.** Keep the permission matrix green and extend it.
- **Audit.** Write an audit entry for every new action (DOC-1000).
- **Load.** Measure the read paths on wave PF's real-data dump with 10% of pages controlled (DOC-1306), never only on generated data.
- **Accessibility.** Meet the checks of chapter 10.
- **Today's approvals.** Keep them working until DC5 offers the migration.

## DC1: the model and readers held on the approved version

**Scope:**
- DOC-010, DOC-011 and DOC-015;
- DOC-100 to DOC-105 and DOC-110 to DOC-112;
- DOC-140 and DOC-141;
- DOC-300 to DOC-303 and DOC-310 to DOC-312;
- DOC-320 to DOC-322 and DOC-330;
- DOC-400, DOC-401, DOC-410, DOC-411 and DOC-420 to DOC-422;
- DOC-1230, DOC-1231, DOC-1233 and DOC-1250;
- DOC-1300 and DOC-1301.

**Data:**
- `workflows` and `workflow_versions`: states and transitions as JSON, validated in Go;
- `page_control`;
- `workflow_rounds` and `workflow_answers`.

**Permissions:** `space.workflows` and `page.workflow`, split off with the migration proof of wave PM. All migrations are expand only.

**Tests:**
- the graph validator: a table test per rule, plus a fuzz test that no accepted graph lacks a path to approved (DOC-102);
- workflow versions: a running round keeps its version, removing an occupied state is refused (DOC-110);
- the visibility matrix: reader, author, reviewer, former reviewer, administrator and guest, against draft, in review, approved, rejected and obsolete, on the page, tree, search, feed, links, the content screen and the API (DOC-300);
- "never approved is absent" on every route (DOC-301);
- what restarts review, as a table, including a silent publish and a version restore (DOC-310 to DOC-312);
- obsolete versions and version links per kind of person (DOC-320, DOC-321);
- a publish during a round supersedes it, and old answers do not count (DOC-330);
- space over page, and branch inheritance, move in and move out (DOC-400 to DOC-422);
- assignment preview counts, and a failed assignment leaves no half-assigned page (DOC-140);
- concurrency: two final answers, publish against final approval, and reading during a transition (DOC-1230, DOC-1231, DOC-1233);
- fault injection during a transition (DOC-1250);
- query count: a controlled read costs at most one extra query (DOC-1300);
- EXPLAIN on the real-data dump, with a hash join kept (DOC-1301).

**Mutations:**
- the visibility answer;
- restart on change;
- supersede on publish;
- the space-over-page rule;
- the validator's path rule.

**Not in it:** reviewers beyond the person taking the transition, signatures, due dates, conditions, review dates, read and understood, the editor form (DC1 ships an API and a minimal settings list).

## DC2: reviewers, the workflow editor and the review panel

**Scope:**
- DOC-013;
- DOC-130 and DOC-131;
- DOC-200 to DOC-203;
- DOC-210 to DOC-213;
- DOC-220 and DOC-221;
- DOC-250 and DOC-251;
- DOC-600, DOC-602 and DOC-608;
- DOC-900 to DOC-913, DOC-920 to DOC-922, DOC-924 and DOC-930 to DOC-950;
- DOC-1221, DOC-1222 and DOC-1224.

**Tests:**
- reviewer resolution frozen at the start, the limit of 50, and the empty-list refusal (DOC-200, DOC-201);
- removal needs a right and a reason, and a "changes" answer survives removal (DOC-202);
- a restricted page opens to its reviewers for the round only (DOC-203);
- all, quorum and any; one "changes" ends the round under every rule (DOC-210, DOC-211);
- an answer and its transition share one transaction (DOC-213);
- independence with one and several authors, and no per-round override (DOC-220, DOC-221);
- the compare base is the approved version, and review comments are never shown to readers (DOC-250, DOC-251, DOC-501);
- instance templates are copied, not linked, and both examples pass the validator (DOC-130, DOC-131);
- the editor built by keyboard only, validator messages next to their state, and the diagram's text equivalent (DOC-910 to DOC-912);
- e2e pictures and accessibility checks of the band, the panel and the editor, including RTL and 360 px (DOC-920 to DOC-950);
- groups and roles changing or removed while a round runs, and guests refused (DOC-1221, DOC-1222, DOC-1224).

**Mutations:**
- quorum counting;
- "changes" ending a round;
- independence;
- reviewer read access ending with the round.

**Built:** see `docs/reports/2026-11-13-wave-dc2.md`. Migration 0058 adds `workflow_reviewers`, `workflow_review_comments`, `workflow_templates`, the stage of a round and `perm_review_pages`.

**Load (DOC-1300):**
- Reading a controlled page asks one more `EXISTS` inside the query the band already makes, on the partial index `workflow_reviewers_user`, so it adds no query.
- `approval.read` alone loads the reviewer fact: one query, and only on that action.
- The list of notifications joins `perm_review_pages`, an index scan of the person's own open reviews.
- A step into a review state resolves its reviewers in one query and writes them in one statement.
- An answer locks the page and its control row, then makes three to five small queries.

**Decided on the product owner's behalf on 2026-11-13, may be overruled:**

| # | Decision | Reason |
|---|---|---|
| DC2-1 | Document control is not an optional part under ADR-043: it is the first enterprise feature planned for 1.0 (ADR-038), DC1 shipped without a switch, and nothing changes in a space until someone assigns a workflow. DC2 has no switch of its own | The feature is opt-in per space already |
| DC2-2 | A review state that names no review block keeps the DC1 behaviour: its authors take its steps | Workflows saved under DC1 keep working |
| DC2-3 | A review state with reviewers has exactly one step for "approved" and one for "changes". The system takes them, and no person ever does. No author step leaves such a state for an approved one | DOC-013, DOC-1424: nobody steps around the reviewers |
| DC2-4 | Independence is on by default for every review state, not only for those that lead into approval | Stricter than DOC-1403, and simpler to explain |
| DC2-5 | Reviewers are resolved to members who view the space. Guests, disabled people and people without the space are left out. An entry that names nobody refuses the step and names the entry | DOC-200, DOC-1224 |
| DC2-6 | A role resolves to the people and groups that hold it in the page's space. An "all members" or "all guests" holding names nobody | Thousands of reviewers is never meant |
| DC2-7 | A round asks at most 50 people under "all" or a quorum, and at most 500 under "any one". A state names at most 20 people, groups and roles | DOC-1408, with a bound for "any one" too |
| DC2-8 | A round that could not pass is refused when it would start, with the reason: nobody independent, too many people, or a quorum above who can review | The author sees why at once, instead of a round that waits for ever |
| DC2-9 | "Ask for changes" needs a comment | The authors must know what to change |
| DC2-10 | A removal that leaves too few people to pass is refused until someone is added. Escalation comes with DC5 | DOC-202, without the due-date machinery of DC5 |
| DC2-11 | Being asked opens the page itself through `approval.read`, past restrictions and "never approved", while the round runs and the page sits in that review state. The tree, search, the feed and the pages around it keep the ordinary rule, and the breadcrumbs are empty. Afterwards the person keeps the record of the round (names, answers, comments), never its text | DOC-203 with the narrowest grant, and no cost on the read paths of everyone |
| DC2-12 | Four notification kinds: asked to review, an answer, the end of a review, and a reviewer taken off. They go by mail by default. A reviewer taken off hears of it only while they may still see the page | DOC-600, DOC-602, DOC-608 |
| DC2-13 | "Approved by" on the band names the reviewer whose answer passed the round; the panel lists every answer | One name fits the band; the record keeps all of them |
| DC2-14 | Review comments are separate from page comments. The authors and the people the round asked read them, and nobody deletes them | DOC-251, DOC-501, DOC-502 |
| DC2-15 | The two examples are starting points: a copy of one records no template. The screens translate their names, and the copy keeps the words that are saved. "Two-step review" gets its signature in DC4 | DOC-131; signatures are DC4 |
| DC2-16 | An instance template has one current version. A copy records the template and the version. "Take the new version" saves the template's definition as the copy's next version, and running rounds keep theirs. An archived template leaves its copies as they are | DOC-130, DOC-1405, DOC-110 |
| DC2-17 | Each entry into a review state is a new stage of the round, and it asks its reviewers afresh. A stage ends when the page leaves that state | One round per version, as in DC1, with a clear record per stage |
| DC2-18 | DOC-924 (due dates) shows nothing until due dates exist in DC5 | No code for a need that does not exist yet |
| DC2-19 | The validator runs while editing through `POST /api/v1/workflows/check`, which answers the first problem with the state or step it names | One validator, in Go, for the editor and for saving (DOC-912) |

## DC3: the content around the page

**Scope:**
- DOC-500 to DOC-502;
- DOC-510 to DOC-512;
- DOC-520 and DOC-521;
- DOC-530 to DOC-532;
- DOC-540;
- DOC-550 to DOC-552;
- DOC-560;
- DOC-570 to DOC-572;
- DOC-603 to DOC-607 and DOC-609;
- DOC-1302 and DOC-1253.

**Tests:**
- comment visibility across an approval (DOC-500);
- a reader downloads the pinned attachment version after a new upload, and an upload restarts review (DOC-510, DOC-511);
- a word only in the working version is never found by a reader, in the body, the title or a file name (DOC-520);
- delayed indexing never shows the working version (DOC-1253);
- the export per format as reader and author, and the control block read from the PDF text and Word XML (DOC-530, DOC-531);
- bundle round trip (DOC-532);
- the public site never gets a working version (DOC-540);
- API reads per kind of caller, and signing through a token refused (DOC-550, DOC-551);
- webhook events per subscription (DOC-552);
- every notification kind rendered for a reader of a controlled page, with no working text (DOC-604);
- watcher notifications by kind of watcher, held mentions and digests (DOC-603, DOC-605, DOC-606);
- the feed on the real-data dump within budget (DOC-570).

**Mutations:**
- search choosing the entry;
- export choosing the version;
- notifications using the visibility rule;
- the site publishing only approved versions.

## DC4: electronic signatures

**Scope:**
- DOC-014;
- DOC-700 and DOC-710 to DOC-712;
- DOC-720, DOC-721 and DOC-730 to DOC-733;
- DOC-740 to DOC-742;
- DOC-750, DOC-751 and DOC-760;
- DOC-770 to DOC-772;
- DOC-790;
- DOC-923;
- DOC-1010, DOC-1020 and DOC-1021;
- DOC-1225 and DOC-1303.

**Data:** `signatures`, and a stored content hash per version, with the version of its canonical form.

**Tests, one per Part 11 and Annex 11 point in the table of 09.8:**
- each signature record field (DOC-710);
- any change to the title, body or an attachment changes the hash, and a record copied to another version fails verification (DOC-711, DOC-741);
- a failed transition leaves no signature (DOC-712);
- the dialog names the meaning, as an e2e and accessibility check (DOC-720, DOC-923);
- a reused confirmation, an expired one, a password without a second factor, and a passkey without user verification are all refused (DOC-730);
- an old identity-provider authentication time is refused, with the IdP suite (DOC-731);
- signing failures count toward lockout (DOC-732);
- token, automation, impersonation and support sessions cannot sign (DOC-733);
- tampering in the database turns the mark to "does not match" (DOC-741);
- the documented outside verification, run by a script, verifies an export (DOC-742);
- signing needs to be enabled per person, and disabled accounts' signatures still verify (DOC-750, DOC-751);
- delete, purge and version removal keep signed versions (DOC-770);
- erasure with and without signatures (DOC-771);
- signatures verify after a full restore, as a drill step (DOC-772);
- every DOC line of 09.8 is named in the validation pack (DOC-790);
- renamed signers keep their printed name (DOC-1225);
- signing and display benchmarks (DOC-1303).

**Mutations:**
- the hash covering each part;
- verification;
- the freshness and single use of the confirmation;
- no signing by token;
- retention against deletion.

**Guide:** `docs/operations/regulated-use.md`, covering what the software provides and what remains the customer's (RMP-015).

## DC5: time, review, read and understood, moves, imports and migration

**Scope:**
- DOC-120 to DOC-122;
- DOC-230 to DOC-232 and DOC-240 to DOC-242;
- DOC-402;
- DOC-601;
- DOC-800 to DOC-805 and DOC-820 to DOC-824;
- DOC-1000 to DOC-1002 and DOC-1030 to DOC-1031;
- DOC-1100 to DOC-1107;
- DOC-1200 to DOC-1204, DOC-1210 to DOC-1213, DOC-1220, DOC-1223 and DOC-1232;
- DOC-1240 to DOC-1242, DOC-1251 and DOC-1252;
- DOC-1304 to DOC-1306.

**Tests:**
- conditions per kind, failing closed on a removed property (DOC-120);
- an automation run without the right is refused (DOC-121);
- due dates across a weekend, reminders, escalation, and a time-out that never approves (DOC-230, DOC-231, DOC-105);
- a clock moved backwards gives one reminder (DOC-232, DOC-1241);
- delegation and the absence chain with its limit, and a disabled reviewer escalating (DOC-240 to DOC-242);
- exempt labels need `space.workflows` (DOC-402);
- periodic review dates, confirmation without a new version, and the report (DOC-800 to DOC-805);
- read and understood: resolution, late joiners, re-asking on a new version, the list and the button only after opening (DOC-820 to DOC-824);
- an audit entry per event, a reason where required, and prune keeping control entries (DOC-1000 to DOC-1002);
- reports within budget on the real-data dump (DOC-1030);
- migration:
  - today's approvals still work after the migration;
  - the generated workflow gives today's outcome;
  - the three landing cases;
  - no signature for carried-over approvals;
  - undo before the first round, refused after it (DOC-1100 to DOC-1107);
- moves across and within spaces, copies without signatures, delete and restore from the trash, archive (DOC-1200 to DOC-1204);
- a full restore with an open round that escalates, as a drill step (DOC-1211);
- imported signatures verify and do not approve (DOC-1212);
- an import into a controlled space makes working versions (DOC-1213);
- the due job on two workers claims each round once (DOC-1232);
- database time on a node with a skewed clock (DOC-1240);
- a large assignment killed halfway and resumed, and a failing mail sender never blocks a transition (DOC-1251, DOC-1252);
- the due job with 10,000 rounds in passes (DOC-1304);
- reports and exports bounded by the read limit or run as background work (DOC-1305);
- the budgets of DOC-1306 on the real-data dump.

**Mutations:**
- escalation never approving;
- the job claiming once;
- carried-over approvals not becoming signatures;
- the move taking the target's rule;
- copies dropping signatures.

## Load budget (ARC-238)

DOC-1300 to DOC-1306:
- **Reads:** a controlled read costs one row, joined into the visible-pages hash of wave PF. The second search entry exists only while a draft differs from the approved version.
- **Signatures:** one stored SHA-256 per version.
- **Due-date job:** one per worker, at most 500 rounds a pass.
- **Measured budgets,** on the real-data dump with 10% of pages controlled:
  - a controlled read within 5 ms of an ordinary read;
  - the tree and the feed within PF's budgets plus 10%;
  - a transition within 20 ms;
  - signing within 50 ms plus the confirmation.
- **Server:** the next stress test re-measures on the server.

## What these waves do not cover

- qualified electronic signatures under eIDAS through an outside provider (DOC-791, open for the product owner);
- validation of a customer's use of the system (the customer's duty, DOC-790);
- a drag-and-drop workflow editor (DOC-914);
- conditions on outside tools (DOC-122);
- quizzes before read and understood (DOC-825);
- approving templates for others' use (guided templates, later);
- draft and published spaces (their own study, DOC-431).

## Open for the product owner

1. **Qualified signatures:** should Diffluence ever offer qualified signatures through a trust provider? That costs a provider and a contract.
2. **Retention against erasure:** should signed records in controlled spaces be kept against an erasure request (decided for now, DOC-1417), or should that be a choice per space?
