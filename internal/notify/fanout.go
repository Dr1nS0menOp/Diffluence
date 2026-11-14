// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/comments"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/tasks"
	"github.com/diffluence/diffluence/internal/watches"
)

// payload is what the events of pages, comments and requests carry. Ids only.
type payload struct {
	PageID          uuid.UUID   `json:"page_id"`
	SpaceID         uuid.UUID   `json:"space_id"`
	ActorID         uuid.UUID   `json:"actor_id"`
	CommentID       uuid.UUID   `json:"comment_id"`
	ParentID        *uuid.UUID  `json:"parent_id"`
	Silent          bool        `json:"silent"`
	First           bool        `json:"first"`
	EditorsOnly     bool        `json:"editors_only"`
	Mentions        []uuid.UUID `json:"mentions"`
	MentionedGroups []uuid.UUID `json:"mentioned_groups"`
	Assigned        []uuid.UUID `json:"assigned"`
	OwnerID         uuid.UUID   `json:"owner_id"`
	TargetType      string      `json:"target_type"`
	TargetID        uuid.UUID   `json:"target_id"`
	RequestID       uuid.UUID   `json:"request_id"`
	AskedID         uuid.UUID   `json:"asked_id"`
	RequesterID     uuid.UUID   `json:"requester_id"`
	Version         int         `json:"version"`
	UserID          uuid.UUID   `json:"user_id"`
	ExportID        uuid.UUID   `json:"export_id"`
	Filename        string      `json:"filename"`
	ObjectID        string      `json:"object_id"`
	// UserIDs are the people a review event is for: the reviewers asked, or the authors.
	UserIDs []uuid.UUID `json:"user_ids"`
	Answer  string      `json:"answer"`
	Outcome string      `json:"outcome"`
	Rule    string      `json:"rule"`
	Needed  int         `json:"needed"`
}

// Handle turns one event into notifications. It runs in the worker, and may run more
// than once for the same event: a row is unique per person and event.
func (s *Service) Handle(ctx context.Context, m outbox.Message) error {
	handlers := map[string]func(context.Context, uuid.UUID, payload) error{
		"page.published":        s.onPublished,
		"comment.created":       s.onComment,
		"comment.edited":        s.onCommentEdited,
		"reaction.added":        s.onReaction,
		"access.requested":      s.onAccessRequested,
		"access.allowed":        s.onAccessAnswered,
		"access.refused":        s.onAccessAnswered,
		"export.finished":       s.onExportFinished,
		"access.recovered":      s.onAccessRecovered,
		"page.draft_expiring":   s.onDraftExpiring,
		"page.review_asked":     s.onReviewAsked,
		"page.review_answered":  s.onReviewAnswered,
		"page.review_ended":     s.onReviewEnded,
		"page.reviewer_removed": s.onReviewerRemoved,
	}
	handle, ok := handlers[m.Type]
	if !ok {
		return nil
	}
	var p payload
	if err := json.Unmarshal(m.Payload, &p); err != nil {
		return fmt.Errorf("notify: read %s: %w", m.Type, err)
	}
	return handle(ctx, m.ID, p)
}

// Publish lets the outbox relay hand events straight to the handler, which tests use
// instead of the queue.
func (s *Service) Publish(ctx context.Context, _ string, data []byte, _ string) error {
	var m outbox.Message
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	return s.Handle(ctx, m)
}

// onExportFinished tells the person who asked for a file that it is there. The row
// hangs on a page in the file: when they lose that page, the row goes with it.
func (s *Service) onExportFinished(ctx context.Context, event uuid.UUID, p payload) error {
	return s.Deliver(ctx, s.pool, Row{UserID: p.UserID, Type: TypeExportReady, Reason: ReasonExport, PageID: p.PageID, SpaceID: p.SpaceID, Event: event,
		Data: map[string]any{"export_id": p.ExportID, "filename": p.Filename}})
}

func (s *Service) autoWatch(ctx context.Context, user, pageID uuid.UUID, want func(Settings) bool) error {
	if user == uuid.Nil {
		return nil
	}
	choices, err := load(ctx, s.pool, user)
	if err != nil || !want(choices) {
		return err
	}
	return s.watches.Auto(ctx, user, pageID)
}

// groupMembers expands the groups of a mention, when mentioning groups is allowed.
func (s *Service) groupMembers(ctx context.Context, groups []uuid.UUID) ([]uuid.UUID, error) {
	if len(groups) == 0 {
		return nil, nil
	}
	values, err := s.settings.Get(ctx)
	if err != nil || !values.GroupMentions {
		return nil, err
	}
	out := []uuid.UUID{}
	for _, group := range groups {
		rows, err := s.pool.Query(ctx, "SELECT user_id FROM group_members WHERE group_id = $1 LIMIT $2", group, maxGroupMembers+1)
		if err != nil {
			return nil, err
		}
		members, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return nil, err
		}
		if len(members) <= maxGroupMembers {
			out = append(out, members...)
		}
	}
	return out, nil
}

