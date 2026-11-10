// SPDX-License-Identifier: Apache-2.0

package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrNewerContract means a newer release ran a contract migration: it removed
// something this release may still read, so this release must not serve.
var ErrNewerContract = errors.New("db: a newer release already removed what this release may need")

// ErrPostgresTooOld means the database server is older than the oldest one supported.
var ErrPostgresTooOld = errors.New("db: PostgreSQL is too old")

// minPostgresVersion is PostgreSQL 16, in the form of server_version_num
// (docs/blueprint/04-architecture/01-system-and-roles.md).
const minPostgresVersion = 160000

// RequirePostgres refuses a database server older than the oldest one supported.
func RequirePostgres(ctx context.Context, q Querier) error {
	var number int
	var text string
	err := q.QueryRow(ctx, "SELECT current_setting('server_version_num')::int, current_setting('server_version')").Scan(&number, &text)
	if err != nil {
		return fmt.Errorf("db: read the PostgreSQL version: %w", err)
	}
	return checkPostgresVersion(number, text)
}

func checkPostgresVersion(number int, text string) error {
	if number < minPostgresVersion {
		return fmt.Errorf("%w: version 16 or newer is needed, this server runs %s", ErrPostgresTooOld, text)
	}
	return nil
}

// CheckCompatible refuses a database in which a newer release ran a contract
// migration. Expand migrations of a newer release are fine: they only add.
func CheckCompatible(ctx context.Context, q Querier, migs []Migration) error {
	exists, err := hasMigrationsTable(ctx, q)
	if err != nil || !exists {
		return err
	}
	newest := 0
	if n := len(migs); n > 0 {
		newest = migs[n-1].Version
	}
	var version int
	var name string
	err = q.QueryRow(ctx, `SELECT version, name FROM schema_migrations WHERE phase = $1 AND version > $2
		ORDER BY version LIMIT 1`, string(PhaseContract), newest).Scan(&version, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: contract migration %04d_%s ran; start that release or a newer one, or restore the backup made before the upgrade",
		ErrNewerContract, version, name)
}

// SchemaStatus says how far the database is, as seen by this release.
type SchemaStatus struct {
	Newest          int // the highest migration applied, also one of a newer release; 0 = none yet
	PendingExpand   int // expand migrations of this release not applied yet
	PendingContract int // contract migrations of this release not applied yet
}

// Status reads how far the database is. A database without migrations is not an error.
func Status(ctx context.Context, q Querier, migs []Migration) (SchemaStatus, error) {
	var st SchemaStatus
	exists, err := hasMigrationsTable(ctx, q)
	if err != nil {
		return st, err
	}
	applied := map[int]string{}
	if exists {
		if applied, err = appliedChecksums(ctx, q); err != nil {
			return st, err
		}
	}
	for v := range applied {
		st.Newest = max(st.Newest, v)
	}
	for _, m := range migs {
		if _, ok := applied[m.Version]; ok {
			continue
		}
		if m.Phase == PhaseContract {
			st.PendingContract++
		} else {
			st.PendingExpand++
		}
	}
	return st, nil
}

// hasMigrationsTable reports whether any release ever migrated this database.
func hasMigrationsTable(ctx context.Context, q Querier) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, "SELECT to_regclass('schema_migrations') IS NOT NULL").Scan(&exists)
	return exists, err
}
