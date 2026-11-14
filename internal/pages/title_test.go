// SPDX-License-Identifier: Apache-2.0

package pages_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/testutil"
)

// Another module that may already show a page reads its title through Title, not the table.
func TestTitleNamesAPageAndFailsForOneThatIsGone(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	space, page, someone := uuid.New(), uuid.New(), uuid.New()
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by)
		VALUES ($1, $1, 'RV5', 'Space', 'active', $2)`, space, someone); err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO pages (id, origin_instance_id, space_id, title, status, created_by, updated_by)
		VALUES ($1, $2, $2, 'Plan for the year', 'published', $3, $3)`, page, space, someone); err != nil {
		t.Fatal(err)
	}
	title, err := pages.Title(ctx, tdb.Pool, page)
	if err != nil || title != "Plan for the year" {
		t.Fatalf("title %q, err %v", title, err)
	}
	if _, err := pages.Title(ctx, tdb.Pool, uuid.New()); err == nil {
		t.Fatal("a page that does not exist has a title")
	}
}
