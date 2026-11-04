// SPDX-License-Identifier: Apache-2.0

package permissions_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/testutil"
)

// The generated matrix from blueprint 05: user type × group membership × space
// grant × restriction position × action. Three independent answers are compared:
//
//  1. literalRule below, a direct transcription of the rule in spec 002
//  2. permissions.Evaluate, what the application uses for single decisions
//  3. the SQL functions, what lists, search and exports use
//
// Every bug found later must add a case here.

type userCase struct {
	name    string
	typ     string
	active  bool
	product bool // holds use_product
	inGroup bool
}

var userCases = []userCase{
	{"member", "member", true, true, false},
	{"member-in-group", "member", true, true, true},
	{"guest", "guest", true, true, false},
	{"guest-in-group", "guest", true, true, true},
	{"disabled-member", "member", false, true, true},
	{"member-no-product", "member", true, false, true},
}

// grantCase says which subject holds which permissions in the space.
type grantCase struct {
	name    string
	subject string // user, group, all_members, all_guests, none
	perms   []string
}

var viewerPerms = []string{permissions.SpaceView, permissions.SpaceExport}
var editorPerms = []string{permissions.SpaceView, permissions.PageCreate, permissions.PageEdit, permissions.PageArchive,
	permissions.PageDeleteOwn, permissions.AttachmentAdd, permissions.AttachmentDeleteOwn}
var adminPerms = permissions.AllSpacePermissions

var grantCases = []grantCase{
	{"none", "none", nil},
	{"user-viewer", "user", viewerPerms},
	{"user-editor", "user", editorPerms},
	{"user-admin", "user", adminPerms},
	{"group-viewer", "group", viewerPerms},
	{"group-editor", "group", editorPerms},
	{"members-viewer", "all_members", viewerPerms},
	{"members-editor", "all_members", editorPerms},
	{"guests-viewer", "all_guests", viewerPerms},
	{"guests-editor", "all_guests", editorPerms},
	{"edit-without-view", "user", []string{permissions.PageEdit}},
}

// restrictionCase places restrictions on the chain root → mid → leaf; the page
// under test is the leaf. Each field lists who is on that restriction.
type restrictionCase struct {
	name                string
	viewRoot, viewLeaf  string // "", user, group, other
	editLeaf, editOnMid string
}

var restrictionCases = []restrictionCase{
	{name: "unrestricted"},
	{name: "view-root-user", viewRoot: "user"},
	{name: "view-root-group", viewRoot: "group"},
	{name: "view-root-other", viewRoot: "other"},
	{name: "view-leaf-user", viewLeaf: "user"},
	{name: "view-leaf-other", viewLeaf: "other"},
	{name: "view-root-user-leaf-other", viewRoot: "user", viewLeaf: "other"},
	{name: "view-root-other-leaf-user", viewRoot: "other", viewLeaf: "user"},
	{name: "edit-leaf-user", editLeaf: "user"},
	{name: "edit-leaf-group", editLeaf: "group"},
	{name: "edit-leaf-other", editLeaf: "other"},
	{name: "edit-on-parent-only", editOnMid: "other"},
	{name: "view-user-edit-other", viewLeaf: "user", editLeaf: "other"},
}

var pageStates = []struct {
	name, status string
	ownDraft     bool
}{
	{"published", "published", false},
	{"others-draft", "draft", false},
	{"own-draft", "draft", true},
	{"trashed", "trashed", false},
}

var spaceStates = []string{"active", "archived", "trashed"}

var actions = []string{
	permissions.ActionPageView, permissions.PageCreate, permissions.PageEdit, permissions.PageArchive,
	permissions.ActionPageDelete, permissions.AttachmentAdd, permissions.ActionAttachmentDelete,
	permissions.RestrictionManage, permissions.SpaceExport, permissions.SpaceAdminister,
}

