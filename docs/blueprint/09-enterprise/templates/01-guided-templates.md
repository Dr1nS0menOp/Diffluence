# 09.T1: Guided templates

Blueprint 09 holds the enterprise document features that come before 1.0 (ADR-038). Document control came first ([01-document-control.md](../01-document-control.md), spec 064). This part covers **guided templates**. One action creates a whole set of linked pages from a template. A form with many kinds of field guides the person who creates them, without scripting. Conditions decide which content goes in. Rights are set per template, and an administrator approves the templates that others may use. The plan to build it is [spec 074](../../../specs/074-guided-templates.md).

## How to read this study

Every line has a number (TPL-NNN) and a status:
- **built**: in the code today;
- **partly**: started, with what is still open;
- **new**: planned in spec 074;
- **later**: wanted, but not in spec 074.

Each line gives the behaviour, then:
- *Why*: the reason, citing research or a standard where one applies;
- *Edge cases*;
- *Proof*: how a test will show it.

The sources are listed in [sources.md](sources.md).

This is a clean-room study (`docs/blueprint/02-clean-room-and-licensing.md`). Blueprint 08 names Templating.app only to say which behaviour is meant. Its public pages describe an app that makes issues in an issue tracker; here the same behaviour is meant for pages. The study read that app's public documentation for behaviour only: what it does, never how it looks. No text, field names, icons or layouts were taken from it. The rest of the study comes from:
- form builders, document assembly and project generators, read through their public documentation;
- research on forms and wizards;
- WCAG 2.2;
- Diffluence's own code and blueprints.

No product's source was read.

## Chapters

| Chapter | Lines | About |
|---|---|---|
| [02 What people need](02-what-people-need.md) | TPL-010 to TPL-029 | The situations, and the needs that follow from them |
| [03 Sets of linked pages](03-template-sets.md) | TPL-100 to TPL-159 | One action, many pages: the structure, the links between them, naming, all or nothing |
| [04 Variables and their scope](04-variables.md) | TPL-200 to TPL-239 | Answers, built-in values, links to pages of the set, where each can be used |
| [05 Fields and the form](05-fields-and-the-form.md) | TPL-300 to TPL-359 | Field types, validation, defaults, required fields, steps, checking the answers |
| [06 Conditions](06-conditions.md) | TPL-400 to TPL-439 | What decides, what a condition switches, and how it is evaluated safely without scripting |
| [07 Rights and approval](07-rights-and-approval.md) | TPL-500 to TPL-549 | Who sees, uses and edits a template, where it is offered, and approval by an administrator |
| [08 Versions](08-versions.md) | TPL-600 to TPL-629 | Versions of a template, and pages made from an older version |
| [09 Search, audit and notifications](09-search-audit-notifications.md) | TPL-700 to TPL-729 | Finding templates, the audit record, who is told what |
| [10 Edge cases and failure paths](10-edge-cases.md) | TPL-800 to TPL-859 | Missing spaces, missing rights, half-created sets, large sets, forbidden content |
| [11 Load budget](11-load-budget.md) | TPL-900 to TPL-909 | What it costs, with numbers |
| [12 The screens](12-screens.md) | TPL-1000 to TPL-1049 | The template editor, the form, approval, following the design standard |
| [13 Migration](13-migration.md) | TPL-1100 to TPL-1109 | Today's templates keep working |
| [14 What this feature will not do](14-what-it-will-not-do.md) | TPL-1200 to TPL-1219 | Scope kept out, with reasons |

## 1.1 What exists today

Read in the code on 2026-11-14. The files read:
- `internal/templates/templates.go` and `spacetemplates.go`;
- `internal/pages/templates.go`;
- `internal/document/template.go` and `attrs.go`;
- `internal/automation/actions.go`;
- migration `0008_organising.sql`;
- `web/src/components/template-gallery.tsx` and `web/src/views/templates.tsx`;
- spec 011, section D4.

- **TPL-001** · built · A template is a hidden page of kind `template` (spec 011, D6). It has a row in `templates` holding:
  - its name, description and category;
  - its labels, a title pattern and its steps;
  - a default parent;
  - whether it is hidden or promoted, and whether it is a starter.

  A template of a space has a `space_id`; a template of all spaces has none, and lives in the system space. `perm_visible_pages` and `Evaluate` refuse a template page to everyone, so the templates module holds the rule of who may use one (ORG-045 to ORG-047).
