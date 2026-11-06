// SPDX-License-Identifier: Apache-2.0

// Package comments owns comments on pages and pieces of text (spec 010, C1 to C3).
// Every method takes the Caller and checks permissions itself.
package comments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/trash"
)

// Thread statuses.
const (
	StatusOpen     = "open"
	StatusResolved = "resolved"
)

// The kinds of anchor.
const (
	AnchorText  = "text"
	AnchorImage = "image"
)

const (
	maxExactRunes    = 1000
	maxContextRunes  = 32
	maxPerMinute     = 30
	defaultThreads   = 30
	maxThreads       = 100
	maxInlineThreads = 500
	trashTitleRunes  = 80
)

// Anchor points a thread at text or an image. It is kept beside the document, so a
// reader can comment without a new version (spec 010, D1).
type Anchor struct {
	Kind         string     `json:"kind"`
	Exact        string     `json:"exact,omitempty"`
	Prefix       string     `json:"prefix,omitempty"`
	Suffix       string     `json:"suffix,omitempty"`
	Offset       int        `json:"offset,omitempty"`
	Version      int        `json:"version,omitempty"`
	AttachmentID *uuid.UUID `json:"attachment_id,omitempty"`
}

// Can says what the caller may do with a comment.
type Can struct {
	Edit    bool `json:"edit"`
	Delete  bool `json:"delete"`
	Resolve bool `json:"resolve"`
}

// Comment is one comment, with its replies when it starts a thread.
type Comment struct {
	ID           uuid.UUID       `json:"id"`
	PageID       uuid.UUID       `json:"page_id"`
	ParentID     *uuid.UUID      `json:"parent_id"`
	AuthorID     uuid.UUID       `json:"author_id"`
	AuthorName   string          `json:"author_name"`
	AuthorActive bool            `json:"author_active"`
	Body         json.RawMessage `json:"body"`
	Anchor       *Anchor         `json:"anchor"`
	Status       string          `json:"status"`
	EditorsOnly  bool            `json:"editors_only"`
	Removed      bool            `json:"removed"`
	Unread       bool            `json:"unread"`
	CreatedAt    time.Time       `json:"created_at"`
	EditedAt     *time.Time      `json:"edited_at"`
	ResolvedAt   *time.Time      `json:"resolved_at"`
	ResolvedBy   string          `json:"resolved_by"`
	Reactions    []ReactionCount `json:"reactions"`
	Can          Can             `json:"can"`
	Replies      []Comment       `json:"replies"`
}

// Service manages comments.
type Service struct {
	pool       *pgxpool.Pool
	perms      *permissions.Service
	settings   *settings.Service
	features   *features.Service
	instanceID uuid.UUID
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, st *settings.Service, f *features.Service, instanceID uuid.UUID) *Service {
	return &Service{pool: pool, perms: perms, settings: st, features: f, instanceID: instanceID}
}

type row struct {
	Comment
	spaceID   uuid.UUID
	trashed   bool
	purged    bool
	rawAnchor []byte
}

const columns = `c.id, c.page_id, c.parent_id, c.space_id, c.author_id, COALESCE(u.display_name, ''), COALESCE(u.status = 'active', false),
	c.body, c.anchor, c.status, c.editors_only, c.trash_item_id IS NOT NULL, c.purged, c.created_at, c.edited_at, c.resolved_at,
	COALESCE(r.display_name, '')`

const fromComments = ` FROM comments c
	LEFT JOIN users u ON u.id = c.author_id
	LEFT JOIN users r ON r.id = c.resolved_by `

func scan(r pgx.Row) (row, error) {
	var v row
	err := r.Scan(&v.ID, &v.PageID, &v.ParentID, &v.spaceID, &v.AuthorID, &v.AuthorName, &v.AuthorActive,
		&v.Body, &v.rawAnchor, &v.Status, &v.EditorsOnly, &v.trashed, &v.purged, &v.CreatedAt, &v.EditedAt, &v.ResolvedAt, &v.ResolvedBy)
	if err != nil {
		return v, err
	}
	if v.rawAnchor != nil {
		v.Anchor = &Anchor{}
		if err := json.Unmarshal(v.rawAnchor, v.Anchor); err != nil {
			return v, fmt.Errorf("comments: read anchor: %v", err)
		}
	}
	return v, nil
}

