// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/keys"
	"github.com/diffluence/diffluence/internal/restore"
	"github.com/diffluence/diffluence/internal/testutil"
)

// The backup service carries out a restore only with what the server never holds: a code it
// wrote to its own log, and a seal made with a key derived from the backup key (spec 070).
// Failure paths first.

// testGate is the gate of a backup service whose backup key is always the same.
func testGate(t *testing.T) restore.Gate {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backup.key")
	if err := os.WriteFile(path, []byte("dGhlIGJhY2t1cCBrZXkgb2YgdGhlIHRlc3RzIG9mIHNwZWMgMDcw\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := keys.LoadWorker(path)
	if err != nil {
		t.Fatal(err)
	}
	return restore.NewGate(w.MAC)
}

// shownCode has the worker look at the restore that waits, and returns the code it writes to
// its log.
func shownCode(t *testing.T, pool *pgxpool.Pool, g restore.Gate) string {
	t.Helper()
	c, err := restore.Claim(context.Background(), pool, g)
	if err != nil || c.Show == nil || c.Code == "" || c.Restore != nil {
		t.Fatalf("the worker shows a code: %+v %v", c, err)
	}
	return c.Code
}

func latestRestore(t *testing.T, pool *pgxpool.Pool) restore.Restore {
	t.Helper()
	r, err := restore.Latest(context.Background(), pool)
	if err != nil || r == nil {
		t.Fatalf("the latest restore: %v", err)
	}
	return *r
}

// twoRoleEnv runs the server as an application role that owns nothing, as Compose does,
// and returns the owner's pool for the backup service.
func twoRoleEnv(t *testing.T) (*env, *pgxpool.Pool) {
	t.Helper()
	tdb := testutil.Database(t)
	ctx := context.Background()
	role := "dfl_app_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if _, err := tdb.Pool.Exec(ctx, "CREATE ROLE "+role+" LOGIN PASSWORD 'app'"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = tdb.Pool.Exec(context.Background(), "DROP OWNED BY "+role)
		_, _ = tdb.Pool.Exec(context.Background(), "DROP ROLE "+role)
	})
	u, _ := url.Parse(tdb.URL)
	u.User = url.UserPassword(role, "app")
	e := newEnvWith(t, func(c *config.Config) {
		c.DatabaseURL = u.String()
		c.MigrateDatabaseURL = tdb.URL
	})
	return e, tdb.Pool
}

// Without its backup key the backup service cannot make a code, so it refuses at once and
// says why.
func TestAWorkerWithoutItsKeyRefusesEveryRestore(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	name := madeBackup(t, e, time.Now().Add(-time.Hour))
	if _, err := restore.Request(ctx, e.app.Pool(), restore.Ask{BackupName: name, Confirm: "Test", Installation: "Test",
		CallerName: "the command line"}); err != nil {
		t.Fatal(err)
	}
	c, err := restore.Claim(ctx, e.app.Pool(), restore.Gate{})
	if err != nil || c.Restore != nil || c.Show != nil {
		t.Fatalf("a worker without a key: %+v %v", c, err)
	}
	got := latestRestore(t, e.app.Pool())
	if got.State != restore.StateRefused || got.Open() || !strings.Contains(got.Step, "no backup key") {
		t.Fatalf("after a worker without a key: %+v", got)
	}
	if e.auditActions("backup.restore_refused") != 1 {
		t.Fatal("the refusal is not in the audit log")
	}
}

