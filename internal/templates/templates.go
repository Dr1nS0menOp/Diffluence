// SPDX-License-Identifier: Apache-2.0

// Package templates owns page templates and space templates (spec 011). A template
// is a hidden page of kind template, so it gets the editor, versions and copied
// files for free. Evaluate refuses it to everyone, so this module holds the rule of
// who may use it, and reaches the page through unchecked calls, as exchange does.
package templates

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/labels"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/spaces"
)

const (
	maxNameRunes        = 100
	maxDescriptionRunes = 500
	maxCategoryRunes    = 50
	maxPatternRunes     = 255
	maxSteps            = 10
)

// Step groups fields, for a template that asks its questions one screen at a time.
type Step struct {
	Title  string   `json:"title"`
	Fields []string `json:"fields"`
}

// Template is a way to start a page. SpaceID is nil for templates of all spaces.
type Template struct {
	ID            uuid.UUID        `json:"id"`
	SpaceID       *uuid.UUID       `json:"space_id"`
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	Category      string           `json:"category"`
	Labels        []string         `json:"labels"`
	TitlePattern  string           `json:"title_pattern"`
	Steps         []Step           `json:"steps"`
	DefaultParent *uuid.UUID       `json:"default_parent"`
	Hidden        bool             `json:"hidden"`
	Promoted      bool             `json:"promoted"`
	Starter       bool             `json:"starter"`
	Favourite     bool             `json:"favourite"`
	UsedAt        *time.Time       `json:"used_at"`
	Version       int              `json:"version"`
	Fields        []document.Field `json:"fields,omitempty"`
	// Guided says the template uses a part of guided templates (spec 074, TPL-050).
	Guided bool `json:"guided"`
	// TemplateVersion is the version a form made from this answer is bound to (TPL-603).
	TemplateVersion int             `json:"template_version"`
	Document        json.RawMessage `json:"document,omitempty"`
	pageID          uuid.UUID
	ownerID         uuid.UUID
	// today is the database's date when the row was read, for what a set fills in (TPL-857).
	today string
}

// Input is what a person fills in for a template.
type Input struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Category      string          `json:"category"`
	Labels        []string        `json:"labels"`
	TitlePattern  string          `json:"title_pattern"`
	Steps         []Step          `json:"steps"`
	DefaultParent *uuid.UUID      `json:"default_parent"`
	Hidden        bool            `json:"hidden"`
	Promoted      bool            `json:"promoted"`
	Document      json.RawMessage `json:"document"`
}

func (in *Input) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Name == "" || utf8.RuneCountInString(in.Name) > maxNameRunes:
		return apperr.Invalid("A template needs a name of at most 100 characters.")
	case utf8.RuneCountInString(in.Description) > maxDescriptionRunes:
		return apperr.Invalid("A description can have at most 500 characters.")
	case utf8.RuneCountInString(in.Category) > maxCategoryRunes:
		return apperr.Invalid("A category can have at most 50 characters.")
	case utf8.RuneCountInString(in.TitlePattern) > maxPatternRunes:
		return apperr.Invalid("A title pattern can have at most 255 characters.")
	case len(in.Steps) > maxSteps:
		return apperr.Invalid("A template can have at most ten steps.")
	}
	for i, raw := range in.Labels {
		label, err := labels.Clean(raw)
		if err != nil {
			return err
		}
		in.Labels[i] = label
	}
	if in.Labels == nil {
		in.Labels = []string{}
	}
	if in.Steps == nil {
		in.Steps = []Step{}
	}
	return nil
}

func (in Input) document() (document.Node, error) {
	if len(in.Document) == 0 {
		return document.Empty(), nil
	}
	doc, err := document.Normalize(in.Document)
	if err != nil {
		return doc, err
	}
	return doc, document.CheckFields(doc)
}

// Service manages templates.
type Service struct {
	pool     *pgxpool.Pool
	perms    *permissions.Service
	pages    *pages.Service
	spaces   *spaces.Service
	identity *identity.Service
	files    *attachments.Service
	features *features.Service
	jobs     *jobs.Service
	seeded   atomic.Bool
	hooks    atomic.Pointer[SetHooks]
}

