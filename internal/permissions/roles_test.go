// SPDX-License-Identifier: Apache-2.0

package permissions_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/testutil"
)

// snapshot is every decision this test can ask for: Evaluate on every page and space
// for every action, and what the SQL lists say, per user.
func snapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool, users, spaces, pages []uuid.UUID) map[string]bool {
	t.Helper()
	svc := permissions.NewService(pool)
	out := map[string]bool{}
	spaceFacts := map[uuid.UUID]permissions.SpaceFacts{}
	for _, sp := range spaces {
		f, err := svc.LoadSpace(ctx, pool, sp)
		if err != nil {
			t.Fatal(err)
		}
		spaceFacts[sp] = f
	}
	type loaded struct {
		space permissions.SpaceFacts
		page  permissions.PageFacts
	}
	pageFacts := map[uuid.UUID]loaded{}
	for _, p := range pages {
		sp, pf, err := svc.LoadPage(ctx, pool, p)
		if err != nil {
			t.Fatal(err)
		}
		pageFacts[p] = loaded{sp, pf}
	}
	spaceActions := append(slices.Clone(permissions.AllSpacePermissions), permissions.ActionPageView)
	for _, u := range users {
		facts, err := svc.LoadUser(ctx, pool, u)
		if err != nil {
			t.Fatal(err)
		}
		for _, sp := range spaces {
			for _, a := range spaceActions {
				d := permissions.Evaluate(permissions.Input{User: facts, Space: spaceFacts[sp], Action: a})
				out[fmt.Sprintf("evaluate space %s %s %s", u, sp, a)] = d.Allowed
			}
		}
		for _, p := range pages {
			l := pageFacts[p]
			for _, a := range actions {
				d := permissions.Evaluate(permissions.Input{User: facts, Space: l.space, Page: &l.page, Action: a, Owner: l.page.CreatedBy})
				out[fmt.Sprintf("evaluate page %s %s %s allowed", u, p, a)] = d.Allowed
				out[fmt.Sprintf("evaluate page %s %s %s visible", u, p, a)] = d.Visible
			}
		}
		for _, fn := range []string{"perm_visible_pages", "perm_listed_pages", "perm_editable_pages"} {
			rows, err := pool.Query(ctx, "SELECT page_id FROM "+fn+"($1)", u)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var id uuid.UUID
				if err := rows.Scan(&id); err != nil {
					t.Fatal(err)
				}
				out[fmt.Sprintf("%s %s %s", fn, u, id)] = true
			}
			rows.Close()
		}
		for _, p := range permissions.AllSpacePermissions {
			rows, err := pool.Query(ctx, "SELECT space_id FROM perm_spaces($1, $2)", u, p)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var id uuid.UUID
				if err := rows.Scan(&id); err != nil {
					t.Fatal(err)
				}
				out[fmt.Sprintf("perm_spaces %s %s %s", u, p, id)] = true
			}
			rows.Close()
		}
		for _, g := range identity.AllGlobalPermissions {
			var ok bool
			if err := pool.QueryRow(ctx, "SELECT perm_has_global($1, $2)", u, g).Scan(&ok); err != nil {
				t.Fatal(err)
			}
			out[fmt.Sprintf("perm_has_global %s %s", u, g)] = ok
		}
	}
	return out
}

func noDrift(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM rbac_drift()").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d grants differ from what roles add up to", n)
	}
}

