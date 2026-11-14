// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/spaces"
)

// Runs as the release screen shows them: one with its progress and report, the history, and
// cancelling and resuming (PUB-453 to PUB-455, PUB-910, PUB-1214, PUB-1215).

// historyPage is the number of runs the history shows at once.
const historyPage = 50

// Run is a run as those who may release from the draft space see it.
type Run struct {
	ID          uuid.UUID       `json:"id"`
	Scope       string          `json:"scope"`
	Status      string          `json:"status"`
	Note        string          `json:"note"`
	Silent      bool            `json:"silent"`
	ActorName   string          `json:"actor_name"`
	Total       int             `json:"total"`
	Done        int             `json:"done"`
	Batches     int             `json:"batches"`
	BatchesDone int             `json:"batches_done"`
	StopReason  string          `json:"stop_reason,omitempty"`
	PreviewedAt time.Time       `json:"previewed_at"`
	StartedAt   *time.Time      `json:"started_at"`
	FinishedAt  *time.Time      `json:"finished_at"`
	Counts      json.RawMessage `json:"counts"`
	// Pages is the report: what became of each page (PUB-453). The history leaves it out.
	Pages []RunPage `json:"pages,omitempty"`
}

// runColumns reads a run; a running run without a recent sign of life shows as stopped, so it
// can be resumed after its node died or the database was restored (PUB-1032, PUB-1061).
const runColumns = `r.id, r.scope, CASE WHEN r.status = 'running' AND (r.heartbeat_at IS NULL OR r.heartbeat_at < now() - $3::interval)
		THEN 'stopped' ELSE r.status END, r.note, r.silent, COALESCE(u.display_name, ''), r.total, r.done, r.batches, r.batches_done,
		r.stop_reason, r.previewed_at, r.started_at, r.finished_at, r.counts
	FROM release_runs r LEFT JOIN users u ON u.id = r.actor_id`

func scanRun(row pgx.Row) (Run, error) {
	var r Run
	err := row.Scan(&r.ID, &r.Scope, &r.Status, &r.Note, &r.Silent, &r.ActorName, &r.Total, &r.Done, &r.Batches, &r.BatchesDone,
		&r.StopReason, &r.PreviewedAt, &r.StartedAt, &r.FinishedAt, &r.Counts)
	return r, err
}

// RunView reads one run with its report, for those with space.release in the draft space.
func (s *Service) RunView(ctx context.Context, c identity.Caller, spaceID, runID uuid.UUID) (Run, error) {
	p, err := s.draftPairToRead(ctx, c, spaceID)
	if err != nil {
		return Run{}, err
	}
	r, err := scanRun(s.pool.QueryRow(ctx, `SELECT `+runColumns+` WHERE r.id = $1 AND r.pair_id = $2`, runID, p.ID, s.limits.StaleAfter.String()))
	if noRows(err) {
		return r, apperr.NotFound()
	}
	if err != nil {
		return r, err
	}
	rows, err := s.pool.Query(ctx, `SELECT rp.draft_page_id, rp.twin_page_id, rp.title, rp.action, rp.aspects, rp.reason, rp.outcome
		FROM release_run_pages rp WHERE rp.run_id = $1
		  AND (rp.draft_page_id NOT IN (SELECT h.page_id FROM perm_hidden_pages($2) h))
		ORDER BY rp.ord LIMIT 1000`, runID, c.UserID)
	if err != nil {
		return r, err
	}
	r.Pages, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (RunPage, error) {
		var page RunPage
		err := row.Scan(&page.PageID, &page.TwinPageID, &page.Title, &page.Action, &page.Aspects, &page.Reason, &page.Outcome)
		return page, err
	})
	if r.Pages == nil {
		r.Pages = []RunPage{}
	}
	return r, err
}

// draftPairToRead finds the pair of a draft space for someone who may see its releases:
// space.release in the draft space. A space in no pair, or the published side, has none.
func (s *Service) draftPairToRead(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (pair, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceRelease); err != nil {
		return pair{}, err
	}
	p, err := activePairOf(ctx, s.pool, spaceID, false)
	if err != nil {
		return p, err
	}
	if p.Draft != spaceID {
		return p, apperr.Conflict("not_the_draft_space", "Releases are made from the draft space.")
	}
	return p, nil
}

// History lists the runs of a pair, newest first, previews that were never confirmed left out
// (PUB-910).
func (s *Service) History(ctx context.Context, c identity.Caller, spaceID uuid.UUID, offset int) ([]Run, error) {
	p, err := s.draftPairToRead(ctx, c, spaceID)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+runColumns+` WHERE r.pair_id = $1 AND r.status NOT IN ('previewed', 'expired')
		ORDER BY COALESCE(r.started_at, r.previewed_at) DESC, r.id DESC LIMIT $4 OFFSET $2`, p.ID, max(offset, 0), s.limits.StaleAfter.String(), historyPage)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Run, error) { return scanRun(row) })
	if out == nil {
		out = []Run{}
	}
	return out, err
}

