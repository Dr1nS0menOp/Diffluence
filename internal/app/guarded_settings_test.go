// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/backups"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/restore"
	"github.com/diffluence/diffluence/internal/settings"
)

// Some changes of the console let more through than before: fewer backups kept, backups
// less often, a shorter audit retention, a weaker rule for logging in. They are a system
// administrator's, with a fresh login and a second factor, and fewer backups also need the
// code from the backup service's log (spec 076). Refusals first, then what is allowed.

// heldDefaults is a worker that keeps what it holds to in a file of its own.
func heldDefaults(t *testing.T) backups.Defaults {
	t.Helper()
	return heldAt(t, filepath.Join(t.TempDir(), "held.json"))
}

// heldAt is a worker that keeps what it holds to in path.
func heldAt(t *testing.T, path string) backups.Defaults {
	t.Helper()
	h, err := backups.LoadHeld(path)
	if err != nil {
		t.Fatal(err)
	}
	d := workerDefaults
	d.Held = &h
	return d
}

// checkedIn has the worker ask once, so the console knows its defaults.
func checkedIn(t *testing.T, e *env, d backups.Defaults) {
	t.Helper()
	if _, err := e.app.Backups().Next(context.Background(), d); err != nil {
		t.Fatal(err)
	}
}

func confirmed(settings map[string]any, code, reason string) map[string]any {
	return map[string]any{"settings": settings, "code": code, "reason": reason}
}

// confirmedChange stores a section as the route of a confirmed change does once it checked
// the person, for tests about what the change does rather than who may make it.
func confirmedChange(e *env, section string, v map[string]any) {
	e.t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := settings.NewService(e.app.Pool(), nil).SetSectionConfirmed(context.Background(), identity.SystemCaller("test"),
		section, bytes.NewReader(body), "A test needs it"); err != nil {
		e.t.Fatal(err)
	}
}

// keptNow is how many backups the worker keeps after this round.
func keptNow(t *testing.T, e *env, d backups.Defaults) int {
	t.Helper()
	dec, err := e.app.Backups().Next(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	return dec.Keep
}

var logCode = regexp.MustCompile(`[0-9]{5}-[0-9]{5}`)

// workerCode has the worker look at the ask that waits, and returns the code it writes to its
// log.
func workerCode(t *testing.T, e *env, g restore.Gate, d backups.Defaults) string {
	t.Helper()
	line, err := e.app.Backups().ConfirmFewer(context.Background(), g, d)
	if err != nil {
		t.Fatal(err)
	}
	code := logCode.FindString(line)
	if code == "" {
		t.Fatalf("the worker shows no code: %q", line)
	}
	return code
}

func workerRound(t *testing.T, e *env, g restore.Gate, d backups.Defaults) string {
	t.Helper()
	line, err := e.app.Backups().ConfirmFewer(context.Background(), g, d)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func backupPage(t *testing.T, u *user) backups.Overview {
	t.Helper()
	var page backups.Overview
	if err := json.Unmarshal([]byte(u.want(http.StatusOK, "GET", "/api/v1/admin/backups", nil)), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func TestKeepingFewerBackupsNeedsASystemAdministrator(t *testing.T) {
	e := newEnv(t)
	d := heldDefaults(t)
	clerk := e.user("clerk@example.com", identity.PermUseProduct, identity.PermAdministrator)
	_, recovery := clerk.enrolTOTP()
	// Before any worker said its defaults, a change to or from its default cannot be judged.
	clerk.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/backups", map[string]any{"interval_hours": 0, "keep": 7})
	checkedIn(t, e, d)
	for _, fewer := range []map[string]any{{"interval_hours": 0, "keep": 7}, {"interval_hours": 168, "keep": 0}} {
		if out := clerk.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/backups", fewer); !strings.Contains(out, "confirmation_required") {
			t.Fatalf("an administrator keeps fewer backups: %s", out)
		}
	}
	clerk.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/backups/confirmed",
		confirmed(map[string]any{"interval_hours": 0, "keep": 7}, recovery[0], ""))
	if page := backupPage(t, clerk); page.Settings.Keep != 0 || page.Confirmation != nil {
		t.Fatalf("a refused change was stored or asked for: %+v", page)
	}
	if e.auditActions(backups.ActionScheduleAsked) != 0 {
		t.Fatal("a refused change asked the worker")
	}
	// Keeping more, and backing up more often, stays an administrator's change, and the
	// worker follows it at once.
	clerk.ok("PUT", "/api/v1/admin/settings/backups", map[string]any{"interval_hours": 2, "keep": 60})
	if got := keptNow(t, e, d); got != 60 || d.Held.IntervalSeconds != 2*3600 {
		t.Fatalf("more backups wait: keep %d, every %ds", got, d.Held.IntervalSeconds)
	}
}

func TestKeepingFewerBackupsNeedsAFreshLoginAndASecondFactor(t *testing.T) {
	e := newEnv(t)
	d := heldDefaults(t)
	checkedIn(t, e, d)
	root := admin(e)
	fewer := map[string]any{"interval_hours": 0, "keep": 7}
	if out := root.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/backups", fewer); !strings.Contains(out, "confirmation_required") {
		t.Fatalf("a system administrator's session alone keeps fewer backups: %s", out)
	}
	if out := root.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/backups/confirmed",
		confirmed(fewer, "", "")); !strings.Contains(out, "second_factor_required") {
		t.Fatalf("no second factor: %s", out)
	}
	_, recovery := root.enrolTOTP()
	if out := root.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/backups/confirmed",
		confirmed(fewer, "000000", "")); !strings.Contains(out, "second_factor_required") {
		t.Fatalf("a wrong second factor: %s", out)
	}
	e.sql("UPDATE sessions SET created_at = now() - interval '11 minutes', reauth_at = NULL")
	if out := root.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/backups/confirmed",
		confirmed(fewer, recovery[0], "")); !strings.Contains(out, "reauth_required") {
		t.Fatalf("an old login: %s", out)
	}
	if page := backupPage(t, root); page.Settings.Keep != 0 || page.Confirmation != nil {
		t.Fatalf("a refused change was stored or asked for: %+v", page)
	}
}

