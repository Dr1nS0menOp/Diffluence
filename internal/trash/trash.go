// SPDX-License-Identifier: Apache-2.0

// Package trash keeps deleted items for one universal retention period
// (ADR-014). The module that owns an object does the actual restore and purge
// through a Handler; this package only keeps the list and decides when.
package trash

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Object types in the trash.
const (
	TypePage       = "page"
	TypeAttachment = "attachment"
	TypeSpace      = "space"
)

// Item is one deleted object.
type Item struct {
	ID         uuid.UUID `json:"id"`
	SpaceID    uuid.UUID `json:"space_id"`
	ObjectType string    `json:"object_type"`
	ObjectID   uuid.UUID `json:"object_id"`
	Title      string    `json:"title"`
	DeletedBy  uuid.UUID `json:"deleted_by"`
	DeletedAt  time.Time `json:"deleted_at"`
	PurgeAfter time.Time `json:"purge_after"`
}

// Add records a deleted object inside tx and returns the item id.
func Add(ctx context.Context, tx pgx.Tx, it Item, retention time.Duration) (uuid.UUID, error) {
	id := ids.New()
	_, err := tx.Exec(ctx, `INSERT INTO trash_items (id, space_id, object_type, object_id, title, deleted_by, purge_after)
		VALUES ($1, $2, $3, $4, $5, $6, now() + make_interval(secs => $7))`,
		id, it.SpaceID, it.ObjectType, it.ObjectID, it.Title, it.DeletedBy, retention.Seconds())
	return id, err
}

// RemoveFor drops the items of objects that are gone, inside tx.
func RemoveFor(ctx context.Context, tx pgx.Tx, objectIDs []uuid.UUID) error {
	_, err := tx.Exec(ctx, "DELETE FROM trash_items WHERE object_id = ANY($1)", objectIDs)
	return err
}

const columns = "id, space_id, object_type, object_id, title, deleted_by, deleted_at, purge_after"

func scan(r pgx.Row) (Item, error) {
	var it Item
	err := r.Scan(&it.ID, &it.SpaceID, &it.ObjectType, &it.ObjectID, &it.Title, &it.DeletedBy, &it.DeletedAt, &it.PurgeAfter)
	return it, err
}

func collect(rows pgx.Rows, err error) ([]Item, error) {
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Item, error) { return scan(r) })
	if out == nil {
		out = []Item{}
	}
	return out, err
}

// Handler restores and purges objects of one type, inside tx.
type Handler interface {
	Restore(ctx context.Context, tx pgx.Tx, c identity.Caller, it Item) error
	Purge(ctx context.Context, tx pgx.Tx, c identity.Caller, it Item) error
}

// Service restores and purges trash items.
type Service struct {
	pool     *pgxpool.Pool
	perms    *permissions.Service
	identity *identity.Service
	handlers map[string]Handler
	log      *slog.Logger
}

// NewService creates the service. Modules add themselves with Register.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, id *identity.Service, log *slog.Logger) *Service {
	return &Service{pool: pool, perms: perms, identity: id, handlers: map[string]Handler{}, log: log}
}

// Register sets the handler for an object type.
func (s *Service) Register(objectType string, h Handler) { s.handlers[objectType] = h }

// ListSpace shows a space's trash to its administrators.
func (s *Service) ListSpace(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]Item, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT "+columns+` FROM trash_items
		WHERE space_id = $1 AND object_type <> 'space' ORDER BY deleted_at DESC LIMIT 500`, spaceID)
	return collect(rows, err)
}

// ListSpaces shows deleted spaces. The route requires administrator.
func (s *Service) ListSpaces(ctx context.Context) ([]Item, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+columns+` FROM trash_items
		WHERE object_type = 'space' ORDER BY deleted_at DESC LIMIT 500`)
	return collect(rows, err)
}

// authorize: a space's own trash belongs to its administrators. A deleted space
// has no working permissions any more, so only global administrators can act on it.
func (s *Service) authorize(ctx context.Context, q db.Querier, c identity.Caller, it Item) error {
	if it.ObjectType == TypeSpace {
		ok, err := s.identity.HasGlobalPermission(ctx, c.UserID, identity.PermAdministrator)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.NotFound()
		}
		return nil
	}
	return s.perms.RequireSpace(ctx, q, c.UserID, it.SpaceID, permissions.SpaceAdminister)
}

func (s *Service) run(ctx context.Context, c identity.Caller, id uuid.UUID, act func(Handler, pgx.Tx, Item) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		it, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM trash_items WHERE id = $1 FOR UPDATE", id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound()
		}
		if err != nil {
			return err
		}
		if err := s.authorize(ctx, tx, c, it); err != nil {
			return err
		}
		h, ok := s.handlers[it.ObjectType]
		if !ok {
			return fmt.Errorf("trash: no handler for %q", it.ObjectType)
		}
		if err := act(h, tx, it); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "DELETE FROM trash_items WHERE id = $1", id)
		return err
	})
}

// Restore puts an item back.
func (s *Service) Restore(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return s.run(ctx, c, id, func(h Handler, tx pgx.Tx, it Item) error { return h.Restore(ctx, tx, c, it) })
}

// undoWindow is longer than the message that offers Undo, to allow for a slow
// connection. Anything older belongs to the trash and its administrators.
const undoWindow = 60 * time.Second

// UndoDelete lets the person who just deleted a page take it back, also without
// administer (PAG-085). Nobody else can, and it does not work for long.
func (s *Service) UndoDelete(ctx context.Context, c identity.Caller, pageID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		it, err := scan(tx.QueryRow(ctx, "SELECT "+columns+` FROM trash_items
			WHERE object_id = $1 AND object_type = 'page' AND deleted_by = $2
			  AND deleted_at > now() - make_interval(secs => $3) FOR UPDATE`, pageID, c.UserID, undoWindow.Seconds()))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound()
		}
		if err != nil {
			return err
		}
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, it.SpaceID, permissions.SpaceView); err != nil {
			return err
		}
		h, ok := s.handlers[it.ObjectType]
		if !ok {
			return fmt.Errorf("trash: no handler for %q", it.ObjectType)
		}
		if err := h.Restore(ctx, tx, c, it); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "DELETE FROM trash_items WHERE id = $1", it.ID)
		return err
	})
}

// Purge removes an item for good, before its retention period ends.
func (s *Service) Purge(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return s.run(ctx, c, id, func(h Handler, tx pgx.Tx, it Item) error { return h.Purge(ctx, tx, c, it) })
}

// PurgeExpired removes items past their retention period. Each item is its own
// transaction, so one failing item cannot block the others.
func (s *Service) PurgeExpired(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+columns+" FROM trash_items WHERE purge_after <= now() ORDER BY purge_after LIMIT 500")
	items, err := collect(rows, err)
	if err != nil {
		return 0, err
	}
	c := identity.SystemCaller("trash-purge")
	n := 0
	for _, it := range items {
		err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			// The item may already be gone when its parent was purged first.
			tag, err := tx.Exec(ctx, "DELETE FROM trash_items WHERE id = $1", it.ID)
			if err != nil || tag.RowsAffected() == 0 {
				return err
			}
			h, ok := s.handlers[it.ObjectType]
			if !ok {
				return fmt.Errorf("trash: no handler for %q", it.ObjectType)
			}
			return h.Purge(ctx, tx, c, it)
		})
		if err != nil {
			s.log.Error("purging a trash item failed", "error", err, "item", it.ID)
			continue
		}
		n++
	}
	return n, nil
}
