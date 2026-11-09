// SPDX-License-Identifier: Apache-2.0

package permissions

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// memo keeps the facts of one request, so a page view that checks five actions loads the
// person and the space once instead of five times (ARC-197). It lives exactly as long as
// the request: a permission change is seen by the next request.
type memo struct {
	mu     sync.Mutex
	users  map[uuid.UUID]UserFacts
	spaces map[uuid.UUID]SpaceFacts
	// gen is the permission generation this request saw first (ARC-231).
	gen    int64
	hasGen bool
}

type memoKey struct{}

// WithMemo returns a context in which facts, once loaded, are reused. Use it only for a
// request that changes no permission, such as a read: within a write, a grant made a
// moment ago must count.
func WithMemo(ctx context.Context) context.Context {
	return context.WithValue(ctx, memoKey{}, &memo{users: map[uuid.UUID]UserFacts{}, spaces: map[uuid.UUID]SpaceFacts{}})
}

// Memoized reports whether facts are reused in this context.
func Memoized(ctx context.Context) bool {
	_, ok := ctx.Value(memoKey{}).(*memo)
	return ok
}

func memoOf(ctx context.Context) *memo {
	m, _ := ctx.Value(memoKey{}).(*memo)
	return m
}

func (m *memo) user(id uuid.UUID) (UserFacts, bool) {
	if m == nil {
		return UserFacts{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[id]
	return u, ok
}

func (m *memo) keepUser(u UserFacts) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[u.ID] = u
}

func (m *memo) generation() (int64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.gen, m.hasGen
}

func (m *memo) keepGeneration(gen int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gen, m.hasGen = gen, true
}

func (m *memo) space(id uuid.UUID) (SpaceFacts, bool) {
	if m == nil {
		return SpaceFacts{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.spaces[id]
	return s, ok
}

func (m *memo) keepSpace(s SpaceFacts) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spaces[s.ID] = s
}
