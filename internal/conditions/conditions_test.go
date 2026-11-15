// SPDX-License-Identifier: Apache-2.0

package conditions

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The table of cases that TypeScript runs as well (spec 074, TPL-401, TPL-403, TPL-431): the
// form's preview and the server never disagree on a case it holds.

type caseTable struct {
	Today     string         `json:"today"`
	Operators map[Kind][]Op  `json:"operators"`
	Tests     []testCase     `json:"tests"`
	Combined  []combinedCase `json:"conditions"`
	Validate  []validateCase `json:"validate"`
	Schema    tableSchema    `json:"schema"`
}

type testCase struct {
	Name   string   `json:"name"`
	Kind   Kind     `json:"kind"`
	Op     Op       `json:"op"`
	Value  string   `json:"value"`
	Values []string `json:"values"`
	Today  bool     `json:"today"`
	Answer string   `json:"answer"`
	Result bool     `json:"result"`
}

type combinedCase struct {
	Name          string            `json:"name"`
	Condition     Condition         `json:"condition"`
	Answers       map[string]Answer `json:"answers"`
	Space         string            `json:"space"`
	Groups        []string          `json:"groups"`
	MissingGroups []string          `json:"missing_groups"`
	Result        bool              `json:"result"`
}

type validateCase struct {
	Name      string          `json:"name"`
	Condition json.RawMessage `json:"condition"`
	Space     bool            `json:"space"`
	Problem   string          `json:"problem"`
	Test      *int            `json:"test"`
}

type tableSchema struct {
	Fields map[string]Field `json:"fields"`
	Later  []string         `json:"later"`
}

func readTable(t *testing.T) caseTable {
	t.Helper()
	raw, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var tb caseTable
	if err := json.Unmarshal(raw, &tb); err != nil {
		t.Fatal(err)
	}
	return tb
}

