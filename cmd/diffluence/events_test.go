// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/db"
)

// `events check` passes on a sound log and names the rule a broken one breaks.
func TestEventsCheckNamesTheFirstBreach(t *testing.T) {
	url := commandEnv(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"events", "check"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("a sound log failed: %d %s", code, errOut.String())
	}
	pool, err := db.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// A head ahead of an empty log is fine; a log that ends below the head is not.
	for _, sql := range []string{"UPDATE event_head SET pos = 5",
		"INSERT INTO event_log (pos, lane, id, type, payload, created_at) VALUES (3, 0, gen_random_uuid(), 'page.updated', '{}', now())"} {
		if _, err := pool.Exec(context.Background(), sql); err != nil {
			t.Fatal(err)
		}
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"events", "check"}, strings.NewReader(""), &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "L1") {
		t.Fatalf("a broken log: %d %q", code, errOut.String())
	}
}
