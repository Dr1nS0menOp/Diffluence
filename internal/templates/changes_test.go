// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/conditions"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/templates/fields"
)

// Spec 074, wave GT5, TPL-602: the readable change list names every kind of change.

func questionText(names ...string) document.Node {
	var content []document.Node
	for _, n := range names {
		content = append(content, document.Node{Type: document.TypeTemplateField, Attrs: map[string]any{"name": n, "kind": "line"}})
	}
	return document.Node{Type: "doc", Content: []document.Node{{Type: "paragraph", Content: content}}}
}

func baseManifest() Manifest {
	page := uuid.New()
	m := Manifest{Entries: []Entry{
		{Key: RootKey, Kind: EntryPage, PageID: &page, PageVersion: 1},
		{Key: "a", Name: "Equipment", Kind: EntryPage, Parent: RootKey, PageID: &page, PageVersion: 1},
		{Key: "b", Name: "Access", Kind: EntryPage, Parent: RootKey, PageID: &page, PageVersion: 1},
	}, Steps: []Step{{Title: "About you", Fields: []string{"Team"}}, {Title: "Work", Fields: []string{}}}}
	m.normalise()
	return m
}

func codesOf(list []SettingChange) []string {
	out := []string{}
	for _, c := range list {
		out = append(out, c.Code)
	}
	return out
}

func TestTheChangeListNamesEveryKindOfChange(t *testing.T) {
	onSite := &conditions.Condition{Match: conditions.All, Tests: []conditions.Test{
		{Subject: conditions.SubjectField, Field: "Works on site", Op: conditions.OpNo}}}
	texts := map[string]document.Node{RootKey: questionText("Team", "Works on site")}
	cases := []struct {
		name   string
		change func(m *Manifest, texts map[string]document.Node)
		want   string
	}{
		{"an entry added", func(m *Manifest, _ map[string]document.Node) {
			m.Entries = append(m.Entries, Entry{Key: "c", Name: "Desk", Kind: EntryFolder, Parent: RootKey})
		}, ChangeEntryAdded},
		{"an entry removed", func(m *Manifest, _ map[string]document.Node) { m.Entries = m.Entries[:2] }, ChangeEntryRemoved},
		{"an entry renamed", func(m *Manifest, _ map[string]document.Node) { m.Entries[1].Name = "Gear" }, ChangeEntryRenamed},
		{"an entry moved", func(m *Manifest, _ map[string]document.Node) { m.Entries[2].Parent = "a" }, ChangeEntryMoved},
		{"entries reordered", func(m *Manifest, _ map[string]document.Node) {
			m.Entries[1], m.Entries[2] = m.Entries[2], m.Entries[1]
		}, ChangeEntryReordered},
		{"a title pattern", func(m *Manifest, _ map[string]document.Node) { m.Entries[1].TitlePattern = "{Team} gear" }, ChangeEntryTitle},
		{"labels", func(m *Manifest, _ map[string]document.Node) { m.Entries[1].Labels = []string{"hr"} }, ChangeEntryLabels},
		{"a restriction", func(m *Manifest, _ map[string]document.Node) {
			m.Entries[1].Restrictions.View = []permissions.Subject{{Type: permissions.SubjectGroup, ID: uuid.New()}}
		}, ChangeEntryRestrictions},
		{"a workflow", func(m *Manifest, _ map[string]document.Node) { id := uuid.New(); m.Entries[1].WorkflowID = &id }, ChangeEntryWorkflow},
		{"an entry's condition", func(m *Manifest, _ map[string]document.Node) { m.Entries[1].Condition = onSite }, ChangeEntryCondition},
		{"a question added", func(_ *Manifest, texts map[string]document.Node) {
			texts[RootKey] = questionText("Team", "Works on site", "Start date")
		}, ChangeFieldAdded},
		{"a question removed", func(_ *Manifest, texts map[string]document.Node) { texts[RootKey] = questionText("Team") }, ChangeFieldRemoved},
		{"a question's kind", func(m *Manifest, _ map[string]document.Node) {
			m.Fields = []fields.Spec{{Name: "Team", Kind: fields.KindText}}
		}, ChangeFieldKind},
		{"a question made required", func(m *Manifest, _ map[string]document.Node) {
			m.Fields = []fields.Spec{{Name: "Team", Kind: fields.KindLine, Required: true}}
		}, ChangeFieldRequired},
		{"a question's help", func(m *Manifest, _ map[string]document.Node) {
			m.Fields = []fields.Spec{{Name: "Team", Kind: fields.KindLine, Help: "Your team"}}
		}, ChangeFieldSettings},
		{"a question's condition", func(m *Manifest, _ map[string]document.Node) {
			m.Fields = []fields.Spec{{Name: "Team", Kind: fields.KindLine, Condition: onSite}}
		}, ChangeFieldCondition},
		{"a step added", func(m *Manifest, _ map[string]document.Node) {
			m.Steps = append(m.Steps, Step{Title: "Done", Fields: []string{}})
		}, ChangeStepAdded},
		{"a step removed", func(m *Manifest, _ map[string]document.Node) { m.Steps = m.Steps[:1] }, ChangeStepRemoved},
		{"a step's questions", func(m *Manifest, _ map[string]document.Node) { m.Steps[1].Fields = []string{"Works on site"} }, ChangeStepQuestions},
		{"a step's condition", func(m *Manifest, _ map[string]document.Node) { m.Steps[1].Condition = onSite }, ChangeStepCondition},
		{"steps reordered", func(m *Manifest, _ map[string]document.Node) { m.Steps[0], m.Steps[1] = m.Steps[1], m.Steps[0] }, ChangeStepsReordered},
		{"publishing at once", func(m *Manifest, _ map[string]document.Node) { m.Publish = true }, ChangePublishOn},
		{"the default parent", func(m *Manifest, _ map[string]document.Node) { id := uuid.New(); m.DefaultParent = &id }, ChangeDefaultParent},
		{"a label rule", func(m *Manifest, _ map[string]document.Node) {
			m.LabelRules = []LabelRule{{Entry: "a", Label: "laptop", When: onSite}}
		}, ChangeRuleAdded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := baseManifest()
			after := baseManifest()
			after.Entries[0].PageID, after.Entries[1].PageID, after.Entries[2].PageID = before.Entries[0].PageID, before.Entries[1].PageID, before.Entries[2].PageID
			textsAfter := map[string]document.Node{RootKey: texts[RootKey]}
			tc.change(&after, textsAfter)
			after.normalise()
			got := codesOf(describeChanges(before, after, texts, textsAfter))
			if !slices.Contains(got, tc.want) {
				t.Fatalf("want %s in %v", tc.want, got)
			}
			if len(got) != 1 {
				t.Fatalf("one change gave %v", got)
			}
		})
	}
}

