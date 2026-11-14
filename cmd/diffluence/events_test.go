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

// An operator switches the events to PostgreSQL and back, and reads where the switch stands;
// a node of an older release holds the switch up, by name (spec 068, 2.11).
func TestEventsSwitchAndStatus(t *testing.T) {
	url := commandEnv(t)
	pool, err := db.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	events := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := run(append([]string{"events"}, args...), strings.NewReader(""), &out, &errOut)
		return code, out.String(), errOut.String()
	}
	if code, out, _ := events("status"); code != 0 || !strings.Contains(out, "transport: nats") ||
		!strings.Contains(out, "release without NATS: not yet") {
		t.Fatalf("status on NATS: %d %q", code, out)
	}
	if code, _, errOut := events("switch", "kafka"); code != 2 || !strings.Contains(errOut, "Usage") {
		t.Fatalf("switch to kafka: %d %q", code, errOut)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO nodes (id, role, version, max_migration)
		VALUES (gen_random_uuid(), 'worker', '0.9.0', 52)`); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := events("switch", "postgres"); code != 1 || !strings.Contains(errOut, "0.9.0") {
		t.Fatalf("switch beside an older node: %d %q", code, errOut)
	}
	if code, out, _ := events("status"); code != 0 || !strings.Contains(out, "0.9.0") {
		t.Fatalf("status names no older node: %q", out)
	}
	if _, err := pool.Exec(context.Background(), "DELETE FROM nodes"); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := events("switch", "postgres"); code != 0 || !strings.Contains(out, "switched to postgres") {
		t.Fatalf("switch: %d %q %q", code, out, errOut)
	}
	if code, out, _ := events("status"); code != 0 || !strings.Contains(out, "transport: postgres") ||
		!strings.Contains(out, "NATS: not drained yet") {
		t.Fatalf("status after the switch: %q", out)
	}
	if code, out, _ := events("switch", "nats"); code != 0 || !strings.Contains(out, "switched to nats") {
		t.Fatalf("switch back: %d %q", code, out)
	}
}
