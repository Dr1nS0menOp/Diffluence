// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/identity"
)

const exampleKey = "WELCOME"

func withExample(c *config.Config) { c.ExampleSpace = true }

func (e *env) tick() {
	e.t.Helper()
	e.app.Jobs().Tick(context.Background(), e.app.PeriodicTasks())
}

func (e *env) spaceCount() int {
	e.t.Helper()
	var n int
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM spaces").Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// The lease of a periodic task only opens again after its interval.
func (e *env) tickAgain() {
	e.t.Helper()
	if _, err := e.app.Pool().Exec(context.Background(), "UPDATE job_leases SET last_run = 'epoch'"); err != nil {
		e.t.Fatal(err)
	}
	e.tick()
}

func TestANewInstanceGetsTheExampleSpaceOnceItHasAnAdministrator(t *testing.T) {
	e := newEnvWith(t, withExample)
	e.tick()
	if e.spaceCount() != 0 {
		t.Fatal("nobody can own it yet, so the task must wait")
	}

	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	member := e.user("member@example.com", identity.PermUseProduct)
	e.tickAgain()
	if e.spaceCount() != 1 {
		t.Fatalf("spaces: %d", e.spaceCount())
	}

	space := member.ok("GET", "/api/v1/space-keys/"+exampleKey, nil)
	id := space["id"].(string)
	pages := member.ok("GET", "/api/v1/spaces/"+id+"/tree", nil)["pages"].([]any)
	if len(pages) < 10 {
		t.Fatalf("pages: %d", len(pages))
	}
	if len(member.ok("GET", "/api/v1/spaces/"+id+"/shortcuts", nil)["shortcuts"].([]any)) < 2 {
		t.Fatal("the example should show shortcuts")
	}

	var locked, editable string
	for _, p := range pages {
		row := p.(map[string]any)
		if row["id"] == space["home_page_id"] {
			continue
		}
		if row["restricted"] == true {
			locked = row["id"].(string)
		} else {
			editable = row["id"].(string)
		}
	}
	if locked == "" || editable == "" {
		t.Fatal("the example needs a restricted page beside open ones")
	}
	if member.ok("GET", "/api/v1/pages/"+locked+"/summary", nil)["can"].(map[string]any)["page.edit"] != false {
		t.Fatal("a member must not be able to edit the restricted page")
	}
	if admin.ok("GET", "/api/v1/pages/"+locked+"/summary", nil)["can"].(map[string]any)["page.edit"] != true {
		t.Fatal("the owner must be able to edit it")
	}
	if member.ok("GET", "/api/v1/pages/"+editable+"/summary", nil)["can"].(map[string]any)["page.edit"] != true {
		t.Fatal("all members may edit the example")
	}
	if got := len(member.ok("GET", "/api/v1/pages/"+editable+"/versions", nil)["versions"].([]any)); got != 2 {
		t.Fatalf("a page should have two versions, so history has something to show: %d", got)
	}
	if e.auditActions("space.created") != 1 || e.outboxCount("space.created") != 1 {
		t.Fatal("the example goes through the services, so it is audited")
	}

	var together string
	for _, p := range pages {
		if row := p.(map[string]any); row["title"] == "Working together" {
			together = row["id"].(string)
		}
	}
	if together == "" {
		t.Fatal("the example needs a page about working together")
	}
	list := member.ok("GET", "/api/v1/pages/"+together+"/comments", nil)
	if len(threads(list, "threads")) != 1 || len(threads(list, "inline")) != 1 {
		t.Fatalf("the example shows a comment under the page and one on words: %v", list)
	}
	if len(member.ok("GET", "/api/v1/pages/"+together+"/reactions", nil)["reactions"].([]any)) != 1 {
		t.Fatal("the example shows a reaction")
	}
	if len(admin.ok("GET", "/api/v1/me/tasks?tab=assigned&status=open", nil)["tasks"].([]any)) != 1 {
		t.Fatal("the example gives its owner a task")
	}
	if body := member.want(200, "GET", "/api/v1/pages/"+together, nil); !strings.Contains(body, `"label":"Test admin@example.com"`) {
		t.Fatalf("the example mentions its owner by name: %s", body)
	}

	e.tickAgain()
	if e.spaceCount() != 1 {
		t.Fatal("it must be made once")
	}
}

func TestTheExampleShowsHowASpaceIsKeptInOrder(t *testing.T) {
	e := newEnvWith(t, withExample)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	member := e.user("member@example.com", identity.PermUseProduct)
	e.tick()
	space := member.ok("GET", "/api/v1/space-keys/"+exampleKey, nil)["id"].(string)

	var organised, folder string
	for _, node := range member.ok("GET", "/api/v1/spaces/"+space+"/tree", nil)["pages"].([]any) {
		row := node.(map[string]any)
		if row["title"] == "Keeping things in order" {
			organised = row["id"].(string)
		}
		if row["kind"] == "folder" {
			folder = row["id"].(string)
		}
	}
	if organised == "" || folder == "" {
		t.Fatal("the example needs a page about order, and a folder")
	}
	page := member.ok("GET", "/api/v1/pages/"+organised, nil)
	if len(page["labels"].([]any)) == 0 || page["stage_text"] == "" || page["owner_id"] != admin.id {
		t.Fatalf("the example shows labels, a status and an owner: %v", page)
	}
	if posts := member.ok("GET", "/api/v1/spaces/"+space+"/posts", nil)["posts"].([]any); len(posts) == 0 {
		t.Fatal("the example shows a blog post")
	}
	found := false
	for _, entry := range member.ok("GET", "/api/v1/spaces/"+space+"/templates", nil)["templates"].([]any) {
		found = found || entry.(map[string]any)["name"] == "Team update"
	}
	if !found {
		t.Fatal("the example shows a template of its own")
	}
	if owners := member.ok("GET", "/api/v1/spaces/"+space+"/overview", nil)["owners"].([]any); len(owners) != 1 {
		t.Fatalf("the example says who to ask: %v", owners)
	}
}

func TestADeletedExampleStaysGone(t *testing.T) {
	e := newEnvWith(t, withExample)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	e.tick()
	id := admin.ok("GET", "/api/v1/space-keys/"+exampleKey, nil)["id"].(string)
	admin.ok("DELETE", "/api/v1/spaces/"+id, nil)
	item := admin.ok("GET", "/api/v1/admin/trash/spaces", nil)["items"].([]any)[0].(map[string]any)
	admin.ok("DELETE", "/api/v1/trash/"+item["id"].(string), nil)
	if e.spaceCount() != 0 {
		t.Fatal("the space should be gone")
	}
	e.tickAgain()
	if e.spaceCount() != 0 {
		t.Fatal("a deleted example must not come back")
	}
}

func TestAnInstanceWithSpacesGetsNoExampleByItself(t *testing.T) {
	e := newEnvWith(t, withExample)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	admin.space("ENG")
	e.tick()
	if e.spaceCount() != 1 {
		t.Fatalf("spaces: %d", e.spaceCount())
	}
}

func TestTheSettingTurnsTheExampleOff(t *testing.T) {
	e := newEnv(t)
	e.user("admin@example.com", identity.AllGlobalPermissions...)
	e.tick()
	if e.spaceCount() != 0 {
		t.Fatal("the setting is off, so no example")
	}
}

func TestAnAdministratorCanAddTheExampleLater(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	member := e.user("member@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	admin.space("ENG")

	member.want(403, "POST", "/api/v1/admin/example-space", nil)
	got := admin.ok("POST", "/api/v1/admin/example-space", nil)
	if got["key"] != exampleKey {
		t.Fatalf("answer: %v", got)
	}
	if out := admin.want(409, "POST", "/api/v1/admin/example-space", nil); !strings.Contains(out, "key_taken") {
		t.Fatalf("a second attempt: %s", out)
	}
	if e.spaceCount() != 2 {
		t.Fatalf("spaces: %d", e.spaceCount())
	}
}
