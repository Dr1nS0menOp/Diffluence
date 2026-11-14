// SPDX-License-Identifier: Apache-2.0

package spaces

import (
	"context"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/db"
)

// HomePage returns the home page of a space through q, or nil while it has none, so
// another module never names the table of spaces for it (ARC-027). It decides nothing:
// the caller checks who may act on the space.
func HomePage(ctx context.Context, q db.Querier, id uuid.UUID) (*uuid.UUID, error) {
	var home *uuid.UUID
	err := q.QueryRow(ctx, "SELECT home_page_id FROM spaces WHERE id = $1", id).Scan(&home)
	return home, err
}
