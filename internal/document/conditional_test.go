// SPDX-License-Identifier: Apache-2.0

package document

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kenwerk/kenwerk/internal/conditions"
)

// Spec 074, wave GT3: a section of text a condition switches (TPL-422).

const onSite = `{"match":"all","tests":[{"subject":"field","field":"Works on site","op":"no"}]}`

func section(condition, inner string) string {
	return `{"type":"conditional","attrs":{"condition":` + condition + `},"content":[` + inner + `]}`
}

func words(text string) string {
	return `{"type":"paragraph","content":[{"type":"text","text":"` + text + `"}]}`
}

func TestASectionNeedsAConditionOfTheRightShape(t *testing.T) {
	bad := map[string]string{
		"no condition":             `{"type":"conditional","content":[` + words("x") + `]}`,
		"a condition of words":     section(`"Works on site is no"`, words("x")),
		"no test":                  section(`{"match":"all","tests":[]}`, words("x")),
		"eleven tests":             section(`{"match":"all","tests":[`+strings.Repeat(`{"subject":"field","field":"A","op":"empty"},`, 10)+`{"subject":"field","field":"A","op":"empty"}]}`, words("x")),
		"a group inside a test":    section(`{"match":"all","tests":[{"match":"any","tests":[{"subject":"field","field":"A","op":"empty"}]}]}`, words("x")),
		"an unknown key":           section(`{"match":"all","tests":[{"subject":"field","field":"A","op":"empty","script":"x"}]}`, words("x")),
		"an expression":            section(`{"match":"all","tests":[{"subject":"field","field":"A","op":"matches","value":"^a.*"}]}`, words("x")),
		"an unknown subject":       section(`{"match":"all","tests":[{"subject":"time","op":"empty"}]}`, words("x")),
		"a way to combine":         section(`{"match":"not","tests":[{"subject":"field","field":"A","op":"empty"}]}`, words("x")),
		"a value over 200 letters": section(`{"match":"all","tests":[{"subject":"field","field":"A","op":"is","value":"`+strings.Repeat("a", 201)+`"}]}`, words("x")),
		"no content":               `{"type":"conditional","attrs":{"condition":` + onSite + `}}`,
	}
	for name, block := range bad {
		if _, err := Normalize(doc(block)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	n, err := Normalize(doc(section(onSite, words("Bring safety shoes"))))
	if err != nil {
		t.Fatal(err)
	}
	got := n.Content[0]
	if got.Type != TypeConditional || len(got.Content) != 1 || !HasConditionals(n) {
		t.Fatalf("section: %+v", got)
	}
	if list := ConditionsIn(n); len(list) != 1 || list[0].Tests[0].Field != "Works on site" {
		t.Fatalf("conditions: %+v", list)
	}
}

func TestSectionsNestThreeDeepAndNoDeeper(t *testing.T) {
	three := section(onSite, section(onSite, section(onSite, words("deep"))))
	if _, err := Normalize(doc(three)); err != nil {
		t.Fatalf("three deep: %v", err)
	}
	if _, err := Normalize(doc(section(onSite, three))); err == nil {
		t.Fatal("four deep was accepted")
	}
	// Other blocks between them do not hide the depth.
	hidden := section(onSite, `{"type":"panel","attrs":{"kind":"info"},"content":[`+section(onSite, section(onSite, section(onSite, words("x"))))+`]}`)
	if _, err := Normalize(doc(hidden)); err == nil {
		t.Fatal("four deep inside a panel was accepted")
	}
}

func TestAPageWithASectionCannotBePublished(t *testing.T) {
	n, err := Normalize(doc(section(onSite, words("x"))))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ForPublish(n); err == nil {
		t.Fatal("a section of a template reached a reader")
	}
}

func TestResolvingSectionsKeepsTheTrueAndDropsTheFalse(t *testing.T) {
	yes := `{"match":"all","tests":[{"subject":"field","field":"Y","op":"yes"}]}`
	no := `{"match":"all","tests":[{"subject":"field","field":"N","op":"yes"}]}`
	n, err := Normalize(doc(words("start") + "," + section(yes, words("kept")+","+section(no, words("gone"))+","+section(yes, words("inner"))) +
		"," + section(no, words("dropped")+","+section(yes, words("never"))) + "," + words("end")))
	if err != nil {
		t.Fatal(err)
	}
	resolved := ResolveConditionals(n, func(c conditions.Condition) bool { return c.Tests[0].Field == "Y" })
	if HasConditionals(resolved) {
		t.Fatal("a wrapper stayed")
	}
	text := Text(resolved)
	if text != "start\nkept\ninner\nend\n" {
		t.Fatalf("resolved: %q", text)
	}
	// The original is left as it was.
	if !HasConditionals(n) {
		t.Fatal("resolving changed the template")
	}
}

func TestASectionThatLeavesNothingLeavesAParagraph(t *testing.T) {
	no := `{"match":"all","tests":[{"subject":"field","field":"N","op":"yes"}]}`
	n, err := Normalize(doc(section(no, words("gone"))))
	if err != nil {
		t.Fatal(err)
	}
	resolved := ResolveConditionals(n, func(conditions.Condition) bool { return false })
	if len(resolved.Content) != 1 || resolved.Content[0].Type != TypeParagraph {
		t.Fatalf("an empty page needs a paragraph: %+v", resolved.Content)
	}
	// A list item or a cell keeps a paragraph too, so the document stays valid.
	inItem, err := Normalize(doc(`{"type":"bulletList","content":[{"type":"listItem","content":[` + section(no, words("gone")) + `]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := Marshal(ResolveConditionals(inItem, func(conditions.Condition) bool { return false }))
	if _, err := Normalize(raw); err != nil || !strings.Contains(string(raw), `"listItem","content":[{"type":"paragraph"}]`) {
		t.Fatalf("an emptied list item: %v %s", err, raw)
	}
}

func TestRenamingAFieldReachesItsSectionsAndItsPlaces(t *testing.T) {
	n, err := Normalize(doc(section(onSite, `{"type":"paragraph","content":[{"type":"templateField","attrs":{"name":"Works on site","kind":"line"}}]}`)))
	if err != nil {
		t.Fatal(err)
	}
	renamed, changed := RenameField(n, "Works on site", "On site")
	if !changed {
		t.Fatal("nothing changed")
	}
	raw, _ := json.Marshal(renamed)
	if strings.Contains(string(raw), "Works on site") || strings.Count(string(raw), "On site") != 2 {
		t.Fatalf("renamed: %s", raw)
	}
	if _, changed := RenameField(n, "Other", "New"); changed {
		t.Fatal("a rename of a field the text does not ask changed it")
	}
	// The rename makes a copy; the text it came from stays.
	if raw, _ := json.Marshal(n); !strings.Contains(string(raw), "Works on site") {
		t.Fatal("the rename changed the text it was given")
	}
}
