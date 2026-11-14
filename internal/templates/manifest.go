// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/labels"
	"github.com/diffluence/diffluence/internal/permissions"
)

// The bounds of a template (spec 074, TPL-102, TPL-825, TPL-906).
const (
	maxEntries       = 100
	maxLevels        = 6
	maxSetFields     = 40
	maxManifestBytes = 64 << 10
	maxTemplateText  = 5 << 20

	manifestFormat = 1
	// RootKey is the key of the first entry of every template: the page that has always
	// been the template's text, templates.page_id.
	RootKey = "root"

	EntryPage   = "page"
	EntryFolder = "folder"
)

// Restrictions say who may view and who may edit the page made from an entry (TPL-130).
// They come only from the template, never from an answer (TPL-131).
type Restrictions struct {
	View []permissions.Subject `json:"view"`
	Edit []permissions.Subject `json:"edit"`
}

func (r Restrictions) any() bool { return len(r.View) > 0 || len(r.Edit) > 0 }

// Entry is one page a template makes (TPL-100). Key is made by Diffluence and survives
// renaming and moving, so links keep pointing at it.
type Entry struct {
	Key          string       `json:"key"`
	Name         string       `json:"name"`
	Kind         string       `json:"kind"`
	Parent       string       `json:"parent"`
	PageID       *uuid.UUID   `json:"page_id"`
	PageVersion  int          `json:"page_version"`
	TitlePattern string       `json:"title_pattern"`
	Labels       []string     `json:"labels"`
	Restrictions Restrictions `json:"restrictions"`
	WorkflowID   *uuid.UUID   `json:"workflow_id"`
}

// Manifest is everything one version of a template makes (TPL-600). The text of each
// entry stays in the page versions of its hidden page; the manifest pins their numbers.
// The name, description, category and the hidden and promoted flags are not in it: they
// change how a template is found, not what it makes.
type Manifest struct {
	Format        int        `json:"format"`
	Entries       []Entry    `json:"entries"`
	Steps         []Step     `json:"steps"`
	Publish       bool       `json:"publish"`
	DefaultParent *uuid.UUID `json:"default_parent"`
}

func (m Manifest) entry(key string) (Entry, int, bool) {
	for i, e := range m.Entries {
		if e.Key == key {
			return e, i, true
		}
	}
	return Entry{}, -1, false
}

func (m Manifest) hasChildren(key string) bool {
	return slices.ContainsFunc(m.Entries, func(e Entry) bool { return e.Parent == key })
}

// ordered lists the entries from the root down, each right after its parent and its
// siblings in the order the template gives, as the tree shows them. A page is so always
// made after its parent.
func (m Manifest) ordered() []Entry {
	out := make([]Entry, 0, len(m.Entries))
	var below func(key string)
	below = func(key string) {
		for _, e := range m.Entries {
			if e.Key != RootKey && e.Parent == key {
				out = append(out, e)
				below(e.Key)
			}
		}
	}
	out = append(out, m.Entries[0])
	below(RootKey)
	return out
}

// normalise gives every list a value, so that two equal manifests marshal alike.
func (m *Manifest) normalise() {
	m.Format = manifestFormat
	if m.Steps == nil {
		m.Steps = []Step{}
	}
	for i := range m.Entries {
		e := &m.Entries[i]
		if e.Labels == nil {
			e.Labels = []string{}
		}
		if e.Restrictions.View == nil {
			e.Restrictions.View = []permissions.Subject{}
		}
		if e.Restrictions.Edit == nil {
			e.Restrictions.Edit = []permissions.Subject{}
		}
	}
}

// validate checks the shape of a manifest: one root first, unique keys, every parent
// present, no loop, and the bounds of TPL-102.
func (m *Manifest) validate() error {
	m.normalise()
	if len(m.Entries) == 0 || m.Entries[0].Key != RootKey || m.Entries[0].Parent != "" || m.Entries[0].Kind != EntryPage {
		return apperr.Invalid("A template starts with its own page.")
	}
	if len(m.Entries) > maxEntries {
		return apperr.Invalid(fmt.Sprintf("A template can make at most %d pages.", maxEntries))
	}
	keys := map[string]string{}
	for i := range m.Entries {
		e := &m.Entries[i]
		if err := e.clean(i == 0); err != nil {
			return err
		}
		if _, twice := keys[e.Key]; twice {
			return apperr.Invalid("Two pages of the template have the same key.")
		}
		keys[e.Key] = e.Parent
	}
	for _, e := range m.Entries {
		if _, err := levelOf(keys, e.Key); err != nil {
			return err
		}
	}
	return nil
}

