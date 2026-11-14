// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"maps"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/pages"
)

// notAvailable is what a released document shows where the draft held something readers
// cannot have: an include of a page that is not released, or a database (PUB-510, PUB-523).
const notAvailable = "This part is not available here."

// kindCreateFromTemplate is the dynamic block that makes pages from a template of the space.
// A published space has no templates and nobody makes pages in it (PUB-233, PUB-220).
const kindCreateFromTemplate = "create_from_template"

// spaceRef is a space as a rewrite names it.
type spaceRef struct {
	ID  uuid.UUID
	Key string
}

// rewriteInput is what rewriting a released document needs to know (chapter 06).
type rewriteInput struct {
	draft, published spaceRef
	// twins maps a draft page to its twin, the page being released included.
	twins map[uuid.UUID]uuid.UUID
	// targets are the pages the document names that exist, with their space.
	targets map[uuid.UUID]pages.LinkTarget
	// files maps a file of the draft space to its copy on the twin.
	files map[uuid.UUID]uuid.UUID
	// foreign are files of other spaces, which stay as they are, as links to other spaces do.
	foreign map[uuid.UUID]bool
}

// rewritten is a released document and what the rewrite found.
type rewritten struct {
	doc document.Node
	// textLinks are the draft pages whose links became text, because they had no twin.
	textLinks []uuid.UUID
	// linkMap maps each draft id that was rewritten to the id it became, for verification
	// (PUB-732, decision D9).
	linkMap map[string]string
}

// pageOutcome is what becomes of a reference to a page.
type pageOutcome int

const (
	// keepPage: a page of another space, an ordinary link that stays (PUB-520).
	keepPage pageOutcome = iota
	// toTwin: a draft page with a twin, which the reference now names (PUB-500).
	toTwin
	// asText: a draft page without a twin, a database, or a page that is gone; the reference
	// becomes words, so no draft id reaches a reader (PUB-510).
	asText
)

type rewriter struct {
	in        rewriteInput
	textLinks map[uuid.UUID]bool
	linkMap   map[string]string
}

// rewrite makes a draft document fit for the published space: references to draft pages name
// their twins, files their copies, blocks that name the draft space name the published one,
// and what belongs to drafting is gone (PUB-500 to PUB-534). It never fails: what cannot be
// mapped becomes words.
func rewrite(doc document.Node, in rewriteInput) rewritten {
	r := &rewriter{in: in, textLinks: map[uuid.UUID]bool{}, linkMap: map[string]string{}}
	out := doc
	out.Content = r.nodes(doc.Content)
	if len(out.Content) == 0 {
		out.Content = []document.Node{{Type: document.TypeParagraph}}
	}
	res := rewritten{doc: out, linkMap: r.linkMap}
	for id := range r.textLinks {
		res.textLinks = append(res.textLinks, id)
	}
	return res
}

func (r *rewriter) resolve(id uuid.UUID) (uuid.UUID, pageOutcome) {
	target, known := r.in.targets[id]
	if known && target.SpaceID != r.in.draft.ID {
		return id, keepPage
	}
	if twin, ok := r.in.twins[id]; ok && known && target.Kind != pages.KindDatabase {
		r.linkMap[id.String()] = twin.String()
		return twin, toTwin
	}
	if known {
		r.textLinks[id] = true
	}
	return uuid.Nil, asText
}

// titleOf is the words a link to a page without a twin becomes: the title of the target at
// release time, or neutral words when it is gone (PUB-510).
func (r *rewriter) titleOf(id uuid.UUID) string {
	if t, ok := r.in.targets[id]; ok && strings.TrimSpace(t.Title) != "" {
		return t.Title
	}
	return document.LinkedPageWords
}

func (r *rewriter) nodes(list []document.Node) []document.Node {
	out := make([]document.Node, 0, len(list))
	for _, n := range list {
		out = append(out, r.node(n)...)
	}
	return out
}

func textNode(words string) []document.Node {
	if words == "" {
		return nil
	}
	return []document.Node{{Type: document.TypeText, Text: words}}
}

func unavailable() []document.Node {
	return []document.Node{{Type: document.TypeParagraph, Content: textNode(notAvailable)}}
}

func (r *rewriter) node(n document.Node) []document.Node {
	switch n.Type {
	case document.TypeInstruction:
		return nil // instruction text belongs to drafting (PUB-511)
	case document.TypeTemplateField:
		name, _ := n.Attrs["name"].(string)
		return textNode(name)
	case document.TypePlaceholderLink:
		// Following a placeholder makes a page, which nobody does in a published space (PUB-504).
		title, _ := n.Attrs["title"].(string)
		return textNode(title)
	case document.TypePageLink, document.TypePageCard:
		return r.pageReference(n)
	case document.TypeDynamic:
		return r.dynamic(n)
	case document.TypeText:
		n.Marks = r.marks(n.Marks)
		return []document.Node{n}
	}
	if document.ShowsFile(n.Type) {
		return r.file(n)
	}
	n.Content = r.nodes(n.Content)
	return []document.Node{n}
}

