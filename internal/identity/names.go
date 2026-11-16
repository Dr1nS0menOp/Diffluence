// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"strings"
	"unicode"
)

// maxNameWords bounds how many typed words a search for a name asks about.
const maxNameWords = 8

// NameWords turns typed text into a text-search query in which each typed word is the start
// of a word of a name, in any order (NAV-121), for to_tsquery('simple', ...) against
// to_tsvector('simple', name). Letters and digits make a word and everything else separates
// words, so nothing typed can reach the syntax of the query. It is empty when nothing typed
// is a word.
func NameWords(text string) string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(words) > maxNameWords {
		words = words[:maxNameWords]
	}
	terms := make([]string, len(words))
	for i, w := range words {
		terms[i] = "'" + w + "':*"
	}
	return strings.Join(terms, " & ")
}

// addressStart is a LIKE pattern for an e-mail address that starts with the typed text, taken
// literally: the signs LIKE reads are escaped with its default escape, the backslash.
func addressStart(text string) string {
	if text == "" {
		return ""
	}
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(text))
	return escaped + "%"
}
