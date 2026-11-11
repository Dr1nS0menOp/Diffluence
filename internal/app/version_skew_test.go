// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
)

// During an upgrade old and new nodes share one database (docs/operations/version-skew.md).
// What one release writes, the other must leave alone or read, never fail on (OPS-070).

// A job a newer release queued waits for a node that knows its kind, instead of failing on
// an older worker that happened to look first.
func TestAJobOfANewerReleaseWaitsForANodeThatKnowsIt(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	id := uuid.New()
	if _, err := e.app.Pool().Exec(ctx, `INSERT INTO jobs (id, kind, status, created_by, created_at)
		VALUES ($1, 'from.a.newer.release', 'queued', $2, now() - interval '1 hour')`, id, uuid.New()); err != nil {
		t.Fatal(err)
	}
	for range 50 {
		ran, err := e.app.Jobs().RunOnce(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !ran {
			break
		}
	}
	var status string
	if err := e.app.Pool().QueryRow(ctx, "SELECT status FROM jobs WHERE id = $1", id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("a job of an unknown kind was taken and is now %s", status)
	}
}

// Every listener of the queue gets every event. One from a newer release, of a type this
// release does not know or with a field it does not know, must pass without an error, or
// it would be retried and set aside as a failure.
func TestEveryListenerLetsAnEventOfANewerReleasePass(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	payloads := map[string]string{
		"an unknown type":           `{"page_id":"` + uuid.NewString() + `","space_id":"` + uuid.NewString() + `"}`,
		"an unknown type, no field": `{}`,
	}
	for name, payload := range payloads {
		msg, err := json.Marshal(outbox.Message{ID: uuid.New(), Type: "future.thing_happened", CreatedAt: time.Now(),
			Payload: json.RawMessage(payload)})
		if err != nil {
			t.Fatal(err)
		}
		for listener, handle := range e.app.EventListenersForTest() {
			if err := handle(ctx, msg); err != nil {
				t.Errorf("%s: %s refused it: %v", name, listener, err)
			}
		}
	}
}

// A section of settings saved by this release keeps the fields it does not know, so a node
// of the previous release does not erase what a newer one stored (OPS-070).
func TestSavingSettingsKeepsFieldsOfANewerRelease(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admin := e.user("admin@example.com", identity.PermAdministrator)
	body := admin.ok("GET", "/api/v1/admin/settings/features", nil)
	admin.ok("PUT", "/api/v1/admin/settings/features", body)
	if _, err := e.app.Pool().Exec(ctx, `UPDATE settings SET value = value || '{"from_a_newer_release": "kept"}'
		WHERE key = 'section.features'`); err != nil {
		t.Fatal(err)
	}
	body["presence"] = false
	admin.ok("PUT", "/api/v1/admin/settings/features", body)
	var kept *string
	var presence bool
	if err := e.app.Pool().QueryRow(ctx, `SELECT value->>'from_a_newer_release', (value->>'presence')::boolean
		FROM settings WHERE key = 'section.features'`).Scan(&kept, &presence); err != nil {
		t.Fatal(err)
	}
	if kept == nil || *kept != "kept" {
		t.Fatalf("the newer field was dropped: %v", kept)
	}
	if presence {
		t.Fatal("the change itself was not saved")
	}
}
