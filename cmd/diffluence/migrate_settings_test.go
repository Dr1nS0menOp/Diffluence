// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// diffluence migrate on a restored database with nothing left to migrate still sets the
// defaults the dump did not carry, so a restore that ends with it plans well (spec 065).
func TestMigrateSetsTheSessionSettingsAgainWithNothingToMigrate(t *testing.T) {
	url := commandEnv(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"migrate"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("migrate: %d %s", code, errOut.String())
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	var name string
	if err := conn.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{"plan_cache_mode", "jit"} {
		if _, err := conn.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{name}.Sanitize()+" RESET "+setting); err != nil {
			t.Fatal(err)
		}
	}
	conn.Close(ctx)
	if code := run([]string{"migrate"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("migrate again: %d %s", code, errOut.String())
	}
	fresh, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close(ctx)
	for setting, want := range map[string]string{"plan_cache_mode": "force_custom_plan", "jit": "off"} {
		var got string
		if err := fresh.QueryRow(ctx, "SHOW "+setting).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s = %q after migrate with nothing to migrate, want %q", setting, got, want)
		}
	}
}
