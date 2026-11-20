// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"context"

	"github.com/google/uuid"

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/permissions"
	"github.com/kenwerk/kenwerk/internal/watches"
)

// Notifications of draft and published spaces (spec 073, PUB-813 to PUB-823): a run of many
// pages is told once per person, through the existing channels, outbox and rate limits.

// ReleasedRun is what a run did, as notifications need it: ids only.
type ReleasedRun struct {
	Released []uuid.UUID
	TakenOff []uuid.UUID
	// PublishedHome and DraftHome are the home pages the rows hang on, so a person who loses
	// the space loses the row.
	PublishedHome *uuid.UUID
	DraftHome     *uuid.UUID
}

// SetReleasedPages gives the module the reader of runs, which the publishing module owns.
func (s *Service) SetReleasedPages(read func(context.Context, uuid.UUID) (ReleasedRun, error)) {
	s.releasedPages = read
}

// EndedPair is a pair that deleting one of its spaces ended: the space that stays, its home page
// and the people who run it. Ids only.
type EndedPair struct {
	SpaceID uuid.UUID
	Home    uuid.UUID
	Admins  []uuid.UUID
	// Side is the side of the space that was deleted: draft or published.
	Side string
}

// SetEndedPairs gives the module the reader of pairs that a deleted space ended, which the
// publishing module owns.
func (s *Service) SetEndedPairs(read func(context.Context, uuid.UUID) (*EndedPair, error)) {
	s.endedPair = read
}

// onSpaceTrashed tells the people who run the space that stays when deleting the other space of
// a pair ended it: their space is ordinary again, or no longer has drafts (PUB-1046, PUB-1047).
// The person who deleted it knows.
func (s *Service) onSpaceTrashed(ctx context.Context, event uuid.UUID, p payload) error {
	if s.endedPair == nil {
		return nil
	}
	ended, err := s.endedPair(ctx, p.SpaceID)
	if err != nil || ended == nil {
		return err
	}
	for _, person := range ended.Admins {
		if person == p.ActorID {
			continue
		}
		if err := s.Deliver(ctx, s.pool, Row{UserID: person, Type: TypePairEnded, Reason: ReasonAuthor, ActorID: p.ActorID,
			PageID: ended.Home, SpaceID: ended.SpaceID, Event: event, Data: map[string]any{"deleted_side": ended.Side}}); err != nil {
			return err
		}
	}
	return nil
}

// onReleaseFinished tells the releaser that the run ended, with a link to its report (PUB-822),
// and every watcher of the pages it changed or took off once, with how many of the pages they
// watch and may see it changed (PUB-813, PUB-818). A silent run tells only the releaser
// (PUB-821).
func (s *Service) onReleaseFinished(ctx context.Context, event uuid.UUID, p payload) error {
	if s.releasedPages == nil {
		return nil
	}
	run, err := s.releasedPages(ctx, p.RunID)
	if err != nil {
		return err
	}
	if err := s.tellApproval(ctx, event, p); err != nil {
		return err
	}
	if run.DraftHome != nil && p.ActorID != uuid.Nil {
		// No actor: the releaser is told of their own run whatever they chose for their own work.
		if err := s.Deliver(ctx, s.pool, Row{UserID: p.ActorID, Type: TypeReleaseFinished, Reason: ReasonAuthor, PageID: *run.DraftHome,
			SpaceID: p.DraftSpaceID, Event: event, Data: map[string]any{"run_id": p.RunID, "outcome": p.Outcome,
				"released": len(run.Released), "taken_off": len(run.TakenOff)}}); err != nil {
			return err
		}
	}
	if p.Silent || p.Outcome != "done" || run.PublishedHome == nil {
		return nil
	}
	released, err := s.watchersOf(ctx, run.Released, p.SpaceID, true)
	if err != nil {
		return err
	}
	takenOff, err := s.watchersOf(ctx, run.TakenOff, p.SpaceID, false)
	if err != nil {
		return err
	}
	people := map[uuid.UUID]bool{}
	for person := range released {
		people[person] = true
	}
	for person := range takenOff {
		people[person] = true
	}
	for person := range people {
		if released[person] == 0 && takenOff[person] == 0 {
			continue
		}
		if err := s.Deliver(ctx, s.pool, Row{UserID: person, Type: TypePagesReleased, Reason: ReasonWatching, ActorID: p.ActorID,
			PageID: *run.PublishedHome, SpaceID: p.SpaceID, Event: event,
			Data: map[string]any{"run_id": p.RunID, "released": released[person], "taken_off": takenOff[person]}}); err != nil {
			return err
		}
	}
	return nil
}

// tellApproval tells the approver and the authors of an approval release, on the draft page,
// that it was released, or that it was not and why, as a code the screen puts in words
// (PUB-820). A run of a person has no approver, and tells nobody here.
func (s *Service) tellApproval(ctx context.Context, event uuid.UUID, p payload) error {
	if p.CausedBy == nil || p.DraftPageID == uuid.Nil {
		return nil
	}
	outcome, reason := p.PageOutcome, p.PageReason
	if p.Outcome != "done" {
		outcome, reason = p.Outcome, p.StopReason
	}
	people := append([]uuid.UUID{*p.CausedBy}, p.UserIDs...)
	told := map[uuid.UUID]bool{}
	for _, person := range people {
		if person == uuid.Nil || told[person] {
			continue
		}
		told[person] = true
		if err := s.Deliver(ctx, s.pool, Row{UserID: person, Type: TypeApprovalRelease, Reason: ReasonAuthor, PageID: p.DraftPageID,
			SpaceID: p.DraftSpaceID, Event: event, Data: map[string]any{"run_id": p.RunID, "outcome": outcome, "why": reason}}); err != nil {
			return err
		}
	}
	return nil
}

// watchersOf counts, per person, the pages among list they watch, by the page, its branch or
// the space. Pages still there count only when the person may view them; a page taken off is
// in the trash, and counts for whoever watched it, without its title (PUB-818).
func (s *Service) watchersOf(ctx context.Context, list []uuid.UUID, spaceID uuid.UUID, mustView bool) (map[uuid.UUID]int, error) {
	out := map[uuid.UUID]int{}
	for _, page := range list {
		people, err := watches.Who(ctx, s.pool, page, spaceID)
		if err != nil {
			return nil, err
		}
		for _, person := range people {
			if mustView {
				d, err := s.perms.CheckPage(ctx, s.pool, person, page, permissions.ActionPageView, uuid.Nil)
				if err != nil || !d.Allowed {
					if err != nil && !apperr.IsNotFound(err) {
						return nil, err
					}
					continue
				}
			}
			out[person]++
		}
	}
	return out, nil
}
