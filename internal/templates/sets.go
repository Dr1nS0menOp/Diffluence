// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/spaces"
)

const (
	// backgroundAbove is the size above which a set is made as background work (TPL-121).
	backgroundAbove = 25
	// setDeadline is the hard stop of one set's transaction (TPL-826).
	setDeadline = 60 * time.Second
	ellipsis    = "…"
	maxKeyBytes = 100
)

// SetInput says where a set goes and what was answered.
type SetInput struct {
	SpaceID  uuid.UUID         `json:"space_id"`
	ParentID *uuid.UUID        `json:"parent_id"`
	Title    string            `json:"title"`
	Answers  map[string]string `json:"answers"`
	// Version binds the form to the version it showed (TPL-603); 0 takes the newest.
	Version int `json:"version"`
	// RequestKey is made when the form opens, so a resent request makes one set (TPL-123).
	RequestKey string `json:"request_key"`
	// Replaces is the empty page that was waiting for a template (TPL-122).
	Replaces *uuid.UUID `json:"replaces"`
}

// PlannedPage is one page a set makes, or would make.
type PlannedPage struct {
	Entry       string    `json:"entry"`
	EntryName   string    `json:"entry_name"`
	ParentEntry string    `json:"parent_entry"`
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	// Exists marks a title a page of the space already has (TPL-141).
	Exists bool `json:"exists"`
}

// SetResult is a set that was made, or the job that will make it.
type SetResult struct {
	SetID         uuid.UUID     `json:"set_id"`
	Page          pages.Page    `json:"page"`
	Pages         []PlannedPage `json:"pages"`
	LabelsLeftOut []string      `json:"labels_left_out"`
	JobID         *uuid.UUID    `json:"job_id,omitempty"`
	// Again says the request was made before, and this is the set it made.
	Again bool `json:"-"`
}

// SetCheck is what the check step shows before anything is made (TPL-340, TPL-811).
type SetCheck struct {
	Version  int           `json:"version"`
	Pages    []PlannedPage `json:"pages"`
	ParentID *uuid.UUID    `json:"parent_id"`
	// DefaultParentUnusable says the template's default parent is gone or out of reach,
	// so the set goes to the top of the space (TPL-802).
	DefaultParentUnusable bool `json:"default_parent_unusable"`
	Background            bool `json:"background"`
}

// plan is a set worked out in full before a page exists.
type plan struct {
	t       Template
	version Version
	spaceID uuid.UUID
	parent  *uuid.UUID
	pages   []plannedEntry
	fields  []string
	// texts are the texts of the entries at the version, read once.
	texts map[string]document.Node
	// spaceName and today are fixed when the set is worked out (TPL-222); the date is the
	// database's, so nodes agree (TPL-857).
	spaceName, today string
	replaces         *uuid.UUID
	key              string
	check            SetCheck
}

type plannedEntry struct {
	PlannedPage
	entry  Entry
	parent *uuid.UUID
	doc    document.Node
}

// refusal is a set refused before anything was made, naming every entry and why (TPL-810).
func refusal(kind apperr.Kind, code string, reasons []string) error {
	return &apperr.Error{Kind: kind, Code: code, Message: "You cannot make this set. " + strings.Join(reasons, " ")}
}

var errSpaceGone = &apperr.Error{Kind: apperr.KindNotFound, Code: "space_gone",
	Message: "This space no longer exists, or you cannot add pages to it."}

// prepare works out a set and checks every right it needs, writing nothing but the
// version a plain template may be owed (TPL-1103).
func (s *Service) prepare(ctx context.Context, c identity.Caller, id uuid.UUID, in SetInput) (*plan, error) {
	if len(in.RequestKey) > maxKeyBytes {
		return nil, apperr.Invalid("The request key is too long.")
	}
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return nil, err
	}
	if t.Hidden || (t.SpaceID != nil && *t.SpaceID != in.SpaceID) {
		return nil, apperr.NotFound()
	}
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, in.SpaceID, permissions.PageCreate); err != nil {
		if e, ok := apperr.As(err); ok && e.Kind == apperr.KindNotFound {
			return nil, errSpaceGone
		}
		return nil, err
	}
	v, err := s.versionFor(ctx, c, t, in.Version)
	if err != nil {
		return nil, err
	}
	p := &plan{t: t, version: v, spaceID: in.SpaceID, replaces: in.Replaces, key: in.RequestKey}
	if p.spaceName, err = spaces.Name(ctx, s.pool, in.SpaceID); err != nil {
		return nil, errSpaceGone
	}
	p.today = t.today
	if p.texts, _, err = s.textsOf(ctx, s.pool, v.Manifest); err != nil {
		return nil, err
	}
	if err := s.requireGuidedPart(ctx, p); err != nil {
		return nil, err
	}
	if err := s.checkRights(ctx, c, p.version.Manifest, in.SpaceID); err != nil {
		return nil, err
	}
	if err := s.place(ctx, c, p, in); err != nil {
		return nil, err
	}
	if err := s.fill(ctx, c, p, in); err != nil {
		return nil, err
	}
	return p, nil
}

