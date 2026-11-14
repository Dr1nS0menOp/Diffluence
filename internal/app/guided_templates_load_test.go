// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"
)

// medianUse makes pages from a template n times and gives the median and the slowest.
func medianUse(u *user, n int, path string, body func(i int) map[string]any, want int) (median, slowest time.Duration) {
	u.e.t.Helper()
	took := make([]time.Duration, 0, n)
	for i := range n {
		started := time.Now()
		if status, out := u.do("POST", path, body(i)); status != want {
			u.e.t.Fatalf("%s: %d %s", path, status, out)
		}
		took = append(took, time.Since(started))
	}
	sort.Slice(took, func(i, j int) bool { return took[i] < took[j] })
	return took[len(took)/2], took[len(took)-1]
}

// TestASinglePageFromATemplateCostsWhatItDidBefore times today's way of starting one
// page, the case every set must stay within 10% of (TPL-901). It runs only when asked,
// outside the race detector, which makes every timing meaningless.
func TestASinglePageFromATemplateCostsWhatItDidBefore(t *testing.T) {
	if os.Getenv("DIFFLUENCE_LOAD_BUDGET") == "" {
		t.Skip("DIFFLUENCE_LOAD_BUDGET is not set")
	}
	e := newEnv(t)
	owner, _, _, space := e.team("LOAD")
	tpl := owner.ok("POST", "/api/v1/spaces/"+space+"/templates", map[string]any{"name": "Note", "labels": []string{"note"},
		"title_pattern": "Note on {Topic}", "document": fieldDocument("Topic", true)})
	path := "/api/v1/templates/" + tpl["id"].(string) + "/use"
	body := func(int) map[string]any {
		return map[string]any{"space_id": space, "answers": map[string]string{"Topic": "budget"}}
	}
	medianUse(owner, 5, path, body, 201) // warm up
	median, slowest := medianUse(owner, 31, path, body, 201)
	t.Logf("one page from a template: median %v, slowest %v", median.Round(10*time.Microsecond), slowest.Round(10*time.Microsecond))
}

// p95 gives the 95th percentile of sorted timings.
func p95(took []time.Duration) time.Duration {
	sort.Slice(took, func(i, j int) bool { return took[i] < took[j] })
	return took[(len(took)*95+99)/100-1]
}

// TestTheLoadBudgetOfASet measures TPL-901 to TPL-903 on generated data: a set of 10 pages
// within 300 ms at p95, of 100 pages within 3 s as background work, and the audit queue
// empty within 5 s of a 100-page set. Run it outside the race detector.
func TestTheLoadBudgetOfASet(t *testing.T) {
	if os.Getenv("DIFFLUENCE_LOAD_BUDGET") == "" {
		t.Skip("DIFFLUENCE_LOAD_BUDGET is not set")
	}
	e := newEnv(t)
	owner, _, _, space := e.team("LOAD")
	template := func(name string, pages int) string {
		tpl := owner.ok("POST", "/api/v1/spaces/"+space+"/templates", map[string]any{"name": name, "title_pattern": "{Topic}",
			"labels": []string{"set"}, "document": fieldDocument("Topic", true)})["id"].(string)
		for i := range pages - 1 {
			owner.entry(tpl, map[string]any{"name": "Page " + strconv.Itoa(i), "parent": "root", "labels": []string{"set"},
				"document": textWith(words("Part of "), valueOf("set"), words(" on "), valueOf("date"))})
		}
		return tpl
	}
	body := func(i int) map[string]any {
		return map[string]any{"space_id": space, "answers": map[string]string{"Topic": "run " + strconv.Itoa(i)}}
	}

	ten := template("Ten", 10)
	medianUse(owner, 3, "/api/v1/templates/"+ten+"/sets", body, 201) // warm up
	took := make([]time.Duration, 0, 40)
	for i := range 40 {
		started := time.Now()
		if status, out := owner.makeSet(ten, body(i)); status != 201 {
			t.Fatalf("set of 10: %d %v", status, out)
		}
		took = append(took, time.Since(started))
	}
	tenP95 := p95(took)
	t.Logf("a set of 10 pages: p95 %v, median %v", tenP95.Round(time.Millisecond), took[len(took)/2].Round(time.Millisecond))
	if tenP95 > 300*time.Millisecond {
		t.Errorf("a set of 10 pages takes %v at p95, budget 300 ms", tenP95)
	}

	hundred := template("Hundred", 100)
	took = took[:0]
	var sealed time.Duration
	for i := range 7 {
		status, out := owner.makeSet(hundred, body(i))
		if status != 202 {
			t.Fatalf("set of 100: %d %v", status, out)
		}
		started := time.Now()
		if _, err := e.app.Jobs().RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
		took = append(took, time.Since(started))
		if got := e.scalar("SELECT status FROM jobs WHERE id = $1", out["job_id"]); got != "done" {
			t.Fatalf("job: %s", got)
		}
		// The sealer runs every second; it must empty the queue of about 101 events within 5 s.
		started = time.Now()
		for e.auditWaiting() > 0 {
			if err := e.app.SealAudit(context.Background()); err != nil {
				t.Fatal(err)
			}
		}
		sealed = max(sealed, time.Since(started))
	}
	hundredP95 := p95(took)
	t.Logf("a set of 100 pages in the background: p95 %v, median %v; sealing its audit events took at most %v",
		hundredP95.Round(time.Millisecond), took[len(took)/2].Round(time.Millisecond), sealed.Round(time.Millisecond))
	if hundredP95 > 3*time.Second {
		t.Errorf("a set of 100 pages takes %v at p95, budget 3 s", hundredP95)
	}
	if sealed > 4*time.Second {
		t.Errorf("sealing a 100-page set took %v; with the one-second tick that is over the 5 s budget", sealed)
	}
}
