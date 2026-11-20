// SPDX-License-Identifier: Apache-2.0

package permissions

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/db"
)

// WriterPermissions are the permissions that make a grant one of a writer: creating and editing
// pages, the permissions split off those two, and running the space. A draft space started from
// a space with content takes over only the grants that hold one of them, never a grant that only
// views, so readers of the old space never see the drafts (spec 073, PUB-1302).
func WriterPermissions() []string {
	out := []string{PageCreate, PageEdit, SpaceAdminister}
	for finer, broad := range SplitFrom {
		if broad == PageCreate || broad == PageEdit {
			out = append(out, finer)
		}
	}
	slices.Sort(out)
	return out
}

// CopyWriterGrants gives a new draft space the grants of a space whose subjects hold one of the
// writer permissions, each subject with exactly the permissions it holds there, inside tx and
// with no check: the caller has checked the rights to start the draft space (PUB-1302). It
// returns how many subjects it gave grants.
func CopyWriterGrants(ctx context.Context, tx pgx.Tx, from, to uuid.UUID) (int, error) {
	rows, err := tx.Query(ctx, "SELECT subject_type, subject_id, permission FROM space_grants WHERE space_id = $1", from)
	if err != nil {
		return 0, err
	}
	grants, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Grant, error) {
		var g Grant
		err := r.Scan(&g.Subject.Type, &g.Subject.ID, &g.Permission)
		return g, err
	})
	if err != nil {
		return 0, err
	}
	held := map[Subject][]string{}
	for _, g := range grants {
		held[g.Subject] = append(held[g.Subject], g.Permission)
	}
	writing := WriterPermissions()
	copied := 0
	for sub, perms := range held {
		if !slices.ContainsFunc(perms, func(p string) bool { return slices.Contains(writing, p) }) {
			continue
		}
		slices.Sort(perms)
		if _, err := tx.Exec(ctx, "SELECT rbac_set_exact($1, $2, $3, $4::text[])", to, sub.Type, sub.ID, perms); err != nil {
			return copied, err
		}
		copied++
	}
	return copied, nil
}

// ViewRestrictedAmong counts the pages among list that carry a view restriction of their own, for
// the preview of a start (PUB-1309).
func ViewRestrictedAmong(ctx context.Context, q db.Querier, list []uuid.UUID) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(DISTINCT page_id) FROM page_restrictions WHERE kind = 'view' AND page_id = ANY($1)`, list).Scan(&n)
	return n, err
}

// AdministratorUsers lists the people who run a space, directly or through a group, at most
// limit, so they can be told what happened to it (spec 073, PUB-1046, PUB-1047).
func AdministratorUsers(ctx context.Context, q db.Querier, spaceID uuid.UUID, limit int) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT u FROM (
			SELECT g.subject_id AS u FROM space_grants g WHERE g.space_id = $1 AND g.permission = $2 AND g.subject_type = 'user'
			UNION SELECT gm.user_id FROM space_grants g JOIN group_members gm ON gm.group_id = g.subject_id
			WHERE g.space_id = $1 AND g.permission = $2 AND g.subject_type = 'group') a
		ORDER BY u LIMIT $3`, spaceID, SpaceAdminister, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}
