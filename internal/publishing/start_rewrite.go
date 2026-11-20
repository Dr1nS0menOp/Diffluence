// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"fmt"
	"maps"
	"strings"

	"github.com/google/uuid"

	"github.com/kenwerk/kenwerk/internal/document"
)

// The copy a start makes of a page leads its links to the other copies, so writers in the draft
// space stay in the draft space (PUB-1310); a release maps them back by the page pairs. Unlike a
// release, a start takes nothing away: it changes ids and nothing else, so a copy is its source
// with other ids, and verification can do the same change again from the map it keeps (PUB-920).

// startMap is what a start does with the ids a document names.
type startMap struct {
	// from is the space the start began from, which becomes the published space; to is the new
	// draft space.
	from, to spaceRef
	// pages maps a page of the old space to its copy, and files a file to its copy.
	pages map[uuid.UUID]uuid.UUID
	files map[uuid.UUID]uuid.UUID
}

// The keys of the map a start keeps for each page. mapStart marks the map of a start, whose
// direction is the reverse of a release's: from the page readers have to its copy.
const (
	mapStart     = "start:"
	mapStartFrom = "start:from_id"
	mapStartKey  = "start:from_key"
	mapStartTo   = "start:to_key"
	mapStartPage = "start_page:"
	mapStartFile = "start_file:"
)

// startDocument is the copy of a document a start writes, normalised as every version is, and the
// map that leads to it again.
func startDocument(doc document.Node, m startMap) (document.Node, map[string]string, error) {
	r := startRemapper{m: m, used: map[string]string{mapStart: "1", mapStartFrom: m.from.ID.String(), mapStartKey: m.from.Key,
		mapStartTo: m.to.Key}}
	out := doc
	out.Content = r.nodes(doc.Content)
	raw, err := document.Marshal(out)
	if err != nil {
		return out, nil, err
	}
	normal, err := document.Normalize(raw)
	return normal, r.used, err
}

// startMapFrom reads back the map a start kept for a page.
func startMapFrom(links map[string]string) (startMap, error) {
	from, err := uuid.Parse(links[mapStartFrom])
	if err != nil {
		return startMap{}, err
	}
	m := startMap{from: spaceRef{ID: from, Key: links[mapStartKey]}, to: spaceRef{Key: links[mapStartTo]},
		pages: map[uuid.UUID]uuid.UUID{}, files: map[uuid.UUID]uuid.UUID{}}
	for key, value := range links {
		for prefix, into := range map[string]map[uuid.UUID]uuid.UUID{mapStartPage: m.pages, mapStartFile: m.files} {
			rest, ok := strings.CutPrefix(key, prefix)
			if !ok {
				continue
			}
			a, errA := uuid.Parse(rest)
			b, errB := uuid.Parse(value)
			if errA != nil || errB != nil {
				return m, fmt.Errorf("publishing: the map of a start holds %q", key)
			}
			into[a] = b
		}
	}
	return m, nil
}

type startRemapper struct {
	m    startMap
	used map[string]string
}

func (r *startRemapper) page(id uuid.UUID) (uuid.UUID, bool) {
	to, ok := r.m.pages[id]
	if ok {
		r.used[mapStartPage+id.String()] = to.String()
	}
	return to, ok
}

func (r *startRemapper) nodes(list []document.Node) []document.Node {
	out := make([]document.Node, len(list))
	for i, n := range list {
		out[i] = r.node(n)
	}
	return out
}

func (r *startRemapper) node(n document.Node) document.Node {
	switch {
	case n.Type == document.TypeText:
		n.Marks = r.marks(n.Marks)
		return n
	case n.Type == document.TypeDynamic:
		return r.dynamic(n)
	case document.ShowsFile(n.Type):
		s, _ := n.Attrs[document.FileAttribute].(string)
		if id, err := uuid.Parse(s); err == nil {
			if to, ok := r.m.files[id]; ok {
				r.used[mapStartFile+id.String()] = to.String()
				n.Attrs = maps.Clone(n.Attrs)
				n.Attrs[document.FileAttribute] = to.String()
			}
		}
	}
	if key, ok := document.PageAttribute(n.Type); ok {
		s, _ := n.Attrs[key].(string)
		if id, err := uuid.Parse(s); err == nil {
			if to, ok := r.page(id); ok {
				n.Attrs = maps.Clone(n.Attrs)
				n.Attrs[key] = to.String()
			}
		}
	}
	n.Content = r.nodes(n.Content)
	return n
}

// marks leads a link to a page of the old space to its copy, and a link into the old space to the
// draft space.
func (r *startRemapper) marks(list []document.Mark) []document.Mark {
	out := make([]document.Mark, len(list))
	for i, m := range list {
		href, _ := m.Attrs["href"].(string)
		if m.Type == document.MarkLink && href != "" {
			if found := pageHref.FindStringSubmatch(href); found != nil {
				if id, err := uuid.Parse(found[1]); err == nil {
					if to, ok := r.page(id); ok {
						m.Attrs = maps.Clone(m.Attrs)
						m.Attrs["href"] = "/pages/" + to.String() + href[len(found[0]):]
					}
				}
			} else if rest, ok := r.fromSpaceHref(href); ok {
				m.Attrs = maps.Clone(m.Attrs)
				m.Attrs["href"] = "/spaces/" + r.m.to.Key + rest
			}
		}
		out[i] = m
	}
	return out
}

func (r *startRemapper) fromSpaceHref(href string) (string, bool) {
	rest, ok := strings.CutPrefix(href, "/spaces/")
	if !ok {
		return "", false
	}
	end := strings.IndexAny(rest, "/?#")
	if end < 0 {
		end = len(rest)
	}
	name := rest[:end]
	if strings.EqualFold(name, r.m.from.Key) || strings.EqualFold(name, r.m.from.ID.String()) {
		return rest[end:], true
	}
	return "", false
}

// dynamic leads the settings of a block that name pages or the old space to the copies and the
// draft space.
func (r *startRemapper) dynamic(n document.Node) document.Node {
	kind, _ := n.Attrs["kind"].(string)
	settings, _ := n.Attrs["settings"].(map[string]any)
	if len(settings) == 0 {
		return n
	}
	settings = maps.Clone(settings)
	single, lists := document.DynamicPageSettings(kind)
	for _, key := range single {
		if s, ok := settings[key].(string); ok {
			if id, err := uuid.Parse(s); err == nil {
				if to, ok := r.page(id); ok {
					settings[key] = to.String()
				}
			}
		}
	}
	for _, key := range lists {
		items, ok := settings[key].([]any)
		if !ok {
			continue
		}
		led := make([]any, len(items))
		for i, item := range items {
			led[i] = item
			if s, ok := item.(string); ok {
				if id, err := uuid.Parse(s); err == nil {
					if to, ok := r.page(id); ok {
						led[i] = to.String()
					}
				}
			}
		}
		settings[key] = led
	}
	for _, key := range document.DynamicSpaceSettings(kind) {
		items, ok := settings[key].([]any)
		if !ok {
			continue
		}
		named := make([]any, len(items))
		for i, item := range items {
			named[i] = item
			if s, ok := item.(string); ok && (strings.EqualFold(s, r.m.from.Key) || strings.EqualFold(s, r.m.from.ID.String())) {
				named[i] = r.m.to.Key
			}
		}
		settings[key] = named
	}
	n.Attrs = maps.Clone(n.Attrs)
	n.Attrs["settings"] = settings
	return n
}
