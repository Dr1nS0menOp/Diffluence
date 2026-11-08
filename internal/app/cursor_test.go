// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/notify"
)

// Notifications that share a time must all be reached, page by page, with the exact cursor.
func TestPagingNotificationsWithACursorSkipsNothingThatSharesATime(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner, _, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	want := map[string]bool{}
	for i := 0; i < 3; i++ {
		id := uuid.New()
		want[id.String()] = true
		_, err := e.app.Pool().Exec(ctx, `INSERT INTO notifications
			(id, user_id, type, reason, page_id, space_id, group_key, source_event, read_at, created_at)
			VALUES ($1, $2, $3, 'watching', $4, $5, $6, $7, now(), '2026-01-02T03:04:05Z')`,
			id, viewer.id, notify.Types[0], page, space, fmt.Sprintf("g%d", i), uuid.New())
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	query := "limit=1"
	for range 5 {
		got := viewer.ok("GET", "/api/v1/me/notifications?"+query, nil)
		list, _ := got["notifications"].([]any)
		if len(list) == 0 {
			break
		}
		item := list[0].(map[string]any)
		seen[item["id"].(string)] = true
		query = "limit=1&before=" + url.QueryEscape(item["at"].(string)) + "&before_id=" + item["id"].(string)
	}
	for id := range want {
		if !seen[id] {
			t.Fatalf("a notification was skipped: saw %v of %v", seen, want)
		}
	}

	// before_id alone is refused.
	viewer.want(400, "GET", "/api/v1/me/notifications?before_id="+uuid.New().String(), nil)
}
