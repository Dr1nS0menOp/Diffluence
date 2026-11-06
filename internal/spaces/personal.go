// SPDX-License-Identifier: Apache-2.0

package spaces

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
)

const (
	// A team key cannot hold ~, so a personal key can never clash. It holds no name,
	// so it survives a rename and an anonymisation (spec 013, D2).
	personalKeyPrefix = "~"
	personalKeyLength = 10
	maxCategoryName   = 100
)

// PersonalKey is the key of a person's personal space.
func PersonalKey(userID uuid.UUID) string {
	return personalKeyPrefix + strings.ReplaceAll(userID.String(), "-", "")[:personalKeyLength]
}

// CreatePersonal makes the caller's personal space. It is an ordinary space with
// an ordinary grant: no second access rule exists (spec 013, D1). The route
// requires "create personal space".
func (s *Service) CreatePersonal(ctx context.Context, c identity.Caller, name string) (Space, error) {
	var out Space
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		id := ids.New()
		var err error
		out, err = scan(tx.QueryRow(ctx, `INSERT INTO spaces (id, origin_instance_id, key, name, status, created_by, kind, owner_id)
			VALUES ($1, $2, $3, $4, 'active', $5, 'personal', $5) RETURNING `+columns,
			id, s.instanceID, PersonalKey(c.UserID), name, c.UserID))
		if db.IsUniqueViolation(err) {
			return apperr.Conflict("personal_space_exists", "You already have a personal space.")
		}
		if err != nil {
			return err
		}
		if err := permissions.ApplyTemplateTx(ctx, tx, id, permissions.Subject{Type: permissions.SubjectUser, ID: c.UserID}, permissions.TemplateSpaceAdmin); err != nil {
			return err
		}
		if err := s.features.Start(ctx, tx, out.ID); err != nil {
			return err
		}
		if err := record(ctx, tx, c, "space.created", out, map[string]any{"name": out.Name, "kind": "personal"}); err != nil {
			return err
		}
		home, err := s.pages.CreatePublished(ctx, tx, c, id, nil, name, document.Empty(), c.UserID)
		if err != nil {
			return err
		}
		out, err = scan(tx.QueryRow(ctx, "UPDATE spaces SET home_page_id = $2 WHERE id = $1 RETURNING "+columns, id, home.ID))
		return err
	})
	return out, err
}

// PersonalOf returns a person's personal space, if they have one.
func (s *Service) PersonalOf(ctx context.Context, userID uuid.UUID) (*Space, error) {
	sp, err := scan(s.pool.QueryRow(ctx, "SELECT "+columns+" FROM spaces WHERE kind = 'personal' AND owner_id = $1 AND status <> 'trashed'", userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sp, nil
}

// FollowRename gives a personal space the new name of its owner.
func (s *Service) FollowRename(ctx context.Context, userID uuid.UUID, name string) error {
	_, err := s.pool.Exec(ctx, "UPDATE spaces SET name = $2, updated_at = now() WHERE kind = 'personal' AND owner_id = $1", userID, name)
	return err
}

// Handover gives a personal space to someone else, when its owner has left. The
// grant goes to that person, and not to the administrator who does this. The route
// requires administrator.
func (s *Service) Handover(ctx context.Context, c identity.Caller, id, to uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		sp, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if sp.Kind != "personal" {
			return apperr.Conflict("not_personal", "Only a personal space is handed over.")
		}
		var active bool
		err = tx.QueryRow(ctx, "SELECT status = 'active' FROM users WHERE id = $1", to).Scan(&active)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !active) {
			return apperr.Invalid("Choose an active person.")
		}
		if err != nil {
			return err
		}
		if err := permissions.ApplyTemplateTx(ctx, tx, id, permissions.Subject{Type: permissions.SubjectUser, ID: to}, permissions.TemplateSpaceAdmin); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE spaces SET owner_id = $2, updated_at = now() WHERE id = $1", id, to); err != nil {
			return err
		}
		return record(ctx, tx, c, "space.handed_over", sp, map[string]any{"to": to})
	})
}

