// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/search"
)

// maxListedPlaceholders keeps the list in space settings to what someone can work through.
const maxListedPlaceholders = 500

// LinkFact is what a link may show about a page. A document stores none of it, so it
// is looked up for each reader (spec 023, D2).
type LinkFact struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Emoji     string    `json:"emoji"`
	SpaceKey  string    `json:"space_key"`
	SpaceName string    `json:"space_name"`
	Excerpt   string    `json:"excerpt"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LinkFacts answers for the pages the caller can open. A page that is hidden and a page
// that is gone are both left out, so the two cannot be told apart.
func (s *Service) LinkFacts(ctx context.Context, c identity.Caller, ids []uuid.UUID) ([]LinkFact, error) {
	if len(ids) > maxReachableIDs {
		return nil, apperr.Invalid("That is too many pages to ask about at once.")
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji, sp.key, sp.name, p.updated_at
		FROM pages p
		JOIN perm_visible_pages($1) v ON v.page_id = p.id
		JOIN spaces sp ON sp.id = p.space_id
		WHERE p.id = ANY($2)`, c.UserID, ids)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (LinkFact, error) {
		var f LinkFact
		err := r.Scan(&f.ID, &f.Title, &f.Emoji, &f.SpaceKey, &f.SpaceName, &f.UpdatedAt)
		return f, err
	})
	if err != nil {
		return nil, err
	}
	found := make([]uuid.UUID, len(out))
	for i, f := range out {
		found[i] = f.ID
	}
	excerpts, err := search.Excerpts(ctx, s.pool, found)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Excerpt = excerpts[out[i].ID]
	}
	return out, nil
}

// LinkTitles gives the titles of the pages a document links to, as far as the caller may
// see them, for a document that leaves the instance.
func (s *Service) LinkTitles(ctx context.Context, c identity.Caller, doc document.Node) (document.PageTitle, error) {
	facts, err := s.LinkFacts(ctx, c, document.LinkedPages(doc))
	if err != nil {
		return nil, err
	}
	titles := make(map[uuid.UUID]string, len(facts))
	for _, f := range facts {
		titles[f.ID] = f.Title
	}
	return func(id uuid.UUID) (string, bool) {
		title, ok := titles[id]
		return title, ok
	}, nil
}

// LinkTargets is what a link can point at inside a page.
type LinkTargets struct {
	Headings []LinkHeading `json:"headings"`
	Anchors  []string      `json:"anchors"`
}

// LinkHeading is a heading with the name an address uses for it.
type LinkHeading struct {
	Slug  string `json:"slug"`
	Level int    `json:"level"`
	Text  string `json:"text"`
}

// Targets lists the headings and anchors of the current version.
func (s *Service) Targets(ctx context.Context, c identity.Caller, id uuid.UUID) (LinkTargets, error) {
	var out LinkTargets
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
			return err
		}
		p, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		out = LinkTargets{Headings: []LinkHeading{}, Anchors: []string{}}
		if p.CurrentVersion == 0 {
			return nil // a draft has no version yet
		}
		doc, err := currentDocument(ctx, tx, p)
		if err != nil {
			return err
		}
		for _, h := range document.Headings(doc) {
			out.Headings = append(out.Headings, LinkHeading{Slug: h.ID, Level: h.Level, Text: h.Text})
		}
		out.Anchors = document.Anchors(doc)
		return nil
	})
	return out, err
}

