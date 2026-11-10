# 05.8: Every permission, and who holds it

The rule that uses them is in 05.9.

## 8.1 Global permissions

Granted to a person or a group in `global_grants`.

| Number | Status | Permission | Gives | Default |
|---|---|---|---|---|
| **PRM-001** | built | `use_product` | Nothing works without it | Members and guests |
| **PRM-002** | built | `create_space` | Create a team space, and check whether a key is free | Nobody |
| **PRM-003** | built | `view_profiles` | The directory of people and groups, people in quick search, pickers | Members. Guests only when granted |
| **PRM-004** | built | `administrator` | Users, groups, settings, audit, spaces as metadata, recover access. **No content** | First person, through the command line |
| **PRM-005** | built | `system_administrator` | Infrastructure, keys, backups, maintenance mode | First person |
| **PRM-006** | built | `create_personal_space` | One personal space | Members who can use the product |
| **PRM-009** | built | `invite_people` | Ask for an invitation for someone without an account. An administrator approves each one (ADR-033, spec 022) | Nobody |

- **PRM-007** · built · At least one active administrator must remain. People cannot break that. It does not hold for an identity provider on purpose, because a sync that fails would lock people out. The way back is `diffluence user create-admin`.
- **PRM-008** · built · Only a system administrator gives or takes `system_administrator`.

## 8.2 Space permissions

Granted to a person, a group, all members or all guests, in `space_grants`.

| Number | Status | Permission | Gives |
|---|---|---|---|
| **PRM-010** | built | `space.view` | The space and its pages. Every other permission needs this one too |
| **PRM-011** | built | `space.export` | Export what the person can see |
| **PRM-012** | built | `space.administer` | Name, settings, grants, trash, archive, delete. What specs 036 and 040 split off it is in 8.7 |
| **PRM-013** | built | `page.create` | New pages, folders and links, moving and copying them into the space, importing. Blog posts and databases have their own since spec 045 (PRM-206, PRM-215) |
| **PRM-014** | built | `page.edit` | Editing, renaming, page look, publishing, restoring a version. Databases have their own since spec 049 (PRM-216) |
| **PRM-015** | built, used from wave D | `page.archive` | Archiving a page. Exists with nothing behind it today |
| **PRM-016** | built | `page.delete_own` | Delete a page the person created |
| **PRM-017** | built | `page.delete_any` | Delete any page |
| **PRM-018** | built, used from wave C | `comment.add` | Comment, and edit own comments |
| **PRM-019** | built, used from wave C | `comment.delete_own` | Delete own comments |
| **PRM-020** | built, used from wave C | `comment.delete_any` | Delete any comment |
| **PRM-021** | built | `attachment.add` | Add files and versions |
| **PRM-022** | built | `attachment.delete_own` | Delete a file the person added first |
| **PRM-023** | built | `attachment.delete_any` | Delete any file |
| **PRM-024** | built | `restriction.manage` | Add and lift restrictions on pages the person can see |
| **PRM-025** | built | `space.manage_access` | The grants of a space, without *administer*. Implied by *administer* in `Evaluate`. Cannot touch a subject that holds *administer* |
| **PRM-026** | built | `analytics.view` | Insights of the space. The migration gives it to who holds *administer* |

## 8.3 Actions that are not permissions

| Number | Status | Action | Decided by |
|---|---|---|---|
| **PRM-027** | built | `page.view` | `space.view` and the restrictions |
| **PRM-028** | built | `page.delete` | `delete_any`, or `delete_own` for the person who created the page |
| **PRM-029** | built | `attachment.delete` | `delete_any`, or `delete_own` for the person who added the file. A new version by someone else does not change who that is |
| **PRM-030** | built | `comment.delete` | `delete_any`, or `delete_own` for the author |

"Own" always means the creator. Handing a page to a new owner gives no right (wave D, D9). The field `Input.Owner` holds the creator, and should be renamed before `pages.owner_id` lands, because two meanings of one word cause bugs.

## 8.4 Named sets

A set fills in boxes. It is not a rule, and it can be changed box by box afterwards.

