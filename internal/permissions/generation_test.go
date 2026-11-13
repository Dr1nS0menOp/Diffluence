// SPDX-License-Identifier: Apache-2.0

package permissions_test

import (
	"context"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/testutil"
)

// A long transaction that changes who may do what, such as an import or a sync of people,
// does not make every other such change wait for it: the change is counted when it commits.
// A change that rolls back counts nothing.
func TestALongPermissionChangeDoesNotHoldUpOthers(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	pool := tdb.Pool
	inst, first, second, third, grp := ids.New(), ids.New(), ids.New(), ids.New(), ids.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, 'member', 'active', 'a@example.com', 'a')", []any{first, inst}},
		{"INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, 'member', 'active', 'b@example.com', 'b')", []any{second, inst}},
		{"INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, 'member', 'active', 'c@example.com', 'c')", []any{third, inst}},
		{"INSERT INTO groups (id, origin_instance_id, name, source) VALUES ($1, $2, 'g', 'local')", []any{grp, inst}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	generation := func() int64 {
		t.Helper()
		var n int64
		if err := pool.QueryRow(ctx, "SELECT n FROM permission_generation").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	rolledBack, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before := generation()
	if _, err := rolledBack.Exec(ctx, "INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)", grp, third); err != nil {
		t.Fatal(err)
	}
	if err := rolledBack.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if generation() != before {
		t.Fatal("a change that rolled back was counted")
	}

	long, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = long.Rollback(ctx) }()
	for _, person := range []any{first, third} {
		if _, err := long.Exec(ctx, "INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)", grp, person); err != nil {
			t.Fatal(err)
		}
	}

	short, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := pool.Exec(short, "INSERT INTO group_members (group_id, user_id) VALUES ($1, $2)", grp, second); err != nil {
		t.Fatalf("a change waited for a long one: %v", err)
	}
	afterShort := generation()
	if afterShort <= before {
		t.Fatal("the short change was not counted")
	}
	if err := long.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if got := generation(); got != afterShort+1 {
		t.Fatalf("the long change of two statements counted %d times, want once at its commit", got-afterShort)
	}
}
