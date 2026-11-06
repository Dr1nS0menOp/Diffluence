// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/failures"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/jobs"
)

func TestAnEventGivenUpOnIsShownAndCanBeReplayed(t *testing.T) {
	e := newEnv(t)
	admin := e.user("root@example.com", identity.PermUseProduct, identity.PermAdministrator)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	ctx := context.Background()
	if err := e.app.Failures().Record(ctx, "notifications", []byte(`{"no":"event"}`), errors.New("broke")); err != nil {
		t.Fatal(err)
	}
	reader.want(403, "GET", "/api/v1/admin/event-failures", nil)
	list := admin.ok("GET", "/api/v1/admin/event-failures", nil)["failures"].([]any)
	if len(list) != 1 {
		t.Fatalf("%v", list)
	}
	item := list[0].(map[string]any)
	if item["consumer"] != "notifications" || item["error"] != "broke" {
		t.Fatalf("%v", item)
	}
	if !strings.Contains(admin.want(200, "GET", "/api/v1/admin/attention", nil), "events_failed") {
		t.Fatal("no administrator hears of it")
	}
	id := item["id"].(string)
	reader.want(403, "POST", "/api/v1/admin/event-failures/"+id+"/replay", nil)
	admin.want(204, "POST", "/api/v1/admin/event-failures/"+id+"/replay", nil)
	if e.auditActions("event.replay_requested") != 1 || e.outboxCount("event.replay_requested") != 1 {
		t.Fatal("not recorded")
	}

	var ran []string
	handlers := map[string]failures.Handler{"notifications": func(_ context.Context, data []byte) error {
		ran = append(ran, string(data))
		return nil
	}}
	if err := e.app.Failures().RunReplays(ctx, handlers); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 1 || ran[0] != `{"no":"event"}` {
		t.Fatalf("%v", ran)
	}
	if left := admin.ok("GET", "/api/v1/admin/event-failures", nil)["failures"].([]any); len(left) != 0 {
		t.Fatalf("still listed: %v", left)
	}
	if err := e.app.Failures().RunReplays(ctx, handlers); err != nil || len(ran) != 1 {
		t.Fatalf("ran twice: %v %v", err, ran)
	}
	admin.want(404, "POST", "/api/v1/admin/event-failures/"+id+"/replay", nil)
}

func TestAReplayThatFailsStaysWithItsNewError(t *testing.T) {
	e := newEnv(t)
	admin := e.user("root@example.com", identity.PermUseProduct, identity.PermAdministrator)
	ctx := context.Background()
	if err := e.app.Failures().Record(ctx, "notifications", []byte("x"), errors.New("first")); err != nil {
		t.Fatal(err)
	}
	id := admin.ok("GET", "/api/v1/admin/event-failures", nil)["failures"].([]any)[0].(map[string]any)["id"].(string)
	admin.want(204, "POST", "/api/v1/admin/event-failures/"+id+"/replay", nil)
	handlers := map[string]failures.Handler{"notifications": func(context.Context, []byte) error { return errors.New("second") }}
	if err := e.app.Failures().RunReplays(ctx, handlers); err != nil {
		t.Fatal(err)
	}
	item := admin.ok("GET", "/api/v1/admin/event-failures", nil)["failures"].([]any)[0].(map[string]any)
	if item["error"] != "second" || item["replay_requested_at"] != nil {
		t.Fatalf("%v", item)
	}
}

func TestOldFailuresArePurged(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.app.Failures().Record(ctx, "notifications", []byte("x"), errors.New("old")); err != nil {
		t.Fatal(err)
	}
	e.sql("UPDATE event_failures SET failed_at = now() - interval '31 days'")
	if err := e.app.Failures().Purge(ctx); err != nil {
		t.Fatal(err)
	}
	if n := e.count("SELECT count(*) FROM event_failures"); n != 0 {
		t.Fatal("kept")
	}
}

func TestEventsWaitingForADayAreShownToAnAdministrator(t *testing.T) {
	e := newEnv(t)
	admin := e.user("root@example.com", identity.PermUseProduct, identity.PermAdministrator, identity.PermCreateSpace)
	admin.space("ENG")
	e.sql("UPDATE outbox_events SET created_at = now() - interval '2 days', published_at = NULL")
	if !strings.Contains(admin.want(200, "GET", "/api/v1/admin/attention", nil), "events_waiting") {
		t.Fatal("no warning")
	}
}

func enqueue(t *testing.T, e *env, kind string) {
	t.Helper()
	ctx := context.Background()
	err := pgx.BeginFunc(ctx, e.app.Pool(), func(tx pgx.Tx) error {
		_, err := e.app.Jobs().Enqueue(ctx, tx, kind, map[string]any{}, uuid.Nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLongJobsLeaveRoomForAShortOne(t *testing.T) {
	e := newEnv(t)
	release := make(chan struct{})
	defer close(release)
	started := make(chan struct{}, 8)
	e.app.Jobs().RegisterLong("test.long", func(context.Context, jobs.Job) (any, error) {
		started <- struct{}{}
		<-release
		return nil, nil
	})
	finished := make(chan struct{})
	e.app.Jobs().Register("test.short", func(context.Context, jobs.Job) (any, error) {
		close(finished)
		return nil, nil
	})
	for range 6 {
		enqueue(t, e, "test.long")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.app.Jobs().Run(ctx)
	for range 3 {
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			t.Fatal("the long jobs did not start side by side")
		}
	}
	enqueue(t, e, "test.short")
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("a short job waited for long ones")
	}
	select {
	case <-started:
		t.Fatal("a long job took the short slot")
	case <-time.After(1500 * time.Millisecond):
	}
}

func TestASlowTaskHoldsUpNoOtherTask(t *testing.T) {
	e := newEnv(t)
	release := make(chan struct{})
	defer close(release)
	done := make(chan struct{})
	tasks := []jobs.Task{
		{Name: "test-slow", Every: time.Hour, Run: func(context.Context) error { <-release; return nil }},
		{Name: "test-quick", Every: time.Hour, Run: func(context.Context) error { close(done); return nil }},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.app.Jobs().RunPeriodic(ctx, tasks)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the quick task waited")
	}
}

func TestPresenceIsNotWrittenToTheLog(t *testing.T) {
	e := newEnv(t)
	if n := e.count("SELECT count(*) FROM pg_class WHERE relname = 'page_presence' AND relpersistence = 'u'"); n != 1 {
		t.Fatal("logged")
	}
}
