// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/webhooks"
)

func onComment(actions ...map[string]any) map[string]any {
	return map[string]any{"trigger": map[string]any{"kind": "page.commented"}, "actions": actions}
}

func (u *user) rule(space, name string, definition map[string]any) string {
	u.e.t.Helper()
	body := map[string]any{"name": name, "enabled": true, "definition": definition}
	status, out := u.do("POST", "/api/v1/spaces/"+space+"/rules", body)
	if status != 201 {
		u.e.t.Fatalf("a rule was refused: %d %s", status, out)
	}
	var made map[string]any
	if err := json.Unmarshal([]byte(out), &made); err != nil {
		u.e.t.Fatal(err)
	}
	return made["id"].(string)
}

// settle moves every event and job until nothing is left, as the worker does.
func (e *env) settle() {
	e.t.Helper()
	ctx := context.Background()
	for range 20 {
		if err := e.app.DeliverEvents(ctx); err != nil {
			e.t.Fatal(err)
		}
		ran, err := e.app.Jobs().RunOnce(ctx)
		if err != nil {
			e.t.Fatal(err)
		}
		for ran {
			if ran, err = e.app.Jobs().RunOnce(ctx); err != nil {
				e.t.Fatal(err)
			}
		}
		if e.count("SELECT count(*) FROM outbox_events WHERE published_at IS NULL") == 0 && e.count("SELECT count(*) FROM jobs WHERE status = 'queued'") == 0 {
			return
		}
	}
}

func labelsOf(u *user, page string) []any {
	u.e.t.Helper()
	got, _ := u.ok("GET", "/api/v1/pages/"+page, nil)["labels"].([]any)
	return got
}

func ruleTeam(t *testing.T, e *env) (owner, reader *user, space, page string) {
	t.Helper()
	owner = e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermAdministrator)
	reader = e.user("reader@example.com", identity.PermUseProduct)
	space, _ = owner.space("RUL")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page = owner.page(space, nil, "Plan", "x")
	return owner, reader, space, page
}

func TestARuleRunsOnAnEventAndEveryActionIsAuditedWithItsName(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	id := owner.rule(space, "Tag commented pages", onComment(map[string]any{"type": "add_label", "label": "discussed"}))
	reader.comment(page, "hello")
	e.settle()
	if got := labelsOf(owner, page); len(got) != 1 || got[0] != "discussed" {
		t.Fatalf("labels %v", got)
	}
	if got := e.count(`SELECT count(*) FROM audit_events WHERE action = 'page.labelled' AND payload LIKE '%Tag commented pages%'`); got != 1 {
		t.Fatalf("%d audit rows with the rule's name", got)
	}
	runs := owner.ok("GET", "/api/v1/rules/"+id+"/runs", nil)["runs"].([]any)
	if len(runs) != 1 || runs[0].(map[string]any)["state"] != "done" {
		t.Fatalf("runs %v", runs)
	}
}

func TestConditionsDecideAndTheLogSaysWhy(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	definition := onComment(map[string]any{"type": "add_label", "label": "x"})
	definition["conditions"] = map[string]any{"kind": "has_label", "value": "wanted"}
	id := owner.rule(space, "Only wanted", definition)
	reader.comment(page, "hello")
	e.settle()
	if len(labelsOf(owner, page)) != 0 {
		t.Fatal("the rule ran past its condition")
	}
	run := owner.ok("GET", "/api/v1/rules/"+id+"/runs", nil)["runs"].([]any)[0].(map[string]any)
	if run["state"] != "skipped" || run["report"] != "conditions_not_met" {
		t.Fatalf("run %v", run)
	}
}

