// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/health"
)

// Draft and published spaces, wave PB4 of spec 073: the consistency check runs by itself once a
// week and once after the part is turned on again, its result is a line of the health page, and a
// button there fixes what it found (PUB-343, PUB-1305).

// publishingLine is the health line of draft and published spaces, read fresh.
func publishingLine(t *testing.T, e *env) health.Check {
	t.Helper()
	facts, err := e.app.ReadFreshHealthFacts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range health.Evaluate(facts) {
		if c.Name == "publishing" {
			return c
		}
	}
	t.Fatalf("the health page has no line for draft and published spaces")
	return health.Check{}
}

// TestTurningThePartOffAndOnAgainKeepsThePairsAndChecksThemOnce is PUB-1304 and PUB-1305: an off
// part keeps its pairs as they are, turning it on again brings them back with their statuses as
// stored, and the consistency check runs once then; otherwise it runs once a week.
func TestTurningThePartOffAndOnAgainKeepsThePairsAndChecksThemOnce(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "TOGGLE")
	page := w.page(w.writer, w.draftHome, "Guide", "first words")
	w.release(w.admin, page, "")
	w.edit(w.writer, page, "Guide", para("second words"))
	twin := w.twin(page)
	selfCheck := func() {
		t.Helper()
		if err := e.app.Publishing().SelfCheck(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	selfCheck()
	selfCheck()
	if n := e.auditActions("publishing.checked"); n != 1 {
		t.Fatalf("the check ran %d times within its week", n)
	}
	if line := publishingLine(t, e); line.State != health.OK || line.Code != "publishing_ok" {
		t.Fatalf("the line after a clean check: %+v", line)
	}
	root := admin(e)
	all := []string{"automation", "bundles", "console_restore", "databases", "publishing"}
	root.ok("PUT", "/api/v1/admin/experimental", map[string]any{"on": all[:4]})
	selfCheck()
	if n := e.auditActions("publishing.checked"); n != 1 {
		t.Fatalf("the check ran while the part was off")
	}
	root.ok("PUT", "/api/v1/admin/experimental", map[string]any{"on": all})
	// Back as it was: the pair, the managed space, and the page still changed since its release.
	if got := w.status(w.admin, page); got["status"] != "changed" {
		t.Fatalf("the status after turning on again: %v", got)
	}
	w.admin.want(http.StatusConflict, "POST", "/api/v1/pages/"+twin+"/publish", map[string]any{"title": "Guide", "document": para("by hand"), "base_version": 1})
	selfCheck()
	selfCheck()
	if n := e.auditActions("publishing.checked"); n != 2 {
		t.Fatalf("turning the part on again ran the check %d times, want once", n-1)
	}
}

// TestTheHealthPageShowsWhatTheCheckFoundAndItsButtonFixesIt is PUB-343: a stored fact that
// drifted is a warning on the health page, and the button there recomputes it.
func TestTheHealthPageShowsWhatTheCheckFoundAndItsButtonFixesIt(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "HCHECK")
	page := w.page(w.writer, w.draftHome, "Guide", "first words")
	w.release(w.admin, page, "")
	if _, err := e.app.Pool().Exec(context.Background(), `UPDATE page_pairs SET changed = '{title}', changed_since = now() WHERE draft_page_id = $1`, page); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Publishing().SelfCheck(context.Background()); err != nil {
		t.Fatal(err)
	}
	line := publishingLine(t, e)
	if line.State != health.Warn || line.Code != "publishing_drift" || line.Values["count"] != 1 {
		t.Fatalf("the line after a drift: %+v", line)
	}
	if got := w.status(w.admin, page); got["status"] != "changed" {
		t.Fatalf("the check that only looks fixed it: %v", got)
	}
	root := admin(e)
	w.admin.want(http.StatusForbidden, "POST", "/api/v1/admin/publishing/check", map[string]any{"fix": true})
	out := root.want(http.StatusOK, "POST", "/api/v1/admin/publishing/check", map[string]any{"fix": true})
	if !strings.Contains(out, `"found":1`) || !strings.Contains(out, `"fixed":true`) {
		t.Fatalf("the button: %s", out)
	}
	if got := w.status(w.admin, page); got["status"] != "released" {
		t.Fatalf("the button did not fix the status: %v", got)
	}
	if line := publishingLine(t, e); line.Code != "publishing_ok" {
		t.Fatalf("the line after the fix: %+v", line)
	}
}
