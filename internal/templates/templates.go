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

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/attachments"
	"github.com/kenwerk/kenwerk/internal/audit"
	"github.com/kenwerk/kenwerk/internal/conditions"
	"github.com/kenwerk/kenwerk/internal/db"
	"github.com/kenwerk/kenwerk/internal/document"
	"github.com/kenwerk/kenwerk/internal/features"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/ids"
	"github.com/kenwerk/kenwerk/internal/jobs"
	"github.com/kenwerk/kenwerk/internal/labels"
	"github.com/kenwerk/kenwerk/internal/outbox"
	"github.com/kenwerk/kenwerk/internal/pages"
	"github.com/kenwerk/kenwerk/internal/permissions"
	"github.com/kenwerk/kenwerk/internal/spaces"
	"github.com/kenwerk/kenwerk/internal/templates/fields"
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
	// Condition shows the step only when it holds, testing the questions of the steps before
	// it (spec 074, TPL-421). A step it hides hides its questions.
	Condition *conditions.Condition `json:"condition,omitempty"`
}

// Template is a way to start a page. SpaceID is nil for templates of all spaces.
type Template struct {
	ID            uuid.UUID  `json:"id"`
	SpaceID       *uuid.UUID `json:"space_id"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Category      string     `json:"category"`
	Labels        []string   `json:"labels"`
	TitlePattern  string     `json:"title_pattern"`
	Steps         []Step     `json:"steps"`
	DefaultParent *uuid.UUID `json:"default_parent"`
	Hidden        bool       `json:"hidden"`
	Promoted      bool       `json:"promoted"`
	Starter       bool       `json:"starter"`
	Favourite     bool       `json:"favourite"`
	UsedAt        *time.Time `json:"used_at"`
	Version       int        `json:"version"`
	// Fields are the questions the form asks, with their settings, as the version it is
	// bound to asks them (spec 074, TPL-603).
	Fields []fields.Spec `json:"fields,omitempty"`
	// Pages is how many pages the template makes, for the gallery (TPL-1021).
	Pages int `json:"pages,omitempty"`
	// Guided says the template uses a part of guided templates (spec 074, TPL-050).
	Guided bool `json:"guided"`
	// TemplateVersion is the version a form made from this answer is bound to (TPL-603).
	TemplateVersion int `json:"template_version"`
	// WorkflowID is the workflow the first page made from a template of a space enters,
	// without the part of guided templates (spec 064, DOC-560).
	WorkflowID *uuid.UUID      `json:"workflow_id"`
	Document   json.RawMessage `json:"document,omitempty"`
	// ConditionGroups are the groups the form's conditions test that the reader is in, and
	// those that are gone, so the form can preview them (spec 074, TPL-402). The server
	// decides again on submit.
	ConditionGroups *GroupFacts `json:"condition_groups,omitempty"`
	// State is the state of the newest version: draft, waiting, approved, sent back or
	// withdrawn (TPL-510). What is offered is TemplateVersion, the approved one.
	State string `json:"state"`
	// Mine says the person asking owns it.
	Mine bool `json:"mine"`
	// GroupsGone says a condition of the newest version tests a group that was deleted, for the
	// list's warning (TPL-402, TPL-1468).
	GroupsGone bool `json:"groups_gone,omitempty"`
	// ImportNotes say what an import left out or asks to choose again, in the answer to the
	// import only (TPL-852).
	ImportNotes *ImportNotes `json:"import_notes,omitempty"`
	pageID      uuid.UUID
	ownerID     uuid.UUID
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
	WorkflowID    *uuid.UUID      `json:"workflow_id"`
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
	}
	if err := checkSteps(in.Steps); err != nil {
		return err
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

// guidedOf says a template uses a part of guided templates: in what it makes, or in who has it,
// so that a template with a list is hidden while the part is off, never opened to all (TPL-050).
const guidedOf = `(t.guided OR t.offered_in_chosen OR EXISTS (SELECT 1 FROM template_rights r WHERE r.template_id = t.id))`

const columns = `t.id, t.page_id, t.space_id, t.name, t.description, t.category, t.labels, t.title_pattern, t.steps,
	t.default_parent, t.hidden, t.promoted, t.starter_key IS NOT NULL, f.user_id IS NOT NULL, u.used_at, p.current_version,
	` + guidedOf + `,
	COALESCE(t.approved_version, 0), COALESCE(t.owner_id, t.created_by), to_char(now(), 'YYYY-MM-DD'), t.workflow_id, t.state,
	COALESCE(t.owner_id, t.created_by) = $1`

const joins = `FROM templates t
	JOIN pages p ON p.id = t.page_id
	LEFT JOIN template_favourites f ON f.template_id = t.id AND f.user_id = $1
	LEFT JOIN template_uses u ON u.template_id = t.id AND u.user_id = $1`

func scan(r pgx.Row) (Template, error) {
	var t Template
	err := r.Scan(&t.ID, &t.pageID, &t.SpaceID, &t.Name, &t.Description, &t.Category, &t.Labels, &t.TitlePattern, &t.Steps,
		&t.DefaultParent, &t.Hidden, &t.Promoted, &t.Starter, &t.Favourite, &t.UsedAt, &t.Version,
		&t.Guided, &t.TemplateVersion, &t.ownerID, &t.today, &t.WorkflowID, &t.State, &t.Mine)
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

// requireManager refuses a person who does not manage the templates of a scope: a space, or
// all spaces when spaceID is nil.
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

// File opens a file of a template for whoever may see that template. A file of any other
// page does not come through here, whatever its id.
func (s *Service) File(ctx context.Context, c identity.Caller, id, fileID uuid.UUID) (attachments.Attachment, io.ReadSeekCloser, error) {
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return attachments.Attachment{}, nil, err
	}
	if err := s.requireSeen(ctx, c, t); err != nil {
		return attachments.Attachment{}, nil, err
	}
	pageIDs, err := s.entryPages(ctx, t)
	if err != nil {
		return attachments.Attachment{}, nil, err
	}
	return s.files.OpenOfPages(ctx, pageIDs, fileID)
}

// requireSeen answers "not found" to whoever may not know the template exists (TPL-504).
func (s *Service) requireSeen(ctx context.Context, c identity.Caller, t Template) error {
	st, err := s.standingOf(ctx, s.pool, c, t)
	if err != nil {
		return err
	}
	seen, err := s.maySee(ctx, s.pool, st)
	if err != nil || seen {
		return err
	}
	return apperr.NotFound()
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

// galleryFilter is who is offered a template in a space, as one query with the lists joined
// (TPL-900): $1 is the person, $2 the space, $3 whether guided templates are on and $4 the
// person's groups. A template of all spaces is offered in its chosen spaces only when it has
// them (TPL-502). Only an approved version is offered, or a draft to its own owner, to try it
// (TPL-510). The users list only narrows: page.create in the space is asked before (TPL-501).
const galleryFilter = `WHERE NOT t.hidden
	AND (t.space_id = $2 OR (t.space_id IS NULL AND (NOT t.offered_in_chosen
		OR EXISTS (SELECT 1 FROM template_spaces ts WHERE ts.template_id = t.id AND ts.space_id = $2))))
	AND (NOT ` + guidedOf + ` OR $3)
	AND (t.approved_version IS NOT NULL OR (COALESCE(t.owner_id, t.created_by) = $1 AND t.state IN ('draft', 'sent_back')))
	AND (NOT EXISTS (SELECT 1 FROM template_rights r WHERE r.template_id = t.id AND r.kind = 'use')
		OR EXISTS (SELECT 1 FROM template_rights r WHERE r.template_id = t.id AND r.kind = 'use'
			AND ((r.subject_type = 'user' AND r.subject_id = $1) OR (r.subject_type = 'group' AND r.subject_id = ANY ($4)))))`

// Gallery lists what a person can start a page from in a space: favourites and
// recently used first, then promoted ones.
func (s *Service) Gallery(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]Template, error) {
	return s.gallery(ctx, c, spaceID, "")
}

// maxSearchRunes bounds what the gallery's search box sends.
const maxSearchRunes = 100

// gallerySearch narrows the gallery to templates whose name, description or category holds
// the typed words, or the name of a page of the version in use (TPL-701). $5 is a LIKE pattern
// of what was typed, or empty for no search.
const gallerySearch = `
	AND ($5 = '' OR lower(t.name) LIKE $5 OR lower(t.description) LIKE $5 OR lower(t.category) LIKE $5
		OR EXISTS (SELECT 1 FROM template_versions sv, jsonb_array_elements(sv.manifest -> 'entries') se
			WHERE sv.template_id = t.id AND sv.number = t.approved_version AND lower(se ->> 'name') LIKE $5))`

// SearchGallery finds templates of the gallery on the server, for a space that offers more
// than the browser lists (TPL-701). It is the gallery with one more condition, so it never
// returns a template the person may not use (TPL-504).
func (s *Service) SearchGallery(ctx context.Context, c identity.Caller, spaceID uuid.UUID, text string) ([]Template, error) {
	if err := s.features.RequireExperimental(ctx, s.pool, features.ExperimentalGuidedTemplates); err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > maxSearchRunes {
		return nil, apperr.Invalid("Search for at most 100 characters.")
	}
	return s.gallery(ctx, c, spaceID, text)
}

// likeOf makes typed text a LIKE pattern that holds it anywhere, with % and _ taken as
// letters.
func likeOf(text string) string {
	if text == "" {
		return ""
	}
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(text)) + "%"
}

func (s *Service) gallery(ctx context.Context, c identity.Caller, spaceID uuid.UUID, text string) ([]Template, error) {
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
	user, err := s.perms.LoadUser(ctx, s.pool, c.UserID)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT "+columns+" "+joins+" "+galleryFilter+gallerySearch+`
		ORDER BY f.user_id IS NOT NULL DESC, u.used_at DESC NULLS LAST, t.promoted DESC, lower(t.name), t.id`,
		c.UserID, spaceID, guidedOn, user.Groups, likeOf(text))
	return collect(rows, err)
}

// Manage lists templates for their managers, hidden ones as well, with the state of each and
// a warning for a condition that tests a deleted group (TPL-1001, TPL-1468). A draft that was
// never proposed stays its owner's own while the owner is active (TPL-510, TPL-853).
func (s *Service) Manage(ctx context.Context, c identity.Caller, spaceID *uuid.UUID) ([]Template, error) {
	if err := s.requireManager(ctx, s.pool, c, spaceID); err != nil {
		return nil, err
	}
	if err := s.seed(ctx); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT "+columns+" "+joins+` WHERE t.space_id IS NOT DISTINCT FROM $2
		ORDER BY lower(t.name), t.id`, c.UserID, spaceID)
	list, err := collect(rows, err)
	if err != nil {
		return nil, err
	}
	if list, err = s.withoutUnproposedDrafts(ctx, list); err != nil {
		return nil, err
	}
	return list, s.markGroupsGone(ctx, list)
}

// Mine lists the templates a person owns in a space, for one who may propose templates there
// and need not manage them: My templates (TPL-1002).
func (s *Service) Mine(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]Template, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.TemplatePropose); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT "+columns+" "+joins+` WHERE t.space_id = $2 AND COALESCE(t.owner_id, t.created_by) = $1
		ORDER BY lower(t.name), t.id`, c.UserID, spaceID)
	list, err := collect(rows, err)
	if err != nil {
		return nil, err
	}
	return list, s.markGroupsGone(ctx, list)
}

// Get returns a template as a form opened now by this person shows it: the version offered
// to them, with its text and questions (TPL-523, TPL-603). Who manages it, or owns it, sees
// the newest version when none is approved yet.
func (s *Service) Get(ctx context.Context, c identity.Caller, id uuid.UUID) (Template, error) {
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return t, err
	}
	st, err := s.standingOf(ctx, s.pool, c, t)
	if err != nil {
		return t, err
	}
	if seen, err := s.maySee(ctx, s.pool, st); err != nil || !seen {
		return t, errOr(err, apperr.NotFound())
	}
	v, err := s.offeredVersion(ctx, c, st)
	if err != nil {
		return t, err
	}
	return s.withVersion(ctx, c, t, v)
}

// Newest returns a template as its newest version holds it, for its editor and for those who
// review it (TPL-1010).
func (s *Service) Newest(ctx context.Context, c identity.Caller, id uuid.UUID) (Template, error) {
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return t, err
	}
	if _, err := s.requireTo(ctx, s.pool, c, t, standing.mayLook); err != nil {
		return t, err
	}
	v, err := s.newestVersion(ctx, c, t)
	if err != nil {
		return t, err
	}
	return s.withVersion(ctx, c, t, v)
}

// withVersion fills in what one version of a template holds: the text of its first page, its
// questions, steps, title pattern and labels.
func (s *Service) withVersion(ctx context.Context, c identity.Caller, t Template, v Version) (Template, error) {
	docs, _, err := s.textsOf(ctx, s.pool, v.Manifest)
	if err != nil {
		return t, err
	}
	form, err := s.withGoneDefaults(ctx, s.pool, formOf(v.Manifest, docs))
	if err != nil {
		return t, err
	}
	t.Fields, t.Pages, t.TemplateVersion, t.Steps = form, len(v.Manifest.Entries), v.Number, v.Manifest.Steps
	if root, _, ok := v.Manifest.entry(RootKey); ok {
		t.TitlePattern, t.Labels = root.TitlePattern, root.Labels
	}
	if t.ConditionGroups, err = s.groupFactsOf(ctx, c, Form{Fields: form, Steps: v.Manifest.Steps}); err != nil {
		return t, err
	}
	t.Document, err = document.Marshal(texts(docs, RootKey))
	return t, err
}

// errOr gives err when there is one, and otherwise the refusal.
func errOr(err, refusal error) error {
	if err != nil {
		return err
	}
	return refusal
}

// GroupFacts are what the form needs to preview tests of the reader's groups.
type GroupFacts struct {
	Member  []string `json:"member"`
	Missing []string `json:"missing"`
}

// groupFactsOf reads the reader's groups only for a form whose questions or steps test them.
func (s *Service) groupFactsOf(ctx context.Context, c identity.Caller, form Form) (*GroupFacts, error) {
	tested := map[string]bool{}
	note := func(cond *conditions.Condition) {
		if cond != nil {
			for _, id := range cond.Groups() {
				tested[id] = true
			}
		}
	}
	for _, f := range form.Fields {
		note(f.Condition)
	}
	for _, st := range form.Steps {
		note(st.Condition)
	}
	if len(tested) == 0 {
		return nil, nil
	}
	user, err := s.perms.LoadUser(ctx, s.pool, c.UserID)
	if err != nil {
		return nil, err
	}
	out := &GroupFacts{Member: []string{}}
	for _, g := range user.Groups {
		if tested[g.String()] {
			out.Member = append(out.Member, g.String())
		}
	}
	if out.Missing, err = s.missingGroups(ctx, s.pool, groupIDs(tested)); out.Missing == nil {
		out.Missing = []string{}
	}
	return out, err
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

// insert makes a template and its version 1, which becomes what outcome says.
func (s *Service) insert(ctx context.Context, tx pgx.Tx, c identity.Caller, spaceID *uuid.UUID, in Input, doc document.Node, starter *string,
	outcome versionOutcome) (Template, error) {
	t, head, err := s.insertRow(ctx, tx, c, spaceID, in, doc, starter)
	if err != nil {
		return t, err
	}
	// Version 1, as every template that existed before versions got one (TPL-620).
	_, err = s.saveVersion(ctx, tx, c, t, rowManifest(t, head), outcome)
	return t, err
}

// insertRow makes the row of a template and the hidden page of its first entry, with no
// version yet, and gives the number of that page's text.
func (s *Service) insertRow(ctx context.Context, tx pgx.Tx, c identity.Caller, spaceID *uuid.UUID, in Input, doc document.Node,
	starter *string) (Template, int, error) {
	home, err := s.homeOf(ctx, tx, spaceID)
	if err != nil {
		return Template{}, 0, err
	}
	p, err := s.pages.NewTemplatePage(ctx, tx, c, home, in.Name, doc)
	if err != nil {
		return Template{}, 0, err
	}
	steps, err := json.Marshal(in.Steps)
	if err != nil {
		return Template{}, 0, err
	}
	id := ids.New()
	if err := s.checkWorkflow(ctx, tx, spaceID, in.WorkflowID); err != nil {
		return Template{}, 0, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO templates (id, page_id, space_id, name, description, category, labels, title_pattern,
			steps, default_parent, hidden, promoted, starter_key, created_by, owner_id, workflow_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14, $15)`,
		id, p.ID, spaceID, in.Name, in.Description, in.Category, in.Labels, in.TitlePattern, steps, in.DefaultParent,
		in.Hidden, in.Promoted, starter, c.UserID, in.WorkflowID); err != nil {
		return Template{}, 0, err
	}
	t := Template{ID: id, SpaceID: spaceID, Name: in.Name, TitlePattern: in.TitlePattern, Labels: in.Labels, Steps: in.Steps,
		DefaultParent: in.DefaultParent, pageID: p.ID, ownerID: c.UserID}
	return t, p.CurrentVersion, nil
}