func TestKeepingFewerBackupsNeedsTheWorkerCode(t *testing.T) {
	e := newEnv(t)
	d := heldDefaults(t)
	g := testGate(t)
	checkedIn(t, e, d)
	root := admin(e)
	clerk := e.user("clerk@example.com", identity.PermUseProduct, identity.PermAdministrator)
	_, recovery := root.enrolTOTP()
	root.ok("PUT", "/api/v1/admin/settings/backups/confirmed", confirmed(map[string]any{"interval_hours": 0, "keep": 7}, recovery[0], ""))
	if got := keptNow(t, e, d); got != 42 {
		t.Fatalf("the worker keeps %d before its code was entered", got)
	}
	root.want(http.StatusConflict, "POST", "/api/v1/admin/backups/schedule/code", map[string]any{"code": "12345-67890"})
	right := workerCode(t, e, g, d)
	if again := workerRound(t, e, g, d); again != "" {
		t.Fatalf("the code is shown again: %q", again)
	}
	if page := backupPage(t, root); page.Confirmation == nil || !page.Confirmation.CodeShown || page.Confirmation.Keep != 7 {
		t.Fatalf("the console does not say a code waits: %+v", page.Confirmation)
	}
	clerk.want(http.StatusForbidden, "POST", "/api/v1/admin/backups/schedule/code", map[string]any{"code": right})
	root.want(http.StatusBadRequest, "POST", "/api/v1/admin/backups/schedule/code", map[string]any{"code": "12345"})
	wrong := "00000-00000"
	if wrong == right {
		wrong = "11111-11111"
	}
	for try := 1; try <= backups.ConfirmationTries; try++ {
		root.want(http.StatusNoContent, "POST", "/api/v1/admin/backups/schedule/code", map[string]any{"code": wrong})
		root.want(http.StatusConflict, "POST", "/api/v1/admin/backups/schedule/code", map[string]any{"code": right})
		workerRound(t, e, g, d)
		if e.auditActions(backups.ActionScheduleCodeWrong) != try {
			t.Fatalf("wrong code %d is not in the audit log", try)
		}
		if got := keptNow(t, e, d); got != 42 {
			t.Fatalf("after wrong code %d the worker keeps %d", try, got)
		}
	}
	if e.auditActions(backups.ActionScheduleRefused) != 1 || e.auditActions(backups.ActionScheduleConfirmed) != 0 {
		t.Fatal("the end of the ask is not in the audit log")
	}
	if page := backupPage(t, root); page.Confirmation != nil || page.Worker == nil || page.Worker.HeldKeep != 42 {
		t.Fatalf("after three wrong codes: %+v %+v", page.Confirmation, page.Worker)
	}
	// The same ask is not shown again: a new one is a new confirmed change.
	if again := workerRound(t, e, g, d); again != "" {
		t.Fatalf("a refused ask is shown again: %q", again)
	}
}