// TestAdoptingGrantsChangesNoDecision is the proof of migration 0036: single grants of
// every shape, on users, groups, all members and all guests, beside restrictions and
// drafts, become roles, and not one decision of Evaluate or the SQL lists changes.
func TestAdoptingGrantsChangesNoDecision(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	pool := tdb.Pool
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	inst := ids.New()
	grp := ids.New()
	exec("INSERT INTO groups (id, origin_instance_id, name, source) VALUES ($1, $2, 'g', 'local')", grp, inst)
	var users []uuid.UUID
	for i, u := range userCases {
		id := ids.New()
		status := "active"
		if !u.active {
			status = "disabled"
		}
		exec(`INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, $3, $4, $5, $5)`,
			id, inst, u.typ, status, fmt.Sprintf("u%d@example.com", i))
		if u.expires == "past" {
			exec("UPDATE users SET expires_at = now() - interval '1 hour' WHERE id = $1", id)
		}
		// Single global grants in several shapes, written straight into the table as
		// before roles existed.
		globals := []string{}
		if u.product {
			globals = append(globals, "use_product")
		}
		switch i % 4 {
		case 1:
			globals = append(globals, "view_profiles")
		case 2:
			globals = append(globals, "view_profiles", "create_space")
		case 3:
			globals = append(globals, "administrator")
		}
		for _, g := range globals {
			exec("INSERT INTO global_grants (subject_type, subject_id, permission) VALUES ('user', $1, $2)", id, g)
		}
		if u.inGroup {
			exec("INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)", grp, id)
		}
		users = append(users, id)
	}
	exec("INSERT INTO global_grants (subject_type, subject_id, permission) VALUES ('group', $1, 'view_profiles')", grp)
	exec("INSERT INTO permission_sets (id, name, permissions) VALUES ($1, 'Reviewer', $2)", ids.New(),
		[]string{permissions.SpaceView, permissions.CommentAdd, permissions.AnalyticsView})
	exec("INSERT INTO permission_sets (id, name, permissions) VALUES ($1, 'Keeper', $2)", ids.New(),
		[]string{permissions.SpaceView, permissions.SpaceAdminister})

	other := users[0]
	addPage := func(space uuid.UUID, parent *uuid.UUID, status string, by uuid.UUID) uuid.UUID {
		id := ids.New()
		exec(`INSERT INTO pages (id, origin_instance_id, space_id, parent_id, title, status, created_by, updated_by, kind)
			VALUES ($1, $2, $3, $4, 't', $5, $6, $6, 'page')`, id, inst, space, parent, status, by)
		exec("INSERT INTO page_tree (ancestor_id, descendant_id, depth) VALUES ($1, $1, 0)", id)
		if parent != nil {
			exec(`INSERT INTO page_tree (ancestor_id, descendant_id, depth)
				SELECT ancestor_id, $2, depth + 1 FROM page_tree WHERE descendant_id = $1`, *parent, id)
		}
		return id
	}
	// Every grant case, a few spaces where several subjects add up, and an archived copy.
	// Grants from before roles know nothing of the permissions spec 036 split off.
	beforeSplit := func(perms []string) []string {
		return slices.DeleteFunc(slices.Clone(perms), func(p string) bool { _, ok := permissions.SplitFrom[p]; return ok })
	}
	adminBeforeSplit := beforeSplit(adminPerms)
	cases := []grantCase{
		{name: "reviewer-set", subject: "user", perms: []string{permissions.SpaceView, permissions.CommentAdd, permissions.AnalyticsView}},
		{name: "view-and-analytics", subject: "group", perms: []string{permissions.SpaceView, permissions.AnalyticsView}},
		{name: "group-admin", subject: "group", perms: adminBeforeSplit},
		{name: "members-admin", subject: "all_members", perms: adminBeforeSplit},
		{name: "guests-admin", subject: "all_guests", perms: adminBeforeSplit},
		{name: "manage-access-only", subject: "user", perms: []string{permissions.SpaceView, permissions.SpaceManageAccess}},
	}
	// The extra cases of spec 040 test the rule, not the move to roles.
	for _, g := range grantCases {
		if !g.extra {
			cases = append(cases, grantCase{name: g.name, subject: g.subject, perms: beforeSplit(g.perms)})
		}
	}
	var spaces, pages []uuid.UUID
	for n, g := range cases {
		for _, status := range []string{"active", "archived"} {
			space := ids.New()
			exec(`INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by) VALUES ($1, $2, $3, 's', $4, $5)`,
				space, inst, space.String(), status, other)
			target := users[n%len(users)]
			grant := func(subject string, perms []string) {
				sub := uuid.Nil
				switch subject {
				case "user":
					sub = target
				case "group":
					sub = grp
				case "none":
					return
				}
				for _, p := range perms {
					exec(`INSERT INTO space_grants (space_id, subject_type, subject_id, permission) VALUES ($1, $2, $3, $4)
						ON CONFLICT DO NOTHING`, space, subject, sub, p)
				}
			}
			grant(g.subject, g.perms)
			if n%3 == 0 {
				grant("all_members", viewerPerms)
				grant("group", []string{permissions.SpaceView, permissions.PageEdit})
			}
			spaces = append(spaces, space)
			for _, r := range restrictionCases {
				if r.extra {
					continue
				}
				for _, state := range []string{"published", "draft"} {
					creator := other
					if state == "draft" && n%2 == 0 {
						creator = target
					}
					root := addPage(space, nil, "published", other)
					leaf := addPage(space, &root, state, creator)
					restrict := func(page uuid.UUID, kind, who string) {
						if who == "" {
							return
						}
						typ, id := "user", other
						switch who {
						case "user":
							id = target
						case "group":
							typ, id = "group", grp
						}
						exec("INSERT INTO page_restrictions (page_id, kind, subject_type, subject_id) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING",
							page, kind, typ, id)
					}
					restrict(root, "view", r.viewRoot)
					restrict(leaf, "view", r.viewLeaf)
					restrict(leaf, "edit", r.editLeaf)
					pages = append(pages, leaf)
				}
			}
		}
	}

	before := snapshot(t, ctx, pool, users, spaces, pages)
	exec("SELECT rbac_adopt()")
	after := snapshot(t, ctx, pool, users, spaces, pages)

	// A permission split off a broader one decides after adoption as the broader one did
	// before, or as it did itself where someone already held it.
	expected := maps.Clone(before)
	split := 0
	for k, v := range before {
		if !v {
			continue
		}
		fields := strings.Fields(k)
		for finer, broad := range permissions.SplitFrom {
			i := slices.Index(fields, broad)
			if i < 0 {
				continue
			}
			swapped := slices.Clone(fields)
			swapped[i] = finer
			key := strings.Join(swapped, " ")
			if !expected[key] {
				expected[key] = true
				split++
			}
		}
	}
	allowed := 0
	for k, v := range expected {
		if after[k] != v {
			t.Fatalf("%s changed from %v to %v", k, v, after[k])
		}
		if v {
			allowed++
		}
	}
	for k, v := range after {
		if v && !expected[k] {
			t.Fatalf("%s appeared after adoption", k)
		}
	}
	if len(before) < 50_000 || allowed < 5_000 || split < 500 {
		t.Fatalf("too small to prove anything: %d decisions, %d allowed, %d through a split", len(before), allowed, split)
	}
	t.Logf("%d decisions compared, %d of them allowed, %d of those through a permission split off a broader one",
		len(expected), allowed, split)
	noDrift(t, ctx, pool)

	var orphans int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM space_grants g WHERE NOT EXISTS (SELECT 1 FROM role_assignments a
		WHERE a.space_id = g.space_id AND a.subject_type = g.subject_type AND a.subject_id = g.subject_id)`).Scan(&orphans); err != nil {
		t.Fatal(err)
	}
	if orphans != 0 {
		t.Fatalf("%d grants have no role behind them", orphans)
	}
	var reviewer bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM roles r JOIN role_assignments a ON a.role_id = r.id
		WHERE r.name = 'Reviewer')`).Scan(&reviewer); err != nil {
		t.Fatal(err)
	}
	if !reviewer {
		t.Fatal("a set an administrator made did not become a role under its own name")
	}
	var keeper []string
	if err := pool.QueryRow(ctx, "SELECT permissions FROM roles WHERE name = 'Keeper'").Scan(&keeper); err != nil {
		t.Fatal(err)
	}
	for finer, broad := range permissions.SplitFrom {
		if broad == permissions.SpaceAdminister && !slices.Contains(keeper, finer) {
			t.Fatalf("a set with space administration became a role without %s: %v", finer, keeper)
		}
	}
}

