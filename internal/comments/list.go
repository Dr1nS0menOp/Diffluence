// SPDX-License-Identifier: Apache-2.0

package comments

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Order of a list of comments.
const (
	OrderOldest = "oldest"
	OrderNewest = "newest"
)

// ListOptions asks for a part of the threads.
type ListOptions struct {
	Limit  int
	Offset int
	Order  string
}

// Page is everything the comments of a page need.
type Page struct {
	Threads []Comment `json:"threads"`
	Inline  []Comment `json:"inline"`
	Total   int       `json:"total"`
	Unread  int       `json:"unread"`
	Closed  bool      `json:"closed"`
	SeenAt  time.Time `json:"seen_at"`
}

type asker struct {
	user     uuid.UUID
	canEdit  bool
	canAdmin bool
	canAny   bool
	canOwn   bool
	seen     time.Time
}

func (s *Service) asker(ctx context.Context, c identity.Caller, pageID uuid.UUID) (asker, error) {
	a := asker{user: c.UserID}
	u, err := s.perms.LoadUser(ctx, s.pool, c.UserID)
	if err != nil {
		return a, err
	}
	space, page, err := s.perms.LoadPage(ctx, s.pool, pageID)
	if err != nil {
		return a, err
	}
	allowed := func(action string, owner uuid.UUID) bool {
		return permissions.Evaluate(permissions.Input{User: u, Space: space, Page: &page, Action: action, Owner: owner}).Allowed
	}
	a.canEdit = allowed(permissions.PageEdit, uuid.Nil)
	a.canAdmin = allowed(permissions.SpaceAdminister, uuid.Nil)
	a.canAny = allowed(permissions.ActionCommentDelete, uuid.Nil)
	a.canOwn = allowed(permissions.ActionCommentDelete, c.UserID)
	err = s.pool.QueryRow(ctx, "SELECT COALESCE((SELECT seen_at FROM comment_seen WHERE user_id = $1 AND page_id = $2), 'epoch')", c.UserID, pageID).Scan(&a.seen)
	return a, err
}

func (a asker) can(v row) Can {
	mine := v.AuthorID == a.user
	gone := v.trashed || v.purged
	return Can{
		Edit:    !gone && (mine || a.canAdmin),
		Delete:  !gone && (a.canAny || (mine && a.canOwn)),
		Resolve: a.canEdit && v.ParentID == nil,
	}
}

// showable leaves out what the asker may not see, and what has nothing to show.
func (a asker) showable(v row, hasReplies bool) bool {
	if v.EditorsOnly && !a.canEdit {
		return false
	}
	return !(v.trashed || v.purged) || hasReplies
}

func (a asker) view(v row, names map[uuid.UUID]string) (Comment, error) {
	out := v.Comment
	out.Can = a.can(v)
	out.Unread = v.CreatedAt.After(a.seen) && v.AuthorID != a.user
	out.Reactions = []ReactionCount{}
	out.Replies = []Comment{}
	if v.trashed || v.purged {
		out.Removed = true
		out.Body = nil
		out.Anchor = nil
		return out, nil
	}
	if !document.HasMentions(out.Body) {
		return out, nil
	}
	n, err := document.Parse(out.Body)
	if err != nil {
		return out, err
	}
	document.FillMentionNames(&n, names)
	raw, err := document.Marshal(n)
	out.Body = json.RawMessage(raw)
	return out, err
}

func mentionedIn(list []row) []uuid.UUID {
	out := []uuid.UUID{}
	for _, v := range list {
		if !document.HasMentions(v.Body) {
			continue
		}
		if n, err := document.Parse(v.Body); err == nil {
			out = append(out, document.MentionedUsers(n)...)
			out = append(out, document.MentionedGroups(n)...)
		}
	}
	return out
}

func (s *Service) rows(ctx context.Context, pageID uuid.UUID) ([]row, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+columns+fromComments+"WHERE c.page_id = $1 ORDER BY c.created_at, c.id", pageID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) { return scan(r) })
}

func window(list []Comment, opts ListOptions) []Comment {
	limit := opts.Limit
	if limit <= 0 || limit > maxThreads {
		limit = defaultThreads
	}
	from := min(max(opts.Offset, 0), len(list))
	return list[from:min(from+limit, len(list))]
}

// List returns the comments of a page: page comments in parts, and every inline
// thread, because the highlights in the text need them all.
func (s *Service) List(ctx context.Context, c identity.Caller, pageID uuid.UUID, opts ListOptions) (Page, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return Page{}, err
	}
	a, err := s.asker(ctx, c, pageID)
	if err != nil {
		return Page{}, err
	}
	all, err := s.rows(ctx, pageID)
	if err != nil {
		return Page{}, err
	}
	names, err := identity.DisplayNames(ctx, s.pool, mentionedIn(all))
	if err != nil {
		return Page{}, err
	}
	replies := map[uuid.UUID][]row{}
	for _, v := range all {
		if v.ParentID != nil {
			replies[*v.ParentID] = append(replies[*v.ParentID], v)
		}
	}
	out := Page{Threads: []Comment{}, Inline: []Comment{}, SeenAt: a.seen}
	for _, v := range all {
		if v.ParentID != nil {
			continue
		}
		thread, ok, err := a.thread(v, replies[v.ID], names, &out.Unread)
		if err != nil {
			return out, err
		}
		if !ok {
			continue
		}
		if v.Anchor != nil {
			out.Inline = append(out.Inline, thread)
		} else {
			out.Threads = append(out.Threads, thread)
		}
	}
	if len(out.Inline) > maxInlineThreads {
		out.Inline = out.Inline[len(out.Inline)-maxInlineThreads:]
	}
	if opts.Order == OrderNewest {
		for i, j := 0, len(out.Threads)-1; i < j; i, j = i+1, j-1 {
			out.Threads[i], out.Threads[j] = out.Threads[j], out.Threads[i]
		}
	}
	out.Total = len(out.Threads)
	out.Threads = window(out.Threads, opts)
	closed, err := s.pages.CommentsClosed(ctx, s.pool, pageID)
	if err != nil {
		return out, err
	}
	out.Closed = closed
	return out, s.addReactions(ctx, c, &out)
}

func (a asker) thread(root row, replyRows []row, names map[uuid.UUID]string, unread *int) (Comment, bool, error) {
	live := []row{}
	for _, r := range replyRows {
		if a.showable(r, false) {
			live = append(live, r)
		}
	}
	if !a.showable(root, len(live) > 0) {
		return Comment{}, false, nil
	}
	out, err := a.view(root, names)
	if err != nil {
		return out, false, err
	}
	if out.Unread {
		*unread++
	}
	for _, r := range live {
		reply, err := a.view(r, names)
		if err != nil {
			return out, false, err
		}
		if reply.Unread {
			*unread++
		}
		out.Replies = append(out.Replies, reply)
	}
	return out, true, nil
}
