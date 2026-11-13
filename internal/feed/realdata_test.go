// SPDX-License-Identifier: Apache-2.0

package feed_test

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/feed"
	"github.com/diffluence/diffluence/internal/identity"
)

// realDataBudget is the most one feed may take on the seeded data of the stress test
// (5,000 people, 60,000 pages), with nothing else running (spec 061). The worst person
// there watches every page. Popular counts a whole week of activity, 32,000 comments
// on that data, so it gets more.
var realDataBudget = map[string]time.Duration{
	feed.TabRecent:  150 * time.Millisecond,
	feed.TabForYou:  500 * time.Millisecond,
	feed.TabPopular: 250 * time.Millisecond,
}

// TestTheFeedIsFastOnRealData runs only against a restored dump of the seeded data
// (scripts/stress/real-data.sh). Generated data in a test database is too small and too
// even to show what the planner does with real data, which is how a feed of 20 ms in
// development became one of minutes on the server.
func TestTheFeedIsFastOnRealData(t *testing.T) {
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
	people := map[string]string{
		// An ordinary member in the most groups, so of many spaces; and the person who
		// watches the most pages, on the seeded data the administrator, who sees them all.
		"member": `SELECT u.id FROM users u
			WHERE NOT EXISTS (SELECT 1 FROM global_grants g WHERE g.subject_id = u.id AND g.permission = 'administrator')
			ORDER BY (SELECT count(*) FROM group_members gm WHERE gm.user_id = u.id) DESC, u.id LIMIT 1`,
		"watcher": `SELECT user_id FROM watches GROUP BY 1 ORDER BY count(*) DESC LIMIT 1`,
	}
	service := feed.NewService(pool)
	for who, pick := range people {
		var id uuid.UUID
		if err := pool.QueryRow(ctx, pick).Scan(&id); err != nil {
			t.Fatalf("%s: %v", who, err)
		}
		for _, tab := range []string{feed.TabRecent, feed.TabForYou, feed.TabPopular} {
			took := make([]time.Duration, 0, 5)
			for range 5 {
				started := time.Now()
				if _, err := service.List(ctx, identity.Caller{UserID: id}, feed.Query{Tab: tab}); err != nil {
					t.Fatalf("%s, %s: %v", who, tab, err)
				}
				took = append(took, time.Since(started))
			}
			sort.Slice(took, func(i, j int) bool { return took[i] < took[j] })
			median := took[len(took)/2]
			t.Logf("%-8s %-8s median %v, slowest %v", who, tab, median.Round(time.Millisecond), took[len(took)-1].Round(time.Millisecond))
			if median > realDataBudget[tab] {
				t.Errorf("%s, %s: median %v, budget %v", who, tab, median, realDataBudget[tab])
			}
		}
	}
}
