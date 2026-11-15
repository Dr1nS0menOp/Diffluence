// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/pages"
)

// Version is one numbered, unchangeable snapshot of a template (TPL-600).
type Version struct {
	Number    int       `json:"number"`
	AuthorID  uuid.UUID `json:"author_id"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	// SupersededAt is when an approved version was replaced by the next approval; a form
	// opened on it before keeps working for a day (TPL-603).
	SupersededAt *time.Time `json:"superseded_at,omitempty"`
	Manifest     Manifest   `json:"-"`
}

func scanVersion(r pgx.Row) (Version, error) {
	var v Version
	var raw []byte
	if err := r.Scan(&v.Number, &raw, &v.AuthorID, &v.State, &v.CreatedAt, &v.SupersededAt); err != nil {
		return v, err
	}
	err := json.Unmarshal(raw, &v.Manifest)
	v.Manifest.normalise()
	return v, err
}

const versionColumns = "number, manifest, author_id, state, created_at, superseded_at"

// versionAt reads one version; a number the template never had is not found.
func versionAt(ctx context.Context, q db.Querier, templateID uuid.UUID, number int) (Version, error) {
	v, err := scanVersion(q.QueryRow(ctx, "SELECT "+versionColumns+" FROM template_versions WHERE template_id = $1 AND number = $2",
		templateID, number))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, apperr.NotFound()
	}
	return v, err
}

// latestVersion reads the newest version, or reports that there is none yet.
func latestVersion(ctx context.Context, q db.Querier, templateID uuid.UUID) (Version, bool, error) {
	v, err := scanVersion(q.QueryRow(ctx, "SELECT "+versionColumns+" FROM template_versions WHERE template_id = $1 ORDER BY number DESC LIMIT 1",
		templateID))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, false, nil
	}
	return v, err == nil, err
}

// rowManifest is what the row and the text of a template make on their own: a template
// with one page, as every template was before versions (TPL-620).
func rowManifest(t Template, head int) Manifest {
	page := t.pageID
	m := Manifest{Entries: []Entry{{Key: RootKey, Kind: EntryPage, PageID: &page, PageVersion: head,
		TitlePattern: t.TitlePattern, Labels: slices.Clone(t.Labels)}}, Steps: t.Steps, DefaultParent: t.DefaultParent}
	m.normalise()
	return m
}

// withRow brings the parts of a manifest that the row also holds up to date with it: the
// first entry's title pattern and labels, the steps, the default parent and its text.
func withRow(m Manifest, t Template, head int) Manifest {
	// A copy: the manifest given is still compared with what this returns.
	m.Entries = slices.Clone(m.Entries)
	root := &m.Entries[0]
	root.TitlePattern, root.Labels, root.PageVersion = t.TitlePattern, slices.Clone(t.Labels), head
	m.Steps, m.DefaultParent = slices.Clone(t.Steps), t.DefaultParent
	m.normalise()
	return m
}

// lockTemplate takes the row of a template for the rest of tx, so that two saves, or a
// save and a first use, make their versions one after the other. NO KEY leaves a set that is
// being made from it, which holds a key share through template_sets, to go on.
func lockTemplate(ctx context.Context, tx pgx.Tx, userID, id uuid.UUID) (Template, error) {
	if _, err := tx.Exec(ctx, "SELECT 1 FROM templates WHERE id = $1 FOR NO KEY UPDATE", id); err != nil {
		return Template{}, err
	}
	return load(ctx, tx, userID, id)
}

// currentVersion is the newest version of a template, which its editor changes. A plain
// template that the previous release saved, which writes no version, gets its next version
// from its row and text here, on first read (TPL-1103); it takes the state of the version
// before it. tx holds the lock of lockTemplate.
func (s *Service) currentVersion(ctx context.Context, tx pgx.Tx, c identity.Caller, t Template) (Version, error) {
	// t.Version is the current version of the template's text, read with the row.
	head := t.Version
	latest, found, err := latestVersion(ctx, tx, t.ID)
	if err != nil {
		return Version{}, err
	}
	if found && t.Guided {
		return latest, nil
	}
	want := rowManifest(t, head)
	if found {
		want = withRow(latest.Manifest, t, head)
		same, err := sameManifest(want, latest.Manifest)
		if err != nil || same {
			return latest, err
		}
	}
	outcome := approveAtOnce
	if found && latest.State != StateApproved {
		outcome = keepAsDraft
	}
	number, err := s.saveVersion(ctx, tx, identity.Caller{UserID: t.ownerID, Meta: c.Meta}, t, want, outcome)
	if err != nil {
		return Version{}, err
	}
	return versionAt(ctx, tx, t.ID, number)
}

// textsOf reads the text of every page entry at the version the manifest pins, with the
// stored size of each, in one query.
func (s *Service) textsOf(ctx context.Context, q db.Querier, m Manifest) (map[string]document.Node, map[string]int, error) {
	want := make([]pages.TemplateText, 0, len(m.Entries))
	for _, e := range m.Entries {
		if e.PageID != nil {
			want = append(want, pages.TemplateText{PageID: *e.PageID, Number: e.PageVersion})
		}
	}
	docs, stored, err := s.pages.TemplateDocumentsAt(ctx, q, want)
	if err != nil {
		return nil, nil, err
	}
	texts, sizes := map[string]document.Node{}, map[string]int{}
	for _, e := range m.Entries {
		if e.PageID == nil {
			continue
		}
		key := pages.TemplateText{PageID: *e.PageID, Number: e.PageVersion}
		texts[e.Key], sizes[e.Key] = docs[key], stored[key]
	}
	return texts, sizes, nil
}

// versionOutcome is what a saved version becomes.
type versionOutcome int

const (
	// approveAtOnce: whoever saved it publishes directly, as managers do (TPL-511, TPL-1411).
	approveAtOnce versionOutcome = iota
	// keepAsDraft: it waits for its author to propose it, and the approved version stays in
	// use meanwhile (TPL-512, TPL-523).
	keepAsDraft
)

// outcomeFor is what a version saved by this person becomes.
func outcomeFor(ctx context.Context, q db.Querier, st standing) (versionOutcome, error) {
	direct, err := publishesDirectly(ctx, q, st)
	if err != nil || direct {
		return approveAtOnce, err
	}
	return keepAsDraft, nil
}

// saveVersion stores m as the next version of a template, unless it equals the newest. A
// version approved at once becomes the one the template offers, and the one it replaces stays
// usable for a form already open; a draft leaves the approved version in use. Either way a
// proposal that waited is over, because what it proposed changed (TPL-522).
func (s *Service) saveVersion(ctx context.Context, tx pgx.Tx, c identity.Caller, t Template, m Manifest, outcome versionOutcome) (int, error) {
	texts, err := s.checkVersion(ctx, tx, t, m)
	if err != nil {
		return 0, err
	}
	latest, found, err := latestVersion(ctx, tx, t.ID)
	if err != nil {
		return 0, err
	}
	guided := m.usesGuidedParts(texts)
	if found {
		same, err := sameManifest(m, latest.Manifest)
		if err != nil {
			return 0, err
		}
		if same {
			_, err := tx.Exec(ctx, "UPDATE templates SET guided = $2 WHERE id = $1", t.ID, guided)
			return latest.Number, err
		}
	}
	if outcome == keepAsDraft {
		// A node of the previous release would offer a draft to everyone (TPL-1103).
		if err := s.holdForOlderRelease(ctx); err != nil {
			return 0, err
		}
	}
	raw, err := m.marshal()
	if err != nil {
		return 0, err
	}
	number := latest.Number + 1
	if _, err := tx.Exec(ctx, `UPDATE template_versions SET state = 'superseded' WHERE template_id = $1 AND state = 'waiting'`, t.ID); err != nil {
		return 0, err
	}
	state := StateDraft
	if outcome == approveAtOnce {
		state = StateApproved
		if err := supersedeApproved(ctx, tx, t.ID); err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO template_versions (template_id, number, manifest, author_id, state)
		VALUES ($1, $2, $3, $4, $5)`, t.ID, number, raw, c.UserID, state); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE templates SET guided = $3, state = $4,
			approved_version = CASE WHEN $4 = 'approved' THEN $2 ELSE approved_version END WHERE id = $1`,
		t.ID, number, guided, state); err != nil {
		return 0, err
	}
	return number, auditTemplateDetails(ctx, tx, c, "template.version_saved", t,
		map[string]any{"version": number, "entries": len(m.Entries), "state": state})
}

// checkVersion runs every check a version of a template must pass: its shape, its texts, its
// questions and its conditions. Approving runs them again, as the world changes under a
// template (TPL-525). It gives the texts it read.
func (s *Service) checkVersion(ctx context.Context, tx pgx.Tx, t Template, m Manifest) (map[string]document.Node, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	texts, sizes, err := s.textsOf(ctx, tx, m)
	if err != nil {
		return nil, err
	}
	if err := m.checkTexts(texts, sizes); err != nil {
		return nil, err
	}
	if err := s.checkSpecs(ctx, tx, m, texts, t.today); err != nil {
		return nil, err
	}
	return texts, s.checkConditions(ctx, tx, t, m, texts)
}

// supersedeApproved marks the approved version of a template as replaced, from now, so that
// a form opened on it keeps working for a day (TPL-523, TPL-603).
func supersedeApproved(ctx context.Context, tx pgx.Tx, templateID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE template_versions SET state = 'superseded', superseded_at = now()
		WHERE template_id = $1 AND state = 'approved'`, templateID)
	return err
}

