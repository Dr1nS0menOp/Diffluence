// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/workflow"
)

// Due dates, reminders and escalation (spec 064 wave DC5, DOC-230 to DOC-232, DOC-601).
// Every moment is the database's (DOC-1240): the due date is counted there from now() of the
// transaction that starts the stage, in the instance's zone and working week, and stored.
// The job claims a deadline by its stored moment and moves that moment on in the transaction
// that acts, so a clock that goes back never acts twice (DOC-1241), and two workers never
// take the same one (DOC-1232).

const (
	// maxDuePass is how many deadlines one pass reads (DOC-1304).
	maxDuePass = 500
	// maxDuePasses bounds one run of the job, so it always ends; the next run goes on.
	maxDuePasses = 40
	// causeOverdue marks the system's step when a due date passed (DOC-105).
	causeOverdue = "overdue"
	// viaEscalated marks a reviewer the system added when time ran out or the round could
	// no longer pass (DOC-231, DOC-242).
	viaEscalated = "escalated"
)

// systemWorkflow is the caller of what the workflow does by itself.
var systemWorkflow = identity.Caller{System: "workflow"}

// workingTime is the zone and working week the due dates of new stages are counted in.
func (s *Service) workingTime(ctx context.Context, q db.Querier) (string, []int, error) {
	g := settings.DefaultGeneral
	if err := s.settings.ReadSectionIn(ctx, q, settings.SectionGeneral, &g); err != nil {
		return "", nil, err
	}
	return g.Zone(), g.Week(), nil
}

// startDeadline sets the due date of a stage that just began, when its state has a due
// period. A later change of the working week leaves it as it is (DOC-1242).
func (s *Service) startDeadline(ctx context.Context, tx pgx.Tx, pageID, round uuid.UUID, stage int, due *workflow.Due) error {
	if _, err := tx.Exec(ctx, "UPDATE review_deadlines SET next_at = NULL WHERE round_id = $1 AND stage < $2 AND next_at IS NOT NULL", round, stage); err != nil {
		return err
	}
	if due == nil {
		return nil
	}
	zone, week, err := s.workingTime(ctx, tx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO review_deadlines (round_id, stage, page_id, started_at, remind_at, due_at, next_at)
		SELECT $1, $2, $3, now(), x.remind, x.due, COALESCE(x.remind, x.due)
		FROM (SELECT CASE WHEN $5::int > 0 THEN control_working_days_after(now(), $4 - $5, $6, $7) END AS remind,
		             control_working_days_after(now(), $4, $6, $7) AS due) x
		ON CONFLICT DO NOTHING`, round, stage, pageID, due.WorkingDays, due.RemindBefore, zone, week)
	return err
}

// closeDeadlines ends the deadlines of a page's open round: nothing is reminded once a round
// ends (DOC-601).
func closeDeadlines(ctx context.Context, tx pgx.Tx, pageID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE review_deadlines SET next_at = NULL WHERE page_id = $1 AND next_at IS NOT NULL
		AND round_id IN (SELECT id FROM workflow_rounds WHERE page_id = $1 AND status = 'open')`, pageID)
	return err
}

// RunControlDue is the periodic job of document control, one per worker (DOC-1304): people
// who left are dropped from their rounds first, so their rounds escalate; then due dates,
// review dates and the people who joined a group that confirms reading.
func (s *Service) RunControlDue(ctx context.Context) error {
	steps := []func(context.Context) (int, error){s.dropLeavers, s.closeDeadDeadlines, s.dueDeadlinesPass, s.periodicPass, s.askLateReaders}
	// A step that fails stops its own passes, never the steps after it; the failures come
	// back together, so the job records them.
	var failed []error
	for _, step := range steps {
		for range maxDuePasses {
			n, err := step(ctx)
			if err != nil {
				failed = append(failed, err)
				break
			}
			if n < maxDuePass {
				break
			}
		}
	}
	return errors.Join(failed...)
}

