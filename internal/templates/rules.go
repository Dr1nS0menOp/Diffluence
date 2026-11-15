// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/conditions"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/labels"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/spaces"
	"github.com/diffluence/diffluence/internal/templates/fields"
)

// Conditions of a template (spec 074, wave GT3): what they may test where they stand, the
// check when a template is saved, and what they decide when a set is made. The server's
// decision is the only one that counts (TPL-1406).

// maxLabelRules is the most label rules a template holds.
const maxLabelRules = 50

// LabelRule adds a label to the page of an entry (TPL-423): the label it names, or the label
// chosen in a question of the kind label, always or when its condition holds.
type LabelRule struct {
	Entry string                `json:"entry"`
	Label string                `json:"label,omitempty"`
	Field string                `json:"field,omitempty"`
	When  *conditions.Condition `json:"when,omitempty"`
}

// askedAt is a question of the form with the step that asks it: an index of the manifest's
// steps, or one past the last for a question in no step, which the form asks last.
type askedAt struct {
	spec fields.Spec
	step int
}

// formOrder lists the questions in the order the form asks them, as the browser groups them
// (stepsOf in web/src/components/template-form.tsx): step by step, each step's questions in
// the order of the texts, and the questions of no step last.
func formOrder(m Manifest, texts map[string]document.Node) []askedAt {
	form := formOf(m, texts)
	out := make([]askedAt, 0, len(form))
	placed := map[string]bool{}
	for i, st := range m.Steps {
		for _, f := range form {
			if slices.Contains(st.Fields, f.Name) && !placed[f.Name] {
				out = append(out, askedAt{spec: f, step: i})
				placed[f.Name] = true
			}
		}
	}
	for _, f := range form {
		if !placed[f.Name] {
			out = append(out, askedAt{spec: f, step: len(m.Steps)})
		}
	}
	return out
}

// schemaOf is what a condition may test: the questions given, with every other question of
// the form named as later, so a test of one is refused as such (TPL-421).
func schemaOf(order []askedAt, allowed func(askedAt) bool, space bool) conditions.Schema {
	s := conditions.Schema{Fields: map[string]conditions.Field{}, Later: map[string]bool{}, Space: space}
	for _, a := range order {
		if allowed(a) {
			s.Fields[a.spec.Name] = conditions.Field{Kind: conditions.Kind(a.spec.Kind), Options: a.spec.Options}
		} else {
			s.Later[a.spec.Name] = true
		}
	}
	return s
}

// The words of the problems a template has with its rules, beside those of a condition.
var ruleMessages = map[string]string{
	"root_condition": "The first page is always made, so it cannot have a condition.",
	"too_many_rules": "A template can have at most 50 label rules.",
	"bad_rule":       "A rule adds one label, fixed or chosen in a label question.",
	"rule_entry":     "This page is not part of the template.",
	"rule_field":     "Choose a question of the kind label.",
}

func ruleProblem(where, code string) apperr.FieldProblem {
	return apperr.FieldProblem{Field: where, Problem: code, Message: ruleMessages[code]}
}

func conditionProblem(where string, p conditions.Problem) apperr.FieldProblem {
	out := apperr.FieldProblem{Field: where, Problem: p.Code, Message: p.Message()}
	if p.Test >= 0 {
		out.Params = map[string]string{"test": strconv.Itoa(p.Test + 1)}
	}
	return out
}

// placedCondition is a condition with the place that holds it, as a problem names it: a
// question by its name, "step:2", "entry:<key>", "section:<key>" or "rule:3".
type placedCondition struct {
	where     string
	condition *conditions.Condition
	schema    conditions.Schema
}

// conditionsOf lists every condition of a version with what it may test where it stands.
func conditionsOf(t Template, m Manifest, texts map[string]document.Node) []placedCondition {
	order := formOrder(m, texts)
	space := t.SpaceID == nil
	position := make(map[string]int, len(order))
	for i, a := range order {
		position[a.spec.Name] = i
	}
	var out []placedCondition
	for i, a := range order {
		if a.spec.Condition != nil {
			earlier := func(b askedAt) bool { return position[b.spec.Name] < i }
			out = append(out, placedCondition{a.spec.Name, a.spec.Condition, schemaOf(order, earlier, space)})
		}
	}
	for k, st := range m.Steps {
		if st.Condition != nil {
			out = append(out, placedCondition{fmt.Sprintf("step:%d", k+1), st.Condition, schemaOf(order, func(b askedAt) bool { return b.step < k }, space)})
		}
	}
	whole := schemaOf(order, func(askedAt) bool { return true }, space)
	for _, e := range m.Entries {
		if e.Condition != nil && e.Key != RootKey {
			out = append(out, placedCondition{"entry:" + e.Key, e.Condition, whole})
		}
		for _, c := range document.ConditionsIn(texts[e.Key]) {
			out = append(out, placedCondition{"section:" + e.Key, &c, whole})
		}
	}
	for i, r := range m.LabelRules {
		if r.When != nil {
			out = append(out, placedCondition{fmt.Sprintf("rule:%d", i+1), r.When, whole})
		}
	}
	return out
}