// Versions lists the versions of a template, newest first, with every proposal and decision,
// for those who look after it (ORG-060, TPL-602).
func (s *Service) Versions(ctx context.Context, c identity.Caller, id uuid.UUID) (History, error) {
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return History{}, err
	}
	st, err := s.requireTo(ctx, s.pool, c, t, standing.mayLook)
	if err != nil {
		return History{}, err
	}
	rows, err := s.pool.Query(ctx, "SELECT "+versionColumns+" FROM template_versions WHERE template_id = $1 ORDER BY number DESC", id)
	if err != nil {
		return History{}, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Version, error) { return scanVersion(r) })
	if err != nil {
		return History{}, err
	}
	decisions, err := reviewsOf(ctx, s.pool, id)
	if err != nil {
		return History{}, err
	}
	people := make([]uuid.UUID, 0, len(list)+len(decisions))
	for _, v := range list {
		people = append(people, v.AuthorID)
	}
	for _, d := range decisions {
		people = append(people, d.PersonID)
	}
	names, err := identity.DisplayNames(ctx, s.pool, people)
	if err != nil {
		return History{}, err
	}
	out := History{Versions: make([]VersionRow, 0, len(list)), Approved: t.TemplateVersion, State: t.State}
	for _, v := range list {
		row := VersionRow{Version: v, AuthorName: names[v.AuthorID], Pages: len(v.Manifest.Entries), Decisions: []Decision{}}
		for _, d := range decisions {
			if d.Version == v.Number {
				d.PersonName = names[d.PersonID]
				row.Decisions = append(row.Decisions, d)
			}
		}
		out.Versions = append(out.Versions, row)
	}
	out.Can, err = s.historyCan(ctx, st, list)
	return out, err
}

