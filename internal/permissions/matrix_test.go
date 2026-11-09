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
	// expires says when the account ends: "", "past" or "future".
	expires string
}

var userCases = []userCase{
	{name: "member", typ: "member", active: true, product: true},
	{name: "member-in-group", typ: "member", active: true, product: true, inGroup: true},
	{name: "guest", typ: "guest", active: true, product: true},
	{name: "guest-in-group", typ: "guest", active: true, product: true, inGroup: true},
	{name: "disabled-member", typ: "member", product: true, inGroup: true},
	{name: "member-no-product", typ: "member", active: true, inGroup: true},
	{name: "expired-guest", typ: "guest", active: true, product: true, expires: "past"},
	{name: "expired-member", typ: "member", active: true, product: true, inGroup: true, expires: "past"},
	{name: "guest-expiring-tomorrow", typ: "guest", active: true, product: true, expires: "future"},
}

// grantCase says which subject holds which permissions in the space.
type grantCase struct {
	name    string
	subject string // user, group, all_members, all_guests, none
	perms   []string
}

var viewerPerms = []string{permissions.SpaceView, permissions.SpaceExport, permissions.CommentAdd, permissions.CommentDeleteOwn}
var editorPerms = []string{permissions.SpaceView, permissions.PageCreate, permissions.PageEdit, permissions.PageArchive,
	permissions.PageDeleteOwn, permissions.AttachmentAdd, permissions.AttachmentDeleteOwn,
	permissions.CommentAdd, permissions.CommentDeleteOwn}
var adminPerms = permissions.AllSpacePermissions
var managerPerms = permissions.Templates[permissions.TemplateManager]

