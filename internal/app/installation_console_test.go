// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
)

// Wave CS1 of spec 075: the console shows what runs the installation, and changes the log
// level for a while and the disk thresholds. Who may see and change each area first, then
// that no answer holds a secret, then what each area does.

// installationReads are the areas every administrator may read (CSL-610).
var installationReads = []string{
	"/api/v1/admin/upgrades",
	"/api/v1/admin/events",
	"/api/v1/admin/audit-overview",
	"/api/v1/admin/monitoring",
	"/api/v1/admin/monitoring/rules",
	"/api/v1/admin/monitoring/dashboard",
	"/api/v1/admin/log-level",
	"/api/v1/admin/sessions/count",
	"/api/v1/admin/configuration",
	"/api/v1/admin/settings/health",
}

// installationWrites are the changes of the wave, with a body that would be valid.
var installationWrites = []struct {
	method, path string
	body         map[string]any
}{
	{"PUT", "/api/v1/admin/log-level", map[string]any{"level": "debug", "hours": 1}},
	{"DELETE", "/api/v1/admin/log-level", nil},
	{"PUT", "/api/v1/admin/settings/health", map[string]any{"disk_warn_percent": 20, "disk_refuse_percent": 5}},
}

func TestAMemberReachesNoAreaOfTheInstallation(t *testing.T) {
	e := newEnv(t)
	member := e.user("member@example.com", identity.PermUseProduct)
	for _, path := range installationReads {
		member.want(http.StatusForbidden, "GET", path, nil)
	}
	for _, w := range installationWrites {
		member.want(http.StatusForbidden, w.method, w.path, w.body)
	}
	member.want(http.StatusForbidden, "PUT", "/api/v1/admin/log-level/confirmed", map[string]any{"level": "error", "hours": 1})
	if n := e.auditActions("settings.log_level_set"); n != 0 {
		t.Fatalf("a refused change was recorded: %d", n)
	}
}

// A token is for scripts: it never reaches the console's areas (CSL-1015).
func TestATokenReachesNoAreaOfTheInstallation(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	token := root.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "script"})["secret"].(string)
	for _, path := range installationReads {
		if status, out := e.bearer("GET", path, token, ""); status != http.StatusForbidden || !strings.Contains(out, "session_only") {
			t.Errorf("GET %s with a token: %d %s", path, status, out)
		}
	}
	for _, w := range installationWrites {
		body, _ := json.Marshal(w.body)
		if status, _ := e.bearer(w.method, w.path, token, string(body)); status != http.StatusForbidden {
			t.Errorf("%s %s with a token: %d", w.method, w.path, status)
		}
	}
}

// What lets more through is a system administrator's, with a fresh login and a second
// factor (spec 076): a quieter log level, lower disk thresholds.
func TestAnAdministratorCannotMakeTheInstallationQuieterOrFuller(t *testing.T) {
	e := newEnv(t)
	clerk := e.user("clerk@example.com", identity.PermUseProduct, identity.PermAdministrator)
	_, recovery := clerk.enrolTOTP()
	if out := clerk.want(http.StatusForbidden, "PUT", "/api/v1/admin/log-level", map[string]any{"level": "error", "hours": 1}); !strings.Contains(out, "confirmation_required") {
		t.Fatalf("a quieter level as an administrator: %s", out)
	}
	clerk.want(http.StatusForbidden, "PUT", "/api/v1/admin/log-level/confirmed", map[string]any{"level": "error", "hours": 1, "code": recovery[0]})
	if out := clerk.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/health", map[string]any{"disk_warn_percent": 5, "disk_refuse_percent": 1}); !strings.Contains(out, "confirmation_required") {
		t.Fatalf("lower thresholds as an administrator: %s", out)
	}
	clerk.want(http.StatusForbidden, "PUT", "/api/v1/admin/settings/health/confirmed",
		confirmed(map[string]any{"disk_warn_percent": 5, "disk_refuse_percent": 1}, recovery[0], ""))
	if got := clerk.ok("GET", "/api/v1/admin/settings/health", nil); got["disk_warn_percent"] != float64(0) {
		t.Fatalf("a refused change was stored: %v", got)
	}
	// Louder, and sooner, stay an administrator's.
	clerk.ok("PUT", "/api/v1/admin/log-level", map[string]any{"level": "debug", "hours": 4})
	clerk.ok("PUT", "/api/v1/admin/settings/health", map[string]any{"disk_warn_percent": 20, "disk_refuse_percent": 5})
}

