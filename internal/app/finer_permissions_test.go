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
	role := f.admin.ok("POST", "/api/v1/admin/roles", map[string]any{
		"scope": permissions.ScopeSpace, "name": "Role of " + email, "description": "", "permissions": perms})
	u := e.user(email, identity.PermUseProduct)
	f.owner.ok("PUT", "/api/v1/spaces/"+f.space+"/role-assignments", map[string]any{
		"subject": map[string]any{"type": permissions.SubjectUser, "id": u.id}, "roles": []any{role["id"]}})
	return u
}

// finerAction is one thing spec 036 took out of space administration.
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
	for i, p := range []string{permissions.SitePublish, permissions.SpaceAutomation, permissions.SpaceTemplates, permissions.SpaceAudit} {
		holders[p] = f.holder(e, "holder"+string(rune('a'+i))+"@example.com", permissions.SpaceView, p)
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
