// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/search"
	"github.com/diffluence/diffluence/internal/workflow"
)

// The record of document control in a bundle (spec 064, DOC-532): the workflows of the
// controlled pages, where each page stands, and its rounds with their answers, reviewers and
// review comments, so an import elsewhere keeps the record. Ids of people are those of the
// bundle; the importer maps them.

// BundledWorkflow is a workflow at the version its pages follow.
type BundledWorkflow struct {
	ID          uuid.UUID           `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Definition  workflow.Definition `json:"definition"`
}

// BundledControl is where one controlled page stands.
type BundledControl struct {
	PageID          uuid.UUID  `json:"page_id"`
	WorkflowID      uuid.UUID  `json:"workflow_id"`
	StateID         string     `json:"state_id"`
	ApprovedVersion *int       `json:"approved_version"`
	ApprovedStateID string     `json:"approved_state_id"`
	ApprovedAt      *time.Time `json:"approved_at"`
	ApprovedBy      *uuid.UUID `json:"approved_by"`
	HeadVersion     int        `json:"head_version"`
}

// BundledAnswer is one step taken in a round.
type BundledAnswer struct {
	UserID       *uuid.UUID `json:"user_id"`
	TransitionID string     `json:"transition_id"`
	From         string     `json:"from_state"`
	To           string     `json:"to_state"`
	Version      int        `json:"version_number"`
	Comment      string     `json:"comment"`
	Cause        string     `json:"cause"`
	At           time.Time  `json:"answered_at"`
}

// BundledReviewer is a person a stage of a round asked, with their answer.
type BundledReviewer struct {
	Stage      int        `json:"stage"`
	StateID    string     `json:"state_id"`
	UserID     uuid.UUID  `json:"user_id"`
	AskedAt    time.Time  `json:"asked_at"`
	Answer     *string    `json:"answer"`
	Comment    string     `json:"comment"`
	AnsweredAt *time.Time `json:"answered_at"`
	RemovedAt  *time.Time `json:"removed_at"`
	Reason     string     `json:"removed_reason"`
}

// BundledComment is a review comment.
type BundledComment struct {
	UserID uuid.UUID `json:"user_id"`
	Body   string    `json:"body"`
	At     time.Time `json:"created_at"`
}

// BundledRound is one round of a page, with all that belongs to it.
type BundledRound struct {
	PageID    uuid.UUID         `json:"page_id"`
	Version   int               `json:"version_number"`
	Status    string            `json:"status"`
	Reason    string            `json:"ended_reason"`
	Stage     int               `json:"review_stage"`
	StartedAt time.Time         `json:"started_at"`
	EndedAt   *time.Time        `json:"ended_at"`
	Answers   []BundledAnswer   `json:"answers"`
	Reviewers []BundledReviewer `json:"reviewers"`
	Comments  []BundledComment  `json:"comments"`
}

// ControlRecord is all of document control a bundle of a space carries.
type ControlRecord struct {
	Workflows []BundledWorkflow `json:"workflows"`
	Pages     []BundledControl  `json:"pages"`
	Rounds    []BundledRound    `json:"rounds"`
}

// People is how an import finds the people of a bundle here: ok is false for someone with no
// match, and Importer stands in where a person must be named.
type People struct {
	Find     func(uuid.UUID) (uuid.UUID, bool)
	Importer uuid.UUID
}

func (p People) author(id uuid.UUID) uuid.UUID {
	if to, ok := p.Find(id); ok {
		return to
	}
	return p.Importer
}

func (p People) optional(id *uuid.UUID) *uuid.UUID {
	if id == nil {
		return nil
	}
	if to, ok := p.Find(*id); ok {
		return &to
	}
	return nil
}

// BundleControl reads the record of the controlled pages of a space, in the transaction that
// reads its pages.
func (s *Service) BundleControl(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID) (ControlRecord, error) {
	out := ControlRecord{Workflows: []BundledWorkflow{}, Pages: []BundledControl{}, Rounds: []BundledRound{}}
	rows, err := tx.Query(ctx, `SELECT pc.page_id, pc.workflow_id, pc.state_id, pc.approved_version, COALESCE(pc.approved_state_id, ''),
			pc.approved_at, pc.approved_by, pc.head_version
		FROM page_control pc JOIN pages p ON p.id = pc.page_id WHERE p.space_id = $1 AND p.status = 'published' ORDER BY pc.page_id`, spaceID)
	if err != nil {
		return out, err
	}
	if out.Pages, err = pgx.CollectRows(rows, pgx.RowToStructByPos[BundledControl]); err != nil {
		return out, err
	}
	rows, err = tx.Query(ctx, `SELECT w.id, w.name, w.description, v.definition FROM workflows w
		JOIN workflow_versions v ON v.workflow_id = w.id AND v.version = w.current_version
		WHERE w.id IN (SELECT pc.workflow_id FROM page_control pc JOIN pages p ON p.id = pc.page_id WHERE p.space_id = $1) ORDER BY w.id`, spaceID)
	if err != nil {
		return out, err
	}
	out.Workflows, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (BundledWorkflow, error) {
		var w BundledWorkflow
		var raw []byte
		if err := r.Scan(&w.ID, &w.Name, &w.Description, &raw); err != nil {
			return w, err
		}
		return w, json.Unmarshal(raw, &w.Definition)
	})
	if err != nil {
		return out, err
	}
	return out, s.bundleRounds(ctx, tx, spaceID, &out)
}

// bundleRounds reads the rounds of the pages of a space with their parts.
func (s *Service) bundleRounds(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID, out *ControlRecord) error {
	rows, err := tx.Query(ctx, `SELECT r.id, r.page_id, r.version_number, r.status, r.ended_reason, r.review_stage, r.started_at, r.ended_at
		FROM workflow_rounds r JOIN pages p ON p.id = r.page_id
		WHERE p.space_id = $1 AND p.status = 'published' ORDER BY r.started_at, r.id`, spaceID)
	if err != nil {
		return err
	}
	var roundIDs []uuid.UUID
	out.Rounds, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (BundledRound, error) {
		var b BundledRound
		var id uuid.UUID
		err := r.Scan(&id, &b.PageID, &b.Version, &b.Status, &b.Reason, &b.Stage, &b.StartedAt, &b.EndedAt)
		roundIDs = append(roundIDs, id)
		b.Answers, b.Reviewers, b.Comments = []BundledAnswer{}, []BundledReviewer{}, []BundledComment{}
		return b, err
	})
	if err != nil {
		return err
	}
	for i, id := range roundIDs {
		round := &out.Rounds[i]
		rows, err := tx.Query(ctx, `SELECT user_id, transition_id, from_state, to_state, version_number, comment, cause, answered_at
			FROM workflow_answers WHERE round_id = $1 ORDER BY answered_at, id`, id)
		if err != nil {
			return err
		}
		if round.Answers, err = pgx.CollectRows(rows, pgx.RowToStructByPos[BundledAnswer]); err != nil {
			return err
		}
		rows, err = tx.Query(ctx, `SELECT stage, state_id, user_id, asked_at, answer, comment, answered_at, removed_at, removed_reason
			FROM workflow_reviewers WHERE round_id = $1 ORDER BY stage, asked_at, user_id`, id)
		if err != nil {
			return err
		}
		if round.Reviewers, err = pgx.CollectRows(rows, pgx.RowToStructByPos[BundledReviewer]); err != nil {
			return err
		}
		rows, err = tx.Query(ctx, `SELECT user_id, body, created_at FROM workflow_review_comments WHERE round_id = $1 ORDER BY created_at, id`, id)
		if err != nil {
			return err
		}
		if round.Comments, err = pgx.CollectRows(rows, pgx.RowToStructByPos[BundledComment]); err != nil {
			return err
		}
	}
	return nil
}

// How a round that came with a bundle is marked ended, when it was open, and why the system
// put an imported page in its state.
const (
	importedRound = "imported"
	causeImported = "imported"
)

// InsertImportedControl puts the controlled pages of an import under copies of their
// workflows, after their pages and files are in. Readers see what readers of the bundle's
// installation saw: the approved version, or nothing of a page never approved. A page that
// waited for review starts review again here, because its reviewers were asked elsewhere.
// The rounds come along as a record marked with the installation they came from, and never
// count here (DOC-532, DOC-1212).
func (s *Service) InsertImportedControl(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID, rec ControlRecord, page func(uuid.UUID) uuid.UUID,
	people People, source uuid.UUID) error {
	defs := map[uuid.UUID]workflow.Definition{}
	copies := map[uuid.UUID]uuid.UUID{}
	for _, w := range rec.Workflows {
		if err := workflow.Validate(w.Definition); err != nil {
			return apperr.Invalid(fmt.Sprintf("A workflow in this bundle is not valid: %s", w.Name))
		}
		raw, err := json.Marshal(w.Definition)
		if err != nil {
			return err
		}
		id := ids.New()
		if _, err := tx.Exec(ctx, `INSERT INTO workflows (id, space_id, name, description, created_by) VALUES ($1, $2, $3, $4, $5)`,
			id, spaceID, w.Name, w.Description, people.Importer); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_versions (workflow_id, version, definition, saved_by) VALUES ($1, 1, $2, $3)`,
			id, raw, people.Importer); err != nil {
			return err
		}
		defs[w.ID], copies[w.ID] = w.Definition, id
	}
	for _, pc := range rec.Pages {
		def, ok := defs[pc.WorkflowID]
		if !ok {
			return apperr.Invalid("This bundle names a workflow it does not hold.")
		}
		if err := s.importControl(ctx, tx, page(pc.PageID), pc, copies[pc.WorkflowID], def, people); err != nil {
			return err
		}
	}
	for _, r := range rec.Rounds {
		if err := importRound(ctx, tx, page(r.PageID), r, people, source); err != nil {
			return err
		}
	}
	return nil
}

