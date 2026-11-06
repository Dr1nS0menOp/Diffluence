// SPDX-License-Identifier: Apache-2.0

package document

import (
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// Node types of wave M (spec 023). A link that follows its page holds the id alone: a
// version never changes, so a stored title would outlive a rename, and would show to
// readers who may not see the page.
const (
	TypePageLink        = "pageLink"
	TypePageCard        = "pageCard"
	TypePlaceholderLink = "placeholderLink"
)

// LinkedPageWords stands in for the title of a page the reader of a file may not see.
const LinkedPageWords = "Linked page"

const maxFragmentBytes = 200

var (
	linkDisplays    = choices("tag")
	fragmentPattern = regexp.MustCompile(`^[\p{L}\p{N}-]+$`)
)

func keepPageID(in, out map[string]any, what string) error {
	s, _ := in["page_id"].(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return errors.New(what + " must point to a page")
	}
	out["page_id"] = id.String()
	return nil
}

func cleanPageLink(in, out map[string]any) error {
	if err := keepPageID(in, out, "a page link"); err != nil {
		return err
	}
	linkDisplays.keep(in, out, "display")
	if fragment, _ := in["fragment"].(string); len(fragment) <= maxFragmentBytes && fragmentPattern.MatchString(fragment) {
		out["fragment"] = fragment
	}
	return nil
}

func cleanPageCard(in, out map[string]any) error {
	return keepPageID(in, out, "a page card")
}

func cleanPlaceholderLink(in, out map[string]any) error {
	title, _ := stringAttr(in, "title", maxTitleBytes)
	if title = strings.TrimSpace(title); title == "" {
		return errors.New("a link to a page that does not exist yet needs a title")
	}
	out["title"] = title
	return nil
}

func linkedPageOf(n Node) (uuid.UUID, bool) {
	if n.Type != TypePageLink && n.Type != TypePageCard {
		return uuid.Nil, false
	}
	s, _ := n.Attrs["page_id"].(string)
	id, err := uuid.Parse(s)
	return id, err == nil
}

// Placeholders lists the pages a document links to that do not exist yet, each once.
func Placeholders(n Node) []string {
	seen := map[string]bool{}
	out := []string{}
	Walk(n, func(c Node) {
		title, _ := c.Attrs["title"].(string)
		if c.Type == TypePlaceholderLink && title != "" && !seen[title] {
			seen[title] = true
			out = append(out, title)
		}
	})
	return out
}

// Anchors lists the names of the anchors of a document, in order.
func Anchors(n Node) []string {
	out := []string{}
	Walk(n, func(c Node) {
		if name, _ := c.Attrs["name"].(string); c.Type == TypeAnchor && name != "" {
			out = append(out, name)
		}
	})
	return out
}

// PageTitle gives the title of a page for whoever the document is resolved for. No
// answer means they may not see it.
type PageTitle func(id uuid.UUID) (string, bool)

func resolvedLink(n Node, id uuid.UUID, title PageTitle) Node {
	words, ok := "", false
	if title != nil {
		words, ok = title(id)
	}
	if !ok {
		return Node{Type: TypeText, Text: LinkedPageWords}
	}
	href := "/pages/" + id.String()
	if fragment, _ := n.Attrs["fragment"].(string); fragment != "" {
		href += "#" + fragment
	}
	return Node{Type: TypeText, Text: words, Marks: []Mark{{Type: MarkLink, Attrs: map[string]any{"href": href}}}}
}

// ResolveLinks turns links that follow their page into words and ordinary links, for
// a document that leaves the instance. Every writer can then simply ignore them. What
// the title function does not answer becomes neutral words without a link (spec 023, D4).
func ResolveLinks(n Node, title PageTitle) Node {
	if len(n.Content) == 0 {
		return n
	}
	out := n
	out.Content = make([]Node, 0, len(n.Content))
	for _, c := range n.Content {
		id, linked := linkedPageOf(c)
		switch {
		case linked && c.Type == TypePageCard:
			out.Content = append(out.Content, Node{Type: TypeParagraph, Content: []Node{resolvedLink(c, id, title)}})
		case linked:
			out.Content = append(out.Content, resolvedLink(c, id, title))
		case c.Type == TypePlaceholderLink:
			out.Content = append(out.Content, Node{Type: TypeText, Text: wordsOf(c)})
		default:
			out.Content = append(out.Content, ResolveLinks(c, title))
		}
	}
	return out
}

// TableCaption returns the line that belongs to a table, and whether it goes below.
func TableCaption(n Node) (caption string, below bool) {
	caption, _ = n.Attrs["caption"].(string)
	below, _ = n.Attrs["caption_below"].(bool)
	return caption, below
}

// blocksIn returns what the cells below a nested table hold, in reading order.
func blocksIn(n Node) []Node {
	var out []Node
	for _, c := range n.Content {
		switch c.Type {
		case TypeTable, TypeTableRow, TypeTableCell, TypeTableHeader:
			out = append(out, blocksIn(c)...)
		default:
			out = append(out, c)
		}
	}
	return out
}

// liftNestedTables keeps the words of a table inside a table and drops the inner grid.
// The editor refuses one, but a Word file or an older draft can hold it, and those must
// still save (spec 023, D15).
func liftNestedTables(n *Node, inTable bool) {
	kept := make([]Node, 0, len(n.Content))
	for _, c := range n.Content {
		if inTable && (c.Type == TypeTable || c.Type == TypePageFacts) {
			for _, block := range blocksIn(c) {
				liftNestedTables(&block, true)
				kept = append(kept, block)
			}
			continue
		}
		liftNestedTables(&c, inTable || c.Type == TypeTable)
		kept = append(kept, c)
	}
	n.Content = kept
}
