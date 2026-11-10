// SPDX-License-Identifier: Apache-2.0

package permissions_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
)

// withoutDatabaseEdit is a list of permissions as it stood before migration 0044.
func withoutDatabaseEdit(perms []string) []string {
	return slices.DeleteFunc(slices.Clone(perms), func(p string) bool { return p == permissions.DatabaseEdit })
}

// databaseWorld is a space with a database and a page in it, and people who hold roles of
// every shape that matters to editing, written as before migration 0044.
type databaseWorld struct {
	roleWorld
	database, page uuid.UUID
	people         map[string]uuid.UUID
}

func newDatabaseWorld(t *testing.T) databaseWorld {
	t.Helper()
	w := databaseWorld{roleWorld: newRoleWorld(t), people: map[string]uuid.UUID{}}
	inst := ids.New()
	for _, key := range []string{permissions.TemplateEditor, permissions.TemplateManager, permissions.TemplateSpaceAdmin} {
		w.exec(t, "UPDATE roles SET permissions = $2 WHERE builtin = $1", key, withoutDatabaseEdit(permissions.Templates[key]))
	}
	shapes := map[string][]string{
		"writer":       {permissions.SpaceView, permissions.PageEdit},
		"reader":       {permissions.SpaceView, permissions.CommentAdd},
		"editor":       withoutDatabaseEdit(permissions.Templates[permissions.TemplateEditor]),
		"edit-no-move": {permissions.SpaceView, permissions.PageEdit, permissions.PageCreate},
	}
	for name, perms := range shapes {
		id := ids.New()
		w.exec(t, `INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, 'member', 'active', $3, $3)`,
			id, inst, name+"@example.com")
		w.exec(t, "SELECT rbac_set_exact(NULL, 'user', $1, ARRAY['use_product'])", id)
		w.exec(t, "SELECT rbac_set_exact($1, 'user', $2, $3::text[])", w.space, id, perms)
		w.people[name] = id
	}
	w.people["manager"], w.people["owner"] = w.manager.UserID, w.owner.UserID
	addItem := func(kind string) uuid.UUID {
		id := ids.New()
		w.exec(t, `INSERT INTO pages (id, origin_instance_id, space_id, title, status, created_by, updated_by, kind)
			VALUES ($1, $2, $3, 't', 'published', $4, $4, $5)`, id, inst, w.space, w.owner.UserID, kind)
		w.exec(t, "INSERT INTO page_tree (ancestor_id, descendant_id, depth) VALUES ($1, $1, 0)", id)
		return id
	}
	w.database, w.page = addItem("database"), addItem("page")
	return w
}

// mayEdit asks the rule whether someone may edit an item. asKind overrides the kind of
// the item, to ask as the rule before 0044 did, which never looked at the kind.
func (w databaseWorld) mayEdit(t *testing.T, who, item uuid.UUID, asKind string) bool {
	t.Helper()
	facts, err := w.svc.LoadUser(w.ctx, w.pool, who)
	if err != nil {
		t.Fatal(err)
	}
	space, page, err := w.svc.LoadPage(w.ctx, w.pool, item)
	if err != nil {
		t.Fatal(err)
	}
	if asKind != "" {
		page.Kind = asKind
	}
	return permissions.Evaluate(permissions.Input{User: facts, Space: space, Page: &page, Action: permissions.PageEdit}).Allowed
}