| Number | Status | Set | Permissions |
|---|---|---|---|
| **PRM-032** | built | Viewer | view, export, comment add, comment delete own |
| **PRM-033** | built | Editor | view, export, page create, edit, move out, archive, delete own, comment add, comment delete own, attachment add, attachment delete own |
| **PRM-034** | built | Space administrator | All space permissions |
| **PRM-035** | built | Manager | Editor, plus delete any page, comment and file, manage restrictions, manage access, ask for guests |
| **PRM-036** | built | Own sets | Defined by administrators, for the whole instance. They became roles in spec 036 (PRM-190) |
| **PRM-037** | built | Presets | Team space: all members get Editor. Read-only knowledge base: all members get Viewer. Other entries stay |

## 8.5 Rules of a route

- **PRM-031** · built · Every route is registered with an access rule, or registration fails: `public`, `authenticated`, `permission`, `object`, `scim-token`. `docs/api/routes.txt` lists all of them, and a test compares it. Wave F adds `Sensitive` and `SessionOnly`.
- **PRM-038** · built · Token scopes: `read`, `write`, `export`, `admin`. The scope limits a token, and the rule still runs for its owner. A token never reaches `SessionOnly` routes.
- **PRM-039** · built · `object` declares the rule, and the module makes the decision, because only it knows the object. A route cannot skip the check, because the module's method checks by itself.

## 8.6 Roles

Spec 036, ADR-034. A role is not a set that fills in boxes: people and groups hold roles, and what they may do follows the role when it changes.

- **PRM-178** · built · A role is a named set of permissions, for a space or for the instance. People and groups get roles, and what they may do is what all their roles add up to.
- **PRM-179** · built · The built-in space roles Viewer, Editor, Manager and Space admin keep the meaning of PRM-032 to PRM-035, and cannot be changed or deleted. A test compares them with the templates in the code.
- **PRM-180** · built · Administrators make, rename and change roles. Anyone who uses the product may read the list, so a space manager can choose one.
- **PRM-181** · built · A role with `system_administrator` needs a system administrator to make, change or give, as PRM-008 says for the permission.
- **PRM-182** · built · A role in use cannot be deleted. The people who hold it get another role first, where it can be seen.
- **PRM-183** · built · A change to the permissions of a role in use needs a preview and a confirmation. The preview says how many assignments, spaces and people it reaches, and what they gain and lose.
- **PRM-184** · built · A space role in use cannot gain `space.administer`. Who administers a space is chosen in that space.
- **PRM-185** · built · A change to a role cannot leave a space without an administrator, or the instance without an active administrator.
- **PRM-186** · built · Space roles are given by whoever manages access in the space, within PRM-025: only a space administrator makes or unmakes an administrator, a space keeps one, and the owner of a personal space keeps theirs.
- **PRM-187** · built · Instance roles are given by administrators, to users and groups only.
- **PRM-188** · built · Every change to a role or to who holds it is audited, with before and after: the names of the roles and the permissions.
- **PRM-189** · built · Permissions set one by one, through the older routes, become one role with exactly that set: the built-in role when it matches, otherwise a role with a short name of its own (PRM-218). The same set shares one role.
- **PRM-190** · built · Own sets of PRM-036 became space roles under their own names.
- **PRM-191** · built · Screens for roles and for who holds them, in the settings pattern of ADR-034: Administration > Roles makes and changes roles, with the permissions in groups and a line on what each allows, and shows who holds each role and where; a space's settings give roles under People and roles; who holds which instance permission stays as a list to read.

## 8.7 Finer permissions

Spec 036. The product owner asked whether one screen of permissions was all there is. Four things hid under `space.administer` that a space can reasonably give apart: they are now permissions of their own. Whoever held *administer* holds them too, so nobody gained or lost anything by the split. Spec 040 split eight more in the same way: PRM-207 to PRM-214.

| ID | Status | Permission | What it allows | Split from |
|---|---|---|---|---|
| **PRM-198** | built | `site.publish` | Public sites of the space, and public links to its pages | `space.administer` |
| **PRM-199** | built | `space.automation` | Automation rules and webhooks of the space. Those who hold it are told when a rule stops because its owner left | `space.administer` |
| **PRM-200** | built | `space.templates` | Making, importing and changing the templates of the space, and saving a page as one | `space.administer` |
| **PRM-201** | built | `space.audit` | Reading the audit of the space | `space.administer` |

