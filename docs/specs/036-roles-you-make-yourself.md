# Spec 036: Roles you make yourself

Status: built on branch `wave-r`, screens included, in the settings pattern of spec 033.
Asked for by the product owner (ADR-034): permissions stop being a matrix of ticks. An administrator makes roles, gives each role fine-grained permissions, and gives roles to people and groups, per space and for the instance. Looking at Administration > Permissions, the product owner then asked whether that was all the permissions there are, and expected more: this spec also splits permissions that were too broad, puts every permission in plain words, and turns that screen into the roles screen.

## Study

### What there was

- **Single grants.** `space_grants` holds one row per subject and permission in a space, `global_grants` one per subject and global permission. `permissions.Evaluate` reads the first through `LoadSpace`, every `perm_*` SQL function reads both, and the matrix test (`internal/permissions/matrix_test.go`) holds the two to one rule with a literal transcription.
- **Templates and sets.** Viewer, Editor, Manager and Space admin are named sets in Go (`permissions.Templates`), and administrators could make their own sets (`permission_sets`). Both only filled in boxes: after that, nothing remembered where a grant came from, and changing a set changed nobody.
- **Writers.** Grants were written in four packages: `permissions` (templates, `SetGrants`), `exchange` (bundle import), and `identity` (new accounts, service accounts, global permissions, deleting a group).
- **Keys on screens.** Administration > Permissions listed who holds which global permission with the raw key (`create_personal_space`), the named sets took permissions as keys typed in a text box, and the dialog for changing a person's type listed keys.

### Choices

