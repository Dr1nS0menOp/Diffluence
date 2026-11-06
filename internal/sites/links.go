// SPDX-License-Identifier: Apache-2.0

package sites

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
)

const (
	linksFolder = "links"
	tokenBytes  = 16 // 128 random bits (PRM-099)
)

// Link is a single public page.
type Link struct {
	ID        uuid.UUID `json:"id"`
	PageID    uuid.UUID `json:"page_id"`
	PageTitle string    `json:"page_title"`
	Version   int       `json:"version"`
	Path      string    `json:"path"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) requireLinks(ctx context.Context) error {
	limits, err := s.exchange.Limits(ctx)
	if err != nil {
		return err
	}
	if !limits.PublicLinks || s.options.PublishDir == "" {
		return apperr.Conflict("links_off", "Public links are off on this instance. An administrator can turn them on.")
	}
	return nil
}

func (s *Service) requireAdminister(ctx context.Context, q db.Querier, c identity.Caller, pageID uuid.UUID) (uuid.UUID, error) {
	if err := s.perms.RequirePage(ctx, q, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return uuid.Nil, err
	}
	spaceID, err := s.pages.SpaceOf(ctx, c, pageID)
	if err != nil {
		return uuid.Nil, err
	}
	return spaceID, s.perms.RequireSpace(ctx, q, c.UserID, spaceID, permissions.SpaceAdminister)
}

const linkSelect = `SELECT l.id, l.page_id, p.title, l.version, l.token, COALESCE(u.display_name, ''), l.created_at
	FROM public_links l JOIN pages p ON p.id = l.page_id LEFT JOIN users u ON u.id = l.created_by WHERE l.withdrawn_at IS NULL`

func scanLink(row pgx.Row) (Link, error) {
	var l Link
	var token string
	err := row.Scan(&l.ID, &l.PageID, &l.PageTitle, &l.Version, &token, &l.CreatedBy, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, apperr.NotFound()
	}
	l.Path = "/" + linksFolder + "/" + token + "/"
	return l, err
}

// LinkOf gives the public link of a page.
func (s *Service) LinkOf(ctx context.Context, c identity.Caller, pageID uuid.UUID) (Link, error) {
	if _, err := s.requireAdminister(ctx, s.pool, c, pageID); err != nil {
		return Link{}, err
	}
	return scanLink(s.pool.QueryRow(ctx, linkSelect+` AND l.page_id = $1`, pageID))
}

// Links lists all of them, for the administrator (G16).
func (s *Service) Links(ctx context.Context) ([]Link, error) {
	rows, err := s.pool.Query(ctx, linkSelect+` ORDER BY l.created_at DESC`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Link, error) { return scanLink(r) })
}

func newToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Service) linkFolder(token string) string {
	return filepath.Join(s.options.PublishDir, linksFolder, token)
}

// MakeLink builds one page as a static page under an unguessable address. It shows
// this version: a later publish does not reach it until the person says so (G16).
func (s *Service) MakeLink(ctx context.Context, c identity.Caller, pageID uuid.UUID) (Link, error) {
	spaceID, err := s.requireAdminister(ctx, s.pool, c, pageID)
	if err != nil {
		return Link{}, err
	}
	if err := s.requireLinks(ctx); err != nil {
		return Link{}, err
	}
	allowed, err := s.mayBePublic(ctx, s.pool, spaceID, []uuid.UUID{pageID})
	if err != nil {
		return Link{}, err
	}
	if !allowed[pageID] {
		return Link{}, apperr.Conflict("page_not_public", "This page has a restriction, or is not published.")
	}
	token, err := newToken()
	if err != nil {
		return Link{}, err
	}
	version, err := s.buildLink(ctx, pageID, token)
	if err != nil {
		_ = os.RemoveAll(s.linkFolder(token))
		return Link{}, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.closeLinks(ctx, tx, pageID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO public_links (id, page_id, version, token, created_by) VALUES ($1, $2, $3, $4, $5)`,
			ids.New(), pageID, version, token, c.UserID); err != nil {
			return err
		}
		return recordLink(ctx, tx, c, "public_link.created", pageID, spaceID, version)
	})
	if err != nil {
		_ = os.RemoveAll(s.linkFolder(token))
		return Link{}, err
	}
	return scanLink(s.pool.QueryRow(ctx, linkSelect+` AND l.page_id = $1`, pageID))
}

func recordLink(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, pageID, spaceID uuid.UUID, version int) error {
	if _, err := audit.Record(ctx, tx, c.Event(action, "page", pageID.String(), spaceID.String(), map[string]any{"version": version})); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"page_id": pageID, "space_id": spaceID})
	return err
}