- **PRM-202** · built · A permission split off a broader one works wherever the broader one worked, in an archived space and on an archived page too, so the split changes no decision. `permissions.SplitFrom` names the pairs; the matrix test holds `Evaluate`, the SQL and the literal rule to it.
- **PRM-203** · built · The migration gives each split-off permission to every subject and own set that held the broader one, before turning grants into roles. Its test compares 369,861 decisions before and after, 720 of them allowed through a split, and none changed. Spec 040 does the same for its eight (migration 0040, `rbac_split_off`): the test then compared 949,740 decisions, 3,506 of them allowed through a split, and none changed. Spec 045 does it for posts and databases (migration 0041): 1,016,664 decisions, 4,224 allowed through a split, none changed, and a second test runs the migration on roles that existed before it.
- **PRM-204** · built · A bundle from before a split gives the split-off permissions with the broader one: format 1 those of specs 036, 040 and 045, format 2 those of specs 040 and 045, format 3 those of spec 045. Bundles are now format 4 and taken as they are. `exchange.heldSince` names the first format that holds each permission, and a test fails when one is missing.
- **PRM-205** · built · Every permission has a name and one line on what it allows, in the translation files, grouped as pages, comments, files, people and access, space, and administration. No key reaches a screen: a role made from single permissions is named in the same words, and a test holds the database and the interface to one wording.
- **PRM-206** · built · `post.create`, "Create blog posts": writing new blog posts, apart from `page.create` (ADR-038, spec 045). Turning an unpublished page into a post needs it as well as `page.edit`, and turning a post back into a page needs `page.create`. Only the creator sees an unpublished page, and they made it with `page.create`, so the migration takes this from nobody who could do it then.
- **PRM-207** · built · `space.labels`, "Labels": the fixed labels of a space and renaming a label, apart from `space.administer` (spec 040).
- **PRM-208** · built · `space.statuses`, "Statuses and reviews": the page statuses and the review period of a space, apart from `space.administer`.
- **PRM-209** · built · `space.approvals`, "Approval rule": reading and setting the approval rule of a space and its fixed approvers, apart from `space.administer`.
- **PRM-210** · built · `page.move_out`, "Move pages to another space": moving a page and the pages below it to another space, apart from `page.edit`. The page still needs create where it is and where it goes, and an edit restriction on the page stops it as it stops editing. Copying out stays as it was, because a copy takes nothing away from the space and needs only view there: asking more would take access from people who have it.
- **PRM-211** · built · Export per format. `space.bundle`, "Bundle the space", stands apart from `space.administer`; a bundle still needs `space.export` too, because it is an export. Every other format stays under the one `space.export`, as the product owner decided (ADR-038).
- **PRM-212** · built · `page.watchers`, "Manage who watches": seeing who watches a page, and adding or removing someone else, apart from `space.administer`. The block that lists contributors follows it too.
- **PRM-213** · built · `space.invite_guests`, "Ask for guests": asking for a guest in one space, apart from `space.manage_access`, which held it before. *Administer* implies it, as it implies managing access, and the instance-wide `invite_people` still decides who may ask at all.
- **PRM-214** · built · `space.look`, "Look and shortcuts": the shortcuts, picture, title alignment and home page of a space, apart from `space.administer`. The name, description and kind of icon stay with *administer*, because they say what the space is.
- **PRM-215** · built · `database.create`, "Create databases": making a database, and copying or moving one into a space, apart from `page.create` (ADR-038, spec 045). It was proposed as `space.databases`; it guards making only, because editing a database stays `page.edit`, as for every kind of page. Each module asks for the permission of the kind it makes (`permissions.CreatePermission`), also on the parent and for every kind in a branch that is copied or moved in. Folders and links stay with `page.create`. Editing a database got its own permission later: PRM-216.
- **PRM-216** · built · `database.edit`, "Edit databases": changing a database that is already there, its rows, columns and views, and everything `page.edit` allows on any other item, apart from `page.edit` (ADR-040, spec 049). The rule asks for it wherever `page.edit` is asked of an item of kind `database`, so no module can forget it, and `perm_editable_pages` does the same. Whoever held `page.edit` got it in migration 0044, which touches no other split.
- **PRM-217** · built · A person or a group can hold several roles in one space, and may do what any of them allows (ADR-040). The Access screen and a space's People and roles show every role held, add one at a time, and take one away at a time; taking the last one away asks first.
- **PRM-218** · built · A combination of single permissions nobody named (PRM-189) is named "Custom combination N (K permissions)": never cut off, with a number that stays with it, and the count follows when its permissions change. Its permissions show when it is opened on the Access screen, and an administrator can name it.
