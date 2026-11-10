// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/testutil"
)

var witnessKey = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))

// signAndLog signs a checkpoint the way the server does, and writes its witness line.
func signAndLog(t *testing.T, tdb testutil.TestDB, log *slog.Logger) {
	t.Helper()
	cp, signed, err := audit.SignCheckpoint(context.Background(), tdb.Pool, witnessKey)
	if err != nil || !signed {
		t.Fatalf("signed %v: %v", signed, err)
	}
	log.Info(audit.WitnessMessage, cp.WitnessAttrs()...)
}

// A log collector or the container runtime can put a prefix before each line, and the
// same log holds every other kind of line too.
func TestTheWitnessIsReadFromAMixedLog(t *testing.T) {
	tdb := testutil.Database(t)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	record(t, tdb, sample("page.created"))
	signAndLog(t, tdb, log)
	log.Info("request", "path", "/api/v1/me", "status", 200)
	record(t, tdb, sample("page.updated"))
	signAndLog(t, tdb, log)

	var mixed strings.Builder
	for i, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if i%2 == 0 {
			mixed.WriteString("2026-11-10T01:00:00.000Z stderr F ")
		}
		mixed.WriteString(line + "\nnot json at all\n")
	}
	witness, err := audit.ReadWitness(strings.NewReader(mixed.String()))
	if err != nil || len(witness) != 2 {
		t.Fatalf("witness %d: %v", len(witness), err)
	}
	res, err := audit.CompareWitness(context.Background(), tdb.Pool, witnessKey.Public().(ed25519.PublicKey), witness)
	if err != nil || !res.OK || res.Compared != 2 {
		t.Fatalf("%+v, %v", res, err)
	}
}

// A witness line that was cut or changed must stop the check, not be skipped.
func TestABrokenWitnessLineIsAnError(t *testing.T) {
	for name, line := range map[string]string{
		"cut":          `{"msg":"audit checkpoint","witness":"audit-checkpoint-1","seq":4,"hash":"ab`,
		"bad hex":      `{"msg":"audit checkpoint","witness":"audit-checkpoint-1","seq":4,"hash":"zz","signature":"00","public_key":"00"}`,
		"bad position": `{"msg":"audit checkpoint","witness":"audit-checkpoint-1","seq":0,"hash":"00","signature":"00","public_key":"00"}`,
	} {
		if _, err := audit.ReadWitness(strings.NewReader(line + "\n")); err == nil {
			t.Errorf("%s: read without an error", name)
		}
	}
}

// Checkpoints from before the retention period have no rows left, and that is no damage.
func TestWitnessCheckpointsBeforeAPruneAreSkipped(t *testing.T) {
	tdb := testutil.Database(t)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	record(t, tdb, sample("page.created"))
	signAndLog(t, tdb, log)
	if _, err := audit.Prune(context.Background(), tdb.Pool, time.Now().Add(time.Hour), witnessKey); err != nil {
		t.Fatal(err)
	}
	record(t, tdb, sample("page.updated"))
	signAndLog(t, tdb, log)
	witness, err := audit.ReadWitness(&buf)
	if err != nil {
		t.Fatal(err)
	}
	res, err := audit.CompareWitness(context.Background(), tdb.Pool, witnessKey.Public().(ed25519.PublicKey), witness)
	if err != nil || !res.OK || res.Pruned != 1 || res.Compared != 1 {
		t.Fatalf("%+v, %v", res, err)
	}
}

// A chain changed at a checkpoint the witness holds disagrees with it, also when the
// changed rows were made to verify again.
func TestAChainThatDiffersFromTheWitnessIsNamed(t *testing.T) {
	tdb := testutil.Database(t)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	record(t, tdb, sample("page.created"))
	signAndLog(t, tdb, log)
	witness, err := audit.ReadWitness(&buf)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, q := range []string{
		"ALTER TABLE audit_events DISABLE TRIGGER audit_events_no_update_delete",
		"UPDATE audit_events SET hash = decode(repeat('11', 32), 'hex') WHERE seq = (SELECT min(seq) FROM audit_events)",
		"ALTER TABLE audit_events ENABLE TRIGGER audit_events_no_update_delete",
	} {
		if _, err := tdb.Pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	res, err := audit.CompareWitness(ctx, tdb.Pool, witnessKey.Public().(ed25519.PublicKey), witness)
	if err != nil || res.OK || res.FirstBadSeq != witness[0].Seq || !strings.Contains(res.Reason, "differs") {
		t.Fatalf("%+v, %v", res, err)
	}
}
