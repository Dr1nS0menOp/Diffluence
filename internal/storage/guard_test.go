// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/apperr"
)

// roomStore is a disk whose room a test sets, and that counts how often it was asked.
type roomStore struct {
	*Disk
	free, total uint64
	err         error
	asked       int
}

func (r *roomStore) Space(context.Context) (uint64, uint64, error) {
	r.asked++
	return r.free, r.total, r.err
}

func newRoomStore(t *testing.T, free, total uint64) *roomStore {
	t.Helper()
	d, err := NewDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &roomStore{Disk: d, free: free, total: total}
}

func put(g *Guarded) error {
	_, err := g.Put(context.Background(), "attachments/a", strings.NewReader("hello"), 100)
	return err
}

func TestAStoreBelowTwoPercentRefusesFilesWithNoRoom(t *testing.T) {
	inner := newRoomStore(t, 19, 1000)
	err := put(NewGuarded(inner))
	e, ok := apperr.As(err)
	if !ok || e.Kind != apperr.KindNoRoom {
		t.Fatalf("got %v, want no_room", err)
	}
	if _, err := inner.Open(context.Background(), "attachments/a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a refused file was stored: %v", err)
	}
}

func TestAStoreAtTwoPercentStillTakesFiles(t *testing.T) {
	if err := put(NewGuarded(newRoomStore(t, 20, 1000))); err != nil {
		t.Fatal(err)
	}
}

func TestAStoreThatCannotTellItsRoomTakesFiles(t *testing.T) {
	inner := newRoomStore(t, 0, 0)
	inner.err = errors.New("no statfs here")
	if err := put(NewGuarded(inner)); err != nil {
		t.Fatal(err)
	}
}

func TestAStoreWithoutASpacerTakesFiles(t *testing.T) {
	d, err := NewDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := put(NewGuarded(plainStore{d})); err != nil {
		t.Fatal(err)
	}
}

// plainStore hides the Spacer of a disk, as object storage would have none.
type plainStore struct{ Store }

func TestTheRoomIsReadAtMostEveryThirtySeconds(t *testing.T) {
	inner := newRoomStore(t, 500, 1000)
	g := NewGuarded(inner)
	clock := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	g.now = func() time.Time { return clock }
	for range 3 {
		if err := put(g); err != nil {
			t.Fatal(err)
		}
	}
	if inner.asked != 1 {
		t.Fatalf("asked %d times within 30 seconds", inner.asked)
	}
	inner.free = 1
	clock = clock.Add(roomFreshFor - time.Second)
	if err := put(g); err != nil {
		t.Fatalf("a fresh answer was not reused: %v", err)
	}
	clock = clock.Add(2 * time.Second)
	if err := put(g); err == nil {
		t.Fatal("an answer older than 30 seconds was reused")
	}
	if inner.asked != 2 {
		t.Fatalf("asked %d times", inner.asked)
	}
}

func TestTheGuardPassesOnTheRoomAndTheRestOfTheStore(t *testing.T) {
	inner := newRoomStore(t, 300, 1000)
	g := NewGuarded(inner)
	free, total, err := g.Space(context.Background())
	if err != nil || free != 300 || total != 1000 {
		t.Fatalf("space: %d %d %v", free, total, err)
	}
	if err := put(g); err != nil {
		t.Fatal(err)
	}
	rc, err := g.Open(context.Background(), "attachments/a")
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()
	if list, err := g.List(context.Background()); err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
	if err := g.Delete(context.Background(), "attachments/a"); err != nil {
		t.Fatal(err)
	}
	if err := g.Healthy(context.Background()); err != nil {
		t.Fatal(err)
	}
}
