// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/health"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/metrics"
)

func mustMigrations(t *testing.T) []db.Migration {
	t.Helper()
	migs, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	return migs
}

// metricName finds the names of metrics in the monitoring files. Recording rules are named
// with colons (diffluence:...), so they are not taken for metrics the server sends.
var metricName = regexp.MustCompile(`\bdiffluence_[a-z0-9_]+`)

// monitoringMetricNames lists every metric the shipped rules and dashboard use, with the
// suffixes of a histogram's series taken off.
func monitoringMetricNames(t *testing.T) []string {
	t.Helper()
	const dir = "../../deploy/monitoring"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range metricName.FindAllString(string(raw), -1) {
			for _, suffix := range []string{"_bucket", "_sum", "_count"} {
				name = strings.TrimSuffix(name, suffix)
			}
			seen[name] = true
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// fixedRoom is a disk with the room a test gives it.
type fixedRoom struct{ free, total uint64 }

func (f fixedRoom) Space(context.Context) (uint64, uint64, error) { return f.free, f.total, nil }

func healthOf(t *testing.T, root *user) map[string]map[string]any {
	t.Helper()
	var body struct {
		Checks []map[string]any `json:"checks"`
	}
	if err := json.Unmarshal([]byte(root.want(200, "GET", "/api/v1/admin/health", nil)), &body); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]any{}
	for _, c := range body.Checks {
		out[c["name"].(string)] = c
	}
	if len(out) != len(health.Names) {
		t.Fatalf("got checks %v, want one for each of %v", out, health.Names)
	}
	return out
}

// A disk that is nearly full refuses new files with a clear answer, while text still
// saves, and /readyz stays ready with a note (OPS-104, spec 051 D4 and D5).
func TestANearlyFullDiskRefusesFilesButTextStillSaves(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Page", "before")
	if status, body := owner.upload(page, "a.txt", "room enough"); status != 201 {
		t.Fatalf("an upload with room: %d %s", status, body)
	}

	e.app.StoreGuard().UseRoomOf(fixedRoom{free: 1, total: 100})
	status, body := owner.upload(page, "b.txt", "no room")
	if status != http.StatusInsufficientStorage || !strings.Contains(body, `"no_room"`) || !strings.Contains(body, "Text still saves") {
		t.Fatalf("an upload on a full disk: %d %s", status, body)
	}
	owner.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Page", "document": para("after"), "base_version": 1})

	resp, ready := e.do(http.DefaultClient, http.MethodGet, "/readyz", "")
	if resp.StatusCode != http.StatusOK || !strings.Contains(ready, `"disk_low"`) {
		t.Fatalf("a node with a full disk still serves text and says why: %d %s", resp.StatusCode, ready)
	}

	e.app.StoreGuard().UseRoomOf(fixedRoom{free: 2, total: 100})
	if status, body := owner.upload(page, "c.txt", "room again"); status != 201 {
		t.Fatalf("an upload at 2%% free: %d %s", status, body)
	}
	if _, ready := e.do(http.DefaultClient, http.MethodGet, "/readyz", ""); strings.Contains(ready, "notes") {
		t.Fatalf("no note while files are taken: %s", ready)
	}
}

// The health page judges the facts of the installation: a fresh one has no backup, a
// noted backup is seen, and the band of every console page can read the disk (OPS-077,
// OPS-103).
func TestTheHealthPageShowsTheBackupAndTheDisk(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	e.app.StoreGuard().UseRoomOf(fixedRoom{free: 5, total: 100})
	checks := healthOf(t, root)
	if c := checks["backup"]; c["state"] != "warn" || c["code"] != "backup_none" {
		t.Fatalf("a fresh install has no backup: %v", c)
	}
	if c := checks["disk"]; c["state"] != "warn" || c["code"] != "disk_low" {
		t.Fatalf("5%% free warns: %v", c)
	}
	if c := checks["mail"]; c["code"] != "mail_off" {
		t.Fatalf("no mail server is set in tests: %v", c)
	}

	if err := app.NoteBackup(context.Background(), e.app.Pool()); err != nil {
		t.Fatal(err)
	}
	// The answer is kept for 30 seconds, so a fresh reading is asked for directly.
	facts, err := app.ReadHealthFacts(context.Background(), app.HealthSource{Pool: e.app.Pool(), Store: e.app.StoreGuard(),
		Migrations: mustMigrations(t)}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if facts.LastBackup.IsZero() || time.Since(facts.LastBackup) > time.Minute {
		t.Fatalf("the noted backup is not seen: %v", facts.LastBackup)
	}
	if facts.DiskFree != 5 || facts.DiskTotal != 100 {
		t.Fatalf("disk: %d of %d", facts.DiskFree, facts.DiskTotal)
	}
	if facts.LastMigrationAt.IsZero() || facts.PendingContract != 0 {
		t.Fatalf("migrations: %v, %d pending", facts.LastMigrationAt, facts.PendingContract)
	}
}

func TestOnlyAdministratorsSeeTheHealthPage(t *testing.T) {
	e := newEnv(t)
	member := e.user("member@example.com", identity.PermUseProduct)
	member.want(http.StatusForbidden, "GET", "/api/v1/admin/health", nil)
}

// When the reverse proxy is not trusted, everyone logs in from its address. That shows,
// but only for a private address and only when more than one person logged in.
func TestLoginsThatAllComeFromOnePrivateAddressAreSeen(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	src := app.HealthSource{Pool: e.app.Pool(), Migrations: mustMigrations(t)}
	anna := e.user("anna@example.com", identity.PermUseProduct)
	bob := e.user("bob@example.com", identity.PermUseProduct)
	seen := func() string {
		t.Helper()
		facts, err := app.ReadHealthFacts(ctx, src, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return facts.OneLoginAddress
	}
	e.sql("UPDATE sessions SET ip = '172.18.0.1'")
	if got := seen(); got != "172.18.0.1" {
		t.Fatalf("logins of two people from one private address: %q", got)
	}
	e.sql("UPDATE sessions SET ip = '203.0.113.7'")
	if got := seen(); got != "" {
		t.Fatalf("a shared public address is no proxy: %q", got)
	}
	e.sql("UPDATE sessions SET ip = '172.18.0.1' WHERE user_id = $1", anna.id)
	e.sql("UPDATE sessions SET ip = '172.18.0.2' WHERE user_id = $1", bob.id)
	if got := seen(); got != "" {
		t.Fatalf("two addresses: %q", got)
	}
	e.sql("DELETE FROM sessions WHERE user_id = $1", bob.id)
	if got := seen(); got != "" {
		t.Fatalf("one person alone: %q", got)
	}
}

// The size of the database is measured once an hour, and its growth over 30 days comes
// from the measurement nearest to 30 days ago (spec 051, D3).
func TestTheDatabaseGrowthComesFromTheHourlySamples(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	src := app.HealthSource{Pool: e.app.Pool(), Migrations: mustMigrations(t)}
	facts, err := app.ReadHealthFacts(ctx, src, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if facts.DatabaseBytes != 0 || facts.GrowthKnown {
		t.Fatalf("nothing measured yet: %+v", facts)
	}
	e.runTask("sample-database-size")
	if _, err := e.app.Pool().Exec(ctx, `INSERT INTO database_size_samples (taken_at, bytes) VALUES
		(now() - interval '31 days', 1000), (now() - interval '45 days', 10)`); err != nil {
		t.Fatal(err)
	}
	if facts, err = app.ReadHealthFacts(ctx, src, time.Now()); err != nil {
		t.Fatal(err)
	}
	if facts.DatabaseBytes <= 1000 || !facts.GrowthKnown || facts.Growth30d != facts.DatabaseBytes-1000 {
		t.Fatalf("size %d, growth %d (known %v)", facts.DatabaseBytes, facts.Growth30d, facts.GrowthKnown)
	}
}

// What would grow for ever is cleaned daily, and only what is old enough (OPS-105).
func TestTheCleanerRemovesOnlyOldFinishedWork(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	pool := e.app.Pool()
	someone := admin(e).id
	for _, statement := range []string{
		`INSERT INTO jobs (id, kind, status, created_by, finished_at) VALUES
			(gen_random_uuid(), 'x', 'done', '` + someone + `', now() - interval '31 days'),
			(gen_random_uuid(), 'x', 'done', '` + someone + `', now() - interval '29 days'),
			(gen_random_uuid(), 'x', 'failed', '` + someone + `', now() - interval '91 days'),
			(gen_random_uuid(), 'x', 'failed', '` + someone + `', now() - interval '89 days'),
			(gen_random_uuid(), 'x', 'queued', '` + someone + `', NULL)`,
		`INSERT INTO mail_queue (id, user_id, kind, status, created_at) VALUES
			(gen_random_uuid(), '` + someone + `', 'test', 'sent', now() - interval '31 days'),
			(gen_random_uuid(), '` + someone + `', 'test', 'dropped', now() - interval '31 days'),
			(gen_random_uuid(), '` + someone + `', 'test', 'sent', now() - interval '29 days'),
			(gen_random_uuid(), '` + someone + `', 'test', 'failed', now() - interval '91 days'),
			(gen_random_uuid(), '` + someone + `', 'test', 'failed', now() - interval '89 days')`,
		`INSERT INTO nodes (id, role, version, max_migration, last_seen_at) VALUES
			(gen_random_uuid(), 'all', 'old', 1, now() - interval '25 hours'),
			(gen_random_uuid(), 'all', 'recent', 1, now() - interval '23 hours')`,
		`INSERT INTO database_size_samples (taken_at, bytes) VALUES
			(now() - interval '41 days', 1), (now() - interval '39 days', 2)`,
	} {
		if _, err := pool.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	e.runTask("purge-finished-work")
	// Which rows are left, not only how many: a cleaner that removed the young ones and
	// kept the old would leave as many.
	for _, tc := range []struct {
		sql  string
		want int
	}{
		{"SELECT count(*) FROM jobs WHERE kind = 'x' AND status = 'done'", 1},
		{"SELECT count(*) FROM jobs WHERE kind = 'x' AND status = 'done' AND finished_at > now() - interval '30 days'", 1},
		{"SELECT count(*) FROM jobs WHERE kind = 'x' AND status = 'failed'", 1},
		{"SELECT count(*) FROM jobs WHERE kind = 'x' AND status = 'failed' AND finished_at > now() - interval '90 days'", 1},
		{"SELECT count(*) FROM jobs WHERE kind = 'x' AND status = 'queued'", 1},
		{"SELECT count(*) FROM mail_queue WHERE status IN ('sent', 'dropped')", 1},
		{"SELECT count(*) FROM mail_queue WHERE status = 'sent' AND created_at > now() - interval '30 days'", 1},
		{"SELECT count(*) FROM mail_queue WHERE status = 'failed'", 1},
		{"SELECT count(*) FROM mail_queue WHERE status = 'failed' AND created_at > now() - interval '90 days'", 1},
		{"SELECT count(*) FROM nodes WHERE version IN ('old', 'recent')", 1},
		{"SELECT count(*) FROM nodes WHERE version = 'recent'", 1},
		{"SELECT count(*) FROM database_size_samples", 1},
		{"SELECT count(*) FROM database_size_samples WHERE taken_at > now() - interval '40 days'", 1},
	} {
		var n int
		if err := pool.QueryRow(ctx, tc.sql).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != tc.want {
			t.Errorf("%s: %d left, want %d", tc.sql, n, tc.want)
		}
	}
}

// Every metric the shipped rules and dashboard use is one the server sends, so a rename
// breaks this test and not an operator's alert (spec 051, D9).
func TestTheMonitoringFilesUseOnlyMetricsTheServerSends(t *testing.T) {
	e := newEnv(t)
	sent := map[string]bool{}
	var b strings.Builder
	if err := metrics.NewRequests().Write(&b, e.app.Gauges(context.Background())); err != nil {
		t.Fatal(err)
	}
	// A fact that is not known on a test installation is still a metric the server sends.
	known := health.Facts{DiskTotal: 1, DatabaseBytes: 1, BuildDate: time.Now()}
	if err := metrics.NewRequests().Write(&b, app.HealthGauges(known)); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(b.String(), "\n") {
		if name, ok := strings.CutPrefix(line, "# TYPE "); ok {
			sent[strings.Fields(name)[0]] = true
		}
	}
	used := monitoringMetricNames(t)
	if len(used) < 10 {
		t.Fatalf("found only %d metric names in deploy/monitoring: %v", len(used), used)
	}
	for _, name := range used {
		if !sent[name] {
			t.Errorf("deploy/monitoring uses %s, which /metrics does not send", name)
		}
	}
}
