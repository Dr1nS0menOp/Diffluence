// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/testutil"
)

// erase erases the personal parts of person and records the erasure with its count, in one
// transaction, as anonymising someone does.
func erase(t *testing.T, tdb testutil.TestDB, person string) int64 {
	t.Helper()
	ctx := context.Background()
	var erased int64
	err := pgx.BeginFunc(ctx, tdb.Pool, func(tx pgx.Tx) error {
		var err error
		if erased, err = audit.ErasePersonal(ctx, tx, person); err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, audit.Event{ActorType: audit.ActorSystem, Action: "user.anonymised", ObjectType: "user",
			ObjectID: person, Result: audit.ResultSuccess, Details: map[string]any{audit.DetailErasedParts: erased}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	seal(t, tdb)
	return erased
}

// A personal part that is gone while no erasure in the chain counted it is found.
func TestAPersonalPartRemovedWithoutARecordedErasureIsFound(t *testing.T) {
	tdb := testutil.Database(t)
	record(t, tdb, sample("page.updated"))
	record(t, tdb, sample("page.created"))
	if _, err := tdb.Pool.Exec(context.Background(), "DELETE FROM audit_personal WHERE seq = (SELECT min(seq) FROM audit_personal)"); err != nil {
		t.Fatal(err)
	}
	res := verify(t, tdb)
	if res.OK || res.ErasedParts != 1 || res.RecordedErasures != 0 || !strings.Contains(res.Reason, "erasure") {
		t.Fatalf("%+v", res)
	}
}

// An erasure recorded with its count keeps the check whole, and the check says how many
// parts are gone.
func TestAnErasureRecordedWithItsCountKeepsTheCheckWhole(t *testing.T) {
	tdb := testutil.Database(t)
	record(t, tdb, sample("page.updated"))
	record(t, tdb, sample("page.created"))
	if n := erase(t, tdb, "u1"); n != 2 {
		t.Fatalf("erased %d parts, want 2", n)
	}
	res := verify(t, tdb)
	if !res.OK || res.ErasedParts != 2 || res.RecordedErasures != 2 {
		t.Fatalf("%+v", res)
	}
	// One more part going quietly after a recorded erasure is still found.
	record(t, tdb, sample("page.deleted"))
	if _, err := tdb.Pool.Exec(context.Background(), "DELETE FROM audit_personal"); err != nil {
		t.Fatal(err)
	}
	if res := verify(t, tdb); res.OK || res.ErasedParts != 3 || res.RecordedErasures != 2 {
		t.Fatalf("%+v", res)
	}
}
