// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/testutil"
)

// natsStream stands in for NATS: it keeps the id of every event published to it, from several
// relays at once.
type natsStream struct {
	mu  sync.Mutex
	ids []uuid.UUID
	// held, when set, is called with each event before it is kept, while the relay holds
	// the event's row.
	held func()
}

func (n *natsStream) Publish(_ context.Context, _ string, _ []byte, msgID string) error {
	id, err := uuid.Parse(msgID)
	if err != nil {
		return err
	}
	if n.held != nil {
		n.held()
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.ids = append(n.ids, id)
	return nil
}

func (n *natsStream) published() []uuid.UUID {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.ids)
}

// theConsumers are the four consumers of the worker, as the switch creates their cursors.
var theConsumers = []string{"notifications", "automation", "webhooks", "collab"}

func switchTo(t *testing.T, pool *pgxpool.Pool, to Transport) bool {
	t.Helper()
	changed, err := Switch(context.Background(), pool, to, theConsumers)
	if err != nil {
		t.Fatal(err)
	}
	return changed
}

// addNode registers a node whose newest migration is maxMigration, last seen ago.
func addNode(t *testing.T, pool *pgxpool.Pool, maxMigration int, ago time.Duration) {
	t.Helper()
	exec(t, pool, `INSERT INTO nodes (id, role, version, max_migration, last_seen_at)
		VALUES ($1, 'all', $2, $3, now() - make_interval(secs => $4))`,
		uuid.New(), fmt.Sprint("release-", maxMigration), maxMigration, ago.Seconds())
}

