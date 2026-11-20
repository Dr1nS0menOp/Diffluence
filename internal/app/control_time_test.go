// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kenwerk/kenwerk/internal/notify"
)

// Document control, wave DC5 of spec 064: due dates, reminders, escalation and the due job.
// The refusals come first: a time-out never approves, the job never acts twice, and a clock
// that goes back never repeats a reminder.

// dueDef is reviewedDef with a due period on its review state. late adds the step the system
// takes when the date passes; escalate names who is added then.
func dueDef(rv map[string]any, days, before int, late string, escalate ...map[string]any) map[string]any {
	due := map[string]any{"working_days": days, "remind_before": before}
	if len(escalate) > 0 {
		list := []any{}
		for _, e := range escalate {
			list = append(list, e)
		}
		due["escalate"] = list
	}
	rv["due"] = due
	d := reviewedDef(rv)
	if late != "" {
		d["transitions"] = append(d["transitions"].([]any),
			map[string]any{"id": "late", "name": "Too late", "from": "review", "to": late, "outcome": "overdue"})
	}
	return d
}

// passDue makes every moment of a page's deadline lie in the past, as if its time came.
func passDue(e *env, page string) {
	e.t.Helper()
	e.sql(`UPDATE review_deadlines SET remind_at = now() - interval '2 hours', due_at = now() - interval '1 hour',
		next_at = LEAST(next_at, now() - interval '1 hour') WHERE page_id = $1 AND next_at IS NOT NULL`, page)
}

// passReminder makes only the reminder before the date come.
func passReminder(e *env, page string) {
	e.t.Helper()
	e.sql(`UPDATE review_deadlines SET remind_at = now() - interval '1 minute', next_at = now() - interval '1 minute'
		WHERE page_id = $1`, page)
}

func (e *env) dueJob() {
	e.t.Helper()
	e.runTask("control-due")
}

// TestATimeOutNeverApproves is DOC-105 and DOC-1401: the validator refuses an overdue step
// into approval, and a definition that holds one anyway, written past the validator, is not
// taken by the job either. The page stays in review, readers see nothing, and nothing approves.
func TestATimeOutNeverApproves(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DTA")
	bad := dueDef(review("all", 0, entry("user", w.r1.id)), 2, 0, "approved")
	out := w.admin.want(409, "POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": "Bad", "definition": bad})
	if !strings.Contains(out, "timeout_never_approves") {
		t.Fatal(out)
	}
	good := w.workflowOf("Late to draft", dueDef(review("all", 0, entry("user", w.r1.id)), 2, 0, "draft"))
	page := w.reviewedPage(good, "Timed out")
	// Someone with the database changes the stored definition so that the time-out approves.
	raw, err := json.Marshal(bad)
	if err != nil {
		t.Fatal(err)
	}
	e.sql("UPDATE workflow_versions SET definition = $2 WHERE workflow_id = $1", good, string(raw))
	passDue(e, page)
	e.dueJob()
	if w.state(page) == "approved" || w.control(w.author, page)["approved_version"] != nil {
		t.Fatal("a time-out approved a page")
	}
	if seen := w.seenBy(w.reader, page, "Timed"); seen["page"] {
		t.Fatal("a reader sees a page that a time-out touched")
	}
	if e.count("SELECT count(*) FROM workflow_answers WHERE page_id = $1 AND cause = 'overdue'", page) != 0 {
		t.Fatal("the job took a step into approval")
	}
	// With the definition as saved, the time-out takes the page back to draft, as the system.
	e.sql("UPDATE workflow_versions SET definition = (SELECT definition FROM workflow_versions WHERE workflow_id = $1 AND version = 1) WHERE workflow_id = $1", good)
	fixed, err := json.Marshal(dueDef(review("all", 0, entry("user", w.r1.id)), 2, 0, "draft"))
	if err != nil {
		t.Fatal(err)
	}
	e.sql("UPDATE workflow_versions SET definition = $2 WHERE workflow_id = $1", good, string(fixed))
	e.sql("UPDATE review_deadlines SET next_at = now() - interval '1 minute' WHERE page_id = $1", page)
	e.dueJob()
	if w.state(page) != "draft" {
		t.Fatalf("the time-out did not take its step: %s", w.state(page))
	}
	if e.count("SELECT count(*) FROM workflow_answers WHERE page_id = $1 AND cause = 'overdue' AND user_id IS NULL", page) != 1 {
		t.Fatal("the time-out is not recorded as the system's")
	}
	if e.auditActions("page.review_overdue") != 1 {
		t.Fatal("the time-out is not audited")
	}
}

