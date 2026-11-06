// SPDX-License-Identifier: Apache-2.0

package settings

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/storage"
)

const (
	keyLogo       = "logo"
	logoSniffSize = 512
	maxLogoBytes  = 1 << 20
)

// logoTypes are the only images a logo can be. SVG is not one of them: it can carry
// scripts, and this file is served without a login.
var logoTypes = []string{"image/png", "image/jpeg", "image/webp"}

// Logo is the logo of the instance.
type Logo struct {
	ObjectKey   string `json:"object_key"`
	ContentType string `json:"content_type"`
	Version     string `json:"version"`
}

func loadLogo(ctx context.Context, q db.Querier) (Logo, bool, error) {
	var raw []byte
	err := q.QueryRow(ctx, "SELECT value FROM settings WHERE key = $1", keyLogo).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Logo{}, false, nil
	}
	if err != nil {
		return Logo{}, false, fmt.Errorf("settings: read logo: %v", err)
	}
	var logo Logo
	if err := json.Unmarshal(raw, &logo); err != nil {
		return Logo{}, false, fmt.Errorf("settings: decode logo: %v", err)
	}
	return logo, true, nil
}

// CurrentLogo returns the logo, if there is one.
func (s *Service) CurrentLogo(ctx context.Context) (Logo, bool, error) { return loadLogo(ctx, s.pool) }

// OpenLogo returns the file of the logo.
func (s *Service) OpenLogo(ctx context.Context) (Logo, io.ReadSeekCloser, error) {
	logo, found, err := loadLogo(ctx, s.pool)
	if err != nil {
		return logo, nil, err
	}
	if !found {
		return logo, nil, apperr.NotFound()
	}
	f, err := s.store.Open(ctx, logo.ObjectKey)
	if errors.Is(err, storage.ErrNotFound) {
		return logo, nil, apperr.NotFound()
	}
	return logo, f, err
}

// SetLogo stores a logo. What the file really is decides, not what it claims. The
// route requires administrator.
func (s *Service) SetLogo(ctx context.Context, c identity.Caller, body io.Reader) error {
	reader := bufio.NewReaderSize(body, logoSniffSize)
	head, err := reader.Peek(logoSniffSize)
	if err != nil && !errors.Is(err, io.EOF) {
		return apperr.TooLarge("A logo can be at most 1 MiB.")
	}
	contentType := http.DetectContentType(head)
	if !slices.Contains(logoTypes, contentType) {
		return apperr.Invalid("A logo is a PNG, JPEG or WebP image.")
	}
	version := ids.New().String()
	logo := Logo{ObjectKey: "appearance/logo-" + version, ContentType: contentType, Version: version}
	if _, err := s.store.Put(ctx, logo.ObjectKey, reader, maxLogoBytes); err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			return apperr.TooLarge("A logo can be at most 1 MiB.")
		}
		return fmt.Errorf("settings: store logo: %v", err)
	}
	return s.changeLogo(ctx, c, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, keyLogo, logo)
		return err
	})
}

// RemoveLogo goes back to our own mark.
func (s *Service) RemoveLogo(ctx context.Context, c identity.Caller) error {
	return s.changeLogo(ctx, c, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "DELETE FROM settings WHERE key = $1", keyLogo)
		return err
	})
}

func (s *Service) changeLogo(ctx context.Context, c identity.Caller, apply func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		previous, found, err := loadLogo(ctx, tx)
		if err != nil {
			return err
		}
		if found {
			if err := storage.QueueDeletion(ctx, tx, previous.ObjectKey); err != nil {
				return err
			}
		}
		if err := apply(tx); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("settings.logo_changed", "settings", keyLogo, "", nil)); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "settings.logo_changed", map[string]any{})
		return err
	})
}
