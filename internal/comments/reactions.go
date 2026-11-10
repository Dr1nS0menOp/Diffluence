// SPDX-License-Identifier: Apache-2.0

package comments

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
)

// What a reaction can be on.
const (
	TargetPage    = "page"
	TargetComment = "comment"
)

const (
	maxEmojiRunes     = 16
	maxEmojiPerTarget = 20
	maxNamesShown     = 20
)

// ReactionCount is one emoji on something, and whether the caller added it.
type ReactionCount struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Mine  bool   `json:"mine"`
}

// Reactor is who reacted with one emoji.
type Reactor struct {
	Emoji string   `json:"emoji"`
	Names []string `json:"names"`
	More  int      `json:"more"`
}

// isEmoji refuses anything that reads as text or markup. Our list of emoji lives in
// the browser, and this only has to keep a reaction from carrying words.
func isEmoji(s string) bool {
	if s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxEmojiRunes {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune(`<>&"'`, r) {
			return false
		}
	}
	return true
}

type reactionTarget struct {
	kind    string
	id      uuid.UUID
	pageID  uuid.UUID
	spaceID uuid.UUID
	owner   uuid.UUID
}

// resolveTarget checks that the caller can see what they react to, and that reactions
// are on. Both the space and the instance can turn them off (spec 010, C4).
func (s *Service) resolveTarget(ctx context.Context, q db.Querier, c identity.Caller, kind string, id uuid.UUID) (reactionTarget, error) {
	t := reactionTarget{kind: kind, id: id, pageID: id}
	if kind == TargetComment {
		v, err := load(ctx, q, id)
		if err != nil {
			return t, err
		}
		if err := s.visible(ctx, q, c, v); err != nil {
			return t, err
		}
		if v.trashed || v.purged {
			return t, apperr.NotFound()
		}
		t.pageID, t.spaceID, t.owner = v.PageID, v.spaceID, v.AuthorID
		return t, nil
	}
	if err := s.perms.RequirePage(ctx, q, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return t, err
	}
	err := q.QueryRow(ctx, "SELECT space_id, created_by FROM pages WHERE id = $1", id).Scan(&t.spaceID, &t.owner)
	return t, err
}

func (s *Service) reactionsOn(ctx context.Context, q db.Querier, spaceID uuid.UUID) (bool, error) {
	values, err := s.settings.GetIn(ctx, q)
	if err != nil {
		return false, err
	}
	on, err := s.features.Enabled(ctx, q, spaceID, features.Reactions)
	if err != nil {
		return false, err
	}
	return values.ReactionsEnabled && on, nil
}

// SetReaction adds or removes the caller's own reaction.
func (s *Service) SetReaction(ctx context.Context, c identity.Caller, kind string, id uuid.UUID, emoji string, on bool) error {
	if !isEmoji(emoji) {
		return apperr.Invalid("A reaction is one emoji.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		t, err := s.resolveTarget(ctx, tx, c, kind, id)
		if err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, t.pageID, permissions.ActionReact, uuid.Nil); err != nil {
			return err
		}
		enabled, err := s.reactionsOn(ctx, tx, t.spaceID)
		if err != nil {
			return err
		}
		if !enabled {
			return apperr.Conflict("reactions_off", "Reactions are turned off here.")
		}
		if !on {
			return s.dropReaction(ctx, tx, c, t, emoji)
		}
		return s.addReaction(ctx, tx, c, t, emoji)
	})
}

func (s *Service) addReaction(ctx context.Context, tx pgx.Tx, c identity.Caller, t reactionTarget, emoji string) error {
	var kinds int
	var known bool
	err := tx.QueryRow(ctx, `SELECT count(DISTINCT emoji), COALESCE(bool_or(emoji = $3), false)
		FROM reactions WHERE target_type = $1 AND target_id = $2`, t.kind, t.id, emoji).Scan(&kinds, &known)
	if err != nil {
		return err
	}
	if !known && kinds >= maxEmojiPerTarget {
		return apperr.Conflict("too_many_reactions", "This has as many kinds of reaction as it can hold.")
	}
	tag, err := tx.Exec(ctx, `INSERT INTO reactions (target_type, target_id, user_id, emoji) VALUES ($1, $2, $3, $4)
		ON CONFLICT DO NOTHING`, t.kind, t.id, c.UserID, emoji)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	return recordReaction(ctx, tx, c, "reaction.added", t, emoji)
}

func (s *Service) dropReaction(ctx context.Context, tx pgx.Tx, c identity.Caller, t reactionTarget, emoji string) error {
	tag, err := tx.Exec(ctx, "DELETE FROM reactions WHERE target_type = $1 AND target_id = $2 AND user_id = $3 AND emoji = $4",
		t.kind, t.id, c.UserID, emoji)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	return recordReaction(ctx, tx, c, "reaction.removed", t, emoji)
}