// versionFor is the version a form is bound to, or the one the template offers now. Only a
// template that is owed a version (TPL-1103) takes the lock of its row, to make it.
func (s *Service) versionFor(ctx context.Context, c identity.Caller, t Template, number int) (Version, error) {
	if number > 0 {
		return versionAt(ctx, s.pool, t.ID, number)
	}
	latest, found, err := latestVersion(ctx, s.pool, t.ID)
	if err != nil {
		return latest, err
	}
	if found {
		if t.Guided {
			return latest, nil
		}
		same, err := sameManifest(withRow(latest.Manifest, t, t.Version), latest.Manifest)
		if err != nil || same {
			return latest, err
		}
	}
	var v Version
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		locked, err := lockTemplate(ctx, tx, c.UserID, t.ID)
		if err != nil {
			return err
		}
		v, err = s.currentVersion(ctx, tx, c, locked)
		return err
	})
	return v, err
}

// requireGuidedPart hides a template that uses this feature while its part is off, which
// fails closed: it never falls back to something looser (TPL-050).
func (s *Service) requireGuidedPart(ctx context.Context, p *plan) error {
	if !p.version.Manifest.usesGuidedParts(p.texts) && !p.t.Guided {
		return nil
	}
	return s.features.RequireExperimental(ctx, s.pool, features.ExperimentalGuidedTemplates)
}

// checkRights refuses the whole set when the creator lacks a right that part of it needs,
// listing every such entry at once (TPL-130, TPL-152, TPL-810).
func (s *Service) checkRights(ctx context.Context, c identity.Caller, m Manifest, spaceID uuid.UUID) error {
	var restricted, governed bool
	for _, e := range m.Entries {
		restricted = restricted || e.Restrictions.any()
		governed = governed || e.WorkflowID != nil
	}
	if !restricted && !governed {
		return nil // a set that needs no right beyond page.create costs no query here
	}
	may := func(needed bool, permission string) (bool, error) {
		if !needed {
			return true, nil
		}
		err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permission)
		if _, refused := apperr.As(err); refused {
			return false, nil
		}
		return err == nil, err
	}
	mayRestrict, err := may(restricted, permissions.RestrictionManage)
	if err != nil {
		return err
	}
	spaceWorkflow := false
	if governed {
		if spaceWorkflow, err = s.pages.SpaceHasWorkflow(ctx, s.pool, spaceID); err != nil {
			return err
		}
	}
	mayWorkflow, err := may(governed && !spaceWorkflow, permissions.PageWorkflow)
	if err != nil {
		return err
	}
	var missingRights, missingPeople []string
	for _, e := range m.ordered() {
		if e.Restrictions.any() && !mayRestrict {
			missingRights = append(missingRights, entryLabel(e)+" is restricted, which needs the right to manage restrictions.")
		}
		if e.WorkflowID != nil && !spaceWorkflow && !mayWorkflow {
			missingRights = append(missingRights, entryLabel(e)+" follows a workflow, which needs the right to choose workflows.")
		}
		if !e.Restrictions.any() {
			continue
		}
		gone, err := s.perms.MissingSubjects(ctx, s.pool, append(append([]permissions.Subject{}, e.Restrictions.View...), e.Restrictions.Edit...))
		if err != nil {
			return err
		}
		if len(gone) > 0 {
			missingPeople = append(missingPeople, entryLabel(e)+" is restricted to a person or group that no longer exists.")
		}
	}
	if len(missingRights) > 0 {
		return refusal(apperr.KindForbidden, "set_refused", missingRights)
	}
	if len(missingPeople) > 0 {
		return refusal(apperr.KindConflict, "set_refused", missingPeople)
	}
	return nil
}

