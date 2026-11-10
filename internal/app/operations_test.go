// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

const metricsToken = "scrape-secret-with-enough-length"

func withMetrics(c *config.Config) { c.MetricsToken = metricsToken }

func TestMetricsDoNotExistWithoutAToken(t *testing.T) {
	e := newEnv(t)
	resp, _ := e.do(e.client(), "GET", "/metrics", "", "Authorization", "Bearer ")
	if resp.StatusCode != 404 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestMetricsNeedTheRightToken(t *testing.T) {
	e := newEnvWith(t, withMetrics)
	for name, headers := range map[string][]string{
		"none":     nil,
		"wrong":    {"Authorization", "Bearer not-the-secret"},
		"longer":   {"Authorization", "Bearer " + metricsToken + "x"},
		"no kind":  {"Authorization", metricsToken},
		"as basic": {"Authorization", "Basic " + metricsToken},
	} {
		if resp, body := e.do(e.client(), "GET", "/metrics", "", headers...); resp.StatusCode != 401 || strings.Contains(body, "diffluence_") {
			t.Errorf("%s: %d %s", name, resp.StatusCode, body)
		}
	}
}

func TestMetricsCountRequestsAndShowWhatWaits(t *testing.T) {
	e := newEnvWith(t, withMetrics)
	e.do(e.client(), "GET", "/livez", "")
	e.do(e.client(), "GET", "/api/v1/nothing-here", "")
	resp, body := e.do(e.client(), "GET", "/metrics", "", "Authorization", "Bearer "+metricsToken)
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain; version=0.0.4") {
		t.Fatalf("%d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	for _, want := range []string{
		`diffluence_http_requests_total{method="GET",status="2xx"}`,
		`diffluence_http_requests_total{method="GET",status="4xx"}`,
		`diffluence_http_request_duration_seconds_bucket{le="+Inf"}`,
		"diffluence_database_up 1", "diffluence_outbox_unpublished ", "diffluence_jobs_queued ", "diffluence_jobs_failed_7d ",
		"diffluence_mail_queued ", "diffluence_storage_deletions_queued ", "diffluence_db_pool_acquired_connections ",
		"diffluence_db_pool_max_connections ", "diffluence_goroutines ", "diffluence_queue_healthy 1",
		"diffluence_migrations_pending_expand 0", "diffluence_migrations_pending_contract 0",
		fmt.Sprintf("diffluence_schema_migration %d\n", newestMigration(t)),
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "nothing-here") {
		t.Fatal("a path reached a label")
	}
}

func TestAFileAnswersNotModifiedButStillChecksAccess(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Plan", "x")
	file := owner.file(page, "plan.txt", "the plan")
	path := "/api/v1/attachments/" + file + "/content"

	resp, body := e.do(reader.c, "GET", path, "")
	etag := resp.Header.Get("ETag")
	if resp.StatusCode != 200 || body != "the plan" || len(etag) != 66 {
		t.Fatalf("%d %q %q", resp.StatusCode, body, etag)
	}
	if got := resp.Header.Get("Cache-Control"); got != "private, no-cache" {
		t.Fatalf("cache: %q", got)
	}
	resp, body = e.do(reader.c, "GET", path, "", "If-None-Match", etag)
	if resp.StatusCode != 304 || body != "" {
		t.Fatalf("%d %q", resp.StatusCode, body)
	}
	if resp.Header.Get("Content-Security-Policy") == "" {
		t.Fatal("a 304 lost its policy")
	}
	owner.restrict(page, "view", owner.id)
	if resp, _ = e.do(reader.c, "GET", path, "", "If-None-Match", etag); resp.StatusCode != 404 {
		t.Fatalf("a kept copy got past a restriction: %d", resp.StatusCode)
	}
}

func TestANewVersionOfAFileChangesItsTag(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Plan", "x")
	file := owner.file(page, "plan.txt", "first")
	path := "/api/v1/attachments/" + file + "/content"
	first, _ := e.do(owner.c, "GET", path, "")
	if status, body := owner.upload(page, "plan.txt", "second"); status != 201 {
		t.Fatalf("%d %s", status, body)
	}
	resp, body := e.do(owner.c, "GET", path, "", "If-None-Match", first.Header.Get("ETag"))
	if resp.StatusCode != 200 || body != "second" {
		t.Fatalf("%d %q", resp.StatusCode, body)
	}
}

func viewedAt(t *testing.T, e *env, user *user, page string) time.Time {
	t.Helper()
	var at time.Time
	err := e.app.Pool().QueryRow(context.Background(), "SELECT viewed_at FROM recent_pages WHERE user_id = $1 AND page_id = $2", user.id, page).Scan(&at)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestARepeatedViewWritesTheRecentListOnceAMinute(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Plan", "x")
	owner.ok("GET", "/api/v1/pages/"+page, nil)
	first := viewedAt(t, e, owner, page)
	owner.ok("GET", "/api/v1/pages/"+page, nil)
	if got := viewedAt(t, e, owner, page); !got.Equal(first) {
		t.Fatal("a second view within a minute wrote again")
	}
	e.sql("UPDATE recent_pages SET viewed_at = now() - interval '2 minutes' WHERE page_id = $1", page)
	old := viewedAt(t, e, owner, page)
	owner.ok("GET", "/api/v1/pages/"+page, nil)
	if got := viewedAt(t, e, owner, page); !got.After(old) {
		t.Fatal("a view after a minute was not written")
	}
}

func TestGoingBackToAPageMovesItUpWithinTheMinute(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	first := owner.page(space, nil, "First", "x")
	second := owner.page(space, nil, "Second", "x")
	for _, page := range []string{first, second, first} {
		owner.ok("GET", "/api/v1/pages/"+page, nil)
	}
	if !viewedAt(t, e, owner, first).After(viewedAt(t, e, owner, second)) {
		t.Fatal("the page last visited is not on top")
	}
}

func TestANewVersionIsSeenAtOnceWithinTheMinute(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Plan", "x")
	owner.ok("GET", "/api/v1/pages/"+page, nil)
	e.sql("UPDATE recent_pages SET seen_version = 0 WHERE page_id = $1", page)
	owner.ok("GET", "/api/v1/pages/"+page, nil)
	if n := e.count("SELECT count(*) FROM recent_pages WHERE page_id = $1 AND seen_version = 0", page); n != 0 {
		t.Fatal("the seen version waited for the minute")
	}
}

func TestTheRecentListIsTrimmedByATaskToFiftyPerPerson(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	for i := range 53 {
		page := owner.page(space, nil, fmt.Sprintf("Page %d", i), "x")
		e.sql("INSERT INTO recent_pages (user_id, page_id, viewed_at) VALUES ($1, $2, now() - make_interval(mins => $3))", owner.id, page, i)
	}
	if n := e.count("SELECT count(*) FROM recent_pages WHERE user_id = $1", owner.id); n != 53 {
		t.Fatalf("before: %d", n)
	}
	if err := e.app.Pages().TrimRecent(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := e.count("SELECT count(*) FROM recent_pages WHERE user_id = $1", owner.id); n != 50 {
		t.Fatalf("after: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM recent_pages r JOIN pages p ON p.id = r.page_id WHERE r.user_id = $1 AND p.title = 'Page 0'`, owner.id); n != 1 {
		t.Fatal("the newest went")
	}
}

func newestMigration(t *testing.T) int {
	t.Helper()
	migs, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	return migs[len(migs)-1].Version
}
