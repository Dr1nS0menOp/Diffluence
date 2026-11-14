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
	Manifest  Manifest  `json:"-"`
}

func scanVersion(r pgx.Row) (Version, error) {
	var v Version
	var raw []byte
	if err := r.Scan(&v.Number, &raw, &v.AuthorID, &v.State, &v.CreatedAt); err != nil {
		return v, err
	}
	err := json.Unmarshal(raw, &v.Manifest)
	v.Manifest.normalise()
	return v, err
}

const versionColumns = "number, manifest, author_id, state, created_at"

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

// currentVersion is the version a template offers. A plain template that the previous
// release saved, which writes no version, gets its next version from its row and text
// here, on first read (TPL-1103). tx holds the lock of lockTemplate.
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
	number, err := s.saveVersion(ctx, tx, identity.Caller{UserID: t.ownerID, Meta: c.Meta}, t, want)
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

// saveVersion stores m as the next version of a template, unless it equals the newest,
// and makes it the version the template offers. Managers publish their own templates
// without approval (TPL-1411), so in this wave every version is approved at once.
func (s *Service) saveVersion(ctx context.Context, tx pgx.Tx, c identity.Caller, t Template, m Manifest) (int, error) {
	if err := m.validate(); err != nil {
		return 0, err
	}
	texts, sizes, err := s.textsOf(ctx, tx, m)
	if err != nil {
		return 0, err
	}
	if err := m.checkTexts(texts, sizes); err != nil {
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
	raw, err := m.marshal()
	if err != nil {
		return 0, err
	}
	number := latest.Number + 1
	if _, err := tx.Exec(ctx, `INSERT INTO template_versions (template_id, number, manifest, author_id, state)
		VALUES ($1, $2, $3, $4, 'approved')`, t.ID, number, raw, c.UserID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE templates SET approved_version = $2, guided = $3, state = 'approved' WHERE id = $1`,
		t.ID, number, guided); err != nil {
		return 0, err
	}
	return number, auditTemplateDetails(ctx, tx, c, "template.version_saved", t, map[string]any{"version": number, "entries": len(m.Entries)})
}

// Versions lists the versions of a template, newest first, for its managers.
func (s *Service) Versions(ctx context.Context, c identity.Caller, id uuid.UUID) ([]Version, error) {
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireManager(ctx, s.pool, c, t.SpaceID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT "+versionColumns+" FROM template_versions WHERE template_id = $1 ORDER BY number DESC", id)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Version, error) { return scanVersion(r) })
	if out == nil {
		out = []Version{}
	}
	return out, err
}