// TestBuiltInRolesMatchTheTemplates keeps the built-in roles and the Go templates one
// meaning: a viewer is a viewer in both.
func TestBuiltInRolesMatchTheTemplates(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	for name, perms := range permissions.Templates {
		var got []string
		if err := tdb.Pool.QueryRow(ctx, "SELECT ARRAY(SELECT p FROM unnest(permissions) p ORDER BY 1) FROM roles WHERE builtin = $1",
			name).Scan(&got); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := slices.Sorted(slices.Values(perms))
		if !slices.Equal(got, want) {
			t.Fatalf("%s: role %v, template %v", name, got, want)
		}
	}
}

type roleWorld struct {
	ctx                   context.Context
	pool                  *pgxpool.Pool
	svc                   *permissions.Service
	admin, sysadmin       identity.Caller
	manager, plain, owner identity.Caller
	space                 uuid.UUID
}

func newRoleWorld(t *testing.T) roleWorld {
	t.Helper()
	tdb := testutil.Database(t)
	w := roleWorld{ctx: context.Background(), pool: tdb.Pool, svc: permissions.NewService(tdb.Pool)}
	inst := ids.New()
	mk := func(name string, globals ...string) identity.Caller {
		id := ids.New()
		w.exec(t, `INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, 'member', 'active', $3, $3)`,
			id, inst, name+"@example.com")
		w.exec(t, "SELECT rbac_set_exact(NULL, 'user', $1, $2::text[])", id, append([]string{"use_product"}, globals...))
		return identity.Caller{UserID: id}
	}
	w.admin = mk("admin", identity.PermAdministrator)
	w.sysadmin = mk("sysadmin", identity.PermAdministrator, identity.PermSystemAdministrator)
	w.manager = mk("manager")
	w.plain = mk("plain")
	w.owner = mk("owner")
	w.space = ids.New()
	w.exec(t, `INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by) VALUES ($1, $2, 'R', 'r', 'active', $3)`,
		w.space, inst, w.owner.UserID)
	w.exec(t, "SELECT rbac_set_exact($1, 'user', $2, $3::text[])", w.space, w.owner.UserID, permissions.Templates[permissions.TemplateSpaceAdmin])
	w.exec(t, "SELECT rbac_set_exact($1, 'user', $2, $3::text[])", w.space, w.manager.UserID, permissions.Templates[permissions.TemplateManager])
	return w
}