// Deps are the modules templates work through.
type Deps struct {
	Pool     *pgxpool.Pool
	Perms    *permissions.Service
	Pages    *pages.Service
	Spaces   *spaces.Service
	Identity *identity.Service
	Files    *attachments.Service
	Features *features.Service
	Jobs     *jobs.Service
}

// NewService creates the service.
func NewService(d Deps) *Service {
	s := &Service{pool: d.Pool, perms: d.Perms, pages: d.Pages, spaces: d.Spaces, identity: d.Identity, files: d.Files,
		features: d.Features, jobs: d.Jobs}
	d.Jobs.Register(jobSet, s.runSetJob)
	return s
}

const columns = `t.id, t.page_id, t.space_id, t.name, t.description, t.category, t.labels, t.title_pattern, t.steps,
	t.default_parent, t.hidden, t.promoted, t.starter_key IS NOT NULL, f.user_id IS NOT NULL, u.used_at, p.current_version,
	t.guided, COALESCE(t.approved_version, 0), COALESCE(t.owner_id, t.created_by), to_char(now(), 'YYYY-MM-DD')`

const joins = `FROM templates t
	JOIN pages p ON p.id = t.page_id
	LEFT JOIN template_favourites f ON f.template_id = t.id AND f.user_id = $1
	LEFT JOIN template_uses u ON u.template_id = t.id AND u.user_id = $1`

func scan(r pgx.Row) (Template, error) {
	var t Template
	err := r.Scan(&t.ID, &t.pageID, &t.SpaceID, &t.Name, &t.Description, &t.Category, &t.Labels, &t.TitlePattern, &t.Steps,
		&t.DefaultParent, &t.Hidden, &t.Promoted, &t.Starter, &t.Favourite, &t.UsedAt, &t.Version,
		&t.Guided, &t.TemplateVersion, &t.ownerID, &t.today)
	return t, err
}

func collect(rows pgx.Rows, err error) ([]Template, error) {
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Template, error) { return scan(r) })
	if out == nil {
		out = []Template{}
	}
	return out, err
}

func load(ctx context.Context, q db.Querier, userID, id uuid.UUID) (Template, error) {
	t, err := scan(q.QueryRow(ctx, "SELECT "+columns+" "+joins+" WHERE t.id = $2", userID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return t, apperr.NotFound()
	}
	return t, err
}

// mayRead: a template of all spaces is for everyone who uses the product, and one
// of a space for those who may view that space.
func (s *Service) mayRead(ctx context.Context, q db.Querier, c identity.Caller, t Template) error {
	if t.SpaceID != nil {
		return s.perms.RequireSpace(ctx, q, c.UserID, *t.SpaceID, permissions.SpaceView)
	}
	return nil
}

func (s *Service) requireManager(ctx context.Context, q db.Querier, c identity.Caller, spaceID *uuid.UUID) error {
	if spaceID != nil {
		return s.perms.RequireSpace(ctx, q, c.UserID, *spaceID, permissions.SpaceTemplates)
	}
	admin, err := s.identity.HasGlobalPermissionIn(ctx, q, c.UserID, identity.PermAdministrator)
	if err != nil {
		return err
	}
	if !admin {
		return apperr.Forbidden()
	}
	return nil
}

// File opens a file of a template for whoever may read that template. A file of any
// other page does not come through here, whatever its id.
func (s *Service) File(ctx context.Context, c identity.Caller, id, fileID uuid.UUID) (attachments.Attachment, io.ReadSeekCloser, error) {
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return attachments.Attachment{}, nil, err
	}
	if err := s.mayRead(ctx, s.pool, c, t); err != nil {
		return attachments.Attachment{}, nil, err
	}
	pageIDs, err := s.entryPages(ctx, t)
	if err != nil {
		return attachments.Attachment{}, nil, err
	}
	return s.files.OpenOfPages(ctx, pageIDs, fileID)
}

