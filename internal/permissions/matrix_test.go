// SPDX-License-Identifier: Apache-2.0

package permissions_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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
	// productViaGroup gives use_product through the group, not to the person (PRM-155).
	productViaGroup bool
	// viewProfiles holds view_profiles, which decides about people and never about pages.
	viewProfiles bool
	// inGroup2 puts the person in a second group as well.
	inGroup2 bool
	// extra marks a value added by spec 040. The SQL half pairs extra values with the
	// plain values of the other dimensions only (PRM-173).
	extra bool
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
	{name: "service-account", typ: "service", active: true, product: true, extra: true},
	{name: "service-in-group", typ: "service", active: true, product: true, inGroup: true, extra: true},
	{name: "member-product-through-group", typ: "member", active: true, product: true, inGroup: true, productViaGroup: true, extra: true},
	{name: "member-who-sees-people", typ: "member", active: true, product: true, viewProfiles: true, extra: true},
	{name: "member-in-two-groups", typ: "member", active: true, product: true, inGroup: true, inGroup2: true, extra: true},
	{name: "member-in-second-group-only", typ: "member", active: true, product: true, inGroup2: true, extra: true},
}

// grantCase says which subject holds which permissions in the space. A second subject
// can hold other permissions at the same time (PRM-158).
type grantCase struct {
	name    string
	subject string // user, group, group2, all_members, all_guests, none
	perms   []string
	also    string // a second subject, or ""
	alsoP   []string
	extra   bool
}

var viewerPerms = []string{permissions.SpaceView, permissions.SpaceExport, permissions.CommentAdd, permissions.CommentDeleteOwn}
var editorPerms = []string{permissions.SpaceView, permissions.PageCreate, permissions.PageEdit, permissions.PageArchive,
	permissions.PageDeleteOwn, permissions.AttachmentAdd, permissions.AttachmentDeleteOwn,
	permissions.CommentAdd, permissions.CommentDeleteOwn, permissions.PostCreate, permissions.DatabaseCreate}
var adminPerms = permissions.AllSpacePermissions
var managerPerms = permissions.Templates[permissions.TemplateManager]

// splitOffAdministerPerms is everything split off space administration, in specs 036 and 040.
var splitOffAdministerPerms = []string{permissions.SitePublish, permissions.SpaceAutomation, permissions.SpaceTemplates,
	permissions.SpaceAudit, permissions.SpaceLabels, permissions.SpaceStatuses, permissions.SpaceApprovals,
	permissions.SpaceBundle, permissions.SpaceLook, permissions.PageWatchers}

