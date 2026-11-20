// SPDX-License-Identifier: Apache-2.0

package fields

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/kenwerk/kenwerk/internal/document"
)

// The table of cases that TypeScript runs as well (spec 074, TPL-325). Go and the form
// never disagree on a case it holds.

const tableCreator = "01938f2e-7b3a-7cde-8f00-00000000c0de"

type table struct {
	Today   string `json:"today"`
	Answers []struct {
		Name    string            `json:"name"`
		Field   json.RawMessage   `json:"field"`
		Answer  string            `json:"answer"`
		Today   string            `json:"today"`
		Problem string            `json:"problem"`
		Params  map[string]string `json:"params"`
		Value   *string           `json:"value"`
	} `json:"answers"`
	Specs []struct {
		Name    string          `json:"name"`
		Field   json.RawMessage `json:"field"`
		Problem string          `json:"problem"`
	} `json:"specs"`
	Defaults []struct {
		Name  string          `json:"name"`
		Field json.RawMessage `json:"field"`
		Value string          `json:"value"`
	} `json:"defaults"`
}

func readTable(t *testing.T) table {
	t.Helper()
	raw, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var tb table
	if err := json.Unmarshal(raw, &tb); err != nil {
		t.Fatal(err)
	}
	return tb
}

// expand makes the long values the table names with @, as the TypeScript suite does.
func expand(v string) string {
	switch {
	case v == "@creator":
		return tableCreator
	case strings.HasPrefix(v, "@people"):
		n, _ := strconv.Atoi(strings.TrimPrefix(v, "@people"))
		ids := make([]string, n)
		for i := range ids {
			ids[i] = fmt.Sprintf("01938f2e-7b3a-7cde-8f00-%012d", i+1)
		}
		return strings.Join(ids, "\n")
	case strings.HasPrefix(v, "@label"):
		n, _ := strconv.Atoi(strings.TrimPrefix(v, "@label"))
		return strings.Repeat("a", n)
	case strings.HasPrefix(v, "@link"):
		n, _ := strconv.Atoi(strings.TrimPrefix(v, "@link"))
		prefix := "https://example.org/"
		return prefix + strings.Repeat("a", n-len(prefix))
	case strings.HasPrefix(v, "@") && len(v) > 1:
		if n, err := strconv.Atoi(v[1:]); err == nil {
			return strings.Repeat("a", n)
		}
	}
	return v
}

