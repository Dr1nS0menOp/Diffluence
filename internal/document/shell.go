// SPDX-License-Identifier: Apache-2.0

package document

import (
	"strings"
)

// NavItem is an entry of the page tree in an exported file.
type NavItem struct {
	Title    string
	Href     string
	Current  bool
	Children []NavItem
}

// NavHTML writes a tree of pages as nested lists of plain links, so it works
// without script and reads well on a screen reader.
func NavHTML(items []NavItem) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<ul>")
	for _, item := range items {
		b.WriteString("<li>")
		current := ""
		if item.Current {
			current = ` aria-current="page"`
		}
		b.WriteString(`<a href="` + esc(item.Href) + `"` + current + `>` + esc(item.Title) + `</a>`)
		b.WriteString(NavHTML(item.Children))
		b.WriteString("</li>")
	}
	b.WriteString("</ul>")
	return b.String()
}

// Shell is what surrounds a document in a file of its own.
type Shell struct {
	Title     string // the page, for the tab and the heading
	Site      string // the site or space it belongs to
	Language  string
	StyleHref string
	NavLabel  string
	SkipLabel string
	Nav       string // from NavHTML
	Head      string // more tags for the head, written by the caller
	Body      string // from ToHTML
	After     string // more markup after the article, written by the caller
}

// HTML writes a whole page with a skip link, landmarks and a language (SEC-382:
// it loads nothing from elsewhere).
func (s Shell) HTML() string {
	language := s.Language
	if language == "" {
		language = "en"
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="` + esc(language) + `"><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	title := s.Title
	if s.Site != "" && s.Site != s.Title {
		title += " · " + s.Site
	}
	b.WriteString(`<title>` + esc(title) + `</title>`)
	b.WriteString(`<link rel="stylesheet" href="` + esc(s.StyleHref) + `">`)
	b.WriteString(s.Head)
	b.WriteString(`</head><body>`)
	b.WriteString(`<a class="skip" href="#content">` + esc(s.SkipLabel) + `</a><div class="layout">`)
	if s.Nav != "" {
		b.WriteString(`<nav class="site-nav" aria-label="` + esc(s.NavLabel) + `">` + s.Nav + `</nav>`)
	}
	b.WriteString(`<main id="content"><article><h1>` + esc(s.Title) + `</h1>`)
	b.WriteString(s.Body)
	b.WriteString(`</article>` + s.After + `</main></div></body></html>`)
	return b.String()
}
