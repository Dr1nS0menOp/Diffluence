// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/health"
	"github.com/diffluence/diffluence/internal/identity"
)

func (e *env) auditWaiting() int {
	e.t.Helper()
	return e.count("SELECT count(*) FROM audit_pending")
}

// ADR-041: a worker seals what waits every second, so the chain is at most a moment behind.
func TestAWorkerSealsEverySecond(t *testing.T) {
	if app.SealEveryForTest != time.Second {
		t.Fatalf("a worker seals every %s, want 1s", app.SealEveryForTest)
	}
}

func TestTheWorkerSealsWhatWaits(t *testing.T) {
	e := newEnv(t)
	e.user("anna@example.com", identity.PermUseProduct)
	if e.auditWaiting() == 0 {
		t.Fatal("creating a person left nothing for the sealer")
	}
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { e.app.RunSealerForTest(ctx, 20*time.Millisecond); close(stopped) }()
	deadline := time.Now().Add(10 * time.Second)
	for e.auditWaiting() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-stopped
	if n := e.auditWaiting(); n != 0 {
		t.Fatalf("%d events still wait", n)
	}
	res, err := audit.Verify(context.Background(), e.app.Pool())
	if err != nil || !res.OK || res.Checked == 0 {
		t.Fatalf("%+v, %v", res, err)
	}
}

// A worker that stops seals what is left before it goes.
func TestAStoppingWorkerSealsWhatIsLeft(t *testing.T) {
	e := newEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { e.app.RunSealerForTest(ctx, time.Hour); close(stopped) }()
	e.user("anna@example.com", identity.PermUseProduct)
	if e.auditWaiting() == 0 {
		t.Fatal("nothing waits")
	}
	cancel()
	<-stopped
	if n := e.auditWaiting(); n != 0 {
		t.Fatalf("%d events were left behind", n)
	}
}

// The check in the console covers every event committed before it was asked.
func TestACheckSealsWhatWaitsFirst(t *testing.T) {
	e := newEnv(t)
	e.user("anna@example.com", identity.PermUseProduct)
	res, err := e.app.CheckAudit(context.Background())
	if err != nil || !res.OK || res.Unsealed != 0 || res.Checked == 0 {
		t.Fatalf("%+v, %v", res, err)
	}
}

// An event that waits longer than a minute means no worker seals: the health page and
// the metrics say so (spec 052).
func TestEventsLeftUnsealedShowOnTheHealthPage(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	src := app.HealthSource{Pool: e.app.Pool(), Migrations: mustMigrations(t)}
	if err := e.app.SealAudit(ctx); err != nil {
		t.Fatal(err)
	}
	facts, err := app.ReadHealthFacts(ctx, src, time.Now())
	if err != nil || facts.AuditWaiting != 0 || !facts.OldestAuditWaiting.IsZero() {
		t.Fatalf("nothing waits: %+v, %v", facts, err)
	}
	old := time.Now().Add(-2 * time.Minute)
	if err := audit.RecordStandalone(ctx, e.app.Pool(), audit.Event{ActorType: audit.ActorSystem, Action: "test.left_waiting",
		Result: audit.ResultSuccess, OccurredAt: old}); err != nil {
		t.Fatal(err)
	}
	if facts, err = app.ReadHealthFacts(ctx, src, time.Now()); err != nil {
		t.Fatal(err)
	}
	if facts.AuditWaiting != 1 || !facts.OldestAuditWaiting.Equal(old.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("waiting %d since %v", facts.AuditWaiting, facts.OldestAuditWaiting)
	}
	for _, c := range health.Evaluate(facts) {
		if c.Name == "audit" && (c.State != health.Warn || c.Code != "audit_lagging") {
			t.Fatalf("audit check: %+v", c)
		}
	}
	gauges := map[string]float64{}
	for _, g := range app.HealthGauges(facts) {
		gauges[g.Name] = g.Value
	}
	if gauges["diffluence_audit_unsealed_events"] != 1 || gauges["diffluence_audit_sealing_lag_seconds"] < 110 {
		t.Fatalf("metrics: %v", gauges)
	}
}
