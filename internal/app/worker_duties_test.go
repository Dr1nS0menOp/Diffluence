// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"errors"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/testutil"
)

// In Compose the backup service migrates and signs checkpoints, and the server holds only
// the application role (spec 070). Failure paths first.

// appRoleOnly makes an application role on a fresh, unmigrated database, and returns the
// configuration of a server that holds only that role, as compose.yaml now gives it, with the
// owner's pool for the backup service.
func appRoleOnly(t *testing.T) (config.Config, *pgxpool.Pool, string) {
	t.Helper()
	tdb := testutil.EmptyDatabase(t)
	ctx := context.Background()
	role := "dfl_app_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if _, err := tdb.Pool.Exec(ctx, "CREATE ROLE "+role+" LOGIN PASSWORD 'app'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = tdb.Pool.Exec(context.Background(), "DROP OWNED BY "+role)
		_, _ = tdb.Pool.Exec(context.Background(), "DROP ROLE "+role)
	})
	u, _ := url.Parse(tdb.URL)
	u.User = url.UserPassword(role, "app")
	public, _ := url.Parse("http://localhost")
	cfg := config.Config{
		Role: config.RoleAll, HTTPAddr: "127.0.0.1:0", PublicURL: public, InstanceName: "Test",
		DatabaseURL: u.String(), DataDir: t.TempDir(), MigrateOnStart: false,
		SessionIdleTimeout: time.Hour, SessionMaxAge: 24 * time.Hour, ContractGrace: 24 * time.Hour,
		UpgradeBackupMaxAge: 24 * time.Hour,
	}
	return cfg, tdb.Pool, role
}

