// SPDX-License-Identifier: Apache-2.0

package permissions_test

import (
	"context"
	"slices"
	"testing"

	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/testutil"
)

func TestTheMemoKeepsEachPersonsOwnFactsAndLivesOneContext(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	pool := tdb.Pool
	svc := permissions.NewService(pool)
	inst, member, guest, space := ids.New(), ids.New(), ids.New(), ids.New()
	for _, sql := range []struct {
		q    string
		args []any
	}{
		{"INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, 'member', 'active', 'm@example.com', 'm')", []any{member, inst}},
		{"INSERT INTO users (id, origin_instance_id, type, status, email, display_name) VALUES ($1, $2, 'guest', 'active', 'g@example.com', 'g')", []any{guest, inst}},
		{"INSERT INTO global_grants (subject_type, subject_id, permission) VALUES ('user', $1, 'use_product')", []any{member}},
		{"INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by) VALUES ($1, $2, 'MEMO', 's', 'active', $3)", []any{space, inst, member}},
	} {
		if _, err := pool.Exec(ctx, sql.q, sql.args...); err != nil {
			t.Fatal(err)
		}
	}

	request := permissions.WithMemo(ctx)
	m, err := svc.LoadUser(request, pool, member)
	if err != nil {
		t.Fatal(err)
	}
	g, err := svc.LoadUser(request, pool, guest)
	if err != nil {
		t.Fatal(err)
	}
	if m.Type != "member" || g.Type != "guest" || !slices.Contains(m.Global, "use_product") || slices.Contains(g.Global, "use_product") {
		t.Fatalf("facts mixed up: member %+v, guest %+v", m, g)
	}

	// Within the request the first answer stands; the next request sees the change.
	if before, err := svc.LoadSpace(request, pool, space); err != nil || len(before.Grants) != 0 {
		t.Fatalf("before the grant: %+v, %v", before, err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO space_grants (space_id, subject_type, subject_id, permission) VALUES ($1, 'user', $2, 'space.view')", space, guest); err != nil {
		t.Fatal(err)
	}
	if same, err := svc.LoadSpace(request, pool, space); err != nil || len(same.Grants) != 0 {
		t.Fatalf("the request did not reuse its facts: %+v, %v", same, err)
	}
	next := permissions.WithMemo(ctx)
	f, err := svc.LoadSpace(next, pool, space)
	if err != nil || len(f.Grants) != 1 {
		t.Fatalf("a new request missed the grant: %+v, %v", f, err)
	}
	if plain, err := svc.LoadSpace(ctx, pool, space); err != nil || len(plain.Grants) != 1 {
		t.Fatalf("without a memo the grant was missed: %+v, %v", plain, err)
	}
}
