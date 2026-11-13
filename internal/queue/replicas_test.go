// SPDX-License-Identifier: Apache-2.0

package queue

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

// A node raises the copies of an existing stream to its setting, and never lowers them by
// itself: one node started with the setting left out must not take copies away from a
// cluster that has them (spec 063).
func TestTheStreamGetsMoreCopiesButNeverFewer(t *testing.T) {
	cases := []struct{ existing, configured, want int }{
		{0, 1, 1}, // a new stream
		{0, 3, 3},
		{1, 3, 3},
		{3, 3, 3},
		{3, 1, 3},
		{5, 3, 5},
	}
	for _, c := range cases {
		if got := streamReplicas(c.existing, c.configured); got != c.want {
			t.Errorf("existing %d, configured %d: got %d, want %d", c.existing, c.configured, got, c.want)
		}
	}
}

// The embedded server keeps one copy of the stream.
func TestTheEmbeddedStreamHasOneCopy(t *testing.T) {
	q, err := Open(context.Background(), "", t.TempDir(), 1, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	info, err := q.js.Stream(context.Background(), StreamEvents)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.CachedInfo().Config.Replicas; got != 1 {
		t.Errorf("replicas = %d, want 1", got)
	}
}

// Asking one server for three copies fails at start with a reason that says so, instead
// of a node that runs without the copies it was told to keep.
func TestOneServerRefusesThreeCopies(t *testing.T) {
	q, err := Open(context.Background(), "", t.TempDir(), 3, slog.New(slog.DiscardHandler))
	if err == nil {
		q.Close()
		t.Fatal("one server accepted three copies")
	}
	if !strings.Contains(err.Error(), "3 copies") {
		t.Errorf("err = %v, want it to say how many copies were asked for", err)
	}
}