func TestARuleCannotDoMoreThanItsAccount(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	id := owner.rule(space, "Tag", onComment(map[string]any{"type": "add_label", "label": "x"}))
	// The account of the rule loses the right to edit, as can happen any day.
	e.sql("UPDATE automation_rules SET account_id = $2 WHERE id = $1", id, reader.id)
	reader.comment(page, "hello")
	e.settle()
	if len(labelsOf(owner, page)) != 0 {
		t.Fatal("a rule did what its account may not")
	}
	run := owner.ok("GET", "/api/v1/rules/"+id+"/runs", nil)["runs"].([]any)[0].(map[string]any)
	if run["state"] != "failed" {
		t.Fatalf("run %v", run)
	}
}

func TestAPageTheAccountCannotSeeStopsTheRun(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	id := owner.rule(space, "Tag", onComment(map[string]any{"type": "add_label", "label": "x"}))
	e.sql("UPDATE automation_rules SET account_id = $2 WHERE id = $1", id, e.user("other@example.com", identity.PermUseProduct).id)
	reader.comment(page, "hello")
	e.settle()
	run := owner.ok("GET", "/api/v1/rules/"+id+"/runs", nil)["runs"].([]any)[0].(map[string]any)
	if run["report"] != "not_visible" {
		t.Fatalf("run %v", run)
	}
}

func TestARuleCannotTriggerItself(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	id := owner.rule(space, "Echo", onComment(map[string]any{"type": "add_comment", "text": "again"}))
	reader.comment(page, "hello")
	e.settle()
	if got := e.count("SELECT count(*) FROM comments WHERE page_id = $1", page); got != 2 {
		t.Fatalf("%d comments", got)
	}
	if got := e.count("SELECT count(*) FROM automation_runs WHERE rule_id = $1 AND report = 'own_chain'", id); got != 1 {
		t.Fatalf("%d runs stopped on the chain", got)
	}
}

func chained(t *testing.T, e *env, space, page string, links int) {
	t.Helper()
	chain := make([]string, links)
	for i := range chain {
		chain[i] = uuid.NewString()
	}
	message, err := json.Marshal(map[string]any{"id": uuid.New(), "type": "comment.created",
		"payload": map[string]any{"page_id": page, "space_id": space, "rule_chain": chain}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.app.Automation().Handle(context.Background(), message); err != nil {
		t.Fatal(err)
	}
}

func TestAChainOfRulesStopsAtFive(t *testing.T) {
	e := newEnv(t)
	owner, _, space, page := ruleTeam(t, e)
	id := owner.rule(space, "Tag", onComment(map[string]any{"type": "add_label", "label": "x"}))
	chained(t, e, space, page, 4)
	if got := e.count("SELECT count(*) FROM automation_runs WHERE rule_id = $1 AND state = 'queued'", id); got != 1 {
		t.Fatalf("a chain of four must still run: %d", got)
	}
	chained(t, e, space, page, 5)
	if got := e.count("SELECT count(*) FROM automation_runs WHERE rule_id = $1 AND report = 'chain_too_long'", id); got != 1 {
		t.Fatalf("a chain of five must stop: %d", got)
	}
}

func TestAnActionPassesTheChainOn(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	id := owner.rule(space, "Tag", onComment(map[string]any{"type": "add_label", "label": "x"}))
	reader.comment(page, "go")
	e.settle()
	if got := e.count(`SELECT count(*) FROM outbox_events WHERE type = 'page.labelled' AND payload->'rule_chain' ? $1`, id); got != 1 {
		t.Fatalf("%d events carry the rule", got)
	}
}

func TestALimitPausesARuleAndTellsItsOwner(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	owner.ok("PUT", "/api/v1/admin/automation", map[string]any{"runs_per_hour": 2})
	id := owner.rule(space, "Busy", onComment(map[string]any{"type": "add_label", "label": "x"}))
	for range 3 {
		reader.comment(page, "hello")
	}
	e.settle()
	rule := owner.ok("GET", "/api/v1/rules/"+id, nil)
	if rule["paused_at"] == nil {
		t.Fatal("the rule did not pause")
	}
	if got := e.count("SELECT count(*) FROM automation_runs WHERE rule_id = $1", id); got != 2 {
		t.Fatalf("%d runs", got)
	}
	if e.count("SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'rule'", owner.id) != 1 {
		t.Fatal("the owner was not told")
	}
	if e.auditActions("rule.paused") != 1 {
		t.Fatal("the pause was not audited")
	}
}

func TestARedeliveredEventRunsOnce(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	id := owner.rule(space, "Tag", onComment(map[string]any{"type": "add_label", "label": "x"}))
	reader.comment(page, "hello")
	var payload []byte
	var eventID uuid.UUID
	var eventType string
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT id, type, payload FROM outbox_events WHERE type = 'comment.created'").Scan(&eventID, &eventType, &payload); err != nil {
		t.Fatal(err)
	}
	message, err := json.Marshal(map[string]any{"id": eventID, "type": eventType, "payload": json.RawMessage(payload)})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := e.app.Automation().Handle(context.Background(), message); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.count("SELECT count(*) FROM automation_runs WHERE rule_id = $1", id); got != 1 {
		t.Fatalf("%d runs for one event", got)
	}
}

func TestARetryDoesNotRepeatFinishedSteps(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	id := owner.rule(space, "Two steps", onComment(map[string]any{"type": "add_comment", "text": "from the rule"},
		map[string]any{"type": "set_owner", "target_id": reader.id}))
	reader.comment(page, "hello")
	e.settle()
	// The second step failed: a viewer cannot own a page here, or it passed. Either way
	// a retry of the run must leave the first step alone.
	var run uuid.UUID
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT id FROM automation_runs WHERE rule_id = $1 ORDER BY created_at LIMIT 1", id).Scan(&run); err != nil {
		t.Fatal(err)
	}
	e.sql("UPDATE automation_runs SET state = 'failed' WHERE id = $1", run)
	before := e.count("SELECT count(*) FROM comments WHERE page_id = $1", page)
	if err := e.app.Automation().Execute(context.Background(), run); err != nil {
		t.Log(err)
	}
	if got := e.count("SELECT count(*) FROM comments WHERE page_id = $1", page); got != before {
		t.Fatalf("a finished step ran again: %d to %d", before, got)
	}
}

