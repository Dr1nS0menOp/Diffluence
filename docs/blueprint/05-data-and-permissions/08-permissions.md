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
| **PRM-006** | wave F | `create_personal_space` | One personal space | Members who can use the product |
| **PRM-009** | built | `invite_people` | Ask for an invitation for someone without an account. An administrator approves each one (ADR-033, spec 022) | Nobody |

- **PRM-007** · built · At least one active administrator must remain. People cannot break that. It does not hold for an identity provider on purpose, because a sync that fails would lock people out. The way back is `diffluence user create-admin`.
- **PRM-008** · built · Only a system administrator gives or takes `system_administrator`.

## 8.2 Space permissions

Granted to a person, a group, all members or all guests, in `space_grants`.

| Number | Status | Permission | Gives |
|---|---|---|---|
| **PRM-010** | built | `space.view` | The space and its pages. Every other permission needs this one too |
| **PRM-011** | built | `space.export` | Export what the person can see |
| **PRM-012** | built | `space.administer` | Name, settings, grants, trash, archive, delete, shortcuts |
| **PRM-013** | built | `page.create` | New pages, moving and copying into the space, importing |
| **PRM-014** | built | `page.edit` | Editing, renaming, page look, publishing, restoring a version |
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
| **PRM-025** | wave F | `space.manage_access` | The grants of a space, without *administer*. Implied by *administer* in `Evaluate`. Cannot touch a subject that holds *administer* |
| **PRM-026** | wave H | `analytics.view` | Insights of the space. The migration gives it to who holds *administer* |

## 8.3 Actions that are not permissions

| Number | Status | Action | Decided by |
|---|---|---|---|
| **PRM-027** | built | `page.view` | `space.view` and the restrictions |
| **PRM-028** | built | `page.delete` | `delete_any`, or `delete_own` for the person who created the page |
| **PRM-029** | built | `attachment.delete` | `delete_any`, or `delete_own` for the person who added the file. A new version by someone else does not change who that is |
| **PRM-030** | wave C | `comment.delete` | `delete_any`, or `delete_own` for the author |

"Own" always means the creator. Handing a page to a new owner gives no right (wave D, D9). The field `Input.Owner` holds the creator, and should be renamed before `pages.owner_id` lands, because two meanings of one word cause bugs.

## 8.4 Named sets

A set fills in boxes. It is not a rule, and it can be changed box by box afterwards.

| Number | Status | Set | Permissions |
|---|---|---|---|
| **PRM-032** | built | Viewer | view, export, comment add, comment delete own |
| **PRM-033** | built | Editor | view, export, page create, edit, archive, delete own, comment add, comment delete own, attachment add, attachment delete own |
| **PRM-034** | built | Space administrator | All space permissions |
| **PRM-035** | wave F | Manager | Editor, plus delete any page, comment and file, manage restrictions, manage access |
| **PRM-036** | wave F | Own sets | Defined by administrators, for the whole instance |
| **PRM-037** | built | Presets | Team space: all members get Editor. Read-only knowledge base: all members get Viewer. Other entries stay |

## 8.5 Rules of a route

- **PRM-031** · built · Every route is registered with an access rule, or registration fails: `public`, `authenticated`, `permission`, `object`, `scim-token`. `docs/api/routes.txt` lists all of them, and a test compares it. Wave F adds `Sensitive` and `SessionOnly`.
- **PRM-038** · wave F and H · Token scopes: `read`, `write`, `export`, `admin`. The scope limits a token, and the rule still runs for its owner. A token never reaches `SessionOnly` routes.
- **PRM-039** · built · `object` declares the rule, and the module makes the decision, because only it knows the object. A route cannot skip the check, because the module's method checks by itself.