// literalRule is the rule of spec 002, written out independently of Evaluate.
// It deliberately works on the case tables, not on the production types.
func literalRule(u userCase, g grantCase, r restrictionCase, pageStatus string, ownPage bool, spaceStatus, action string) bool {
	has := func(perm string) bool {
		applies := false
		switch g.subject {
		case "user":
			applies = true
		case "group":
			applies = u.inGroup
		case "all_members":
			applies = u.typ == "member"
		case "all_guests":
			applies = u.typ == "guest"
		}
		if !applies {
			return false
		}
		for _, p := range g.perms {
			if p == perm {
				return true
			}
		}
		return false
	}
	matches := func(who string) bool {
		switch who {
		case "":
			return true // no restriction
		case "user":
			return true
		case "group":
			return u.inGroup
		}
		return false // "other": someone else is listed
	}

	// Global layer and account.
	if !u.active || !u.product {
		return false
	}
	// Space state, and every action needs view.
	if spaceStatus == "trashed" || !has(permissions.SpaceView) {
		return false
	}
	// Page state.
	if pageStatus == "trashed" || (pageStatus == "draft" && !ownPage) {
		return false
	}
	// View restrictions on every ancestor including the page.
	if !matches(r.viewRoot) || !matches(r.viewLeaf) {
		return false
	}
	// Archived spaces are read-only.
	switch action {
	case permissions.ActionPageView, permissions.SpaceExport, permissions.SpaceAdminister:
	default:
		if spaceStatus == "archived" {
			return false
		}
	}
	// Space permission for the action.
	switch action {
	case permissions.ActionPageView:
	case permissions.ActionPageDelete:
		if !has(permissions.PageDeleteAny) && !(ownPage && has(permissions.PageDeleteOwn)) {
			return false
		}
	case permissions.ActionAttachmentDelete:
		if !has(permissions.AttachmentDeleteAny) && !(ownPage && has(permissions.AttachmentDeleteOwn)) {
			return false
		}
	default:
		if !has(action) {
			return false
		}
	}
	// Edit restriction on the page itself, for edits only. A restriction on the
	// parent never counts.
	switch action {
	case permissions.PageEdit, permissions.PageArchive, permissions.ActionPageDelete,
		permissions.AttachmentAdd, permissions.ActionAttachmentDelete:
		if !matches(r.editLeaf) {
			return false
		}
	}
	return true
}

// TestMatrixEvaluate runs the whole generated matrix through the pure function.
func TestMatrixEvaluate(t *testing.T) {
	me, grp, other := ids.New(), ids.New(), ids.New()
	root, leaf := ids.New(), ids.New()
	subject := func(who string) []permissions.Subject {
		switch who {
		case "user":
			return []permissions.Subject{{Type: permissions.SubjectUser, ID: me}}
		case "group":
			return []permissions.Subject{{Type: permissions.SubjectGroup, ID: grp}}
		case "other":
			return []permissions.Subject{{Type: permissions.SubjectUser, ID: other}}
		}
		return nil
	}

	n := 0
	for _, u := range userCases {
		facts := permissions.UserFacts{ID: me, Type: u.typ, Active: u.active}
		if u.inGroup {
			facts.Groups = []uuid.UUID{grp}
		}
		if u.product {
			facts.Global = []string{"use_product"}
		}
		for _, g := range grantCases {
			var grants []permissions.Grant
			for _, p := range g.perms {
				sub := permissions.Subject{Type: g.subject}
				switch g.subject {
				case "user":
					sub.ID = me
				case "group":
					sub.ID = grp
				}
				grants = append(grants, permissions.Grant{Subject: sub, Permission: p})
			}
			for _, r := range restrictionCases {
				var chain []permissions.RestrictionLevel
				if r.viewRoot != "" {
					chain = append(chain, permissions.RestrictionLevel{PageID: root, Subjects: subject(r.viewRoot)})
				}
				if r.viewLeaf != "" {
					chain = append(chain, permissions.RestrictionLevel{PageID: leaf, Subjects: subject(r.viewLeaf)})
				}
				for _, ps := range pageStates {
					owner := other
					if ps.ownDraft {
						owner = me
					}
					page := permissions.PageFacts{ID: leaf, Status: ps.status, CreatedBy: owner,
						ViewChain: chain, EditSubjects: subject(r.editLeaf)}
					for _, ss := range spaceStates {
						for _, action := range actions {
							n++
							got := permissions.Evaluate(permissions.Input{User: facts,
								Space: permissions.SpaceFacts{ID: ids.New(), Status: ss, Grants: grants},
								Page:  &page, Action: action, Owner: owner})
							want := literalRule(u, g, r, ps.status, ps.ownDraft, ss, action)
							if got.Allowed != want {
								t.Fatalf("user=%s grant=%s restriction=%s page=%s space=%s action=%s: Evaluate=%v, rule=%v\nchecks: %+v",
									u.name, g.name, r.name, ps.name, ss, action, got.Allowed, want, got.Checks)
							}
						}
					}
				}
			}
		}
	}
	t.Logf("%d cases compared", n)
	if n < 10_000 {
		t.Fatalf("matrix shrank to %d cases", n)
	}
}

