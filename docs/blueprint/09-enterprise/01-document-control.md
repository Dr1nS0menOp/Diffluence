# 09.1: Document control

Blueprint 09 holds the enterprise document features that come before 1.0 (ADR-038). This first part is **document control**: documents that follow a defined path of review and approval, where readers only ever see the approved version, and where regulated teams can sign electronically. The plan to build it is [spec 064](../../specs/064-document-control.md).

## How to read this blueprint

Every line has a number (DOC-NNN) and a status: **built**, **partly**, **new** (planned in spec 064) or **later**. Each line gives the behaviour, then *Why* (with the clause of a standard where one applies), *Edge cases*, and *Proof* (how a test will show it). The standards are listed in [sources.md](sources.md).

This is a clean-room study (`docs/blueprint/02-clean-room-and-licensing.md`). Blueprint 08 names a document-management product only to say which behaviour is meant. This study did not read that product's pages, and every requirement comes from the standards (ISO 9001 clause 7.5, 21 CFR Part 11, EU GMP Annex 11, eIDAS) and from Diffluence's own code and blueprints. No text, names, workflow or layout of any product is used.

## Chapters

| Chapter | Lines | About |
|---|---|---|
| [02 The workflow model](02-workflow-model.md) | DOC-100 to DOC-141 | Workflows, states and kinds, transitions, versions, conditions, instance templates, turning on and off |
| [03 Reviewers](03-reviewers.md) | DOC-200 to DOC-251 | Who is asked, rules to pass, independence, due dates, escalation, delegation and absence |
| [04 What readers see](04-what-readers-see.md) | DOC-300 to DOC-332 | The visibility rule, what restarts review, obsolete versions, edits during review |
| [05 Space and page workflows](05-space-and-page-workflows.md) | DOC-400 to DOC-431 | Space workflows, branch and page workflows, which wins |
| [06 Content around the page](06-content-around-the-page.md) | DOC-500 to DOC-572 | Comments, attachments, search, export, public sites, the API, webhooks, templates, the feed |
| [07 Notifications](07-notifications.md) | DOC-600 to DOC-609 | Who is told what, and how drafts never leak by mail |
| [08 Electronic signatures](08-electronic-signatures.md) | DOC-700 to DOC-791 | Part 11 and Annex 11 point by point, the signing step, verification, keeping signatures |
| [09 Periodic review and read and understood](09-review-and-read.md) | DOC-800 to DOC-825 | Review dates, confirmations, reports |
| [10 The editor and the screens](10-editor-and-screens.md) | DOC-900 to DOC-950 | Where things live, the workflow form, keyboard, accessibility, RTL, small screens |
| [11 Audit and reports](11-audit-and-reports.md) | DOC-1000 to DOC-1031 | What is audited, the control history, the auditor's export, reports |
| [12 Migration](12-migration.md) | DOC-1100 to DOC-1107 | How today's approvals move, by choice |
| [13 Edge cases and failure paths](13-edge-cases.md) | DOC-1200 to DOC-1253 | Moves, copies, deletes, restores, imports, people leaving, several nodes, time, failures |
| [14 Load budget](14-load-budget.md) | DOC-1300 to DOC-1306 | What it costs, measured on real data |

## 1.1 What exists today

Read in the code on 2026-11-13 (`internal/pages/approvals.go`, migration `0008_organising.sql`, `internal/permissions/evaluate.go`, `internal/identity/tokens.go`).

- **DOC-001** · built · An approval round (`approval_rounds`) freezes the title and document of a page at a base version, names approvers (`approval_answers`), takes "approve" or "changes" with a comment from each, and ends approved (with the approved version) or closed (PAG-062 to PAG-066).
- **DOC-002** · built · A space can require approval before publishing (`spaces.approval_required`), with up to ten fixed approvers (`space_approvers`); publishing checks that this exact text was approved (`requireApproved`) (PAG-068).
- **DOC-003** · built · Publishing ends any open round of the page (`endRoundsOnPublish`); a new version after approval removes the Approved mark and links to the older approved version (PAG-067).
- **DOC-004** · built · Past rounds stay on record (PAG-069). Setting the rule needs `space.approvals`, split off `space.administer` (wave R); someone named in a round reads it through `approval.read`.
- **DOC-005** · built · Re-authentication for sensitive steps by password, second factor or passkey within `ReauthWindow` (10 minutes), used by the restore of wave RS.
- **DOC-006** · built · The audit log is hash-chained, sealed after saving (ADR-041), signed every five minutes and witnessed outside the database (ADR-039).
- **DOC-007** · partly · Page statuses per space (SPC-047) exist; on controlled pages the workflow state takes their place and the status picker is hidden.

