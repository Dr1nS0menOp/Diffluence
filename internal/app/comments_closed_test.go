// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// The pages module owns the switch; the comments module asks it at the moment of each
// comment, so closing and opening count at once (spec 041).
func TestClosingCommentsStopsNewOnesAtOnce(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")
	root := reader.comment(page, "before")

	owner.ok("PATCH", "/api/v1/pages/"+page+"/comments-closed", map[string]any{"closed": true})
	reader.want(409, "POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("after")})
	reader.want(409, "POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("a reply"), "parent_id": root})
	list := reader.ok("GET", "/api/v1/pages/"+page+"/comments", nil)
	if list["closed"] != true || len(threads(list, "threads")) != 1 {
		t.Fatalf("a closed page must say so and keep its comments: %v", list)
	}

	owner.ok("PATCH", "/api/v1/pages/"+page+"/comments-closed", map[string]any{"closed": false})
	reader.comment(page, "open again")
	if reader.ok("GET", "/api/v1/pages/"+page+"/comments", nil)["closed"] != false {
		t.Fatal("an opened page must say so")
	}
}
