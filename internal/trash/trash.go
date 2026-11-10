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
	TypeComment    = "comment"
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
	// DeletedByName is filled in for the trash screen.
	DeletedByName string `json:"deleted_by_name"`
	// Hidden says the item sits on a page the person may not see. Its title is then
	// left out, and the screen calls it a restricted page.
	Hidden bool `json:"hidden"`
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

// ListSpace shows a space's trash to its administrators. Text narrows it by title,
// and sort is title or, by default, the newest first. Administering a space does not
// mean reading all of it, so a hidden item keeps its title from them, also through
// the search and the order.
func (s *Service) ListSpace(ctx context.Context, c identity.Caller, spaceID uuid.UUID, text, sort string) ([]Item, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT t.id, t.space_id, t.object_type, t.object_id,
			CASE WHEN h.page_id IS NULL THEN t.title ELSE '' END, t.deleted_by, t.deleted_at, t.purge_after,
			COALESCE(u.display_name, ''), h.page_id IS NOT NULL
		FROM trash_items t
		LEFT JOIN users u ON u.id = t.deleted_by
		LEFT JOIN attachments a ON t.object_type = 'attachment' AND a.id = t.object_id
		LEFT JOIN comments m ON t.object_type = 'comment' AND m.id = t.object_id
		LEFT JOIN perm_hidden_pages($4) h ON h.page_id = COALESCE(a.page_id, m.page_id, t.object_id)
		WHERE t.space_id = $1 AND t.object_type <> 'space'
		  AND ($2 = '' OR (h.page_id IS NULL AND position(lower($2) IN lower(t.title)) > 0))
		ORDER BY CASE WHEN $3 = 'title' THEN lower(CASE WHEN h.page_id IS NULL THEN t.title ELSE '' END) END,
			t.deleted_at DESC LIMIT 500`, spaceID, text, sort, c.UserID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Item, error) {
		var it Item
		err := r.Scan(&it.ID, &it.SpaceID, &it.ObjectType, &it.ObjectID, &it.Title, &it.DeletedBy, &it.DeletedAt, &it.PurgeAfter, &it.DeletedByName, &it.Hidden)
		return it, err
	})
	if out == nil {
		out = []Item{}
	}
	return out, err
}

// BulkFailure says why one item did not work.
type BulkFailure struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

const maxBulkItems = 200

// Bulk restores or purges several items, each in its own transaction. An item
// that fails does not stop the rest, such as a page whose parent is in the trash.
func (s *Service) Bulk(ctx context.Context, c identity.Caller, spaceID uuid.UUID, action string, itemIDs []uuid.UUID) (int, []BulkFailure, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
		return 0, nil, err
	}
	if len(itemIDs) == 0 || len(itemIDs) > maxBulkItems {
		return 0, nil, apperr.Invalid("Choose between 1 and 200 items.")
	}
	act := s.Restore
	switch action {
	case "restore":
	case "purge":
		act = s.Purge
	default:
		return 0, nil, apperr.Invalid("The action is restore or purge.")
	}
	done := 0
	failed := []BulkFailure{}
	for _, id := range itemIDs {
		if err := s.inSpace(ctx, id, spaceID); err != nil {
			return done, failed, err
		}
		if err := act(ctx, c, id); err != nil {
			reason := "This item could not be changed."
			if e, ok := apperr.As(err); ok {
				reason = e.Message
			}
			failed = append(failed, BulkFailure{ID: id, Reason: reason})
			continue
		}
		done++
	}
	return done, failed, nil
}

func (s *Service) inSpace(ctx context.Context, id, spaceID uuid.UUID) error {
	var found uuid.UUID
	err := s.pool.QueryRow(ctx, "SELECT space_id FROM trash_items WHERE id = $1", id).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) || found != spaceID {
		return apperr.NotFound()
	}
	return err
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
		ok, err := s.identity.HasGlobalPermissionIn(ctx, q, c.UserID, identity.PermAdministrator)
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

// UndoDelete lets the person who just deleted a page or a comment take it back, also
// without administer (PAG-085). Nobody else can, and it does not work for long.
func (s *Service) UndoDelete(ctx context.Context, c identity.Caller, objectType string, objectID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		it, err := scan(tx.QueryRow(ctx, "SELECT "+columns+` FROM trash_items
			WHERE object_id = $1 AND object_type = $4 AND deleted_by = $2
			  AND deleted_at > now() - make_interval(secs => $3) FOR UPDATE`, objectID, c.UserID, undoWindow.Seconds(), objectType))
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

// hiddenFrom says whether an item holds something the person may not see: the page
// of a file or a comment, or a page or anything that would go along with it.
func hiddenFrom(ctx context.Context, q db.Querier, userID uuid.UUID, it Item) (bool, error) {
	if it.ObjectType == TypeSpace {
		return false, nil
	}
	var hidden bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM perm_hidden_pages($1) h
		WHERE h.page_id = (SELECT a.page_id FROM attachments a WHERE a.id = $2)
		   OR h.page_id = (SELECT m.page_id FROM comments m WHERE m.id = $2)
		   OR h.page_id IN (SELECT t.descendant_id FROM page_tree t WHERE t.ancestor_id = $2))`, userID, it.ObjectID).Scan(&hidden)
	return hidden, err
}

// Purge removes an item for good, before its retention period ends. Nobody destroys
// what they cannot see: such an item stays until retention ends, which PurgeExpired
// does as a system task.
func (s *Service) Purge(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return s.run(ctx, c, id, func(h Handler, tx pgx.Tx, it Item) error {
		hidden, err := hiddenFrom(ctx, tx, c.UserID, it)
		if err != nil {
			return err
		}
		if hidden {
			return apperr.Conflict("hidden_item", "This holds something you cannot see, so you cannot delete it for good. It goes by itself when its time in the trash ends.")
		}
		return h.Purge(ctx, tx, c, it)
	})
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
