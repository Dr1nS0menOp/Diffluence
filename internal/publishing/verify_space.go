// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/audit"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/jobs"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// The space-wide verification (spec 073, wave PB4, PUB-920): every twin of a pair is checked as
// one is (PUB-732), in the background, and the report says per page whether it is the same as
// its draft version, differs, or has no draft version left to compare with.

// JobKindVerify is the background work of a space-wide verification.
const JobKindVerify = "release.verify"

// maxVerifiedListed bounds the pages a report lists; the counts cover every page.
const maxVerifiedListed = 2000

// verifyPayload names the pair a verification checks.
type verifyPayload struct {
	PairID uuid.UUID `json:"pair_id"`
}

// VerifiedPage is one twin in the report of a space-wide verification.
type VerifiedPage struct {
	TwinPageID   uuid.UUID  `json:"twin_page_id"`
	DraftPageID  *uuid.UUID `json:"draft_page_id,omitempty"`
	Title        string     `json:"title"`
	Outcome      string     `json:"outcome"`
	DraftVersion int        `json:"draft_version,omitempty"`
}

// SpaceVerification is the report of a space-wide verification: how many twins had each outcome,
// and the twins the person may see, those that are not the same first. Twins they may not see are
// counted only.
type SpaceVerification struct {
	Counts map[string]int `json:"counts"`
	Pages  []VerifiedPage `json:"pages"`
	Hidden int            `json:"hidden"`
}

// VerifySpace queues the verification of every twin of the pair of a draft space, for those who
// see the draft space's audit, as for one page (PUB-920).
func (s *Service) VerifySpace(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (StartJob, error) {
	var out StartJob
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceAudit); err != nil {
			return err
		}
		p, err := activePairOf(ctx, tx, spaceID, false)
		if err != nil {
			return err
		}
		if p.Draft != spaceID {
			return apperr.Conflict("not_the_draft_space", "Verification starts from the draft space.")
		}
		if out.JobID, err = s.jobs.Enqueue(ctx, tx, JobKindVerify, verifyPayload{PairID: p.ID}, c.UserID); err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, c.Event("release.verify_asked", "space", p.Draft.String(), p.Draft.String(),
			map[string]any{"pair_id": p.ID, "job_id": out.JobID}))
		return err
	})
	return out, err
}

// verifyJob checks every twin of a pair, in one transaction that reads one moment of the
// database, and records the counts in the draft space's audit.
func (s *Service) verifyJob(ctx context.Context, j jobs.Job) (any, error) {
	var in verifyPayload
	if err := json.Unmarshal(j.Payload, &in); err != nil {
		return nil, err
	}
	c := identity.Caller{UserID: j.CreatedBy, Meta: identity.RequestMeta{RequestID: "job-" + j.ID.String()}}
	out := SpaceVerification{Counts: map[string]int{}, Pages: []VerifiedPage{}}
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead}, func(tx pgx.Tx) error {
		p, err := scanPair(tx.QueryRow(ctx, `SELECT `+pairColumns+` FROM space_pairs WHERE id = $1 AND ended_at IS NULL`, in.PairID))
		if noRows(err) {
			return errNotPaired()
		}
		if err != nil {
			return err
		}
		// The rights are checked again as the work begins: they may have gone since it was asked.
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, p.Draft, permissions.SpaceAudit); err != nil {
			return err
		}
		if err := s.verifyPairs(ctx, tx, c, p, &out); err != nil {
			return err
		}
		if err := s.verifyRemoved(ctx, tx, c, p, &out); err != nil {
			return err
		}
		sortVerified(out.Pages)
		if len(out.Pages) > maxVerifiedListed {
			out.Pages = out.Pages[:maxVerifiedListed]
		}
		_, err = audit.Record(ctx, tx, c.Event("release.space_verified", "space", p.Draft.String(), p.Draft.String(),
			map[string]any{"pair_id": p.ID, "counts": out.Counts}))
		return err
	})
	return out, err
}

// verifiedTwin is a twin of a pair as the report lists it.
type verifiedTwin struct {
	draft  uuid.UUID
	twin   uuid.UUID
	title  string
	hidden bool
}

func (s *Service) verifyPairs(ctx context.Context, tx pgx.Tx, c identity.Caller, p pair, out *SpaceVerification) error {
	rows, err := tx.Query(ctx, `SELECT pp.draft_page_id, pp.twin_page_id, t.title,
			pp.twin_page_id IN (SELECT h.page_id FROM perm_hidden_pages($2) h) OR pp.draft_page_id IN (SELECT h.page_id FROM perm_hidden_pages($2) h)
		FROM page_pairs pp JOIN pages t ON t.id = pp.twin_page_id
		WHERE pp.pair_id = $1 AND pp.released_version > 0 ORDER BY t.title, pp.twin_page_id`, p.ID, c.UserID)
	if err != nil {
		return err
	}
	twins, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (verifiedTwin, error) {
		var v verifiedTwin
		err := r.Scan(&v.draft, &v.twin, &v.title, &v.hidden)
		return v, err
	})
	if err != nil {
		return err
	}
	for _, t := range twins {
		res, err := s.verify(ctx, tx, p, t.draft)
		if err != nil {
			return err
		}
		out.Counts[res.Outcome]++
		if t.hidden {
			out.Hidden++
			continue
		}
		draft := t.draft
		out.Pages = append(out.Pages, VerifiedPage{TwinPageID: t.twin, DraftPageID: &draft, Title: t.title, Outcome: res.Outcome,
			DraftVersion: res.DraftVersion})
	}
	return nil
}

// verifyRemoved reports the twins whose draft page was purged: nothing is left to compare them
// with (PUB-1012). A twin whose draft page moved away is the moved page's, and is left out.
func (s *Service) verifyRemoved(ctx context.Context, tx pgx.Tx, c identity.Caller, p pair, out *SpaceVerification) error {
	rows, err := tx.Query(ctx, `SELECT r.twin_page_id, t.title, r.twin_page_id IN (SELECT h.page_id FROM perm_hidden_pages($2) h)
		FROM release_removals r JOIN pages t ON t.id = r.twin_page_id AND t.status <> 'trashed'
		WHERE r.pair_id = $1 AND r.cause = 'purged' ORDER BY t.title, r.twin_page_id`, p.ID, c.UserID)
	if err != nil {
		return err
	}
	twins, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (verifiedTwin, error) {
		var v verifiedTwin
		err := r.Scan(&v.twin, &v.title, &v.hidden)
		return v, err
	})
	if err != nil {
		return err
	}
	for _, t := range twins {
		out.Counts[VerifyDraftPurged]++
		if t.hidden {
			out.Hidden++
			continue
		}
		out.Pages = append(out.Pages, VerifiedPage{TwinPageID: t.twin, Title: t.title, Outcome: VerifyDraftPurged})
	}
	return nil
}

// sortVerified puts the twins that are not the same first, keeping the order of their titles.
func sortVerified(list []VerifiedPage) {
	slices.SortStableFunc(list, func(a, b VerifiedPage) int {
		return boolOrder(a.Outcome == VerifyOK) - boolOrder(b.Outcome == VerifyOK)
	})
}