func TestACodeForFewerBackupsEndsAfterHalfAnHour(t *testing.T) {
	e := newEnv(t)
	d := heldDefaults(t)
	g := testGate(t)
	checkedIn(t, e, d)
	root := admin(e)
	_, recovery := root.enrolTOTP()
	root.ok("PUT", "/api/v1/admin/settings/backups/confirmed", confirmed(map[string]any{"interval_hours": 0, "keep": 7}, recovery[0], ""))
	right := workerCode(t, e, g, d)
	d.Held.Challenge.ShownAt = time.Now().Add(-31 * time.Minute)
	workerRound(t, e, g, d)
	if e.auditActions(backups.ActionScheduleRefused) != 1 {
		t.Fatal("an old code did not end the ask")
	}
	root.want(http.StatusConflict, "POST", "/api/v1/admin/backups/schedule/code", map[string]any{"code": right})
	if got := keptNow(t, e, d); got != 42 {
		t.Fatalf("the worker keeps %d after an old code", got)
	}
}

// Without its backup key the worker cannot make a code, so it ends the ask and keeps what it
// holds to.
func TestAWorkerWithoutItsKeyKeepsItsBackups(t *testing.T) {
	e := newEnv(t)
	d := heldDefaults(t)
	checkedIn(t, e, d)
	root := admin(e)
	_, recovery := root.enrolTOTP()
	root.ok("PUT", "/api/v1/admin/settings/backups/confirmed", confirmed(map[string]any{"interval_hours": 0, "keep": 7}, recovery[0], ""))
	if line := workerRound(t, e, restore.Gate{}, d); line != "" {
		t.Fatalf("a worker without a key shows a code: %q", line)
	}
	if e.auditActions(backups.ActionScheduleRefused) != 1 || keptNow(t, e, d) != 42 {
		t.Fatal("a worker without a key took fewer backups or did not say why")
	}
}

// What the application role writes into the database, past the console, never makes the
// worker keep fewer backups or back up less often: neither the section, nor an ask with a
// guessed code, nor the code of another ask.
func TestTheWorkerKeepsItsBackupsWhateverTheDatabaseSays(t *testing.T) {
	e := newEnv(t)
	d := heldDefaults(t)
	g := testGate(t)
	checkedIn(t, e, d)
	e.sql(`INSERT INTO settings (key, value) VALUES ('section.backups', '{"interval_hours": 168, "keep": 1}')
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`)
	if line := workerRound(t, e, g, d); line != "" {
		t.Fatalf("a section written past the console asks for a code: %q", line)
	}
	if got := keptNow(t, e, d); got != 42 || d.Held.IntervalSeconds != 4*3600 {
		t.Fatalf("the worker follows a section written past the console: keep %d, every %ds", got, d.Held.IntervalSeconds)
	}

	// An ask for other values than the section holds is not what anyone confirmed.
	other := ids.New()
	e.sql("INSERT INTO backup_schedule_requests (id, interval_hours, keep) VALUES ($1, 168, 2)", other)
	if line := workerRound(t, e, g, d); line != "" {
		t.Fatalf("an ask for other values shows a code: %q", line)
	}
	e.sql("UPDATE backup_schedule_requests SET finished_at = now(), outcome = 'replaced' WHERE id = $1", other)

	first := ids.New()
	e.sql("INSERT INTO backup_schedule_requests (id, interval_hours, keep) VALUES ($1, 168, 1)", first)
	firstCode := workerCode(t, e, g, d)
	for range backups.ConfirmationTries {
		e.sql("UPDATE backup_schedule_requests SET confirmation = '9999999999' WHERE finished_at IS NULL")
		workerRound(t, e, g, d)
		if d.Held.Keep != 42 {
			t.Fatalf("a guessed code lowered what the worker holds to: %d", d.Held.Keep)
		}
	}
	if e.auditActions(backups.ActionScheduleRefused) != 1 {
		t.Fatal("the guesses did not end the ask")
	}

	// The code of one ask confirms no other.
	second := ids.New()
	e.sql("INSERT INTO backup_schedule_requests (id, interval_hours, keep) VALUES ($1, 168, 1)", second)
	workerCode(t, e, g, d)
	e.sql("UPDATE backup_schedule_requests SET confirmation = $1 WHERE id = $2", strings.ReplaceAll(firstCode, "-", ""), second)
	workerRound(t, e, g, d)
	if got := keptNow(t, e, d); got != 42 || d.Held.IntervalSeconds != 4*3600 {
		t.Fatalf("the code of another ask lowered what the worker holds to: keep %d, every %ds", got, d.Held.IntervalSeconds)
	}
	if e.auditActions(backups.ActionScheduleConfirmed) != 0 {
		t.Fatal("an ask past the console was confirmed")
	}
}

