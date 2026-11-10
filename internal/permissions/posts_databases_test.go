// SPDX-License-Identifier: Apache-2.0

package permissions_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
)

// TestTheMigrationGivesWhoeverCreatesPagesPostsAndDatabases is the proof of migration 0041
// on roles that exist before it: every role and own set that holds page.create gains
// post.create and database.create, the grants of their holders follow, and nothing else
// changes. TestAdoptingGrantsChangesNoDecision proves the same for grants from before roles.
func TestTheMigrationGivesWhoeverCreatesPagesPostsAndDatabases(t *testing.T) {
	w := newRoleWorld(t)
	ctx := w.ctx
	inst := ids.New()
	person := func(email string) uuid.UUID {
		id := ids.New()
		w.exec(t, `INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, 'member', 'active', $3, $3)`,
			id, inst, email)
		return id
	}
	grants := func(subject uuid.UUID) []string {
		t.Helper()
		var got []string
		if err := w.pool.QueryRow(ctx, `SELECT ARRAY(SELECT permission FROM space_grants WHERE space_id = $1 AND subject_id = $2 ORDER BY 1)`,
			w.space, subject).Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	without := func(perms []string) []string {
		return slices.DeleteFunc(slices.Clone(perms), func(p string) bool {
			return p == permissions.PostCreate || p == permissions.DatabaseCreate
		})
	}

	// The state before the migration: built-in roles, a role made by hand, an own set, and
	// someone who holds no page creation at all.
	for _, key := range []string{permissions.TemplateEditor, permissions.TemplateManager, permissions.TemplateSpaceAdmin} {
		w.exec(t, "UPDATE roles SET permissions = $2 WHERE builtin = $1", key, without(permissions.Templates[key]))
	}
	writer, reader := person("writer@example.com"), person("reader@example.com")
	w.exec(t, "SELECT rbac_set_exact($1, 'user', $2, ARRAY['space.view', 'page.create', 'page.edit'])", w.space, writer)
	w.exec(t, "SELECT rbac_set_exact($1, 'user', $2, ARRAY['space.view', 'comment.add'])", w.space, reader)
	w.exec(t, "INSERT INTO permission_sets (id, name, permissions) VALUES ($1, 'Author', ARRAY['space.view', 'page.create'])", ids.New())
	readerBefore := grants(reader)
	if slices.Contains(grants(w.manager.UserID), permissions.PostCreate) || slices.Contains(grants(writer), permissions.PostCreate) {
		t.Fatal("the state before the migration already holds post.create")
	}

	w.exec(t, "SELECT rbac_split_roles()")

	for _, who := range []uuid.UUID{writer, w.manager.UserID, w.owner.UserID} {
		got := grants(who)
		if !slices.Contains(got, permissions.PostCreate) || !slices.Contains(got, permissions.DatabaseCreate) {
			t.Fatalf("someone who created pages lost posts or databases in the migration: %v", got)
		}
	}
	if got := grants(reader); !slices.Equal(got, readerBefore) {
		t.Fatalf("someone who never created pages gained %v (had %v)", got, readerBefore)
	}
	var author []string
	if err := w.pool.QueryRow(ctx, "SELECT ARRAY(SELECT p FROM unnest(permissions) p ORDER BY 1) FROM permission_sets WHERE name = 'Author'").
		Scan(&author); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(author, permissions.PostCreate) || !slices.Contains(author, permissions.DatabaseCreate) {
		t.Fatalf("an own set with page creation was left without posts or databases: %v", author)
	}
	// The built-in roles mean again what their templates say.
	for _, key := range []string{permissions.TemplateEditor, permissions.TemplateManager, permissions.TemplateSpaceAdmin} {
		var got []string
		if err := w.pool.QueryRow(ctx, "SELECT ARRAY(SELECT p FROM unnest(permissions) p ORDER BY 1) FROM roles WHERE builtin = $1", key).
			Scan(&got); err != nil {
			t.Fatal(err)
		}
		if want := slices.Sorted(slices.Values(permissions.Templates[key])); !slices.Equal(got, want) {
			t.Fatalf("%s after the migration: %v, template %v", key, got, want)
		}
	}
	noDrift(t, ctx, w.pool)

	// A script on the older grants route writes lists from before the split.
	scripted := person("scripted@example.com")
	if err := w.svc.SetGrants(ctx, w.owner, w.space, permissions.Subject{Type: permissions.SubjectUser, ID: scripted},
		[]string{permissions.SpaceView, permissions.PageCreate}); err != nil {
		t.Fatal(err)
	}
	if got := grants(scripted); !slices.Contains(got, permissions.PostCreate) || !slices.Contains(got, permissions.DatabaseCreate) {
		t.Fatalf("an old list with page creation came in without posts or databases: %v", got)
	}
	noDrift(t, ctx, w.pool)
}

// TestTheCreatePermissionOfEachKind keeps the rule and the kinds of item in step.
func TestTheCreatePermissionOfEachKind(t *testing.T) {
	for kind, want := range map[string]string{
		"page": permissions.PageCreate, "folder": permissions.PageCreate, "link": permissions.PageCreate,
		"post": permissions.PostCreate, "database": permissions.DatabaseCreate,
	} {
		if got := permissions.CreatePermission(kind); got != want {
			t.Fatalf("%s is made with %s, want %s", kind, got, want)
		}
	}
}
