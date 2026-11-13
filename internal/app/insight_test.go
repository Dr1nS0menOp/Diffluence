// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// analyticsTeam is an administrator with analytics on, a space with a page, and a
// reader who may view it.
func analyticsTeam(t *testing.T, e *env, mode string) (admin, reader *user, space, page string) {
	t.Helper()
	admin = e.user("admin@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermAdministrator)
	reader = e.user("reader@example.com", identity.PermUseProduct)
	admin.ok("PUT", "/api/v1/admin/analytics", map[string]any{"enabled": true, "mode": mode, "retention_days": 365})
	space, _ = admin.space("ANA")
	admin.grant(space, "user", reader.id, permissions.TemplateViewer)
	page = admin.page(space, nil, "Handbook", "x")
	return admin, reader, space, page
}

func (e *env) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.app.Pool().QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) viewsOf(page string) int {
	e.t.Helper()
	return e.count("SELECT COALESCE(sum(views), 0)::int FROM page_view_days WHERE page_id = $1", page)
}

func TestWithAnalyticsOffNothingIsRecorded(t *testing.T) {
	e := newEnv(t)
	admin, reader, _, page := analyticsTeam(t, e, "counts")
	admin.ok("PUT", "/api/v1/admin/analytics", map[string]any{"enabled": false, "mode": "counts", "retention_days": 365})
	reader.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})
	if n := e.viewsOf(page) + e.count("SELECT count(*) FROM page_view_seen") + e.count("SELECT count(*) FROM analytics_salts"); n != 0 {
		t.Fatalf("%d traces with analytics off", n)
	}
}

func TestASpaceCanTurnAnalyticsOff(t *testing.T) {
	e := newEnv(t)
	admin, reader, space, page := analyticsTeam(t, e, "counts")
	admin.ok("PUT", "/api/v1/spaces/"+space+"/features", map[string]any{"analytics": false})
	reader.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})
	if e.viewsOf(page) != 0 {
		t.Fatal("a view was counted in a space with analytics off")
	}
}

func TestOwnPageIsNotCountedAndAPersonCountsOncePerHalfHour(t *testing.T) {
	e := newEnv(t)
	admin, reader, _, page := analyticsTeam(t, e, "counts")
	admin.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})
	if e.viewsOf(page) != 0 {
		t.Fatal("the owner's own view was counted")
	}
	reader.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})
	reader.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})
	if got := e.viewsOf(page); got != 1 {
		t.Fatalf("%d views within half an hour", got)
	}
	e.sql("UPDATE page_view_seen SET last_at = last_at - interval '31 minutes'")
	reader.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})
	if got := e.viewsOf(page); got != 2 {
		t.Fatalf("%d views after half an hour", got)
	}
	if got := e.count("SELECT readers FROM page_view_days WHERE page_id = $1", page); got != 1 {
		t.Fatalf("%d readers for one person", got)
	}
}

func TestAPageInAnArchivedSpaceIsNotCounted(t *testing.T) {
	e := newEnv(t)
	_, reader, space, page := analyticsTeam(t, e, "counts")
	e.sql("UPDATE spaces SET status = 'archived' WHERE id = $1", space)
	reader.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})
	if e.viewsOf(page) != 0 {
		t.Fatal("a page in an archived space was counted")
	}
}

func TestSomeoneWhoCannotSeeThePageCannotCountIt(t *testing.T) {
	e := newEnv(t)
	_, _, _, page := analyticsTeam(t, e, "counts")
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	stranger.want(404, "POST", "/api/v1/pages/"+page+"/views", map[string]any{})
}

func TestEngagedCountsOnceAndOnlyAfterAVisit(t *testing.T) {
	e := newEnv(t)
	_, reader, _, page := analyticsTeam(t, e, "counts")
	reader.want(204, "PATCH", "/api/v1/pages/"+page+"/views", nil)
	if e.count("SELECT count(*) FROM page_view_days") != 0 {
		t.Fatal("engaged without a visit counted")
	}
	reader.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})
	reader.want(204, "PATCH", "/api/v1/pages/"+page+"/views", nil)
	reader.want(204, "PATCH", "/api/v1/pages/"+page+"/views", nil)
	if got := e.count("SELECT engaged FROM page_view_days WHERE page_id = $1", page); got != 1 {
		t.Fatalf("engaged %d", got)
	}
}

