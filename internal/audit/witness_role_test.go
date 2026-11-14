// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/testutil"
)

// appRole makes a role with the grants of the application role in the test database, and
// removes it after the test.
func appRole(t *testing.T, tdb testutil.TestDB) string {
	t.Helper()
	ctx := context.Background()
	role := "dfl_app_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if _, err := tdb.Pool.Exec(ctx, "CREATE ROLE "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := tdb.Pool.Exec(ctx, "DROP OWNED BY "+role); err != nil {
			t.Error(err)
		}
		if _, err := tdb.Pool.Exec(ctx, "DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	if err := db.GrantApp(ctx, tdb.Pool, role); err != nil {
		t.Fatal(err)
	}
	return role
}

// recordAs records a witness report as role, in a transaction of its own.
func recordAs(ctx context.Context, tdb testutil.TestDB, role string, w audit.WitnessCheck) error {
	return pgx.BeginFunc(ctx, tdb.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+pgx.Identifier{role}.Sanitize()); err != nil {
			return err
		}
		return audit.RecordWitnessCheck(ctx, tx, w)
	})
}

// Witness reports are recorded by the owner of the tables, which the backup service holds;
// the application role reads them and records none (spec 077).
func TestOnlyTheOwnerOfTheTablesRecordsAWitnessReport(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	role := appRole(t, tdb)
	err := recordAs(ctx, tdb, role, audit.WitnessCheck{OK: true, Compared: 5})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
		t.Fatalf("the application role recorded a witness report (%v)", err)
	}
	if err := audit.RecordWitnessCheck(ctx, tdb.Pool, audit.WitnessCheck{OK: true, Compared: 5}); err != nil {
		t.Fatalf("the owner records a report: %v", err)
	}
	var got *audit.WitnessCheck
	err = pgx.BeginFunc(ctx, tdb.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+pgx.Identifier{role}.Sanitize()); err != nil {
			return err
		}
		var err error
		got, err = audit.LatestWitnessCheck(ctx, tx)
		return err
	})
	if err != nil || got == nil || !got.OK || got.Compared != 5 {
		t.Fatalf("the application role reads the owner's report: %+v %v", got, err)
	}
}

// The console shows only reports the owner of the tables recorded: one another role added,
// as the application role could before spec 077, stays in the table and is not shown.
func TestAWitnessReportOfAnotherRoleIsNotShown(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	role := appRole(t, tdb)
	// The grant of the release before spec 077.
	if _, err := tdb.Pool.Exec(ctx, "GRANT INSERT ON audit_witness_checks TO "+role); err != nil {
		t.Fatal(err)
	}
	if err := recordAs(ctx, tdb, role, audit.WitnessCheck{OK: true, Compared: 9}); err != nil {
		t.Fatal(err)
	}
	if got, err := audit.LatestWitnessCheck(ctx, tdb.Pool); err != nil || got != nil {
		t.Fatalf("a report of the application role is shown: %+v %v", got, err)
	}
	if ever, err := audit.WitnessEverReported(ctx, tdb.Pool); err != nil || ever {
		t.Fatalf("a report of the application role counts: %v %v", ever, err)
	}
	if err := audit.RecordWitnessCheck(ctx, tdb.Pool, audit.WitnessCheck{OK: false, Compared: 2, FirstBadSeq: 3,
		Reason: "the chain differs from a checkpoint in the witness log"}); err != nil {
		t.Fatal(err)
	}
	if err := recordAs(ctx, tdb, role, audit.WitnessCheck{OK: true, Compared: 9}); err != nil {
		t.Fatal(err)
	}
	got, err := audit.LatestWitnessCheck(ctx, tdb.Pool)
	if err != nil || got == nil || got.OK || got.FirstBadSeq != 3 {
		t.Fatalf("the owner's report gives way to a newer one of another role: %+v %v", got, err)
	}
	var unshown int
	if err := tdb.Pool.QueryRow(ctx, "SELECT count(*) FROM audit_witness_checks WHERE recorded_by = $1", role).Scan(&unshown); err != nil || unshown != 2 {
		t.Fatalf("the reports of the application role are kept with its name: %d %v", unshown, err)
	}
}

// The database stamps a report with its own role and time, whatever an insert names.
func TestTheDatabaseStampsTheRoleAndTimeOfAWitnessReport(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO audit_witness_checks (ok, compared, pruned, checked_at, recorded_by)
		VALUES (true, 1, 0, now() + interval '1 year', 'someone else')`); err != nil {
		t.Fatal(err)
	}
	var at time.Time
	var by, owner string
	if err := tdb.Pool.QueryRow(ctx, `SELECT checked_at, recorded_by, current_user FROM audit_witness_checks`).Scan(&at, &by, &owner); err != nil {
		t.Fatal(err)
	}
	if by != owner || time.Until(at) > time.Minute {
		t.Fatalf("stamped by %q at %v; want %q, now", by, at, owner)
	}
}
