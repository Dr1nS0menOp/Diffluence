// SPDX-License-Identifier: Apache-2.0

package document

import (
	"html"
	"strconv"
	"strings"
)

// maxMailChanges keeps a mail readable: past this, the page itself is the better place.
const maxMailChanges = 20

// Mail clients ignore most style sheets, so a mail carries its own colours. The
// words and the underline and strikethrough mean it never rests on colour alone.
const (
	mailAddedStyle   = "background:#d9f2e3;color:#1e2a3b;text-decoration:underline"
	mailRemovedStyle = "background:#fbdcd9;color:#1e2a3b;text-decoration:line-through"
	mailBlockStyle   = "margin:0 0 8px"
)

// ChangeLabels are the words beside a change, in the language of the reader. More
// holds {count} where the number of changes that were left out goes.
type ChangeLabels struct {
	Added   string
	Removed string
	More    string
}

// shownChanges leaves out what stayed the same, and says how many changes do not fit.
func shownChanges(blocks []DiffBlock) (shown []DiffBlock, left int) {
	for _, b := range blocks {
		if b.Op != OpEqual {
			shown = append(shown, b)
		}
	}
	if len(shown) > maxMailChanges {
		return shown[:maxMailChanges], len(shown) - maxMailChanges
	}
	return shown, 0
}

func moreLine(l ChangeLabels, left int) string {
	return strings.ReplaceAll(l.More, "{count}", strconv.Itoa(left))
}

func wordsAsText(words []WordOp) string {
	parts := make([]string, len(words))
	for i, w := range words {
		switch w.Op {
		case OpAdded:
			parts[i] = "{+" + w.Text + "+}"
		case OpRemoved:
			parts[i] = "[-" + w.Text + "-]"
		default:
			parts[i] = w.Text
		}
	}
	return strings.Join(parts, " ")
}

// ChangesText is a comparison as plain text, for the text part of a mail.
func ChangesText(blocks []DiffBlock, l ChangeLabels) string {
	shown, left := shownChanges(blocks)
	var b strings.Builder
	for _, block := range shown {
		switch block.Op {
		case OpAdded:
			b.WriteString(l.Added + ": " + block.After)
		case OpRemoved:
			b.WriteString(l.Removed + ": " + block.Before)
		default:
			b.WriteString(wordsAsText(block.Words))
		}
		b.WriteByte('\n')
	}
	if left > 0 {
		b.WriteString(moreLine(l, left) + "\n")
	}
	return b.String()
}

func wordsAsHTML(words []WordOp) string {
	parts := make([]string, len(words))
	for i, w := range words {
		text := html.EscapeString(w.Text)
		switch w.Op {
		case OpAdded:
			parts[i] = `<ins style="` + mailAddedStyle + `">` + text + `</ins>`
		case OpRemoved:
			parts[i] = `<del style="` + mailRemovedStyle + `">` + text + `</del>`
		default:
			parts[i] = text
		}
	}
	return strings.Join(parts, " ")
}

func labelled(label, tag, style, text string) string {
	return html.EscapeString(label) + `: <` + tag + ` style="` + style + `">` + html.EscapeString(text) + `</` + tag + `>`
}

// ChangesHTML is a comparison as safe HTML with inline styles, for a mail. Every
// word comes from a page or a translation, so all of it is escaped.
func ChangesHTML(blocks []DiffBlock, l ChangeLabels) string {
	shown, left := shownChanges(blocks)
	var b strings.Builder
	for _, block := range shown {
		b.WriteString(`<p style="` + mailBlockStyle + `">`)
		switch block.Op {
		case OpAdded:
			b.WriteString(labelled(l.Added, "ins", mailAddedStyle, block.After))
		case OpRemoved:
			b.WriteString(labelled(l.Removed, "del", mailRemovedStyle, block.Before))
		default:
			b.WriteString(wordsAsHTML(block.Words))
		}
		b.WriteString("</p>")
	}
	if left > 0 {
		b.WriteString(`<p style="` + mailBlockStyle + `">` + html.EscapeString(moreLine(l, left)) + "</p>")
	}
	return b.String()
}