func recordReaction(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, t reactionTarget, emoji string) error {
	if _, err := audit.Record(ctx, tx, c.Event(action, t.kind, t.id.String(), t.spaceID.String(), map[string]any{"emoji": emoji})); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"target_type": t.kind, "target_id": t.id, "page_id": t.pageID,
		"space_id": t.spaceID, "actor_id": c.UserID, "owner_id": t.owner})
	return err
}

// Reactions lists the counters on something, or nothing when reactions are off.
func (s *Service) Reactions(ctx context.Context, c identity.Caller, kind string, id uuid.UUID) ([]ReactionCount, error) {
	t, err := s.resolveTarget(ctx, s.pool, c, kind, id)
	if err != nil {
		return nil, err
	}
	enabled, err := s.reactionsOn(ctx, s.pool, t.spaceID)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return []ReactionCount{}, nil
	}
	got, err := s.counts(ctx, c.UserID, kind, []uuid.UUID{id})
	if err != nil {
		return nil, err
	}
	if list := got[id]; list != nil {
		return list, nil
	}
	return []ReactionCount{}, nil
}

func (s *Service) counts(ctx context.Context, user uuid.UUID, kind string, targets []uuid.UUID) (map[uuid.UUID][]ReactionCount, error) {
	rows, err := s.pool.Query(ctx, `SELECT target_id, emoji, count(*), bool_or(user_id = $3)
		FROM reactions WHERE target_type = $1 AND target_id = ANY($2)
		GROUP BY target_id, emoji ORDER BY target_id, min(created_at)`, kind, targets, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID][]ReactionCount{}
	for rows.Next() {
		var id uuid.UUID
		var r ReactionCount
		if err := rows.Scan(&id, &r.Emoji, &r.Count, &r.Mine); err != nil {
			return nil, err
		}
		out[id] = append(out[id], r)
	}
	return out, rows.Err()
}

// addReactions fills in the counters of a whole page of comments at once.
func (s *Service) addReactions(ctx context.Context, c identity.Caller, p *Page) error {
	var pageID uuid.UUID
	targets := []uuid.UUID{}
	collect := func(list []Comment) {
		for _, t := range list {
			pageID = t.PageID
			targets = append(targets, t.ID)
			for _, r := range t.Replies {
				targets = append(targets, r.ID)
			}
		}
	}
	collect(p.Threads)
	collect(p.Inline)
	if len(targets) == 0 {
		return nil
	}
	spaceID, err := s.spaceOf(ctx, s.pool, pageID)
	if err != nil {
		return err
	}
	enabled, err := s.reactionsOn(ctx, s.pool, spaceID)
	if err != nil || !enabled {
		return err
	}
	counts, err := s.counts(ctx, c.UserID, TargetComment, targets)
	if err != nil {
		return err
	}
	fill := func(list []Comment) {
		for i := range list {
			if got := counts[list[i].ID]; got != nil {
				list[i].Reactions = got
			}
			for j := range list[i].Replies {
				if got := counts[list[i].Replies[j].ID]; got != nil {
					list[i].Replies[j].Reactions = got
				}
			}
		}
	}
	fill(p.Threads)
	fill(p.Inline)
	return nil
}

// Reactors says who reacted, up to a limit per emoji.
func (s *Service) Reactors(ctx context.Context, c identity.Caller, kind string, id uuid.UUID) ([]Reactor, error) {
	if _, err := s.resolveTarget(ctx, s.pool, c, kind, id); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT r.emoji, COALESCE(u.display_name, '') FROM reactions r
		LEFT JOIN users u ON u.id = r.user_id WHERE r.target_type = $1 AND r.target_id = $2
		ORDER BY r.emoji, r.created_at`, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Reactor{}
	for rows.Next() {
		var emoji, name string
		if err := rows.Scan(&emoji, &name); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].Emoji != emoji {
			out = append(out, Reactor{Emoji: emoji, Names: []string{}})
		}
		last := &out[len(out)-1]
		if len(last.Names) < maxNamesShown {
			last.Names = append(last.Names, name)
		} else {
			last.More++
		}
	}
	return out, rows.Err()
}

// Author says who wrote a comment, for notifications.
func Author(ctx context.Context, q db.Querier, id uuid.UUID) (uuid.UUID, error) {
	var author uuid.UUID
	err := q.QueryRow(ctx, "SELECT author_id FROM comments WHERE id = $1", id).Scan(&author)
	if errors.Is(err, pgx.ErrNoRows) {
		return author, apperr.NotFound()
	}
	return author, err
}

// ThreadPeople lists who wrote in a thread, for replies.
func ThreadPeople(ctx context.Context, q db.Querier, root uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, "SELECT DISTINCT author_id FROM comments WHERE id = $1 OR parent_id = $1", root)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}
