// SPDX-License-Identifier: Apache-2.0

package fields

import (
	"strings"

	"github.com/kenwerk/kenwerk/internal/document"
)

// The words a yes/no field puts in a page. Kenwerk speaks one language today, so the
// words of a space are these (TPL-212).
const (
	wordYes = "Yes"
	wordNo  = "No"
)

func text(s string) document.Node { return document.Node{Type: document.TypeText, Text: s} }

// Fill gives what an answer puts in a page, by kind (TPL-212): text, paragraphs, a date
// mark, mentions or a page link. Inline is used where the field sits among other words;
// Blocks, when set, replaces a paragraph that holds the field alone. Answers stay content
// the product makes, never markup from the person (TPL-842). value is a checked answer.
func Fill(s Spec, value string) document.Filling {
	switch s.Kind {
	case KindText:
		return fillText(value)
	case KindDate:
		if value == "" {
			return document.Filling{}
		}
		return inline(document.Node{Type: document.TypeDate, Attrs: map[string]any{"value": value}})
	case KindYesNo:
		switch {
		case s.Show == ShowNone:
			return document.Filling{}
		case value == "yes":
			return inline(text(wordYes))
		default:
			return inline(text(wordNo))
		}
	case KindChoices:
		return fillChoices(s, value)
	case KindPerson, KindPeople:
		return fillPeople(value)
	case KindPage:
		if value == "" {
			return document.Filling{}
		}
		return inline(document.Node{Type: document.TypePageLink, Attrs: map[string]any{"page_id": value}})
	}
	if value == "" {
		return document.Filling{}
	}
	return inline(text(value))
}

func inline(nodes ...document.Node) document.Filling { return document.Filling{Inline: nodes} }

// lines gives the lines of a text with a hard break between each two.
func lines(block string) []document.Node {
	out := []document.Node{}
	for i, line := range strings.Split(block, "\n") {
		if i > 0 {
			out = append(out, document.Node{Type: document.TypeHardBreak})
		}
		if line != "" {
			out = append(out, text(line))
		}
	}
	return out
}

// fillText keeps the lines of a text: inline, each line break a hard break; alone in its
// paragraph, a paragraph for each block of lines.
func fillText(value string) document.Filling {
	if value == "" {
		return document.Filling{}
	}
	f := document.Filling{Inline: lines(value)}
	for _, block := range strings.Split(value, "\n\n") {
		block = strings.Trim(block, "\n")
		if block == "" {
			continue
		}
		f.Blocks = append(f.Blocks, document.Node{Type: document.TypeParagraph, Content: lines(block)})
	}
	return f
}

func fillChoices(s Spec, value string) document.Filling {
	if value == "" {
		return document.Filling{}
	}
	chosen := strings.Split(value, "\n")
	f := inline(text(strings.Join(chosen, ", ")))
	if s.Show == ShowList {
		list := document.Node{Type: document.TypeBulletList}
		for _, c := range chosen {
			list.Content = append(list.Content, document.Node{Type: document.TypeListItem,
				Content: []document.Node{{Type: document.TypeParagraph, Content: []document.Node{text(c)}}}})
		}
		f.Blocks = []document.Node{list}
	}
	return f
}

func fillPeople(value string) document.Filling {
	f := document.Filling{}
	if value == "" {
		return f
	}
	for i, id := range strings.Split(value, "\n") {
		if i > 0 {
			f.Inline = append(f.Inline, text(", "))
		}
		f.Inline = append(f.Inline, document.Node{Type: document.TypeMention,
			Attrs: map[string]any{"id": id, "kind": document.MentionUser}})
	}
	return f
}
