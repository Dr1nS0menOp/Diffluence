// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/audit"
	"github.com/kenwerk/kenwerk/internal/db"
	"github.com/kenwerk/kenwerk/internal/features"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/ids"
	"github.com/kenwerk/kenwerk/internal/outbox"
	"github.com/kenwerk/kenwerk/internal/pages"
	"github.com/kenwerk/kenwerk/internal/spaces"
	"github.com/kenwerk/kenwerk/internal/workflow"
)

// Approval releases (spec 073, wave PB3, chapter 08 of the study). Document control runs in the
// draft space. When a page enters an approved state marked "release when approved", a run of
// that page is queued in the same queue as every run of the pair (PUB-452, PUB-711); when a
// controlled page is withdrawn, its twin is taken off (PUB-720). Both follow the event of the
// step, so they act only after the step committed: a release that fails never undoes an
// approval (decision D8).

// Who starts a run (PUB-902): a person, approval, or the workflow.
const (
	actorUser     = "user"
	actorApproval = "approval"
	actorWorkflow = "workflow"
)

// The events of document control this module follows. A new approved version is announced by
// page.version_approved, on every path that approves, with the approver as its actor (DC3).
const (
	eventApproved  = "page.version_approved"
	eventWithdrawn = "page.withdrawn"
)

// maxToldAuthors bounds the authors an approval release tells (PUB-820).
const maxToldAuthors = 50

// reasonApprovalOnly is why a run skips a controlled page under "only approval releases".
const reasonApprovalOnly = "approval_only"

// workflowEvent is what an event of document control carries that this module reads: ids only.
type workflowEvent struct {
	PageID  uuid.UUID `json:"page_id"`
	SpaceID uuid.UUID `json:"space_id"`
	ActorID uuid.UUID `json:"actor_id"`
}

// FollowWorkflow acts on the steps of document control in a draft space. It runs in the worker,
// may run more than once for one event, and does nothing twice. Approval releases wait while the
// part is off (PUB-1303); withdrawal takes a twin off whatever the switch says, because a
// withdrawn procedure must not stay in front of readers (PUB-720, decision D10).
func (s *Service) FollowWorkflow(ctx context.Context, m outbox.Message) error {
	if m.Type != eventApproved && m.Type != eventWithdrawn {
		return nil
	}
	var e workflowEvent
	if err := json.Unmarshal(m.Payload, &e); err != nil {
		return fmt.Errorf("publishing: read %s: %w", m.Type, err)
	}
	if m.Type == eventWithdrawn {
		return s.takeOffWithdrawn(ctx, e)
	}
	on, err := s.features.ExperimentalOn(ctx, s.pool, features.ExperimentalPublishing)
	if err != nil || !on {
		return err
	}
	return s.queueApprovalRelease(ctx, e)
}

// draftPairOfEvent finds the active pair whose draft space the event happened in, locked so
// runs of the pair are queued one after the other; ok is false anywhere else.
func draftPairOfEvent(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID) (pair, bool, error) {
	p, err := activePairOf(ctx, tx, spaceID, true)
	if hasCode(err, codeNotPaired) {
		return p, false, nil
	}
	if err != nil {
		return p, false, err
	}
	return p, p.Draft == spaceID, nil
}

// approval is the approval of a page as an approval release reads it.
type approval struct {
	version  int
	by       *uuid.UUID
	round    *uuid.UUID
	released int
	// inStep says the twin has what the draft page has: an approval of the same text again,
	// such as a return to the approved text, has nothing to release (PUB-712).
	inStep bool
	// releases says the approved state carries "release when approved".
	releases bool
}

// approvalOf reads the approval a page has now. The mark is read on the state the approved
// version was approved in, so a page that moved on since still releases what was approved.
func approvalOf(ctx context.Context, q db.Querier, pairID, pageID uuid.UUID) (approval, bool, error) {
	var a approval
	c, ok, err := pages.ReleaseControlOf(ctx, q, pageID)
	if err != nil || !ok || c.ApprovedVersion == nil || c.StateKind == workflow.KindObsolete {
		return a, false, err
	}
	a.version, a.by, a.round = *c.ApprovedVersion, c.ApprovedBy, c.Round
	a.releases = c.ApprovedState.ReleaseWhenApproved
	err = q.QueryRow(ctx, `SELECT released_version, released_version > 0 AND cardinality(changed) = 0 FROM page_pairs
		WHERE draft_page_id = $1 AND pair_id = $2`, pageID, pairID).Scan(&a.released, &a.inStep)
	if noRows(err) {
		return a, true, nil
	}
	return a, err == nil, err
}

