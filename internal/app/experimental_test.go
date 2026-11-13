// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
)

// Automation rules, databases, bundles and restore from the console are experimental for
// 1.0 (spec 069, ADR-043): off on a new installation, on after an upgrade that used them.

const experimentalMigration = 54

func noSettings(*config.Config) {}

func experimentalOn(u *user) []string {
	u.e.t.Helper()
	raw, _ := u.ok("GET", "/api/v1/admin/experimental", nil)["on"].([]any)
	out := []string{}
	for _, part := range raw {
		out = append(out, part.(string))
	}
	slices.Sort(out)
	return out
}

func databaseBody(space string) map[string]any {
	return map[string]any{"space_id": space, "title": "Sums", "kind": "database"}
}

func TestANewInstallationRefusesEveryExperimentalPart(t *testing.T) {
	e := newInstalledEnv(t, noSettings)
	root := admin(e)
	space, _ := root.space("EXP")
	page := root.page(space, nil, "Plan", "x")
	rule := map[string]any{"name": "Tag", "enabled": true, "definition": onComment(map[string]any{"type": "add_label", "label": "x"})}
	refused := []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/v1/spaces/" + space + "/rules", rule},
		{"GET", "/api/v1/spaces/" + space + "/rules", nil},
		{"GET", "/api/v1/admin/rules", nil},
		{"POST", "/api/v1/admin/rules", rule},
		{"GET", "/api/v1/admin/automation", nil},
		{"POST", "/hooks/" + strings.Repeat("a", 64), "{}"},
		{"POST", "/api/v1/pages", databaseBody(space)},
		{"POST", "/api/v1/spaces/" + space + "/bundles", nil},
		{"GET", "/api/v1/admin/bundles/key", nil},
		{"POST", "/api/v1/admin/bundles?filename=a.zip", "x"},
		{"POST", "/api/v1/admin/backups/restore", map[string]any{"name": "diffluence-20260101T000000Z", "confirm": "Test", "code": "123456"}},
	}
	for _, r := range refused {
		if out := root.want(http.StatusNotFound, r.method, r.path, r.body); !strings.Contains(out, "experimental_off") {
			t.Errorf("%s %s does not say the part is off: %s", r.method, r.path, out)
		}
	}
	// Every page asks which rules it can run, and hears: none.
	if rules, _ := root.ok("GET", "/api/v1/pages/"+page+"/rules", nil)["rules"].([]any); len(rules) != 0 {
		t.Fatalf("rules of a page while automation is off: %v", rules)
	}
	if n := e.count("SELECT count(*) FROM pages WHERE kind = 'database'"); n != 0 {
		t.Fatalf("%d databases were made while databases are off", n)
	}
}

func TestANewInstallationStartsWithEveryExperimentalPartOff(t *testing.T) {
	e := newInstalledEnv(t, noSettings)
	root := admin(e)
	if on := experimentalOn(root); len(on) != 0 {
		t.Fatalf("on at the start: %v", on)
	}
	capabilities := root.ok("GET", "/api/v1/system/capabilities", nil)
	for _, part := range []string{"automation", "databases", "bundles", "console_restore"} {
		if capabilities[part] != false {
			t.Errorf("the screens offer %s: %v", part, capabilities[part])
		}
	}
	// Analytics and public sites keep the switches they had, which start off.
	if on := root.ok("GET", "/api/v1/admin/analytics", nil)["enabled"]; on != false {
		t.Fatalf("analytics at the start: %v", on)
	}
	limits, _ := root.ok("GET", "/api/v1/admin/export-policy", nil)["limits"].(map[string]any)
	if limits["public_sites_enabled"] != false {
		t.Fatalf("public sites at the start: %v", limits)
	}
}

func TestOnlyAnExperimentalPartCanBeTurnedOn(t *testing.T) {
	e := newInstalledEnv(t, noSettings)
	root := admin(e)
	root.want(http.StatusBadRequest, "PUT", "/api/v1/admin/experimental", map[string]any{"on": []string{"databases", "teleport"}})
	member := e.user("member@example.com", identity.PermUseProduct)
	if status, _ := member.do("PUT", "/api/v1/admin/experimental", map[string]any{"on": []string{"databases"}}); status != http.StatusForbidden {
		t.Fatalf("a member turned a part on: %d", status)
	}
	if on := experimentalOn(root); len(on) != 0 {
		t.Fatalf("on after two refusals: %v", on)
	}
}

func TestAnAdministratorTurnsOnOneExperimentalPart(t *testing.T) {
	e := newInstalledEnv(t, noSettings)
	root := admin(e)
	space, _ := root.space("EXP")
	before := e.auditActions("settings.updated")
	root.want(http.StatusNoContent, "PUT", "/api/v1/admin/experimental", map[string]any{"on": []string{"databases", "databases"}})
	if on := experimentalOn(root); !slices.Equal(on, []string{"databases"}) {
		t.Fatalf("on: %v", on)
	}
	if e.auditActions("settings.updated") != before+1 {
		t.Fatal("turning a part on is not in the audit log")
	}
	root.ok("POST", "/api/v1/pages", databaseBody(space))
	if capabilities := root.ok("GET", "/api/v1/system/capabilities", nil); capabilities["databases"] != true || capabilities["automation"] != false {
		t.Fatalf("capabilities: %v", capabilities)
	}
	root.want(http.StatusNotFound, "GET", "/api/v1/admin/rules", nil)
}

