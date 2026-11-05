// SPDX-License-Identifier: Apache-2.0

package example

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/document"
)

func documents(t *testing.T) map[string]document.Node {
	t.Helper()
	plan, err := loadPlan()
	if err != nil {
		t.Fatal(err)
	}
	refs := references{files: map[string]uuid.UUID{}, pages: map[string]uuid.UUID{}}
	for _, name := range plan.assets() {
		refs.files[name] = uuid.New()
	}
	plan.walk(func(p planPage, _ *planPage) { refs.pages[p.Slug] = uuid.New() })

	out := map[string]document.Node{}
	add := func(file string) {
		raw, err := content.ReadFile("content/" + file)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		filled, err := refs.fill(raw)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		doc, err := document.Normalize(filled)
		if err != nil {
			t.Fatalf("%s does not pass the gate: %v", file, err)
		}
		out[file] = doc
	}
	add(plan.Home)
	plan.walk(func(p planPage, _ *planPage) { add(p.Document) })
	return out
}

func TestEveryDocumentPassesTheGate(t *testing.T) {
	if len(documents(t)) < 10 {
		t.Fatal("the example should have ten documents at least")
	}
}

// "Everything that is possible" would go stale without anyone noticing. When this
// fails, the format gained something: show it in internal/example/content.
func TestTheExampleShowsEverythingTheFormatCanHold(t *testing.T) {
	nodes, marks := map[string]bool{}, map[string]bool{}
	panels, colours, highlights := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, doc := range documents(t) {
		document.Walk(doc, func(n document.Node) {
			nodes[n.Type] = true
			if n.Type == document.TypePanel {
				kind, _ := n.Attrs["kind"].(string)
				panels[kind] = true
			}
			for _, m := range n.Marks {
				marks[m.Type] = true
				color, _ := m.Attrs["color"].(string)
				if m.Type == document.MarkTextColor {
					colours[color] = true
				}
				if m.Type == document.MarkHighlight {
					highlights[color] = true
				}
			}
		})
	}
	missing := []string{}
	for _, name := range document.NodeTypes() {
		if !nodes[name] {
			missing = append(missing, "node "+name)
		}
	}
	for _, name := range document.MarkTypes() {
		if !marks[name] {
			missing = append(missing, "mark "+name)
		}
	}
	for _, name := range document.PanelKinds() {
		if !panels[name] {
			missing = append(missing, "panel "+name)
		}
	}
	for _, name := range document.TextColors() {
		if !colours[name] {
			missing = append(missing, "text colour "+name)
		}
	}
	for _, name := range document.MarkColors() {
		if !highlights[name] {
			missing = append(missing, "highlight "+name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("the example does not show: %s. Add it to internal/example/content.", strings.Join(missing, ", "))
	}
}

func TestAPlaceholderWithoutATargetIsRefused(t *testing.T) {
	refs := references{files: map[string]uuid.UUID{}, pages: map[string]uuid.UUID{}}
	if _, err := refs.fill([]byte(`{"href":"/pages/@page:nowhere"}`)); err == nil {
		t.Fatal("an unknown page must not pass silently")
	}
	if _, err := refs.fill([]byte(`{"attachment_id":"@file:missing.png"}`)); err == nil {
		t.Fatal("an unknown file must not pass silently")
	}
}
