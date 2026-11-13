// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"crypto/ed25519"
	"time"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/mail"
	"github.com/diffluence/diffluence/internal/sharing"
)

// AddMigrationForTest makes this binary know one more migration.
func (a *App) AddMigrationForTest(m db.Migration) { a.migrations = append(a.migrations, m) }

// InstallationKeyForTest is the checkpoint key derived from master.key, which endorses the
// backup service's key (spec 070).
func (a *App) InstallationKeyForTest() ed25519.PrivateKey {
	key, err := a.keyring.SigningKey()
	if err != nil {
		panic(err)
	}
	return key
}

// SignCheckpointForTest runs the daily job.
func (a *App) SignCheckpointForTest(ctx context.Context) error { return a.signCheckpoint(ctx) }

// CheckWitnessForTest compares the chain with checkpoints taken from an application log,
// as `diffluence audit verify --witness` does.
func (a *App) CheckWitnessForTest(ctx context.Context, witness []audit.Checkpoint) (audit.WitnessResult, error) {
	key, err := a.keyring.SigningKey()
	if err != nil {
		return audit.WitnessResult{}, err
	}
	public, _ := key.Public().(ed25519.PublicKey)
	return audit.CompareWitness(ctx, a.pool, public, witness)
}

// RunSealerForTest runs the sealing loop of a worker until ctx ends.
func (a *App) RunSealerForTest(ctx context.Context, every time.Duration) { a.runSealer(ctx, every) }

// SealEveryForTest is how often a worker seals.
const SealEveryForTest = sealEvery

// VerifyAuditForTest runs the daily job.
func (a *App) VerifyAuditForTest(ctx context.Context) error { return a.verifyAudit(ctx) }

// EncryptStoredFilesForTest runs the job that encrypts files written before encryption.
func (a *App) EncryptStoredFilesForTest(ctx context.Context) error { return a.encryptStoredFiles(ctx) }

// EncryptStoredFilesWithinForTest runs the encryption job with a budget of its own.
func (a *App) EncryptStoredFilesWithinForTest(ctx context.Context, budget time.Duration) error {
	return a.encryptStoredFilesWithin(ctx, budget)
}

// ResealWithHookForTest runs ResealKeys with a function that runs between reading a value and
// replacing it, so a test can change the value meanwhile.
func ResealWithHookForTest(ctx context.Context, t *KeyTools, hook func(table, column, id string)) (ResealReport, error) {
	r := resealer{pool: t.Pool, store: t.Store, keyring: t.Keyring, deadline: time.Now().Add(time.Hour),
		beforeUpdate: func(c sealedColumn, id string) { hook(c.table, c.column, id) }}
	return r.run(ctx)
}

// UseMailForTest sends account mail through the queue, as with a mail server.
func (a *App) UseMailForTest() {
	a.identity.SetMailer(accountMailer{keyring: a.keyring}, "http://example.test")
}

// RenderMailForTest writes a mail from the queue.
func (a *App) RenderMailForTest(ctx context.Context, j mail.Job) (mail.Rendered, error) {
	return a.renderMail(ctx, j)
}

// EventListenersForTest gives every durable listener of the queue by name.
func (a *App) EventListenersForTest() map[string]func(context.Context, []byte) error {
	out := map[string]func(context.Context, []byte) error{}
	for name, handle := range a.eventListeners() {
		out[name] = handle
	}
	return out
}

// SharingForTest is the sharing module, for a test that calls it without the HTTP layer.
func (a *App) SharingForTest() *sharing.Service { return a.sharing }

// InMaintenanceForTest tells whether mail and jobs wait.
func (a *App) InMaintenanceForTest(ctx context.Context) (bool, error) { return a.inMaintenance(ctx) }
