// SPDX-License-Identifier: Apache-2.0

// Package conditions decides whether part of something applies, from structured tests picked
// from lists: "all of" or "any of" at most ten tests, on one level, with no expression and no
// text that is parsed (spec 074, TPL-400 to TPL-403, TPL-430; decided TPL-1405).
//
// Everything here is pure. Holds never fails and never loops over more than a fixed number
// of steps: a malformed condition is false, so content that a condition adds never appears
// by surprise. Validate refuses every malformed condition when it is saved, so Holds only
// meets malformed ones that were never saved.
//
// Combine and MaxTests are the part every kind of condition shares, so document control's
// transitions (spec 064, DC5) reuse them (TPL-433). The form in the browser mirrors this
// package in web/src/template-conditions.ts, and both run testdata/cases.json.
package conditions

import (
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Match says how the tests of a condition combine.
type Match string

// The two ways tests combine. There is no "not" around them and no nesting (TPL-400).
const (
	All Match = "all"
	Any Match = "any"
)

// The bounds of a condition (TPL-400).
const (
	// MaxTests is the most tests one condition holds.
	MaxTests = 10
	// MaxValues is the most values one test lists, as many as a question has options.
	MaxValues = 50
	// MaxValueRunes is the longest value a test of words compares with.
	MaxValueRunes = 200
	// maxItems is the most items an answer of several lists: the options of a question.
	maxItems = 50
	// maxSteps bounds the work of one condition: each test, and each comparison of a value
	// with an item of a list.
	maxSteps = MaxTests * (1 + MaxValues*maxItems)
	// maxLabelRunes is the longest label, as the labels module allows.
	maxLabelRunes = 64
)

// Combine joins the outcomes of n tests, test(0) to test(n-1), as match says. No test, more
// than MaxTests, or an unknown match is false: a condition never fails, it is false (TPL-403).
func Combine(match Match, n int, test func(i int) bool) bool {
	if n == 0 || n > MaxTests {
		return false
	}
	switch match {
	case All:
		for i := range n {
			if !test(i) {
				return false
			}
		}
		return true
	case Any:
		for i := range n {
			if test(i) {
				return true
			}
		}
	}
	return false
}

// Subject is what a test looks at.
type Subject string

// The subjects of a test (TPL-400).
const (
	SubjectField  Subject = "field"
	SubjectSpace  Subject = "space"
	SubjectGroups Subject = "groups"
)

// Kind is the kind of what a test looks at: the kind of a question, the space or the groups.
type Kind string

// The kinds of subject. Those of a question are the kinds of field (internal/templates/fields).
const (
	KindLine    Kind = "line"
	KindText    Kind = "text"
	KindLink    Kind = "link"
	KindNumber  Kind = "number"
	KindDate    Kind = "date"
	KindYesNo   Kind = "yesno"
	KindChoice  Kind = "choice"
	KindChoices Kind = "choices"
	KindPerson  Kind = "person"
	KindPeople  Kind = "people"
	KindPage    Kind = "page"
	KindLabel   Kind = "label"
	KindSpace   Kind = "space"
	KindGroups  Kind = "groups"
)

// Op is what a test checks.
type Op string

// The operators (TPL-401).
const (
	OpEmpty       Op = "empty"
	OpNotEmpty    Op = "not_empty"
	OpIs          Op = "is"
	OpIsNot       Op = "is_not"
	OpContains    Op = "contains"
	OpEq          Op = "eq"
	OpNe          Op = "ne"
	OpLt          Op = "lt"
	OpLe          Op = "le"
	OpGt          Op = "gt"
	OpGe          Op = "ge"
	OpBefore      Op = "before"
	OpAfter       Op = "after"
	OpOnOrBefore  Op = "on_or_before"
	OpOnOrAfter   Op = "on_or_after"
	OpYes         Op = "yes"
	OpNo          Op = "no"
	OpOneOf       Op = "one_of"
	OpIncludes    Op = "includes"
	OpExcludes    Op = "excludes"
	OpIncludesAll Op = "includes_all"
	OpMember      Op = "member"
	OpNotMember   Op = "not_member"
)

var words = []Op{OpEmpty, OpNotEmpty, OpIs, OpIsNot, OpContains}

// Operators lists, per kind, the operators a test may use, in the order the editor offers
// them (TPL-401). Each is total and cheap, and defined for an empty value. A test of a person
// or a page stops at "is empty": testing who was chosen would tie a template to identities.
var Operators = map[Kind][]Op{
	KindLine:    words,
	KindText:    words,
	KindLink:    words,
	KindNumber:  {OpEmpty, OpNotEmpty, OpEq, OpNe, OpLt, OpLe, OpGt, OpGe},
	KindDate:    {OpEmpty, OpNotEmpty, OpIs, OpBefore, OpAfter, OpOnOrBefore, OpOnOrAfter},
	KindYesNo:   {OpYes, OpNo},
	KindChoice:  {OpEmpty, OpIs, OpIsNot, OpOneOf},
	KindChoices: {OpEmpty, OpIncludes, OpExcludes, OpIncludesAll},
	KindPerson:  {OpEmpty, OpNotEmpty},
	KindPeople:  {OpEmpty, OpNotEmpty},
	KindPage:    {OpEmpty, OpNotEmpty},
	KindLabel:   {OpIs, OpIsNot, OpOneOf},
	KindSpace:   {OpOneOf},
	KindGroups:  {OpMember, OpNotMember},
}

// Test is one test of a condition. Value holds what one value is compared with, Values the
// list of "is one of" and "includes all of", and Today stands for the day the set is made in
// a test of a date.
type Test struct {
	Subject Subject  `json:"subject"`
	Field   string   `json:"field,omitempty"`
	Op      Op       `json:"op"`
	Value   string   `json:"value,omitempty"`
	Values  []string `json:"values,omitempty"`
	Today   bool     `json:"today,omitempty"`
}

// Condition is "all of" or "any of" a list of tests (TPL-400).
type Condition struct {
	Match Match  `json:"match"`
	Tests []Test `json:"tests"`
}

// Answer is the checked answer of one question with its kind, as it lands in a page. Several
// people or options come one per line. An empty value is no answer.
type Answer struct {
	Kind  Kind   `json:"kind"`
	Value string `json:"value"`
}

// Facts are what a condition is decided on (TPL-430): the answers, the day the set is made as
// YYYY-MM-DD, the space, and the groups of the creator. MissingGroups lists groups the
// condition names that no longer exist, which make their tests false (TPL-402).
type Facts struct {
	Answers       map[string]Answer
	SpaceID       string
	Groups        []string
	MissingGroups []string
	Today         string
	// read holds each answer as the tests read it, made once by Prepared.
	read map[string]readAnswer
}

// readAnswer is an answer as the tests read it: trimmed, in small letters for tests of words,
// and as a set of items for tests of lists. tooMany marks more items than a question can hold.
type readAnswer struct {
	value, lower string
	items        map[string]struct{}
	tooMany      bool
}

// Prepared reads every answer once, so that deciding many conditions on the same facts reads
// no answer twice. Holds prepares facts that were not prepared.
func (f Facts) Prepared() Facts {
	f.read = make(map[string]readAnswer, len(f.Answers))
	for name, a := range f.Answers {
		value := strings.TrimSpace(a.Value)
		r := readAnswer{value: value, lower: strings.ToLower(value)}
		lines := strings.Split(value, "\n")
		r.tooMany = len(lines) > maxItems
		if !r.tooMany {
			r.items = make(map[string]struct{}, len(lines))
			for _, line := range lines {
				r.items[strings.TrimSpace(line)] = struct{}{}
			}
		}
		f.read[name] = r
	}
	return f
}

// Holds decides a condition. A part without a condition always applies, so nil holds.
func Holds(c *Condition, f Facts) bool {
	steps := 0
	return evaluate(c, f, &steps)
}

// evaluate decides a condition and counts its steps, which the fuzz test bounds.
func evaluate(c *Condition, f Facts, steps *int) bool {
	if c == nil {
		return true
	}
	if f.read == nil {
		f = f.Prepared()
	}
	return Combine(c.Match, len(c.Tests), func(i int) bool { return holds(c.Tests[i], f, steps) })
}

func holds(t Test, f Facts, steps *int) bool {
	*steps++
	if len(t.Values) > MaxValues {
		return false
	}
	switch t.Subject {
	case SubjectSpace:
		return t.Op == OpOneOf && f.SpaceID != "" && listed(t.Values, f.SpaceID, steps)
	case SubjectGroups:
		// A group that is gone decides nothing, so its test is false either way (TPL-402).
		if t.Value == "" || slices.Contains(f.MissingGroups, t.Value) {
			return false
		}
		member := slices.Contains(f.Groups, t.Value)
		switch t.Op {
		case OpMember:
			return member
		case OpNotMember:
			return !member
		}
		return false
	case SubjectField:
		return answerHolds(t, f.Answers[t.Field].Kind, f.read[t.Field], f.Today, steps)
	}
	return false
}

// listed says whether value is in list, counting a step per comparison.
func listed(list []string, value string, steps *int) bool {
	for _, item := range list {
		*steps++
		if item == value {
			return true
		}
	}
	return false
}

// answerHolds tests one answer of a question of the given kind. Every test of an empty
// answer is false, except "is empty", "is not", "≠", "does not include" and "is no", which
// are true (TPL-403): an unanswered yes/no question is a no, because no is an answer (TPL-305).
func answerHolds(t Test, kind Kind, a readAnswer, today string, steps *int) bool {
	if kind != "" && !slices.Contains(Operators[kind], t.Op) {
		return false
	}
	value := a.value
	empty := value == ""
	switch t.Op {
	case OpEmpty:
		return empty
	case OpNotEmpty:
		return !empty
	case OpYes:
		return value == "yes"
	case OpNo:
		return value != "yes"
	case OpIsNot, OpNe, OpExcludes:
		if empty {
			return true
		}
	default:
		if empty {
			return false
		}
	}
	switch t.Op {
	case OpIs, OpIsNot:
		same := sameAnswer(kind, a, t, today)
		return same == (t.Op == OpIs)
	case OpContains:
		return strings.Contains(a.lower, strings.ToLower(t.Value))
	case OpEq, OpNe, OpLt, OpLe, OpGt, OpGe:
		return compareNumbers(t.Op, value, t.Value)
	case OpBefore, OpAfter, OpOnOrBefore, OpOnOrAfter:
		return compareDays(t.Op, value, dayOf(t, today))
	case OpOneOf:
		return listed(t.Values, value, steps)
	case OpIncludes, OpExcludes:
		if a.tooMany {
			return false
		}
		*steps++
		_, found := a.items[t.Value]
		return found == (t.Op == OpIncludes)
	case OpIncludesAll:
		if a.tooMany || len(t.Values) == 0 {
			return false
		}
		for _, want := range t.Values {
			*steps++
			if _, found := a.items[want]; !found {
				return false
			}
		}
		return true
	}
	return false
}

// sameAnswer compares an answer with the value of "is": words ignoring case, a day with its
// day, and an option or a label exactly as written.
func sameAnswer(kind Kind, a readAnswer, t Test, today string) bool {
	switch kind {
	case KindLine, KindText, KindLink:
		return a.lower == strings.ToLower(t.Value)
	case KindDate:
		day := dayOf(t, today)
		return day != "" && a.value == day
	}
	return a.value == t.Value
}

// ParseNumber reads a number with a point or a comma as its decimal sign, as a number
// question takes it (TPL-303).
func ParseNumber(value string) (float64, bool) {
	digits := strings.TrimPrefix(value, "-")
	whole := digits
	sign := strings.IndexAny(digits, ".,")
	if sign >= 0 {
		whole = digits[:sign]
		if !isDigits(digits[sign+1:]) {
			return 0, false
		}
		if digits[sign] == ',' {
			value = strings.Replace(value, ",", ".", 1)
		}
	}
	if !isDigits(whole) {
		return 0, false
	}
	n, err := strconv.ParseFloat(value, 64)
	return n, err == nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func compareNumbers(op Op, answer, value string) bool {
	a, ok := ParseNumber(answer)
	b, ok2 := ParseNumber(value)
	if !ok || !ok2 {
		return false
	}
	switch op {
	case OpEq:
		return a == b
	case OpNe:
		return a != b
	case OpLt:
		return a < b
	case OpLe:
		return a <= b
	case OpGt:
		return a > b
	case OpGe:
		return a >= b
	}
	return false
}

// dayOf is the day a test of a date compares with, or "" when it names none that exists.
func dayOf(t Test, today string) string {
	day := t.Value
	if t.Today {
		day = today
	}
	if !IsDay(day) {
		return ""
	}
	return day
}

// IsDay accepts a calendar date written as YYYY-MM-DD that exists.
func IsDay(value string) bool {
	if len(value) != len(time.DateOnly) {
		return false
	}
	_, err := time.Parse(time.DateOnly, value)
	return err == nil
}

// compareDays compares two days. Days written as YYYY-MM-DD sort as text in the order of the
// calendar, and are days, not moments, so no time zone moves them.
func compareDays(op Op, answer, day string) bool {
	if day == "" || !IsDay(answer) {
		return false
	}
	switch op {
	case OpBefore:
		return answer < day
	case OpAfter:
		return answer > day
	case OpOnOrBefore:
		return answer <= day
	case OpOnOrAfter:
		return answer >= day
	}
	return false
}

// Field is a question a condition may test, as the validator knows it.
type Field struct {
	Kind    Kind     `json:"kind"`
	Options []string `json:"options"`
}

// Schema is what a condition may test where it stands. Fields are the questions it may test;
// Later names the questions it may not, because they are asked at or after it (TPL-421).
// Space says the template is one of all spaces, so a test of the space means something.
type Schema struct {
	Fields map[string]Field
	Later  map[string]bool
	Space  bool
}

// Problem is why a condition is refused: a code the editor translates, and the test it
// concerns, counted from 0, or -1 for the condition as a whole.
type Problem struct {
	Code string `json:"problem"`
	Test int    `json:"test"`
}

// The words of each problem, in English, for an answer of the API. The editor has its own,
// in the translation files, under templates.conditionProblem.
var messages = map[string]string{
	"bad_match":         "Choose all of or any of.",
	"no_tests":          "Add at least one test.",
	"too_many_tests":    "A condition can have at most 10 tests.",
	"bad_subject":       "Test a question, the space or the groups of the person who makes the pages.",
	"unknown_field":     "This question does not exist in the template.",
	"later_field":       "Test only questions asked before this one.",
	"space_not_offered": "Only a template of all spaces can test the space.",
	"bad_op":            "This test cannot be made on this kind of question.",
	"bad_value":         "Give a value that fits this test.",
	"not_an_option":     "Choose one of the options of the question.",
	"group_not_found":   "This group no longer exists. Choose another.",
}

// Message gives the problem in words, naming the test it concerns.
func (p Problem) Message() string {
	msg := messages[p.Code]
	if p.Test >= 0 && p.Code != "too_many_tests" {
		return "Test " + strconv.Itoa(p.Test+1) + ": " + msg
	}
	return msg
}

// Validate refuses a condition its author got wrong, before it is saved (TPL-432): a way to
// combine, no test or more than ten, a subject or operator that does not fit, a question that
// does not exist or comes later, and a value of the wrong kind or no option. Whether a group
// still exists needs the database, so the caller checks that.
func Validate(c Condition, s Schema) *Problem {
	if c.Match != All && c.Match != Any {
		return &Problem{Code: "bad_match", Test: -1}
	}
	if len(c.Tests) == 0 {
		return &Problem{Code: "no_tests", Test: -1}
	}
	if len(c.Tests) > MaxTests {
		return &Problem{Code: "too_many_tests", Test: MaxTests}
	}
	for i, t := range c.Tests {
		if code := checkTest(t, s); code != "" {
			return &Problem{Code: code, Test: i}
		}
	}
	return nil
}

// CheckShape refuses what no template could make valid, where the questions are not known,
// such as in a section of text (TPL-422): a way to combine, no test or more than ten, an
// unknown subject or operator, and values too many, too long or over more than one line.
// Validate checks the rest when the template is saved.
func CheckShape(c Condition) *Problem {
	if c.Match != All && c.Match != Any {
		return &Problem{Code: "bad_match", Test: -1}
	}
	if len(c.Tests) == 0 {
		return &Problem{Code: "no_tests", Test: -1}
	}
	if len(c.Tests) > MaxTests {
		return &Problem{Code: "too_many_tests", Test: MaxTests}
	}
	for i, t := range c.Tests {
		if t.Subject != SubjectField && t.Subject != SubjectSpace && t.Subject != SubjectGroups {
			return &Problem{Code: "bad_subject", Test: i}
		}
		if !knownOp(t.Op) {
			return &Problem{Code: "bad_op", Test: i}
		}
		if len(t.Values) > MaxValues || utf8.RuneCountInString(t.Field) > MaxValueRunes {
			return &Problem{Code: "bad_value", Test: i}
		}
		for _, v := range append([]string{t.Value}, t.Values...) {
			if !utf8.ValidString(v) || strings.ContainsAny(v, "\r\n") || utf8.RuneCountInString(v) > MaxValueRunes {
				return &Problem{Code: "bad_value", Test: i}
			}
		}
	}
	return nil
}

// knownOp says whether any kind has the operator.
func knownOp(op Op) bool {
	for _, ops := range Operators {
		if slices.Contains(ops, op) {
			return true
		}
	}
	return false
}

func checkTest(t Test, s Schema) string {
	var field Field
	switch t.Subject {
	case SubjectField:
		if s.Later[t.Field] {
			return "later_field"
		}
		known, ok := s.Fields[t.Field]
		if !ok {
			return "unknown_field"
		}
		field = known
	case SubjectSpace:
		if !s.Space {
			return "space_not_offered"
		}
		field = Field{Kind: KindSpace}
	case SubjectGroups:
		field = Field{Kind: KindGroups}
	default:
		return "bad_subject"
	}
	if !slices.Contains(Operators[field.Kind], t.Op) {
		return "bad_op"
	}
	return checkValue(t, field)
}

// checkValue holds the value of a test to what its operator compares with.
func checkValue(t Test, f Field) string {
	switch t.Op {
	case OpEmpty, OpNotEmpty, OpYes, OpNo:
		if t.Value != "" || len(t.Values) > 0 || t.Today {
			return "bad_value"
		}
		return ""
	case OpOneOf, OpIncludesAll:
		if t.Value != "" || t.Today || len(t.Values) == 0 || len(t.Values) > MaxValues {
			return "bad_value"
		}
		for i, v := range t.Values {
			if slices.Contains(t.Values[:i], v) {
				return "bad_value"
			}
			if code := checkOne(v, f); code != "" {
				return code
			}
		}
		return ""
	}
	if len(t.Values) > 0 {
		return "bad_value"
	}
	if f.Kind == KindDate {
		if t.Today == (t.Value != "") || (!t.Today && !IsDay(t.Value)) {
			return "bad_value"
		}
		return ""
	}
	if t.Today {
		return "bad_value"
	}
	return checkOne(t.Value, f)
}

// checkOne holds one value to the kind of its subject.
func checkOne(v string, f Field) string {
	if v == "" || !utf8.ValidString(v) || strings.ContainsAny(v, "\r\n") || utf8.RuneCountInString(v) > MaxValueRunes {
		return "bad_value"
	}
	switch f.Kind {
	case KindNumber:
		if _, ok := ParseNumber(v); !ok {
			return "bad_value"
		}
	case KindChoice, KindChoices:
		if !slices.Contains(f.Options, v) {
			return "not_an_option"
		}
	case KindLabel:
		if !isLabel(v) {
			return "bad_value"
		}
	case KindSpace, KindGroups:
		if !isID(v) {
			return "bad_value"
		}
	}
	return ""
}

// isLabel accepts a label written the one way the labels module writes it (ORG-043).
func isLabel(v string) bool {
	if utf8.RuneCountInString(v) > maxLabelRunes || strings.ToLower(v) != v {
		return false
	}
	for _, r := range v {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// isID accepts an id only in the one form the product writes.
func isID(v string) bool {
	if len(v) != len("01938f2e-7b3a-7cde-8f00-0123456789ab") {
		return false
	}
	for i, r := range v {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
				return false
			}
		}
	}
	return true
}

// Fields lists the questions a condition tests, once each, in the order it names them.
func (c Condition) Fields() []string {
	out := []string{}
	for _, t := range c.Tests {
		if t.Subject == SubjectField && !slices.Contains(out, t.Field) {
			out = append(out, t.Field)
		}
	}
	return out
}

// Groups lists the groups a condition tests, once each.
func (c Condition) Groups() []string {
	out := []string{}
	for _, t := range c.Tests {
		if t.Subject == SubjectGroups && t.Value != "" && !slices.Contains(out, t.Value) {
			out = append(out, t.Value)
		}
	}
	return out
}

// RenameField points every test of a question at its new name (TPL-230), and says whether
// anything changed.
func (c *Condition) RenameField(from, to string) bool {
	changed := false
	for i := range c.Tests {
		if c.Tests[i].Subject == SubjectField && c.Tests[i].Field == from {
			c.Tests[i].Field = to
			changed = true
		}
	}
	return changed
}
