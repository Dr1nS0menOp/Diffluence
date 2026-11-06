// SPDX-License-Identifier: Apache-2.0

package automation

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestConditionTreesCombine(t *testing.T) {
	now := time.Now()
	f := facts{labels: []string{"handbook"}, title: "Team Plan", createdAt: now.Add(-48 * time.Hour)}
	label := Condition{Kind: CondLabel, Value: "Handbook"}
	other := Condition{Kind: CondLabel, Value: "other"}
	cases := []struct {
		name string
		c    Condition
		want bool
	}{
		{"all passes", Condition{Kind: CondAll, Items: []Condition{label, {Kind: CondTitle, Value: "plan"}}}, true},
		{"all fails on one", Condition{Kind: CondAll, Items: []Condition{label, other}}, false},
		{"any passes on one", Condition{Kind: CondAny, Items: []Condition{other, label}}, true},
		{"any fails on none", Condition{Kind: CondAny, Items: []Condition{other}}, false},
		{"not turns around", Condition{Kind: CondNot, Items: []Condition{other}}, true},
		{"older than a day", Condition{Kind: CondOlder, Days: 1}, true},
		{"not older than a week", Condition{Kind: CondOlder, Days: 7}, false},
		{"an unknown kind never passes", Condition{Kind: "x"}, false},
	}
	for _, c := range cases {
		if got := c.c.holds(f, now); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
	var none *Condition
	if !none.holds(f, now) {
		t.Error("no conditions must pass")
	}
}

func TestAPlaceholderIsPlainText(t *testing.T) {
	values := map[string]string{"page.title": "{rule.name} <b>", "rule.name": "secret"}
	if got := expand("See {page.title} {unknown}", values); got != "See {rule.name} <b> {unknown}" {
		t.Fatalf("got %q", got)
	}
}

func TestAValueCannotChangeTheShapeOfAJSONBody(t *testing.T) {
	got, err := expandJSON(`{"text":"{page.title}"}`, map[string]string{"page.title": `x","admin":"true`})
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"text":"x\",\"admin\":\"true"}` {
		t.Fatalf("got %s", got)
	}
	if _, err := expandJSON(`{"text":`, nil); err == nil {
		t.Fatal("a broken body passed")
	}
}

func TestProblemsNameTheStep(t *testing.T) {
	d := Definition{Trigger: Trigger{Kind: TriggerSchedule, Every: EveryDay, At: "09:00"},
		Actions: []Action{{Type: ActAddLabel, Label: "old"}, {Type: ActComment}}}
	got := d.Problems()
	if len(got) != 2 || got[0].Step != 0 || got[1].Step != 1 {
		t.Fatalf("got %+v", got)
	}
	d.Actions = append([]Action{{Type: ActFind}}, Action{Type: ActAddLabel, Label: "old"})
	if got := d.Problems(); len(got) != 0 {
		t.Fatalf("a Find step gives later steps a page: %+v", got)
	}
}

func TestALoopOfConditionsHasAnEnd(t *testing.T) {
	c := Condition{Kind: CondLabel, Value: "x"}
	for range maxCondDepth + 2 {
		c = Condition{Kind: CondNot, Items: []Condition{c}}
	}
	if got := (Definition{Trigger: Trigger{Kind: TriggerManual}, Conditions: &c,
		Actions: []Action{{Type: ActArchive}}}).Problems(); len(got) != 1 || got[0].Step != stepConditions {
		t.Fatalf("got %+v", got)
	}
}

func TestSchedulesGiveTheNextMoment(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 30, 0, 0, time.UTC)
	cases := []struct {
		t    Trigger
		want time.Time
	}{
		{Trigger{Every: EveryHour}, time.Date(2026, 3, 10, 13, 0, 0, 0, time.UTC)},
		{Trigger{Every: EveryDay, At: "13:00"}, time.Date(2026, 3, 10, 13, 0, 0, 0, time.UTC)},
		{Trigger{Every: EveryDay, At: "09:00"}, time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC)},
		{Trigger{Every: EveryWeek, At: "09:00"}, time.Date(2026, 3, 17, 9, 0, 0, 0, time.UTC)},
		{Trigger{Every: EveryMonth, At: "09:00"}, time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		if got := nextRun(c.t, now); !got.Equal(c.want) {
			t.Errorf("%+v: %s", c.t, got)
		}
	}
}

func TestEventsSortToTriggers(t *testing.T) {
	cases := []struct {
		event   string
		payload map[string]any
		want    []string
	}{
		{"page.published", map[string]any{"version": float64(1)}, []string{"page.published", "blog.published"}},
		{"page.published", map[string]any{"version": float64(2)}, []string{"page.edited"}},
		{"page.published", map[string]any{"version": float64(2), "mentions": []any{uuid.NewString()}, "assigned": []any{uuid.NewString()}},
			[]string{"page.edited", "person.mentioned", "task.created"}},
		{"comment.created", map[string]any{}, []string{"page.commented", "blog.commented"}},
		{"page.labelled", map[string]any{}, []string{"page.labelled", "blog.labelled"}},
		{"task.done", map[string]any{}, []string{"task.ticked"}},
		{"page.renamed", map[string]any{}, nil},
	}
	for _, c := range cases {
		got := triggersFor(c.event, c.payload)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v", c.event, got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v", c.event, got)
			}
		}
	}
}
