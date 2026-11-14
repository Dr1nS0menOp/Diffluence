// SPDX-License-Identifier: Apache-2.0

package spaces

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
)

// Name returns the name of a space through q, for a module that fills it into a page
// (spec 074, TPL-201) without the cost of Get. It decides nothing: the caller checks who
// may act on the space.
func Name(ctx context.Context, q db.Querier, id uuid.UUID) (string, error) {
	var name string
	err := q.QueryRow(ctx, "SELECT name FROM spaces WHERE id = $1", id).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.NotFound()
	}
	return name, err
}

// HomePage returns the home page of a space through q, or nil while it has none, so
// another module never names the table of spaces for it (ARC-027). It decides nothing:
// the caller checks who may act on the space.
func HomePage(ctx context.Context, q db.Querier, id uuid.UUID) (*uuid.UUID, error) {
	var home *uuid.UUID
	err := q.QueryRow(ctx, "SELECT home_page_id FROM spaces WHERE id = $1", id).Scan(&home)
	return home, err
}
