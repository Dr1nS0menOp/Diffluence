// SPDX-License-Identifier: Apache-2.0

package spaces

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
)

const (
	maxShortcuts     = 20
	maxShortcutLabel = 100
)

// Shortcut is a link in the sidebar of a space, chosen by its administrators.
type Shortcut struct {
	ID     uuid.UUID `json:"id"`
	Label  string    `json:"label"`
	Target string    `json:"target"`
}

// ShortcutInput is one shortcut in the order it should show.
type ShortcutInput struct {
	Label  string `json:"label"`
	Target string `json:"target"`
}

func (in ShortcutInput) validated() (ShortcutInput, error) {
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" || utf8.RuneCountInString(in.Label) > maxShortcutLabel || strings.ContainsAny(in.Label, "\r\n\x00") {
		return in, apperr.Invalid("A shortcut needs a label of at most 100 characters.")
	}
	target, err := document.SafeHref(in.Target)
	lower := strings.ToLower(target)
	if err != nil || strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(target, "#") {
		return in, apperr.Invalid("A shortcut goes to an address in this instance, or to an http or https address.")
	}
	in.Target = target
	return in, nil
}

// Shortcuts lists the shortcuts of a space, in order.
func (s *Service) Shortcuts(ctx context.Context, c identity.Caller, id uuid.UUID) ([]Shortcut, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, id, permissions.SpaceView); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT id, label, target FROM space_shortcuts WHERE space_id = $1 ORDER BY position, id", id)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Shortcut, error) {
		var v Shortcut
		err := r.Scan(&v.ID, &v.Label, &v.Target)
		return v, err
	})
	if out == nil {
		out = []Shortcut{}
	}
	return out, err
}

// SetShortcuts replaces the shortcuts of a space.
func (s *Service) SetShortcuts(ctx context.Context, c identity.Caller, id uuid.UUID, list []ShortcutInput) error {
	if len(list) > maxShortcuts {
		return apperr.Invalid("A space can have at most 20 shortcuts.")
	}
	clean := make([]ShortcutInput, len(list))
	for i, in := range list {
		v, err := in.validated()
		if err != nil {
			return err
		}
		clean[i] = v
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, id, permissions.SpaceAdminister); err != nil {
			return err
		}
		sp, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM space_shortcuts WHERE space_id = $1", id); err != nil {
			return err
		}
		for i, v := range clean {
			if _, err := tx.Exec(ctx, "INSERT INTO space_shortcuts (id, space_id, label, target, position) VALUES ($1, $2, $3, $4, $5)",
				ids.New(), id, v.Label, v.Target, i); err != nil {
				return err
			}
		}
		return record(ctx, tx, c, "space.shortcuts_changed", sp, map[string]any{"count": len(clean)})
	})
}
