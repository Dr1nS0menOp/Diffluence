// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/notify"
	"github.com/diffluence/diffluence/internal/permissions"
)

func deliver(t *testing.T, e *env) {
	t.Helper()
	if err := e.app.DeliverEvents(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func notificationCount(e *env, u *user, kind string) int {
	return e.count("SELECT count(*) FROM notifications WHERE user_id = $1 AND type = $2", u.id, kind)
}

func TestTheOwnerHearsWhenAnAdministratorRecoversTheirPage(t *testing.T) {
	e := newEnv(t)
	root := e.user("root@example.com", identity.PermUseProduct, identity.PermAdministrator, identity.PermCreateSpace)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Private", "x")
	owner.restrict(page, "view", owner.id)
	root.want(204, "POST", "/api/v1/admin/pages/"+page+"/recover", map[string]any{"reason": "audit request"})
	deliver(t, e)
	if notificationCount(e, owner, notify.TypeAccessRecovered) != 1 {
		t.Fatal("the owner was not told")
	}
	if notificationCount(e, root, notify.TypeAccessRecovered) != 0 {
		t.Fatal("the wrong person was told")
	}
}

func TestRecoveringASpaceTellsNoPageOwner(t *testing.T) {
	e := newEnv(t)
	root := e.user("root@example.com", identity.PermUseProduct, identity.PermAdministrator, identity.PermCreateSpace)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	root.want(204, "POST", "/api/v1/admin/spaces/"+space+"/recover", map[string]any{"reason": "nobody left"})
	deliver(t, e)
	if notificationCount(e, owner, notify.TypeAccessRecovered) != 0 {
		t.Fatal("told")
	}
}

func TestAWarningGoesOutBeforeADraftIsRemoved(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Plan", "x")
	owner.saveDraft(page, "Plan", 1, "half done")
	e.sql("UPDATE drafts SET updated_at = now() - interval '355 days' WHERE page_id = $1", page)
	if err := e.app.Pages().ExpireDrafts(context.Background()); err != nil {
		t.Fatal(err)
	}
	deliver(t, e)
	if notificationCount(e, owner, notify.TypeDraftExpiring) != 1 {
		t.Fatal("no warning")
	}
}

func TestARestrictedPageWithNobodyLeftIsShownWithoutItsTitle(t *testing.T) {
	e := newEnv(t)
	root := e.user("root@example.com", identity.PermUseProduct, identity.PermAdministrator, identity.PermCreateSpace)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	writer := e.user("writer@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", writer.id, permissions.TemplateSpaceAdmin)
	alone := writer.page(space, nil, "Salaries", "x")
	shared := writer.page(space, nil, "Shared", "x")
	writer.restrict(alone, "view", writer.id)
	writer.restrict(shared, "view", writer.id, owner.id)

	if strings.Contains(root.want(200, "GET", "/api/v1/admin/attention", nil), "restrictions_orphaned") {
		t.Fatal("too early")
	}
	root.ok("POST", "/api/v1/admin/users/"+writer.id+"/disable", nil)
	got := root.ok("GET", "/api/v1/admin/attention", nil)
	pages := got["orphaned_pages"].([]any)
	if len(pages) != 1 || pages[0].(map[string]any)["id"] != alone {
		t.Fatalf("%v", pages)
	}
	if body := root.want(200, "GET", "/api/v1/admin/attention", nil); strings.Contains(body, "Salaries") {
		t.Fatal("the title leaked")
	}
	root.want(204, "POST", "/api/v1/admin/pages/"+alone+"/recover", map[string]any{"reason": "the writer left"})
	if pages := root.ok("GET", "/api/v1/admin/attention", nil)["orphaned_pages"].([]any); len(pages) != 0 {
		t.Fatalf("still listed: %v", pages)
	}
}

func TestAProfileListsMentionsTheViewerMaySee(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermViewProfiles)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	viewer := e.user("viewer@example.com", identity.PermUseProduct, identity.PermViewProfiles)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", viewer.id, permissions.TemplateViewer)
	open := owner.page(space, nil, "Open note", "x")
	hidden := owner.page(space, nil, "Hidden note", "x")
	for _, page := range []string{open, hidden} {
		owner.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Note " + page, "base_version": 1, "document": mentionDoc(anna.id)})
	}
	owner.restrict(hidden, "view", owner.id)
	got := viewer.want(200, "GET", "/api/v1/people/"+anna.id+"/mentions", nil)
	if !strings.Contains(got, open) || strings.Contains(got, hidden) {
		t.Fatal(got)
	}
	if got := owner.want(200, "GET", "/api/v1/people/"+anna.id+"/mentions", nil); !strings.Contains(got, hidden) {
		t.Fatal(got)
	}
	nobody := e.user("nobody@example.com", identity.PermUseProduct)
	nobody.want(404, "GET", "/api/v1/people/"+anna.id+"/mentions", nil)
}

func TestFollowersAreListedUnlessThePersonHidesThem(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct, identity.PermViewProfiles)
	bob := e.user("bob@example.com", identity.PermUseProduct, identity.PermViewProfiles)
	bob.want(204, "PUT", "/api/v1/users/"+anna.id+"/follow", nil)
	if got := bob.want(200, "GET", "/api/v1/people/"+anna.id+"/follows", nil); !strings.Contains(got, bob.id) {
		t.Fatal(got)
	}
	choices := anna.ok("GET", "/api/v1/me/preferences", nil)
	choices["hide_follows"] = true
	anna.ok("PUT", "/api/v1/me/preferences", choices)
	if got := bob.want(200, "GET", "/api/v1/people/"+anna.id+"/follows", nil); strings.Contains(got, bob.id) {
		t.Fatal(got)
	}
	if got := anna.want(200, "GET", "/api/v1/people/"+anna.id+"/follows", nil); !strings.Contains(got, bob.id) {
		t.Fatal("they cannot see their own")
	}
}

func TestTheConsoleShowsTotalsPerSpaceAndTheLargestPages(t *testing.T) {
	e := newEnv(t)
	root := e.user("root@example.com", identity.PermUseProduct, identity.PermAdministrator, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := root.space("ENG")
	page := root.page(space, nil, "Secret title", strings.Repeat("words ", 200))
	root.file(page, "a.txt", "12345")
	reader.want(403, "GET", "/api/v1/admin/spaces", nil)
	var mine map[string]any
	for _, row := range root.ok("GET", "/api/v1/admin/spaces", nil)["totals"].([]any) {
		if row.(map[string]any)["id"] == space {
			mine = row.(map[string]any)
		}
	}
	if mine == nil || mine["pages"].(float64) < 1 || mine["bytes"].(float64) != 5 || mine["last_activity"] == nil {
		t.Fatalf("%v", mine)
	}
	body := root.want(200, "GET", "/api/v1/admin/reports/largest", nil)
	if !strings.Contains(body, "ENG/"+page) || strings.Contains(body, "Secret title") {
		t.Fatal(body)
	}
}
