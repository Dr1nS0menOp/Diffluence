// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/events"
	"github.com/diffluence/diffluence/internal/outbox"
)

// eventsLagBudget is the pass mark of spec 068 (D9): an event handled by every consumer
// within a second (p95) at four times the model, 20 events a second.
const eventsLagBudget = time.Second

// lagRecorder keeps, per consumer, how long after its write each event was handled.
type lagRecorder struct {
	mu      sync.Mutex
	lags    map[string][]time.Duration
	handled atomic.Int64
}

func (l *lagRecorder) wrap(c events.Consumer) events.Consumer {
	inner := c.Handle
	c.Handle = func(ctx context.Context, data []byte) error {
		err := inner(ctx, data)
		var m outbox.Message
		if jerr := json.Unmarshal(data, &m); jerr != nil {
			return jerr
		}
		l.mu.Lock()
		l.lags[c.Name] = append(l.lags[c.Name], time.Since(m.CreatedAt))
		l.mu.Unlock()
		l.handled.Add(1)
		return err
	}
	return c
}

func (l *lagRecorder) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lags = map[string][]time.Duration{}
	l.handled.Store(0)
}

// quantile is the q-quantile of a list, which it sorts.
func quantile(list []time.Duration, q float64) time.Duration {
	if len(list) == 0 {
		return 0
	}
	slices.Sort(list)
	return list[min(len(list)-1, int(float64(len(list))*q))]
}

// queueStats is what pg_stat_statements and the statistics views say about one stage.
type queueStats struct {
	sequencerMs, cursorMs, allMs  float64
	sequencerCalls, cursorCalls   int64
	walBytes, outboxDead, logDead int64
}

// readQueueStats sorts the statements since the last reset into the sequencer's, the
// cursors' and the rest. BEGIN and COMMIT count in the rest.
func readQueueStats(t *testing.T, pool *pgxpool.Pool, walBefore int64) queueStats {
	t.Helper()
	var s queueStats
	err := pool.QueryRow(context.Background(), `SELECT
		coalesce(sum(total_exec_time) FILTER (WHERE seq), 0), coalesce(sum(calls) FILTER (WHERE seq), 0),
		coalesce(sum(total_exec_time) FILTER (WHERE cur), 0), coalesce(sum(calls) FILTER (WHERE cur), 0),
		coalesce(sum(total_exec_time), 0)
		FROM (SELECT total_exec_time, calls,
			(query ~ 'event_head|event_transport|pg_notify|pg_advisory_xact_lock|INSERT INTO event_log|FROM outbox_events WHERE published_at IS NULL|DELETE FROM outbox_events WHERE id') AS seq,
			(query ~ 'event_cursors|event_retries|synchronous_commit|FROM event_log\s+WHERE lane') AS cur
			FROM pg_stat_statements WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())) q`).
		Scan(&s.sequencerMs, &s.sequencerCalls, &s.cursorMs, &s.cursorCalls, &s.allMs)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `SELECT (SELECT wal_bytes FROM pg_stat_wal)::bigint - $1,
		(SELECT n_dead_tup FROM pg_stat_user_tables WHERE relname = 'outbox_events'),
		(SELECT n_dead_tup FROM pg_stat_user_tables WHERE relname = 'event_log')`, walBefore).
		Scan(&s.walBytes, &s.outboxDead, &s.logDead); err != nil {
		t.Fatal(err)
	}
	return s
}

func resetQueueStats(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var wal int64
	if _, err := pool.Exec(context.Background(), "SELECT pg_stat_statements_reset()"); err != nil {
		t.Fatalf("pg_stat_statements is needed (scripts/stress/real-data.sh loads it): %v", err)
	}
	if err := pool.QueryRow(context.Background(), "SELECT wal_bytes::bigint FROM pg_stat_wal").Scan(&wal); err != nil {
		t.Fatal(err)
	}
	return wal
}

// realEvent is a page.published as a save writes it, for a real page of the dump.
type realEvent struct{ page, space, actor uuid.UUID }