// expandCondition makes the long parts the table names with @, as the TypeScript suite does:
// "@testsN" is N tests, and "@N" as a value is N letters.
func expandCondition(t *testing.T, raw json.RawMessage) Condition {
	t.Helper()
	var loose map[string]any
	if err := json.Unmarshal(raw, &loose); err != nil {
		t.Fatal(err)
	}
	if s, ok := loose["tests"].(string); ok {
		n, _ := strconv.Atoi(strings.TrimPrefix(s, "@tests"))
		tests := make([]any, n)
		for i := range tests {
			tests[i] = map[string]any{"subject": "field", "field": "Name", "op": "empty"}
		}
		loose["tests"] = tests
	}
	for _, test := range loose["tests"].([]any) {
		m := test.(map[string]any)
		if v, ok := m["value"].(string); ok && strings.HasPrefix(v, "@") {
			n, _ := strconv.Atoi(v[1:])
			m["value"] = strings.Repeat("a", n)
		}
	}
	again, err := json.Marshal(loose)
	if err != nil {
		t.Fatal(err)
	}
	var c Condition
	if err := json.Unmarshal(again, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTheOperatorsOfEachKindFollowTheSharedTable(t *testing.T) {
	tb := readTable(t)
	if len(tb.Operators) != len(Operators) {
		t.Fatalf("kinds: %d in the table, %d in Go", len(tb.Operators), len(Operators))
	}
	for kind, ops := range tb.Operators {
		if !slices.Equal(Operators[kind], ops) {
			t.Errorf("%s: %v, want %v", kind, Operators[kind], ops)
		}
	}
}

func TestEveryOperatorFollowsTheSharedTable(t *testing.T) {
	tb := readTable(t)
	// Every operator of every kind of question has a case on an empty answer (TPL-403).
	emptyCase := map[string]bool{}
	for _, c := range tb.Tests {
		if c.Answer == "" {
			emptyCase[string(c.Kind)+" "+string(c.Op)] = true
		}
	}
	for kind, ops := range tb.Operators {
		if kind == KindSpace || kind == KindGroups {
			continue
		}
		for _, op := range ops {
			if !emptyCase[string(kind)+" "+string(op)] {
				t.Errorf("no case of %s %s on an empty answer", kind, op)
			}
		}
	}
	for _, c := range tb.Tests {
		t.Run(c.Name, func(t *testing.T) {
			cond := Condition{Match: All, Tests: []Test{{Subject: SubjectField, Field: "Q", Op: c.Op, Value: c.Value, Values: c.Values, Today: c.Today}}}
			facts := Facts{Answers: map[string]Answer{"Q": {Kind: c.Kind, Value: c.Answer}}, Today: tb.Today}
			if got := Holds(&cond, facts); got != c.Result {
				t.Fatalf("%s %s %q on %q: %v, want %v", c.Kind, c.Op, c.Value, c.Answer, got, c.Result)
			}
		})
	}
}

func TestConditionsFollowTheSharedTable(t *testing.T) {
	tb := readTable(t)
	for _, c := range tb.Combined {
		t.Run(c.Name, func(t *testing.T) {
			facts := Facts{Answers: c.Answers, SpaceID: c.Space, Groups: c.Groups, MissingGroups: c.MissingGroups, Today: tb.Today}
			if got := Holds(&c.Condition, facts); got != c.Result {
				t.Fatalf("%v, want %v", got, c.Result)
			}
		})
	}
}

func TestNoConditionAlwaysHolds(t *testing.T) {
	if !Holds(nil, Facts{}) {
		t.Fatal("a part without a condition must always apply")
	}
}

func TestTheValidatorFollowsTheSharedTable(t *testing.T) {
	tb := readTable(t)
	later := map[string]bool{}
	for _, name := range tb.Schema.Later {
		later[name] = true
	}
	for _, c := range tb.Validate {
		t.Run(c.Name, func(t *testing.T) {
			p := Validate(expandCondition(t, c.Condition), Schema{Fields: tb.Schema.Fields, Later: later, Space: c.Space})
			got := ""
			if p != nil {
				got = p.Code
			}
			if got != c.Problem {
				t.Fatalf("problem %q, want %q (%+v)", got, c.Problem, p)
			}
			if p != nil && c.Test != nil && p.Test != *c.Test {
				t.Fatalf("test %d, want %d", p.Test, *c.Test)
			}
		})
	}
}

func TestEveryProblemHasWords(t *testing.T) {
	for code := range messages {
		p := Problem{Code: code, Test: 2}
		if msg := p.Message(); msg == "" || strings.Contains(msg, "{") {
			t.Errorf("%s: %q", code, msg)
		}
	}
	if msg := (Problem{Code: "too_many_tests", Test: 10}).Message(); !strings.Contains(msg, "10") {
		t.Fatalf("the limit is named: %q", msg)
	}
	if msg := (Problem{Code: "bad_op", Test: 2}).Message(); !strings.HasPrefix(msg, "Test 3: ") {
		t.Fatalf("a test is counted from one: %q", msg)
	}
}

func TestCombineIsTheOneWayToJoinTests(t *testing.T) {
	yes := func(int) bool { return true }
	no := func(int) bool { return false }
	second := func(i int) bool { return i == 1 }
	for _, c := range []struct {
		match Match
		n     int
		test  func(int) bool
		want  bool
	}{
		{All, 3, yes, true}, {All, 3, second, false}, {Any, 3, second, true}, {Any, 3, no, false},
		{All, 0, yes, false}, {Any, 0, yes, false}, {All, MaxTests + 1, yes, false}, {"most", 1, yes, false},
	} {
		if got := Combine(c.match, c.n, c.test); got != c.want {
			t.Errorf("%s of %d: %v, want %v", c.match, c.n, got, c.want)
		}
	}
}

// The evaluator never panics and never takes more than a fixed number of steps, whatever it is
// given (TPL-430): a structure that cannot express a loop cannot hang.
func FuzzEvaluationIsTotalAndBounded(f *testing.F) {
	f.Add("all", "field", "Q", "includes_all", "a\nb", "line", "a\nb\nc", "2027-03-01", false)
	f.Add("any", "groups", "", "member", "g", "", "", "", true)
	f.Add("all", "field", "Q", "lt", "1e308", "number", "-1e308", "x", false)
	f.Fuzz(func(t *testing.T, match, subject, field, op, value, kind, answer, today string, useToday bool) {
		values := strings.Split(value, "\n")
		tests := make([]Test, 0, MaxTests+2)
		for i := range MaxTests + 2 {
			tests = append(tests, Test{Subject: Subject(subject), Field: field, Op: Op(op), Value: value, Values: values, Today: useToday && i%2 == 0})
			cond := Condition{Match: Match(match), Tests: tests}
			facts := Facts{Answers: map[string]Answer{field: {Kind: Kind(kind), Value: answer}}, Today: today, SpaceID: value,
				Groups: values, MissingGroups: values[:len(values)/2]}
			steps := 0
			evaluate(&cond, facts, &steps)
			if steps > maxSteps {
				t.Fatalf("%d steps, more than %d", steps, maxSteps)
			}
			_ = Validate(cond, Schema{Fields: map[string]Field{field: {Kind: Kind(kind), Options: values}}})
		}
	})
}

// Every condition of a template of 100 entries is evaluated within 1 ms (TPL-905): one on
// each of the 99 entries below the first, the 40 questions and the 10 steps, and 100 sections
// of text, 249 in all, each of 10 tests that go round every operator of every kind.
func BenchmarkEveryConditionOfAHundredEntryTemplate(b *testing.B) {
	conds, facts := bigTemplate(mixedTests)
	benchmarkConditions(b, conds, facts)
}

// The same template where every test is the costliest there is: "includes all of" 50 options
// on an answer of 50 options. No template needs it; it bounds the cost from above.
func BenchmarkEveryConditionAtItsCostliest(b *testing.B) {
	conds, facts := bigTemplate(costliestTests)
	benchmarkConditions(b, conds, facts)
}

func benchmarkConditions(b *testing.B, conds []Condition, facts Facts) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		// A set reads its answers once, and then decides every condition on them.
		facts := facts.Prepared()
		for i := range conds {
			Holds(&conds[i], facts)
		}
	}
}