// importControl puts one page under the copy of its workflow, as a page workflow.
func (s *Service) importControl(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, pc BundledControl, workflowID uuid.UUID, def workflow.Definition,
	people People) error {
	approved := pc.ApprovedVersion != nil && pc.HeadVersion == *pc.ApprovedVersion
	state, ok := def.State(pc.StateID)
	if !approved || !ok || state.Kind != workflow.KindApproved {
		state = def.StartState()
	}
	if approved {
		if st, found := def.State(pc.ApprovedStateID); found {
			state = st
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO page_workflows (page_id, workflow_id, branch, set_by) VALUES ($1, $2, false, $3)`,
		pageID, workflowID, people.Importer); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO page_control (page_id, workflow_id, workflow_version, state_id, state_kind, approved_version,
			approved_state_id, approved_at, approved_by, head_version, governed_by, governed_from)
		VALUES ($1, $2, 1, $3, $4, $5, NULLIF($6, ''), $7, $8, $9, 'page', $1)`,
		pageID, workflowID, state.ID, state.Kind, pc.ApprovedVersion, pc.ApprovedStateID, pc.ApprovedAt, people.optional(pc.ApprovedBy),
		pc.HeadVersion); err != nil {
		return err
	}
	var current int
	if err := tx.QueryRow(ctx, "SELECT current_version FROM pages WHERE id = $1", pageID).Scan(&current); err != nil {
		return err
	}
	for _, number := range []int{current, pc.HeadVersion} {
		if err := attachments.Pin(ctx, tx, pageID, number); err != nil {
			return err
		}
	}
	if state.Kind == workflow.KindApproved {
		return nil
	}
	round := ids.New()
	if _, err := tx.Exec(ctx, `INSERT INTO workflow_rounds (id, page_id, workflow_id, workflow_version, version_number, status)
		VALUES ($1, $2, $3, 1, $4, 'open')`, round, pageID, workflowID, pc.HeadVersion); err != nil {
		return err
	}
	if err := systemAnswer(ctx, tx, pageID, &round, "", state.ID, causeImported, pc.HeadVersion); err != nil {
		return err
	}
	if pc.HeadVersion <= current {
		return nil
	}
	var title string
	var raw []byte
	if err := tx.QueryRow(ctx, "SELECT title, document FROM page_versions WHERE page_id = $1 AND number = $2", pageID, pc.HeadVersion).
		Scan(&title, &raw); err != nil {
		return err
	}
	doc, err := document.Parse(raw)
	if err != nil {
		return err
	}
	return search.IndexWorking(ctx, tx, pageID, title, document.Text(doc))
}

// importRound writes the record of one round from a bundle. An open round ends here: the
// page starts its review again under this installation's reviewers.
func importRound(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, r BundledRound, people People, source uuid.UUID) error {
	var workflowID uuid.UUID
	if err := tx.QueryRow(ctx, "SELECT workflow_id FROM page_control WHERE page_id = $1", pageID).Scan(&workflowID); err != nil {
		return err
	}
	status, reason, ended := r.Status, r.Reason, r.EndedAt
	if status == roundStatusOpen {
		now := time.Now()
		status, reason, ended = roundStatusEnded, importedRound, &now
	}
	id := ids.New()
	if _, err := tx.Exec(ctx, `INSERT INTO workflow_rounds (id, page_id, workflow_id, workflow_version, version_number, status, ended_reason,
			review_stage, started_at, ended_at, imported_from)
		VALUES ($1, $2, $3, 1, $4, $5, $6, $7, $8, $9, $10)`, id, pageID, workflowID, r.Version, status, reason, r.Stage, r.StartedAt, ended, source); err != nil {
		return err
	}
	for _, a := range r.Answers {
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_answers (id, round_id, page_id, user_id, transition_id, from_state, to_state, version_number,
				comment, cause, answered_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			ids.New(), id, pageID, people.optional(a.UserID), a.TransitionID, a.From, a.To, a.Version, a.Comment, a.Cause, a.At); err != nil {
			return err
		}
	}
	for _, rv := range r.Reviewers {
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_reviewers (round_id, stage, state_id, user_id, source_type, asked_at, answer, comment,
				answered_at, removed_at, removed_reason) VALUES ($1, $2, $3, $4, 'added', $5, $6, $7, $8, $9, $10) ON CONFLICT DO NOTHING`,
			id, rv.Stage, rv.StateID, people.author(rv.UserID), rv.AskedAt, rv.Answer, rv.Comment, rv.AnsweredAt, rv.RemovedAt, rv.Reason); err != nil {
			return err
		}
	}
	for _, c := range r.Comments {
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_review_comments (id, round_id, user_id, body, created_at) VALUES ($1, $2, $3, $4, $5)`,
			ids.New(), id, people.author(c.UserID), c.Body, c.At); err != nil {
			return err
		}
	}
	return nil
}