// Cancel stops a run that waits, or asks a running one to stop before its next batch; batches
// done stay done (PUB-455). Anyone with space.release may cancel.
func (s *Service) Cancel(ctx context.Context, c identity.Caller, spaceID, runID uuid.UUID) (Run, error) {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := s.draftPair(ctx, tx, c, spaceID, false)
		if err != nil {
			return err
		}
		r, status, err := loadRunForChange(ctx, tx, p, runID, s.limits.StaleAfter)
		if err != nil {
			return err
		}
		switch status {
		case "queued", "stopped":
			if err := s.dropUnwritten(ctx, tx, runID); err != nil {
				return err
			}
			return s.closeRun(ctx, tx, c, r, "cancelled", "cancelled")
		case "running":
			if _, err := tx.Exec(ctx, "UPDATE release_runs SET cancel_asked = true WHERE id = $1", runID); err != nil {
				return err
			}
			_, err = audit.Record(ctx, tx, c.Event("release.cancel_asked", "release_run", runID.String(), p.Draft.String(), map[string]any{"pair_id": p.ID}))
			return err
		}
		return apperr.Conflict("run_not_going", "This run is not waiting or going.")
	})
	if err != nil {
		return Run{}, err
	}
	return s.RunView(ctx, c, spaceID, runID)
}

// Resume carries on a run that stopped, with the versions of its preview; the person who
// resumes it releases the rest, with their own rights (PUB-451, PUB-1061, decision D27).
func (s *Service) Resume(ctx context.Context, c identity.Caller, spaceID, runID uuid.UUID) (Run, error) {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := s.draftPair(ctx, tx, c, spaceID, true)
		if err != nil {
			return err
		}
		if _, _, err := bothActive(ctx, tx, p); err != nil {
			return err
		}
		_, status, err := loadRunForChange(ctx, tx, p, runID, s.limits.StaleAfter)
		if err != nil {
			return err
		}
		if status != "stopped" {
			return apperr.Conflict("run_not_stopped", "Only a run that stopped can be resumed.")
		}
		if _, err := tx.Exec(ctx, `UPDATE release_runs SET status = 'queued', stop_reason = '', cancel_asked = false, actor_id = $2,
			heartbeat_at = NULL WHERE id = $1`, runID, c.UserID); err != nil {
			return err
		}
		if _, err := s.jobs.Enqueue(ctx, tx, JobKindRun, runPayload{RunID: runID}, c.UserID); err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, c.Event("release.resumed", "release_run", runID.String(), p.Draft.String(), map[string]any{"pair_id": p.ID}))
		return err
	})
	if err != nil {
		return Run{}, err
	}
	return s.RunView(ctx, c, spaceID, runID)
}

// RunPages is what a run did, for its notifications: the twins it released and took off, and
// the home pages of both spaces. Ids only; the notification module checks who may hear.
type RunPages struct {
	Released      []uuid.UUID
	TakenOff      []uuid.UUID
	PublishedHome *uuid.UUID
	DraftHome     *uuid.UUID
}

// RunPages reads what a run did, for the worker that tells people about it.
func (s *Service) RunPages(ctx context.Context, runID uuid.UUID) (RunPages, error) {
	var out RunPages
	var draft, published uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT
			ARRAY(SELECT twin_page_id FROM release_run_pages WHERE run_id = r.id AND outcome = 'released' AND twin_page_id IS NOT NULL),
			ARRAY(SELECT twin_page_id FROM release_run_pages WHERE run_id = r.id AND outcome = 'taken_off' AND twin_page_id IS NOT NULL),
			sp.draft_space_id, sp.published_space_id
		FROM release_runs r JOIN space_pairs sp ON sp.id = r.pair_id WHERE r.id = $1`, runID).
		Scan(&out.Released, &out.TakenOff, &draft, &published)
	if noRows(err) {
		return out, nil // the pair went with a space in the meantime
	}
	if err != nil {
		return out, err
	}
	for _, side := range []struct {
		id   uuid.UUID
		home **uuid.UUID
	}{{draft, &out.DraftHome}, {published, &out.PublishedHome}} {
		sp, err := spaces.LoadIn(ctx, s.pool, side.id)
		if apperr.IsNotFound(err) {
			continue
		}
		if err != nil {
			return out, err
		}
		*side.home = sp.HomePageID
	}
	return out, nil
}

// loadRunForChange locks a run of a pair and reads its status as the screen shows it.
func loadRunForChange(ctx context.Context, tx pgx.Tx, p pair, runID uuid.UUID, staleAfter time.Duration) (runRow, string, error) {
	var r runRow
	var status string
	err := tx.QueryRow(ctx, `SELECT id, pair_id, actor_id, scope, note, silent, batches,
			CASE WHEN status = 'running' AND (heartbeat_at IS NULL OR heartbeat_at < now() - $3::interval) THEN 'stopped' ELSE status END
		FROM release_runs WHERE id = $1 AND pair_id = $2 AND scope <> 'page' FOR UPDATE`, runID, p.ID, staleAfter.String()).
		Scan(&r.id, &r.pair, &r.actor, &r.scope, &r.note, &r.silent, &r.batches, &status)
	if noRows(err) {
		return r, "", apperr.NotFound()
	}
	if err == nil && status == "stopped" {
		// A run whose node died is still marked running: it stops now, so its next turn starts clean.
		_, err = tx.Exec(ctx, "UPDATE release_runs SET status = 'stopped' WHERE id = $1 AND status = 'running'", runID)
	}
	r.status = status
	return r, status, err
}