// checkConditions refuses a version whose conditions or rules do not fit its form, before it
// is saved (TPL-432): every problem is named at once, next to the condition that has it.
func (s *Service) checkConditions(ctx context.Context, q db.Querier, t Template, m Manifest, texts map[string]document.Node) error {
	problems := checkRules(m, texts)
	if root, _, _ := m.entry(RootKey); root.Condition != nil {
		problems = append(problems, ruleProblem("entry:"+RootKey, "root_condition"))
	}
	placed := conditionsOf(t, m, texts)
	type groupTest struct {
		where string
		test  int
	}
	tested := map[string][]groupTest{}
	for _, pc := range placed {
		if p := conditions.Validate(*pc.condition, pc.schema); p != nil {
			problems = append(problems, conditionProblem(pc.where, *p))
			continue
		}
		for i, test := range pc.condition.Tests {
			if test.Subject == conditions.SubjectGroups {
				tested[test.Value] = append(tested[test.Value], groupTest{pc.where, i})
			}
		}
	}
	gone, err := s.missingGroups(ctx, q, groupIDs(tested))
	if err != nil {
		return err
	}
	for _, id := range gone {
		for _, g := range tested[id] {
			problems = append(problems, conditionProblem(g.where, conditions.Problem{Code: "group_not_found", Test: g.test}))
		}
	}
	if len(problems) > 0 {
		return apperr.InvalidFields("invalid_conditions", problems)
	}
	return nil
}

