# Spec 049: The answers of 10 November

Status: built on branch `wave-fu`.
Decided by the product owner in ADR-040 (2026-11-10). This wave builds four of its answers:

1. editing a database gets its own permission;
2. a person or group may hold several roles in a space;
3. a combination nobody named gets a short name that is never cut off;
4. an upgrade refuses to start without a backup of the last 24 hours.

The editor, search and storage answers are in spec 047. The tools, S3 and Helm answers wait for the waves of spec 048.

## Study

### 1. Where editing a database is decided

A database is an item of kind `database` whose document is one sheet (`pages.kind`, migration 0019). Its rows, columns and views live in that document. Changing them is the same as changing any page: autosave of a draft, publishing, renaming, restoring a version, labels, the look, the owner, the schedule, approvals, moving within the space, the service for editing together, and import into the page. The study read every `RequirePage` and `CheckPage` with `page.edit`: 41 places in 14 packages. The route table also declares `Object("page.edit")` on 30 routes. Two places decide editing without `Evaluate`:

| Where | What it asks | Kind known there |
|---|---|---|
| `permissions.Evaluate` | the space grant of the action, here `page.edit` | yes: `PageFacts.Kind`, loaded with every page |
| `perm_editable_pages` (migration 0008) | `perm_spaces(u, 'page.edit')`, for comments open only to editors | yes: `pages.kind` |
| The capabilities of a page (`pages.capabilities`) | `Evaluate` for `page.edit`; the screens show Edit, labels, tasks and approvals on it | through `Evaluate` |
| The collab service | `CheckPage(..., page.edit)` before it lets a browser join | through `Evaluate` |

Blueprint 05 (PRM-215) asked for a database permission once the rule can tell kinds apart, and spec 045 made `database.create`. Spec 045 D1 kept editing a database under `page.edit`. ADR-040 now splits it off.

### 2. Several roles

Spec 036 made role assignments a list: `role_assignments` holds one row per role, and `SetSpaceRoles` takes a list of role ids. Spec 043 (wave AR) gave the Access screen one list box per person, which replaced whatever was held with the one chosen. The server, its preview and its audit already handle a list. Only the screen changes. The preview compares what the subject holds now with what the new list adds up to, and asks first when `space.view` would go.

### 3. Names of combinations

`rbac_role_for` (migration 0036) names a role it makes for a set of single permissions `Custom: ` plus the words of those permissions, cut at 90 characters. A long set ends mid-word ("…, Creat"). Wave AR found this and left it to the product owner. The screens told such a role apart by the prefix `Custom: `.

### 4. The backup check

Wave O7 (spec 044) made a node check, before it migrates, that the last noted backup is no older than `DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE`, and only when that is set (D2). A new installation is never checked. Spec 048 recommended refusing by default after 24 hours, and ADR-040 decided it.

## Choices

| # | Question | Choice | Why |
|---|---|---|---|
| D1 | The name of the permission | `database.edit`, "Edit databases" | It sits beside `database.create`, as `page.edit` beside `page.create` |
| D2 | Where the kind enters the rule | In `Evaluate`: asking `page.edit` of an item of kind `database` asks for `database.edit`. Modules keep asking `page.edit` | One place, so none of the 41 checks or 30 routes can forget a database. The matrix and the SQL list change with it |
| D3 | What `database.edit` covers | Everything `page.edit` allows on any other item: rows, columns and views, the title, labels, the look, the owner, versions, approvals, moving within the space and comments for editors | A database is one item. Half of its edits under one permission and half under another would surprise everyone |
| D4 | Asked on its own | `database.edit` as an action asks for itself. It counts as an edit for edit restrictions and does not work on an archived page or in an archived space, like `page.edit` | So the rule stays the same rule for every edit |
| D5 | The migration | A new SQL function `rbac_split_roles_from(broad, finer)` gives `database.edit` to every space role and own set that holds `page.edit` and not `database.edit`, and to nothing else. Bundles go to format 5. The older grants route completes a list with `page.edit` and no `database.edit` | `rbac_split_roles` would run every earlier split again, so a role made after migration 0040 without `page.move_out` would gain it. This wave touches only its own split |
| D6 | The SQL list | `perm_editable_pages` asks `database.edit` for databases and `page.edit` for the rest, from two lists of spaces each read once | The matrix test compares the list with the rule for every case |
| D7 | The Access screen | Each person and group shows every role it holds. A role opens to show its permissions, and a button takes that one role away. A list box adds a role without touching the others. "Remove access" takes all of them away. The owner of the space cannot lose their last role here | The server already takes lists. One role at a time keeps each change small and its preview clear |
| D8 | Giving access to someone who already holds roles | The new role is added to what they hold | Choosing a person a second time must not take roles away by surprise |
| D9 | The short name | `Custom combination N (K permissions)`, with N one more than the highest such number in that scope, and "permission" for one | Never cut off, still says what kind of role it is. The number stays with the role, so two people see the same name |
| D10 | Telling unnamed roles apart | The server marks a role `unnamed` when its name has that shape and it is not built in. The screens read the mark, not the name | One place for the rule |
| D11 | Changing an unnamed role | When its permissions change and the name stays as given, the count in the name follows. A name an administrator gives stays | So the name never says something untrue |
| D12 | Roles made before | Migration 0044 renames each role named `Custom: …` with the description the database gives, in the order they were made. Other roles keep their names | Only names the database made change; nobody gains or loses anything |
| D13 | The backup age | 24 hours unless `DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE` is set. `off` switches the check off; then the node only warns. `0`, a negative age or anything else is refused at start | A typing mistake must not switch a safety check off |
| D14 | Where the backup check runs | Where wave O7 put the other checks: a node that migrates on start, and only when there is something to migrate. `diffluence migrate`, the deliberate route by hand, does not check the backup, as it does not check the queue | Kept as spec 044 built it. A new installation is never refused |
| D15 | The upgrade test and the dev environment | `scripts/upgrade-test.sh` notes a backup before it starts the new release, as an operator does. The end-to-end tests start from an empty database, which is a new installation | So the test proves the path an operator takes |