// History is the versions of a template and what the person reading it may do with them.
type History struct {
	Versions []VersionRow `json:"versions"`
	// Approved is the version offered now, 0 for none; State that of the newest.
	Approved int        `json:"approved"`
	State    string     `json:"state"`
	Can      HistoryCan `json:"can"`
}

// HistoryCan says which actions the Versions section offers, so a control that would be
// refused is absent (design standard, States). The server decides again on each.
type HistoryCan struct {
	Propose  bool `json:"propose"`
	Decide   bool `json:"decide"`
	Withdraw bool `json:"withdraw"`
	// NobodyElse says a waiting version has nobody who may decide on it but its author, so it
	// waits until someone else may (TPL-520, TPL-521).
	NobodyElse bool `json:"nobody_else"`
}

func (s *Service) historyCan(ctx context.Context, st standing, list []Version) (HistoryCan, error) {
	var can HistoryCan
	if len(list) == 0 {
		return can, nil
	}
	newest := list[0]
	can.Propose = st.mayEdit() && slices.Contains([]string{StateDraft, StateSentBack, StateWithdrawn}, newest.State)
	can.Withdraw = st.manages() && st.t.TemplateVersion > 0
	if newest.State != StateWaiting {
		return can, nil
	}
	selfOK, err := mayDecideOwn(ctx, s.pool, st, newest)
	if err != nil {
		return can, err
	}
	can.Decide = st.manages() && selfOK
	others, err := s.approversOf(ctx, s.pool, st.t, st.rights, newest.AuthorID)
	can.NobodyElse = len(others) == 0
	return can, err
}

