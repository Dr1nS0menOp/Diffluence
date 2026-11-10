// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"testing"

	"github.com/diffluence/diffluence/internal/permissions"
)

// The create permissions of spec 045: a blog post and a database are made with a
// permission of their own, and pages, folders and links with page.create.

// assign gives a person, in a space other than the fixture's, one role with exactly these
// permissions.
func (f finerSpace) assign(e *env, space, name string, u *user, perms ...string) {
	e.t.Helper()
	role := f.admin.ok("POST", "/api/v1/admin/roles", map[string]any{
		"scope": permissions.ScopeSpace, "name": name, "description": "", "permissions": perms})
	f.owner.ok("PUT", "/api/v1/spaces/"+space+"/role-assignments", map[string]any{
		"subject": map[string]any{"type": permissions.SubjectUser, "id": u.id}, "roles": []any{role["id"]}})
}

func newItem(space string, parent any, kind, title string) map[string]any {
	return map[string]any{"space_id": space, "parent_id": parent, "kind": kind, "title": title}
}

func TestPagesWithoutPostsOrDatabasesCannotMakeEither(t *testing.T) {
	e, f := newFinerSpace(t)
	writer := f.holder(e, "writer@example.com", permissions.SpaceView, permissions.PageCreate, permissions.PageEdit)
	writer.want(403, "POST", "/api/v1/pages", newItem(f.space, nil, "post", "News"))
	writer.want(403, "POST", "/api/v1/pages", newItem(f.space, nil, "database", "Stock"))
	writer.want(403, "POST", "/api/v1/pages", newItem(f.space, f.home, "database", "Stock below"))
	// Pages, folders and links are still made with page.create.
	writer.ok("POST", "/api/v1/pages", newItem(f.space, nil, "page", "Notes"))
	writer.ok("POST", "/api/v1/pages", newItem(f.space, nil, "folder", "Box"))
	space := writer.ok("GET", "/api/v1/spaces/"+f.space, nil)
	can := space["can"].(map[string]any)
	if can[permissions.PageCreate] != true || can[permissions.PostCreate] != false || can[permissions.DatabaseCreate] != false {
		t.Fatalf("the space offers what it would refuse: %v", can)
	}
}

func TestEachKindIsMadeWithItsOwnPermissionAlone(t *testing.T) {
	e, f := newFinerSpace(t)
	blogger := f.holder(e, "blogger@example.com", permissions.SpaceView, permissions.PostCreate)
	builder := f.holder(e, "builder@example.com", permissions.SpaceView, permissions.DatabaseCreate)

	post := blogger.ok("POST", "/api/v1/pages", newItem(f.space, nil, "post", "News"))
	if post["kind"] != "post" {
		t.Fatalf("a post came out as %v", post["kind"])
	}
	blogger.want(403, "POST", "/api/v1/pages", newItem(f.space, nil, "page", "Notes"))
	blogger.want(403, "POST", "/api/v1/pages", newItem(f.space, nil, "database", "Stock"))

	builder.ok("POST", "/api/v1/pages", newItem(f.space, nil, "database", "Stock"))
	// Below a page too: the parent asks for the kind that goes under it.
	builder.ok("POST", "/api/v1/pages", newItem(f.space, f.home, "database", "Stock below"))
	builder.want(403, "POST", "/api/v1/pages", newItem(f.space, nil, "page", "Notes"))
	builder.want(403, "POST", "/api/v1/pages", newItem(f.space, nil, "post", "News"))

	can := blogger.ok("GET", "/api/v1/spaces/"+f.space, nil)["can"].(map[string]any)
	if can[permissions.PostCreate] != true || can[permissions.PageCreate] != false || can[permissions.DatabaseCreate] != false {
		t.Fatalf("the space tells a blogger the wrong things: %v", can)
	}
}

