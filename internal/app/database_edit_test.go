// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"testing"

	"github.com/diffluence/diffluence/internal/permissions"
)

// Spec 049: editing a database has its own permission. Everything that changes a database
// asks for database.edit, and everything that changes another item asks for page.edit.

// databaseEdits are the routes that change an item, as one person tries them on it.
func databaseEdits(u *user, id, title string) []func() int {
	status := func(method, path string, body any) int {
		code, _ := u.do(method, path, body)
		return code
	}
	return []func() int{
		func() int {
			return status("PUT", "/api/v1/pages/"+id+"/draft", map[string]any{"title": title, "document": sheetWith("x"), "base_version": 1})
		},
		func() int {
			return status("POST", "/api/v1/pages/"+id+"/publish", map[string]any{"title": title, "document": sheetWith("y"), "base_version": 1})
		},
		func() int { return status("PATCH", "/api/v1/pages/"+id, map[string]any{"title": title + " renamed"}) },
		func() int { return status("POST", "/api/v1/pages/"+id+"/labels", map[string]any{"label": "stock"}) },
		func() int { return status("POST", "/api/v1/pages/"+id+"/versions/1/restore", map[string]any{}) },
	}
}

func TestEditingPagesIsNotEditingDatabases(t *testing.T) {
	e, f := newFinerSpace(t)
	writer := f.holder(e, "writer@example.com", permissions.SpaceView, permissions.PageEdit)
	database := f.owner.ok("POST", "/api/v1/pages", newItem(f.space, nil, "database", "Stock"))["id"].(string)
	f.owner.publish(database, "Stock", 1, sheetWith("tea"))

	for i, try := range databaseEdits(writer, database, "Stock") {
		if got := try(); got != 403 {
			t.Fatalf("change %d of a database by someone who only edits pages: %d, want 403", i, got)
		}
	}
	can := writer.ok("GET", "/api/v1/pages/"+database, nil)["can"].(map[string]any)
	if can[permissions.PageEdit] != false {
		t.Fatalf("the database offers editing to someone who only edits pages: %v", can)
	}
	// A page of the same space is still theirs to edit.
	if can := writer.ok("GET", "/api/v1/pages/"+f.home, nil)["can"].(map[string]any); can[permissions.PageEdit] != true {
		t.Fatalf("someone who edits pages lost a page: %v", can)
	}
}

func TestEditingDatabasesIsNotEditingPages(t *testing.T) {
	e, f := newFinerSpace(t)
	keeper := f.holder(e, "keeper@example.com", permissions.SpaceView, permissions.DatabaseEdit)
	database := f.owner.ok("POST", "/api/v1/pages", newItem(f.space, nil, "database", "Stock"))["id"].(string)
	f.owner.publish(database, "Stock", 1, sheetWith("tea"))

	keeper.ok("POST", "/api/v1/pages/"+database+"/publish", map[string]any{"title": "Stock", "document": sheetWith("coffee"), "base_version": 2})
	if can := keeper.ok("GET", "/api/v1/pages/"+database, nil)["can"].(map[string]any); can[permissions.PageEdit] != true {
		t.Fatalf("the database hides editing from someone who edits databases: %v", can)
	}
	keeper.want(403, "PATCH", "/api/v1/pages/"+f.home, map[string]any{"title": "Home renamed"})
	if can := keeper.ok("GET", "/api/v1/pages/"+f.home, nil)["can"].(map[string]any); can[permissions.PageEdit] != false {
		t.Fatalf("a page offers editing to someone who only edits databases: %v", can)
	}
}

func TestTheBuiltInEditorEditsDatabases(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	database := owner.ok("POST", "/api/v1/pages", newItem(space, nil, "database", "Stock"))["id"].(string)
	owner.publish(database, "Stock", 1, sheetWith("tea"))
	editor.ok("POST", "/api/v1/pages/"+database+"/publish", map[string]any{"title": "Stock", "document": sheetWith("cake"), "base_version": 2})
}
