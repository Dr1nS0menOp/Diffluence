// SPDX-License-Identifier: Apache-2.0

package search

import "strings"

// minStem is the shortest stem worth searching on: below it a stem matches too many words.
const minStem = 3

// stemSuffixes are endings of plurals and simple verb forms in English and Dutch, longest
// first. The index keeps every word as written ('simple'), so word forms are found by
// asking for a few forms at once instead of by a dictionary per language (ADR-037).
var stemSuffixes = []string{"ies", "es", "en", "s"}

// stem strips one ending of a plural or simple verb form, or returns the word unchanged.
func stem(word string) string {
	for _, suffix := range stemSuffixes {
		base, found := strings.CutSuffix(word, suffix)
		if !found || len([]rune(base)) < minStem {
			continue
		}
		if suffix == "ies" {
			return base + "y"
		}
		return base
	}
	return word
}

// forms is the word with the forms that people mean by it: the stem, and the stem with the
// common plural endings. plans finds plan, and plan finds plans.
func forms(word string) []string {
	if strings.ContainsAny(word, "0123456789") {
		// A number or a code has no plural: 2026s would only match noise.
		return []string{word}
	}
	base := stem(word)
	out := []string{word}
	add := func(form string) {
		for _, seen := range out {
			if seen == form {
				return
			}
		}
		out = append(out, form)
	}
	add(base)
	if len([]rune(base)) >= minStem {
		if root, ok := strings.CutSuffix(base, "y"); ok {
			add(root + "ies")
		} else {
			add(base + "s")
		}
		add(base + "en")
	}
	return out
}

// formsTerm writes a word as an OR of its forms, each with the same suffix (a prefix mark or
// a weight). A single form needs no group.
func formsTerm(word, suffix string) string {
	all := forms(strings.ToLower(word))
	if len(all) == 1 {
		return "'" + all[0] + "'" + suffix
	}
	parts := make([]string, len(all))
	for i, form := range all {
		parts[i] = "'" + form + "'" + suffix
	}
	return "(" + strings.Join(parts, " | ") + ")"
}
