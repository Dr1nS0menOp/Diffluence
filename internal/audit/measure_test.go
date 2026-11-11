// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"context"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/testutil"
)

// TestMeasureSavesUnderTheChainLock measures what ADR-041 changes, for spec 052: many saves
// at once, each one transaction with an audit event, first holding the lock of the chain
// until commit as every save did before, then without it, with a sealer running. It only
// runs when DIFFLUENCE_MEASURE is set, because it measures rather than checks.
func TestMeasureSavesUnderTheChainLock(t *testing.T) {
	if os.Getenv("DIFFLUENCE_MEASURE") == "" {
		t.Skip("set DIFFLUENCE_MEASURE=1 to measure")
	}
	const writers, each = 32, 40
	for _, holdLock := range []bool{true, false} {
		tdb := testutil.Database(t)
		ctx, stop := context.WithCancel(context.Background())
		var sealer sync.WaitGroup
		if !holdLock {
			sealer.Go(func() {
				for ctx.Err() == nil {
					_, _ = audit.SealWaiting(ctx, tdb.Pool)
					time.Sleep(time.Second)
				}
			})
		}
		var mu sync.Mutex
		var took []time.Duration
		var saves sync.WaitGroup
		started := time.Now()
		for range writers {
			saves.Go(func() {
				for range each {
					begin := time.Now()
					err := pgx.BeginFunc(context.Background(), tdb.Pool, func(tx pgx.Tx) error {
						if holdLock {
							if _, err := tx.Exec(context.Background(), "SELECT pg_advisory_xact_lock($1)", chainLock); err != nil {
								return err
							}
						}
						_, err := audit.Record(context.Background(), tx, sample("page.saved_in_measure"))
						return err
					})
					if err != nil {
						t.Error(err)
						return
					}
					mu.Lock()
					took = append(took, time.Since(begin))
					mu.Unlock()
				}
			})
		}
		saves.Wait()
		wall := time.Since(started)
		stop()
		sealer.Wait()
		slices.Sort(took)
		at := func(q float64) time.Duration { return took[int(q*float64(len(took)-1))].Round(time.Microsecond) }
		t.Logf("lock held to commit=%v: %d saves by %d writers in %s (%.0f/s), p50 %s, p95 %s, p99 %s, pool %d",
			holdLock, len(took), writers, wall.Round(time.Millisecond), float64(len(took))/wall.Seconds(),
			at(0.50), at(0.95), at(0.99), tdb.Pool.Config().MaxConns)
		if !holdLock {
			seal(t, tdb)
			if res := verify(t, tdb); !res.OK || res.Checked != writers*each {
				t.Fatalf("%+v", res)
			}
		}
	}
}