// Turning a part off never deletes anything: a rule stays, and stops running.
func TestARuleStopsRunningWhenAutomationIsTurnedOff(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	owner.rule(space, "Tag commented pages", onComment(map[string]any{"type": "add_label", "label": "discussed"}))
	owner.want(http.StatusNoContent, "PUT", "/api/v1/admin/experimental", map[string]any{"on": []string{}})
	reader.comment(page, "hello")
	e.settle()
	if got := labelsOf(owner, page); len(got) != 0 {
		t.Fatalf("a rule ran while automation is off: %v", got)
	}
	if n := e.count("SELECT count(*) FROM automation_runs"); n != 0 {
		t.Fatalf("%d runs while automation is off", n)
	}
	if n := e.count("SELECT count(*) FROM automation_rules"); n != 1 {
		t.Fatalf("%d rules after turning automation off", n)
	}
}

// A database made before databases were turned off keeps working; only new ones wait.
func TestADatabaseOutlivesTheSwitch(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	root := admin(e)
	id := owner.ok("POST", "/api/v1/pages", databaseBody(space))["id"].(string)
	root.want(http.StatusNoContent, "PUT", "/api/v1/admin/experimental", map[string]any{"on": []string{}})
	owner.want(http.StatusNotFound, "POST", "/api/v1/pages", databaseBody(space))
	owner.ok("PUT", "/api/v1/pages/"+id+"/draft", map[string]any{"title": "Sums", "document": sheetWith("kept"), "base_version": 1})
}

// rerunExperimentalMigration runs migration 0054 again, as an upgrade from the release
// before it does. It may run again by design, after a restore of an older backup.
func rerunExperimentalMigration(t *testing.T, e *env) {
	t.Helper()
	migs, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migs {
		if m.Version == experimentalMigration {
			if _, err := e.app.Pool().Exec(context.Background(), m.SQL); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("migration 0054 is missing")
}

func forgetExperimental(t *testing.T, e *env) {
	t.Helper()
	if _, err := e.app.Pool().Exec(context.Background(), "DELETE FROM settings WHERE key = 'experimental'"); err != nil {
		t.Fatal(err)
	}
}

func TestAnUpgradeTurnsOnOnlyWhatWasUsed(t *testing.T) {
	e := newEnv(t)
	owner, _, space, _ := ruleTeam(t, e)
	owner.rule(space, "Tag commented pages", onComment(map[string]any{"type": "add_label", "label": "discussed"}))
	forgetExperimental(t, e)
	rerunExperimentalMigration(t, e)
	if on := experimentalOn(owner); !slices.Equal(on, []string{"automation"}) {
		t.Fatalf("on after an upgrade with one rule: %v", on)
	}
}

func TestAnUpgradeKeepsEveryExperimentalPartInUse(t *testing.T) {
	e := newEnv(t)
	owner, _, space, _ := ruleTeam(t, e)
	owner.rule(space, "Tag commented pages", onComment(map[string]any{"type": "add_label", "label": "discussed"}))
	owner.ok("POST", "/api/v1/pages", databaseBody(space))
	ctx := context.Background()
	if _, err := e.app.Pool().Exec(ctx, `INSERT INTO jobs (id, kind, status, created_by) VALUES (gen_random_uuid(), 'bundle_export', 'done', $1)`, owner.id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Pool().Exec(ctx, `INSERT INTO ops.restores (id, backup_name, requested_by_name, state, finished_at)
		VALUES (gen_random_uuid(), 'diffluence-20260101T000000Z', 'Test', 'refused', now())`); err != nil {
		t.Fatal(err)
	}
	forgetExperimental(t, e)
	rerunExperimentalMigration(t, e)
	if on := experimentalOn(owner); !slices.Equal(on, []string{"automation", "bundles", "console_restore", "databases"}) {
		t.Fatalf("on after an upgrade that used every part: %v", on)
	}
}

func TestAnUpgradeKeepsAChoiceAlreadyMade(t *testing.T) {
	e := newEnv(t)
	owner, _, space, _ := ruleTeam(t, e)
	owner.rule(space, "Tag commented pages", onComment(map[string]any{"type": "add_label", "label": "discussed"}))
	owner.want(http.StatusNoContent, "PUT", "/api/v1/admin/experimental", map[string]any{"on": []string{"bundles"}})
	rerunExperimentalMigration(t, e)
	if on := experimentalOn(owner); !slices.Equal(on, []string{"bundles"}) {
		t.Fatalf("on after running the migration again: %v", on)
	}
}

func TestTheExampleLeavesItsDatabaseOutWhileDatabasesAreOff(t *testing.T) {
	e := newInstalledEnv(t, withExample)
	e.user("admin@example.com", identity.AllGlobalPermissions...)
	e.tick()
	if e.spaceCount() != 1 {
		t.Fatalf("spaces: %d", e.spaceCount())
	}
	if n := e.count("SELECT count(*) FROM pages WHERE kind = 'database'"); n != 0 {
		t.Fatalf("the example made %d databases while databases are off", n)
	}
	if n := e.count("SELECT count(*) FROM pages WHERE title = 'Holidays'"); n != 1 {
		t.Fatalf("the page beside the database is missing: %d", n)
	}
}

func TestTheExampleShowsItsDatabaseWhileDatabasesAreOn(t *testing.T) {
	e := newEnvWith(t, withExample)
	e.user("admin@example.com", identity.AllGlobalPermissions...)
	e.tick()
	if n := e.count("SELECT count(*) FROM pages WHERE kind = 'database'"); n != 1 {
		t.Fatalf("databases in the example: %d", n)
	}
}
