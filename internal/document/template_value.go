// SPDX-License-Identifier: Apache-2.0

package document

import (
	"errors"
	"regexp"
	"slices"
	"strings"
)

// The built-in values a template knows without asking (spec 074, TPL-201), and the prefix
// of a link to another entry of the same template (TPL-110, TPL-211).
const (
	ValueDate     = "date"
	ValueSpace    = "space"
	ValueCreator  = "creator"
	ValueSet      = "set"
	ValueTemplate = "template"

	valueEntryPrefix = "entry:"
)

var (
	builtInValues = []string{ValueDate, ValueSpace, ValueCreator, ValueSet, ValueTemplate}
	// An entry key is made by Diffluence, so it is short and plain; anything else is refused.
	entryKeyPattern = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
)

// EntrySource is the source of a templateValue that links to the page made from an entry.
func EntrySource(key string) string { return valueEntryPrefix + key }

func cleanTemplateValue(in, out map[string]any) error {
	source, _ := in["source"].(string)
	if slices.Contains(builtInValues, source) {
		out["source"] = source
		return nil
	}
	key, isEntry := strings.CutPrefix(source, valueEntryPrefix)
	if !isEntry || !entryKeyPattern.MatchString(key) {
		return errors.New("a template value needs a known source")
	}
	out["source"] = source
	return nil
}

// HasTemplateValues reports whether a document holds any templateValue. Only the hidden
// page of a template may (TPL-211).
func HasTemplateValues(n Node) bool {
	found := false
	Walk(n, func(c Node) { found = found || c.Type == TypeTemplateValue })
	return found
}

// LinkedEntries lists, sorted and once each, the keys of the entries a document links to.
func LinkedEntries(n Node) []string {
	out := []string{}
	Walk(n, func(c Node) {
		if c.Type != TypeTemplateValue {
			return
		}
		source, _ := c.Attrs["source"].(string)
		if key, ok := strings.CutPrefix(source, valueEntryPrefix); ok && !slices.Contains(out, key) {
			out = append(out, key)
		}
	})
	slices.Sort(out)
	return out
}

// FillValues replaces every templateValue with what value gives for its source. A source
// value cannot give leaves nothing behind, so no templateValue survives.
func FillValues(n Node, value func(source string) (Node, bool)) Node {
	if len(n.Content) == 0 {
		return n
	}
	content := make([]Node, 0, len(n.Content))
	for _, c := range n.Content {
		if c.Type != TypeTemplateValue {
			content = append(content, FillValues(c, value))
			continue
		}
		source, _ := c.Attrs["source"].(string)
		if filled, ok := value(source); ok {
			content = append(content, filled)
		}
	}
	n.Content = content
	return n
}