// Create makes a template, of a space or of all spaces, for one who manages the templates
// there. Its first version is approved at once, unless the scope asks for a second person
// (TPL-511).
func (s *Service) Create(ctx context.Context, c identity.Caller, spaceID *uuid.UUID, in Input) (Template, error) {
	return s.create(ctx, c, spaceID, in, func(tx pgx.Tx) (versionOutcome, error) {
		if err := s.requireManager(ctx, tx, c, spaceID); err != nil {
			return approveAtOnce, err
		}
		on, err := fourEyesOn(ctx, tx, spaceID)
		if err != nil || !on {
			return approveAtOnce, err
		}
		return keepAsDraft, nil
	})
}

// CreateDraft makes a template of a space for one who may propose templates there, in draft:
// its owner tries it alone and proposes it when it is ready (TPL-061, TPL-512). A person who
// also manages the templates of the space publishes it as Create does.
func (s *Service) CreateDraft(ctx context.Context, c identity.Caller, spaceID uuid.UUID, in Input) (Template, error) {
	return s.create(ctx, c, &spaceID, in, func(tx pgx.Tx) (versionOutcome, error) {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.TemplatePropose); err != nil {
			return keepAsDraft, err
		}
		manager, err := s.perms.CheckSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceTemplates)
		if err != nil || !manager.Allowed {
			return keepAsDraft, err
		}
		on, err := fourEyesOn(ctx, tx, &spaceID)
		if err != nil || on {
			return keepAsDraft, err
		}
		return approveAtOnce, nil
	})
}

