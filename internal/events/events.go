// SPDX-License-Identifier: Apache-2.0

// Package events carries the domain events through PostgreSQL alone (spec 068): a sequencer
// moves committed rows from the outbox into event_log with gapless positions, and every
// consumer reads that log through one cursor per lane, held by a lease. A failed event waits
// in event_retries and, after its last try, goes to event_failures. NOTIFY only wakes the
// loops; each loop also polls, so a lost notification costs latency, never an event.
//
// The path is built beside NATS. event_transport says which one the relay takes, and it
// stays 'nats' until wave EV2 switches it.
package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/outbox"
)

// Lanes is how many lanes each consumer has (D5). The migration fixes it in a CHECK, and
// changing it needs every cursor at the head first.
const Lanes = 4

// Channel is what the sequencer notifies and the worker listens on (D6). The payload is the
// head position as text.
const Channel = "diffluence_events"

// lockKey serialises the relay and the sequencer across nodes, so positions follow the
// order in which events became visible (D1).
const lockKey int64 = 0x6466_6c75_6576_6e74 // "dfluevnt"

// Transport is the path events take from the outbox.
type Transport string

// The transports.
const (
	TransportNATS     Transport = "nats"
	TransportPostgres Transport = "postgres"
)

// CurrentTransport reads which path events take.
func CurrentTransport(ctx context.Context, q db.Querier) (Transport, error) {
	var t Transport
	if err := q.QueryRow(ctx, "SELECT transport FROM event_transport").Scan(&t); err != nil {
		return "", fmt.Errorf("events: read the transport: %w", err)
	}
	return t, nil
}

// Handler takes one event, as the JSON body of outbox.Message, the same bytes NATS carried.
type Handler func(ctx context.Context, data []byte) error

// Consumer is one durable reader of every event.
type Consumer struct {
	Name   string
	Handle Handler
}

// event is one row of event_log.
type event struct {
	pos int64
	msg outbox.Message
}

// body is the event as a handler takes it.
func (e event) body() ([]byte, error) {
	data, err := json.Marshal(e.msg)
	if err != nil {
		return nil, fmt.Errorf("events: encode event %d: %w", e.pos, err)
	}
	return data, nil
}

// scanEvent reads pos, id, type, payload and created_at.
func scanEvent(row pgx.CollectableRow) (event, error) {
	var e event
	var payload []byte
	err := row.Scan(&e.pos, &e.msg.ID, &e.msg.Type, &payload, &e.msg.CreatedAt)
	e.msg.Payload = payload
	return e, err
}

// handlerWait is how long a handler gets for one event, as NATS gave it (consumeWait).
const handlerWait = time.Minute

// handle runs one handler on one event within its time.
func handle(ctx context.Context, h Handler, e event) error {
	data, err := e.body()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, handlerWait)
	defer cancel()
	return h(ctx, data)
}

// errLeaseLost says another worker took the lane while this one ran a batch. The batch then
// counts as not done, and the new holder runs it again.
var errLeaseLost = errors.New("events: the lease of the lane was lost")