- **TPL-002** · built · **Fields** are an inline block, `templateField`, with a name and one of three kinds: `line`, `text` or `choice`. A field has options and can be required. Fields with the same name share one answer. A template has at most 20 fields, and an answer has at most 2,000 characters. An answer is plain text: it becomes a text node, so it can carry no link and no block (`FillFields`, ORG-050). A field nobody filled in stops the publish (`ForPublish`).
- **TPL-003** · built · **Instruction text** shows only in the editor and is removed when a page is published (ORG-049).
- **TPL-004** · built · The **title pattern** fills in `{date}`, `{space}` and the names of fields, in one pass (`ExpandTitle`, ORG-051).
- **TPL-005** · partly · **Steps** group fields into up to ten screens of a form (ORG-061). Still open:
  - writing steps on the template screen;
  - the last step for labels and parent.
- **TPL-006** · built · **Using** a template makes **one** unpublished page with a draft, in one transaction (`StartFromTemplate`):
  - it checks `page.create` and the parent;
  - it copies the files;
  - it adds the labels, and reports those a fixed list of the space left out.

  Later changes to the template change nothing (ORG-058). The browser deletes the empty page that was waiting for the template **in a second call**, so that step is not part of the transaction.
- **TPL-007** · built · Each save of a template's text is a page version of its hidden page (ORG-060). The `templates` row itself (steps, title pattern, labels) is not versioned.
- **TPL-008** · partly · A **space template** is a list of pages, stored as JSON, used to create a new space (ORG-055, SPC-006). `StartSpace` creates the space and then each page **in its own transaction**. If page 30 of 50 fails, the space exists with 29 pages. This study does not change space templates (TPL-1203), but sets must not repeat this (TPL-120). The finding is recorded here so that a fix can be planned on its own.
- **TPL-009** · built · Rights today:
  - a template of a space is seen by those who may view the space, and one of all spaces by everyone;
  - using one needs `page.create` in the target space;
  - managing needs `space.templates`, or administrator for templates of all spaces;
  - the automation action `new_from_template` uses a template with its rule owner's rights, and gives no answers.

What is missing is everything in chapters 03 to 13:
- a set of pages at once;
- more field types, with defaults and validation;
- conditions;
- rights per template;
- approval;
- versions of the whole template.

## 1.2 Experimental, and off by default

- **TPL-050** · new · Guided templates are an **experimental part** under ADR-043, named `guided_templates` in the `experimental` setting of spec 069. The part is off on a new installation and off after an upgrade. An administrator turns it on in Administration, Organising, "Experimental parts" (decided, TPL-1401).

  *Why:*
  - ADR-043 freezes new parts before 1.0, and makes optional parts experimental and off by default.
  - ADR-038 still puts the enterprise features before 1.0, and the product owner wants this one built before 1.0.
  - Building it as an experimental part meets both: the code ships in 1.0, but the 1.0 claim does not rest on it.
  - It also adds new rights (chapter 07), which the one outside reader at 1.0 (ADR-043) would otherwise have to cover.

  *Edge cases:*
  - With the part off, templates behave **exactly as today** (TPL-001 to TPL-009).
  - The remaining "partly" lines of today's templates (ORG-051, ORG-060, ORG-061) are finished whether the part is on or off, because they complete a part that exists (TPL-1104).
  - A template that uses anything of this feature is **not offered** while the part is off. That covers a set, a new field type, a condition, rights, or a waiting approval. Its use route answers 404 `experimental_off`. Managers still see it in their list, marked "needs guided templates", and can edit or delete it.
  - Hiding the template, instead of falling back to today's behaviour, **fails closed**. A template with a restricted audience never becomes open to everyone because a switch changed.

  *Proof:*
  - a test on a fresh installation that every new route answers `experimental_off`;
  - a test that a guided template is not in the gallery while the part is off;
  - the test of spec 069 that only an administrator turns a part on.

## 1.3 Permissions

- **TPL-060** · built · `space.templates` keeps its meaning: make, change, hide, promote and delete templates of a space. With this feature it also covers approving templates proposed for the space (TPL-520) and setting rights per template (TPL-500).
- **TPL-061** · new · `template.propose`: make a template **in draft**, see and test it alone, and propose it for a space. It is a new space permission. The migration gives it to every role that holds `page.create`, so nothing is lost, and an administrator can take it away (decided, TPL-1410). *Why:* the roadmap asks for approval of templates that others may use. That only has meaning if people other than managers can write templates. *Proof:* the adoption test of wave PM, showing that no decision changes except the new permission.
- **TPL-062** · built · The instance permission `administrator` covers templates of all spaces, and so approving them (TPL-521).
- **TPL-063** · new · The permission matrix gains the template actions as a dimension: see, use, edit, propose, approve and set rights. Each is tested against:
  - manager, proposer and member;
  - a member outside the template's audience;
  - a guest, an administrator, and a person in no group.

  *Proof:* the matrix test, extended.

