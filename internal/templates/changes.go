// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/diffluence/diffluence/internal/conditions"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/templates/fields"
)

// The readable change list of a version (spec 074, TPL-602): what changed besides the texts,
// one line per change, for approvers and auditors. Each line is a code the browser translates
// with the names it carries; names are the author's words, never an answer or an id.

// The codes of the change list.
const (
	ChangeEntryAdded        = "entry_added"
	ChangeEntryRemoved      = "entry_removed"
	ChangeEntryRenamed      = "entry_renamed"
	ChangeEntryMoved        = "entry_moved"
	ChangeEntryReordered    = "entry_reordered"
	ChangeEntryTitle        = "entry_title"
	ChangeEntryLabels       = "entry_labels"
	ChangeEntryRestrictions = "entry_restrictions"
	ChangeEntryWorkflow     = "entry_workflow"
	ChangeEntryCondition    = "entry_condition"
	ChangeEntryNoCondition  = "entry_no_condition"
	ChangeFieldAdded        = "field_added"
	ChangeFieldRemoved      = "field_removed"
	ChangeFieldKind         = "field_kind"
	ChangeFieldRequired     = "field_required"
	ChangeFieldOptional     = "field_optional"
	ChangeFieldSettings     = "field_settings"
	ChangeFieldCondition    = "field_condition"
	ChangeFieldNoCondition  = "field_no_condition"
	ChangeStepAdded         = "step_added"
	ChangeStepRemoved       = "step_removed"
	ChangeStepQuestions     = "step_questions"
	ChangeStepCondition     = "step_condition"
	ChangeStepNoCondition   = "step_no_condition"
	ChangeStepsReordered    = "steps_reordered"
	ChangePublishOn         = "publish_on"
	ChangePublishOff        = "publish_off"
	ChangeDefaultParent     = "default_parent"
	ChangeRuleAdded         = "rule_added"
	ChangeRuleRemoved       = "rule_removed"
)

// maxChangeLines bounds the list an audit event keeps; the screen reads the whole list.
const maxChangeLines = 50

// SettingChange is one line of the change list. Entry, Field and Step name what it is about;
// Value carries the words the line needs, such as a former name, a kind or a title pattern.
type SettingChange struct {
	Code      string                `json:"code"`
	Entry     string                `json:"entry,omitempty"`
	Field     string                `json:"field,omitempty"`
	Step      string                `json:"step,omitempty"`
	Value     string                `json:"value,omitempty"`
	Required  bool                  `json:"required,omitempty"`
	Condition *conditions.Condition `json:"condition,omitempty"`
}

// sameJSON says two values marshal alike, which is how manifests are compared everywhere.
func sameJSON(a, b any) bool {
	ra, errA := json.Marshal(a)
	rb, errB := json.Marshal(b)
	// Both are plain data of this package, which always marshals.
	return errA == nil && errB == nil && string(ra) == string(rb)
}

// describeChanges lists what after changed against before, besides the texts of its pages:
// the pages, the questions, the steps, the rules and where the pages go.
func describeChanges(before, after Manifest, textsBefore, textsAfter map[string]document.Node) []SettingChange {
	out := entryChanges(before, after)
	out = append(out, fieldChanges(formOf(before, textsBefore), formOf(after, textsAfter))...)
	out = append(out, stepChanges(before.Steps, after.Steps)...)
	out = append(out, ruleChanges(before, after)...)
	if before.Publish != after.Publish {
		code := ChangePublishOff
		if after.Publish {
			code = ChangePublishOn
		}
		out = append(out, SettingChange{Code: code})
	}
	if !sameJSON(before.DefaultParent, after.DefaultParent) {
		out = append(out, SettingChange{Code: ChangeDefaultParent})
	}
	if out == nil {
		out = []SettingChange{}
	}
	return out
}

// nameOf names an entry as the change list shows it: the first page has no name of its own.
func nameOf(m Manifest, key string) string {
	if e, _, ok := m.entry(key); ok {
		return e.Name
	}
	return ""
}

// siblingsOf lists the keys below one parent, in the template's order.
func siblingsOf(m Manifest, parent string) []string {
	var out []string
	for _, e := range m.Entries {
		if e.Key != RootKey && e.Parent == parent {
			out = append(out, e.Key)
		}
	}
	return out
}

