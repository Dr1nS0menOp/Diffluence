// SPDX-License-Identifier: Apache-2.0

package spaces

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
)

const maxIconBytes = 1 << 20

// iconTypes are the only files a space can have as its picture. SVG is not here,
// for the same reason as with attachments: it can carry scripts.
var iconTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

func (s *Service) requireAdmin(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID) (Space, error) {
	if err := s.perms.RequireSpace(ctx, tx, c.UserID, id, permissions.SpaceAdminister); err != nil {
		return Space{}, err
	}
	return load(ctx, tx, id)
}

// systemSpaceKey is the space that holds templates of all spaces. It has no grants,
// so nobody can open it, and it is left out of every list.
const systemSpaceKey = "__templates"

// SystemSpace finds that space, and makes it the first time.
func (s *Service) SystemSpace(ctx context.Context, tx pgx.Tx) (uuid.UUID, error) {
	id := ids.New()
	if _, err := tx.Exec(ctx, `INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by, system)
		VALUES ($1, $2, $3, 'Templates', 'active', $4, true) ON CONFLICT DO NOTHING`, id, s.instanceID, systemSpaceKey, uuid.Nil); err != nil {
		return uuid.Nil, err
	}
	err := tx.QueryRow(ctx, "SELECT id FROM spaces WHERE system AND key = $1", systemSpaceKey).Scan(&id)
	return id, err
}

// keyTaken reports whether a key belongs to a space or lives on as an old key.
func keyTaken(ctx context.Context, q db.Querier, key string, except uuid.UUID) (bool, error) {
	var taken bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM spaces WHERE lower(key) = lower($1) AND id <> $2)
		OR EXISTS (SELECT 1 FROM space_key_aliases WHERE alias = lower($1) AND space_id <> $2)`, key, except).Scan(&taken)
	return taken, err
}

// ChangeKey gives a space a new key. The old key keeps working and stays taken.
func (s *Service) ChangeKey(ctx context.Context, c identity.Caller, id uuid.UUID, key string) (Space, error) {
	if !keyPattern.MatchString(key) {
		return Space{}, apperr.Invalid("A key has 2 to 32 letters, digits, dashes or underscores.")
	}
	var out Space
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		sp, err := s.requireAdmin(ctx, tx, c, id)
		if err != nil {
			return err
		}
		taken, err := keyTaken(ctx, tx, key, id)
		if err != nil {
			return err
		}
		if taken {
			return apperr.Conflict("key_taken", "This key is already in use.")
		}
		if strings.EqualFold(sp.Key, key) {
			out = sp
		} else if _, err := tx.Exec(ctx, "INSERT INTO space_key_aliases (alias, space_id) VALUES (lower($1), $2) ON CONFLICT DO NOTHING", sp.Key, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM space_key_aliases WHERE alias = lower($1) AND space_id = $2", key, id); err != nil {
			return err
		}
		out, err = scan(tx.QueryRow(ctx, "UPDATE spaces SET key = $2, updated_at = now() WHERE id = $1 RETURNING "+columns, id, key))
		if db.IsUniqueViolation(err) {
			return apperr.Conflict("key_taken", "This key is already in use.")
		}
		if err != nil {
			return err
		}
		return record(ctx, tx, c, "space.key_changed", out, map[string]any{"from": sp.Key})
	})
	return out, err
}

// ReleaseKey lets go of an old key, so someone else can use it.
func (s *Service) ReleaseKey(ctx context.Context, c identity.Caller, id uuid.UUID, alias string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		sp, err := s.requireAdmin(ctx, tx, c, id)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM space_key_aliases WHERE alias = lower($1) AND space_id = $2", alias, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound()
		}
		return record(ctx, tx, c, "space.key_released", sp, map[string]any{"alias": alias})
	})
}

// OldKeys lists the keys a space used to have.
func (s *Service) OldKeys(ctx context.Context, c identity.Caller, id uuid.UUID) ([]string, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, id, permissions.SpaceAdminister); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT alias FROM space_key_aliases WHERE space_id = $1 ORDER BY alias", id)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if out == nil {
		out = []string{}
	}
	return out, err
}

// Person is a name to show, with who it is.
type Person struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Owners lists who to ask about a space. Those need not hold any rights.
func (s *Service) Owners(ctx context.Context, q db.Querier, id uuid.UUID) ([]Person, error) {
	rows, err := q.Query(ctx, `SELECT o.user_id, u.display_name FROM space_owners o JOIN users u ON u.id = o.user_id
		WHERE o.space_id = $1 ORDER BY lower(u.display_name)`, id)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Person])
	if out == nil {
		out = []Person{}
	}
	return out, err
}

// SetOwners replaces who to ask about a space.
func (s *Service) SetOwners(ctx context.Context, c identity.Caller, id uuid.UUID, users []uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		sp, err := s.requireAdmin(ctx, tx, c, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM space_owners WHERE space_id = $1", id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO space_owners (space_id, user_id)
			SELECT $1, u.id FROM users u WHERE u.id = ANY($2)`, id, users); err != nil {
			return err
		}
		return record(ctx, tx, c, "space.owners_changed", sp, map[string]any{"count": len(users)})
	})
}

