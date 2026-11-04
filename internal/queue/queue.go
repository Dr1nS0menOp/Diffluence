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
)

// StreamEvents holds all domain events published by the outbox relay.
const StreamEvents = "DIFFLUENCE_EVENTS"

// Queue is a JetStream connection, plus the embedded server if there is one.
type Queue struct {
	nc       *nats.Conn
	js       jetstream.JetStream
	embedded *server.Server
}

// Open connects to natsURL, or starts an embedded server (no network listener)
// storing its data under dataDir when natsURL is empty.
func Open(ctx context.Context, natsURL, dataDir string, log *slog.Logger) (*Queue, error) {
	q := &Queue{}
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

// Publish sends data with a de-duplication id (satisfies outbox.Publisher).
func (q *Queue) Publish(ctx context.Context, subject string, data []byte, msgID string) error {
	_, err := q.js.Publish(ctx, subject, data, jetstream.WithMsgID(msgID))
	return err
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
