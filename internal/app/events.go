// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"time"

	"github.com/diffluence/diffluence/internal/events"
	"github.com/diffluence/diffluence/internal/failures"
)

// transportCheckEvery is how often a worker looks whether the events moved to PostgreSQL.
const transportCheckEvery = time.Minute

// eventConsumers are the consumers of the event log: the same handlers, under the same
// names, as the durable consumers of NATS.
func (a *App) eventConsumers() []events.Consumer {
	listeners := a.eventListeners()
	out := make([]events.Consumer, 0, len(listeners))
	for _, name := range []string{notificationConsumer, automationConsumer, webhookConsumer, collabConsumer} {
		out = append(out, events.Consumer{Name: name, Handle: events.Handler(listeners[name])})
	}
	return out
}

// EventRunner is the consumers of the event log as this node runs them (spec 068). Tests
// and the measurement step it directly.
func (a *App) EventRunner() *events.Runner {
	return &events.Runner{Pool: a.pool, Node: a.nodeID, Consumers: a.eventConsumers(),
		GiveUp: failures.RecordIn, Log: a.log}
}

// runEventLog waits until the events run through PostgreSQL, then runs their consumers and
// the listening connection until ctx ends. While the transport is nats, as it stays until
// wave EV2, it only reads one row a minute.
func (a *App) runEventLog(ctx context.Context) {
	for {
		t, err := events.CurrentTransport(ctx, a.pool)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			a.log.Warn("events: the transport could not be read; trying again", "error", err)
		case t == events.TransportPostgres:
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

// cleanUpEventLog deletes the events every consumer is done with after seven days.
func (a *App) cleanUpEventLog(ctx context.Context) error {
	n, err := events.Cleanup(ctx, a.pool)
	if n > 0 {
		a.log.Info("cleaned up the event log", "events", n)
	}
	return err
}