// closeLinks ends the links of a page, and removes their folders.
func (s *Service) closeLinks(ctx context.Context, q db.Querier, pageID uuid.UUID) error {
	rows, err := q.Query(ctx, `UPDATE public_links SET withdrawn_at = now() WHERE page_id = $1 AND withdrawn_at IS NULL RETURNING token`, pageID)
	if err != nil {
		return err
	}
	tokens, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, token := range tokens {
		if err := os.RemoveAll(s.linkFolder(token)); err != nil {
			return err
		}
	}
	return nil
}

// WithdrawLink takes a public page down.
func (s *Service) WithdrawLink(ctx context.Context, c identity.Caller, pageID uuid.UUID) error {
	spaceID, err := s.requireAdminister(ctx, s.pool, c, pageID)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.closeLinks(ctx, tx, pageID); err != nil {
			return err
		}
		return recordLink(ctx, tx, c, "public_link.withdrawn", pageID, spaceID, 0)
	})
}

// enforceLinks withdraws every link whose page or version may no longer be public.
func (s *Service) enforceLinks(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT l.page_id, l.version, p.space_id FROM public_links l JOIN pages p ON p.id = l.page_id WHERE l.withdrawn_at IS NULL`)
	if err != nil {
		return err
	}
	type live struct {
		page    uuid.UUID
		version int
		space   uuid.UUID
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (live, error) {
		var l live
		err := r.Scan(&l.page, &l.version, &l.space)
		return l, err
	})
	if err != nil {
		return err
	}
	for _, l := range list {
		allowed, err := s.mayBePublic(ctx, s.pool, l.space, []uuid.UUID{l.page})
		if err != nil {
			return err
		}
		gone, err := s.versionsGone(ctx, map[uuid.UUID]int{l.page: l.version})
		if err != nil {
			return err
		}
		if allowed[l.page] && !gone {
			continue
		}
		if err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			if err := s.closeLinks(ctx, tx, l.page); err != nil {
				return err
			}
			return recordLink(ctx, tx, system(), "public_link.withdrawn", l.page, l.space, l.version)
		}); err != nil {
			return err
		}
	}
	// A purged page takes its rows along, so its folder has to go by what is left.
	return s.removeOrphanLinks(ctx)
}

// removeOrphanLinks removes folders no row speaks for: the row of a purged page is gone.
func (s *Service) removeOrphanLinks(ctx context.Context) error {
	if s.options.PublishDir == "" {
		return nil
	}
	folders, err := os.ReadDir(filepath.Join(s.options.PublishDir, linksFolder))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := s.pool.Query(ctx, `SELECT token FROM public_links WHERE withdrawn_at IS NULL`)
	if err != nil {
		return err
	}
	tokens, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	live := map[string]bool{}
	for _, token := range tokens {
		live[token] = true
	}
	for _, f := range folders {
		if !live[f.Name()] {
			if err := os.RemoveAll(s.linkFolder(f.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// buildLink writes the page into its folder, by the rules of a site that shows no names.
func (s *Service) buildLink(ctx context.Context, pageID uuid.UUID, token string) (int, error) {
	list, err := s.pages.ForPublic(ctx, []uuid.UUID{pageID}, false)
	if err != nil {
		return 0, err
	}
	report := newReport()
	prepared, err := s.prepare(ctx, list, report)
	if err != nil {
		return 0, err
	}
	if len(prepared) != 1 {
		return 0, apperr.Invalid("This page cannot be read.")
	}
	dir := s.linkFolder(token)
	if err := os.MkdirAll(dir, folderMode); err != nil {
		return 0, err
	}
	p := prepared[0]
	p.file = "index.html"
	v := Site{Name: p.page.Title, Language: "en", Accent: "blue"}
	b := &builder{s: s, v: v, dir: dir, list: []sitePage{p}, report: report, links: map[uuid.UUID]string{}, single: true}
	if err := b.linkFiles(ctx, p); err != nil {
		return 0, err
	}
	if err := s.scrub(ctx, dir); err != nil {
		return 0, err
	}
	return p.page.Version, nil
}

func (b *builder) linkFiles(ctx context.Context, p sitePage) error {
	for name, body := range map[string]string{"style.css": document.ExportStyle, "theme.css": b.theme(),
		"robots.txt": "User-agent: *\nDisallow: /\n"} {
		if err := b.write(name, body); err != nil {
			return err
		}
	}
	return b.page(ctx, p)
}
