// SPDX-License-Identifier: Apache-2.0

// Package settings holds the few options an administrator can change at runtime.
// Everything that holds a secret stays in the environment on purpose.
package settings

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
)

const (
	keyTrashRetentionDays = "trash_retention_days"
	keyMaxUploadMiB       = "max_upload_mib"
)

// Values are the runtime settings.
type Values struct {
	TrashRetentionDays int `json:"trash_retention_days"`
	MaxUploadMiB       int `json:"max_upload_mib"`
}

// Defaults apply until an administrator changes something (spec 003).
var Defaults = Values{TrashRetentionDays: 30, MaxUploadMiB: 100}

// Service reads and writes settings.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Get returns the current values.
func (s *Service) Get(ctx context.Context) (Values, error) {
	v := Defaults
	rows, err := s.pool.Query(ctx, "SELECT key, value FROM settings WHERE key = ANY($1)",
		[]string{keyTrashRetentionDays, keyMaxUploadMiB})
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return v, err
		}
		var n int
		if err := json.Unmarshal(raw, &n); err != nil {
			continue // a damaged value falls back to the default
		}
		switch key {
		case keyTrashRetentionDays:
			v.TrashRetentionDays = n
		case keyMaxUploadMiB:
			v.MaxUploadMiB = n
		}
	}
	return v, rows.Err()
}

// TrashRetention is how long deleted items are kept.
func (s *Service) TrashRetention(ctx context.Context) (time.Duration, error) {
	v, err := s.Get(ctx)
	return time.Duration(v.TrashRetentionDays) * 24 * time.Hour, err
}

// MaxUploadBytes is the largest attachment accepted.
func (s *Service) MaxUploadBytes(ctx context.Context) (int64, error) {
	v, err := s.Get(ctx)
	return int64(v.MaxUploadMiB) << 20, err
}

// Update stores new values. The caller must be an administrator (route rule).
func (s *Service) Update(ctx context.Context, c identity.Caller, v Values) error {
	if v.TrashRetentionDays < 1 || v.TrashRetentionDays > 3650 {
		return apperr.Invalid("Trash retention must be between 1 and 3650 days.")
	}
	if v.MaxUploadMiB < 1 || v.MaxUploadMiB > 10240 {
		return apperr.Invalid("The maximum upload size must be between 1 MiB and 10 GiB.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for key, value := range map[string]int{keyTrashRetentionDays: v.TrashRetentionDays, keyMaxUploadMiB: v.MaxUploadMiB} {
			if _, err := tx.Exec(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)
				ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, key, value); err != nil {
				return err
			}
		}
		if _, err := audit.Record(ctx, tx, c.Event("settings.updated", "settings", "", "", map[string]any{
			"trash_retention_days": v.TrashRetentionDays, "max_upload_mib": v.MaxUploadMiB})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "settings.updated", v)
		return err
	})
}