func load(ctx context.Context, q db.Querier, id uuid.UUID) (row, error) {
	v, err := scan(q.QueryRow(ctx, "SELECT "+columns+fromComments+"WHERE c.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, apperr.NotFound()
	}
	return v, err
}

func record(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, v row, payload map[string]any) error {
	if _, err := audit.Record(ctx, tx, c.Event(action, "comment", v.ID.String(), v.spaceID.String(), map[string]any{"page_id": v.PageID})); err != nil {
		return err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["comment_id"], payload["page_id"], payload["space_id"], payload["actor_id"] = v.ID, v.PageID, v.spaceID, c.UserID
	_, err := outbox.Enqueue(ctx, tx, action, payload)
	return err
}

// visible requires that the caller may see this comment. A thread for editors stays
// hidden from everyone else until the page is published (spec 010, C2).
func (s *Service) visible(ctx context.Context, q db.Querier, c identity.Caller, v row) error {
	if err := s.perms.RequirePage(ctx, q, c.UserID, v.PageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return err
	}
	if !v.EditorsOnly {
		return nil
	}
	d, err := s.perms.CheckPage(ctx, q, c.UserID, v.PageID, permissions.PageEdit, uuid.Nil)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return apperr.NotFound()
	}
	return nil
}

// NewComment is what a person posts.
type NewComment struct {
	Body        json.RawMessage
	ParentID    *uuid.UUID
	Anchor      *Anchor
	EditorsOnly bool
}

func clip(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit])
}

func (s *Service) cleanAnchor(ctx context.Context, q db.Querier, pageID uuid.UUID, a *Anchor) (*Anchor, error) {
	if a == nil {
		return nil, nil
	}
	switch a.Kind {
	case AnchorText:
		count := utf8.RuneCountInString(a.Exact)
		if strings.TrimSpace(a.Exact) == "" || !utf8.ValidString(a.Exact) {
			return nil, apperr.Invalid("Select some words to comment on.")
		}
		if count > maxExactRunes {
			return nil, apperr.Invalid("A comment can point at most at 1000 characters.")
		}
		return &Anchor{Kind: AnchorText, Exact: a.Exact, Prefix: clip(a.Prefix, maxContextRunes), Suffix: clip(a.Suffix, maxContextRunes),
			Offset: max(a.Offset, 0), Version: max(a.Version, 0)}, nil
	case AnchorImage:
		if a.AttachmentID == nil {
			return nil, apperr.Invalid("Choose an image to comment on.")
		}
		if err := attachmentsOnPage(ctx, q, pageID, []uuid.UUID{*a.AttachmentID}); err != nil {
			return nil, err
		}
		return &Anchor{Kind: AnchorImage, AttachmentID: a.AttachmentID}, nil
	}
	return nil, apperr.Invalid("A comment points at text or an image.")
}

func attachmentsOnPage(ctx context.Context, q db.Querier, pageID uuid.UUID, list []uuid.UUID) error {
	if len(list) == 0 {
		return nil
	}
	var found int
	err := q.QueryRow(ctx, "SELECT count(*) FROM attachments WHERE page_id = $1 AND id = ANY($2) AND status = 'active'", pageID, list).Scan(&found)
	if err != nil {
		return err
	}
	if found != len(list) {
		return apperr.Invalid("An image in a comment must be an attachment of this page.")
	}
	return nil
}

func (s *Service) checkOpen(ctx context.Context, q db.Querier, pageID, spaceID uuid.UUID) error {
	on, err := s.features.Enabled(ctx, q, spaceID, features.Comments)
	if err != nil {
		return err
	}
	var closed bool
	if err := q.QueryRow(ctx, "SELECT comments_closed FROM pages WHERE id = $1", pageID).Scan(&closed); err != nil {
		return err
	}
	if !on || closed {
		return apperr.Conflict("comments_closed", "Comments are closed here.")
	}
	return nil
}

func (s *Service) checkRate(ctx context.Context, q db.Querier, user uuid.UUID) error {
	var recent int
	err := q.QueryRow(ctx, "SELECT count(*) FROM comments WHERE author_id = $1 AND created_at > now() - interval '1 minute'", user).Scan(&recent)
	if err != nil {
		return err
	}
	if recent >= maxPerMinute {
		return apperr.Conflict("too_many_comments", "That is a lot of comments in one minute. Try again in a moment.")
	}
	return nil
}

func (s *Service) spaceOf(ctx context.Context, q db.Querier, pageID uuid.UUID) (uuid.UUID, error) {
	var space uuid.UUID
	err := q.QueryRow(ctx, "SELECT space_id FROM pages WHERE id = $1", pageID).Scan(&space)
	if errors.Is(err, pgx.ErrNoRows) {
		return space, apperr.NotFound()
	}
	return space, err
}

