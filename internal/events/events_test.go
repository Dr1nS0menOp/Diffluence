// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/diffluence/diffluence/internal/failures"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/testutil"
	"github.com/diffluence/diffluence/internal/tracing"
)

// recorder keeps what each consumer was handed, in order, and fails the events it is told to.
type recorder struct {
	mu   sync.Mutex
	seen map[string][]uuid.UUID
	// fails says whether an event fails, for a consumer; nil = none does.
	fails func(consumer string, id uuid.UUID) error
}

func newRecorder() *recorder { return &recorder{seen: map[string][]uuid.UUID{}} }

func (r *recorder) consumer(name string) Consumer {
	return Consumer{Name: name, Handle: func(_ context.Context, data []byte) error {
		var m outbox.Message
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		r.seen[name] = append(r.seen[name], m.ID)
		if r.fails != nil {
			return r.fails(name, m.ID)
		}
		return nil
	}}
}

func (r *recorder) of(name string) []uuid.UUID {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.seen[name])
}

func usePostgres(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "UPDATE event_transport SET transport = 'postgres'"); err != nil {
		t.Fatal(err)
	}
}

func enqueueIn(t *testing.T, tx pgx.Tx, payload map[string]any) uuid.UUID {
	t.Helper()
	id, err := outbox.Enqueue(context.Background(), tx, "page.updated", payload)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func enqueue(t *testing.T, pool *pgxpool.Pool, payload map[string]any) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pgx.BeginFunc(context.Background(), pool, func(tx pgx.Tx) error {
		id = enqueueIn(t, tx, payload)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// sequenceAll runs the relay with the sequencer until nothing waits.
func sequenceAll(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	relay := &outbox.Relay{Pool: pool, Sequencer: Sequencer{}}
	total := 0
	for {
		n, err := relay.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			return total
		}
		total += n
	}
}

func newRunner(pool *pgxpool.Pool, consumers ...Consumer) *Runner {
	return &Runner{Pool: pool, Node: uuid.New(), Consumers: consumers, GiveUp: failures.RecordIn,
		Log: slog.New(slog.DiscardHandler)}
}

func start(t *testing.T, r *Runner) []*lane {
	t.Helper()
	if err := r.EnsureCursors(context.Background()); err != nil {
		t.Fatal(err)
	}
	return r.lanes()
}

// drain steps every lane until a whole round hands nothing over. Retries are not due yet.
// A lane that never settles fails the test rather than running for ever.
func drain(t *testing.T, r *Runner, lanes []*lane) int {
	t.Helper()
	total := 0
	for rounds := 0; ; rounds++ {
		if rounds > 200 {
			t.Fatalf("the lanes never settle: %d handed over", total)
		}
		handed := 0
		for _, l := range lanes {
			n, err := r.step(context.Background(), l)
			if err != nil {
				t.Fatal(err)
			}
			handed += n
		}
		if handed == 0 {
			return total
		}
		total += handed
	}
}

func posOf(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) int64 {
	t.Helper()
	var pos int64
	if err := pool.QueryRow(context.Background(), "SELECT pos FROM event_log WHERE id = $1", id).Scan(&pos); err != nil {
		t.Fatalf("event %s has no position: %v", id, err)
	}
	return pos
}

func mustPass(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	breach, err := Check(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if breach != nil {
		t.Fatalf("the ledger check failed: %s", breach)
	}
}

// A transaction that started first and committed last gets the later position, so a
// consumer that already passed the first position still sees it.
func TestALateCommitIsNotSkipped(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	usePostgres(t, pool)
	rec := newRecorder()
	r := newRunner(pool, rec.consumer("notifications"))
	lanes := start(t, r)
	page := uuid.NewString()

	late, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = late.Rollback(ctx) }()
	lateID := enqueueIn(t, late, map[string]any{"page_id": page})
	earlyID := enqueue(t, pool, map[string]any{"page_id": page})
	sequenceAll(t, pool)
	drain(t, r, lanes)
	if err := late.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	sequenceAll(t, pool)
	drain(t, r, lanes)

	if posOf(t, pool, lateID) <= posOf(t, pool, earlyID) {
		t.Fatal("the late commit got the earlier position")
	}
	if got := rec.of("notifications"); !slices.Equal(got, []uuid.UUID{earlyID, lateID}) {
		t.Fatalf("handed %v, want the early then the late event", got)
	}
	mustPass(t, pool)
}

func TestPositionsHaveNoGaps(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	usePostgres(t, pool)
	const writers, each = 8, 40
	var wg sync.WaitGroup
	var mu sync.Mutex
	var committed []uuid.UUID
	for w := range writers {
		wg.Go(func() {
			for i := range each {
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Error(err)
					return
				}
				id, err := outbox.Enqueue(ctx, tx, "page.updated", map[string]any{"page_id": fmt.Sprint(w, "-", i)})
				if err != nil {
					t.Error(err)
					return
				}
				// Every third transaction rolls back: its event must leave no hole.
				if i%3 == 0 {
					_ = tx.Rollback(ctx)
					continue
				}
				if err := tx.Commit(ctx); err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				committed = append(committed, id)
				mu.Unlock()
			}
		})
	}
	stop := make(chan struct{})
	var sequencers sync.WaitGroup
	for range 3 {
		sequencers.Go(func() {
			relay := &outbox.Relay{Pool: pool, Sequencer: Sequencer{BatchSize: 7}}
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
	wg.Wait()
	close(stop)
	sequencers.Wait()
	sequenceAll(t, pool)

	var head, count, lowest, highest int64
	err := pool.QueryRow(ctx, `SELECT (SELECT pos FROM event_head), count(*), min(pos), max(pos) FROM event_log`).
		Scan(&head, &count, &lowest, &highest)
	if err != nil {
		t.Fatal(err)
	}
	if count != int64(len(committed)) || lowest != 1 || highest != count || head != count {
		t.Fatalf("head %d, %d rows from %d to %d, %d committed", head, count, lowest, highest, len(committed))
	}
	for _, id := range committed {
		posOf(t, pool, id)
	}
	mustPass(t, pool)
}

// Two sequencers on two nodes: the second waits for the first, so positions cannot cross.
func TestOnlyOneSequencerRunsAtATime(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	usePostgres(t, pool)
	enqueue(t, pool, map[string]any{"page_id": "p"})
	holder, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// A failure below must still give the connection back, or closing the pool waits for ever.
	defer func() { _ = holder.Rollback(ctx) }()
	if _, err := holder.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() {
		relay := &outbox.Relay{Pool: pool, Sequencer: Sequencer{}}
		n, err := relay.RunOnce(ctx)
		if err != nil {
			t.Error(err)
		}
		done <- n
	}()
	select {
	case n := <-done:
		t.Fatalf("a second sequencer moved %d events while the first held the lock", n)
	case <-time.After(500 * time.Millisecond):
	}
	if err := holder.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case n := <-done:
		if n != 1 {
			t.Fatalf("moved %d events once the lock was free", n)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the second sequencer never ran")
	}
}

// While the transport is nats the relay publishes as before and the log stays empty, so
// this release changes nothing until the switch.
func TestUnderNATSTheRelayPublishesAndTheLogStaysEmpty(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	enqueue(t, pool, map[string]any{"page_id": "p"})
	publisher := &countingPublisher{}
	relay := &outbox.Relay{Pool: pool, Publisher: publisher, Sequencer: Sequencer{}}
	if n, err := relay.RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("relayed %d, err %v", n, err)
	}
	var logged, waiting int
	err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM event_log),
		(SELECT count(*) FROM outbox_events WHERE published_at IS NULL)`).Scan(&logged, &waiting)
	if err != nil {
		t.Fatal(err)
	}
	if publisher.published != 1 || logged != 0 || waiting != 0 {
		t.Fatalf("published %d, logged %d, waiting %d", publisher.published, logged, waiting)
	}
}

type countingPublisher struct{ published int }

func (p *countingPublisher) Publish(context.Context, string, []byte, string) error {
	p.published++
	return nil
}

// An event the relay already published to NATS stays out of the log after the switch, so
// it goes one way only.
func TestAPublishedEventIsNotSequencedAgain(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	enqueue(t, pool, map[string]any{"page_id": "p"})
	relay := &outbox.Relay{Pool: pool, Publisher: &countingPublisher{}, Sequencer: Sequencer{}}
	if n, err := relay.RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("published %d, err %v", n, err)
	}
	usePostgres(t, pool)
	if n := sequenceAll(t, pool); n != 0 {
		t.Fatalf("sequenced %d published events", n)
	}
}

// The sequencer moves each event once: what it put in the log leaves the outbox.
func TestASequencedEventLeavesTheOutbox(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	usePostgres(t, pool)
	enqueue(t, pool, map[string]any{"page_id": "p"})
	if n := sequenceAll(t, pool); n != 1 {
		t.Fatalf("moved %d", n)
	}
	var left int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events").Scan(&left); err != nil || left != 0 {
		t.Fatalf("%d rows left in the outbox, err %v", left, err)
	}
}

func TestEveryEventReachesEveryConsumer(t *testing.T) {
	pool := testutil.Database(t).Pool
	usePostgres(t, pool)
	rec := newRecorder()
	names := []string{"notifications", "automation", "webhooks", "collab"}
	var consumers []Consumer
	for _, n := range names {
		consumers = append(consumers, rec.consumer(n))
	}
	r := newRunner(pool, consumers...)
	lanes := start(t, r)
	var want []uuid.UUID
	keys := []string{"page_id", "space_id", "user_id", "group_id", "other"}
	for i := range 150 {
		want = append(want, enqueue(t, pool, map[string]any{keys[i%len(keys)]: uuid.NewString()}))
	}
	sequenceAll(t, pool)
	drain(t, r, lanes)
	slices.SortFunc(want, uuidOrder)
	for _, n := range names {
		got := rec.of(n)
		slices.SortFunc(got, uuidOrder)
		if !slices.Equal(got, want) {
			t.Errorf("%s was handed %d events, want each of %d once", n, len(got), len(want))
		}
	}
	var lanesUsed int
	if err := pool.QueryRow(context.Background(), "SELECT count(DISTINCT lane) FROM event_log").Scan(&lanesUsed); err != nil || lanesUsed != Lanes {
		t.Fatalf("events spread over %d lanes, err %v", lanesUsed, err)
	}
	mustPass(t, pool)
}

func uuidOrder(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) }

func TestAFailedEventDoesNotHoldItsLane(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	usePostgres(t, pool)
	rec := newRecorder()
	r := newRunner(pool, rec.consumer("webhooks"))
	lanes := start(t, r)
	page := uuid.NewString()
	bad := enqueue(t, pool, map[string]any{"page_id": page})
	good := enqueue(t, pool, map[string]any{"page_id": page})
	rec.fails = func(_ string, id uuid.UUID) error {
		if id == bad {
			return errors.New("the endpoint is down")
		}
		return nil
	}
	sequenceAll(t, pool)
	before := dbNow(t, pool)
	drain(t, r, lanes)
	after := dbNow(t, pool)

	if got := rec.of("webhooks"); !slices.Equal(got, []uuid.UUID{bad, good}) {
		t.Fatalf("handed %v", got)
	}
	var tries int
	var next time.Time
	var lastError string
	err := pool.QueryRow(ctx, `SELECT tries, next_at, last_error
		FROM event_retries WHERE consumer = 'webhooks' AND event_id = $1`, bad).Scan(&tries, &next, &lastError)
	if err != nil {
		t.Fatalf("the failed event waits nowhere: %v", err)
	}
	if tries != 1 || !waitsFor(next, retryWaits[0], before, after) || lastError != "the endpoint is down" {
		t.Fatalf("tries %d, next at %v, error %q", tries, next, lastError)
	}
	var cursor int64
	if err := pool.QueryRow(ctx, "SELECT pos FROM event_cursors WHERE consumer = 'webhooks' AND lane = (SELECT lane FROM event_log WHERE id = $1)", good).
		Scan(&cursor); err != nil || cursor != posOf(t, pool, good) {
		t.Fatalf("cursor %d, err %v", cursor, err)
	}
	mustPass(t, pool)
}

// dbNow is the database's clock, which sets every wait.
func dbNow(t *testing.T, pool *pgxpool.Pool) time.Time {
	t.Helper()
	var now time.Time
	if err := pool.QueryRow(context.Background(), "SELECT now()").Scan(&now); err != nil {
		t.Fatal(err)
	}
	return now
}

// waitsFor says whether next is wait after a moment between before and after, read from the
// database around the step that set it. Bounds taken this way hold on a machine of any pace.
func waitsFor(next time.Time, wait time.Duration, before, after time.Time) bool {
	return !next.Before(before.Add(wait)) && !next.After(after.Add(wait))
}

// makeRetriesDue moves every waiting retry to now.
func makeRetriesDue(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "UPDATE event_retries SET next_at = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
}

func TestARetryEndsInFailures(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	usePostgres(t, pool)
	rec := newRecorder()
	rec.fails = func(string, uuid.UUID) error { return errors.New("poison") }
	consumer := rec.consumer("automation")
	r := newRunner(pool, consumer)
	lanes := start(t, r)
	poison := enqueue(t, pool, map[string]any{"page_id": "p"})
	sequenceAll(t, pool)
	drain(t, r, lanes)

	// Each retry waits longer: 10 s, then 1, 5, 15 and 60 minutes, as NATS waited.
	for i, wait := range retryWaits[1:] {
		makeRetriesDue(t, pool)
		before := dbNow(t, pool)
		if n, err := r.retryStep(ctx, consumer); err != nil || n != 1 {
			t.Fatalf("retry %d: ran %d, err %v", i+1, n, err)
		}
		after := dbNow(t, pool)
		var tries int
		var next time.Time
		err := pool.QueryRow(ctx, `SELECT tries, next_at FROM event_retries WHERE consumer = 'automation'`).Scan(&tries, &next)
		if err != nil {
			t.Fatalf("retry %d: %v", i+1, err)
		}
		if tries != i+2 || !waitsFor(next, wait, before, after) {
			t.Fatalf("retry %d: tries %d, next at %v, want %v after a moment between %v and %v", i+1, tries, next, wait, before, after)
		}
		if failed := countFailures(t, pool); failed != 0 {
			t.Fatalf("gave up after retry %d", i+1)
		}
	}
	makeRetriesDue(t, pool)
	if n, err := r.retryStep(ctx, consumer); err != nil || n != 1 {
		t.Fatalf("last try: ran %d, err %v", n, err)
	}
	if got := len(rec.of("automation")); got != 1+len(retryWaits) {
		t.Fatalf("the event ran %d times, want %d", got, 1+len(retryWaits))
	}
	var consumerName, cause string
	var payload []byte
	err := pool.QueryRow(ctx, "SELECT consumer, error, payload FROM event_failures").Scan(&consumerName, &cause, &payload)
	if err != nil {
		t.Fatalf("the poison event is not in the failures: %v", err)
	}
	var m outbox.Message
	if err := json.Unmarshal(payload, &m); err != nil || m.ID != poison || consumerName != "automation" || cause != "poison" {
		t.Fatalf("failure %s %q %s, err %v", consumerName, cause, payload, err)
	}
	var waiting int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM event_retries").Scan(&waiting); err != nil || waiting != 0 {
		t.Fatalf("%d retries left, err %v", waiting, err)
	}
	makeRetriesDue(t, pool)
	if n, err := r.retryStep(ctx, consumer); err != nil || n != 0 {
		t.Fatalf("a failure ran again: %d, err %v", n, err)
	}
}

func countFailures(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM event_failures").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A retry that succeeds leaves the table and is never handed over again.
func TestARetryThatSucceedsIsDone(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	usePostgres(t, pool)
	rec := newRecorder()
	failing := true
	rec.fails = func(string, uuid.UUID) error {
		if failing {
			return errors.New("not yet")
		}
		return nil
	}
	consumer := rec.consumer("collab")
	r := newRunner(pool, consumer)
	lanes := start(t, r)
	enqueue(t, pool, map[string]any{"page_id": "p"})
	sequenceAll(t, pool)
	drain(t, r, lanes)
	// Not due yet: nothing runs.
	if n, err := r.retryStep(ctx, consumer); err != nil || n != 0 {
		t.Fatalf("a retry ran before its time: %d, err %v", n, err)
	}
	failing = false
	makeRetriesDue(t, pool)
	if n, err := r.retryStep(ctx, consumer); err != nil || n != 1 {
		t.Fatalf("ran %d, err %v", n, err)
	}
	makeRetriesDue(t, pool)
	if n, err := r.retryStep(ctx, consumer); err != nil || n != 0 {
		t.Fatalf("ran again: %d, err %v", n, err)
	}
	if got := len(rec.of("collab")); got != 2 || countFailures(t, pool) != 0 {
		t.Fatalf("ran %d times", got)
	}
}

// Each consumer retries only its own events.
func TestARetryRunsOnlyForItsConsumer(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	usePostgres(t, pool)
	rec := newRecorder()
	rec.fails = func(name string, _ uuid.UUID) error {
		if name == "webhooks" {
			return errors.New("down")
		}
		return nil
	}
	webhooks, automation := rec.consumer("webhooks"), rec.consumer("automation")
	r := newRunner(pool, webhooks, automation)
	lanes := start(t, r)
	enqueue(t, pool, map[string]any{"page_id": "p"})
	sequenceAll(t, pool)
	drain(t, r, lanes)
	makeRetriesDue(t, pool)
	if n, err := r.retryStep(ctx, automation); err != nil || n != 0 {
		t.Fatalf("automation ran the retry of webhooks: %d, err %v", n, err)
	}
	if n, err := r.retryStep(ctx, webhooks); err != nil || n != 1 {
		t.Fatalf("webhooks ran %d, err %v", n, err)
	}
}

// A worker that crashed holding a lease: after the lease ends, another worker takes the lane
// and hands over what waits there.
func TestAnEndedLeaseIsTakenOver(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	usePostgres(t, pool)
	recA, recB := newRecorder(), newRecorder()
	a := newRunner(pool, recA.consumer("notifications"))
	b := newRunner(pool, recB.consumer("notifications"))
	lanesA, lanesB := start(t, a), start(t, b)
	drain(t, a, lanesA) // a holds every lane, then dies
	id := enqueue(t, pool, map[string]any{"page_id": "p"})
	sequenceAll(t, pool)
	if n := drain(t, b, lanesB); n != 0 {
		t.Fatalf("b took %d events from lanes a still holds", n)
	}
	if _, err := pool.Exec(ctx, "UPDATE event_cursors SET lease_until = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if n := drain(t, b, lanesB); n != 1 {
		t.Fatalf("b took %d events after the lease ended", n)
	}
	if got := recB.of("notifications"); !slices.Equal(got, []uuid.UUID{id}) || len(recA.of("notifications")) != 0 {
		t.Fatalf("b %v, a %v", got, recA.of("notifications"))
	}
	var owners int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM event_cursors WHERE lease_owner = $1", b.Node).Scan(&owners); err != nil || owners != Lanes {
		t.Fatalf("b holds %d lanes, err %v", owners, err)
	}
}

// A lease that is still running keeps the lane, also when its holder has not renewed it.
func TestALiveLeaseIsNotTaken(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	usePostgres(t, pool)
	a := newRunner(pool, newRecorder().consumer("webhooks"))
	b := newRunner(pool, newRecorder().consumer("webhooks"))
	lanesA, lanesB := start(t, a), start(t, b)
	drain(t, a, lanesA)
	// Live for far longer than the steps below take on a slow machine.
	if _, err := pool.Exec(ctx, "UPDATE event_cursors SET lease_until = now() + interval '1 minute'"); err != nil {
		t.Fatal(err)
	}
	drain(t, b, lanesB)
	var heldByB int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM event_cursors WHERE lease_owner = $1", b.Node).Scan(&heldByB); err != nil || heldByB != 0 {
		t.Fatalf("b took %d live leases, err %v", heldByB, err)
	}
}

// The lease is lost while a batch runs: the cursor does not move, and the new holder runs the
// batch again. A cursor commit lost in a crash moves the cursor back: the events run again.
// Either way events repeat and are never skipped.
func TestALostCursorCommitRepeatsAndNeverSkips(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	usePostgres(t, pool)
	recA, recB := newRecorder(), newRecorder()
	a := newRunner(pool, recA.consumer("collab"))
	b := newRunner(pool, recB.consumer("collab"))
	lanesA, lanesB := start(t, a), start(t, b)
	drain(t, a, lanesA)
	page := uuid.NewString()
	var want []uuid.UUID
	for range 3 {
		want = append(want, enqueue(t, pool, map[string]any{"page_id": page}))
	}
	sequenceAll(t, pool)
	lane := laneOf(t, pool, want[0], lanesA)
	stolen := false
	recA.fails = func(_ string, id uuid.UUID) error {
		if !stolen {
			// b takes the lane while a's handler runs, as after a stall of over a minute.
			stolen = true
			if _, err := pool.Exec(ctx, "UPDATE event_cursors SET lease_owner = $1, lease_until = now() + interval '1 minute' WHERE consumer = 'collab' AND lane = $2",
				b.Node, lane.number); err != nil {
				return err
			}
		}
		if id == want[2] {
			return errors.New("down")
		}
		return nil
	}
	if _, err := a.step(ctx, lane); !errors.Is(err, errLeaseLost) {
		t.Fatalf("a's commit went through without its lease: %v", err)
	}
	var cursor int64
	if err := pool.QueryRow(ctx, "SELECT pos FROM event_cursors WHERE consumer = 'collab' AND lane = $1", lane.number).Scan(&cursor); err != nil || cursor != 0 {
		t.Fatalf("the cursor moved to %d without a lease, err %v", cursor, err)
	}
	var retries int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM event_retries").Scan(&retries); err != nil || retries != 0 {
		t.Fatalf("%d retries kept from a batch that did not count, err %v", retries, err)
	}
	drain(t, b, lanesB)
	if got := recB.of("collab"); !slices.Equal(got, want) {
		t.Fatalf("b ran %v, want %v", got, want)
	}

	// The asynchronous commit of b's cursor is lost in a crash: the cursor is back at 1.
	if _, err := pool.Exec(ctx, "UPDATE event_cursors SET pos = $1, lease_owner = NULL, lease_until = NULL WHERE consumer = 'collab' AND lane = $2",
		posOf(t, pool, want[0]), lane.number); err != nil {
		t.Fatal(err)
	}
	recC := newRecorder()
	c := newRunner(pool, recC.consumer("collab"))
	drain(t, c, start(t, c))
	if got := recC.of("collab"); !slices.Equal(got, want[1:]) {
		t.Fatalf("after the lost commit c ran %v, want %v", got, want[1:])
	}
	mustPass(t, pool)
}

func laneOf(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, lanes []*lane) *lane {
	t.Helper()
	var number int16
	if err := pool.QueryRow(context.Background(), "SELECT lane FROM event_log WHERE id = $1", id).Scan(&number); err != nil {
		t.Fatal(err)
	}
	for _, l := range lanes {
		if l.number == number {
			return l
		}
	}
	t.Fatalf("no lane %d", number)
	return nil
}

// A worker that stops gives its leases back, so another takes the lanes at once.
func TestAStoppedWorkerGivesItsLeasesBack(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	usePostgres(t, pool)
	a := newRunner(pool, newRecorder().consumer("webhooks"))
	lanesA := start(t, a)
	drain(t, a, lanesA)
	a.releaseAll(lanesA)
	var held int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM event_cursors WHERE lease_owner IS NOT NULL").Scan(&held); err != nil || held != 0 {
		t.Fatalf("%d leases kept after stopping, err %v", held, err)
	}
}

// A lost notification costs latency, never an event: the loops poll without one.
func TestPollingDeliversWithoutNotify(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	usePostgres(t, pool)
	rec := newRecorder()
	r := newRunner(pool, rec.consumer("notifications"))
	if err := r.EnsureCursors(ctx); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); r.Run(ctx) }()
	id := enqueue(t, pool, map[string]any{"page_id": "p"})
	sequenceAll(t, pool)
	waitUntil(t, "an event arrives without a notification", func() bool { return len(rec.of("notifications")) > 0 })
	if got := rec.of("notifications"); !slices.Equal(got, []uuid.UUID{id}) {
		t.Fatalf("handed %v", got)
	}
	cancel()
	<-done
}

// A notification wakes the loops at once, long before their next poll.
func TestANotificationWakesTheConsumers(t *testing.T) {
	tdb := testutil.Database(t)
	pool := tdb.Pool
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	usePostgres(t, pool)
	rec := newRecorder()
	r := newRunner(pool, rec.consumer("notifications"))
	r.Poll = time.Hour
	r.Wake = NewWakeups()
	// The listener wakes every loop once LISTEN runs, as after a reconnect.
	listening := r.Wake.Subscribe()
	go Listen(ctx, tdb.URL, r.Wake, slog.New(slog.DiscardHandler))
	<-listening
	// The cursors exist before the event, or a slow start of Run would put them past it.
	if err := r.EnsureCursors(ctx); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); r.Run(ctx) }()
	// Run passes every lane once at its start, taking its lease; let that pass first, so only
	// a wake-up can hand the event over within the hour of the poll.
	waitUntil(t, "every lane passes once", func() bool {
		var held int
		err := pool.QueryRow(ctx, "SELECT count(*) FROM event_cursors WHERE lease_owner = $1", r.Node).Scan(&held)
		return err == nil && held == Lanes
	})
	enqueue(t, pool, map[string]any{"page_id": "p"})
	sequenceAll(t, pool)
	waitUntil(t, "the notification wakes a consumer", func() bool { return len(rec.of("notifications")) > 0 })
	cancel()
	<-done
}

// A save never notifies (D7): only the sequencer does, with the head position.
func TestASaveDoesNotNotify(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	usePostgres(t, tdb.Pool)
	conn, err := pgx.Connect(ctx, tdb.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		t.Fatal(err)
	}
	enqueue(t, tdb.Pool, map[string]any{"page_id": "p"})
	quiet, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	n, err := conn.WaitForNotification(quiet)
	cancel()
	if err == nil {
		t.Fatalf("a save notified: %+v", n)
	}
	sequenceAll(t, tdb.Pool)
	wait, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	n, err = conn.WaitForNotification(wait)
	if err != nil {
		t.Fatalf("the sequencer did not notify: %v", err)
	}
	if n.Channel != Channel || n.Payload != "1" {
		t.Fatalf("notified %q on %q", n.Payload, n.Channel)
	}
}

// LISTEN that does not work leaves polling: the loops still deliver.
func TestAListenerThatCannotConnectLeavesPolling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w := NewWakeups()
	wake := w.Subscribe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		Listen(ctx, "postgres://nobody@127.0.0.1:1/none?connect_timeout=1", w, slog.New(slog.DiscardHandler))
	}()
	<-done
	select {
	case <-wake:
		t.Fatal("a listener that never connected woke the loops")
	default:
	}
}

func TestEventsOfOnePageKeepTheirOrder(t *testing.T) {
	pool := testutil.Database(t).Pool
	usePostgres(t, pool)
	rec := newRecorder()
	a := newRunner(pool, rec.consumer("automation"))
	b := newRunner(pool, rec.consumer("automation"))
	lanesA, lanesB := start(t, a), start(t, b)
	page := uuid.NewString()
	var want []uuid.UUID
	pageEvents := map[uuid.UUID]bool{}
	for i := range 40 {
		id := enqueue(t, pool, map[string]any{"page_id": page})
		want = append(want, id)
		pageEvents[id] = true
		enqueue(t, pool, map[string]any{"page_id": fmt.Sprint("other-", i)})
		if i%7 == 0 {
			// The lanes move between two workers as they stop and start.
			sequenceAll(t, pool)
			drain(t, a, lanesA)
			a.releaseAll(lanesA)
			enqueue(t, pool, map[string]any{"page_id": fmt.Sprint("between-", i)})
			sequenceAll(t, pool)
			drain(t, b, lanesB)
			b.releaseAll(lanesB)
		}
	}
	sequenceAll(t, pool)
	drain(t, b, lanesB)
	drain(t, a, lanesA)
	if len(rec.of("automation")) != 40*2+6 {
		t.Fatalf("handed %d events", len(rec.of("automation")))
	}
	var got []uuid.UUID
	for _, id := range rec.of("automation") {
		if pageEvents[id] {
			got = append(got, id)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("the events of one page came in another order")
	}
}

// A worker that was down finds the backlog waiting and works through it in batches.
func TestABacklogAfterDowntimeIsDelivered(t *testing.T) {
	pool := testutil.Database(t).Pool
	usePostgres(t, pool)
	rec := newRecorder()
	r := newRunner(pool, rec.consumer("notifications"), rec.consumer("webhooks"))
	lanes := start(t, r)
	const backlog = 1200
	err := pgx.BeginFunc(context.Background(), pool, func(tx pgx.Tx) error {
		for i := range backlog {
			enqueueIn(t, tx, map[string]any{"page_id": fmt.Sprint(i % 50)})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := sequenceAll(t, pool); n != backlog {
		t.Fatalf("sequenced %d", n)
	}
	if n := drain(t, r, lanes); n != 2*backlog {
		t.Fatalf("handed over %d, want %d", n, 2*backlog)
	}
	for _, name := range []string{"notifications", "webhooks"} {
		if got := len(rec.of(name)); got != backlog {
			t.Fatalf("%s got %d", name, got)
		}
	}
	mustPass(t, pool)
}

// The trace of the request that saved an event reaches its consumer, through the sequencer,
// on the first run and on a retry (spec 068, 4.1).
func TestATraceGoesFromTheSaveToTheConsumer(t *testing.T) {
	pool := testutil.Database(t).Pool
	ctx := context.Background()
	stop, err := tracing.Start(ctx, tracing.Options{Enabled: true, Sample: 1, Exporter: tracetest.NewInMemoryExporter()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stop(ctx) })
	usePostgres(t, pool)
	var mu sync.Mutex
	var traces []trace.TraceID
	consumer := Consumer{Name: "webhooks", Handle: func(ctx context.Context, _ []byte) error {
		mu.Lock()
		defer mu.Unlock()
		traces = append(traces, trace.SpanContextFromContext(ctx).TraceID())
		if len(traces) == 1 {
			return errors.New("down once")
		}
		return nil
	}}
	r := newRunner(pool, consumer)
	lanes := start(t, r)
	saveCtx, end := tracing.Span(ctx, "request", "")
	want := trace.SpanContextFromContext(saveCtx).TraceID()
	err = pgx.BeginFunc(saveCtx, pool, func(tx pgx.Tx) error {
		_, err := outbox.Enqueue(saveCtx, tx, "page.published", map[string]any{"page_id": "p"})
		return err
	})
	end(nil)
	if err != nil {
		t.Fatal(err)
	}
	sequenceAll(t, pool)
	drain(t, r, lanes)
	makeRetriesDue(t, pool)
	if n, err := r.retryStep(ctx, consumer); err != nil || n != 1 {
		t.Fatalf("retried %d, err %v", n, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(traces) != 2 || traces[0] != want || traces[1] != want {
		t.Fatalf("handled in traces %v, want twice %v", traces, want)
	}
}

// A new consumer starts at the head, and a second start keeps the cursors where they are.
func TestCursorsStartAtTheHeadOnce(t *testing.T) {
	pool := testutil.Database(t).Pool
	usePostgres(t, pool)
	enqueue(t, pool, map[string]any{"page_id": "p"})
	sequenceAll(t, pool)
	rec := newRecorder()
	r := newRunner(pool, rec.consumer("notifications"))
	lanes := start(t, r)
	if n := drain(t, r, lanes); n != 0 {
		t.Fatalf("a new consumer was handed %d events from before it existed", n)
	}
	enqueue(t, pool, map[string]any{"page_id": "p"})
	sequenceAll(t, pool)
	// A restarted worker knows nothing but what the cursors say.
	if n := drain(t, r, start(t, r)); n != 1 {
		t.Fatalf("handed %d after a second start", n)
	}
}