// closeDeadDeadlines ends deadlines whose stage is over, in bulk.
func (s *Service) closeDeadDeadlines(ctx context.Context) (int, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE review_deadlines SET next_at = NULL WHERE (round_id, stage) IN (
		SELECT d.round_id, d.stage FROM review_deadlines d JOIN workflow_rounds r ON r.id = d.round_id
		WHERE d.next_at <= now() AND NOT (r.status = 'open' AND r.review_stage = d.stage)
		LIMIT $1 FOR UPDATE OF d SKIP LOCKED)`, maxDuePass)
	return int(tag.RowsAffected()), err
}

type deadline struct {
	round  uuid.UUID
	stage  int
	page   uuid.UUID
	due    bool
	passed bool
	// escalated says the date already added the escalation targets once.
	escalated bool
}

// dueDeadlinesPass acts on up to maxDuePass deadlines whose moment came, each in its own
// short transaction that locks the page first, as a publish does.
func (s *Service) dueDeadlinesPass(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `SELECT round_id, stage, page_id FROM review_deadlines WHERE next_at <= now() ORDER BY next_at LIMIT $1`, maxDuePass)
	if err != nil {
		return 0, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (deadline, error) {
		var d deadline
		return d, r.Scan(&d.round, &d.stage, &d.page)
	})
	if err != nil {
		return 0, err
	}
	// Each deadline is its own transaction, so one that fails holds up no other; the job
	// reports every failure at the end.
	var failed []error
	for _, d := range list {
		if err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return s.actOnDeadline(ctx, tx, d) }); err != nil {
			failed = append(failed, fmt.Errorf("pages: deadline of page %s: %w", d.page, err))
		}
	}
	return len(list), errors.Join(failed...)
}

// claimPage locks a page for the job, or answers false when someone else holds it now; the
// next pass takes it then.
func claimPage(ctx context.Context, tx pgx.Tx, id uuid.UUID) (bool, error) {
	var got uuid.UUID
	err := tx.QueryRow(ctx, "SELECT id FROM pages WHERE id = $1 FOR NO KEY UPDATE SKIP LOCKED", id).Scan(&got)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// actOnDeadline does what one deadline asks now: the reminder before the date, or what the
// state says when the date passed.
func (s *Service) actOnDeadline(ctx context.Context, tx pgx.Tx, d deadline) error {
	claimed, err := claimPage(ctx, tx, d.page)
	if err != nil || !claimed {
		return err
	}
	err = tx.QueryRow(ctx, `SELECT due_at <= now(), passed_at IS NOT NULL, escalated_at IS NOT NULL FROM review_deadlines
		WHERE round_id = $1 AND stage = $2 AND next_at <= now() FOR UPDATE SKIP LOCKED`, d.round, d.stage).Scan(&d.due, &d.passed, &d.escalated)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	rc, err := s.lockReview(ctx, tx, d.page)
	var conflict *apperr.Error
	if errors.As(err, &conflict) {
		return endDeadline(ctx, tx, d)
	}
	if err != nil {
		return err
	}
	if rc.stage.round != d.round || rc.stage.number != d.stage || rc.state.Review.Due == nil {
		return endDeadline(ctx, tx, d)
	}
	switch {
	case !d.due:
		return s.remindAndMoveOn(ctx, tx, rc, d, "due_at")
	case d.passed:
		return s.remindAndMoveOn(ctx, tx, rc, d, "next_day")
	}
	if late, ok := rc.def.Outcome(rc.ctl.stateID, workflow.OutcomeOverdue); ok {
		return s.timeOut(ctx, tx, rc, d, late)
	}
	if _, err := tx.Exec(ctx, "UPDATE review_deadlines SET passed_at = now() WHERE round_id = $1 AND stage = $2", d.round, d.stage); err != nil {
		return err
	}
	if !d.escalated && len(rc.state.Escalation()) > 0 {
		if _, err := s.escalate(ctx, tx, rc, causeOverdue); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE review_deadlines SET escalated_at = now() WHERE round_id = $1 AND stage = $2", d.round, d.stage); err != nil {
			return err
		}
	}
	return s.remindAndMoveOn(ctx, tx, rc, d, "next_day")
}

func endDeadline(ctx context.Context, tx pgx.Tx, d deadline) error {
	_, err := tx.Exec(ctx, "UPDATE review_deadlines SET next_at = NULL WHERE round_id = $1 AND stage = $2", d.round, d.stage)
	return err
}

// remindAndMoveOn reminds the reviewers who have not answered and sets the next moment: the
// due date after the reminder before it, or the next working day once it passed.
func (s *Service) remindAndMoveOn(ctx context.Context, tx pgx.Tx, rc reviewContext, d deadline, next string) error {
	zone, week, err := s.workingTime(ctx, tx)
	if err != nil {
		return err
	}
	// The next moment is the due date when this was the reminder before it, else the next
	// working day; both are of the database's clock.
	if _, err := tx.Exec(ctx, `UPDATE review_deadlines SET reminders = reminders + 1,
			next_at = CASE WHEN $3 THEN due_at ELSE control_working_days_after(now(), 1, $4, $5) END
		WHERE round_id = $1 AND stage = $2`, d.round, d.stage, next == "due_at", zone, week); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT wr.user_id FROM workflow_reviewers wr JOIN users u ON u.id = wr.user_id AND u.type = 'member' AND u.status = 'active'
		WHERE wr.round_id = $1 AND wr.stage = $2 AND wr.removed_at IS NULL AND wr.answer IS NULL`, d.round, d.stage)
	if err != nil {
		return err
	}
	people, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	return recordEvent(ctx, tx, systemWorkflow, "page.review_reminded", rc.page,
		map[string]any{"round": d.round, "stage": d.stage, "due": d.due, "people": len(people)}, map[string]any{"user_ids": people})
}

