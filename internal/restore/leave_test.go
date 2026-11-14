// SPDX-License-Identifier: Apache-2.0

package restore_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/restore"
	"github.com/diffluence/diffluence/internal/testutil"
)

// A node that stops starting says so at once, so a restore need not wait for it to go quiet.
func TestALeavingNodeNoLongerCountsAsBusy(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	leaving, staying := uuid.New(), uuid.New()
	for _, node := range []uuid.UUID{leaving, staying} {
		if err := restore.Seen(ctx, tdb.Pool, node, restore.ModeStarting); err != nil {
			t.Fatal(err)
		}
	}
	if err := restore.Leave(ctx, tdb.Pool, leaving); err != nil {
		t.Fatal(err)
	}
	busy, err := restore.Busy(ctx, tdb.Pool)
	if err != nil || busy != 1 {
		t.Fatalf("busy %d, err %v", busy, err)
	}
}
