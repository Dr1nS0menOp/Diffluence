// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/testutil"
)

func record(t *testing.T, tdb testutil.TestDB, e audit.Event) {
	t.Helper()
	if err := audit.RecordStandalone(context.Background(), tdb.Pool, e); err != nil {
		t.Fatal(err)
	}
}

func sample(action string) audit.Event {
	return audit.Event{ActorType: audit.ActorUser, ActorID: "u1", Action: action, ObjectType: "page",
		ObjectID: "p1", Result: audit.ResultSuccess, IP: "10.0.0.1", Details: map[string]any{"title": "Hello", "n": 3}}
}

func TestChainVerifies(t *testing.T) {
	tdb := testutil.Database(t)
	for _, a := range []string{"page.created", "page.updated", "page.deleted"} {
		record(t, tdb, sample(a))
	}
	res, err := audit.Verify(context.Background(), tdb.Pool)
	if err != nil || !res.OK || res.Checked != 3 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestConcurrentAppendsKeepTheChainIntact(t *testing.T) {
	tdb := testutil.Database(t)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = audit.RecordStandalone(context.Background(), tdb.Pool, sample("page.viewed_in_test"))
		}()
	}
	wg.Wait()
	res, err := audit.Verify(context.Background(), tdb.Pool)
	if err != nil || !res.OK || res.Checked != 20 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestRolledBackChangeLeavesNoEvent(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	tx, err := tdb.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := audit.Record(ctx, tx, sample("page.created")); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	evs, _ := audit.List(ctx, tdb.Pool, audit.Filter{})
	if len(evs) != 0 {
		t.Fatalf("event survived rollback: %+v", evs)
	}
}

func TestAppendOnlyIsEnforcedByTheDatabase(t *testing.T) {
	tdb := testutil.Database(t)
	record(t, tdb, sample("page.created"))
	ctx := context.Background()
	for _, sql := range []string{
		"UPDATE audit_events SET action = 'x.y'",
		"DELETE FROM audit_events",
		"TRUNCATE audit_events",
	} {
		if _, err := tdb.Pool.Exec(ctx, sql); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s: want append-only error, got %v", sql, err)
		}
	}
}

// tamper bypasses the trigger the way an attacker with superuser rights could.
func tamper(t *testing.T, tdb testutil.TestDB, sql string) {
	t.Helper()
	err := pgx.BeginFunc(context.Background(), tdb.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), "ALTER TABLE audit_events DISABLE TRIGGER USER"); err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(), sql); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), "ALTER TABLE audit_events ENABLE TRIGGER USER")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTamperingIsDetected(t *testing.T) {
	cases := map[string]string{
		"payload changed": "UPDATE audit_events SET payload = replace(payload, 'Hello', 'Bye') WHERE seq = 2",
		"column changed":  "UPDATE audit_events SET actor_id = 'someone-else' WHERE seq = 2",
		"row deleted":     "DELETE FROM audit_events WHERE seq = 2",
	}
	for name, sql := range cases {
		t.Run(name, func(t *testing.T) {
			tdb := testutil.Database(t)
			for range 3 {
				record(t, tdb, sample("page.updated"))
			}
			tamper(t, tdb, sql)
			res, err := audit.Verify(context.Background(), tdb.Pool)
			if err != nil {
				t.Fatal(err)
			}
			if res.OK {
				t.Fatalf("tampering not detected: %+v", res)
			}
			if res.FirstBadSeq < 2 || res.FirstBadSeq > 3 {
				t.Errorf("first bad seq = %d", res.FirstBadSeq)
			}
		})
	}
}

func TestInvalidEventsAreRejected(t *testing.T) {
	tdb := testutil.Database(t)
	bad := []audit.Event{
		{ActorType: "robot", Action: "page.created", Result: audit.ResultSuccess},
		{ActorType: audit.ActorUser, Action: "Page Created", Result: audit.ResultSuccess},
		{ActorType: audit.ActorUser, Action: "page.created", Result: "maybe"},
	}
	for _, e := range bad {
		if err := audit.RecordStandalone(context.Background(), tdb.Pool, e); err == nil {
			t.Errorf("accepted invalid event %+v", e)
		}
	}
}

func TestListFiltersAndCSVExport(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	record(t, tdb, sample("page.created"))
	e := sample("page.deleted")
	e.ActorID = "u2"
	e.Details = map[string]any{"title": "=HYPERLINK(\"http://evil\")"}
	record(t, tdb, e)

	evs, err := audit.List(ctx, tdb.Pool, audit.Filter{ActorID: "u2"})
	if err != nil || len(evs) != 1 || evs[0].Action != "page.deleted" {
		t.Fatalf("filter by actor: %+v %v", evs, err)
	}
	evs, _ = audit.List(ctx, tdb.Pool, audit.Filter{Limit: 1})
	if len(evs) != 1 || evs[0].Seq != 2 {
		t.Fatalf("newest first with limit: %+v", evs)
	}

	var buf bytes.Buffer
	n, err := audit.ExportCSV(ctx, tdb.Pool, audit.Filter{}, 100, &buf)
	if err != nil || n != 2 {
		t.Fatalf("export n=%d err=%v", n, err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil || len(rows) != 3 || rows[0][0] != "seq" {
		t.Fatalf("csv: %v %v", rows, err)
	}
	// Details is JSON so it starts with '{'; check a cell that starts with '=' directly.
	e2 := sample("page.renamed")
	e2.ObjectID = "=cmd|' /C calc'!A0"
	record(t, tdb, e2)
	buf.Reset()
	_, _ = audit.ExportCSV(ctx, tdb.Pool, audit.Filter{Action: "page.renamed"}, 100, &buf)
	rows, _ = csv.NewReader(&buf).ReadAll()
	if got := rows[1][7]; !strings.HasPrefix(got, "'=") {
		t.Fatalf("formula not neutralised: %q", got)
	}
}
