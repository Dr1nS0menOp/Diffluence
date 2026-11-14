// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

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

// Document control (spec 064, wave DC1). A controlled page has one page_control row. Once
// approved, the pages row holds the approved version, so every reader's route shows
// approved content without asking; a change is written as a newer version that only its
// authors see, the head, and approval makes the head current.

// How a page came under its workflow (DOC-422).
const (
	governedBySpace  = "space"
	governedByPage   = "page"
	governedByBranch = "branch"
)

// Round statuses. A round is one pass of one version through the workflow (DOC-330).
const (
	roundStatusOpen       = "open"
	roundStatusApproved   = "approved"
	roundStatusWithdrawn  = "withdrawn"
	roundStatusSuperseded = "superseded"
	roundStatusEnded      = "ended"
)

// Why the system moved a page, for the record of a round (DOC-105).
const (
	causeChanged         = "changed"
	causeRestored        = "restored"
	causeEqualToApproved = "equal_to_approved"
	causeAssigned        = "assigned"
	causeSpaceWorkflow   = "space_workflow"
	causeControlRemoved  = "control_removed"
	// systemTransition marks a move the system made, never a person's choice.
	systemTransition = "_system"
)

const maxTransitionComment = 2000

// control is the state of a controlled page.
type control struct {
	workflowID      uuid.UUID
	workflowVersion int
	stateID         string
	stateKind       workflow.Kind
	approvedVersion *int
	approvedStateID string
	headVersion     int
	governedBy      string
	governedFrom    *uuid.UUID
}

func (c control) approved() bool { return c.approvedVersion != nil }

const controlColumns = `workflow_id, workflow_version, state_id, state_kind, approved_version, COALESCE(approved_state_id, ''),
	head_version, governed_by, governed_from`

func scanControl(r pgx.Row) (control, error) {
	var c control
	err := r.Scan(&c.workflowID, &c.workflowVersion, &c.stateID, &c.stateKind, &c.approvedVersion, &c.approvedStateID,
		&c.headVersion, &c.governedBy, &c.governedFrom)
	return c, err
}

// lockControl reads the control of a page and locks it. Callers lock the pages row first,
// so a publish and a transition take their locks in the same order (DOC-1231).
func lockControl(ctx context.Context, tx pgx.Tx, pageID uuid.UUID) (control, bool, error) {
	c, err := scanControl(tx.QueryRow(ctx, "SELECT "+controlColumns+" FROM page_control WHERE page_id = $1 FOR UPDATE", pageID))
	if errors.Is(err, pgx.ErrNoRows) {
		return c, false, nil
	}
	return c, err == nil, err
}

// headVersion is the newest version of a page: what its authors see and build on. It is
// the current version, unless a controlled page has a newer one under review.
func headVersion(ctx context.Context, q db.Querier, p Page) (int, error) {
	var head int
	err := q.QueryRow(ctx, "SELECT GREATEST($2::int, (SELECT head_version FROM page_control WHERE page_id = $1))", p.ID, p.CurrentVersion).Scan(&head)
	return head, err
}

// nextVersionNumber is the number of the next version. Versions above the current one can
// exist on a page that was controlled, and a removed version keeps its number.
func nextVersionNumber(ctx context.Context, tx pgx.Tx, p Page) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `SELECT GREATEST($2::int, (SELECT max(number) FROM page_versions WHERE page_id = $1),
		(SELECT max(number) FROM removed_versions WHERE page_id = $1)) + 1`, p.ID, p.CurrentVersion).Scan(&n)
	return n, err
}

// openRound returns the open round of a page, starting one for its head when there is none.
func openRound(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, ctl control) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, "SELECT id FROM workflow_rounds WHERE page_id = $1 AND status = 'open' FOR UPDATE", pageID).Scan(&id)
	if err == nil || !errors.Is(err, pgx.ErrNoRows) {
		return id, err
	}
	id = ids.New()
	_, err = tx.Exec(ctx, `INSERT INTO workflow_rounds (id, page_id, workflow_id, workflow_version, version_number, status)
		VALUES ($1, $2, $3, $4, $5, 'open')`, id, pageID, ctl.workflowID, ctl.workflowVersion, ctl.headVersion)
	return id, err
}

// endRound closes the open round of a page, if any, keeping its record (DOC-330).
func endRound(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, status, reason string) error {
	_, err := tx.Exec(ctx, `UPDATE workflow_rounds SET status = $2, ended_reason = $3, ended_at = now()
		WHERE page_id = $1 AND status = 'open'`, pageID, status, reason)
	return err
}

