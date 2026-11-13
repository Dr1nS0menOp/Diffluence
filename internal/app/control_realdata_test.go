// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// realDataControlBudget is what spec 064 allows on the seeded data of the stress test, with a
// tenth of the pages under control: a controlled page within 5 ms of an ordinary one, and a
// step of a workflow within 20 ms.
const (
	controlledReadExtra = 5 * time.Millisecond
	transitionBudget    = 20 * time.Millisecond
)

// markControlled puts about a tenth of the pages of every space under a workflow: most of
// them approved, some never approved and a few withdrawn, so the rule of view has rows to
// read. It runs once; a dump that has control already keeps it.
func markControlled(t *testing.T, e *env) {
	t.Helper()
	if e.count("SELECT count(*) FROM page_control") > 0 {
		return
	}
	e.sql(`INSERT INTO workflows (id, space_id, name, created_by)
		SELECT gen_random_uuid(), s.id, 'Review', s.created_by FROM spaces s`)
	e.sql(`INSERT INTO workflow_versions (workflow_id, version, definition, saved_by)
		SELECT w.id, 1, $1::jsonb, w.created_by FROM workflows w`, realDataFlow)
	e.sql(`INSERT INTO page_control (page_id, workflow_id, workflow_version, state_id, state_kind, approved_version,
			approved_state_id, approved_at, head_version, governed_by, governed_from)
		SELECT p.id, w.id, 1,
			CASE WHEN h < 80 THEN 'approved' WHEN h < 95 THEN 'draft' ELSE 'withdrawn' END,
			CASE WHEN h < 80 THEN 'approved' WHEN h < 95 THEN 'draft' ELSE 'obsolete' END,
			CASE WHEN h < 80 OR h >= 95 THEN p.current_version END,
			CASE WHEN h < 80 OR h >= 95 THEN 'approved' END,
			CASE WHEN h < 80 OR h >= 95 THEN now() END,
			p.current_version, 'page', p.id
		FROM (SELECT p.*, abs(hashtext(p.id::text)) % 100 AS h, abs(hashtext(p.id::text || 'c')) % 10 AS pick FROM pages p) p
		JOIN workflows w ON w.space_id = p.space_id
		WHERE p.pick = 0 AND p.kind IN ('page', 'post', 'database') AND p.status = 'published'`)
	e.sql("ANALYZE page_control")
}

const realDataFlow = `{"states": [
	{"id": "draft", "name": "Draft", "kind": "draft", "start": true},
	{"id": "review", "name": "In review", "kind": "in_review"},
	{"id": "approved", "name": "Approved", "kind": "approved"},
	{"id": "withdrawn", "name": "Withdrawn", "kind": "obsolete"}],
 "transitions": [
	{"id": "submit", "name": "Send for review", "from": "draft", "to": "review"},
	{"id": "approve", "name": "Approve", "from": "review", "to": "approved"},
	{"id": "withdraw", "name": "Withdraw", "from": "approved", "to": "withdrawn"}]}`

// medianOf runs a request seven times and gives the median.
func medianOf(u *user, method, path string, body any) time.Duration {
	u.e.t.Helper()
	took := make([]time.Duration, 0, 7)
	for range 7 {
		started := time.Now()
		if status, out := u.do(method, path, body); status != 200 {
			u.e.t.Fatalf("%s %s: %d %s", method, path, status, out)
		}
		took = append(took, time.Since(started))
	}
	sort.Slice(took, func(i, j int) bool { return took[i] < took[j] })
	return took[len(took)/2]
}

