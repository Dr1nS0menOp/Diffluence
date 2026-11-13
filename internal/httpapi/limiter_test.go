// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"fmt"
	"testing"
	"time"
)

// One IPv6 network holds 2^64 addresses, so an address of such a network counts as its /64.
func TestAnIPv6NetworkCountsAsOneAddress(t *testing.T) {
	same := [][2]string{
		{"2001:db8:1:2::1", "2001:db8:1:2:ffff:ffff:ffff:ffff"},
		{"2001:db8:1:2::1", "2001:db8:1:2:abcd::9"},
		{"192.0.2.7", "::ffff:192.0.2.7"},
	}
	for _, pair := range same {
		if addressKey(pair[0]) != addressKey(pair[1]) {
			t.Errorf("%s and %s must count together: %q, %q", pair[0], pair[1], addressKey(pair[0]), addressKey(pair[1]))
		}
	}
	apart := [][2]string{
		{"2001:db8:1:2::1", "2001:db8:1:3::1"},
		{"192.0.2.7", "192.0.2.8"},
		{"", "192.0.2.8"},
	}
	for _, pair := range apart {
		if addressKey(pair[0]) == addressKey(pair[1]) {
			t.Errorf("%s and %s must count apart: %q", pair[0], pair[1], addressKey(pair[0]))
		}
	}
}

// An expired window goes when a later attempt comes, whatever the number of keys, so the
// memory follows the attempts of one window and no attempt walks every key.
func TestTheLimiterForgetsExpiredWindowsAsItGoes(t *testing.T) {
	l := newLimiter(2, time.Minute)
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }
	for i := range 100 {
		l.allow(fmt.Sprintf("old-%d", i))
	}
	now = now.Add(30 * time.Second)
	l.allow("middle")
	now = now.Add(31 * time.Second)
	l.allow("new")
	if len(l.hits) != 2 {
		t.Fatalf("keys kept after the old windows ended: %d, want middle and new", len(l.hits))
	}
	if !l.allow("middle") || l.allow("middle") {
		t.Fatal("a window still running keeps its count")
	}
}

// A key that was reset and counted again keeps its new window when its old one expires.
func TestAResetKeyKeepsItsNewWindow(t *testing.T) {
	l := newLimiter(1, time.Minute)
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }
	l.allow("k")
	l.reset("k")
	now = now.Add(30 * time.Second)
	l.allow("k")
	now = now.Add(31 * time.Second)
	l.allow("other")
	if l.allow("k") {
		t.Fatal("the new window of k was forgotten with its old one")
	}
}
