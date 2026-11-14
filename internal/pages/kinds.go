// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/search"
)

const (
	defaultPosts = 20
	maxPosts     = 100

	// The columns of a new database, ready to rename.
	firstColumnName  = "Name"
	secondColumnName = "Notes"
)

// checkDocumentKind keeps a sheet where it belongs: a database is exactly one sheet,
// and nothing else holds one. This is the one place all new text passes.
func checkDocumentKind(kind string, doc document.Node) error {
	if kind == KindDatabase && !document.IsSheetDocument(doc) {
		return apperr.Invalid("A database holds one sheet and nothing else.")
	}
	if kind != KindDatabase && document.HasSheet(doc) {
		return apperr.Invalid("Only a database holds a sheet.")
	}
	// A value of a template is filled in when a page is made; a leftover must never reach
	// a draft or a reader (spec 074, TPL-211).
	if kind != KindTemplate && document.HasTemplateValues(doc) {
		return apperr.Invalid("A value of a template cannot be part of a page. Remove it first.")
	}
	return nil
}

// SetKind turns a page into another kind. A folder keeps its old text in its
// versions, so it comes back when it is a page again.
func (s *Service) SetKind(ctx context.Context, c identity.Caller, id uuid.UUID, kind string) (Page, error) {
	if kind != KindPage && kind != KindFolder && kind != KindPost {
		return Page{}, apperr.Invalid("A page can become a page, a folder or a blog post.")
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
		// Turning a page into a blog post makes a post, and back makes a page, so each asks
		// for what making that kind asks (spec 045). A folder is a page in this sense.
		if made := permissions.CreatePermission(kind); made != permissions.CreatePermission(p.Kind) {
			if err := s.perms.RequirePage(ctx, tx, c.UserID, id, made, uuid.Nil); err != nil {
				return err
			}
		}
		if err := s.checkKindChange(ctx, tx, p, kind); err != nil {
			return err
		}
		out, err = scanPage(tx.QueryRow(ctx, `UPDATE pages SET kind = $2, parent_id = CASE WHEN $2 = 'post' THEN NULL ELSE parent_id END,
			updated_at = now() WHERE id = $1 RETURNING `+pageColumns, id, kind))
		if err != nil {
			return err
		}
		if kind == KindPost && p.ParentID != nil {
			if err := relink(ctx, tx, id, nil); err != nil {
				return err
			}
		}
		// A folder holds nothing to approve, and a space workflow can govern some kinds only.
		if err := s.settleBranch(ctx, tx, c, id, settleKind); err != nil {
			return err
		}
		return record(ctx, tx, c, "page.kind_changed", out, map[string]any{"from": p.Kind, "to": kind})
	})
	return out, err
}

func (s *Service) checkKindChange(ctx context.Context, tx pgx.Tx, p Page, kind string) error {
	if p.Kind == KindLink {
		return apperr.Conflict("link_entry", "A link entry stays a link entry.")
	}
	if p.Kind == KindDatabase {
		return apperr.Conflict("database", "A database stays a database.")
	}
	if kind == KindFolder {
		home, err := isHomePage(ctx, tx, p)
		if err != nil {
			return err
		}
		if home {
			return apperr.Conflict("home_page", "The home page of a space cannot be a folder.")
		}
	}
	if kind != KindPost && p.Kind != KindPost {
		return nil
	}
	if p.CurrentVersion > 0 {
		return apperr.Conflict("already_published", "Only an unpublished page can become a blog post, or come back from one.")
	}
	if kind == KindPost {
		if err := s.requireFeature(ctx, tx, p.SpaceID, features.Blog); err != nil {
			return err
		}
		children, err := directChildren(ctx, tx, p.ID)
		if err != nil {
			return err
		}
		if len(children) > 0 {
			return apperr.Conflict("has_children", "A page with pages below it cannot become a blog post.")
		}
	}
	return nil
}

// DatabasesOn reports whether new databases may be made (spec 069).
func (s *Service) DatabasesOn(ctx context.Context) (bool, error) {
	return s.features.ExperimentalOn(ctx, s.pool, features.ExperimentalDatabases)
}

// Post is a blog post in a list.
type Post struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Emoji       string    `json:"emoji"`
	SpaceID     uuid.UUID `json:"space_id"`
	SpaceKey    string    `json:"space_key"`
	SpaceName   string    `json:"space_name"`
	AuthorName  string    `json:"author_name"`
	PublishedAt time.Time `json:"published_at"`
	Excerpt     string    `json:"excerpt"`
}

// Posts lists blog posts, newest first, of one space or of all the person can see.
func (s *Service) Posts(ctx context.Context, c identity.Caller, spaceID *uuid.UUID, limit, offset int) ([]Post, int, error) {
	if spaceID != nil {
		if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, *spaceID, permissions.SpaceView); err != nil {
			return nil, 0, err
		}
	}
	if limit <= 0 || limit > maxPosts {
		limit = defaultPosts
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji, p.space_id, sp.key, sp.name, COALESCE(u.display_name, ''),
			p.published_at, count(*) OVER ()
		FROM pages p
		JOIN perm_listed_pages($1) v ON v.page_id = p.id
		JOIN spaces sp ON sp.id = p.space_id
		LEFT JOIN users u ON u.id = p.created_by
		WHERE p.kind = 'post' AND p.status = 'published' AND ($2::uuid IS NULL OR p.space_id = $2)
		ORDER BY p.published_at DESC, p.id LIMIT $3 OFFSET $4`, c.UserID, spaceID, limit, max(offset, 0))
	if err != nil {
		return nil, 0, err
	}
	total := 0
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Post, error) {
		var v Post
		err := r.Scan(&v.ID, &v.Title, &v.Emoji, &v.SpaceID, &v.SpaceKey, &v.SpaceName, &v.AuthorName, &v.PublishedAt, &total)
		return v, err
	})
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, len(list))
	for i, p := range list {
		ids[i] = p.ID
	}
	excerpts, err := search.Excerpts(ctx, s.pool, ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range list {
		list[i].Excerpt = excerpts[list[i].ID]
	}
	if list == nil {
		list = []Post{}
	}
	return list, total, nil
}

// CanBeHome reports whether a page can be what a space opens on.
func (s *Service) CanBeHome(ctx context.Context, q db.Querier, c identity.Caller, id, spaceID uuid.UUID) (bool, error) {
	d, err := s.perms.CheckPage(ctx, q, c.UserID, id, permissions.ActionPageView, uuid.Nil)
	if err != nil {
		if _, ok := apperr.As(err); ok {
			return false, nil
		}
		return false, err
	}
	if !d.Allowed {
		return false, nil
	}
	p, err := load(ctx, q, id)
	if err != nil {
		return false, err
	}
	return p.SpaceID == spaceID && p.Status == StatusPublished && p.Kind == KindPage && p.ArchivedAt == nil, nil
}