// entryPages lists the hidden pages of a template: its own and those of its entries.
func (s *Service) entryPages(ctx context.Context, t Template) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `SELECT page_id FROM template_entry_pages WHERE template_id = $1`, t.ID)
	if err != nil {
		return nil, err
	}
	more, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	return append([]uuid.UUID{t.pageID}, more...), err
}

// Gallery lists what a person can start a page from in a space: favourites and
// recently used first, then promoted ones.
func (s *Service) Gallery(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]Template, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.PageCreate); err != nil {
		return nil, err
	}
	if err := s.seed(ctx); err != nil {
		return nil, err
	}
	// A template that uses guided templates is not offered while that part is off (TPL-050).
	guidedOn, err := s.features.ExperimentalOn(ctx, s.pool, features.ExperimentalGuidedTemplates)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT "+columns+" "+joins+` WHERE NOT t.hidden AND (t.space_id = $2 OR t.space_id IS NULL)
		AND (NOT t.guided OR $3)
		ORDER BY f.user_id IS NOT NULL DESC, u.used_at DESC NULLS LAST, t.promoted DESC, lower(t.name), t.id`, c.UserID, spaceID, guidedOn)
	return collect(rows, err)
}

// Manage lists templates for their managers, hidden ones as well.
func (s *Service) Manage(ctx context.Context, c identity.Caller, spaceID *uuid.UUID) ([]Template, error) {
	if err := s.requireManager(ctx, s.pool, c, spaceID); err != nil {
		return nil, err
	}
	if err := s.seed(ctx); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT "+columns+" "+joins+` WHERE t.space_id IS NOT DISTINCT FROM $2
		ORDER BY lower(t.name), t.id`, c.UserID, spaceID)
	return collect(rows, err)
}

// Get returns a template with its text and questions.
func (s *Service) Get(ctx context.Context, c identity.Caller, id uuid.UUID) (Template, error) {
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return t, err
	}
	if err := s.mayRead(ctx, s.pool, c, t); err != nil {
		return t, err
	}
	doc, err := s.pages.TemplateDocument(ctx, s.pool, t.pageID)
	if err != nil {
		return t, err
	}
	t.Fields = document.Fields(doc)
	t.Document, err = document.Marshal(doc)
	return t, err
}

func auditTemplate(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, t Template) error {
	return auditTemplateDetails(ctx, tx, c, action, t, map[string]any{})
}

func auditTemplateDetails(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, t Template, details map[string]any) error {
	spaceID := ""
	if t.SpaceID != nil {
		spaceID = t.SpaceID.String()
	}
	details["name"] = t.Name
	if _, err := audit.Record(ctx, tx, c.Event(action, "template", t.ID.String(), spaceID, details)); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"template_id": t.ID})
	return err
}

// homeOf is where the hidden page of a template lives: its space, or the system
// space for a template of all spaces.
func (s *Service) homeOf(ctx context.Context, tx pgx.Tx, spaceID *uuid.UUID) (uuid.UUID, error) {
	if spaceID != nil {
		return *spaceID, nil
	}
	return s.spaces.SystemSpace(ctx, tx)
}

func (s *Service) insert(ctx context.Context, tx pgx.Tx, c identity.Caller, spaceID *uuid.UUID, in Input, doc document.Node, starter *string) (Template, error) {
	home, err := s.homeOf(ctx, tx, spaceID)
	if err != nil {
		return Template{}, err
	}
	p, err := s.pages.NewTemplatePage(ctx, tx, c, home, in.Name, doc)
	if err != nil {
		return Template{}, err
	}
	steps, err := json.Marshal(in.Steps)
	if err != nil {
		return Template{}, err
	}
	id := ids.New()
	if _, err := tx.Exec(ctx, `INSERT INTO templates (id, page_id, space_id, name, description, category, labels, title_pattern,
			steps, default_parent, hidden, promoted, starter_key, created_by, owner_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14)`,
		id, p.ID, spaceID, in.Name, in.Description, in.Category, in.Labels, in.TitlePattern, steps, in.DefaultParent,
		in.Hidden, in.Promoted, starter, c.UserID); err != nil {
		return Template{}, err
	}
	t := Template{ID: id, SpaceID: spaceID, Name: in.Name, TitlePattern: in.TitlePattern, Labels: in.Labels, Steps: in.Steps,
		DefaultParent: in.DefaultParent, pageID: p.ID, ownerID: c.UserID}
	// Version 1, as every template that existed before versions got one (TPL-620).
	_, err = s.saveVersion(ctx, tx, c, t, rowManifest(t, p.CurrentVersion))
	return t, err
}

// Create makes a template, of a space or of all spaces.
func (s *Service) Create(ctx context.Context, c identity.Caller, spaceID *uuid.UUID, in Input) (Template, error) {
	if err := in.validate(); err != nil {
		return Template{}, err
	}
	doc, err := in.document()
	if err != nil {
		return Template{}, err
	}
	var out Template
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.requireManager(ctx, tx, c, spaceID); err != nil {
			return err
		}
		var err error
		if out, err = s.insert(ctx, tx, c, spaceID, in, doc, nil); err != nil {
			return err
		}
		return auditTemplate(ctx, tx, c, "template.created", out)
	})
	if err != nil {
		return out, err
	}
	return s.Get(ctx, c, out.ID)
}

// Update changes a template. New text makes a new version, and changes no page
// that was made from it.
func (s *Service) Update(ctx context.Context, c identity.Caller, id uuid.UUID, in Input) (Template, error) {
	if err := in.validate(); err != nil {
		return Template{}, err
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		t, err := lockTemplate(ctx, tx, c.UserID, id)
		if err != nil {
			return err
		}
		if err := s.requireManager(ctx, tx, c, t.SpaceID); err != nil {
			return err
		}
		before, err := s.currentVersion(ctx, tx, c, t)
		if err != nil {
			return err
		}
		if len(in.Document) > 0 {
			doc, err := in.document()
			if err != nil {
				return err
			}
			if err := s.pages.NewTemplateVersion(ctx, tx, c, t.pageID, in.Name, doc); err != nil {
				return err
			}
		}
		steps, err := json.Marshal(in.Steps)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE templates SET name = $2, description = $3, category = $4, labels = $5, title_pattern = $6,
				steps = $7, default_parent = $8, hidden = $9, promoted = $10, updated_at = now() WHERE id = $1`,
			id, in.Name, in.Description, in.Category, in.Labels, in.TitlePattern, steps, in.DefaultParent, in.Hidden, in.Promoted); err != nil {
			return err
		}
		if err := auditTemplate(ctx, tx, c, "template.updated", t); err != nil {
			return err
		}
		// Whatever this changed of what the template makes is its next version (TPL-600).
		changed, err := load(ctx, tx, c.UserID, id)
		if err != nil {
			return err
		}
		heads, err := s.pages.TemplateHeads(ctx, tx, []uuid.UUID{t.pageID})
		if err != nil {
			return err
		}
		_, err = s.saveVersion(ctx, tx, c, changed, withRow(before.Manifest, changed, heads[t.pageID]))
		return err
	})
	if err != nil {
		return Template{}, err
	}
	return s.Get(ctx, c, id)
}

