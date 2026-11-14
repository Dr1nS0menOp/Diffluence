// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// The consistency check of a pair (PUB-343, PUB-921): it recomputes every status from the
// versions and compares it with the stored facts, and lists twin versions that no release
// wrote. A difference is fixed by recomputing.

// CheckItem is one difference the check found.
type CheckItem struct {
	PageID  uuid.UUID `json:"page_id"`
	Version int       `json:"version"`
	// What is "facts" (the facts of the last release disagree with that version), "status"
	// (the aspects out of step were wrong) or "unreleased_version" (a twin version no release wrote).
	What string `json:"what"`
}

// CheckResult is what the check of one pair found.
type CheckResult struct {
	PairID uuid.UUID   `json:"pair_id"`
	Pages  int         `json:"pages"`
	Found  []CheckItem `json:"found"`
	Fixed  bool        `json:"fixed"`
}

// errCheckOnly rolls back the recomputation of a check that only looks.
var errCheckOnly = errors.New("publishing: the check only looks")

// Check runs the consistency check of the pair of a draft space, for its administrators, and
// fixes what it finds when asked.
func (s *Service) Check(ctx context.Context, c identity.Caller, spaceID uuid.UUID, fix bool) (CheckResult, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
		return CheckResult{}, err
	}
	p, err := activePairOf(ctx, s.pool, spaceID, false)
	if err != nil {
		return CheckResult{}, err
	}
	return checkPair(ctx, s.pool, c, p, fix)
}

// CheckAll runs the check of every active pair, for `diffluence check publishing`.
func CheckAll(ctx context.Context, pool *pgxpool.Pool, fix bool) ([]CheckResult, error) {
	rows, err := pool.Query(ctx, `SELECT `+pairColumns+` FROM space_pairs WHERE ended_at IS NULL ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	pairs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (pair, error) { return scanPair(r) })
	if err != nil {
		return nil, err
	}
	out := []CheckResult{}
	for _, p := range pairs {
		res, err := checkPair(ctx, pool, identity.Caller{System: "cli"}, p, fix)
		if err != nil {
			return out, err
		}
		out = append(out, res)
	}
	return out, nil
}

func checkPair(ctx context.Context, pool *pgxpool.Pool, c identity.Caller, p pair, fix bool) (CheckResult, error) {
	out := CheckResult{PairID: p.ID, Found: []CheckItem{}}
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		facts, err := tx.Query(ctx, `SELECT pp.draft_page_id, pp.released_version FROM page_pairs pp
			JOIN page_versions v ON v.page_id = pp.draft_page_id AND v.number = pp.released_version
			WHERE pp.pair_id = $1 AND pp.released_version > 0 AND pp.released_hash <> ''
			  AND (pp.released_title IS DISTINCT FROM v.title OR pp.released_hash IS DISTINCT FROM page_pair_content_hash(v.document))
			ORDER BY pp.draft_page_id`, p.ID)
		if err != nil {
			return err
		}
		if err := collectItems(facts, "facts", &out); err != nil {
			return err
		}
		// The facts are put back from the version they name; then every status is recomputed.
		if _, err := tx.Exec(ctx, `UPDATE page_pairs pp SET released_title = v.title, released_hash = page_pair_content_hash(v.document)
			FROM page_versions v WHERE v.page_id = pp.draft_page_id AND v.number = pp.released_version AND pp.pair_id = $1 AND pp.released_hash <> ''
			  AND (pp.released_title IS DISTINCT FROM v.title OR pp.released_hash IS DISTINCT FROM page_pair_content_hash(v.document))`, p.ID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM page_pairs WHERE pair_id = $1", p.ID).Scan(&out.Pages); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `CREATE TEMPORARY TABLE page_pairs_before ON COMMIT DROP AS
			SELECT draft_page_id, released_version, changed FROM page_pairs WHERE pair_id = $1`, p.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT page_pair_refresh(draft_page_id) FROM page_pairs_before"); err != nil {
			return err
		}
		statuses, err := tx.Query(ctx, `SELECT b.draft_page_id, b.released_version FROM page_pairs_before b
			JOIN page_pairs pp ON pp.draft_page_id = b.draft_page_id WHERE pp.changed IS DISTINCT FROM b.changed ORDER BY b.draft_page_id`)
		if err != nil {
			return err
		}
		if err := collectItems(statuses, "status", &out); err != nil {
			return err
		}
		// Every twin version is the result of a release (PUB-921).
		// The version a new published space starts with, made with the pair, is no release's.
		versions, err := tx.Query(ctx, `SELECT v.page_id, v.number FROM page_pairs pp JOIN page_versions v ON v.page_id = pp.twin_page_id
			JOIN space_pairs sp ON sp.id = pp.pair_id
			WHERE pp.pair_id = $1 AND v.created_at > sp.created_at AND NOT EXISTS (SELECT 1 FROM release_run_pages rp
				WHERE rp.twin_page_id = v.page_id AND rp.twin_version = v.number AND rp.outcome = 'released')
			ORDER BY v.page_id, v.number`, p.ID)
		if err != nil {
			return err
		}
		if err := collectItems(versions, "unreleased_version", &out); err != nil {
			return err
		}
		if !fix {
			return errCheckOnly
		}
		out.Fixed = true
		_, err = audit.Record(ctx, tx, c.Event("release.checked", "space", p.Draft.String(), p.Draft.String(),
			map[string]any{"pair_id": p.ID, "found": len(out.Found), "fixed": true}))
		return err
	})
	if errors.Is(err, errCheckOnly) {
		err = nil
	}
	return out, err
}

func collectItems(rows pgx.Rows, what string, out *CheckResult) error {
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (CheckItem, error) {
		item := CheckItem{What: what}
		err := r.Scan(&item.PageID, &item.Version)
		return item, err
	})
	out.Found = append(out.Found, items...)
	return err
}
