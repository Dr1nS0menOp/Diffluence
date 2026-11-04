// SPDX-License-Identifier: Apache-2.0

package outbox_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/queue"
	"github.com/diffluence/diffluence/internal/testutil"
)

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
