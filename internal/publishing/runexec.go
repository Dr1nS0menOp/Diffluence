// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/spaces"
	"github.com/diffluence/diffluence/internal/storage"
)

// Carrying out a run of more than one page, in the worker (PUB-450 to PUB-455, PUB-1061): up
// to the limit in one transaction, past it in batches, each its own transaction, with the
// twins made first so every link of an early batch resolves (PUB-514).

// Why a run stopped before its end.
const (
	stopRights    = "rights"
	stopPairEnded = "pair_ended"
	stopSpaces    = "space_not_active"
	stopRoom      = "room"
	stopFailed    = "failed"
)

// runPayload names the run a job carries out.
type runPayload struct {
	RunID uuid.UUID `json:"run_id"`
}

// StartRun confirms a preview of a run of more than one page: the run is queued for the
// worker with its note, and keeps the versions its preview showed (PUB-431, PUB-433).
func (s *Service) StartRun(ctx context.Context, c identity.Caller, spaceID, runID uuid.UUID, in RunInput) (Run, error) {
	if err := checkNote(in.Note); err != nil {
		return Run{}, err
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := s.draftPair(ctx, tx, c, spaceID, true)
		if err != nil {
			return err
		}
		if _, _, err := bothActive(ctx, tx, p); err != nil {
			return err
		}
		var fresh, fits bool
		err = tx.QueryRow(ctx, `SELECT previewed_at > now() - $4::interval, COALESCE((counts->>'fits')::boolean, true) FROM release_runs
			WHERE id = $1 AND pair_id = $2 AND actor_id = $3 AND status = 'previewed' AND scope <> 'page' FOR UPDATE`,
			runID, p.ID, c.UserID, previewLife.String()).Scan(&fresh, &fits)
		if noRows(err) {
			return apperr.Conflict("no_preview", "Make a preview of this release first. A preview is confirmed once.")
		}
		if err != nil {
			return err
		}
		if !fresh {
			return apperr.Conflict("preview_expired", "This preview is more than an hour old. Make a new one to see what the release will do now.")
		}
		if !fits {
			return apperr.Conflict("no_room", "The files of this release do not fit in the published space. Make room there, or release fewer pages.")
		}
		if _, err := tx.Exec(ctx, `UPDATE release_runs SET status = 'queued', note = $2, silent = $3 WHERE id = $1`, runID, in.Note, in.Silent); err != nil {
			return err
		}
		if _, err := s.jobs.Enqueue(ctx, tx, JobKindRun, runPayload{RunID: runID}, c.UserID); err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, c.Event("release.started", "release_run", runID.String(), p.Draft.String(),
			map[string]any{"pair_id": p.ID, "note": in.Note, "silent": in.Silent}))
		return err
	})
	if err != nil {
		return Run{}, err
	}
	return s.RunView(ctx, c, spaceID, runID)
}

// runRow is a run as its worker reads it.
type runRow struct {
	id, pair, actor uuid.UUID
	draftSpace      uuid.UUID
	scope, status   string
	note            string
	silent          bool
	batches         int
	cancelAsked     bool
	// actorType is who started the run: a person, approval or the workflow (PUB-902).
	actorType string
	// causedBy is the person whose act started a run of the system: the approver, or the
	// person who withdrew the page.
	causedBy *uuid.UUID
}

// system says the run was started by approval or the workflow, not by a person: it runs with
// the rights of the release path, which the pair's administrator chose (PUB-711).
func (r runRow) system() bool { return r.actorType == actorApproval || r.actorType == actorWorkflow }

// caller is who the run acts as: the releaser, or the system with no person.
func (r runRow) caller(meta identity.RequestMeta) identity.Caller {
	if r.system() {
		return identity.Caller{System: r.actorType, Meta: meta}
	}
	return identity.Caller{UserID: r.actor, Meta: meta}
}

// details adds to audit details of the draft space who caused a run of the system (PUB-902).
func (r runRow) details(d map[string]any) map[string]any {
	if r.causedBy != nil {
		d["caused_by"] = *r.causedBy
	}
	return d
}

func (s *Service) runJob(ctx context.Context, j jobs.Job) (any, error) {
	var in runPayload
	if err := json.Unmarshal(j.Payload, &in); err != nil {
		return nil, err
	}
	// At most RunsPerNode runs at once on this node; a third waits here (PUB-1105).
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.slots }()
	r, ok, err := s.takeTurn(ctx, in.RunID)
	if err != nil || !ok {
		return nil, err
	}
	c := r.caller(identity.RequestMeta{RequestID: "job-" + j.ID.String()})
	if err := s.carryOut(ctx, c, r); err != nil {
		return map[string]any{"run_id": r.id}, errors.Join(err, s.stop(ctx, c, r, err))
	}
	return map[string]any{"run_id": r.id}, nil
}

