// SPDX-License-Identifier: Apache-2.0

// Package testutil gives every integration test its own fresh database.
package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
)

// EnvDatabaseURL must point to a PostgreSQL role that may create databases.
const EnvDatabaseURL = "DIFFLUENCE_TEST_DATABASE_URL"

// TestDB is a throw-away database.
type TestDB struct {
	Pool *pgxpool.Pool
	URL  string
}

// Database creates an empty, migrated database and drops it after the test.
// The test is skipped when DIFFLUENCE_TEST_DATABASE_URL is not set.
func Database(t testing.TB) TestDB {
	t.Helper()
	tdb := EmptyDatabase(t)
	migs, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Migrate(context.Background(), tdb.Pool, migs, db.Options{}); err != nil {
		t.Fatal(err)
	}
	return tdb
}

// EmptyDatabase is Database without migrations.
func EmptyDatabase(t testing.TB) TestDB {
	t.Helper()
	base := os.Getenv(EnvDatabaseURL)
	if base == "" {
		t.Skipf("%s not set; skipping integration test", EnvDatabaseURL)
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect to test server: %v", err)
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "dfl_test_" + hex.EncodeToString(b)
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("%s must be a URL: %v", EnvDatabaseURL, err)
	}
	u.Path = "/" + name
	pool, err := db.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)")
		_ = admin.Close(context.Background())
	})
	return TestDB{Pool: pool, URL: u.String()}
}
