// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Who a rule runs as, and who may change it (spec 092).

func secondManager(t *testing.T, e *env, space string, owner *user) *user {
	t.Helper()
	second := e.user("second@example.com", identity.PermUseProduct)
	owner.grant(space, "user", second.id, permissions.TemplateSpaceAdmin)
	return second
}

func ruleBody(name string, enabled bool, definition map[string]any) map[string]any {
	return map[string]any{"name": name, "enabled": enabled, "definition": definition}
}

// errorCode reads the code of an error answer.
func errorCode(t *testing.T, out string) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return body.Error.Code
}

func TestSomeoneWhoDoesNotHoldARuleCannotChangeWhatItDoes(t *testing.T) {
	e := newEnv(t)
	owner, _, space, _ := ruleTeam(t, e)
	second := secondManager(t, e, space, owner)
	tag := onComment(map[string]any{"type": "add_label", "label": "x"})
	id := owner.rule(space, "Tag", tag)

	moved := onComment(map[string]any{"type": "move", "space_id": space})
	if code := errorCode(t, second.want(409, "PUT", "/api/v1/rules/"+id, ruleBody("Tag", true, moved))); code != "rule_held" {
		t.Fatalf("code %q", code)
	}
	// Not even together with switching it off.
	second.want(409, "PUT", "/api/v1/rules/"+id, ruleBody("Renamed", false, tag))
	second.want(409, "PUT", "/api/v1/rules/"+id, map[string]any{"name": "Tag", "enabled": true, "definition": tag, "account_id": second.id})

	rule := owner.ok("GET", "/api/v1/rules/"+id, nil)
	action := rule["definition"].(map[string]any)["actions"].([]any)[0].(map[string]any)
	if action["type"] != "add_label" || rule["name"] != "Tag" || rule["account_id"] != owner.id || rule["enabled"] != true {
		t.Fatalf("the rule changed: %v", rule)
	}
	// The holder still changes it.
	owner.ok("PUT", "/api/v1/rules/"+id, ruleBody("Tag", true, onComment(map[string]any{"type": "add_label", "label": "y"})))
}

func TestSomeoneWhoDoesNotHoldARuleCanSwitchItOffAndDeleteIt(t *testing.T) {
	e := newEnv(t)
	owner, _, space, _ := ruleTeam(t, e)
	second := secondManager(t, e, space, owner)
	id := owner.rule(space, "Tag", onComment(map[string]any{"type": "add_label", "label": "x"}))
	definition := owner.ok("GET", "/api/v1/rules/"+id, nil)["definition"]

	off := second.ok("PUT", "/api/v1/rules/"+id, map[string]any{"name": "Tag", "enabled": false, "definition": definition})
	if off["enabled"] != false || off["account_id"] != owner.id {
		t.Fatalf("got %v", off)
	}
	// Switching it on again lends the holder's rights at a moment they did not choose.
	second.want(409, "PUT", "/api/v1/rules/"+id, map[string]any{"name": "Tag", "enabled": true, "definition": definition})
	second.want(204, "DELETE", "/api/v1/rules/"+id, nil)
}

func TestATakeOverMakesTheRuleActAsWhoeverTookItOver(t *testing.T) {
	e := newEnv(t)
	owner, _, space, _ := ruleTeam(t, e)
	second := secondManager(t, e, space, owner)
	id := owner.rule(space, "From outside", map[string]any{"trigger": map[string]any{"kind": "incoming"},
		"actions": []map[string]any{{"type": "find_pages"}, {"type": "add_label", "label": "x"}}})
	owner.ok("POST", "/api/v1/rules/"+id+"/hook", map[string]any{})

	changed := map[string]any{"trigger": map[string]any{"kind": "incoming"},
		"actions": []map[string]any{{"type": "find_pages"}, {"type": "add_label", "label": "mine"}}}
	got := second.ok("PUT", "/api/v1/rules/"+id, map[string]any{"name": "From outside", "enabled": true, "definition": changed, "take_over": true})
	if got["account_id"] != second.id || got["owner_id"] != second.id {
		t.Fatalf("the rule still acts as before: %v", got)
	}
	if got["enabled"] != false || got["has_hook"] != false {
		t.Fatalf("a rule taken over must start off and without its old address: %v", got)
	}
	if e.auditActions("rule.taken_over") != 1 {
		t.Fatal("the take-over was not audited")
	}
	if got := e.count(`SELECT count(*) FROM audit_entries WHERE action = 'rule.taken_over' AND actor_id = $1 AND payload LIKE '%' || $2 || '%'`,
		second.id, owner.id); got != 1 {
		t.Fatalf("%d audit rows name who took it over from whom", got)
	}
	if e.count("SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'rule' AND data->>'what' = 'taken_over'", owner.id) != 1 {
		t.Fatal("the previous holder was not told")
	}
	// Now it is theirs: they switch it on, and the previous holder no longer changes it.
	second.ok("PUT", "/api/v1/rules/"+id, ruleBody("From outside", true, changed))
	owner.want(409, "PUT", "/api/v1/rules/"+id, ruleBody("From outside", true, changed))
}

