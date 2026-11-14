// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/db"
)

// Title returns the title of a page through q, so another module never names the table
// of pages for it (ARC-027). It decides nothing: the caller has already checked that the
// person it shows the title to may view the page.
func Title(ctx context.Context, q db.Querier, id uuid.UUID) (string, error) {
	var title string
	err := q.QueryRow(ctx, "SELECT title FROM pages WHERE id = $1", id).Scan(&title)
	return title, err
}
