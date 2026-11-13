// SPDX-License-Identifier: Apache-2.0

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/testutil"
)

func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	tdb := testutil.Database(t)
	pool, err := db.Open(context.Background(), tdb.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestAQueryOfARequestStopsAtItsLimit(t *testing.T) {
	pool := openPool(t)
	// The outer deadline only keeps the test from hanging when the limit does not work.
	outer, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ctx := db.WithQueryLimit(outer, 200*time.Millisecond)
	started := time.Now()
	if _, err := pool.Exec(ctx, "SELECT pg_sleep(5)"); err == nil {
		t.Fatal("a query past its limit succeeded")
	}
	if took := time.Since(started); took > 3*time.Second {
		t.Errorf("the query took %s, want about 200ms", took)
	}
	if !db.QueryRanOut(ctx) {
		t.Error("the request does not know its query ran out")
	}
	// The pool is not left with a broken connection: the next query works.
	var one int
	if err := pool.QueryRow(context.Background(), "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("the next query got %d, %v", one, err)
	}
}

func TestAQueryWithinItsLimitOrWithoutOneRunsToTheEnd(t *testing.T) {
	pool := openPool(t)
	limited := db.WithQueryLimit(context.Background(), 5*time.Second)
	for _, ctx := range []context.Context{limited, context.Background()} {
		if _, err := pool.Exec(ctx, "SELECT pg_sleep(0.3)"); err != nil {
			t.Fatal(err)
		}
	}
	if db.QueryRanOut(limited) {
		t.Error("a query that finished in time counts as running out")
	}
}

func TestAQueryThatFailsForAnotherReasonDoesNotCountAsRunningOut(t *testing.T) {
	pool := openPool(t)
	ctx := db.WithQueryLimit(context.Background(), 5*time.Second)
	if _, err := pool.Exec(ctx, "SELECT 1/0"); err == nil {
		t.Fatal("division by zero succeeded")
	}
	if db.QueryRanOut(ctx) {
		t.Error("a query that failed on its own counts as running out")
	}
}

// A cached generic plan cannot see which optional filters are empty, and on a large
// data set it chose plans hundreds of times slower than the plan for the actual values
// (spec 061). Every connection of the pool plans with the values it is given.
func TestThePoolPlansEveryQueryWithItsValues(t *testing.T) {
	pool := openPool(t)
	var mode string
	if err := pool.QueryRow(context.Background(), "SHOW plan_cache_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "force_custom_plan" {
		t.Errorf("plan_cache_mode is %q, want force_custom_plan", mode)
	}
}

// JIT compiling a plan cost more than half a second a query on real data, where the
// queries themselves take milliseconds (spec 061).
func TestThePoolCompilesNoPlans(t *testing.T) {
	pool := openPool(t)
	var jit string
	if err := pool.QueryRow(context.Background(), "SHOW jit").Scan(&jit); err != nil {
		t.Fatal(err)
	}
	if jit != "off" {
		t.Errorf("jit is %q, want off", jit)
	}
}