// timeOut takes the state's step for a passed due date. It never approves: takeOutcome refuses
// an approved target for anything but a passed round (DOC-105, DOC-1401), and a definition
// that holds one anyway is left in review with the refusal on record.
func (s *Service) timeOut(ctx context.Context, tx pgx.Tx, rc reviewContext, d deadline, late workflow.Transition) error {
	if _, err := tx.Exec(ctx, "UPDATE review_deadlines SET passed_at = now(), next_at = NULL WHERE round_id = $1 AND stage = $2", d.round, d.stage); err != nil {
		return err
	}
	_, err := s.takeOutcome(ctx, tx, systemWorkflow, rc.page, rc.ctl, rc.def, rc.stage, workflow.OutcomeOverdue)
	if !errors.Is(err, errTimeOutApproves) {
		return err
	}
	zone, week, werr := s.workingTime(ctx, tx)
	if werr != nil {
		return werr
	}
	if _, err := tx.Exec(ctx, `UPDATE review_deadlines SET passed_at = NULL, next_at = control_working_days_after(now(), 1, $3, $4)
		WHERE round_id = $1 AND stage = $2`, d.round, d.stage, zone, week); err != nil {
		return err
	}
	return recordAudit(ctx, tx, systemWorkflow, "page.review_overdue_refused", "page", rc.page.ID, rc.page.SpaceID,
		map[string]any{"transition": late.ID, "reason": "a time-out never approves"})
}

// errTimeOutApproves is the refusal of a step into approval that no passed round took.
var errTimeOutApproves = errors.New("pages: only a passed round approves")

// escalate asks the escalation targets of the state in the current stage (DOC-231, DOC-242):
// resolved now, never an author under independence, never someone this stage asked already,
// so escalating twice adds nobody twice. It answers who was added.
func (s *Service) escalate(ctx context.Context, tx pgx.Tx, rc reviewContext, cause string) ([]uuid.UUID, error) {
	targets := rc.state.Escalation()
	if len(targets) == 0 {
		return nil, nil
	}
	found, _, err := resolveEntries(ctx, tx, rc.page.SpaceID, targets)
	if err != nil {
		return nil, err
	}
	var authors []uuid.UUID
	if !rc.state.Review.AuthorsMayReview {
		if authors, err = changeAuthors(ctx, tx, rc.page.ID, rc.ctl); err != nil {
			return nil, err
		}
	}
	rows, err := tx.Query(ctx, "SELECT user_id FROM workflow_reviewers WHERE round_id = $1 AND stage = $2", rc.stage.round, rc.stage.number)
	if err != nil {
		return nil, err
	}
	present, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	asked, _, _, err := tally(ctx, tx, rc.stage)
	if err != nil {
		return nil, err
	}
	var added []uuid.UUID
	for _, c := range found {
		if slices.Contains(present, c.user) || slices.Contains(authors, c.user) || slices.Contains(added, c.user) {
			continue
		}
		if asked+len(added) >= rc.state.Review.MaxResolved() {
			break
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_reviewers (round_id, stage, state_id, user_id, source_type, source_id, via, via_reason)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, rc.stage.round, rc.stage.number, rc.state.ID, c.user, c.sourceType, c.sourceID,
			viaEscalated, cause); err != nil {
			return nil, err
		}
		added = append(added, c.user)
	}
	if err := recordAudit(ctx, tx, systemWorkflow, "page.review_escalated", "page", rc.page.ID, rc.page.SpaceID,
		map[string]any{"round": rc.stage.round, "stage": rc.stage.number, "cause": cause, "user_ids": added}); err != nil {
		return nil, err
	}
	if len(added) == 0 {
		return nil, nil
	}
	return added, s.askReviewers(ctx, tx, systemWorkflow, rc.page, rc.state, added)
}

