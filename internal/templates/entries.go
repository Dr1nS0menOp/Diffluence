// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
)

// EntryInput is what the editor of a template says about one of its pages.
type EntryInput struct {
	Name         string          `json:"name"`
	Kind         string          `json:"kind"`
	Parent       string          `json:"parent"`
	TitlePattern string          `json:"title_pattern"`
	Labels       []string        `json:"labels"`
	Restrictions Restrictions    `json:"restrictions"`
	WorkflowID   *uuid.UUID      `json:"workflow_id"`
	Document     json.RawMessage `json:"document"`
}

// EntryView is an entry as its editor sees it, with its current text when asked for one.
type EntryView struct {
	Entry
	Document json.RawMessage `json:"document,omitempty"`
}

// Entries is the structure of a template as it stands: its newest version.
type Entries struct {
	Version       int        `json:"version"`
	Publish       bool       `json:"publish"`
	DefaultParent *uuid.UUID `json:"default_parent"`
	Entries       []Entry    `json:"entries"`
}

// editTemplate runs change on the newest manifest of a template that the caller manages,
// and saves what it returns as the next version, all in one transaction.
func (s *Service) editTemplate(ctx context.Context, c identity.Caller, id uuid.UUID,
	change func(tx pgx.Tx, t Template, m Manifest) (Manifest, error)) (Manifest, error) {
	var out Manifest
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		t, err := lockTemplate(ctx, tx, c.UserID, id)
		if err != nil {
			return err
		}
		if err := s.requireManager(ctx, tx, c, t.SpaceID); err != nil {
			return err
		}
		current, err := s.currentVersion(ctx, tx, c, t)
		if err != nil {
			return err
		}
		next, err := change(tx, t, current.Manifest)
		if err != nil {
			return err
		}
		if _, err := s.saveVersion(ctx, tx, c, t, next); err != nil {
			return err
		}
		out = next
		return syncRow(ctx, tx, t.ID, next)
	})
	return out, err
}

// syncRow keeps the row of a template in step with its first entry, so the previous
// release and the gallery read what the newest version says.
func syncRow(ctx context.Context, tx pgx.Tx, id uuid.UUID, m Manifest) error {
	root := m.Entries[0]
	_, err := tx.Exec(ctx, `UPDATE templates SET title_pattern = $2, labels = $3, default_parent = $4, updated_at = now() WHERE id = $1`,
		id, root.TitlePattern, root.Labels, m.DefaultParent)
	return err
}

// Entries lists the pages a template makes, for its managers.
func (s *Service) Entries(ctx context.Context, c identity.Caller, id uuid.UUID) (Entries, error) {
	var out Entries
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		t, err := lockTemplate(ctx, tx, c.UserID, id)
		if err != nil {
			return err
		}
		if err := s.requireManager(ctx, tx, c, t.SpaceID); err != nil {
			return err
		}
		v, err := s.currentVersion(ctx, tx, c, t)
		if err != nil {
			return err
		}
		out = Entries{Version: v.Number, Publish: v.Manifest.Publish, DefaultParent: v.Manifest.DefaultParent, Entries: v.Manifest.ordered()}
		return nil
	})
	return out, err
}

// Entry gives one entry of a template with its current text, for its managers.
func (s *Service) Entry(ctx context.Context, c identity.Caller, id uuid.UUID, key string) (EntryView, error) {
	list, err := s.Entries(ctx, c, id)
	if err != nil {
		return EntryView{}, err
	}
	i := slices.IndexFunc(list.Entries, func(e Entry) bool { return e.Key == key })
	if i < 0 {
		return EntryView{}, apperr.NotFound()
	}
	view := EntryView{Entry: list.Entries[i]}
	if view.PageID == nil {
		return view, nil
	}
	doc, err := s.pages.TemplateDocument(ctx, s.pool, *view.PageID)
	if err != nil {
		return view, err
	}
	view.Document, err = document.Marshal(doc)
	return view, err
}

// applyInput puts what the editor said into an entry.
func applyInput(e *Entry, in EntryInput) {
	e.Name, e.TitlePattern, e.Labels = in.Name, in.TitlePattern, slices.Clone(in.Labels)
	e.Restrictions, e.WorkflowID = in.Restrictions, in.WorkflowID
	if e.Key != RootKey {
		e.Parent = in.Parent
	}
}

func (in EntryInput) document() (document.Node, error) {
	if len(in.Document) == 0 {
		return document.Empty(), nil
	}
	doc, err := document.Normalize(in.Document)
	if err != nil {
		return doc, err
	}
	return doc, document.CheckFields(doc)
}