// place decides where the root goes: the chosen parent, which must still be usable
// (TPL-803), or the default parent, or the top of the space when that is gone (TPL-802).
func (s *Service) place(ctx context.Context, c identity.Caller, p *plan, in SetInput) error {
	if in.ParentID != nil {
		if err := s.pages.CheckParentForSet(ctx, s.pool, c, in.SpaceID, *in.ParentID); err != nil {
			return parentUnusable(err)
		}
		p.parent = in.ParentID
	} else if p.version.Manifest.DefaultParent != nil {
		p.parent = s.pages.UsableParent(ctx, c, p.version.Manifest.DefaultParent, in.SpaceID)
		p.check.DefaultParentUnusable = p.parent == nil
	}
	p.check.ParentID = p.parent
	var templatePages []uuid.UUID
	for _, e := range p.version.Manifest.Entries {
		if e.PageID != nil {
			templatePages = append(templatePages, *e.PageID)
		}
	}
	// One page is checked by the copy itself, before it stores anything; a set is checked
	// whole first, so none of its files is stored for nothing (TPL-103).
	if len(templatePages) < 2 {
		return nil
	}
	return attachments.RefuseCopiesThatDoNotFit(ctx, s.pool, templatePages, in.SpaceID)
}

func parentUnusable(err error) error {
	if e, ok := apperr.As(err); ok {
		return &apperr.Error{Kind: e.Kind, Code: "parent_unusable", Message: "The page chosen as the place for this set was deleted, " +
			"moved to another space, or closed to you. Choose another place; nothing was made."}
	}
	return err
}

// fill works out each page: its id, title, parent and text, with every answer and value
// in place. Answers become text, never markup (TPL-142, TPL-842).
func (s *Service) fill(ctx context.Context, c identity.Caller, p *plan, in SetInput) error {
	m := p.version.Manifest
	texts := p.texts
	answers := map[string]string{}
	for name, answer := range in.Answers {
		answers[name] = answer
	}
	seen := map[string]bool{}
	for _, e := range m.ordered() {
		for _, f := range document.Fields(texts[e.Key]) {
			if f.Required && strings.TrimSpace(answers[f.Name]) == "" {
				return apperr.Invalid("Fill in " + f.Name + ".")
			}
			if m.Publish {
				// A page published at once holds no question left open (TPL-151).
				answers[f.Name] = in.Answers[f.Name]
			}
			if !seen[f.Name] {
				seen[f.Name] = true
				p.fields = append(p.fields, f.Name)
			}
		}
	}
	spaceName, today := p.spaceName, p.today
	// In a title, a question nobody answered counts as empty, so the entry's name stands in.
	titleAnswers := map[string]string{}
	for _, name := range p.fields {
		titleAnswers[name] = answers[name]
	}
	ordered := m.ordered()
	pageOf := map[string]uuid.UUID{}
	for _, e := range ordered {
		pageOf[e.Key] = ids.New()
	}
	for i, e := range ordered {
		title, err := s.titleOf(e, i == 0, in.Title, spaceName, titleAnswers, today)
		if err != nil {
			return err
		}
		planned := plannedEntry{entry: e, PlannedPage: PlannedPage{Entry: e.Key, EntryName: e.Name, ParentEntry: e.Parent,
			ID: pageOf[e.Key], Title: title}}
		if i == 0 {
			planned.parent = p.parent
		} else {
			parent := pageOf[e.Parent]
			planned.parent = &parent
		}
		if e.PageID != nil {
			doc, err := document.FillFields(texts[e.Key], answers)
			if err != nil {
				return err
			}
			planned.doc = document.FillValues(doc, valuesOf(c, p.t, spaceName, today, pageOf))
		}
		p.pages = append(p.pages, planned)
	}
	p.summarise()
	return nil
}

// summarise fills what the check step and the answer of a set show.
func (p *plan) summarise() {
	p.check.Pages = make([]PlannedPage, 0, len(p.pages))
	for _, pp := range p.pages {
		p.check.Pages = append(p.check.Pages, pp.PlannedPage)
	}
	p.check.Version = p.version.Number
	p.check.Background = len(p.pages) > backgroundAbove
}

