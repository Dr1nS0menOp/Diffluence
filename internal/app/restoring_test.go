// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/restore"
	"github.com/diffluence/diffluence/internal/testutil"
)

// putRestore writes a restore in a state, as the console and the worker would.
func putRestore(t *testing.T, tdb testutil.TestDB, state string, changed bool, step string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := tdb.Pool.Exec(context.Background(), `INSERT INTO ops.restores (id, backup_name, requested_by_name, state, changed, step)
		VALUES ($1, 'diffluence-20261111T071618Z', 'test', $2, $3, $4)`, id, state, changed, step); err != nil {
		t.Fatal(err)
	}
	return id
}

// The waiting page of a restore names the step, never the backup or the person, and lets
// the node go once the restore is done.
func TestANodeWaitsOnThePageWhileARestoreRunsAndGoesOnAfter(t *testing.T) {
	tdb := testutil.Database(t)
	id := putRestore(t, tdb, restore.StateRestoring, true, "restoring the database")
	r, err := restore.MustWait(context.Background(), tdb.Pool)
	if err != nil || r == nil {
		t.Fatalf("a restore that changes the installation keeps the nodes waiting: %+v %v", r, err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := "http://" + ln.Addr().String()
	node := uuid.New()
	done := make(chan error, 1)
	go func() {
		done <- waitForRestore(context.Background(), tdb.Pool, ln, r, node, 50*time.Millisecond, slog.New(slog.DiscardHandler))
	}()
	resp, err := http.Get(addr + "/somewhere")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), "restoring the database") ||
		strings.Contains(string(body), "diffluence-2026") || strings.Contains(string(body), "test") {
		t.Fatalf("the page: %d %s", resp.StatusCode, body)
	}
	resp, err = http.Get(addr + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	var ready map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&ready)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable || ready["status"] != "restoring" {
		t.Fatalf("/readyz while restoring: %d %v", resp.StatusCode, ready)
	}
	// The page says when it failed, with the way back.
	if err := restore.Fail(context.Background(), tdb.Pool, id, "failed while restoring the stored files and keys"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get(addr + "/")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if strings.Contains(string(body), "way-back") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the page never showed the way back: %s", body)
		}
		time.Sleep(50 * time.Millisecond)
	}
	select {
	case err := <-done:
		t.Fatalf("the node stopped waiting while the restore had failed: %v", err)
	default:
	}
	var mode string
	if err := tdb.Pool.QueryRow(context.Background(), "SELECT mode FROM ops.nodes WHERE node_id = $1", node).Scan(&mode); err != nil || mode != restore.ModeWaiting {
		t.Fatalf("the node does not say it waits: %q %v", mode, err)
	}
	if err := restore.Finish(context.Background(), tdb.Pool, id); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the wait ended with %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the node kept waiting after the restore was done")
	}
}

// A restore that is asked for or being checked leaves the nodes serving; from the moment the
// worker stops them, they wait.
func TestTheNodesWaitOnlyOnceTheWorkerStopsThem(t *testing.T) {
	ctx := context.Background()
	for state, wait := range map[string]bool{
		restore.StateRequested: false, restore.StatePreparing: false, restore.StateStopping: true,
		restore.StateBackingUp: true, restore.StateRestoring: true, restore.StateChecking: true,
	} {
		tdb := testutil.Database(t)
		putRestore(t, tdb, state, false, "")
		r, err := restore.MustWait(ctx, tdb.Pool)
		if err != nil {
			t.Fatal(err)
		}
		if (r != nil) != wait {
			t.Fatalf("in state %s the nodes wait: %v, want %v", state, r != nil, wait)
		}
	}
}

// A node that starts while a restore runs never serves, and a serving node stops once one
// begins.
func TestAServingNodeStopsWhenARestoreBegins(t *testing.T) {
	tdb := testutil.Database(t)
	a := &App{pool: tdb.Pool, log: slog.New(slog.DiscardHandler)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	go a.watchRestore(ctx, started)
	select {
	case <-started:
		t.Fatal("the node stopped without a restore")
	case <-time.After(100 * time.Millisecond):
	}
	putRestore(t, tdb, restore.StateStopping, false, "stopping every node")
	select {
	case <-started:
	case <-time.After(3 * restoreWatchEvery):
		t.Fatal("the node kept serving after the restore began")
	}
	if !a.restoreBegan(ctx) {
		t.Fatal("a node that starts now would serve")
	}
}

// ErrRestoring is what Serve gives back, so the start loop waits instead of exiting.
func TestServeRefusesToServeDuringARestore(t *testing.T) {
	tdb := testutil.Database(t)
	putRestore(t, tdb, restore.StateRestoring, true, "")
	a := &App{pool: tdb.Pool, log: slog.New(slog.DiscardHandler)}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Serve(context.Background(), ln); !errors.Is(err, ErrRestoring) {
		t.Fatalf("Serve during a restore: %v", err)
	}
}