// systemAnswer records a move the system made, with its cause and no person (DOC-105).
func systemAnswer(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, round *uuid.UUID, from, to, cause string, version int) error {
	_, err := tx.Exec(ctx, `INSERT INTO workflow_answers (id, round_id, page_id, user_id, transition_id, from_state, to_state, version_number, cause)
		VALUES ($1, $2, $3, NULL, $4, $5, $6, $7, $8)`, ids.New(), round, pageID, systemTransition, from, to, version, cause)
	return err
}

// recordSystem audits a move of the workflow as the system's, naming the person whose
// change caused it apart from the actor (DOC-105, Part 11 §11.10(e)).
func recordSystem(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, p Page, details map[string]any) error {
	details["caused_by"] = c.UserID
	system := identity.Caller{System: "workflow", Meta: c.Meta}
	return recordAudit(ctx, tx, system, action, "page", p.ID, p.SpaceID, details)
}

// sameContent says whether two versions hold the same title and text.
func sameContent(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, a, b int) (bool, error) {
	var same bool
	err := tx.QueryRow(ctx, `SELECT x.title = y.title AND x.document = y.document FROM page_versions x, page_versions y
		WHERE x.page_id = $1 AND x.number = $2 AND y.page_id = $1 AND y.number = $3`, pageID, a, b).Scan(&same)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return same, err
}

// restartReview follows a new version of a controlled page: every published change starts
// review again, silent or restored ones too (DOC-310 to DOC-312). The round on the
// version before ends as superseded, and its answers stay on record without counting for
// the new one (DOC-330). A new version equal to the approved one returns the page to its
// approved state without review (DOC-1410).
func (s *Service) restartReview(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, ctl control, number int, cause string) error {
	if ctl.approved() {
		same, err := sameContent(ctx, tx, p.ID, *ctl.approvedVersion, number)
		if err != nil {
			return err
		}
		if same {
			return s.returnToApproved(ctx, tx, c, p, ctl, number)
		}
	}
	var current int
	if err := tx.QueryRow(ctx, "SELECT current_version FROM workflows WHERE id = $1", ctl.workflowID).Scan(&current); err != nil {
		return err
	}
	def, err := definitionOf(ctx, tx, ctl.workflowID, current)
	if err != nil {
		return err
	}
	start := def.StartState()
	if err := endRound(ctx, tx, p.ID, roundStatusSuperseded, cause); err != nil {
		return err
	}
	from := ctl.stateID
	ctl.workflowVersion, ctl.stateID, ctl.stateKind, ctl.headVersion = current, start.ID, start.Kind, number
	if _, err := tx.Exec(ctx, `UPDATE page_control SET workflow_version = $2, state_id = $3, state_kind = $4, head_version = $5, updated_at = now()
		WHERE page_id = $1`, p.ID, current, start.ID, start.Kind, number); err != nil {
		return err
	}
	round, err := openRound(ctx, tx, p.ID, ctl)
	if err != nil {
		return err
	}
	if err := systemAnswer(ctx, tx, p.ID, &round, from, start.ID, cause, number); err != nil {
		return err
	}
	return recordSystem(ctx, tx, c, "page.review_restarted", p, map[string]any{"version": number, "cause": cause, "state": start.ID})
}