var grantCases = []grantCase{
	{name: "none", subject: "none"},
	{name: "user-viewer", subject: "user", perms: viewerPerms},
	{name: "user-editor", subject: "user", perms: editorPerms},
	{name: "user-admin", subject: "user", perms: adminPerms},
	{name: "user-manager", subject: "user", perms: managerPerms},
	{name: "group-manager", subject: "group", perms: managerPerms},
	{name: "administer-only", subject: "user", perms: []string{permissions.SpaceView, permissions.SpaceAdminister}},
	{name: "split-off-administer", subject: "user", perms: append([]string{permissions.SpaceView}, splitOffAdministerPerms...)},
	{name: "group-viewer", subject: "group", perms: viewerPerms},
	{name: "group-editor", subject: "group", perms: editorPerms},
	{name: "members-viewer", subject: "all_members", perms: viewerPerms},
	{name: "members-editor", subject: "all_members", perms: editorPerms},
	{name: "guests-viewer", subject: "all_guests", perms: viewerPerms},
	{name: "guests-editor", subject: "all_guests", perms: editorPerms},
	{name: "edit-without-view", subject: "user", perms: []string{permissions.PageEdit}},
	// PRM-158 and PRM-159.
	{name: "user-viewer-and-group-editor", subject: "user", perms: viewerPerms, also: "group", alsoP: editorPerms, extra: true},
	{name: "members-viewer-and-user-admin", subject: "all_members", perms: viewerPerms, also: "user",
		alsoP: []string{permissions.SpaceAdminister}, extra: true},
	{name: "delete-own-only", subject: "user", perms: []string{permissions.SpaceView, permissions.PageDeleteOwn,
		permissions.AttachmentDeleteOwn, permissions.CommentDeleteOwn}, extra: true},
	{name: "delete-any-only", subject: "user", perms: []string{permissions.SpaceView, permissions.PageDeleteAny,
		permissions.AttachmentDeleteAny, permissions.CommentDeleteAny}, extra: true},
	{name: "comment-rights", subject: "group", perms: []string{permissions.SpaceView, permissions.CommentAdd,
		permissions.CommentDeleteOwn}, extra: true},
	{name: "restrict-without-administer", subject: "user", perms: []string{permissions.SpaceView, permissions.RestrictionManage}, extra: true},
	{name: "manage-access-without-administer", subject: "user", perms: []string{permissions.SpaceView, permissions.SpaceManageAccess}, extra: true},
	{name: "second-group-editor", subject: "group2", perms: editorPerms, extra: true},
	// Spec 040: the finer permissions alone, and edit without moving out.
	{name: "move-out-without-edit", subject: "user", perms: []string{permissions.SpaceView, permissions.PageMoveOut,
		permissions.PageCreate}, extra: true},
	{name: "invite-guests-only", subject: "user", perms: []string{permissions.SpaceView, permissions.SpaceInviteGuests}, extra: true},
	{name: "edit-without-move-out", subject: "user", perms: []string{permissions.SpaceView, permissions.PageEdit,
		permissions.PageCreate}, extra: true},
	// Spec 045: each kind of item made on its own, and pages without the other two.
	{name: "posts-only", subject: "user", perms: []string{permissions.SpaceView, permissions.PostCreate}, extra: true},
	{name: "databases-only", subject: "group", perms: []string{permissions.SpaceView, permissions.DatabaseCreate}, extra: true},
	{name: "pages-without-posts-or-databases", subject: "user", perms: []string{permissions.SpaceView, permissions.PageCreate,
		permissions.PageEdit}, extra: true},
}

// restrictionCase places restrictions on the chain root → mid → leaf; the page
// under test is the leaf. Each field lists who is on that restriction, as names joined
// by "+": user, group, group2, other (someone else), disabled (a disabled person), gone
// (a group that no longer exists).
type restrictionCase struct {
	name                        string
	viewRoot, viewMid, viewLeaf string
	editLeaf, editOnMid         string
	extra                       bool
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
	// PRM-161.
	{name: "view-mid-user", viewMid: "user", extra: true},
	{name: "view-mid-other", viewMid: "other", extra: true},
	{name: "view-three-levels", viewRoot: "group", viewMid: "user", viewLeaf: "user+group", extra: true},
	{name: "view-two-on-a-list", viewLeaf: "other+user", extra: true},
	{name: "view-second-group", viewLeaf: "group2", extra: true},
	{name: "view-gone-group", viewLeaf: "gone", extra: true},
	{name: "view-gone-group-and-user", viewLeaf: "gone+user", extra: true},
	{name: "view-disabled-person", viewLeaf: "disabled", extra: true},
	{name: "edit-two-on-a-list", editLeaf: "other+group", extra: true},
	{name: "edit-gone-group", editLeaf: "gone", extra: true},
}

type pageState struct {
	name, status string
	ownDraft     bool
	archived     bool
	kind         string
	// noPage asks at space level, with no page at all (PRM-166).
	noPage bool
}

var pageStates = []pageState{
	{"published", "published", false, false, "page", false},
	{"others-draft", "draft", false, false, "page", false},
	{"own-draft", "draft", true, false, "page", false},
	{"trashed", "trashed", false, false, "page", false},
	{"archived", "published", false, true, "page", false},
	{"own-archived", "published", true, true, "page", false},
	{"post", "published", false, false, "post", false},
	{"template", "published", false, false, "template", false},
	{"own-template", "published", true, false, "template", false},
	{"folder", "published", false, false, "folder", false},
	{"link", "published", false, false, "link", false},
	{"database", "published", false, false, "database", false},
	{name: "no-page", noPage: true},
}