func TestAMessageReachesNobodyWhoCannotSeeThePage(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	owner.rule(space, "Tell", onComment(
		map[string]any{"type": "notify", "to": "user", "target_id": stranger.id, "text": "See {page.title}"},
		map[string]any{"type": "notify", "to": "user", "target_id": reader.id, "text": "See {page.title}"}))
	owner.comment(page, "hello")
	e.settle()
	if e.count("SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'rule'", stranger.id) != 0 {
		t.Fatal("someone heard about a page they cannot see")
	}
	if e.count("SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'rule' AND data->>'text' = 'See Plan'", reader.id) != 1 {
		t.Fatal("the reader heard nothing, or the value was not filled in")
	}
}

func TestAMessageReachesNobodyWhoCannotSeeEveryPageItRefersTo(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	found := owner.page(space, nil, "Found", "x")
	owner.ok("POST", "/api/v1/pages/"+found+"/labels", map[string]any{"label": "wanted"})
	id := owner.rule(space, "Tell about both", map[string]any{"trigger": map[string]any{"kind": "manual"},
		"actions": []map[string]any{{"type": "find_pages", "label": "wanted"},
			{"type": "notify", "to": "user", "target_id": reader.id, "text": "Started from {rule.name}"}}})
	// The reader can see the found page, and not the page the run started from.
	owner.restrict(page, "view", owner.id)
	owner.want(202, "POST", "/api/v1/rules/"+id+"/run", map[string]any{"page_id": page})
	e.settle()
	if got := e.count("SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'rule'", reader.id); got != 0 {
		t.Fatalf("%d messages about a page the reader cannot see", got)
	}
}