// valuesOf gives what a template knows without asking (TPL-201) and the pages of the
// set its links point to (TPL-110), as of the moment the set is made (TPL-222).
func valuesOf(c identity.Caller, t Template, spaceName, today string, pageOf map[string]uuid.UUID) func(string) (document.Node, bool) {
	link := func(id uuid.UUID) document.Node {
		return document.Node{Type: document.TypePageLink, Attrs: map[string]any{"page_id": id.String()}}
	}
	return func(source string) (document.Node, bool) {
		switch source {
		case document.ValueDate:
			return document.Node{Type: document.TypeDate, Attrs: map[string]any{"value": today}}, true
		case document.ValueSpace:
			return document.Node{Type: document.TypeText, Text: spaceName}, spaceName != ""
		case document.ValueTemplate:
			return document.Node{Type: document.TypeText, Text: t.Name}, t.Name != ""
		case document.ValueCreator:
			return document.Node{Type: document.TypeMention, Attrs: map[string]any{"id": c.UserID.String(), "kind": document.MentionUser}}, true
		case document.ValueSet:
			return link(pageOf[RootKey]), true
		}
		key, _ := strings.CutPrefix(source, "entry:")
		id, ok := pageOf[key]
		return link(id), ok
	}
}

// titleOf fills an entry's title pattern once (TPL-140, TPL-142). The first page takes
// the title the person typed, when there is one; an empty title takes the entry's name.
func (s *Service) titleOf(e Entry, root bool, typed, spaceName string, answers map[string]string, today string) (string, error) {
	if root && strings.TrimSpace(typed) != "" {
		title := strings.TrimSpace(typed)
		if !utf8.ValidString(title) || utf8.RuneCountInString(title) > maxTitleRunes || strings.ContainsAny(title, "\r\n\x00") {
			return "", apperr.Invalid("A title is one line of at most 255 characters.")
		}
		return title, nil
	}
	day, err := time.Parse(time.DateOnly, today)
	if err != nil {
		return "", err
	}
	title := strings.Join(strings.Fields(ExpandTitle(e.TitlePattern, spaceName, answers, day)), " ")
	if title == "" {
		title = e.Name
	}
	return cutTitle(title), nil
}

const maxTitleRunes = 255

// cutTitle keeps a title within its limit, cut at a word with an ellipsis (TPL-140).
func cutTitle(title string) string {
	runes := []rune(title)
	if len(runes) <= maxTitleRunes {
		return title
	}
	cut := string(runes[:maxTitleRunes-1])
	if space := strings.LastIndex(cut, " "); space > 0 {
		cut = cut[:space]
	}
	return strings.TrimRight(cut, " ") + ellipsis
}

// markTaken marks each title a page of the space already has, for the check step only
// (TPL-141): the real run does not need it.
func (s *Service) markTaken(ctx context.Context, p *plan) error {
	titles := make([]string, 0, len(p.pages))
	for _, pp := range p.pages {
		titles = append(titles, pp.Title)
	}
	taken, err := s.pages.TitlesTaken(ctx, s.pool, p.spaceID, titles)
	if err != nil {
		return err
	}
	for i := range p.check.Pages {
		p.check.Pages[i].Exists = taken[p.check.Pages[i].Title]
	}
	return nil
}

// CheckSet runs every check of a set and makes nothing: the dry run of the check step,
// which refuses exactly as the real run would (TPL-811).
func (s *Service) CheckSet(ctx context.Context, c identity.Caller, id uuid.UUID, in SetInput) (SetCheck, error) {
	p, err := s.prepare(ctx, c, id, in)
	if err != nil {
		return SetCheck{}, err
	}
	return p.check, s.markTaken(ctx, p)
}

// MakeSet makes a set from a template: every page, file, label, restriction and origin
// record in one transaction, or nothing (TPL-120). A set of more than 25 pages is
// queued as background work, still one transaction (TPL-121); a rule runs it at once.
func (s *Service) MakeSet(ctx context.Context, c identity.Caller, id uuid.UUID, in SetInput) (SetResult, error) {
	p, err := s.prepare(ctx, c, id, in)
	if err != nil {
		return SetResult{}, err
	}
	if len(p.pages) > backgroundAbove && c.Rule == nil {
		in.Version = p.version.Number
		job, err := s.queueSet(ctx, c, id, in)
		return SetResult{JobID: &job}, err
	}
	return s.makeFor(ctx, c, p)
}