func (s *Service) tell(ctx context.Context, people []uuid.UUID, base Row, kind, reason string) error {
	for _, person := range people {
		r := base
		r.UserID, r.Type, r.Reason = person, kind, reason
		if err := s.Deliver(ctx, s.pool, r); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) tellMentions(ctx context.Context, base Row, p payload) error {
	members, err := s.groupMembers(ctx, p.MentionedGroups)
	if err != nil {
		return err
	}
	return s.tell(ctx, append(append([]uuid.UUID{}, p.Mentions...), members...), base, TypeMention, ReasonMentioned)
}

// onPublished tells assigned and mentioned people, then watchers. A row is unique per
// person and event, so whoever fits the first gets that, the most personal one. A
// silent update reaches no watcher, and a new mention still reaches its person.
func (s *Service) onPublished(ctx context.Context, event uuid.UUID, p payload) error {
	want := func(v Settings) bool { return v.AutoWatchEdit }
	if p.First {
		want = func(v Settings) bool { return v.AutoWatchCreate }
	}
	if !p.Silent {
		if err := s.autoWatch(ctx, p.ActorID, p.PageID, want); err != nil {
			return err
		}
	}
	base := Row{ActorID: p.ActorID, PageID: p.PageID, SpaceID: p.SpaceID, Event: event}
	if err := s.tell(ctx, p.Assigned, base, TypeTaskAssigned, ReasonAssigned); err != nil {
		return err
	}
	if err := s.tellMentions(ctx, base, p); err != nil {
		return err
	}
	if p.Silent {
		return nil
	}
	watchers, err := watches.Who(ctx, s.pool, p.PageID, p.SpaceID)
	if err != nil {
		return err
	}
	base.Data = map[string]any{"version": p.Version}
	return s.tell(ctx, watchers, base, TypePageChanged, ReasonWatching)
}

func (s *Service) onComment(ctx context.Context, event uuid.UUID, p payload) error {
	if err := s.autoWatch(ctx, p.ActorID, p.PageID, func(v Settings) bool { return v.AutoWatchComment }); err != nil {
		return err
	}
	base := Row{ActorID: p.ActorID, PageID: p.PageID, SpaceID: p.SpaceID, CommentID: &p.CommentID, Event: event, NeedsEdit: p.EditorsOnly}
	if err := s.tellMentions(ctx, base, p); err != nil {
		return err
	}
	if p.ParentID != nil {
		people, err := comments.ThreadPeople(ctx, s.pool, *p.ParentID)
		if err != nil {
			return err
		}
		if err := s.tell(ctx, people, base, TypeReply, ReasonThread); err != nil {
			return err
		}
	}
	watchers, err := watches.Who(ctx, s.pool, p.PageID, p.SpaceID)
	if err != nil {
		return err
	}
	return s.tell(ctx, watchers, base, TypeComment, ReasonWatching)
}

func (s *Service) onCommentEdited(ctx context.Context, event uuid.UUID, p payload) error {
	base := Row{ActorID: p.ActorID, PageID: p.PageID, SpaceID: p.SpaceID, CommentID: &p.CommentID, Event: event, NeedsEdit: p.EditorsOnly}
	return s.tellMentions(ctx, base, p)
}

func (s *Service) onReaction(ctx context.Context, event uuid.UUID, p payload) error {
	base := Row{UserID: p.OwnerID, Type: TypeReaction, Reason: ReasonAuthor, ActorID: p.ActorID, PageID: p.PageID, SpaceID: p.SpaceID, Event: event}
	if p.TargetType == comments.TargetComment {
		base.CommentID = &p.TargetID
	}
	return s.Deliver(ctx, s.pool, base)
}

func (s *Service) onAccessRequested(ctx context.Context, event uuid.UUID, p payload) error {
	return s.Deliver(ctx, s.pool, Row{UserID: p.AskedID, Type: TypeAccessRequest, Reason: ReasonAsked, ActorID: p.ActorID,
		PageID: p.PageID, SpaceID: p.SpaceID, Event: event, Data: map[string]any{"request_id": p.RequestID}})
}

func (s *Service) onAccessAnswered(ctx context.Context, event uuid.UUID, p payload) error {
	return s.Deliver(ctx, s.pool, Row{UserID: p.RequesterID, Type: TypeAccessAnswer, Reason: ReasonAsked, ActorID: p.ActorID,
		PageID: p.PageID, SpaceID: p.SpaceID, Event: event})
}

// onAccessRecovered tells the owner of a page that an administrator took access to
// it, because that is the one person a restriction was meant to answer to (ACC-071).
// The event also covers spaces, which have no such owner.
func (s *Service) onAccessRecovered(ctx context.Context, event uuid.UUID, p payload) error {
	pageID, err := uuid.Parse(p.ObjectID)
	if err != nil {
		return fmt.Errorf("notify: read access.recovered: %w", err)
	}
	var owner *uuid.UUID
	var spaceID uuid.UUID
	err = s.pool.QueryRow(ctx, "SELECT owner_id, space_id FROM pages WHERE id = $1", pageID).Scan(&owner, &spaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil || owner == nil {
		return err
	}
	return s.Deliver(ctx, s.pool, Row{UserID: *owner, Type: TypeAccessRecovered, Reason: ReasonAuthor, ActorID: p.ActorID,
		PageID: pageID, SpaceID: spaceID, Event: event})
}

// onDraftExpiring warns the person whose draft is about to go (PAG-019).
func (s *Service) onDraftExpiring(ctx context.Context, event uuid.UUID, p payload) error {
	var spaceID uuid.UUID
	err := s.pool.QueryRow(ctx, "SELECT space_id FROM pages WHERE id = $1", p.PageID).Scan(&spaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.Deliver(ctx, s.pool, Row{UserID: p.UserID, Type: TypeDraftExpiring, Reason: ReasonAuthor, PageID: p.PageID, SpaceID: spaceID, Event: event})
}

// onReviewAsked asks each reviewer for their answer, through the reading being asked gives
// them, so a reviewer outside a restriction hears of it and nobody else does (DOC-600).
func (s *Service) onReviewAsked(ctx context.Context, event uuid.UUID, p payload) error {
	base := Row{ActorID: p.ActorID, PageID: p.PageID, SpaceID: p.SpaceID, Event: event, NeedsReview: true,
		Data: map[string]any{"rule": p.Rule, "needed": p.Needed}}
	return s.tell(ctx, p.UserIDs, base, TypeReviewAsked, ReasonAsked)
}

// onReviewAnswered tells the authors of the change each answer. It reaches only people who
// may still edit the page, because it is about the working version (DOC-602, DOC-604).
func (s *Service) onReviewAnswered(ctx context.Context, event uuid.UUID, p payload) error {
	base := Row{ActorID: p.ActorID, PageID: p.PageID, SpaceID: p.SpaceID, Event: event, NeedsEdit: true,
		Data: map[string]any{"answer": p.Answer}}
	return s.tell(ctx, p.UserIDs, base, TypeReviewAnswered, ReasonAuthor)
}

// onReviewEnded tells the authors how the review ended (DOC-602).
func (s *Service) onReviewEnded(ctx context.Context, event uuid.UUID, p payload) error {
	base := Row{ActorID: p.ActorID, PageID: p.PageID, SpaceID: p.SpaceID, Event: event, NeedsEdit: true,
		Data: map[string]any{"outcome": p.Outcome}}
	return s.tell(ctx, p.UserIDs, base, TypeReviewEnded, ReasonAuthor)
}

// onReviewerRemoved tells both sides (DOC-608): the person taken off, as far as they may
// still see the page, and the authors.
func (s *Service) onReviewerRemoved(ctx context.Context, event uuid.UUID, p payload) error {
	base := Row{ActorID: p.ActorID, PageID: p.PageID, SpaceID: p.SpaceID, Event: event}
	if err := s.tell(ctx, []uuid.UUID{p.UserID}, base, TypeReviewerRemoved, ReasonAsked); err != nil {
		return err
	}
	base.NeedsEdit = true
	authors := slices.DeleteFunc(slices.Clone(p.UserIDs), func(id uuid.UUID) bool { return id == p.UserID })
	return s.tell(ctx, authors, base, TypeReviewerRemoved, ReasonAuthor)
}

// RemindDue tells people about tasks that reached their date, once.
func (s *Service) RemindDue(ctx context.Context) error {
	today := s.now().UTC().Format("2006-01-02")
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		due, err := tasks.TakeDue(ctx, tx, today)
		if err != nil {
			return err
		}
		for _, d := range due {
			r := Row{UserID: d.Assignee, Type: TypeTaskDue, Reason: ReasonAssigned, PageID: d.PageID, SpaceID: d.SpaceID, Event: d.ID}
			if err := s.Deliver(ctx, tx, r); err != nil {
				return err
			}
		}
		return nil
	})
}
