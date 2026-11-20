// SPDX-License-Identifier: Apache-2.0

package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/db"
)

// migrated is the database a test binary migrates once and copies for each test that asks for
// a migrated one, after its TestMain called MainWithTemplate. It is package state on purpose:
// a cache of one process, never shared with another, because each test binary embeds its own
// migrations and a mutated one must never reuse what an unmutated one built.
var migrated struct {
	// wanted is set by MainWithTemplate before any test runs, and only read after.
	wanted bool
	once   sync.Once
	name   string
	err    error
}

// MainWithTemplate runs the tests of a package, migrating one database the first time a test
// asks for a migrated one, and giving each such test a copy of it: the same tables, rows and
// database settings as migrating anew, at the cost of a file copy. It drops that database at
// the end, and returns the exit code for os.Exit.
func MainWithTemplate(m *testing.M) int {
	migrated.wanted = true
	code := m.Run()
	if migrated.name != "" {
		if err := dropDatabase(migrated.name); err != nil {
			fmt.Fprintln(os.Stderr, "testutil: drop the migrated template:", err)
			code = 1
		}
	}
	return code
}

// cloneMigrated creates a copy of the migrated database for the test.
func cloneMigrated(t testing.TB) TestDB {
	t.Helper()
	if os.Getenv(EnvDatabaseURL) == "" {
		t.Skipf("%s not set; skipping integration test", EnvDatabaseURL)
	}
	migrated.once.Do(func() { migrated.name, migrated.err = buildMigrated() })
	if migrated.err != nil {
		t.Fatalf("migrate the template database: %v", migrated.err)
	}
	return createDatabase(t, migrated.name)
}

// buildMigrated creates and migrates the database every copy starts from, and closes every
// connection to it, since PostgreSQL copies only a database nobody is connected to.
func buildMigrated() (string, error) {
	ctx := context.Background()
	base := os.Getenv(EnvDatabaseURL)
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		return "", fmt.Errorf("connect to test server: %w", err)
	}
	defer func() { _ = admin.Close(ctx) }()
	name := randomName("dfl_template_")
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return "", fmt.Errorf("create the template: %w", err)
	}
	// The name comes back also when migrating fails, so MainWithTemplate drops the database.
	return name, migrateDatabase(ctx, base, name)
}

// migrateDatabase runs every migration on the database name of the server at base.
func migrateDatabase(ctx context.Context, base, name string) error {
	u, err := url.Parse(base)
	if err != nil {
		return fmt.Errorf("%s must be a URL: %w", EnvDatabaseURL, err)
	}
	u.Path = "/" + name
	pool, err := db.Open(ctx, u.String())
	if err != nil {
		return err
	}
	defer pool.Close()
	migs, err := db.Migrations()
	if err != nil {
		return err
	}
	_, err = db.Migrate(ctx, pool, migs, db.Options{})
	return err
}

// copyDatabaseSettings sets on database to the defaults that the migrations set on database
// from with ALTER DATABASE ... SET: PostgreSQL keeps those per database and does not copy
// them with it. Names and values come from the catalog and are quoted by format().
func copyDatabaseSettings(ctx context.Context, admin *pgx.Conn, from, to string) error {
	rows, err := admin.Query(ctx, `SELECT format('ALTER DATABASE %I SET %I = %L', $1::text,
			split_part(setting, '=', 1), substr(setting, strpos(setting, '=') + 1))
		FROM pg_db_role_setting s JOIN pg_database d ON d.oid = s.setdatabase, unnest(s.setconfig) setting
		WHERE d.datname = $2 AND s.setrole = 0`, to, from)
	if err != nil {
		return fmt.Errorf("read the settings of the template: %w", err)
	}
	statements, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("read the settings of the template: %w", err)
	}
	for _, stmt := range statements {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("copy a setting of the template: %w", err)
		}
	}
	return nil
}

// dropDatabase drops the database name of the test server.
func dropDatabase(name string) error {
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, os.Getenv(EnvDatabaseURL))
	if err != nil {
		return err
	}
	defer func() { _ = admin.Close(ctx) }()
	_, err = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
	return err
}

// randomName is prefix and twelve random hex digits, a name no other test takes.
func randomName(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