// takeTurn waits until no other run of the pair is going, and marks this one running: one run
// per pair at a time, in the order they were queued (PUB-452, PUB-1060).
func (s *Service) takeTurn(ctx context.Context, runID uuid.UUID) (runRow, bool, error) {
	for {
		var r runRow
		var mine bool
		err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			err := tx.QueryRow(ctx, `SELECT r.id, r.pair_id, COALESCE(r.actor_id, $2), r.scope, r.status, r.note, r.silent, r.batches,
					r.cancel_asked, sp.draft_space_id, r.actor_type, r.caused_by
				FROM release_runs r JOIN space_pairs sp ON sp.id = r.pair_id WHERE r.id = $1`, runID, uuid.Nil).
				Scan(&r.id, &r.pair, &r.actor, &r.scope, &r.status, &r.note, &r.silent, &r.batches, &r.cancelAsked, &r.draftSpace,
					&r.actorType, &r.causedBy)
			if err != nil {
				return err
			}
			if r.status != "queued" {
				return nil // cancelled while it waited, or carried out already
			}
			if _, err := tx.Exec(ctx, "SELECT 1 FROM space_pairs WHERE id = $1 FOR UPDATE", r.pair); err != nil {
				return err
			}
			var earlier bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM release_runs WHERE pair_id = $1 AND id <> $2
				AND status = 'running' AND heartbeat_at > now() - $3::interval)`, r.pair, r.id, s.limits.StaleAfter.String()).Scan(&earlier); err != nil {
				return err
			}
			if earlier {
				return nil
			}
			mine = true
			_, err = tx.Exec(ctx, `UPDATE release_runs SET status = 'running', started_at = COALESCE(started_at, now()), heartbeat_at = now()
				WHERE id = $1`, r.id)
			return err
		})
		if err != nil {
			return r, false, err
		}
		if mine {
			r.status = "running"
			return r, true, nil
		}
		if r.status != "queued" {
			return r, false, nil
		}
		select {
		case <-ctx.Done():
			return r, false, ctx.Err()
		case <-time.After(s.limits.Pause):
		}
	}
}

// batchState is what each transaction of a run reads first.
type batchState struct {
	pair      pair
	draft     spaces.Space
	published spaces.Space
	// stopped is why the run cannot go on, or "" when it can.
	stopped string
}

// openBatch starts a transaction of a run: the release mark, the pair row, and the rights of
// the releaser at this moment (PUB-454). A run whose pair ended, whose spaces are no longer in
// use, or whose releaser lost a right stops with what was done; one asked to cancel stops too.
func (s *Service) openBatch(ctx context.Context, tx pgx.Tx, r runRow) (batchState, error) {
	var st batchState
	// The mark lets the trigger of a published space through, for this transaction only.
	if _, err := tx.Exec(ctx, "SELECT set_config('diffluence.release', 'on', true)"); err != nil {
		return st, err
	}
	var cancel bool
	if err := tx.QueryRow(ctx, "SELECT cancel_asked FROM release_runs WHERE id = $1 FOR UPDATE", r.id).Scan(&cancel); err != nil {
		return st, err
	}
	p, err := scanPair(tx.QueryRow(ctx, `SELECT `+pairColumns+` FROM space_pairs WHERE id = $1 AND ended_at IS NULL FOR UPDATE`, r.pair))
	if noRows(err) {
		st.stopped = stopPairEnded
		return st, nil
	}
	if err != nil {
		return st, err
	}
	st.pair = p
	if cancel {
		st.stopped = "cancelled"
		return st, nil
	}
	if st.draft, st.published, err = bothActive(ctx, tx, p); err != nil {
		if hasCode(err, "space_not_active") {
			st.stopped = stopSpaces
			return st, nil
		}
		return st, err
	}
	return st, s.checkRunRights(ctx, tx, r, &st)
}

// checkRunRights stops a run whose releaser lost a right (PUB-454). A run of the system has the
// rights of the release path, never those of the person who caused it: approving needs no right
// to release, because the pair's administrator chose approval releases (PUB-711).
func (s *Service) checkRunRights(ctx context.Context, tx pgx.Tx, r runRow, st *batchState) error {
	if r.system() {
		return nil
	}
	if err := s.runRights(ctx, tx, r.actor, st.pair); err != nil {
		if _, ok := apperr.As(err); ok {
			st.stopped = stopRights
			return nil
		}
		return err
	}
	return nil
}

// carryOut writes a run: in one transaction when it is small, in batches past that.
func (s *Service) carryOut(ctx context.Context, c identity.Caller, r runRow) error {
	if r.batches == 0 {
		return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			st, err := s.openBatch(ctx, tx, r)
			if err != nil || st.stopped != "" {
				return s.endEarly(ctx, tx, c, r, st, err)
			}
			twins, err := s.makeTwins(ctx, tx, c, st, r)
			if err != nil {
				return err
			}
			var written []writtenPage
			if _, err := s.writeBatch(ctx, tx, c, st, r, twins, 0, &written); err != nil {
				return err
			}
			return s.finish(ctx, tx, c, st, r, written)
		})
	}
	var twins map[uuid.UUID]uuid.UUID
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		st, err := s.openBatch(ctx, tx, r)
		if err != nil || st.stopped != "" {
			return s.endEarly(ctx, tx, c, r, st, err)
		}
		twins, err = s.makeTwins(ctx, tx, c, st, r)
		return err
	})
	if err != nil || twins == nil {
		return err
	}
	for {
		var more, ended bool
		err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			st, err := s.openBatch(ctx, tx, r)
			if err != nil || st.stopped != "" {
				ended = true
				return s.endEarly(ctx, tx, c, r, st, err)
			}
			var written []writtenPage
			n, err := s.writeBatch(ctx, tx, c, st, r, twins, s.limits.BatchSize, &written)
			if err != nil {
				return err
			}
			if n > 0 {
				more = true
				if err := s.orderWritten(ctx, tx, st, written); err != nil {
					return err
				}
				_, err = tx.Exec(ctx, "UPDATE release_runs SET batches_done = batches_done + 1 WHERE id = $1", r.id)
				return err
			}
			return s.finish(ctx, tx, c, st, r, nil)
		})
		if err != nil || ended || !more {
			return err
		}
	}
}

// endEarly closes a run that cannot go on, inside the transaction that found out.
func (s *Service) endEarly(ctx context.Context, tx pgx.Tx, c identity.Caller, r runRow, st batchState, err error) error {
	if err != nil {
		return err
	}
	status := "stopped"
	if st.stopped == "cancelled" {
		status = "cancelled"
		if err := s.dropUnwritten(ctx, tx, r.id); err != nil {
			return err
		}
	}
	return s.closeRun(ctx, tx, c, r, status, st.stopped)
}

// closeRun ends a run that did not finish, and tells the releaser (PUB-453, PUB-822).
func (s *Service) closeRun(ctx context.Context, tx pgx.Tx, c identity.Caller, r runRow, status, reason string) error {
	tag, err := tx.Exec(ctx, `UPDATE release_runs SET status = $2, stop_reason = $3, finished_at = CASE WHEN $2 = 'cancelled' THEN now() END
		WHERE id = $1 AND status IN ('running', 'queued', 'stopped')`, r.id, status, reason)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	var draft, published uuid.UUID
	if err := tx.QueryRow(ctx, "SELECT draft_space_id, published_space_id FROM space_pairs WHERE id = $1", r.pair).Scan(&draft, &published); err != nil {
		return err
	}
	if _, err := audit.Record(ctx, tx, c.Event("release."+status, "release_run", r.id.String(), draft.String(),
		r.details(map[string]any{"pair_id": r.pair, "reason": reason}))); err != nil {
		return err
	}
	finished := map[string]any{"run_id": r.id, "space_id": published, "draft_space_id": draft,
		"actor_id": r.actor, "silent": true, "outcome": status, "stop_reason": reason}
	if err := addApprovalTelling(ctx, tx, r, finished); err != nil {
		return err
	}
	_, err = outbox.Enqueue(ctx, tx, "release.finished", finished)
	return err
}

// addApprovalTelling adds to the end of an approval release who is told and what became of the
// page, so the approver and the authors hear that it was released, or why not (PUB-820). The
// event holds ids and codes only.
func addApprovalTelling(ctx context.Context, tx pgx.Tx, r runRow, payload map[string]any) error {
	if r.actorType != actorApproval || r.causedBy == nil {
		return nil
	}
	var page uuid.UUID
	var outcome, reason string
	var authors []uuid.UUID
	err := tx.QueryRow(ctx, `SELECT rp.draft_page_id, rp.outcome, rp.reason,
			ARRAY(SELECT jsonb_array_elements_text(COALESCE(r.options->'authors', '[]'::jsonb))::uuid)
		FROM release_runs r JOIN release_run_pages rp ON rp.run_id = r.id WHERE r.id = $1 LIMIT 1`, r.id).Scan(&page, &outcome, &reason, &authors)
	if noRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	payload["caused_by"], payload["draft_page_id"], payload["user_ids"] = *r.causedBy, page, authors
	payload["page_outcome"], payload["page_reason"] = outcome, reason
	return nil
}

// stop marks a run stopped after a transaction of it failed: each batch is done or untouched,
// and resuming goes on with the same versions (PUB-1061). It runs on a fresh context, since
// the outcome must be kept even when the worker is shutting down.
func (s *Service) stop(ctx context.Context, c identity.Caller, r runRow, cause error) error {
	reason := stopFailed
	if e, ok := apperr.As(cause); ok && e.Kind == apperr.KindTooLarge {
		reason = stopRoom
	}
	keep, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	// The cause goes to the job runner's log; the run keeps only why, in words of its own.
	return pgx.BeginFunc(keep, s.pool, func(tx pgx.Tx) error {
		return s.closeRun(keep, tx, c, r, "stopped", reason)
	})
}

// makeTwins makes, parents first, the twins of the pages a run releases for the first time,
// with no version: nobody but the releaser sees them until their batch lands (PUB-514). It
// returns every twin the run made, also those of an earlier attempt that it resumes.
func (s *Service) makeTwins(ctx context.Context, tx pgx.Tx, c identity.Caller, st batchState, r runRow) (map[uuid.UUID]uuid.UUID, error) {
	made := map[uuid.UUID]uuid.UUID{}
	rows, err := tx.Query(ctx, `SELECT draft_page_id, twin_page_id FROM release_run_pages
		WHERE run_id = $1 AND action = $2 ORDER BY ord`, r.id, actionMake)
	if err != nil {
		return nil, err
	}
	type pending struct {
		draft uuid.UUID
		twin  *uuid.UUID
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pending, error) {
		var p pending
		err := row.Scan(&p.draft, &p.twin)
		return p, err
	})
	if err != nil {
		return nil, err
	}
	for _, p := range list {
		if p.twin != nil {
			made[p.draft] = *p.twin
			continue
		}
		src, err := pages.SourceForRelease(ctx, tx, p.draft)
		if apperr.IsNotFound(err) {
			continue // gone since the preview: skipped when its batch comes
		}
		if err != nil {
			return nil, err
		}
		if src.SpaceID != st.pair.Draft || src.Status != pages.StatusPublished {
			continue
		}
		known, err := twinsOf(ctx, tx, st.pair.ID, append(slices.Clone(src.Ancestors), src.ID))
		if err != nil {
			return nil, err
		}
		if _, ok := known[src.ID]; ok {
			continue // the home page, paired since the link
		}
		for draft, twin := range made {
			known[draft] = twin
		}
		place := placeTwin(src.Ancestors, known)
		twin, err := s.makeTwin(ctx, tx, c, st.pair, st.published.ID, place.Parent, src.Page)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, "UPDATE release_run_pages SET twin_page_id = $3 WHERE run_id = $1 AND draft_page_id = $2", r.id, src.ID, twin.ID); err != nil {
			return nil, err
		}
		made[src.ID] = twin.ID
	}
	_, err = tx.Exec(ctx, "UPDATE release_runs SET heartbeat_at = now() WHERE id = $1", r.id)
	return made, err
}

// writtenPage is a page a run wrote, for ordering its twin among its siblings.
type writtenPage struct {
	draft       uuid.UUID
	draftParent *uuid.UUID
	twin        uuid.UUID
	twinParent  *uuid.UUID
}

// runPageRow is a page of a run that waits to be written.
type runPageRow struct {
	draft   uuid.UUID
	version int
}

// writeBatch writes the next pages of a run, up to limit (0 for all), and says how many it
// took. A page whose file cannot be read stops with its pages below, and the others go on
// (PUB-1065); a page deleted, moved away, left out or hidden from the releaser since the
// preview is skipped (PUB-431).
func (s *Service) writeBatch(ctx context.Context, tx pgx.Tx, c identity.Caller, st batchState, r runRow, runTwins map[uuid.UUID]uuid.UUID, limit int, out *[]writtenPage) (int, error) {
	if limit <= 0 {
		limit = s.limits.MaxPages + 1
	}
	rows, err := tx.Query(ctx, `SELECT draft_page_id, draft_version FROM release_run_pages
		WHERE run_id = $1 AND outcome = 'waiting' AND action = ANY($2) ORDER BY ord LIMIT $3`,
		r.id, []string{actionMake, actionChange, actionUpdate}, limit)
	if err != nil {
		return 0, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (runPageRow, error) {
		var p runPageRow
		err := row.Scan(&p.draft, &p.version)
		return p, err
	})
	if err != nil {
		return 0, err
	}
	failedRows, err := tx.Query(ctx, `SELECT draft_page_id FROM release_run_pages WHERE run_id = $1 AND reason = ANY($2)`,
		r.id, []string{skipFileMissing, skipBelowFailed})
	if err != nil {
		return 0, err
	}
	failedList, err := pgx.CollectRows(failedRows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, err
	}
	failed := map[uuid.UUID]bool{}
	for _, id := range failedList {
		failed[id] = true
	}
	sources, gate, err := s.readBatch(ctx, tx, r, list)
	if err != nil {
		return 0, err
	}
	silent := r.silent || !st.pair.Settings.NotifyWatchers
	for _, p := range list {
		src := sources[p.draft]
		reason := gate.why(src, st, failed)
		if reason != "" {
			if reason == skipBelowFailed {
				failed[p.draft] = true
			}
			if err := skipPage(ctx, tx, r.id, p.draft, reason); err != nil {
				return 0, err
			}
			continue
		}
		page, err := s.writeRunPage(ctx, tx, c, runPageInput{st: st, r: r, src: src, version: p.version, runTwins: runTwins, silent: silent})
		if errors.Is(err, storage.ErrNotFound) {
			failed[p.draft] = true
			if err := skipPage(ctx, tx, r.id, p.draft, skipFileMissing); err != nil {
				return 0, err
			}
			continue
		}
		if err != nil {
			return 0, err
		}
		if page != nil {
			*out = append(*out, *page)
		}
	}
	if _, err := tx.Exec(ctx, "UPDATE release_runs SET done = done + $2, heartbeat_at = now() WHERE id = $1", r.id, len(list)); err != nil {
		return 0, err
	}
	return len(list), nil
}

// batchGate is what a batch reads once for all its pages: which are hidden from the releaser
// and which are left out, so a page costs no query to be let through (PUB-413, PUB-420).
type batchGate struct {
	viewsDraft bool
	hidden     map[uuid.UUID]bool
	excluded   map[uuid.UUID]bool // left out, the page itself
	branches   map[uuid.UUID]bool // left out with every page below
	// restricted are pages with a view restriction in the draft, which a run of the system
	// leaves to a person (PUB-422).
	restricted map[uuid.UUID]bool
}

// readBatch reads the pages of a batch, and what decides whether each may still go.
func (s *Service) readBatch(ctx context.Context, tx pgx.Tx, r runRow, list []runPageRow) (map[uuid.UUID]pages.ReleaseSource, batchGate, error) {
	gate := batchGate{hidden: map[uuid.UUID]bool{}, excluded: map[uuid.UUID]bool{}, branches: map[uuid.UUID]bool{},
		restricted: map[uuid.UUID]bool{}}
	sources := map[uuid.UUID]pages.ReleaseSource{}
	var ids, around []uuid.UUID
	for _, p := range list {
		src, err := pages.SourceForRelease(ctx, tx, p.draft)
		if apperr.IsNotFound(err) {
			continue // gone: skipped
		}
		if err != nil {
			return nil, gate, err
		}
		sources[p.draft] = src
		ids = append(ids, p.draft)
		around = append(append(around, p.draft), src.Ancestors...)
	}
	if err := s.readHidden(ctx, tx, r, ids, &gate); err != nil {
		return nil, gate, err
	}
	marks, err := tx.Query(ctx, "SELECT draft_page_id, with_children FROM release_exclusions WHERE draft_page_id = ANY($1)", around)
	if err != nil {
		return nil, gate, err
	}
	defer marks.Close()
	for marks.Next() {
		var id uuid.UUID
		var withChildren bool
		if err := marks.Scan(&id, &withChildren); err != nil {
			return nil, gate, err
		}
		gate.excluded[id] = true
		gate.branches[id] = withChildren
	}
	return sources, gate, marks.Err()
}

// readHidden reads which pages of a batch the run may not take. A person's run takes what the
// releaser sees (PUB-413). A run of the system sees the draft space as the release path does,
// and leaves out a page with a view restriction in the draft, which only a person who sees it
// releases by naming it (PUB-422).
func (s *Service) readHidden(ctx context.Context, tx pgx.Tx, r runRow, list []uuid.UUID, gate *batchGate) error {
	var hidden pgx.Rows
	var err error
	if r.system() {
		gate.viewsDraft = true
		hidden, err = tx.Query(ctx, `SELECT DISTINCT t.descendant_id FROM page_tree t
			JOIN page_restrictions pr ON pr.page_id = t.ancestor_id AND pr.kind = 'view' WHERE t.descendant_id = ANY($1)`, list)
		if err != nil {
			return err
		}
		restricted, err := pgx.CollectRows(hidden, pgx.RowTo[uuid.UUID])
		for _, id := range restricted {
			gate.restricted[id] = true
		}
		return err
	}
	d, err := s.perms.CheckSpace(ctx, tx, r.actor, r.draftSpace, permissions.SpaceView)
	if err != nil && !apperr.IsNotFound(err) {
		return err
	}
	gate.viewsDraft = err == nil && d.Allowed
	hidden, err = tx.Query(ctx, `SELECT h.page_id FROM perm_hidden_pages($1) h WHERE h.page_id = ANY($2)
		UNION SELECT ch.page_id FROM perm_control_hidden_pages($1) ch WHERE ch.page_id = ANY($2)`, r.actor, list)
	if err != nil {
		return err
	}
	hiddenList, err := pgx.CollectRows(hidden, pgx.RowTo[uuid.UUID])
	for _, id := range hiddenList {
		gate.hidden[id] = true
	}
	return err
}

// why says why a page of a run is skipped now, or "" when it goes: deleted or moved away since
// the preview, below a page whose file was missing, left out, or hidden from the releaser
// (PUB-431, PUB-1065, PUB-420, PUB-454).
func (g batchGate) why(src pages.ReleaseSource, st batchState, failed map[uuid.UUID]bool) string {
	switch {
	case src.ID == uuid.Nil || src.SpaceID != st.pair.Draft || src.Status != pages.StatusPublished:
		return skipGone
	case slices.ContainsFunc(src.Ancestors, func(a uuid.UUID) bool { return failed[a] }):
		return skipBelowFailed
	case g.excluded[src.ID] || slices.ContainsFunc(src.Ancestors, func(a uuid.UUID) bool { return g.branches[a] }):
		return skipLeftOut
	case !g.viewsDraft || g.hidden[src.ID]:
		return skipHidden
	case g.restricted[src.ID]:
		return skipRestricted
	}
	return ""
}

func skipPage(ctx context.Context, tx pgx.Tx, runID, draft uuid.UUID, reason string) error {
	_, err := tx.Exec(ctx, "UPDATE release_run_pages SET outcome = 'skipped', reason = $3 WHERE run_id = $1 AND draft_page_id = $2", runID, draft, reason)
	return err
}

// runPageInput is one page of a run to write, with the run around it.
type runPageInput struct {
	st       batchState
	r        runRow
	src      pages.ReleaseSource
	version  int
	runTwins map[uuid.UUID]uuid.UUID
	silent   bool
}

// writeRunPage writes one page of a run. A page that copies files writes inside a savepoint,
// so a file that cannot be read undoes that page alone (PUB-1065).
func (s *Service) writeRunPage(ctx context.Context, tx pgx.Tx, c identity.Caller, in runPageInput) (*writtenPage, error) {
	r, src := in.r, in.src
	pl, err := s.planPageWith(ctx, tx, planInput{pair: in.st.pair, src: src, number: in.version, draft: in.st.draft,
		published: in.st.published, runTwins: in.runTwins})
	if err != nil {
		return nil, err
	}
	if pl.action == actionNone {
		_, err := tx.Exec(ctx, "UPDATE release_run_pages SET outcome = 'unchanged' WHERE run_id = $1 AND draft_page_id = $2", r.id, src.ID)
		return nil, err
	}
	q := tx
	if slices.ContainsFunc(pl.files, fileStep.needed) {
		sp, err := tx.Begin(ctx)
		if err != nil {
			return nil, err
		}
		defer sp.Rollback(ctx) //nolint:errcheck // after Commit this does nothing; on an error that error is returned
		q = sp
	}
	w, err := s.writePage(ctx, q, c, &pl, pageWrite{note: r.note, silent: in.silent, runID: r.id, bulk: &r.id})
	if err != nil {
		return nil, err
	}
	v, err := pages.VersionForRelease(ctx, q, w.twin, w.twinVersion)
	if err != nil {
		return nil, err
	}
	twin := w.twin
	if err := s.recordPage(ctx, q, c, pl, pageRecord{runID: r.id, note: r.note, silent: r.silent, outcome: "released",
		twin: &twin, twinVersion: w.twinVersion, twinHash: v.Hash, links: w.res.linkMap, hash: pl.version.Hash, causedBy: r.causedBy}); err != nil {
		return nil, err
	}
	if q != tx {
		if err := q.Commit(ctx); err != nil {
			return nil, err
		}
	}
	return &writtenPage{draft: src.ID, draftParent: src.ParentID, twin: w.twin, twinParent: pl.place.Parent}, nil
}

// finish ends a run: what was removed in the draft is taken off or archived, twins are ordered
// as the draft orders their pages, pages whose links can lead somewhere now are marked, and the
// run is reported and told (PUB-402, PUB-412, PUB-453, PUB-512, PUB-810, PUB-822).
func (s *Service) finish(ctx context.Context, tx pgx.Tx, c identity.Caller, st batchState, r runRow, written []writtenPage) error {
	if err := s.orderWritten(ctx, tx, st, written); err != nil {
		return err
	}
	if err := s.applyRemovals(ctx, tx, c, st, r); err != nil {
		return err
	}
	if err := s.markLinksWaiting(ctx, tx, st, r); err != nil {
		return err
	}
	if err := s.dropUnwritten(ctx, tx, r.id); err != nil {
		return err
	}
	counts, err := runOutcomes(ctx, tx, r.id)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE release_runs SET status = 'done', finished_at = now(), heartbeat_at = now(),
		counts = counts || jsonb_build_object('outcomes', $2::jsonb) WHERE id = $1`, r.id, counts); err != nil {
		return err
	}
	if _, err := audit.Record(ctx, tx, c.Event("release.run", "release_run", r.id.String(), st.pair.Draft.String(),
		r.details(map[string]any{"pair_id": st.pair.ID, "scope": r.scope, "outcomes": counts, "note": r.note, "silent": r.silent}))); err != nil {
		return err
	}
	// The published space learns that a run landed, with numbers only (PUB-901).
	if _, err := audit.Record(ctx, tx, c.Event("space.released", "space", st.published.ID.String(), st.published.ID.String(),
		map[string]any{"run_id": r.id, "outcomes": counts, "note": r.note})); err != nil {
		return err
	}
	finished := map[string]any{"run_id": r.id, "space_id": st.published.ID, "draft_space_id": st.pair.Draft,
		"actor_id": r.actor, "silent": r.silent || !st.pair.Settings.NotifyWatchers, "outcome": "done"}
	if err := addApprovalTelling(ctx, tx, r, finished); err != nil {
		return err
	}
	_, err = outbox.Enqueue(ctx, tx, "release.finished", finished)
	return err
}