// TestDocumentControlOnRealData measures spec 064 on a restored dump of the seeded data
// (scripts/stress/real-data.sh). It migrates the dump, puts a tenth of the pages under
// control, and times reads and steps through the whole application.
func TestDocumentControlOnRealData(t *testing.T) {
	realURL := os.Getenv("DIFFLUENCE_REALDATA_URL")
	if realURL == "" {
		t.Skip("DIFFLUENCE_REALDATA_URL is not set; see scripts/stress/real-data.sh")
	}
	public, _ := url.Parse("http://localhost")
	cfg := config.Config{Role: config.RoleAll, HTTPAddr: "127.0.0.1:0", PublicURL: public, InstanceName: "Real data",
		DatabaseURL: realURL, DataDir: t.TempDir(), MigrateOnStart: true, SessionIdleTimeout: time.Hour, SessionMaxAge: 24 * time.Hour}
	a, err := app.New(context.Background(), cfg, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() { srv.Close(); a.Close() })
	e := &env{t: t, app: a, srv: srv}
	markControlled(t, e)

	stamp := time.Now().Format("150405")
	author := e.user("dc1-author-"+stamp+"@example.com", identity.PermUseProduct)
	reader := e.user("dc1-reader-"+stamp+"@example.com", identity.PermUseProduct)
	space := e.scalar(`SELECT space_id::text FROM pages GROUP BY space_id ORDER BY count(*) DESC LIMIT 1`)
	e.sql("SELECT rbac_set_exact($1::uuid, 'user', $2::uuid, $3::text[])", space, author.id, permissions.Templates[permissions.TemplateEditor])
	e.sql("SELECT rbac_set_exact($1::uuid, 'user', $2::uuid, $3::text[])", space, reader.id, permissions.Templates[permissions.TemplateViewer])
	pick := func(controlled string) string {
		return e.scalar(`SELECT p.id::text FROM pages p LEFT JOIN page_control pc ON pc.page_id = p.id
			WHERE p.space_id = $1 AND p.kind = 'page' AND p.status = 'published' AND p.archived_at IS NULL
			  AND NOT EXISTS (SELECT 1 FROM page_restrictions r JOIN page_tree t ON t.ancestor_id = r.page_id WHERE t.descendant_id = p.id)
			  AND (pc.page_id IS NULL) = ($2 = 'no') AND ($2 = 'no' OR pc.state_kind = 'approved')
			ORDER BY p.id LIMIT 1`, space, controlled)
	}
	plain, controlled := pick("no"), pick("yes")
	for _, u := range []*user{reader, author} {
		ordinary := medianOf(u, "GET", "/api/v1/pages/"+plain, nil)
		governed := medianOf(u, "GET", "/api/v1/pages/"+controlled, nil)
		t.Logf("%s: open an ordinary page %v, a controlled page %v", u.email[:10], ordinary.Round(time.Microsecond*100), governed.Round(time.Microsecond*100))
		if governed > ordinary+controlledReadExtra {
			t.Errorf("%s: a controlled page takes %v, an ordinary one %v", u.email, governed, ordinary)
		}
	}
	t.Logf("tree of the largest space %v", medianOf(reader, "GET", "/api/v1/spaces/"+space+"/tree/branches", nil).Round(time.Millisecond))
	t.Logf("search on a common word %v", medianOf(reader, "GET", "/api/v1/search?q=plan", nil).Round(time.Millisecond))
	t.Logf("feed, recent %v", medianOf(reader, "GET", "/api/v1/feed?tab=recent", nil).Round(time.Millisecond))

	// A step: publish a change on approved pages, send it for review, and time the approval,
	// which makes the change current, search and links included.
	rows, err := a.Pool().Query(context.Background(), `SELECT pc.page_id::text FROM page_control pc JOIN pages p ON p.id = pc.page_id
		WHERE p.space_id = $1 AND p.kind = 'page' AND pc.state_kind = 'approved' AND p.archived_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM page_restrictions r WHERE r.page_id = p.id) ORDER BY p.id LIMIT 7`, space)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	var steps []time.Duration
	for _, id := range ids {
		page := author.ok("GET", "/api/v1/pages/"+id, nil)
		author.ok("POST", "/api/v1/pages/"+id+"/publish", map[string]any{"title": page["title"], "document": para("changed for review"),
			"base_version": page["current_version"]})
		head := author.ok("GET", "/api/v1/pages/"+id, nil)["control"].(map[string]any)["head_version"]
		author.ok("POST", "/api/v1/pages/"+id+"/transitions", map[string]any{"transition": "submit", "version": head})
		started := time.Now()
		author.ok("POST", "/api/v1/pages/"+id+"/transitions", map[string]any{"transition": "approve", "version": head})
		steps = append(steps, time.Since(started))
	}
	sort.Slice(steps, func(i, j int) bool { return steps[i] < steps[j] })
	median := steps[len(steps)/2]
	t.Logf("%d approvals: median %v, slowest %v", len(steps), median.Round(time.Microsecond*100), steps[len(steps)-1].Round(time.Microsecond*100))
	if median > transitionBudget {
		t.Errorf("an approval takes %v, budget %v", median, transitionBudget)
	}
}
