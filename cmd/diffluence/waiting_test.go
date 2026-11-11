// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/app"
)

var refused = &app.UpgradeRefused{Problems: []app.UpgradeProblem{{Kind: app.ProblemBackup, Text: "no backup was noted"}}}

// backup.sh sends SIGHUP after noting a backup, so a waiting node tries again at once
// instead of after the minute.
func TestAWaitingNodeTriesAgainOnSIGHUP(t *testing.T) {
	hup := make(chan os.Signal, 1)
	hup <- syscall.SIGHUP
	done := make(chan error, 1)
	go func() {
		done <- waitBeforeUpgrade(context.Background(), "127.0.0.1:0", refused, hup, slog.New(slog.DiscardHandler))
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a retry is no failure: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("SIGHUP did not end the wait")
	}
}

func TestAWaitingNodeStoppedBySIGTERMExitsCleanly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitBeforeUpgrade(ctx, "127.0.0.1:0", refused, make(chan os.Signal), slog.New(slog.DiscardHandler))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if stoppedCleanly(err) != nil {
		t.Fatal("a stop is reported as a failure")
	}
	if boom := errors.New("listen failed"); !errors.Is(stoppedCleanly(boom), boom) {
		t.Fatal("a real failure is hidden")
	}
}
