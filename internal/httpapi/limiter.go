// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"sync"
	"time"
)

// limiter is a fixed-window counter per key, held in memory per node.
// Good enough for brute-force protection on one node; in HA each node limits
// on its own (a shared limiter can come later if needed).
type limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	now    func() time.Time
	hits   map[string]*windowCount
}

type windowCount struct {
	start time.Time
	n     int
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, now: time.Now, hits: map[string]*windowCount{}}
}

// allow counts one attempt for key and reports whether it is within the limit.
func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.hits) > 50_000 {
		for k, c := range l.hits {
			if now.Sub(c.start) >= l.window {
				delete(l.hits, k)
			}
		}
	}
	c, ok := l.hits[key]
	if !ok || now.Sub(c.start) >= l.window {
		l.hits[key] = &windowCount{start: now, n: 1}
		return true
	}
	c.n++
	return c.n <= l.max
}

// giveBack returns one attempt, for an attempt that turned out to be fine.
func (l *limiter) giveBack(key string) {
	l.mu.Lock()
	if c, ok := l.hits[key]; ok && c.n > 0 {
		c.n--
	}
	l.mu.Unlock()
}

// reset forgets key (after a successful login).
func (l *limiter) reset(key string) {
	l.mu.Lock()
	delete(l.hits, key)
	l.mu.Unlock()
}