// create makes a template once allowed has said who may and what its first version becomes.
func (s *Service) create(ctx context.Context, c identity.Caller, spaceID *uuid.UUID, in Input,
	allowed func(tx pgx.Tx) (versionOutcome, error)) (Template, error) {
	var out Template
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Who may comes first, so whoever may not learns nothing from what they sent.
		outcome, err := allowed(tx)
		if err != nil {
			return err
		}
		if err := in.validate(); err != nil {
			return err
		}
		doc, err := in.document()
		if err != nil {
			return err
		}
		if outcome == keepAsDraft {
			in.Hidden, in.Promoted = false, false
		}
		if out, err = s.insert(ctx, tx, c, spaceID, in, doc, nil, outcome); err != nil {
			return err
		}
		return auditTemplate(ctx, tx, c, "template.created", out)
	})
	if err != nil {
		return out, err
	}
	return s.Newest(ctx, c, out.ID)
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
		st, err := s.requireTo(ctx, tx, c, t, standing.mayEdit)
		if err != nil {
			return err
		}
		if !st.manages() {
			// Hiding and promoting are for those who manage templates (TPL-060).
			in.Hidden, in.Promoted = t.Hidden, t.Promoted
		}
		outcome, err := outcomeFor(ctx, tx, st)
		if err != nil {
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
		if err := s.checkWorkflow(ctx, tx, t.SpaceID, in.WorkflowID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE templates SET name = $2, description = $3, category = $4, labels = $5, title_pattern = $6,
				steps = $7, default_parent = $8, hidden = $9, promoted = $10, workflow_id = $11, updated_at = now() WHERE id = $1`,
			id, in.Name, in.Description, in.Category, in.Labels, in.TitlePattern, steps, in.DefaultParent, in.Hidden, in.Promoted, in.WorkflowID); err != nil {
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
		_, err = s.saveVersion(ctx, tx, c, changed, withRow(before.Manifest, changed, heads[t.pageID]), outcome)
		return err
	})
	if err != nil {
		return Template{}, err
	}
	return s.Newest(ctx, c, id)
}

// Delete removes a template. Pages made from it stay as they are.
func (s *Service) Delete(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		t, err := load(ctx, tx, c.UserID, id)
		if err != nil {
			return err
		}
		// Its managers delete it, and its owner a draft that was never approved (TPL-853).
		if _, err := s.requireTo(ctx, tx, c, t, func(st standing) bool {
			return st.manages() || (st.owner && st.proposer && st.t.TemplateVersion == 0)
		}); err != nil {
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
	if err := s.requireSeen(ctx, c, t); err != nil {
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
	// Version binds the form to the version it showed (TPL-603); 0 takes the one offered now.
	Version int `json:"version"`
}

// Use starts pages from a template: one page, or the set a guided template makes, with
// the answers as plain text. The pages are unpublished pages with a draft unless the
// template publishes at once, and later changes to the template change nothing. It is
// the one-page case of MakeSet, so there is one way to make pages from a template.
func (s *Service) Use(ctx context.Context, c identity.Caller, id uuid.UUID, in UseInput) (SetResult, error) {
	return s.MakeSet(ctx, c, id, SetInput{SpaceID: in.SpaceID, ParentID: in.ParentID, Title: in.Title, Answers: in.Answers,
		Replaces: in.Replaces, Version: in.Version})
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

// checkWorkflow allows a template to name a workflow only of its own space: workflows belong
// to a space, so a template of all spaces names none (spec 064, DOC-560).
func (s *Service) checkWorkflow(ctx context.Context, tx pgx.Tx, spaceID, workflowID *uuid.UUID) error {
	if workflowID == nil {
		return nil
	}
	if spaceID == nil {
		return apperr.Invalid("A template of all spaces cannot name a workflow: workflows belong to a space.")
	}
	return s.pages.WorkflowOfSpace(ctx, tx, *workflowID, *spaceID)
}
