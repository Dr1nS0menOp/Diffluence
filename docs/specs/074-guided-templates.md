# Spec 074: Guided templates

Study: [blueprint 09, guided templates](../blueprint/09-enterprise/templates/01-guided-templates.md), chapters 02 to 14, lines TPL-001 to TPL-1214. Basis: [08-roadmap/02-enterprise-document-needs.md](../blueprint/08-roadmap/02-enterprise-document-needs.md) (RMP-014). This is the third of the three enterprise features before 1.0 (ADR-038, spec 048, D14). It is a clean-room build. Templating.app is named only to say which behaviour is meant: its public pages describe issues, and here the same behaviour is meant for pages. The requirements come from that behaviour, from research on forms, and from Kenwerk's own blueprints, never from a product's text, layout or source.

Decisions made on the product owner's behalf on 2026-11-14 are TPL-1401 to TPL-1424, in section 1.4 of the study. They are repeated in the decision table below. Each may be overruled before the wave that uses it.

## Core idea

A template is today one hidden page with a row of settings (spec 011, D6). This spec makes a template a list of **entries**. Each entry is a hidden page of kind `template`, as today, so text, files and page versions come for free. A **manifest** in `template_versions` snapshots what one version makes:
- the entries, with their page versions;
- the fields and steps;
- the conditions and rules.

Using a template runs one function twice:
- as a **dry run**, for the check step;
- for **real**, in one transaction that makes the whole set or nothing.

Conditions are structured tests that a pure Go function evaluates. Rights per template only narrow the rights of the space. Approval binds to one version, while the approved version stays in use.

## Experimental, and off by default (ADR-043)

Guided templates are the experimental part `guided_templates` in the `experimental` setting of spec 069 (TPL-050, decided TPL-1401). It is off on a new installation and after an upgrade. While it is off:
- templates behave exactly as today;
- a template that uses any part of this spec is not offered, and its use route answers 404 `experimental_off`;
- managers still list, edit and delete it.

**Why experimental:**
- ADR-043 freezes new parts before 1.0, and makes optional parts experimental and off by default.
- ADR-038 puts this feature before 1.0, and the product owner wants it built before 1.0.
- An experimental part meets both: the code ships, the 1.0 claim does not rest on it, and it earns "on by default" after 1.0.
- It also adds rights, which the one outside reader at 1.0 would otherwise have to cover.
- Failing closed matters here: a template with a restricted audience is hidden while the part is off, never opened to everyone.

The lines that finish today's templates (ORG-045, ORG-051, ORG-060, ORG-061; TPL-1104) are **not** experimental: they complete a part that exists.

## Rules for every wave

- **Tests first.** Start from failing tests, failure paths before the happy path. Chapter 10 of the study comes first where it applies.
- **Mutations.** Add a mutation for each new check.
- **Permissions.** Keep the permission matrix green, and extend it with the template actions (TPL-063).
- **Audit.** Write an audit entry for every new action (TPL-711). No event, notification or mail holds an answer's value (TPL-1409, TPL-723).
- **Load.** Measure on wave PF's real-data dump (TPL-1422), never only on generated data.
- **Accessibility and design.** Meet chapter 12, with a picture for every new screen in `web/e2e/0-design.spec.ts`.
- **Experimental.** Every new route answers `experimental_off` while the part is off. A test proves it per route.
- **Today's templates.** Keep them working unchanged. Today's template tests run in every wave without edits.

## GT1: the model, versions and the all-or-nothing set

**Scope:**
- TPL-050, TPL-063;
- TPL-100 to TPL-103 and TPL-110 to TPL-114;
- TPL-120 to TPL-123;
- TPL-130, TPL-131;
- TPL-140 to TPL-143;
- TPL-150 to TPL-152;
- TPL-200 to TPL-202, TPL-210, TPL-211, TPL-220 to TPL-222 and TPL-230;
- TPL-600, TPL-601, TPL-603 and TPL-610;
- TPL-711 (`template.version_saved`, `template.set_created`, `template.set_failed`);
- TPL-800 to TPL-804, TPL-810 to TPL-812, TPL-820 to TPL-823, TPL-825 to TPL-827;
- TPL-840 to TPL-843 and TPL-850, TPL-851;
- TPL-855 to TPL-857;
- TPL-901, TPL-902, TPL-903, TPL-906 and TPL-908;
- TPL-1100, TPL-1101 and TPL-1103.

**Data** (migration `00xx_guided_templates`, expand only):
- `template_versions`: template, number, manifest (JSON, at most 64 KiB, validated in Go), author, time, state;
- `template_entry_pages`: the hidden pages of a template's entries, so deletion cascades and the file route of the templates module (spec 011, D35) accepts files of any entry;
- `template_sets`: template, version, space, root page, creator, time, page count. No answers;
- `page_template_origin`: page, set, entry key;
- the pending copies of TPL-821;
- on `templates`: `owner_id`, `state` (default approved), `approved_version`, `guided`;
- a version 1 for every existing template, approved (TPL-1101).

**Code:**
- `internal/templates` gains the manifest, its validator, and `MakeSet`, which runs as a dry run or for real (TPL-341 shares it in GT2);
- `pages.StartFromTemplate` becomes the one-page case of `MakeSet`, so there is one path;
- the empty page waiting for a template is removed inside the transaction (TPL-122);
- the inline block `templateValue` is added to the document model, allowed only in templates.

