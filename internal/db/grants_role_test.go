// SPDX-License-Identifier: Apache-2.0

package db_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/testutil"
)

// TestApplicationRoleGrantsOnlyThroughRoles: the application role cannot write a
// grant itself, only give a role, which the owner's triggers turn into grants
// (spec 036). So no code path can grant past a role.
func TestApplicationRoleGrantsOnlyThroughRoles(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	role := "dfl_app_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if _, err := tdb.Pool.Exec(ctx, "CREATE ROLE "+role); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = tdb.Pool.Exec(ctx, "DROP OWNED BY "+role)
		_, _ = tdb.Pool.Exec(ctx, "DROP ROLE "+role)
	})
	if err := db.GrantApp(ctx, tdb.Pool, role); err != nil {
		t.Fatal(err)
	}
	inst, user, space := ids.New(), ids.New(), ids.New()
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO users (id, origin_instance_id, type, status, email, display_name)
		VALUES ($1, $2, 'member', 'active', 'a@example.com', 'a')`, user, inst); err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(ctx, `INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by)
		VALUES ($1, $2, 'K', 'k', 'active', $3)`, space, inst, user); err != nil {
		t.Fatal(err)
	}
	asApp := func(sql string, args ...any) error {
		return pgx.BeginFunc(ctx, tdb.Pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+role); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, sql, args...)
			return err
		})
	}
	for _, write := range []string{
		"INSERT INTO space_grants (space_id, subject_type, subject_id, permission) VALUES ($1, 'user', $2, 'space.view')",
		"INSERT INTO global_grants (subject_type, subject_id, permission) SELECT 'user', $2, 'administrator' WHERE $1::uuid IS NOT NULL",
		"DELETE FROM space_grants WHERE space_id = $1 AND subject_id = $2",
	} {
		err := asApp(write, space, user)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("%s: the application role wrote a grant itself (%v)", write, err)
		}
	}
	if err := asApp("SELECT rbac_set_exact($1, 'user', $2, ARRAY['space.view', 'space.export'])", space, user); err != nil {
		t.Fatalf("giving a role as the application role: %v", err)
	}
	var n int
	if err := tdb.Pool.QueryRow(ctx, "SELECT count(*) FROM space_grants WHERE space_id = $1 AND subject_id = $2", space, user).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("a role added %d grants, want 2", n)
	}
	// Deleting a space still takes its grants along, through the foreign key.
	if err := asApp("DELETE FROM spaces WHERE id = $1", space); err != nil {
		t.Fatalf("deleting a space as the application role: %v", err)
	}
	if err := tdb.Pool.QueryRow(ctx, "SELECT count(*) FROM space_grants WHERE space_id = $1", space).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("%d grants outlived their space", n)
	}
}
