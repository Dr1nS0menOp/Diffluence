// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/netip"
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
	// started lists the windows in the order they began, so the ones that ended are found
	// at its front: each attempt forgets those, and never walks every key.
	started []windowStart
	// first is the oldest window in started that may still run.
	first int
}

type windowCount struct {
	start time.Time
	n     int
}

type windowStart struct {
	key   string
	start time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, now: time.Now, hits: map[string]*windowCount{}}
}

// allow counts one attempt for key and reports whether it is within the limit.
func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.forgetEnded(now)
	c, ok := l.hits[key]
	if !ok || now.Sub(c.start) >= l.window {
		l.hits[key] = &windowCount{start: now, n: 1}
		l.started = append(l.started, windowStart{key: key, start: now})
		return true
	}
	c.n++
	return c.n <= l.max
}

// forgetEnded drops the windows that ended before now. A key that was reset, or began a
// new window since, keeps what it has now.
func (l *limiter) forgetEnded(now time.Time) {
	for l.first < len(l.started) && now.Sub(l.started[l.first].start) >= l.window {
		w := l.started[l.first]
		if c, ok := l.hits[w.key]; ok && c.start.Equal(w.start) {
			delete(l.hits, w.key)
		}
		l.started[l.first] = windowStart{}
		l.first++
	}
	// Moving what is left to the front once half is gone keeps the list from growing for
	// ever, at a cost spread over the attempts that filled it.
	if l.first > len(l.started)/2 {
		l.started = append(l.started[:0], l.started[l.first:]...)
		l.first = 0
	}
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

// ipv6NetworkBits is the block one network gets in IPv6 (RFC 6177). Its 2^64 addresses
// would otherwise each count as a separate address.
const ipv6NetworkBits = 64

// addressKey is what the limits count a client address by: an IPv4 address itself, and an
// IPv6 address by its network. Something that is not an address counts as itself.
func addressKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "ip:" + ip
	}
	addr = addr.Unmap().WithZone("")
	if !addr.Is6() {
		return "ip:" + addr.String()
	}
	// Prefix fails only for a length past the address, and 64 fits every IPv6 address.
	network, _ := addr.Prefix(ipv6NetworkBits)
	return "ip:" + network.String()
}
