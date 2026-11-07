// SPDX-License-Identifier: Apache-2.0

package outbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/queue"
	"github.com/diffluence/diffluence/internal/testutil"
)

type countingPublisher struct{ published []string }

func (p *countingPublisher) Publish(_ context.Context, subject string, _ []byte, _ string) error {
	p.published = append(p.published, subject)
	return nil
}

// The queue refuses a message past its limit, and a relay that retried it would stop every
// event behind it, for good.
func TestAnEventTooLargeForTheQueueDoesNotStopTheOthers(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	err := pgx.BeginFunc(ctx, tdb.Pool, func(tx pgx.Tx) error {
		if _, err := outbox.Enqueue(ctx, tx, "page.huge", map[string]string{"x": strings.Repeat("y", outbox.MaxEventBytes)}); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "page.created", map[string]string{"page_id": "p1"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var refused []string
	publisher := &countingPublisher{}
	relay := &outbox.Relay{Pool: tdb.Pool, Publisher: publisher, Refused: func(_ context.Context, data []byte, cause error) error {
		refused = append(refused, cause.Error())
		if len(data) <= outbox.MaxEventBytes {
			t.Errorf("refused %d bytes", len(data))
		}
		return nil
	}}
	if n, err := relay.RunOnce(ctx); err != nil || n != 2 {
		t.Fatalf("handled %d, err %v", n, err)
	}
	if len(publisher.published) != 1 || publisher.published[0] != outbox.SubjectPrefix+"page.created" {
		t.Fatalf("published %v", publisher.published)
	}
	if len(refused) != 1 {
		t.Fatalf("refused %v", refused)
	}
	if n, err := relay.RunOnce(ctx); err != nil || n != 0 {
		t.Fatalf("the refused event came back: %d, err %v", n, err)
	}
}

func TestAnEventThatCouldNotBeSetAsideStaysInTheOutbox(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	err := pgx.BeginFunc(ctx, tdb.Pool, func(tx pgx.Tx) error {
		_, err := outbox.Enqueue(ctx, tx, "page.huge", map[string]string{"x": strings.Repeat("y", outbox.MaxEventBytes)})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	broken := errors.New("no table")
	relay := &outbox.Relay{Pool: tdb.Pool, Publisher: &countingPublisher{}, Refused: func(context.Context, []byte, error) error { return broken }}
	if _, err := relay.RunOnce(ctx); !errors.Is(err, broken) {
		t.Fatalf("err %v", err)
	}
	var waiting int
	if err := tdb.Pool.QueryRow(ctx, "SELECT count(*) FROM outbox_events WHERE published_at IS NULL").Scan(&waiting); err != nil || waiting != 1 {
		t.Fatalf("waiting %d, err %v", waiting, err)
	}
}

func TestRelayPublishesCommittedEventsOnce(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	q, err := queue.Open(ctx, "", t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()

	// One committed and one rolled-back event.
	err = pgx.BeginFunc(ctx, tdb.Pool, func(tx pgx.Tx) error {
		_, err := outbox.Enqueue(ctx, tx, "page.created", map[string]string{"page_id": "p1"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, _ := tdb.Pool.Begin(ctx)
	if _, err := outbox.Enqueue(ctx, tx, "page.created", map[string]string{"page_id": "ghost"}); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)

	relay := &outbox.Relay{Pool: tdb.Pool, Publisher: q}
	n, err := relay.RunOnce(ctx)
	if err != nil || n != 1 {
		t.Fatalf("published %d, err %v", n, err)
	}
	if n, _ := relay.RunOnce(ctx); n != 0 {
		t.Fatalf("second run published %d again", n)
	}

	stream, err := q.JetStream().Stream(ctx, queue.StreamEvents)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := stream.Info(ctx)
	if info.State.Msgs != 1 {
		t.Fatalf("stream has %d messages, want 1", info.State.Msgs)
	}
	raw, err := stream.GetLastMsgForSubject(ctx, outbox.SubjectPrefix+"page.created")
	if err != nil {
		t.Fatal(err)
	}
	var m outbox.Message
	if err := json.Unmarshal(raw.Data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Type != "page.created" || string(m.Payload) != `{"page_id": "p1"}` && string(m.Payload) != `{"page_id":"p1"}` {
		t.Fatalf("unexpected message %+v payload %s", m, m.Payload)
	}
}

func TestDuplicatePublishIsDeduplicated(t *testing.T) {
	ctx := context.Background()
	q, err := queue.Open(ctx, "", t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	// Simulates a crash between "published" and "marked as published".
	for range 2 {
		if err := q.Publish(ctx, outbox.SubjectPrefix+"x.y", []byte("{}"), "same-id"); err != nil {
			t.Fatal(err)
		}
	}
	stream, _ := q.JetStream().Stream(ctx, queue.StreamEvents)
	info, _ := stream.Info(ctx)
	if info.State.Msgs != 1 {
		t.Fatalf("stream has %d messages, want 1", info.State.Msgs)
	}
}

func TestInvalidEventTypeIsRejected(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	tx, _ := tdb.Pool.Begin(ctx)
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := outbox.Enqueue(ctx, tx, "Not A Type", nil); err == nil {
		t.Fatal("accepted invalid type")
	}
}
