// SPDX-License-Identifier: Apache-2.0

// Package document owns the structured document format (ADR-004, spec 001):
// a JSON tree based on the ProseMirror model. Normalize is the security gate:
// only allow-listed nodes, marks and attributes get past it, so stored content
// can never carry scripts, event handlers or external resources.
package document

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
)

// SchemaVersion is the current document schema. Old documents are upgraded on read.
const SchemaVersion = 1

// Limits (spec 001, spec 008).
const (
	MaxBytes   = 5 << 20
	MaxDepth   = 100
	minColumns = 2
	maxColumns = 5
)

// Node is one element of a document.
type Node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []Node         `json:"content,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
	Text    string         `json:"text,omitempty"`
}

// Mark is inline formatting on a text node.
type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// Node types.
const (
	TypeDoc            = "doc"
	TypeParagraph      = "paragraph"
	TypeHeading        = "heading"
	TypeBulletList     = "bulletList"
	TypeOrderedList    = "orderedList"
	TypeListItem       = "listItem"
	TypeTaskList       = "taskList"
	TypeTaskItem       = "taskItem"
	TypeBlockquote     = "blockquote"
	TypeCodeBlock      = "codeBlock"
	TypeHorizontalRule = "horizontalRule"
	TypeHardBreak      = "hardBreak"
	TypeImage          = "image"
	TypeFile           = "file"
	TypePanel          = "panel"
	TypeExpand         = "expand"
	TypeColumns        = "columns"
	TypeColumn         = "column"
	TypeDecision       = "decision"
	TypeStatus         = "status"
	TypeDate           = "date"
	TypeTemplateField  = "templateField"
	TypeInstruction    = "instruction"
	TypeMention        = "mention"
	TypeTable          = "table"
	TypeTableRow       = "tableRow"
	TypeTableCell      = "tableCell"
	TypeTableHeader    = "tableHeader"
	TypeText           = "text"
)

// Mark types.
const (
	MarkBold        = "bold"
	MarkItalic      = "italic"
	MarkUnderline   = "underline"
	MarkStrike      = "strike"
	MarkCode        = "code"
	MarkLink        = "link"
	MarkSubscript   = "subscript"
	MarkSuperscript = "superscript"
	MarkTextColor   = "textColor"
	MarkHighlight   = "highlight"
)

var (
	blockTypes  = []string{TypeParagraph, TypeHeading, TypeBulletList, TypeOrderedList, TypeTaskList, TypeBlockquote, TypeCodeBlock, TypeHorizontalRule, TypeImage, TypeFile, TypeTable, TypePanel, TypeExpand, TypeColumns, TypeDecision, TypeDynamic, TypePageFacts, TypeExcerpt, TypeChart, TypeMedia, TypePageCard}
	inlineTypes = []string{TypeText, TypeHardBreak, TypeStatus, TypeDate, TypeMention, TypeTemplateField, TypeInstruction, TypeAnchor, TypePageLink, TypePlaceholderLink}
)

// children says which node types may appear inside each node type.
var children = map[string][]string{
	TypeDoc:             append(slices.Clone(blockTypes), TypeSheet),
	TypeParagraph:       inlineTypes,
	TypeHeading:         inlineTypes,
	TypeBulletList:      {TypeListItem},
	TypeOrderedList:     {TypeListItem},
	TypeListItem:        blockTypes,
	TypeTaskList:        {TypeTaskItem},
	TypeTaskItem:        blockTypes,
	TypeBlockquote:      blockTypes,
	TypeCodeBlock:       {TypeText},
	TypeHorizontalRule:  {},
	TypeHardBreak:       {},
	TypeImage:           {},
	TypeFile:            {},
	TypePanel:           blockTypes,
	TypeExpand:          blockTypes,
	TypeColumns:         {TypeColumn},
	TypeColumn:          blockTypes,
	TypeDecision:        inlineTypes,
	TypeStatus:          {},
	TypeDate:            {},
	TypeTemplateField:   {},
	TypeInstruction:     {},
	TypeMention:         {},
	TypeDynamic:         {},
	TypePageFacts:       {TypeTable},
	TypeExcerpt:         blockTypes,
	TypeAnchor:          {},
	TypeChart:           {},
	TypeSheet:           {},
	TypeMedia:           {},
	TypePageLink:        {},
	TypePageCard:        {},
	TypePlaceholderLink: {},
	TypeTable:           {TypeTableRow},
	TypeTableRow:        {TypeTableCell, TypeTableHeader},
	TypeTableCell:       blockTypes,
	TypeTableHeader:     blockTypes,
	TypeText:            {},
}

// NodeTypes lists every node a document can hold.
func NodeTypes() []string {
	out := make([]string, 0, len(children))
	for name := range children {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// MarkTypes lists every mark a text can have.
func MarkTypes() []string {
	return []string{MarkBold, MarkItalic, MarkUnderline, MarkStrike, MarkCode, MarkLink,
		MarkSubscript, MarkSuperscript, MarkTextColor, MarkHighlight}
}

var languagePattern = regexp.MustCompile(`^[a-zA-Z0-9_+#.-]{1,32}$`)

// Empty returns a document with one empty paragraph.
func Empty() Node {
	return Node{Type: TypeDoc, Content: []Node{{Type: TypeParagraph}}}
}

// Normalize parses and checks a document. Unknown nodes and marks are refused,
// unknown attributes are dropped, and the result is safe to store.
func Normalize(raw []byte) (Node, error) {
	if len(raw) > MaxBytes {
		return Node{}, apperr.TooLarge("The document is larger than the limit of 5 MiB.")
	}
	var n Node
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&n); err != nil {
		return Node{}, apperr.Invalid("The document is not valid JSON.")
	}
	if n.Type != TypeDoc {
		return Node{}, apperr.Invalid("A document must have type doc at the top.")
	}
	if err := clean(&n, 0); err != nil {
		return Node{}, apperr.Invalid(err.Error())
	}
	liftNestedTables(&n, false)
	if len(n.Content) == 0 {
		n.Content = []Node{{Type: TypeParagraph}}
	}
	return n, nil
}

