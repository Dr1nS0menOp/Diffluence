// SPDX-License-Identifier: Apache-2.0

// Package queue connects to NATS JetStream: an embedded server for single-node
// installs, or an external cluster for HA (docs/blueprint/10, ADR-007).
package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/diffluence/diffluence/internal/tracing"
)

// StreamEvents holds all domain events published by the outbox relay.
const StreamEvents = "DIFFLUENCE_EVENTS"

// Queue is a JetStream connection, plus the embedded server if there is one.
type Queue struct {
	nc       *nats.Conn
	js       jetstream.JetStream
	embedded *server.Server
	log      *slog.Logger
	// retryWaits holds one pause per try after the first.
	retryWaits []time.Duration
}

// Open connects to natsURL, or starts an embedded server (no network listener)
// storing its data under dataDir when natsURL is empty.
func Open(ctx context.Context, natsURL, dataDir string, log *slog.Logger) (*Queue, error) {
	q := &Queue{log: log, retryWaits: defaultRetryWaits}
	opts := []nats.Option{
		nats.Name("diffluence"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				log.Warn("nats disconnected", "error", err)
			}
		}),
		nats.ReconnectHandler(func(*nats.Conn) { log.Info("nats reconnected") }),
	}

	var err error
	if natsURL == "" {
		ns, nerr := server.NewServer(&server.Options{
			ServerName: "diffluence-embedded",
			DontListen: true, // only reachable in-process
			JetStream:  true,
			StoreDir:   filepath.Join(dataDir, "nats"),
			NoSigs:     true,
			NoLog:      true,
		})
		if nerr != nil {
			return nil, fmt.Errorf("queue: create embedded NATS: %w", nerr)
		}
		go ns.Start()
		if !ns.ReadyForConnections(15 * time.Second) {
			ns.Shutdown()
			return nil, errors.New("queue: embedded NATS did not start")
		}
		q.embedded = ns
		opts = append(opts, nats.InProcessServer(ns))
		q.nc, err = nats.Connect(ns.ClientURL(), opts...)
		log.Info("embedded NATS JetStream started", "store", filepath.Join(dataDir, "nats"))
	} else {
		q.nc, err = nats.Connect(natsURL, opts...)
	}
	if err != nil {
		q.Close()
		return nil, fmt.Errorf("queue: connect: %w", err)
	}

	q.js, err = jetstream.New(q.nc)
	if err != nil {
		q.Close()
		return nil, err
	}
	sctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	_, err = q.js.CreateOrUpdateStream(sctx, jetstream.StreamConfig{
		Name:       StreamEvents,
		Subjects:   []string{"diffluence.events.>"},
		Storage:    jetstream.FileStorage,
		Retention:  jetstream.LimitsPolicy,
		MaxAge:     7 * 24 * time.Hour,
		Duplicates: 2 * time.Minute,
	})
	if err != nil {
		q.Close()
		return nil, fmt.Errorf("queue: create stream: %w", err)
	}
	return q, nil
}

// deliver runs the listener on one event, and tells the server what became of it.
func (q *Queue) deliver(ctx context.Context, l Listener, m jetstream.Msg) {
	ctx, end := tracing.Consume(ctx, m.Subject(), l.Durable, m.Headers())
	cause := l.Handle(ctx, m.Data())
	end(cause)
	if cause == nil {
		q.settle(m.Ack(), l.Durable)
		return
	}
	tries := q.triesOf(m)
	if tries <= len(q.retryWaits) {
		q.settle(m.NakWithDelay(q.retryWaits[tries-1]), l.Durable)
		return
	}
	if err := l.GiveUp(ctx, l.Durable, m.Data(), cause); err != nil {
		q.log.Error("queue: a failed event could not be stored", "consumer", l.Durable, "error", err)
		q.settle(m.NakWithDelay(q.retryWaits[len(q.retryWaits)-1]), l.Durable)
		return
	}
	q.log.Warn("queue: gave up on an event", "consumer", l.Durable, "tries", tries, "error", cause)
	q.settle(m.Term(), l.Durable)
}

// triesOf says how often the event has been delivered, this time included.
func (q *Queue) triesOf(m jetstream.Msg) int {
	meta, err := m.Metadata()
	if err != nil {
		q.log.Warn("queue: an event has no metadata", "error", err)
		return 1
	}
	return int(meta.NumDelivered)
}

// settle logs an answer that did not reach the server. The event then simply comes
// back after its wait, so a listener runs twice, which it must be able to.
func (q *Queue) settle(err error, durable string) {
	if err != nil {
		q.log.Warn("queue: could not answer the server", "consumer", durable, "error", err)
	}
}

// Publish sends data with a de-duplication id (satisfies outbox.Publisher).
func (q *Queue) Publish(ctx context.Context, subject string, data []byte, msgID string) error {
	if !tracing.On() {
		_, err := q.js.Publish(ctx, subject, data, jetstream.WithMsgID(msgID))
		return err
	}
	// The trace goes with the event in its headers, so the listener's work shows under the
	// publish that caused it (spec 060 D8).
	msg := &nats.Msg{Subject: subject, Data: data, Header: nats.Header{}}
	ctx, end := tracing.Publish(ctx, subject, msg.Header)
	_, err := q.js.PublishMsg(ctx, msg, jetstream.WithMsgID(msgID))
	end(err)
	return err
}

const (
	// consumeWait is how long a handler gets for one event before it is sent again.
	consumeWait = time.Minute
	// giveUpRoom is how many more deliveries a failure gets to reach the database,
	// because a listener mostly fails when the database is down.
	giveUpRoom = 20
)

// defaultRetryWaits are the pauses between the tries of one event. By default
// JetStream sends an event again for ever, so one bad event would never leave (ARC-050).
var defaultRetryWaits = []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}

// Listener is what a durable consumer does with events.
type Listener struct {
	Durable string
	Handle  func(ctx context.Context, data []byte) error
	// GiveUp stores an event after its last try. Only then does the queue drop it.
	GiveUp func(ctx context.Context, durable string, data []byte, cause error) error
}

// Consume hands every event to the listener, until ctx is cancelled. A durable consumer
// keeps its place across restarts, and shared by several workers each event goes to one
// of them. An event that fails is sent again, so a handler must be safe to run twice.
func (q *Queue) Consume(ctx context.Context, l Listener) error {
	consumer, err := q.js.CreateOrUpdateConsumer(ctx, StreamEvents, jetstream.ConsumerConfig{
		Durable: l.Durable, AckPolicy: jetstream.AckExplicitPolicy, AckWait: consumeWait,
		MaxDeliver: len(q.retryWaits) + 1 + giveUpRoom})
	if err != nil {
		return fmt.Errorf("queue: create consumer: %w", err)
	}
	running, err := consumer.Consume(func(m jetstream.Msg) { q.deliver(ctx, l, m) })
	if err != nil {
		return fmt.Errorf("queue: consume: %w", err)
	}
	<-ctx.Done()
	running.Stop()
	return nil
}

// JetStream exposes the JetStream API for consumers.
func (q *Queue) JetStream() jetstream.JetStream { return q.js }

// Healthy reports whether the connection is up.
func (q *Queue) Healthy() bool { return q.nc != nil && q.nc.IsConnected() }

// Close drains the connection and stops the embedded server.
func (q *Queue) Close() {
	if q.nc != nil {
		_ = q.nc.Drain()
		for i := 0; i < 50 && !q.nc.IsClosed(); i++ {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if q.embedded != nil {
		q.embedded.Shutdown()
		q.embedded.WaitForShutdown()
	}
}
