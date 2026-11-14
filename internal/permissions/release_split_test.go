// SPDX-License-Identifier: Apache-2.0

package permissions_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
)

// withoutRelease is a list of permissions as it stood before migration 0061.
func withoutRelease(perms []string) []string {
	return slices.DeleteFunc(slices.Clone(perms), func(p string) bool { return p == permissions.SpaceRelease })
}

// releaseWorld is a space with people whose roles were written before space.release existed.
type releaseWorld struct {
	roleWorld
	people map[string]uuid.UUID
	pages  []uuid.UUID
}

func newReleaseWorld(t *testing.T) releaseWorld {
	t.Helper()
	w := releaseWorld{roleWorld: newRoleWorld(t), people: map[string]uuid.UUID{}}
	inst := ids.New()
	w.exec(t, "UPDATE roles SET permissions = $2 WHERE builtin = $1", permissions.TemplateSpaceAdmin,
		withoutRelease(permissions.Templates[permissions.TemplateSpaceAdmin]))
	w.exec(t, "SELECT rbac_set_exact($1, 'user', $2, $3::text[])", w.space, w.owner.UserID,
		withoutRelease(permissions.Templates[permissions.TemplateSpaceAdmin]))
	shapes := map[string][]string{
		"writer":          {permissions.SpaceView, permissions.PageEdit, permissions.PageCreate},
		"reader":          {permissions.SpaceView, permissions.CommentAdd},
		"administer-only": {permissions.SpaceView, permissions.SpaceAdminister},
		"view-and-audit":  {permissions.SpaceView, permissions.SpaceAudit},
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
	for _, archived := range []bool{false, true} {
		id := ids.New()
		w.exec(t, `INSERT INTO pages (id, origin_instance_id, space_id, title, status, created_by, updated_by, kind, archived_at)
			VALUES ($1, $2, $3, 't', 'published', $4, $4, 'page', CASE WHEN $5::boolean THEN now() END)`, id, inst, w.space, w.owner.UserID, archived)
		w.exec(t, "INSERT INTO page_tree (ancestor_id, descendant_id, depth) VALUES ($1, $1, 0)", id)
		w.pages = append(w.pages, id)
	}
	return w
}

// TestSplittingOffReleaseChangesNoDecision is the proof of migration 0061 (PUB-1301): no
// decision of Evaluate or of the SQL lists changes, and space.release is held by exactly those
// who could administer the space.
func TestSplittingOffReleaseChangesNoDecision(t *testing.T) {
	w := newReleaseWorld(t)
	users := make([]uuid.UUID, 0, len(w.people))
	for _, id := range w.people {
		users = append(users, id)
	}
	spaces := []uuid.UUID{w.space}
	before := snapshot(t, w.ctx, w.pool, users, spaces, w.pages)

	w.exec(t, "SELECT rbac_split_roles_from('space.administer', ARRAY['space.release'])")

	after := snapshot(t, w.ctx, w.pool, users, spaces, w.pages)
	compared := 0
	for key, got := range after {
		if strings.Contains(key, " "+permissions.SpaceRelease) {
			broad := strings.Replace(key, " "+permissions.SpaceRelease, " "+permissions.SpaceAdminister, 1)
			if got != before[broad] {
				t.Errorf("%s is %v after the migration; %s was %v before", key, got, broad, before[broad])
			}
			compared++
			continue
		}
		if got != before[key] {
			t.Errorf("%s changed from %v to %v", key, before[key], got)
		}
		compared++
	}
	for key, was := range before {
		if _, ok := after[key]; !ok && was {
			t.Errorf("%s was true before and is gone", key)
		}
	}
	if compared < 200 {
		t.Fatalf("only %d decisions compared", compared)
	}
	noDrift(t, w.ctx, w.pool)

	var held []uuid.UUID
	rows, err := w.pool.Query(w.ctx, "SELECT subject_id FROM space_grants WHERE space_id = $1 AND permission = 'space.release' ORDER BY 1", w.space)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		held = append(held, id)
	}
	rows.Close()
	want := []uuid.UUID{w.people["administer-only"], w.people["owner"]}
	slices.SortFunc(want, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	if !slices.Equal(held, want) {
		t.Fatalf("space.release is held by %v, want the administrators %v", held, want)
	}
}

// TestTheBuiltInSpaceAdministratorReleases: the template of a space administrator holds
// space.release, and a role with administer from before the split gets it from an old list.
func TestTheBuiltInSpaceAdministratorReleases(t *testing.T) {
	if !slices.Contains(permissions.Templates[permissions.TemplateSpaceAdmin], permissions.SpaceRelease) {
		t.Fatal("the space administrator template lacks space.release")
	}
	if slices.Contains(permissions.Templates[permissions.TemplateEditor], permissions.SpaceRelease) ||
		slices.Contains(permissions.Templates[permissions.TemplateManager], permissions.SpaceRelease) {
		t.Fatal("an editor or a manager releases without being given it")
	}
	got := permissions.WithSplitOff([]string{permissions.SpaceView, permissions.SpaceAdminister, permissions.SpaceWorkflows})
	if !slices.Contains(got, permissions.SpaceRelease) {
		t.Fatalf("a list from before spec 073 came in without space.release: %v", got)
	}
}
