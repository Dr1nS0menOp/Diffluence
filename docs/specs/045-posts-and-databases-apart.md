# Spec 045: Blog posts and databases apart from pages

Status: built on branch `wave-pm`.
Decided by the product owner in ADR-038: blog posts get their own permission (`post.create`, PRM-206), and databases get theirs (PRM-215) once the rule can tell kinds of item apart. Whoever may create pages today receives both, so nobody loses anything. Export stays one permission (PRM-211).

## Study

### Where creating is checked

The study read every `PageCreate` in the code and every route that makes, copies, moves or imports an item.

| Where | What it checked | Kind known there |
|---|---|---|
| `pages.Create` (`POST /pages`) | `page.create` in the space, and on the parent through `checkParent` | yes, the kind asked for |
| `pages.SetKind` (`POST /pages/{id}/kind`) | `page.edit` on the page | yes, before and after |
| `pages.Move` within a space | `page.edit` and `page.create` on the page | yes, the page's |
| `pages.moveToSpace` | `page.move_out` and `page.create` on the page, `space.view` and `page.create` in the target | the page's, and every kind in its branch |
| `pages.Copy` and `copyOneStep` (large copies) | `page.create` in the target, and on the parent | yes, each copied item's |
| `pages.StartFromTemplate`, `pages.Follow` (placeholder), space templates | `page.create` | always a page |
| `exchange.StartImport` (Markdown, Word, HTML, archives) | `page.create` | always pages |
| Bundles | make a new space, whose maker holds Space admin | everything, through the role |
| Example space at setup | the first administrator, who holds Space admin | everything |
| Automation | goes through `Copy` and `Move` with the rights of the rule's owner | as above |
| SCIM | makes no items | |

How kinds are stored: `pages.kind` is one of `page`, `post`, `folder`, `link`, `database`, `template`. A post has no place in the tree; a database is one sheet and nothing else; a template is never a page to anyone.

### Choices

| # | Question | Choice | Why |
|---|---|---|---|
| D1 | The name of the database permission | `database.create`, "Create databases", not the `space.databases` the blueprint proposed | It guards making a database, as `post.create` guards making a post. Editing a database stays `page.edit`, like every kind of page, so the name says only what it guards |
| D2 | Folders and links | Stay with `page.create` | ADR-038 names posts and databases only. A folder and a link have no content of their own to set apart |
| D3 | Where the kind enters the rule | `permissions.CreatePermission(kind)`: the module asks for the permission of the kind. `Evaluate` treats the two like `page.create`: not in an archived space, not on an archived page, not stopped by an edit restriction | No new layer in the rule. The matrix and the SQL need no change of meaning, only the two actions, which keeps the risk of this wave small |
| D4 | Turning an unpublished page into a post | Needs `post.create` as well as `page.edit`; turning a post back into a page needs `page.create`. Page and folder turn into each other with `page.edit` alone | Otherwise a way around the new permission. Only an unpublished page can change to or from a post, and only its creator sees one; they made it with `page.create`, which brings `post.create` in the migration. So nobody who could do it at the migration loses it |
| D5 | Below a parent | The parent is checked with the permission of the kind that goes under it | A role that may only make databases can still put one below a page |
| D6 | A copy or a move of a branch | Every kind in the branch asks for its permission where it arrives. A copy of a page with a database below it is refused whole without `database.create`; a large copy, page by page, reports that one item as not copied | The branch arrives as new items in that space. Refusing the whole copy keeps the result what the person asked for |
| D7 | Moving within a space | Asks for the permission of the page's kind, where `page.create` was asked before | Placing an item is part of making that kind; after the migration the same people hold it |
| D8 | Defaults | Whoever holds `page.create` gets both, in every role and own set, and in the built-in Editor, Manager and Space admin. Bundles of format 3 and older bring both with `page.create`. The older grants route completes a list that holds `page.create` and neither of the two | The split moves no access, as specs 036 and 040 did it (`permissions.SplitFrom`, `rbac_split_off`, `WithSplitOff`) |
| D9 | Export | Stays one permission | ADR-038 |
| D10 | The route table | `POST /pages`, copy, imports and the template gallery still declare `page.create` | A route names one permission, and the module behind it decides by the kind. Imports and the gallery only make pages; making and copying ask the kind's permission in the module, which the tests above prove |
| D11 | The audit | `page.created` now names the kind. A refusal is recorded as every refusal is, with the permission it lacked | So a reader of the audit can tell a post or a database from a page |

