// SPDX-License-Identifier: Apache-2.0

package search

import (
	"regexp"
	"strings"
	"unicode"
)

const (
	maxTerms     = 20
	maxWildcards = 3
	titleWeight  = "A"
)

// Parsed is what a typed search means. Every term in TSQuery is letters and digits
// only, so nothing typed can act as query syntax (spec 011, D18).
type Parsed struct {
	TSQuery string
	// Patterns check wildcards inside a word against the text. There is no index
	// for these, so a search has at most three.
	Patterns []string
	Labels   []string
	Spaces   []string
	Authors  []string
	// AnyTerm holds each typed word as an alternative, to look for names of people
	// mentioned. Empty for a search with no words.
	AnyTerm string
	// Plain is typed words without any of the language, so the text can also be a title.
	Plain bool
}

type tokenKind int

const (
	tokenWord tokenKind = iota
	tokenPhrase
	tokenField
	tokenAnd
	tokenOr
	tokenNot
	tokenOpen
	tokenClose
)

type token struct {
	kind  tokenKind
	text  string
	field string
}

var fieldPattern = regexp.MustCompile(`^(title|label|space|author):(.*)$`)

func isTermRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// words splits text at everything that is not a letter or digit.
func words(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool { return !isTermRune(r) })
}

func readQuoted(runes []rune, from int) (string, int, bool) {
	for i := from; i < len(runes); i++ {
		if runes[i] == '"' {
			return string(runes[from:i]), i + 1, true
		}
	}
	return "", from, false
}

func readBare(runes []rune, from int) (string, int) {
	i := from
	for i < len(runes) && !unicode.IsSpace(runes[i]) && runes[i] != '(' && runes[i] != ')' && runes[i] != '"' {
		i++
	}
	return string(runes[from:i]), i
}

func wordToken(text string) token {
	switch text {
	case "AND":
		return token{kind: tokenAnd}
	case "OR":
		return token{kind: tokenOr}
	case "NOT":
		return token{kind: tokenNot}
	}
	return token{kind: tokenWord, text: text}
}

func tokenize(input string) []token {
	runes := []rune(input)
	var out []token
	for i := 0; i < len(runes); {
		r := runes[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '(':
			out = append(out, token{kind: tokenOpen})
			i++
		case r == ')':
			out = append(out, token{kind: tokenClose})
			i++
		case r == '"':
			text, next, closed := readQuoted(runes, i+1)
			if !closed {
				i++
				continue
			}
			out = append(out, token{kind: tokenPhrase, text: text})
			i = next
		default:
			text, next := readBare(runes, i)
			i = next
			m := fieldPattern.FindStringSubmatch(text)
			if m == nil {
				out = append(out, wordToken(text))
				continue
			}
			value := m[2]
			if value == "" && i < len(runes) && runes[i] == '"' {
				if quoted, after, closed := readQuoted(runes, i+1); closed {
					value, i = quoted, after
				}
			}
			out = append(out, token{kind: tokenField, field: m[1], text: value})
		}
	}
	return out
}

type parser struct {
	tokens []token
	at     int
	terms  int
	weight string
	out    *Parsed
	failed bool
	// anyTerms are the words seen, for Parsed.AnyTerm.
	anyTerms []string
}

func (p *parser) next() (token, bool) {
	if p.at >= len(p.tokens) {
		return token{}, false
	}
	return p.tokens[p.at], true
}

// term writes one word with its common forms. A word past the limit is left out.
func (p *parser) term(word string) string {
	if !p.count(word) {
		return ""
	}
	return formsTerm(word, weightSuffix("", p.weight))
}

// exactTerm writes one word as typed: quotes ask for these words and no other forms.
func (p *parser) exactTerm(word string) string {
	if !p.count(word) {
		return ""
	}
	return "'" + strings.ToLower(word) + "'" + weightSuffix("", p.weight)
}

func (p *parser) count(word string) bool {
	if p.terms >= maxTerms {
		return false
	}
	p.terms++
	p.anyTerms = append(p.anyTerms, "'"+strings.ToLower(word)+"'")
	return true
}

func weightSuffix(prefix, weight string) string {
	if prefix == "" && weight == "" {
		return ""
	}
	return ":" + prefix + weight
}

// wildcard turns pl*n into the prefix pl, with a check on the text for the rest.
func (p *parser) wildcard(word string) string {
	head := strings.IndexAny(word, "*?")
	stem := strings.ToLower(word[:head])
	if p.terms >= maxTerms {
		return ""
	}
	p.terms++
	p.anyTerms = append(p.anyTerms, "'"+stem+"':*")
	onlyEnd := head == len(word)-1 && word[head] == '*'
	if !onlyEnd && len(p.out.Patterns) < maxWildcards {
		p.out.Patterns = append(p.out.Patterns, wildcardPattern(word))
	}
	return "'" + stem + "'" + weightSuffix("*", p.weight)
}

