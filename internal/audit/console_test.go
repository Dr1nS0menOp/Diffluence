// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"testing"
	"time"

	"github.com/kenwerk/kenwerk/internal/audit"
	"github.com/kenwerk/kenwerk/internal/testutil"
)

// Before any witness report, the console has nothing to show and nothing warns (CSL-533).
func TestNoWitnessReportIsNoWitness(t *testing.T) {
	tdb := testutil.Database(t)
	got, err := audit.LatestWitnessCheck(context.Background(), tdb.Pool)
	if err != nil || got != nil {
		t.Fatalf("no report yet: %+v %v", got, err)
	}
}

// A report is what the comparison found, as the command wrote it, and the newest one is
// what the console shows (CSL-532).
func TestTheNewestWitnessReportIsTheOneShown(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	if err := audit.RecordWitnessCheck(ctx, tdb.Pool, audit.WitnessCheck{OK: true, Compared: 12, Pruned: 1}); err != nil {
		t.Fatal(err)
	}
	if err := audit.RecordWitnessCheck(ctx, tdb.Pool, audit.WitnessCheck{OK: false, Compared: 3, FirstBadSeq: 41,
		Reason: "the chain differs from a checkpoint in the witness log"}); err != nil {
		t.Fatal(err)
	}
	got, err := audit.LatestWitnessCheck(ctx, tdb.Pool)
	if err != nil || got == nil {
		t.Fatalf("latest: %+v %v", got, err)
	}
	if got.OK || got.FirstBadSeq != 41 || got.Compared != 3 || got.Reason == "" || time.Since(got.CheckedAt) > time.Minute {
		t.Fatalf("the newest report: %+v", got)
	}
	ever, err := audit.WitnessEverReported(ctx, tdb.Pool)
	if err != nil || !ever {
		t.Fatalf("ever reported: %v %v", ever, err)
	}
}

// A report that says it matched cannot name a position that did not; a reason is one short line.
func TestAWitnessReportIsRefusedWhenItContradictsItself(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	for _, bad := range []audit.WitnessCheck{
		{OK: true, FirstBadSeq: 4},
		{OK: false},
		{OK: true, Compared: -1},
	} {
		if err := audit.RecordWitnessCheck(ctx, tdb.Pool, bad); err == nil {
			t.Errorf("%+v was recorded", bad)
		}
	}
}

// The audit area shows the newest checkpoint and every key that signs, each with whether the
// installation endorsed it (CSL-530), and never more of a key than its start.
func TestTheAuditOverviewNamesTheCheckpointAndTheKeys(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	empty, err := audit.ReadOverview(ctx, tdb.Pool, installation)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Checkpoint != nil || len(empty.Keys) != 1 || !empty.Keys[0].Installation {
		t.Fatalf("a new installation: %+v", empty)
	}
	record(t, tdb, sample("page.created"))
	sign(t, tdb, installationKey)
	from := changeKey(t, tdb, installationKey, workerKey)
	record(t, tdb, sample("page.updated"))
	newest := sign(t, tdb, workerKey)

	got, err := audit.ReadOverview(ctx, tdb.Pool, installation)
	if err != nil {
		t.Fatal(err)
	}
	if got.Checkpoint == nil || got.Checkpoint.Seq != newest.Seq || time.Since(got.Checkpoint.At) > time.Minute {
		t.Fatalf("newest checkpoint: %+v", got.Checkpoint)
	}
	if len(got.Keys) != 2 || got.Keys[1].FromSeq != from || !got.Keys[1].Endorsed || got.Keys[1].Installation {
		t.Fatalf("keys: %+v", got.Keys)
	}
	whole := hex.EncodeToString(workerKey.Public().(ed25519.PublicKey))
	if got.Keys[1].PublicKeyStart != whole[:audit.KeyStartLength] || len(got.Keys[1].PublicKeyStart) >= len(whole) {
		t.Fatalf("a key shows only its start: %q", got.Keys[1].PublicKeyStart)
	}

	// A key the installation did not endorse is marked, not hidden.
	if _, err := tdb.Pool.Exec(ctx, "UPDATE audit_signing_keys SET endorsement = $1", make([]byte, 64)); err != nil {
		t.Fatal(err)
	}
	got, err = audit.ReadOverview(ctx, tdb.Pool, installation)
	if err != nil || len(got.Keys) != 2 || got.Keys[1].Endorsed {
		t.Fatalf("an unendorsed key: %+v %v", got.Keys, err)
	}
}

// The upgrade page names the newest marker of an upgrade and the backup it names (CSL-560).
func TestTheNewestEventOfAnActionIsFound(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	none, err := audit.LatestOf(ctx, tdb.Pool, "system.upgrade_started")
	if err != nil || none != nil {
		t.Fatalf("none yet: %+v %v", none, err)
	}
	first := sample("system.upgrade_started")
	first.Details = map[string]any{"to_migration": 64}
	if err := audit.RecordSealed(ctx, tdb.Pool, first); err != nil {
		t.Fatal(err)
	}
	second := sample("system.upgrade_started")
	second.Details = map[string]any{"to_migration": 65}
	if err := audit.RecordSealed(ctx, tdb.Pool, second); err != nil {
		t.Fatal(err)
	}
	got, err := audit.LatestOf(ctx, tdb.Pool, "system.upgrade_started")
	if err != nil || got == nil || got.Details["to_migration"] != float64(65) {
		t.Fatalf("newest: %+v %v", got, err)
	}
}
