// SPDX-License-Identifier: Apache-2.0

package settings_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/testutil"
)

// Another module reads its own stored choice through Load, so only settings names its table.
func TestLoadFillsAStoredValueAndLeavesAMissingOneAlone(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	type choice struct {
		Mode string `json:"mode"`
		Days int    `json:"days"`
	}
	got := choice{Mode: "default", Days: 7}
	if err := settings.Load(ctx, tdb.Pool, "rv5-choice", &got); err != nil {
		t.Fatal(err)
	}
	if got != (choice{Mode: "default", Days: 7}) {
		t.Fatalf("a missing value changed the defaults: %+v", got)
	}
	if err := pgx.BeginFunc(ctx, tdb.Pool, func(tx pgx.Tx) error {
		return settings.Store(ctx, tx, "rv5-choice", []byte(`{"mode":"names"}`))
	}); err != nil {
		t.Fatal(err)
	}
	if err := settings.Load(ctx, tdb.Pool, "rv5-choice", &got); err != nil {
		t.Fatal(err)
	}
	if got != (choice{Mode: "names", Days: 7}) {
		t.Fatalf("the stored value was not read over the defaults: %+v", got)
	}
	if err := settings.Load(ctx, tdb.Pool, "rv5-choice", new(int)); err == nil {
		t.Fatal("a value that does not fit is no error")
	}
}
