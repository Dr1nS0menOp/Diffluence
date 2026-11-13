// SPDX-License-Identifier: Apache-2.0

package httpapi

import "sync"

// collabSocketPath is the WebSocket of an editor. It stays open as long as the editor,
// so it never waits in the order of work and is not timed as a request: it holds no
// database connection, and it has a cap of its own below.
const collabSocketPath = "/collab"

// A node takes at most socketsPerNode open editors, and one login at most
// socketsPerLogin. A person edits a few pages at once, in a few tabs; the node cap bounds
// the memory and file handles of proxied sockets, which are cheap but not free.
const (
	socketsPerNode  = 512
	socketsPerLogin = 8
)

// socketCap counts open editing sockets, per node and per key (a login).
type socketCap struct {
	mu        sync.Mutex
	total     int
	perKey    map[string]int
	maxTotal  int
	maxPerKey int
}

func newSocketCap(maxTotal, maxPerKey int) *socketCap {
	return &socketCap{perKey: map[string]int{}, maxTotal: maxTotal, maxPerKey: maxPerKey}
}

// acquire takes a place for key, or says no when the node or the key is full.
func (c *socketCap) acquire(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.total >= c.maxTotal || c.perKey[key] >= c.maxPerKey {
		return false
	}
	c.total++
	c.perKey[key]++
	return true
}

// release gives back a place of key.
func (c *socketCap) release(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.perKey[key] == 0 {
		return
	}
	c.total--
	c.perKey[key]--
	if c.perKey[key] == 0 {
		delete(c.perKey, key)
	}
}
