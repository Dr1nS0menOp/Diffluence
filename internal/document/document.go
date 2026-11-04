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
	"math"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
)

// SchemaVersion is the current document schema. Old documents are upgraded on read.
const SchemaVersion = 1

// Limits (spec 001).
const (
	MaxBytes = 5 << 20
	MaxDepth = 100
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
	TypeTable          = "table"
	TypeTableRow       = "tableRow"
	TypeTableCell      = "tableCell"
	TypeTableHeader    = "tableHeader"
	TypeText           = "text"
)

// Mark types.
const (
	MarkBold      = "bold"
	MarkItalic    = "italic"
	MarkUnderline = "underline"
	MarkStrike    = "strike"
	MarkCode      = "code"
	MarkLink      = "link"
)

var (
	blockTypes  = []string{TypeParagraph, TypeHeading, TypeBulletList, TypeOrderedList, TypeTaskList, TypeBlockquote, TypeCodeBlock, TypeHorizontalRule, TypeImage, TypeTable}
	inlineTypes = []string{TypeText, TypeHardBreak}
)

// children says which node types may appear inside each node type.
var children = map[string][]string{
	TypeDoc:            blockTypes,
	TypeParagraph:      inlineTypes,
	TypeHeading:        inlineTypes,
	TypeBulletList:     {TypeListItem},
	TypeOrderedList:    {TypeListItem},
	TypeListItem:       blockTypes,
	TypeTaskList:       {TypeTaskItem},
	TypeTaskItem:       blockTypes,
	TypeBlockquote:     blockTypes,
	TypeCodeBlock:      {TypeText},
	TypeHorizontalRule: {},
	TypeHardBreak:      {},
	TypeImage:          {},
	TypeTable:          {TypeTableRow},
	TypeTableRow:       {TypeTableCell, TypeTableHeader},
	TypeTableCell:      blockTypes,
	TypeTableHeader:    blockTypes,
	TypeText:           {},
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

func intAttr(v any) (int, bool) {
	f, ok := v.(float64)
	if !ok {
		i, isInt := v.(int)
		return i, isInt
	}
	if f != math.Trunc(f) || math.Abs(f) > 1e9 {
		return 0, false
	}
	return int(f), true
}

func stringAttr(attrs map[string]any, key string, limit int) (string, bool) {
	s, ok := attrs[key].(string)
	if !ok || !utf8.ValidString(s) {
		return "", false
	}
	return truncate(s, limit), true
}

// cleanAttrs rebuilds the attributes from scratch, keeping only known ones.
func cleanAttrs(n *Node) error {
	in := n.Attrs
	out := map[string]any{}
	switch n.Type {
	case TypeHeading:
		level, ok := intAttr(in["level"])
		if !ok || level < 1 || level > 6 {
			return errors.New("a heading needs a level from 1 to 6")
		}
		out["level"] = level
	case TypeOrderedList:
		if start, ok := intAttr(in["start"]); ok && start > 0 && start != 1 {
			out["start"] = start
		}
	case TypeTaskItem:
		checked, _ := in["checked"].(bool)
		out["checked"] = checked
	case TypeCodeBlock:
		if lang, ok := in["language"].(string); ok && languagePattern.MatchString(lang) {
			out["language"] = lang
		}
	case TypeImage:
		s, _ := in["attachment_id"].(string)
		id, err := uuid.Parse(s)
		if err != nil {
			return errors.New("images must point to an attachment; external images are not allowed")
		}
		out["attachment_id"] = id.String()
		if alt, ok := stringAttr(in, "alt", 1000); ok && alt != "" {
			out["alt"] = alt
		}
		if title, ok := stringAttr(in, "title", 1000); ok && title != "" {
			out["title"] = title
		}
	case TypeTableCell, TypeTableHeader:
		for _, key := range []string{"colspan", "rowspan"} {
			if v, ok := intAttr(in[key]); ok && v > 1 && v <= 1000 {
				out[key] = v
			}
		}
		if widths, ok := in["colwidth"].([]any); ok {
			var clean []any
			for _, w := range widths {
				if v, ok := intAttr(w); ok && v > 0 && v <= 10000 {
					clean = append(clean, v)
				} else {
					clean = nil
					break
				}
			}
			if len(clean) > 0 && len(clean) <= 1000 {
				out["colwidth"] = clean
			}
		}
	}
	n.Attrs = nil
	if len(out) > 0 {
		n.Attrs = out
	}
	return nil
}

func cleanMarks(in []Mark) ([]Mark, error) {
	var out []Mark
	for _, m := range in {
		switch m.Type {
		case MarkBold, MarkItalic, MarkUnderline, MarkStrike, MarkCode:
			out = append(out, Mark{Type: m.Type})
		case MarkLink:
			href, _ := m.Attrs["href"].(string)
			safe, err := SafeHref(href)
			if err != nil {
				return nil, err
			}
			attrs := map[string]any{"href": safe}
			if title, ok := stringAttr(m.Attrs, "title", 1000); ok && title != "" {
				attrs["title"] = title
			}
			out = append(out, Mark{Type: MarkLink, Attrs: attrs})
		default:
			return nil, fmt.Errorf("unknown mark %q", truncate(m.Type, 40))
		}
	}
	return out, nil
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

func inlineText(n Node, b *strings.Builder) {
	if n.Type == TypeText {
		b.WriteString(n.Text)
	}
	if n.Type == TypeHardBreak {
		b.WriteByte(' ')
	}
	for _, c := range n.Content {
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
		case TypeParagraph, TypeHeading, TypeCodeBlock:
			if t := BlockText(c); t != "" {
				b.WriteString(t)
				b.WriteByte('\n')
			}
		case TypeImage:
			if alt, ok := c.Attrs["alt"].(string); ok && alt != "" {
				b.WriteString(alt)
				b.WriteByte('\n')
			}
		}
	})
	return b.String()
}

// AttachmentIDs lists the attachments a document shows.
func AttachmentIDs(n Node) []uuid.UUID {
	var out []uuid.UUID
	Walk(n, func(c Node) {
		if c.Type == TypeImage {
			if s, ok := c.Attrs["attachment_id"].(string); ok {
				if id, err := uuid.Parse(s); err == nil {
					out = append(out, id)
				}
			}
		}
	})
	return out
}

// RemapAttachments points images at new attachments, after copying a page.
func RemapAttachments(n *Node, mapping map[uuid.UUID]uuid.UUID) {
	if n.Type == TypeImage {
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