// Delete removes a template. Pages made from it stay as they are.
func (s *Service) Delete(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		t, err := load(ctx, tx, c.UserID, id)
		if err != nil {
			return err
		}
		if err := s.requireManager(ctx, tx, c, t.SpaceID); err != nil {
			return err
		}
		// The hidden pages of the entries go with it; the pages made from it stay (TPL-601).
		rows, err := tx.Query(ctx, `SELECT page_id FROM template_entry_pages WHERE template_id = $1`, id)
		if err != nil {
			return err
		}
		entryPages, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		if err := s.pages.DeleteTemplatePages(ctx, tx, entryPages); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM pages WHERE id = $1", t.pageID); err != nil {
			return err
		}
		return auditTemplate(ctx, tx, c, "template.deleted", t)
	})
}

// SetFavourite stars a template for a person.
func (s *Service) SetFavourite(ctx context.Context, c identity.Caller, id uuid.UUID, on bool) error {
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return err
	}
	if err := s.mayRead(ctx, s.pool, c, t); err != nil {
		return err
	}
	if on {
		_, err = s.pool.Exec(ctx, "INSERT INTO template_favourites (user_id, template_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", c.UserID, id)
	} else {
		_, err = s.pool.Exec(ctx, "DELETE FROM template_favourites WHERE user_id = $1 AND template_id = $2", c.UserID, id)
	}
	return err
}