**Tests:**
- fault injection at the first page, a middle page, the last page, a file copy, a label, a restriction and the audit write, finding no trace of the set (TPL-120, TPL-820);
- a read from a second connection during a slow set sees none of it (TPL-820);
- a rolled-back set leaves no stored object after the task of TPL-821;
- a background set killed halfway leaves nothing and completes on retry (TPL-121);
- a double submit makes one set (TPL-123);
- rights on part of the set refuse the whole set, listing every missing right (TPL-810); a restricted entry without `restriction.manage` (TPL-130); a deleted group (TPL-130);
- links between entries resolve in one pass, with no extra versions (TPL-110, TPL-111);
- titles: empty, too long, an answer holding `{space}`, an existing title (TPL-140 to TPL-142);
- drafts by default, "publish at once", and a controlled space (TPL-150, TPL-151); a workflow without `page.workflow` (TPL-152);
- missing space, template space deleted, default parent gone, chosen parent moved, a space no longer offered (TPL-800 to TPL-804);
- the limits at 100 and 101 entries, 6 and 7 levels, 40 and 41 fields, 5 MiB (TPL-102, TPL-825);
- two sets in one space at once (TPL-822); two background sets per space and one per person (TPL-827);
- the forbidden-content rules on save and import (TPL-840 to TPL-842);
- a form bound to its version across a new version (TPL-603);
- the migration: the gallery of every space is unchanged before and after (TPL-1101); turning the part on during the grace period is refused (TPL-1103);
- every new route answers `experimental_off` while the part is off (TPL-050);
- the benchmark: a set of 10 pages within 300 ms, of 100 within 3 s, a single page within 10% of today, and the audit queue empty within 5 s (TPL-901 to TPL-903).

**Mutations:**
- a set committed per page instead of once;
- a skipped entry instead of a refused set;
- a restricted entry made without its restriction;
- a `templateValue` let through into a published page;
- a link resolved to the wrong entry.

**Not in it:** new field kinds, conditions, rights per template, approval, the editor's new sections (GT1 ships the API and builds sets from the existing editor with a minimal Pages list).

**Status (2026-11-14):** built, with migration `0060_guided_templates`. Report: [2026-11-14-wave-gt1.md](../reports/2026-11-14-wave-gt1.md). Left for later, each with its reason in the decision table below: the bell for a background set (TPL-1433, with TPL-720 in GT4); the check step in the form, whose dry run GT1 ships as `POST /templates/{id}/sets/check` (GT2); restrictions and workflows of an entry are set through the API only, as the minimal Pages list does not show them; the measurements on the real-data dump (TPL-1422), because no dump was at hand.

## GT2: fields, the form and the check step

**Scope:**
- TPL-212, TPL-231;
- TPL-300 to TPL-317;
- TPL-320 to TPL-325;
- TPL-330 to TPL-333;
- TPL-340 and TPL-341;
- TPL-904;
- TPL-1010 to TPL-1013, TPL-1020 to TPL-1026, TPL-1040 to TPL-1043;
- TPL-1104 (finishing ORG-045, ORG-051, ORG-060 and ORG-061, not experimental).

**Code:**
- the field kinds are defined once in Go (`internal/templates/fields`) and once in TypeScript;
- a JSON table of cases in the repository is read by both test suites (TPL-325);
- the form keeps `Dialog`, `Field` and `SubjectPicker`.

**Tests:**
- table tests per kind, run by Go and by TypeScript from the shared table, including empty values, bounds, separators, month ends and leap days (TPL-300 to TPL-311, TPL-325);
- API tests that send every invalid value the browser would stop (TPL-320);
- page, person and people fields with ids the creator cannot see, a guest, a disabled person (TPL-306 to TPL-308);
- link fields with other schemes (TPL-310);
- defaults that turned invalid (TPL-316);
- the error summary, focus and messages by keyboard; no error while typing in an empty field (TPL-321, TPL-322);
- answers kept on Back and on a server refusal; asking before closing (TPL-324, TPL-332);
- the dry run equals the real run for 200 random answer sets (TPL-341);
- the check step lists the pages, labels, restrictions and existing titles (TPL-340);
- pictures of every field kind, empty, filled and in error, in light, dark, narrow and RTL (TPL-1022);
- the editor's sections by keyboard only (TPL-1010 to TPL-1012);
- the acceptance tests of ORG-045, ORG-051, ORG-060 and ORG-061 (TPL-1104);
- the dry run within 150 ms for 10 pages and 1 s for 100 (TPL-904).

**Mutations:**
- a required field not checked on the server;
- a range check on one side only;
- a page field that accepts a page the creator cannot see;
- the dry run and the real run drifting apart.

**Status (2026-11-14):** built, with no migration. Report: [2026-11-14-wave-gt2.md](../reports/2026-11-14-wave-gt2.md). The kinds live in `internal/templates/fields` and `web/src/template-fields.ts`, and both run `internal/templates/fields/testdata/cases.json`. Left for later, each with its reason in the decision table below: the Rules and Who can use it sections (GT3, GT4); the text of a page other than the first is still written in place in the Pages section, not yet in the Focus frame (TPL-1011); the test of dates in two locales (TPL-1043); the measurements on the real-data dump (TPL-1422).

## GT3: conditions

**Scope:**
- TPL-203;
- TPL-400 to TPL-403;
- TPL-420 to TPL-424;
- TPL-430 to TPL-434;
- TPL-905;
- TPL-1014.

**Code:**
- `internal/conditions`: the condition shape and a pure evaluator. If DC5 (spec 064) lands first, it is reused here; otherwise DC5 reuses it (TPL-433);
- the block `conditional`, allowed only in templates, up to 3 deep.

