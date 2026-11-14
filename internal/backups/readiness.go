// SPDX-License-Identifier: Apache-2.0

package backups

import (
	"context"

	"github.com/diffluence/diffluence/internal/db"
)

// Readiness is what a restore must know about backups before it is asked for.
type Readiness struct {
	// Complete says the worker reported the named backup whole.
	Complete bool
	// BackupOpen says a backup was asked for and is not finished yet.
	BackupOpen bool
}

// ReadinessToRestore reads, through q, whether the named backup can be restored now, so
// restore never names the tables of backups (ARC-027).
func ReadinessToRestore(ctx context.Context, q db.Querier, name string) (Readiness, error) {
	var r Readiness
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM backup_runs WHERE name = $1 AND ok),
			EXISTS (SELECT 1 FROM backup_requests WHERE finished_at IS NULL)`, name).Scan(&r.Complete, &r.BackupOpen)
	return r, err
}
