// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"testing"
)

// Deleting a parent row on its own must never lift what its children inherit (PRM-177,
// GAP-019). The database refuses it, and a whole branch can still go at once.
func TestDeletingAParentAloneIsRefusedSoNothingFallsOpen(t *testing.T) {
	e := newEnv(t)
	owner, _, other, space := e.team("ENG")
	parent := owner.page(space, nil, "Secret parent", "x")
	child := owner.page(space, parent, "Child", "x")
	owner.restrict(parent, "view", owner.id)
	if resp, _ := e.do(other.c, "GET", "/api/v1/pages/"+child, ""); resp.StatusCode != 404 {
		t.Fatalf("the child must inherit the restriction: %d", resp.StatusCode)
	}

	ctx := context.Background()
	if _, err := e.app.Pool().Exec(ctx, "DELETE FROM pages WHERE id = $1", parent); err == nil {
		t.Fatal("the parent was deleted on its own")
	}
	if resp, _ := e.do(other.c, "GET", "/api/v1/pages/"+child, ""); resp.StatusCode != 404 {
		t.Fatalf("the child became visible: %d", resp.StatusCode)
	}
	if _, err := e.app.Pool().Exec(ctx, "DELETE FROM pages WHERE id = ANY($1)", []string{parent, child}); err != nil {
		t.Fatalf("a whole branch at once: %v", err)
	}
}