func pickRealEvents(t *testing.T, pool *pgxpool.Pool) []realEvent {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT p.id, p.space_id, u.id FROM
		(SELECT id, space_id, row_number() OVER () AS n FROM pages WHERE kind = 'page' AND status = 'published' ORDER BY random() LIMIT 3000) p
		JOIN (SELECT id, row_number() OVER () AS n FROM users WHERE status = 'active' ORDER BY random() LIMIT 3000) u ON u.n = p.n`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (realEvent, error) {
		var e realEvent
		return e, r.Scan(&e.page, &e.space, &e.actor)
	})
	if err != nil || len(out) == 0 {
		t.Fatalf("%d pages, err %v", len(out), err)
	}
	return out
}

func writeRealEvent(ctx context.Context, pool *pgxpool.Pool, e realEvent) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		_, err := outbox.Enqueue(ctx, tx, "page.published", map[string]any{
			"page_id": e.page, "space_id": e.space, "actor_id": e.actor, "version": 2, "first": false})
		return err
	})
}

// TestEventsOnRealData measures spec 068 on a restored dump of the seeded data
// (scripts/stress/real-data.sh): B1, the whole path with its four real consumers under 32
// writers at rising rates; B4, the same without LISTEN; a backlog after downtime (part of
// B5); and B2, the notifications consumer alone on a space with many watchers.
func TestEventsOnRealData(t *testing.T) {
	realURL := os.Getenv("DIFFLUENCE_REALDATA_URL")
	if realURL == "" {
		t.Skip("DIFFLUENCE_REALDATA_URL is not set; see scripts/stress/real-data.sh")
	}
	public, _ := url.Parse("http://localhost")
	cfg := config.Config{Role: config.RoleAll, HTTPAddr: "127.0.0.1:0", PublicURL: public, InstanceName: "Real data",
		DatabaseURL: realURL, DataDir: t.TempDir(), MigrateOnStart: true, SessionIdleTimeout: time.Hour, SessionMaxAge: 24 * time.Hour}
	ctx := context.Background()
	a, err := app.New(ctx, cfg, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	pool := a.Pool()
	for _, sql := range []string{"UPDATE event_transport SET transport = 'postgres'", "DELETE FROM outbox_events"} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	picks := pickRealEvents(t, pool)

	rec := &lagRecorder{lags: map[string][]time.Duration{}}
	runner := a.EventRunner()
	for i, c := range runner.Consumers {
		runner.Consumers[i] = rec.wrap(c)
	}
	consumers := int64(len(runner.Consumers))
	if err := runner.EnsureCursors(ctx); err != nil {
		t.Fatal(err)
	}

	// start runs the worker's loops: the relay with the sequencer every 250 ms (D1), the
	// consumers, and LISTEN unless it is off.
	start := func(listen bool) (stop func()) {
		loopCtx, cancel := context.WithCancel(ctx)
		var wg sync.WaitGroup
		relay := &outbox.Relay{Pool: pool, Sequencer: events.Sequencer{}, Interval: 250 * time.Millisecond}
		wg.Go(func() { relay.Run(loopCtx) })
		runner.Wake = nil
		if listen {
			runner.Wake = events.NewWakeups()
			wg.Go(func() { events.Listen(loopCtx, realURL, runner.Wake, slog.New(slog.DiscardHandler)) })
		}
		wg.Go(func() { runner.Run(loopCtx) })
		return func() { cancel(); wg.Wait() }
	}

	// stage writes events at a rate from 32 writers for a while, waits until every consumer
	// handled every one, and reports.
	stage := func(name string, rate float64, listen bool) (p95 time.Duration, achieved float64) {
		stop := start(listen)
		defer stop()
		time.Sleep(500 * time.Millisecond)
		rec.reset()
		walBefore := resetQueueStats(t, pool)
		const writers, length = 32, 20 * time.Second
		var written atomic.Int64
		began := time.Now()
		var wg sync.WaitGroup
		for w := range writers {
			wg.Go(func() {
				every := time.Duration(float64(writers) / rate * float64(time.Second))
				next := began.Add(time.Duration(w) * every / writers)
				for i := w; time.Since(began) < length; i += writers {
					time.Sleep(time.Until(next))
					next = next.Add(every)
					if err := writeRealEvent(ctx, pool, picks[i%len(picks)]); err != nil {
						t.Error(err)
						return
					}
					written.Add(1)
				}
			})
		}
		wg.Wait()
		took := time.Since(began)
		achieved = float64(written.Load()) / took.Seconds()
		deadline := time.Now().Add(2 * time.Minute)
		for rec.handled.Load() < written.Load()*consumers {
			if time.Now().After(deadline) {
				t.Errorf("%s: handled %d of %d", name, rec.handled.Load(), written.Load()*consumers)
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		drained := time.Since(began)
		s := readQueueStats(t, pool, walBefore)
		n := float64(written.Load())
		rec.mu.Lock()
		var all []time.Duration
		line := ""
		for _, c := range runner.Consumers {
			lags := rec.lags[c.Name]
			all = append(all, lags...)
			line += fmt.Sprintf(" %s p50 %v p95 %v;", c.Name, quantile(lags, 0.5).Round(time.Millisecond), quantile(lags, 0.95).Round(time.Millisecond))
		}
		rec.mu.Unlock()
		p95 = quantile(all, 0.95)
		t.Logf("%s: asked %.0f/s, wrote %.1f/s (%d), all handled after %v;%s", name, rate, achieved, written.Load(), drained.Round(100*time.Millisecond), line)
		t.Logf("%s: sequencer %.3f ms and %.1f statements per event, cursors %.3f ms and %.1f statements per event; queue %.1f ms of database time a second, all statements %.1f ms a second; WAL %.0f bytes per event; dead rows outbox %d, log %d",
			name, s.sequencerMs/n, float64(s.sequencerCalls)/n, s.cursorMs/n, float64(s.cursorCalls)/n,
			(s.sequencerMs+s.cursorMs)/drained.Seconds(), s.allMs/drained.Seconds(), float64(s.walBytes)/n, s.outboxDead, s.logDead)
		return p95, achieved
	}

	// B1: the model, four times, the peak, then doubling until the lag passes its mark.
	for _, rate := range []float64{5, 20, 85, 170, 340, 680} {
		p95, achieved := stage(fmt.Sprintf("B1 %.0f/s", rate), rate, true)
		if rate == 20 && p95 > eventsLagBudget {
			t.Errorf("at four times the model the lag p95 is %v, mark %v", p95, eventsLagBudget)
		}
		if p95 > 5*eventsLagBudget || achieved < 0.8*rate {
			break
		}
	}
	// B4: wake-ups off, polling only.
	stage("B4 20/s without LISTEN", 20, false)

	// B5, the backlog: a worker down while 10,000 events wait.
	const backlog = 10_000
	rec.reset()
	began := time.Now()
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		for i := range backlog {
			p := picks[i%len(picks)]
			if _, err := outbox.Enqueue(ctx, tx, "page.published", map[string]any{
				"page_id": p.page, "space_id": p.space, "actor_id": p.actor, "version": 2, "first": false}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("B5 backlog: %d events written in %v", backlog, time.Since(began).Round(time.Millisecond))
	began = time.Now()
	stop := start(true)
	for rec.handled.Load() < backlog*consumers && time.Since(began) < 10*time.Minute {
		time.Sleep(100 * time.Millisecond)
	}
	worked := time.Since(began)
	stop()
	t.Logf("B5 backlog: %d events through %d consumers in %v, %.0f events a second", rec.handled.Load()/consumers, consumers,
		worked.Round(100*time.Millisecond), float64(rec.handled.Load()/consumers)/worked.Seconds())

	// B2: the notifications consumer alone, on page.published in the space most watched,
	// with every active person of the dump watching it if the dump has fewer.
	var space uuid.UUID
	var watchers int
	if err := pool.QueryRow(ctx, `SELECT s.id, (SELECT count(*) FROM watches w WHERE w.target_type = 'space' AND w.target_id = s.id)
		FROM spaces s ORDER BY 2 DESC, (SELECT count(*) FROM pages p WHERE p.space_id = s.id) DESC LIMIT 1`).Scan(&space, &watchers); err != nil {
		t.Fatal(err)
	}
	t.Logf("B2: the most watched space has %d watchers", watchers)
	if watchers < 1000 {
		if _, err := pool.Exec(ctx, `INSERT INTO watches (user_id, target_type, target_id)
			SELECT id, 'space', $1 FROM users WHERE status = 'active' ORDER BY id LIMIT 1000 ON CONFLICT DO NOTHING`, space); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM watches WHERE target_type = 'space' AND target_id = $1", space).Scan(&watchers); err != nil {
			t.Fatal(err)
		}
		t.Logf("B2: gave the space %d watchers", watchers)
	}
	var notifier events.Consumer
	for _, c := range a.EventRunner().Consumers {
		if c.Name == "notifications" {
			notifier = c
		}
	}
	alone := a.EventRunner()
	alone.Consumers = []events.Consumer{notifier}
	if _, err := alone.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	pages, err := pool.Query(ctx, "SELECT id FROM pages WHERE space_id = $1 AND kind = 'page' AND status = 'published' LIMIT 100", space)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := pgx.CollectRows(pages, pgx.RowTo[uuid.UUID])
	if err != nil || len(ids) == 0 {
		t.Fatalf("%d pages, err %v", len(ids), err)
	}
	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		for i, id := range ids {
			actor := picks[rand.IntN(len(picks))].actor
			if _, err := outbox.Enqueue(ctx, tx, "page.published", map[string]any{
				"page_id": id, "space_id": space, "actor_id": actor, "version": 2 + i, "first": false}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
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
	var before int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM notifications").Scan(&before); err != nil {
		t.Fatal(err)
	}
	began = time.Now()
	n, err := alone.Drain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	took := time.Since(began)
	var after int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM notifications").Scan(&after); err != nil {
		t.Fatal(err)
	}
	perEvent := took / time.Duration(max(n, 1))
	t.Logf("B2: %d page.published to %d watchers: %v an event, %d notifications written, %.0f notifications a second; one lane at this rate carries %.1f such events a second, four lanes %.1f",
		n, watchers, perEvent.Round(time.Millisecond), after-before, float64(after-before)/took.Seconds(),
		1/perEvent.Seconds(), 4/perEvent.Seconds())
	if breach, err := events.Check(ctx, pool); err != nil || breach != nil {
		t.Errorf("after the measurement: breach %v, err %v", breach, err)
	}
}
