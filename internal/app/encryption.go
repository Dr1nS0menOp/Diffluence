// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/storage"
)

// encryptRunBudget bounds one run of the job, so a large store is done over several runs and a
// worker is never held for hours.
const encryptRunBudget = 2 * time.Minute

// everyFileEncrypted says whether the job has encrypted every stored file.
func (a *App) everyFileEncrypted(ctx context.Context) (bool, error) {
	return storageAllEncrypted(ctx, a.pool)
}

// storageAllEncrypted reads the state without an App, for the key commands.
func storageAllEncrypted(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var done bool
	err := pool.QueryRow(ctx, "SELECT encrypted_all_at IS NOT NULL FROM storage_state").Scan(&done)
	if err != nil {
		return false, fmt.Errorf("app: storage state: %v", err)
	}
	return done, nil
}

// encryptStoredFiles encrypts files written before encryption existed (SEC-725). Files keep
// their names, so nothing in the database changes. Each run starts after the last file the
// previous run finished, in name order, so a store of millions of files gets through (spec
// 034). When a whole pass from the first file finds no plain file, it records that, and from
// then on plain files are refused.
func (a *App) encryptStoredFiles(ctx context.Context) error {
	return a.encryptStoredFilesWithin(ctx, encryptRunBudget)
}

// encryptStoredFilesWithin is one run with its own budget. It always finishes at least one file,
// so a run with a short budget still moves on.
func (a *App) encryptStoredFilesWithin(ctx context.Context, budget time.Duration) error {
	done, err := a.everyFileEncrypted(ctx)
	if err != nil || done {
		return err
	}
	var cursor string
	if err := a.pool.QueryRow(ctx, "SELECT encrypt_cursor FROM storage_state").Scan(&cursor); err != nil {
		return fmt.Errorf("app: encrypt cursor: %v", err)
	}
	names, err := sortedKeys(ctx, a.encrypted)
	if err != nil {
		return err
	}
	fromStart := cursor == ""
	deadline := time.Now().Add(budget)
	encrypted, visited := 0, 0
	for _, name := range names {
		if name <= cursor {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if visited > 0 && time.Now().After(deadline) {
			a.log.Info("encrypted stored files", "files", encrypted, "stopped_after", cursor)
			return saveCursor(ctx, a.pool, "encrypt_cursor", cursor)
		}
		cursor = name
		visited++
		isEncrypted, err := a.encrypted.IsEncrypted(ctx, name)
		if errors.Is(err, storage.ErrNotFound) {
			continue // deleted while this pass ran
		}
		if err != nil {
			return err
		}
		if isEncrypted {
			continue
		}
		if err := a.encrypted.EncryptInPlace(ctx, name); err != nil && !errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("app: encrypt %s: %w", name, err)
		}
		encrypted++
	}
	if err := saveCursor(ctx, a.pool, "encrypt_cursor", ""); err != nil {
		return err
	}
	if encrypted > 0 {
		a.log.Info("encrypted stored files", "files", encrypted)
	}
	if encrypted > 0 || !fromStart {
		return nil // the next pass, from the first file, confirms that nothing plain is left
	}
	if _, err := a.pool.Exec(ctx, "UPDATE storage_state SET encrypted_all_at = now() WHERE encrypted_all_at IS NULL"); err != nil {
		return fmt.Errorf("app: record encryption: %v", err)
	}
	a.encrypted.MarkEveryFileEncrypted()
	return audit.RecordStandalone(ctx, a.pool, audit.Event{ActorType: audit.ActorSystem, ActorID: "worker",
		Action: "storage.encrypted_all", ObjectType: "storage", Result: audit.ResultSuccess,
		Details: map[string]any{"files": len(names)}})
}
