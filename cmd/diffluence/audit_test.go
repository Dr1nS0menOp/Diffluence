// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

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

// --report records what the comparison found, so the console can show it (spec 075,
// CSL-532): a match, and a mismatch with the position, written also when the command fails.
func TestAuditVerifyReportsTheWitnessComparison(t *testing.T) {
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
	if _, err := keys.LoadOrCreate(cfg.KeyFile(), true); err != nil {
		t.Fatal(err)
	}
	key, err := signingKey(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := audit.SealWaiting(ctx, pool); err != nil {
		t.Fatal(err)
	}
	cp, signed, err := audit.SignCheckpoint(ctx, pool, key)
	if err != nil || !signed {
		t.Fatalf("signed %v: %v", signed, err)
	}
	var witness bytes.Buffer
	slog.New(slog.NewJSONHandler(&witness, nil)).Info(audit.WitnessMessage, cp.WitnessAttrs()...)
	line := witness.String()

	errOut.Reset()
	if code := run([]string{"audit", "verify", "--report"}, strings.NewReader(""), &out, &errOut); code == 0 ||
		!strings.Contains(errOut.String(), "--witness") {
		t.Fatalf("a report without a witness: %d %q", code, errOut.String())
	}
	if got, _ := audit.LatestWitnessCheck(ctx, pool); got != nil {
		t.Fatalf("a refused command reported: %+v", got)
	}
	out.Reset()
	if code := run([]string{"audit", "verify", "--witness", "-", "--report"}, strings.NewReader(line), &out, &errOut); code != 0 {
		t.Fatalf("a whole chain: %d %s", code, errOut.String())
	}
	got, err := audit.LatestWitnessCheck(ctx, pool)
	if err != nil || got == nil || !got.OK || got.Compared != 1 {
		t.Fatalf("the report of a match: %+v %v", got, err)
	}
	if !strings.Contains(out.String(), "reported") {
		t.Fatalf("the command says it reported: %q", out.String())
	}

	// The owner removes the tail with its checkpoint; the witness still holds it.
	for _, q := range []string{
		"ALTER TABLE audit_events DISABLE TRIGGER audit_events_no_update_delete",
		"DELETE FROM audit_checkpoints",
		"DELETE FROM audit_personal WHERE seq >= $1",
		"DELETE FROM audit_events WHERE seq >= $1",
		"ALTER TABLE audit_events ENABLE TRIGGER audit_events_no_update_delete",
	} {
		args := []any{}
		if strings.Contains(q, "$1") {
			args = append(args, cp.Seq)
		}
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	if code := run([]string{"audit", "verify", "--witness", "-", "--report"}, strings.NewReader(line), &out, &errOut); code == 0 {
		t.Fatal("a removed tail passed the witness")
	}
	got, err = audit.LatestWitnessCheck(ctx, pool)
	if err != nil || got == nil || got.OK || got.FirstBadSeq != cp.Seq || got.Reason == "" {
		t.Fatalf("the report of a mismatch: %+v %v", got, err)
	}
}

// A process that holds only the application role, as the server of compose.yaml, compares but
// records no report, and says where the owner of the tables is; with the owner's address, as
// the backup service holds it, the report is recorded and shown (spec 077).
func TestAuditVerifyReportsAsTheOwnerOfTheTables(t *testing.T) {
	ownerAddr := commandEnv(t)
	ctx := context.Background()
	pool, err := db.Open(ctx, ownerAddr)
	if err != nil {
		t.Fatal(err)
	}
	// Closed after the role below is dropped through it.
	t.Cleanup(pool.Close)
	role := "dfl_app_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if _, err := pool.Exec(ctx, "CREATE ROLE "+role+" LOGIN PASSWORD 'app'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DROP OWNED BY "+role); err != nil {
			t.Error(err)
		}
		if _, err := pool.Exec(context.Background(), "DROP ROLE "+role); err != nil {
			t.Error(err)
		}
	})
	if err := db.GrantApp(ctx, pool, role); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.FromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := keys.LoadOrCreate(cfg.KeyFile(), true); err != nil {
		t.Fatal(err)
	}
	key, err := signingKey(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := audit.RecordSealed(ctx, pool, audit.Event{ActorType: audit.ActorSystem, ActorID: "test",
		Action: "audit.verified", ObjectType: "audit_log", Result: audit.ResultSuccess}); err != nil {
		t.Fatal(err)
	}
	cp, signed, err := audit.SignCheckpoint(ctx, pool, key)
	if err != nil || !signed {
		t.Fatalf("signed %v: %v", signed, err)
	}
	var witness bytes.Buffer
	slog.New(slog.NewJSONHandler(&witness, nil)).Info(audit.WitnessMessage, cp.WitnessAttrs()...)

	u, err := url.Parse(ownerAddr)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(role, "app")
	t.Setenv("DIFFLUENCE_DATABASE_URL", u.String())
	t.Setenv("DIFFLUENCE_MIGRATE_ON_START", "false")
	var out, errOut bytes.Buffer
	if code := run([]string{"audit", "verify", "--witness", "-", "--report"}, strings.NewReader(witness.String()), &out, &errOut); code == 0 ||
		!strings.Contains(errOut.String(), "owner of the tables") || !strings.Contains(errOut.String(), "backup") {
		t.Fatalf("a report with only the application role: %d %q", code, errOut.String())
	}
	if got, err := audit.LatestWitnessCheck(ctx, pool); err != nil || got != nil {
		t.Fatalf("a report without the owner was recorded: %+v %v", got, err)
	}
	t.Setenv("DIFFLUENCE_MIGRATE_DATABASE_URL", ownerAddr)
	if code := run([]string{"audit", "verify", "--witness", "-", "--report"}, strings.NewReader(witness.String()), &out, &errOut); code != 0 {
		t.Fatalf("a report with the owner's address: %d %s", code, errOut.String())
	}
	got, err := audit.LatestWitnessCheck(ctx, pool)
	if err != nil || got == nil || !got.OK || got.Compared != 1 {
		t.Fatalf("the report of the owner: %+v %v", got, err)
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
