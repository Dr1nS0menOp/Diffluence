// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/events"
	"github.com/diffluence/diffluence/internal/outbox"
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