func TestEveryConditionOfAHundredEntryTemplateTakesUnderAMillisecond(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("a measurement, not run under the race detector or in short mode")
	}
	mixed := testing.Benchmark(BenchmarkEveryConditionOfAHundredEntryTemplate)
	costliest := testing.Benchmark(BenchmarkEveryConditionAtItsCostliest)
	t.Logf("every condition of a 100-entry template: %d ns, %d allocations; every test at its costliest: %d ns",
		mixed.NsPerOp(), mixed.AllocsPerOp(), costliest.NsPerOp())
	if mixed.NsPerOp() > 1_000_000 {
		t.Fatalf("%d ns, over the budget of 1 ms", mixed.NsPerOp())
	}
}

const (
	benchConditions = 99 + 40 + 10 + 100
	benchQuestions  = 40
	benchOptions    = 50
)

// bigTemplate is the largest template the limits allow, with every condition's tests made by
// tests, and answers to every question.
func bigTemplate(tests func(i int, options []string) []Test) ([]Condition, Facts) {
	options := make([]string, benchOptions)
	for i := range options {
		options[i] = fmt.Sprintf("Option %d", i)
	}
	answers := map[string]Answer{}
	kinds := []Answer{{KindLine, "A laptop for the new starter"}, {KindNumber, "41,5"}, {KindDate, "2027-04-01"},
		{KindYesNo, "yes"}, {KindChoice, "Option 7"}, {KindChoices, strings.Join(options[:10], "\n")}, {KindLabel, "fixed-price"}}
	for i := range benchQuestions {
		answers[fmt.Sprintf("Q%d", i)] = kinds[i%len(kinds)]
	}
	answers["All"] = Answer{Kind: KindChoices, Value: strings.Join(options, "\n")}
	conds := make([]Condition, 0, benchConditions)
	for i := range benchConditions {
		conds = append(conds, Condition{Match: All, Tests: tests(i, options)})
	}
	return conds, Facts{Answers: answers, Today: "2027-03-01", Groups: []string{"g1", "g2"}}
}

// mixedTests goes round the operators of each kind, so that every test is made about as often.
func mixedTests(i int, options []string) []Test {
	shapes := []Test{
		{Op: OpContains, Value: "laptop"}, {Op: OpGe, Value: "40"}, {Op: OpOnOrAfter, Today: true}, {Op: OpYes},
		{Op: OpOneOf, Values: options[:10]}, {Op: OpIncludesAll, Values: options[:5]}, {Op: OpIs, Value: "fixed-price"},
	}
	out := make([]Test, 0, MaxTests)
	for j := range MaxTests {
		q := (i + j) % benchQuestions
		t := shapes[q%len(shapes)]
		t.Subject, t.Field = SubjectField, fmt.Sprintf("Q%d", q)
		out = append(out, t)
	}
	return out
}

func costliestTests(_ int, options []string) []Test {
	out := make([]Test, 0, MaxTests)
	for range MaxTests {
		out = append(out, Test{Subject: SubjectField, Field: "All", Op: OpIncludesAll, Values: options})
	}
	return out
}
