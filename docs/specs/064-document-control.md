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

**Built:** see `docs/reports/2026-11-14-wave-dc3.md`. Migration 0063 adds `attachment_pins`, `search_working`, `approval_subscriptions`, the column `needs_edit` of a notification, the workflow of a template and the source of an imported round, and the functions `reader_pins`, `reader_pinned_at`, `perm_unpinned_files` and `search_working_names`.

**Load (DOC-1302, DOC-570):**
- A reader's read of a file of a controlled page asks one more query for the pins, and one more for the rule when the page pins its files.
- Search keeps a second entry only for drafts in progress. It asks the small table of working entries for the typed words, then the rule of editing for the few pages found; the shown rows ask it once more.
- The lists of files ask `perm_unpinned_files`: the changed files of pinned pages first, then the rule of editing for their pages only.
- The feed asks whether a person approved each page it shows, by key, for the rows of one page of the feed. Measured on the real-data dump: see the report.
- A notification for editors asks the rule of editing for its own page when it is shown or mailed.

**Decided on the product owner's behalf on 2026-11-14, may be overruled:**

| # | Decision | Reason |
|---|---|---|
| DC3-1 | Every door around a page asks one question, `permissions.ReadsWorking`: authors (whoever may edit the page) and the reviewers of its open round read the working version; everyone else reads the version on the pages row | DOC-300: one rule in one place |
| DC3-2 | A comment or reply written by someone who may edit, while a working version differs from the approved one, is kept for editors, as a thread made while editing is. It reaches readers when a version that holds what it points at is approved. A reader's comment belongs to the approved version, and everyone sees it | DOC-500, with what comments already had |
| DC3-3 | A reviewer who may not edit cannot add a page comment while the working version differs: their remarks go in the review | A thread kept for editors would be hidden from them, and an open one would reach readers (DOC-251) |
| DC3-4 | Editing a comment keeps the version it belongs to | A comment belongs to the version it was made on |
| DC3-5 | Each version of a controlled page pins the files it shows, with their version and name. Readers get the pins of the version they read. A file the page holds but that version does not show is not a reader's | DOC-510, DOC-512 |
| DC3-6 | A new version, an older version made current again, or a new name of a file the working version shows writes a new version of the page with the same text, which starts review again. A file that no version shows changes nothing | DOC-511: reviewers approve the files they saw |
| DC3-7 | A file the approved version shows, or its pinned version, cannot be deleted, moved or removed until a version without it is approved | Simpler than keeping a file in the trash for readers (the edge of DOC-510), and nothing to purge around |
| DC3-8 | The Files screen, its filters and quick file search leave out, for a reader, a file of a controlled page whose version or name waits for review; the page's own list shows the approved one | Only pinned versions, without a second set of names to search (DOC-512) |
| DC3-9 | The second entry of search holds the working title, text and file names, only while they differ from the approved version. An author matches both entries and sees the working title and excerpt. A reviewer keeps the ordinary rule | DOC-520, DOC-1302, DC2-11 |
| DC3-10 | The newly approved version is indexed inside the transaction of the approval | DOC-1253: until it commits, readers find the version approved before |
| DC3-11 | Every export format puts a control line above a controlled page: its state, version, approval date and approver, or "Not approved" for a working version. A PDF that holds a working version says "Not approved" in the header of every page. Signatures join in DC4 | DOC-531 |
| DC3-12 | Old versions of a controlled page in an export follow the history rule: none for a reader | DOC-320, DOC-321 |
| DC3-13 | A bundle (version 8) carries the record: workflows, where each page stands, rounds with answers, reviewers and review comments, and the files as readers see them. An import copies the workflows into the new space as page workflows and keeps the version readers saw. A page that waited for review lands in the start state, and the rounds are a record marked with their installation that never counts | DOC-532; how imported approvals and signatures count further is DOC-1212, in DC5 |
| DC3-14 | A token takes steps, answers a review and reads its record with the checks of the screens. Refusing a signature through a token is proved in DC4, which brings signing | DOC-551, DOC-733 |
| DC3-15 | Two webhook events: `page.version_approved` reaches owners who may view the page, `page.working_changed` only owners who may edit it, checked at delivery | DOC-552 |
| DC3-16 | Watchers who may edit hear of working changes. Every watcher hears of a new approved version, a new kind "version approved", in the digest by default. Its mail shows the change between the two approved versions | DOC-603, DOC-604 |
| DC3-17 | A notice about a working version, or about a thread for editors, is shown and mailed only while its person may still edit the page | The edge of DOC-603 |
| DC3-18 | A mail is built through the rule whatever a row says: a reader of a controlled page gets no change but one between approved versions, and no excerpt of a thread for editors | DOC-604 |
| DC3-19 | A mention in a working version reaches at once those who may edit; the others hear when it is approved, once | DOC-606 |
| DC3-20 | A digest groups the approvals of one page in one line, with the newest version | DOC-605 |
| DC3-21 | The approvals digest is a choice per person and space, in the menu of the space, and goes through the channels and settings of every notification | DOC-607, DOC-609 |
| DC3-22 | A template of a space may name an active workflow of that space, without the part of guided templates. The first page made from it enters it as a page workflow in its start state, through the set path of guided templates, unless its entry names one, or a space or branch workflow governs it. The person using the template needs the right to choose workflows, as for a workflow an entry names (TPL-152). A template of all spaces names none | DOC-560 on the model of spec 074; workflows belong to a space |
| DC3-23 | The feed shows a reader a controlled page that a person approved as "approved", under the time its approved version was written; the approval does not move it up. A page that came under control already approved shows as before | DOC-570 within its budget: placing approvals by their time read every version on the real data (DC3 report) |
| DC3-24 | Analytics record the version a person read | DOC-571 |
| DC3-25 | A draft that waits at the upgrade gets its second search entry at its next publish | A migration runs no Go code |

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

