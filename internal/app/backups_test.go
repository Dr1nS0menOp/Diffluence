// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/backups"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/health"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/settings"
)

// workerDefaults is what a worker started with DIFFLUENCE_BACKUP_INTERVAL=4h and
// DIFFLUENCE_BACKUP_KEEP=42 tells the console.
var workerDefaults = backups.Defaults{Interval: 4 * time.Hour, Keep: 42, KeyFingerprint: "0123456789abcdef"}

func stamp(at time.Time) string { return "diffluence-" + at.UTC().Format("20060102T150405Z") }

// Only administrators reach the backup page, the configuration and logging people out.
func TestOnlyAdministratorsRunTheInstallation(t *testing.T) {
	e := newEnv(t)
	member := e.user("member@example.com", identity.PermUseProduct)
	member.want(http.StatusForbidden, "GET", "/api/v1/admin/backups", nil)
	member.want(http.StatusForbidden, "POST", "/api/v1/admin/backups/request", nil)
	member.want(http.StatusForbidden, "GET", "/api/v1/admin/configuration", nil)
	member.want(http.StatusForbidden, "POST", "/api/v1/admin/sessions/end", map[string]any{"all": true})
	member.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/backups", map[string]any{"interval_hours": 1, "keep": 1})
	if n := e.auditActions("backup.requested"); n != 0 {
		t.Fatalf("a refused request was recorded as asked: %d", n)
	}
}

// "Back up now" waits for the worker, only one can wait, and it cannot be pressed in a
// loop: a backup reads the whole database and every file (spec 058, D5).
func TestBackUpNowCanWaitOnceAndIsLimitedPerHour(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	ctx := context.Background()
	root.want(http.StatusAccepted, "POST", "/api/v1/admin/backups/request", nil)
	if out := root.want(http.StatusConflict, "POST", "/api/v1/admin/backups/request", nil); !strings.Contains(out, "backup_requested") {
		t.Fatalf("a second request while one waits: %s", out)
	}
	if e.auditActions("backup.requested") != 1 {
		t.Fatal("the request is not in the audit log")
	}
	for i := 1; i < backups.RequestsPerHour; i++ {
		d, err := e.app.Backups().Next(ctx, workerDefaults)
		if err != nil || !d.Now || d.Reason != backups.ReasonRequest {
			t.Fatalf("the worker takes the request: %+v %v", d, err)
		}
		if err := e.app.Backups().ReportRun(ctx, backups.Report{Name: stamp(time.Now().Add(time.Duration(i) * time.Second)),
			Reason: backups.ReasonRequest, StartedAt: time.Now(), OK: true, SizeBytes: 10}); err != nil {
			t.Fatal(err)
		}
		root.want(http.StatusAccepted, "POST", "/api/v1/admin/backups/request", nil)
	}
	d, err := e.app.Backups().Next(ctx, workerDefaults)
	if err != nil || !d.Now {
		t.Fatalf("the last request: %+v %v", d, err)
	}
	if err := e.app.Backups().ReportRun(ctx, backups.Report{Name: stamp(time.Now().Add(time.Minute)),
		Reason: backups.ReasonRequest, StartedAt: time.Now(), OK: true}); err != nil {
		t.Fatal(err)
	}
	root.want(http.StatusServiceUnavailable, "POST", "/api/v1/admin/backups/request", nil)
}

// The worker makes a backup when none was ever tried, takes a request once, and waits on
// the console's schedule; the console's choices win over the worker's defaults.
func TestTheWorkerFollowsTheConsole(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	ctx := context.Background()
	svc := e.app.Backups()
	d, err := svc.Next(ctx, workerDefaults)
	if err != nil || !d.Now || d.Reason != backups.ReasonSchedule || d.Keep != 42 {
		t.Fatalf("a first backup is due at once, keeping the default: %+v %v", d, err)
	}
	if err := svc.ReportRun(ctx, backups.Report{Name: stamp(time.Now()), Reason: backups.ReasonSchedule,
		StartedAt: time.Now(), OK: true, SizeBytes: 2048}); err != nil {
		t.Fatal(err)
	}
	d, err = svc.Next(ctx, workerDefaults)
	if err != nil || d.Now || d.Wait <= 0 || d.Wait > time.Minute {
		t.Fatalf("after a backup the worker waits at most a minute before asking again: %+v %v", d, err)
	}

	root.ok("PUT", "/api/v1/admin/settings/backups", map[string]any{"interval_hours": 12, "keep": 7})
	d, err = svc.Next(ctx, workerDefaults)
	if err != nil || d.Keep != 7 {
		t.Fatalf("the console's keep wins: %+v %v", d, err)
	}

	root.want(http.StatusAccepted, "POST", "/api/v1/admin/backups/request", nil)
	if d, err = svc.Next(ctx, workerDefaults); err != nil || !d.Now || d.Reason != backups.ReasonRequest {
		t.Fatalf("a request is taken: %+v %v", d, err)
	}
	if d, err = svc.Next(ctx, workerDefaults); err != nil || d.Now {
		t.Fatalf("a taken request is not taken twice: %+v %v", d, err)
	}

	var page backups.Overview
	if err := json.Unmarshal([]byte(root.want(200, "GET", "/api/v1/admin/backups", nil)), &page); err != nil {
		t.Fatal(err)
	}
	if page.Worker == nil || page.Worker.Absent || !page.Worker.KeyPresent || page.Worker.KeyFingerprint != "0123456789abcdef" ||
		page.Worker.DefaultIntervalHours != 4 || page.Worker.DefaultKeep != 42 {
		t.Fatalf("the worker as the console sees it: %+v", page.Worker)
	}
	if page.Request == nil || page.Request.ClaimedAt == nil {
		t.Fatalf("the taken request shows: %+v", page.Request)
	}
	if len(page.Runs) != 1 || !page.Runs[0].OK || page.Runs[0].SizeBytes == nil || *page.Runs[0].SizeBytes != 2048 {
		t.Fatalf("the backup shows: %+v", page.Runs)
	}
	if page.Settings.IntervalHours != 12 || page.Settings.Keep != 7 {
		t.Fatalf("the settings show: %+v", page.Settings)
	}
}

