// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"encoding/json"
	"slices"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/conditions"
	"github.com/kenwerk/kenwerk/internal/document"
	"github.com/kenwerk/kenwerk/internal/features"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/permissions"
	"github.com/kenwerk/kenwerk/internal/templates/fields"
)

// A template as one file, for moving it between installations (ORG-059; spec 074, wave GT5,
// TPL-852, TPL-1106). The file holds what the template makes and nothing about who has it:
// no users or editors list, no state, no history, no owner, no reviewer and no person. People
// and workflows belong to one installation, so they never travel; a group travels by id and
// name only to be chosen again, and the import never approves anything.

// bundleFormat numbers the files guided templates export. A file without a number is from
// before them and holds one page. An older release knows no field "format" and refuses such a
// file whole, instead of importing a part of it (TPL-1106).
const bundleFormat = 2

// maxKeyRunes bounds the key of an entry a file names; Kenwerk makes keys of 36.
const maxKeyRunes = 64

// BundleSubject is a group a restriction of the file names, with its name so the importer
// can tell which to choose again.
type BundleSubject struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// BundleRestrictions are the restrictions of one page of the file: its groups, and how many
// people the export left out of each list.
type BundleRestrictions struct {
	View       []BundleSubject `json:"view,omitempty"`
	Edit       []BundleSubject `json:"edit,omitempty"`
	ViewPeople int             `json:"view_people,omitempty"`
	EditPeople int             `json:"edit_people,omitempty"`
}

func (r BundleRestrictions) any() bool {
	return len(r.View)+len(r.Edit)+r.ViewPeople+r.EditPeople > 0
}

// BundleEntry is one page of the file beyond the first.
type BundleEntry struct {
	Key          string                `json:"key"`
	Name         string                `json:"name"`
	Kind         string                `json:"kind"`
	Parent       string                `json:"parent"`
	TitlePattern string                `json:"title_pattern"`
	Labels       []string              `json:"labels"`
	Restrictions BundleRestrictions    `json:"restrictions"`
	Condition    *conditions.Condition `json:"condition,omitempty"`
	Document     json.RawMessage       `json:"document,omitempty"`
}

// Bundle is a template as one file. The fields up to Document are those of every file,
// which hold the first page; the rest come with format 2.
type Bundle struct {
	Format       int             `json:"format,omitempty"`
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Category     string          `json:"category"`
	Labels       []string        `json:"labels"`
	TitlePattern string          `json:"title_pattern"`
	Steps        []Step          `json:"steps"`
	Document     json.RawMessage `json:"document"`
	// Restrictions are those of the first page.
	Restrictions *BundleRestrictions `json:"restrictions,omitempty"`
	Entries      []BundleEntry       `json:"entries,omitempty"`
	Fields       []fields.Spec       `json:"fields,omitempty"`
	LabelRules   []LabelRule         `json:"label_rules,omitempty"`
	Publish      bool                `json:"publish,omitempty"`
}

// ImportNotes say what an import left out and what to choose again before approval.
type ImportNotes struct {
	// PeopleLeftOut counts the people of the other installation left out of restrictions.
	PeopleLeftOut int `json:"people_left_out"`
	// GroupsToChoose names the groups of restrictions that are not here.
	GroupsToChoose []string `json:"groups_to_choose"`
	// ConditionGroupsToChoose counts the groups conditions test that are not here: their tests
	// are false, and approval is refused until they are chosen again.
	ConditionGroupsToChoose int `json:"condition_groups_to_choose"`
	// SuggestionsLeftOut names the questions whose suggested person or page was left out.
	SuggestionsLeftOut []string `json:"suggestions_left_out"`
}

// Export gives a template as one file, to someone who manages it: the version in use, or the
// newest while none is approved.
func (s *Service) Export(ctx context.Context, c identity.Caller, id uuid.UUID) (Bundle, error) {
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return Bundle{}, err
	}
	if _, err := s.requireTo(ctx, s.pool, c, t, standing.manages); err != nil {
		return Bundle{}, err
	}
	v, err := s.newestVersion(ctx, c, t)
	if err != nil {
		return Bundle{}, err
	}
	if t.TemplateVersion > 0 && t.TemplateVersion != v.Number {
		if v, err = versionAt(ctx, s.pool, id, t.TemplateVersion); err != nil {
			return Bundle{}, err
		}
	}
	texts, _, err := s.textsOf(ctx, s.pool, v.Manifest)
	if err != nil {
		return Bundle{}, err
	}
	names, err := s.groupNamesOf(ctx, v.Manifest)
	if err != nil {
		return Bundle{}, err
	}
	return bundleOf(t, v.Manifest, texts, names)
}

