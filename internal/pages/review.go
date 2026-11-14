// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/workflow"
)

// Reviewers of document control (spec 064, wave DC2). When a page enters a state of kind in
// review that names reviewers, the round gains a stage: its people, groups and roles are
// resolved once and stored, and only their answers move the page on. The system takes the
// step that follows, in the transaction of the answer that decides it (DOC-213).

// Answers a reviewer gives.
const (
	ReviewApprove = "approve"
	ReviewChanges = "changes"
)

// Why the system moved a page out of a review state (DOC-105).
const (
	causeReviewPassed = "review_passed"
	causeChangesAsked = "changes_asked"
	// sourceAdded marks a reviewer added to a running round (DOC-202).
	sourceAdded = "added"
)

const (
	maxReviewComment = 4000
	maxRemovalReason = 500
)

// stage is the review a round is in now: its number and the state it asks for.
type stage struct {
	round  uuid.UUID
	number int
}

// openStage reads and locks the open round of a page, with its current stage.
func openStage(ctx context.Context, tx pgx.Tx, pageID uuid.UUID) (stage, bool, error) {
	var st stage
	err := tx.QueryRow(ctx, "SELECT id, review_stage FROM workflow_rounds WHERE page_id = $1 AND status = 'open' FOR UPDATE", pageID).
		Scan(&st.round, &st.number)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, false, nil
	}
	return st, err == nil, err
}

// candidate is one person an entry of a review state names.
type candidate struct {
	entry      int
	sourceType string
	sourceID   uuid.UUID
	user       uuid.UUID
}

