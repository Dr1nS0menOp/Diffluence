// SPDX-License-Identifier: Apache-2.0

// Package sites builds public websites and public pages as static files. The worker
// writes them below a folder, and a web server of someone's own choosing serves
// them: the instance itself never does (ADR-010, spec 014). A built site is a copy
// outside every rule, so all that goes in passes MayBePublic (PRM-096 to PRM-099).
package sites

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/exchange"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
)

// JobKindBuild is the background job that builds a site.
const JobKindBuild = "site_build"

// The ways a site is kept up to date.
const (
	UpdateOnPublish = "publish"
	UpdateDaily     = "daily"
	UpdateManual    = "manual"
)

const (
	maxNameRunes    = 200
	maxAddressRunes = 500
	publishDebounce = time.Minute
)

// Accents are the colours a site can choose, as light and dark values.
var Accents = map[string][2]string{
	"blue": {"#1d5fbf", "#7fb0f5"}, "green": {"#1b7a3d", "#7fd19c"}, "teal": {"#0b7570", "#6fd0ca"},
	"purple": {"#7e22ce", "#c9a0f0"}, "red": {"#b3261e", "#f2a8a2"}, "orange": {"#a84300", "#f3b184"},
}

var languageTag = regexp.MustCompile(`^[a-zA-Z]{2,3}(-[a-zA-Z0-9]{2,8})*$`)

