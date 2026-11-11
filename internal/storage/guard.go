// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/health"
)

// roomFreshFor is how long one reading of the free space is trusted (spec 051 load
// budget): one statfs per node per 30 seconds, and nothing per upload.
const roomFreshFor = 30 * time.Second

// errNoRoom is what every refused file gets. It names what still works, so a person
// knows their text is safe.
var errNoRoom = apperr.NoRoom("The server is almost out of disk space, so files cannot be added right now. " +
	"Text still saves. An administrator needs to make room.")

// Guarded refuses new files when the disk below is nearly full (OPS-104, spec 051 D4).
// Every upload, import, export, preview, picture, logo and font goes through Put, so one
// guard covers them all, while text, which lives in the database, goes on.
type Guarded struct {
	inner Store
	now   func() time.Time

	mu      sync.Mutex
	room    Spacer // nil = the store does not know its room
	readAt  time.Time
	refuses bool
}

// NewGuarded wraps a store. A store that does not know its room is never refused.
func NewGuarded(inner Store) *Guarded {
	room, _ := inner.(Spacer)
	return &Guarded{inner: inner, now: time.Now, room: room}
}

// UseRoomOf reads the room from s from now on, and forgets the last reading. Tests use it
// to stand in for a disk that is nearly full.
func (g *Guarded) UseRoomOf(s Spacer) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.room, g.readAt = s, time.Time{}
}

// Refusing says whether files are refused now, reading the room when the last reading
// is stale. A reading that fails counts as room enough. /readyz asks it too, so it never
// waits for the database.
func (g *Guarded) Refusing(ctx context.Context) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.room == nil {
		return false
	}
	if !g.readAt.IsZero() && g.now().Sub(g.readAt) < roomFreshFor {
		return g.refuses
	}
	free, total, err := g.room.Space(ctx)
	g.readAt = g.now()
	g.refuses = err == nil && health.RefusesLargeWrites(free, total)
	return g.refuses
}

// Put stores r, unless the disk is nearly full.
func (g *Guarded) Put(ctx context.Context, key string, r io.Reader, maxBytes int64) (Object, error) {
	if g.Refusing(ctx) {
		return Object{}, errNoRoom
	}
	return g.inner.Put(ctx, key, r, maxBytes)
}

// Open reads an object; reading never needs room.
func (g *Guarded) Open(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	return g.inner.Open(ctx, key)
}

// Delete removes an object; removing makes room, so it is never refused.
func (g *Guarded) Delete(ctx context.Context, key string) error { return g.inner.Delete(ctx, key) }

// List names the stored objects.
func (g *Guarded) List(ctx context.Context) ([]Listed, error) { return g.inner.List(ctx) }

// Healthy checks the store below.
func (g *Guarded) Healthy(ctx context.Context) error { return g.inner.Healthy(ctx) }

// Space passes on the room the guard reads, when it knows it.
func (g *Guarded) Space(ctx context.Context) (free, total uint64, err error) {
	g.mu.Lock()
	room := g.room
	g.mu.Unlock()
	if room == nil {
		return 0, 0, errors.New("storage: the store does not know its room")
	}
	return room.Space(ctx)
}