// Unmanaged is a space nobody active can look after.
type Unmanaged struct {
	ID   uuid.UUID `json:"id"`
	Key  string    `json:"key"`
	Name string    `json:"name"`
	Kind string    `json:"kind"`
}

// WithoutActiveAdministrator lists spaces with no active administrator, directly
// or through a group. The route requires administrator. It shows no content.
func (s *Service) WithoutActiveAdministrator(ctx context.Context) ([]Unmanaged, error) {
	rows, err := s.pool.Query(ctx, `SELECT s.id, s.key, s.name, s.kind FROM spaces s
		WHERE s.status = 'active' AND NOT EXISTS (
			SELECT 1 FROM space_grants g
			LEFT JOIN users u ON g.subject_type = 'user' AND u.id = g.subject_id
			LEFT JOIN group_members m ON g.subject_type = 'group' AND m.group_id = g.subject_id
			LEFT JOIN users gu ON gu.id = m.user_id
			WHERE g.space_id = s.id AND g.permission = 'space.administer' AND (u.status = 'active' OR gu.status = 'active'))
		ORDER BY lower(s.name) LIMIT 500`)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Unmanaged, error) {
		var v Unmanaged
		err := r.Scan(&v.ID, &v.Key, &v.Name, &v.Kind)
		return v, err
	})
	if out == nil {
		out = []Unmanaged{}
	}
	return out, err
}

// Category is a label for spaces, from one list kept by administrators (D15).
type Category struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Categories lists the categories.
func (s *Service) Categories(ctx context.Context) ([]Category, error) {
	rows, err := s.pool.Query(ctx, "SELECT id, name FROM space_categories ORDER BY lower(name)")
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Category, error) {
		var v Category
		err := r.Scan(&v.ID, &v.Name)
		return v, err
	})
	if out == nil {
		out = []Category{}
	}
	return out, err
}

// AddCategory adds to the list. The route requires administrator.
func (s *Service) AddCategory(ctx context.Context, c identity.Caller, name string) (Category, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxCategoryName || strings.ContainsAny(name, "\r\n\x00") {
		return Category{}, apperr.Invalid("A category has a name of at most 100 characters.")
	}
	out := Category{ID: ids.New(), Name: name}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO space_categories (id, name) VALUES ($1, $2)", out.ID, out.Name)
		if db.IsUniqueViolation(err) {
			return apperr.Conflict("name_taken", "This category already exists.")
		}
		if err != nil {
			return err
		}
		return recordSetting(ctx, tx, c, "category.added", out.ID, name)
	})
	return out, err
}

// DeleteCategory removes a category from the list and from every space.
func (s *Service) DeleteCategory(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM space_categories WHERE id = $1", id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound()
		}
		return recordSetting(ctx, tx, c, "category.deleted", id, "")
	})
}

// CategoriesOf lists the ids of the categories of a space.
func (s *Service) CategoriesOf(ctx context.Context, c identity.Caller, id uuid.UUID) ([]uuid.UUID, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, id, permissions.SpaceView); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT category_id FROM space_category_links WHERE space_id = $1", id)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if out == nil {
		out = []uuid.UUID{}
	}
	return out, err
}

// SetCategories chooses the categories of a space.
func (s *Service) SetCategories(ctx context.Context, c identity.Caller, id uuid.UUID, categories []uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, id, permissions.SpaceAdminister); err != nil {
			return err
		}
		sp, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM space_category_links WHERE space_id = $1", id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO space_category_links (space_id, category_id)
			SELECT $1, c.id FROM space_categories c WHERE c.id = ANY($2)`, id, categories); err != nil {
			return err
		}
		return record(ctx, tx, c, "space.categories_changed", sp, map[string]any{"count": len(categories)})
	})
}

func recordSetting(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, id uuid.UUID, name string) error {
	if _, err := audit.Record(ctx, tx, c.Event(action, "category", id.String(), "", map[string]any{"name": name})); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"category_id": id})
	return err
}