// Site is a public website of a space.
type Site struct {
	ID            uuid.UUID  `json:"id"`
	SpaceID       uuid.UUID  `json:"space_id"`
	Name          string     `json:"name"`
	Address       string     `json:"address"`
	Target        string     `json:"target"`
	Language      string     `json:"language"`
	Accent        string     `json:"accent"`
	ShowNames     bool       `json:"show_names"`
	SearchEngines bool       `json:"search_engines"`
	UpdateMode    string     `json:"update_mode"`
	LiveBuild     int        `json:"live_build"`
	WithdrawnAt   *time.Time `json:"withdrawn_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

// Options are the parts of the service that are not other modules.
type Options struct {
	PublishDir string // empty = no sites on this instance
	Origin     string // the instance's own address, which no site may hold
	Log        *slog.Logger
}

// Service makes public sites and public links.
type Service struct {
	pool        *pgxpool.Pool
	perms       *permissions.Service
	pages       *pages.Service
	attachments *attachments.Service
	exchange    *exchange.Service
	jobs        *jobs.Service
	options     Options
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, pg *pages.Service, att *attachments.Service,
	ex *exchange.Service, jb *jobs.Service, o Options) *Service {
	s := &Service{pool: pool, perms: perms, pages: pg, attachments: att, exchange: ex, jobs: jb, options: o}
	jb.RegisterLong(JobKindBuild, s.runBuild)
	return s
}

const siteColumns = `id, space_id, name, address, target, language, accent, show_names, search_engines, update_mode, live_build, withdrawn_at, created_at`

func scan(row pgx.Row) (Site, error) {
	var v Site
	err := row.Scan(&v.ID, &v.SpaceID, &v.Name, &v.Address, &v.Target, &v.Language, &v.Accent, &v.ShowNames, &v.SearchEngines,
		&v.UpdateMode, &v.LiveBuild, &v.WithdrawnAt, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, apperr.NotFound()
	}
	return v, err
}

func load(ctx context.Context, q db.Querier, id uuid.UUID) (Site, error) {
	return scan(q.QueryRow(ctx, `SELECT `+siteColumns+` FROM public_sites WHERE id = $1`, id))
}

// owned loads a site for someone who publishes for its space. For anyone who may not
// see that space, the site does not exist.
func (s *Service) owned(ctx context.Context, c identity.Caller, id uuid.UUID) (Site, error) {
	v, err := load(ctx, s.pool, id)
	if err != nil {
		return v, err
	}
	return v, s.perms.RequireSpace(ctx, s.pool, c.UserID, v.SpaceID, permissions.SitePublish)
}

func (s *Service) requireEnabled(ctx context.Context) error {
	limits, err := s.exchange.Limits(ctx)
	if err != nil {
		return err
	}
	if !limits.PublicSites || s.options.PublishDir == "" {
		return apperr.Conflict("sites_off", "Public sites are off on this instance. An administrator can turn them on.")
	}
	return nil
}

// Input is what a person sets on a site.
type Input struct {
	Name          string `json:"name"`
	Address       string `json:"address"`
	Language      string `json:"language"`
	Accent        string `json:"accent"`
	ShowNames     bool   `json:"show_names"`
	SearchEngines bool   `json:"search_engines"`
	UpdateMode    string `json:"update_mode"`
}

func (in *Input) clean() error {
	in.Name, in.Address = strings.TrimSpace(in.Name), strings.TrimRight(strings.TrimSpace(in.Address), "/")
	if in.Name == "" || len([]rune(in.Name)) > maxNameRunes || strings.ContainsAny(in.Name, "\r\n\x00") {
		return apperr.Invalid("Give the site a name of at most 200 characters.")
	}
	if len([]rune(in.Address)) > maxAddressRunes {
		return apperr.Invalid("The address is too long.")
	}
	if in.Address != "" && !strings.HasPrefix(in.Address, "https://") && !strings.HasPrefix(in.Address, "http://") {
		return apperr.Invalid("The address starts with https://.")
	}
	if in.Language == "" {
		in.Language = "en"
	}
	if !languageTag.MatchString(in.Language) {
		return apperr.Invalid("That is not a language tag, such as en or nl.")
	}
	if in.Accent == "" {
		in.Accent = "blue"
	}
	if _, ok := Accents[in.Accent]; !ok {
		return apperr.Invalid("Choose one of the accent colours.")
	}
	if in.UpdateMode == "" {
		in.UpdateMode = UpdateManual
	}
	if in.UpdateMode != UpdateOnPublish && in.UpdateMode != UpdateDaily && in.UpdateMode != UpdateManual {
		return apperr.Invalid("Choose how the site stays up to date.")
	}
	return nil
}

var notInTarget = regexp.MustCompile(`[^a-z0-9]+`)

// targetFor makes the name of the folder. It holds no more than letters, digits and
// dashes, so it can never lead out of the folder for sites.
func targetFor(name string, id uuid.UUID) string {
	slug := strings.Trim(notInTarget.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	if slug == "" {
		slug = "site"
	}
	return slug + "-" + strings.ReplaceAll(id.String(), "-", "")[24:]
}

func record(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, v Site, details map[string]any) error {
	if _, err := audit.Record(ctx, tx, c.Event(action, "site", v.ID.String(), v.SpaceID.String(), details)); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"site_id": v.ID, "space_id": v.SpaceID})
	return err
}

// Create makes a site. It holds nothing until pages are chosen and it is published.
func (s *Service) Create(ctx context.Context, c identity.Caller, spaceID uuid.UUID, in Input) (Site, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SitePublish); err != nil {
		return Site{}, err
	}
	if err := s.requireEnabled(ctx); err != nil {
		return Site{}, err
	}
	if err := in.clean(); err != nil {
		return Site{}, err
	}
	id := ids.New()
	var out Site
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		out, err = scan(tx.QueryRow(ctx, `INSERT INTO public_sites (id, space_id, name, address, target, language, accent, show_names, search_engines, update_mode, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING `+siteColumns,
			id, spaceID, in.Name, in.Address, targetFor(in.Name, id), in.Language, in.Accent, in.ShowNames, in.SearchEngines, in.UpdateMode, c.UserID))
		if err != nil {
			return err
		}
		return record(ctx, tx, c, "site.created", out, map[string]any{"name": in.Name})
	})
	return out, err
}

// List gives the sites of a space.
func (s *Service) List(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]Site, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SitePublish); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+siteColumns+` FROM public_sites WHERE space_id = $1 ORDER BY name`, spaceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Site, error) { return scan(r) })
}

// Get gives one site.
func (s *Service) Get(ctx context.Context, c identity.Caller, id uuid.UUID) (Site, error) {
	return s.owned(ctx, c, id)
}

// Update changes a site. It reaches the world with the next build.
func (s *Service) Update(ctx context.Context, c identity.Caller, id uuid.UUID, in Input) (Site, error) {
	if _, err := s.owned(ctx, c, id); err != nil {
		return Site{}, err
	}
	if err := in.clean(); err != nil {
		return Site{}, err
	}
	var out Site
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		out, err = scan(tx.QueryRow(ctx, `UPDATE public_sites SET name = $2, address = $3, language = $4, accent = $5, show_names = $6,
			search_engines = $7, update_mode = $8, updated_at = now() WHERE id = $1 RETURNING `+siteColumns,
			id, in.Name, in.Address, in.Language, in.Accent, in.ShowNames, in.SearchEngines, in.UpdateMode))
		if err != nil {
			return err
		}
		return record(ctx, tx, c, "site.changed", out, map[string]any{"show_names": in.ShowNames, "search_engines": in.SearchEngines})
	})
	return out, err
}

// Delete takes a site down and removes it.
func (s *Service) Delete(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	v, err := s.owned(ctx, c, id)
	if err != nil {
		return err
	}
	if err := s.removeFolder(v); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM public_sites WHERE id = $1`, id); err != nil {
			return err
		}
		return record(ctx, tx, c, "site.withdrawn", v, map[string]any{"deleted": true})
	})
}

