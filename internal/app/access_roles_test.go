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