func TestARuleTakenOverRunsWithTheRightsOfWhoeverTookItOver(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	second := secondManager(t, e, space, owner)
	tag := onComment(map[string]any{"type": "add_label", "label": "x"})
	id := owner.rule(space, "Tag", tag)
	second.ok("PUT", "/api/v1/rules/"+id, map[string]any{"name": "Tag", "enabled": false, "definition": tag, "take_over": true})
	second.ok("PUT", "/api/v1/rules/"+id, ruleBody("Tag", true, tag))
	// Only the previous holder and the reader see the page now.
	owner.restrict(page, "view", owner.id, reader.id)
	reader.comment(page, "hello")
	e.settle()
	if len(labelsOf(owner, page)) != 0 {
		t.Fatal("the rule used the rights of the previous holder")
	}
	run := second.ok("GET", "/api/v1/rules/"+id+"/runs", nil)["runs"].([]any)[0].(map[string]any)
	if run["report"] != "not_visible" {
		t.Fatalf("run %v", run)
	}
}

func TestATestRunBySomeoneElseReadsPagesAsThem(t *testing.T) {
	e := newEnv(t)
	owner, _, space, page := ruleTeam(t, e)
	second := secondManager(t, e, space, owner)
	owner.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "secret"})
	owner.restrict(page, "view", owner.id)
	id := owner.rule(space, "Find the secret", map[string]any{"trigger": map[string]any{"kind": "manual"},
		"actions": []map[string]any{{"type": "find_pages", "label": "secret"}, {"type": "add_label", "label": "seen"}}})

	found := func(u *user) any {
		got := u.ok("POST", "/api/v1/rules/"+id+"/test", map[string]any{})
		return got["steps"].([]any)[0].(map[string]any)["note"]
	}
	if got := found(owner); got != "1 found" {
		t.Fatalf("the holder's test found %v", got)
	}
	if got := found(second); got != "0 found" {
		t.Fatalf("someone else's test found %v", got)
	}
	if got := second.ok("POST", "/api/v1/rules/"+id+"/test", map[string]any{"page_id": page})["outcome"]; got != "not_visible" {
		t.Fatalf("someone else's test on a page they cannot see: %v", got)
	}
}

func TestATestRunBySomeoneElseSaysWhatTheyMayNotDo(t *testing.T) {
	e := newEnv(t)
	owner, _, space, page := ruleTeam(t, e)
	// Someone who looks after automation here and may only read pages.
	keeper := e.user("keeper@example.com", identity.PermUseProduct)
	e.sql("SELECT rbac_set_exact($1::uuid, 'user', $2::uuid, $3::text[])", space, keeper.id,
		append(append([]string{}, permissions.Templates[permissions.TemplateViewer]...), permissions.SpaceAutomation))
	id := owner.rule(space, "Tag", onComment(map[string]any{"type": "add_label", "label": "x"}))
	note := func(u *user) any {
		got := u.ok("POST", "/api/v1/rules/"+id+"/test", map[string]any{"page_id": page})
		return got["steps"].([]any)[0].(map[string]any)["note"]
	}
	if got := note(owner); got != "would_add_label" {
		t.Fatalf("the holder's test says %v", got)
	}
	if got := note(keeper); got != "account_may_not" {
		t.Fatalf("someone else's test says %v", got)
	}
}

