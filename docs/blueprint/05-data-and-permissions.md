# 05: Data and permission model

This is the full model behind blueprint 03: every entity with its fields and life, the document format, every permission, the rule with its edge cases, how each list stays safe, and the permission matrix as test cases.

It came from a study on 2026-11-05 of blueprint 03, specs 002 and 008 to 016, the code, and the branches of the waves being built. `05-data-and-permissions/sources.md` lists what was read.

## How to read it

Every item has a number that never changes and is never reused, with the prefix **DAT** for data and **PRM** for permissions, and one status:

| Status | Meaning |
|---|---|
| **built** | On `main`, and tests prove it |
| **partly** | Part of it works, and the line says which part |
| **wave C** to **wave H** | Planned in that wave's spec, or being built on its branch |
| **phase N** | On the roadmap (08) |
| **new** | Found in this study, and in no plan yet |
| **later** | Worth keeping, not planned yet |

A problem found in the study has a **GAP** number, in chapter 12. When something is built, its status changes in the same commit, as in blueprint 03.

## The chapters

| Chapter | Covers | Numbers |
|---|---|---|
| [1. Conventions](05-data-and-permissions/01-conventions.md) | Rules for every table, retention, where personal data sits | DAT-001 to 015 |
| [2. People](05-data-and-permissions/02-people.md) | Users, ways to log in, groups, global grants | DAT-020 to 043 |
| [3. Spaces and pages](05-data-and-permissions/03-spaces-and-pages.md) | Spaces, grants, pages, versions, drafts, restrictions, labels | DAT-050 to 077 |
| [4. Documents and files](05-data-and-permissions/04-documents-and-files.md) | The document format, attachments, search, derived tables | DAT-080 to 099 |
| [5. Working together](05-data-and-permissions/05-working-together.md) | Comments, reactions, watching, notifications, mail, requests | DAT-110 to 119 |
| [6. Operations](05-data-and-permissions/06-operations.md) | Trash, jobs, audit, exchange, analytics, automation | DAT-130 to 150 |
| [7. Life cycles](05-data-and-permissions/07-life-cycles.md) | A page, a space and a person, and what goes along | DAT-160 to 173 |
| [8. Permissions](05-data-and-permissions/08-permissions.md) | Every permission, action, named set, route rule and role | PRM-001 to 039, PRM-178 to 191, PRM-198 to 215 |
| [9. The rule](05-data-and-permissions/09-the-rule.md) | The rule, and every edge case per subject, and roles | PRM-040 to 108, PRM-192 to 197 |
| [10. Safe lists](05-data-and-permissions/10-safe-lists.md) | Every list and what guards it, and the side doors | PRM-120 to 149 |
| [11. The test matrix](05-data-and-permissions/11-test-matrix.md) | Dimensions, the literal rule, tests beside the matrix | PRM-150 to 177 |
| [12. Gaps and contradictions](05-data-and-permissions/12-gaps-and-contradictions.md) | What is wrong, how bad, and how to fix it | GAP-001 to 030 |
| [Sources](05-data-and-permissions/sources.md) | What was read | |

In total 318 numbered items: 283 built, 7 partly, 17 still marked with the wave that planned them, 10 new and 1 for later, with 30 gaps. Counted again in spec 035, and the roles of spec 036 added.

## The model in short

**Three layers.** Global permissions say who may use the product at all. Space permissions, in a matrix per space, say what a person may do. Page restrictions narrow that further: a view restriction holds for a page and everything below it, an edit restriction for that page only.

**The rule.** A person may do action *A* on page *P* when: their account is active, and they may use the product, and they hold the space permission for *A* and view on the space, and no view restriction on *P* or above it excludes them, and for edits no edit restriction on *P* excludes them, and neither the space nor the page is in a state that forbids it. Chapter 9 has all twelve checks.

**Design rules.** Permissions only allow, and there is no deny entry. No grant means no access, also for administrators. Someone who may not see a thing gets "not found". One function decides single questions, and SQL functions with the same rule decide lists. A generated matrix compares the two with a third, literal copy of the rule.

**A restriction only closes.** Nothing that happens to a page, a group or a person may open what was closed: not a move, a copy, a purge, a deleted group or an import.

## What the study found

Chapter 12 has all 30. These need action first:

| Gap | What | When |
|---|---|---|
| GAP-004 | A mention stores a name inside immutable versions, so anonymising cannot reach it | Before wave C merges |
| GAP-006 | A thread on draft text becomes public at publishing, with words that were never published | Before wave C merges |
| GAP-001 | A space administrator sees titles of hidden pages in the trash, and can purge them | Built today |
| GAP-003 | A copy loses inherited restrictions without a word | Built today, wider in wave D |
| GAP-005, 002, 018 | Waves C, D and F collide on feature switches, `space_id` copies and the list for anonymising | At the merge |
| GAP-010, 015 | Public sites have no rule for what a page refers to, and miss events that must withdraw a page | Before wave G |
| GAP-016 | A rule can send a title to someone who may not see that page | Before wave H |

Six decisions for the product owner are in chapter 12, section 12.5.
