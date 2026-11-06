// SPDX-License-Identifier: Apache-2.0

// Package office writes documents as Word (DOCX) and OpenDocument (ODT) files,
// and reads Word files. It covers the short list of blocks a document can hold,
// with archive/zip and encoding/xml only, because no module under our licences
// does this well (spec 014, D2).
package office

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/document"
)

// Section is one page in a file. Depth moves its headings down, so a branch reads
// as one document with each page a level below its parent.
type Section struct {
	Title    string
	Depth    int
	Document document.Node
}

// Image is the content of an image in a document.
type Image struct {
	Data []byte
	Ext  string // png, jpeg or gif
}

// Options say how a file refers to what is outside it.
type Options struct {
	// Image gives the bytes of an image. Without them it becomes its alt text.
	Image func(id uuid.UUID) (Image, bool)
}

const maxHeadingLevel = 6

// Light values of the colour names, because a printed page has no dark mode.
var textColors = map[string]string{
	"grey": "5B6472", "slate": "475569", "red": "B3261E", "orange": "A84300", "amber": "855400", "green": "1B7A3D",
	"teal": "0B7570", "blue": "1D5FBF", "indigo": "4338CA", "purple": "7E22CE", "pink": "BE185D", "brown": "7C4A21",
}

var markColors = map[string]string{
	"grey": "E8EBF0", "yellow": "FDF0C4", "green": "D9F2E3", "blue": "DBE8FB", "purple": "EADFFB", "red": "FBE0DD",
}

func attr(n document.Node, key string) string {
	s, _ := n.Attrs[key].(string)
	return s
}

func number(n document.Node, key string) int {
	switch v := n.Attrs[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func headingLevel(n document.Node, depth int) int {
	level := number(n, "level")
	if level < 1 {
		level = 2
	}
	return min(level+depth, maxHeadingLevel)
}

// escape makes text safe inside XML.
func escape(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return ""
	}
	return b.String()
}

func imageID(n document.Node) (uuid.UUID, bool) {
	id, err := uuid.Parse(attr(n, "attachment_id"))
	return id, err == nil
}

// panelLabel is the word that carries the meaning of a panel.
func panelLabel(n document.Node) string {
	kind := attr(n, "kind")
	if kind == "" {
		return ""
	}
	return strings.ToUpper(kind[:1]) + kind[1:]
}

func inlineWords(n document.Node) string { return document.BlockText(n) }

func mentionWords(n document.Node) string {
	if label := attr(n, "label"); label != "" {
		return "@" + label
	}
	return ""
}

func fallbackWords(n document.Node) string {
	return strings.TrimSpace(fmt.Sprintf("%s %s", attr(n, "alt"), attr(n, "caption")))
}