func TestTheRightWorkerCodeKeepsFewerBackups(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "held.json")
	d := heldAt(t, path)
	g := testGate(t)
	checkedIn(t, e, d)
	root := admin(e)
	_, recovery := root.enrolTOTP()
	root.ok("PUT", "/api/v1/admin/settings/backups/confirmed",
		confirmed(map[string]any{"interval_hours": 168, "keep": 7}, recovery[0], ""))
	code := workerCode(t, e, g, d)
	// Spaces and dashes are the reader's, not part of the code.
	root.want(http.StatusNoContent, "POST", "/api/v1/admin/backups/schedule/code",
		map[string]any{"code": " " + strings.ReplaceAll(code, "-", " ") + " "})
	workerRound(t, e, g, d)
	if got := keptNow(t, e, d); got != 7 || d.Held.IntervalSeconds != 168*3600 {
		t.Fatalf("after the right code: keep %d, every %ds", got, d.Held.IntervalSeconds)
	}
	if e.auditActions(backups.ActionScheduleCodeEntered) != 1 || e.auditActions(backups.ActionScheduleConfirmed) != 1 {
		t.Fatal("entering and confirming the code are not in the audit log")
	}
	page := backupPage(t, root)
	if page.Confirmation != nil || page.Worker == nil || page.Worker.HeldKeep != 7 || page.Worker.HeldIntervalHours != 168 {
		t.Fatalf("after the right code the console shows: %+v %+v", page.Confirmation, page.Worker)
	}
	for _, action := range []string{backups.ActionScheduleCodeEntered, backups.ActionScheduleConfirmed} {
		events, err := audit.List(ctx, e.app.Pool(), audit.Filter{Action: action, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(fmt.Sprintf("%+v", events), strings.ReplaceAll(code, "-", "")) {
			t.Fatal("the code reached the audit log")
		}
	}
	// What the worker holds to outlives the process: it starts again holding to 7.
	again, err := backups.LoadHeld(path)
	if err != nil || again.Keep != 7 {
		t.Fatalf("what the worker holds to after it starts again: %+v %v", again, err)
	}
}

func TestAWeakerRuleForLoggingInNeedsAConfirmedChange(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	clerk := e.user("clerk@example.com", identity.PermUseProduct, identity.PermAdministrator)
	_, recovery := root.enrolTOTP()
	_, clerkRecovery := clerk.enrolTOTP()
	access := func(change map[string]any) map[string]any {
		a := root.ok("GET", "/api/v1/admin/settings/access", nil)
		for k, v := range change {
			a[k] = v
		}
		return a
	}
	// Tightening first, by an administrator: an idle time, and no guests.
	clerk.ok("PUT", "/api/v1/admin/settings/access", access(map[string]any{"idle_minutes": 30, "guest_invites": false}))
	clerk.ok("PUT", "/api/v1/admin/settings/access", access(map[string]any{"idle_minutes": 20}))
	clerk.ok("PUT", "/api/v1/admin/settings/access", access(map[string]any{"second_factor": "administrators"}))
	for name, change := range map[string]map[string]any{
		"no second factor":        {"second_factor": "none"},
		"the rule of a new one":   {"second_factor": "administrators_after_grace"},
		"a longer idle time":      {"idle_minutes": 60},
		"no idle time":            {"idle_minutes": 0},
		"guests again":            {"guest_invites": true},
		"a domain for signing up": {"signup_domains": []string{"example.com"}},
	} {
		for _, u := range []*user{clerk, root} {
			if out := u.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/access", access(change)); !strings.Contains(out, "confirmation_required") {
				t.Fatalf("%s by a session alone: %s", name, out)
			}
		}
	}
	weaker := access(map[string]any{"second_factor": "none"})
	clerk.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/access/confirmed", confirmed(weaker, clerkRecovery[0], ""))
	if out := root.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/access/confirmed", confirmed(weaker, "", "")); !strings.Contains(out, "second_factor_required") {
		t.Fatalf("no second factor: %s", out)
	}
	e.sql("UPDATE sessions SET created_at = now() - interval '11 minutes', reauth_at = NULL WHERE user_id = $1", root.id)
	if out := root.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/access/confirmed", confirmed(weaker, recovery[0], "")); !strings.Contains(out, "reauth_required") {
		t.Fatalf("an old login: %s", out)
	}
	if got := root.ok("GET", "/api/v1/admin/settings/access", nil); got["second_factor"] != "administrators" {
		t.Fatalf("a refused change was stored: %v", got)
	}
	root.ok("POST", "/api/v1/auth/reauth", map[string]any{"password": adminPassword})
	root.ok("PUT", "/api/v1/admin/settings/access/confirmed", confirmed(weaker, recovery[0], ""))
	if got := root.ok("GET", "/api/v1/admin/settings/access", nil); got["second_factor"] != "none" {
		t.Fatalf("a confirmed change was not stored: %v", got)
	}
	root.want(http.StatusNotFound, "PUT", "/api/v1/admin/settings/look/confirmed", confirmed(map[string]any{}, recovery[1], ""))
}