func TestClosingADayLeavesNothingThatPointsAtAPerson(t *testing.T) {
	e := newEnv(t)
	_, reader, _, page := analyticsTeam(t, e, "counts")
	reader.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})
	e.sql("UPDATE page_view_seen SET day = day - 1")
	e.sql("UPDATE analytics_salts SET day = day - 1")
	e.sql("UPDATE page_view_days SET day = day - 1")
	if err := e.app.CloseAnalyticsDays(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := e.count("SELECT count(*) FROM page_view_seen") + e.count("SELECT count(*) FROM analytics_salts"); n != 0 {
		t.Fatalf("%d traces left", n)
	}
	if e.count("SELECT count(*) FROM page_view_people") != 0 {
		t.Fatal("counts mode stored a name")
	}
	if got := e.count("SELECT active_users FROM analytics_days"); got != 1 {
		t.Fatalf("%d active users", got)
	}
	if e.viewsOf(page) != 1 {
		t.Fatal("the count went with the hashes")
	}
}

func TestRetentionRemovesOldCounts(t *testing.T) {
	e := newEnv(t)
	_, reader, _, page := analyticsTeam(t, e, "counts")
	reader.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})
	e.sql("UPDATE page_view_days SET day = day - 400")
	if err := e.app.CloseAnalyticsDays(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.viewsOf(page) != 0 {
		t.Fatal("a count outlived its retention")
	}
}

func TestLeavingNameModeDeletesNamesForGoodAndIsAudited(t *testing.T) {
	e := newEnv(t)
	admin, reader, _, page := analyticsTeam(t, e, "names")
	reader.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})
	if e.count("SELECT count(*) FROM page_view_people") != 1 {
		t.Fatal("name mode stored no name")
	}
	admin.ok("PUT", "/api/v1/admin/analytics", map[string]any{"enabled": true, "mode": "counts", "retention_days": 365})
	if e.count("SELECT count(*) FROM page_view_people") != 0 {
		t.Fatal("a name survived the stricter mode")
	}
	admin.ok("PUT", "/api/v1/admin/analytics", map[string]any{"enabled": true, "mode": "names", "retention_days": 365})
	if got := admin.ok("GET", "/api/v1/pages/"+page+"/insights", nil)["readers"].([]any); len(got) != 0 {
		t.Fatalf("names came back: %v", got)
	}
	if e.auditActions("analytics.settings_changed") < 3 || e.outboxCount("analytics.settings_changed") < 3 {
		t.Fatal("a change of mode was not recorded")
	}
}

func TestPageViewsStayOutOfTheAuditLog(t *testing.T) {
	e := newEnv(t)
	_, reader, _, page := analyticsTeam(t, e, "names")
	before := e.count("SELECT count(*) FROM audit_entries")
	reader.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})
	if got := e.count("SELECT count(*) FROM audit_entries"); got != before {
		t.Fatal("a page view entered the audit log")
	}
}

func TestOnlyPeopleWithThePermissionSeeInsights(t *testing.T) {
	e := newEnv(t)
	admin, reader, space, page := analyticsTeam(t, e, "counts")
	reader.want(403, "GET", "/api/v1/pages/"+page+"/insights", nil)
	reader.want(403, "GET", "/api/v1/spaces/"+space+"/insights", nil)
	reader.want(403, "GET", "/api/v1/admin/insights", nil)
	if got := reader.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})["views"]; got != nil {
		t.Fatalf("a reader heard the count: %v", got)
	}
	if got := admin.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})["views"]; got != float64(1) {
		t.Fatalf("the administrator heard %v", got)
	}
}