// cannotPass says whether the stage can no longer pass with the people still asked.
func cannotPass(ctx context.Context, tx pgx.Tx, rc reviewContext) (bool, error) {
	remaining, _, _, err := tally(ctx, tx, rc.stage)
	if err != nil {
		return false, err
	}
	return remaining == 0 || rc.state.Review.Needed(remaining) > remaining, nil
}

// dropLeavers takes people who were disabled or left the instance off the open rounds that
// asked them, with an audit entry each; a round that can no longer pass escalates, and one
// that now passes takes its step (DOC-242, DOC-1220).
func (s *Service) dropLeavers(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT r.page_id FROM workflow_reviewers wr
		JOIN workflow_rounds r ON r.id = wr.round_id AND r.status = 'open' AND r.review_stage = wr.stage
		LEFT JOIN users u ON u.id = wr.user_id
		WHERE wr.removed_at IS NULL AND (u.id IS NULL OR u.status <> 'active' OR u.type <> 'member') LIMIT $1`, maxDuePass)
	if err != nil {
		return 0, err
	}
	pageIDs, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, err
	}
	var failed []error
	for _, id := range pageIDs {
		if err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return s.dropLeaversOf(ctx, tx, id) }); err != nil {
			failed = append(failed, fmt.Errorf("pages: reviewers who left page %s: %w", id, err))
		}
	}
	return len(pageIDs), errors.Join(failed...)
}

func (s *Service) dropLeaversOf(ctx context.Context, tx pgx.Tx, pageID uuid.UUID) error {
	claimed, err := claimPage(ctx, tx, pageID)
	if err != nil || !claimed {
		return err
	}
	rc, err := s.lockReview(ctx, tx, pageID)
	var conflict *apperr.Error
	if errors.As(err, &conflict) {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `UPDATE workflow_reviewers wr SET removed_at = now(), removed_reason = 'left'
		WHERE wr.round_id = $1 AND wr.stage = $2 AND wr.removed_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM users u WHERE u.id = wr.user_id AND u.status = 'active' AND u.type = 'member')
		RETURNING wr.user_id`, rc.stage.round, rc.stage.number)
	if err != nil {
		return err
	}
	left, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil || len(left) == 0 {
		return err
	}
	for _, user := range left {
		if err := recordAudit(ctx, tx, systemWorkflow, "page.reviewer_dropped", "page", pageID, rc.page.SpaceID,
			map[string]any{"user_id": user, "round": rc.stage.round, "stage": rc.stage.number, "reason": "left"}); err != nil {
			return err
		}
	}
	return s.afterLosingReviewers(ctx, tx, systemWorkflow, rc, "left")
}

// afterLosingReviewers follows a stage that lost people: it escalates when the round can no
// longer pass, and takes the step when the answers now decide it.
func (s *Service) afterLosingReviewers(ctx context.Context, tx pgx.Tx, c identity.Caller, rc reviewContext, cause string) error {
	impossible, err := cannotPass(ctx, tx, rc)
	if err != nil {
		return err
	}
	if impossible {
		if _, err := s.escalate(ctx, tx, rc, cause); err != nil {
			return err
		}
	}
	_, _, err = s.settleReview(ctx, tx, c, rc.page, rc.ctl, rc.stage)
	return err
}

// Deadline is the due date of the stage a page is in, for the band and the review panel.
type Deadline struct {
	DueAt     string `json:"due_at"`
	Overdue   bool   `json:"overdue"`
	Escalated bool   `json:"escalated"`
}

// deadlineOf reads the due date of a page's open stage, or nil.
func deadlineOf(ctx context.Context, q db.Querier, pageID uuid.UUID) (*Deadline, error) {
	var d Deadline
	err := q.QueryRow(ctx, `SELECT to_char(d.due_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), d.due_at <= now(), d.escalated_at IS NOT NULL
		FROM review_deadlines d JOIN workflow_rounds r ON r.id = d.round_id AND r.status = 'open' AND r.review_stage = d.stage
		WHERE r.page_id = $1`, pageID).Scan(&d.DueAt, &d.Overdue, &d.Escalated)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &d, err
}