func groupIDs[T any](tested map[string]T) []string {
	out := make([]string, 0, len(tested))
	for id := range tested {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// checkRules holds the label rules to their shape: a page of the template, one label that is
// a label or a question of the kind label, and at most maxLabelRules.
func checkRules(m Manifest, texts map[string]document.Node) []apperr.FieldProblem {
	if len(m.LabelRules) > maxLabelRules {
		return []apperr.FieldProblem{ruleProblem("rules", "too_many_rules")}
	}
	kinds := map[string]fields.Kind{}
	for _, f := range formOf(m, texts) {
		kinds[f.Name] = f.Kind
	}
	var problems []apperr.FieldProblem
	for i, r := range m.LabelRules {
		where := fmt.Sprintf("rule:%d", i+1)
		_, _, found := m.entry(r.Entry)
		switch {
		case !found:
			problems = append(problems, ruleProblem(where, "rule_entry"))
		case (r.Label == "") == (r.Field == ""):
			problems = append(problems, ruleProblem(where, "bad_rule"))
		case r.Label != "":
			if clean, err := labels.Clean(r.Label); err != nil || clean != r.Label {
				problems = append(problems, ruleProblem(where, "bad_rule"))
			}
		case kinds[r.Field] != fields.KindLabel:
			problems = append(problems, ruleProblem(where, "rule_field"))
		}
	}
	return problems
}

// missingGroups lists the groups of ids that no longer exist.
func (s *Service) missingGroups(ctx context.Context, q db.Querier, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	subjects := make([]permissions.Subject, 0, len(ids))
	var gone []string
	for _, id := range ids {
		parsed, err := uuid.Parse(id)
		if err != nil {
			gone = append(gone, id)
			continue
		}
		subjects = append(subjects, permissions.Subject{Type: permissions.SubjectGroup, ID: parsed})
	}
	missing, err := s.perms.MissingSubjects(ctx, q, subjects)
	if err != nil {
		return nil, err
	}
	for _, sub := range missing {
		gone = append(gone, sub.ID.String())
	}
	return gone, nil
}

// groupsTested lists the groups any condition of a version tests, so the creator's groups
// are read only for a template that tests them.
func groupsTested(t Template, m Manifest, texts map[string]document.Node) []string {
	seen := map[string]bool{}
	for _, pc := range conditionsOf(t, m, texts) {
		for _, id := range pc.condition.Groups() {
			seen[id] = true
		}
	}
	return groupIDs(seen)
}

// baseFacts are the facts every condition of a set is decided on, before any answer: the day
// the set is made, the space, and the creator's groups as the permission rule resolves them,
// read only when a condition tests them (TPL-402, TPL-430).
func (s *Service) baseFacts(ctx context.Context, c identity.Caller, p *plan) (conditions.Facts, error) {
	f := conditions.Facts{Answers: map[string]conditions.Answer{}, SpaceID: p.spaceID.String(), Today: p.today}
	tested := groupsTested(p.t, p.version.Manifest, p.texts)
	if len(tested) == 0 {
		return f, nil
	}
	user, err := s.perms.LoadUser(ctx, s.pool, c.UserID)
	if err != nil {
		return f, err
	}
	for _, g := range user.Groups {
		f.Groups = append(f.Groups, g.String())
	}
	f.MissingGroups, err = s.missingGroups(ctx, s.pool, tested)
	return f, err
}

// conditionNames gives today's names of the groups and spaces the conditions of a version
// test, by id, so the editor's sentences name them. A group or space that is gone has none.
func (s *Service) conditionNames(ctx context.Context, q db.Querier, t Template, m Manifest, texts map[string]document.Node) (map[string]string, error) {
	var groups []uuid.UUID
	spaceIDs := map[uuid.UUID]bool{}
	for _, pc := range conditionsOf(t, m, texts) {
		for _, test := range pc.condition.Tests {
			switch test.Subject {
			case conditions.SubjectGroups:
				if id, err := uuid.Parse(test.Value); err == nil {
					groups = append(groups, id)
				}
			case conditions.SubjectSpace:
				for _, v := range test.Values {
					if id, err := uuid.Parse(v); err == nil {
						spaceIDs[id] = true
					}
				}
			}
		}
	}
	out := map[string]string{}
	known, err := identity.DisplayNames(ctx, q, groups)
	if err != nil {
		return nil, err
	}
	for id, name := range known {
		out[id.String()] = name
	}
	for id := range spaceIDs {
		name, err := spaces.Name(ctx, q, id)
		if apperr.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[id.String()] = name
	}
	return out, nil
}

// labelsOf gives the labels of an entry's page: its own, and those its rules add (TPL-423).
func labelsOf(e Entry, rules []LabelRule, facts conditions.Facts) []string {
	out := slices.Clone(e.Labels)
	if out == nil {
		out = []string{}
	}
	for _, r := range rules {
		if r.Entry != e.Key || !conditions.Holds(r.When, facts) {
			continue
		}
		label := r.Label
		if r.Field != "" {
			label = facts.Answers[r.Field].Value
		}
		if label != "" && !slices.Contains(out, label) {
			out = append(out, label)
		}
	}
	return out
}

// filesLeftOut lists the files the text of a template shows that the page made from it no
// longer shows, because a section that held them was removed (TPL-422).
func filesLeftOut(template, made document.Node) []uuid.UUID {
	kept := document.FilesShown(made)
	var out []uuid.UUID
	for _, id := range document.FilesShown(template) {
		if !slices.Contains(kept, id) {
			out = append(out, id)
		}
	}
	return out
}

// SetEntryCondition gives an entry of a template its condition, or takes it away with nil,
// as the template's next version (TPL-420).
func (s *Service) SetEntryCondition(ctx context.Context, c identity.Caller, id uuid.UUID, key string, condition *conditions.Condition) error {
	_, err := s.editTemplate(ctx, c, id, func(_ pgx.Tx, _ Template, m Manifest) (Manifest, error) {
		_, i, found := m.entry(key)
		if !found {
			return m, apperr.NotFound()
		}
		m.Entries = slices.Clone(m.Entries)
		m.Entries[i].Condition = condition
		return m, nil
	})
	return err
}

// RulesInput is the label rules of a template, from the Rules section.
type RulesInput struct {
	LabelRules []LabelRule `json:"label_rules"`
}

// SetLabelRules saves the label rules of a template as its next version (TPL-423, TPL-1010).
func (s *Service) SetLabelRules(ctx context.Context, c identity.Caller, id uuid.UUID, in RulesInput) ([]LabelRule, error) {
	rules := slices.Clone(in.LabelRules)
	for i := range rules {
		// A label is written the one way labels are; one that cannot be is left for the check.
		if clean, err := labels.Clean(rules[i].Label); err == nil && rules[i].Label != "" {
			rules[i].Label = clean
		}
	}
	if len(rules) == 0 {
		rules = nil
	}
	m, err := s.editTemplate(ctx, c, id, func(_ pgx.Tx, _ Template, m Manifest) (Manifest, error) {
		m.LabelRules = rules
		return m, nil
	})
	if m.LabelRules == nil {
		return []LabelRule{}, err
	}
	return m.LabelRules, err
}

// RenameInput renames a question of a template.
type RenameInput struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func renameRefused(name, code string) error {
	messages := map[string]string{
		"bad_name":      "A question needs a name of one line, at most 100 bytes.",
		"name_taken":    "Another question has this name, ignoring capitals. Choose another.",
		"reserved_name": "This name is kept for a value the template knows. Choose another name.",
		"not_asked":     "This template does not ask this question.",
	}
	return apperr.InvalidFields("invalid_rename", []apperr.FieldProblem{{Field: name, Problem: code, Message: messages[code]}})
}

// maxFieldNameBytes is the longest name of a question, as the text holds it.
const maxFieldNameBytes = 100

// RenameField renames a question everywhere a template uses it, as one change and one new
// version (TPL-230): where each text asks it, in the sections of the texts, in the title
// patterns, in its settings and its step, and in every condition and rule.
func (s *Service) RenameField(ctx context.Context, c identity.Caller, id uuid.UUID, in RenameInput) error {
	from, to := in.From, strings.TrimSpace(in.To)
	if to == "" || len(to) > maxFieldNameBytes || strings.ContainsAny(to, "\r\n{}") {
		return renameRefused(from, "bad_name")
	}
	_, err := s.editTemplate(ctx, c, id, func(tx pgx.Tx, t Template, m Manifest) (Manifest, error) {
		texts, _, err := s.textsOf(ctx, tx, m)
		if err != nil {
			return m, err
		}
		asked := false
		for _, f := range formOf(m, texts) {
			if f.Name == from {
				asked = true
			} else if strings.EqualFold(f.Name, to) {
				return m, renameRefused(from, "name_taken")
			}
		}
		switch {
		case !asked:
			return m, renameRefused(from, "not_asked")
		case slices.Contains(reservedNames, strings.ToLower(to)):
			return m, renameRefused(from, "reserved_name")
		}
		m.Entries = slices.Clone(m.Entries)
		for i := range m.Entries {
			if err := s.renameInEntry(ctx, tx, c, t, &m.Entries[i], texts, from, to); err != nil {
				return m, err
			}
		}
		m.Fields = renameInSpecs(m.Fields, from, to)
		m.Steps = renameInSteps(m.Steps, from, to)
		m.LabelRules = renameInRules(m.LabelRules, from, to)
		return m, saveRowSteps(ctx, tx, t.ID, m.Steps)
	})
	return err
}

// renameInEntry renames a question in an entry's title pattern, its condition, and its text,
// which gets a new version of its hidden page when the question is in it.
func (s *Service) renameInEntry(ctx context.Context, tx pgx.Tx, c identity.Caller, t Template, e *Entry, texts map[string]document.Node, from, to string) error {
	e.TitlePattern = strings.ReplaceAll(e.TitlePattern, "{"+from+"}", "{"+to+"}")
	e.Condition = renamedCondition(e.Condition, from, to)
	if e.PageID == nil {
		return nil
	}
	doc, changed := document.RenameField(texts[e.Key], from, to)
	if !changed {
		return nil
	}
	title := e.Name
	if e.Key == RootKey {
		title = t.Name
	}
	if err := s.pages.NewTemplateVersion(ctx, tx, c, *e.PageID, title, doc); err != nil {
		return err
	}
	heads, err := s.pages.TemplateHeads(ctx, tx, []uuid.UUID{*e.PageID})
	if err != nil {
		return err
	}
	e.PageVersion = heads[*e.PageID]
	return nil
}

func renamedCondition(c *conditions.Condition, from, to string) *conditions.Condition {
	if c == nil {
		return nil
	}
	renamed := *c
	renamed.Tests = slices.Clone(c.Tests)
	renamed.RenameField(from, to)
	return &renamed
}

func renameInSpecs(specs []fields.Spec, from, to string) []fields.Spec {
	out := slices.Clone(specs)
	for i := range out {
		if out[i].Name == from {
			out[i].Name = to
		}
		out[i].Condition = renamedCondition(out[i].Condition, from, to)
	}
	return out
}

func renameInSteps(steps []Step, from, to string) []Step {
	out := slices.Clone(steps)
	for i := range out {
		names := slices.Clone(out[i].Fields)
		for j := range names {
			if names[j] == from {
				names[j] = to
			}
		}
		out[i].Fields = names
		out[i].Condition = renamedCondition(out[i].Condition, from, to)
	}
	return out
}

func renameInRules(rules []LabelRule, from, to string) []LabelRule {
	out := slices.Clone(rules)
	for i := range out {
		if out[i].Field == from {
			out[i].Field = to
		}
		out[i].When = renamedCondition(out[i].When, from, to)
	}
	return out
}
