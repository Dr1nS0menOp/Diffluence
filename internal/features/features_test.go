// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/testutil"
)

// A new space is made inside a transaction that already holds a connection. Start must read
// the instance defaults through that transaction: asking the pool for a second connection
// waits forever once every connection is held by such a transaction (spec 050). A pool of one
// connection makes that wait certain, where a larger pool hides it behind luck.
func TestStartingASpaceReadsTheDefaultsThroughItsOwnTransaction(t *testing.T) {
	tdb := testutil.InstalledDatabase(t)
	joiner := "?"
	if strings.Contains(tdb.URL, "?") {
		joiner = "&"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := db.Open(ctx, tdb.URL+joiner+"pool_max_conns=1")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	svc := features.NewService(pool, settings.NewService(pool, nil))

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	waitLimit, stopWaiting := context.WithTimeout(ctx, 3*time.Second)
	defer stopWaiting()
	if err := svc.Start(waitLimit, tx, uuid.New()); err != nil {
		t.Fatalf("starting a space waited for a second connection: %v", err)
	}
}
