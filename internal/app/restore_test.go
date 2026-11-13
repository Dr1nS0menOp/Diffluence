// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/backups"
	"github.com/diffluence/diffluence/internal/restore"
)

// A restore from the console rewinds the whole installation (spec 059). These tests prove
// each way it is refused or stops, failure paths first.

// madeBackup has the worker report a complete backup, as the console lists it.
func madeBackup(t *testing.T, e *env, at time.Time) string {
	t.Helper()
	name := stamp(at)
	if err := e.app.Backups().ReportRun(context.Background(), backups.Report{Name: name, Reason: backups.ReasonSchedule,
		StartedAt: at, OK: true, SizeBytes: 4096}); err != nil {
		t.Fatal(err)
	}
	return name
}

func openRestores(t *testing.T, e *env) int {
	t.Helper()
	var n int
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM ops.restores").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The name typed must be the installation's, and it is checked before the second factor,
// so a typing error costs no code.
func TestARestoreAsksForTheNameOfTheInstallation(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	_, recovery := root.enrolTOTP()
	name := madeBackup(t, e, time.Now().Add(-time.Hour))
	out := root.want(http.StatusBadRequest, "POST", "/api/v1/admin/backups/restore",
		map[string]any{"name": name, "confirm": "Tset", "code": recovery[0]})
	if !strings.Contains(out, "Test") {
		t.Fatalf("the refusal does not say what to type: %s", out)
	}
	if openRestores(t, e) != 0 || e.auditActions("backup.restore_requested") != 0 {
		t.Fatal("a restore with the wrong name was recorded")
	}
	if e.auditActions("auth.second_factor_confirmed") != 0 {
		t.Fatal("the code was used up by a typing error")
	}
}

// The password alone is not enough: a code from the authenticator app, a recovery code, or
// a passkey confirmed just before.
func TestARestoreAsksForASecondFactor(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	name := madeBackup(t, e, time.Now().Add(-time.Hour))
	ask := func(code string) string {
		return root.want(http.StatusForbidden, "POST", "/api/v1/admin/backups/restore",
			map[string]any{"name": name, "confirm": "Test", "code": code})
	}
	if out := ask(""); !strings.Contains(out, "second_factor_required") {
		t.Fatalf("no second factor at all: %s", out)
	}
	if out := ask("123456"); !strings.Contains(out, "second_factor_required") {
		t.Fatalf("a code without an authenticator app: %s", out)
	}
	_, recovery := root.enrolTOTP()
	if out := ask("000000"); !strings.Contains(out, "second_factor_required") {
		t.Fatalf("a wrong code: %s", out)
	}
	if openRestores(t, e) != 0 {
		t.Fatal("a restore without a second factor was recorded")
	}
	root.want(http.StatusAccepted, "POST", "/api/v1/admin/backups/restore",
		map[string]any{"name": name, "confirm": "Test", "code": recovery[0]})
	if e.auditActions("backup.restore_requested") != 1 {
		t.Fatal("the restore is not in the audit log")
	}
}

// A member never reaches it, and only a backup the worker reported complete can be restored.
func TestOnlyAnAdministratorRestoresACompleteBackup(t *testing.T) {
	e := newEnv(t)
	member := e.user("member@example.com")
	member.want(http.StatusForbidden, "POST", "/api/v1/admin/backups/restore", map[string]any{"name": "x", "confirm": "Test"})
	member.want(http.StatusForbidden, "DELETE", "/api/v1/admin/backups/restore", nil)
	root := admin(e)
	_, recovery := root.enrolTOTP()
	if err := e.app.Backups().ReportRun(context.Background(), backups.Report{Name: stamp(time.Now().Add(-time.Hour)),
		Reason: backups.ReasonSchedule, StartedAt: time.Now().Add(-time.Hour), Message: "dumping the database"}); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{stamp(time.Now().Add(-time.Hour)), stamp(time.Now().Add(-2 * time.Hour)), "../secrets"} {
		root.want(http.StatusBadRequest, "POST", "/api/v1/admin/backups/restore",
			map[string]any{"name": name, "confirm": "Test", "code": recovery[i]})
	}
	if openRestores(t, e) != 0 {
		t.Fatal("a failed or unknown backup was asked to be restored")
	}
}

// One restore at a time, none while a backup is asked for, and at most one a day.
func TestOneRestoreAtATimeAndOneADay(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	root := admin(e)
	_, recovery := root.enrolTOTP()
	name := madeBackup(t, e, time.Now().Add(-time.Hour))
	root.want(http.StatusAccepted, "POST", "/api/v1/admin/backups/request", nil)
	if out := root.want(http.StatusConflict, "POST", "/api/v1/admin/backups/restore",
		map[string]any{"name": name, "confirm": "Test", "code": recovery[0]}); !strings.Contains(out, "backup_requested") {
		t.Fatalf("a restore while a backup is asked for: %s", out)
	}
	if _, err := e.app.Pool().Exec(ctx, "UPDATE backup_requests SET finished_at = now()"); err != nil {
		t.Fatal(err)
	}
	root.want(http.StatusAccepted, "POST", "/api/v1/admin/backups/restore",
		map[string]any{"name": name, "confirm": "Test", "code": recovery[1]})
	// The same day, by command line too, which skips the second factor but not the limits,
	// nor the name typed out.
	wrong := restore.Ask{BackupName: name, Confirm: "Tset", Installation: "Test", CallerName: "the command line"}
	if _, err := restore.Request(ctx, e.app.Pool(), wrong); err == nil || !strings.Contains(err.Error(), "Type the name") {
		t.Fatalf("the command line with the wrong name: %v", err)
	}
	rs := restore.Ask{BackupName: name, Confirm: "Test", Installation: "Test", CallerName: "the command line"}
	if _, err := restore.Request(ctx, e.app.Pool(), rs); err == nil || !strings.Contains(err.Error(), "restore_limit") {
		t.Fatalf("a second restore the same day: %v", err)
	}
	if _, err := e.app.Pool().Exec(ctx, "UPDATE ops.restores SET requested_at = now() - interval '2 days'"); err != nil {
		t.Fatal(err)
	}
	if _, err := restore.Request(ctx, e.app.Pool(), rs); err == nil || !strings.Contains(err.Error(), "restore_open") {
		t.Fatalf("a second restore while one is open: %v", err)
	}
	// A restore the worker has not picked up can be taken back.
	root.want(http.StatusNoContent, "DELETE", "/api/v1/admin/backups/restore", nil)
	root.want(http.StatusConflict, "DELETE", "/api/v1/admin/backups/restore", nil)
	if e.auditActions("backup.restore_cancelled") != 1 {
		t.Fatal("taking a restore back is not in the audit log")
	}
}

// fakeSteps fail where a test chooses, and say what ran.
type fakeSteps struct {
	newest      int
	failAt      string
	ran         []string
	dbRestored  bool
	recorded    *restore.Restore
	failCheckOn string
}

func (f *fakeSteps) step(name string) error {
	f.ran = append(f.ran, name)
	if f.failAt == name {
		return errors.New("broke on purpose")
	}
	return nil
}
func (f *fakeSteps) Check(_ context.Context, name string) error {
	if f.failCheckOn != "" && name == f.failCheckOn {
		return errors.New("checksum differs")
	}
	return f.step("check")
}
func (f *fakeSteps) NewestMigration(context.Context, string) (int, error) {
	return f.newest, f.step("newest")
}
func (f *fakeSteps) StopOthers(context.Context) error { return f.step("stop-others") }
func (f *fakeSteps) RestoreDatabase(context.Context, string) error {
	err := f.step("database")
	f.dbRestored = err == nil
	return err
}
func (f *fakeSteps) RestoreFiles(context.Context, string) error { return f.step("files") }
func (f *fakeSteps) Migrate(context.Context) error              { return f.step("migrate") }
func (f *fakeSteps) Record(_ context.Context, r restore.Restore) error {
	f.recorded = &r
	return f.step("record")
}
func (f *fakeSteps) Verify(context.Context) error { return f.step("verify") }

// claimed asks for a restore by command line and has the worker claim it.
func claimed(t *testing.T, e *env) (*restore.Restore, string) {
	t.Helper()
	ctx := context.Background()
	name := madeBackup(t, e, time.Now().Add(-time.Hour))
	if _, err := restore.Request(ctx, e.app.Pool(), restore.Ask{BackupName: name, Confirm: "Test", Installation: "Test",
		CallerName: "the command line"}); err != nil {
		t.Fatal(err)
	}
	rs, err := restore.Claim(ctx, e.app.Pool())
	if err != nil || rs == nil || rs.State != restore.StatePreparing {
		t.Fatalf("the worker takes the restore: %+v %v", rs, err)
	}
	return rs, name
}

func runner(e *env, f *fakeSteps) restore.Runner {
	return restore.Runner{Pool: e.app.Pool(), Steps: f, NewestKnown: 100, StopWithin: 300 * time.Millisecond,
		Poll: 50 * time.Millisecond, Log: slog.New(slog.DiscardHandler)}
}

func mustWait(t *testing.T, e *env) *restore.Restore {
	t.Helper()
	r, err := restore.MustWait(context.Background(), e.app.Pool())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// A safety backup that fails stops the restore before anything is touched, and the nodes
// serve again.
func TestASafetyBackupThatFailsChangesNothing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	rs, _ := claimed(t, e)
	f := &fakeSteps{newest: 90}
	r := runner(e, f)
	if err := r.Prepare(ctx, rs.ID); err != nil {
		t.Fatal(err)
	}
	if w := mustWait(t, e); w == nil || w.State != restore.StateBackingUp {
		t.Fatalf("the nodes wait while the safety backup is made: %+v", w)
	}
	if err := r.Abort(ctx, rs.ID, "dumping the database"); err == nil {
		t.Fatal("an aborted restore reports success")
	}
	got, _ := restore.Get(ctx, e.app.Pool(), rs.ID)
	if got.State != restore.StateRefused || got.Changed || got.Open() || !strings.Contains(got.Step, "safety backup failed") {
		t.Fatalf("after a failed safety backup: %+v", got)
	}
	if mustWait(t, e) != nil {
		t.Fatal("the nodes still wait after a restore that changed nothing")
	}
	for _, s := range f.ran {
		if s == "database" || s == "files" || s == "stop-others" {
			t.Fatalf("a step that changes the installation ran: %v", f.ran)
		}
	}
}

// A backup made by a newer release is refused before any node stops.
func TestABackupOfANewerReleaseIsRefused(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	rs, _ := claimed(t, e)
	err := runner(e, &fakeSteps{newest: 101}).Prepare(ctx, rs.ID)
	if !errors.Is(err, restore.ErrNewerBackup) {
		t.Fatalf("a backup of a newer release: %v", err)
	}
	got, _ := restore.Get(ctx, e.app.Pool(), rs.ID)
	if got.State != restore.StateRefused || got.Changed || !strings.Contains(got.Step, "newer release") {
		t.Fatalf("after a newer backup: %+v", got)
	}
	if mustWait(t, e) != nil {
		t.Fatal("the nodes were stopped for a backup that could not be restored")
	}
}

// A backup that is not whole is refused before any node stops.
func TestABackupThatIsNotWholeIsRefused(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	rs, name := claimed(t, e)
	if err := runner(e, &fakeSteps{newest: 90, failCheckOn: name}).Prepare(ctx, rs.ID); err == nil {
		t.Fatal("a backup that is not whole was prepared")
	}
	got, _ := restore.Get(ctx, e.app.Pool(), rs.ID)
	if got.State != restore.StateRefused || got.Changed {
		t.Fatalf("after a broken backup: %+v", got)
	}
}

// A node that keeps serving stops the restore before anything changes: the worker never
// restores under a node that could still write.
func TestANodeThatKeepsServingStopsTheRestore(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	rs, _ := claimed(t, e)
	stubborn := uuid.New()
	if err := restore.Seen(ctx, e.app.Pool(), stubborn, restore.ModeServing); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_ = restore.Seen(ctx, e.app.Pool(), stubborn, restore.ModeServing)
			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}()
	err := runner(e, &fakeSteps{newest: 90}).Prepare(ctx, rs.ID)
	close(stop)
	<-done
	if !errors.Is(err, restore.ErrNodesServing) {
		t.Fatalf("a node that keeps serving: %v", err)
	}
	got, _ := restore.Get(ctx, e.app.Pool(), rs.ID)
	if got.State != restore.StateRefused || got.Changed || !strings.Contains(got.Step, "kept serving") {
		t.Fatalf("after a node kept serving: %+v", got)
	}
}