func clean(n *Node, depth int) error {
	if depth > MaxDepth {
		return fmt.Errorf("the document is nested more than %d levels deep", MaxDepth)
	}
	allowed, known := children[n.Type]
	if !known {
		return fmt.Errorf("unknown node type %q", truncate(n.Type, 40))
	}
	if !utf8.ValidString(n.Text) {
		return errors.New("text is not valid UTF-8")
	}
	if n.Type != TypeText {
		n.Text = ""
		n.Marks = nil
	}
	if err := cleanAttrs(n); err != nil {
		return err
	}
	if n.Type == TypeColumns && (len(n.Content) < minColumns || len(n.Content) > maxColumns) {
		return fmt.Errorf("columns need %d to %d columns", minColumns, maxColumns)
	}
	if n.Type == TypeText {
		n.Content = nil
		marks, err := cleanMarks(n.Marks)
		if err != nil {
			return err
		}
		n.Marks = marks
		return nil
	}
	kept := n.Content[:0]
	for i := range n.Content {
		child := &n.Content[i]
		if !contains(allowed, child.Type) {
			return fmt.Errorf("%q cannot contain %q", n.Type, truncate(child.Type, 40))
		}
		if child.Type == TypeText && child.Text == "" {
			continue // ProseMirror refuses empty text nodes
		}
		if err := clean(child, depth+1); err != nil {
			return err
		}
		if n.Type == TypeCodeBlock {
			child.Marks = nil
		}
		kept = append(kept, *child)
	}
	n.Content = kept
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// SafeHref allows http, https and mailto links and relative paths. Everything
// else is refused, including javascript: and data: in any spelling.
func SafeHref(href string) (string, error) {
	href = strings.TrimSpace(href)
	bad := errors.New("links must use http, https or mailto, or be a relative path")
	if href == "" || len(href) > 2048 || strings.Contains(href, "\\") {
		return "", bad
	}
	for _, r := range href {
		if r < 0x20 || r == 0x7f {
			return "", bad
		}
	}
	if strings.HasPrefix(href, "//") {
		return "", bad // protocol-relative: leaves the site without saying so
	}
	u, err := url.Parse(href)
	if err != nil {
		return "", bad
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return "", bad
		}
	case "mailto":
	case "":
		if u.Host != "" || !(strings.HasPrefix(href, "/") || strings.HasPrefix(href, "#")) {
			return "", bad
		}
	default:
		return "", bad
	}
	return href, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// Marshal encodes a document for storage.
func Marshal(n Node) ([]byte, error) { return json.Marshal(n) }

// Parse decodes a stored document. Stored documents were normalized on the way in.
func Parse(raw []byte) (Node, error) {
	var n Node
	err := json.Unmarshal(raw, &n)
	return n, err
}

// Walk visits n and everything below it.
func Walk(n Node, visit func(Node)) {
	visit(n)
	for _, c := range n.Content {
		Walk(c, visit)
	}
}

// wordsOf returns what a node itself says, apart from its children.
func wordsOf(n Node) string {
	key := map[string]string{TypeFile: "filename", TypeStatus: "text", TypeDate: "value", TypeImage: "caption", TypeExpand: "title", TypePlaceholderLink: "title"}[n.Type]
	if n.Type == TypeText {
		return n.Text
	}
	if n.Type == TypeHardBreak {
		return " "
	}
	if n.Type == TypeMention {
		return mentionText(n)
	}
	if n.Type == TypeSheet {
		return sheetText(n)
	}
	s, _ := n.Attrs[key].(string)
	return s
}

func inlineText(n Node, b *strings.Builder) {
	b.WriteString(wordsOf(n))
	for i, c := range n.Content {
		// Blocks are separate things: without a space, two list items become one word.
		startsBlock := !contains(inlineTypes, c.Type) && (i > 0 || b.Len() > 0)
		if startsBlock && !strings.HasSuffix(b.String(), " ") {
			b.WriteByte(' ')
		}
		inlineText(c, b)
	}
}

// BlockText returns the plain text of one node.
func BlockText(n Node) string {
	var b strings.Builder
	inlineText(n, &b)
	return b.String()
}

// Text returns the plain text of a document, one line per text block, for search.
func Text(n Node) string {
	var b strings.Builder
	Walk(n, func(c Node) {
		switch c.Type {
		case TypeParagraph, TypeHeading, TypeCodeBlock, TypeFile, TypeDecision, TypeSheet:
			if t := BlockText(c); t != "" {
				b.WriteString(t)
				b.WriteByte('\n')
			}
		case TypeImage:
			alt, _ := c.Attrs["alt"].(string)
			if t := strings.TrimSpace(alt + " " + wordsOf(c)); t != "" {
				b.WriteString(t)
				b.WriteByte('\n')
			}
		case TypeExpand:
			if t := wordsOf(c); t != "" {
				b.WriteString(t)
				b.WriteByte('\n')
			}
		}
	})
	return b.String()
}

// pageLink finds the page in a link inside this instance. Anything after the id,
// such as a heading, still points at that page.
var pageLink = regexp.MustCompile(`^/pages/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})(?:[/?#]|$)`)

// LinkedPages lists the pages a document links to, each once.
func LinkedPages(n Node) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	out := []uuid.UUID{}
	add := func(id uuid.UUID) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	Walk(n, func(c Node) {
		if id, ok := linkedPageOf(c); ok {
			add(id)
		}
		for _, m := range c.Marks {
			href, _ := m.Attrs["href"].(string)
			found := pageLink.FindStringSubmatch(href)
			if m.Type != MarkLink || found == nil {
				continue
			}
			if id, err := uuid.Parse(found[1]); err == nil {
				add(id)
			}
		}
	})
	return out
}