// A request waits for the code; the code goes to the worker's log once, and a wrong code is
// counted, recorded and cleared; three end the restore.
func TestThreeWrongCodesEndTheRestore(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	g := testGate(t)
	name := madeBackup(t, e, time.Now().Add(-time.Hour))
	if _, err := restore.Request(ctx, e.app.Pool(), restore.Ask{BackupName: name, Confirm: "Test", Installation: "Test",
		CallerName: "the command line"}); err != nil {
		t.Fatal(err)
	}
	if err := restore.EnterCode(ctx, e.app.Pool(), identity.SystemCaller("test"), "12345-67890"); err == nil {
		t.Fatal("a code was taken before the worker showed one")
	}
	right := shownCode(t, e.app.Pool(), g)
	if again, err := restore.Claim(ctx, e.app.Pool(), g); err != nil || again.Show != nil || again.Restore != nil {
		t.Fatalf("the code is shown again, or the restore went on without it: %+v %v", again, err)
	}
	if got := latestRestore(t, e.app.Pool()); !got.CodeShown || got.State != restore.StateRequested {
		t.Fatalf("while the code waits: %+v", got)
	}
	wrong := "00000-00000"
	if wrong == right {
		wrong = "11111-11111"
	}
	for try := 1; try <= restore.CodeTries; try++ {
		if err := restore.EnterCode(ctx, e.app.Pool(), identity.SystemCaller("test"), wrong); err != nil {
			t.Fatalf("try %d: %v", try, err)
		}
		if err := restore.EnterCode(ctx, e.app.Pool(), identity.SystemCaller("test"), right); err == nil {
			t.Fatal("a second code was taken while one waits for the worker")
		}
		c, err := restore.Claim(ctx, e.app.Pool(), g)
		if err != nil || c.Restore != nil {
			t.Fatalf("a wrong code went on: %+v %v", c, err)
		}
		if e.auditActions("backup.restore_code_wrong") != try {
			t.Fatalf("wrong code %d is not in the audit log", try)
		}
	}
	got := latestRestore(t, e.app.Pool())
	if got.State != restore.StateRefused || got.Open() || got.Changed || !strings.Contains(got.Step, "wrong") {
		t.Fatalf("after three wrong codes: %+v", got)
	}
	if e.auditActions("backup.restore_refused") != 1 || e.auditActions("backup.restore_confirmed") != 0 {
		t.Fatal("the end of the restore is not in the audit log")
	}
	entered, err := audit.List(ctx, e.app.Pool(), audit.Filter{Action: "backup.restore_code_entered", Limit: 10})
	if err != nil || len(entered) != restore.CodeTries {
		t.Fatalf("the codes entered are not in the audit log: %d %v", len(entered), err)
	}
	if text := fmt.Sprintf("%+v", entered); strings.Contains(text, strings.ReplaceAll(right, "-", "")) ||
		strings.Contains(text, strings.ReplaceAll(wrong, "-", "")) {
		t.Fatal("the code reached the audit log")
	}
}

// A code nobody entered ends the restore after half an hour.
func TestARestoreWithoutItsCodeEndsAfterHalfAnHour(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	g := testGate(t)
	name := madeBackup(t, e, time.Now().Add(-time.Hour))
	if _, err := restore.Request(ctx, e.app.Pool(), restore.Ask{BackupName: name, Confirm: "Test", Installation: "Test",
		CallerName: "the command line"}); err != nil {
		t.Fatal(err)
	}
	shownCode(t, e.app.Pool(), g)
	if _, err := e.app.Pool().Exec(ctx, "UPDATE ops.restores SET code_shown_at = now() - interval '31 minutes'"); err != nil {
		t.Fatal(err)
	}
	if c, err := restore.Claim(ctx, e.app.Pool(), g); err != nil || c.Restore != nil {
		t.Fatalf("an old request went on: %+v %v", c, err)
	}
	got := latestRestore(t, e.app.Pool())
	if got.State != restore.StateRefused || got.Open() || !strings.Contains(got.Step, "30 minutes") {
		t.Fatalf("after half an hour without a code: %+v", got)
	}
	if e.auditActions("backup.restore_refused") != 1 {
		t.Fatal("the end of the restore is not in the audit log")
	}
}

// The right code from the console goes on, recorded, and only for the restore it was made for.
func TestTheRightCodeFromTheConsoleConfirmsTheRestore(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	_, recovery := root.enrolTOTP()
	member := e.user("member@example.com")
	ctx := context.Background()
	g := testGate(t)
	name := madeBackup(t, e, time.Now().Add(-time.Hour))
	root.want(http.StatusConflict, "POST", "/api/v1/admin/backups/restore/code", map[string]any{"code": "12345-67890"})
	root.want(http.StatusAccepted, "POST", "/api/v1/admin/backups/restore",
		map[string]any{"name": name, "confirm": "Test", "code": recovery[0]})
	code := shownCode(t, e.app.Pool(), g)
	if out := root.want(http.StatusOK, "GET", "/api/v1/admin/backups", nil); !strings.Contains(out, `"code_shown":true`) {
		t.Fatalf("the console does not say a code waits: %s", out)
	}
	member.want(http.StatusForbidden, "POST", "/api/v1/admin/backups/restore/code", map[string]any{"code": code})
	root.want(http.StatusBadRequest, "POST", "/api/v1/admin/backups/restore/code", map[string]any{"code": "12345"})
	// Spaces and dashes are the reader's, not part of the code.
	root.want(http.StatusNoContent, "POST", "/api/v1/admin/backups/restore/code",
		map[string]any{"code": " " + strings.ReplaceAll(code, "-", " ") + " "})
	c, err := restore.Claim(ctx, e.app.Pool(), g)
	if err != nil || c.Restore == nil || c.Restore.State != restore.StatePreparing {
		t.Fatalf("the right code: %+v %v", c, err)
	}
	if e.auditActions("backup.restore_code_entered") != 1 || e.auditActions("backup.restore_confirmed") != 1 {
		t.Fatal("entering and confirming the code are not in the audit log")
	}
	// A code is for one restore: the code of another id or backup is another code.
	if other := g.Code(ids.New(), name); other == code {
		t.Fatal("two restores share a code")
	}
}

