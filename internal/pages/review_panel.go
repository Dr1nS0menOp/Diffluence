// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/workflow"
)

// The review panel (DOC-250, DOC-251, DOC-921): the round of a page as its reviewers and
// authors see it. Readers never see it, also not after approval (DOC-501).

// ReviewerRow is one person a round asked.
type ReviewerRow struct {
	UserID uuid.UUID `json:"user_id"`
	Name   string    `json:"name"`
	// Source says how they came to be asked: user, group, role or added, with the name of
	// the group or role.
	Source        string     `json:"source"`
	SourceName    string     `json:"source_name"`
	Answer        string     `json:"answer"`
	Comment       string     `json:"comment"`
	AnsweredAt    *time.Time `json:"answered_at"`
	Removed       bool       `json:"removed"`
	RemovedReason string     `json:"removed_reason"`
}

// ReviewComment is a comment that belongs to the round, not to the page (DOC-251).
type ReviewComment struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Name      string    `json:"name"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// ReviewText is one side of the comparison the panel shows.
type ReviewText struct {
	Version  int             `json:"version"`
	Title    string          `json:"title"`
	Document json.RawMessage `json:"document"`
}

// ReviewCan says what the person may do in the panel now.
type ReviewCan struct {
	Answer  bool `json:"answer"`
	Add     bool `json:"add"`
	Remove  bool `json:"remove"`
	Comment bool `json:"comment"`
}

// ReviewPanel is the round of a page: the open one, or else the last one.
type ReviewPanel struct {
	RoundID     *uuid.UUID `json:"round_id"`
	Status      string     `json:"status"`
	EndedReason string     `json:"ended_reason"`
	StartedAt   *time.Time `json:"started_at"`
	StateID     string     `json:"state_id"`
	StateName   string     `json:"state_name"`
	// InReview says the page waits for the answers of this stage now.
	InReview    bool            `json:"in_review"`
	Rule        workflow.Rule   `json:"rule"`
	Needed      int             `json:"needed"`
	Approved    int             `json:"approved"`
	Asked       int             `json:"asked"`
	Independent bool            `json:"independent"`
	MyAnswer    string          `json:"my_answer"`
	Reviewers   []ReviewerRow   `json:"reviewers"`
	Comments    []ReviewComment `json:"comments"`
	// Base is the approved version, which the change is compared with (DOC-250); nil when the
	// page was never approved. Head is the version under review. Both only reach someone who
	// may see the working version.
	Base *ReviewText `json:"base"`
	Head *ReviewText `json:"head"`
	// Changes is the comparison of Base and Head, block by block, for the same people.
	Changes []document.DiffBlock `json:"changes,omitempty"`
	Can     ReviewCan            `json:"can"`
}

// lastRound reads the open round of a page, or else its newest one.
func lastRound(ctx context.Context, q db.Querier, pageID uuid.UUID) (stage, string, string, time.Time, bool, error) {
	var st stage
	var status, reason string
	var started time.Time
	err := q.QueryRow(ctx, `SELECT id, review_stage, status, ended_reason, started_at FROM workflow_rounds WHERE page_id = $1
		ORDER BY status = 'open' DESC, started_at DESC, id DESC LIMIT 1`, pageID).Scan(&st.round, &st.number, &status, &reason, &started)
	if errors.Is(err, pgx.ErrNoRows) {
		return st, "", "", started, false, nil
	}
	return st, status, reason, started, err == nil, err
}

// namedInRound says a person was asked in any stage of a round: they keep the record of it,
// also after it ended or after they were taken off (DOC-203, PAG-065).
func namedInRound(ctx context.Context, q db.Querier, round, user uuid.UUID) (bool, error) {
	var named bool
	err := q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM workflow_reviewers WHERE round_id = $1 AND user_id = $2)", round, user).Scan(&named)
	return named, err
}

// Review returns the review panel of a page for its authors and the people the round asked.
func (s *Service) Review(ctx context.Context, c identity.Caller, id uuid.UUID) (ReviewPanel, error) {
	out := ReviewPanel{Reviewers: []ReviewerRow{}, Comments: []ReviewComment{}}
	author, err := s.perms.CheckPage(ctx, s.pool, c.UserID, id, permissions.PageEdit, uuid.Nil)
	if err != nil {
		return out, err
	}
	st, status, reason, started, found, err := lastRound(ctx, s.pool, id)
	if err != nil {
		return out, err
	}
	named := false
	if found {
		if named, err = namedInRound(ctx, s.pool, st.round, c.UserID); err != nil {
			return out, err
		}
	}
	if !author.Allowed && !named {
		// Readers learn nothing of a round; strangers learn nothing of the page.
		if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
			return out, err
		}
		return out, apperr.Forbidden()
	}
	p, err := load(ctx, s.pool, id)
	if err != nil {
		return out, err
	}
	var ctl control
	if ctl, err = scanControl(s.pool.QueryRow(ctx, "SELECT "+controlColumns+" FROM page_control WHERE page_id = $1", id)); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, apperr.Conflict("not_controlled", "This page follows no workflow.")
		}
		return out, err
	}
	def, err := definitionOf(ctx, s.pool, ctl.workflowID, ctl.workflowVersion)
	if err != nil {
		return out, err
	}
	out.StateID = ctl.stateID
	if state, ok := def.State(ctl.stateID); ok {
		out.StateName = state.Name
		if state.Review != nil {
			out.Rule = state.Review.Rule
			out.Independent = !state.Review.AuthorsMayReview
		}
	}
	reviewing := false
	if found {
		out.RoundID, out.Status, out.EndedReason, out.StartedAt = &st.round, status, reason, &started
		if out.Reviewers, err = reviewersOf(ctx, s.pool, st); err != nil {
			return out, err
		}
		if out.Comments, err = reviewCommentsOf(ctx, s.pool, st.round); err != nil {
			return out, err
		}
		if status == roundStatusOpen && st.number > 0 && out.Rule != "" {
			out.InReview = currentStageState(ctx, s.pool, st) == ctl.stateID
		}
		var asked bool
		if out.InReview {
			if asked, out.MyAnswer, err = myReview(ctx, s.pool, st, c.UserID); err != nil {
				return out, err
			}
		}
		reviewing = asked
		out.Can.Answer = asked
		out.Can.Remove = asked
	}
	if out.InReview {
		state, _ := def.State(ctl.stateID)
		for _, r := range out.Reviewers {
			if !r.Removed {
				out.Asked++
				if r.Answer == ReviewApprove {
					out.Approved++
				}
			}
		}
		out.Needed = state.Review.Needed(out.Asked)
		manager, err := s.perms.CheckSpace(ctx, s.pool, c.UserID, p.SpaceID, permissions.SpaceWorkflows)
		if err != nil {
			return out, err
		}
		out.Can.Add = manager.Allowed || (author.Allowed && state.Review.AuthorsMayAdd)
		out.Can.Remove = out.Can.Remove || manager.Allowed
	}
	out.Can.Comment = found && status == roundStatusOpen && (author.Allowed || reviewing)
	if author.Allowed || reviewing {
		if out.Base, out.Head, err = reviewTexts(ctx, s.pool, p.ID, ctl); err != nil {
			return out, err
		}
		if out.Changes, err = s.reviewChanges(ctx, out.Base, out.Head); err != nil {
			return out, err
		}
	}
	return out, nil
}

// reviewChanges compares the version under review with the approved one, or with an empty
// page when there is none, so a reviewer sees the whole change readers will get (DOC-250).
func (s *Service) reviewChanges(ctx context.Context, base, head *ReviewText) ([]document.DiffBlock, error) {
	if head == nil {
		return nil, nil
	}
	after, err := s.readable(ctx, head.Document)
	if err != nil {
		return nil, err
	}
	before := document.Empty()
	if base != nil {
		if before, err = s.readable(ctx, base.Document); err != nil {
			return nil, err
		}
	}
	return document.Compare(before, after), nil
}

// currentStageState reads the state the current stage of a round asks for.
func currentStageState(ctx context.Context, q db.Querier, st stage) string {
	var state string
	if err := q.QueryRow(ctx, "SELECT state_id FROM workflow_reviewers WHERE round_id = $1 AND stage = $2 LIMIT 1", st.round, st.number).Scan(&state); err != nil {
		return ""
	}
	return state
}

func reviewersOf(ctx context.Context, q db.Querier, st stage) ([]ReviewerRow, error) {
	rows, err := q.Query(ctx, `SELECT wr.user_id, COALESCE(u.display_name, ''), wr.source_type, COALESCE(g.name, ro.name, ''),
			COALESCE(wr.answer, ''), wr.comment, wr.answered_at, wr.removed_at IS NOT NULL, wr.removed_reason
		FROM workflow_reviewers wr
		LEFT JOIN users u ON u.id = wr.user_id
		LEFT JOIN groups g ON wr.source_type = 'group' AND g.id = wr.source_id
		LEFT JOIN roles ro ON wr.source_type = 'role' AND ro.id = wr.source_id
		WHERE wr.round_id = $1 AND wr.stage = $2 ORDER BY wr.removed_at IS NOT NULL, lower(u.display_name), wr.user_id LIMIT $3`,
		st.round, st.number, workflow.MaxAnyReviewers*2)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ReviewerRow])
	if out == nil {
		out = []ReviewerRow{}
	}
	return out, err
}

func reviewCommentsOf(ctx context.Context, q db.Querier, round uuid.UUID) ([]ReviewComment, error) {
	rows, err := q.Query(ctx, `SELECT c.id, c.user_id, COALESCE(u.display_name, ''), c.body, c.created_at
		FROM workflow_review_comments c LEFT JOIN users u ON u.id = c.user_id
		WHERE c.round_id = $1 ORDER BY c.created_at, c.id LIMIT 500`, round)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ReviewComment])
	if out == nil {
		out = []ReviewComment{}
	}
	return out, err
}

// reviewTexts reads the approved version and the version under review, for the comparison.
func reviewTexts(ctx context.Context, q db.Querier, pageID uuid.UUID, ctl control) (*ReviewText, *ReviewText, error) {
	text := func(number int) (*ReviewText, error) {
		t := ReviewText{Version: number}
		var schema int
		err := q.QueryRow(ctx, "SELECT title, document, schema_version FROM page_versions WHERE page_id = $1 AND number = $2", pageID, number).
			Scan(&t.Title, &t.Document, &schema)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if t.Document, err = document.Upgrade(t.Document, schema); err != nil {
			return nil, err
		}
		return &t, nil
	}
	head, err := text(ctl.headVersion)
	if err != nil || ctl.approvedVersion == nil {
		return nil, head, err
	}
	base, err := text(*ctl.approvedVersion)
	return base, head, err
}

// AddReviewComment adds a comment to the open round. Its reviewers and the page's authors
// write them, and only they read them (DOC-251, DOC-501).
func (s *Service) AddReviewComment(ctx context.Context, c identity.Caller, id uuid.UUID, body string) (ReviewComment, error) {
	body, err := cleanComment(body, maxReviewComment)
	if err != nil {
		return ReviewComment{}, err
	}
	if body == "" {
		return ReviewComment{}, apperr.Invalid("Write a comment first.")
	}
	out := ReviewComment{ID: ids.New(), UserID: c.UserID, Body: body}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.ActionApprovalRead, uuid.Nil); err != nil {
			return err
		}
		p, err := lockPage(ctx, tx, id)
		if err != nil {
			return err
		}
		st, open, err := openStage(ctx, tx, id)
		if err != nil {
			return err
		}
		if !open {
			return apperr.Conflict("round_closed", "This round has ended.")
		}
		author, err := s.perms.CheckPage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil)
		if err != nil {
			return err
		}
		asked, _, err := myReview(ctx, tx, st, c.UserID)
		if err != nil {
			return err
		}
		if !author.Allowed && !asked {
			return apperr.Forbidden()
		}
		if err := tx.QueryRow(ctx, `INSERT INTO workflow_review_comments (id, round_id, user_id, body) VALUES ($1, $2, $3, $4)
			RETURNING created_at`, out.ID, st.round, c.UserID, body).Scan(&out.CreatedAt); err != nil {
			return err
		}
		return recordAudit(ctx, tx, c, "page.review_commented", "page", id, p.SpaceID, map[string]any{"round": st.round, "comment": out.ID})
	})
	return out, err
}
