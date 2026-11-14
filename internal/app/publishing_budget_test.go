// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// The load budget of wave PB2 (spec 073, PUB-1103 to PUB-1106), measured on generated data:
// a whole-space release of 2,000 pages in batches and of 1,000 pages in one transaction, the
// release screen at 5,000 pages, a preview of 2,000 pages and a second run with nothing to do.
// The bounds checked here are wide, for a loaded test machine; the numbers go in the report.

// manyDraftPages adds sections of pages below the home page of a draft space, with one version
// each, faster than the API could: sections times perSection pages, plus the sections.
func (w pubWorld) manyDraftPages(prefix string, sections, perSection int) {
	w.e.t.Helper()
	doc := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` +
		"A page of the handbook with a few sentences of ordinary text, as a page of a guide has. " +
		"It explains one step, names the people to ask, and says what to do when the step fails." + `"}]}]}`
	insert := func(parentFilter string, count int) {
		w.e.sql(`WITH made AS (
				INSERT INTO pages (id, origin_instance_id, space_id, parent_id, position, title, status, current_version,
					created_by, updated_by, owner_id, kind, published_at)
				SELECT gen_random_uuid(), p.origin_instance_id, p.space_id, p.id, n, $3 || ' ' || p.position || '.' || n, 'published', 1,
					p.created_by, p.created_by, p.created_by, 'page', now()
				FROM pages p, generate_series(1, $2) n WHERE `+parentFilter+`
				RETURNING id, parent_id)
			INSERT INTO page_tree (ancestor_id, descendant_id, depth)
			SELECT t.ancestor_id, m.id, t.depth + 1 FROM made m JOIN page_tree t ON t.descendant_id = m.parent_id
			UNION ALL SELECT m.id, m.id, 0 FROM made m`, w.draftHome, count, prefix)
	}
	insert("p.id = $1", sections)
	insert(fmt.Sprintf("p.parent_id = $1 AND p.title LIKE '%s %%'", prefix), perSection)
	w.e.sql(`INSERT INTO page_versions (id, origin_instance_id, page_id, number, title, document, schema_version, author_id, summary)
		SELECT gen_random_uuid(), p.origin_instance_id, p.id, 1, p.title, $2::jsonb,
			(SELECT schema_version FROM page_versions WHERE page_id = $1 LIMIT 1), p.created_by, ''
		FROM pages p WHERE p.space_id = (SELECT space_id FROM pages WHERE id = $1) AND p.current_version = 1
		  AND NOT EXISTS (SELECT 1 FROM page_versions v WHERE v.page_id = p.id)`, w.draftHome, doc)
}

func (w pubWorld) timedRun(scope runBody) (preview, run time.Duration, out map[string]any) {
	w.e.t.Helper()
	start := time.Now()
	p := w.previewRun(w.admin, scope)
	preview = time.Since(start)
	id := p["id"].(string)
	w.admin.want(http.StatusAccepted, "POST", "/api/v1/spaces/"+w.draft+"/releases/"+id+"/start", map[string]any{"note": "measured"})
	start = time.Now()
	w.e.runJobs()
	run = time.Since(start)
	return preview, run, w.admin.ok("GET", "/api/v1/spaces/"+w.draft+"/releases/"+id, nil)
}

func TestAWholeSpaceReleaseFitsItsBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("a measurement")
	}
	e := newEnv(t)
	w := newPubWorld(e, "LOAD")
	w.manyDraftPages("Section", 40, 49) // 40 sections of 49 pages and the home page: 2,001 pages
	pages := e.count("SELECT count(*) FROM pages WHERE space_id = $1", w.draft)
	preview, run, out := w.timedRun(runBody{"scope": "space"})
	if out["status"] != "done" || out["batches"].(float64) < 1 {
		t.Fatalf("a whole-space release of %d pages: %v", pages, out)
	}
	rate := float64(pages) / run.Seconds()
	t.Logf("whole space, %d pages in %d batches: preview %v, run %v, %.0f pages a second", pages, int(out["batches"].(float64)), preview, run, rate)
	if rate < 10 {
		t.Fatalf("a whole-space release ran at %.0f pages a second", rate)
	}
	// A second run with nothing to do answers at once (PUB-1108: within 2 s).
	start := time.Now()
	w.admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+w.draft+"/releases/preview", runBody{"scope": "space"})
	nothing := time.Since(start)
	t.Logf("a second whole-space preview with nothing to do: %v", nothing)
	// The release screen at 5,000 pages, 3,000 of them not released yet (PUB-1103: 100 ms).
	w.manyDraftPages("Annex", 60, 49)
	total := e.count("SELECT count(*) FROM pages WHERE space_id = $1", w.draft)
	var screens []time.Duration
	for range 10 {
		start := time.Now()
		w.admin.ok("GET", "/api/v1/spaces/"+w.draft+"/releases", nil)
		screens = append(screens, time.Since(start))
	}
	t.Logf("the release screen at %d pages over HTTP: first %v, best %v", total, screens[0], minDuration(screens))
	if best := minDuration(screens); best > 2*time.Second {
		t.Fatalf("the release screen took %v at %d pages", best, total)
	}

	// One transaction of 1,000 pages in a pair of its own (PUB-450, PUB-1106: 30 s at most).
	one := newPubWorld(e, "LOADONE")
	one.manyDraftPages("Part", 20, 48) // 980 pages, with the sections and the home page 1,001 at most
	count := e.count("SELECT count(*) FROM pages WHERE space_id = $1", one.draft)
	preview, run, out = one.timedRun(runBody{"scope": "space"})
	if out["status"] != "done" || out["batches"] != 0.0 {
		t.Fatalf("a release of %d pages in one transaction: %v", count, out)
	}
	t.Logf("whole space, %d pages in one transaction: preview %v, run %v, %.0f pages a second", count, preview, run, float64(count)/run.Seconds())
}

func minDuration(list []time.Duration) time.Duration {
	best := list[0]
	for _, d := range list[1:] {
		best = min(best, d)
	}
	return best
}
