// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"slices"
	"testing"
)

// ADR-037: a search finds the common forms of a word, and the title as typed stays on top.
func TestSearchFindsWordFormsAndKeepsTheExactTitleFirst(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	owner.page(space, nil, "Plan", "x")
	owner.page(space, nil, "Travel plans", "x")
	owner.page(space, nil, "Policies", "the budget of each team")
	owner.page(space, nil, "Planet", "x")
	e.settle()

	plan := titlesIn(viewer.ok("GET", "/api/v1/search?q=plan", nil), "results")
	if len(plan) < 2 || plan[0] != "Plan" || !slices.Contains(plan, "Travel plans") {
		t.Fatalf("plan: %v, want Plan first and Travel plans found", plan)
	}
	plans := titlesIn(viewer.ok("GET", "/api/v1/search?q=plans%20travel", nil), "results")
	if !slices.Contains(plans, "Travel plans") {
		t.Fatalf("plans travel: %v", plans)
	}
	if got := titlesIn(viewer.ok("GET", "/api/v1/search?q=plans%20x", nil), "results"); !slices.Contains(got, "Plan") {
		t.Fatalf("plans did not find plan: %v", got)
	}
	if got := titlesIn(viewer.ok("GET", "/api/v1/search?q=policy%20team", nil), "results"); !slices.Contains(got, "Policies") {
		t.Fatalf("policy did not find policies: %v", got)
	}
	if got := titlesIn(viewer.ok("GET", "/api/v1/search?q=budgets%20team", nil), "results"); !slices.Contains(got, "Policies") {
		t.Fatalf("budgets did not find budget: %v", got)
	}
	// Quotes ask for the words as typed.
	if got := titlesIn(viewer.ok("GET", "/api/v1/search?q=%22plans%22", nil), "results"); slices.Contains(got, "Plan") {
		t.Fatalf("a quoted word found another form: %v", got)
	}
}
