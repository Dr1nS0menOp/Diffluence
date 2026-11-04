// SPDX-License-Identifier: Apache-2.0

// Package instance identifies this installation. The id is recorded on every
// piece of content (origin_instance_id) so instances can be merged later.
package instance

import (
	"context"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/ids"
)

// Ensure returns the instance id, creating the instance row on first start.
// Safe to call concurrently from several nodes.
func Ensure(ctx context.Context, q db.Querier, name string) (uuid.UUID, error) {
	if _, err := q.Exec(ctx,
		"INSERT INTO instance (id, name) VALUES ($1, $2) ON CONFLICT (singleton) DO NOTHING",
		ids.New(), name); err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err := q.QueryRow(ctx, "SELECT id FROM instance").Scan(&id)
	return id, err
}