// bundleOf writes one version of a template as a file.
func bundleOf(t Template, m Manifest, texts map[string]document.Node, groupNames map[uuid.UUID]string) (Bundle, error) {
	ordered := m.ordered()
	root := ordered[0]
	doc, err := document.Marshal(texts[RootKey])
	if err != nil {
		return Bundle{}, err
	}
	b := Bundle{Format: bundleFormat, Name: t.Name, Description: t.Description, Category: t.Category, Labels: root.Labels,
		TitlePattern: root.TitlePattern, Steps: m.Steps, Document: doc, LabelRules: m.LabelRules, Publish: m.Publish,
		Entries: []BundleEntry{}, Fields: []fields.Spec{}}
	if root.Restrictions.any() {
		r := exportRestrictions(root.Restrictions, groupNames)
		b.Restrictions = &r
	}
	for _, e := range ordered[1:] {
		out := BundleEntry{Key: e.Key, Name: e.Name, Kind: e.Kind, Parent: e.Parent, TitlePattern: e.TitlePattern,
			Labels: e.Labels, Restrictions: exportRestrictions(e.Restrictions, groupNames), Condition: e.Condition}
		if e.PageID != nil {
			if out.Document, err = document.Marshal(texts[e.Key]); err != nil {
				return Bundle{}, err
			}
		}
		b.Entries = append(b.Entries, out)
	}
	for _, spec := range m.Fields {
		kept, _ := withoutLocalIDs(spec)
		b.Fields = append(b.Fields, kept)
	}
	return b, nil
}

// exportRestrictions keeps the groups of a restriction with their names, and counts its people.
func exportRestrictions(r Restrictions, names map[uuid.UUID]string) BundleRestrictions {
	var out BundleRestrictions
	keep := func(list []permissions.Subject) ([]BundleSubject, int) {
		groups, people := []BundleSubject{}, 0
		for _, sub := range list {
			if sub.Type != permissions.SubjectGroup {
				people++
				continue
			}
			groups = append(groups, BundleSubject{Type: permissions.SubjectGroup, ID: sub.ID, Name: names[sub.ID]})
		}
		return groups, people
	}
	out.View, out.ViewPeople = keep(r.View)
	out.Edit, out.EditPeople = keep(r.Edit)
	return out
}

// withoutLocalIDs drops from a question what names a thing of one installation: a suggested
// person or page, and the space a page question looks in. It says whether anything was dropped.
func withoutLocalIDs(spec fields.Spec) (fields.Spec, bool) {
	dropped := spec.SpaceID != ""
	spec.SpaceID, spec.DefaultGone = "", false
	local := spec.Kind == fields.KindPerson || spec.Kind == fields.KindPeople || spec.Kind == fields.KindPage
	if local && spec.Default != nil && spec.Default.Value != "" {
		spec.Default, dropped = nil, true
	}
	return spec, dropped
}

// groupNamesOf names the groups the restrictions of a manifest list.
func (s *Service) groupNamesOf(ctx context.Context, m Manifest) (map[uuid.UUID]string, error) {
	var groups []uuid.UUID
	for _, e := range m.Entries {
		for _, sub := range slices.Concat(e.Restrictions.View, e.Restrictions.Edit) {
			if sub.Type == permissions.SubjectGroup {
				groups = append(groups, sub.ID)
			}
		}
	}
	return identity.DisplayNames(ctx, s.pool, groups)
}

// usesGuidedParts says the file needs the part guided templates (TPL-050).
func (b Bundle) usesGuidedParts(root document.Node) bool {
	stepConditions := slices.ContainsFunc(b.Steps, func(st Step) bool { return st.Condition != nil })
	restricted := b.Restrictions != nil && b.Restrictions.any()
	return len(b.Entries) > 0 || len(b.Fields) > 0 || len(b.LabelRules) > 0 || b.Publish || restricted || stepConditions ||
		document.HasTemplateValues(root) || document.HasConditionals(root)
}