func TestAFindStepGivesLaterStepsItsPages(t *testing.T) {
	e := newEnv(t)
	owner, _, space, page := ruleTeam(t, e)
	other := owner.page(space, nil, "Other", "x")
	e.sql("UPDATE pages SET updated_at = now() - interval '400 days' WHERE id = $1", page)
	id := owner.rule(space, "Old pages", map[string]any{
		"trigger": map[string]any{"kind": "schedule", "every": "daily", "at": "09:00"},
		"actions": []map[string]any{{"type": "find_pages", "older_days": 365}, {"type": "add_label", "label": "stale"}}})
	e.sql("UPDATE automation_rules SET next_run_at = now() - interval '1 minute' WHERE id = $1", id)
	if err := e.app.Automation().RunSchedules(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.settle()
	if got := labelsOf(owner, page); len(got) != 1 {
		t.Fatalf("the old page has %v", got)
	}
	if got := labelsOf(owner, other); len(got) != 0 {
		t.Fatalf("the new page has %v", got)
	}
}

func TestARuleWithAProblemCannotBeTurnedOn(t *testing.T) {
	e := newEnv(t)
	owner, _, space, _ := ruleTeam(t, e)
	body := map[string]any{"name": "Broken", "enabled": true, "definition": onComment(map[string]any{"type": "add_label"})}
	owner.want(400, "POST", "/api/v1/spaces/"+space+"/rules", body)
	body["enabled"] = false
	made := owner.ok("POST", "/api/v1/spaces/"+space+"/rules", body)
	if got := made["problems"].([]any); len(got) != 1 {
		t.Fatalf("problems %v", got)
	}
}

func TestAGroupThatIsGoneStopsARule(t *testing.T) {
	e := newEnv(t)
	owner, _, space, _ := ruleTeam(t, e)
	group := uuid.NewString()
	id := owner.rule(space, "Tell a group", map[string]any{"trigger": map[string]any{"kind": "manual"},
		"actions": []map[string]any{{"type": "add_label", "label": "x"}}})
	owner.ok("PUT", "/api/v1/rules/"+id, map[string]any{"name": "Tell a group", "enabled": false, "definition": map[string]any{
		"trigger": map[string]any{"kind": "manual"}, "actions": []map[string]any{{"type": "notify", "to": "group", "target_id": group, "text": "x"}}}})
	if got := owner.ok("GET", "/api/v1/rules/"+id, nil)["problems"].([]any); len(got) != 1 {
		t.Fatalf("problems %v", got)
	}
}

func TestATestRunChangesNothingAndSaysWhatTheAccountMayNotDo(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	id := owner.rule(space, "Tag", onComment(map[string]any{"type": "add_label", "label": "x"}, map[string]any{"type": "add_comment", "text": "hi"}))
	got := owner.ok("POST", "/api/v1/rules/"+id+"/test", map[string]any{"page_id": page})
	if got["outcome"] != "would_run" || len(got["steps"].([]any)) != 2 || len(got["warnings"].([]any)) != 1 {
		t.Fatalf("got %v", got)
	}
	if len(labelsOf(owner, page)) != 0 || e.count("SELECT count(*) FROM comments") != 0 {
		t.Fatal("a test run changed something")
	}
	e.sql("UPDATE automation_rules SET account_id = $2 WHERE id = $1", id, reader.id)
	got = owner.ok("POST", "/api/v1/rules/"+id+"/test", map[string]any{"page_id": page})
	if note := got["steps"].([]any)[0].(map[string]any)["note"]; note != "account_may_not" {
		t.Fatalf("got %v", got)
	}
}

func TestByHandARuleNeedsOnlySightOfThePage(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	id := owner.rule(space, "Label now", map[string]any{"trigger": map[string]any{"kind": "manual"},
		"actions": []map[string]any{{"type": "add_label", "label": "x"}}})
	if got := reader.ok("GET", "/api/v1/pages/"+page+"/rules", nil)["rules"].([]any); len(got) != 1 {
		t.Fatalf("rules %v", got)
	}
	stranger.want(404, "POST", "/api/v1/rules/"+id+"/run", map[string]any{"page_id": page})
	reader.want(202, "POST", "/api/v1/rules/"+id+"/run", map[string]any{"page_id": page})
	e.settle()
	if len(labelsOf(owner, page)) != 1 {
		t.Fatal("the rule did not run")
	}
}

func TestAnIncomingHookNeedsItsToken(t *testing.T) {
	e := newEnv(t)
	owner, _, space, _ := ruleTeam(t, e)
	id := owner.rule(space, "From outside", map[string]any{"trigger": map[string]any{"kind": "incoming"},
		"actions": []map[string]any{{"type": "find_pages"}, {"type": "add_label", "label": "x"}}})
	address := owner.ok("POST", "/api/v1/rules/"+id+"/hook", map[string]any{})["address"].(string)
	token := address[strings.LastIndex(address, "/")+1:]
	if resp, _ := e.do(e.client(), "POST", "/hooks/wrong", ""); resp.StatusCode != 404 {
		t.Fatalf("a wrong token got %d", resp.StatusCode)
	}
	if resp, _ := e.do(e.client(), "POST", "/hooks/"+token, ""); resp.StatusCode != 202 {
		t.Fatalf("the token got %d", resp.StatusCode)
	}
	if e.count("SELECT count(*) FROM automation_runs WHERE rule_id = $1", id) != 1 {
		t.Fatal("no run started")
	}
	if e.count("SELECT count(*) FROM automation_hooks WHERE token_hash = $1", []byte(token)) != 0 {
		t.Fatal("the token is stored as it is")
	}
}

func TestOnlyAdministratorsManageRules(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, _ := ruleTeam(t, e)
	id := owner.rule(space, "Tag", onComment(map[string]any{"type": "add_label", "label": "x"}))
	reader.want(403, "GET", "/api/v1/rules/"+id, nil)
	reader.want(403, "POST", "/api/v1/spaces/"+space+"/rules", map[string]any{"name": "x"})
	reader.want(403, "GET", "/api/v1/admin/rules", nil)
}

func instanceRule(u *user, name string, spaces []string, definition map[string]any) string {
	u.e.t.Helper()
	definition["spaces"] = spaces
	body := map[string]any{"name": name, "enabled": true, "definition": definition}
	status, out := u.do("POST", "/api/v1/admin/rules", body)
	if status != 201 {
		u.e.t.Fatalf("an instance rule was refused: %d %s", status, out)
	}
	var made map[string]any
	if err := json.Unmarshal([]byte(out), &made); err != nil {
		u.e.t.Fatal(err)
	}
	return made["id"].(string)
}

func TestAnInstanceRuleActsOnlyInTheSpacesItWasGiven(t *testing.T) {
	e := newEnv(t)
	owner, reader, chosen, inChosen := ruleTeam(t, e)
	other, _ := owner.space("OTH")
	owner.grant(other, "user", reader.id, permissions.TemplateViewer)
	inOther := owner.page(other, nil, "Elsewhere", "x")
	id := instanceRule(owner, "Tag in chosen spaces", []string{chosen}, onComment(map[string]any{"type": "add_label", "label": "seen"}))
	reader.comment(inOther, "hello")
	reader.comment(inChosen, "hello")
	e.settle()
	if got := labelsOf(owner, inOther); len(got) != 0 {
		t.Fatalf("the rule acted outside its spaces: %v", got)
	}
	if got := labelsOf(owner, inChosen); len(got) != 1 || got[0] != "seen" {
		t.Fatalf("labels in the chosen space %v", got)
	}
	spaces := owner.ok("GET", "/api/v1/rules/"+id, nil)["definition"].(map[string]any)["spaces"].([]any)
	if len(spaces) != 1 || spaces[0] != chosen {
		t.Fatalf("spaces %v", spaces)
	}
}

func TestAFindStepOfAnInstanceRuleLooksOnlyInItsSpaces(t *testing.T) {
	e := newEnv(t)
	owner, _, chosen, inChosen := ruleTeam(t, e)
	other, _ := owner.space("OTH")
	inOther := owner.page(other, nil, "Elsewhere", "x")
	e.sql("UPDATE pages SET updated_at = now() - interval '400 days' WHERE id = ANY($1)", []string{inChosen, inOther})
	id := instanceRule(owner, "Old pages in chosen spaces", []string{chosen}, map[string]any{
		"trigger": map[string]any{"kind": "schedule", "every": "daily", "at": "09:00"},
		"actions": []map[string]any{{"type": "find_pages", "older_days": 365}, {"type": "add_label", "label": "stale"}}})
	e.sql("UPDATE automation_rules SET next_run_at = now() - interval '1 minute' WHERE id = $1", id)
	if err := e.app.Automation().RunSchedules(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.settle()
	if got := labelsOf(owner, inChosen); len(got) != 1 {
		t.Fatalf("the old page in the chosen space has %v", got)
	}
	if got := labelsOf(owner, inOther); len(got) != 0 {
		t.Fatalf("the lookup went outside its spaces: %v", got)
	}
}

func TestAnInstanceRuleWithoutSpacesActsEverywhere(t *testing.T) {
	e := newEnv(t)
	owner, reader, _, page := ruleTeam(t, e)
	instanceRule(owner, "Tag everywhere", nil, onComment(map[string]any{"type": "add_label", "label": "seen"}))
	reader.comment(page, "hello")
	e.settle()
	if got := labelsOf(owner, page); len(got) != 1 {
		t.Fatalf("labels %v", got)
	}
}

func TestOnlyAnInstanceRuleChoosesSpacesAndOnlyRealOnes(t *testing.T) {
	e := newEnv(t)
	owner, _, space, _ := ruleTeam(t, e)
	definition := onComment(map[string]any{"type": "add_label", "label": "x"})
	definition["spaces"] = []string{space}
	owner.want(400, "POST", "/api/v1/spaces/"+space+"/rules", map[string]any{"name": "Scoped", "definition": definition})
	definition["spaces"] = []string{uuid.NewString()}
	owner.want(400, "POST", "/api/v1/admin/rules", map[string]any{"name": "Nowhere", "definition": definition})
}

func TestAChosenAccountMustBeAServiceAccount(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, _ := ruleTeam(t, e)
	owner.want(400, "POST", "/api/v1/spaces/"+space+"/rules", map[string]any{"name": "Borrowed", "account_id": reader.id,
		"definition": onComment(map[string]any{"type": "add_label", "label": "x"})})
}

func TestADisabledOwnerStopsTheRule(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	id := owner.rule(space, "Tag", onComment(map[string]any{"type": "add_label", "label": "x"}))
	e.sql("UPDATE users SET status = 'disabled' WHERE id = $1", owner.id)
	reader.comment(page, "hello")
	e.settle()
	if e.count("SELECT count(*) FROM automation_rules WHERE id = $1 AND enabled", id) != 0 {
		t.Fatal("the rule stayed on")
	}
	if e.auditActions("rule.stopped") != 1 {
		t.Fatal("not audited")
	}
}

func TestTheLogNamesOnlyWhatThePersonLookingMaySee(t *testing.T) {
	e := newEnv(t)
	owner, reader, space, page := ruleTeam(t, e)
	id := owner.rule(space, "Tag", onComment(map[string]any{"type": "add_label", "label": "x"}))
	admin := e.user("second@example.com", identity.PermUseProduct)
	owner.grant(space, "user", admin.id, permissions.TemplateSpaceAdmin)
	reader.comment(page, "hello")
	e.settle()
	owner.restrict(page, "view", owner.id)
	if strings.Contains(admin.want(200, "GET", "/api/v1/rules/"+id+"/runs", nil), "Plan") {
		t.Fatal("the log named a page this person cannot see")
	}
	if !strings.Contains(owner.want(200, "GET", "/api/v1/rules/"+id+"/runs", nil), "Plan") {
		t.Fatal("the log lost the name for the owner")
	}
}

// receiver records what it gets, and answers with a chosen status.
type receiver struct {
	sync.Mutex
	*httptest.Server
	status   int
	requests []*http.Request
	bodies   []string
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()
	r := &receiver{status: 200}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.Lock()
		defer r.Unlock()
		body, _ := io.ReadAll(req.Body)
		r.requests, r.bodies = append(r.requests, req), append(r.bodies, string(body))
		w.WriteHeader(r.status)
	}))
	t.Cleanup(r.Close)
	return r
}