// Every wrong second factor leaves a trace, and guessing stops after a few, as for a restore.
func TestWrongCodesForAConfirmedChangeAreLimited(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	_, recovery := root.enrolTOTP()
	weaker := confirmed(map[string]any{"second_factor": "none", "idle_minutes": 0, "signup_domains": []string{}, "guest_invites": true}, "000000", "")
	limited := false
	for range 20 {
		if status, _ := root.do("PUT", "/api/v1/admin/settings/access/confirmed", weaker); status == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited || e.auditActions("auth.second_factor_failed") == 0 {
		t.Fatal("twenty wrong codes in a row were all tried, or left no trace")
	}
	weaker["code"] = recovery[0]
	root.want(http.StatusTooManyRequests, "PUT", "/api/v1/admin/settings/access/confirmed", weaker)
}

func TestAShorterAuditRetentionNeedsAConfirmedChangeAndAReason(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	root := admin(e)
	clerk := e.user("clerk@example.com", identity.PermUseProduct, identity.PermAdministrator)
	_, recovery := root.enrolTOTP()
	days := func() int {
		t.Helper()
		n, err := settings.AuditDays(ctx, e.app.Pool())
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, u := range []*user{clerk, root} {
		if out := u.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/retention", map[string]any{"audit_days": 90}); !strings.Contains(out, "confirmation_required") {
			t.Fatalf("a shorter retention by a session alone: %s", out)
		}
	}
	root.want(http.StatusBadRequest, "PUT", "/api/v1/admin/settings/retention/confirmed",
		confirmed(map[string]any{"audit_days": 90}, recovery[0], " "))
	root.ok("PUT", "/api/v1/admin/settings/retention/confirmed",
		confirmed(map[string]any{"audit_days": 90}, recovery[1], "The contract that asked for a year ended"))
	if got := days(); got != 365 {
		t.Fatalf("a shorter retention holds at once: %d", got)
	}
	got := root.ok("GET", "/api/v1/admin/settings/retention", nil)
	lowering, _ := got["lowering"].(map[string]any)
	if got["audit_days"] != float64(365) || lowering == nil || lowering["audit_days"] != float64(90) {
		t.Fatalf("the console shows: %v", got)
	}
	// An ordinary change keeps the shorter time waiting, and cannot set one itself.
	clerk.ok("PUT", "/api/v1/admin/settings/retention", map[string]any{"audit_days": 400, "longer_because": "A review",
		"lowering": map[string]any{"audit_days": 1, "because": "now", "from": "2000-01-01T00:00:00Z"}})
	if got := days(); got != 400 {
		t.Fatalf("after a longer time by an administrator: %d", got)
	}
	got = root.ok("GET", "/api/v1/admin/settings/retention", nil)
	if lowering, _ = got["lowering"].(map[string]any); lowering == nil || lowering["audit_days"] != float64(90) {
		t.Fatalf("an ordinary change replaced the waiting time: %v", got)
	}
	e.sql(`UPDATE settings SET value = jsonb_set(value, '{lowering,from}', to_jsonb(now() - interval '1 minute'))
		WHERE key = 'section.retention'`)
	if got := days(); got != 90 {
		t.Fatalf("at its time the shorter retention holds: %d", got)
	}
}

func TestAnAdministratorTakesBackAShorterAuditRetention(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	root := admin(e)
	clerk := e.user("clerk@example.com", identity.PermUseProduct, identity.PermAdministrator)
	_, recovery := root.enrolTOTP()
	clerk.want(http.StatusConflict, "DELETE", "/api/v1/admin/settings/retention/lowering", nil)
	root.ok("PUT", "/api/v1/admin/settings/retention/confirmed",
		confirmed(map[string]any{"audit_days": 30}, recovery[0], "Less to keep"))
	clerk.want(http.StatusNoContent, "DELETE", "/api/v1/admin/settings/retention/lowering", nil)
	e.sql(`UPDATE settings SET value = jsonb_set(value, '{lowering,from}', to_jsonb(now() - interval '1 minute'))
		WHERE key = 'section.retention' AND value ? 'lowering'`)
	if n, err := settings.AuditDays(ctx, e.app.Pool()); err != nil || n != 365 {
		t.Fatalf("after taking it back: %d %v", n, err)
	}
	if got := root.ok("GET", "/api/v1/admin/settings/retention", nil); got["lowering"] != nil {
		t.Fatalf("the shorter time still waits: %v", got)
	}
	if e.auditActions("settings.retention_lowering_cancelled") != 1 {
		t.Fatal("taking it back is not in the audit log")
	}
}

func TestOnlyASystemAdministratorTakesBackARestore(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	clerk := e.user("clerk@example.com", identity.PermUseProduct, identity.PermAdministrator)
	_, recovery := root.enrolTOTP()
	name := madeBackup(t, e, time.Now().Add(-time.Hour))
	root.want(http.StatusAccepted, "POST", "/api/v1/admin/backups/restore", map[string]any{"name": name, "confirm": "Test", "code": recovery[0]})
	clerk.want(http.StatusForbidden, "DELETE", "/api/v1/admin/backups/restore", nil)
	if got := latestRestore(t, e.app.Pool()); got.State != restore.StateRequested {
		t.Fatalf("an administrator took a restore back: %+v", got)
	}
	root.want(http.StatusNoContent, "DELETE", "/api/v1/admin/backups/restore", nil)
	if got := latestRestore(t, e.app.Pool()); got.State != restore.StateRefused || e.auditActions("backup.restore_cancelled") != 1 {
		t.Fatalf("a system administrator could not take it back: %+v", got)
	}
}

func TestTheRestoreCodeIsTakenDuringMaintenance(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	clerk := e.user("clerk@example.com", identity.PermUseProduct, identity.PermAdministrator)
	_, recovery := root.enrolTOTP()
	g := testGate(t)
	name := madeBackup(t, e, time.Now().Add(-time.Hour))
	root.want(http.StatusAccepted, "POST", "/api/v1/admin/backups/restore", map[string]any{"name": name, "confirm": "Test", "code": recovery[0]})
	code := shownCode(t, e.app.Pool(), g)
	root.ok("PUT", "/api/v1/admin/maintenance", map[string]any{"on": true, "message": "Restoring"})
	clerk.want(http.StatusForbidden, "POST", "/api/v1/admin/backups/restore/code", map[string]any{"code": code})
	root.want(http.StatusNoContent, "POST", "/api/v1/admin/backups/restore/code", map[string]any{"code": code})
	if c, err := restore.Claim(context.Background(), e.app.Pool(), g); err != nil || c.Restore == nil {
		t.Fatalf("the code entered during maintenance: %+v %v", c, err)
	}
	// Other writes still wait for the end of maintenance.
	root.want(http.StatusServiceUnavailable, "POST", "/api/v1/admin/backups/request", nil)
}
