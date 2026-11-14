# Spec 074: Guided templates

Study: [blueprint 09, guided templates](../blueprint/09-enterprise/templates/01-guided-templates.md), chapters 02 to 14, lines TPL-001 to TPL-1214. Basis: [08-roadmap/02-enterprise-document-needs.md](../blueprint/08-roadmap/02-enterprise-document-needs.md) (RMP-014). This is the third of the three enterprise features before 1.0 (ADR-038, spec 048, D14). It is a clean-room build. Templating.app is named only to say which behaviour is meant: its public pages describe issues, and here the same behaviour is meant for pages. The requirements come from that behaviour, from research on forms, and from Diffluence's own blueprints, never from a product's text, layout or source.

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

## Migrations

| Wave | Migration | What it does |
|---|---|---|
| GT1 | `00xx_guided_templates` | `template_versions`, `template_entry_pages`, `template_sets`, `page_template_origin`, pending copies; new columns on `templates`; version 1 of every template, approved |
| GT4 | `00xx_template_rights` | `template_rights`, `template_spaces`, `template_reviews`; `template.propose` for every role with `page.create`; the four-eyes setting |

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