func hookTeam(t *testing.T) (*env, *user, string, string, *receiver) {
	t.Helper()
	e := newEnvWith(t, func(c *config.Config) { c.OutwardAllowPrivate = true })
	owner, _, space, page := ruleTeam(t, e)
	r := newReceiver(t)
	owner.ok("PUT", "/api/v1/admin/outgoing-hosts", map[string]any{"hosts": []string{"127.0.0.1"}})
	return e, owner, space, page, r
}

func (e *env) sendHooks() {
	e.t.Helper()
	e.settle()
	if err := e.app.Webhooks().DeliverDue(context.Background()); err != nil {
		e.t.Fatal(err)
	}
}

func TestAnAddressOnAHostNotAllowedIsRefused(t *testing.T) {
	e := newEnv(t)
	owner, _, space, _ := ruleTeam(t, e)
	owner.want(400, "POST", "/api/v1/spaces/"+space+"/webhooks", map[string]any{"address": "https://hooks.example.com/x", "events": []string{"page.labelled"}})
}

func TestAWebhookIsSignedAndHoldsNoText(t *testing.T) {
	e, owner, space, page, r := hookTeam(t)
	made := owner.ok("POST", "/api/v1/spaces/"+space+"/webhooks", map[string]any{"address": r.URL + "/in", "events": []string{"page.labelled"}})
	secret := made["secret"].(string)
	owner.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "x"})
	e.sendHooks()
	if len(r.requests) != 1 {
		t.Fatalf("%d requests", len(r.requests))
	}
	req, body := r.requests[0], r.bodies[0]
	if strings.Contains(body, "Plan") {
		t.Fatal("a title left without being asked for")
	}
	if req.Header.Get("webhook-id") == "" || !strings.HasPrefix(req.Header.Get("webhook-signature"), "v1,") {
		t.Fatalf("headers %v", req.Header)
	}
	assertSigned(t, secret, req, body)
}

