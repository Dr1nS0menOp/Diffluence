// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/testutil"
)

// The installation's own key (derived from master.key in the product) and the backup
// service's key (derived from the backup key), spec 070.
var (
	installationKey = witnessKey
	installation    = installationKey.Public().(ed25519.PublicKey)
	workerKey       = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{9}, ed25519.SeedSize))
	strangerKey     = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{3}, ed25519.SeedSize))
)

func sign(t *testing.T, tdb testutil.TestDB, key ed25519.PrivateKey) audit.Checkpoint {
	t.Helper()
	cp, signed, err := audit.SignCheckpoint(context.Background(), tdb.Pool, key)
	if err != nil || !signed {
		t.Fatalf("signed %v: %v", signed, err)
	}
	return cp
}

func changeKey(t *testing.T, tdb testutil.TestDB, endorser, next ed25519.PrivateKey) int64 {
	t.Helper()
	from, changed, err := audit.ChangeKey(context.Background(), tdb.Pool, endorser, next)
	if err != nil || !changed {
		t.Fatalf("changed %v: %v", changed, err)
	}
	return from
}

func check(t *testing.T, tdb testutil.TestDB) audit.VerifyResult {
	t.Helper()
	if _, err := audit.SealWaiting(context.Background(), tdb.Pool); err != nil {
		t.Fatal(err)
	}
	res, err := audit.Check(context.Background(), tdb.Pool, installation)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// A checkpoint signed by the installation, a change of key, and checkpoints signed by the
// backup service: all of it verifies with only the installation's public key, and the witness
// lines of both keys compare.
func TestCheckpointsVerifyAcrossTheChangeOfKey(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	record(t, tdb, sample("page.created"))
	before := sign(t, tdb, installationKey)
	log.Info(audit.WitnessMessage, before.WitnessAttrs()...)

	from := changeKey(t, tdb, installationKey, workerKey)
	if from <= before.Seq {
		t.Fatalf("the change point %d is not past the last checkpoint %d", from, before.Seq)
	}
	if _, changed, err := audit.ChangeKey(ctx, tdb.Pool, installationKey, workerKey); err != nil || changed {
		t.Fatalf("a key already in force was changed again: %v %v", changed, err)
	}
	if inForce, err := audit.KeyInForce(ctx, tdb.Pool); err != nil || !inForce.Equal(workerKey.Public()) {
		t.Fatalf("the key in force: %x %v", inForce, err)
	}
	if _, err := audit.SealWaiting(ctx, tdb.Pool); err != nil {
		t.Fatal(err)
	}
	after := sign(t, tdb, workerKey)
	log.Info(audit.WitnessMessage, after.WitnessAttrs()...)
	record(t, tdb, sample("page.updated"))
	if _, err := audit.SealWaiting(ctx, tdb.Pool); err != nil {
		t.Fatal(err)
	}
	log.Info(audit.WitnessMessage, sign(t, tdb, workerKey).WitnessAttrs()...)

	if res := check(t, tdb); !res.OK || res.Checkpoints != 3 {
		t.Fatalf("across the change of key: %+v", res)
	}
	var changes int
	if err := tdb.Pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE action = 'audit.checkpoint_key_changed'").Scan(&changes); err != nil {
		t.Fatal(err)
	}
	if changes != 1 {
		t.Fatalf("the change of key is recorded %d times", changes)
	}
	witness, err := audit.ReadWitness(&buf)
	if err != nil {
		t.Fatal(err)
	}
	res, err := audit.CompareWitness(ctx, tdb.Pool, installation, witness)
	if err != nil || !res.OK || res.Compared != 3 {
		t.Fatalf("the witness across the change: %+v, %v", res, err)
	}
}