var grantCases = []grantCase{
	{"none", "none", nil},
	{"user-viewer", "user", viewerPerms},
	{"user-editor", "user", editorPerms},
	{"user-admin", "user", adminPerms},
	{"user-manager", "user", managerPerms},
	{"group-manager", "group", managerPerms},
	{"administer-only", "user", []string{permissions.SpaceView, permissions.SpaceAdminister}},
	{"split-off-administer", "user", []string{permissions.SpaceView, permissions.SitePublish, permissions.SpaceAutomation,
		permissions.SpaceTemplates, permissions.SpaceAudit}},
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

type pageState struct {
	name, status string
	ownDraft     bool
	archived     bool
	kind         string
}

var pageStates = []pageState{
	{"published", "published", false, false, "page"},
	{"others-draft", "draft", false, false, "page"},
	{"own-draft", "draft", true, false, "page"},
	{"trashed", "trashed", false, false, "page"},
	{"archived", "published", false, true, "page"},
	{"own-archived", "published", true, true, "page"},
	{"post", "published", false, false, "post"},
	{"template", "published", false, false, "template"},
	{"own-template", "published", true, false, "template"},
}

var spaceStates = []string{"active", "archived", "trashed"}

var actions = []string{
	permissions.ActionPageView, permissions.PageCreate, permissions.PageEdit, permissions.PageArchive,
	permissions.ActionPageDelete, permissions.AttachmentAdd, permissions.ActionAttachmentDelete,
	permissions.RestrictionManage, permissions.SpaceExport, permissions.SpaceAdminister, permissions.SpaceManageAccess,
	permissions.CommentAdd, permissions.ActionCommentDelete, permissions.ActionReact, permissions.AnalyticsView,
	permissions.SitePublish, permissions.SpaceAutomation, permissions.SpaceTemplates, permissions.SpaceAudit,
}

// literalRule is the rule of spec 002, written out independently of Evaluate.
// It deliberately works on the case tables, not on the production types.
func literalRule(u userCase, g grantCase, r restrictionCase, ps pageState, spaceStatus, action string) bool {
	pageStatus, ownPage := ps.status, ps.ownDraft
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

	// Global layer and account. Past its end date an account counts as not active.
	if !u.active || u.expires == "past" || !u.product {
		return false
	}
	// Space state, and every action needs view.
	if spaceStatus == "trashed" || !has(permissions.SpaceView) {
		return false
	}
	// Page state. A template is never a page, whoever asks.
	if pageStatus == "trashed" || (pageStatus == "draft" && !ownPage) || ps.kind == "template" {
		return false
	}
	// View restrictions on every ancestor including the page.
	if !matches(r.viewRoot) || !matches(r.viewLeaf) {
		return false
	}
	// Archived spaces are read-only.
	switch action {
	case permissions.ActionPageView, permissions.SpaceExport, permissions.SpaceAdminister, permissions.SpaceManageAccess,
		permissions.AnalyticsView, permissions.SitePublish, permissions.SpaceAutomation, permissions.SpaceTemplates, permissions.SpaceAudit:
	default:
		if spaceStatus == "archived" {
			return false
		}
	}
	// An archived page can be read, exported and deleted, brought back, and
	// restricted. Nothing else.
	switch action {
	case permissions.ActionPageView, permissions.SpaceExport, permissions.SpaceAdminister,
		permissions.ActionPageDelete, permissions.PageArchive, permissions.RestrictionManage, permissions.AnalyticsView,
		permissions.SitePublish, permissions.SpaceAutomation, permissions.SpaceTemplates, permissions.SpaceAudit:
	default:
		if ps.archived {
			return false
		}
	}
	// Space permission for the action.
	switch action {
	case permissions.ActionPageView, permissions.ActionReact:
	case permissions.ActionPageDelete:
		if !has(permissions.PageDeleteAny) && !(ownPage && has(permissions.PageDeleteOwn)) {
			return false
		}
	case permissions.ActionAttachmentDelete:
		if !has(permissions.AttachmentDeleteAny) && !(ownPage && has(permissions.AttachmentDeleteOwn)) {
			return false
		}
	case permissions.SpaceManageAccess:
		if !has(permissions.SpaceManageAccess) && !has(permissions.SpaceAdminister) {
			return false
		}
	case permissions.ActionCommentDelete:
		if !has(permissions.CommentDeleteAny) && !(ownPage && has(permissions.CommentDeleteOwn)) {
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
		// LoadUser turns the end date into Active. The SQL test covers that step.
		facts := permissions.UserFacts{ID: me, Type: u.typ, Active: u.active && u.expires != "past"}
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
					page := permissions.PageFacts{ID: leaf, Status: ps.status, CreatedBy: owner, Kind: ps.kind, Archived: ps.archived,
						ViewChain: chain, EditSubjects: subject(r.editLeaf)}
					for _, ss := range spaceStates {
						for _, action := range actions {
							n++
							got := permissions.Evaluate(permissions.Input{User: facts,
								Space: permissions.SpaceFacts{ID: ids.New(), Status: ss, Grants: grants},
								Page:  &page, Action: action, Owner: owner})
							want := literalRule(u, g, r, ps, ss, action)
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
		switch u.expires {
		case "past":
			exec("UPDATE users SET expires_at = now() - interval '1 hour' WHERE id = $1", id)
		case "future":
			exec("UPDATE users SET expires_at = now() + interval '1 day' WHERE id = $1", id)
		}
		if u.product {
			exec("INSERT INTO global_grants (subject_type, subject_id, permission) VALUES ('user', $1, 'use_product')", id)
		}
		if u.inGroup {
			exec("INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)", grp, id)
		}
		users = append(users, realUser{id, u})
	}

	addPage := func(space uuid.UUID, parent *uuid.UUID, ps pageState, by uuid.UUID) uuid.UUID {
		id := ids.New()
		exec(`INSERT INTO pages (id, origin_instance_id, space_id, parent_id, title, status, created_by, updated_by, kind, archived_at)
			VALUES ($1, $2, $3, $4, 't', $5, $6, $6, $7, CASE WHEN $8 THEN now() END)`, id, inst, space, parent, ps.status, by, ps.kind, ps.archived)
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
		page        pageState
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
						root := addPage(space, nil, pageStates[0], other)
						mid := addPage(space, &root, pageStates[0], other)
						leaf := addPage(space, &mid, ps, creator)
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
						all = append(all, built{space, leaf, g, r, ss, ps, creator})
					}
				}
			}
		}
		// Compare this user's copy right away; it keeps the working set small.
		pagesIn := func(function string) map[uuid.UUID]bool {
			t.Helper()
			found := map[uuid.UUID]bool{}
			rows, err := pool.Query(ctx, "SELECT page_id FROM "+function+"($1)", ru.id)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			for rows.Next() {
				var id uuid.UUID
				if err := rows.Scan(&id); err != nil {
					t.Fatal(err)
				}
				found[id] = true
			}
			return found
		}
		visible, listed := pagesIn("perm_visible_pages"), pagesIn("perm_listed_pages")

		facts, err := svc.LoadUser(ctx, pool, ru.id)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range all {
			name := fmt.Sprintf("user=%s grant=%s restriction=%s page=%s space=%s", ru.name, b.g.name, b.r.name, b.page.name, b.spaceStatus)
			space, page, err := svc.LoadPage(ctx, pool, b.leaf)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			want := literalRule(ru.userCase, b.g, b.r, b.page, b.spaceStatus, permissions.ActionPageView)
			if visible[b.leaf] != want {
				t.Fatalf("%s: SQL said %v, rule says %v", name, visible[b.leaf], want)
			}
			if listed[b.leaf] != (want && !b.page.archived) {
				t.Fatalf("%s: lists said %v, rule says %v", name, listed[b.leaf], want && !b.page.archived)
			}
			for _, action := range actions {
				d := permissions.Evaluate(permissions.Input{User: facts, Space: space, Page: &page, Action: action, Owner: page.CreatedBy})
				if rule := literalRule(ru.userCase, b.g, b.r, b.page, b.spaceStatus, action); d.Allowed != rule {
					t.Fatalf("%s action=%s: loaded facts gave %v, rule says %v\nchecks: %+v", name, action, d.Allowed, rule, d.Checks)
				}
			}
		}
		all = all[:0]
	}
}

