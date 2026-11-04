# 002: Permissions

Status: built, waiting for the product owner's acceptance session (written by Claude from the blueprint)
Phase: 1 (basic roles) and 2 (full model). Built as one piece, because the basic roles are simply templates over the full matrix.

## Problem
Broken access control is the most common serious bug in wikis. The rules must be simple enough to test completely and to explain to a user.

## Behaviour

**The rule (blueprint 05).** A user may perform action *A* on page *P* when: **use product** AND **space permission for A** AND **for every ancestor of P including P: no view restriction, or the user matches it** AND (for edits) **no edit restriction on P, or the user matches it**.

Clarifications that make the rule fully testable:
1. Every action also needs *space view*. A grant to edit without view gives nothing.
2. A user matches a restriction when listed directly or through a group.
3. Disabled users can do nothing.
4. In an archived space only viewing, exporting and *administer* work. In a trashed space nothing works except through the trash.
5. Trashed pages cannot be reached except through the trash. An unpublished page is reachable only by its creator.
6. "Edits" means: edit, archive, delete, add or delete attachments. Managing restrictions is not an edit: a space admin must be able to lift an edit restriction. It still needs view access to the page.
7. "Delete own" applies to items the user created. "Delete any" applies to all.
8. Moving a page needs edit on the page and create in the space. When the move would remove an inherited view restriction, it also needs *manage restrictions*.
9. Restoring a page from the trash is refused while its parent is still in the trash, and purging a page also purges everything below it. A page can therefore never lose a restriction it inherited just by passing through the trash.

**Space roles are templates.** Viewer, Editor and Space admin are named sets of the space permissions from blueprint 05. Applying a template replaces that subject's permissions in the space. Two more templates set up a whole space at once: *Team space* (all members edit) and *Read-only knowledge base* (all members view). After that, any box in the matrix can be changed. A space must always keep at least one direct user or group with *administer*.

**No administrator bypass.** Global administrators have no automatic access to content. They can use **recover access**, which gives themselves space admin (for a space) or adds them to all restrictions on a page and its ancestors. It is always audited, with the reason they give.

**Why can or can't I?** Space admins and administrators can ask, for any user and any action on a page, for the result and which layer decided it. The answer comes from the same evaluation as the real check.

**Not found versus forbidden.** A user who cannot view something gets "not found", so the existence of restricted pages does not leak. A user who can view but not act gets "forbidden". Both are audited as denied.

**Where the rules live.** One Go function evaluates the rule for a single decision. Lists, search and exports filter with SQL functions that express the same rule. Tests compare the two on every generated case, and compare both against a separate, literal transcription of the rule.

## Permissions and audit
Events: `permission.space.granted|revoked`, `permission.template.applied`, `permission.global.granted|revoked`, `restriction.added|removed`, `access.recovered`, `access.denied`.

## Acceptance criteria
- [x] The generated matrix (user type × group membership × space grant × restriction position × action) runs thousands of cases with no mismatch between Go, SQL and the literal rule
- [x] A view restriction on a parent hides all descendants, including from search, tree, favourites and exports
- [x] An edit restriction on a parent does not affect its children
- [x] Restrictions only narrow: being on a restriction list without a space grant gives nothing
- [x] Guests are never included in "all members"
- [x] An administrator without a grant cannot read page content; recover access works and is audited
- [x] A space cannot lose its last administrator
- [x] A user without access gets 404 on every object route, and a viewer gets 403 on every write route (one test walks all routes)

## How the product owner tests it
Follow the guest and restricted-page scenarios in the handoff checklist: restrict a page, log in as a guest, confirm the page cannot be found by tree, search, URL or export. Then use "why can't I?".

## Provenance
Own design from docs/blueprint/05 and ADR-009.