func TestTitlesLeaveOnlyWhenAnEndpointAsks(t *testing.T) {
	e, owner, space, page, r := hookTeam(t)
	owner.ok("POST", "/api/v1/spaces/"+space+"/webhooks", map[string]any{"address": r.URL, "events": []string{"page.labelled"}, "with_titles": true})
	owner.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "x"})
	e.sendHooks()
	if !strings.Contains(r.bodies[0], "Plan") {
		t.Fatal("the title is missing")
	}
}

func TestAFailedDeliveryIsTriedAgainWithTheSameId(t *testing.T) {
	e, owner, space, page, r := hookTeam(t)
	r.status = 500
	made := owner.ok("POST", "/api/v1/spaces/"+space+"/webhooks", map[string]any{"address": r.URL, "events": []string{"page.labelled"}})
	id := made["webhook"].(map[string]any)["id"].(string)
	owner.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "x"})
	e.sendHooks()
	e.sql("UPDATE webhook_deliveries SET next_attempt = now()")
	r.status = 200
	e.sendHooks()
	if len(r.requests) != 2 || r.requests[0].Header.Get("webhook-id") != r.requests[1].Header.Get("webhook-id") {
		t.Fatalf("%d requests", len(r.requests))
	}
	log := owner.ok("GET", "/api/v1/webhooks/"+id+"/deliveries", nil)["deliveries"].([]any)[0].(map[string]any)
	if log["state"] != "delivered" || log["attempts"] != float64(2) {
		t.Fatalf("log %v", log)
	}
	owner.want(202, "POST", "/api/v1/webhooks/"+id+"/deliveries/"+log["id"].(string)+"/resend", map[string]any{})
	e.sendHooks()
	if len(r.requests) != 3 {
		t.Fatalf("Send again made %d requests", len(r.requests))
	}
}

