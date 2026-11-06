// SPDX-License-Identifier: Apache-2.0

// Package spaces owns spaces (spec 001): the unit of ownership and permissions.
package spaces

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/storage"
	"github.com/diffluence/diffluence/internal/trash"
)

// Statuses.
const (
	StatusActive   = "active"
	StatusArchived = "archived"
	StatusTrashed  = "trashed"
)

// Space is a space.
type Space struct {
	ID          uuid.UUID       `json:"id"`
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Icon        string          `json:"icon"`
	Status      string          `json:"status"`
	HomePageID  *uuid.UUID      `json:"home_page_id"`
	CreatedBy   uuid.UUID       `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Favourite   bool            `json:"favourite"`
	Kind        string          `json:"kind"`
	OwnerID     *uuid.UUID      `json:"owner_id"`
	Can         map[string]bool `json:"can,omitempty"`
	// The settings of a space.
	HasIcon          bool            `json:"has_icon"`
	TitleAlign       string          `json:"title_align"`
	ReviewMonths     int             `json:"review_months"`
	ApprovalRequired bool            `json:"approval_required"`
	Features         map[string]bool `json:"features,omitempty"`
}

// Input is what a user fills in.
type Input struct {
	Key         string
	Name        string
	Description string
	Icon        string
	// Document is the starter text of the home page, from the browser, so the words
	// are in the person's language (spec 013, D16). It passes the usual gate.
	Document json.RawMessage
}

// Service manages spaces.
type Service struct {
	pool       *pgxpool.Pool
	perms      *permissions.Service
	pages      *pages.Service
	settings   *settings.Service
	features   *features.Service
	store      storage.Store
	instanceID uuid.UUID
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, pg *pages.Service, st *settings.Service, ft *features.Service, store storage.Store, instanceID uuid.UUID) *Service {
	return &Service{pool: pool, perms: perms, pages: pg, settings: st, features: ft, store: store, instanceID: instanceID}
}

// maxIconRunes fits the longest animal, and a few characters of a person's own.
const maxIconRunes = 12

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$`)

func (in *Input) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.Icon = strings.TrimSpace(in.Icon)
	switch {
	case in.Name == "" || utf8.RuneCountInString(in.Name) > 200 || strings.ContainsAny(in.Name, "\r\n\x00"):
		return apperr.Invalid("A space needs a name of at most 200 characters.")
	case utf8.RuneCountInString(in.Description) > 2000 || strings.Contains(in.Description, "\x00"):
		return apperr.Invalid("A description can have at most 2000 characters.")
	case utf8.RuneCountInString(in.Icon) > maxIconRunes || strings.ContainsAny(in.Icon, "\r\n\x00<>"):
		return apperr.Invalid("An icon is the name of an animal, one emoji or a few characters.")
	}
	return nil
}

const columns = `id, key, name, description, icon, status, home_page_id, created_by, created_at, updated_at,
	icon_object <> '', title_align, review_months, approval_required, kind, owner_id`

func scan(r pgx.Row) (Space, error) {
	var s Space
	err := r.Scan(&s.ID, &s.Key, &s.Name, &s.Description, &s.Icon, &s.Status, &s.HomePageID, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
		&s.HasIcon, &s.TitleAlign, &s.ReviewMonths, &s.ApprovalRequired, &s.Kind, &s.OwnerID)
	return s, err
}

func load(ctx context.Context, q db.Querier, id uuid.UUID) (Space, error) {
	s, err := scan(q.QueryRow(ctx, "SELECT "+columns+" FROM spaces WHERE id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return s, apperr.NotFound()
	}
	return s, err
}

func record(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, s Space, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	details["key"] = s.Key
	if _, err := audit.Record(ctx, tx, c.Event(action, "space", s.ID.String(), s.ID.String(), details)); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"space_id": s.ID})
	return err
}

// Create makes a space with a published home page. The route requires the
// global "create spaces" permission. The creator becomes space admin.
func (s *Service) Create(ctx context.Context, c identity.Caller, in Input) (Space, error) {
	if !keyPattern.MatchString(in.Key) {
		return Space{}, apperr.Invalid("A key has 2 to 32 letters, digits, dashes or underscores.")
	}
	if err := in.validate(); err != nil {
		return Space{}, err
	}
	home := document.Empty()
	if len(in.Document) > 0 {
		var err error
		if home, err = document.Normalize(in.Document); err != nil {
			return Space{}, err
		}
	}
	var out Space
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		id := ids.New()
		taken, err := keyTaken(ctx, tx, in.Key, id)
		if err != nil {
			return err
		}
		if taken {
			return apperr.Conflict("key_taken", "This key is already in use.")
		}
		out, err = scan(tx.QueryRow(ctx, `INSERT INTO spaces (id, origin_instance_id, key, name, description, icon, status, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, 'active', $7) RETURNING `+columns,
			id, s.instanceID, in.Key, in.Name, in.Description, in.Icon, c.UserID))
		if db.IsUniqueViolation(err) {
			return apperr.Conflict("key_taken", "This key is already in use.")
		}
		if err != nil {
			return err
		}
		if err := permissions.ApplyTemplateTx(ctx, tx, id, permissions.Subject{Type: permissions.SubjectUser, ID: c.UserID}, permissions.TemplateSpaceAdmin); err != nil {
			return err
		}
		if err := s.features.Start(ctx, tx, id); err != nil {
			return err
		}
		if err := record(ctx, tx, c, "space.created", out, map[string]any{"name": out.Name}); err != nil {
			return err
		}
		page, err := s.pages.CreatePublished(ctx, tx, c, id, nil, in.Name, home, c.UserID)
		if err != nil {
			return err
		}
		out, err = scan(tx.QueryRow(ctx, "UPDATE spaces SET home_page_id = $2 WHERE id = $1 RETURNING "+columns, id, page.ID))
		return err
	})
	return out, err
}