// What the worker says is checked, and a message reaches the console as one plain line.
func TestAWorkerReportIsCheckedAndKeptPlain(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := e.app.Backups()
	for name, r := range map[string]backups.Report{
		"a name that is not a backup": {Name: "../etc", Reason: backups.ReasonSchedule, StartedAt: time.Now(), OK: true},
		"an unknown reason":           {Name: stamp(time.Now()), Reason: "whim", StartedAt: time.Now(), OK: true},
		"no start":                    {Name: stamp(time.Now()), Reason: backups.ReasonSchedule, OK: true},
		"a negative size":             {Name: stamp(time.Now()), Reason: backups.ReasonSchedule, StartedAt: time.Now(), OK: true, SizeBytes: -1},
	} {
		if err := svc.ReportRun(ctx, r); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	if _, err := svc.Next(ctx, backups.Defaults{Interval: time.Second, Keep: 1}); err == nil {
		t.Fatal("an interval under a minute was accepted")
	}
	if _, err := svc.Next(ctx, backups.Defaults{Interval: time.Hour, Keep: 1, KeyFingerprint: "not hex"}); err == nil {
		t.Fatal("a fingerprint that is not hexadecimal was accepted")
	}
	long := "failed while dumping\nthe database\x07<script>" + strings.Repeat("x", 600)
	if err := svc.ReportRun(ctx, backups.Report{Name: stamp(time.Now()), Reason: backups.ReasonSchedule,
		StartedAt: time.Now(), Message: long}); err != nil {
		t.Fatal(err)
	}
	page, err := svc.Overview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	msg := page.Runs[0].Message
	if strings.ContainsAny(msg, "\n\r\x07") || len([]rune(msg)) > 300 || page.Runs[0].OK || page.Runs[0].SizeBytes != nil {
		t.Fatalf("a failed run with a long message: %+v", page.Runs[0])
	}
	if e.auditActions("backup.failed") != 1 {
		t.Fatal("the failure is not in the audit log")
	}
}

// A failed backup and a failed test restore show on the health page, even while an older
// backup is recent (spec 058).
func TestFailedBackupsAndRestoresShowOnTheHealthPage(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	svc := e.app.Backups()
	if err := app.NoteBackup(ctx, e.app.Pool()); err != nil {
		t.Fatal(err)
	}
	read := func() string {
		t.Helper()
		f, err := app.ReadHealthFacts(ctx, app.HealthSource{Pool: e.app.Pool(), Migrations: mustMigrations(t)}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range health.Evaluate(f) {
			if c.Name == "backup" {
				return c.Code
			}
		}
		t.Fatal("no backup check")
		return ""
	}
	if got := read(); got != "backup_ok" {
		t.Fatalf("a noted backup: %s", got)
	}
	if err := svc.ReportRun(ctx, backups.Report{Name: stamp(time.Now()), Reason: backups.ReasonSchedule,
		StartedAt: time.Now(), Message: "failed while dumping the database"}); err != nil {
		t.Fatal(err)
	}
	if got := read(); got != "backup_failed" {
		t.Fatalf("after a failed backup: %s", got)
	}
	if err := svc.ReportRun(ctx, backups.Report{Name: stamp(time.Now().Add(time.Second)), Reason: backups.ReasonSchedule,
		StartedAt: time.Now(), OK: true}); err != nil {
		t.Fatal(err)
	}
	if got := read(); got != "backup_ok" {
		t.Fatalf("a good backup after a failed one: %s", got)
	}
	if err := svc.ReportRestore(ctx, backups.RestoreReport{Name: stamp(time.Now()), Seconds: 9, Message: "did not restore"}); err != nil {
		t.Fatal(err)
	}
	if got := read(); got != "restore_check_failed" {
		t.Fatalf("after a failed restore: %s", got)
	}
	if err := svc.ReportRestore(ctx, backups.RestoreReport{Name: stamp(time.Now()), OK: true, Seconds: 9}); err != nil {
		t.Fatal(err)
	}
	if got := read(); got != "backup_ok" {
		t.Fatalf("a restore that passed: %s", got)
	}
	if e.auditActions("backup.restore_checked") != 2 {
		t.Fatal("restore checks are not in the audit log")
	}
}

// sentMail is a mailer that keeps what it was asked to send.
type sentMail struct{ to []identity.AccountMail }

func (s *sentMail) Send(_ context.Context, _ db.Querier, m identity.AccountMail) error {
	s.to = append(s.to, m)
	return nil
}

// Administrators get a mail when a backup fails, but at most once in 12 hours per kind:
// a backup that fails every few hours must not fill their inbox.
func TestAFailedBackupMailsTheAdministratorsAtMostTwiceADay(t *testing.T) {
	e := newEnv(t)
	admin(e)
	e.user("member@example.com", identity.PermUseProduct)
	ctx := context.Background()
	mail := &sentMail{}
	svc := backups.NewService(e.app.Pool(), settings.NewService(e.app.Pool(), nil), mail, "https://wiki.example.com/admin/backups")
	if err := svc.MailFailures(ctx); err != nil || len(mail.to) != 0 {
		t.Fatalf("nothing failed, nothing mailed: %v %v", mail.to, err)
	}
	fail := func(at time.Time) {
		t.Helper()
		if err := svc.ReportRun(ctx, backups.Report{Name: stamp(at), Reason: backups.ReasonSchedule, StartedAt: time.Now(),
			Message: "failed while packing the stored files"}); err != nil {
			t.Fatal(err)
		}
	}
	fail(time.Now())
	if err := svc.MailFailures(ctx); err != nil {
		t.Fatal(err)
	}
	if len(mail.to) != 1 || mail.to[0].To != "admin@example.com" || mail.to[0].Kind != backups.MailBackupFailed ||
		mail.to[0].Link != "https://wiki.example.com/admin/backups" {
		t.Fatalf("only the administrator is mailed, with the page: %+v", mail.to)
	}
	fail(time.Now().Add(time.Second))
	if err := svc.MailFailures(ctx); err != nil || len(mail.to) != 1 {
		t.Fatalf("a second failure within 12 hours is folded in: %+v %v", mail.to, err)
	}
	if err := svc.ReportRestore(ctx, backups.RestoreReport{Name: stamp(time.Now()), Message: "did not restore"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.MailFailures(ctx); err != nil || len(mail.to) != 2 || mail.to[1].Kind != backups.MailRestoreCheckFailed {
		t.Fatalf("a failed restore is its own kind: %+v %v", mail.to, err)
	}
	if _, err := e.app.Pool().Exec(ctx, "UPDATE backup_mail SET sent_at = now() - interval '13 hours'"); err != nil {
		t.Fatal(err)
	}
	fail(time.Now().Add(2 * time.Second))
	if err := svc.MailFailures(ctx); err != nil || len(mail.to) != 3 {
		t.Fatalf("after 12 hours a new failure mails again: %+v %v", mail.to, err)
	}
}

// The configuration shows where each setting came from, and never a secret.
func TestTheConsoleShowsTheConfigurationWithoutSecrets(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	t.Setenv("DIFFLUENCE_SMTP_PASSWORD", "very-secret-mail-password")
	t.Setenv("DIFFLUENCE_DATABASE_URL", "postgres://app:very-secret-db-password@db.example.com:5432/diffluence")
	out := root.want(200, "GET", "/api/v1/admin/configuration", nil)
	if strings.Contains(out, "very-secret") {
		t.Fatalf("a secret is shown: %s", out)
	}
	var page struct {
		Settings []struct{ Name, Value, Source string } `json:"settings"`
	}
	if err := json.Unmarshal([]byte(out), &page); err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, s := range page.Settings {
		found[s.Name] = s.Value + " | " + s.Source
	}
	if found["SMTP_PASSWORD"] != "(set) | environment" || !strings.Contains(found["DATABASE_URL"], "db.example.com") {
		t.Fatalf("the secrets as the page shows them: %q %q", found["SMTP_PASSWORD"], found["DATABASE_URL"])
	}
}

// An administrator can log one person out everywhere, or everyone, but never everyone by
// leaving out the address; each is in the audit log.
func TestAnAdministratorLogsPeopleOutFromTheConsole(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	root.want(http.StatusBadRequest, "POST", "/api/v1/admin/sessions/end", map[string]any{})
	root.want(http.StatusBadRequest, "POST", "/api/v1/admin/sessions/end", map[string]any{"email": "anna@example.com", "all": true})
	root.want(http.StatusNotFound, "POST", "/api/v1/admin/sessions/end", map[string]any{"email": "nobody@example.com"})
	root.ok("POST", "/api/v1/admin/sessions/end", map[string]any{"email": "anna@example.com"})
	anna.want(http.StatusUnauthorized, "GET", "/api/v1/me/security", nil)
	root.want(200, "GET", "/api/v1/admin/backups", nil)
	if e.auditActions("sessions.ended") != 1 {
		t.Fatal("logging out is not in the audit log")
	}
	root.ok("POST", "/api/v1/admin/sessions/end", map[string]any{"all": true})
	root.want(http.StatusUnauthorized, "GET", "/api/v1/admin/backups", nil)
}
