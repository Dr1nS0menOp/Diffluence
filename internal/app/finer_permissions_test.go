// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// finerSpace is a space where each person holds one role the system administrator made.
type finerSpace struct {
	owner *user
	admin *user
	space string
	home  string
}

func newFinerSpace(t *testing.T) (*env, finerSpace) {
	t.Helper()
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, home := owner.space("FIN")
	return e, finerSpace{owner: owner, admin: admin, space: space, home: home}
}

// holder makes a person who holds, in the space, one role with exactly these permissions.
func (f finerSpace) holder(e *env, email string, perms ...string) *user {
	e.t.Helper()
	return f.holderWith(e, email, []string{identity.PermUseProduct}, perms...)
}

// holderWith is holder for someone who also holds these instance permissions.
func (f finerSpace) holderWith(e *env, email string, globals []string, perms ...string) *user {
	e.t.Helper()
	role := f.admin.ok("POST", "/api/v1/admin/roles", map[string]any{
		"scope": permissions.ScopeSpace, "name": "Role of " + email, "description": "", "permissions": perms})
	u := e.user(email, globals...)
	f.owner.ok("PUT", "/api/v1/spaces/"+f.space+"/role-assignments", map[string]any{
		"subject": map[string]any{"type": permissions.SubjectUser, "id": u.id}, "roles": []any{role["id"]}})
	return u
}

// finerAction is one thing specs 036 and 040 took out of space administration.
type finerAction struct {
	permission   string
	method, path string
	body         any
}

func (f finerSpace) finerActions() []finerAction {
	return []finerAction{
		{permissions.SitePublish, "POST", "/api/v1/spaces/" + f.space + "/sites", map[string]any{"name": "Docs"}},
		{permissions.SitePublish, "PUT", "/api/v1/pages/" + f.home + "/public-link", map[string]any{}},
		{permissions.SpaceAutomation, "POST", "/api/v1/spaces/" + f.space + "/rules", map[string]any{"name": "x"}},
		{permissions.SpaceAutomation, "POST", "/api/v1/spaces/" + f.space + "/webhooks", map[string]any{}},
		{permissions.SpaceTemplates, "POST", "/api/v1/spaces/" + f.space + "/templates", map[string]any{"name": "Minutes"}},
		{permissions.SpaceTemplates, "GET", "/api/v1/spaces/" + f.space + "/manage-templates", nil},
		{permissions.SpaceAudit, "GET", "/api/v1/spaces/" + f.space + "/audit", nil},
		{permissions.SpaceLabels, "PUT", "/api/v1/spaces/" + f.space + "/labels", map[string]any{"fixed": false, "labels": []string{}}},
		{permissions.SpaceStatuses, "PUT", "/api/v1/spaces/" + f.space + "/statuses", map[string]any{"free": true, "statuses": []any{}}},
		{permissions.SpaceStatuses, "PUT", "/api/v1/spaces/" + f.space + "/review", map[string]any{"months": 6}},
		{permissions.SpaceApprovals, "GET", "/api/v1/spaces/" + f.space + "/approvals", nil},
		{permissions.SpaceApprovals, "PUT", "/api/v1/spaces/" + f.space + "/approvals", map[string]any{"required": false, "approvers": []string{}}},
		{permissions.SpaceLook, "PUT", "/api/v1/spaces/" + f.space + "/shortcuts", map[string]any{"shortcuts": []any{}}},
		{permissions.SpaceLook, "PUT", "/api/v1/spaces/" + f.space + "/title-align", map[string]any{"align": "center"}},
		{permissions.SpaceLook, "PUT", "/api/v1/spaces/" + f.space + "/home", map[string]any{"page_id": f.home}},
		{permissions.PageWatchers, "GET", "/api/v1/pages/" + f.home + "/watchers", nil},
		{permissions.SpaceBundle, "POST", "/api/v1/spaces/" + f.space + "/bundles", map[string]any{}},
	}
}

func TestAdministeringASpaceNoLongerHoldsWhatWasSplitOff(t *testing.T) {
	e, f := newFinerSpace(t)
	keeper := f.holder(e, "keeper@example.com", permissions.SpaceView, permissions.SpaceAdminister, permissions.SpaceManageAccess)
	for _, a := range f.finerActions() {
		if got, out := keeper.do(a.method, a.path, a.body); got != 403 {
			t.Errorf("%s %s without %s: got %d, want 403: %s", a.method, a.path, a.permission, got, out)
		}
	}
	// Administering itself still works.
	keeper.want(200, "GET", "/api/v1/spaces/"+f.space+"/trash", nil)
}