func TestReportsLeaveOutHiddenPages(t *testing.T) {
	e := newEnv(t)
	admin, reader, space, page := analyticsTeam(t, e, "counts")
	analyst := e.user("analyst@example.com", identity.PermUseProduct)
	admin.ok("PUT", "/api/v1/spaces/"+space+"/grants", map[string]any{"subject": map[string]any{"type": "user", "id": analyst.id},
		"permissions": []string{permissions.SpaceView, permissions.AnalyticsView}})
	hidden := admin.page(space, nil, "Salaries", "x")
	reader.ok("POST", "/api/v1/pages/"+hidden+"/views", map[string]any{})
	reader.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{"from": hidden})
	admin.restrict(hidden, "view", admin.id)

	report := analyst.want(200, "GET", "/api/v1/spaces/"+space+"/insights/content", nil)
	if strings.Contains(report, "Salaries") {
		t.Fatal("the content table showed a hidden page")
	}
	if strings.Contains(analyst.want(200, "GET", "/api/v1/pages/"+page+"/insights", nil), "Salaries") {
		t.Fatal("the links showed a hidden page")
	}
	if strings.Contains(analyst.want(200, "GET", "/api/v1/spaces/"+space+"/insights", nil), "Salaries") {
		t.Fatal("popular pages showed a hidden page")
	}
	analyst.want(404, "GET", "/api/v1/pages/"+hidden+"/insights", nil)
}

func TestSearchInsightsHoldNoPeopleAndHideRareTerms(t *testing.T) {
	e := newEnv(t)
	admin, reader, _, _ := analyticsTeam(t, e, "counts")
	for range 3 {
		reader.want(200, "GET", "/api/v1/search?q=handbook", nil)
	}
	reader.want(200, "GET", "/api/v1/search?q=jansen", nil)
	got := admin.want(200, "GET", "/api/v1/admin/insights/search", nil)
	if !strings.Contains(got, "handbook") {
		t.Fatalf("a common term is missing: %s", got)
	}
	if strings.Contains(got, "jansen") {
		t.Fatal("a rare term was shown")
	}
}

func TestEveryInsightsExportIsGuarded(t *testing.T) {
	e := newEnv(t)
	admin, _, space, _ := analyticsTeam(t, e, "counts")
	admin.page(space, nil, `=HYPERLINK("x")`, "x")
	got := admin.want(200, "GET", "/api/v1/spaces/"+space+"/insights/content?format=csv", nil)
	if !strings.Contains(got, `"'=HYPERLINK(""x"")"`) {
		t.Fatalf("the cell was not guarded: %s", got)
	}
}

func TestAnAuditExportUsesTheOneGuard(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermAdministrator)
	space, _ := admin.space("CSV")
	admin.page(space, nil, "＝1+1", "x")
	if got := admin.want(200, "GET", "/api/v1/admin/audit-events/export", nil); strings.Contains(got, ",＝1+1") {
		t.Fatalf("a full-width start went through: %s", got)
	}
}

func TestDeletingViewDataIsAudited(t *testing.T) {
	e := newEnv(t)
	admin, reader, _, page := analyticsTeam(t, e, "names")
	reader.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})
	admin.want(204, "DELETE", "/api/v1/admin/analytics/data", nil)
	if e.viewsOf(page)+e.count("SELECT count(*) FROM page_view_people") != 0 {
		t.Fatal("view data stayed")
	}
	if e.auditActions("analytics.data_deleted") != 1 || e.outboxCount("analytics.data_deleted") != 1 {
		t.Fatal("deleting was not recorded")
	}
}

func TestPeopleOnAPageSeeEachOther(t *testing.T) {
	e := newEnv(t)
	admin, reader, _, page := analyticsTeam(t, e, "counts")
	reader.ok("PUT", "/api/v1/pages/"+page+"/presence", map[string]any{"mode": "reading"})
	got := admin.ok("PUT", "/api/v1/pages/"+page+"/presence", map[string]any{"mode": "editing"})
	people := got["people"].([]any)
	if len(people) != 1 || people[0].(map[string]any)["id"] != reader.id || people[0].(map[string]any)["mode"] != "reading" {
		t.Fatalf("got %v", people)
	}
	got = reader.ok("PUT", "/api/v1/pages/"+page+"/presence", map[string]any{"mode": "reading"})
	if got["people"].([]any)[0].(map[string]any)["mode"] != "editing" {
		t.Fatalf("got %v", got)
	}
}