// PageChoice is a line of the list a person chooses from.
type PageChoice struct {
	ID       uuid.UUID  `json:"id"`
	ParentID *uuid.UUID `json:"parent_id"`
	Title    string     `json:"title"`
	Chosen   bool       `json:"chosen"`
	Blocked  bool       `json:"blocked"`
}

// Choices lists the pages a person can pick. A page that may not be public is
// listed as blocked: only someone who publishes for the space sees this, and drafts
// are not offered at all.
func (s *Service) Choices(ctx context.Context, c identity.Caller, id uuid.UUID) ([]PageChoice, error) {
	v, err := s.owned(ctx, c, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.parent_id, p.title,
			EXISTS (SELECT 1 FROM public_site_pages k WHERE k.site_id = $2 AND k.page_id = p.id),
			EXISTS (SELECT 1 FROM page_tree t JOIN page_restrictions r ON r.page_id = t.ancestor_id AND r.kind = 'view' WHERE t.descendant_id = p.id)
		FROM pages p JOIN perm_visible_pages($3) vis ON vis.page_id = p.id
		WHERE p.space_id = $1 AND p.status = 'published' AND p.archived_at IS NULL AND p.kind = ANY($4)
		ORDER BY (SELECT count(*) FROM page_tree a WHERE a.descendant_id = p.id), p.position, p.id`,
		v.SpaceID, id, c.UserID, publicKinds)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (PageChoice, error) {
		var p PageChoice
		err := r.Scan(&p.ID, &p.ParentID, &p.Title, &p.Chosen, &p.Blocked)
		return p, err
	})
}

// SetPages chooses the pages of a site, by hand (D15). A page that may not be public
// is refused, and it stays out.
func (s *Service) SetPages(ctx context.Context, c identity.Caller, id uuid.UUID, list []uuid.UUID) error {
	v, err := s.owned(ctx, c, id)
	if err != nil {
		return err
	}
	allowed, err := s.mayBePublic(ctx, s.pool, v.SpaceID, list)
	if err != nil {
		return err
	}
	if len(allowed) != len(uniqueIDs(list)) {
		return apperr.Conflict("page_not_public", "A page you chose has a restriction, or is not published.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM public_site_pages WHERE site_id = $1`, id); err != nil {
			return err
		}
		for page := range allowed {
			if _, err := tx.Exec(ctx, `INSERT INTO public_site_pages (site_id, page_id) VALUES ($1, $2)`, id, page); err != nil {
				return err
			}
		}
		return record(ctx, tx, c, "site.changed", v, map[string]any{"pages": len(allowed)})
	})
}

func uniqueIDs(list []uuid.UUID) map[uuid.UUID]bool {
	out := make(map[uuid.UUID]bool, len(list))
	for _, id := range list {
		out[id] = true
	}
	return out
}