**Built:** see `docs/reports/2026-11-15-wave-dc4.md`. Migration 0069 adds `signatures`, `version_hashes`, `signing_rights`, `signing_confirmations` and `signature_meanings`, the signature of a reviewer's answer, an index of the audit entries of pages, and the functions `signed_file_versions` and `holds_signed_records`. The validation pack is `docs/operations/validation-pack.md`.

**Load (DOC-1303):**
- Signing hashes the version once, when it is first signed, and keeps the hash in `version_hashes`; later signatures of that version compare with it.
- Showing a controlled page reads its signatures inside the query the band already makes, so it adds no query. The history adds one query for the signatures of the versions it lists.
- A full verification recomputes the hash and reads the audit entry with its two neighbours: three small queries per signature, only on demand, in the control history, the auditor's export and `kenwerk signatures verify`.

**Decided on the product owner's behalf on 2026-11-15, may be overruled:**

| # | Decision | Reason |
|---|---|---|
| DC4-1 | Signatures have no switch of their own, like the rest of document control (DC2-1). Nothing signs until a workflow names a meaning on a step and an administrator enables a person | Opt-in per step and per person already; a switch would be one more thing to validate |
| DC4-2 | A step requires a signature by naming a meaning. On an author's step its taker signs; on the step that follows an approved review, every reviewer who approves signs. A request for changes is never signed | DOC-700; a signature states agreement with a version |
| DC4-3 | A signed approval is final for its stage: the reviewer cannot change it into a request for changes | A signature stands; DOC-212 still holds for answers that are not signed |
| DC4-4 | Four meanings every space has (reviewed, approved, released, verified); a space adds its own, which are never changed or removed. A template of the instance names only the four. The record keeps the meaning's words as shown | DOC-721; records and workflows keep pointing at what was meant |
| DC4-5 | An administrator enables signing per person and says how they identified them, as one of four choices, never free text; never their own signing; never for a guest or a service account. Disabling keeps the signatures made | DOC-014, DOC-751, DOC-1422; the audit chain keeps the choice for good, so it must hold no number of an identity document |
| DC4-6 | Signing is begun for one signature (the page, its version, and the step or the stage of the round), in the person's own session; confirmed within ten minutes; then taken once, within two minutes of the confirmation. A person has one step waiting for confirmation at a time | DOC-730, DOC-1416 |
| DC4-7 | A confirmation is the password with a code of the authenticator app (never a recovery code), a passkey with user verification, or, only for someone without a password, a new login at the identity provider after the step began, allowing one minute of clock difference as the step-up does. That login opens no session, and never makes or links an account | DOC-730, DOC-731; a recovery code is written down where others may find it |
| DC4-8 | A wrong password or code of a confirmation counts toward the stop after 100 wrong passwords, with the same answer for both; a failed passkey is audited but does not count, because a passkey is not guessed | DOC-732 |
| DC4-9 | The signed content is canonical form 1: title, document format version, the document as canonical JSON, and the files the version shows with their version, name and SHA-256, each with its length, as written in `docs/operations/regulated-use.md` | DOC-711; an auditor can rebuild it by hand |
| DC4-10 | The page shows the quick mark (the record against the hash its version keeps); the history verifies in full on demand, as do the control history, the auditor's export and `kenwerk signatures verify` | DOC-741, DOC-1303 |
| DC4-11 | Signed records are kept without end: signatures, the versions they sign and the file versions those show. A trash item that holds them cannot be purged and stays past its time; a signed version cannot be removed; pruning the audit log stops at the first signature | DOC-770; Kenwerk has no retention period per space yet |
| DC4-12 | The control history lists every audit entry about the page with its place in the chain, the newest 2000, with the verification of each signature. It shows no title or text, and needs `space.audit` | DOC-1010; reading the audit of a space is not reading every page |
| DC4-13 | The auditor's export is JSON Lines, made while the request waits, for at most 1000 pages, with a period. A page the exporter may not read gives its record without title or text. The PDF report and the export as background work come with DC5 and DOC-1305 | DOC-1020 partly; the machine-readable copy first, so outside verification works now |
| DC4-14 | The outside verification is `scripts/verify-signatures.mjs`, which needs only Node.js, and the same steps written for doing by hand | DOC-742 |
| DC4-15 | Bundles do not carry signatures yet; DC5 brings them with DOC-1212. A workflow imported from a bundle that names a meaning the space lacks refuses signing until the space adds it | Imported signatures need their own record (DOC-1421) |
| DC4-16 | Every export's control line names the signatures of the version in the file | DC3-11, Part 11 §11.50(b) |
| DC4-17 | "Two-step review" signs its quality approval as approved | DC2-15 |
| DC4-18 | Kenwerk has no impersonation and no support sessions. The signature itself refuses everything but a person's own live session without a pending second factor, so such a session, if one comes, cannot sign without its own decision | DOC-733 |
| DC4-19 | Qualified signatures through a trust provider (open question 1): not now. Recommended for after 1.0, when a customer needs them, because they cost a provider and a contract | DOC-791 |
| DC4-20 | Retention against erasure (open question 2): keep DOC-1417 for every controlled space, without a choice per space, until there are retention periods per space | A choice per space would need a period to end at |
| DC4-21 | PUB-731 comes with DC4, which landed after PB3: the published copy of a signed version shows its signatures in its band, with the printed name, date and time, and meaning, whatever "show approvers to readers" says; no record is copied. The control block of PDF and Word exports of a published copy comes with PB4, because an export of the published space would need the pair's records, which belong to publishing | Part 11 §11.50(b) on the screen now; the exports with the wave that owns them |

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

