// SPDX-License-Identifier: Apache-2.0

package permissions

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAKeptAccountEndsAtItsEndDate(t *testing.T) {
	c := newFactCache()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	end := now.Add(10 * time.Second)
	id := uuid.New()
	c.keepUser(1, UserFacts{ID: id, Type: "member", Active: true}, "active", &end)
	if u, ok := c.user(1, id); !ok || !u.Active {
		t.Fatalf("before its end: %+v %v", u, ok)
	}
	now = now.Add(15 * time.Second) // past the end, still young enough to be kept
	if u, ok := c.user(1, id); !ok || u.Active {
		t.Fatalf("an account past its end date stayed active while kept: %+v %v", u, ok)
	}
}

func TestKeptFactsEndWithTheirGenerationAndTheirAge(t *testing.T) {
	c := newFactCache()
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	id := uuid.New()
	c.keepSpace(5, SpaceFacts{ID: id, Status: "active"})
	if _, ok := c.space(4, id); ok {
		t.Fatal("a request from before a change read what was kept after it")
	}
	if _, ok := c.space(6, id); ok {
		t.Fatal("a newer generation still read what was kept before it")
	}
	c.keepSpace(5, SpaceFacts{ID: id, Status: "active"})
	if _, ok := c.space(6, id); ok {
		t.Fatal("an older request kept facts under a newer generation")
	}
	c.keepSpace(6, SpaceFacts{ID: id, Status: "active"})
	now = now.Add(FactMaxAge)
	if _, ok := c.space(6, id); ok {
		t.Fatal("facts outlived their age")
	}
}