// TestTheDueJobOnTwoWorkersActsOnceForEachRound is DOC-1232: two workers run the job at the
// same moment on the same due rounds, and each round is reminded once.
func TestTheDueJobOnTwoWorkersActsOnceForEachRound(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DTB")
	flow := w.workflowOf("Due", dueDef(review("any", 0, entry("user", w.r1.id)), 5, 2, ""))
	var pages []string
	for _, title := range []string{"One", "Two", "Three", "Four", "Five", "Six"} {
		page := w.reviewedPage(flow, title)
		passReminder(e, page)
		pages = append(pages, page)
	}
	var task func(context.Context) error
	for _, tk := range e.app.PeriodicTasks() {
		if tk.Name == "control-due" {
			task = tk.Run
		}
	}
	if task == nil {
		t.Fatal("no due job")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- task(context.Background())
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, page := range pages {
		if n := e.count("SELECT reminders FROM review_deadlines WHERE page_id = $1", page); n != 1 {
			t.Fatalf("a round was reminded %d times", n)
		}
	}
	if n := e.auditActions("page.review_reminded"); n != len(pages) {
		t.Fatalf("%d reminders audited for %d rounds", n, len(pages))
	}
}

// TestAClockMovedBackGivesOneReminder is DOC-232 and DOC-1241. Moving every stored moment
// later is what the clock going back looks like to the job.
func TestAClockMovedBackGivesOneReminder(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DTC")
	page := w.reviewedPage(w.workflowOf("Due", dueDef(review("any", 0, entry("user", w.r1.id)), 5, 2, "")), "Clock")
	passReminder(e, page)
	e.dueJob()
	e.sql(`UPDATE review_deadlines SET remind_at = remind_at + interval '3 hours', due_at = due_at + interval '3 hours',
		next_at = next_at + interval '3 hours' WHERE page_id = $1`, page)
	e.dueJob()
	e.sql(`UPDATE review_deadlines SET remind_at = remind_at - interval '3 hours', due_at = due_at - interval '3 hours',
		next_at = next_at - interval '3 hours' WHERE page_id = $1`, page)
	e.dueJob()
	if n := e.count("SELECT reminders FROM review_deadlines WHERE page_id = $1", page); n != 1 {
		t.Fatalf("%d reminders", n)
	}
	e.deliver()
	if n := notificationCount(e, w.r1, notify.TypeReviewReminder); n != 1 {
		t.Fatalf("the reviewer was reminded %d times", n)
	}
}

// TestDueDatesCountWorkingDaysInTheInstancesZone is DOC-230, DOC-1240 and DOC-1242: the due
// moment is counted in working days from the database's own time, across a weekend, and a
// new working week applies to new rounds only.
func TestDueDatesCountWorkingDaysInTheInstancesZone(t *testing.T) {
	e := newEnv(t)
	// Friday 15:00 in Amsterdam plus one working day is Monday 15:00 there.
	got := e.scalar(`SELECT to_char(control_working_days_after('2026-11-13 14:00+00', 1, 'Europe/Amsterdam', ARRAY[1,2,3,4,5])
		AT TIME ZONE 'Europe/Amsterdam', 'Dy HH24:MI')`)
	if got != "Mon 15:00" {
		t.Fatalf("got %s", got)
	}
	// A week of Sunday to Thursday makes Friday's next working day Sunday.
	if got := e.scalar(`SELECT to_char(control_working_days_after('2026-11-13 14:00+00', 1, 'UTC', ARRAY[7,1,2,3,4]), 'Dy')`); got != "Sun" {
		t.Fatalf("got %s", got)
	}
	// A week without a working day still ends.
	if got := e.scalar(`SELECT to_char(control_working_days_after('2026-11-13 14:00+00', 3, 'UTC', ARRAY[]::int[]), 'Dy')`); got != "Mon" {
		t.Fatalf("got %s", got)
	}

	w := newReviewWorld(e, "DTD")
	flow := w.workflowOf("Due", dueDef(review("any", 0, entry("user", w.r1.id)), 3, 1, ""))
	page := w.reviewedPage(flow, "Counted")
	// The stage started at the database's time, and the due date is counted from that.
	if e.count(`SELECT count(*) FROM review_deadlines d JOIN workflow_rounds r ON r.id = d.round_id
		WHERE d.page_id = $1 AND d.started_at = (SELECT asked_at FROM workflow_reviewers wr WHERE wr.round_id = d.round_id LIMIT 1)
		  AND d.due_at = control_working_days_after(d.started_at, 3, 'UTC', ARRAY[1,2,3,4,5])
		  AND d.remind_at = control_working_days_after(d.started_at, 2, 'UTC', ARRAY[1,2,3,4,5])`, page) != 1 {
		t.Fatal("the due date is not counted in working days from the database's time")
	}
	before := e.scalar("SELECT due_at::text FROM review_deadlines WHERE page_id = $1", page)
	root := w.root
	general := root.ok("GET", "/api/v1/admin/settings/general", nil)
	general["working_days"] = []int{1, 2, 3, 4, 5, 6, 7}
	root.ok("PUT", "/api/v1/admin/settings/general", general)
	if after := e.scalar("SELECT due_at::text FROM review_deadlines WHERE page_id = $1", page); after != before {
		t.Fatal("a new working week moved a running round")
	}
	later := w.reviewedPage(flow, "Counted later")
	if e.count(`SELECT count(*) FROM review_deadlines WHERE page_id = $1
		AND due_at = control_working_days_after(started_at, 3, 'UTC', ARRAY[1,2,3,4,5,6,7])`, later) != 1 {
		t.Fatal("a new round does not follow the new working week")
	}
	// The review panel and the band show the due date (DOC-924).
	panel := w.r1.ok("GET", "/api/v1/pages/"+later+"/review", nil)
	if panel["due_at"] == nil {
		t.Fatalf("the panel shows no due date: %v", panel)
	}
	if c := w.control(w.author, later); c["due_at"] == nil {
		t.Fatal("the band shows no due date")
	}
}

// TestEscalationAddsTheTargetOnceAndNeverAnAuthor is DOC-231 and DOC-601: when the date passes
// the escalation target is asked and told, an author among them is left out under
// independence, a second pass adds nobody again, and reminders stop once the round ends.
func TestEscalationAddsTheTargetOnceAndNeverAnAuthor(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DTE")
	escalation := w.newGroup("Escalation of DTE", w.r3, w.author)
	flow := w.workflowOf("Escalating", dueDef(review("all", 0, entry("user", w.r1.id)), 2, 1, "", entry("group", escalation)))
	page := w.reviewedPage(flow, "Escalated")
	passDue(e, page)
	e.dueJob()
	if n := e.count(`SELECT count(*) FROM workflow_reviewers wr JOIN workflow_rounds r ON r.id = wr.round_id
		WHERE r.page_id = $1 AND wr.via = 'escalated' AND wr.removed_at IS NULL`, page); n != 1 {
		t.Fatalf("%d people were added by escalation", n)
	}
	if e.count(`SELECT count(*) FROM workflow_reviewers wr JOIN workflow_rounds r ON r.id = wr.round_id
		WHERE r.page_id = $1 AND wr.user_id = $2`, page, w.author.id) != 0 {
		t.Fatal("escalation asked the author of the change")
	}
	e.sql("UPDATE review_deadlines SET next_at = now() - interval '1 minute' WHERE page_id = $1", page)
	e.dueJob()
	if n := e.count(`SELECT count(*) FROM workflow_reviewers wr JOIN workflow_rounds r ON r.id = wr.round_id
		WHERE r.page_id = $1 AND wr.via = 'escalated'`, page); n != 1 {
		t.Fatalf("a second pass escalated again: %d", n)
	}
	if w.state(page) != "review" {
		t.Fatal("escalation moved the page")
	}
	e.deliver()
	if notificationCount(e, w.r3, notify.TypeReviewAsked) != 1 {
		t.Fatal("the escalation target was not told")
	}
	if e.auditActions("page.review_escalated") != 1 {
		t.Fatal("the escalation is not audited")
	}
	// Under "all" the escalated reviewer counts too; when both approve, reminders stop.
	w.answer(w.r1, page, "approve")
	w.answer(w.r3, page, "approve")
	if w.state(page) != "approved" {
		t.Fatalf("the round did not pass: %s", w.state(page))
	}
	if e.count("SELECT count(*) FROM review_deadlines WHERE page_id = $1 AND next_at IS NOT NULL", page) != 0 {
		t.Fatal("an ended round is still reminded")
	}
}

// TestTheDueJobWorksThroughManyRoundsInPasses is DOC-1304: at most 500 rounds a pass, each
// pass in its own short transaction, until every round was reminded.
func TestTheDueJobWorksThroughManyRoundsInPasses(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds many rounds")
	}
	e := newEnv(t)
	w := newReviewWorld(e, "DTF")
	flow := w.workflowOf("Due", dueDef(review("any", 0, entry("user", w.r1.id)), 5, 2, ""))
	page := w.reviewedPage(flow, "Seed")
	// 1200 copies of the deadline of one round, on rounds made by SQL, stand for many pages.
	e.sql(`INSERT INTO workflow_rounds (id, page_id, workflow_id, workflow_version, version_number, status, review_stage)
		SELECT gen_random_uuid(), p.id, $2, 1, 1, 'ended', 1 FROM pages p, generate_series(1, 1200) WHERE p.id = $1`, page, flow)
	e.sql(`INSERT INTO review_deadlines (round_id, stage, page_id, started_at, remind_at, due_at, next_at)
		SELECT r.id, 1, r.page_id, now(), now() - interval '1 minute', now() + interval '1 day', now() - interval '1 minute'
		FROM workflow_rounds r WHERE r.page_id = $1 AND r.status = 'ended'`, page)
	start := time.Now()
	e.dueJob()
	if left := e.count("SELECT count(*) FROM review_deadlines WHERE page_id = $1 AND next_at <= now()", page); left != 0 {
		t.Fatalf("%d deadlines left after the job", left)
	}
	t.Logf("1200 deadlines in %s", time.Since(start))
}