**Built:** see `docs/reports/2026-11-15-wave-dc5.md`. Migration 0071 adds `review_deadlines`, `absences`, `periodic_reviews`, `read_requests`, `read_confirmations` and `imported_signatures`, the columns `via`, `via_from` and `via_reason` of a reviewer, `review_months` and `carried_round` of `page_control`, `exempt_labels` and `from_approval_rule` of a space workflow, the function `control_working_days_after`, and imported signatures in `holds_signed_records`. `0070_reserved.sql` holds the number of GT4.

**Not in this wave:**
- the PDF report of the auditor's export and the export as background work (DOC-1020 partly, DOC-1305 partly);
- a signature of meaning "read and understood" (DOC-821 partly): confirming is an acknowledgement for now;
- conditions on page properties, which Kenwerk does not have (DOC-120 partly);
- a screen for absences and for the escalation targets and reading groups of a state; the API takes them (DOC-241, DOC-231, DOC-820);
- the restore drill step with an open round (DOC-1211), resuming a large assignment killed halfway (DOC-1251), and the budgets of DOC-1306 on the real-data dump.

**Load (DOC-1304 to DOC-1306):**
- A controlled read asks its due date, review date and reading request inside the query the band already makes: no query more. A workflow with conditions adds three to five small queries for its authors' band only.
- A stage with a due period writes one row when it starts; the job reads due rows by the partial index `review_deadlines_next`, at most 500 a pass, and locks one page at a time.
- Late readers are found in one bounded statement a minute; its cost grows with the open reading requests times the members of their groups.
- Reports read one space or the spaces a person audits, at most 5000 rows, under the 15-second limit of every request.

**Decided on the product owner's behalf on 2026-11-15, may be overruled:**

