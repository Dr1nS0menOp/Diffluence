// SPDX-License-Identifier: Apache-2.0

package document

import (
	"strings"
	"unicode/utf8"

	"github.com/diffluence/diffluence/internal/apperr"
)

const (
	maxTemplateFields = 20
	maxAnswerRunes    = 2000
)

// Field is a question a template asks.
type Field struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Options  []string `json:"options,omitempty"`
	Required bool     `json:"required"`
}

// Fields lists the questions of a template. Fields with the same name share one
// answer, so a name shows once.
func Fields(n Node) []Field {
	seen := map[string]bool{}
	out := []Field{}
	Walk(n, func(c Node) {
		if c.Type != TypeTemplateField {
			return
		}
		name, _ := c.Attrs["name"].(string)
		if seen[name] {
			return
		}
		seen[name] = true
		kind, _ := c.Attrs["kind"].(string)
		options, _ := c.Attrs["options"].(string)
		required, _ := c.Attrs["required"].(bool)
		f := Field{Name: name, Kind: kind, Required: required}
		if options != "" {
			f.Options = strings.Split(options, "\n")
		}
		out = append(out, f)
	})
	return out
}

// CheckFields keeps a template within its limit.
func CheckFields(n Node) error {
	if len(Fields(n)) > maxTemplateFields {
		return apperr.Invalid("A template can have at most 20 fields.")
	}
	return nil
}

// FillFields puts answers where the fields are. An answer is plain text: it
// becomes a text node, so it can carry no link and no block. A field without an
// answer stays, for the writer to fill in.
func FillFields(n Node, answers map[string]string) (Node, error) {
	for name, answer := range answers {
		if !utf8.ValidString(answer) || utf8.RuneCountInString(answer) > maxAnswerRunes {
			return n, apperr.Invalid("The answer for " + truncate(name, maxFieldNameBytes) + " is too long.")
		}
	}
	return fillNode(n, answers), nil
}

func fillNode(n Node, answers map[string]string) Node {
	if len(n.Content) == 0 {
		return n
	}
	content := make([]Node, 0, len(n.Content))
	for _, c := range n.Content {
		name, _ := c.Attrs["name"].(string)
		answer, answered := answers[name]
		switch {
		case c.Type != TypeTemplateField:
			content = append(content, fillNode(c, answers))
		case !answered:
			content = append(content, c)
		case answer != "":
			content = append(content, Node{Type: TypeText, Text: answer})
		}
	}
	n.Content = content
	return n
}

func stripInstructions(n Node) Node {
	content := make([]Node, 0, len(n.Content))
	for _, c := range n.Content {
		if c.Type != TypeInstruction {
			content = append(content, stripInstructions(c))
		}
	}
	if len(n.Content) > 0 {
		n.Content = content
	}
	return n
}

// ForPublish makes a draft fit for readers: instruction text goes, and a field
// nobody filled in stops the publish.
func ForPublish(n Node) (Node, error) {
	if len(Fields(n)) > 0 {
		return n, apperr.Invalid("Fill in or remove the template fields before publishing.")
	}
	if HasTemplateValues(n) {
		return n, apperr.Invalid("A value of a template cannot be published. Remove it first.")
	}
	return stripInstructions(n), nil
}

// WithoutFiles leaves out images and files, for something that holds text alone.
func WithoutFiles(n Node) Node {
	content := make([]Node, 0, len(n.Content))
	for _, c := range n.Content {
		if !pointsToAttachment(c) {
			content = append(content, WithoutFiles(c))
		}
	}
	if len(n.Content) > 0 {
		n.Content = content
	}
	if n.Type == TypeDoc && len(n.Content) == 0 {
		n.Content = []Node{{Type: TypeParagraph}}
	}
	return n
}
