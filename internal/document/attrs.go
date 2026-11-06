// SPDX-License-Identifier: Apache-2.0

package document

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Limits on attribute values.
const (
	maxFilenameBytes = 255 // matches the limit on attachment names
	maxIndent        = 6
	maxStatusBytes   = 40
	maxCaptionBytes  = 500
	maxTitleBytes    = 500
	maxCodeTitle     = 200
	maxTableStretch  = 100 // the whole width of the frame around the page

	maxFieldNameBytes   = 100
	maxFieldOptionBytes = 2000
)

// choice is a short list of allowed names. Colours, sizes and alignment are
// names and never free values, so a document cannot carry a style, and every
// colour has a light and a dark version that passes contrast.
type choice map[string]bool

func choices(names ...string) choice {
	c := choice{}
	for _, name := range names {
		c[name] = true
	}
	return c
}

var (
	textColors    = choices("grey", "red", "orange", "amber", "green", "teal", "blue", "indigo", "purple", "pink", "brown", "slate")
	markColors    = choices("grey", "yellow", "green", "blue", "purple", "red")
	panelKinds    = choices("info", "note", "success", "warning", "error")
	textAligns    = choices("center", "end")
	imageAligns   = choices("start", "center", "end")
	imageSizes    = choices("small", "medium", "large")
	columnLayouts = choices("wide-start", "wide-end")
	fieldKinds    = choices("line", "text", "choice")
	mentionKinds  = choices(MentionUser, MentionGroup)
)

// IsMarkColor reports whether a colour name is on the short list for marks.
func IsMarkColor(name string) bool { return markColors[name] }