func TestASystemAdministratorConfirmsAQuieterLevelWithASecondFactor(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	_, recovery := root.enrolTOTP()
	root.want(http.StatusForbidden, "PUT", "/api/v1/admin/log-level/confirmed", map[string]any{"level": "error", "hours": 1, "code": "000000"})
	if n := e.auditActions("settings.log_level_set"); n != 0 {
		t.Fatalf("a wrong second factor stored a level: %d", n)
	}
	e.sql("UPDATE sessions SET created_at = now() - interval '11 minutes', reauth_at = NULL WHERE user_id = $1", root.id)
	if out := root.want(http.StatusForbidden, "PUT", "/api/v1/admin/log-level/confirmed",
		map[string]any{"level": "error", "hours": 1, "code": recovery[0]}); !strings.Contains(out, "reauth_required") {
		t.Fatalf("an old login: %s", out)
	}
	root.ok("POST", "/api/v1/auth/reauth", map[string]any{"password": adminPassword})
	root.ok("PUT", "/api/v1/admin/log-level/confirmed", map[string]any{"level": "error", "hours": 1, "code": recovery[1]})
	root.ok("PUT", "/api/v1/admin/settings/health/confirmed",
		confirmed(map[string]any{"disk_warn_percent": 5, "disk_refuse_percent": 1}, recovery[2], ""))
	if e.auditActions("settings.log_level_set") != 1 || e.auditActions("settings.updated") == 0 {
		t.Fatal("the confirmed changes are not in the audit log")
	}
}

// No answer of these areas holds a secret, or any part of one: only that it is set and where
// it comes from (CSL-304, CSL-604).
func TestNoAreaOfTheInstallationShowsASecret(t *testing.T) {
	const secret = "planted-installation-secret"
	t.Setenv("DIFFLUENCE_SMTP_PASSWORD", secret+"-mail")
	t.Setenv("DIFFLUENCE_METRICS_TOKEN", secret+"-metrics")
	t.Setenv("DIFFLUENCE_DATABASE_URL", "postgres://app:"+secret+"@db.example.com:5432/diffluence")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "authorization=Bearer "+secret)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "https://otel:"+secret+"@collector.example.com/?token="+secret)
	e := newEnvWith(t, func(c *config.Config) { c.MetricsToken = secret + "-metrics" })
	root := admin(e)
	// Changed after the start, so the page holds both the running and the next value.
	t.Setenv("DIFFLUENCE_SMTP_PASSWORD", secret+"-mail-next")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "authorization=Bearer "+secret+"-next")
	for _, path := range installationReads {
		if out := root.want(http.StatusOK, "GET", path, nil); strings.Contains(out, secret) {
			t.Errorf("%s shows a secret: %s", path, out)
		}
	}
	out := root.want(http.StatusOK, "GET", "/api/v1/admin/monitoring", nil)
	if !strings.Contains(out, `"metrics_on":true`) {
		t.Fatalf("the page says /metrics is on: %s", out)
	}
}

