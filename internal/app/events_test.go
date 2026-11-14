// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/events"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Once the events run through PostgreSQL the worker's own consumers take them from the event
// log, and a person gets the notification the queue gave them before (spec 068).
func TestTheConsumersOfTheWorkerTakeEventsFromTheLog(t *testing.T) {
	e, owner, space, parent, _ := exportSetup(t)
	ctx := context.Background()
	pool := e.app.Pool()
	if _, err := pool.Exec(ctx, "UPDATE event_transport SET transport = 'postgres'"); err != nil {
		t.Fatal(err)
	}
	runner := e.app.EventRunner()
	if err := runner.EnsureCursors(ctx); err != nil {
		t.Fatal(err)
	}
	owner.export(map[string]any{"space_id": space, "format": "text", "root_id": parent})
	relay := &outbox.Relay{Pool: pool, Sequencer: events.Sequencer{}}
	for {
		n, err := relay.RunOnce(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
	}
	if n, err := runner.Drain(ctx); err != nil || n == 0 {
		t.Fatalf("handed over %d, err %v", n, err)
	}
	var count, cursors int
	err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM notifications WHERE type = 'export_ready'),
		(SELECT count(DISTINCT consumer) FROM event_cursors WHERE pos > 0)`).Scan(&count, &cursors)
	if err != nil || count != 1 {
		t.Fatalf("notifications %d, err %v", count, err)
	}
	// Every one of the four consumers passed at least one event.
	if cursors != 4 {
		t.Fatalf("%d consumers moved", cursors)
	}
	breach, err := events.Check(ctx, pool)
	if err != nil || breach != nil {
		t.Fatalf("breach %v, err %v", breach, err)
	}
}

// A running worker follows an operator's switch: before it a person hears of a mention through
// NATS, after it through the event log, within seconds; once its NATS consumers have nothing
// left, the drain is noted for the release without NATS (spec 068, 2.11). Every NATS durable
// is asked, by the name the log's consumers share with it.
func TestARunningWorkerFollowsTheSwitchAndNotesTheDrain(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", anna.id, permissions.TemplateViewer)
	before := owner.page(space, nil, "Before", "x")
	after := owner.page(space, nil, "After", "x")
	ctx := context.Background()
	pool := e.app.Pool()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func(a *app.App) { done <- a.Serve(serveCtx, ln) }(e.app)
	t.Cleanup(func() {
		stop()
		<-done
	})
	unread := func(want float64) func() bool {
		return func() bool { return anna.ok("GET", "/api/v1/me/notifications/count", nil)["unread"].(float64) == want }
	}

	owner.publish(before, "Before", 1, commentBody("hello ", mention(anna.id, "Anna")))
	waitFor(t, "the mention through NATS", unread(1))
	if err := e.app.NoteNATSDrainedForTest(ctx); err != nil {
		t.Fatal(err)
	}
	if s, err := events.ReadState(ctx, pool); err != nil || !s.DrainedAt.IsZero() {
		t.Fatalf("drained at %v on NATS, err %v", s.DrainedAt, err)
	}

	if changed, err := app.SwitchEvents(ctx, pool, events.TransportPostgres); err != nil || !changed {
		t.Fatalf("switched %v, err %v", changed, err)
	}
	owner.publish(after, "After", 1, commentBody("hello again ", mention(anna.id, "Anna")))
	waitFor(t, "the mention through the event log", unread(2))
	var cursors int
	if err := pool.QueryRow(ctx, "SELECT count(DISTINCT consumer) FROM event_cursors WHERE pos > 0").Scan(&cursors); err != nil || cursors != 4 {
		t.Fatalf("%d consumers passed an event of the log, err %v", cursors, err)
	}
	waitFor(t, "the drain of NATS", func() bool {
		if err := e.app.NoteNATSDrainedForTest(ctx); err != nil {
			t.Fatal(err)
		}
		s, err := events.ReadState(ctx, pool)
		return err == nil && !s.DrainedAt.IsZero()
	})
	if problem, err := events.NATSRemovalProblem(ctx, pool); err != nil || problem != "" {
		t.Fatalf("the release without NATS is refused: %q, err %v", problem, err)
	}
}

// waitFor polls a condition with a deadline far beyond any machine's pace.
func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("waited a minute: %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// System health reads how long an event waits for the sequencer and how far the slowest
// consumer is behind.
func TestSystemHealthReadsTheEventLag(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	pool := e.app.Pool()
	src := app.HealthSource{Pool: pool}
	for _, sql := range []string{
		"INSERT INTO event_log (pos, lane, id, type, payload, created_at, sequenced_at) VALUES (1, 2, gen_random_uuid(), 'page.updated', '{}', now(), now() - interval '10 minutes')",
		"UPDATE event_head SET pos = 1",
		"INSERT INTO event_cursors (consumer, lane, pos) VALUES ('webhooks', 2, 0), ('collab', 2, 1)",
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	f, err := app.ReadHealthFacts(ctx, src, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if f.EventLagConsumer != "webhooks" || f.EventLag < 9*time.Minute || !f.OldestEventWaiting.IsZero() {
		t.Fatalf("lag %v of %q, waiting since %v", f.EventLag, f.EventLagConsumer, f.OldestEventWaiting)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO outbox_events (id, type, payload, created_at) VALUES (gen_random_uuid(), 'page.updated', '{}', now() - interval '2 minutes')"); err != nil {
		t.Fatal(err)
	}
	if f, err = app.ReadHealthFacts(ctx, src, time.Now()); err != nil || time.Since(f.OldestEventWaiting) < 2*time.Minute {
		t.Fatalf("waiting since %v, err %v", f.OldestEventWaiting, err)
	}
}

// Each consumer has its own series of lag and retries, under one metric each.
func TestEventGaugesNameEachConsumer(t *testing.T) {
	gauges := app.EventGauges([]events.ConsumerState{
		{Consumer: "webhooks", Lag: 3 * time.Second, Retries: 2},
		{Consumer: "collab"},
	})
	want := []string{
		`diffluence_event_lag_seconds{consumer="webhooks"} 3`, `diffluence_event_lag_seconds{consumer="collab"} 0`,
		`diffluence_event_retries{consumer="webhooks"} 2`, `diffluence_event_retries{consumer="collab"} 0`,
	}
	if len(gauges) != len(want) {
		t.Fatalf("%d gauges", len(gauges))
	}
	for i, g := range gauges {
		if got := fmt.Sprintf("%s{%s=%q} %g", g.Name, g.Label.Name, g.Label.Value, g.Value); got != want[i] {
			t.Errorf("gauge %d is %s, want %s", i, got, want[i])
		}
	}
}
