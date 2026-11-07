// SPDX-License-Identifier: Apache-2.0

// Package savedblocks keeps blocks a person saved to use again (EDT-026, spec 027, D16).
// A block belongs to one person. Sharing lets other members read it, and nothing more.
package savedblocks

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
)

const (
	maxNameRunes   = 100
	maxListed      = 200
	maxPerPerson   = 200
	objectType     = "saved_block"
	emptySpaceName = ""
)

// Block is one saved block.
type Block struct {
	ID        uuid.UUID       `json:"id"`
	OwnerID   uuid.UUID       `json:"owner_id"`
	Name      string          `json:"name"`
	Document  json.RawMessage `json:"document"`
	Shared    bool            `json:"shared"`
	CreatedAt time.Time       `json:"created_at"`
	IsOwn     bool            `json:"is_own"`
}

// Service reads and writes saved blocks.
type Service struct {
	pool *pgxpool.Pool
}

// NewService makes the service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func cleanName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > maxNameRunes {
		return "", apperr.Invalid("A saved block needs a name of at most 100 characters.")
	}
	return name, nil
}

// List returns the blocks of this person, and what members shared. A guest sees only
// their own: a shared block is for the people of the organisation.
func (s *Service) List(ctx context.Context, c identity.Caller) ([]Block, error) {
	rows, err := s.pool.Query(ctx, `SELECT b.id, b.owner_id, b.name, b.document, b.shared, b.created_at
		FROM saved_blocks b
		WHERE b.owner_id = $1
		   OR (b.shared AND EXISTS (SELECT 1 FROM users u WHERE u.id = $1 AND u.type = 'member'))
		ORDER BY lower(b.name), b.id LIMIT $2`, c.UserID, maxListed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Block{}
	for rows.Next() {
		var b Block
		if err := rows.Scan(&b.ID, &b.OwnerID, &b.Name, &b.Document, &b.Shared, &b.CreatedAt); err != nil {
			return nil, err
		}
		b.IsOwn = b.OwnerID == c.UserID
		out = append(out, b)
	}
	return out, rows.Err()
}

// Create saves a block. What is saved passes the same gate as a page.
func (s *Service) Create(ctx context.Context, c identity.Caller, name string, raw []byte, shared bool) (Block, error) {
	clean, err := cleanName(name)
	if err != nil {
		return Block{}, err
	}
	doc, err := document.Normalize(raw)
	if err != nil {
		return Block{}, err
	}
	stored, err := document.Marshal(doc)
	if err != nil {
		return Block{}, err
	}
	b := Block{ID: ids.New(), OwnerID: c.UserID, Name: clean, Document: stored, Shared: shared, IsOwn: true}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM saved_blocks WHERE owner_id = $1", c.UserID).Scan(&count); err != nil {
			return err
		}
		if count >= maxPerPerson {
			return apperr.Invalid("You saved as many blocks as a person can. Delete one first.")
		}
		if err := tx.QueryRow(ctx, `INSERT INTO saved_blocks (id, owner_id, name, document, shared)
			VALUES ($1, $2, $3, $4, $5) RETURNING created_at`, b.ID, c.UserID, clean, stored, shared).Scan(&b.CreatedAt); err != nil {
			return err
		}
		_, err := audit.Record(ctx, tx, c.Event("saved_block.created", objectType, b.ID.String(), emptySpaceName, map[string]any{"shared": shared}))
		return err
	})
	return b, err
}

// SetShared opens a block to other members, or closes it again. Only the owner can.
func (s *Service) SetShared(ctx context.Context, c identity.Caller, id uuid.UUID, shared bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE saved_blocks SET shared = $3 WHERE id = $1 AND owner_id = $2", id, c.UserID, shared)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound()
		}
		_, err = audit.Record(ctx, tx, c.Event("saved_block.shared", objectType, id.String(), emptySpaceName, map[string]any{"shared": shared}))
		return err
	})
}

// Delete removes a block. Only the owner can, also when it is shared. Somebody else's
// block looks the same as one that does not exist.
func (s *Service) Delete(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM saved_blocks WHERE id = $1 AND owner_id = $2", id, c.UserID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound()
		}
		_, err = audit.Record(ctx, tx, c.Event("saved_block.deleted", objectType, id.String(), emptySpaceName, nil))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound()
		}
		return err
	})
}
