// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Only the holder of a rule starts it outside its trigger (spec 093).

func incomingRule() map[string]any {
	return map[string]any{"trigger": map[string]any{"kind": "incoming"},
		"actions": []map[string]any{{"type": "find_pages"}, {"type": "add_label", "label": "x"}}}
}

func hookToken(address string) string {
	return address[strings.LastIndex(address, "/")+1:]
}

func TestOnlyTheHolderOfARuleMakesItsIncomingAddress(t *testing.T) {
	e := newEnv(t)
	owner, _, space, _ := ruleTeam(t, e)
	second := secondManager(t, e, space, owner)
	id := owner.rule(space, "From outside", incomingRule())
	token := hookToken(owner.ok("POST", "/api/v1/rules/"+id+"/hook", map[string]any{})["address"].(string))

	if code := errorCode(t, second.want(409, "POST", "/api/v1/rules/"+id+"/hook", map[string]any{})); code != "rule_held" {
		t.Fatalf("code %q", code)
	}
	// The holder's address still works: it was not replaced.
	if resp, _ := e.do(e.client(), "POST", "/hooks/"+token, ""); resp.StatusCode != 202 {
		t.Fatalf("the holder's address got %d", resp.StatusCode)
	}
	if got := e.auditActions("rule.hook_changed"); got != 1 {
		t.Fatalf("%d address changes recorded", got)
	}
	// Once they take it over, it is theirs to give an address.
	second.ok("PUT", "/api/v1/rules/"+id, map[string]any{"name": "From outside", "enabled": false, "definition": incomingRule(), "take_over": true})
	second.ok("POST", "/api/v1/rules/"+id+"/hook", map[string]any{})
}

func TestOnlyTheHolderStartsByHandARuleWithAnotherTrigger(t *testing.T) {
	e := newEnv(t)
	owner, _, space, page := ruleTeam(t, e)
	second := secondManager(t, e, space, owner)
	id := owner.rule(space, "Tag", onComment(map[string]any{"type": "add_label", "label": "x"}))

	if code := errorCode(t, second.want(409, "POST", "/api/v1/rules/"+id+"/run", map[string]any{"page_id": page})); code != "rule_held" {
		t.Fatalf("code %q", code)
	}
	e.settle()
	if got := e.count("SELECT count(*) FROM automation_runs WHERE rule_id = $1", id); got != 0 {
		t.Fatalf("%d runs started by someone who does not hold the rule", got)
	}
	if got := labelsOf(owner, page); len(got) != 0 {
		t.Fatalf("the rule acted: %v", got)
	}
	owner.want(202, "POST", "/api/v1/rules/"+id+"/run", map[string]any{"page_id": page})
}

func TestOnlyTheHolderStartsAnInstanceRuleByHand(t *testing.T) {
	e := newEnv(t)
	owner, _, space, page := ruleTeam(t, e)
	other := e.user("admin2@example.com", identity.PermUseProduct, identity.PermAdministrator)
	owner.grant(space, "user", other.id, permissions.TemplateViewer)
	id := instanceRule(owner, "Tag everywhere", nil, onComment(map[string]any{"type": "add_label", "label": "x"}))
	other.want(409, "POST", "/api/v1/rules/"+id+"/run", map[string]any{"page_id": page})
	if got := e.count("SELECT count(*) FROM automation_runs WHERE rule_id = $1", id); got != 0 {
		t.Fatalf("%d runs started by another administrator", got)
	}
}

func TestByHandAnInstanceRuleStartsOnlyInItsSpaces(t *testing.T) {
	e := newEnv(t)
	owner, reader, chosen, _ := ruleTeam(t, e)
	other, _ := owner.space("OTH")
	owner.grant(other, "user", reader.id, permissions.TemplateViewer)
	elsewhere := owner.page(other, nil, "Elsewhere", "x")
	id := instanceRule(owner, "Label now", []string{chosen}, map[string]any{"trigger": map[string]any{"kind": "manual"},
		"actions": []map[string]any{{"type": "add_label", "label": "x"}}})

	reader.want(404, "POST", "/api/v1/rules/"+id+"/run", map[string]any{"page_id": elsewhere})
	e.settle()
	if got := labelsOf(owner, elsewhere); len(got) != 0 {
		t.Fatalf("the rule acted outside its spaces: %v", got)
	}
}