// AddEntry adds a page or a folder to a template (TPL-100). A page gets its own hidden
// page, so it has the editor, versions and files of any template.
func (s *Service) AddEntry(ctx context.Context, c identity.Caller, id uuid.UUID, in EntryInput) (Entry, error) {
	if in.Kind == "" {
		in.Kind = EntryPage
	}
	doc, err := in.document()
	if err != nil {
		return Entry{}, err
	}
	added := Entry{Key: ids.New().String(), Kind: in.Kind}
	applyInput(&added, in)
	_, err = s.editTemplate(ctx, c, id, func(tx pgx.Tx, t Template, m Manifest) (Manifest, error) {
		if len(m.Entries) >= maxEntries {
			return m, apperr.Invalid(fmt.Sprintf("A template can make at most %d pages.", maxEntries))
		}
		if added.Kind == EntryPage {
			home, err := s.homeOf(ctx, tx, t.SpaceID)
			if err != nil {
				return m, err
			}
			p, err := s.pages.NewTemplatePage(ctx, tx, c, home, added.Name, doc)
			if err != nil {
				return m, err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO template_entry_pages (page_id, template_id, entry_key) VALUES ($1, $2, $3)",
				p.ID, t.ID, added.Key); err != nil {
				return m, err
			}
			added.PageID, added.PageVersion = &p.ID, p.CurrentVersion
		}
		m.Entries = append(m.Entries, added)
		return m, nil
	})
	return added, err
}

// UpdateEntry changes an entry, and its text when the input holds one.
func (s *Service) UpdateEntry(ctx context.Context, c identity.Caller, id uuid.UUID, key string, in EntryInput) (Entry, error) {
	var doc *document.Node
	if len(in.Document) > 0 {
		d, err := in.document()
		if err != nil {
			return Entry{}, err
		}
		doc = &d
	}
	var changed Entry
	_, err := s.editTemplate(ctx, c, id, func(tx pgx.Tx, _ Template, m Manifest) (Manifest, error) {
		_, i, found := m.entry(key)
		if !found {
			return m, apperr.NotFound()
		}
		e := &m.Entries[i]
		applyInput(e, in)
		if doc != nil {
			if e.PageID == nil {
				return m, apperr.Invalid("A folder of a template has no text.")
			}
			if err := s.pages.NewTemplateVersion(ctx, tx, c, *e.PageID, e.Name, *doc); err != nil {
				return m, err
			}
			heads, err := s.pages.TemplateHeads(ctx, tx, []uuid.UUID{*e.PageID})
			if err != nil {
				return m, err
			}
			e.PageVersion = heads[*e.PageID]
		}
		changed = *e
		return m, nil
	})
	return changed, err
}

// RemoveEntry takes an entry out of a template. Its hidden page stays, because the older
// versions still point to its text (TPL-601). An entry with pages below it, or one that
// another entry links to, stays until those change.
func (s *Service) RemoveEntry(ctx context.Context, c identity.Caller, id uuid.UUID, key string) error {
	_, err := s.editTemplate(ctx, c, id, func(tx pgx.Tx, _ Template, m Manifest) (Manifest, error) {
		e, i, found := m.entry(key)
		if !found {
			return m, apperr.NotFound()
		}
		if key == RootKey {
			return m, apperr.Conflict("first_page", "The first page of a template cannot be removed.")
		}
		if m.hasChildren(key) {
			return m, apperr.Conflict("has_children", "Remove or move the pages below "+entryLabel(e)+" first.")
		}
		texts, _, err := s.textsOf(ctx, tx, m)
		if err != nil {
			return m, err
		}
		for _, other := range m.Entries {
			if other.Key != key && slices.Contains(document.LinkedEntries(texts[other.Key]), key) {
				return m, apperr.Conflict("linked", "The text of "+entryLabel(other)+" links to "+entryLabel(e)+". Remove that link first.")
			}
		}
		m.Entries = slices.Delete(m.Entries, i, i+1)
		return m, nil
	})
	return err
}

// SetPublishing says whether the pages of a set are published as they are made, or start
// as drafts of their creator (TPL-150, TPL-151).
func (s *Service) SetPublishing(ctx context.Context, c identity.Caller, id uuid.UUID, publish bool) error {
	_, err := s.editTemplate(ctx, c, id, func(_ pgx.Tx, _ Template, m Manifest) (Manifest, error) {
		m.Publish = publish
		return m, nil
	})
	return err
}