// checkFormat refuses a file of a format this release does not know, and one without a
// number that holds what only numbered files hold, rather than importing a part of it.
func (b Bundle) checkFormat() error {
	switch b.Format {
	case 0:
		if len(b.Entries) > 0 || len(b.Fields) > 0 || len(b.LabelRules) > 0 || b.Publish || b.Restrictions != nil {
			return apperr.Invalid("This file is not a template file this release can read.")
		}
	case bundleFormat:
	default:
		return apperr.Invalid("This template file was made by a newer release. Upgrade before you import it.")
	}
	if len(b.Entries) >= maxEntries {
		return apperr.Invalid("A template can make at most 100 pages.")
	}
	return nil
}

// Import adds a template from a file. It goes through the same checks as saving, so an
// unknown block refuses the whole file. While guided templates are on, it makes a draft of
// the importer, which follows the approval rule of its scope (TPL-852); while they are off, a
// plain file is imported as before, and one that needs them is refused (TPL-050).
func (s *Service) Import(ctx context.Context, c identity.Caller, spaceID *uuid.UUID, b Bundle) (Template, error) {
	if err := b.checkFormat(); err != nil {
		return Template{}, err
	}
	in := Input{Name: b.Name, Description: b.Description, Category: b.Category, Labels: b.Labels,
		TitlePattern: b.TitlePattern, Steps: b.Steps, Document: b.Document}
	guidedOn, err := s.features.ExperimentalOn(ctx, s.pool, features.ExperimentalGuidedTemplates)
	if err != nil {
		return Template{}, err
	}
	if !guidedOn {
		root, err := in.document()
		if err != nil {
			return Template{}, err
		}
		if b.usesGuidedParts(root) {
			return Template{}, s.features.RequireExperimental(ctx, s.pool, features.ExperimentalGuidedTemplates)
		}
		return s.Create(ctx, c, spaceID, in)
	}
	var made Template
	var notes ImportNotes
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Who may comes first, so whoever may not learns nothing from what they sent.
		if err := s.requireManager(ctx, tx, c, spaceID); err != nil {
			return err
		}
		if err := in.validate(); err != nil {
			return err
		}
		root, err := in.document()
		if err != nil {
			return err
		}
		// An imported template has not been checked here: it waits as a draft, never hidden or
		// promoted, whoever imports it (TPL-852).
		in.Hidden, in.Promoted = false, false
		t, head, err := s.insertRow(ctx, tx, c, spaceID, in, root, nil)
		if err != nil {
			return err
		}
		m, err := s.importedManifest(ctx, tx, c, t, head, b, &notes)
		if err != nil {
			return err
		}
		if _, err := s.saveVersion(ctx, tx, c, t, m, keepAsDraft); err != nil {
			return err
		}
		made = t
		return auditTemplateDetails(ctx, tx, c, "template.created", t, map[string]any{"imported": true, "format": b.Format})
	})
	if err != nil {
		return Template{}, err
	}
	out, err := s.Newest(ctx, c, made.ID)
	out.ImportNotes = &notes
	return out, err
}

// importedManifest builds the first version of an imported template: the hidden page of
// every entry, its restrictions without the people of the other installation, and the
// questions without what names a thing of it.
func (s *Service) importedManifest(ctx context.Context, tx pgx.Tx, c identity.Caller, t Template, head int, b Bundle,
	notes *ImportNotes) (Manifest, error) {
	m := rowManifest(t, head)
	m.Publish, m.LabelRules = b.Publish, slices.Clone(b.LabelRules)
	if b.Restrictions != nil {
		m.Entries[0].Restrictions = importRestrictions(*b.Restrictions, c.UserID, notes)
	}
	home, err := s.homeOf(ctx, tx, t.SpaceID)
	if err != nil {
		return m, err
	}
	keys := map[string]bool{}
	for _, be := range b.Entries {
		if be.Key == "" || be.Key == RootKey || utf8.RuneCountInString(be.Key) > maxKeyRunes || keys[be.Key] {
			return m, apperr.Invalid("A page of this template file has no key, or one that cannot be used.")
		}
		keys[be.Key] = true
		e := Entry{Key: be.Key, Name: be.Name, Kind: be.Kind, Parent: be.Parent, TitlePattern: be.TitlePattern,
			Labels: slices.Clone(be.Labels), Condition: be.Condition,
			Restrictions: importRestrictions(be.Restrictions, c.UserID, notes)}
		if e.Kind == EntryPage {
			doc, err := EntryInput{Document: be.Document}.document()
			if err != nil {
				return m, err
			}
			p, err := s.pages.NewTemplatePage(ctx, tx, c, home, be.Name, doc)
			if err != nil {
				return m, err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO template_entry_pages (page_id, template_id, entry_key) VALUES ($1, $2, $3)",
				p.ID, t.ID, e.Key); err != nil {
				return m, err
			}
			e.PageID, e.PageVersion = &p.ID, p.CurrentVersion
		}
		m.Entries = append(m.Entries, e)
	}
	for _, spec := range b.Fields {
		kept, dropped := withoutLocalIDs(spec)
		if dropped {
			notes.SuggestionsLeftOut = append(notes.SuggestionsLeftOut, spec.Name)
		}
		m.Fields = append(m.Fields, kept)
	}
	return m, s.noteMissingGroups(ctx, tx, t, m, b, notes)
}

