// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/identity"
)

// smallPool gives the app a pool of n connections, so a request that holds one
// connection and asks the pool for another shows up with a handful of requests.
func smallPool(n int) func(*config.Config) {
	return func(cfg *config.Config) {
		joiner := "?"
		if strings.Contains(cfg.DatabaseURL, "?") {
			joiner = "&"
		}
		cfg.DatabaseURL += fmt.Sprintf("%spool_max_conns=%d", joiner, n)
	}
}

// TestManySpacesCreatedAtOnceAllFinish creates more spaces at once than the pool has
// connections. Each creation must finish on its own connection, so all of them end
// in time instead of waiting on each other (spec 050, step 1).
func TestManySpacesCreatedAtOnceAllFinish(t *testing.T) {
	e := newEnvWith(t, smallPool(3))
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	client := &http.Client{Jar: owner.c.Jar, Timeout: 30 * time.Second}

	const creations = 12
	var wg sync.WaitGroup
	statuses := make([]int, creations)
	failures := make([]error, creations)
	started := time.Now()
	for i := range creations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := fmt.Sprintf(`{"key":"PAR%d","name":"Parallel %d"}`, i, i)
			req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/spaces", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				failures[i] = err
				return
			}
			statuses[i] = resp.StatusCode
			_ = resp.Body.Close()
		}()
	}
	wg.Wait()
	for i := range creations {
		if failures[i] != nil {
			t.Fatalf("creation %d did not finish: %v", i, failures[i])
		}
		if statuses[i] != http.StatusCreated {
			t.Errorf("creation %d: status %d, want 201", i, statuses[i])
		}
	}
	if took := time.Since(started); took > 20*time.Second {
		t.Errorf("%d creations on a pool of 3 took %s", creations, took)
	}
}

// TestARequestOnAFullPoolIsToldBusyInTime holds every connection of the pool, as stuck
// work would. A request then answers "busy" with a time to come back, instead of
// waiting for as long as the work stays stuck.
func TestARequestOnAFullPoolIsToldBusyInTime(t *testing.T) {
	e := newEnvWith(t, smallPool(2))
	reader := e.user("reader@example.com", identity.PermUseProduct)
	for range 2 {
		held, err := e.app.Pool().Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(held.Release)
	}
	client := &http.Client{Jar: reader.c.Jar, Timeout: 60 * time.Second}
	started := time.Now()
	resp, err := client.Get(e.srv.URL + "/api/v1/spaces")
	if err != nil {
		t.Fatalf("the request did not end: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("Retry-After") == "" {
		t.Errorf("status %d, Retry-After %q; want 503 with a time to come back", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	if took := time.Since(started); took > 20*time.Second {
		t.Errorf("the answer took %s", took)
	}
}
