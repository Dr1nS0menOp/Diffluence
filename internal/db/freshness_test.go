// SPDX-License-Identifier: Apache-2.0

package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func pgxStart() pgx.TraceQueryStartData { return pgx.TraceQueryStartData{SQL: "SELECT 1"} }

func TestAReadIsCurrentUnlessMarkedAndCurrentWinsInside(t *testing.T) {
	ctx := context.Background()
	if FreshnessOf(ctx) != Current {
		t.Fatal("an unmarked read must be current")
	}
	lag := WithMayLag(ctx)
	if FreshnessOf(lag) != MayLag {
		t.Fatal("a marked read may lag")
	}
	// A permission check inside a list must still see the latest commit.
	if FreshnessOf(MustBeCurrent(lag)) != Current {
		t.Fatal("MustBeCurrent must win inside a request that may lag")
	}
	if MustBeCurrent(ctx) != ctx {
		t.Fatal("MustBeCurrent on a current context should change nothing")
	}
}

func TestTheCounterSeparatesReadsThatMayLag(t *testing.T) {
	var q queryCounter
	q.TraceQueryStart(context.Background(), nil, pgxStart())
	q.TraceQueryStart(WithMayLag(context.Background()), nil, pgxStart())
	q.TraceQueryStart(MustBeCurrent(WithMayLag(context.Background())), nil, pgxStart())
	if q.n.Load() != 3 || q.lag.Load() != 1 {
		t.Fatalf("counted %d queries and %d that may lag, want 3 and 1", q.n.Load(), q.lag.Load())
	}
}