// returnToApproved makes an unchanged version the approved one again: readers see the same
// text under the newest number, and nothing waits for review.
func (s *Service) returnToApproved(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, ctl control, number int) error {
	def, err := definitionOf(ctx, tx, ctl.workflowID, ctl.workflowVersion)
	if err != nil {
		return err
	}
	state, ok := def.State(ctl.approvedStateID)
	if !ok {
		state = def.FirstApproved()
	}
	if err := endRound(ctx, tx, p.ID, roundStatusSuperseded, causeEqualToApproved); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE page_control SET state_id = $2, state_kind = $3, approved_version = $4, approved_state_id = $2,
		head_version = $4, updated_at = now() WHERE page_id = $1`, p.ID, state.ID, state.Kind, number); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE pages SET current_version = $2 WHERE id = $1", p.ID, number); err != nil {
		return err
	}
	if err := systemAnswer(ctx, tx, p.ID, nil, ctl.stateID, state.ID, causeEqualToApproved, number); err != nil {
		return err
	}
	return recordSystem(ctx, tx, c, "page.returned_to_approved", p, map[string]any{"version": number})
}

// requireApprovedUnlessControlled is the approval rule of the space for a publish, except on
// a controlled page: there the workflow decides, and the publish starts its review.
func (s *Service) requireApprovedUnlessControlled(ctx context.Context, tx pgx.Tx, p Page, title string, doc document.Node) error {
	var controlled bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM page_control WHERE page_id = $1)", p.ID).Scan(&controlled); err != nil {
		return err
	}
	if controlled {
		return nil
	}
	return s.requireApproved(ctx, tx, p, title, doc)
}

// historyView says which versions of a page a person may see in its history.
type historyView struct {
	// only is the one version a reader of a controlled page sees; 0 means no such limit.
	only int
	// upTo is the newest version a reader of a page that was controlled sees; 0 means all.
	upTo int
}

func (h historyView) shows(number int) bool {
	return (h.only == 0 || number == h.only) && (h.upTo == 0 || number <= h.upTo)
}

// historyOf decides the history a person sees. On an ordinary page, and for the authors of
// a controlled one, it is every version. Anyone else sees only the approved version of a
// controlled page: a working or obsolete version answers as if it did not exist (DOC-320,
// DOC-321). A version that waited for review when control was removed stays out of a
// reader's history until the next publish (DOC-141).
func (s *Service) historyOf(ctx context.Context, c identity.Caller, id uuid.UUID) (historyView, error) {
	var currentVersion int
	var controlled, newer bool
	if err := s.pool.QueryRow(ctx, `SELECT p.current_version, EXISTS (SELECT 1 FROM page_control pc WHERE pc.page_id = p.id),
			EXISTS (SELECT 1 FROM page_versions v WHERE v.page_id = p.id AND v.number > p.current_version)
		FROM pages p WHERE p.id = $1`, id).Scan(&currentVersion, &controlled, &newer); err != nil {
		return historyView{}, err
	}
	if !controlled && !newer {
		return historyView{}, nil
	}
	d, err := s.perms.CheckPage(ctx, s.pool, c.UserID, id, permissions.PageEdit, uuid.Nil)
	if err != nil {
		return historyView{}, err
	}
	switch {
	case d.Allowed:
		return historyView{}, nil
	case controlled:
		return historyView{only: max(currentVersion, 1)}, nil
	}
	return historyView{upTo: currentVersion}, nil
}

// TransitionInput is a step a person takes in a page's workflow.
type TransitionInput struct {
	Transition string
	// Version is the version the person looked at. A newer one means their answer is
	// about a text that has been replaced (DOC-330).
	Version int
	Comment string
}

// Transition takes a step of the workflow of a page (DOC-104). Its authors take the steps;
// the steps that follow a review are taken by the system on the reviewers' answers
// (DOC-013). A step into a review state asks its reviewers, or is refused with them.
func (s *Service) Transition(ctx context.Context, c identity.Caller, id uuid.UUID, in TransitionInput) (Page, error) {
	comment := strings.TrimSpace(in.Comment)
	if !utf8.ValidString(comment) || utf8.RuneCountInString(comment) > maxTransitionComment || strings.ContainsRune(comment, 0) {
		return Page{}, apperr.Invalid("A comment can have at most 2000 characters.")
	}
	var out Page
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		p, err := lockPage(ctx, tx, id)
		if err != nil {
			return err
		}
		ctl, controlled, err := lockControl(ctx, tx, id)
		if err != nil {
			return err
		}
		if !controlled {
			return apperr.Conflict("not_controlled", "This page follows no workflow.")
		}
		def, err := definitionOf(ctx, tx, ctl.workflowID, ctl.workflowVersion)
		if err != nil {
			return err
		}
		tr, ok := def.Transition(in.Transition)
		if !ok {
			return apperr.Invalid("This workflow has no such step.")
		}
		// The step that follows a review is the reviewers' answers, never a person's own
		// choice, not even an administrator's (DOC-013, DOC-1424).
		if tr.Outcome != "" {
			return apperr.Conflict("taken_by_review", "This step follows the review: the reviewers' answers take it.")
		}
		if tr.From != ctl.stateID {
			return apperr.Conflict("state_changed", "This page has moved on since you looked: it is in another state now.")
		}
		if in.Version != ctl.headVersion {
			return apperr.Conflict("superseded", "A newer version of this page was published. Look at it first.")
		}
		if tr.RequiresComment && comment == "" {
			return apperr.Invalid("This step needs a comment.")
		}
		to, ok := def.State(tr.To)
		if !ok {
			return apperr.Invalid("This workflow has no such state.")
		}
		round, err := openRound(ctx, tx, id, ctl)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_answers (id, round_id, page_id, user_id, transition_id, from_state, to_state, version_number, comment)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, ids.New(), round, id, c.UserID, tr.ID, ctl.stateID, to.ID, ctl.headVersion, comment); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE page_control SET state_id = $2, state_kind = $3, updated_at = now() WHERE page_id = $1",
			id, to.ID, to.Kind); err != nil {
			return err
		}
		details := map[string]any{"transition": tr.ID, "from": ctl.stateID, "to": to.ID, "version": ctl.headVersion}
		switch to.Kind {
		case workflow.KindApproved:
			if err := endRound(ctx, tx, id, roundStatusApproved, ""); err != nil {
				return err
			}
			out, err = s.approve(ctx, tx, c, p, ctl, to)
			if err != nil {
				return err
			}
			return record(ctx, tx, c, "page.approved", out, details)
		case workflow.KindObsolete:
			if err := endRound(ctx, tx, id, roundStatusWithdrawn, ""); err != nil {
				return err
			}
			out = p
			return record(ctx, tx, c, "page.withdrawn", out, details)
		}
		out = p
		if err := recordAudit(ctx, tx, c, "page.transition", "page", id, p.SpaceID, details); err != nil {
			return err
		}
		if to.Review == nil {
			return nil
		}
		ctl.stateID, ctl.stateKind = to.ID, to.Kind
		asked, err := s.enterReview(ctx, tx, p, ctl, round, to)
		if err != nil {
			return err
		}
		return s.askReviewers(ctx, tx, c, p, to, asked)
	})
	return out, err
}

// approve makes the head the version readers see (DOC-320): the version approved before
// becomes obsolete, and search, links, tasks and watchers follow the new one.
func (s *Service) approve(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, ctl control, to workflow.State) (Page, error) {
	if _, err := tx.Exec(ctx, `UPDATE page_control SET approved_version = $2, approved_state_id = $3, approved_at = now(), approved_by = $4
		WHERE page_id = $1`, p.ID, ctl.headVersion, to.ID, c.UserID); err != nil {
		return p, err
	}
	if ctl.headVersion == p.CurrentVersion {
		// Never approved before: the page already holds this version, and appears to readers now.
		return p, nil
	}
	var title string
	var raw []byte
	var authorID uuid.UUID
	if err := tx.QueryRow(ctx, "SELECT title, document, author_id FROM page_versions WHERE page_id = $1 AND number = $2",
		p.ID, ctl.headVersion).Scan(&title, &raw, &authorID); err != nil {
		return p, err
	}
	doc, err := document.Parse(raw)
	if err != nil {
		return p, err
	}
	return s.makeCurrent(ctx, tx, c, p, current{number: ctl.headVersion, title: title, doc: doc, author: authorID})
}

// ControlInfo is what the page band shows about the workflow of a page (DOC-302, DOC-303,
// DOC-422).
type ControlInfo struct {
	WorkflowID      uuid.UUID     `json:"workflow_id"`
	WorkflowName    string        `json:"workflow_name"`
	WorkflowVersion int           `json:"workflow_version"`
	StateID         string        `json:"state_id"`
	StateName       string        `json:"state_name"`
	StateKind       workflow.Kind `json:"state_kind"`
	// ApprovedVersion is what readers see; nil when the page was never approved.
	ApprovedVersion *int       `json:"approved_version"`
	ApprovedAt      *time.Time `json:"approved_at"`
	ApprovedBy      string     `json:"approved_by"`
	HeadVersion     int        `json:"head_version"`
	// ShowsWorking says the detail holds the head, not the approved version.
	ShowsWorking bool       `json:"shows_working"`
	GovernedBy   string     `json:"governed_by"`
	GovernedFrom *uuid.UUID `json:"governed_from"`
	// GovernedFromTitle names the page a page or branch workflow is set on. It is an
	// ancestor or the page itself, whose titles the breadcrumbs show already.
	GovernedFromTitle string `json:"governed_from_title"`
	SetBy             string `json:"set_by"`
	// Transitions are the steps this person may take now. The steps that follow a review
	// are never among them: the reviewers' answers take those (DOC-013).
	Transitions []workflow.Transition `json:"transitions"`
	// Reviewing says the person is asked to review the page now, so the band offers the
	// review panel; a reviewer sees the working version like an author (DOC-300).
	Reviewing bool `json:"reviewing"`
	// InReview says the page waits for the answers of reviewers.
	InReview bool `json:"in_review"`
}

// fillControl adds the band of a controlled page to its detail. Its authors and reviewers
// get the head with a mark that it is not approved yet; anyone else keeps the approved
// version and learns nothing of the head (DOC-300, DOC-303).
func (s *Service) fillControl(ctx context.Context, userID uuid.UUID, d *Detail) error {
	ci, def, headTitle, err := controlInfo(ctx, s.pool, userID, d.Page)
	if err != nil || ci == nil {
		return err
	}
	if st, ok := def.State(ci.StateID); ok && st.Review != nil {
		ci.InReview = true
	}
	author := d.Can[permissions.PageEdit]
	if !author && !ci.Reviewing {
		ci.HeadVersion = d.CurrentVersion
		d.Control = ci
		return nil
	}
	if ci.HeadVersion != d.CurrentVersion {
		d.Title, d.CurrentVersion, ci.ShowsWorking = headTitle, ci.HeadVersion, true
	}
	if author {
		for _, tr := range def.From(ci.StateID) {
			if tr.Outcome == "" {
				ci.Transitions = append(ci.Transitions, tr)
			}
		}
	}
	if ci.Transitions == nil {
		ci.Transitions = []workflow.Transition{}
	}
	d.Control = ci
	return nil
}

// controlInfo reads the band of a controlled page, the title of its head and whether the
// person reviews it now, in one query (DOC-1300).
func controlInfo(ctx context.Context, q db.Querier, userID uuid.UUID, p Page) (*ControlInfo, workflow.Definition, string, error) {
	var ci ControlInfo
	var raw []byte
	var def workflow.Definition
	var headTitle string
	err := q.QueryRow(ctx, `SELECT w.id, w.name, pc.workflow_version, v.definition, pc.state_id, pc.state_kind, pc.approved_version,
			pc.approved_at, COALESCE(ua.display_name, ''), pc.head_version, pc.governed_by, pc.governed_from, COALESCE(gp.title, ''),
			COALESCE(us.display_name, ''),
			COALESCE((SELECT hv.title FROM page_versions hv WHERE hv.page_id = pc.page_id AND hv.number = pc.head_version), ''),
			EXISTS (SELECT 1 FROM workflow_reviewers wr
				JOIN workflow_rounds r ON r.id = wr.round_id AND r.status = 'open' AND r.review_stage = wr.stage
				JOIN users ru ON ru.id = wr.user_id AND ru.type = 'member'
				WHERE r.page_id = pc.page_id AND wr.user_id = $3 AND wr.state_id = pc.state_id AND wr.removed_at IS NULL)
		FROM page_control pc
		JOIN workflows w ON w.id = pc.workflow_id
		JOIN workflow_versions v ON v.workflow_id = pc.workflow_id AND v.version = pc.workflow_version
		LEFT JOIN users ua ON ua.id = pc.approved_by
		LEFT JOIN pages gp ON gp.id = pc.governed_from
		LEFT JOIN page_workflows pw ON pw.page_id = pc.governed_from
		LEFT JOIN space_workflows sw ON sw.space_id = $2 AND pc.governed_by = 'space'
		LEFT JOIN users us ON us.id = COALESCE(pw.set_by, sw.set_by)
		WHERE pc.page_id = $1`, p.ID, p.SpaceID, userID).
		Scan(&ci.WorkflowID, &ci.WorkflowName, &ci.WorkflowVersion, &raw, &ci.StateID, &ci.StateKind, &ci.ApprovedVersion,
			&ci.ApprovedAt, &ci.ApprovedBy, &ci.HeadVersion, &ci.GovernedBy, &ci.GovernedFrom, &ci.GovernedFromTitle, &ci.SetBy, &headTitle,
			&ci.Reviewing)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, def, "", nil
	}
	if err != nil {
		return nil, def, "", err
	}
	if err := json.Unmarshal(raw, &def); err != nil {
		return nil, def, "", fmt.Errorf("pages: read workflow of page %s: %v", p.ID, err)
	}
	if st, ok := def.State(ci.StateID); ok {
		ci.StateName = st.Name
	}
	ci.Transitions = []workflow.Transition{}
	return &ci, def, headTitle, nil
}