func TestEachSplitPermissionWorksWithoutAdministering(t *testing.T) {
	e, f := newFinerSpace(t)
	holders := map[string]*user{}
	for i, p := range []string{permissions.SitePublish, permissions.SpaceAutomation, permissions.SpaceTemplates, permissions.SpaceAudit,
		permissions.SpaceLabels, permissions.SpaceStatuses, permissions.SpaceApprovals, permissions.SpaceLook,
		permissions.PageWatchers, permissions.SpaceBundle} {
		// A bundle is an export too, so it asks for both.
		holders[p] = f.holder(e, "holder"+string(rune('a'+i))+"@example.com", permissions.SpaceView, permissions.SpaceExport, p)
	}
	for _, a := range f.finerActions() {
		// Past the access rule, the request may still be refused for what it says or for a
		// setting of the instance, but never for want of permission.
		if got, out := holders[a.permission].do(a.method, a.path, a.body); got == 403 || got == 404 {
			t.Errorf("%s %s with only %s: got %d: %s", a.method, a.path, a.permission, got, out)
		}
		for p, other := range holders {
			if p == a.permission {
				continue
			}
			if got, _ := other.do(a.method, a.path, a.body); got != 403 {
				t.Errorf("%s %s with only %s: got %d, want 403", a.method, a.path, p, got)
			}
		}
	}
}

func TestABundleStillNeedsExport(t *testing.T) {
	e, f := newFinerSpace(t)
	bundler := f.holder(e, "bundler@example.com", permissions.SpaceView, permissions.SpaceBundle)
	bundler.want(403, "POST", "/api/v1/spaces/"+f.space+"/bundles", map[string]any{})
}

func TestAskingForGuestsIsItsOwnPermission(t *testing.T) {
	e, f := newFinerSpace(t)
	path := "/api/v1/spaces/" + f.space + "/guests"
	body := map[string]any{"email": "guest@example.com"}
	// The instance-wide invite_people still decides who may ask at all.
	inviters := []string{identity.PermUseProduct, identity.PermInvitePeople}
	asker := f.holderWith(e, "asker@example.com", inviters, permissions.SpaceView, permissions.SpaceInviteGuests)
	if got, out := asker.do("POST", path, body); got == 403 || got == 404 {
		t.Fatalf("asking for a guest with only that permission: %d %s", got, out)
	}
	// Manage access no longer brings it in a role made after the split.
	manager := f.holderWith(e, "manager@example.com", inviters, permissions.SpaceView, permissions.SpaceManageAccess)
	manager.want(403, "POST", path, map[string]any{"email": "other@example.com"})
	// Administering a space still does, as it does manage access.
	keeper := f.holderWith(e, "keeper@example.com", inviters, permissions.SpaceView, permissions.SpaceAdminister)
	if got, out := keeper.do("POST", path, map[string]any{"email": "third@example.com"}); got == 403 || got == 404 {
		t.Fatalf("asking for a guest as someone who runs the space: %d %s", got, out)
	}
}

func TestMovingAPageToAnotherSpaceIsItsOwnPermission(t *testing.T) {
	e, f := newFinerSpace(t)
	target, _ := f.owner.space("TGT")
	editor := f.holder(e, "editor@example.com", permissions.SpaceView, permissions.PageCreate, permissions.PageEdit)
	mover := f.holder(e, "mover@example.com", permissions.SpaceView, permissions.PageCreate, permissions.PageMoveOut)
	for _, u := range []*user{editor, mover} {
		f.owner.grant(target, "user", u.id, permissions.TemplateEditor)
	}
	page := f.owner.page(f.space, f.home, "Moving", "text")
	to := map[string]any{"space_id": target}
	editor.want(403, "GET", "/api/v1/pages/"+page+"/move-impact?space_id="+target, nil)
	editor.want(403, "POST", "/api/v1/pages/"+page+"/move", to)
	// Editing alone still moves a page within its space.
	other := f.owner.page(f.space, nil, "Elsewhere", "")
	editor.ok("POST", "/api/v1/pages/"+page+"/move", map[string]any{"parent_id": other})

	mover.ok("GET", "/api/v1/pages/"+page+"/move-impact?space_id="+target, nil)
	moved := mover.ok("POST", "/api/v1/pages/"+page+"/move", to)
	if moved["space_id"] != target {
		t.Fatalf("the page did not move: %v", moved)
	}
}
