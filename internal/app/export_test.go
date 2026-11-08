// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/mail"
)

// AddMigrationForTest makes this binary know one more migration.
func (a *App) AddMigrationForTest(m db.Migration) { a.migrations = append(a.migrations, m) }

// SignCheckpointForTest runs the daily job.
func (a *App) SignCheckpointForTest(ctx context.Context) error { return a.signCheckpoint(ctx) }

// VerifyAuditForTest runs the daily job.
func (a *App) VerifyAuditForTest(ctx context.Context) error { return a.verifyAudit(ctx) }

// EncryptStoredFilesForTest runs the job that encrypts files written before encryption.
func (a *App) EncryptStoredFilesForTest(ctx context.Context) error { return a.encryptStoredFiles(ctx) }

// UseMailForTest sends account mail through the queue, as with a mail server.
func (a *App) UseMailForTest() {
	a.identity.SetMailer(accountMailer{pool: a.pool, keyring: a.keyring}, "http://example.test")
}

// RenderMailForTest writes a mail from the queue.
func (a *App) RenderMailForTest(ctx context.Context, j mail.Job) (mail.Rendered, error) {
	return a.renderMail(ctx, j)
}

// InMaintenanceForTest tells whether mail and jobs wait.
func (a *App) InMaintenanceForTest(ctx context.Context) (bool, error) { return a.inMaintenance(ctx) }