func (w databaseWorld) editableBySQL(t *testing.T, who, item uuid.UUID) bool {
	t.Helper()
	var ok bool
	if err := w.pool.QueryRow(w.ctx, "SELECT EXISTS (SELECT 1 FROM perm_editable_pages($1) WHERE page_id = $2)", who, item).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

// TestEditingADatabaseChangesForNobodyInTheMigration is the proof of migration 0044: whoever
// could edit a database before it, by page.edit, can after it, by database.edit, and
// whoever could not still cannot. Pages are untouched. The SQL list agrees with the rule.
func TestEditingADatabaseChangesForNobodyInTheMigration(t *testing.T) {
	w := newDatabaseWorld(t)
	before := map[string]bool{}
	for name, who := range w.people {
		before[name+" database"] = w.mayEdit(t, who, w.database, "page")
		before[name+" page"] = w.mayEdit(t, who, w.page, "")
	}
	if !before["writer database"] || before["reader database"] {
		t.Fatalf("the world before the migration is not what this test needs: %v", before)
	}

	w.exec(t, "SELECT rbac_split_roles_from('page.edit', ARRAY['database.edit'])")

	compared := 0
	for name, who := range w.people {
		for item, id := range map[string]uuid.UUID{"database": w.database, "page": w.page} {
			key := name + " " + item
			if got := w.mayEdit(t, who, id, ""); got != before[key] {
				t.Errorf("%s: may edit %v after the migration, %v before", key, got, before[key])
			}
			if got := w.editableBySQL(t, who, id); got != before[key] {
				t.Errorf("%s: the SQL list says %v, the rule before said %v", key, got, before[key])
			}
			compared++
		}
	}
	t.Logf("%d decisions compared", compared)
	noDrift(t, w.ctx, w.pool)
}

// TestTheMigrationAddsOnlyDatabaseEditing: roles and own sets that hold page.edit gain
// database.edit and nothing else; a role made without moving pages out stays without it,
// and the built-in roles mean again what their templates say.
func TestTheMigrationAddsOnlyDatabaseEditing(t *testing.T) {
	w := newDatabaseWorld(t)
	w.exec(t, "INSERT INTO permission_sets (id, name, permissions) VALUES ($1, 'Writer', ARRAY['space.view', 'page.edit'])", ids.New())
	grants := func(who uuid.UUID) []string {
		t.Helper()
		var got []string
		if err := w.pool.QueryRow(w.ctx, `SELECT ARRAY(SELECT permission FROM space_grants WHERE space_id = $1 AND subject_id = $2 ORDER BY 1)`,
			w.space, who).Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	before := map[string][]string{}
	for name, who := range w.people {
		before[name] = grants(who)
	}

	w.exec(t, "SELECT rbac_split_roles_from('page.edit', ARRAY['database.edit'])")

	for name, who := range w.people {
		want := slices.Clone(before[name])
		if slices.Contains(want, permissions.PageEdit) {
			want = append(want, permissions.DatabaseEdit)
		}
		slices.Sort(want)
		if got := grants(who); !slices.Equal(got, want) {
			t.Errorf("%s holds %v after the migration, want %v", name, got, want)
		}
	}
	if slices.Contains(grants(w.people["edit-no-move"]), permissions.PageMoveOut) {
		t.Fatal("the migration gave moving pages out to a role made without it")
	}
	var set []string
	if err := w.pool.QueryRow(w.ctx, "SELECT ARRAY(SELECT p FROM unnest(permissions) p ORDER BY 1) FROM permission_sets WHERE name = 'Writer'").
		Scan(&set); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(set, permissions.DatabaseEdit) {
		t.Fatalf("an own set with page.edit was left without database.edit: %v", set)
	}
	for _, key := range []string{permissions.TemplateEditor, permissions.TemplateManager, permissions.TemplateSpaceAdmin} {
		var got []string
		if err := w.pool.QueryRow(w.ctx, "SELECT ARRAY(SELECT p FROM unnest(permissions) p ORDER BY 1) FROM roles WHERE builtin = $1", key).
			Scan(&got); err != nil {
			t.Fatal(err)
		}
		if want := slices.Sorted(slices.Values(permissions.Templates[key])); !slices.Equal(got, want) {
			t.Fatalf("%s after the migration: %v, template %v", key, got, want)
		}
	}
	noDrift(t, w.ctx, w.pool)

	// A script on the older grants route writes a list from before the split.
	scripted := w.people["reader"]
	if err := w.svc.SetGrants(w.ctx, w.owner, w.space, permissions.Subject{Type: permissions.SubjectUser, ID: scripted},
		[]string{permissions.SpaceView, permissions.PageEdit}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(grants(scripted), permissions.DatabaseEdit) {
		t.Fatalf("an old list with page.edit came in without database.edit: %v", grants(scripted))
	}
}

// TestDatabasesAndPagesAreEditedApart: after the split a role can edit one kind without
// the other, and an edit restriction on a database still counts.
func TestDatabasesAndPagesAreEditedApart(t *testing.T) {
	w := newDatabaseWorld(t)
	pagesOnly, databasesOnly := w.people["writer"], w.people["reader"]
	w.exec(t, "SELECT rbac_set_exact($1, 'user', $2, ARRAY['space.view', 'page.edit'])", w.space, pagesOnly)
	w.exec(t, "SELECT rbac_set_exact($1, 'user', $2, ARRAY['space.view', 'database.edit'])", w.space, databasesOnly)
	for _, tc := range []struct {
		who            uuid.UUID
		item           uuid.UUID
		want           bool
		name, itemName string
	}{
		{pagesOnly, w.page, true, "pages only", "page"},
		{pagesOnly, w.database, false, "pages only", "database"},
		{databasesOnly, w.page, false, "databases only", "page"},
		{databasesOnly, w.database, true, "databases only", "database"},
	} {
		if got := w.mayEdit(t, tc.who, tc.item, ""); got != tc.want {
			t.Errorf("%s editing a %s: %v, want %v", tc.name, tc.itemName, got, tc.want)
		}
		if got := w.editableBySQL(t, tc.who, tc.item); got != tc.want {
			t.Errorf("%s editing a %s, SQL list: %v, want %v", tc.name, tc.itemName, got, tc.want)
		}
	}
	w.exec(t, "INSERT INTO page_restrictions (page_id, kind, subject_type, subject_id) VALUES ($1, 'edit', 'user', $2)",
		w.database, w.owner.UserID)
	if w.mayEdit(t, databasesOnly, w.database, "") || w.editableBySQL(t, databasesOnly, w.database) {
		t.Fatal("an edit restriction on a database did not hold")
	}
}

// TestUnnamedRoleNamesAreShortAndTheSameInGoAndSQL: a combination nobody named gets
// "Custom combination N (K permissions)", never cut off, with a number that grows, and Go
// reads it as unnamed.
func TestUnnamedRoleNamesAreShortAndTheSameInGoAndSQL(t *testing.T) {
	w := newRoleWorld(t)
	for _, count := range []int{1, 2, 31} {
		var got string
		if err := w.pool.QueryRow(w.ctx, "SELECT rbac_unnamed_role_name(7, $1)", count).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if want := permissions.UnnamedRoleName(7, count); got != want {
			t.Fatalf("SQL names it %q, Go %q", got, want)
		}
	}
	long := slices.Clone(permissions.AllSpacePermissions)
	long = slices.DeleteFunc(long, func(p string) bool { return p == permissions.SpaceAdminister })
	var names []string
	for i, perms := range [][]string{{permissions.SpaceView, permissions.CommentAdd}, long} {
		who := ids.New()
		w.exec(t, `INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, 'member', 'active', $3, $3)`,
			who, ids.New(), fmt.Sprintf("combo%d@example.com", i))
		w.exec(t, "SELECT rbac_set_exact($1, 'user', $2, $3::text[])", w.space, who, perms)
		var name string
		if err := w.pool.QueryRow(w.ctx, `SELECT r.name FROM role_assignments a JOIN roles r ON r.id = a.role_id
			WHERE a.space_id = $1 AND a.subject_id = $2`, w.space, who).Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if names[0] != permissions.UnnamedRoleName(1, 2) || names[1] != permissions.UnnamedRoleName(2, len(long)) {
		t.Fatalf("names %q", names)
	}
	roles, err := w.svc.Roles(w.ctx, w.admin, permissions.ScopeSpace)
	if err != nil {
		t.Fatal(err)
	}
	unnamed := 0
	for _, r := range roles {
		if r.Unnamed {
			unnamed++
			if r.BuiltIn != "" || !slices.Contains(names, r.Name) {
				t.Fatalf("%q is marked unnamed", r.Name)
			}
		}
	}
	if unnamed != 2 {
		t.Fatalf("want two unnamed roles, got %d", unnamed)
	}
}

// TestCombinationsNamedBeforeGetTheShortName: a role the database named after its
// permissions before migration 0044 gets the short name, in the order they were made; a
// role an administrator named, even "Custom: …", keeps its name, and nobody's grants change.
func TestCombinationsNamedBeforeGetTheShortName(t *testing.T) {
	w := newRoleWorld(t)
	const madeOneByOne = "Made from permissions that were given one by one."
	older, newer, named := ids.New(), ids.New(), ids.New()
	w.exec(t, `INSERT INTO roles (id, scope, name, description, permissions, created_at) VALUES
		($1, 'space', 'Custom: Add comments, View the space', $4, ARRAY['comment.add', 'space.view'], now() - interval '2 days'),
		($2, 'space', 'Custom: Add comments, Add files, Archive pages, Create pages, Delete own fil', $4,
			ARRAY['attachment.add', 'comment.add', 'page.archive', 'page.create', 'space.view'], now() - interval '1 day'),
		($3, 'space', 'Custom: our reviewers', 'Named by an administrator.', ARRAY['space.view', 'analytics.view'], now())`,
		older, newer, named, madeOneByOne)
	who := ids.New()
	w.exec(t, `INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, 'member', 'active', 'n@example.com', 'n')`,
		who, ids.New())
	w.exec(t, "INSERT INTO role_assignments (role_id, space_id, subject_type, subject_id) VALUES ($1, $2, 'user', $3)", newer, w.space, who)
	var before []string
	if err := w.pool.QueryRow(w.ctx, "SELECT ARRAY(SELECT permission FROM space_grants WHERE subject_id = $1 ORDER BY 1)", who).Scan(&before); err != nil {
		t.Fatal(err)
	}

	w.exec(t, "SELECT rbac_rename_unnamed()")

	name := func(id uuid.UUID) string {
		t.Helper()
		var n string
		if err := w.pool.QueryRow(w.ctx, "SELECT name FROM roles WHERE id = $1", id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := name(older); got != permissions.UnnamedRoleName(1, 2) {
		t.Fatalf("the older combination is %q", got)
	}
	if got := name(newer); got != permissions.UnnamedRoleName(2, 5) {
		t.Fatalf("the newer combination is %q", got)
	}
	if got := name(named); got != "Custom: our reviewers" {
		t.Fatalf("a name an administrator gave changed to %q", got)
	}
	var after []string
	if err := w.pool.QueryRow(w.ctx, "SELECT ARRAY(SELECT permission FROM space_grants WHERE subject_id = $1 ORDER BY 1)", who).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before, after) {
		t.Fatalf("renaming changed the grants: %v, then %v", before, after)
	}
	noDrift(t, w.ctx, w.pool)
}

// TestAnUnnamedRoleKeepsItsCountTrue: changing the permissions of an unnamed combination
// that keeps its name changes the count in the name; naming it keeps the new name.
func TestAnUnnamedRoleKeepsItsCountTrue(t *testing.T) {
	w := newRoleWorld(t)
	who := ids.New()
	w.exec(t, `INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, 'member', 'active', 'c@example.com', 'c')`,
		who, ids.New())
	w.exec(t, "SELECT rbac_set_exact($1, 'user', $2, ARRAY['space.view', 'comment.add'])", w.space, who)
	var id uuid.UUID
	var name string
	if err := w.pool.QueryRow(w.ctx, `SELECT r.id, r.name FROM role_assignments a JOIN roles r ON r.id = a.role_id
		WHERE a.space_id = $1 AND a.subject_id = $2`, w.space, who).Scan(&id, &name); err != nil {
		t.Fatal(err)
	}
	three := []string{permissions.SpaceView, permissions.CommentAdd, permissions.SpaceExport}
	r, err := w.svc.UpdateRole(w.ctx, w.admin, id, permissions.RoleInput{Name: name, Permissions: three}, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := permissions.UnnamedRoleName(1, 3); r.Name != want || !r.Unnamed {
		t.Fatalf("after the change the role is %q (unnamed %v), want %q", r.Name, r.Unnamed, want)
	}
	r, err = w.svc.UpdateRole(w.ctx, w.admin, id, permissions.RoleInput{Name: "Commenters", Permissions: three}, true)
	if err != nil || r.Name != "Commenters" || r.Unnamed {
		t.Fatalf("naming it: %+v %v", r, err)
	}
}