// levelOf counts how far an entry sits below the root, refusing a loop and more than
// maxLevels levels.
func levelOf(parents map[string]string, key string) (int, error) {
	level := 0
	for key != RootKey {
		parent, ok := parents[key]
		if !ok || parent == "" {
			return 0, apperr.Invalid("A page of the template sits below a page that is not in it.")
		}
		level++
		if level > maxLevels {
			return 0, apperr.Invalid(fmt.Sprintf("A template can go at most %d levels below its first page.", maxLevels))
		}
		key = parent
	}
	return level, nil
}

func (e *Entry) clean(root bool) error {
	e.Name = strings.TrimSpace(e.Name)
	if !root && (e.Name == "" || utf8.RuneCountInString(e.Name) > maxNameRunes) {
		return apperr.Invalid("A page of a template needs a name of at most 100 characters.")
	}
	if utf8.RuneCountInString(e.Name) > maxNameRunes {
		return apperr.Invalid("A page of a template needs a name of at most 100 characters.")
	}
	if utf8.RuneCountInString(e.TitlePattern) > maxPatternRunes {
		return apperr.Invalid("A title pattern can have at most 255 characters.")
	}
	switch e.Kind {
	case EntryPage:
		if e.PageID == nil || e.PageVersion < 1 {
			return apperr.Invalid("A page of a template needs its text.")
		}
	case EntryFolder:
		if e.PageID != nil {
			return apperr.Invalid("A folder of a template has no text.")
		}
		if e.WorkflowID != nil {
			return apperr.Invalid("A folder does not follow a workflow.")
		}
	default:
		return apperr.Invalid("A page of a template is a page or a folder.")
	}
	for i, raw := range e.Labels {
		label, err := labels.Clean(raw)
		if err != nil {
			return err
		}
		e.Labels[i] = label
	}
	for _, sub := range slices.Concat(e.Restrictions.View, e.Restrictions.Edit) {
		if (sub.Type != permissions.SubjectUser && sub.Type != permissions.SubjectGroup) || sub.ID == uuid.Nil {
			return apperr.Invalid("Only people and groups can be listed on a restriction.")
		}
	}
	return nil
}

// marshal gives the stored form, refusing one over its bound.
func (m Manifest) marshal() ([]byte, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxManifestBytes {
		return nil, apperr.TooLarge("This template has grown past what one version may hold. Make it smaller.")
	}
	return raw, nil
}

func sameManifest(a, b Manifest) (bool, error) {
	ra, err := json.Marshal(a)
	if err != nil {
		return false, err
	}
	rb, err := json.Marshal(b)
	return bytes.Equal(ra, rb), err
}

// usesGuidedParts says whether a template needs the experimental part guided templates
// to be offered (TPL-050): more than one page, publishing at once, a restriction, a
// workflow, or a value of the template in its text.
func (m Manifest) usesGuidedParts(texts map[string]document.Node) bool {
	if len(m.Entries) > 1 || m.Publish {
		return true
	}
	for _, e := range m.Entries {
		if e.Restrictions.any() || e.WorkflowID != nil {
			return true
		}
	}
	for _, doc := range texts {
		if document.HasTemplateValues(doc) {
			return true
		}
	}
	return false
}

// checkTexts holds the texts of all entries to the bounds of a set (TPL-825) and refuses
// a link to an entry that is not there (TPL-840). sizes holds the stored size of each text.
func (m Manifest) checkTexts(texts map[string]document.Node, sizes map[string]int) error {
	total := 0
	for _, size := range sizes {
		total += size
	}
	if total > maxTemplateText {
		return apperr.TooLarge("The text of all pages of a template together must stay under 5 MiB.")
	}
	names := map[string]bool{}
	for _, e := range m.Entries {
		doc, ok := texts[e.Key]
		if !ok {
			continue
		}
		for _, f := range document.Fields(doc) {
			names[strings.ToLower(f.Name)] = true
		}
		for _, key := range document.LinkedEntries(doc) {
			if _, _, found := m.entry(key); !found {
				return apperr.Invalid(fmt.Sprintf("The text of %s links to a page that is not part of this template.", entryLabel(e)))
			}
		}
	}
	if len(names) > maxSetFields {
		return apperr.Invalid(fmt.Sprintf("A template can ask at most %d questions.", maxSetFields))
	}
	return nil
}

// entryLabel names an entry in a message.
func entryLabel(e Entry) string {
	if e.Name == "" {
		return "the first page"
	}
	return "“" + e.Name + "”"
}
