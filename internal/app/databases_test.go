// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"strings"
	"testing"
)

func sheetWith(word string) map[string]any {
	return map[string]any{"type": "doc", "content": []any{map[string]any{"type": "sheet", "attrs": map[string]any{
		"columns": []any{map[string]any{"id": "a", "name": "Item", "type": "text"}},
		"rows":    []any{map[string]any{"id": "r1", "cells": map[string]any{"a": word}}},
	}}}}
}

func TestADocumentMustFitItsKind(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	base := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Sums", "kind": "database"})
	id := base["id"].(string)
	page := owner.page(space, nil, "Words", "text")

	owner.want(400, "POST", "/api/v1/pages/"+id+"/publish", map[string]any{"title": "Sums", "document": para("x"), "base_version": 1})
	owner.want(400, "PUT", "/api/v1/pages/"+id+"/draft", map[string]any{"title": "Sums", "document": para("x"), "base_version": 1})
	owner.want(400, "POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Words", "document": sheetWith("x"), "base_version": 1})
	owner.want(400, "PUT", "/api/v1/pages/"+page+"/draft", map[string]any{"title": "Words", "document": sheetWith("x"), "base_version": 1})
}

func TestADatabaseStaysADatabaseAndNothingBecomesOne(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	id := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Sums", "kind": "database"})["id"].(string)
	page := owner.page(space, nil, "Words", "text")
	owner.want(409, "POST", "/api/v1/pages/"+id+"/kind", map[string]any{"kind": "page"})
	owner.want(400, "POST", "/api/v1/pages/"+page+"/kind", map[string]any{"kind": "database"})
}

func TestADatabaseOnlyOpensForThoseWhoMaySeeIt(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	id := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Salaries", "kind": "database"})["id"].(string)
	owner.publish(id, "Salaries", 1, sheetWith("zebrafish"))
	viewer.want(403, "POST", "/api/v1/pages/"+id+"/publish", map[string]any{"title": "Salaries", "document": sheetWith("x"), "base_version": 2})
	if out := viewer.want(200, "GET", "/api/v1/search?q=zebrafish", nil); !strings.Contains(out, "Salaries") {
		t.Fatalf("the cell is not found: %s", out)
	}
	owner.restrict(id, "view", owner.id)
	viewer.want(404, "GET", "/api/v1/pages/"+id, nil)
	if out := viewer.want(200, "GET", "/api/v1/search?q=zebrafish", nil); strings.Contains(out, "Salaries") {
		t.Fatalf("a hidden cell is found: %s", out)
	}
}

func TestADatabaseIsAnItemLikeAnyOther(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	folder := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Group", "kind": "folder"})["id"].(string)
	created := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "parent_id": folder, "title": "Sums", "kind": "database"})
	if created["status"] != "published" {
		t.Fatalf("a database has its grid at once: %v", created)
	}
	id := created["id"].(string)
	if doc := mustJSON(t, owner.ok("GET", "/api/v1/pages/"+id, nil)["document"]); !strings.Contains(doc, `"sheet"`) {
		t.Fatalf("no sheet: %s", doc)
	}
	if tree := owner.want(200, "GET", "/api/v1/spaces/"+space+"/tree", nil); !strings.Contains(tree, "Sums") {
		t.Fatalf("not in the tree: %s", tree)
	}
	before := e.auditActions("page.published")
	owner.publish(id, "Sums", 1, sheetWith("tea"))
	if e.auditActions("page.published") != before+1 {
		t.Fatal("a change is not audited")
	}
	copied := owner.ok("POST", "/api/v1/pages/"+id+"/copy", map[string]any{})
	if copied["kind"] != "database" {
		t.Fatalf("a copy of a database is a database: %v", copied)
	}
	owner.ok("POST", "/api/v1/pages/"+id+"/versions/1/restore", map[string]any{})
	if doc := mustJSON(t, owner.ok("GET", "/api/v1/pages/"+id, nil)["document"]); strings.Contains(doc, "tea") {
		t.Fatalf("the old version is back: %s", doc)
	}
}

func TestATemplateCannotHoldASheet(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	id := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Sums", "kind": "database"})["id"].(string)
	owner.want(400, "POST", "/api/v1/pages/"+id+"/save-as-template", map[string]any{"name": "Sheet"})
}