// A restore that fails halfway keeps every node waiting, and the way back is the safety
// backup; once it ran, the nodes serve again.
func TestARestoreThatFailsHalfwayKeepsTheNodesWaitingUntilTheWayBack(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	rs, _ := claimed(t, e)
	f := &fakeSteps{newest: 90, failAt: "files"}
	r := runner(e, f)
	if err := r.Prepare(ctx, rs.ID); err != nil {
		t.Fatal(err)
	}
	safety := stamp(time.Now())
	if err := r.Run(ctx, rs.ID, safety); err == nil {
		t.Fatal("a restore that broke reports success")
	}
	w := mustWait(t, e)
	if w == nil || w.State != restore.StateFailed || !w.Changed || w.SafetyName != safety || !strings.Contains(w.Step, "stored files") {
		t.Fatalf("after a restore broke halfway: %+v", w)
	}
	// The way back restores the safety backup with every node still waiting.
	f.failAt = ""
	back, err := r.WayBack(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if back.BackupName != safety {
		t.Fatalf("the way back restored %s, not the safety backup", back.BackupName)
	}
	if mustWait(t, e) != nil {
		t.Fatal("the nodes still wait after the way back")
	}
	got, _ := restore.Get(ctx, e.app.Pool(), rs.ID)
	if got.State != restore.StateDone {
		t.Fatalf("after the way back: %+v", got)
	}
}

// A restore whose checks fail afterwards keeps the nodes waiting too.
func TestARestoreWhoseChecksFailKeepsTheNodesWaiting(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	rs, _ := claimed(t, e)
	f := &fakeSteps{newest: 90, failAt: "verify"}
	r := runner(e, f)
	if err := r.Prepare(ctx, rs.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(ctx, rs.ID, stamp(time.Now())); err == nil {
		t.Fatal("a restore whose checks failed reports success")
	}
	w := mustWait(t, e)
	if w == nil || w.State != restore.StateFailed || !strings.Contains(w.Step, "audit log and the keys") {
		t.Fatalf("after the checks failed: %+v", w)
	}
	if !f.dbRestored {
		t.Fatal("the database step did not run before the checks")
	}
}

// A restore that passes writes itself into the restored audit log and lets the nodes serve.
func TestARestoreThatPassesIsRecordedAndEndsTheWait(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	rs, name := claimed(t, e)
	f := &fakeSteps{newest: 90}
	r := runner(e, f)
	if err := r.Prepare(ctx, rs.ID); err != nil {
		t.Fatal(err)
	}
	safety := stamp(time.Now())
	if err := r.Run(ctx, rs.ID, safety); err != nil {
		t.Fatal(err)
	}
	want := []string{"check", "newest", "stop-others", "database", "files", "migrate", "record", "verify"}
	if strings.Join(f.ran, " ") != strings.Join(want, " ") {
		t.Fatalf("the steps ran as %v, not %v", f.ran, want)
	}
	if f.recorded == nil || f.recorded.BackupName != name || f.recorded.SafetyName != safety || f.recorded.RequestedByName != "the command line" {
		t.Fatalf("what the restored audit log is told: %+v", f.recorded)
	}
	if mustWait(t, e) != nil {
		t.Fatal("the nodes still wait after a restore that passed")
	}
}

// A worker that died before it changed anything does not keep the site down for ever.
func TestARestoreThatStoodStillLongBeforeAnyChangeIsNotWaitedFor(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	rs, _ := claimed(t, e)
	if err := runner(e, &fakeSteps{newest: 90}).Prepare(ctx, rs.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Pool().Exec(ctx, "UPDATE ops.restores SET updated_at = now() - interval '7 hours'"); err != nil {
		t.Fatal(err)
	}
	if mustWait(t, e) != nil {
		t.Fatal("the nodes wait for a worker that stopped hours ago")
	}
	if _, err := restore.Claim(ctx, e.app.Pool()); err != nil {
		t.Fatal(err)
	}
	got, _ := restore.Get(ctx, e.app.Pool(), rs.ID)
	if got.State != restore.StateRefused || got.Open() {
		t.Fatalf("an abandoned restore is not ended: %+v", got)
	}
}