var spaceStates = []string{"active", "archived", "trashed"}

var actions = []string{
	permissions.SpaceView, permissions.ActionPageView, permissions.PageCreate, permissions.PageEdit, permissions.PageArchive,
	permissions.ActionPageDelete, permissions.AttachmentAdd, permissions.ActionAttachmentDelete,
	permissions.RestrictionManage, permissions.SpaceExport, permissions.SpaceAdminister, permissions.SpaceManageAccess,
	permissions.CommentAdd, permissions.ActionCommentDelete, permissions.ActionReact, permissions.AnalyticsView,
	permissions.SitePublish, permissions.SpaceAutomation, permissions.SpaceTemplates, permissions.SpaceAudit,
	permissions.SpaceLabels, permissions.SpaceStatuses, permissions.SpaceApprovals, permissions.SpaceBundle,
	permissions.SpaceLook, permissions.PageWatchers, permissions.PageMoveOut, permissions.SpaceInviteGuests,
	permissions.PostCreate, permissions.DatabaseCreate,
}

// isDeleteAction says whether the creator of the object decides, through "delete own".
func isDeleteAction(action string) bool {
	return action == permissions.ActionPageDelete || action == permissions.ActionAttachmentDelete ||
		action == permissions.ActionCommentDelete
}

// literalRule is the rule of spec 002, written out independently of Evaluate.
// It deliberately works on the case tables, not on the production types. ownObject says
// that the person asking made the object the action is about, which for a file or a
// comment need not be the page (PRM-166).
func literalRule(u userCase, g grantCase, r restrictionCase, ps pageState, spaceStatus, action string, ownObject bool) bool {
	pageStatus, ownPage := ps.status, ps.ownDraft
	applies := func(subject string) bool {
		switch subject {
		case "user":
			return true
		case "group":
			return u.inGroup
		case "group2":
			return u.inGroup2
		case "all_members":
			return u.typ == "member"
		case "all_guests":
			return u.typ == "guest"
		}
		return false
	}
	has := func(perm string) bool {
		return (applies(g.subject) && slices.Contains(g.perms, perm)) ||
			(g.also != "" && applies(g.also) && slices.Contains(g.alsoP, perm))
	}
	matches := func(list string) bool {
		if list == "" {
			return true // no restriction
		}
		for _, who := range strings.Split(list, "+") {
			switch {
			case who == "user", who == "group" && u.inGroup, who == "group2" && u.inGroup2:
				return true
			}
		}
		return false // someone else, a disabled person or a group that is gone
	}

	// Global layer and account. Past its end date an account counts as not active.
	if !u.active || u.expires == "past" || !u.product {
		return false
	}
	// Space state, and every action needs view.
	if spaceStatus == "trashed" || !has(permissions.SpaceView) {
		return false
	}
	if !ps.noPage {
		// Page state. A template is never a page, whoever asks.
		if pageStatus == "trashed" || (pageStatus == "draft" && !ownPage) || ps.kind == "template" {
			return false
		}
		// View restrictions on every ancestor including the page.
		if !matches(r.viewRoot) || !matches(r.viewMid) || !matches(r.viewLeaf) {
			return false
		}
	}
	// Archived spaces are read-only. What only looks after the space keeps working.
	switch action {
	case permissions.SpaceView, permissions.ActionPageView, permissions.SpaceExport, permissions.SpaceAdminister,
		permissions.SpaceManageAccess, permissions.SpaceInviteGuests, permissions.AnalyticsView,
		permissions.SitePublish, permissions.SpaceAutomation, permissions.SpaceTemplates, permissions.SpaceAudit,
		permissions.SpaceLabels, permissions.SpaceStatuses, permissions.SpaceApprovals, permissions.SpaceBundle,
		permissions.SpaceLook, permissions.PageWatchers:
	default:
		if spaceStatus == "archived" {
			return false
		}
	}
	// An archived page can be read, exported and deleted, brought back, and
	// restricted, and the space around it looked after. Nothing else.
	switch action {
	case permissions.ActionPageView, permissions.SpaceExport, permissions.SpaceAdminister,
		permissions.ActionPageDelete, permissions.PageArchive, permissions.RestrictionManage, permissions.AnalyticsView,
		permissions.SitePublish, permissions.SpaceAutomation, permissions.SpaceTemplates, permissions.SpaceAudit,
		permissions.SpaceLabels, permissions.SpaceStatuses, permissions.SpaceApprovals, permissions.SpaceBundle,
		permissions.SpaceLook, permissions.PageWatchers:
	default:
		if ps.archived {
			return false
		}
	}
	// Space permission for the action.
	switch action {
	case permissions.SpaceView, permissions.ActionPageView, permissions.ActionReact:
	case permissions.ActionPageDelete:
		if !has(permissions.PageDeleteAny) && !(ownObject && has(permissions.PageDeleteOwn)) {
			return false
		}
	case permissions.ActionAttachmentDelete:
		if !has(permissions.AttachmentDeleteAny) && !(ownObject && has(permissions.AttachmentDeleteOwn)) {
			return false
		}
	case permissions.SpaceManageAccess, permissions.SpaceInviteGuests:
		if !has(action) && !has(permissions.SpaceAdminister) {
			return false
		}
	case permissions.ActionCommentDelete:
		if !has(permissions.CommentDeleteAny) && !(ownObject && has(permissions.CommentDeleteOwn)) {
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
		permissions.AttachmentAdd, permissions.ActionAttachmentDelete, permissions.PageMoveOut:
		if !ps.noPage && !matches(r.editLeaf) {
			return false
		}
	}
	return true
}

// TestMatrixEvaluate runs the whole generated matrix through the pure function, one
// person at a time in parallel.
func TestMatrixEvaluate(t *testing.T) {
	me, grp, grp2, productGroup, other, disabled, gone := ids.New(), ids.New(), ids.New(), ids.New(), ids.New(), ids.New(), ids.New()
	root, mid, leaf := ids.New(), ids.New(), ids.New()
	subjects := func(list string) []permissions.Subject {
		if list == "" {
			return nil
		}
		var out []permissions.Subject
		for _, who := range strings.Split(list, "+") {
			switch who {
			case "user":
				out = append(out, permissions.Subject{Type: permissions.SubjectUser, ID: me})
			case "group":
				out = append(out, permissions.Subject{Type: permissions.SubjectGroup, ID: grp})
			case "group2":
				out = append(out, permissions.Subject{Type: permissions.SubjectGroup, ID: grp2})
			case "other":
				out = append(out, permissions.Subject{Type: permissions.SubjectUser, ID: other})
			case "disabled":
				out = append(out, permissions.Subject{Type: permissions.SubjectUser, ID: disabled})
			case "gone":
				out = append(out, permissions.Subject{Type: permissions.SubjectGroup, ID: gone})
			}
		}
		return out
	}
	grantSubject := func(who string) permissions.Subject {
		sub := permissions.Subject{Type: who}
		switch who {
		case "user":
			sub.ID = me
		case "group":
			sub.ID = grp
		case "group2":
			sub = permissions.Subject{Type: permissions.SubjectGroup, ID: grp2}
		}
		return sub
	}

	var n atomic.Int64
	t.Run("people", func(t *testing.T) {
		for _, u := range userCases {
			t.Run(u.name, func(t *testing.T) {
				t.Parallel()
				n.Add(evaluateOnePerson(t, u, matrixIDs{me: me, grp: grp, grp2: grp2, productGroup: productGroup, other: other,
					root: root, mid: mid, leaf: leaf}, subjects, grantSubject))
			})
		}
	})
	t.Logf("%d cases compared", n.Load())
	if n.Load() < 1_000_000 {
		t.Fatalf("matrix shrank to %d cases", n.Load())
	}
}

// matrixIDs are the fixed ids of the pure half of the matrix.
type matrixIDs struct {
	me, grp, grp2, productGroup, other uuid.UUID
	root, mid, leaf                    uuid.UUID
}

// evaluateOnePerson compares Evaluate with the rule for every case of one person, and
// says how many it compared.
func evaluateOnePerson(t *testing.T, u userCase, id matrixIDs, subjects func(string) []permissions.Subject,
	grantSubject func(string) permissions.Subject) int64 {
	// LoadUser turns the end date into Active. The SQL test covers that step.
	facts := permissions.UserFacts{ID: id.me, Type: u.typ, Active: u.active && u.expires != "past"}
	if u.inGroup {
		facts.Groups = append(facts.Groups, id.grp)
	}
	if u.inGroup2 {
		facts.Groups = append(facts.Groups, id.grp2)
	}
	if u.productViaGroup {
		facts.Groups = append(facts.Groups, id.productGroup)
	}
	if u.product {
		facts.Global = append(facts.Global, "use_product")
	}
	if u.viewProfiles {
		facts.Global = append(facts.Global, "view_profiles")
	}
	var n int64
	for _, g := range grantCases {
		var grants []permissions.Grant
		for _, p := range g.perms {
			grants = append(grants, permissions.Grant{Subject: grantSubject(g.subject), Permission: p})
		}
		for _, p := range g.alsoP {
			grants = append(grants, permissions.Grant{Subject: grantSubject(g.also), Permission: p})
		}
		for _, r := range restrictionCases {
			var chain []permissions.RestrictionLevel
			for _, level := range []struct {
				page uuid.UUID
				list string
			}{{id.root, r.viewRoot}, {id.mid, r.viewMid}, {id.leaf, r.viewLeaf}} {
				if level.list != "" {
					chain = append(chain, permissions.RestrictionLevel{PageID: level.page, Subjects: subjects(level.list)})
				}
			}
			for _, ps := range pageStates {
				if ps.noPage && r.name != "unrestricted" {
					continue // restrictions sit on pages
				}
				creator := id.other
				if ps.ownDraft {
					creator = id.me
				}
				var page *permissions.PageFacts
				if !ps.noPage {
					page = &permissions.PageFacts{ID: id.leaf, Status: ps.status, CreatedBy: creator, Kind: ps.kind,
						Archived: ps.archived, ViewChain: chain, EditSubjects: subjects(r.editLeaf)}
				}
				for _, ss := range spaceStates {
					space := permissions.SpaceFacts{ID: id.leaf, Status: ss, Grants: grants}
					for _, action := range actions {
						owners := []uuid.UUID{creator}
						if isDeleteAction(action) {
							// A file or comment of mine on someone else's page, and the other way round.
							owners = append(owners, map[uuid.UUID]uuid.UUID{id.me: id.other, id.other: id.me}[creator])
						}
						for _, owner := range owners {
							n++
							got := permissions.Evaluate(permissions.Input{User: facts, Space: space, Page: page, Action: action, Owner: owner})
							want := literalRule(u, g, r, ps, ss, action, owner == id.me)
							if got.Allowed != want {
								t.Fatalf("user=%s grant=%s restriction=%s page=%s space=%s action=%s own=%v: Evaluate=%v, rule=%v\nchecks: %+v",
									u.name, g.name, r.name, ps.name, ss, action, owner == id.me, got.Allowed, want, got.Checks)
							}
						}
					}
				}
			}
		}
	}
	return n
}

// sqlShape gives, for the SQL half, the space and page states a combination is built
// in, and false when it is left to Evaluate. Values added in spec 040 are paired with
// the plain values of the other dimensions only, in fewer states (PRM-173): the rows are
// real, and the full product would take minutes.
func sqlShape(u userCase, g grantCase, r restrictionCase) ([]string, []pageState, bool) {
	extras := 0
	for _, e := range []bool{u.extra, g.extra, r.extra} {
		if e {
			extras++
		}
	}
	var pages []pageState
	for _, ps := range pageStates {
		if !ps.noPage {
			pages = append(pages, ps)
		}
	}
	switch extras {
	case 0:
		return spaceStates, pages, true
	case 1:
		return []string{"active", "archived"}, []pageState{pageStates[0], pageStates[1], pageStates[2], pageStates[4]}, true
	}
	return nil, nil, false
}

// TestMatrixSQLAgreesWithEvaluate builds the matrix as real rows and compares
// what lists see (SQL) with what direct access decides (Go), for every
// user × space × page combination that sqlShape keeps.
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
	grp, grp2, other, disabled, gone := ids.New(), ids.New(), ids.New(), ids.New(), ids.New()
	exec("INSERT INTO groups (id, origin_instance_id, name, source) VALUES ($1, $2, 'g', 'local'), ($3, $2, 'g2', 'local')", grp, inst, grp2)
	mkUser := func(name, typ, status string) uuid.UUID {
		id := ids.New()
		exec(`INSERT INTO users (id, origin_instance_id, type, status, email, display_name)
			VALUES ($1, $2, $3, $4, $5, $5)`, id, inst, typ, status, name+"@example.com")
		return id
	}
	exec(`INSERT INTO users (id, origin_instance_id, type, status, email, display_name)
		VALUES ($1, $2, 'member', 'active', 'other@example.com', 'o'), ($3, $2, 'member', 'disabled', 'disabled@example.com', 'd')`,
		other, inst, disabled)

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
		switch {
		case u.productViaGroup:
			// A group of its own, so nobody else in the shared groups gains the product.
			productGroup := ids.New()
			exec("INSERT INTO groups (id, origin_instance_id, name, source) VALUES ($1, $2, $3, 'local')", productGroup, inst, "p-"+u.name)
			exec("INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)", productGroup, id)
			exec("INSERT INTO global_grants (subject_type, subject_id, permission) VALUES ('group', $1, 'use_product')", productGroup)
		case u.product:
			exec("INSERT INTO global_grants (subject_type, subject_id, permission) VALUES ('user', $1, 'use_product')", id)
		}
		if u.viewProfiles {
			exec("INSERT INTO global_grants (subject_type, subject_id, permission) VALUES ('user', $1, 'view_profiles')", id)
		}
		if u.inGroup {
			exec("INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)", grp, id)
		}
		if u.inGroup2 {
			exec("INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)", grp2, id)
		}
		users = append(users, realUser{id, u})
	}

	// The rows of one person go to the database in one batch, in order: one round trip
	// for thousands of rows keeps the test quick.
	var batch pgx.Batch
	queue := func(sql string, args ...any) { batch.Queue(sql, args...) }
	flush := func() {
		t.Helper()
		if err := pool.SendBatch(ctx, &batch).Close(); err != nil {
			t.Fatal(err)
		}
		batch = pgx.Batch{}
	}
	addPage := func(space uuid.UUID, parent *uuid.UUID, ps pageState, by uuid.UUID) uuid.UUID {
		id := ids.New()
		queue(`INSERT INTO pages (id, origin_instance_id, space_id, parent_id, title, status, created_by, updated_by, kind, archived_at)
			VALUES ($1, $2, $3, $4, 't', $5, $6, $6, $7, CASE WHEN $8::boolean THEN now() END)`, id, inst, space, parent, ps.status, by, ps.kind, ps.archived)
		queue("INSERT INTO page_tree (ancestor_id, descendant_id, depth) VALUES ($1, $1, 0)", id)
		if parent != nil {
			queue(`INSERT INTO page_tree (ancestor_id, descendant_id, depth)
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
	}
	var all []built
	compared := 0

	// The "user" subject in a grant or restriction has to name someone, so each
	// user gets their own copy of the user-specific cases.
	for _, ru := range users {
		grantTo := func(space uuid.UUID, who string, perms []string) {
			typ, sub := who, uuid.Nil
			switch who {
			case "user":
				sub = ru.id
			case "group":
				sub = grp
			case "group2":
				typ, sub = "group", grp2
			case "none", "":
				return
			}
			for _, p := range perms {
				queue("INSERT INTO space_grants (space_id, subject_type, subject_id, permission) VALUES ($1, $2, $3, $4)",
					space, typ, sub, p)
			}
		}
		restrict := func(page uuid.UUID, kind, list string) {
			if list == "" {
				return
			}
			for _, who := range strings.Split(list, "+") {
				typ, id := "user", other
				switch who {
				case "user":
					id = ru.id
				case "group":
					typ, id = "group", grp
				case "group2":
					typ, id = "group", grp2
				case "disabled":
					id = disabled
				case "gone":
					typ, id = "group", gone
				}
				queue("INSERT INTO page_restrictions (page_id, kind, subject_type, subject_id) VALUES ($1, $2, $3, $4)",
					page, kind, typ, id)
			}
		}
		for _, g := range grantCases {
			for _, r := range restrictionCases {
				states, pages, ok := sqlShape(ru.userCase, g, r)
				if !ok {
					continue
				}
				for _, ss := range states {
					space := ids.New()
					queue(`INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by)
						VALUES ($1, $2, $3, 's', $4, $5)`, space, inst, space.String(), ss, other)
					grantTo(space, g.subject, g.perms)
					grantTo(space, g.also, g.alsoP)
					for _, ps := range pages {
						creator := other
						if ps.ownDraft {
							creator = ru.id
						}
						root := addPage(space, nil, pageStates[0], other)
						mid := addPage(space, &root, pageStates[0], other)
						leaf := addPage(space, &mid, ps, creator)
						restrict(root, "view", r.viewRoot)
						restrict(mid, "view", r.viewMid)
						restrict(leaf, "view", r.viewLeaf)
						restrict(leaf, "edit", r.editLeaf)
						restrict(mid, "edit", r.editOnMid)
						all = append(all, built{space, leaf, g, r, ss, ps})
					}
				}
			}
		}
		flush()
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
		visible, listed, editable := pagesIn("perm_visible_pages"), pagesIn("perm_listed_pages"), pagesIn("perm_editable_pages")

		facts, err := svc.LoadUser(ctx, pool, ru.id)
		if err != nil {
			t.Fatal(err)
		}
		// Loading the facts of each page is one round trip after another, so several run
		// at once. The first difference found ends the test.
		check := func(b built) error {
			name := fmt.Sprintf("user=%s grant=%s restriction=%s page=%s space=%s", ru.name, b.g.name, b.r.name, b.page.name, b.spaceStatus)
			space, page, err := svc.LoadPage(ctx, pool, b.leaf)
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			own := b.page.ownDraft
			want := literalRule(ru.userCase, b.g, b.r, b.page, b.spaceStatus, permissions.ActionPageView, own)
			if visible[b.leaf] != want {
				return fmt.Errorf("%s: SQL said %v, rule says %v", name, visible[b.leaf], want)
			}
			if listed[b.leaf] != (want && !b.page.archived) {
				return fmt.Errorf("%s: lists said %v, rule says %v", name, listed[b.leaf], want && !b.page.archived)
			}
			if rule := literalRule(ru.userCase, b.g, b.r, b.page, b.spaceStatus, permissions.PageEdit, own); editable[b.leaf] != rule {
				return fmt.Errorf("%s: editable list said %v, rule says %v", name, editable[b.leaf], rule)
			}
			for _, action := range actions {
				d := permissions.Evaluate(permissions.Input{User: facts, Space: space, Page: &page, Action: action, Owner: page.CreatedBy})
				if rule := literalRule(ru.userCase, b.g, b.r, b.page, b.spaceStatus, action, own); d.Allowed != rule {
					return fmt.Errorf("%s action=%s: loaded facts gave %v, rule says %v\nchecks: %+v", name, action, d.Allowed, rule, d.Checks)
				}
			}
			return nil
		}
		if err := checkAll(all, check); err != nil {
			t.Fatal(err)
		}
		compared += len(all)
		all = all[:0]
	}
	t.Logf("%d pages compared", compared)
}

// checkAll runs check on every item, eight at a time, and returns the first error.
func checkAll[T any](items []T, check func(T) error) error {
	const workers = 8
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		first error
		next  atomic.Int64
	)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= len(items) {
					return
				}
				mu.Lock()
				failed := first != nil
				mu.Unlock()
				if failed {
					return
				}
				if err := check(items[i]); err != nil {
					mu.Lock()
					if first == nil {
						first = err
					}
					mu.Unlock()
					return
				}
			}
		}()
	}
	wg.Wait()
	return first
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