What is missing is everything in chapters 02 to 14: own states and transitions, readers held on the approved version, drafts kept from readers, space workflows over page workflows, signatures with a meaning, periodic review, read and understood, and an auditor's export.

## 1.2 Permissions

- **DOC-010** · partly · `space.workflows`: make, change, archive and assign workflows in a space, manage exempt labels and the signing setup of the space. Split off `space.administer` with the migration proof of wave PM, so every space administrator keeps it. *Proof:* the adoption test that no decision changes except the split.
- **DOC-011** · partly · `page.workflow`: set a workflow of the space on a page or branch where the space has none, and a shorter review period on a page. Split off `page.edit`. *Proof:* as above.
- **DOC-012** · built · `space.audit` ("See the audit") also covers the control history, the auditor's export, the reports and the read-and-understood lists. *Proof:* matrix cases.
- **DOC-013** · built · Taking a transition needs the right named on it: a reviewer of the round, an author (`page.edit`), the escalation target, or the page owner for periodic review. There is no permission to move any page to any state, not even for administrators (decided, DOC-1424); an administrator who must step in reassigns reviewers, which is audited. *Proof:* a matrix test over all transitions and kinds of person.
- **DOC-014** · new · Signing is enabled per person (DOC-751) and is never part of a role, so a role change never gives signing rights. *Proof:* a test.
- **DOC-015** · built · The permission matrix gains the workflow actions and the visibility of the working version, decided in one place (DOC-300). *Proof:* the matrix test, extended.

## 1.3 Decided on the product owner's behalf on 2026-11-13

Each may be overruled before the wave that uses it.

| Line | Decision | Reason |
|---|---|---|
| DOC-1401 | A time-out never moves a page into an approved state | Approval must be a person's act |
| DOC-1402 | One "changes" answer ends a round under any rule | Approving over an open objection is what review exists to prevent |
| DOC-1403 | Independence is on by default for states that lead to approval | Four-eyes is what auditors expect |
| DOC-1404 | Conditions without scripting | Readable and auditable; flows come later |
| DOC-1405 | Instance templates are copied into spaces, not linked | A change elsewhere never moves controlled pages silently |
| DOC-1406 | A never-approved controlled page is invisible to readers | Readers can only rely on approved content |
| DOC-1407 | Turning control off keeps the approved version visible until the next publish | No surprise exposure of unapproved text |
| DOC-1408 | At most 50 reviewers per round under "all" or a quorum | Nobody can review with hundreds of sign-offs |
| DOC-1409 | Working days are Monday to Friday unless the instance sets otherwise; holidays are not known | Simple and stated |
| DOC-1410 | A working version that equals the approved one returns the page to approved without review | Nothing differs from what was approved |
| DOC-1411 | Locking edits during review is off by default | Most teams keep writing during review |
| DOC-1412 | Turning off a space workflow does not bring back old page workflows | No silent re-activation |
| DOC-1413 | No signing through the API or tokens | A signature needs a person present |
| DOC-1414 | Watchers who are readers hear only about approved versions | They are told about what they can see |
| DOC-1415 | Mentions in a working version are held from people who may not see it until approval | A mention would leak the draft |
| DOC-1416 | A signing confirmation counts for one signature and expires after two minutes | Stricter than Part 11 §11.200, because a browser session is hard to prove continuous |
| DOC-1417 | Signed records in controlled spaces are kept against erasure; the privacy page says so | Retention duties of regulated work |
| DOC-1418 | People who join a read-and-understood group later are asked when they join | Everyone in the group ends up confirming |
| DOC-1419 | The first workflow editor is a form with a read-only diagram | Accessible and testable from the start |
| DOC-1420 | Today's approval rules are offered a generated workflow, never forced | No behaviour change without an administrator's choice |
| DOC-1421 | Imported signatures verify but do not count as approvals until a local round approves | Trust is local to an installation's audit chain |
| DOC-1422 | Guests can never review or sign | Guests are outside the organisation whose procedure it is |
| DOC-1423 | Load budgets are measured with 10% of pages controlled | A realistic share for a mixed instance |
| DOC-1424 | No administrator override of states | Every step stays a person's act on record |

## 1.4 Only the product owner can decide

- **Qualified signatures:** whether Diffluence should ever offer qualified electronic signatures under eIDAS through an outside trust service provider (DOC-791). It needs a provider and a contract, and likely money, which this project does not have.
- **Retention against erasure:** whether keeping signed records against erasure requests (DOC-1417) is acceptable for the European audience, or should be a per-space choice that the space owner signs off on.

## Sources

See [sources.md](sources.md).
