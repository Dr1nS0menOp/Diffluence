// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/httpapi"
)

func TestAFullPoolIsBusyNotAGoneDatabase(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.app.Ready(ctx); err != nil {
		t.Fatalf("an idle node is not ready: %v", err)
	}
	pool := e.app.Pool()
	var held []*pgxpool.Conn
	for range pool.Config().MaxConns {
		c, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, c)
	}
	release := func() {
		for _, c := range held {
			c.Release()
		}
		held = nil
	}
	// The app closes its pool after the test, which waits for every connection.
	defer release()
	err := e.app.Ready(ctx)
	var reason httpapi.ReadyError
	if !errors.As(err, &reason) || reason != "busy" {
		t.Fatalf("a full pool reported %v", err)
	}
	release()
	if err := e.app.Ready(ctx); err != nil {
		t.Fatalf("a free pool is not ready: %v", err)
	}
}

func TestLivezStaysOkWhileReadyzSaysBusy(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	pool := e.app.Pool()
	var held []*pgxpool.Conn
	for range pool.Config().MaxConns {
		c, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, c)
	}
	defer func() {
		for _, c := range held {
			c.Release()
		}
	}()
	c := e.client()
	if resp, body := e.do(c, "GET", "/readyz", ""); resp.StatusCode != 503 || body == "" {
		t.Fatalf("readyz: %d %s", resp.StatusCode, body)
	}
	if resp, _ := e.do(c, "GET", "/livez", ""); resp.StatusCode != 200 {
		t.Fatalf("livez: %d", resp.StatusCode)
	}
}
