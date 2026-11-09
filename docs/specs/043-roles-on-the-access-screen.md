# 043: Roles on the Access screen, and typed text that outlives a reload

Status: built on branch `wave-ar`.

The product owner asked that permissions are given as roles, not as a matrix of ticks. Spec 036 made roles and the Roles screen, and listed what it left: "Replacing the matrix of ticks in the space access screen with role choices". This spec does that, keeps "0 holders" on one line on the Roles screen, and lets comment boxes and settings forms join the save before a reload of spec 041.

## 1. The Access screen gives roles

### Study

**What there was.** The Access screen of a space (`/spaces/{key}/access`, tab Permissions) listed each subject with a select of the four templates and the named sets, a button "Detailed permissions" that opened a matrix of checkboxes over every space permission, and Remove access. It wrote through `PUT /spaces/{id}/grants` with single permissions, which since migration 0036 finds or makes the role with exactly that set. The settings of a space had a second place, People and roles, that gave roles with a checkbox per role in a dialog, and showed no preview at all.

**What a role is now.** A role holds exactly its permissions (spec 036, D1 and D15). The grants of a subject are what its roles add up to, kept equal by triggers. A set that matches no role became a role named "Custom: " and its permissions in words (`rbac_role_for`).

### Choices

| # | Question | Choice | Why |
|---|---|---|---|
| D1 | What a row shows | The person or group, what kind it is, and one role chosen from a list, in the member rows of the People screen (Stil) | The product owner asked for roles, not ticks. The member rows are the settings pattern for people already |
| D2 | What the list holds | Every space role, in three groups: built in, made for this instance, combinations without a name | Built-in and custom roles, as asked. The third group keeps an old combination visible instead of hiding it under "Custom" |
| D3 | A combination that matches no named role | Shown under its own name, "Custom: View the space, Edit pages, ...", with the line "This combination of permissions matches no named role." | Honest: the row says what the person holds. The name is the one the database gave it |
| D4 | Turning it into a named role | "Name this role", for administrators: a new name, the same permissions | Renaming changes nobody's access (`UpdateRole` asks for no confirmation when the permissions stay). Others see "An administrator can give it a name.", because making and naming roles is an administrator's (spec 036, D8) |
| D5 | Someone holds two roles | The list shows "Several roles: A, B" until one is chosen | Only the old dialog could give two; choosing here gives exactly one |
| D6 | The preview | A new route, `POST /spaces/{id}/role-assignments/preview`, with the same body as the change | The change goes through `SetSpaceRoles`, so the preview must work out the same permissions: the union of the roles, exactly, with nothing filled in |
| D7 | When the screen asks | When someone loses the space, as before. Every other change is saved and then says what was gained and lost | The behaviour wave R and spec 033 settled on. Nothing changes without the person seeing what it did |
| D8 | Limits | The preview applies the limits of the change itself: only a space administrator makes or unmakes an administrator, the owner of a personal space keeps theirs | A preview must never promise a change that will be refused |
| D9 | Split-off permissions | Unchanged: the built-in Space admin role lists all of them; a role holds what it lists | `permissions.WithSplitOff` stays on the older single-permission routes, where a list may predate a split. A role says what it means |
| D10 | The settings part People and roles | Shows the same panel | One way to give roles, not two |
| D11 | The matrix | Removed from the screen. The older routes for single permissions stay, for scripts | Retiring them is a separate, breaking change (spec 036, Left) |

### Load budget (ARC-238)

| What | Cost | Class |
|---|---|---|
| Opening the screen | Two reads: the assignments of the space (one grouped query) and the space roles (one query, with a count per role). The old screen read the grants instead | reading |
| A preview | One transaction: the access check, the roles chosen, the grants of one subject, the administrator check; for a group, one count of its members | reading |
| A change | Unchanged from spec 036: one statement per role given or taken, and the triggers write only the difference | saving |
| Naming a role | One update of the role; its permissions stay, so no trigger rewrites a grant | saving |

The list of assignments is not paged. It grows with the subjects given a role in one space, not with content; most spaces give roles to a few groups. This was not measured past a few dozen rows.

## 2. "0 holders" on one line

The Held by cell of the Roles screen gets `white-space: nowrap`, so the count never breaks between the number and the word.

## 3. Typed text outlives a reload

Spec 041 made the Reload of a new version wait for the open editor to save. Two kinds of open work were left out.

**Comment boxes.** A comment box already keeps its draft in the browser on every change. It now also joins `diffluence:before-reload`: it writes the draft once more and notes, in this tab's `sessionStorage`, that it was open with text. After the reload a reply box and the box of a comment being changed open again by themselves with the text; for a comment being changed, the kept text wins over the saved comment. An empty box is not opened again. A comment on a selection keeps its draft, but its box does not open again, because the selection is gone.

**Settings forms.** Every text field of a form in the settings area (the administration and a space's settings) that someone typed in is written to `sessionStorage` before the reload, with the address of the screen. After the reload each value goes back into its field as the form appears, for up to 15 seconds, the way typing does, so the screen's own state follows. Nothing is saved to the server. Password fields are never kept.

**Load budget (ARC-238).** No request and no query: a few small writes to `sessionStorage`, on Reload only.

## Routes

| Route | Rule |
|---|---|
| `POST /api/v1/spaces/{id}/role-assignments/preview` | object: `space.manage_access`; the module checks access, the subject, the roles and the limits of the change |

## Tests

Failure paths first. `internal/permissions/roles_test.go`, `TestPreviewingRolesSaysWhatChangesAndChangesNothing`: a stranger gets not found, an unknown role and an unknown kind of subject are refused, a manager cannot preview making an administrator; then a role's gains, a narrower role's losses, taking every role away losing the space, the Space admin role bringing its split-off permissions because it lists them, and a preview changing nothing. `internal/app/access_roles_test.go` covers the route: not found for a stranger, a bad body refused, and a preview that gives no access. Four mutations break the access check, the limits, the roles chosen and the "loses the space" flag of the preview.

`e2e/access-roles.spec.ts`: a manager cannot make an administrator (the alert names why, the role stays), Remove access asks first and Cancel changes nothing, a narrower role says what it took; a combination without a name shows as it is, a taken name is refused, and naming it changes no permission. `e2e/new-version.spec.ts`: an empty reply box is not opened again; a reply and a comment being changed come back open, with their text, and the server has neither; text typed into a space's settings comes back, unsaved. Pictures: `space-access`, `space-roles`, `name-role` and `admin-roles` in `0-design.spec.ts`.

## Left

- The People screen of a space (spec 033) still offers only the four templates in its role choice. It should offer the same list as the Access screen.
- The Holder table on the People tab of the Access screen still names a role by template, or "Custom".
- Retiring the single-permission routes (`PUT /spaces/{id}/grants`, presets) once no script needs them, and dropping `permission_sets` (PRM-197).
- Opening a comment on a selection again after a reload: it would need the selection kept as well.