func entryChanges(before, after Manifest) []SettingChange {
	var out []SettingChange
	for _, e := range after.ordered() {
		old, _, existed := before.entry(e.Key)
		if !existed {
			// Version 1 is compared with nothing: only its pages beyond the first are news.
			if e.Key != RootKey {
				out = append(out, SettingChange{Code: ChangeEntryAdded, Entry: e.Name, Value: e.Kind})
			}
			continue
		}
		out = append(out, oneEntryChanges(after, old, e)...)
	}
	for _, e := range before.ordered() {
		if _, _, kept := after.entry(e.Key); !kept {
			out = append(out, SettingChange{Code: ChangeEntryRemoved, Entry: e.Name})
		}
	}
	return append(out, reorderings(before, after)...)
}

// reorderings names each page whose pages below it stand in another order, once, looking only
// at the pages both versions have below it.
func reorderings(before, after Manifest) []SettingChange {
	var out []SettingChange
	// below keeps the keys of list that sit below parent in other as well.
	below := func(list []string, other Manifest, parent string) []string {
		return slices.DeleteFunc(list, func(k string) bool {
			e, _, ok := other.entry(k)
			return !ok || e.Parent != parent
		})
	}
	for _, parent := range after.ordered() {
		was := below(siblingsOf(before, parent.Key), after, parent.Key)
		now := below(siblingsOf(after, parent.Key), before, parent.Key)
		if !slices.Equal(was, now) {
			out = append(out, SettingChange{Code: ChangeEntryReordered, Entry: parent.Name})
		}
	}
	return out
}

func oneEntryChanges(after Manifest, old, e Entry) []SettingChange {
	var out []SettingChange
	line := func(code, value string) {
		out = append(out, SettingChange{Code: code, Entry: e.Name, Value: value})
	}
	if old.Name != e.Name {
		line(ChangeEntryRenamed, old.Name)
	}
	if old.Parent != e.Parent {
		line(ChangeEntryMoved, nameOf(after, e.Parent))
	}
	if old.TitlePattern != e.TitlePattern {
		line(ChangeEntryTitle, e.TitlePattern)
	}
	if !sameJSON(old.Labels, e.Labels) {
		line(ChangeEntryLabels, strings.Join(e.Labels, ", "))
	}
	// Who may see a page are people and groups of this installation: the line says that the
	// list changed, and the comparison of the Pages section shows to whom.
	if !sameJSON(old.Restrictions, e.Restrictions) {
		line(ChangeEntryRestrictions, "")
	}
	if !sameJSON(old.WorkflowID, e.WorkflowID) {
		line(ChangeEntryWorkflow, "")
	}
	if !sameJSON(old.Condition, e.Condition) {
		if e.Condition == nil {
			line(ChangeEntryNoCondition, "")
		} else {
			out = append(out, SettingChange{Code: ChangeEntryCondition, Entry: e.Name, Condition: e.Condition})
		}
	}
	return out
}

func fieldChanges(before, after []fields.Spec) []SettingChange {
	var out []SettingChange
	find := func(list []fields.Spec, name string) (fields.Spec, bool) {
		i := slices.IndexFunc(list, func(s fields.Spec) bool { return s.Name == name })
		if i < 0 {
			return fields.Spec{}, false
		}
		return list[i], true
	}
	for _, f := range after {
		old, existed := find(before, f.Name)
		if !existed {
			out = append(out, SettingChange{Code: ChangeFieldAdded, Field: f.Name, Value: string(f.Kind), Required: f.Required,
				Condition: f.Condition})
			continue
		}
		out = append(out, oneFieldChanges(old, f)...)
	}
	for _, f := range before {
		if _, kept := find(after, f.Name); !kept {
			out = append(out, SettingChange{Code: ChangeFieldRemoved, Field: f.Name})
		}
	}
	return out
}