## Migration

`0044_database_edit_and_role_names.sql` is an expand migration. It:

- teaches `rbac_split_off` that `page.edit` splits into `page.move_out` and `database.edit`;
- gives `rbac_words` the new name;
- runs `rbac_split_roles_from('page.edit', ARRAY['database.edit'])`;
- replaces `perm_editable_pages`;
- gives `rbac_role_for` the short names;
- renames the combinations made before, with `rbac_rename_unnamed()`.

The previous release does not read `database.edit`. It keeps asking `page.edit` of databases, which the same people still hold, so old and new nodes give the same answers side by side.

**Proof:**

- `TestEditingADatabaseChangesForNobodyInTheMigration` builds a space with a database and a page, and people with every shape of role, written as before 0044. It records for each person whether the rule as it was, which never looked at the kind, lets them edit each item, runs the migration step, and compares with the rule as it is and with the SQL list. Nobody's answer changes.
- `TestTheMigrationAddsOnlyDatabaseEditing` shows that the step adds `database.edit` where `page.edit` is and nothing else. A role made without `page.move_out` stays without it, an own set gains it, and the built-in roles equal their templates again. `rbac_drift()` stays empty, and the older grants route completes an old list.
- `TestCombinationsNamedBeforeGetTheShortName` renames combinations the database named, in the order they were made, leaves a name an administrator gave, and changes no grant.
- `TestAdoptingGrantsChangesNoDecision` (spec 036) still holds with the new split.
- The permission matrix compares every case of the rule with the literal rule and with the SQL lists. It gains the database as a kind of item, `database.edit` as an action, and two grant cases: editing pages but not databases, and databases only.

## Load budget (ARC-238)

| What | Cost | Class |
|---|---|---|
| Any check of `page.edit` | Unchanged: the kind is already in the page facts every check loads | reading, saving |
| `perm_editable_pages` | One more list of spaces, read once per call, beside the one it read | reading |
| The Access screen | Unchanged: the same list of assignments and roles; adding or removing a role is one save with its preview | saving |
| Making a combination | One more query for the highest number in the scope: a scan of the roles of that scope, which are counted in tens or hundreds, not thousands | saving, rare |
| The migration | One update of the roles that hold `page.edit`; the trigger rewrites the grants of their holders in the same transaction, as the migrations of specs 036, 040 and 045 did. The renaming touches only roles the database named | once |
| The backup check | One read of the last `system.backup_noted` event, only when there is something to migrate | once per upgrade |

## Acceptance

- [x] Someone with `page.edit` and not `database.edit` cannot edit a database, and the database shows no Edit
- [x] Someone with only `database.edit` edits databases and nothing else
- [x] An edit restriction on a database holds for `database.edit`
- [x] The migration changes no one's answer and gives `database.edit` to everyone who could edit pages
- [x] The Access screen shows every role someone holds, adds one without removing the others, and takes one away at a time
- [x] Taking the last role away asks first
- [x] A combination nobody named has a short name that is never cut off, its permissions show when it is opened, and it can be named
- [x] An upgrade without a backup of the last 24 hours is refused by default, and the refusal names both ways on
- [x] `off` lets an upgrade go on without a recent backup; a new installation is never refused

## How the product owner tests it

1. Administration > Roles > New role: tick View the space and Edit pages, not Edit databases. Give it to a test person in a space with a database.
2. As that person, open a page: Edit shows. Open the database: no Edit.
3. Open the Access screen of that space as a space administrator. Under the test person, choose Viewer in "Add a role": they now hold both roles. Open a role to see its permissions. Take one away with the cross beside it.
4. To see the backup check, update a test installation without running `scripts/backup.sh` first: it does not start, and its log says to run the backup or to set `DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE=off`.

## Provenance

Own design, on the decisions of ADR-040.
