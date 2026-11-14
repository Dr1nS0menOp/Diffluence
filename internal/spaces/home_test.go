// SPDX-License-Identifier: Apache-2.0

package spaces_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/spaces"
	"github.com/diffluence/diffluence/internal/testutil"
)

// Another module finds a space's home page through HomePage, not the table.
func TestHomePageIsNilUntilASpaceHasOne(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	space, home, someone := uuid.New(), uuid.New(), uuid.New()
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by)
		VALUES ($1, $1, 'RV5', 'Space', 'active', $2)`, space, someone); err != nil {
		t.Fatal(err)
	}
	got, err := spaces.HomePage(ctx, tdb.Pool, space)
	if err != nil || got != nil {
		t.Fatalf("a space without a home page: %v, err %v", got, err)
	}
	if _, err := tdb.Pool.Exec(ctx, "UPDATE spaces SET home_page_id = $2 WHERE id = $1", space, home); err != nil {
		t.Fatal(err)
	}
	got, err = spaces.HomePage(ctx, tdb.Pool, space)
	if err != nil || got == nil || *got != home {
		t.Fatalf("home page %v, err %v", got, err)
	}
	if _, err := spaces.HomePage(ctx, tdb.Pool, uuid.New()); err == nil {
		t.Fatal("a space that does not exist has an answer")
	}
}
