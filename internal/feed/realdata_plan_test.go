// SPDX-License-Identifier: Apache-2.0

package feed

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
)

// TestTheFeedKeepsItsHashOnRealData is DOC-1301: with document control on a tenth of the
// pages, the recent feed still builds the visible set once, as a hash, and still walks the
// versions through their index rather than reading them all. It runs only against a restored dump of the seeded data
// (scripts/stress/real-data.sh) after the pages are put under control.
func TestTheFeedKeepsItsHashOnRealData(t *testing.T) {
	url := os.Getenv("DIFFLUENCE_REALDATA_URL")
	if url == "" {
		t.Skip("DIFFLUENCE_REALDATA_URL is not set; see scripts/stress/real-data.sh")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var migrated bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('page_control') IS NOT NULL").Scan(&migrated); err != nil {
		t.Fatal(err)
	}
	if !migrated {
		t.Skip("the dump is from before spec 064: run TestDocumentControlOnRealData first")
	}
	var controlled int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM page_control").Scan(&controlled); err != nil {
		t.Fatal(err)
	}
	if controlled == 0 {
		t.Skip("no page is controlled yet: run TestDocumentControlOnRealData first")
	}
	var member uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT u.id FROM users u
		WHERE NOT EXISTS (SELECT 1 FROM global_grants g WHERE g.subject_id = u.id AND g.permission = 'administrator')
		ORDER BY (SELECT count(*) FROM group_members gm WHERE gm.user_id = u.id) DESC, u.id LIMIT 1`).Scan(&member); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, "EXPLAIN (ANALYZE, COSTS OFF) "+changesSQL, member, nil, nil, defaultItems, excerptChars, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	plan := strings.Join(lines, "\n")
	t.Logf("%d controlled pages; the recent feed of the member:\n%s", controlled, plan)
	if !strings.Contains(plan, "hashed SubPlan") && !strings.Contains(plan, "Hash") {
		t.Error("the visible set is no longer built once as a hash")
	}
	if strings.Contains(plan, "Seq Scan on page_versions") {
		t.Error("the versions are read whole, not newest first through their index")
	}
	compareWithFormerFeed(t, pool, member)
}

// formerSQL is a feed query as it was before spec 064, which never asked for the current
// version: the same database and moment then show what document control costs.
func formerSQL(query string) string {
	query = strings.Replace(query, " AND "+readable("v"), "", 1)
	query = strings.Replace(query, "\n\t\t\tAND "+readable("n")+")", ")", 1)
	query = strings.Replace(query, "\n\t\tJOIN pages cp ON cp.id = k.page_id", "", 1)
	return strings.Replace(query, " AND n.number <= cp.current_version", "", 1)
}

// compareWithFormerFeed times the recent and "for you" feeds against their former queries,
// alternating, so the cost of document control shows at one moment on one machine. Spec 064
// allows the budgets of spec 061 plus a tenth.
func compareWithFormerFeed(t *testing.T, pool *pgxpool.Pool, member uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	var watcher uuid.UUID
	if err := pool.QueryRow(ctx, "SELECT user_id FROM watches GROUP BY 1 ORDER BY count(*) DESC LIMIT 1").Scan(&watcher); err != nil {
		t.Fatal(err)
	}
	run := func(query string, who uuid.UUID, forYou bool) time.Duration {
		started := time.Now()
		rows, err := pool.Query(ctx, query, who, nil, nil, defaultItems, excerptChars, forYou, nil)
		if err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if rows.Err() != nil {
			t.Fatal(rows.Err())
		}
		return time.Since(started)
	}
	for _, c := range []struct {
		name   string
		query  string
		forYou bool
		budget time.Duration
	}{{"recent", changesSQL, false, 150 * time.Millisecond}, {"for you", forYouSQL, true, 500 * time.Millisecond}} {
		if former := formerSQL(c.query); former == c.query || strings.Contains(former, "current_version") {
			t.Fatalf("the former %s query could not be rebuilt", c.name)
		}
		for who, id := range map[string]uuid.UUID{"member": member, "watcher": watcher} {
			var now, before []time.Duration
			for range 5 {
				before = append(before, run(formerSQL(c.query), id, c.forYou))
				now = append(now, run(c.query, id, c.forYou))
			}
			slices.Sort(now)
			slices.Sort(before)
			t.Logf("%-7s %-7s before %v, now %v", c.name, who, before[2].Round(time.Millisecond), now[2].Round(time.Millisecond))
			if now[2] > c.budget+c.budget/10 {
				t.Errorf("%s, %s: %v now, %v before, over the budget of %v plus a tenth", c.name, who, now[2], before[2], c.budget)
			}
		}
	}
}
