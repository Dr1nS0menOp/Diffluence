// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/storage"
)

// encryptRunBudget bounds one run of the job, so a large store is done over several runs and a
// worker is never held for hours. The job picks up where it stopped, because it skips what is
// encrypted already.
const encryptRunBudget = 2 * time.Minute

// everyFileEncrypted says whether the job has encrypted every stored file.
func (a *App) everyFileEncrypted(ctx context.Context) (bool, error) {
	var done bool
	err := a.pool.QueryRow(ctx, "SELECT encrypted_all_at IS NOT NULL FROM storage_state").Scan(&done)
	if err != nil {
		return false, fmt.Errorf("app: storage state: %v", err)
	}
	return done, nil
}

// encryptStoredFiles encrypts files written before encryption existed (SEC-725). Files keep
// their names, so nothing in the database changes. When a whole pass finds no plain file, it
// records that, and from then on plain files are refused.
func (a *App) encryptStoredFiles(ctx context.Context) error {
	done, err := a.everyFileEncrypted(ctx)
	if err != nil || done {
		return err
	}
	listed, err := a.encrypted.List(ctx)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(encryptRunBudget)
	encrypted, plainLeft := 0, 0
	for _, obj := range listed {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		isEncrypted, err := a.encrypted.IsEncrypted(ctx, obj.Key)
		if errors.Is(err, storage.ErrNotFound) {
			continue // deleted while this pass ran
		}
		if err != nil {
			return err
		}
		if isEncrypted {
			continue
		}
		if time.Now().After(deadline) {
			plainLeft++
			continue
		}
		if err := a.encrypted.EncryptInPlace(ctx, obj.Key); err != nil && !errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("app: encrypt %s: %w", obj.Key, err)
		}
		encrypted++
	}
	if encrypted > 0 {
		a.log.Info("encrypted stored files", "files", encrypted, "left", plainLeft)
	}
	if encrypted > 0 || plainLeft > 0 {
		return nil // the next run confirms that nothing plain is left
	}
	if _, err := a.pool.Exec(ctx, "UPDATE storage_state SET encrypted_all_at = now() WHERE encrypted_all_at IS NULL"); err != nil {
		return fmt.Errorf("app: record encryption: %v", err)
	}
	a.encrypted.MarkEveryFileEncrypted()
	return audit.RecordStandalone(ctx, a.pool, audit.Event{ActorType: audit.ActorSystem, ActorID: "worker",
		Action: "storage.encrypted_all", ObjectType: "storage", Result: audit.ResultSuccess,
		Details: map[string]any{"files": len(listed)}})
}