**Tests:**
- the table of operators by kind, with an empty value for each, run by Go and TypeScript (TPL-401, TPL-403, TPL-431);
- a fuzz target over random conditions and answers that never panics and stays within a fixed number of steps (TPL-430);
- the validator: nesting, an 11th test, a wrong operator, a missing field, a later field, a deleted group (TPL-400, TPL-421, TPL-432);
- an entry left out with its children and the links to it (TPL-420);
- hidden steps and fields: the answer is dropped, and a hidden required field does not block (TPL-421);
- sections true, false and nested; files in a removed section not copied; a page with `conditional` cannot be published (TPL-422);
- label rules and a fixed list (TPL-423);
- the creator's groups: member, non-member, deleted group (TPL-402);
- a rename of a field carried into its conditions (TPL-230, TPL-432);
- the condition editor by keyboard, and its sentence matching the table (TPL-1014);
- evaluation of every condition of a 100-entry template within 1 ms (TPL-905).

**Mutations:**
- each operator;
- an empty value treated as non-empty;
- an answer to a hidden field kept;
- the server trusting the browser's evaluation.

**Status (2026-11-15):** built, with no migration. Report: [2026-11-15-wave-gt3.md](../reports/2026-11-15-wave-gt3.md). `internal/conditions` holds the shape, the validator and the evaluator, with `Combine` and `MaxTests` as the part document control's transitions reuse (TPL-433); `web/src/template-conditions.ts` mirrors it, and both run `internal/conditions/testdata/cases.json`. Left for later, each with its reason in the decision table below: the warning in the template list for a deleted group (TPL-1468, with the state column of GT4); the measurements on the real-data dump (TPL-1422).

## GT4: rights per template and approval

**Scope:**
- TPL-060 to TPL-062;
- TPL-500 to TPL-505;
- TPL-510 to TPL-512;
- TPL-520 to TPL-525;
- TPL-530, TPL-531;
- TPL-602 (the comparison, for approvers);
- TPL-711 (the approval and rights events), TPL-712;
- TPL-720 to TPL-723;
- TPL-853;
- TPL-900;
- TPL-1000 to TPL-1002, TPL-1030;
- TPL-1102.

**Data** (migration `00xx_template_rights`, expand only):
- `template_rights`: template, use or edit, subject;
- `template_spaces`: the spaces a template of all spaces is offered in;
- `template_reviews`: template, version, decision, person, comment or reason, time;
- the permission `template.propose`, given to every role with `page.create` (TPL-1102);
- the space and instance setting "every template change needs a second person", off.

**Tests:**
- the permission matrix over see, use, edit, propose, approve and set rights, for manager, proposer, member, outsider, guest, administrator and a person in no group (TPL-063);
- the audience lists only narrow; a listed person without `page.create` cannot use; the lock-out refusal (TPL-500, TPL-501);
- the spaces offered, a deleted space, an empty list (TPL-502);
- rights checked again on submit (TPL-503);
- "not found" on every route for an outsider, including the automation editor (TPL-504, TPL-505);
- the four states and who sees each (TPL-510);
- direct publishing for managers, the four-eyes setting, and no self-approval (TPL-511, TPL-520);
- an administrator approving templates of all spaces, and the single-administrator case (TPL-521);
- approval bound to a version; a change after proposal withdraws it (TPL-522);
- the approved version stays in use while a new one waits (TPL-523);
- withdrawal: an open form is refused on submit (TPL-524);
- approval refused for a template that no longer validates (TPL-525);
- two approvers at once give one decision; approving a superseded version is refused (TPL-855, TPL-856);
- one notification per set for watchers, partial access, mentions grouped, approval notifications, and no answer values in any notification (TPL-720 to TPL-723);
- the adoption test for `template.propose` (TPL-1102);
- the gallery query count and EXPLAIN with audience lists (TPL-900);
- pictures of the lists, the waiting section and the decision screen (TPL-1001, TPL-1002, TPL-1030).

**Mutations:**
- an audience list that grants instead of narrowing;
- self-approval with four eyes on;
- a waiting version offered before approval;
- "forbidden" instead of "not found" for an outsider.

**Status (2026-11-15):** built, with migration `0070_template_rights`. Report: [2026-11-15-wave-gt4.md](../reports/2026-11-15-wave-gt4.md). A template has a users list and an editors list that only narrow, a template of all spaces its chosen spaces, and its versions the states draft, waiting, approved, sent back, superseded and withdrawn. `template.propose` is split off `page.create`; a person with it writes drafts in My templates and proposes them. Managers publish directly unless the scope asks for a second person. Waiting and withdrawn versions make no pages, for anyone. Left for later, each with its reason in the decision table below: the readable change list of TPL-602 (GT5; the decision screen shows the text comparison against the version in use); widening a template of a space to all spaces (TPL-512, TPL-1487); the pictures of a list of fifty templates (TPL-1001); a test of an automation rule whose owner left a users list (TPL-505); the measurements on the real-data dump (TPL-1422).

## GT5: history, origin, search, export and the rest

**Scope:**
- TPL-602 (the full history screen);
- TPL-604;
- TPL-611 to TPL-613 and TPL-620;
- TPL-700 to TPL-703;
- TPL-852 and TPL-854;
- TPL-907;
- TPL-1031;
- TPL-1105 to TPL-1108.

**Tests:**
- the change list between two versions for every kind of change (TPL-602);
- automation uses the approved version, and fails clearly on a required field without a default (TPL-604, TPL-1105);
- origin in Details for both kinds of reader; no page changes when a new version is approved (TPL-611, TPL-612);
- the sets of a version, never showing a page the manager cannot see (TPL-613);
- templates and entries never indexed; the server gallery search with 150 templates and audience lists; `template:` in search (TPL-700, TPL-701, TPL-703);
- export and import round trip, an imported template in draft, a missing group (TPL-852, TPL-1106);
- a restore drill step with a waiting background set (TPL-854);
- turning the part off and on again (TPL-1107);
- the burst of 20 sets of 10 pages in a minute in the next stress test on the server (TPL-907).

