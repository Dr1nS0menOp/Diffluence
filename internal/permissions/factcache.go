// SPDX-License-Identifier: Apache-2.0

package permissions

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/db"
)

// FactMaxAge is the longest a node keeps the facts of a person or a space between
// requests (ARC-231). The permission generation already ends every entry the moment a
// fact changes, on every node; this age only bounds how long a change that no trigger
// counts could go unnoticed.
const FactMaxAge = 30 * time.Second

// factCacheLimit bounds memory. Past it the cache starts again from empty.
const factCacheLimit = 20000

// factCache keeps facts between reads while permission_generation stays the same. Only
// requests with a memo come here, and those change no permission (WithMemo).
type factCache struct {
	mu     sync.Mutex
	gen    int64
	users  map[uuid.UUID]cachedUser
	spaces map[uuid.UUID]cachedSpace
	now    func() time.Time
	hits   atomic.Int64
	misses atomic.Int64
}

// cachedUser keeps what makes an account active, not the answer: an account past its
// end date stops being active at that moment, also while it sits here.
type cachedUser struct {
	facts   UserFacts
	status  string
	expires *time.Time
	at      time.Time
}

type cachedSpace struct {
	facts SpaceFacts
	at    time.Time
}

func newFactCache() *factCache {
	return &factCache{users: map[uuid.UUID]cachedUser{}, spaces: map[uuid.UUID]cachedSpace{}, now: time.Now}
}

// usable reports whether entries may be read or kept under gen. A newer generation
// empties the cache; an older one, seen by a request that started before a change,
// neither reads nor writes.
func (c *factCache) usable(gen int64) bool {
	if gen > c.gen {
		c.gen = gen
		clear(c.users)
		clear(c.spaces)
	}
	return gen == c.gen
}

func (c *factCache) fresh(at time.Time) bool { return c.now().Sub(at) < FactMaxAge }

func (c *factCache) user(gen int64, id uuid.UUID) (UserFacts, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.usable(gen) {
		return UserFacts{}, false
	}
	e, ok := c.users[id]
	if !ok || !c.fresh(e.at) {
		c.misses.Add(1)
		return UserFacts{}, false
	}
	c.hits.Add(1)
	u := e.facts
	u.Active = activeAt(e.status, e.expires, c.now())
	return u, true
}

func (c *factCache) keepUser(gen int64, u UserFacts, status string, expires *time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.usable(gen) {
		return
	}
	if len(c.users) >= factCacheLimit {
		clear(c.users)
	}
	c.users[u.ID] = cachedUser{facts: u, status: status, expires: expires, at: c.now()}
}

func (c *factCache) space(gen int64, id uuid.UUID) (SpaceFacts, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.usable(gen) {
		return SpaceFacts{}, false
	}
	e, ok := c.spaces[id]
	if !ok || !c.fresh(e.at) {
		c.misses.Add(1)
		return SpaceFacts{}, false
	}
	c.hits.Add(1)
	return e.facts, true
}

func (c *factCache) keepSpace(gen int64, f SpaceFacts) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.usable(gen) {
		return
	}
	if len(c.spaces) >= factCacheLimit {
		clear(c.spaces)
	}
	c.spaces[f.ID] = cachedSpace{facts: f, at: c.now()}
}

// activeAt is the rule of an active account, also used when the facts are first read.
func activeAt(status string, expires *time.Time, now time.Time) bool {
	return status == "active" && (expires == nil || expires.After(now))
}

// generation reads the permission generation once per request, through the memo. It is
// read before any fact, so facts kept under a number are never older than that number.
func (s *Service) generation(ctx context.Context, q db.Querier, m *memo) (int64, error) {
	if gen, ok := m.generation(); ok {
		return gen, nil
	}
	var gen int64
	if err := q.QueryRow(ctx, "SELECT n FROM permission_generation").Scan(&gen); err != nil {
		return 0, err
	}
	m.keepGeneration(gen)
	return gen, nil
}

// CacheCounts says how often the facts of a person or a space came from memory, for
// the measurements of spec 040.
func (s *Service) CacheCounts() (hits, misses int64) {
	return s.cache.hits.Load(), s.cache.misses.Load()
}
