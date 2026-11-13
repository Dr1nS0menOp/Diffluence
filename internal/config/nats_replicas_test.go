// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
)

// The embedded server is one node, so its stream keeps one copy (spec 063).
func TestTheEventStreamKeepsOneCopyByDefault(t *testing.T) {
	c, err := Load(env(map[string]string{"DIFFLUENCE_DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.NATSReplicas != 1 {
		t.Errorf("NATSReplicas = %d, want 1", c.NATSReplicas)
	}
}

// A cluster of three or five keeps that many copies, so losing one node loses no event.
func TestAClusterKeepsThreeOrFiveCopies(t *testing.T) {
	for _, n := range []string{"3", "5"} {
		c, err := Load(env(map[string]string{
			"DIFFLUENCE_DATABASE_URL": "postgres://x", "DIFFLUENCE_NATS_URL": "nats://a:4222,nats://b:4222,nats://c:4222",
			"DIFFLUENCE_NATS_REPLICAS": n,
		}))
		if err != nil {
			t.Fatalf("%s copies: %v", n, err)
		}
		if want := int(n[0] - '0'); c.NATSReplicas != want {
			t.Errorf("NATSReplicas = %d, want %d", c.NATSReplicas, want)
		}
	}
}

// Any other count is refused at start, with the reason, before a node touches the queue.
func TestOtherCopyCountsAreRefused(t *testing.T) {
	for _, n := range []string{"0", "2", "4", "7", "-1"} {
		_, err := Load(env(map[string]string{
			"DIFFLUENCE_DATABASE_URL": "postgres://x", "DIFFLUENCE_NATS_URL": "nats://a:4222",
			"DIFFLUENCE_NATS_REPLICAS": n,
		}))
		if err == nil || !strings.Contains(err.Error(), "NATS_REPLICAS") || !strings.Contains(err.Error(), "1, 3 or 5") {
			t.Errorf("%s copies: err = %v, want a refusal naming 1, 3 or 5", n, err)
		}
	}
}

// The embedded server cannot hold more than one copy, so asking for more is refused.
func TestTheEmbeddedServerRefusesMoreThanOneCopy(t *testing.T) {
	_, err := Load(env(map[string]string{"DIFFLUENCE_DATABASE_URL": "postgres://x", "DIFFLUENCE_NATS_REPLICAS": "3"}))
	if err == nil || !strings.Contains(err.Error(), "NATS_URL") {
		t.Errorf("err = %v, want a refusal that names NATS_URL", err)
	}
}