func TestAnUnchangedVersionHasAnEmptyChangeList(t *testing.T) {
	texts := map[string]document.Node{RootKey: questionText("Team")}
	if got := describeChanges(baseManifest(), baseManifest(), texts, texts); len(got) != 0 {
		t.Fatalf("nothing changed, yet: %v", got)
	}
}

func TestTheChangeListKeepsTheWordsItNeeds(t *testing.T) {
	before, after := baseManifest(), baseManifest()
	after.Entries[1].Name = "Gear"
	m := Manifest{Fields: []fields.Spec{{Name: "Team", Kind: fields.KindLine, Required: true}}}
	got := describeChanges(before, after, nil, nil)
	if len(got) != 1 || got[0].Entry != "Gear" || got[0].Value != "Equipment" {
		t.Fatalf("a rename names both: %+v", got)
	}
	added := fieldChanges(nil, formOf(Manifest{Entries: []Entry{{Key: RootKey, Kind: EntryPage}}, Fields: m.Fields},
		map[string]document.Node{RootKey: questionText("Team")}))
	if len(added) != 1 || !added[0].Required || added[0].Value != string(fields.KindLine) {
		t.Fatalf("an added question says its kind and that it is required: %+v", added)
	}
	if codes := changeCodes(got); len(codes) != 1 || codes[0] != "entry_renamed: Gear" {
		t.Fatalf("the audit keeps the code and the name: %v", codes)
	}
}
