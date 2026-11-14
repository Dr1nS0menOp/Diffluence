// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/events"
	"github.com/diffluence/diffluence/internal/failures"
)

// transportCheckEvery is how often a worker looks whether it must run the consumers of the
// event log. After an operator switches, events wait in the log at most this long for them.
const transportCheckEvery = 5 * time.Second

// EventConsumerNames are the consumers of the event log, under the names of the durable
// consumers of NATS.
func EventConsumerNames() []string {
	return []string{notificationConsumer, automationConsumer, webhookConsumer, collabConsumer}
}

// eventConsumers are the consumers of the event log: the same handlers, under the same
// names, as the durable consumers of NATS.
func (a *App) eventConsumers() []events.Consumer {
	listeners := a.eventListeners()
	names := EventConsumerNames()
	out := make([]events.Consumer, 0, len(names))
	for _, name := range names {
		out = append(out, events.Consumer{Name: name, Handle: events.Handler(listeners[name])})
	}
	return out
}

// SwitchEvents sets the path events take, on an operator's command (spec 068, 2.11), and
// reports whether it changed. The cursors of every consumer start at the head of the switch.
func SwitchEvents(ctx context.Context, pool *pgxpool.Pool, to events.Transport) (bool, error) {
	return events.Switch(ctx, pool, to, EventConsumerNames())
}

// EventRunner is the consumers of the event log as this node runs them (spec 068). Tests
// and the measurement step it directly.
func (a *App) EventRunner() *events.Runner {
	return &events.Runner{Pool: a.pool, Node: a.nodeID, Consumers: a.eventConsumers(),
		GiveUp: failures.RecordIn, Log: a.log}
}

// runEventLog waits until the consumers of the event log are needed, then runs them and the
// listening connection until ctx ends. They are needed while the events run through
// PostgreSQL, and after a switch back to NATS until the log holds nothing unhandled.
// Meanwhile it reads one row every few seconds.
func (a *App) runEventLog(ctx context.Context) {
	for {
		needed, err := events.ConsumersNeeded(ctx, a.pool)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			a.log.Warn("events: the transport could not be read; trying again", "error", err)
		case needed:
			r := a.EventRunner()
			r.Wake = events.NewWakeups()
			listenCtx, stopListening := context.WithCancel(ctx)
			defer stopListening()
			go events.Listen(listenCtx, a.cfg.ListenURL(), r.Wake, a.log)
			r.Run(ctx)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(transportCheckEvery):
		}
	}
}

// noteNATSDrained notes when NATS has nothing left after the switch, for the release that
// removes it. NATS is asked only between the switch and the drain.
func (a *App) noteNATSDrained(ctx context.Context) error {
	durables := EventConsumerNames()
	_, err := events.NoteDrained(ctx, a.pool, func(ctx context.Context) (int, error) {
		return a.queue.Waiting(ctx, durables)
	})
	return err
}

// cleanUpEventLog deletes the events every consumer is done with after seven days.
func (a *App) cleanUpEventLog(ctx context.Context) error {
	n, err := events.Cleanup(ctx, a.pool)
	if n > 0 {
		a.log.Info("cleaned up the event log", "events", n)
	}
	return err
}
