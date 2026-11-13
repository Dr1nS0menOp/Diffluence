// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/testutil"
)

// logOf sequences n events of n pages and starts one consumer with its cursors at zero.
func logOf(t *testing.T, n int) *pgxpool.Pool {
	t.Helper()
	pool := testutil.Database(t).Pool
	usePostgres(t, pool)
	r := newRunner(pool, newRecorder().consumer("notifications"))
	start(t, r)
	for i := range n {
		enqueue(t, pool, map[string]any{"page_id": fmt.Sprint(i)})
	}
	sequenceAll(t, pool)
	return pool
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

// remaining is the lowest position left and how many rows are left.
func remaining(t *testing.T, pool *pgxpool.Pool) (lowest int64, count int) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), "SELECT coalesce(min(pos), 0), count(*) FROM event_log").Scan(&lowest, &count); err != nil {
		t.Fatal(err)
	}
	return lowest, count
}

func cleanupNow(t *testing.T, pool *pgxpool.Pool, batch int) int64 {
	t.Helper()
	n, err := cleanup(context.Background(), pool, keepFor, batch)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

const ageAll = "UPDATE event_log SET sequenced_at = now() - interval '8 days'"

func TestCleanupKeepsWhatACursorHasNotPassed(t *testing.T) {
	pool := logOf(t, 10)
	exec(t, pool, ageAll)
	exec(t, pool, "UPDATE event_cursors SET pos = 10")
	exec(t, pool, "UPDATE event_cursors SET pos = 3 WHERE lane = 0")
	if n := cleanupNow(t, pool, cleanupBatch); n != 3 {
		t.Fatalf("deleted %d", n)
	}
	if lowest, count := remaining(t, pool); lowest != 4 || count != 7 {
		t.Fatalf("left %d rows from %d", count, lowest)
	}
	mustPass(t, pool)
}

func TestCleanupKeepsWhatARetryNames(t *testing.T) {
	pool := logOf(t, 10)
	exec(t, pool, ageAll)
	exec(t, pool, "UPDATE event_cursors SET pos = 10")
	exec(t, pool, `INSERT INTO event_retries (consumer, pos, event_id, tries, next_at, last_error)
		SELECT 'notifications', pos, id, 1, now(), 'down' FROM event_log WHERE pos = 6`)
	if n := cleanupNow(t, pool, cleanupBatch); n != 5 {
		t.Fatalf("deleted %d", n)
	}
	if lowest, _ := remaining(t, pool); lowest != 6 {
		t.Fatalf("left from %d", lowest)
	}
	mustPass(t, pool)
}

// Only a run from the start is deleted: an event newer than seven days keeps everything
// after it, also older ones, so the log never gets a hole.
func TestCleanupKeepsTheLastSevenDays(t *testing.T) {
	pool := logOf(t, 10)
	exec(t, pool, "UPDATE event_cursors SET pos = 10")
	exec(t, pool, "UPDATE event_log SET sequenced_at = now() - interval '8 days' WHERE pos <= 6 AND pos <> 3")
	if n := cleanupNow(t, pool, cleanupBatch); n != 2 {
		t.Fatalf("deleted %d", n)
	}
	if lowest, count := remaining(t, pool); lowest != 3 || count != 8 {
		t.Fatalf("left %d rows from %d", count, lowest)
	}
	mustPass(t, pool)
}

func TestCleanupKeepsEventsJustInsideTheirDays(t *testing.T) {
	pool := logOf(t, 3)
	exec(t, pool, "UPDATE event_cursors SET pos = 3")
	exec(t, pool, "UPDATE event_log SET sequenced_at = now() - interval '7 days' + interval '1 minute'")
	if n := cleanupNow(t, pool, cleanupBatch); n != 0 {
		t.Fatalf("deleted %d events younger than seven days", n)
	}
}

func TestCleanupDeletesInBatchesUntilDone(t *testing.T) {
	pool := logOf(t, 10)
	exec(t, pool, ageAll)
	exec(t, pool, "UPDATE event_cursors SET pos = 10")
	if n := cleanupNow(t, pool, 3); n != 10 {
		t.Fatalf("deleted %d", n)
	}
	if _, count := remaining(t, pool); count != 0 {
		t.Fatalf("left %d", count)
	}
	mustPass(t, pool)
}

// No consumer has a cursor yet: nobody has read anything, so nothing may go.
func TestCleanupWithoutCursorsDeletesNothing(t *testing.T) {
	pool := logOf(t, 3)
	exec(t, pool, ageAll)
	exec(t, pool, "DELETE FROM event_cursors")
	if n := cleanupNow(t, pool, cleanupBatch); n != 0 {
		t.Fatalf("deleted %d", n)
	}
}

func TestTheCheckFindsAGap(t *testing.T) {
	pool := logOf(t, 6)
	exec(t, pool, "DELETE FROM event_log WHERE pos = 4")
	expectBreach(t, pool, "L1")
}

func TestTheCheckFindsAHeadBehindTheLog(t *testing.T) {
	pool := logOf(t, 3)
	exec(t, pool, "UPDATE event_head SET pos = 2")
	expectBreach(t, pool, "L1")
}

func TestTheCheckFindsACursorPastTheHead(t *testing.T) {
	pool := logOf(t, 3)
	exec(t, pool, "UPDATE event_cursors SET pos = 4 WHERE lane = 1")
	expectBreach(t, pool, "L2")
}

func TestTheCheckFindsARetryAboveItsCursor(t *testing.T) {
	pool := logOf(t, 3)
	exec(t, pool, `INSERT INTO event_retries (consumer, pos, event_id, tries, next_at, last_error)
		SELECT 'notifications', pos, id, 1, now(), 'down' FROM event_log WHERE pos = 2`)
	expectBreach(t, pool, "L2")
	exec(t, pool, "UPDATE event_cursors SET pos = 3")
	mustPass(t, pool)
}

func TestTheCheckFindsARetryWithoutItsEvent(t *testing.T) {
	pool := logOf(t, 3)
	exec(t, pool, "UPDATE event_cursors SET pos = 3")
	exec(t, pool, `INSERT INTO event_retries (consumer, pos, event_id, tries, next_at, last_error)
		SELECT 'notifications', 9, id, 1, now(), 'down' FROM event_log WHERE pos = 2`)
	expectBreach(t, pool, "L2")
}

func TestTheCheckFindsARetryOfAnotherEvent(t *testing.T) {
	pool := logOf(t, 3)
	exec(t, pool, "UPDATE event_cursors SET pos = 3")
	exec(t, pool, `INSERT INTO event_retries (consumer, pos, event_id, tries, next_at, last_error)
		VALUES ('notifications', 2, $1, 1, now(), 'down')`, uuid.New())
	expectBreach(t, pool, "L2")
}

func TestTheCheckFindsAnEventWaitingTooLongWhileAWorkerRuns(t *testing.T) {
	pool := logOf(t, 1)
	exec(t, pool, "INSERT INTO outbox_events (id, type, payload, created_at) VALUES ($1, 'page.updated', '{}', now() - interval '2 minutes')", uuid.New())
	// No worker runs: the event may wait.
	mustPass(t, pool)
	// A worker that started a moment ago may still be working through a backlog.
	exec(t, pool, "INSERT INTO nodes (id, role, version, max_migration) VALUES ($1, 'worker', 'test', 53)", uuid.New())
	mustPass(t, pool)
	exec(t, pool, "UPDATE nodes SET started_at = now() - interval '2 minutes'")
	expectBreach(t, pool, "L3")
	exec(t, pool, "UPDATE outbox_events SET created_at = now() - interval '50 seconds'")
	mustPass(t, pool)
}

func TestAnAPINodeDoesNotCountAsAWorker(t *testing.T) {
	pool := logOf(t, 1)
	exec(t, pool, "INSERT INTO outbox_events (id, type, payload, created_at) VALUES ($1, 'page.updated', '{}', now() - interval '2 minutes')", uuid.New())
	exec(t, pool, "INSERT INTO nodes (id, role, version, max_migration, started_at) VALUES ($1, 'api', 'test', 53, now() - interval '1 hour')", uuid.New())
	exec(t, pool, `INSERT INTO nodes (id, role, version, max_migration, started_at, last_seen_at)
		VALUES ($1, 'all', 'test', 53, now() - interval '1 hour', now() - interval '10 minutes')`, uuid.New())
	mustPass(t, pool)
}

func expectBreach(t *testing.T, pool *pgxpool.Pool, rule string) {
	t.Helper()
	breach, err := Check(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if breach == nil || breach.Rule != rule {
		t.Fatalf("breach %v, want %s", breach, rule)
	}
	if !strings.HasPrefix(breach.String(), rule+": ") {
		t.Fatalf("says %q", breach.String())
	}
}

func TestTheLagIsTheOldestEventPastACursor(t *testing.T) {
	pool := logOf(t, 4)
	ctx := context.Background()
	// Position p happened 10 - p minutes ago, all in lane 0.
	exec(t, pool, "UPDATE event_log SET lane = 0, sequenced_at = now() - make_interval(mins => 10 - pos::int)")
	exec(t, pool, `INSERT INTO event_retries (consumer, pos, event_id, tries, next_at, last_error)
		SELECT 'notifications', pos, id, 1, now(), 'down' FROM event_log WHERE pos = 1`)
	exec(t, pool, "UPDATE event_cursors SET pos = CASE WHEN lane = 0 THEN 2 ELSE 4 END")
	states, err := ReadConsumers(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].Consumer != "notifications" || states[0].Retries != 1 {
		t.Fatalf("%+v", states)
	}
	if lag := states[0].Lag; lag < 7*time.Minute-time.Second || lag > 7*time.Minute+5*time.Second {
		t.Fatalf("lag %v, want the age of position 3, 7 minutes", lag)
	}
	exec(t, pool, "UPDATE event_cursors SET pos = 4")
	if states, err = ReadConsumers(ctx, pool); err != nil || states[0].Lag != 0 {
		t.Fatalf("a consumer at the head lags %v, err %v", states[0].Lag, err)
	}
}