// resolveEntries turns the people, groups and roles of a review state into members who may
// view the space. A guest, someone disabled or someone without the space is never asked.
// It answers with the people and, for each entry that names nobody, its name (DOC-200).
func resolveEntries(ctx context.Context, q db.Querier, spaceID uuid.UUID, entries []workflow.Reviewer) ([]candidate, []string, error) {
	raw, err := json.Marshal(entries)
	if err != nil {
		return nil, nil, err
	}
	rows, err := q.Query(ctx, `SELECT e.ord, COALESCE(un.display_name, g.name, ro.name, ''), m.id
		FROM ROWS FROM (jsonb_to_recordset($2::jsonb) AS (type text, id uuid)) WITH ORDINALITY AS e(type, id, ord)
		LEFT JOIN users un ON e.type = 'user' AND un.id = e.id
		LEFT JOIN groups g ON e.type = 'group' AND g.id = e.id
		LEFT JOIN roles ro ON e.type = 'role' AND ro.id = e.id
		LEFT JOIN LATERAL (
			SELECT e.id AS user_id WHERE e.type = 'user'
			UNION SELECT gm.user_id FROM group_members gm WHERE e.type = 'group' AND gm.group_id = e.id
			UNION SELECT ra.subject_id FROM role_assignments ra
			      WHERE e.type = 'role' AND ra.role_id = e.id AND ra.space_id = $1 AND ra.subject_type = 'user'
			UNION SELECT gm.user_id FROM role_assignments ra JOIN group_members gm ON gm.group_id = ra.subject_id
			      WHERE e.type = 'role' AND ra.role_id = e.id AND ra.space_id = $1 AND ra.subject_type = 'group'
		) cand ON true
		LEFT JOIN users m ON m.id = cand.user_id AND m.type = 'member'
			AND $1 IN (SELECT ps.space_id FROM perm_spaces(m.id, 'space.view') ps)
		ORDER BY e.ord, lower(m.display_name), m.id`, spaceID, raw)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	names := make([]string, len(entries))
	found := make([]bool, len(entries))
	var out []candidate
	for rows.Next() {
		var ord int
		var name string
		var user *uuid.UUID
		if err := rows.Scan(&ord, &name, &user); err != nil {
			return nil, nil, err
		}
		i := ord - 1
		names[i] = name
		if user == nil {
			continue
		}
		found[i] = true
		out = append(out, candidate{entry: i, sourceType: entries[i].Type, sourceID: entries[i].ID, user: *user})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	var empty []string
	for i := range entries {
		if !found[i] {
			empty = append(empty, names[i])
		}
	}
	return out, empty, nil
}

// changeAuthors are everyone who published a version since the last approved one: under
// independence none of them counts as a reviewer, so two authors cannot approve each
// other's part of one change (DOC-220).
func changeAuthors(ctx context.Context, q db.Querier, pageID uuid.UUID, ctl control) ([]uuid.UUID, error) {
	since := 0
	if ctl.approvedVersion != nil {
		since = *ctl.approvedVersion
	}
	rows, err := q.Query(ctx, `SELECT DISTINCT author_id FROM page_versions
		WHERE page_id = $1 AND number > $2 AND number <= $3 AND author_id IS NOT NULL`, pageID, since, ctl.headVersion)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// enterReview starts a stage of the round for a review state: the reviewers are resolved
// now and stored, so a later change of a group or role leaves the round as it is
// (DOC-201, DOC-1221, DOC-1222). A stage that would ask nobody, too many, or fewer than
// its rule needs is refused, and so is the step that led to it.
func (s *Service) enterReview(ctx context.Context, tx pgx.Tx, p Page, ctl control, round uuid.UUID, st workflow.State) ([]uuid.UUID, error) {
	rv := st.Review
	if len(rv.Reviewers) == 0 {
		return nil, apperr.Conflict("no_reviewers", fmt.Sprintf("The state %q names nobody to review. Whoever looks after the workflows names its reviewers first.", st.Name))
	}
	found, empty, err := resolveEntries(ctx, tx, p.SpaceID, rv.Reviewers)
	if err != nil {
		return nil, err
	}
	if len(empty) > 0 {
		return nil, apperr.Conflict("reviewers_empty", fmt.Sprintf("Nobody who may review is in %s, which the state %q names. Ask whoever looks after the workflows.",
			strings.Join(quoted(empty), ", "), st.Name))
	}
	var authors []uuid.UUID
	if !rv.AuthorsMayReview {
		if authors, err = changeAuthors(ctx, tx, p.ID, ctl); err != nil {
			return nil, err
		}
	}
	var asked []candidate
	seen := map[uuid.UUID]bool{}
	for _, c := range found {
		if seen[c.user] || slices.Contains(authors, c.user) {
			continue
		}
		seen[c.user] = true
		asked = append(asked, c)
	}
	switch {
	case len(asked) == 0:
		return nil, apperr.Conflict("no_independent_reviewer", fmt.Sprintf("Everyone %q names wrote part of this change, and the review must be independent. Ask for another reviewer.", st.Name))
	case len(asked) > rv.MaxResolved():
		return nil, apperr.Conflict("too_many_reviewers", fmt.Sprintf("The state %q would ask %d people; a round asks at most %d.", st.Name, len(asked), rv.MaxResolved()))
	case rv.Needed(len(asked)) > len(asked):
		return nil, apperr.Conflict("quorum_unreachable", fmt.Sprintf("The state %q needs %d approvals, but only %d people can review.", st.Name, rv.Needed(len(asked)), len(asked)))
	}
	var number int
	if err := tx.QueryRow(ctx, "UPDATE workflow_rounds SET review_stage = review_stage + 1 WHERE id = $1 RETURNING review_stage", round).Scan(&number); err != nil {
		return nil, err
	}
	people := make([]uuid.UUID, len(asked))
	types := make([]string, len(asked))
	sources := make([]uuid.UUID, len(asked))
	for i, c := range asked {
		people[i], types[i], sources[i] = c.user, c.sourceType, c.sourceID
	}
	if _, err := tx.Exec(ctx, `INSERT INTO workflow_reviewers (round_id, stage, state_id, user_id, source_type, source_id)
		SELECT $1, $2, $3, u, t, src FROM unnest($4::uuid[], $5::text[], $6::uuid[]) AS x(u, t, src)`,
		round, number, st.ID, people, types, sources); err != nil {
		return nil, err
	}
	return people, nil
}

func quoted(list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}

// tally counts the stage: who is still asked, how many approved, and whether anyone asked
// for changes. A "changes" answer counts even after its reviewer was taken off (DOC-202).
func tally(ctx context.Context, tx pgx.Tx, st stage) (asked, approved, changes int, err error) {
	err = tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE removed_at IS NULL),
			count(*) FILTER (WHERE removed_at IS NULL AND answer = 'approve'),
			count(*) FILTER (WHERE answer = 'changes')
		FROM workflow_reviewers WHERE round_id = $1 AND stage = $2`, st.round, st.number).Scan(&asked, &approved, &changes)
	return asked, approved, changes, err
}

// settleReview takes the step that follows the review when the answers decide it, in the same
// transaction (DOC-211, DOC-213). It answers with the outcome.
func (s *Service) settleReview(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, ctl control, st stage) (workflow.Outcome, Page, error) {
	def, err := definitionOf(ctx, tx, ctl.workflowID, ctl.workflowVersion)
	if err != nil {
		return "", p, err
	}
	state, ok := def.State(ctl.stateID)
	if !ok || state.Review == nil {
		return workflow.OutcomePending, p, nil
	}
	asked, approved, changes, err := tally(ctx, tx, st)
	if err != nil {
		return "", p, err
	}
	outcome := state.Review.Count(asked, approved, changes)
	if outcome == workflow.OutcomePending {
		return outcome, p, nil
	}
	out, err := s.takeOutcome(ctx, tx, c, p, ctl, def, st, outcome)
	return outcome, out, err
}

// takeOutcome is the system's step out of a review state. Approval only ever follows a
// passed round, never a person's own step (DOC-105, DOC-1401).
func (s *Service) takeOutcome(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, ctl control, def workflow.Definition, st stage, outcome workflow.Outcome) (Page, error) {
	tr, ok := def.Outcome(ctl.stateID, outcome)
	if !ok {
		return p, fmt.Errorf("pages: workflow %s version %d has no %s step from %s", ctl.workflowID, ctl.workflowVersion, outcome, ctl.stateID)
	}
	to, ok := def.State(tr.To)
	if !ok {
		return p, fmt.Errorf("pages: workflow %s version %d has no state %s", ctl.workflowID, ctl.workflowVersion, tr.To)
	}
	cause := causeReviewPassed
	if outcome == workflow.OutcomeChanges {
		cause = causeChangesAsked
	}
	if _, err := tx.Exec(ctx, `INSERT INTO workflow_answers (id, round_id, page_id, user_id, transition_id, from_state, to_state, version_number, cause)
		VALUES ($1, $2, $3, NULL, $4, $5, $6, $7, $8)`, ids.New(), st.round, p.ID, tr.ID, ctl.stateID, to.ID, ctl.headVersion, cause); err != nil {
		return p, err
	}
	if _, err := tx.Exec(ctx, "UPDATE page_control SET state_id = $2, state_kind = $3, updated_at = now() WHERE page_id = $1", p.ID, to.ID, to.Kind); err != nil {
		return p, err
	}
	authors, err := changeAuthors(ctx, tx, p.ID, ctl)
	if err != nil {
		return p, err
	}
	details := map[string]any{"transition": tr.ID, "from": ctl.stateID, "to": to.ID, "version": ctl.headVersion, "cause": cause}
	if err := recordSystem(ctx, tx, c, "page.review_"+string(outcome), p, details); err != nil {
		return p, err
	}
	system := identity.Caller{System: "workflow", Meta: c.Meta}
	ended := map[string]any{"outcome": string(outcome), "user_ids": authors, "actor_id": c.UserID}
	switch {
	case to.Kind == workflow.KindApproved:
		if err := endRound(ctx, tx, p.ID, roundStatusApproved, cause); err != nil {
			return p, err
		}
		out, err := s.approve(ctx, tx, c, p, ctl, to)
		if err != nil {
			return p, err
		}
		if err := recordEvent(ctx, tx, system, "page.approved", out, map[string]any{"transition": tr.ID, "version": ctl.headVersion, "caused_by": c.UserID}, nil); err != nil {
			return p, err
		}
		return out, recordEvent(ctx, tx, system, "page.review_ended", out, map[string]any{"outcome": string(outcome)}, ended)
	case to.Review != nil:
		ctl.stateID, ctl.stateKind = to.ID, to.Kind
		asked, err := s.enterReview(ctx, tx, p, ctl, st.round, to)
		if err != nil {
			return p, err
		}
		return p, s.askReviewers(ctx, tx, system, p, to, asked)
	case outcome == workflow.OutcomeChanges:
		if err := endRound(ctx, tx, p.ID, roundStatusEnded, cause); err != nil {
			return p, err
		}
	}
	return p, recordEvent(ctx, tx, system, "page.review_ended", p, map[string]any{"outcome": string(outcome)}, ended)
}

// askReviewers tells the people a stage asks (DOC-600). The event holds ids and the rule;
// the notification reads the rest when it is shown, through the reviewer's own access.
func (s *Service) askReviewers(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, st workflow.State, people []uuid.UUID) error {
	return recordEvent(ctx, tx, c, "page.review_asked", p, map[string]any{"state": st.ID, "people": len(people)},
		map[string]any{"user_ids": people, "rule": string(st.Review.Rule), "needed": st.Review.Needed(len(people))})
}

// reviewContext is what every reviewer action reads first, with the page and its control row
// locked in the order of a publish (DOC-1231).
type reviewContext struct {
	page  Page
	ctl   control
	def   workflow.Definition
	state workflow.State
	stage stage
}

// lockReview locks a controlled page in a review state with reviewers, and its open round.
func (s *Service) lockReview(ctx context.Context, tx pgx.Tx, id uuid.UUID) (reviewContext, error) {
	var rc reviewContext
	p, err := lockPage(ctx, tx, id)
	if err != nil {
		return rc, err
	}
	ctl, controlled, err := lockControl(ctx, tx, id)
	if err != nil {
		return rc, err
	}
	if !controlled {
		return rc, apperr.Conflict("not_controlled", "This page follows no workflow.")
	}
	def, err := definitionOf(ctx, tx, ctl.workflowID, ctl.workflowVersion)
	if err != nil {
		return rc, err
	}
	state, ok := def.State(ctl.stateID)
	st, open, err := openStage(ctx, tx, id)
	if err != nil {
		return rc, err
	}
	if !ok || state.Review == nil || !open || st.number == 0 {
		return rc, apperr.Conflict("not_in_review", "This page is not waiting for review.")
	}
	return reviewContext{page: p, ctl: ctl, def: def, state: state, stage: st}, nil
}

// myReview says what a person was asked in the current stage: whether they are a reviewer
// there, and their answer so far.
func myReview(ctx context.Context, q db.Querier, st stage, user uuid.UUID) (bool, string, error) {
	var answer *string
	err := q.QueryRow(ctx, `SELECT wr.answer FROM workflow_reviewers wr JOIN users u ON u.id = wr.user_id AND u.type = 'member'
		WHERE wr.round_id = $1 AND wr.stage = $2 AND wr.user_id = $3 AND wr.removed_at IS NULL`, st.round, st.number, user).Scan(&answer)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	if err != nil || answer == nil {
		return err == nil, "", err
	}
	return true, *answer, nil
}

// ReviewAnswerInput is what a reviewer says.
type ReviewAnswerInput struct {
	Answer  string
	Comment string
	// Version is the version the reviewer looked at; a newer one ended their round (DOC-330).
	Version int
}

func cleanComment(comment string, limit int) (string, error) {
	comment = strings.TrimSpace(comment)
	if !utf8.ValidString(comment) || utf8.RuneCountInString(comment) > limit || strings.ContainsRune(comment, 0) {
		return "", apperr.Invalid(fmt.Sprintf("A comment can have at most %d characters.", limit))
	}
	return comment, nil
}

// AnswerReview records a reviewer's answer, and takes the step that follows when it decides
// the round, in the same transaction (DOC-210 to DOC-213). Only a reviewer of the current
// stage answers; a reviewer may change their answer until the round ends, and both answers
// stay in the audit (DOC-212).
func (s *Service) AnswerReview(ctx context.Context, c identity.Caller, id uuid.UUID, in ReviewAnswerInput) (Page, error) {
	if in.Answer != ReviewApprove && in.Answer != ReviewChanges {
		return Page{}, apperr.Invalid("The answer is approve or changes.")
	}
	comment, err := cleanComment(in.Comment, maxTransitionComment)
	if err != nil {
		return Page{}, err
	}
	if in.Answer == ReviewChanges && comment == "" {
		return Page{}, apperr.Invalid("Say which changes you ask for.")
	}
	var out Page
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Access first: a stranger learns nothing of the page from a refused answer.
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.ActionApprovalRead, uuid.Nil); err != nil {
			return err
		}
		rc, err := s.lockReview(ctx, tx, id)
		if err != nil {
			return err
		}
		asked, before, err := myReview(ctx, tx, rc.stage, c.UserID)
		if err != nil {
			return err
		}
		if !asked {
			return apperr.Forbidden()
		}
		if in.Version != rc.ctl.headVersion {
			return apperr.Conflict("superseded", "A newer version of this page was published. Look at it first.")
		}
		if _, err := tx.Exec(ctx, `UPDATE workflow_reviewers SET answer = $4, comment = $5, answered_at = now()
			WHERE round_id = $1 AND stage = $2 AND user_id = $3`, rc.stage.round, rc.stage.number, c.UserID, in.Answer, comment); err != nil {
			return err
		}
		authors, err := changeAuthors(ctx, tx, id, rc.ctl)
		if err != nil {
			return err
		}
		if err := recordEvent(ctx, tx, c, "page.review_answered", rc.page,
			map[string]any{"answer": in.Answer, "before": before, "round": rc.stage.round, "stage": rc.stage.number, "state": rc.state.ID},
			map[string]any{"answer": in.Answer, "user_ids": authors}); err != nil {
			return err
		}
		_, out, err = s.settleReview(ctx, tx, c, rc.page, rc.ctl, rc.stage)
		return err
	})
	return out, err
}

// requireReviewManager allows what changes who reviews: someone with space.workflows, who
// may step in on any round (DOC-013), or an author when the state lets authors add.
func (s *Service) requireReviewManager(ctx context.Context, tx pgx.Tx, c identity.Caller, rc reviewContext, authorsMay bool) error {
	d, err := s.perms.CheckSpace(ctx, tx, c.UserID, rc.page.SpaceID, permissions.SpaceWorkflows)
	if err != nil || d.Allowed {
		return err
	}
	if authorsMay {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, rc.page.ID, permissions.PageEdit, uuid.Nil); err == nil {
			return nil
		}
	}
	return apperr.Forbidden()
}

// AddReviewer asks one more person in a running round (DOC-202). The authors may when the
// state allows it; whoever looks after the workflows always may. The person must be able
// to review: a member who views the space, never a guest, and not an author of the change
// under independence (DOC-220, DOC-1224).
func (s *Service) AddReviewer(ctx context.Context, c identity.Caller, id, user uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
			return err
		}
		rc, err := s.lockReview(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.requireReviewManager(ctx, tx, c, rc, rc.state.Review.AuthorsMayAdd); err != nil {
			return err
		}
		found, empty, err := resolveEntries(ctx, tx, rc.page.SpaceID, []workflow.Reviewer{{Type: workflow.ReviewerUser, ID: user}})
		if err != nil {
			return err
		}
		if len(empty) > 0 || len(found) == 0 {
			return apperr.Conflict("cannot_review", "This person cannot review here: only members who see the space can, never guests.")
		}
		if !rc.state.Review.AuthorsMayReview {
			authors, err := changeAuthors(ctx, tx, id, rc.ctl)
			if err != nil {
				return err
			}
			if slices.Contains(authors, user) {
				return apperr.Conflict("not_independent", "This person wrote part of this change, and the review must be independent.")
			}
		}
		var asked int
		var present bool
		if err := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE removed_at IS NULL), COALESCE(bool_or(user_id = $3), false)
			FROM workflow_reviewers WHERE round_id = $1 AND stage = $2`, rc.stage.round, rc.stage.number, user).Scan(&asked, &present); err != nil {
			return err
		}
		if present {
			return apperr.Conflict("already_asked", "This person was asked in this round already.")
		}
		if asked+1 > rc.state.Review.MaxResolved() {
			return apperr.Conflict("too_many_reviewers", fmt.Sprintf("A round asks at most %d people.", rc.state.Review.MaxResolved()))
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_reviewers (round_id, stage, state_id, user_id, source_type, added_by)
			VALUES ($1, $2, $3, $4, $5, $6)`, rc.stage.round, rc.stage.number, rc.state.ID, user, sourceAdded, c.UserID); err != nil {
			return err
		}
		if err := recordAudit(ctx, tx, c, "page.reviewer_added", "page", id, rc.page.SpaceID,
			map[string]any{"user_id": user, "round": rc.stage.round, "stage": rc.stage.number}); err != nil {
			return err
		}
		return s.askReviewers(ctx, tx, c, rc.page, rc.state, []uuid.UUID{user})
	})
}

// RemoveReviewer takes a person off a running round. Nobody does that silently: it needs
// space.workflows, or the reviewer's own "I cannot review this", and always a reason, and
// it is audited (DOC-202). A removal that leaves too few to pass is refused; one that
// leaves enough who all approved passes the round at once.
func (s *Service) RemoveReviewer(ctx context.Context, c identity.Caller, id, user uuid.UUID, reason string) (Page, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > maxRemovalReason || strings.ContainsRune(reason, 0) {
		return Page{}, apperr.Invalid("Give a reason of at most 500 characters.")
	}
	var out Page
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.ActionApprovalRead, uuid.Nil); err != nil {
			return err
		}
		rc, err := s.lockReview(ctx, tx, id)
		if err != nil {
			return err
		}
		if user != c.UserID {
			if err := s.requireReviewManager(ctx, tx, c, rc, false); err != nil {
				return err
			}
		}
		asked, _, err := myReview(ctx, tx, rc.stage, user)
		if err != nil {
			return err
		}
		if !asked {
			return apperr.Conflict("not_asked", "This person is not asked in this round.")
		}
		remaining, _, _, err := tally(ctx, tx, rc.stage)
		if err != nil {
			return err
		}
		remaining--
		if remaining == 0 || rc.state.Review.Needed(remaining) > remaining {
			return apperr.Conflict("round_impossible", "Without this person the round cannot pass. Add another reviewer first.")
		}
		if _, err := tx.Exec(ctx, `UPDATE workflow_reviewers SET removed_at = now(), removed_by = $4, removed_reason = $5
			WHERE round_id = $1 AND stage = $2 AND user_id = $3`, rc.stage.round, rc.stage.number, user, c.UserID, reason); err != nil {
			return err
		}
		authors, err := changeAuthors(ctx, tx, id, rc.ctl)
		if err != nil {
			return err
		}
		if err := recordEvent(ctx, tx, c, "page.reviewer_removed", rc.page,
			map[string]any{"user_id": user, "reason": reason, "round": rc.stage.round, "stage": rc.stage.number},
			map[string]any{"user_ids": authors}); err != nil {
			return err
		}
		_, out, err = s.settleReview(ctx, tx, c, rc.page, rc.ctl, rc.stage)
		return err
	})
	return out, err
}