## 1.4 Decided on the product owner's behalf on 2026-11-14

Each decision may be overruled before the wave that uses it.

| Line | Decision | Reason |
|---|---|---|
| TPL-1401 | Guided templates are an experimental part, off by default (TPL-050) | ADR-043 freezes new parts before 1.0; ADR-038 still wants the feature before 1.0. Experimental meets both |
| TPL-1402 | A set lands in one space, as one subtree under one root | One space lock and one transaction keep it all or nothing; rights across spaces would multiply the edge cases |
| TPL-1403 | At most 100 pages in a set; above 25, creation runs as background work, still in one transaction | Large enough for an onboarding or a project, small enough to hold the tree lock briefly |
| TPL-1404 | Pages of a set are created as drafts unless the template says "publish at once" | Today's behaviour is drafts (ORG-050); publishing many pages unseen should be a deliberate choice of the template's author |
| TPL-1405 | Conditions are structured tests in "all of" or "any of", one level, at most 10 tests; no expressions, no scripting | Readable, auditable and total; the same choice as DOC-1404 |
| TPL-1406 | The server evaluates every condition again on submit; the browser only previews | The browser cannot be trusted; the result must not depend on it |
| TPL-1407 | Answers to fields a condition hides are dropped, never stored | The person did not see the question, so the answer cannot be theirs |
| TPL-1408 | No regular expressions in validation for now; length, range and named formats only | Browser and server regular expressions differ, and a pattern is hard for authors to get right |
| TPL-1409 | Answers are not stored beyond the pages they fill; the set record keeps names, not values | Data minimisation: answers can hold personal data |
| TPL-1410 | `template.propose` is given to every role with `page.create`, and can be taken away | Nobody loses anything, and an administrator who wants fewer authors can narrow it |
| TPL-1411 | People with `space.templates` (or administrator, for all spaces) publish their own templates without approval, unless "every change needs a second person" is on | Approval is for templates written by others; four-eyes stays a choice |
| TPL-1412 | "Every change needs a second person" is off by default | Most teams trust their template managers; regulated teams turn it on |
| TPL-1413 | Approval is one decision by one person, not a document-control workflow | One step needs no workflow engine; DOC-561 keeps templates outside document control |
| TPL-1414 | The approved version keeps being offered while a newer version waits | Users are never left without a template because its author is improving it |
| TPL-1415 | Rights per template only narrow; they never grant more than `page.create` in the space | Deny by default: a template is never a way into a space |
| TPL-1416 | Pages made from a template are never updated when the template changes, and there is no "apply the new version" | ORG-058 stays true; merging into living pages loses people's work |
| TPL-1417 | A restriction on a page of the set needs `restriction.manage`; without it the whole set is refused, never created open | A page meant to be restricted must never exist without its restriction, even for a moment |
| TPL-1418 | Repeating a page per item of a list is later | It multiplies sets beyond what one form can review; the need is real but not first |
| TPL-1419 | Variables belong to one template; there is no shared library of variables | A library would need its own rights and versions; no need shown yet |
| TPL-1420 | Space templates (ORG-055) stay as they are; sets do not replace them | Different jobs: a new space against pages in an existing space |
| TPL-1421 | Conditions on the creator's groups are allowed, and the screen says they are not access control | Useful for different text per team; the template text is readable by its editors anyway |
| TPL-1422 | Load budgets are measured on the real-data dump with sets of 10 and 100 pages | Measured, never claimed (ADR-043) |
| TPL-1423 | The approval queue of templates of all spaces is in Administration, Content, Templates | One place per scope, like the rest of the administration |
| TPL-1424 | Templates of all spaces can be offered in every space or in chosen spaces, shown as chips | The design standard's rule for anything the instance applies to spaces |

## 1.5 Only the product owner can decide

- **The freeze:** ADR-038 (2026-11-09) puts the enterprise features before 1.0, and ADR-043 (2026-11-13) freezes new parts before 1.0. This study reads them together as "build it, experimental and off" (TPL-1401). Document control (spec 064) was started before ADR-043 and has no switch. Whether document control should get one too is outside this study, but the product owner may want one rule for all three enterprise features.
- **Who writes templates:** whether people without `space.templates` should write and propose templates at all (TPL-061). If not, approval reduces to the four-eyes setting of TPL-511, and `template.propose` is not needed.

## Sources

See [sources.md](sources.md).
