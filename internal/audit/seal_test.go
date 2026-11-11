// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/csv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/testutil"
)

// chainLock is the advisory lock of the chain, as audit.go names it.
const chainLock int64 = 0x6466_6c75_6175_6474

func waiting(t *testing.T, tdb testutil.TestDB) int64 {
	t.Helper()
	var n int64
	if err := tdb.Pool.QueryRow(context.Background(), "SELECT count(*) FROM audit_pending").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func verify(t *testing.T, tdb testutil.TestDB) audit.VerifyResult {
	t.Helper()
	res, err := audit.Verify(context.Background(), tdb.Pool)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// The point of ADR-041: a save does not wait while someone else holds the chain.
func TestASaveDoesNotWaitForTheChain(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	holder, err := tdb.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback(ctx) }()
	if _, err := holder.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", chainLock); err != nil {
		t.Fatal(err)
	}
	quick, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := audit.RecordStandalone(quick, tdb.Pool, sample("page.updated")); err != nil {
		t.Fatalf("a save waited for the chain: %v", err)
	}
	if n := waiting(t, tdb); n != 1 {
		t.Fatalf("waiting = %d, want 1", n)
	}
}

// The sealer waits while someone else holds the chain, such as a release from before
// ADR-041 that appends straight into it, so the two never link to the same head.
func TestTheSealerWaitsWhileTheChainIsHeld(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	if err := audit.RecordStandalone(ctx, tdb.Pool, sample("page.updated")); err != nil {
		t.Fatal(err)
	}
	holder, err := tdb.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Rollback(ctx) }()
	if _, err := holder.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", chainLock); err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if n, err := audit.SealWaiting(short, tdb.Pool); err == nil || n != 0 {
		t.Fatalf("sealed %d while the chain was held (err %v)", n, err)
	}
	if n := waiting(t, tdb); n != 1 {
		t.Fatalf("waiting = %d, want 1", n)
	}
	if err := holder.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	seal(t, tdb)
	if res := verify(t, tdb); !res.OK || res.Checked != 1 {
		t.Fatalf("%+v", res)
	}
}

// A transaction that began first but committed last is sealed after the ones that
// committed before it, and nothing is skipped or left out.
func TestEventsAreSealedInTheOrderTheyBecameVisible(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	first, err := tdb.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Rollback(ctx) }()
	if _, err := audit.Record(ctx, first, sample("page.began_first")); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"page.committed_one", "page.committed_two"} {
		if err := audit.RecordStandalone(ctx, tdb.Pool, sample(action)); err != nil {
			t.Fatal(err)
		}
	}
	seal(t, tdb)
	if err := first.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	seal(t, tdb)

	res := verify(t, tdb)
	if !res.OK || res.Checked != 3 || res.Unsealed != 0 {
		t.Fatalf("%+v", res)
	}
	evs, err := audit.List(ctx, tdb.Pool, audit.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for i := len(evs) - 1; i >= 0; i-- {
		order = append(order, evs[i].Action)
	}
	want := "page.committed_one page.committed_two page.began_first"
	if got := strings.Join(order, " "); got != want {
		t.Fatalf("chain order %q, want %q", got, want)
	}
}

