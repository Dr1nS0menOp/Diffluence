// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/keys"
)

// ADR-039: `audit verify --witness` compares the chain with the checkpoints the
// application log kept, and fails on a tail that was removed with its checkpoints.
func TestAuditVerifyComparesTheChainWithTheWitnessLog(t *testing.T) {
	url := commandEnv(t)
	ctx := context.Background()
	var out, errOut bytes.Buffer
	if code := run([]string{"user", "create-admin", "--email", "first@example.com", "--name", "First"},
		strings.NewReader("correct horse battery staple\n"), &out, &errOut); code != 0 {
		t.Fatalf("create-admin: %d %s", code, errOut.String())
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	cfg, err := config.FromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	// The first start of a server makes the key; this command only reads it.
	if _, err := keys.LoadOrCreate(cfg.KeyFile(), true); err != nil {
		t.Fatal(err)
	}
	key, err := signingKey(cfg)
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "app.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewJSONHandler(logFile, nil))
	// A worker seals the events of create-admin within a second (ADR-041).
	if _, err := audit.SealWaiting(ctx, pool); err != nil {
		t.Fatal(err)
	}
	cp, signed, err := audit.SignCheckpoint(ctx, pool, key)
	if err != nil || !signed {
		t.Fatalf("signed %v: %v", signed, err)
	}
	log.Info(audit.WitnessMessage, cp.WitnessAttrs()...)
	if err := logFile.Close(); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if code := run([]string{"audit", "verify", "--witness", logPath}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("a whole chain: %d %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "witness log OK (1 checkpoints match") {
		t.Fatalf("the witness was not compared: %q", out.String())
	}

	// The owner of the tables removes the checkpoint and the tail from it on.
	owner := []string{
		"ALTER TABLE audit_events DISABLE TRIGGER audit_events_no_update_delete",
		"DELETE FROM audit_checkpoints",
		"DELETE FROM audit_personal WHERE seq >= $1",
		"DELETE FROM audit_events WHERE seq >= $1",
		"ALTER TABLE audit_events ENABLE TRIGGER audit_events_no_update_delete",
	}
	for _, q := range owner {
		var err error
		if strings.Contains(q, "$1") {
			_, err = pool.Exec(ctx, q, cp.Seq)
		} else {
			_, err = pool.Exec(ctx, q)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if code := run([]string{"audit", "verify"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("without the witness the shortened chain holds, which is why it exists: %d %s", code, errOut.String())
	}
	errOut.Reset()
	if code := run([]string{"audit", "verify", "--witness", logPath}, strings.NewReader(""), &out, &errOut); code == 0 {
		t.Fatal("a removed tail passed the witness")
	}
	if !strings.Contains(errOut.String(), "witness") {
		t.Fatalf("the refusal says why: %q", errOut.String())
	}
}

// A log without checkpoints is the wrong file, or a collector that kept nothing.
func TestAuditVerifyRefusesAWitnessLogWithoutCheckpoints(t *testing.T) {
	commandEnv(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"audit", "verify", "--witness", "-"}, strings.NewReader(`{"msg":"request"}`+"\n"), &out, &errOut); code == 0 {
		t.Fatal("an empty witness log passed")
	}
	if !strings.Contains(errOut.String(), "no audit checkpoints") {
		t.Fatalf("the refusal says why: %q", errOut.String())
	}
}