// TestMatrixSQLAgreesWithEvaluate builds the matrix as real rows and compares
// what lists see (SQL) with what direct access decides (Go), for every
// user × space × page combination.
func TestMatrixSQLAgreesWithEvaluate(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	pool := tdb.Pool
	svc := permissions.NewService(pool)
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}

	inst := ids.New()
	grp, other := ids.New(), ids.New()
	exec("INSERT INTO groups (id, origin_instance_id, name, source) VALUES ($1, $2, 'g', 'local')", grp, inst)
	mkUser := func(name, typ, status string) uuid.UUID {
		id := ids.New()
		exec(`INSERT INTO users (id, origin_instance_id, type, status, email, display_name)
			VALUES ($1, $2, $3, $4, $5, $5)`, id, inst, typ, status, name+"@example.com")
		return id
	}
	exec(`INSERT INTO users (id, origin_instance_id, type, status, email, display_name)
		VALUES ($1, $2, 'member', 'active', 'other@example.com', 'o')`, other, inst)

	type realUser struct {
		id uuid.UUID
		userCase
	}
	var users []realUser
	for _, u := range userCases {
		status := "active"
		if !u.active {
			status = "disabled"
		}
		id := mkUser(u.name, u.typ, status)
		if u.product {
			exec("INSERT INTO global_grants (subject_type, subject_id, permission) VALUES ('user', $1, 'use_product')", id)
		}
		if u.inGroup {
			exec("INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)", grp, id)
		}
		users = append(users, realUser{id, u})
	}

	addPage := func(space uuid.UUID, parent *uuid.UUID, status string, by uuid.UUID) uuid.UUID {
		id := ids.New()
		exec(`INSERT INTO pages (id, origin_instance_id, space_id, parent_id, title, status, created_by, updated_by)
			VALUES ($1, $2, $3, $4, 't', $5, $6, $6)`, id, inst, space, parent, status, by)
		exec("INSERT INTO page_tree (ancestor_id, descendant_id, depth) VALUES ($1, $1, 0)", id)
		if parent != nil {
			exec(`INSERT INTO page_tree (ancestor_id, descendant_id, depth)
				SELECT ancestor_id, $2, depth + 1 FROM page_tree WHERE descendant_id = $1`, *parent, id)
		}
		return id
	}

	type built struct {
		space, leaf uuid.UUID
		g           grantCase
		r           restrictionCase
		spaceStatus string
		pageStatus  string
		creator     uuid.UUID
	}
	var all []built

	// The "user" subject in a grant or restriction has to name someone, so each
	// user gets their own copy of the user-specific cases.
	for _, ru := range users {
		for _, g := range grantCases {
			for _, ss := range spaceStates {
				space := ids.New()
				exec(`INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by)
					VALUES ($1, $2, $3, 's', $4, $5)`, space, inst, space.String(), ss, other)
				for _, p := range g.perms {
					var sub uuid.UUID
					switch g.subject {
					case "user":
						sub = ru.id
					case "group":
						sub = grp
					}
					exec("INSERT INTO space_grants (space_id, subject_type, subject_id, permission) VALUES ($1, $2, $3, $4)",
						space, g.subject, sub, p)
				}
				for _, r := range restrictionCases {
					for _, ps := range pageStates {
						creator := other
						if ps.ownDraft {
							creator = ru.id
						}
						root := addPage(space, nil, "published", other)
						mid := addPage(space, &root, "published", other)
						leaf := addPage(space, &mid, ps.status, creator)
						restrict := func(page uuid.UUID, kind, who string) {
							if who == "" {
								return
							}
							typ, id := "user", other
							switch who {
							case "user":
								id = ru.id
							case "group":
								typ, id = "group", grp
							}
							exec("INSERT INTO page_restrictions (page_id, kind, subject_type, subject_id) VALUES ($1, $2, $3, $4)",
								page, kind, typ, id)
						}
						restrict(root, "view", r.viewRoot)
						restrict(leaf, "view", r.viewLeaf)
						restrict(leaf, "edit", r.editLeaf)
						restrict(mid, "edit", r.editOnMid)
						all = append(all, built{space, leaf, g, r, ss, ps.status, creator})
					}
				}
			}
		}
		// Compare this user's copy right away; it keeps the working set small.
		visible := map[uuid.UUID]bool{}
		rows, err := pool.Query(ctx, "SELECT page_id FROM perm_visible_pages($1)", ru.id)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			visible[id] = true
		}
		rows.Close()

		facts, err := svc.LoadUser(ctx, pool, ru.id)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range all {
			name := fmt.Sprintf("user=%s grant=%s restriction=%s page=%s space=%s", ru.name, b.g.name, b.r.name, b.pageStatus, b.spaceStatus)
			space, page, err := svc.LoadPage(ctx, pool, b.leaf)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			want := literalRule(ru.userCase, b.g, b.r, b.pageStatus, b.creator == ru.id, b.spaceStatus, permissions.ActionPageView)
			if visible[b.leaf] != want {
				t.Fatalf("%s: SQL said %v, rule says %v", name, visible[b.leaf], want)
			}
			for _, action := range actions {
				d := permissions.Evaluate(permissions.Input{User: facts, Space: space, Page: &page, Action: action, Owner: page.CreatedBy})
				if rule := literalRule(ru.userCase, b.g, b.r, b.pageStatus, b.creator == ru.id, b.spaceStatus, action); d.Allowed != rule {
					t.Fatalf("%s action=%s: loaded facts gave %v, rule says %v\nchecks: %+v", name, action, d.Allowed, rule, d.Checks)
				}
			}
		}
		all = all[:0]
	}
}

func TestDeniedVisibilityDecidesNotFoundVersusForbidden(t *testing.T) {
	me := ids.New()
	u := permissions.UserFacts{ID: me, Type: "member", Active: true, Global: []string{"use_product"}}
	view := []permissions.Grant{{Subject: permissions.Subject{Type: permissions.SubjectUser, ID: me}, Permission: permissions.SpaceView}}

	d := permissions.Evaluate(permissions.Input{User: u, Space: permissions.SpaceFacts{Status: "active", Grants: view},
		Page: &permissions.PageFacts{ID: ids.New(), Status: "published"}, Action: permissions.PageEdit})
	if d.Allowed || !d.Visible {
		t.Fatalf("a viewer who may not edit must get forbidden, not not-found: %+v", d)
	}
	d = permissions.Evaluate(permissions.Input{User: u, Space: permissions.SpaceFacts{Status: "active"},
		Page: &permissions.PageFacts{ID: ids.New(), Status: "published"}, Action: permissions.PageEdit})
	if d.Allowed || d.Visible {
		t.Fatalf("a user without view must get not-found: %+v", d)
	}
}
