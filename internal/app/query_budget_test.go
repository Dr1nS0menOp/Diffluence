// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"net/url"
	"testing"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
)

// queryBudgets are the queries each main screen may cost, as one request (ARC-239). A change
// that adds queries fails here, unless the budget is raised on purpose, in the same change,
// with the reason in its spec (ARC-238). A screen that costs less should get a lower budget.
var queryBudgets = map[string]int64{
	"open a page": 25,
	"the tree":    14,
	// Two more since spec 047: titles that hold the typed text, and titles near it, each
	// asked only when the earlier tiers leave room on the page.
	"search":           19,
	"the bell":         3,
	"a page's summary": 13,
	"the space":        11,
}

// queriesOf counts the queries of one request. Background work can add queries in the same
// moment, never take them away, so the lowest of a few runs is the cost of the request.
func queriesOf(t *testing.T, e *env, u *user, path string) int64 {
	t.Helper()
	lowest := int64(-1)
	for range 3 {
		before := db.Queries(e.app.Pool())
		u.ok("GET", path, nil)
		if n := db.Queries(e.app.Pool()) - before; lowest < 0 || n < lowest {
			lowest = n
		}
	}
	return lowest
}

func TestTheMainScreensStayWithinTheirQueryBudget(t *testing.T) {
	e := newEnv(t)
	owner := e.user("budget@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, home := owner.space("BUDGET")
	parent := owner.page(space, home, "Budget parent", "plan and review")
	page := owner.page(space, parent, "Budget page", "the release plan and its review")
	owner.page(space, parent, "Budget sibling", "another plan")

	paths := map[string]string{
		"open a page":      "/api/v1/pages/" + page,
		"the tree":         "/api/v1/spaces/" + space + "/tree/branches",
		"search":           "/api/v1/search?q=" + url.QueryEscape("plan"),
		"the bell":         "/api/v1/me/notifications/count",
		"a page's summary": "/api/v1/pages/" + page + "/summary",
		"the space":        "/api/v1/spaces/" + space,
	}
	for screen, path := range paths {
		got := queriesOf(t, e, owner, path)
		budget := queryBudgets[screen]
		switch {
		case got > budget:
			t.Errorf("%s costs %d queries, over its budget of %d", screen, got, budget)
		case got < budget:
			t.Logf("%s costs %d queries, under its budget of %d: lower the budget", screen, got, budget)
		}
	}
}