func oneFieldChanges(old, f fields.Spec) []SettingChange {
	var out []SettingChange
	if old.Kind != f.Kind {
		out = append(out, SettingChange{Code: ChangeFieldKind, Field: f.Name, Value: string(f.Kind)})
	}
	if old.Required != f.Required {
		code := ChangeFieldOptional
		if f.Required {
			code = ChangeFieldRequired
		}
		out = append(out, SettingChange{Code: code, Field: f.Name})
	}
	if !sameJSON(old.Condition, f.Condition) {
		if f.Condition == nil {
			out = append(out, SettingChange{Code: ChangeFieldNoCondition, Field: f.Name})
		} else {
			out = append(out, SettingChange{Code: ChangeFieldCondition, Field: f.Name, Condition: f.Condition})
		}
	}
	// Everything else a question sets (help, default, bounds, options) is one line.
	rest := func(s fields.Spec) fields.Spec {
		s.Kind, s.Required, s.Condition, s.DefaultGone = "", false, nil, false
		return s
	}
	if !sameJSON(rest(old), rest(f)) {
		out = append(out, SettingChange{Code: ChangeFieldSettings, Field: f.Name})
	}
	return out
}

// stepChanges matches steps by their title, which is how people tell them apart.
func stepChanges(before, after []Step) []SettingChange {
	var out []SettingChange
	titles := func(list []Step) []string {
		names := make([]string, 0, len(list))
		for _, st := range list {
			names = append(names, st.Title)
		}
		return names
	}
	wasTitles, nowTitles := titles(before), titles(after)
	for _, st := range after {
		i := slices.Index(wasTitles, st.Title)
		if i < 0 {
			out = append(out, SettingChange{Code: ChangeStepAdded, Step: st.Title})
			continue
		}
		old := before[i]
		if !sameJSON(old.Fields, st.Fields) {
			out = append(out, SettingChange{Code: ChangeStepQuestions, Step: st.Title, Value: strings.Join(st.Fields, ", ")})
		}
		if !sameJSON(old.Condition, st.Condition) {
			if st.Condition == nil {
				out = append(out, SettingChange{Code: ChangeStepNoCondition, Step: st.Title})
			} else {
				out = append(out, SettingChange{Code: ChangeStepCondition, Step: st.Title, Condition: st.Condition})
			}
		}
	}
	for _, st := range before {
		if !slices.Contains(nowTitles, st.Title) {
			out = append(out, SettingChange{Code: ChangeStepRemoved, Step: st.Title})
		}
	}
	common := func(list, other []string) []string {
		return slices.DeleteFunc(slices.Clone(list), func(title string) bool { return !slices.Contains(other, title) })
	}
	if !slices.Equal(common(wasTitles, nowTitles), common(nowTitles, wasTitles)) {
		out = append(out, SettingChange{Code: ChangeStepsReordered})
	}
	return out
}

// ruleChanges compares the label rules as a list: a rule is its page, its label or question,
// and its condition, so a changed rule is one removed and one added.
func ruleChanges(before, after Manifest) []SettingChange {
	var out []SettingChange
	describe := func(m Manifest, r LabelRule, code string) SettingChange {
		value := r.Label
		if r.Field != "" {
			value = r.Field
		}
		return SettingChange{Code: code, Entry: nameOf(m, r.Entry), Value: value, Condition: r.When}
	}
	has := func(list []LabelRule, r LabelRule) bool {
		return slices.ContainsFunc(list, func(o LabelRule) bool { return sameJSON(o, r) })
	}
	for _, r := range after.LabelRules {
		if !has(before.LabelRules, r) {
			out = append(out, describe(after, r, ChangeRuleAdded))
		}
	}
	for _, r := range before.LabelRules {
		if !has(after.LabelRules, r) {
			out = append(out, describe(before, r, ChangeRuleRemoved))
		}
	}
	return out
}

// changeCodes gives the lines of a change list as the audit keeps them: each code with the
// name it is about, at most maxChangeLines of them (TPL-711).
func changeCodes(list []SettingChange) []string {
	out := make([]string, 0, min(len(list), maxChangeLines))
	for _, c := range list[:min(len(list), maxChangeLines)] {
		name := c.Entry + c.Field + c.Step
		if name == "" {
			out = append(out, c.Code)
			continue
		}
		out = append(out, c.Code+": "+name)
	}
	return out
}