// VersionRow is a version as the history lists it (ORG-060), with its decisions (TPL-602).
type VersionRow struct {
	Version
	AuthorName string `json:"author_name"`
	// Pages is how many pages the version makes.
	Pages     int        `json:"pages"`
	Decisions []Decision `json:"decisions"`
}

// EntryChange is what one version changed in the text of one page of a template, in the
// comparison pages use (ORG-060).
type EntryChange struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Change is added, removed, changed or same.
	Change string               `json:"change"`
	Blocks []document.DiffBlock `json:"blocks"`
}

// Changes is a version of a template compared with the one before it.
type Changes struct {
	Number   int           `json:"number"`
	Previous int           `json:"previous"`
	Entries  []EntryChange `json:"entries"`
	// Settings says that something besides the texts changed: the pages, the questions,
	// the steps or where the pages go.
	Settings bool `json:"settings"`
}

// VersionChanges compares a version of a template with an earlier one, page by page, for
// those who look after it (ORG-060): with against, or with the one before it when against is
// 0. An approver compares a waiting version with the approved one (TPL-522, TPL-1030). The
// first version is compared with an empty template.
func (s *Service) VersionChanges(ctx context.Context, c identity.Caller, id uuid.UUID, number, against int) (Changes, error) {
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return Changes{}, err
	}
	if _, err := s.requireTo(ctx, s.pool, c, t, standing.mayLook); err != nil {
		return Changes{}, err
	}
	after, err := versionAt(ctx, s.pool, id, number)
	if err != nil {
		return Changes{}, err
	}
	if against <= 0 || against >= number {
		against = number - 1
	}
	before := Version{Manifest: Manifest{Entries: []Entry{}}}
	if against > 0 {
		if before, err = versionAt(ctx, s.pool, id, against); err != nil {
			return Changes{}, err
		}
	}
	textsAfter, _, err := s.textsOf(ctx, s.pool, after.Manifest)
	if err != nil {
		return Changes{}, err
	}
	textsBefore, _, err := s.textsOf(ctx, s.pool, before.Manifest)
	if err != nil {
		return Changes{}, err
	}
	out := Changes{Number: number, Previous: before.Number, Entries: []EntryChange{}}
	for _, e := range after.Manifest.ordered() {
		old, _, existed := before.Manifest.entry(e.Key)
		change := EntryChange{Key: e.Key, Name: e.Name, Change: "same"}
		doc, oldDoc := texts(textsAfter, e.Key), texts(textsBefore, e.Key)
		change.Blocks = document.Compare(oldDoc, doc)
		switch {
		case !existed:
			change.Change = "added"
		case old.PageID != nil && e.PageID != nil && old.PageVersion != e.PageVersion:
			change.Change = "changed"
		}
		out.Entries = append(out.Entries, change)
	}
	for _, e := range before.Manifest.Entries {
		if _, _, kept := after.Manifest.entry(e.Key); !kept {
			out.Entries = append(out.Entries, EntryChange{Key: e.Key, Name: e.Name, Change: "removed",
				Blocks: document.Compare(texts(textsBefore, e.Key), document.Empty())})
		}
	}
	out.Settings, err = settingsDiffer(before.Manifest, after.Manifest)
	return out, err
}

// texts gives the text of an entry, or an empty one for a folder or a missing entry.
func texts(all map[string]document.Node, key string) document.Node {
	if doc, ok := all[key]; ok {
		return doc
	}
	return document.Empty()
}

// settingsDiffer says whether two manifests differ in anything but the versions of texts.
func settingsDiffer(a, b Manifest) (bool, error) {
	strip := func(m Manifest) Manifest {
		m.Entries = slices.Clone(m.Entries)
		for i := range m.Entries {
			m.Entries[i].PageVersion = 0
		}
		m.normalise()
		return m
	}
	same, err := sameManifest(strip(a), strip(b))
	return !same, err
}