// What the application role writes straight into ops.restores, past the console and without
// the worker's code or seal, is refused and recorded, and changes nothing.
func TestARestoreWrittenStraightIntoTheTableIsRefused(t *testing.T) {
	e, owner := twoRoleEnv(t)
	ctx := context.Background()
	app := e.app.Pool()
	g := testGate(t)
	name := madeBackup(t, e, time.Now().Add(-time.Hour))
	f := &fakeSteps{newest: 90}
	r := runner(e, f)
	r.Pool = owner
	insert := func(state string, changed bool, safety *string) uuid.UUID {
		t.Helper()
		id := ids.New()
		if _, err := app.Exec(ctx, `INSERT INTO ops.restores (id, backup_name, requested_by_name, state, changed, safety_name, code_shown_at)
			VALUES ($1, $2, 'nobody', $3, $4, $5, now())`, id, name, state, changed, safety); err != nil {
			t.Fatalf("the application role inserts a restore: %v", err)
		}
		return id
	}
	refusals := 0
	refused := func(why string) {
		t.Helper()
		refusals++
		if got := e.auditActions("backup.restore_refused"); got != refusals {
			t.Fatalf("%s: %d refusals recorded, want %d", why, got, refusals)
		}
	}

	// A request with a code the application made up.
	insert(restore.StateRequested, false, nil)
	for range restore.CodeTries {
		if _, err := app.Exec(ctx, "UPDATE ops.restores SET confirmation = '9999999999' WHERE finished_at IS NULL"); err != nil {
			t.Fatal(err)
		}
		if c, err := restore.Claim(ctx, app, g); err != nil || c.Restore != nil {
			t.Fatalf("a made-up code went on: %+v %v", c, err)
		}
	}
	refused("a made-up code")

	// A restore written as already confirmed, past the code.
	id := insert(restore.StatePreparing, false, nil)
	if err := r.Prepare(ctx, id); !errors.Is(err, restore.ErrNotConfirmed) {
		t.Fatalf("a restore the worker never confirmed was prepared: %v", err)
	}
	if got, _ := restore.Get(ctx, owner, id); got.State != restore.StateRefused || got.Open() {
		t.Fatalf("after a restore the worker never confirmed: %+v", got)
	}
	refused("a restore past the code")

	// A confirmed restore whose backup the application changes afterwards.
	if _, err := app.Exec(ctx, "UPDATE ops.restores SET requested_at = now() - interval '2 days'"); err != nil {
		t.Fatal(err)
	}
	rs, _ := claimed(t, e)
	other := madeBackup(t, e, time.Now().Add(-3*time.Hour))
	if _, err := app.Exec(ctx, "UPDATE ops.restores SET backup_name = $1 WHERE id = $2", other, rs.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.Prepare(ctx, rs.ID); !errors.Is(err, restore.ErrNotConfirmed) {
		t.Fatalf("a restore whose backup was changed was prepared: %v", err)
	}
	refused("a changed backup")

	// A failed restore with a safety backup the worker never made, offered to the way back.
	safety := stamp(time.Now().Add(-5 * time.Hour))
	insert(restore.StateFailed, true, &safety)
	if _, err := r.WayBack(ctx); !errors.Is(err, restore.ErrNotConfirmed) {
		t.Fatalf("the way back restored a safety backup the worker never made: %v", err)
	}
	refused("a forged way back")
	for _, s := range f.ran {
		if s == "database" || s == "files" || s == "stop-others" {
			t.Fatalf("a step that changes the installation ran: %v", f.ran)
		}
	}
	// The forged failure keeps the nodes waiting: only a person decides what happened.
	if w := mustWait(t, e); w == nil || w.State != restore.StateFailed {
		t.Fatalf("the nodes stopped waiting on a failure nobody looked at: %+v", w)
	}
}
