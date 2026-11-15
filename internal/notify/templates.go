// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"context"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/watches"
)

// Notifications of guided templates (spec 074, TPL-720 to TPL-723). Rows hold ids and counts,
// never an answer of a form: the words come when a row is shown or mailed, to those who may
// still see what it is about.

// onTemplateDecision tells the people a proposal or a decision about a template is for: those
// who may approve a proposal, the author of a decision, and the author and the managers of a
// withdrawal (TPL-722). A row hangs on the template's hidden page, so it shows only while the
// person still owns the template or looks after it. The first decision closes a proposal for
// every approver: their rows about it are marked read.
func (s *Service) onTemplateDecision(ctx context.Context, event uuid.UUID, p payload) error {
	kind, reason := TypeTemplateDecided, ReasonAuthor
	if p.Decision == "proposed" {
		kind, reason = TypeTemplateProposed, ReasonAsked
	} else if _, err := s.pool.Exec(ctx, `UPDATE notifications SET read_at = now()
		WHERE type = $1 AND page_id = $2 AND read_at IS NULL`, TypeTemplateProposed, p.PageID); err != nil {
		return err
	}
	for _, person := range p.UserIDs {
		if err := s.Deliver(ctx, s.pool, Row{UserID: person, Type: kind, Reason: reason, ActorID: p.ActorID, PageID: p.PageID,
			SpaceID: p.SpaceID, Event: event, AboutTemplate: true,
			Data: map[string]any{"template_id": p.TemplateID, "version": p.Version, "decision": p.Decision}}); err != nil {
			return err
		}
	}
	return nil
}

// onSetCreated tells of a set once (TPL-720): its creator, when a set made in the background
// is ready (TPL-1433), and, when its pages are published, each watcher of where it went with
// the number of its pages they may see. A watcher who may see none is not told.
func (s *Service) onSetCreated(ctx context.Context, event uuid.UUID, p payload) error {
	if p.SetID == nil || len(p.PageIDs) == 0 {
		return nil
	}
	root := p.PageIDs[0]
	if p.Background && p.ActorID != uuid.Nil {
		// No actor: the creator is told of their own set whatever they chose for their own work.
		if err := s.Deliver(ctx, s.pool, Row{UserID: p.ActorID, Type: TypeTemplateSet, Reason: ReasonAuthor, PageID: root,
			SpaceID: p.SpaceID, Event: event, Data: map[string]any{"set_id": *p.SetID, "pages": len(p.PageIDs), "ready": true}}); err != nil {
			return err
		}
	}
	if !p.Published {
		return nil
	}
	watchers, err := watches.Who(ctx, s.pool, root, p.SpaceID)
	if err != nil {
		return err
	}
	for _, person := range watchers {
		first, seen, err := s.pagesSeen(ctx, person, p.PageIDs)
		if err != nil {
			return err
		}
		if seen == 0 {
			continue
		}
		if err := s.Deliver(ctx, s.pool, Row{UserID: person, Type: TypeTemplateSet, Reason: ReasonWatching, ActorID: p.ActorID,
			PageID: first, SpaceID: p.SpaceID, Event: event, Data: map[string]any{"set_id": *p.SetID, "pages": seen}}); err != nil {
			return err
		}
	}
	return nil
}

// pagesSeen counts the pages of a list a person may view, and names the first of them.
func (s *Service) pagesSeen(ctx context.Context, person uuid.UUID, list []uuid.UUID) (uuid.UUID, int, error) {
	first, seen := uuid.Nil, 0
	for _, page := range list {
		d, err := s.perms.CheckPage(ctx, s.pool, person, page, permissions.ActionPageView, uuid.Nil)
		if err != nil && !apperr.IsNotFound(err) {
			return first, seen, err
		}
		if err != nil || !d.Allowed {
			continue
		}
		if seen == 0 {
			first = page
		}
		seen++
	}
	return first, seen, nil
}

// tellSetMentions tells each person a published page of a set mentions, once for the whole
// set: the first page makes the row, and each later page that mentions them, and that they may
// see, is added to its list (TPL-721).
func (s *Service) tellSetMentions(ctx context.Context, base Row, p payload) error {
	members, err := s.groupMembers(ctx, p.MentionedGroups)
	if err != nil {
		return err
	}
	event := uuid.NewSHA1(*p.SetID, []byte("mentions"))
	told := map[uuid.UUID]bool{}
	for _, person := range append(append([]uuid.UUID{}, p.Mentions...), members...) {
		if told[person] {
			continue
		}
		told[person] = true
		r := base
		r.UserID, r.Type, r.Reason, r.Event = person, TypeMention, ReasonMentioned, event
		r.Data = map[string]any{"set_id": *p.SetID, "pages": []string{p.PageID.String()}}
		may, err := s.mayHear(ctx, s.pool, r)
		if err != nil {
			return err
		}
		if !may {
			continue
		}
		if err := s.Deliver(ctx, s.pool, r); err != nil {
			return err
		}
		if _, err := s.pool.Exec(ctx, `UPDATE notifications
			SET data = jsonb_set(data, '{pages}', COALESCE(data->'pages', '[]'::jsonb) || to_jsonb($3::text))
			WHERE user_id = $1 AND source_event = $2 AND NOT COALESCE(data->'pages', '[]'::jsonb) ? $3`,
			person, event, p.PageID.String()); err != nil {
			return err
		}
	}
	return nil
}