func (c choice) names() []string {
	out := make([]string, 0, len(c))
	for name := range c {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// PanelKinds lists the kinds of panel.
func PanelKinds() []string { return panelKinds.names() }

// TextColors lists the colours a writer can give to text.
func TextColors() []string { return textColors.names() }

// MarkColors lists the colours of highlights, status marks and table cells.
func MarkColors() []string { return markColors.names() }

// keep copies a value from the list, and silently leaves out anything else.
func (c choice) keep(in, out map[string]any, key string) {
	if s, ok := in[key].(string); ok && c[s] {
		out[key] = s
	}
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

// keepText copies a string that is not empty.
func keepText(in, out map[string]any, key string, limit int) {
	if s, ok := stringAttr(in, key, limit); ok && s != "" {
		out[key] = s
	}
}

// keepTrue copies a switch that is on. Off is the default, so it is not stored.
func keepTrue(in, out map[string]any, key string) {
	if on, _ := in[key].(bool); on {
		out[key] = true
	}
}

func attachmentID(in map[string]any) (string, bool) {
	s, _ := in["attachment_id"].(string)
	id, err := uuid.Parse(s)
	if err != nil {
		return "", false
	}
	return id.String(), true
}

func cleanTextBlock(in, out map[string]any) error {
	textAligns.keep(in, out, "align")
	if indent, ok := intAttr(in["indent"]); ok && indent >= 1 && indent <= maxIndent {
		out["indent"] = indent
	}
	return nil
}

func cleanHeading(in, out map[string]any) error {
	level, ok := intAttr(in["level"])
	if !ok || level < 1 || level > 6 {
		return errors.New("a heading needs a level from 1 to 6")
	}
	out["level"] = level
	return cleanTextBlock(in, out)
}

func cleanOrderedList(in, out map[string]any) error {
	if start, ok := intAttr(in["start"]); ok && start > 0 && start != 1 {
		out["start"] = start
	}
	return nil
}

func cleanTaskItem(in, out map[string]any) error {
	checked, _ := in["checked"].(bool)
	out["checked"] = checked
	if id, ok := in["id"].(string); ok {
		if parsed, err := uuid.Parse(id); err == nil {
			out["id"] = parsed.String()
		}
	}
	return nil
}

// cleanMention keeps the id alone. Versions cannot change, so a name stored here could
// never be anonymised: reading fills it in (spec 010, D6).
func cleanMention(in, out map[string]any) error {
	id, _ := in["id"].(string)
	parsed, err := uuid.Parse(id)
	if err != nil {
		return errors.New("a mention must point to a person or a group")
	}
	out["id"] = parsed.String()
	mentionKinds.keep(in, out, "kind")
	if out["kind"] == nil {
		out["kind"] = MentionUser
	}
	return nil
}

func cleanCodeBlock(in, out map[string]any) error {
	if lang, ok := in["language"].(string); ok && languagePattern.MatchString(lang) {
		out["language"] = lang
	}
	keepText(in, out, "title", maxCodeTitle)
	keepTrue(in, out, "line_numbers")
	keepTrue(in, out, "wrap")
	return nil
}

func cleanImage(in, out map[string]any) error {
	id, ok := attachmentID(in)
	if !ok {
		return errors.New("images must point to an attachment; external images are not allowed")
	}
	out["attachment_id"] = id
	keepText(in, out, "alt", 1000)
	keepText(in, out, "title", 1000)
	keepText(in, out, "caption", maxCaptionBytes)
	imageSizes.keep(in, out, "size")
	imageAligns.keep(in, out, "align")
	return nil
}

func cleanFile(in, out map[string]any) error {
	id, ok := attachmentID(in)
	if !ok {
		return errors.New("files must point to an attachment")
	}
	name, ok := stringAttr(in, "filename", maxFilenameBytes)
	if !ok || name == "" {
		return errors.New("a file needs a name")
	}
	out["attachment_id"] = id
	out["filename"] = name
	return nil
}

// cleanTable keeps how far a table reaches beyond the text. It is a share and not
// a length, so it means the same on every screen, and our own code makes it a style.
func cleanTable(in, out map[string]any) error {
	if stretch, ok := intAttr(in["stretch"]); ok && stretch >= 1 && stretch <= maxTableStretch {
		out["stretch"] = stretch
	}
	keepTrue(in, out, "numbered")
	keepTrue(in, out, "flexible")
	keepText(in, out, "caption", maxCaptionBytes)
	keepTrue(in, out, "caption_below")
	keepBlockID(in, out)
	return nil
}

func cleanCell(in, out map[string]any) error {
	for _, key := range []string{"colspan", "rowspan"} {
		if v, ok := intAttr(in[key]); ok && v > 1 && v <= 1000 {
			out[key] = v
		}
	}
	if widths := columnWidths(in["colwidth"]); widths != nil {
		out["colwidth"] = widths
	}
	markColors.keep(in, out, "background")
	return nil
}

// columnWidths returns the widths when all of them are valid.
func columnWidths(v any) []any {
	widths, ok := v.([]any)
	if !ok || len(widths) == 0 || len(widths) > 1000 {
		return nil
	}
	out := make([]any, 0, len(widths))
	for _, w := range widths {
		width, ok := intAttr(w)
		if !ok || width <= 0 || width > 10000 {
			return nil
		}
		out = append(out, width)
	}
	return out
}

func cleanPanel(in, out map[string]any) error {
	panelKinds.keep(in, out, "kind")
	if out["kind"] == nil {
		return errors.New("a panel needs a known kind")
	}
	return nil
}

func cleanExpand(in, out map[string]any) error {
	keepText(in, out, "title", maxTitleBytes)
	keepTrue(in, out, "open")
	return nil
}

func cleanColumns(in, out map[string]any) error {
	columnLayouts.keep(in, out, "layout")
	return nil
}

func cleanStatus(in, out map[string]any) error {
	markColors.keep(in, out, "color")
	keepText(in, out, "text", maxStatusBytes)
	if out["color"] == nil || out["text"] == nil {
		return errors.New("a status mark needs words and a known colour")
	}
	return nil
}

func cleanDate(in, out map[string]any) error {
	value, _ := in["value"].(string)
	if _, err := time.Parse(time.DateOnly, value); err != nil {
		return errors.New("a date must look like 2026-11-05")
	}
	out["value"] = value
	return nil
}

func cleanTemplateField(in, out map[string]any) error {
	keepText(in, out, "name", maxFieldNameBytes)
	if out["name"] == nil {
		return errors.New("a field needs a name")
	}
	out["kind"] = "line"
	fieldKinds.keep(in, out, "kind")
	keepText(in, out, "options", maxFieldOptionBytes)
	keepTrue(in, out, "required")
	return nil
}

func cleanInstruction(in, out map[string]any) error {
	keepText(in, out, "text", maxCaptionBytes)
	if out["text"] == nil {
		return errors.New("instruction text cannot be empty")
	}
	return nil
}

// attrCleaners rebuild the attributes of a node from scratch. A type without an
// entry keeps none, so an attribute nobody thought about never gets through.
var attrCleaners = map[string]func(in, out map[string]any) error{
	TypeParagraph:       cleanTextBlock,
	TypeHeading:         cleanHeading,
	TypeOrderedList:     cleanOrderedList,
	TypeTaskItem:        cleanTaskItem,
	TypeCodeBlock:       cleanCodeBlock,
	TypeImage:           cleanImage,
	TypeFile:            cleanFile,
	TypeTable:           cleanTable,
	TypeTableCell:       cleanCell,
	TypeTableHeader:     cleanCell,
	TypePanel:           cleanPanel,
	TypeExpand:          cleanExpand,
	TypeColumns:         cleanColumns,
	TypeStatus:          cleanStatus,
	TypeDate:            cleanDate,
	TypeMention:         cleanMention,
	TypeTemplateField:   cleanTemplateField,
	TypeInstruction:     cleanInstruction,
	TypeDynamic:         cleanDynamic,
	TypePageFacts:       cleanPageFacts,
	TypeExcerpt:         cleanExcerpt,
	TypeAnchor:          cleanAnchor,
	TypeChart:           cleanChart,
	TypeSheet:           cleanSheet,
	TypeMedia:           cleanMedia,
	TypePageLink:        cleanPageLink,
	TypePageCard:        cleanPageCard,
	TypePlaceholderLink: cleanPlaceholderLink}

// cleanAttrs rebuilds the attributes from scratch, keeping only known ones.
func cleanAttrs(n *Node) error {
	out := map[string]any{}
	if cleanNode, ok := attrCleaners[n.Type]; ok {
		if err := cleanNode(n.Attrs, out); err != nil {
			return err
		}
	}
	n.Attrs = nil
	if len(out) > 0 {
		n.Attrs = out
	}
	return nil
}

func cleanColorMark(m Mark, allowed choice) (Mark, error) {
	color, _ := m.Attrs["color"].(string)
	if !allowed[color] {
		return Mark{}, fmt.Errorf("the colour %q is not in the list", truncate(color, 40))
	}
	return Mark{Type: m.Type, Attrs: map[string]any{"color": color}}, nil
}

func cleanLink(m Mark) (Mark, error) {
	href, _ := m.Attrs["href"].(string)
	safe, err := SafeHref(href)
	if err != nil {
		return Mark{}, err
	}
	attrs := map[string]any{"href": safe}
	keepText(m.Attrs, attrs, "title", 1000)
	return Mark{Type: MarkLink, Attrs: attrs}, nil
}

func cleanMark(m Mark) (Mark, error) {
	switch m.Type {
	case MarkBold, MarkItalic, MarkUnderline, MarkStrike, MarkCode, MarkSubscript, MarkSuperscript:
		return Mark{Type: m.Type}, nil
	case MarkLink:
		return cleanLink(m)
	case MarkTextColor:
		return cleanColorMark(m, textColors)
	case MarkHighlight:
		return cleanColorMark(m, markColors)
	}
	return Mark{}, fmt.Errorf("unknown mark %q", truncate(m.Type, 40))
}

func cleanMarks(in []Mark) ([]Mark, error) {
	var out []Mark
	for _, m := range in {
		clean, err := cleanMark(m)
		if err != nil {
			return nil, err
		}
		out = append(out, clean)
	}
	return out, nil
}