| # | Decision | Reason |
|---|---|---|
| DC5-1 | No part of DC5 has an experimental switch. Due dates, escalation, periodic review, reading and conditions are set per state or step, delegation and absence per person, exempt labels per space workflow, and the move from today's approvals is an offer | DC2-1: each part is opt-in already, and a switch would be one more thing to validate |
| DC5-2 | A due period is 1 to 60 working days, counted in the database from the start of the stage, in the instance's time zone and working week (Monday to Friday unless set under General), at the same time of day. Reviewers who have not answered are reminded the chosen number of working days before, on the date, and each working day after | DOC-230, DOC-232, DOC-1240, DOC-1242 |
| DC5-3 | When the date passes, the state's overdue step is taken if it has one; otherwise its escalation targets are asked, once; otherwise the reviewers are reminded. An overdue step never leads into approval: the validator refuses it, and the step itself refuses it whatever a stored definition says, leaving the page in review with the refusal on record | DOC-105, DOC-231, DOC-1401 |
| DC5-4 | The due job runs every minute: at most 500 items a pass and 40 passes a run, each in its own transaction that locks the page with SKIP LOCKED, checks the stored moment again and moves it on. One item that fails holds up no other | DOC-1232, DOC-1241, DOC-1304 |
| DC5-5 | A reviewer who has not answered hands their place only to someone the same entry of the state names. Whoever looks after the workflows may hand anyone's place to any member who may review. Never to a guest, an author under independence, someone the stage asked already, or, for a signed approval, someone who may not sign. A reason is needed; the delegator answers no more; both sides are told; the record keeps who, from whom and why | DOC-240, DOC-608: delegation never widens who may approve, and a signature is never handed on |
| DC5-6 | A person sets their own absence, with dates in the instance's zone and an active member as stand-in. When a stage starts, an absent reviewer's stand-in takes the place only when the state names the stand-in too and they may take it; a chain is followed for three steps, then the escalation targets are asked; with none, the absent person stays asked. A running stage does not change | DOC-241, without widening who reviews |
| DC5-7 | The job drops disabled and removed people from open stages within a minute, with an audit entry each. A stage that can no longer pass escalates, and one that now passes takes its step. A removal by hand that leaves too few escalates where the state names a target, and is refused otherwise (DC2-10) | DOC-242, DOC-1220 |
| DC5-8 | An approved state asks 1 to 10 groups to confirm reading. They are resolved at approval to active members who view the space, never guests; people who join later are asked by the job. A confirmation is for the version readers see now, once, only after the page showed that version in the same session, and never through a token | DOC-820 to DOC-824, DOC-1418 |
| DC5-9 | A timed approved state has a period of 1 to 120 months, 12 when it names none. A page may have a shorter one, set by whoever may set its workflow. The owner, or the space's administrators when the owner left, is reminded 30 days before and on the date, then weekly. On the date the state's periodic review step starts a round on the same text as the system; passing it confirms the version without a new one and moves the date on, and watchers are not told of a new version | DOC-800 to DOC-805, DOC-1223 |
| DC5-10 | A space workflow exempts at most 20 labels. Adding or removing one, and renaming a label onto or away from one, needs space.workflows; a copy keeps an exempt label only for someone who may set it | DOC-402: nobody escapes review with a label |
| DC5-11 | Conditions are a label present or absent, a change of the title, the body or the files since the approved version, and at least one file, joined by "all of" or "any of", at most ten, through `conditions.Combine` and `conditions.MaxTests`. A label the space's fixed list no longer allows fails closed. Kenwerk has no page properties, so there is no property condition yet | DOC-120, TPL-433 |
| DC5-12 | No automation action takes a step. A token takes steps with every check of a person, conditions included, and never signs or confirms reading | DOC-121, DOC-1413 |
| DC5-13 | A move to another space never takes a page workflow along, and an approval under the other space's procedure never counts: under control there the page starts at its newest version and readers see nothing until it is approved; without control it is an ordinary page at its approved version | DOC-1200, DOC-430 |
| DC5-14 | A bundle's workflows arrive without their reviewers, escalation targets and reading groups, which are people of another installation. A bundle whose readers' version is not the approved one is refused. Signatures arrive as imported records, shown with their installation and verified against the content here, and never count as a signature or an approval here. The bundle keeps version 8 with an optional list of signatures | DOC-1212, DOC-1421 |
| DC5-15 | A step's comment is kept in its audit entry. Pruning the audit log stops at the oldest record of document control: a signature, or an entry about a controlled page or a workflow | DOC-1001, DOC-1002 |
| DC5-16 | Five reports (states, overdue, periodic review, reading, approvals per person in a period), for a space with space.audit and across every space the person audits, as JSON or CSV, at most 5000 rows. A page the person may not read is listed without its title | DOC-804, DOC-823, DOC-1030, DOC-1031 |
| DC5-17 | A space that requires approval and has no space workflow is offered "Approval as before": a draft, a review by every fixed approver under "all" with authors allowed to review and add, approved, and changes. A page approved at its newest version lands approved; at an older version, that one is what readers see and the newer one waits; never approved, it starts hidden. Carried approvals are marked, never signed. The old rule is off while the space follows that workflow and stays readable. Undo is open until a person acts under it | DOC-1101 to DOC-1107 |

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

1. **Qualified signatures:** should Kenwerk ever offer qualified signatures through a trust provider? That costs a provider and a contract.
2. **Retention against erasure:** should signed records in controlled spaces be kept against an erasure request (decided for now, DOC-1417), or should that be a choice per space?
