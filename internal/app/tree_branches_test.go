// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

func branchesPath(space, query string) string {
	return "/api/v1/spaces/" + space + "/tree/branches" + query
}

// row finds a page by title in the answer of a branch.
func row(m map[string]any, title string) map[string]any {
	for _, r := range m["pages"].([]any) {
		if entry := r.(map[string]any); entry["title"] == title {
			return entry
		}
	}
	return nil
}

func TestAHiddenChildLeavesNoTraceInTheTree(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	parent := owner.page(space, nil, "Parent", "x")
	secret := owner.page(space, parent, "Merger with Initech", "x")
	owner.restrict(secret, "view", owner.id)

	start := viewer.ok("GET", branchesPath(space, ""), nil)
	if r := row(start, "Parent"); r == nil || r["has_children"] != false {
		t.Fatalf("a hidden child shows as a branch: %v", r)
	}
	for _, path := range []string{
		branchesPath(space, ""),
		branchesPath(space, "?parent="+parent),
		branchesPath(space, "?around="+secret),
		"/api/v1/spaces/" + space + "/tree/find?q=merger",
		"/api/v1/spaces/" + space + "/tree/find?view=alphabet",
	} {
		if got := viewer.want(200, "GET", path, nil); strings.Contains(got, "Merger") || strings.Contains(got, secret) {
			t.Fatalf("%s gives away a hidden page: %s", path, got)
		}
	}
	if r := row(owner.ok("GET", branchesPath(space, ""), nil), "Parent"); r["has_children"] != true {
		t.Fatalf("the owner sees the branch: %v", r)
	}
}

func TestABranchBelowARestrictionStaysClosed(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	top := owner.page(space, nil, "Closed", "x")
	below := owner.page(space, top, "Below", "x")
	owner.page(space, below, "Further down", "x")
	owner.restrict(top, "view", owner.id)
	if got := viewer.want(200, "GET", branchesPath(space, "?parent="+below), nil); strings.Contains(got, "Further") {
		t.Fatalf("asking for a hidden branch by its id: %s", got)
	}
}

func TestTheTreeRoutesNeedTheSpace(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	page := owner.page(space, nil, "Page", "x")
	for _, path := range []string{
		branchesPath(space, ""),
		branchesPath(space, "?parent="+page),
		branchesPath(space, "?around="+page),
		"/api/v1/spaces/" + space + "/tree/find?q=page",
	} {
		stranger.want(404, "GET", path, nil)
	}
}

func TestAPageUnderAnArchivedParentStandsAtTheTop(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	parent := owner.page(space, nil, "Gone", "x")
	owner.page(space, parent, "Left behind", "x")
	owner.ok("POST", "/api/v1/pages/"+parent+"/archive", map[string]any{"include_children": false})
	start := viewer.ok("GET", branchesPath(space, ""), nil)
	if row(start, "Left behind") == nil || row(start, "Gone") != nil {
		t.Fatalf("the top of the tree: %v", titlesIn(start, "pages"))
	}
}

func TestAPageOpensWithTheWayToIt(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	one := owner.page(space, nil, "One", "x")
	two := owner.page(space, one, "Two", "x")
	three := owner.page(space, two, "Three", "x")
	owner.page(space, two, "Sibling", "x")
	four := owner.page(space, three, "Four", "x")
	around := viewer.ok("GET", branchesPath(space, "?around="+four), nil)
	for _, title := range []string{"One", "Two", "Three", "Four", "Sibling"} {
		if row(around, title) == nil {
			t.Fatalf("%s is missing: %v", title, titlesIn(around, "pages"))
		}
	}
}

func TestALargeLevelComesInParts(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	parent := owner.page(space, nil, "Parent", "x")
	e.sql(`INSERT INTO pages (id, origin_instance_id, space_id, parent_id, position, title, status, current_version, created_by, updated_by, kind)
		SELECT gen_random_uuid(), p.origin_instance_id, p.space_id, p.id, n, 'Child ' || n, 'published', 1, p.created_by, p.updated_by, 'page'
		FROM pages p, generate_series(1, 450) n WHERE p.id = $1`, parent)
	first := viewer.ok("GET", branchesPath(space, "?parent="+parent), nil)
	if got := len(first["pages"].([]any)); got != 200 {
		t.Fatalf("first part: %d rows", got)
	}
	more := first["more"].([]any)
	if len(more) != 1 || more[0].(map[string]any)["parent_id"] != parent || more[0].(map[string]any)["offset"].(float64) != 200 {
		t.Fatalf("what goes on: %v", more)
	}
	last := viewer.ok("GET", branchesPath(space, fmt.Sprintf("?parent=%s&offset=%d", parent, 400)), nil)
	if got := len(last["pages"].([]any)); got != 50 || len(last["more"].([]any)) != 0 {
		t.Fatalf("last part: %d rows, %v", got, last["more"])
	}
}

func TestALargeSpaceOpensWithItsTopAlone(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	parent := owner.page(space, nil, "Parent", "x")
	e.sql(`INSERT INTO pages (id, origin_instance_id, space_id, parent_id, position, title, status, current_version, created_by, updated_by, kind)
		SELECT gen_random_uuid(), p.origin_instance_id, p.space_id, NULL, n, 'Top ' || n, 'published', 1, p.created_by, p.updated_by, 'page'
		FROM pages p, generate_series(1, 300) n WHERE p.id = $1`, parent)
	start := viewer.ok("GET", branchesPath(space, ""), nil)
	if got := len(start["pages"].([]any)); got != 200 {
		t.Fatalf("rows at the start: %d", got)
	}
	next := start["more"].([]any)[0].(map[string]any)
	if next["parent_id"] != nil {
		t.Fatalf("the top level goes on: %v", next)
	}
	rest := viewer.ok("GET", branchesPath(space, "?offset=200"), nil)
	if got := len(rest["pages"].([]any)); got == 0 {
		t.Fatal("the rest of the top level is missing")
	}
}

func TestTheFilterLooksPastWhatTheTreeHolds(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	one := owner.page(space, nil, "One", "x")
	two := owner.page(space, one, "Two", "x")
	three := owner.page(space, two, "Three", "x")
	match := owner.page(space, three, "Deep quartermaster", "x")
	found := viewer.ok("GET", "/api/v1/spaces/"+space+"/tree/find?q=QUARTERMASTER", nil)
	if m := found["matches"].([]any); len(m) != 1 || m[0] != match {
		t.Fatalf("matches: %v", m)
	}
	for _, title := range []string{"One", "Two", "Three", "Deep quartermaster"} {
		if row(found, title) == nil {
			t.Fatalf("%s is missing, so the match loses its place: %v", title, titlesIn(found, "pages"))
		}
	}
}

func TestFlatViewsOrderThePagesOnTheServer(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	owner.grant(space, "user", viewer.id, permissions.TemplateViewer)
	zebra := owner.page(space, nil, "Zebra", "x")
	apple := owner.page(space, nil, "Apple", "x")
	alphabet := viewer.ok("GET", "/api/v1/spaces/"+space+"/tree/find?view=alphabet", nil)["matches"].([]any)
	if alphabet[0] != apple {
		t.Fatalf("alphabet: %v", alphabet)
	}
	viewer.ok("GET", "/api/v1/pages/"+zebra, nil)
	visited := viewer.ok("GET", "/api/v1/spaces/"+space+"/tree/find?view=visited", nil)["matches"].([]any)
	if len(visited) != 1 || visited[0] != zebra {
		t.Fatalf("visited: %v", visited)
	}
}