// threadRoot finds the comment a reply belongs to. A reply to a reply joins the same
// thread (spec 010, D5).
func (s *Service) threadRoot(ctx context.Context, q db.Querier, c identity.Caller, pageID, parent uuid.UUID) (row, error) {
	root, err := load(ctx, q, parent)
	if err != nil {
		return root, err
	}
	if root.PageID != pageID {
		return root, apperr.Invalid("A reply stays on the page of its comment.")
	}
	if err := s.visible(ctx, q, c, root); err != nil {
		return root, err
	}
	if root.ParentID == nil {
		return root, nil
	}
	return load(ctx, q, *root.ParentID)
}

// Create posts a comment or a reply.
func (s *Service) Create(ctx context.Context, c identity.Caller, pageID uuid.UUID, in NewComment) (Comment, error) {
	body, err := document.NormalizeComment(in.Body)
	if err != nil {
		return Comment{}, err
	}
	raw, err := document.Marshal(body)
	if err != nil {
		return Comment{}, err
	}
	var out row
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, pageID, permissions.CommentAdd, uuid.Nil); err != nil {
			return err
		}
		spaceID, err := s.spaceOf(ctx, tx, pageID)
		if err != nil {
			return err
		}
		if err := s.checkOpen(ctx, tx, pageID, spaceID); err != nil {
			return err
		}
		if err := s.checkRate(ctx, tx, c.UserID); err != nil {
			return err
		}
		if err := attachmentsOnPage(ctx, tx, pageID, document.AttachmentIDs(body)); err != nil {
			return err
		}
		editorsOnly := in.EditorsOnly
		var parent *uuid.UUID
		var anchor *Anchor
		if in.ParentID != nil {
			root, err := s.threadRoot(ctx, tx, c, pageID, *in.ParentID)
			if err != nil {
				return err
			}
			parent, editorsOnly = &root.ID, root.EditorsOnly
		} else if anchor, err = s.cleanAnchor(ctx, tx, pageID, in.Anchor); err != nil {
			return err
		}
		if editorsOnly && in.ParentID == nil {
			if err := s.perms.RequirePage(ctx, tx, c.UserID, pageID, permissions.PageEdit, uuid.Nil); err != nil {
				return err
			}
		}
		var rawAnchor []byte
		if anchor != nil {
			if rawAnchor, err = json.Marshal(anchor); err != nil {
				return err
			}
		}
		id := ids.New()
		if _, err := tx.Exec(ctx, `INSERT INTO comments (id, origin_instance_id, space_id, page_id, parent_id, author_id, body, anchor, editors_only)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			id, s.instanceID, spaceID, pageID, parent, c.UserID, raw, rawAnchor, editorsOnly); err != nil {
			return err
		}
		if out, err = load(ctx, tx, id); err != nil {
			return err
		}
		if err := markSeen(ctx, tx, c.UserID, pageID); err != nil {
			return err
		}
		return record(ctx, tx, c, "comment.created", out, map[string]any{"parent_id": parent, "editors_only": editorsOnly,
			"mentions": document.MentionedUsers(body), "mentioned_groups": document.MentionedGroups(body)})
	})
	if err != nil {
		return Comment{}, err
	}
	return out.Comment, nil
}

// Edit changes a comment. Only its author or a space administrator may. A mention that
// is new in this edit is told, and one that was already there is not.
func (s *Service) Edit(ctx context.Context, c identity.Caller, id uuid.UUID, rawBody json.RawMessage) (Comment, error) {
	body, err := document.NormalizeComment(rawBody)
	if err != nil {
		return Comment{}, err
	}
	raw, err := document.Marshal(body)
	if err != nil {
		return Comment{}, err
	}
	var out row
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		v, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.visible(ctx, tx, c, v); err != nil {
			return err
		}
		if v.trashed || v.purged {
			return apperr.NotFound()
		}
		if v.AuthorID != c.UserID {
			if err := s.perms.RequirePage(ctx, tx, c.UserID, v.PageID, permissions.SpaceAdminister, uuid.Nil); err != nil {
				return err
			}
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, v.PageID, permissions.CommentAdd, uuid.Nil); err != nil {
			return err
		}
		if err := attachmentsOnPage(ctx, tx, v.PageID, document.AttachmentIDs(body)); err != nil {
			return err
		}
		old, err := document.Parse(v.Body)
		if err != nil {
			return err
		}
		before := document.MentionedUsers(old)
		added := []uuid.UUID{}
		for _, person := range document.MentionedUsers(body) {
			if !slices.Contains(before, person) {
				added = append(added, person)
			}
		}
		if _, err := tx.Exec(ctx, "UPDATE comments SET body = $2, edited_at = now(), edited_by = $3 WHERE id = $1", id, raw, c.UserID); err != nil {
			return err
		}
		if out, err = load(ctx, tx, id); err != nil {
			return err
		}
		return record(ctx, tx, c, "comment.edited", out, map[string]any{"mentions": added, "editors_only": v.EditorsOnly})
	})
	if err != nil {
		return Comment{}, err
	}
	return out.Comment, nil
}

func trashTitle(raw []byte) string {
	n, err := document.Parse(raw)
	if err != nil {
		return "Comment"
	}
	if words := clip(strings.TrimSpace(document.BlockText(n)), trashTitleRunes); words != "" {
		return words
	}
	return "Comment"
}

// Delete sends a comment to the trash of the space.
func (s *Service) Delete(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	retention, err := s.settings.TrashRetention(ctx)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		v, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.visible(ctx, tx, c, v); err != nil {
			return err
		}
		if v.trashed || v.purged {
			return apperr.NotFound()
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, v.PageID, permissions.ActionCommentDelete, v.AuthorID); err != nil {
			return err
		}
		item, err := trash.Add(ctx, tx, trash.Item{SpaceID: v.spaceID, ObjectType: trash.TypeComment, ObjectID: id,
			Title: trashTitle(v.Body), DeletedBy: c.UserID}, retention)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE comments SET trash_item_id = $2 WHERE id = $1", id, item); err != nil {
			return err
		}
		return record(ctx, tx, c, "comment.trashed", v, nil)
	})
}

// Restore implements trash.Handler.
func (s *Service) Restore(ctx context.Context, tx pgx.Tx, c identity.Caller, it trash.Item) error {
	v, err := load(ctx, tx, it.ObjectID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE comments SET trash_item_id = NULL WHERE id = $1", it.ObjectID); err != nil {
		return err
	}
	return record(ctx, tx, c, "comment.restored", v, nil)
}

// Purge implements trash.Handler. The words go for good. A comment with replies keeps
// its place as a line that says it was removed, so the thread stays readable.
func (s *Service) Purge(ctx context.Context, tx pgx.Tx, c identity.Caller, it trash.Item) error {
	var replies bool
	err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM comments WHERE parent_id = $1)", it.ObjectID).Scan(&replies)
	if err != nil {
		return err
	}
	if replies {
		_, err = tx.Exec(ctx, `UPDATE comments SET purged = true, trash_item_id = NULL, anchor = NULL,
			body = '{"type":"doc","content":[{"type":"paragraph"}]}' WHERE id = $1`, it.ObjectID)
	} else {
		_, err = tx.Exec(ctx, "DELETE FROM comments WHERE id = $1", it.ObjectID)
	}
	if err != nil {
		return err
	}
	if _, err := audit.Record(ctx, tx, c.Event("comment.purged", "comment", it.ObjectID.String(), it.SpaceID.String(), nil)); err != nil {
		return err
	}
	_, err = outbox.Enqueue(ctx, tx, "comment.purged", map[string]any{"comment_id": it.ObjectID, "space_id": it.SpaceID})
	return err
}

func (s *Service) setStatus(ctx context.Context, c identity.Caller, id uuid.UUID, status, action string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		v, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.visible(ctx, tx, c, v); err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, v.PageID, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		if v.ParentID != nil {
			return apperr.Invalid("Only a whole thread can be resolved.")
		}
		var by *uuid.UUID
		if status == StatusResolved {
			by = &c.UserID
		}
		if _, err := tx.Exec(ctx, `UPDATE comments SET status = $2, resolved_by = $3,
			resolved_at = CASE WHEN $2 = 'resolved' THEN now() END WHERE id = $1`, id, status, by); err != nil {
			return err
		}
		return record(ctx, tx, c, action, v, nil)
	})
}

// Resolve ends a thread.
func (s *Service) Resolve(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return s.setStatus(ctx, c, id, StatusResolved, "comment.resolved")
}

// Reopen brings a thread back.
func (s *Service) Reopen(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return s.setStatus(ctx, c, id, StatusOpen, "comment.reopened")
}

func markSeen(ctx context.Context, q db.Querier, user, pageID uuid.UUID) error {
	_, err := q.Exec(ctx, `INSERT INTO comment_seen (user_id, page_id) VALUES ($1, $2)
		ON CONFLICT (user_id, page_id) DO UPDATE SET seen_at = now()`, user, pageID)
	return err
}

// MarkSeen moves the moment before which comments count as read.
func (s *Service) MarkSeen(ctx context.Context, c identity.Caller, pageID uuid.UUID) error {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return err
	}
	return markSeen(ctx, s.pool, c.UserID, pageID)
}

// anchored is a thread with what it points at.
type anchored struct {
	id     uuid.UUID
	anchor *Anchor
}

func scanAnchored(r pgx.CollectableRow) (anchored, error) {
	var a anchored
	err := r.Scan(&a.id, &a.anchor)
	return a, err
}

// BlankQuotes takes the words of a removed version out of the threads that quote them.
// An anchor copies text from the page, so otherwise the words live on after the version
// is gone (SEC-407). The thread stays, and says its text is no longer in the page. Words
// that are still in the current version stay too. Removing a version calls this.
func BlankQuotes(ctx context.Context, tx pgx.Tx, c identity.Caller, pageID uuid.UUID, removed, current document.Node) error {
	rows, err := tx.Query(ctx, "SELECT id, anchor FROM comments WHERE page_id = $1 AND anchor ->> 'exact' IS NOT NULL", pageID)
	if err != nil {
		return err
	}
	threads, err := pgx.CollectRows(rows, scanAnchored)
	if err != nil {
		return err
	}
	blanked := []uuid.UUID{}
	for _, t := range threads {
		if document.HoldsQuote(removed, t.anchor.Exact) && !document.HoldsQuote(current, t.anchor.Exact) {
			blanked = append(blanked, t.id)
		}
	}
	if len(blanked) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, "UPDATE comments SET anchor = anchor - 'exact' - 'prefix' - 'suffix' - 'offset' WHERE id = ANY($1)", blanked); err != nil {
		return err
	}
	var spaceID uuid.UUID
	if err := tx.QueryRow(ctx, "SELECT space_id FROM pages WHERE id = $1", pageID).Scan(&spaceID); err != nil {
		return err
	}
	if _, err := audit.Record(ctx, tx, c.Event("comment.quote_blanked", "page", pageID.String(), spaceID.String(), map[string]any{"threads": len(blanked)})); err != nil {
		return err
	}
	_, err = outbox.Enqueue(ctx, tx, "comment.quote_blanked", map[string]any{"page_id": pageID, "space_id": spaceID})
	return err
}

// reachedReaders reports whether what a thread points at is in a published document.
func reachedReaders(a *Anchor, doc document.Node) bool {
	switch {
	case a == nil:
		return true
	case a.Kind == AnchorImage && a.AttachmentID != nil:
		return document.HoldsAttachment(doc, *a.AttachmentID)
	}
	return document.HoldsQuote(doc, a.Exact)
}

// ReleaseEditorsOnly opens threads made while editing, in the transaction that
// publishes the page. A thread opens only when its quote is in the published text: a
// sentence that was never published must not reach readers through a comment.
func ReleaseEditorsOnly(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, doc document.Node) error {
	rows, err := tx.Query(ctx, "SELECT id, anchor FROM comments WHERE page_id = $1 AND editors_only AND parent_id IS NULL", pageID)
	if err != nil {
		return err
	}
	threads, err := pgx.CollectRows(rows, scanAnchored)
	if err != nil {
		return err
	}
	opened := []uuid.UUID{}
	for _, t := range threads {
		if reachedReaders(t.anchor, doc) {
			opened = append(opened, t.id)
		}
	}
	_, err = tx.Exec(ctx, "UPDATE comments SET editors_only = false WHERE id = ANY($1) OR parent_id = ANY($1)", opened)
	return err
}

// Closed turns comments off for one page, or back on. Existing comments stay readable.
func (s *Service) Closed(ctx context.Context, c identity.Caller, pageID uuid.UUID, closed bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, pageID, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		spaceID, err := s.spaceOf(ctx, tx, pageID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE pages SET comments_closed = $2 WHERE id = $1", pageID, closed); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("page.comments_closed", "page", pageID.String(), spaceID.String(), map[string]any{"closed": closed})); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "page.comments_closed", map[string]any{"page_id": pageID, "space_id": spaceID})
		return err
	})
}