func logged(t *testing.T, pool *pgxpool.Pool) []uuid.UUID {
	t.Helper()
	rows, err := pool.Query(context.Background(), "SELECT id FROM event_log ORDER BY pos")
	if err != nil {
		t.Fatal(err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func readState(t *testing.T, pool *pgxpool.Pool) State {
	t.Helper()
	s, err := ReadState(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// oneWay fails unless every committed event went exactly one way: to NATS or to the log.
func oneWay(t *testing.T, committed, published, inLog []uuid.UUID) {
	t.Helper()
	seen := map[uuid.UUID]string{}
	for _, way := range []struct {
		name string
		ids  []uuid.UUID
	}{{"NATS", published}, {"the log", inLog}} {
		for _, id := range way.ids {
			if before, ok := seen[id]; ok {
				t.Fatalf("event %s went to %s and to %s", id, before, way.name)
			}
			seen[id] = way.name
		}
	}
	for _, id := range committed {
		if _, ok := seen[id]; !ok {
			t.Fatalf("event %s went nowhere", id)
		}
	}
	if len(seen) != len(committed) {
		t.Fatalf("%d events arrived, %d were committed", len(seen), len(committed))
	}
}

// writeEvents commits n events, each in its own transaction, and returns their ids.
func writeEvents(ctx context.Context, pool *pgxpool.Pool, writer, n int) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	for i := range n {
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			id, err := outbox.Enqueue(ctx, tx, "page.updated", map[string]any{"page_id": fmt.Sprint(writer, "-", i)})
			ids = append(ids, id)
			return err
		})
		if err != nil {
			return ids, err
		}
	}
	return ids, nil
}

// Relays of this release run while people save, and the switch comes while one of them is
// publishing a batch: every event goes to NATS or to the log, never both and never neither,
// and once the switch is committed nothing more goes to NATS. The consumers, started only
// afterwards, get every event of the log.
func TestEachEventGoesOneWayAcrossTheSwitch(t *testing.T) {
	tdb := testutil.Database(t)
	pool := tdb.Pool
	ctx := context.Background()
	// The operator's command has a connection of its own, so the test can see it wait.
	cfg, err := pgxpool.ParseConfig(tdb.URL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = "events-switch"
	operator, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer operator.Close()
	switched := make(chan struct{})
	var atSwitch atomic.Int64
	var publishes atomic.Int32
	nats := &natsStream{}
	nats.held = func() {
		if publishes.Add(1) != 20 {
			return
		}
		go func() {
			defer close(switched)
			if changed, err := Switch(ctx, operator, TransportPostgres, theConsumers); err != nil || !changed {
				t.Errorf("switched %v, err %v", changed, err)
			}
			atSwitch.Store(int64(len(nats.published())))
		}()
		// This relay holds its batch until the switch is done or waits for the events lock.
		if !holdUntil(ctx, pool, switched) {
			t.Error("the switch neither finished nor waited for the events lock")
		}
	}
	stop := make(chan struct{})
	var relays sync.WaitGroup
	for range 3 {
		relays.Go(func() {
			relay := &outbox.Relay{Pool: pool, Publisher: nats, Sequencer: Sequencer{BatchSize: 5}, BatchSize: 5}
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := relay.RunOnce(ctx); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	var mu sync.Mutex
	var committed []uuid.UUID
	keep := func(ids []uuid.UUID) {
		mu.Lock()
		defer mu.Unlock()
		committed = append(committed, ids...)
	}
	var writers sync.WaitGroup
	for w := range 4 {
		writers.Go(func() {
			ids, err := writeEvents(ctx, pool, w, 40)
			keep(ids)
			if err != nil {
				t.Error(err)
				return
			}
			<-switched
			ids, err = writeEvents(ctx, pool, 100+w, 40)
			keep(ids)
			if err != nil {
				t.Error(err)
			}
		})
	}
	writers.Wait()
	close(stop)
	relays.Wait()
	sequenceAll(t, pool)

	published, inLog := nats.published(), logged(t, pool)
	if n := int64(len(published)) - atSwitch.Load(); n != 0 {
		t.Fatalf("%d events went to NATS after the switch", n)
	}
	if len(published) < 20 || len(inLog) < 160 {
		t.Fatalf("%d events to NATS and %d to the log: the switch did not fall among the saves", len(published), len(inLog))
	}
	oneWay(t, committed, published, inLog)

	rec := newRecorder()
	var consumers []Consumer
	for _, name := range theConsumers {
		consumers = append(consumers, rec.consumer(name))
	}
	r := newRunner(pool, consumers...)
	drain(t, r, start(t, r))
	slices.SortFunc(inLog, uuidOrder)
	for _, name := range theConsumers {
		got := rec.of(name)
		slices.SortFunc(got, uuidOrder)
		if !slices.Equal(got, inLog) {
			t.Errorf("%s was handed %d events, want the %d of the log", name, len(got), len(inLog))
		}
	}
	mustPass(t, pool)
}

// A relay of a release from before the log (no events lock, SKIP LOCKED, publish and mark)
// still runs after the switch, as a node that comes back late does. What it publishes goes to
// NATS, whose consumers this release keeps running; the sequencer takes the rest; no event goes
// both ways, none is lost. A row the old relay holds while it publishes is waited for or left,
// and is never sequenced as well.
func TestAnOldRelayAfterTheSwitchLosesNothing(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	switchTo(t, pool, TransportPostgres)

	held := enqueue(t, pool, map[string]any{"page_id": "p"})
	claimed, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	oldNATS := &natsStream{held: func() {
		once.Do(func() { close(claimed) })
		<-release
	}}
	oldDone := make(chan error, 1)
	go func() {
		_, err := (&outbox.Relay{Pool: pool, Publisher: oldNATS}).RunOnce(ctx)
		oldDone <- err
	}()
	<-claimed
	sequenced := make(chan int, 1)
	go func() {
		n, err := (&outbox.Relay{Pool: pool, Sequencer: Sequencer{}}).RunOnce(ctx)
		if err != nil {
			t.Error(err)
		}
		sequenced <- n
	}()
	// The sequencer either waits for the row or leaves it; let it get that far first.
	waitUntil(t, "the sequencer reaches the held row", func() bool {
		if len(sequenced) > 0 {
			return true
		}
		var waiting int
		err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting)
		return err == nil && waiting > 0
	})
	close(release)
	if err := <-oldDone; err != nil {
		t.Fatal(err)
	}
	if n := <-sequenced; n != 0 {
		t.Fatalf("the sequencer moved %d events, the old relay held the only one", n)
	}
	if got := oldNATS.published(); !slices.Equal(got, []uuid.UUID{held}) || len(logged(t, pool)) != 0 {
		t.Fatalf("NATS %v, log %v", got, logged(t, pool))
	}

	// Both run side by side while people save.
	committed := []uuid.UUID{held}
	var mu sync.Mutex
	stop := make(chan struct{})
	var loops sync.WaitGroup
	for _, relay := range []*outbox.Relay{
		{Pool: pool, Publisher: oldNATS, BatchSize: 3},
		{Pool: pool, Sequencer: Sequencer{BatchSize: 3}},
	} {
		loops.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := relay.RunOnce(ctx); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	var writers sync.WaitGroup
	for w := range 4 {
		writers.Go(func() {
			ids, err := writeEvents(ctx, pool, w, 50)
			mu.Lock()
			committed = append(committed, ids...)
			mu.Unlock()
			if err != nil {
				t.Error(err)
			}
		})
	}
	writers.Wait()
	close(stop)
	loops.Wait()
	sequenceAll(t, pool)
	published, inLog := oldNATS.published(), logged(t, pool)
	oneWay(t, committed, published, inLog)
	if len(published) < 2 || len(inLog) == 0 {
		t.Fatalf("NATS %d, log %d: the two relays did not share the events", len(published), len(inLog))
	}
	mustPass(t, pool)
}

// holdUntil waits until done is closed or the operator's switch waits for a lock, for at most
// 30 seconds, and reports whether either happened. It runs inside a relay's batch, where the
// test must not stop.
func holdUntil(ctx context.Context, pool *pgxpool.Pool, done <-chan struct{}) bool {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			return true
		default:
		}
		var waiting int
		err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND application_name = 'events-switch' AND wait_event_type = 'Lock'`).Scan(&waiting)
		if err == nil && waiting > 0 {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// waitUntil polls a condition with a deadline far beyond any machine's pace, so a slow run
// waits longer instead of failing.
func waitUntil(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("waited 30 s: %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The switch waits while any node seen in the last five minutes runs a release from before
// the log, since such a relay would not take the events lock. Nodes of release A, part 1
// (migration 53) relay under the lock and may run beside it.
func TestTheSwitchWaitsForEveryNode(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	addNode(t, pool, 52, 4*time.Minute)
	addNode(t, pool, 53, 0)
	addNode(t, pool, 59, 0)
	changed, err := Switch(ctx, pool, TransportPostgres, theConsumers)
	var behind *NodesBehindError
	if !errors.As(err, &behind) || changed {
		t.Fatalf("switched with an older node running: changed %v, err %v", changed, err)
	}
	if len(behind.Nodes) != 1 || behind.Nodes[0].MaxMigration != 52 || !strings.Contains(err.Error(), "release-52") {
		t.Fatalf("names %+v: %v", behind.Nodes, err)
	}
	if s := readState(t, pool); s.Transport != TransportNATS || len(s.OlderNodes) != 1 {
		t.Fatalf("state %+v", s)
	}
	var cursors int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM event_cursors").Scan(&cursors); err != nil || cursors != 0 {
		t.Fatalf("a refused switch left %d cursors, err %v", cursors, err)
	}

	// Not seen for over five minutes: gone.
	exec(t, pool, "UPDATE nodes SET last_seen_at = now() - interval '5 minutes 1 second' WHERE max_migration = 52")
	if !switchTo(t, pool, TransportPostgres) {
		t.Fatal("the switch changed nothing")
	}
	if s := readState(t, pool); s.Transport != TransportPostgres || len(s.OlderNodes) != 0 {
		t.Fatalf("state %+v", s)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM event_cursors WHERE pos = 0").Scan(&cursors); err != nil || cursors != len(theConsumers)*Lanes {
		t.Fatalf("%d cursors at the head, err %v", cursors, err)
	}
}

// The cursors start at the head as the switch committed it, not when a worker first looks:
// events sequenced in between are handed over. Switching back keeps the cursors.
func TestTheCursorsStartAtTheHeadOfTheSwitch(t *testing.T) {
	pool := testutil.Database(t).Pool
	// Earlier events, as a measurement or an earlier switch left them.
	usePostgres(t, pool)
	enqueue(t, pool, map[string]any{"page_id": "before"})
	sequenceAll(t, pool)
	exec(t, pool, "UPDATE event_transport SET transport = 'nats'")
	switchTo(t, pool, TransportPostgres)
	after := enqueue(t, pool, map[string]any{"page_id": "after"})
	sequenceAll(t, pool)
	rec := newRecorder()
	r := newRunner(pool, rec.consumer("collab"))
	drain(t, r, start(t, r))
	if got := rec.of("collab"); !slices.Equal(got, []uuid.UUID{after}) {
		t.Fatalf("handed %v, want only the event after the switch", got)
	}
}

// An operator undoes the switch: new events go to NATS again, the consumers of the log still
// hand over what it holds, and a switch again later starts where the cursors stand.
func TestSwitchingBackToNATSLosesNothing(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	needsConsumers(t, pool, false)
	switchTo(t, pool, TransportPostgres)
	// Needed at once, before the first event, so the listening connection runs from the start.
	needsConsumers(t, pool, true)
	var inLog []uuid.UUID
	for range 3 {
		inLog = append(inLog, enqueue(t, pool, map[string]any{"page_id": "p"}))
	}
	sequenceAll(t, pool)
	if !switchTo(t, pool, TransportNATS) {
		t.Fatal("switching back changed nothing")
	}
	nats := &natsStream{}
	toNATS := enqueue(t, pool, map[string]any{"page_id": "p"})
	if n, err := (&outbox.Relay{Pool: pool, Publisher: nats, Sequencer: Sequencer{}}).RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("relayed %d, err %v", n, err)
	}
	if got := nats.published(); !slices.Equal(got, []uuid.UUID{toNATS}) {
		t.Fatalf("NATS got %v", got)
	}
	needsConsumers(t, pool, true)
	rec := newRecorder()
	var consumers []Consumer
	for _, name := range theConsumers {
		consumers = append(consumers, rec.consumer(name))
	}
	r := newRunner(pool, consumers...)
	lanes := start(t, r)
	drain(t, r, lanes)
	if got := rec.of("notifications"); !slices.Equal(got, inLog) {
		t.Fatalf("handed %v, want %v", got, inLog)
	}
	needsConsumers(t, pool, false)

	if !switchTo(t, pool, TransportPostgres) {
		t.Fatal("the second switch changed nothing")
	}
	again := enqueue(t, pool, map[string]any{"page_id": "p"})
	sequenceAll(t, pool)
	drain(t, r, lanes)
	if got := rec.of("notifications"); !slices.Equal(got, append(slices.Clone(inLog), again)) {
		t.Fatalf("after the second switch handed %v", got)
	}
	mustPass(t, pool)
}

func needsConsumers(t *testing.T, pool *pgxpool.Pool, want bool) {
	t.Helper()
	got, err := ConsumersNeeded(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("the consumers are needed: %v, want %v", got, want)
	}
}

// A retry waiting in the log keeps the consumers running after a switch back, too.
func TestARetryKeepsTheConsumersRunningUnderNATS(t *testing.T) {
	pool := logOf(t, 1)
	exec(t, pool, "UPDATE event_cursors SET pos = 1")
	exec(t, pool, "UPDATE event_transport SET transport = 'nats'")
	needsConsumers(t, pool, false)
	exec(t, pool, `INSERT INTO event_retries (consumer, pos, event_id, tries, next_at, last_error)
		SELECT 'notifications', pos, id, 1, now(), 'down' FROM event_log`)
	needsConsumers(t, pool, true)
}

func waiting(n int, err error) Waiting {
	return func(context.Context) (int, error) { return n, err }
}

func noteDrained(t *testing.T, pool *pgxpool.Pool, w Waiting) bool {
	t.Helper()
	drained, err := NoteDrained(context.Background(), pool, w)
	if err != nil {
		t.Fatal(err)
	}
	if s := readState(t, pool); s.DrainedAt.IsZero() == drained {
		t.Fatalf("said drained %v, noted %v", drained, s.DrainedAt)
	}
	return drained
}

// NATS counts as drained only after the switch, with nothing pending or waiting for an ack in
// any consumer, and no node of an older release that could still publish to it.
func TestDrainedIsSetOnlyWhenNothingWaits(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	notAsked := func(why string) Waiting {
		return func(context.Context) (int, error) { t.Errorf("NATS was asked %s", why); return 0, nil }
	}
	if noteDrained(t, pool, notAsked("while it carries the events")) {
		t.Fatal("drained while the events still go through NATS")
	}
	switchTo(t, pool, TransportPostgres)
	if noteDrained(t, pool, waiting(1, nil)) {
		t.Fatal("drained while a retry waits in NATS")
	}
	if drained, err := NoteDrained(ctx, pool, waiting(0, errors.New("NATS does not answer"))); err == nil || drained {
		t.Fatalf("drained %v without an answer from NATS, err %v", drained, err)
	}
	addNode(t, pool, 52, time.Minute)
	if noteDrained(t, pool, notAsked("while an older node may publish to it")) {
		t.Fatal("drained while an older node may publish")
	}
	exec(t, pool, "DELETE FROM nodes")
	// An older node that starts while NATS is asked.
	starts := func(context.Context) (int, error) {
		addNode(t, pool, 52, 0)
		return 0, nil
	}
	if noteDrained(t, pool, starts) {
		t.Fatal("drained while an older node started")
	}
	exec(t, pool, "DELETE FROM nodes")
	if !noteDrained(t, pool, waiting(0, nil)) {
		t.Fatal("not drained with nothing waiting")
	}
	// Once noted, NATS is not asked again.
	if !noteDrained(t, pool, notAsked("again")) {
		t.Fatal("the drain was forgotten")
	}

	// An older node that comes back may publish to NATS again: the drain starts anew.
	addNode(t, pool, 52, 0)
	if noteDrained(t, pool, waiting(0, nil)) {
		t.Fatal("still drained while an older node runs")
	}
	exec(t, pool, "DELETE FROM nodes")
	if !noteDrained(t, pool, waiting(0, nil)) {
		t.Fatal("not drained again")
	}

	// Switching back undoes the drain; switching again starts a new one.
	switchTo(t, pool, TransportNATS)
	if s := readState(t, pool); !s.DrainedAt.IsZero() {
		t.Fatalf("drained at %v after switching back", s.DrainedAt)
	}
	switchTo(t, pool, TransportPostgres)
	if s := readState(t, pool); !s.DrainedAt.IsZero() {
		t.Fatalf("drained at %v right after the switch", s.DrainedAt)
	}
	// A switch back and forth while NATS is asked: the answer is about the transport before.
	flip := func(context.Context) (int, error) {
		switchTo(t, pool, TransportNATS)
		switchTo(t, pool, TransportPostgres)
		return 0, nil
	}
	if noteDrained(t, pool, flip) {
		t.Fatal("drained on an answer from before the last switch")
	}
	if !noteDrained(t, pool, waiting(0, nil)) {
		t.Fatal("not drained after the flip")
	}
}

// Switching to postgres again starts a new drain: the way out when a node of an older release
// published to NATS after it was drained.
func TestSwitchingAgainStartsANewDrain(t *testing.T) {
	pool := testutil.Database(t).Pool
	switchTo(t, pool, TransportPostgres)
	noteDrained(t, pool, waiting(0, nil))
	if switchTo(t, pool, TransportPostgres) {
		t.Fatal("a switch to the transport in use changed it")
	}
	if s := readState(t, pool); !s.DrainedAt.IsZero() || s.Transport != TransportPostgres {
		t.Fatalf("state %+v", s)
	}
}

// A transport that does not exist is refused before anything changes.
func TestTheSwitchRefusesAnUnknownTransport(t *testing.T) {
	pool := testutil.Database(t).Pool
	if _, err := Switch(context.Background(), pool, Transport("kafka"), theConsumers); err == nil {
		t.Fatal("switched to kafka")
	}
	if s := readState(t, pool); s.Transport != TransportNATS {
		t.Fatalf("transport %s", s.Transport)
	}
}

// The check of the release without NATS (release B), against databases as each earlier
// release leaves them: it refuses, naming what to do, until the events run through
// PostgreSQL and NATS is drained.
func TestTheReleaseWithoutNATSRefusesUntilNATSIsDrained(t *testing.T) {
	const drained = "UPDATE event_transport SET transport = 'postgres', nats_drained_at = now() - interval '1 hour'"
	for _, f := range []struct {
		name  string
		setup []string
		want  string // in the problem; empty = may upgrade
	}{
		{"a release before the log", []string{"DROP TABLE event_transport"}, "upgrade to the release that switches"},
		{"release A, part 1", []string{"ALTER TABLE event_transport DROP COLUMN nats_drained_at"}, "upgrade to the release that switches"},
		{"still on NATS", nil, "diffluence events switch postgres"},
		{"switched, not yet drained", []string{"UPDATE event_transport SET transport = 'postgres'"}, "wait until"},
		{"an older node runs", []string{drained, `INSERT INTO nodes (id, role, version, max_migration) VALUES (gen_random_uuid(), 'worker', 'old', 52)`}, "older release"},
		{"published to NATS after the drain", []string{drained,
			`INSERT INTO outbox_events (id, type, payload, published_at) VALUES (gen_random_uuid(), 'page.updated', '{}', now())`}, "switch postgres again"},
		{"published to NATS before the drain", []string{drained,
			`INSERT INTO outbox_events (id, type, payload, published_at) VALUES (gen_random_uuid(), 'page.updated', '{}', now() - interval '2 hours')`}, ""},
		{"drained", []string{drained}, ""},
	} {
		t.Run(f.name, func(t *testing.T) {
			pool := testutil.Database(t).Pool
			for _, sql := range f.setup {
				exec(t, pool, sql)
			}
			problem, err := NATSRemovalProblem(context.Background(), pool)
			if err != nil {
				t.Fatal(err)
			}
			if f.want == "" && problem != "" || !strings.Contains(problem, f.want) {
				t.Fatalf("problem %q, want %q", problem, f.want)
			}
		})
	}
}