// The configuration marks what changes at the next restart, shows the variables of the trace
// exporter and the last try of the outside settings, and has no way to change any of it
// (CSL-601, CSL-604, CSL-608, CSL-609).
func TestTheConfigurationShowsWhatChangesAtTheNextRestart(t *testing.T) {
	t.Setenv("DIFFLUENCE_LOG_LEVEL", "info")
	t.Setenv("OTEL_SERVICE_NAME", "diffluence-test")
	e := newEnv(t)
	root := admin(e)
	probes := &config.ProbeRecord{}
	e.app.UseProbes(probes)
	t.Setenv("DIFFLUENCE_LOG_LEVEL", "debug")
	var page struct {
		Settings []struct {
			Name             string `json:"name"`
			Value            string `json:"value"`
			Source           string `json:"source"`
			ChangesAtRestart bool   `json:"changes_at_restart"`
			Running          *struct {
				Value string `json:"value"`
			} `json:"running"`
		} `json:"settings"`
		Tracing []struct{ Name, Value, Source string } `json:"tracing"`
		Probe   struct {
			Done     bool `json:"done"`
			Warnings []struct {
				Setting string `json:"setting"`
				Message string `json:"message"`
			} `json:"warnings"`
		} `json:"probe"`
	}
	read := func() {
		t.Helper()
		if err := json.Unmarshal([]byte(root.want(http.StatusOK, "GET", "/api/v1/admin/configuration", nil)), &page); err != nil {
			t.Fatal(err)
		}
	}
	read()
	found := false
	for _, s := range page.Settings {
		if s.Name != "LOG_LEVEL" {
			continue
		}
		found = true
		if !s.ChangesAtRestart || s.Value != "debug" || s.Running == nil || s.Running.Value != "info" {
			t.Fatalf("the level changed after the start: %+v", s)
		}
	}
	if !found || len(page.Tracing) != 1 || page.Tracing[0].Value != "diffluence-test" || page.Tracing[0].Source != "environment" {
		t.Fatalf("level found %v, tracing %+v", found, page.Tracing)
	}
	if page.Probe.Done {
		t.Fatal("no try ran yet")
	}
	probes.Keep(time.Now(), []config.Warning{{Setting: "SMTP_HOST", Message: "the mail server does not accept a connection on port 25"}})
	read()
	if !page.Probe.Done || len(page.Probe.Warnings) != 1 || page.Probe.Warnings[0].Setting != "SMTP_HOST" {
		t.Fatalf("the last try: %+v", page.Probe)
	}
	for _, r := range e.app.Routes() {
		if strings.Contains(r.Pattern, "/admin/configuration") && !strings.HasPrefix(r.Pattern, "GET ") {
			t.Fatalf("a route writes the configuration: %s", r.Pattern)
		}
	}
}

