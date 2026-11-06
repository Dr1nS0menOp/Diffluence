// SPDX-License-Identifier: Apache-2.0

// Package settings holds the few options an administrator can change at runtime.
// Everything that holds a secret stays in the environment on purpose.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/storage"
)

const (
	keyTrashRetentionDays = "trash_retention_days"
	keyMaxUploadMiB       = "max_upload_mib"
	keyMailEnabled        = "mail_enabled"
	keyMailExcerpts       = "mail_excerpts"
	keyReactionsEnabled   = "reactions_enabled"
	keyGroupMentions      = "group_mentions"
)

// Values are the runtime settings.
type Values struct {
	TrashRetentionDays int  `json:"trash_retention_days"`
	MaxUploadMiB       int  `json:"max_upload_mib"`
	MailEnabled        bool `json:"mail_enabled"`
	MailExcerpts       bool `json:"mail_excerpts"`
	ReactionsEnabled   bool `json:"reactions_enabled"`
	GroupMentions      bool `json:"group_mentions"`
}

// Defaults apply until an administrator changes something (spec 003, spec 010).
var Defaults = Values{TrashRetentionDays: 30, MaxUploadMiB: 100, MailEnabled: true, ReactionsEnabled: true}

// Service reads and writes settings.
type Service struct {
	pool  *pgxpool.Pool
	store storage.Store
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, store storage.Store) *Service {
	return &Service{pool: pool, store: store}
}

// Get returns the current values.
func (s *Service) Get(ctx context.Context) (Values, error) {
	v := Defaults
	rows, err := s.pool.Query(ctx, "SELECT key, value FROM settings WHERE key = ANY($1)",
		[]string{keyTrashRetentionDays, keyMaxUploadMiB, keyMailEnabled, keyMailExcerpts, keyReactionsEnabled, keyGroupMentions})
	if err != nil {
		return v, err
	}
	defer rows.Close()
	numbers := map[string]*int{keyTrashRetentionDays: &v.TrashRetentionDays, keyMaxUploadMiB: &v.MaxUploadMiB}
	switches := map[string]*bool{keyMailEnabled: &v.MailEnabled, keyMailExcerpts: &v.MailExcerpts,
		keyReactionsEnabled: &v.ReactionsEnabled, keyGroupMentions: &v.GroupMentions}
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return v, err
		}
		// A damaged value falls back to the default.
		if target, ok := numbers[key]; ok {
			var n int
			if json.Unmarshal(raw, &n) == nil {
				*target = n
			}
		} else if target, ok := switches[key]; ok {
			var on bool
			if json.Unmarshal(raw, &on) == nil {
				*target = on
			}
		}
	}
	return v, rows.Err()
}

// Read fills v with one stored value, and leaves it alone when there is none.
func (s *Service) Read(ctx context.Context, key string, v any) error {
	var raw []byte
	err := s.pool.QueryRow(ctx, "SELECT value FROM settings WHERE key = $1", key).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

// Write stores one value, and audits who did it. The route decides who may.
func (s *Service) Write(ctx context.Context, c identity.Caller, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, key, raw); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("settings.updated", "settings", key, "", map[string]any{"key": key})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "settings.updated", map[string]any{"key": key})
		return err
	})
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

// HasMark reports whether something the instance only does once has been done.
func (s *Service) HasMark(ctx context.Context, name string) (bool, error) {
	var found bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM settings WHERE key = $1)", name).Scan(&found)
	return found, err
}

// SetMark remembers that something the instance only does once has been done.
func (s *Service) SetMark(ctx context.Context, name string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO settings (key, value) VALUES ($1, 'true') ON CONFLICT (key) DO NOTHING`, name)
	return err
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
		stored := map[string]any{keyTrashRetentionDays: v.TrashRetentionDays, keyMaxUploadMiB: v.MaxUploadMiB,
			keyMailEnabled: v.MailEnabled, keyMailExcerpts: v.MailExcerpts,
			keyReactionsEnabled: v.ReactionsEnabled, keyGroupMentions: v.GroupMentions}
		for key, value := range stored {
			if _, err := tx.Exec(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)
				ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, key, value); err != nil {
				return err
			}
		}
		if _, err := audit.Record(ctx, tx, c.Event("settings.updated", "settings", "", "", stored)); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "settings.updated", v)
		return err
	})
}