**Mutations:**
- the origin copied by a page copy;
- an import approved without review;
- the search filter showing a template outside the audience.

**Guide:** `docs/operations/templates.md`, for administrators. It covers:
- turning the part on;
- who may propose and approve;
- the four-eyes setting;
- what approval does and does not check (TPL-844).

**Status (2026-11-15):** built, with migration `0074_template_origin_search` (its final number is given when it lands). Report: [2026-11-15-wave-gt5.md](../reports/2026-11-15-wave-gt5.md). Guide: [operations/templates.md](../operations/templates.md).
- **History:** the comparison of two versions gains a readable change list for every kind of change (`internal/templates/changes.go`), which `template.version_saved` also keeps; the Versions section compares a version with any earlier one, counts the sets of each version for its managers and lists them (TPL-602, TPL-613).
- **Origin:** the Details panel says what a page was made from, by name only to who may see the template, with the other pages of its set and "a newer version is in use" for who may edit the page (TPL-611, TPL-612, TPL-1031).
- **Search:** the gallery searches on the server above 100 templates, also by the names of the pages a template makes, through the gallery's own filter (TPL-701); `template:` finds pages by their origin (TPL-703).
- **Export and import:** files carry format 2 and nothing about who has a template; an import is a draft of the importer, people of the other installation never travel, and a restriction to choose again blocks approval (TPL-852, TPL-1106).
- **Automation:** a rule uses the approved version and says why a run made nothing (TPL-505, TPL-604, TPL-1105).
- **Restore and switching:** a step of the restore drill, a test of a waiting background set whose version was withdrawn, and a test of turning the part off and on (TPL-854, TPL-1107).
- Also from GT4: the space audit's template events (TPL-712), the starters after the migration (TPL-531), and the pictures with no template and fifty (TPL-1001).

Left for later, each with its reason in the decision table below: the burst of TPL-907, which belongs to the next stress test on the server (TPL-1512); the files of a template in its file (TPL-1501); a dropdown for `template:` in the filters of search (TPL-1507); the action "new page from a template" in the rule editor of the browser, where the test run warns instead (TPL-1510); widening a template of a space to all spaces (TPL-1487); the measurements on the real-data dump (TPL-1422).

## Migrations

| Wave | Migration | What it does |
|---|---|---|
| GT1 | `0060_guided_templates` | `template_versions`, `template_entry_pages`, `template_sets`, `page_template_origin`, pending copies; new columns on `templates`; version 1 of every template, approved |
| GT4 | `0070_template_rights` | `template_rights`, `template_spaces`, `template_reviews`; `template.propose` for every role with `page.create`; the four-eyes setting |
| GT5 | `0074_template_origin_search` | the function `template_origin_pages`, through which search finds the pages made from templates (TPL-703) |

Numbers are given when each wave lands. Both migrations are expand only. A contract migration may later drop the fallback for templates saved without a manifest (TPL-1108).

## Load budget (ARC-238)