// peopleCase is one viewer and one person, for the rule on who may see whom.
type peopleCase struct {
	viewerType   string
	viewProfiles bool
	viewerActive bool
	shared       string // user, group, all_members, none
	targetType   string
	targetActive bool
}

// literalPeopleRule is the rule of spec 013, F4 and D5, written out by hand.
func literalPeopleRule(c peopleCase) bool {
	if !c.viewerActive || c.targetType == "service" {
		return false
	}
	return c.viewProfiles || c.shared == "user" || c.shared == "group"
}

// TestMatrixPeople compares three answers on who may see whom: the literal rule,
// EvaluatePerson through CheckPerson, and perm_visible_people.
func TestMatrixPeople(t *testing.T) {
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
	n := 0
	for _, viewerType := range []string{"member", "guest"} {
		for _, viewProfiles := range []bool{true, false} {
			for _, viewerActive := range []bool{true, false} {
				for _, shared := range []string{"user", "group", "all_members", "none"} {
					for _, targetType := range []string{"member", "guest", "service"} {
						for _, targetActive := range []bool{true, false} {
							c := peopleCase{viewerType, viewProfiles, viewerActive, shared, targetType, targetActive}
							n++
							viewer, target, grp, space := ids.New(), ids.New(), ids.New(), ids.New()
							status := map[bool]string{true: "active", false: "disabled"}
							exec(`INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, $3, $4, $5, 'v')`,
								viewer, inst, viewerType, status[viewerActive], viewer.String()+"@example.com")
							exec(`INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, $3, $4, $5, 't')`,
								target, inst, targetType, status[targetActive], target.String()+"@example.com")
							exec("INSERT INTO global_grants (subject_type, subject_id, permission) VALUES ('user', $1, 'use_product')", viewer)
							if viewProfiles {
								exec("INSERT INTO global_grants (subject_type, subject_id, permission) VALUES ('user', $1, 'view_profiles')", viewer)
							}
							exec(`INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by) VALUES ($1, $2, $3, 's', 'active', $4)`,
								space, inst, space.String(), viewer)
							grant := func(typ string, id uuid.UUID) {
								exec("INSERT INTO space_grants (space_id, subject_type, subject_id, permission) VALUES ($1, $2, $3, 'space.view')", space, typ, id)
							}
							switch shared {
							case "user":
								grant("user", viewer)
								grant("user", target)
							case "group":
								exec("INSERT INTO groups (id, origin_instance_id, name, source) VALUES ($1, $2, $3, 'local')", grp, inst, grp.String())
								exec("INSERT INTO group_members (group_id, user_id) VALUES ($1, $2), ($1, $3)", grp, viewer, target)
								grant("group", grp)
							case "all_members":
								grant("all_members", uuid.Nil)
							}
							want := literalPeopleRule(c)
							got, err := svc.CheckPerson(ctx, pool, viewer, target)
							if err != nil {
								t.Fatal(err)
							}
							if got != want {
								t.Fatalf("%+v: CheckPerson=%v, rule=%v", c, got, want)
							}
							var listed bool
							if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM perm_visible_people($1) WHERE user_id = $2)", viewer, target).Scan(&listed); err != nil {
								t.Fatal(err)
							}
							if listed != want {
								t.Fatalf("%+v: SQL=%v, rule=%v", c, listed, want)
							}
						}
					}
				}
			}
		}
	}
	t.Logf("%d cases compared", n)
}

// ActionApprovalRead is view without the rule on unpublished pages, and nothing else.
func TestApprovalReadIsViewWithoutTheDraftRule(t *testing.T) {
	reader, author, pageID := uuid.New(), uuid.New(), uuid.New()
	in := func(p permissions.PageFacts, action string) permissions.Input {
		p.ID, p.CreatedBy = pageID, author
		return permissions.Input{
			User:   permissions.UserFacts{ID: reader, Type: "member", Active: true, Global: []string{"use_product"}},
			Space:  permissions.SpaceFacts{Status: "active", Grants: []permissions.Grant{{Subject: permissions.Subject{Type: permissions.SubjectUser, ID: reader}, Permission: permissions.SpaceView}}},
			Page:   &p,
			Action: action,
		}
	}
	draft := permissions.PageFacts{Status: "draft", Kind: "page"}
	if permissions.Evaluate(in(draft, permissions.ActionPageView)).Allowed {
		t.Fatal("an unpublished page is private")
	}
	if !permissions.Evaluate(in(draft, permissions.ActionApprovalRead)).Allowed {
		t.Fatal("someone named in a round reads what they were asked about")
	}
	restricted := draft
	restricted.ViewChain = []permissions.RestrictionLevel{{PageID: pageID, Subjects: []permissions.Subject{{Type: permissions.SubjectUser, ID: author}}}}
	template := draft
	template.Kind = permissions.KindTemplate
	trashed := draft
	trashed.Status = "trashed"
	for name, p := range map[string]permissions.PageFacts{"restricted": restricted, "template": template, "trashed": trashed} {
		if permissions.Evaluate(in(p, permissions.ActionApprovalRead)).Allowed {
			t.Fatalf("every other rule still counts: %s", name)
		}
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