| # | Question | Choice | Why |
|---|---|---|---|
| D1 | Where does the truth live? | `roles` and `role_assignments`. The grant tables stay, as what roles add up to | The rule, its SQL functions and the matrix test stay exactly as they are. A second rule beside the first would be the largest risk of this wave |
| D2 | Who keeps the two equal? | The database, with triggers on both new tables | One place that cannot be skipped by a code path. `rbac_drift()` lists any difference, and every test checks it stays empty |
| D3 | Can the application still write grants? | No. `db.GrantApp` revokes it; the triggers run as the owner (`SECURITY DEFINER`) | A forgotten writer fails loudly instead of granting past a role |
| D4 | Built-in roles | Viewer, Editor, Manager and Space admin, with the meaning of the templates. They cannot change or be deleted | People who learned those words keep their meaning. A test compares the rows with `permissions.Templates` |
| D5 | Deleting a role in use | Refused (`role_in_use`). The database also refuses it (`ON DELETE RESTRICT`) | Deleting would take access from people nobody looked at |
| D6 | Changing a role in use | Allowed after a preview and `confirm`, audited with before and after | That is the point of a role. The preview names how many assignments, spaces and people it reaches |
| D7 | A role in use gains `space.administer` | Refused (`administer_in_use`) | Who administers a space is chosen in that space, by name. A role should never make people administrators of spaces in bulk |
| D8 | Who makes roles | Administrators. Who gives them: in a space whoever manages access there (PRM-025's limits stay), for the instance administrators | Unchanged lines of responsibility. `administrator` still gives no content: instance roles hold only global permissions |
| D9 | The old routes for single permissions | They keep working. A set becomes the role with exactly that set, a built-in one when it matches, named after its permissions otherwise | Screens and scripts that use them keep working until they move. The same set shares one role |
| D10 | Own sets | Became space roles under their own names; the old set routes now act on those roles | One idea instead of two |
| D11 | Scope | A role is for a space or for the instance. An assignment must match: a trigger refuses anything else | No role can quietly mean something in a place it was not made for |
| D12 | Ids made in SQL | `rbac_uuid7()`, because PostgreSQL 16 has no `uuidv7()` | All ids stay UUIDv7, as `ids.New` makes them |
| D13 | Which broad permissions to split | Four that each guard one whole feature a space may want to hand out alone: publishing to the web, automation, templates, the audit. The rest are proposed, not built | Each new permission costs a place in the rule, the SQL, the matrix and a mutation, and must move no access. These four split cleanly: one permission guarded them, and the new one can take its place everywhere |
| D14 | What a split-off permission gets by default | Everyone who held the broader one, in every grant and own set, before the grants become roles | Nobody gains or loses. The built-in Space admin holds all four |
| D15 | Does administering imply them? | No. They stand alone, like every permission except `space.manage_access` | A role that runs the space but may not publish to the web is the point of the split |
| D16 | Where they work | Wherever `space.administer` worked: in an archived space and on an archived page too | The split must change no decision. Whether automation should stop in an archived space is a separate question |
| D17 | Words | A name and one line per permission, in `en.json`, grouped as pages, comments, files, people and access, space, administration. Roles the database names after their permissions use the same words | No key on any screen. Two tests hold the words complete and the database's names equal to them |

### Broad permissions, and what hides under them

The study read every `RequireSpace`, `RequirePage` and `CheckSpace` call and every route rule. What one permission guards today:

| Permission | Guards today | Split now | Proposed |
|---|---|---|---|
| `space.administer` | Name and settings, archive and delete, trash, key and old keys, owners, home page, features, title alignment, icon, shortcuts, categories, labels fixed and renamed, statuses and review, approval rule, the content table, open placeholders, file usage, watchers of a page, PDF design, bundles, the explain view, public sites and links, automation rules and webhooks, templates, the space audit | `site.publish`, `space.automation`, `space.templates`, `space.audit` | `space.labels`, `space.statuses`, `space.approvals`, `space.look`, `page.watchers`, bundles under export |
| `page.create` | New pages and blog posts, moving and copying in, importing, using a template | | `post.create` |
| `page.edit` | Editing, renaming, look, publishing, restoring, moving within and out of the space | | `page.move_out` |
| `space.export` | Files and PDF of what one can see | | Export per format |
| `invite_people` (global) | Inviting people to the instance, guests included | | `space.invite_guests` |

`analytics.view` already stood apart (spec 015). Watching a page oneself, comments and files have their own permissions and need no split.

**Why `post.create` is not built.** It would be given to whoever holds `page.create`. Turning an unpublished page into a post needs `page.edit` today; requiring `post.create` there too takes that from someone who holds `page.edit` without `page.create`, and not requiring it leaves a way around the new permission. That needs a decision by the product owner first (PRM-206).

### The model

- `roles (id, scope, builtin, name, description, permissions)`: `scope` is `space` or `instance`; names are unique per scope, case-insensitively.
- `role_assignments (role_id, space_id, subject_type, subject_id)`: `space_id` is empty for the instance. Subjects are users, groups, all members and all guests in a space, and users and groups for the instance.
- `rbac_sync(space, subject)` makes the grants of one subject in one scope equal to its roles, removing and adding only the difference. Triggers call it when an assignment changes and when a role's permissions change.
- `rbac_set_exact` and `rbac_add_permissions` are how older code gives a set: they find or make the role and assign it. A role they make is named "Custom: " and its permissions in words (`rbac_words`).
- `permissions.SplitFrom` names each split-off permission and the broader one it came from.

### Migration

`0036_roles.sql` is an expand migration. It creates the tables and functions, adds the four built-in roles, and runs `rbac_adopt()`. That function turns own sets into roles, giving a set that held `space.administer` the four split-off permissions too; gives the four to every subject that holds `space.administer` in a space; and then gives every subject that holds grants the role with exactly that set. The previous release reads the same grant tables, ignores the permissions it does not know, and keeps working on this schema.

**Proof.** `TestAdoptingGrantsChangesNoDecision` writes single grants of every shape straight into the tables, as before roles existed and without the split-off permissions: the grant cases of the matrix test, a space administrator as a group, as all members and as all guests, a reviewer set, all members and a group adding up in a third of the spaces, nine kinds of people, global grants in four shapes, two own sets, restrictions on the root and the leaf, drafts of others and of the person, active and archived spaces. It records every decision of `Evaluate` (allowed and visible) for every person, space, page and action, and what `perm_visible_pages`, `perm_listed_pages`, `perm_editable_pages`, `perm_spaces` and `perm_has_global` return. It runs `rbac_adopt()` and records again. Every decision must be what it was, except that a split-off permission must now decide exactly as its broader permission did before. 369,861 decisions were compared, 24,951 of them allowed, 720 of those through a split, and none differed. `rbac_drift()` is empty, no grant is left without a role, and the own set that held administering became a role with the four.

**Bundles.** Bundles of format 1 were written before the split. Importing one gives the split-off permissions with `space.administer` (`permissionsOfGrant`). Bundles are now written as format 2 and taken as they are.

## Plain words

Every permission has `permissions.<key>` (its name) and `permissions.<key>_help` (one line on what it allows) in `web/src/locales/en.json`; global ones are keyed `global_<permission>`. `web/src/permission-words.ts` puts them in six groups and gives the screens `permissionLabel` and `permissionHelp`. Three tests hold this together: a Go test that every permission in `permissions.AllSpacePermissions` and `identity.AllGlobalPermissions` has both lines, a Go test that `rbac_words` in the database uses the same names, and a unit test that the groups hold every named permission exactly once.

## Screens

- **Administration > Roles** (the tab that was Permissions, still at `/admin/permissions`): the space roles and the instance roles as tables, with name, permissions in words, and how many hold each. Built-in roles show as built in and have no edit. The editor shows the permissions in their groups, each a checkbox with its line beneath, and who holds the role and in which space. A change to a role in use shows the preview and asks again. Below: who holds which instance role, given and changed there; who holds which instance permission, read-only and in words; and the categories of spaces. The box for named permission sets is gone: the sets became roles.
- **A space's settings > People and roles**: who holds which role in the space, and giving roles to a person, a group, all members or all guests.
- **Space settings show each part to whoever holds what it needs**: sites to `site.publish`, rules, automation and webhooks to `space.automation`, templates to `space.templates`, the audit to `space.audit`, roles and access to `space.manage_access`, the rest to `space.administer`. The settings link shows for any of them. The page menu offers saving as a template with `space.templates`, and a public link with `site.publish`.

## Load budget (ARC-238)

| What | Cost | Class |
|---|---|---|
| A permission check, and every list | Unchanged: the same queries on the same rows. The query budget test of ARC-239 passes without a change to its budgets | reading |
| The capabilities of a space | Fewer queries than before: the facts are loaded once and `Evaluate` answers all nine actions, where each action loaded them again | reading |
| Giving a subject roles in a space | One statement per role given or taken; the trigger then writes only the difference in that subject's grants | saving |
| Changing a role | One write to the role; the trigger writes the difference for each subject that holds it | saving |
| Who holds a role | One count and one list, at most 200 rows; the total says how many more | reading |

A role held by thousands of subjects rewrites their grants in one transaction when it changes. That is rare, behind a preview and a confirmation, and its cost grows with the number of holders, not with the content. It was not measured at that size.

## Routes

| Route | Rule |
|---|---|
| `GET /api/v1/roles?scope=` | authenticated; the module asks for `use_product` |
| `POST /api/v1/admin/roles`, `PUT /api/v1/admin/roles/{id}`, `POST .../{id}/preview`, `DELETE .../{id}` | session-only, administrator |
| `GET /api/v1/admin/roles/{id}/holders` | administrator |
| `GET` and `PUT /api/v1/admin/role-assignments` | session-only, administrator |
| `GET` and `PUT /api/v1/spaces/{id}/role-assignments` | object: `space.manage_access` |
| Sites and public links | object: `site.publish` (was `space.administer`) |
| Space rules and webhooks | object: `space.automation` (was `space.administer`) |
| Managing, making, importing space templates, saving a page as one | object: `space.templates` (was `space.administer`) |
| `GET /api/v1/spaces/{id}/audit` | object: `space.audit` (was `space.administer`) |

## Tests

Failure paths first, in `internal/permissions/roles_test.go`: a non-administrator cannot make a role; a wrong permission, an empty role and a taken name are refused; system administration needs a system administrator; built-in roles neither change nor go; a role in use cannot be deleted, cannot change without confirm, and cannot gain administration; managers cannot make or unmake administrators; the last administrator of a space and of the instance stay; unknown roles, roles of the wrong scope and unknown subjects are refused; only administrators read who holds a role, and an unknown role is not found. Then the paths that work: a confirmed change reaches its holder at once, a role given and taken away gives and takes its permissions, every change is audited with before and after, and the holders of a role are listed with their space. `internal/db/grants_role_test.go` proves the application role cannot write a grant, and can give a role.

For the split: the matrix test has the four in its actions and a grant case that holds them without administering, and the literal rule names them where administering works on archived spaces and pages. `internal/app/finer_permissions_test.go` shows that someone who administers a space without them is refused all seven routes they guard, that someone who holds only one of them reaches its routes and is refused the routes of the other three. `internal/exchange/grants_test.go` covers bundles of both formats. `e2e/roles.spec.ts` makes a role with the grouped checkboxes, gives it in a space, sees the holder in the editor, and changes it after the preview. Twenty-six mutations cover these rules, the migration, the triggers, the split and the bundles.

## Left

- The proposed permissions PRM-206 to PRM-215, each to be split the same way: a default from the broader one, a place in the rule, the SQL and the matrix, and a mutation. `post.create` waits for a decision (above).
- Replacing the matrix of ticks in the space access screen with role choices, then retiring the single-permission routes.
- Dropping `permission_sets` in a contract migration (PRM-197).
- Built-in instance roles with friendly names. The migration names instance roles after their permissions in words, such as "Custom: Use the product, See people"; an administrator can rename them.