// syncPlaceholders makes page_placeholders follow the current version. A title that stays
// keeps the page it became.
func syncPlaceholders(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, doc document.Node) error {
	titles := document.Placeholders(doc)
	if _, err := tx.Exec(ctx, "DELETE FROM page_placeholders WHERE source_id = $1 AND NOT title = ANY($2)", pageID, titles); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO page_placeholders (source_id, title)
		SELECT $1, title FROM unnest($2::text[]) AS title ON CONFLICT DO NOTHING`, pageID, titles)
	return err
}

// FollowedPlaceholder is a link to a page that does not exist yet and now leads somewhere.
type FollowedPlaceholder struct {
	Title  string    `json:"title"`
	PageID uuid.UUID `json:"page_id"`
}

// Followed lists the placeholders of a page that lead to a page the caller may see.
func (s *Service) Followed(ctx context.Context, c identity.Caller, id uuid.UUID) ([]FollowedPlaceholder, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT h.title, h.target_id FROM page_placeholders h
		JOIN perm_visible_pages($1) v ON v.page_id = h.target_id
		WHERE h.source_id = $2 ORDER BY h.title`, c.UserID, id)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (FollowedPlaceholder, error) {
		var f FollowedPlaceholder
		err := r.Scan(&f.Title, &f.PageID)
		return f, err
	})
	if out == nil {
		out = []FollowedPlaceholder{}
	}
	return out, err
}

// Follow starts the page a placeholder asks for, below the page it stands on, or returns
// the page it already became. The row is locked, so two people get one page.
func (s *Service) Follow(ctx context.Context, c identity.Caller, source uuid.UUID, title string) (uuid.UUID, error) {
	title = strings.TrimSpace(title)
	var out uuid.UUID
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, source, permissions.ActionPageView, uuid.Nil); err != nil {
			return err
		}
		var target *uuid.UUID
		err := tx.QueryRow(ctx, "SELECT target_id FROM page_placeholders WHERE source_id = $1 AND title = $2 FOR UPDATE", source, title).Scan(&target)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound()
		}
		if err != nil {
			return err
		}
		if target != nil {
			out = *target
			return s.requireStarted(ctx, tx, c, *target)
		}
		p, err := load(ctx, tx, source)
		if err != nil {
			return err
		}
		if err := s.checkParent(ctx, tx, c, placement{spaceID: p.SpaceID, parent: source, kind: KindPage}); err != nil {
			return err
		}
		if err := lockTree(ctx, tx, p.SpaceID); err != nil {
			return err
		}
		started, err := s.insert(ctx, tx, c, newPage{spaceID: p.SpaceID, parent: &source, title: title, author: c.UserID})
		if err != nil {
			return err
		}
		out = started.ID
		_, err = tx.Exec(ctx, "UPDATE page_placeholders SET target_id = $3 WHERE source_id = $1 AND title = $2", source, title, started.ID)
		return err
	})
	return out, err
}

// requireStarted lets someone through to a page a placeholder became, when they may see it.
func (s *Service) requireStarted(ctx context.Context, tx pgx.Tx, c identity.Caller, target uuid.UUID) error {
	err := s.perms.RequirePage(ctx, tx, c.UserID, target, permissions.ActionPageView, uuid.Nil)
	if apperr.IsNotFound(err) {
		return apperr.Conflict("placeholder_taken", "Someone already started this page, and you cannot open it yet.")
	}
	return err
}

// OpenPlaceholder is a link in a space to a page nobody has started.
type OpenPlaceholder struct {
	Title  string  `json:"title"`
	Source PageRef `json:"source"`
}

// OpenPlaceholders lists them for a space administrator, as far as they may see the pages.
func (s *Service) OpenPlaceholders(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]OpenPlaceholder, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT h.title, `+refColumns+` FROM page_placeholders h
		JOIN pages p ON p.id = h.source_id `+refJoins+`
		WHERE p.space_id = $2 AND p.status = 'published' AND h.target_id IS NULL
		ORDER BY lower(h.title), p.id LIMIT $3`, c.UserID, spaceID, maxListedPlaceholders)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (OpenPlaceholder, error) {
		var v OpenPlaceholder
		err := r.Scan(&v.Title, &v.Source.ID, &v.Source.Title, &v.Source.Emoji, &v.Source.SpaceKey, &v.Source.SpaceName)
		return v, err
	})
	if out == nil {
		out = []OpenPlaceholder{}
	}
	return out, err
}