func tend(t *testing.T, owner *pgxpool.Pool, role string, migs []db.Migration, grace time.Duration) app.TendReport {
	t.Helper()
	rep, err := app.TendSchema(context.Background(), owner, migs, app.TendOptions{AppRole: role,
		BackupMaxAge: 24 * time.Hour, ContractGrace: grace, Log: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

// A server without the owner's address waits on an unmigrated database, names the kind of
// problem, and looks again soon; once the backup service migrated, it starts and serves.
func TestAServerWithoutTheOwnerWaitsUntilTheBackupServiceMigrated(t *testing.T) {
	cfg, owner, role := appRoleOnly(t)
	ctx := context.Background()
	if !cfg.LeavesMigrations() {
		t.Fatal("a server with only the application role and no migration on start must leave migrations to another")
	}
	_, err := app.New(ctx, cfg, slog.New(slog.DiscardHandler))
	var refused *app.UpgradeRefused
	if !errors.As(err, &refused) {
		t.Fatalf("a server on an unmigrated database: %v", err)
	}
	if kinds := refused.Kinds(); len(kinds) != 1 || kinds[0] != app.ProblemMigrations {
		t.Fatalf("the kinds of problem: %v", kinds)
	}
	if refused.RetryAfter() >= app.RetryUpgradeEvery {
		t.Fatalf("a node that waits only for migrations looks again after %s", refused.RetryAfter())
	}
	migs, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if rep := tend(t, owner, role, migs, 24*time.Hour); rep.Expand != len(migs) {
		t.Fatalf("the backup service applied %d of %d migrations", rep.Expand, len(migs))
	}
	if rep := tend(t, owner, role, migs, 24*time.Hour); rep.Expand != 0 || rep.Contract != 0 {
		t.Fatalf("a second round did something: %+v", rep)
	}
	a, err := app.New(ctx, cfg, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("the server after the backup service migrated: %v", err)
	}
	t.Cleanup(a.Close)
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	resp, err := srv.Client().Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("readyz: %d", resp.StatusCode)
	}
}

// A refused upgrade waits for a backup in the backup service as in a node, and the server
// says so on its waiting page.
func TestTheBackupServiceWaitsForABackupBeforeAnUpgrade(t *testing.T) {
	cfg, owner, role := appRoleOnly(t)
	ctx := context.Background()
	migs, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	// An installation that holds data, upgraded once without a backup, and now meets the
	// newest migration again: it may run twice, as every migration since 0049 may.
	tend(t, owner, role, migs, 24*time.Hour)
	if _, err := owner.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", migs[len(migs)-1].Version); err != nil {
		t.Fatal(err)
	}
	if err := audit.RecordSealed(ctx, owner, audit.Event{ActorType: audit.ActorSystem, ActorID: "cli",
		Action: app.ActionUpgradeWithoutBackup, ObjectType: "backup", Result: audit.ResultSuccess}); err != nil {
		t.Fatal(err)
	}
	_, err = app.TendSchema(ctx, owner, migs, app.TendOptions{AppRole: role, BackupMaxAge: 24 * time.Hour,
		ContractGrace: 24 * time.Hour, Log: slog.New(slog.DiscardHandler)})
	var refused *app.UpgradeRefused
	if !errors.As(err, &refused) {
		t.Fatalf("an upgrade without a backup: %v", err)
	}
	_, err = app.New(ctx, cfg, slog.New(slog.DiscardHandler))
	if !errors.As(err, &refused) || !strings.Contains(strings.Join(kindNames(refused), " "), "backup") {
		t.Fatalf("the server does not name the backup: %v", err)
	}
	if err := app.NoteBackup(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if rep := tend(t, owner, role, migs, 24*time.Hour); rep.Expand != 1 {
		t.Fatalf("after a backup: %+v", rep)
	}
}

func kindNames(r *app.UpgradeRefused) []string {
	var out []string
	for _, k := range r.Kinds() {
		out = append(out, string(k))
	}
	return out
}

// The backup service runs a contract migration once every live node knows it and has run
// for the grace period, and records it; a server without the owner runs none.
func TestContractMigrationsRunInTheBackupServiceAfterTheGrace(t *testing.T) {
	cfg, owner, role := appRoleOnly(t)
	ctx := context.Background()
	migs, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	tend(t, owner, role, migs, 24*time.Hour)
	a, err := app.New(ctx, cfg, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	if err := a.RegisterNode(ctx); err != nil {
		t.Fatal(err)
	}
	next := migs[len(migs)-1].Version + 1
	contract := db.Migration{Version: next, Name: "drop_old", Phase: db.PhaseContract, Checksum: "x",
		SQL: "-- phase: contract\nCREATE TABLE contract_ran (id int);"}
	withContract := append(append([]db.Migration(nil), migs...), contract)
	a.AddMigrationForTest(contract)
	if _, err := owner.Exec(ctx, "UPDATE nodes SET max_migration = $1, started_at = now() - interval '2 days'", next); err != nil {
		t.Fatal(err)
	}
	if n, err := a.RunContractMigrations(ctx); err != nil || n != 0 {
		t.Fatalf("a server without the owner ran a contract migration: %d %v", n, err)
	}
	if _, err := owner.Exec(ctx, "UPDATE nodes SET started_at = now() - interval '1 hour'"); err != nil {
		t.Fatal(err)
	}
	if rep := tend(t, owner, role, withContract, 24*time.Hour); rep.Contract != 0 {
		t.Fatalf("ran within the grace period: %+v", rep)
	}
	if _, err := owner.Exec(ctx, "UPDATE nodes SET started_at = now() - interval '25 hours'"); err != nil {
		t.Fatal(err)
	}
	if rep := tend(t, owner, role, withContract, 24*time.Hour); rep.Contract != 1 {
		t.Fatalf("did not run after the grace period: %+v", rep)
	}
	if _, err := a.Pool().Exec(ctx, "SELECT * FROM contract_ran"); err != nil {
		t.Fatalf("the application role cannot read what the contract migration made: %v", err)
	}
	if _, err := audit.SealWaiting(ctx, owner); err != nil {
		t.Fatal(err)
	}
	var recorded int
	if err := owner.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE action = 'system.contract_migrated'").Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 1 {
		t.Fatalf("the contract migration is recorded %d times", recorded)
	}
}

// Once the backup service signs, the server's own job stops signing, and the chain verifies
// across the change with the server's key alone.
func TestTheServerStopsSigningOnceTheBackupServiceDoes(t *testing.T) {
	e, logs := newEnvWithLog(t)
	ctx := context.Background()
	e.user("anna@example.com")
	if err := e.app.SignCheckpointForTest(ctx); err != nil {
		t.Fatal(err)
	}
	worker := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{5}, ed25519.SeedSize))
	var witness bytes.Buffer
	cp, signed, err := audit.WorkerCheckpoint(ctx, e.app.Pool(), e.app.InstallationKeyForTest(), worker, 5*time.Minute)
	if err != nil || !signed {
		t.Fatalf("the backup service's first checkpoint: %v %v", signed, err)
	}
	slog.New(slog.NewJSONHandler(&witness, nil)).Info(audit.WitnessMessage, cp.WitnessAttrs()...)
	if _, signed, err := audit.WorkerCheckpoint(ctx, e.app.Pool(), e.app.InstallationKeyForTest(), worker, 5*time.Minute); err != nil || signed {
		t.Fatalf("a second checkpoint within five minutes: %v %v", signed, err)
	}
	e.user("bo@example.com")
	if err := e.app.SignCheckpointForTest(ctx); err != nil {
		t.Fatalf("the server's job fails once the backup service signs: %v", err)
	}
	var byServer int
	if err := e.app.Pool().QueryRow(ctx, "SELECT count(*) FROM audit_checkpoints WHERE seq >= (SELECT min(from_seq) FROM audit_signing_keys)").Scan(&byServer); err != nil {
		t.Fatal(err)
	}
	if byServer != 1 {
		t.Fatalf("checkpoints after the change: %d, want only the backup service's", byServer)
	}
	res, err := e.app.CheckAudit(ctx)
	if err != nil || !res.OK || res.Checkpoints != 2 {
		t.Fatalf("the chain across the change: %+v %v", res, err)
	}
	all, err := audit.ReadWitness(strings.NewReader(logs.String() + witness.String()))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := e.app.CheckWitnessForTest(ctx, all); err != nil || !got.OK || got.Compared != 2 {
		t.Fatalf("the witness of both: %+v %v", got, err)
	}
}