func (w roleWorld) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := w.pool.Exec(w.ctx, sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func (w roleWorld) builtin(t *testing.T, key string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := w.pool.QueryRow(w.ctx, "SELECT id FROM roles WHERE builtin = $1", key).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func wantConflict(t *testing.T, err error, code string) {
	t.Helper()
	var e *apperr.Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("want conflict %q, got %v", code, err)
	}
}

func wantKind(t *testing.T, err error, kind apperr.Kind) {
	t.Helper()
	var e *apperr.Error
	if !errors.As(err, &e) || e.Kind != kind {
		t.Fatalf("want %v, got %v", kind, err)
	}
}

// TestRolesRefuseWhatWouldChangeAccessQuietly covers the failure paths of making and
// changing roles first.
func TestRolesRefuseWhatWouldChangeAccessQuietly(t *testing.T) {
	w := newRoleWorld(t)
	ctx := w.ctx
	in := permissions.RoleInput{Name: "Writer", Permissions: []string{permissions.SpaceView, permissions.PageEdit}}

	_, err := w.svc.CreateRole(ctx, w.plain, permissions.ScopeSpace, in)
	wantKind(t, err, apperr.KindForbidden)
	_, err = w.svc.CreateRole(ctx, w.admin, permissions.ScopeSpace, permissions.RoleInput{Name: "Bad", Permissions: []string{"administrator"}})
	wantKind(t, err, apperr.KindInvalid)
	_, err = w.svc.CreateRole(ctx, w.admin, permissions.ScopeSpace, permissions.RoleInput{Name: "Empty"})
	wantKind(t, err, apperr.KindInvalid)
	_, err = w.svc.CreateRole(ctx, w.admin, permissions.ScopeInstance, permissions.RoleInput{Name: "Ops", Permissions: []string{identity.PermSystemAdministrator}})
	wantConflict(t, err, "needs_system_administrator")

	viewer := w.builtin(t, permissions.TemplateViewer)
	_, err = w.svc.UpdateRole(ctx, w.admin, viewer, permissions.RoleInput{Name: "Viewer", Permissions: []string{permissions.SpaceView}}, true)
	wantConflict(t, err, "built_in")
	wantConflict(t, w.svc.DeleteRole(ctx, w.admin, viewer), "built_in")

	role, err := w.svc.CreateRole(ctx, w.admin, permissions.ScopeSpace, in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = w.svc.CreateRole(ctx, w.admin, permissions.ScopeSpace, permissions.RoleInput{Name: "writer", Permissions: []string{permissions.SpaceView}})
	wantConflict(t, err, "name_taken")
	if err := w.svc.SetSpaceRoles(ctx, w.owner, w.space, permissions.Subject{Type: permissions.SubjectUser, ID: w.plain.UserID}, []uuid.UUID{role.ID}); err != nil {
		t.Fatal(err)
	}
	wantConflict(t, w.svc.DeleteRole(ctx, w.admin, role.ID), "role_in_use")
	wider := permissions.RoleInput{Name: "Writer", Permissions: []string{permissions.SpaceView, permissions.PageEdit, permissions.PageCreate}}
	_, err = w.svc.UpdateRole(ctx, w.admin, role.ID, wider, false)
	wantConflict(t, err, "confirm_needed")
	_, err = w.svc.UpdateRole(ctx, w.admin, role.ID, permissions.RoleInput{Name: "Writer",
		Permissions: []string{permissions.SpaceView, permissions.SpaceAdminister}}, true)
	wantConflict(t, err, "administer_in_use")

	impact, err := w.svc.PreviewRole(ctx, w.admin, role.ID, wider.Permissions)
	if err != nil {
		t.Fatal(err)
	}
	if impact.Assignments != 1 || impact.People != 1 || !slices.Equal(impact.Gains, []string{permissions.PageCreate}) {
		t.Fatalf("preview: %+v", impact)
	}
	if _, err := w.svc.UpdateRole(ctx, w.admin, role.ID, wider, true); err != nil {
		t.Fatal(err)
	}
	d, err := w.svc.CheckSpace(ctx, w.pool, w.plain.UserID, w.space, permissions.PageCreate)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Allowed {
		t.Fatal("a confirmed change to a role did not reach the person who holds it")
	}
	noDrift(t, ctx, w.pool)

	var details string
	if err := w.pool.QueryRow(ctx, `SELECT payload FROM audit_events WHERE action = 'role.changed' ORDER BY seq DESC LIMIT 1`).Scan(&details); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"before"`, `"after"`, permissions.PageCreate} {
		if !strings.Contains(details, want) {
			t.Fatalf("audit of a role change misses %s: %s", want, details)
		}
	}
}

// TestAssigningRolesKeepsTheLimitsOfManagingAccess: roles change how access is given,
// not who may give it.
func TestAssigningRolesKeepsTheLimitsOfManagingAccess(t *testing.T) {
	w := newRoleWorld(t)
	ctx := w.ctx
	plain := permissions.Subject{Type: permissions.SubjectUser, ID: w.plain.UserID}
	owner := permissions.Subject{Type: permissions.SubjectUser, ID: w.owner.UserID}
	admin := w.builtin(t, permissions.TemplateSpaceAdmin)
	editor := w.builtin(t, permissions.TemplateEditor)

	wantKind(t, w.svc.SetSpaceRoles(ctx, w.plain, w.space, plain, []uuid.UUID{editor}), apperr.KindNotFound)
	wantConflict(t, w.svc.SetSpaceRoles(ctx, w.manager, w.space, plain, []uuid.UUID{admin}), "needs_administer")
	wantConflict(t, w.svc.SetSpaceRoles(ctx, w.manager, w.space, owner, []uuid.UUID{editor}), "needs_administer")
	wantConflict(t, w.svc.SetSpaceRoles(ctx, w.owner, w.space, owner, []uuid.UUID{editor}), "last_admin")
	wantKind(t, w.svc.SetSpaceRoles(ctx, w.owner, w.space, plain, []uuid.UUID{ids.New()}), apperr.KindInvalid)
	wantKind(t, w.svc.SetSpaceRoles(ctx, w.owner, w.space, permissions.Subject{Type: permissions.SubjectUser, ID: ids.New()},
		[]uuid.UUID{editor}), apperr.KindInvalid)

	instance, err := w.svc.CreateRole(ctx, w.admin, permissions.ScopeInstance, permissions.RoleInput{Name: "Directory",
		Permissions: []string{"use_product", "view_profiles"}})
	if err != nil {
		t.Fatal(err)
	}
	wantKind(t, w.svc.SetSpaceRoles(ctx, w.owner, w.space, plain, []uuid.UUID{instance.ID}), apperr.KindInvalid)
	wantKind(t, w.svc.SetInstanceRoles(ctx, w.plain, plain, []uuid.UUID{instance.ID}), apperr.KindForbidden)

	if err := w.svc.SetSpaceRoles(ctx, w.manager, w.space, plain, []uuid.UUID{editor}); err != nil {
		t.Fatal(err)
	}
	d, err := w.svc.CheckSpace(ctx, w.pool, w.plain.UserID, w.space, permissions.PageEdit)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Allowed {
		t.Fatal("a role given in a space did not give its permissions")
	}
	if err := w.svc.SetSpaceRoles(ctx, w.manager, w.space, plain, nil); err != nil {
		t.Fatal(err)
	}
	d, err = w.svc.CheckSpace(ctx, w.pool, w.plain.UserID, w.space, permissions.SpaceView)
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed {
		t.Fatal("taking every role away left access behind")
	}
	noDrift(t, ctx, w.pool)

	var details string
	if err := w.pool.QueryRow(ctx, `SELECT payload FROM audit_events WHERE action = 'role.assigned' ORDER BY seq DESC LIMIT 1`).Scan(&details); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"before"`, `"after"`, "Editor"} {
		if !strings.Contains(details, want) {
			t.Fatalf("audit of an assignment misses %s: %s", want, details)
		}
	}

	// The last administrator of the instance cannot lose the role that makes them one.
	w.exec(t, "SELECT rbac_set_exact(NULL, 'user', $1, ARRAY['use_product'])", w.sysadmin.UserID)
	wantConflict(t, w.svc.SetInstanceRoles(ctx, w.admin, permissions.Subject{Type: permissions.SubjectUser, ID: w.admin.UserID},
		[]uuid.UUID{instance.ID}), "last_administrator")
}

// TestSettingSinglePermissionsMakesOneRolePerSet: the old way of ticking permissions
// keeps working, and the same set shares one role instead of making many.
func TestSettingSinglePermissionsMakesOneRolePerSet(t *testing.T) {
	w := newRoleWorld(t)
	ctx := w.ctx
	odd := []string{permissions.SpaceView, permissions.AnalyticsView}
	for _, sub := range []permissions.Subject{
		{Type: permissions.SubjectUser, ID: w.plain.UserID},
		{Type: permissions.SubjectAllMembers},
	} {
		if err := w.svc.SetGrants(ctx, w.owner, w.space, sub, odd); err != nil {
			t.Fatal(err)
		}
	}
	var roles int
	if err := w.pool.QueryRow(ctx, `SELECT count(DISTINCT role_id) FROM role_assignments WHERE space_id = $1
		AND role_id NOT IN (SELECT id FROM roles WHERE builtin IS NOT NULL)`, w.space).Scan(&roles); err != nil {
		t.Fatal(err)
	}
	if roles != 1 {
		t.Fatalf("one set made %d roles", roles)
	}
	if err := w.svc.SetGrants(ctx, w.owner, w.space, permissions.Subject{Type: permissions.SubjectUser, ID: w.plain.UserID},
		permissions.Templates[permissions.TemplateViewer]); err != nil {
		t.Fatal(err)
	}
	var builtin string
	if err := w.pool.QueryRow(ctx, `SELECT r.builtin FROM role_assignments a JOIN roles r ON r.id = a.role_id
		WHERE a.space_id = $1 AND a.subject_id = $2`, w.space, w.plain.UserID).Scan(&builtin); err != nil {
		t.Fatal(err)
	}
	if builtin != permissions.TemplateViewer {
		t.Fatalf("the viewer set became %q, not the built-in viewer", builtin)
	}
	noDrift(t, ctx, w.pool)
}

// TestHoldersOfARoleShowOnlyToAdministrators: the list of who holds a role says where,
// and nobody else may read it.
func TestHoldersOfARoleShowOnlyToAdministrators(t *testing.T) {
	w := newRoleWorld(t)
	ctx := w.ctx
	manager := w.builtin(t, permissions.TemplateManager)

	_, _, err := w.svc.RoleHolders(ctx, w.plain, manager)
	wantKind(t, err, apperr.KindForbidden)
	_, _, err = w.svc.RoleHolders(ctx, w.manager, manager)
	wantKind(t, err, apperr.KindForbidden)
	_, _, err = w.svc.RoleHolders(ctx, w.admin, ids.New())
	wantKind(t, err, apperr.KindNotFound)

	list, total, err := w.svc.RoleHolders(ctx, w.admin, manager)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 || list[0].Subject.ID != w.manager.UserID || list[0].SpaceID == nil || *list[0].SpaceID != w.space ||
		list[0].Name != "manager@example.com" || list[0].SpaceName != "r" {
		t.Fatalf("holders of the manager role: %d %+v", total, list)
	}
}

// TestTheSplitIsTheSameInGoAndSQL: migration 0040 hands out finer permissions by
// rbac_split_off, and Evaluate reads SplitFrom. A pair known to one side only would move
// access in an upgrade.
func TestTheSplitIsTheSameInGoAndSQL(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	broad := map[string][]string{}
	for finer, b := range permissions.SplitFrom {
		broad[b] = append(broad[b], finer)
	}
	for _, p := range permissions.AllSpacePermissions {
		var got []string
		if err := tdb.Pool.QueryRow(ctx, "SELECT ARRAY(SELECT x FROM unnest(rbac_split_off($1)) x ORDER BY 1)", p).Scan(&got); err != nil {
			t.Fatal(err)
		}
		want := slices.Sorted(slices.Values(broad[p]))
		if !slices.Equal(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Fatalf("%s: SQL splits off %v, Go %v", p, got, want)
		}
	}
}

// TestEveryWayToAdministerASpaceGivesTheFinerPermissions: nobody who is given space
// administration, or editing, or managing access, in any way the product still offers,
// ends up without what was split off it. Roles made by hand are the one way to hold the
// broader permission alone, and that is what they are for.
func TestEveryWayToAdministerASpaceGivesTheFinerPermissions(t *testing.T) {
	w := newRoleWorld(t)
	ctx := w.ctx
	finerOf := func(broad string) []string {
		var out []string
		for f, b := range permissions.SplitFrom {
			if b == broad {
				out = append(out, f)
			}
		}
		return out
	}
	holds := func(why string, subject uuid.UUID, want []string) {
		t.Helper()
		var got []string
		if err := w.pool.QueryRow(ctx, `SELECT ARRAY(SELECT permission FROM space_grants WHERE space_id = $1 AND subject_id = $2)`,
			w.space, subject).Scan(&got); err != nil {
			t.Fatal(err)
		}
		for _, p := range want {
			if !slices.Contains(got, p) {
				t.Fatalf("%s: %s is missing from %v", why, p, got)
			}
		}
	}

	// The built-in Space admin role, given at creation, handover and bundle import.
	admin := permissions.Templates[permissions.TemplateSpaceAdmin]
	for _, f := range finerOf(permissions.SpaceAdminister) {
		if !slices.Contains(admin, f) {
			t.Fatalf("the Space admin template lacks %s", f)
		}
	}
	holds("the owner, who got Space admin", w.owner.UserID, finerOf(permissions.SpaceAdminister))

	// The older grants route, with a list written before the splits.
	plain := permissions.Subject{Type: permissions.SubjectUser, ID: w.plain.UserID}
	if err := w.svc.SetGrants(ctx, w.owner, w.space, plain, []string{permissions.SpaceView, permissions.SpaceAdminister,
		permissions.PageEdit, permissions.SpaceManageAccess}); err != nil {
		t.Fatal(err)
	}
	holds("an old list on the grants route", w.plain.UserID, append(append(finerOf(permissions.SpaceAdminister),
		finerOf(permissions.PageEdit)...), finerOf(permissions.SpaceManageAccess)...))
	noDrift(t, ctx, w.pool)

	// The migration, on a role that held administration before it.
	other := ids.New()
	w.exec(t, `INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, 'member', 'active', 'old@example.com', 'o')`,
		other, ids.New())
	w.exec(t, "UPDATE roles SET permissions = ARRAY['space.view', 'space.administer'] WHERE builtin = $1", permissions.TemplateSpaceAdmin)
	w.exec(t, "SELECT rbac_set_exact($1, 'user', $2, ARRAY['space.view', 'space.administer'])", w.space, other)
	w.exec(t, "SELECT rbac_split_roles()")
	holds("a holder of a role from before the migration", other, finerOf(permissions.SpaceAdminister))
	holds("the built-in role after the migration", w.owner.UserID, finerOf(permissions.SpaceAdminister))
	noDrift(t, ctx, w.pool)
}