func pointsToAttachment(n Node) bool { return n.Type == TypeImage || n.Type == TypeFile }

// AttachmentIDs lists the attachments a document shows.
func AttachmentIDs(n Node) []uuid.UUID {
	var out []uuid.UUID
	Walk(n, func(c Node) {
		if pointsToAttachment(c) {
			if s, ok := c.Attrs["attachment_id"].(string); ok {
				if id, err := uuid.Parse(s); err == nil {
					out = append(out, id)
				}
			}
		}
	})
	return out
}

// FilesShown lists every attachment a document points at, once each, whatever kind of
// block shows it.
func FilesShown(n Node) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	var out []uuid.UUID
	Walk(n, func(c Node) {
		s, _ := c.Attrs["attachment_id"].(string)
		if id, err := uuid.Parse(s); err == nil && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	})
	return out
}

// RemapAttachments points images and files at new attachments, after copying a page.
func RemapAttachments(n *Node, mapping map[uuid.UUID]uuid.UUID) {
	if pointsToAttachment(*n) {
		if s, ok := n.Attrs["attachment_id"].(string); ok {
			if id, err := uuid.Parse(s); err == nil {
				if to, found := mapping[id]; found {
					n.Attrs["attachment_id"] = to.String()
				}
			}
		}
	}
	for i := range n.Content {
		RemapAttachments(&n.Content[i], mapping)
	}
}