// The log level holds on every node within the reread, for its time only, and each node says
// the level it runs at (CSL-570 to CSL-572).
func TestTheLogLevelChangesForAWhileAndReturnsByItself(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	ctx := context.Background()
	level := new(slog.LevelVar)
	e.app.ControlLogLevel(level)
	if err := e.app.RegisterNode(ctx); err != nil {
		t.Fatal(err)
	}
	root.ok("PUT", "/api/v1/admin/log-level", map[string]any{"level": "debug", "hours": 1})
	now := time.Now()
	if err := e.app.ApplyRuntime(ctx, now); err != nil {
		t.Fatal(err)
	}
	if level.Level() != slog.LevelDebug || nodeLevel(t, e) != "debug" {
		t.Fatalf("within the reread: %v, node says %q", level.Level(), nodeLevel(t, e))
	}
	state := root.ok("GET", "/api/v1/admin/log-level", nil)
	if state["override"] == nil || state["configured"] != "info" {
		t.Fatalf("the area: %v", state)
	}
	// An hour later the configured level is back, without anyone asking.
	if err := e.app.ApplyRuntime(ctx, now.Add(time.Hour+time.Second)); err != nil {
		t.Fatal(err)
	}
	if level.Level() != slog.LevelInfo || nodeLevel(t, e) != "info" {
		t.Fatalf("after its hour: %v, node says %q", level.Level(), nodeLevel(t, e))
	}

	// For one node only: another node is not this one.
	root.ok("PUT", "/api/v1/admin/log-level", map[string]any{"level": "debug", "hours": 4, "node": "0195b4f2-8d40-7000-8000-000000000001"})
	if err := e.app.ApplyRuntime(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if level.Level() != slog.LevelInfo {
		t.Fatal("an override for another node held here")
	}
	var id string
	if err := e.app.Pool().QueryRow(ctx, "SELECT id::text FROM nodes").Scan(&id); err != nil {
		t.Fatal(err)
	}
	root.ok("PUT", "/api/v1/admin/log-level", map[string]any{"level": "debug", "hours": 4, "node": id})
	if err := e.app.ApplyRuntime(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if level.Level() != slog.LevelDebug {
		t.Fatal("an override for this node did not hold")
	}
	root.want(http.StatusNoContent, "DELETE", "/api/v1/admin/log-level", nil)
	if err := e.app.ApplyRuntime(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if level.Level() != slog.LevelInfo {
		t.Fatal("ending the override early did not return the configured level")
	}
	root.want(http.StatusConflict, "DELETE", "/api/v1/admin/log-level", nil)
	if e.auditActions("settings.log_level_set") != 3 || e.auditActions("settings.log_level_ended") != 1 {
		t.Fatal("each change of the level is in the audit log")
	}
	root.want(http.StatusBadRequest, "PUT", "/api/v1/admin/log-level", map[string]any{"level": "debug", "hours": 48})
}

func nodeLevel(t *testing.T, e *env) string {
	t.Helper()
	if err := e.app.RegisterNode(context.Background()); err != nil {
		t.Fatal(err)
	}
	var level string
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT log_level FROM nodes").Scan(&level); err != nil {
		t.Fatal(err)
	}
	return level
}

// Debug never logs a secret or the words of a page (CSL-573), and the lines an operator builds
// on are written at every level (CSL-574).
func TestDebugLogsNoSecretOrTitleAndErrorKeepsTheWitness(t *testing.T) {
	logs := &lockedBuffer{}
	level := new(slog.LevelVar)
	e := newEnvLogging(t, func(*config.Config) {}, slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: level})))
	e.app.ControlLogLevel(level)
	ctx := context.Background()
	root := admin(e)
	root.ok("PUT", "/api/v1/admin/log-level", map[string]any{"level": "debug", "hours": 1})
	if err := e.app.ApplyRuntime(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	const title = "Plantedtitle Quarterly"
	const words = "plantedbody words of the page"
	space, _ := root.space("DBG")
	page := root.page(space, nil, title, words)
	root.ok("GET", "/api/v1/pages/"+page, nil)
	root.want(http.StatusOK, "GET", "/api/v1/search?q=Plantedtitle", nil)
	e.do(e.client(), "GET", "/livez", "")
	token := root.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "planted"})["secret"].(string)
	e.bearer("GET", "/api/v1/me", token, "")
	text := logs.String()
	if !strings.Contains(text, `"level":"DEBUG"`) {
		t.Fatal("nothing was logged at debug, so the test proves nothing")
	}
	for what, planted := range map[string]string{"the title": "Plantedtitle", "the page's words": "plantedbody", "a token": token,
		"the password": adminPassword} {
		if strings.Contains(text, planted) {
			t.Errorf("%s is in the log at debug", what)
		}
	}

	// At error, the witness of the audit log is still written.
	_, recovery := root.enrolTOTP()
	root.ok("PUT", "/api/v1/admin/log-level/confirmed", map[string]any{"level": "error", "hours": 1, "code": recovery[0]})
	if err := e.app.ApplyRuntime(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if level.Level() != slog.LevelError {
		t.Fatalf("level %v", level.Level())
	}
	before := len(e.witness(logs))
	e.createUser("bo@example.com", identity.PermUseProduct)
	if err := e.app.SignCheckpointForTest(ctx); err != nil {
		t.Fatal(err)
	}
	if got := len(e.witness(logs)); got != before+1 {
		t.Fatalf("the witness line at level error: %d, want %d", got, before+1)
	}
}