func TestAnEndpointSwitchesOffAfterFiftyFailures(t *testing.T) {
	e, owner, space, page, r := hookTeam(t)
	r.status = 500
	made := owner.ok("POST", "/api/v1/spaces/"+space+"/webhooks", map[string]any{"address": r.URL, "events": []string{"page.labelled"}})
	id := made["webhook"].(map[string]any)["id"].(string)
	e.sql("UPDATE webhook_endpoints SET failures = 49 WHERE id = $1", id)
	owner.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "x"})
	e.sendHooks()
	if owner.ok("GET", "/api/v1/webhooks/"+id, nil)["enabled"] != false {
		t.Fatal("the endpoint stayed on")
	}
	if e.auditActions("webhook.switched_off") != 1 {
		t.Fatal("not audited")
	}
}

func TestRotationSendsBothSignaturesForADay(t *testing.T) {
	e, owner, space, page, r := hookTeam(t)
	made := owner.ok("POST", "/api/v1/spaces/"+space+"/webhooks", map[string]any{"address": r.URL, "events": []string{"page.labelled"}})
	id := made["webhook"].(map[string]any)["id"].(string)
	owner.ok("POST", "/api/v1/webhooks/"+id+"/rotate", map[string]any{})
	owner.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "x"})
	e.sendHooks()
	if got := strings.Fields(r.requests[0].Header.Get("webhook-signature")); len(got) != 2 {
		t.Fatalf("signatures %v", got)
	}
	assertSigned(t, made["secret"].(string), r.requests[0], r.bodies[0])
}