// importRestrictions keeps the groups of a restriction of the file and never a person: a
// person of the file, by count or by a subject that is not a group, is left out. A list left
// empty by that holds the importer, so the page stays restricted and never opens to everyone;
// the list is then to be chosen again (TPL-852, TPL-1417).
func importRestrictions(r BundleRestrictions, importer uuid.UUID, notes *ImportNotes) Restrictions {
	out := Restrictions{View: []permissions.Subject{}, Edit: []permissions.Subject{}}
	take := func(list []BundleSubject, people int) ([]permissions.Subject, bool) {
		subjects := []permissions.Subject{}
		for _, sub := range list {
			if sub.Type != permissions.SubjectGroup || sub.ID == uuid.Nil {
				people++
				continue
			}
			subjects = append(subjects, permissions.Subject{Type: permissions.SubjectGroup, ID: sub.ID})
		}
		notes.PeopleLeftOut += people
		if people > 0 && len(subjects) == 0 {
			subjects = append(subjects, permissions.Subject{Type: permissions.SubjectUser, ID: importer})
		}
		return subjects, people > 0
	}
	var viewLeft, editLeft bool
	out.View, viewLeft = take(r.View, r.ViewPeople)
	out.Edit, editLeft = take(r.Edit, r.EditPeople)
	out.ChooseAgain = viewLeft || editLeft
	return out
}

// noteMissingGroups marks every restriction that names a group not here to be chosen again,
// and names those groups, with those the conditions test, for the importer.
func (s *Service) noteMissingGroups(ctx context.Context, tx pgx.Tx, t Template, m Manifest, b Bundle, notes *ImportNotes) error {
	named := map[uuid.UUID]string{}
	note := func(list []BundleSubject) {
		for _, sub := range list {
			named[sub.ID] = sub.Name
		}
	}
	if b.Restrictions != nil {
		note(b.Restrictions.View)
		note(b.Restrictions.Edit)
	}
	for _, be := range b.Entries {
		note(be.Restrictions.View)
		note(be.Restrictions.Edit)
	}
	var subjects []permissions.Subject
	for i := range m.Entries {
		subjects = append(subjects, m.Entries[i].Restrictions.View...)
		subjects = append(subjects, m.Entries[i].Restrictions.Edit...)
	}
	gone, err := s.perms.MissingSubjects(ctx, tx, subjects)
	if err != nil {
		return err
	}
	for i := range m.Entries {
		r := &m.Entries[i].Restrictions
		if slices.ContainsFunc(gone, func(g permissions.Subject) bool {
			return slices.Contains(r.View, g) || slices.Contains(r.Edit, g)
		}) {
			r.ChooseAgain = true
		}
	}
	for _, g := range gone {
		if name := named[g.ID]; name != "" && !slices.Contains(notes.GroupsToChoose, name) {
			notes.GroupsToChoose = append(notes.GroupsToChoose, name)
		}
	}
	texts, _, err := s.textsOf(ctx, tx, m)
	if err != nil {
		return err
	}
	// The file names the groups of conditions by id only, so they are counted, not named.
	tested, err := s.missingGroups(ctx, tx, groupsTested(t, m, texts))
	if err != nil {
		return err
	}
	notes.ConditionGroupsToChoose = len(tested)
	if notes.GroupsToChoose == nil {
		notes.GroupsToChoose = []string{}
	}
	if notes.SuggestionsLeftOut == nil {
		notes.SuggestionsLeftOut = []string{}
	}
	return nil
}