TPL-900 to TPL-908:
- **Gallery:** one query, the audience joined as hash joins; 100 templates within 30 ms.
- **A set:** one transaction, writes batched per table. 10 pages within 300 ms, and 100 pages within 3 s as background work. A single page within 10% of today.
- **Tree lock:** held for the set's transaction, in that space only; hard stop at 60 s.
- **Audit:** about *n* + 1 events per set; the queue empty within 5 s of a 100-page set.
- **Dry run:** 150 ms for 10 pages, 1 s for 100.
- **Conditions:** a 100-entry template within 1 ms.
- **Storage:** a manifest of at most 64 KiB.
- **Expected rate:** a few dozen sets a day on an instance of a few hundred people (ADR-043's claim). This is an assumption, checked by the stress test.

## Decisions

Decided on the product owner's behalf on 2026-11-14, may be overruled.

| # | Question | Decision | Reason |
|---|---|---|---|
| TPL-1401 | Experimental under ADR-043? | Yes: `guided_templates`, off by default, also after an upgrade | ADR-043 freezes new parts; ADR-038 wants the feature before 1.0; both hold |
| TPL-1402 | Can a set span spaces? | No: one space, one subtree | One lock, one transaction, one rule set |
| TPL-1403 | How large can a set be? | 100 entries, 6 levels; background work above 25 pages, still one transaction | Covers onboarding and projects; keeps the tree lock short |
| TPL-1404 | Drafts or published? | Drafts by default; the template may say "publish at once" | Today's behaviour; publishing many pages should be the author's choice |
| TPL-1405 | How are conditions written? | All or any of up to 10 tests, one level, picked from lists; no expressions | Readable, total, and auditable; as DOC-1404 |
| TPL-1406 | Who decides a condition? | The server, again on submit; the browser previews | The browser cannot be trusted |
| TPL-1407 | Answers to hidden fields? | Dropped | The person never saw the question |
| TPL-1408 | Regular expressions in validation? | No; length, range and named formats | Browser and server differ; authors get them wrong |
| TPL-1409 | Are answers stored? | No; only in the pages they fill; the set keeps field names | Data minimisation |
| TPL-1410 | Who may propose templates? | `template.propose`, given to every role with `page.create`, removable | Nobody loses anything; administrators can narrow it |
| TPL-1411 | Do managers need approval? | No, unless four eyes is on | Approval is for templates written by others |
| TPL-1412 | Four eyes by default? | Off | Most teams trust their managers; regulated teams turn it on |
| TPL-1413 | Approval through a document-control workflow? | No; one decision by one person | One step needs no workflow; DOC-561 |
| TPL-1414 | What is used while a new version waits? | The approved version | Nobody is left without a template |
| TPL-1415 | Can rights per template grant access? | No; they only narrow | Deny by default |
| TPL-1416 | Do pages follow template changes? | Never; no "apply the new version" | ORG-058; pages are people's work |
| TPL-1417 | Missing `restriction.manage` for a restricted entry? | Refuse the whole set | A restricted page must never exist open |
| TPL-1418 | A page per item of a list? | Later | Sets of unknown size need their own study |
| TPL-1419 | A shared library of variables? | No, per template | No need shown; it would need rights and versions |
| TPL-1420 | Merge space templates and sets? | No; space templates stay | Different jobs |
| TPL-1421 | Conditions on the creator's groups? | Yes, with a note that they are not access control | Real need; the text stays readable by editors |
| TPL-1422 | What data do budgets use? | The real-data dump, sets of 10 and 100 pages | Measured, never claimed |
| TPL-1423 | Where do administrators approve? | Administration, Content, Templates | One place per scope |
| TPL-1424 | Where are templates of all spaces offered? | Every space or chosen spaces, as chips | The design standard's rule for instance-wide things |

### Decided while building GT1

Decided on the product owner's behalf on 2026-11-14, may be overruled.

| # | Question | Decision | Reason |
|---|---|---|---|
| TPL-1425 | Open question 1: one rule for the freeze of all three enterprise features? | No change now: document control keeps no switch; guided templates stay experimental | Document control was built before ADR-043 and is already part of the 1.0 claim; a switch added now would change a shipped part |
| TPL-1426 | Open question 2: may people without `space.templates` write and propose templates? | Yes, as TPL-061 plans, in GT4 | Approval only means something when others can write; nothing of it is built before GT4 |
| TPL-1427 | Open question 3: when is `StartSpace` (TPL-008) fixed? | On its own, in the wave after GT1, with its own test; not inside GT1 | The spec keeps space templates out of these waves (TPL-1203); GT1 already changes the one path of making pages, and two such changes in one wave are harder to review. TPL-006 is fixed in GT1 by TPL-122 |
| TPL-1428 | What does "the grace period of the upgrade" mean for turning the part on (TPL-1103)? | While any node seen in the last minute knows fewer migrations than this one | That is exactly when an older release can meet a guided template; a fixed day would be too long on one node and too short with a stuck node |
| TPL-1429 | What is the first entry's name, and its title when nothing is filled in? | It has no name; an empty title stays empty, as a one-page template always did | A one-page template must make what it made before (TPL-1101); a draft may wait for its title (PAG-003) |
| TPL-1430 | A question nobody answered, in a title pattern | Counts as empty, so the entry's name stands in (TPL-140) | `{Field}` left in a title reads as a mistake |
| TPL-1431 | Where are the fields of a version? | In the texts the manifest pins, not copied into the manifest | One place for them; GT2 adds what a field needs beyond the text (kinds, defaults, validation) to the manifest |
| TPL-1432 | Removing an entry | Takes it out of the next version; its hidden page stays while the template exists | Older versions point to its text (TPL-601) |
| TPL-1433 | How is a ready or failed background set made known? | The job's status, which the form shows; the bell comes with the notifications of GT4 (TPL-720) | Notifications of sets are planned once, with their rules about who is told |
| TPL-1434 | Answers of a background set whose worker stopped | Kept with the job until it is retried or purged; every other ending removes them | A retry must make the same set (TPL-121); a stop is not an end |
| TPL-1435 | A background set over its limit (TPL-827) | Goes back to the queue and rests one second there, letting other jobs pass | No worker spins on it, and nothing behind it waits |
| TPL-1436 | A parent with an edit restriction (TPL-810) | Does not refuse a set: under the access rule, making a page below one is not an edit | The rule of the product holds everywhere; a view restriction does refuse, and is tested |
| TPL-1437 | A link of another scheme in a template | Refused on save, as every document is (`Normalize`) | The same rule as pages; nothing is quietly dropped |
| TPL-1438 | Who is on the restriction of a page made from a restricted entry | The template's people and groups, and its creator, as when a restriction is set by hand (ACC-065) | Nobody makes a page they cannot open |
| TPL-1439 | Which empty page makes way for a set (TPL-122) | Only an unpublished page of one's own with no title, no text and no pages below it | A page with any content is someone's work |
| TPL-1440 | An entry's workflow in a space that has a space workflow | Needs no right: the space workflow wins (DOC-420) | The entry's choice is not used, so nothing is chosen |

### Decided while building GT2

Decided on the product owner's behalf on 2026-11-14, may be overruled.

| # | Question | Decision | Reason |
|---|---|---|---|
| TPL-1441 | Where do the settings of a question live? | In the manifest, as `fields`, holding only what the text does not already say; the field in the text keeps its name, and `line`, `text` or `choice` as its kind | One place per fact (TPL-1431); the previous release reads an unknown kind as a line |
| TPL-1442 | Does saving the Form section make a template guided? | Only when a question gets a setting beyond its text; settings equal to the text are dropped on save | A plain template must stay offered while the part is off (TPL-050) |
| TPL-1443 | Which templates get the check step? | Guided templates, while the part is on. A plain template keeps the form it had, with the place on its last step | Today's templates behave as today, and their tests run unchanged |
| TPL-1444 | Is the place of the pages a step of its own (TPL-333)? | No: it sits under the questions of the last step; a template without questions gets a step with the place alone | "The last step sets the parent"; one more click for every form would add nothing |
| TPL-1445 | How does an answer of several people or options travel? | One per line in the same text answer | The API keeps `answers` as text; an option cannot hold a line break |
| TPL-1446 | How does a number land in the page? | As typed, with the decimal sign the person used. No grouping sign is read: `1,000` is one with three decimals | The server has no locale formatting without a new dependency; what the person typed is how their locale writes it (TPL-303) |
| TPL-1447 | The words of a yes/no answer in a page | "Yes" or "No", or nothing when the author chooses | Kenwerk speaks one language today (TPL-212) |
| TPL-1448 | Which guests and people can be chosen? | Anyone the access rule lets the creator see (`CheckPerson`: a shared space or `profiles.view`), while their account is active | One rule for who sees whom; a disabled person or a service account is never an answer |
| TPL-1449 | A suggested person who later leaves | The form shows the field empty with a note, the server treats it as no answer, and the next save of the template is refused until the suggestion changes | TPL-316; nobody is assigned who cannot act |
| TPL-1450 | How do refused answers come back? | `fields` in the error, each with the field, a problem code, its values and English words; the form translates the code | The form shows the words of its own language beside the field (TPL-324) |
| TPL-1451 | Are the versions routes experimental? | No: listing and comparing versions leave the switch | ORG-060 finishes a part every template has (TPL-1104) |
| TPL-1452 | What does the Versions section compare? | Each page's text with the version before, in the comparison pages use, and a note when anything else changed | ORG-060 asks for the same comparison as pages; the full list of changed settings is TPL-602, GT5 |
| TPL-1453 | Which sections does the editor have in GT2? | General, Pages, Form and Versions | Rules need conditions (GT3) and Who can use it needs rights (GT4) |
| TPL-1454 | Where is the text of a page other than the first written? | In place, below the Pages section's list, as GT1 built it; the first page in the editor | Moving it to a Focus frame of its own is a new screen and route; left for when the sections are finished (TPL-1011) |
| TPL-1455 | Where does the panel of templates stand (ORG-045)? | Beside the sheet where the editor has room for it, otherwise above the sheet | The design standard's rule for side lists: beside when there is room, never over the text |
| TPL-1456 | A step that names a question no text asks any longer | Kept, and the question shows nowhere until a text asks it again | Nothing quietly dropped; a field removed and added back lands in its step |
| TPL-1457 | A line break in a one-line answer | Refused (`one_line`), also through the API | The form cannot type one; the server decides as the form does (TPL-320) |
| TPL-1458 | A question named `date`, `space`, `creator`, `set` or `template` | Keeps working as before; giving it settings is refused until it is renamed | TPL-230: the editor asks for a new name at the next save |
| TPL-1459 | What does Try the form show? | The saved version, with the dry run at its end; a change is tried after Save | The dry run is the server's, and the server knows only what is saved |
| TPL-1460 | The usual place of a template of all spaces | Not offered: only a template of a space has one | A page belongs to one space; the server already ignores a place in another space (TPL-802) |
| TPL-1461 | An upgrade from GT1 with the part on | No grace refusal, because GT2 has no migration: the changelog says to finish the upgrade before changing questions | A node of GT1 keeps no settings when it saves; experimental and off by default, so the note suffices |
| TPL-1462 | An answer given empty to a question with a suggestion | It is an answer, and a required question refuses it; only a question left out takes the suggestion | The person cleared it on purpose |

### Decided while building GT3

Decided on the product owner's behalf on 2026-11-15, may be overruled.

| # | Question | Decision | Reason |
|---|---|---|---|
| TPL-1463 | Is "≠" of a number true on an empty answer? | Yes, like "is not" | "≠" is the "is not" of numbers; TPL-403 names "is not" as true on empty |
| TPL-1464 | What is an unanswered yes/no question? | A no: "is no" holds, "is yes" does not | A yes/no question is never required because no is an answer (TPL-305) |
| TPL-1465 | Do "is" and "is not" on words mind capitals? | No, like "contains"; an option, a label and a date compare exactly | People type words in any case; options and labels have one spelling |
| TPL-1466 | Which rights does a set need when a condition leaves pages out? | Those of the pages it makes, checked after the conditions | A page that is not made needs no right; a condition still never grants one (TPL-424) |
| TPL-1467 | Where is each condition set? | A question's in its row of the Form section, a step's in its row, a page's in the Pages section through its own route, a rule's in the Rules section, a section's from its block in the text | Each condition stands next to what it switches; a route of its own for a page keeps an edit of the page from clearing it |
| TPL-1468 | The warning in the template list for a deleted group (TPL-402) | Not in GT3: a save is refused while a condition names one, and the test is false meanwhile; the warning comes with the list's state column (TPL-1001, GT4) | The list gains its columns once; nothing is made wrongly in the meantime |
| TPL-1469 | A text change that removes a question a condition tests | Refused, naming the condition, until the condition changes | A condition never points at nothing (TPL-230, TPL-432) |
| TPL-1470 | Which files does a page get when a section is removed? | Every file of its template but those only removed sections showed; a file the text never showed is copied as before | TPL-422 asks that files in a removed section are not copied; everything else behaves as today |
| TPL-1471 | How does the form preview a test of the creator's groups? | The template tells the reader which of the tested groups they are in, and which are gone | The reader's own membership leaks nothing; the server decides on submit |
| TPL-1472 | What may a rename choose? | One line of at most 100 bytes, no braces, not the name of another question ignoring capitals, and no reserved name | The same rules as a question has; braces would break a title pattern |
| TPL-1473 | How does a test of the space choose spaces? | Checkboxes of the spaces, for a template of all spaces only | A template of a space is only used in that space |
| TPL-1474 | An answer of more items than a question holds (over 50) | Every test of a list on it is false | Only a tampered answer has that many, and it never adds content |
| TPL-1475 | A question hidden by a condition, with a suggested answer | The suggestion is not used: the question is empty everywhere | A person cannot be bound by an answer they never saw (TPL-1407) |
| TPL-1476 | An upgrade from GT2 with the part on | No grace refusal, because GT3 has no migration: the changelog says to finish the upgrade before adding conditions | As TPL-1461; experimental and off by default |

### Decided while building GT4

Decided on the product owner's behalf on 2026-11-15, may be overruled.

| # | Question | Decision | Reason |
|---|---|---|---|
| TPL-1477 | Who turns "every template change needs a second person" on and off? | Whoever runs the space (`space.administer`), and an administrator for templates of all spaces; those who manage templates see it | The setting binds the managers of templates, so they should not be the ones to turn it off |
| TPL-1478 | May an author decide on their own version? | Only where no second person is asked for, and only as one who manages the template: they could publish it directly anyway | TPL-511 lets managers publish their own work; four eyes forbid it (TPL-520) |
| TPL-1479 | May the owner use a waiting version, as TPL-510's table says? | No: a waiting or withdrawn version makes no pages for anyone. The owner tries a draft, or one sent back, before proposing | What an approver decides on is exactly what the owner tried; a waiting version cannot change, so nothing is lost |
| TPL-1480 | What does a try of a draft make? | Drafts of the owner, even when the template publishes at once | TPL-510: a test use makes real pages as drafts of the owner only |
| TPL-1481 | How long does a form opened on a replaced version work? | A day from the approval that replaced it (TPL-603's 24 hours); a withdrawal ends it at once | The form was opened on an approved version; after a day the check step may no longer be true |
| TPL-1482 | The lock-out of an editors list (TPL-501) | A list that leaves out the person setting it, by name or group, is refused; nobody is added silently | The person sees what the list says; nothing is changed behind their back |
| TPL-1483 | What can a list hold? | People and groups, at most 50 each; not "all members" or "all guests" | An empty list already means everyone; as page restrictions |
| TPL-1484 | What holds while the part is off? | A template with a list or chosen spaces is not offered (fail closed); the new routes answer `experimental_off`; a change in a scope with four eyes still waits, and the approved version stays in use | A narrowed template must never open to everyone (TPL-050); turning the part off must not bypass a second person |
| TPL-1485 | What does a screen read: the newest version or the one in use? | The editor and the settings read the newest (`GET /templates/{id}/newest`); a form, the gallery and the preview the version in use | A new version may wait while another is used (TPL-523) |
| TPL-1486 | A manager changes a template whose newest version is someone's draft | The change builds on that draft; without four eyes it is published with it | The editor shows the newest text; the manager is the one who checks (TPL-1411) |
| TPL-1487 | Widening a template of a space to all spaces (TPL-512) | Not in GT4: a template keeps the scope it was made in | No route widens a template today; it needs its own design for the files and the hidden page in the system space |
| TPL-1488 | Where is Withdraw? | At the end of the Versions section, a `danger` button that opens a dialog for the reason, whose button repeats the action | A template has no More menu yet, and a withdrawal needs a reason (TPL-524) |
| TPL-1489 | Does a reason travel in a notification or a mail? | No: a row holds ids only; the author reads the reason in the Versions section | A notification never holds a sentence (spec 010, D11); a reason may be personal |
| TPL-1490 | A person mentioned in several pages of one published set (TPL-721) | One row per person and set, listing the pages; its mail goes once | One action, one notification |
| TPL-1491 | An upgrade from GT3 with the part on | Lists, drafts and the four-eyes setting are refused while a node of an older release runs (`older_release_running`) | An older node knows no list or state and would offer such a template to everyone |
| TPL-1492 | Who sees a draft that was never proposed? | Its owner only, until the owner's account is no longer active; then the managers of its scope, who may delete it (TPL-853) | A draft is someone's work in progress until they ask for a look |
| TPL-1493 | Who deletes a template? | Those who manage it, and its owner while it was never approved | An owner may throw away their own unused draft; an approved one belongs to the space |
| TPL-1494 | Where does a proposer make a template? | `POST /spaces/{id}/my-templates`, declared for `template.propose`; the managers' route stays declared for `space.templates` | Each route declares the permission it is for |
| TPL-1495 | Is a withdrawal refused when the template no longer validates (TPL-525)? | No: only approval checks again | Taking a template out of use must never be blocked by what is wrong with it |
| TPL-1496 | How do bundles carry `template.propose`? | Bundle version 9; a grant of `page.create` in an older bundle brings it, as the migration did | The rule of every earlier split (spec 045, 049, 064, 073) |
| TPL-1497 | The wider state checks of the migration | Added `NOT VALID`; a later contract migration validates them | No table is read under its lock, and every row passes the narrower check it replaces |

### Decided while building GT5

Decided on the product owner's behalf on 2026-11-15, may be overruled.

| # | Question | Decision | Reason |
|---|---|---|---|
| TPL-1498 | How is the readable change list (TPL-602) kept? | As lines of a code and the names the author typed, in the comparison and in `template.version_saved` (at most 50 lines); the browser words them | One list for the screen and the audit, in every language; it never holds an answer or an id |
| TPL-1499 | How are steps matched between versions? | By their title; a renamed step reads as one removed and one added | Steps have no key, and their title is how people tell them apart |
| TPL-1500 | What does a template file carry (TPL-852)? | The version in use, or the newest while none is approved: pages, texts, questions, steps, conditions, label rules, publishing at once, and the groups of restrictions by id and name with a count of the people left out. Never the users or editors list, the chosen spaces, the state, the history, the owner, a reviewer, a person, a workflow or the usual place | Who has a template belongs to one installation; a person's id from another installation must never become a grant here (as DC5-14 for workflows) |
| TPL-1501 | The files of a template in its file | Not in GT5: the file stays one JSON body, as today's export | A file needs its own transport, as the bundles of a space have; nothing is half carried meanwhile |
| TPL-1502 | What does an import become? | While guided templates are on: a draft of the importer, whoever imports and whatever the four-eyes setting, which follows the approval rule of its scope. While they are off: a plain file imports as before, and one that needs guided templates is refused (`experimental_off`) | An imported template has not been checked here; today's templates behave as today while the part is off |
| TPL-1503 | People and groups of a restriction in an imported file | People are left out; a list they leave empty holds the importer, so the page stays restricted; a group that is not here stays and matches nobody. Either way the page is marked to choose again, and no version is approved, at once or by a decision, until its lists change (`choose_again`) | A restricted page must never open to everyone (TPL-1417), and the importer decides who is meant here |
| TPL-1504 | A group that is gone, in a draft | A draft may keep a condition testing it or a restriction naming it; a version that is approved, at once or by a decision, may not. This narrows TPL-1468 to versions in use | An import must be able to wait for its groups to be chosen; a draft makes pages only for its owner, and then refuses a restriction it cannot set |
| TPL-1505 | How does the gallery search on the server (TPL-701)? | Name, description, category and the names of the pages of the approved version, ignoring capitals, at most 100 characters, as one more condition of the gallery's own query; the browser asks the server above 100 templates | The same filter decides who sees what, so the search cannot show more than the gallery |
| TPL-1506 | What does `template:` match (TPL-703)? | The name of a template, whole and ignoring capitals, among those the reader may see; while the part is off, `template:` reads as words, as before | A filter by a name the reader may not see tells nothing; a search that worked before still works |
| TPL-1507 | A dropdown "made from template" in the filters of search | Not in GT5: the syntax only | The filter bar lists known values; a list of every template a reader may see is its own question to the templates module |
| TPL-1508 | What does the origin in Details show (TPL-611, TPL-612)? | Name, version and date to who may see the template, a link to its history to who looks after it, "a newer version is in use" to who may also edit the page, and the other pages of the set the reader may see; "made from a template" to others | TPL-504: a name can say too much; the history is for those who look after the template |
| TPL-1509 | Who sees the sets of a version (TPL-613)? | Who manages the template: the count of all, the 100 newest whose first page they may see, the rest as a number | A manager learns what an older version made without seeing a page closed to them |
| TPL-1510 | How does an automation rule use a template (TPL-505, TPL-604, TPL-1105)? | Its owner's rights, the version approved when it runs; it refuses with `template_not_approved`, `template_asks` naming the question, or `template_not_offered`. The rule's test run warns about the same; the rule editor of the browser does not offer the action yet | The history of a rule keeps a code, never words that may hold a title; the warning stands where a rule is tried |
| TPL-1511 | What does a full restore do to sets (TPL-854)? | Templates, versions, approvals and origins return to the backup; a set asked for after it is gone with its job; a waiting set checks its version again when it runs | The restore drill proves it; the check of TPL-503 runs in every set's transaction |
| TPL-1512 | The burst of 20 sets of 10 pages (TPL-907) | Measured once in the development containers as an indication; judged in the next stress test on the server | The containers share their machine with other builders and run nothing beside the burst, so they cannot judge it against the normal mix |
| TPL-1513 | The reminder of TPL-844 | One sentence beside Approve | Approval checks that a template works here, not its words |
| TPL-1514 | A list with no template | Says so in a muted sentence, without an empty table | The design standard's state Empty |
| TPL-1515 | A test of the space in an imported template of all spaces | Kept as it is; spaces of another installation match nothing here, so the pages it shows are not made until a space is chosen again | Nothing is quietly widened: a page that should show in some spaces shows in none |
| TPL-1516 | An upgrade from GT4 with the part on | No grace refusal: 0074 adds a function the previous release does not call, and a file of format 2 is refused whole by it | As TPL-1461 |

## What these waves do not cover

Chapter 14 of the study, in short:
- scripting and expression languages (TPL-1200);
- formulas, nested logic and comparisons between fields (TPL-1201);
- a page per item of a list (TPL-1202);
- merging space templates and sets, and the non-atomic `StartSpace` finding (TPL-1203, TPL-008);
- pages that follow their template (TPL-1204);
- rights from answers (TPL-1205);
- a library of variables (TPL-1206);
- fields for files, rich text, signatures or secrets (TPL-1207);
- forms that send data elsewhere (TPL-1208);
- sets across spaces (TPL-1209);
- posts and databases as entries (TPL-1210);
- approval through a workflow (TPL-1211);
- regular expressions (TPL-1212);
- draft and published spaces, which have their own study (TPL-1213);
- reports on answers (TPL-1214).

## Open for the product owner

1. **The freeze:** ADR-038 and ADR-043 are read together as "build it, experimental and off" (TPL-1401). Document control has no such switch. Should the three enterprise features follow one rule?
2. **Who writes templates:** should people without `space.templates` write and propose templates at all (TPL-061)? If not, GT4 shrinks to rights per template and the four-eyes setting.
3. **`StartSpace`** makes a space and then each page in its own transaction (TPL-008). A failure leaves a half-made space. Fix it on its own, before or beside GT1?