// makeFor makes a prepared set for real.
func (s *Service) makeFor(ctx context.Context, c identity.Caller, p *plan) (SetResult, error) {
	hooks := s.hooks.Load()
	deadline := setDeadline
	if hooks != nil && hooks.Deadline > 0 {
		deadline = hooks.Deadline
	}
	ctx, cancel := context.WithTimeoutCause(ctx, deadline, errSetTooLong)
	defer cancel()
	var out SetResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		out, err = s.makeIn(ctx, tx, c, p, hooks)
		return err
	})
	if err != nil && errors.Is(context.Cause(ctx), errSetTooLong) {
		return SetResult{}, errSetTooLong
	}
	if err != nil {
		return SetResult{}, err // nothing of it exists
	}
	return out, nil
}

var errSetTooLong = errors.New("templates: the set took longer than its hard stop")

func step(ctx context.Context, hooks *SetHooks, name string, entry int) error {
	if hooks == nil || hooks.Step == nil {
		return nil
	}
	return hooks.Step(ctx, name, entry)
}

// makeIn is the transaction of a set. Rights were checked for the whole set in prepare;
// the space and the parent are checked again here, under the tree lock.
func (s *Service) makeIn(ctx context.Context, tx pgx.Tx, c identity.Caller, p *plan, hooks *SetHooks) (SetResult, error) {
	if err := s.perms.RequireSpace(ctx, tx, c.UserID, p.spaceID, permissions.PageCreate); err != nil {
		return SetResult{}, errSpaceGone
	}
	if err := s.pages.LockTree(ctx, tx, p.spaceID); err != nil {
		return SetResult{}, err
	}
	if p.parent != nil {
		if err := s.pages.CheckParentForSet(ctx, tx, c, p.spaceID, *p.parent); err != nil {
			return SetResult{}, parentUnusable(err)
		}
	}
	setID, again, err := s.claimSet(ctx, tx, c, p)
	if err != nil || again != nil {
		return derefResult(again), err
	}
	out := SetResult{SetID: setID, Pages: p.check.Pages, LabelsLeftOut: []string{}}
	for i, pp := range p.pages {
		if err := step(ctx, hooks, "page", i); err != nil {
			return out, err
		}
		made, skipped, err := s.pages.AddSetPage(ctx, tx, c, pages.SetPage{ID: pp.ID, SpaceID: p.spaceID, ParentID: pp.parent, Title: pp.Title,
			Folder: pp.entry.Kind == EntryFolder, TemplatePage: pp.entry.PageID, Document: pp.doc, Labels: pp.entry.Labels,
			Publish: p.version.Manifest.Publish, WorkflowID: pp.entry.WorkflowID,
			Before: func(name string) error { return step(ctx, hooks, name, i) }})
		if err != nil {
			return out, err
		}
		if i == 0 {
			out.Page = made
		}
		out.LabelsLeftOut = appendNew(out.LabelsLeftOut, skipped...)
		if err := s.restrict(ctx, tx, c, made.ID, pp.entry, func() error { return step(ctx, hooks, "restriction", i) }); err != nil {
			return out, err
		}
	}
	if err := step(ctx, hooks, "origin", 0); err != nil {
		return out, err
	}
	if err := s.recordOrigins(ctx, tx, c, setID, p); err != nil {
		return out, err
	}
	if p.replaces != nil {
		if err := s.pages.TrashEmptyPage(ctx, tx, c, *p.replaces); err != nil {
			return out, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO template_uses (user_id, template_id) VALUES ($1, $2)
		ON CONFLICT (user_id, template_id) DO UPDATE SET used_at = now()`, c.UserID, p.t.ID); err != nil {
		return out, err
	}
	if err := step(ctx, hooks, "audit", -1); err != nil {
		return out, err
	}
	return out, s.auditSet(ctx, tx, c, p, setID)
}

func derefResult(r *SetResult) SetResult {
	if r == nil {
		return SetResult{}
	}
	return *r
}

func appendNew(list []string, more ...string) []string {
	for _, x := range more {
		if !slices.Contains(list, x) {
			list = append(list, x)
		}
	}
	return list
}

// claimSet writes the row of the set first. A request sent twice waits here for the
// first, and then gets the set the first one made (TPL-123).
func (s *Service) claimSet(ctx context.Context, tx pgx.Tx, c identity.Caller, p *plan) (uuid.UUID, *SetResult, error) {
	setID := ids.New()
	var key *string
	if p.key != "" {
		key = &p.key
	}
	tag, err := tx.Exec(ctx, `INSERT INTO template_sets (id, template_id, version, space_id, root_page_id, created_by, page_count, request_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT (created_by, request_key) WHERE request_key IS NOT NULL DO NOTHING`,
		setID, p.t.ID, p.version.Number, p.spaceID, p.pages[0].ID, c.UserID, len(p.pages), key)
	if err != nil || tag.RowsAffected() == 1 {
		return setID, nil, err
	}
	var earlier SetResult
	var root *uuid.UUID
	if err := tx.QueryRow(ctx, "SELECT id, root_page_id FROM template_sets WHERE created_by = $1 AND request_key = $2",
		c.UserID, p.key).Scan(&earlier.SetID, &root); err != nil {
		return uuid.Nil, nil, err
	}
	if root != nil {
		earlier.Page = pages.Page{ID: *root}
	}
	earlier.Pages, earlier.LabelsLeftOut, earlier.Again = []PlannedPage{}, []string{}, true
	return uuid.Nil, &earlier, nil
}

// restrict puts the restrictions of an entry on its page in the same transaction, so the
// page never exists without them (TPL-130, TPL-1417).
func (s *Service) restrict(ctx context.Context, tx pgx.Tx, c identity.Caller, pageID uuid.UUID, e Entry, before func() error) error {
	if !e.Restrictions.any() {
		return nil
	}
	if err := before(); err != nil {
		return err
	}
	if err := s.perms.RestrictNewPage(ctx, tx, c, pageID, "view", e.Restrictions.View); err != nil {
		return err
	}
	return s.perms.RestrictNewPage(ctx, tx, c, pageID, "edit", e.Restrictions.Edit)
}

// recordOrigins writes where each page came from, in one statement (TPL-610, TPL-901).
func (s *Service) recordOrigins(ctx context.Context, tx pgx.Tx, c identity.Caller, setID uuid.UUID, p *plan) error {
	pageIDs, keys := make([]uuid.UUID, len(p.pages)), make([]string, len(p.pages))
	for i, pp := range p.pages {
		pageIDs[i], keys[i] = pp.ID, pp.Entry
	}
	_, err := tx.Exec(ctx, `INSERT INTO page_template_origin (page_id, set_id, template_id, version, entry_key, created_by)
		SELECT page, $2, $3, $4, entry, $5 FROM unnest($1::uuid[], $6::text[]) AS made (page, entry)`,
		pageIDs, setID, p.t.ID, p.version.Number, c.UserID, keys)
	return err
}

// auditSet records the set: field names and counts, never an answer (TPL-711, TPL-1409).
func (s *Service) auditSet(ctx context.Context, tx pgx.Tx, c identity.Caller, p *plan, setID uuid.UUID) error {
	details := map[string]any{"set_id": setID, "version": p.version.Number, "pages": len(p.pages), "fields": p.fields,
		"root_page_id": p.pages[0].ID, "name": p.t.Name}
	if _, err := audit.Record(ctx, tx, c.Event("template.set_created", "template", p.t.ID.String(), p.spaceID.String(), details)); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, "template.set_created", map[string]any{"template_id": p.t.ID, "set_id": setID,
		"space_id": p.spaceID, "page_id": p.pages[0].ID, "actor_id": c.UserID, "pages": len(p.pages)})
	return err
}

// describe names a step error for the person, never an internal detail.
func describe(err error) string {
	if e, ok := apperr.As(err); ok {
		return e.Message
	}
	if errors.Is(err, errSetTooLong) {
		return fmt.Sprintf("Making the set took longer than %v, so it was stopped and nothing was made.", setDeadline)
	}
	return "The set could not be made, and nothing was made."
}
