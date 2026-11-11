// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/identity"
)

// newEnvWithLog runs the server with a log at warning level, as a quiet installation
// would, and gives the test that log.
func newEnvWithLog(t *testing.T) (*env, *lockedBuffer) {
	t.Helper()
	logs := &lockedBuffer{}
	log := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	return newEnvLogging(t, func(*config.Config) {}, log), logs
}

func (e *env) witness(logs *lockedBuffer) []audit.Checkpoint {
	e.t.Helper()
	witness, err := audit.ReadWitness(strings.NewReader(logs.String()))
	if err != nil {
		e.t.Fatal(err)
	}
	return witness
}

// ADR-039: whoever owns the database can remove the tail of the chain together with
// its checkpoints, and the chain still verifies. The copy in the application log does not.
func TestTheWitnessLogShowsATailRemovedFromTheDatabase(t *testing.T) {
	e, logs := newEnvWithLog(t)
	ctx := context.Background()
	e.user("anna@example.com", identity.PermUseProduct)
	if err := e.app.SignCheckpointForTest(ctx); err != nil {
		t.Fatal(err)
	}
	e.createUser("bo@example.com", identity.PermUseProduct)
	if err := e.app.SignCheckpointForTest(ctx); err != nil {
		t.Fatal(err)
	}
	witness := e.witness(logs)
	if len(witness) != 2 {
		t.Fatalf("a quieter log level must not drop the witness: %d lines in %s", len(witness), logs.String())
	}
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, audit.WitnessMessage) && (strings.Contains(line, "@") || strings.Contains(line, "Test ")) {
			t.Fatalf("a witness line holds personal data: %s", line)
		}
	}
	res, err := e.app.CheckWitnessForTest(ctx, witness)
	if err != nil || !res.OK || res.Compared != 2 {
		t.Fatalf("a whole chain: %+v, %v", res, err)
	}

	// The owner of the tables removes the last checkpoint and everything after the first,
	// sealed by then (ADR-041).
	if err := e.app.SealAudit(ctx); err != nil {
		t.Fatal(err)
	}
	first := witness[0].Seq
	e.sql("ALTER TABLE audit_events DISABLE TRIGGER audit_events_no_update_delete")
	e.sql("DELETE FROM audit_checkpoints WHERE seq > $1", first)
	e.sql("DELETE FROM audit_personal WHERE seq > $1", first)
	e.sql("DELETE FROM audit_events WHERE seq > $1", first)
	e.sql("ALTER TABLE audit_events ENABLE TRIGGER audit_events_no_update_delete")
	if check, err := e.app.CheckAudit(ctx); err != nil || !check.OK {
		t.Fatalf("the shortened chain should still hold on its own, which is why the witness exists: %+v, %v", check, err)
	}
	res, err = e.app.CheckWitnessForTest(ctx, witness)
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.FirstBadSeq != witness[1].Seq || !strings.Contains(res.Reason, "tail") {
		t.Fatalf("a removed tail passed the witness: %+v", res)
	}
}

// A witness line must carry a signature the key made, so a forged log proves nothing.
func TestAForgedWitnessLineIsRefused(t *testing.T) {
	e, logs := newEnvWithLog(t)
	ctx := context.Background()
	e.user("anna@example.com", identity.PermUseProduct)
	if err := e.app.SignCheckpointForTest(ctx); err != nil {
		t.Fatal(err)
	}
	witness := e.witness(logs)
	if len(witness) != 1 {
		t.Fatalf("witness lines: %d", len(witness))
	}
	witness[0].Hash = bytes.Repeat([]byte{1}, len(witness[0].Hash))
	res, err := e.app.CheckWitnessForTest(ctx, witness)
	if err != nil || res.OK || !strings.Contains(res.Reason, "signature") {
		t.Fatalf("a forged line passed: %+v, %v", res, err)
	}
}

// ADR-039: five minutes is the most the tail can lose before the witness holds a copy.
func TestTheHeadOfTheChainIsSignedEveryFiveMinutes(t *testing.T) {
	e := newEnv(t)
	for _, task := range e.app.PeriodicTasks() {
		if task.Name == "audit-checkpoint" {
			if task.Every != 5*time.Minute {
				t.Fatalf("checkpoints every %s, want 5m", task.Every)
			}
			return
		}
	}
	t.Fatal("no job signs checkpoints")
}

// A quiet instance does not sign the same head again every five minutes.
func TestNothingNewMeansNoNewCheckpoint(t *testing.T) {
	e, logs := newEnvWithLog(t)
	ctx := context.Background()
	e.user("anna@example.com", identity.PermUseProduct)
	for range 3 {
		if err := e.app.SignCheckpointForTest(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(e.witness(logs)); got != 1 {
		t.Fatalf("checkpoints without anything new in between: %d, want 1", got)
	}
	if e.auditActions("audit.checkpoint_signed") != 1 {
		t.Fatal("each checkpoint is recorded once")
	}
}