// specOf reads a field of the table, expanding its placeholders.
func specOf(t *testing.T, raw json.RawMessage) Spec {
	t.Helper()
	var loose map[string]any
	if err := json.Unmarshal(raw, &loose); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"help", "placeholder"} {
		if s, ok := loose[key].(string); ok {
			loose[key] = expand(s)
		}
	}
	switch options := loose["options"].(type) {
	case string:
		n, _ := strconv.Atoi(strings.TrimPrefix(options, "@options"))
		list := make([]string, n)
		for i := range list {
			list[i] = fmt.Sprintf("o%d", i+1)
		}
		loose["options"] = list
	case []any:
		for i, o := range options {
			options[i] = expand(o.(string))
		}
	}
	again, err := json.Marshal(loose)
	if err != nil {
		t.Fatal(err)
	}
	var s Spec
	if err := json.Unmarshal(again, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAnswersFollowTheSharedTable(t *testing.T) {
	tb := readTable(t)
	if len(tb.Answers) < 100 {
		t.Fatalf("the table lost cases: %d", len(tb.Answers))
	}
	for _, c := range tb.Answers {
		t.Run(c.Name, func(t *testing.T) {
			today := tb.Today
			if c.Today != "" {
				today = c.Today
			}
			value, problem := Answer(specOf(t, c.Field), expand(c.Answer), today)
			got := ""
			if problem != nil {
				got = problem.Code
			}
			if got != c.Problem {
				t.Fatalf("problem %q, want %q (%+v)", got, c.Problem, problem)
			}
			if problem != nil && len(c.Params) > 0 && fmt.Sprint(problem.Params) != fmt.Sprint(c.Params) {
				t.Fatalf("params %v, want %v", problem.Params, c.Params)
			}
			if c.Value != nil && value != *c.Value {
				t.Fatalf("value %q, want %q", value, *c.Value)
			}
		})
	}
}

func TestSpecsFollowTheSharedTable(t *testing.T) {
	tb := readTable(t)
	for _, c := range tb.Specs {
		t.Run(c.Name, func(t *testing.T) {
			problem := specOf(t, c.Field).Check(tb.Today)
			got := ""
			if problem != nil {
				got = problem.Code
			}
			if got != c.Problem {
				t.Fatalf("problem %q, want %q (%+v)", got, c.Problem, problem)
			}
		})
	}
}

func TestDefaultsFollowTheSharedTable(t *testing.T) {
	tb := readTable(t)
	for _, c := range tb.Defaults {
		t.Run(c.Name, func(t *testing.T) {
			if got := specOf(t, c.Field).DefaultValue(tb.Today, tableCreator); got != expand(c.Value) {
				t.Fatalf("default %q, want %q", got, expand(c.Value))
			}
		})
	}
}

func TestEveryProblemHasAMessage(t *testing.T) {
	for _, code := range problemCodes {
		p := Problem{Code: code, Params: map[string]string{"min": "1", "max": "2", "decimals": "1", "date": "2028-03-01"}}
		if msg := p.Message(); msg == "" || strings.Contains(msg, "{") {
			t.Errorf("%s: %q", code, msg)
		}
	}
	if msg := (Problem{Code: "too_early", Params: map[string]string{"date": "2027-03-01"}}).Message(); msg != "Enter a date on or after 1 March 2027." {
		t.Fatalf("a date in a message is written out: %q", msg)
	}
}

// What lands in a page depends on the kind (TPL-212).
func TestWhatLandsInThePageDependsOnTheKind(t *testing.T) {
	person, other := tableCreator, "01938f2e-7b3a-7cde-8f00-0123456789ab"
	cases := []struct {
		name   string
		spec   Spec
		value  string
		inline string
		blocks string
	}{
		{"a line is text", Spec{Kind: KindLine}, "Ada", `[{"type":"text","text":"Ada"}]`, ""},
		{"a number is text as typed", Spec{Kind: KindNumber}, "3,5", `[{"type":"text","text":"3,5"}]`, ""},
		{"a link is text", Spec{Kind: KindLink}, "https://example.org", `[{"type":"text","text":"https://example.org"}]`, ""},
		{"text keeps its lines", Spec{Kind: KindText}, "a\nb\n\nc",
			`[{"type":"text","text":"a"},{"type":"hardBreak"},{"type":"text","text":"b"},{"type":"hardBreak"},{"type":"hardBreak"},{"type":"text","text":"c"}]`,
			`[{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"hardBreak"},{"type":"text","text":"b"}]},{"type":"paragraph","content":[{"type":"text","text":"c"}]}]`},
		{"a date is a date mark", Spec{Kind: KindDate}, "2028-02-29", `[{"type":"date","attrs":{"value":"2028-02-29"}}]`, ""},
		{"yes in words", Spec{Kind: KindYesNo}, "yes", `[{"type":"text","text":"Yes"}]`, ""},
		{"no in words", Spec{Kind: KindYesNo}, "", `[{"type":"text","text":"No"}]`, ""},
		{"yes/no showing nothing", Spec{Kind: KindYesNo, Show: ShowNone}, "yes", `[]`, ""},
		{"choices joined with commas", Spec{Kind: KindChoices}, "A\nB", `[{"type":"text","text":"A, B"}]`, ""},
		{"choices as a list", Spec{Kind: KindChoices, Show: ShowList}, "A\nB", `[{"type":"text","text":"A, B"}]`,
			`[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"A"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"B"}]}]}]}]`},
		{"a person is a mention", Spec{Kind: KindPerson}, person, `[{"type":"mention","attrs":{"id":"` + person + `","kind":"user"}}]`, ""},
		{"people are mentions with commas", Spec{Kind: KindPeople}, person + "\n" + other,
			`[{"type":"mention","attrs":{"id":"` + person + `","kind":"user"}},{"type":"text","text":", "},{"type":"mention","attrs":{"id":"` + other + `","kind":"user"}}]`, ""},
		{"a page is a page link", Spec{Kind: KindPage}, other, `[{"type":"pageLink","attrs":{"page_id":"` + other + `"}}]`, ""},
		{"a label is text", Spec{Kind: KindLabel}, "team-alpha", `[{"type":"text","text":"team-alpha"}]`, ""},
		{"an empty line leaves nothing", Spec{Kind: KindLine}, "", `[]`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := Fill(c.spec, c.value)
			inline, _ := json.Marshal(nonNil(f.Inline))
			if string(inline) != c.inline {
				t.Fatalf("inline %s, want %s", inline, c.inline)
			}
			blocks := ""
			if f.Blocks != nil {
				raw, _ := json.Marshal(f.Blocks)
				blocks = string(raw)
			}
			if blocks != c.blocks {
				t.Fatalf("blocks %s, want %s", blocks, c.blocks)
			}
		})
	}
}

func nonNil(n []document.Node) []document.Node {
	if n == nil {
		return []document.Node{}
	}
	return n
}