// runOutcomes counts the pages of a run by what became of them.
func runOutcomes(ctx context.Context, tx pgx.Tx, runID uuid.UUID) (map[string]int, error) {
	rows, err := tx.Query(ctx, "SELECT outcome, count(*) FROM release_run_pages WHERE run_id = $1 GROUP BY outcome", runID)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for rows.Next() {
		var outcome string
		var n int
		if err := rows.Scan(&outcome, &n); err != nil {
			rows.Close()
			return nil, err
		}
		out[outcome] = n
	}
	rows.Close()
	return out, rows.Err()
}

// applyRemovals takes off the twins a run takes off, and archives those whose draft page was
// archived (PUB-440, PUB-1010 to PUB-1014). Their page pairs end; a twin goes to the trash of
// the published space and is never purged by the run (PUB-441).
func (s *Service) applyRemovals(ctx context.Context, tx pgx.Tx, c identity.Caller, st batchState, r runRow) error {
	rows, err := tx.Query(ctx, `SELECT rp.draft_page_id, rp.twin_page_id, rp.action FROM release_run_pages rp
		JOIN pages t ON t.id = rp.twin_page_id AND t.space_id = $2 AND t.status <> 'trashed'
		WHERE rp.run_id = $1 AND rp.outcome = 'waiting' AND rp.action = ANY($3) ORDER BY rp.ord`,
		r.id, st.published.ID, []string{actionTakeOff, actionArchive})
	if err != nil {
		return err
	}
	type removal struct {
		draft, twin uuid.UUID
		action      string
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (removal, error) {
		var m removal
		err := row.Scan(&m.draft, &m.twin, &m.action)
		return m, err
	})
	if err != nil {
		return err
	}
	var off []uuid.UUID
	for _, m := range list {
		outcome := "archived"
		if m.action == actionTakeOff {
			off, outcome = append(off, m.twin), "taken_off"
		} else if err := s.pages.SetTwinArchived(ctx, tx, c, m.twin, true); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE release_run_pages SET outcome = $3 WHERE run_id = $1 AND draft_page_id = $2", r.id, m.draft, outcome); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("release."+outcome, "page", m.draft.String(), st.pair.Draft.String(),
			r.details(map[string]any{"pair_id": st.pair.ID, "run_id": r.id, "twin_page_id": m.twin}))); err != nil {
			return err
		}
	}
	if err := s.pages.TakeOffTwins(ctx, tx, c, st.published.ID, off); err != nil {
		return err
	}
	// The page pairs end: releasing a page again makes a new twin (PUB-442).
	if _, err := tx.Exec(ctx, "DELETE FROM page_pairs WHERE pair_id = $1 AND twin_page_id = ANY($2)", st.pair.ID, off); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "DELETE FROM release_removals WHERE twin_page_id = ANY($1)", off)
	return err
}