// Every worker seals, and the lock lets one at a time: no event is sealed twice and the
// chain stays one line while saves go on.
func TestTwoSealersNeverSealAnEventTwice(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	const writers, each = 8, 25
	var saves sync.WaitGroup
	for range writers {
		saves.Go(func() {
			for range each {
				if err := audit.RecordStandalone(ctx, tdb.Pool, sample("page.saved_in_test")); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	done := make(chan struct{})
	var sealers sync.WaitGroup
	for range 2 {
		sealers.Go(func() {
			for {
				if _, err := audit.SealWaiting(ctx, tdb.Pool); err != nil {
					t.Error(err)
					return
				}
				select {
				case <-done:
					return
				case <-time.After(time.Millisecond):
				}
			}
		})
	}
	saves.Wait()
	close(done)
	sealers.Wait()
	seal(t, tdb)
	res := verify(t, tdb)
	if !res.OK || res.Checked != writers*each || res.Unsealed != 0 {
		t.Fatalf("%+v", res)
	}
}

// A long backlog is sealed in steps, and all of it.
func TestABacklogLongerThanOneStepIsSealedWhole(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	err := pgx.BeginFunc(ctx, tdb.Pool, func(tx pgx.Tx) error {
		for range audit.SealBatch + 3 {
			if _, err := audit.Record(ctx, tx, sample("page.bulk_in_test")); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := audit.SealWaiting(ctx, tdb.Pool)
	if err != nil || n != audit.SealBatch+3 {
		t.Fatalf("sealed %d, %v", n, err)
	}
	if res := verify(t, tdb); !res.OK || res.Checked != int64(audit.SealBatch+3) {
		t.Fatalf("%+v", res)
	}
}

// Nothing removes or rewrites an event before it is sealed, except erasing its personal
// part. The trigger holds for every role; only the owner could switch it off.
func TestAWaitingEventCannotBeRemovedOrChanged(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	if err := audit.RecordStandalone(ctx, tdb.Pool, sample("page.updated")); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"DELETE FROM audit_pending",
		"TRUNCATE audit_pending",
		"UPDATE audit_pending SET action = 'page.other'",
		"UPDATE audit_pending SET payload = replace(payload, 'u1', 'u2')",
		"UPDATE audit_pending SET salt = NULL, data = NULL, actor_id = 'u2'",
	} {
		if _, err := tdb.Pool.Exec(ctx, sql); err == nil || !strings.Contains(err.Error(), "audit_pending") {
			t.Errorf("%s: want a refusal, got %v", sql, err)
		}
	}
	if n := waiting(t, tdb); n != 1 {
		t.Fatalf("waiting = %d", n)
	}
	if _, err := tdb.Pool.Exec(ctx, "UPDATE audit_pending SET salt = NULL, data = NULL"); err != nil {
		t.Fatalf("erasing the personal part: %v", err)
	}
	seal(t, tdb)
	if res := verify(t, tdb); !res.OK || res.Checked != 1 {
		t.Fatalf("%+v", res)
	}
}

// Waiting events are no part of the chain yet, and a check says how many there are.
func TestVerifyCountsWaitingEvents(t *testing.T) {
	tdb := testutil.Database(t)
	if err := audit.RecordStandalone(context.Background(), tdb.Pool, sample("page.updated")); err != nil {
		t.Fatal(err)
	}
	if res := verify(t, tdb); !res.OK || res.Checked != 0 || res.Unsealed != 1 {
		t.Fatalf("before sealing: %+v", res)
	}
	seal(t, tdb)
	if res := verify(t, tdb); !res.OK || res.Checked != 1 || res.Unsealed != 0 {
		t.Fatalf("after sealing: %+v", res)
	}
}

// A reader sees an event at once, sealed or not: waiting ones first in a list, last in an
// export, without a seq.
func TestReadersSeeWaitingEventsAtOnce(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	record(t, tdb, sample("page.created"))
	if err := audit.RecordStandalone(ctx, tdb.Pool, sample("page.updated")); err != nil {
		t.Fatal(err)
	}
	evs, err := audit.List(ctx, tdb.Pool, audit.Filter{})
	if err != nil || len(evs) != 2 {
		t.Fatalf("%+v, %v", evs, err)
	}
	if evs[0].Action != "page.updated" || evs[0].Sealed || evs[0].Seq != 0 || !evs[1].Sealed || evs[1].Seq == 0 {
		t.Fatalf("waiting first: %+v", evs)
	}
	if evs[0].IP != "10.0.0.1" || evs[0].Details["title"] != "Hello" {
		t.Fatalf("the personal part of a waiting event: %+v", evs[0].Event)
	}
	older, err := audit.List(ctx, tdb.Pool, audit.Filter{BeforeSeq: evs[1].Seq + 1})
	if err != nil || len(older) != 1 || !older[0].Sealed {
		t.Fatalf("paging back reaches sealed events only: %+v, %v", older, err)
	}
	var buf bytes.Buffer
	if n, err := audit.ExportCSV(ctx, tdb.Pool, audit.Filter{}, 100, &buf); err != nil || n != 2 {
		t.Fatalf("export n=%d err=%v", n, err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil || len(rows) != 3 || rows[1][0] == "" || rows[2][0] != "" || rows[2][5] != "page.updated" {
		t.Fatalf("waiting last, without a seq: %v %v", rows, err)
	}
}

// Erasing a person also reaches the part of an event that has not been sealed yet.
func TestErasingAPersonReachesAWaitingEvent(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	if err := audit.RecordStandalone(ctx, tdb.Pool, sample("page.updated")); err != nil {
		t.Fatal(err)
	}
	err := pgx.BeginFunc(ctx, tdb.Pool, func(tx pgx.Tx) error { return audit.ErasePersonal(ctx, tx, "u1") })
	if err != nil {
		t.Fatal(err)
	}
	seal(t, tdb)
	var left int
	if err := tdb.Pool.QueryRow(ctx, "SELECT count(*) FROM audit_personal").Scan(&left); err != nil || left != 0 {
		t.Fatalf("personal parts left: %d, %v", left, err)
	}
	if res := verify(t, tdb); !res.OK || res.Checked != 1 {
		t.Fatalf("%+v", res)
	}
}

// After a prune that removed every event, the chain goes on from the anchor.
func TestTheChainGoesOnFromTheAnchorAfterEverythingWasPruned(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	record(t, tdb, sample("page.created"))
	if err := audit.RecordStandalone(ctx, tdb.Pool, sample("page.waiting_at_prune")); err != nil {
		t.Fatal(err)
	}
	if _, err := audit.Prune(ctx, tdb.Pool, time.Now().Add(time.Hour), witnessKey); err != nil {
		t.Fatal(err)
	}
	// The prune sealed in its own transaction, so a release that appends straight into the
	// chain never finds it empty.
	if n := waiting(t, tdb); n != 0 {
		t.Fatalf("waiting after the prune: %d", n)
	}
	record(t, tdb, sample("page.updated"))
	res, err := audit.Check(ctx, tdb.Pool, witnessKey.Public().(ed25519.PublicKey))
	if err != nil || !res.OK || res.Checked != 3 {
		t.Fatalf("%+v, %v", res, err)
	}
}

// An event written straight into the chain, as a release from before ADR-041 does and as
// an upgrade marker does, joins the same chain as sealed events.
func TestAnEventSealedDirectlyJoinsTheSameChain(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	if err := audit.RecordStandalone(ctx, tdb.Pool, sample("page.created")); err != nil {
		t.Fatal(err)
	}
	if err := audit.RecordSealed(ctx, tdb.Pool, sample("system.upgrade_started")); err != nil {
		t.Fatal(err)
	}
	if n := waiting(t, tdb); n != 1 {
		t.Fatalf("waiting = %d, want only the first", n)
	}
	seal(t, tdb)
	if res := verify(t, tdb); !res.OK || res.Checked != 2 {
		t.Fatalf("%+v", res)
	}
}
