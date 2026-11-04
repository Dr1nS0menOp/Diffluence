// SPDX-License-Identifier: Apache-2.0

package settings

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

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
	keyCustomFont = "custom_font"
	// MaxFontBytes is plenty for one web font, and keeps page loads fast.
	MaxFontBytes  int64 = 2 << 20
	maxFontName         = 100
	fontSignature       = 4
)

// Font is the typeface an administrator imported. By default Diffluence uses the
// system font, so nothing needs to be shipped or licensed; an organisation can
// bring its own house font here.
type Font struct {
	Name      string `json:"name"`
	Format    string `json:"format"`
	Version   string `json:"version"`
	ObjectKey string `json:"object_key"`
}

// fontFormat recognises a font by its first bytes, never by its file name.
func fontFormat(head []byte) (string, bool) {
	switch string(head) {
	case "wOF2":
		return "woff2", true
	case "wOFF":
		return "woff", true
	}
	return "", false
}

func cleanFontName(filename string) string {
	name := path.Base(strings.ReplaceAll(filename, `\`, "/"))
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name))
	if len(name) > maxFontName {
		name = strings.ToValidUTF8(name[:maxFontName], "")
	}
	return name
}

// ImportFont stores a web font and makes it the font of this installation. The
// route requires administrator.
func (s *Service) ImportFont(ctx context.Context, c identity.Caller, filename string, body io.Reader) (Font, error) {
	reader := bufio.NewReaderSize(body, fontSignature)
	head, err := reader.Peek(fontSignature)
	if err != nil && !errors.Is(err, io.EOF) {
		return Font{}, fmt.Errorf("settings: read font: %v", err)
	}
	format, ok := fontFormat(head)
	if !ok {
		return Font{}, apperr.Invalid("Upload a font in WOFF2 or WOFF format.")
	}
	version := ids.New().String()
	font := Font{Name: cleanFontName(filename), Format: format, Version: version, ObjectKey: "appearance/font-" + version}
	if _, err := s.store.Put(ctx, font.ObjectKey, reader, MaxFontBytes); err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			return Font{}, apperr.TooLarge("A font can be at most 2 MiB.")
		}
		return Font{}, fmt.Errorf("settings: store font: %v", err)
	}
	err = s.changeFont(ctx, c, font.Name, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, keyCustomFont, font)
		return err
	})
	if err != nil {
		return Font{}, errors.Join(err, s.store.Delete(context.WithoutCancel(ctx), font.ObjectKey))
	}
	return font, nil
}

// RemoveFont goes back to the system font. The route requires administrator.
func (s *Service) RemoveFont(ctx context.Context, c identity.Caller) error {
	return s.changeFont(ctx, c, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "DELETE FROM settings WHERE key = $1", keyCustomFont)
		return err
	})
}

// changeFont applies one change to the stored font. The file of the previous
// font is queued for deletion and the change recorded, in the same transaction.
func (s *Service) changeFont(ctx context.Context, c identity.Caller, fontName string, apply func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		previous, found, err := loadFont(ctx, tx)
		if err != nil {
			return err
		}
		if found {
			if err := storage.QueueDeletion(ctx, tx, previous.ObjectKey); err != nil {
				return err
			}
		}
		if err := apply(tx); err != nil {
			return fmt.Errorf("settings: change font: %v", err)
		}
		details := map[string]any{"font": fontName}
		if _, err := audit.Record(ctx, tx, c.Event("settings.font_changed", "settings", keyCustomFont, "", details)); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "settings.font_changed", details)
		return err
	})
}

func loadFont(ctx context.Context, q db.Querier) (Font, bool, error) {
	var raw []byte
	err := q.QueryRow(ctx, "SELECT value FROM settings WHERE key = $1", keyCustomFont).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Font{}, false, nil
	}
	if err != nil {
		return Font{}, false, fmt.Errorf("settings: read font: %v", err)
	}
	var font Font
	if err := json.Unmarshal(raw, &font); err != nil {
		return Font{}, false, fmt.Errorf("settings: decode font: %v", err)
	}
	return font, true, nil
}

// CurrentFont returns the imported font, if there is one.
func (s *Service) CurrentFont(ctx context.Context) (Font, bool, error) {
	return loadFont(ctx, s.pool)
}

// OpenFont returns the imported font's file.
func (s *Service) OpenFont(ctx context.Context) (Font, io.ReadSeekCloser, error) {
	font, found, err := loadFont(ctx, s.pool)
	if err != nil {
		return font, nil, err
	}
	if !found {
		return font, nil, apperr.NotFound()
	}
	file, err := s.store.Open(ctx, font.ObjectKey)
	if errors.Is(err, storage.ErrNotFound) {
		return font, nil, apperr.NotFound()
	}
	if err != nil {
		return font, nil, fmt.Errorf("settings: open font: %v", err)
	}
	return font, file, nil
}