func TestTurningAPageIntoAPostAsksForPosts(t *testing.T) {
	e, f := newFinerSpace(t)
	writer := f.holder(e, "writer@example.com", permissions.SpaceView, permissions.PageCreate, permissions.PageEdit)
	draft := writer.ok("POST", "/api/v1/pages", newItem(f.space, nil, "page", "Draft"))["id"].(string)
	writer.want(403, "POST", "/api/v1/pages/"+draft+"/kind", map[string]any{"kind": "post"})
	// A folder is a page in this sense, so page.create and edit are enough.
	writer.ok("POST", "/api/v1/pages/"+draft+"/kind", map[string]any{"kind": "folder"})

	both := f.holder(e, "both@example.com", permissions.SpaceView, permissions.PageCreate, permissions.PageEdit, permissions.PostCreate)
	other := both.ok("POST", "/api/v1/pages", newItem(f.space, nil, "page", "Other draft"))["id"].(string)
	both.ok("POST", "/api/v1/pages/"+other+"/kind", map[string]any{"kind": "post"})
	both.ok("POST", "/api/v1/pages/"+other+"/kind", map[string]any{"kind": "page"})

	// Back from a post makes a page, which asks for page.create.
	blogger := f.holder(e, "blogger@example.com", permissions.SpaceView, permissions.PostCreate, permissions.PageEdit)
	post := blogger.ok("POST", "/api/v1/pages", newItem(f.space, nil, "post", "News"))["id"].(string)
	blogger.want(403, "POST", "/api/v1/pages/"+post+"/kind", map[string]any{"kind": "page"})
}

func TestCopyingADatabaseAsksForDatabasesWhereTheCopyGoes(t *testing.T) {
	e, f := newFinerSpace(t)
	parent := f.owner.page(f.space, f.home, "Parent", "text")
	db := f.owner.ok("POST", "/api/v1/pages", newItem(f.space, parent, "database", "Stock"))["id"].(string)
	copier := f.holder(e, "copier@example.com", permissions.SpaceView, permissions.PageCreate, permissions.PageEdit)
	copier.want(403, "POST", "/api/v1/pages/"+db+"/copy", map[string]any{})
	// A branch with a database in it cannot be copied whole without it either.
	copier.want(403, "POST", "/api/v1/pages/"+parent+"/copy", map[string]any{"include_children": true})
	// Without the branch, the page alone is a page.
	copier.ok("POST", "/api/v1/pages/"+parent+"/copy", map[string]any{})

	builder := f.holder(e, "builder@example.com", permissions.SpaceView, permissions.DatabaseCreate)
	builder.ok("POST", "/api/v1/pages/"+db+"/copy", map[string]any{})
}

func TestMovingADatabaseToAnotherSpaceAsksForDatabasesThere(t *testing.T) {
	e, f := newFinerSpace(t)
	target, _ := f.owner.space("TGT")
	parent := f.owner.page(f.space, f.home, "Parent", "text")
	f.owner.ok("POST", "/api/v1/pages", newItem(f.space, parent, "database", "Stock"))
	mover := f.holder(e, "mover@example.com", permissions.SpaceView, permissions.PageCreate, permissions.DatabaseCreate,
		permissions.PageEdit, permissions.PageMoveOut)
	f.assign(e, target, "Pages only in TGT", mover, permissions.SpaceView, permissions.PageCreate)
	mover.want(403, "POST", "/api/v1/pages/"+parent+"/move", map[string]any{"space_id": target})

	f.assign(e, target, "Pages and databases in TGT", mover, permissions.SpaceView, permissions.PageCreate, permissions.DatabaseCreate)
	if moved := mover.ok("POST", "/api/v1/pages/"+parent+"/move", map[string]any{"space_id": target}); moved["space_id"] != target {
		t.Fatalf("the branch did not move: %v", moved)
	}
}

func TestThePageCreatedEventSaysWhichKindWasMade(t *testing.T) {
	_, f := newFinerSpace(t)
	f.owner.ok("POST", "/api/v1/pages", newItem(f.space, nil, "database", "Stock"))
	// The owner reads the audit of the space, where titles show to those who may see them.
	audit := f.owner.ok("GET", "/api/v1/spaces/"+f.space+"/audit", nil)
	events, _ := audit["events"].([]any)
	for _, raw := range events {
		ev := raw.(map[string]any)
		details, _ := ev["details"].(map[string]any)
		if details["kind"] == "database" && details["title"] == "Stock" {
			return
		}
	}
	t.Fatalf("no page.created event names the database: %v", audit)
}
