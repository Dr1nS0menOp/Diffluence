// SPDX-License-Identifier: Apache-2.0

package document

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/diffluence/diffluence/internal/conditions"
)

// A section of a template's text that a condition switches (spec 074, TPL-422). When a page
// is made, a true section is unwrapped, so its blocks stay and the wrapper goes, and a false
// one is removed with everything in it. Like a templateValue, it never reaches a reader.

// MaxConditionalDepth is how many sections may sit inside each other. Each has its own
// condition of one level: nesting text is allowed, nesting logic is not.
const MaxConditionalDepth = 3

func cleanConditional(in, out map[string]any) error {
	raw, ok := in["condition"]
	if !ok || raw == nil {
		return errors.New("a section shown when a condition holds needs its condition")
	}
	c, err := decodeCondition(raw)
	if err != nil {
		return errors.New("the condition of a section is not a condition")
	}
	if p := conditions.CheckShape(c); p != nil {
		return errors.New("the condition of a section: " + p.Message())
	}
	attr, err := conditionAttr(c)
	if err != nil {
		return err
	}
	out["condition"] = attr
	return nil
}

// decodeCondition reads a condition from the attribute of a block. A key no condition has,
// such as a group of tests inside a test, refuses it.
func decodeCondition(raw any) (conditions.Condition, error) {
	var c conditions.Condition
	b, err := json.Marshal(raw)
	if err != nil {
		return c, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	err = dec.Decode(&c)
	return c, err
}

// conditionAttr writes a condition as the attribute of a block stores it.
func conditionAttr(c conditions.Condition) (map[string]any, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = json.Unmarshal(b, &out)
	return out, err
}

// conditionOf reads the condition of a section. A section that holds none holds false.
func conditionOf(n Node) (conditions.Condition, bool) {
	c, err := decodeCondition(n.Attrs["condition"])
	return c, err == nil
}

// checkConditionalDepth refuses sections nested deeper than MaxConditionalDepth, whatever
// other blocks stand between them.
func checkConditionalDepth(n Node, depth int) error {
	if n.Type == TypeConditional {
		depth++
		if depth > MaxConditionalDepth {
			return fmt.Errorf("sections shown when a condition holds can sit at most %d inside each other", MaxConditionalDepth)
		}
	}
	for _, c := range n.Content {
		if err := checkConditionalDepth(c, depth); err != nil {
			return err
		}
	}
	return nil
}

// HasConditionals reports whether a document holds a section a condition switches. Only the
// hidden page of a template may.
func HasConditionals(n Node) bool {
	found := false
	Walk(n, func(c Node) { found = found || c.Type == TypeConditional })
	return found
}

// ConditionsIn lists the conditions of every section of a document, outermost first.
func ConditionsIn(n Node) []conditions.Condition {
	out := []conditions.Condition{}
	Walk(n, func(c Node) {
		if c.Type != TypeConditional {
			return
		}
		if cond, ok := conditionOf(c); ok {
			out = append(out, cond)
		}
	})
	return out
}

// ResolveConditionals gives a copy of a document with every section decided: a section whose
// condition holds is unwrapped, and one whose condition does not is removed with all it
// holds, its own sections included. A block left with nothing gets an empty paragraph, so
// the document stays valid.
func ResolveConditionals(n Node, holds func(conditions.Condition) bool) Node {
	if len(n.Content) == 0 {
		return n
	}
	content := make([]Node, 0, len(n.Content))
	for _, c := range n.Content {
		if c.Type != TypeConditional {
			content = append(content, ResolveConditionals(c, holds))
			continue
		}
		if cond, ok := conditionOf(c); ok && holds(cond) {
			content = append(content, ResolveConditionals(c, holds).Content...)
		}
	}
	if len(content) == 0 && contains(children[n.Type], TypeConditional) {
		content = []Node{{Type: TypeParagraph}}
	}
	n.Content = content
	return n
}

// RenameField gives a copy of a document in which a question has its new name, where it is
// asked and in the conditions of its sections (TPL-230), and says whether anything changed.
func RenameField(n Node, from, to string) (Node, bool) {
	changed := false
	switch n.Type {
	case TypeTemplateField:
		if name, _ := n.Attrs["name"].(string); name == from {
			n.Attrs = withAttr(n.Attrs, "name", to)
			changed = true
		}
	case TypeConditional:
		if c, ok := conditionOf(n); ok && c.RenameField(from, to) {
			if attr, err := conditionAttr(c); err == nil {
				n.Attrs = withAttr(n.Attrs, "condition", attr)
				changed = true
			}
		}
	}
	if len(n.Content) == 0 {
		return n, changed
	}
	content := make([]Node, len(n.Content))
	for i, c := range n.Content {
		var moved bool
		content[i], moved = RenameField(c, from, to)
		changed = changed || moved
	}
	n.Content = content
	return n, changed
}

// withAttr copies attributes with one of them changed, so the document it came from stays.
func withAttr(attrs map[string]any, key string, value any) map[string]any {
	// The size is only a hint: a map grows for the one key that may be new.
	out := make(map[string]any, len(attrs))
	for k, v := range attrs {
		out[k] = v
	}
	out[key] = value
	return out
}