func TestPresenceEndsAfterAMinute(t *testing.T) {
	e := newEnv(t)
	admin, reader, _, page := analyticsTeam(t, e, "counts")
	reader.ok("PUT", "/api/v1/pages/"+page+"/presence", map[string]any{"mode": "reading"})
	e.sql("UPDATE page_presence SET seen_at = seen_at - interval '61 seconds'")
	if got := admin.ok("PUT", "/api/v1/pages/"+page+"/presence", map[string]any{"mode": "reading"})["people"].([]any); len(got) != 0 {
		t.Fatalf("someone stayed: %v", got)
	}
}

func TestSomeoneWhoTurnedPresenceOffIsNotShown(t *testing.T) {
	e := newEnv(t)
	admin, reader, _, page := analyticsTeam(t, e, "counts")
	prefs := reader.ok("GET", "/api/v1/me/preferences", nil)
	prefs["show_presence"] = false
	reader.ok("PUT", "/api/v1/me/preferences", prefs)
	reader.ok("PUT", "/api/v1/pages/"+page+"/presence", map[string]any{"mode": "reading"})
	if got := admin.ok("PUT", "/api/v1/pages/"+page+"/presence", map[string]any{"mode": "reading"})["people"].([]any); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestSomeoneWhoLostThePageLeavesPresenceAtOnce(t *testing.T) {
	e := newEnv(t)
	admin, reader, _, page := analyticsTeam(t, e, "counts")
	reader.ok("PUT", "/api/v1/pages/"+page+"/presence", map[string]any{"mode": "reading"})
	admin.restrict(page, "view", admin.id)
	if got := admin.ok("PUT", "/api/v1/pages/"+page+"/presence", map[string]any{"mode": "reading"})["people"].([]any); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	reader.want(404, "PUT", "/api/v1/pages/"+page+"/presence", map[string]any{"mode": "reading"})
}

func TestTheStampChangesWithComments(t *testing.T) {
	e := newEnv(t)
	admin, reader, _, page := analyticsTeam(t, e, "counts")
	before := reader.ok("PUT", "/api/v1/pages/"+page+"/presence", map[string]any{"mode": "reading"})["stamp"]
	admin.comment(page, "hello")
	if reader.ok("PUT", "/api/v1/pages/"+page+"/presence", map[string]any{"mode": "reading"})["stamp"] == before {
		t.Fatal("the stamp stayed the same")
	}
}

func TestAnAdministratorCanTurnPresenceOff(t *testing.T) {
	e := newEnv(t)
	admin, reader, _, page := analyticsTeam(t, e, "counts")
	body := admin.ok("GET", "/api/v1/admin/settings/features", nil)
	body["presence"] = false
	admin.ok("PUT", "/api/v1/admin/settings/features", body)
	reader.ok("PUT", "/api/v1/pages/"+page+"/presence", map[string]any{"mode": "reading"})
	if e.count("SELECT count(*) FROM page_presence") != 0 {
		t.Fatal("presence was stored while off")
	}
}

func TestAScopeLimitsATokenAndATokenNeverExceedsItsOwner(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermAdministrator)
	plain := e.user("plain@example.com", identity.PermUseProduct)
	space, _ := admin.space("TOK")
	page := admin.page(space, nil, "Plan", "x")
	token := func(u *user, scopes ...string) string {
		return u.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "t", "scopes": scopes})["secret"].(string)
	}
	read := token(admin, "read")
	if status, _ := e.bearer("GET", "/api/v1/pages/"+page, read, ""); status != 200 {
		t.Fatalf("read got %d", status)
	}
	if status, _ := e.bearer("PATCH", "/api/v1/pages/"+page, read, `{"title":"y"}`); status != 403 {
		t.Fatalf("a read token wrote: %d", status)
	}
	if status, _ := e.bearer("GET", "/api/v1/spaces/"+space+"/export.zip", read, ""); status != 403 {
		t.Fatalf("a read token exported: %d", status)
	}
	if status, _ := e.bearer("GET", "/api/v1/admin/analytics", read, ""); status != 403 {
		t.Fatalf("a read token reached admin: %d", status)
	}
	if status, _ := e.bearer("GET", "/api/v1/spaces/"+space+"/export.zip", token(admin, "read", "export"), ""); status != 200 {
		t.Fatalf("an export token got %d", status)
	}
	if status, _ := e.bearer("GET", "/api/v1/admin/analytics", token(admin, "admin"), ""); status != 200 {
		t.Fatalf("an admin token got %d", status)
	}
	if status, _ := e.bearer("GET", "/api/v1/admin/analytics", token(plain, "admin"), ""); status != 403 {
		t.Fatalf("a token exceeded its owner: %d", status)
	}
	if e.auditActions("token.refused") < 3 {
		t.Fatal("refusals were not audited")
	}
	admin.want(400, "POST", "/api/v1/me/tokens", map[string]any{"name": "t", "scopes": []string{"everything"}})
}

