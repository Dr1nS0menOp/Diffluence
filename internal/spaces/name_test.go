// SPDX-License-Identifier: Apache-2.0

package spaces_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/spaces"
	"github.com/diffluence/diffluence/internal/testutil"
)

// Another module reads the name of a space through Name, not the table.
func TestNameGivesTheNameOfASpaceAndNotFoundForNone(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	space, someone := uuid.New(), uuid.New()
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by)
		VALUES ($1, $1, 'GT1', 'Engineering', 'active', $2)`, space, someone); err != nil {
		t.Fatal(err)
	}
	if got, err := spaces.Name(ctx, tdb.Pool, space); err != nil || got != "Engineering" {
		t.Fatalf("name %q, err %v", got, err)
	}
	_, err := spaces.Name(ctx, tdb.Pool, uuid.New())
	if e, ok := apperr.As(err); !ok || e.Kind != apperr.KindNotFound {
		t.Fatalf("a space that does not exist: %v", err)
	}
}
