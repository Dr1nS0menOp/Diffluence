// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/keys"
	"github.com/diffluence/diffluence/internal/storage"
)

// KeyTools is what the key commands need, without starting a server: the database, the
// stored files and the keys of this installation (spec 034).
type KeyTools struct {
	Pool    *pgxpool.Pool
	Store   *storage.Encrypted
	Keyring *keys.Keyring
}

// OpenKeyTools loads the keys as they are on disk now, including a key on its way in or out.
// It never creates a key: a missing key file is an error.
func OpenKeyTools(ctx context.Context, cfg config.Config) (*KeyTools, error) {
	keyring, err := keys.LoadOrCreate(cfg.KeyFile(), false)
	if err != nil {
		return nil, err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	disk, err := storage.NewDisk(filepath.Join(cfg.DataDir, "objects"))
	if err != nil {
		pool.Close()
		return nil, err
	}
	allDone := func(ctx context.Context) (bool, error) { return storageAllEncrypted(ctx, pool) }
	return &KeyTools{Pool: pool, Store: storage.NewEncrypted(disk, keyring, allDone), Keyring: keyring}, nil
}

// Close lets go of the database.
func (t *KeyTools) Close() { t.Pool.Close() }
