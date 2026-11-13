// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// listenRetry is the pause before a lost listening connection is opened again. Polling
// carries the events meanwhile.
const listenRetry = 30 * time.Second

// Wakeups hands a notification of the sequencer to every loop of a worker.
type Wakeups struct {
	mu   sync.Mutex
	subs map[chan struct{}]bool
}

// NewWakeups creates an empty set of subscribers.
func NewWakeups() *Wakeups { return &Wakeups{subs: map[chan struct{}]bool{}} }

// Subscribe returns a channel that receives a value after a wake-up. A loop that is busy
// misses none: one wake-up waits for it.
func (w *Wakeups) Subscribe() <-chan struct{} {
	ch := make(chan struct{}, 1)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.subs[ch] = true
	return ch
}

// Unsubscribe stops wake-ups on a channel Subscribe returned.
func (w *Wakeups) Unsubscribe(ch <-chan struct{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for sub := range w.subs {
		if sub == ch {
			delete(w.subs, sub)
		}
	}
}

// wakeAll wakes every subscriber without waiting for any.
func (w *Wakeups) wakeAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for sub := range w.subs {
		select {
		case sub <- struct{}{}:
		default:
		}
	}
}

// Listen holds one connection that listens on Channel and wakes the loops on every
// notification, until ctx ends. It needs a session, so behind PgBouncer in transaction mode
// connString must reach the database directly (D8). When it cannot listen it says so once,
// and the loops run on polling; once it listens again it wakes every loop at once, since
// notifications sent meanwhile were lost (D6).
func Listen(ctx context.Context, connString string, w *Wakeups, log *slog.Logger) {
	warned := false
	for ctx.Err() == nil {
		err := listenOnce(ctx, connString, w, func() {
			if warned {
				log.Info("events: listening for new events again")
			}
			warned = false
		})
		if ctx.Err() != nil {
			return
		}
		if !warned {
			log.Warn("events: cannot listen for new events; the event loops poll every second", "error", err)
			warned = true
		}
		if !sleep(ctx, listenRetry) {
			return
		}
	}
}

// listenOnce connects, listens and wakes the loops until the connection fails or ctx ends.
func listenOnce(ctx context.Context, connString string, w *Wakeups, listening func()) error {
	cfg, err := pgx.ParseConfig(connString)
	if err != nil {
		// Never the address itself: it may hold a password.
		return errors.New("events: the listen address is not valid")
	}
	cfg.RuntimeParams["application_name"] = "diffluence-listen"
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), releaseWait)
		defer cancel()
		// The connection is done with either way; a failed goodbye changes nothing.
		_ = conn.Close(closeCtx)
	}()
	if _, err := conn.Exec(ctx, "LISTEN "+Channel); err != nil {
		return err
	}
	listening()
	w.wakeAll()
	for {
		if _, err := conn.WaitForNotification(ctx); err != nil {
			return err
		}
		w.wakeAll()
	}
}