// List returns the active spaces the user may view. Archived spaces only show in
// the directory, under their own filter (SPC-069).
func (s *Service) List(ctx context.Context, c identity.Caller) ([]Space, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM spaces
		WHERE status = 'active' AND id IN (SELECT space_id FROM perm_spaces($1, 'space.view'))
		ORDER BY lower(name), id`, c.UserID)
	return collect(rows, err)
}

func collect(rows pgx.Rows, err error) ([]Space, error) {
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Space, error) { return scan(r) })
	if out == nil {
		out = []Space{}
	}
	return out, err
}

// ListAll returns every space that is not in the trash. The route requires
// administrator. It shows metadata only: administrators get no content access.
func (s *Service) ListAll(ctx context.Context) ([]Space, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+columns+" FROM spaces WHERE status <> 'trashed' AND NOT system ORDER BY lower(name), id")
	return collect(rows, err)
}

// Get returns a space by id.
func (s *Service) Get(ctx context.Context, c identity.Caller, id uuid.UUID) (Space, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, id, permissions.SpaceView); err != nil {
		return Space{}, err
	}
	sp, err := load(ctx, s.pool, id)
	if err != nil {
		return sp, err
	}
	err = s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM space_favourites WHERE user_id = $1 AND space_id = $2)", c.UserID, id).Scan(&sp.Favourite)
	if err != nil {
		return sp, err
	}
	if sp.Features, err = s.features.ForSpace(ctx, s.pool, id); err != nil {
		return sp, err
	}
	sp.Can = map[string]bool{}
	for _, action := range []string{permissions.SpaceAdminister, permissions.PageCreate, permissions.SpaceExport, permissions.SpaceManageAccess} {
		d, err := s.perms.CheckSpace(ctx, s.pool, c.UserID, id, action)
		if err != nil {
			return sp, err
		}
		sp.Can[action] = d.Allowed
	}
	return sp, nil
}

// GetByKey resolves a key, and then behaves like Get.
func (s *Service) GetByKey(ctx context.Context, c identity.Caller, key string) (Space, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT id FROM spaces WHERE lower(key) = lower($1)
		UNION ALL SELECT space_id FROM space_key_aliases WHERE alias = lower($1) LIMIT 1`, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Space{}, apperr.NotFound()
	}
	if err != nil {
		return Space{}, err
	}
	return s.Get(ctx, c, id)
}

// Update changes the details of a space. The key stays: it identifies the space.
func (s *Service) Update(ctx context.Context, c identity.Caller, id uuid.UUID, in Input) (Space, error) {
	if err := in.validate(); err != nil {
		return Space{}, err
	}
	var out Space
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, id, permissions.SpaceAdminister); err != nil {
			return err
		}
		var err error
		out, err = scan(tx.QueryRow(ctx, `UPDATE spaces SET name = $2, description = $3, icon = $4, updated_at = now()
			WHERE id = $1 RETURNING `+columns, id, in.Name, in.Description, in.Icon))
		if err != nil {
			return err
		}
		return record(ctx, tx, c, "space.updated", out, map[string]any{"name": in.Name})
	})
	return out, err
}

func (s *Service) setStatus(ctx context.Context, c identity.Caller, id uuid.UUID, from, to, action string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, id, permissions.SpaceAdminister); err != nil {
			return err
		}
		sp, err := scan(tx.QueryRow(ctx, "UPDATE spaces SET status = $2, updated_at = now() WHERE id = $1 AND status = $3 RETURNING "+columns, id, to, from))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("wrong_status", "The space is not "+from+".")
		}
		if err != nil {
			return err
		}
		return record(ctx, tx, c, action, sp, nil)
	})
}

// Archive makes a space read-only.
func (s *Service) Archive(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return s.setStatus(ctx, c, id, StatusActive, StatusArchived, "space.archived")
}

// Unarchive makes a space writable again.
func (s *Service) Unarchive(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return s.setStatus(ctx, c, id, StatusArchived, StatusActive, "space.unarchived")
}

// Delete sends a space to the trash.
func (s *Service) Delete(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	retention, err := s.settings.TrashRetention(ctx)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, id, permissions.SpaceAdminister); err != nil {
			return err
		}
		sp, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := trash.Add(ctx, tx, trash.Item{SpaceID: id, ObjectType: trash.TypeSpace, ObjectID: id,
			Title: sp.Name, DeletedBy: c.UserID}, retention); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE spaces SET status = 'trashed', updated_at = now() WHERE id = $1", id); err != nil {
			return err
		}
		return record(ctx, tx, c, "space.trashed", sp, nil)
	})
}

// Restore implements trash.Handler.
func (s *Service) Restore(ctx context.Context, tx pgx.Tx, c identity.Caller, it trash.Item) error {
	sp, err := scan(tx.QueryRow(ctx, "UPDATE spaces SET status = 'active', updated_at = now() WHERE id = $1 RETURNING "+columns, it.ObjectID))
	if err != nil {
		return err
	}
	return record(ctx, tx, c, "space.restored", sp, nil)
}

// Purge implements trash.Handler.
func (s *Service) Purge(ctx context.Context, tx pgx.Tx, c identity.Caller, it trash.Item) error {
	sp, err := load(ctx, tx, it.ObjectID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO storage_deletions (object_key)
		SELECT v.object_key FROM attachment_versions v JOIN attachments a ON a.id = v.attachment_id
		WHERE a.space_id = $1 ON CONFLICT DO NOTHING`, it.ObjectID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM spaces WHERE id = $1", it.ObjectID); err != nil {
		return err
	}
	return record(ctx, tx, c, "space.purged", sp, map[string]any{"name": sp.Name})
}