func (r *rewriter) pageReference(n document.Node) []document.Node {
	key, _ := document.PageAttribute(n.Type)
	s, _ := n.Attrs[key].(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return textNode(document.LinkedPageWords)
	}
	to, how := r.resolve(id)
	switch how {
	case keepPage:
		return []document.Node{n}
	case toTwin:
		n.Attrs = maps.Clone(n.Attrs)
		n.Attrs[key] = to.String()
		return []document.Node{n}
	}
	words := r.titleOf(id)
	if t, ok := r.in.targets[id]; ok && t.Kind == pages.KindDatabase {
		words = notAvailable
	}
	if n.Type == document.TypePageCard {
		return []document.Node{{Type: document.TypeParagraph, Content: textNode(words)}}
	}
	return textNode(words)
}

// pageHref finds the page of an address inside this instance, as document.LinkedPages does.
var pageHref = regexp.MustCompile(`^/pages/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`)

// marks rewrites the address of a link mark: to a twin, to the published space, or away.
func (r *rewriter) marks(list []document.Mark) []document.Mark {
	if len(list) == 0 {
		return list
	}
	out := make([]document.Mark, 0, len(list))
	for _, m := range list {
		href, _ := m.Attrs["href"].(string)
		if m.Type != document.MarkLink || href == "" {
			out = append(out, m)
			continue
		}
		if found := pageHref.FindStringSubmatch(href); found != nil {
			id, err := uuid.Parse(found[1])
			if err != nil {
				continue
			}
			to, how := r.resolve(id)
			switch how {
			case keepPage:
				out = append(out, m)
			case toTwin:
				m.Attrs = maps.Clone(m.Attrs)
				m.Attrs["href"] = "/pages/" + to.String() + href[len(found[0]):]
				out = append(out, m)
			}
			continue // a link to a draft page without a twin keeps its words, not its address
		}
		if rest, ok := r.draftSpaceHref(href); ok {
			m.Attrs = maps.Clone(m.Attrs)
			m.Attrs["href"] = "/spaces/" + r.in.published.Key + rest
		}
		out = append(out, m)
	}
	return out
}

// draftSpaceHref says whether an address leads into the draft space by its key or id, and
// what follows that.
func (r *rewriter) draftSpaceHref(href string) (string, bool) {
	rest, ok := strings.CutPrefix(href, "/spaces/")
	if !ok {
		return "", false
	}
	end := strings.IndexAny(rest, "/?#")
	if end < 0 {
		end = len(rest)
	}
	name := rest[:end]
	if strings.EqualFold(name, r.in.draft.Key) || strings.EqualFold(name, r.in.draft.ID.String()) {
		return rest[end:], true
	}
	return "", false
}

// file points a file block at the copy on the twin (PUB-320). A file of the draft space that
// was not copied, because it is gone, is not shown at all.
func (r *rewriter) file(n document.Node) []document.Node {
	s, _ := n.Attrs[document.FileAttribute].(string)
	id, err := uuid.Parse(s)
	if err == nil && r.in.foreign[id] {
		return []document.Node{n}
	}
	if copied, ok := r.in.files[id]; ok && err == nil {
		n.Attrs = maps.Clone(n.Attrs)
		n.Attrs[document.FileAttribute] = copied.String()
		return []document.Node{n}
	}
	if n.Type == document.TypeInlineImage {
		return nil
	}
	return unavailable()
}

// dynamic rewrites the settings of a block that name pages or spaces (PUB-502, PUB-503).
// Blocks that find pages when they are read are left alone: in the published space "this
// space" is the published space.
func (r *rewriter) dynamic(n document.Node) []document.Node {
	kind, _ := n.Attrs["kind"].(string)
	if kind == kindCreateFromTemplate {
		return unavailable()
	}
	settings, _ := n.Attrs["settings"].(map[string]any)
	if len(settings) == 0 {
		return []document.Node{n}
	}
	settings = maps.Clone(settings)
	single, lists := document.DynamicPageSettings(kind)
	for _, key := range single {
		s, present := settings[key].(string)
		if !present {
			continue
		}
		id, err := uuid.Parse(s)
		if err != nil {
			return unavailable()
		}
		to, how := r.resolve(id)
		switch how {
		case asText:
			return unavailable()
		case toTwin:
			settings[key] = to.String()
		}
	}
	for _, key := range lists {
		items, present := settings[key].([]any)
		if !present {
			continue
		}
		kept := []any{}
		for _, item := range items {
			s, _ := item.(string)
			id, err := uuid.Parse(s)
			if err != nil {
				continue
			}
			if to, how := r.resolve(id); how != asText {
				kept = append(kept, to.String())
			}
		}
		if len(kept) == 0 {
			return unavailable()
		}
		settings[key] = kept
	}
	for _, key := range document.DynamicSpaceSettings(kind) {
		items, present := settings[key].([]any)
		if !present {
			continue
		}
		named := make([]any, 0, len(items))
		for _, item := range items {
			s, _ := item.(string)
			if strings.EqualFold(s, r.in.draft.Key) || strings.EqualFold(s, r.in.draft.ID.String()) {
				s = r.in.published.Key
			}
			named = append(named, s)
		}
		settings[key] = named
	}
	n.Attrs = maps.Clone(n.Attrs)
	n.Attrs["settings"] = settings
	return []document.Node{n}
}
