// SPDX-License-Identifier: Apache-2.0

package db_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/testutil"
)

func TestThePoolHasALargerDefaultThatTheAddressCanChange(t *testing.T) {
	tdb := testutil.Database(t)
	joiner := "?"
	if strings.Contains(tdb.URL, "?") {
		joiner = "&"
	}
	for address, want := range map[string]int32{tdb.URL: 20, tdb.URL + joiner + "pool_max_conns=3": 3} {
		pool, err := db.Open(context.Background(), address)
		if err != nil {
			t.Fatal(err)
		}
		got := pool.Config().MaxConns
		pool.Close()
		if got != want {
			t.Errorf("got %d, want %d", got, want)
		}
	}
}

// onePool opens a pool of one connection and takes that connection, so every other
// wait for a connection has to wait.
func onePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	tdb := testutil.Database(t)
	joiner := "?"
	if strings.Contains(tdb.URL, "?") {
		joiner = "&"
	}
	pool, err := db.Open(context.Background(), tdb.URL+joiner+"pool_max_conns=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	held, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(held.Release)
	return pool
}

func TestAWaitForAConnectionGivesUpAtTheLimitOfItsRequest(t *testing.T) {
	pool := onePool(t)
	// The outer deadline only keeps the test from hanging when the limit does not work.
	guard, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx := db.WithAcquireLimit(guard, 200*time.Millisecond)
	started := time.Now()
	var one int
	err := pool.QueryRow(ctx, "SELECT 1").Scan(&one)
	if err == nil {
		t.Fatal("a query on a full pool succeeded")
	}
	if took := time.Since(started); took > 3*time.Second {
		t.Errorf("the wait took %s, want about 200ms", took)
	}
	if !db.AcquireRanOut(ctx) {
		t.Error("the request does not know its wait ran out")
	}
}

func TestAWaitWithoutALimitIsNotCountedAsRunningOut(t *testing.T) {
	pool := onePool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	limited := db.WithAcquireLimit(context.Background(), time.Minute)
	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err == nil {
		t.Fatal("a query on a full pool succeeded")
	}
	// A request that gives up for its own reason did not run into the pool's limit.
	if db.AcquireRanOut(ctx) || db.AcquireRanOut(limited) {
		t.Error("a wait that ended for another reason counts as running out")
	}
}

func TestAWaitWithinItsLimitGetsTheConnection(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := db.WithAcquireLimit(context.Background(), time.Second)
	var one int
	if err := tdb.Pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("got %d, %v", one, err)
	}
	if db.AcquireRanOut(ctx) {
		t.Error("a wait that succeeded counts as running out")
	}
}