// wildcardPattern is a regular expression built only from letters, digits and
// what the two wildcards stand for.
func wildcardPattern(word string) string {
	var b strings.Builder
	b.WriteString(`\m`)
	for _, r := range strings.ToLower(word) {
		switch r {
		case '*':
			b.WriteString(`\w*`)
		case '?':
			b.WriteString(`\w`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteString(`\M`)
	return b.String()
}

func join(parts []string, operator string) string {
	kept := parts[:0:0]
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, operator)
}

// wordPiece renders typed text that may hold signs and wildcards. Signs split it
// into plain words, so `foo:bar` means foo and bar.
func (p *parser) wordPiece(text string) string {
	var parts []string
	for _, piece := range strings.FieldsFunc(text, func(r rune) bool { return !isTermRune(r) && r != '*' && r != '?' }) {
		trimmed := strings.TrimLeft(piece, "*?")
		switch {
		case trimmed == "":
		case strings.ContainsAny(trimmed, "*?"):
			parts = append(parts, p.wildcard(trimmed))
		default:
			parts = append(parts, p.term(trimmed))
		}
	}
	return join(parts, " & ")
}

func (p *parser) phrase(text string) string {
	var parts []string
	for _, word := range words(text) {
		parts = append(parts, p.exactTerm(word))
	}
	return join(parts, " <-> ")
}

func (p *parser) field(t token) string {
	value := strings.TrimSpace(t.text)
	if value == "" {
		return ""
	}
	switch t.field {
	case "label":
		p.out.Labels = append(p.out.Labels, strings.ToLower(value))
	case "space":
		p.out.Spaces = append(p.out.Spaces, strings.ToLower(value))
	case "author":
		p.out.Authors = append(p.out.Authors, value)
	case "title":
		before := p.weight
		p.weight = titleWeight
		defer func() { p.weight = before }()
		return join([]string{p.phrase(value)}, "")
	}
	return ""
}

func (p *parser) primary() string {
	t, ok := p.next()
	if !ok {
		p.failed = true
		return ""
	}
	p.at++
	switch t.kind {
	case tokenOpen:
		inner := p.or()
		closing, ok := p.next()
		if !ok || closing.kind != tokenClose {
			p.failed = true
			return ""
		}
		p.at++
		if inner == "" {
			return ""
		}
		return "(" + inner + ")"
	case tokenPhrase:
		return p.phrase(t.text)
	case tokenField:
		return p.field(t)
	case tokenWord:
		return p.wordPiece(t.text)
	}
	p.failed = true
	return ""
}

func (p *parser) negated() string {
	t, ok := p.next()
	if !ok || t.kind != tokenNot {
		return p.primary()
	}
	p.at++
	inner := p.negated()
	if inner == "" {
		return ""
	}
	return "!(" + inner + ")"
}

func (p *parser) and() string {
	parts := []string{p.negated()}
	for !p.failed {
		t, ok := p.next()
		if !ok || t.kind == tokenOr || t.kind == tokenClose {
			break
		}
		if t.kind == tokenAnd {
			p.at++
		}
		parts = append(parts, p.negated())
	}
	return join(parts, " & ")
}

func (p *parser) or() string {
	parts := []string{p.and()}
	for !p.failed {
		t, ok := p.next()
		if !ok || t.kind != tokenOr {
			break
		}
		p.at++
		parts = append(parts, p.and())
	}
	return join(parts, " | ")
}

// isPlainWords: a search without any of the language keeps the rule that the
// last word is also a prefix (ORG-012).
func isPlainWords(tokens []token, input string) bool {
	if strings.ContainsAny(input, "*?") {
		return false
	}
	for _, t := range tokens {
		if t.kind != tokenWord {
			return false
		}
	}
	return true
}

func plainWords(input, weight string) string {
	fields := words(input)
	if len(fields) > maxTerms {
		fields = fields[:maxTerms]
	}
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(plainTerms(fields, weight), " & ")
}

// Prefixes reads typed text as plain words, for what has no search language of its own.
func Prefixes(input string) string { return plainWords(input, "") }

// plainTerms writes typed words, with the last one also as a prefix.
func plainTerms(fields []string, weight string) []string {
	terms := make([]string, len(fields))
	for i, f := range fields {
		terms[i] = formsTerm(f, weightSuffix("", weight))
	}
	last := len(fields) - 1
	terms[last] = formsTerm(fields[last], weightSuffix("*", weight))
	return terms
}

func parsedPlain(input, weight string) Parsed {
	query := plainWords(input, weight)
	if query == "" {
		return Parsed{Plain: true}
	}
	fields := words(input)
	return Parsed{TSQuery: query, Plain: true, AnyTerm: strings.Join(plainTerms(fields[:min(len(fields), maxTerms)], ""), " | ")}
}

// Parse reads the search language. It never fails: what cannot be understood falls
// back to plain words (ORG-032).
func Parse(input string, titlesOnly bool) Parsed {
	weight := ""
	if titlesOnly {
		weight = titleWeight
	}
	tokens := tokenize(input)
	if isPlainWords(tokens, input) {
		return parsedPlain(input, weight)
	}
	out := Parsed{}
	p := &parser{tokens: tokens, weight: weight, out: &out}
	query := p.or()
	if p.failed || p.at < len(tokens) {
		return parsedPlain(input, weight)
	}
	out.TSQuery = query
	out.AnyTerm = strings.Join(p.anyTerms, " | ")
	return out
}