// ExpandTitle fills in {date}, {space} and the names of fields.
func ExpandTitle(pattern, spaceName string, answers map[string]string, today time.Time) string {
	pairs := []string{"{date}", today.Format(time.DateOnly), "{space}", spaceName}
	names := make([]string, 0, len(answers))
	for name := range answers {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		pairs = append(pairs, "{"+name+"}", answers[name])
	}
	return strings.TrimSpace(strings.NewReplacer(pairs...).Replace(pattern))
}

// UseInput says where a new page goes, and what was answered.
type UseInput struct {
	SpaceID  uuid.UUID         `json:"space_id"`
	ParentID *uuid.UUID        `json:"parent_id"`
	Title    string            `json:"title"`
	Answers  map[string]string `json:"answers"`
	// Replaces is the empty page that was waiting for the template (TPL-122).
	Replaces *uuid.UUID `json:"replaces"`
}

// Use starts pages from a template: one page, or the set a guided template makes, with
// the answers as plain text. The pages are unpublished pages with a draft unless the
// template publishes at once, and later changes to the template change nothing. It is
// the one-page case of MakeSet, so there is one way to make pages from a template.
func (s *Service) Use(ctx context.Context, c identity.Caller, id uuid.UUID, in UseInput) (SetResult, error) {
	return s.MakeSet(ctx, c, id, SetInput{SpaceID: in.SpaceID, ParentID: in.ParentID, Title: in.Title, Answers: in.Answers,
		Replaces: in.Replaces})
}

// FromPage saves a page as a template of its space. Its files are copied.
func (s *Service) FromPage(ctx context.Context, c identity.Caller, pageID uuid.UUID, name string) (Template, error) {
	detail, err := s.pages.Get(ctx, c, pageID)
	if err != nil {
		return Template{}, err
	}
	in := Input{Name: name, Labels: detail.Labels, Document: detail.Document}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = detail.Title
	}
	if err := s.pages.RequireRoomForFiles(ctx, pageID, detail.SpaceID); err != nil {
		return Template{}, err
	}
	t, err := s.Create(ctx, c, &detail.SpaceID, in)
	if err != nil {
		return t, err
	}
	stored, err := load(ctx, s.pool, c.UserID, t.ID)
	if err != nil {
		return t, err
	}
	return t, s.pages.CopyFilesToTemplate(ctx, c, pageID, stored.pageID, detail.SpaceID)
}

// Bundle is a template as one file, for moving it between instances.
type Bundle struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Category     string          `json:"category"`
	Labels       []string        `json:"labels"`
	TitlePattern string          `json:"title_pattern"`
	Steps        []Step          `json:"steps"`
	Document     json.RawMessage `json:"document"`
}

// Export gives a template as one file, to someone who may manage it.
func (s *Service) Export(ctx context.Context, c identity.Caller, id uuid.UUID) (Bundle, error) {
	t, err := s.Get(ctx, c, id)
	if err != nil {
		return Bundle{}, err
	}
	if err := s.requireManager(ctx, s.pool, c, t.SpaceID); err != nil {
		return Bundle{}, err
	}
	return Bundle{Name: t.Name, Description: t.Description, Category: t.Category, Labels: t.Labels,
		TitlePattern: t.TitlePattern, Steps: t.Steps, Document: t.Document}, nil
}

// Import adds a template from a file. It goes through the same checks as saving,
// so an unknown block refuses the whole file.
func (s *Service) Import(ctx context.Context, c identity.Caller, spaceID *uuid.UUID, b Bundle) (Template, error) {
	return s.Create(ctx, c, spaceID, Input{Name: b.Name, Description: b.Description, Category: b.Category, Labels: b.Labels,
		TitlePattern: b.TitlePattern, Steps: b.Steps, Document: b.Document})
}
