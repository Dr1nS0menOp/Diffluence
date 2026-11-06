// SPDX-License-Identifier: Apache-2.0

package queue

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

func embedded(t *testing.T) *Queue {
	t.Helper()
	q, err := Open(context.Background(), "", t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Close)
	q.retryWaits = []time.Duration{time.Millisecond, time.Millisecond}
	return q
}

func waitFor(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out")
	}
}

func TestABadEventStopsComingBackAndIsHandedOver(t *testing.T) {
	q := embedded(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var tries atomic.Int32
	gaveUp := make(chan struct{})
	var kept []byte
	go func() {
		_ = q.Consume(ctx, Listener{
			Durable: "bad",
			Handle:  func(context.Context, []byte) error { tries.Add(1); return errors.New("always") },
			GiveUp: func(_ context.Context, durable string, data []byte, cause error) error {
				if durable != "bad" || cause == nil {
					t.Errorf("%s %v", durable, cause)
				}
				kept = data
				close(gaveUp)
				return nil
			},
		})
	}()
	if err := q.Publish(ctx, "diffluence.events.test", []byte("the event"), "one"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, gaveUp)
	time.Sleep(100 * time.Millisecond)
	if got := tries.Load(); got != 3 {
		t.Fatalf("%d tries, want 3", got)
	}
	if string(kept) != "the event" {
		t.Fatalf("%q", kept)
	}
}

func TestAnEventThatRecoversIsNotHandedOver(t *testing.T) {
	q := embedded(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var tries atomic.Int32
	done := make(chan struct{})
	go func() {
		_ = q.Consume(ctx, Listener{
			Durable: "flaky",
			Handle: func(context.Context, []byte) error {
				if tries.Add(1) == 1 {
					return errors.New("once")
				}
				close(done)
				return nil
			},
			GiveUp: func(context.Context, string, []byte, error) error { t.Error("gave up"); return nil },
		})
	}()
	if err := q.Publish(ctx, "diffluence.events.test", []byte("x"), "two"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, done)
}

func TestAFailureThatCannotBeStoredIsTriedAgain(t *testing.T) {
	q := embedded(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stores atomic.Int32
	stored := make(chan struct{})
	go func() {
		_ = q.Consume(ctx, Listener{
			Durable: "nodb",
			Handle:  func(context.Context, []byte) error { return errors.New("down") },
			GiveUp: func(context.Context, string, []byte, error) error {
				if stores.Add(1) == 1 {
					return errors.New("database down too")
				}
				close(stored)
				return nil
			},
		})
	}()
	if err := q.Publish(ctx, "diffluence.events.test", []byte("x"), "three"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, stored)
}