// markLinksWaiting marks the pages whose links stayed text because their target had no twin,
// when this run gave that target one: they are Changed since release, with links waiting, and
// their twins stay as they are until someone releases them (PUB-306, PUB-512).
func (s *Service) markLinksWaiting(ctx context.Context, tx pgx.Tx, st batchState, r runRow) error {
	_, err := tx.Exec(ctx, `SELECT page_pair_refresh(pp.draft_page_id) FROM page_pairs pp
		WHERE pp.pair_id = $1 AND pp.text_links && ARRAY(SELECT rp.draft_page_id FROM release_run_pages rp
			WHERE rp.run_id = $2 AND rp.outcome = 'released' AND rp.action = $3)`, st.pair.ID, r.id, actionMake)
	return err
}

// dropUnwritten removes the twins a run made for pages it did not write in the end.
func (s *Service) dropUnwritten(ctx context.Context, tx pgx.Tx, runID uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT twin_page_id FROM release_run_pages WHERE run_id = $1 AND action = $2 AND twin_page_id IS NOT NULL
		AND outcome <> 'released'`, runID, actionMake)
	if err != nil {
		return err
	}
	list, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	return s.pages.DropUnreleasedTwins(ctx, tx, list)
}

// orderWritten puts each twin a run wrote after the twin of the sibling before its draft page,
// so the children of a parent are in the order of the draft, and siblings not released keep
// their own order after them (PUB-412). Every page of the group is then compared again: the
// order among siblings, and for a page written, whether the draft changed since the preview
// (PUB-431).
func (s *Service) orderWritten(ctx context.Context, tx pgx.Tx, st batchState, written []writtenPage) error {
	type group struct {
		draftParent, twinParent *uuid.UUID
		twins                   map[uuid.UUID]bool
	}
	groups := map[string]*group{}
	var keys []string
	for _, w := range written {
		key := "top"
		if w.draftParent != nil {
			key = w.draftParent.String()
		}
		g, ok := groups[key]
		if !ok {
			g = &group{draftParent: w.draftParent, twinParent: w.twinParent, twins: map[uuid.UUID]bool{}}
			groups[key] = g
			keys = append(keys, key)
		}
		if sameID(w.twinParent, g.twinParent) {
			g.twins[w.twin] = true
		}
	}
	for _, key := range keys {
		g := groups[key]
		draftKids, err := pages.ChildrenInOrder(ctx, tx, st.pair.Draft, g.draftParent)
		if err != nil {
			return err
		}
		twinOf, err := twinsOf(ctx, tx, st.pair.ID, draftKids)
		if err != nil {
			return err
		}
		current, err := pages.ChildrenInOrder(ctx, tx, st.published.ID, g.twinParent)
		if err != nil {
			return err
		}
		var inDraftOrder []uuid.UUID
		for _, kid := range draftKids {
			if t, ok := twinOf[kid]; ok && slices.Contains(current, t) {
				inDraftOrder = append(inDraftOrder, t)
			}
		}
		order := placeInOrder(current, inDraftOrder, g.twins)
		if err := s.pages.OrderTwins(ctx, tx, st.published.ID, g.twinParent, order); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT page_pair_refresh(d) FROM unnest($1::uuid[]) AS d", draftKids); err != nil {
			return err
		}
	}
	return nil
}

// placeInOrder moves each twin written, in the order of the draft, right after the twin of the
// sibling before it in the draft, or to the start when none is before it; the others keep
// their places.
func placeInOrder(current, inDraftOrder []uuid.UUID, written map[uuid.UUID]bool) []uuid.UUID {
	order := slices.Clone(current)
	for i, t := range inDraftOrder {
		if !written[t] {
			continue
		}
		order = slices.DeleteFunc(order, func(id uuid.UUID) bool { return id == t })
		at := 0
		if i > 0 {
			at = slices.Index(order, inDraftOrder[i-1]) + 1
		}
		order = slices.Insert(order, at, t)
	}
	return order
}

func sameID(a, b *uuid.UUID) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