// The disk thresholds are bounded settings that the health rules and the store follow
// (CSL-581).
func TestTheDiskThresholdsAreBoundedAndFollowed(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	ctx := context.Background()
	for _, bad := range []map[string]any{
		{"disk_warn_percent": 51, "disk_refuse_percent": 2},
		{"disk_warn_percent": 20, "disk_refuse_percent": 11},
		{"disk_warn_percent": 10, "disk_refuse_percent": 10},
	} {
		root.want(http.StatusBadRequest, "PUT", "/api/v1/admin/settings/health", bad)
	}
	root.ok("PUT", "/api/v1/admin/settings/health", map[string]any{"disk_warn_percent": 20, "disk_refuse_percent": 5})
	if err := e.app.ApplyRuntime(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	e.app.StoreGuard().UseRoomOf(fixedRoom{free: 4, total: 100})
	space, _ := root.space("DSK")
	page := root.page(space, nil, "Files", "x")
	if status, out := root.upload(page, "a.txt", "hello"); status < 400 || !strings.Contains(out, "no_room") {
		t.Fatalf("an upload at 4%% free with the refuse level at 5%%: %d %s", status, out)
	}
	e.app.StoreGuard().UseRoomOf(fixedRoom{free: 6, total: 100})
	if status, out := root.upload(page, "b.txt", "hello"); status >= 400 {
		t.Fatalf("an upload at 6%% free: %d %s", status, out)
	}
	facts, err := e.app.HealthFacts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Thresholds.DiskWarnPercent != 20 || facts.Thresholds.DiskRefusePercent != 5 {
		t.Fatalf("the health rules read %+v", facts.Thresholds)
	}
}

// The backup line follows the schedule: every 4 hours, so it warns after 6 (CSL-582).
func TestTheBackupLineFollowsTheBackupSchedule(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	checkedIn(t, e, workerDefaults)
	facts, err := app.ReadHealthFacts(ctx, app.HealthSource{Pool: e.app.Pool(), Store: e.app.StoreGuard(), Backups: e.app.Backups()}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if facts.BackupInterval != 4*time.Hour {
		t.Fatalf("the worker's default of 4 hours: %v", facts.BackupInterval)
	}
	if _, err := e.app.Backups().SetScheduleConfirmed(ctx, identity.SystemCaller("test"),
		strings.NewReader(`{"interval_hours": 12, "keep": 0}`)); err != nil {
		t.Fatal(err)
	}
	facts, err = app.ReadHealthFacts(ctx, app.HealthSource{Pool: e.app.Pool(), Store: e.app.StoreGuard(), Backups: e.app.Backups()}, time.Now())
	if err != nil || facts.BackupInterval != 12*time.Hour {
		t.Fatalf("the schedule of 12 hours: %v %v", facts.BackupInterval, err)
	}
}

// The audit area shows the newest checkpoint, the keys, and the newest witness report; a
// failed report is a problem on the audit line (CSL-530 to CSL-533).
func TestTheAuditAreaShowsTheWitnessReport(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	ctx := context.Background()
	if err := e.app.SignCheckpointForTest(ctx); err != nil {
		t.Fatal(err)
	}
	var area struct {
		Checkpoint *struct{ Seq int64 } `json:"checkpoint"`
		Keys       []struct {
			Installation bool `json:"installation"`
			Endorsed     bool `json:"endorsed"`
		} `json:"keys"`
		Witness *audit.WitnessCheck `json:"witness"`
	}
	if err := json.Unmarshal([]byte(root.want(http.StatusOK, "GET", "/api/v1/admin/audit-overview", nil)), &area); err != nil {
		t.Fatal(err)
	}
	if area.Checkpoint == nil || len(area.Keys) != 1 || !area.Keys[0].Installation || area.Witness != nil {
		t.Fatalf("before any report: %+v", area)
	}
	if err := audit.RecordWitnessCheck(ctx, e.app.Pool(), audit.WitnessCheck{FirstBadSeq: 7, Reason: "the chain differs"}); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(root.want(http.StatusOK, "GET", "/api/v1/admin/audit-overview", nil)), &area); err != nil {
		t.Fatal(err)
	}
	if area.Witness == nil || area.Witness.OK || area.Witness.FirstBadSeq != 7 {
		t.Fatalf("the failed report: %+v", area.Witness)
	}
	facts, err := app.ReadHealthFacts(ctx, app.HealthSource{Pool: e.app.Pool(), Store: e.app.StoreGuard()}, time.Now())
	if err != nil || !facts.WitnessReported || facts.WitnessOK {
		t.Fatalf("the health facts: %+v %v", facts, err)
	}
}

// The upgrade page shows the nodes, the release, and a contract migration that waits for a
// node that does not know it yet, with the end of its grace (CSL-560, CSL-561).
func TestTheUpgradePageNamesTheNodeAContractMigrationWaitsFor(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.app.RegisterNode(ctx); err != nil {
		t.Fatal(err)
	}
	migs, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	newest := migs[len(migs)-1].Version
	waiting := db.Migration{Version: newest + 1, Name: "drop_old_column", Phase: db.PhaseContract}
	e.sql(`INSERT INTO nodes (id, role, version, max_migration, started_at, last_seen_at)
		VALUES (gen_random_uuid(), 'api', 'v0.9-old', $1, now() - interval '3 hours', now())`, newest)
	page, err := app.ReadUpgrades(ctx, e.app.Pool(), app.UpgradeSource{Migrations: append(migs, waiting),
		Grace: 24 * time.Hour, GraceSource: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Nodes) != 2 || page.Newest != newest || len(page.Contract) != 1 {
		t.Fatalf("the page: %+v", page)
	}
	c := page.Contract[0]
	if c.Name != "drop_old_column" || len(c.NodesBehind) != 2 || c.GraceEnds == nil {
		t.Fatalf("the waiting migration: %+v", c)
	}
	if page.Grace.Value != "24h0m0s" || page.Grace.Source != "default" {
		t.Fatalf("the grace with its source: %+v", page.Grace)
	}
	// Once every node knows it, none is named; the grace still runs from the youngest start.
	e.sql("UPDATE nodes SET max_migration = $1", newest+1)
	page, err = app.ReadUpgrades(ctx, e.app.Pool(), app.UpgradeSource{Migrations: append(migs, waiting), Grace: 24 * time.Hour})
	if err != nil || len(page.Contract[0].NodesBehind) != 0 || page.Contract[0].GraceEnds == nil {
		t.Fatalf("every node knows it: %+v %v", page.Contract, err)
	}
	root := admin(e)
	out := root.want(http.StatusOK, "GET", "/api/v1/admin/upgrades", nil)
	for _, part := range []string{`"nodes"`, `"contract":[]`, `"postgres"`, `"marker"`} {
		if !strings.Contains(out, part) {
			t.Errorf("the page lacks %s: %s", part, out)
		}
	}
}

// The events area says what diffluence events status says (CSL-590).
func TestTheEventsAreaShowsTheTransport(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	got := root.ok("GET", "/api/v1/admin/events", nil)
	if got["transport"] == "" || got["changed_at"] == nil || got["older_nodes"] == nil {
		t.Fatalf("the area: %v", got)
	}
	if _, ok := got["problem"].(string); !ok {
		t.Fatalf("the release without NATS, in words: %v", got)
	}
}

// The files the console offers are the release's own (CSL-585 to CSL-587).
func TestTheMonitoringFilesAreTheReleasesOwn(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	for kind, name := range map[string]string{"rules": "diffluence-rules.yml", "dashboard": "diffluence-dashboard.json"} {
		resp, body := e.do(root.c, "GET", "/api/v1/admin/monitoring/"+kind, "")
		want, err := os.ReadFile("../../deploy/monitoring/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK || body != string(want) {
			t.Fatalf("%s: %d, equal %v", kind, resp.StatusCode, body == string(want))
		}
		if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, name) {
			t.Fatalf("%s is a download: %q", kind, cd)
		}
	}
	root.want(http.StatusNotFound, "GET", "/api/v1/admin/monitoring/README", nil)
	if out := root.want(http.StatusOK, "GET", "/api/v1/admin/monitoring", nil); !strings.Contains(out, `"metrics_on":false`) {
		t.Fatalf("without a token /metrics is off: %s", out)
	}
}

// Before everyone is logged out, the dialog says how many sessions of how many people end
// (CSL-541).
func TestTheCountBeforeLoggingEveryoneOut(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	e.user("anna@example.com", identity.PermUseProduct)
	anna := e.client()
	if e.login(anna, "anna@example.com", adminPassword) != http.StatusOK {
		t.Fatal("second login")
	}
	got := root.ok("GET", "/api/v1/admin/sessions/count", nil)
	if got["sessions"] != float64(3) || got["people"] != float64(2) {
		t.Fatalf("the count: %v", got)
	}
}
