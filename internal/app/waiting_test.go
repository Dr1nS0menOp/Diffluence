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
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var refusedForBackup = &UpgradeRefused{Problems: []UpgradeProblem{
	{ProblemBackup, "the last backup is from 2026-11-08T11:00:00Z, longer ago than 24h0m0s"},
	{ProblemDisk, "the data folder has 300 MiB free and needs 512 MiB"},
}}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// A waiting node must not look healthy to a load balancer, nor dead to Compose.
func TestAWaitingNodeIsAliveButNotReady(t *testing.T) {
	h := waitingHandler(refusedForBackup)
	if rec := get(t, h, "/livez"); rec.Code != http.StatusOK {
		t.Fatalf("/livez %d", rec.Code)
	}
	rec := get(t, h, "/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/readyz %d", rec.Code)
	}
	var body struct {
		Status   string   `json:"status"`
		Problems []string `json:"problems"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "waiting_for_upgrade" || strings.Join(body.Problems, ",") != "backup,disk" {
		t.Fatalf("got %+v", body)
	}
}

func TestTheAPIOfAWaitingNodeSaysToTryAgain(t *testing.T) {
	rec := get(t, waitingHandler(refusedForBackup), "/api/v1/pages/x")
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") != waitingRetryAfter {
		t.Fatalf("%d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if !strings.Contains(rec.Body.String(), `"code":"upgrade_waiting"`) {
		t.Fatalf("got %s", rec.Body.String())
	}
}

// The page is public: it names the kind of problem and the way out, never the figures.
func TestTheWaitingPageNamesTheFixButNoDetails(t *testing.T) {
	rec := get(t, waitingHandler(refusedForBackup), "/spaces/ENG")
	page := rec.Body.String()
	if rec.Code != http.StatusServiceUnavailable || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("%d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, want := range []string{"No recent backup was noted", "docker compose run --rm backup once", "migrate --without-backup", "nearly full"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	for _, secret := range []string{"2026-11-08", "300 MiB"} {
		if strings.Contains(page, secret) {
			t.Errorf("the public page shows %q", secret)
		}
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("CSP %q", csp)
	}
}

// A node that waits for the backup service to migrate says where the migration runs, and
// the backup service's command for an upgrade without a backup (spec 070).
func TestTheWaitingPageNamesTheBackupServiceThatMigrates(t *testing.T) {
	refused := &UpgradeRefused{Problems: []UpgradeProblem{
		{ProblemBackup, "no backup was noted"}, {ProblemMigrations, "3 migration(s) of this release have not run"}}}
	page := get(t, waitingHandler(refused), "/").Body.String()
	for _, want := range []string{"waits for the backup service to migrate", "docker compose up -d --build backup",
		"docker compose logs backup", "--entrypoint diffluence backup migrate --without-backup"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if refused.RetryAfter() != RetryUpgradeEvery {
		t.Errorf("a node that also waits for a backup looks again after %s", refused.RetryAfter())
	}
}

// A retry lets go of the address, so the real server can take it straight after.
func TestAWaitingNodeFreesItsAddressWhenItTriesAgain(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	retry := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- waitOn(context.Background(), ln, refusedForBackup, retry, slog.New(slog.DiscardHandler))
	}()
	resp, err := http.Get("http://" + addr + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("/readyz %d", resp.StatusCode)
	}
	close(retry)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a retry is no failure: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the waiting node did not stop for the retry")
	}
	again, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("the address stayed taken: %v", err)
	}
	_ = again.Close()
}

func TestAStoppedWaitingNodeSaysItWasStopped(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = waitOn(ctx, ln, refusedForBackup, make(chan struct{}), slog.New(slog.DiscardHandler))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}
