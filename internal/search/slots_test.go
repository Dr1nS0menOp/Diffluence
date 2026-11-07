// SPDX-License-Identifier: Apache-2.0

package search

import (
	"context"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/apperr"
)

func TestASearchThatFindsNoSlotHearsThatItIsBusy(t *testing.T) {
	s := &Service{slots: make(chan struct{}, 1), patience: time.Millisecond}
	if err := s.enter(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := s.enter(context.Background())
	if e, ok := apperr.As(err); !ok || e.Kind != apperr.KindBusy {
		t.Fatalf("the second search: %v", err)
	}
	s.leave()
	if err := s.enter(context.Background()); err != nil {
		t.Fatalf("after the first left: %v", err)
	}
}

func TestASearchThatIsCancelledStopsWaiting(t *testing.T) {
	s := &Service{slots: make(chan struct{}, 1), patience: time.Hour}
	if err := s.enter(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.enter(ctx); err == nil {
		t.Fatal("a cancelled search kept waiting")
	}
}