func TestAnInstanceRuleIsChangedOnlyByItsHolder(t *testing.T) {
	e := newEnv(t)
	owner, _, _, _ := ruleTeam(t, e)
	other := e.user("admin2@example.com", identity.PermUseProduct, identity.PermAdministrator)
	tag := onComment(map[string]any{"type": "add_label", "label": "x"})
	id := instanceRule(owner, "Tag everywhere", nil, tag)
	other.want(409, "PUT", "/api/v1/rules/"+id, ruleBody("Tag everywhere", true, onComment(map[string]any{"type": "add_label", "label": "y"})))
	got := other.ok("PUT", "/api/v1/rules/"+id, map[string]any{"name": "Tag everywhere", "enabled": true, "definition": tag, "take_over": true})
	if got["account_id"] != other.id || got["enabled"] != false {
		t.Fatalf("got %v", got)
	}
}

func TestTheAdministratorWhoChoseAServiceAccountHoldsTheRule(t *testing.T) {
	e := newEnv(t)
	owner, _, space, _ := ruleTeam(t, e)
	second := secondManager(t, e, space, owner)
	robot, err := e.app.Identity().CreateServiceAccount(context.Background(), identity.SystemCaller("test"), "Robot")
	if err != nil {
		t.Fatal(err)
	}
	tag := onComment(map[string]any{"type": "add_label", "label": "x"})
	made := owner.ok("POST", "/api/v1/spaces/"+space+"/rules", map[string]any{"name": "Robot", "enabled": true, "definition": tag, "account_id": robot.ID})
	id := made["id"].(string)
	owner.ok("PUT", "/api/v1/rules/"+id, ruleBody("Robot", true, onComment(map[string]any{"type": "add_label", "label": "y"})))
	second.want(409, "PUT", "/api/v1/rules/"+id, ruleBody("Robot", true, tag))
	// The owner without the right to lend a service account no longer holds the rule.
	e.sql("DELETE FROM global_grants WHERE subject_type = 'user' AND subject_id = $1 AND permission = $2", owner.id, identity.PermAdministrator)
	owner.want(409, "PUT", "/api/v1/rules/"+id, ruleBody("Robot", true, tag))
}

func scheduled(conditions map[string]any) map[string]any {
	definition := map[string]any{"trigger": map[string]any{"kind": "schedule", "every": "daily", "at": "09:00"},
		"actions": []map[string]any{{"type": "find_pages"}, {"type": "add_label", "label": "stale"}}}
	if conditions != nil {
		definition["conditions"] = conditions
	}
	return definition
}

func TestConditionsUnderATriggerWithoutAPageKeepARuleOff(t *testing.T) {
	e := newEnv(t)
	owner, _, space, _ := ruleTeam(t, e)
	wanted := map[string]any{"kind": "has_label", "value": "wanted"}
	owner.want(400, "POST", "/api/v1/spaces/"+space+"/rules", ruleBody("Old pages", true, scheduled(wanted)))
	made := owner.ok("POST", "/api/v1/spaces/"+space+"/rules", ruleBody("Old pages", false, scheduled(wanted)))
	problems := made["problems"].([]any)
	if len(problems) != 1 || problems[0].(map[string]any)["step"] != float64(-2) {
		t.Fatalf("problems %v", problems)
	}
}

func TestARunWithoutAPageDoesNothingWhenTheRuleHasConditions(t *testing.T) {
	e := newEnv(t)
	owner, _, space, page := ruleTeam(t, e)
	id := owner.rule(space, "Old pages", scheduled(nil))
	// A rule of this shape saved by an earlier release.
	e.sql(`UPDATE automation_rules SET definition = definition || '{"conditions": {"kind": "has_label", "value": "wanted"}}'::jsonb,
		next_run_at = now() - interval '1 minute' WHERE id = $1`, id)
	if err := e.app.Automation().RunSchedules(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.settle()
	if got := labelsOf(owner, page); len(got) != 0 {
		t.Fatalf("the rule acted past its conditions: %v", got)
	}
	if got := e.count("SELECT count(*) FROM automation_runs WHERE rule_id = $1 AND state = 'skipped' AND report = 'conditions_need_page'", id); got != 1 {
		t.Fatalf("%d runs say why they did nothing", got)
	}
	if got := owner.ok("POST", "/api/v1/rules/"+id+"/test", map[string]any{})["outcome"]; got != "conditions_need_page" {
		t.Fatalf("the test run says %v", got)
	}
}
