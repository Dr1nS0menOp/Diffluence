// SPDX-License-Identifier: Apache-2.0

package queue

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func embedded(t *testing.T) *Queue {
	t.Helper()
	q, err := Open(context.Background(), "", t.TempDir(), 1, slog.New(slog.DiscardHandler))
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

// An event waiting for its retry is still in NATS: the drain after the switch to PostgreSQL
// must wait for it (spec 068, 2.11). One that was handled is not.
func TestAnEventWaitingForItsRetryCountsAsWaiting(t *testing.T) {
	q := embedded(t)
	q.retryWaits = []time.Duration{time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failed, handled := make(chan struct{}), make(chan struct{})
	var failOnce, handleOnce sync.Once
	listeners := []Listener{
		{Durable: "slow", Handle: func(context.Context, []byte) error {
			failOnce.Do(func() { close(failed) })
			return errors.New("down")
		}},
		{Durable: "fast", Handle: func(context.Context, []byte) error {
			handleOnce.Do(func() { close(handled) })
			return nil
		}},
	}
	for _, l := range listeners {
		l.GiveUp = func(context.Context, string, []byte, error) error { return nil }
		go func() { _ = q.Consume(ctx, l) }()
	}
	if _, err := q.Waiting(ctx, []string{"never-made"}); err == nil {
		t.Fatal("a consumer that does not exist has nothing waiting")
	}
	if err := q.Publish(ctx, "diffluence.events.test", []byte("x"), "four"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, failed)
	waitFor(t, handled)
	if n, err := q.Waiting(ctx, []string{"slow"}); err != nil || n != 1 {
		t.Fatalf("waiting %d, err %v", n, err)
	}
	// The acknowledgement reaches the server a moment after the handler returns.
	deadline := time.Now().Add(30 * time.Second)
	for {
		n, err := q.Waiting(ctx, []string{"fast"})
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a handled event still waits: %d", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n, err := q.Waiting(ctx, []string{"slow", "fast"}); err != nil || n != 1 {
		t.Fatalf("waiting in both %d, err %v", n, err)
	}
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
