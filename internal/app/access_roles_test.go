// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// TestTheAccessScreenPreviewsARoleBeforeGivingIt is the route behind the Access screen
// of spec 043: choosing a role shows what it gives or takes before it is saved.
func TestTheAccessScreenPreviewsARoleBeforeGivingIt(t *testing.T) {
	e, f := newFinerSpace(t)
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	person := e.user("person@example.com", identity.PermUseProduct)
	path := "/api/v1/spaces/" + f.space + "/role-assignments/preview"
	subject := map[string]any{"type": permissions.SubjectUser, "id": person.id}
	var editor string
	for _, item := range f.owner.ok("GET", "/api/v1/roles?scope=space", nil)["roles"].([]any) {
		if role := item.(map[string]any); role["built_in"] == permissions.TemplateEditor {
			editor = role["id"].(string)
		}
	}

	stranger.want(404, "POST", path, map[string]any{"subject": subject, "roles": []string{editor}})
	f.owner.want(400, "POST", path, "not an object")
	f.owner.want(400, "POST", path, map[string]any{"subject": subject, "roles": []string{"not a role id"}})

	preview := f.owner.ok("POST", path, map[string]any{"subject": subject, "roles": []string{editor}})
	if preview["loses_view"] != false || len(preview["gains"].([]any)) == 0 {
		t.Fatalf("giving the editor role: %v", preview)
	}
	if got, out := person.do("GET", "/api/v1/spaces/"+f.space, nil); got != 404 {
		t.Fatalf("a preview gave access: %d %s", got, out)
	}
	f.owner.ok("PUT", "/api/v1/spaces/"+f.space+"/role-assignments", map[string]any{"subject": subject, "roles": []string{editor}})
	preview = f.owner.ok("POST", path, map[string]any{"subject": subject, "roles": []string{}})
	if preview["loses_view"] != true {
		t.Fatalf("taking the role away must ask first: %v", preview)
	}
}

// TestSomeoneHoldsSeveralRolesInASpace is spec 049: a second role is added beside the first,
// the person may do what either allows, and taking one away keeps the other.
func TestSomeoneHoldsSeveralRolesInASpace(t *testing.T) {
	e, f := newFinerSpace(t)
	person := e.user("person@example.com", identity.PermUseProduct)
	reviewer := f.admin.ok("POST", "/api/v1/admin/roles", map[string]any{
		"scope": permissions.ScopeSpace, "name": "Reviewer", "description": "", "permissions": []string{permissions.SpaceView, permissions.AnalyticsView}})["id"].(string)
	var viewer string
	for _, item := range f.owner.ok("GET", "/api/v1/roles?scope=space", nil)["roles"].([]any) {
		if role := item.(map[string]any); role["built_in"] == permissions.TemplateViewer {
			viewer = role["id"].(string)
		}
	}
	subject := map[string]any{"type": permissions.SubjectUser, "id": person.id}
	assign := "/api/v1/spaces/" + f.space + "/role-assignments"
	f.owner.ok("PUT", assign, map[string]any{"subject": subject, "roles": []string{viewer}})
	preview := f.owner.ok("POST", assign+"/preview", map[string]any{"subject": subject, "roles": []string{viewer, reviewer}})
	if preview["loses_view"] != false || len(preview["loses"].([]any)) != 0 {
		t.Fatalf("adding a role takes something away: %v", preview)
	}
	f.owner.ok("PUT", assign, map[string]any{"subject": subject, "roles": []string{viewer, reviewer}})

	held := 0
	for _, item := range f.owner.ok("GET", assign, nil)["assignments"].([]any) {
		a := item.(map[string]any)
		if a["subject"].(map[string]any)["id"] == person.id {
			held = len(a["roles"].([]any))
		}
	}
	if held != 2 {
		t.Fatalf("the person holds %d roles, want 2", held)
	}
	can := person.ok("GET", "/api/v1/spaces/"+f.space, nil)["can"].(map[string]any)
	if can[permissions.AnalyticsView] != true || can[permissions.SpaceExport] != true {
		t.Fatalf("two roles do not add up: %v", can)
	}

	// Taking the reviewer away keeps the viewer: the space stays open to them.
	preview = f.owner.ok("POST", assign+"/preview", map[string]any{"subject": subject, "roles": []string{viewer}})
	if preview["loses_view"] != false {
		t.Fatalf("taking one of two roles must not take the space: %v", preview)
	}
	f.owner.ok("PUT", assign, map[string]any{"subject": subject, "roles": []string{viewer}})
	can = person.ok("GET", "/api/v1/spaces/"+f.space, nil)["can"].(map[string]any)
	if can[permissions.AnalyticsView] != false || can[permissions.SpaceExport] != true {
		t.Fatalf("after taking one role away: %v", can)
	}
}