func TestAPageTheOwnerLostIsNotSent(t *testing.T) {
	e, owner, space, page, r := hookTeam(t)
	second := e.user("second@example.com", identity.PermUseProduct)
	owner.grant(space, "user", second.id, permissions.TemplateSpaceAdmin)
	second.ok("POST", "/api/v1/spaces/"+space+"/webhooks", map[string]any{"address": r.URL, "events": []string{"page.labelled"}})
	owner.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "x"})
	e.settle()
	owner.restrict(page, "view", owner.id)
	e.sendHooks()
	if len(r.requests) != 0 {
		t.Fatal("an event about a hidden page left")
	}
	if e.count("SELECT count(*) FROM webhook_deliveries WHERE state = 'dropped'") != 1 {
		t.Fatal("the delivery was not dropped")
	}
}

func TestARuleCallsOutThroughTheAllowedList(t *testing.T) {
	e, owner, space, page, r := hookTeam(t)
	owner.rule(space, "Call", onComment(map[string]any{"type": "webhook", "address": r.URL + "/rule", "body": `{"title":"{page.title}"}`}))
	owner.comment(page, "hello")
	e.settle()
	if len(r.bodies) != 1 || r.bodies[0] != `{"title":"Plan"}` {
		t.Fatalf("bodies %v", r.bodies)
	}
	owner.ok("PUT", "/api/v1/admin/outgoing-hosts", map[string]any{"hosts": []string{}})
	owner.comment(page, "again")
	e.settle()
	if len(r.bodies) != 1 {
		t.Fatal("a call left after its host was taken off the list")
	}
}

func decodeSecret(secret string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
}

func parseUnix(raw string) (time.Time, error) {
	seconds, err := strconv.ParseInt(raw, 10, 64)
	return time.Unix(seconds, 0), err
}

func assertSigned(t *testing.T, secret string, req *http.Request, body string) {
	t.Helper()
	raw, err := decodeSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	at, err := parseUnix(req.Header.Get("webhook-timestamp"))
	if err != nil {
		t.Fatal(err)
	}
	want := webhooks.Sign(raw, req.Header.Get("webhook-id"), at, []byte(body))
	if !strings.Contains(req.Header.Get("webhook-signature"), want) {
		t.Fatalf("%q does not hold %q", req.Header.Get("webhook-signature"), want)
	}
}
