// SPDX-License-Identifier: Apache-2.0

package permissions_test

import (
	"context"
	"testing"

	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/testutil"
)

// TestAChangeEndsKeptFactsAtOnce: a node keeps the facts of a person and a space between
// requests (ARC-231), and every change to them counts from the next request on, on every
// node. Two services stand for two nodes.
func TestAChangeEndsKeptFactsAtOnce(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	pool := tdb.Pool
	nodeA, nodeB := permissions.NewService(pool), permissions.NewService(pool)
	inst, person, grp, space := ids.New(), ids.New(), ids.New(), ids.New()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	exec("INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, 'member', 'active', 'c@example.com', 'c')", person, inst)
	exec("INSERT INTO groups (id, origin_instance_id, name, source) VALUES ($1, $2, 'g', 'local')", grp, inst)
	exec("INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by) VALUES ($1, $2, 'CACHE', 's', 'active', $3)", space, inst, person)
	exec("SELECT rbac_set_exact(NULL, 'user', $1, ARRAY['use_product'])", person)

	may := func(node *permissions.Service, action string) bool {
		t.Helper()
		d, err := node.CheckSpace(permissions.WithMemo(ctx), pool, person, space, action)
		if err != nil {
			t.Fatal(err)
		}
		return d.Allowed
	}
	both := func(action string, want bool, why string) {
		t.Helper()
		for i, node := range []*permissions.Service{nodeA, nodeB} {
			if got := may(node, action); got != want {
				t.Fatalf("node %d, %s: %s is %v, want %v", i, why, action, got, want)
			}
		}
	}

	// Failure paths first: what takes access away counts at once.
	exec("SELECT rbac_set_exact($1, 'user', $2, $3::text[])", space, person, []string{permissions.SpaceView, permissions.PageEdit})
	both(permissions.PageEdit, true, "with the grant")
	both(permissions.PageEdit, true, "kept")
	exec("SELECT rbac_set_exact($1, 'user', $2, $3::text[])", space, person, []string{permissions.SpaceView})
	both(permissions.PageEdit, false, "after the grant was revoked")

	exec("SELECT rbac_set_exact($1, 'group', $2, $3::text[])", space, grp, []string{permissions.SpaceView, permissions.PageEdit})
	exec("INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)", grp, person)
	both(permissions.PageEdit, true, "through the group")
	exec("DELETE FROM group_members WHERE group_id = $1 AND user_id = $2", grp, person)
	both(permissions.PageEdit, false, "after leaving the group")

	exec("SELECT rbac_set_exact($1, 'user', $2, $3::text[])", space, person, []string{permissions.SpaceView, permissions.PageEdit})
	both(permissions.PageEdit, true, "before archiving")
	exec("UPDATE spaces SET status = 'archived' WHERE id = $1", space)
	both(permissions.PageEdit, false, "in an archived space")
	exec("UPDATE spaces SET status = 'active' WHERE id = $1", space)
	both(permissions.SpaceView, true, "before the account changes") // keeps the account as it is now

	exec("UPDATE users SET expires_at = now() - interval '1 second' WHERE id = $1", person)
	both(permissions.SpaceView, false, "past the end of the account")
	exec("UPDATE users SET expires_at = NULL, status = 'disabled' WHERE id = $1", person)
	both(permissions.SpaceView, false, "while disabled")
	exec("UPDATE users SET status = 'active' WHERE id = $1", person)
	exec("SELECT rbac_set_exact(NULL, 'user', $1, ARRAY[]::text[])", person)
	both(permissions.SpaceView, false, "without the product")

	// And the facts are kept: a second request reads them from memory.
	exec("SELECT rbac_set_exact(NULL, 'user', $1, ARRAY['use_product'])", person)
	both(permissions.SpaceView, true, "with everything back")
	hits, _ := nodeA.CacheCounts()
	may(nodeA, permissions.SpaceView)
	if after, _ := nodeA.CacheCounts(); after < hits+2 {
		t.Fatalf("the facts of the person and the space were read again: %d hits, then %d", hits, after)
	}
}

// TestAWriteNeverReadsKeptFacts: a request that may change permissions has no memo, so a
// grant it made counts for its own next check.
func TestAWriteNeverReadsKeptFacts(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	pool := tdb.Pool
	svc := permissions.NewService(pool)
	inst, person, space := ids.New(), ids.New(), ids.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, 'member', 'active', 'w@example.com', 'w')", []any{person, inst}},
		{"INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by) VALUES ($1, $2, 'WRITE', 's', 'active', $3)", []any{space, inst, person}},
		{"SELECT rbac_set_exact(NULL, 'user', $1, ARRAY['use_product'])", []any{person}},
		{"SELECT rbac_set_exact($1, 'user', $2, ARRAY['space.view'])", []any{space, person}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.CheckSpace(permissions.WithMemo(ctx), pool, person, space, permissions.SpaceView); err != nil {
		t.Fatal(err)
	}
	hits, _ := svc.CacheCounts()
	if _, err := svc.CheckSpace(ctx, pool, person, space, permissions.SpaceView); err != nil {
		t.Fatal(err)
	}
	if after, _ := svc.CacheCounts(); after != hits {
		t.Fatalf("a check without a memo read kept facts: %d hits, then %d", hits, after)
	}
}
