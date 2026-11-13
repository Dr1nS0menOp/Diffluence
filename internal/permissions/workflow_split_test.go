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

// withoutWorkflows is a list of permissions as it stood before migration 0051.
func withoutWorkflows(perms []string) []string {
	return slices.DeleteFunc(slices.Clone(perms), func(p string) bool {
		return p == permissions.SpaceWorkflows || p == permissions.PageWorkflow
	})
}

// workflowWorld is a space with pages of every shape that matters to the two permissions of
// spec 064, and people who hold roles written as before migration 0051.
type workflowWorld struct {
	roleWorld
	people map[string]uuid.UUID
	pages  []uuid.UUID
}

func newWorkflowWorld(t *testing.T) workflowWorld {
	t.Helper()
	w := workflowWorld{roleWorld: newRoleWorld(t), people: map[string]uuid.UUID{}}
	inst := ids.New()
	for _, key := range []string{permissions.TemplateEditor, permissions.TemplateManager, permissions.TemplateSpaceAdmin} {
		w.exec(t, "UPDATE roles SET permissions = $2 WHERE builtin = $1", key, withoutWorkflows(permissions.Templates[key]))
	}
	w.exec(t, "SELECT rbac_set_exact($1, 'user', $2, $3::text[])", w.space, w.owner.UserID,
		withoutWorkflows(permissions.Templates[permissions.TemplateSpaceAdmin]))
	w.exec(t, "SELECT rbac_set_exact($1, 'user', $2, $3::text[])", w.space, w.manager.UserID,
		withoutWorkflows(permissions.Templates[permissions.TemplateManager]))
	shapes := map[string][]string{
		"writer":          {permissions.SpaceView, permissions.PageEdit},
		"reader":          {permissions.SpaceView, permissions.CommentAdd},
		"editor":          withoutWorkflows(permissions.Templates[permissions.TemplateEditor]),
		"administer-only": {permissions.SpaceView, permissions.SpaceAdminister},
		"view-and-labels": {permissions.SpaceView, permissions.SpaceLabels},
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
	addPage := func(archived bool) uuid.UUID {
		id := ids.New()
		w.exec(t, `INSERT INTO pages (id, origin_instance_id, space_id, title, status, created_by, updated_by, kind, archived_at)
			VALUES ($1, $2, $3, 't', 'published', $4, $4, 'page', CASE WHEN $5::boolean THEN now() END)`, id, inst, w.space, w.owner.UserID, archived)
		w.exec(t, "INSERT INTO page_tree (ancestor_id, descendant_id, depth) VALUES ($1, $1, 0)", id)
		w.pages = append(w.pages, id)
		return id
	}
	addPage(false)
	restricted := addPage(false)
	w.exec(t, "INSERT INTO page_restrictions (page_id, kind, subject_type, subject_id) VALUES ($1, 'edit', 'user', $2)",
		restricted, w.people["writer"])
	addPage(true)
	controlled := addPage(false)
	workflow := ids.New()
	w.exec(t, `INSERT INTO workflows (id, space_id, name, created_by) VALUES ($1, $2, 'w', $3)`, workflow, w.space, w.owner.UserID)
	w.exec(t, `INSERT INTO page_control (page_id, workflow_id, workflow_version, state_id, state_kind, governed_by)
		VALUES ($1, $2, 1, 'draft', 'draft', 'page')`, controlled, workflow)
	return w
}

func (w workflowWorld) splitOff(t *testing.T) {
	t.Helper()
	w.exec(t, "SELECT rbac_split_roles_from('space.administer', ARRAY['space.workflows'])")
	w.exec(t, "SELECT rbac_split_roles_from('page.edit', ARRAY['page.workflow'])")
}

// TestSplittingOffWorkflowsChangesNoDecision is the proof of migration 0051: no decision of
// Evaluate or of the SQL lists about anything that existed before changes, and the two new
// permissions are held by exactly those who held the broader ones: page.workflow by whoever
// could edit the page, space.workflows by whoever could administer the space.
func TestSplittingOffWorkflowsChangesNoDecision(t *testing.T) {
	w := newWorkflowWorld(t)
	users := make([]uuid.UUID, 0, len(w.people))
	for _, id := range w.people {
		users = append(users, id)
	}
	spaces := []uuid.UUID{w.space}
	before := snapshot(t, w.ctx, w.pool, users, spaces, w.pages)

	w.splitOff(t)

	after := snapshot(t, w.ctx, w.pool, users, spaces, w.pages)
	isNew := func(key string) (string, bool) {
		switch {
		case strings.Contains(key, " "+permissions.PageWorkflow):
			return strings.Replace(key, " "+permissions.PageWorkflow, " "+permissions.PageEdit, 1), true
		case strings.Contains(key, " "+permissions.SpaceWorkflows):
			return strings.Replace(key, " "+permissions.SpaceWorkflows, " "+permissions.SpaceAdminister, 1), true
		}
		return "", false
	}
	compared := 0
	for key, got := range after {
		if broad, ok := isNew(key); ok {
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
	if compared < 1000 {
		t.Fatalf("only %d decisions compared", compared)
	}
	t.Logf("%d decisions compared", compared)
	noDrift(t, w.ctx, w.pool)
}

// TestTheMigrationAddsOnlyTheWorkflowPermissions: roles and own sets that hold page.edit gain
// page.workflow, those that hold space.administer gain space.workflows, and nothing else
// changes; the built-in roles mean again what their templates say, and a script on the older
// grants route that writes a list from before the split gets both.
func TestTheMigrationAddsOnlyTheWorkflowPermissions(t *testing.T) {
	w := newWorkflowWorld(t)
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

	w.splitOff(t)

	for name, who := range w.people {
		want := slices.Clone(before[name])
		if slices.Contains(want, permissions.PageEdit) {
			want = append(want, permissions.PageWorkflow)
		}
		if slices.Contains(want, permissions.SpaceAdminister) {
			want = append(want, permissions.SpaceWorkflows)
		}
		slices.Sort(want)
		if got := grants(who); !slices.Equal(got, want) {
			t.Errorf("%s holds %v after the migration, want %v", name, got, want)
		}
	}
	var set []string
	if err := w.pool.QueryRow(w.ctx, "SELECT ARRAY(SELECT p FROM unnest(permissions) p ORDER BY 1) FROM permission_sets WHERE name = 'Writer'").
		Scan(&set); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(set, permissions.PageWorkflow) {
		t.Fatalf("an own set with page.edit was left without page.workflow: %v", set)
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

	scripted := w.people["reader"]
	if err := w.svc.SetGrants(w.ctx, w.owner, w.space, permissions.Subject{Type: permissions.SubjectUser, ID: scripted},
		[]string{permissions.SpaceView, permissions.PageEdit, permissions.SpaceAdminister}); err != nil {
		t.Fatal(err)
	}
	got := grants(scripted)
	if !slices.Contains(got, permissions.PageWorkflow) || !slices.Contains(got, permissions.SpaceWorkflows) {
		t.Fatalf("an old list with page.edit and space.administer came in without the workflow permissions: %v", got)
	}
}

// TestSettingAWorkflowIsApartFromEditing: after the split a role can edit without setting
// workflows, and an edit restriction on a page also stops setting its workflow.
func TestSettingAWorkflowIsApartFromEditing(t *testing.T) {
	w := newWorkflowWorld(t)
	w.splitOff(t)
	writer := w.people["writer"]
	may := func(who, page uuid.UUID, action string) bool {
		t.Helper()
		facts, err := w.svc.LoadUser(w.ctx, w.pool, who)
		if err != nil {
			t.Fatal(err)
		}
		space, p, err := w.svc.LoadPage(w.ctx, w.pool, page)
		if err != nil {
			t.Fatal(err)
		}
		return permissions.Evaluate(permissions.Input{User: facts, Space: space, Page: &p, Action: action}).Allowed
	}
	open := w.pages[0]
	if !may(writer, open, permissions.PageWorkflow) {
		t.Fatal("after the migration a writer sets workflows as before")
	}
	w.exec(t, "SELECT rbac_set_exact($1, 'user', $2, ARRAY['space.view', 'page.edit'])", w.space, writer)
	if !may(writer, open, permissions.PageEdit) || may(writer, open, permissions.PageWorkflow) {
		t.Fatal("a role with page.edit alone edits and sets no workflow")
	}
	editorOnRestricted := w.people["editor"]
	if may(editorOnRestricted, w.pages[1], permissions.PageWorkflow) {
		t.Fatal("an edit restriction did not stop setting a workflow")
	}
}