// Once the backup service's key is in force, the installation's key signs nothing more:
// the database refuses such a checkpoint, and one slipped past it fails verification.
func TestTheInstallationKeySignsNothingAfterTheChange(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	record(t, tdb, sample("page.created"))
	sign(t, tdb, installationKey)
	changeKey(t, tdb, installationKey, workerKey)
	record(t, tdb, sample("page.updated"))
	if _, err := audit.SealWaiting(ctx, tdb.Pool); err != nil {
		t.Fatal(err)
	}
	if _, _, err := audit.SignCheckpoint(ctx, tdb.Pool, installationKey); !errors.Is(err, audit.ErrKeyNotInForce) {
		t.Fatalf("the installation's key signed after the change: %v", err)
	}
	// Straight into the table, as the application role could try: the trigger refuses it.
	forge := func(key ed25519.PrivateKey) error {
		var seq int64
		var hash []byte
		if err := tdb.Pool.QueryRow(ctx, "SELECT seq, hash FROM audit_events ORDER BY seq DESC LIMIT 1").Scan(&seq, &hash); err != nil {
			t.Fatal(err)
		}
		_, err := tdb.Pool.Exec(ctx, "INSERT INTO audit_checkpoints (seq, hash, signature, public_key) VALUES ($1, $2, $3, $4)",
			seq, hash, audit.SignedHeadForTest(key, seq, hash), []byte(key.Public().(ed25519.PublicKey)))
		return err
	}
	if err := forge(installationKey); err == nil || !strings.Contains(err.Error(), "no longer in force") {
		t.Fatalf("the database took a checkpoint of a key no longer in force: %v", err)
	}
	// With the trigger off, as the owner of the tables could, verification still names it.
	if _, err := tdb.Pool.Exec(ctx, "ALTER TABLE audit_checkpoints DISABLE TRIGGER audit_checkpoints_key_in_force"); err != nil {
		t.Fatal(err)
	}
	if err := forge(installationKey); err != nil {
		t.Fatal(err)
	}
	if res := check(t, tdb); res.OK || !strings.Contains(res.Reason, "bad signature") {
		t.Fatalf("a checkpoint of the installation's key after the change passed: %+v", res)
	}
}

// A key added without the installation's endorsement, or a change point moved, does not
// verify: whoever owns the tables but holds neither key cannot bring in their own.
func TestAKeyWithoutTheInstallationsEndorsementFails(t *testing.T) {
	for name, tamper := range map[string]string{
		"another endorser": "",
		"a moved point":    "UPDATE audit_signing_keys SET from_seq = from_seq + 1",
		"another key":      "UPDATE audit_signing_keys SET public_key = $1",
	} {
		t.Run(name, func(t *testing.T) {
			tdb := testutil.Database(t)
			ctx := context.Background()
			record(t, tdb, sample("page.created"))
			sign(t, tdb, installationKey)
			endorser := installationKey
			if tamper == "" {
				endorser = strangerKey
			}
			changeKey(t, tdb, endorser, workerKey)
			switch {
			case strings.Contains(tamper, "$1"):
				if _, err := tdb.Pool.Exec(ctx, tamper, []byte(strangerKey.Public().(ed25519.PublicKey))); err != nil {
					t.Fatal(err)
				}
			case tamper != "":
				if _, err := tdb.Pool.Exec(ctx, tamper); err != nil {
					t.Fatal(err)
				}
			}
			if res := check(t, tdb); res.OK || !strings.Contains(res.Reason, "not endorsed") {
				t.Fatalf("a key without the installation's endorsement passed: %+v", res)
			}
		})
	}
}

// A prune anchor signed by the key in force verifies; one signed by a stranger does not.
func TestAnAnchorFollowsTheKeysOfTheChain(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	record(t, tdb, sample("page.created"))
	sign(t, tdb, installationKey)
	changeKey(t, tdb, installationKey, workerKey)
	if _, err := audit.SealWaiting(ctx, tdb.Pool); err != nil {
		t.Fatal(err)
	}
	if _, err := audit.Prune(ctx, tdb.Pool, time.Now().Add(time.Hour), workerKey); err != nil {
		t.Fatal(err)
	}
	if res := check(t, tdb); !res.OK {
		t.Fatalf("an anchor of the key in force: %+v", res)
	}
	record(t, tdb, sample("page.updated"))
	if _, err := audit.SealWaiting(ctx, tdb.Pool); err != nil {
		t.Fatal(err)
	}
	if _, err := audit.Prune(ctx, tdb.Pool, time.Now().Add(time.Hour), strangerKey); err != nil {
		t.Fatal(err)
	}
	if res := check(t, tdb); res.OK || !strings.Contains(res.Reason, "anchor") {
		t.Fatalf("an anchor of a stranger's key passed: %+v", res)
	}
}

// The backup service signs only when the last checkpoint is old enough.
func TestACheckpointIsDueOnlyAfterItsInterval(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	if due, err := audit.CheckpointDue(ctx, tdb.Pool, 5*time.Minute); err != nil || !due {
		t.Fatalf("without any checkpoint: %v %v", due, err)
	}
	record(t, tdb, sample("page.created"))
	sign(t, tdb, installationKey)
	if due, err := audit.CheckpointDue(ctx, tdb.Pool, 5*time.Minute); err != nil || due {
		t.Fatalf("right after a checkpoint: %v %v", due, err)
	}
	if _, err := tdb.Pool.Exec(ctx, "UPDATE audit_checkpoints SET created_at = now() - interval '6 minutes'"); err != nil {
		t.Fatal(err)
	}
	if due, err := audit.CheckpointDue(ctx, tdb.Pool, 5*time.Minute); err != nil || !due {
		t.Fatalf("six minutes after a checkpoint: %v %v", due, err)
	}
}