## The screens

- **Create button:** offers only what a person may make in the space. The button itself makes a page, or, for someone who may not make pages, a database, then a blog post. "From a template" shows with page creation. A person who may make nothing in the space gets no menu there.
- **Page menu and tree menu:** Copy shows with the permission of the page's kind. "Turn into a blog post" shows with `post.create`, "Turn into a page" from a post with `page.create`.
- **Roles:** the role editor lists "Create blog posts" and "Create databases" under pages, each with one line on what it allows. "Create pages" now says what it still covers: pages, folders and links, copying and moving them in, and importing.

## Migration

`0041_posts_and_databases.sql` is an expand migration. It teaches `rbac_split_off` that `page.create` splits into the two, gives `rbac_words` their names, and runs `rbac_split_roles()`: every space role and own set that holds `page.create` gains both, and the trigger on roles brings every holder's grants along. The previous release does not read the new names; it keeps asking for `page.create`, which the same people still hold.

**Proof.** `TestAdoptingGrantsChangesNoDecision` writes grants of every shape as before roles existed, without any split-off permission, records every decision of `Evaluate` and of the SQL lists, adopts them into roles, and records again. Every decision must be what it was, except that a split-off permission must now decide exactly as its broader one did. With the two new actions it compared 1,016,664 decisions, 68,830 of them allowed, 4,224 of those through a split, and none differed. `TestTheMigrationGivesWhoeverCreatesPagesPostsAndDatabases` runs the migration step on roles that exist before it: built-in roles without the two, a role made by hand with `page.create`, an own set, and someone who never created pages. After it, everyone who created pages holds both, the person who did not holds exactly what they held, the built-in roles equal their templates again, and `rbac_drift()` is empty. It also shows the older grants route completing a list from before the split.

## Load budget (ARC-238)

| What | Cost | Class |
|---|---|---|
| Making an item | Unchanged: one permission check, now for the kind's permission instead of `page.create` | saving |
| Turning a page into a post or back | One more permission check, from facts already loaded in the request | saving |
| Copying | One permission check per copied item, from kept facts (ARC-231). A copy was already one insert and one version per item | saving, heavy for large copies as before |
| Moving to another space | One more query: the distinct kinds of the branch, through the primary key. At most three checks in the target | saving |
| The capabilities of a space and of a page | Two more answers from facts already loaded, no query | reading |
| The migration | One update of the roles that hold `page.create`; the trigger rewrites the grants of their holders in the same transaction. It grows with the number of holders, as the migrations of specs 036 and 040 did | once |

## Acceptance

- [x] Someone with page creation but neither new permission cannot make a post or a database, at the top or below a page, and the space says so
- [x] Someone with only `post.create` makes posts and nothing else; someone with only `database.create` makes databases, also below a page, and nothing else
- [x] Turning a page into a post needs `post.create`; back needs `page.create`; a folder needs neither
- [x] Copying a database, or a branch with one, needs `database.create` where the copy goes
- [x] Moving a branch with a database to another space needs `database.create` there
- [x] `page.created` in the audit names the kind
- [x] The migration changes no decision, and gives the two to everyone who created pages
- [x] Bundles of format 3 and older bring both with page creation; format 4 bundles hold them themselves
- [x] The Create menu offers only what may be made; the role editor lists both with their lines

## How the product owner tests it

1. Administration > Roles > New role: tick View the space and Create blog posts only, and give the role to a test person in a space.
2. As that person, open the space: Create makes a blog post at once, and its menu offers no page and no database.
3. Give the role Create databases too: the menu now offers New database.
4. As someone with the built-in Editor role, nothing changes: pages, posts and databases can all be made.

## Provenance

Own design, on the decision of ADR-038.
