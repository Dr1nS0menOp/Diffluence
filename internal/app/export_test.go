// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"

	"github.com/diffluence/diffluence/internal/db"
)

// AddMigrationForTest makes this binary know one more migration.
func (a *App) AddMigrationForTest(m db.Migration) { a.migrations = append(a.migrations, m) }

// SignCheckpointForTest runs the daily job.
func (a *App) SignCheckpointForTest(ctx context.Context) error { return a.signCheckpoint(ctx) }

// VerifyAuditForTest runs the daily job.
func (a *App) VerifyAuditForTest(ctx context.Context) error { return a.verifyAudit(ctx) }