// SetHome picks the page a space opens on.
func (s *Service) SetHome(ctx context.Context, c identity.Caller, id, page uuid.UUID) (Space, error) {
	var out Space
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := s.requireAdmin(ctx, tx, c, id); err != nil {
			return err
		}
		ok, err := s.pages.CanBeHome(ctx, tx, c, page, id)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.Invalid("The home page must be a published page of this space.")
		}
		out, err = scan(tx.QueryRow(ctx, "UPDATE spaces SET home_page_id = $2, updated_at = now() WHERE id = $1 RETURNING "+columns, id, page))
		if err != nil {
			return err
		}
		return record(ctx, tx, c, "space.home_changed", out, map[string]any{"page_id": page})
	})
	return out, err
}

// SetTitleAlign sets how titles start out on new pages of a space.
func (s *Service) SetTitleAlign(ctx context.Context, c identity.Caller, id uuid.UUID, align string) error {
	if align != "start" && align != "center" {
		return apperr.Invalid("A title is aligned to the start or the centre.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		sp, err := s.requireAdmin(ctx, tx, c, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE spaces SET title_align = $2 WHERE id = $1", id, align); err != nil {
			return err
		}
		return record(ctx, tx, c, "space.title_align_changed", sp, map[string]any{"align": align})
	})
}

// SetFeatures turns features on and off. Nothing is deleted: turning one back on
// brings everything back.
func (s *Service) SetFeatures(ctx context.Context, c identity.Caller, id uuid.UUID, values map[string]bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		sp, err := s.requireAdmin(ctx, tx, c, id)
		if err != nil {
			return err
		}
		if err := s.features.Set(ctx, tx, id, values); err != nil {
			return err
		}
		return record(ctx, tx, c, "space.features_changed", sp, map[string]any{"features": values})
	})
}

func iconKey(id uuid.UUID) string { return "space-icons/" + id.String() }

// SetIcon stores an uploaded picture for a space. The type comes from the bytes,
// never from what the upload claims.
func (s *Service) SetIcon(ctx context.Context, c identity.Caller, id uuid.UUID, body io.Reader) error {
	raw, err := io.ReadAll(io.LimitReader(body, maxIconBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxIconBytes {
		return apperr.TooLarge("A space picture can be at most 1 MiB.")
	}
	if !iconTypes[http.DetectContentType(raw)] {
		return apperr.Invalid("A space picture is a PNG, JPEG, GIF or WebP file.")
	}
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, id, permissions.SpaceAdminister); err != nil {
		return err
	}
	key := iconKey(id)
	if _, err := s.store.Put(ctx, key, bytes.NewReader(raw), maxIconBytes); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		sp, err := s.requireAdmin(ctx, tx, c, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE spaces SET icon_object = $2 WHERE id = $1", id, key); err != nil {
			return err
		}
		return record(ctx, tx, c, "space.icon_changed", sp, nil)
	})
}

// RemoveIcon goes back to the character or the first letter.
func (s *Service) RemoveIcon(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		sp, err := s.requireAdmin(ctx, tx, c, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE spaces SET icon_object = '' WHERE id = $1", id); err != nil {
			return err
		}
		return record(ctx, tx, c, "space.icon_changed", sp, map[string]any{"removed": true})
	})
}

// Icon opens the picture of a space for someone who may view it.
func (s *Service) Icon(ctx context.Context, c identity.Caller, id uuid.UUID) (io.ReadSeekCloser, string, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, id, permissions.SpaceView); err != nil {
		return nil, "", err
	}
	var key string
	err := s.pool.QueryRow(ctx, "SELECT icon_object FROM spaces WHERE id = $1", id).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) || key == "" {
		return nil, "", apperr.NotFound()
	}
	if err != nil {
		return nil, "", err
	}
	f, err := s.store.Open(ctx, key)
	if err != nil {
		return nil, "", err
	}
	head := make([]byte, 512)
	n, err := f.Read(head)
	if err != nil && !errors.Is(err, io.EOF) {
		_ = f.Close()
		return nil, "", err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, "", err
	}
	return f, http.DetectContentType(head[:n]), nil
}

const defaultPresetKey = "default_space_preset"

// StartingPreset is the permission set of a new space: what the person chose, or
// the default an administrator set. Empty keeps a space private to its creator.
func (s *Service) StartingPreset(ctx context.Context, chosen string) (string, error) {
	if chosen == "private" {
		return "", nil
	}
	if chosen != "" {
		return chosen, nil
	}
	var preset string
	err := s.settings.Read(ctx, defaultPresetKey, &preset)
	return preset, err
}

// DefaultPreset reads the default permission set of new spaces.
func (s *Service) DefaultPreset(ctx context.Context) (string, error) {
	return s.StartingPreset(ctx, "")
}

// SetDefaultPreset sets it. The route requires administrator.
func (s *Service) SetDefaultPreset(ctx context.Context, c identity.Caller, preset string) error {
	if preset != "" && preset != permissions.PresetTeamSpace && preset != permissions.PresetReadOnly {
		return apperr.Invalid("That is not a permission set.")
	}
	return s.settings.Write(ctx, c, defaultPresetKey, preset)
}