// The scope follows the route, not words in the address: a label that happens to be
// called export is read with the read scope, and a real export still needs its own.
func TestTheScopeOfATokenFollowsTheRouteNotTheAddress(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("TOK")
	read := owner.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "t", "scopes": []string{"read"}})["secret"].(string)
	if status, out := e.bearer("GET", "/api/v1/labels/export/pages", read, ""); status != 200 {
		t.Fatalf("a read token could not read the label export: %d %s", status, out)
	}
	if status, _ := e.bearer("GET", "/api/v1/labels/x-export/pages", read, ""); status != 200 {
		t.Fatalf("a read token could not read the label x-export: %d", status)
	}
	if status, _ := e.bearer("GET", "/api/v1/pages/"+uuid.NewString()+"/export.md", read, ""); status != 403 {
		t.Fatalf("a read token reached a page export: %d", status)
	}
	if status, _ := e.bearer("GET", "/api/v1/export-formats", read, ""); status != 403 {
		t.Fatalf("a read token listed export formats: %d", status)
	}
	if status, _ := e.bearer("GET", "/api/v1/spaces/"+space+"/export.zip", read, ""); status != 403 {
		t.Fatalf("a read token exported a space: %d", status)
	}
}

// Every route that exports, now and later, needs more than the read scope: a new export
// route that is missing from the list fails here.
func TestEveryExportRouteNeedsMoreThanTheReadScope(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct)
	read := owner.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "t", "scopes": []string{"read", "write"}})["secret"].(string)
	checked := 0
	for _, r := range e.app.Routes() {
		method, path, _ := strings.Cut(r.Pattern, " ")
		if !strings.Contains(path, "export") || strings.HasPrefix(path, "/api/v1/admin/") {
			continue
		}
		path = strings.ReplaceAll(path, "{id}", uuid.NewString())
		if status, out := e.bearer(method, path, read, ""); status != 403 {
			t.Errorf("%s with a read and write token: %d %s", r.Pattern, status, out)
		}
		checked++
	}
	if checked < 5 {
		t.Fatalf("only %d export routes", checked)
	}
}

func TestTheUseOfATokenIsRecordedOnceAMinute(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.PermUseProduct)
	secret := admin.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "t"})["secret"].(string)
	for range 3 {
		if status, _ := e.bearer("GET", "/api/v1/me", secret, ""); status != 200 {
			t.Fatalf("status %d", status)
		}
	}
	if got := e.auditActions("token.used"); got != 1 {
		t.Fatalf("%d records", got)
	}
}

func TestOutgoingHostsStartEmptyAndChangesAreAudited(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.PermUseProduct, identity.PermAdministrator)
	if got := admin.ok("GET", "/api/v1/admin/outgoing-hosts", nil)["hosts"].([]any); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	admin.ok("PUT", "/api/v1/admin/outgoing-hosts", map[string]any{"hosts": []string{"Hooks.Example.com"}})
	if got := admin.ok("GET", "/api/v1/admin/outgoing-hosts", nil)["hosts"].([]any); len(got) != 1 || got[0] != "hooks.example.com" {
		t.Fatalf("got %v", got)
	}
	if e.auditActions("outward.hosts_changed") != 1 {
		t.Fatal("not audited")
	}
	admin.want(400, "PUT", "/api/v1/admin/outgoing-hosts", map[string]any{"hosts": []string{"a/b"}})
}