// queueApprovalRelease queues the release of an approved page whose state is marked, unless
// that version was released or queued already: returning to the approved text releases
// nothing again (PUB-712), and an event handed over twice queues one run.
func (s *Service) queueApprovalRelease(ctx context.Context, e workflowEvent) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, ok, err := draftPairOfEvent(ctx, tx, e.SpaceID)
		if err != nil || !ok {
			return err
		}
		a, ok, err := approvalOf(ctx, tx, p.ID, e.PageID)
		if err != nil || !ok || !a.releases || a.released == a.version || a.inStep {
			return err
		}
		var queued bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM release_runs r JOIN release_run_pages rp ON rp.run_id = r.id
			WHERE r.pair_id = $1 AND r.actor_type = $2 AND r.root_page_id = $3 AND rp.draft_version = $4)`,
			p.ID, actorApproval, e.PageID, a.version).Scan(&queued); err != nil || queued {
			return err
		}
		authors, err := pages.AuthorsBetween(ctx, tx, e.PageID, a.released, a.version, maxToldAuthors)
		if err != nil {
			return err
		}
		src, err := pages.SourceForRelease(ctx, tx, e.PageID)
		if err != nil {
			return err
		}
		action := actionChange
		if a.released == 0 {
			action = actionMake
		}
		return s.queueRun(ctx, tx, queuedRun{pair: p, actor: actorApproval, cause: a.by, page: e.PageID, title: src.Title, version: a.version,
			action: action, scope: ScopePage, options: map[string]any{"version": a.version, "round_id": a.round, "authors": authors}})
	})
}

// takeOffWithdrawn queues taking off the twin of a withdrawn page, by the workflow with the
// withdrawer as the cause (PUB-720). The published home page stays, as it does for a person.
func (s *Service) takeOffWithdrawn(ctx context.Context, e workflowEvent) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, ok, err := draftPairOfEvent(ctx, tx, e.SpaceID)
		if err != nil || !ok {
			return err
		}
		c, ok, err := pages.ReleaseControlOf(ctx, tx, e.PageID)
		if err != nil || !ok || c.StateKind != workflow.KindObsolete {
			return err
		}
		var twin uuid.UUID
		err = tx.QueryRow(ctx, `SELECT pp.twin_page_id FROM page_pairs pp
			WHERE pp.draft_page_id = $1 AND pp.pair_id = $2 AND pp.released_version > 0
			  AND NOT EXISTS (SELECT 1 FROM release_runs r JOIN release_run_pages rp ON rp.run_id = r.id
				WHERE r.pair_id = $2 AND r.actor_type = $3 AND r.status IN ('queued', 'running') AND rp.twin_page_id = pp.twin_page_id)`,
			e.PageID, p.ID, actorWorkflow).Scan(&twin)
		if noRows(err) {
			return nil
		}
		if err != nil {
			return err
		}
		t, err := pages.SourceForRelease(ctx, tx, twin)
		if err != nil {
			return err
		}
		published, err := spaces.LoadIn(ctx, tx, p.Published)
		if err != nil {
			return err
		}
		if t.Status == pages.StatusTrashed || (published.HomePageID != nil && *published.HomePageID == twin) {
			return nil
		}
		cause := e.ActorID
		return s.queueRun(ctx, tx, queuedRun{pair: p, actor: actorWorkflow, cause: &cause, page: e.PageID, title: t.Title, twin: &twin,
			action: actionTakeOff, scope: ScopeTakeOff, options: map[string]any{}})
	})
}

// queuedRun is a run of one page that the system starts.
type queuedRun struct {
	pair    pair
	actor   string
	cause   *uuid.UUID
	page    uuid.UUID
	title   string
	twin    *uuid.UUID
	version int
	action  string
	scope   string
	options map[string]any
}

// queueRun stores a run of one page with no preview and queues it for the worker, audited as
// the system's with the person who caused it (PUB-902).
func (s *Service) queueRun(ctx context.Context, tx pgx.Tx, q queuedRun) error {
	id := ids.New()
	if _, err := tx.Exec(ctx, `INSERT INTO release_runs (id, pair_id, scope, root_page_id, status, actor_type, actor_id, caused_by, options, total)
		VALUES ($1, $2, $3, $4, 'queued', $5, NULL, $6, $7, 1)`, id, q.pair.ID, q.scope, q.page, q.actor, q.cause, q.options); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO release_run_pages (run_id, draft_page_id, draft_version, twin_page_id, action, outcome, title, ord)
		VALUES ($1, $2, $3, COALESCE($4, (SELECT pp.twin_page_id FROM page_pairs pp WHERE pp.draft_page_id = $2)), $5, 'waiting', $6, 1)`,
		id, q.page, q.version, q.twin, q.action, q.title); err != nil {
		return err
	}
	var by uuid.UUID
	if q.cause != nil {
		by = *q.cause
	}
	if _, err := s.jobs.Enqueue(ctx, tx, JobKindRun, runPayload{RunID: id}, by); err != nil {
		return err
	}
	system := identity.Caller{System: q.actor}
	_, err := audit.Record(ctx, tx, system.Event("release.queued", "release_run", id.String(), q.pair.Draft.String(),
		map[string]any{"pair_id": q.pair.ID, "page_id": q.page, "version": q.version, "scope": q.scope, "caused_by": q.cause,
			"round_id": q.options["round_id"]}))
	return err
}
