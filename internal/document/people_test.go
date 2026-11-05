// SPDX-License-Identifier: Apache-2.0

package document

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

const anna = "018f0000-0000-7000-8000-00000000000a"

func mentionJSON(id, kind, label string) string {
	return `{"type":"mention","attrs":{"id":"` + id + `","kind":"` + kind + `","label":"` + label + `"}}`
}

func TestAMentionIsCheckedLikeAnythingElse(t *testing.T) {
	good := doc(`{"type":"paragraph","content":[` + mentionJSON(anna, "user", "Anna") + `]}`)
	n, err := Normalize(good)
	if err != nil {
		t.Fatal(err)
	}
	if got := MentionedUsers(n); len(got) != 1 || got[0].String() != anna {
		t.Fatalf("mentions: %v", got)
	}
	FillMentionNames(&n, map[uuid.UUID]string{uuid.MustParse(anna): "Anna"})
	if got := Text(n); !strings.Contains(got, "@Anna") {
		t.Fatalf("text: %q", got)
	}
	if got := ToMarkdown(n, nil); !strings.Contains(got, "@Anna") {
		t.Fatalf("markdown: %q", got)
	}
	for name, content := range map[string]string{
		"no id":  `{"type":"paragraph","content":[{"type":"mention","attrs":{"label":"x"}}]}`,
		"bad id": `{"type":"paragraph","content":[{"type":"mention","attrs":{"id":"../x"}}]}`,
	} {
		if _, err := Normalize(doc(content)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	unknownKind, err := Normalize(doc(`{"type":"paragraph","content":[` + mentionJSON(anna, "everyone", "x") + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(MentionedGroups(unknownKind)) != 0 || len(MentionedUsers(unknownKind)) != 1 {
		t.Fatal("an unknown kind must fall back to a person, never to a group")
	}
}

func TestAStoredMentionHoldsNoName(t *testing.T) {
	n, err := Normalize(doc(`{"type":"paragraph","content":[` + mentionJSON(anna, "user", "Anna Visser") + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Anna") || strings.Contains(string(raw), "label") {
		t.Fatalf("a version cannot be changed, so it must not hold a name: %s", raw)
	}
	if got := Text(n); strings.Contains(got, "@") {
		t.Fatalf("search text: %q", got)
	}
}

func TestAQuoteIsLookedForInTheWordsOfADocument(t *testing.T) {
	line := `{"type":"text","text":"Hello "},` + mentionJSON(anna, "user", "Anna") + `,{"type":"text","text":" and welcome"}`
	n, err := Normalize(doc(`{"type":"paragraph","content":[` + line + `]},{"type":"paragraph","content":[{"type":"text","text":"Second block"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, quote := range []string{"Hello", "and welcome", "welcome\nSecond", "Hello ￼ and"} {
		if !HoldsQuote(n, quote) {
			t.Errorf("%q must be found", quote)
		}
	}
	for _, quote := range []string{"", "\n", "never written", "Anna"} {
		if HoldsQuote(n, quote) {
			t.Errorf("%q must not be found", quote)
		}
	}
}

func TestAMentionFollowsARename(t *testing.T) {
	n, err := Normalize(doc(`{"type":"paragraph","content":[` + mentionJSON(anna, "user", "Old name") + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	FillMentionNames(&n, map[uuid.UUID]string{uuid.MustParse(anna): "New name"})
	if !strings.Contains(Text(n), "@New name") {
		t.Fatalf("text: %q", Text(n))
	}
}

func taskList(items ...string) string {
	return `{"type":"taskList","content":[` + strings.Join(items, ",") + `]}`
}

func taskItem(attrs, line string) string {
	return `{"type":"taskItem","attrs":{` + attrs + `},"content":[{"type":"paragraph","content":[` + line + `]}]}`
}

func TestATaskTakesItsPersonAndDateFromItsOwnLine(t *testing.T) {
	second := "018f0000-0000-7000-8000-00000000000b"
	line := `{"type":"text","text":"Write it "},` + mentionJSON(anna, "user", "Anna") + `,` + mentionJSON(second, "user", "Bob") +
		`,{"type":"date","attrs":{"value":"2026-12-01"}},{"type":"date","attrs":{"value":"2027-01-01"}}`
	nested := `{"type":"taskItem","attrs":{"checked":false},"content":[{"type":"paragraph","content":[{"type":"text","text":"Parent"}]},` +
		taskList(taskItem(`"checked":true`, mentionJSON(second, "user", "Bob"))) + `]}`
	n, err := Normalize(doc(taskList(taskItem(`"checked":false`, line), nested)))
	if err != nil {
		t.Fatal(err)
	}
	tasks := Tasks(n)
	if len(tasks) != 3 {
		t.Fatalf("tasks: %v", tasks)
	}
	first := tasks[0]
	if first.AssigneeID == nil || first.AssigneeID.String() != anna || first.Due != "2026-12-01" || first.Done {
		t.Fatalf("first: %+v", first)
	}
	if strings.Contains(first.Text, "Anna") || strings.Contains(first.Text, "2026-12-01") {
		t.Fatalf("the person and date have their own place, not in the words: %q", first.Text)
	}
	if !strings.Contains(first.Text, "Write it") || strings.Contains(first.Text, "Bob") {
		t.Fatalf("the index keeps the words and no name: %q", first.Text)
	}
	if tasks[1].AssigneeID != nil {
		t.Fatal("a mention in a nested task must not count for its parent")
	}
	if !tasks[2].Done || tasks[2].AssigneeID == nil {
		t.Fatalf("nested: %+v", tasks[2])
	}
}

func TestEveryTaskGetsItsOwnID(t *testing.T) {
	taken := uuid.MustParse("018f0000-0000-7000-8000-0000000000aa")
	keep := "018f0000-0000-7000-8000-0000000000bb"
	n, err := Normalize(doc(taskList(
		taskItem(`"checked":false`, `{"type":"text","text":"new"}`),
		taskItem(`"checked":false,"id":"`+keep+`"`, `{"type":"text","text":"keeps"}`),
		taskItem(`"checked":false,"id":"`+keep+`"`, `{"type":"text","text":"repeated"}`),
		taskItem(`"checked":false,"id":"`+taken.String()+`"`, `{"type":"text","text":"from another page"}`),
	)))
	if err != nil {
		t.Fatal(err)
	}
	AssignTaskIDs(&n, func(id uuid.UUID) bool { return id == taken }, uuid.New)
	seen := map[uuid.UUID]bool{}
	for _, task := range Tasks(n) {
		if task.ID == uuid.Nil || task.ID == taken || seen[task.ID] {
			t.Fatalf("task id %s", task.ID)
		}
		seen[task.ID] = true
	}
	if Tasks(n)[1].ID.String() != keep {
		t.Fatal("an id that is fine must stay")
	}
	id := Tasks(n)[2].ID
	if !SetTaskDone(&n, id, true) || !Tasks(n)[2].Done {
		t.Fatal("ticking a task")
	}
	if SetTaskDone(&n, uuid.New(), true) {
		t.Fatal("an unknown task was found")
	}
}

func TestACommentHasAShorterList(t *testing.T) {
	good := doc(`{"type":"paragraph","content":[{"type":"text","text":"hi","marks":[{"type":"bold"}]},` + mentionJSON(anna, "user", "A") + `]},` +
		`{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}]}`)
	if _, err := NormalizeComment(good); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"panel":      `{"type":"panel","attrs":{"kind":"info"},"content":[{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}`,
		"table":      `{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph"}]}]}]}`,
		"colour":     `{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"textColor","attrs":{"color":"red"}}]}]}`,
		"script":     `{"type":"script","content":[{"type":"text","text":"x"}]}`,
		"javascript": `{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}`,
		"empty":      `{"type":"paragraph","content":[{"type":"text","text":"   "}]}`,
	} {
		if _, err := NormalizeComment(json.RawMessage(doc(content))); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	huge := doc(`{"type":"paragraph","content":[{"type":"text","text":"` + strings.Repeat("x", MaxCommentBytes) + `"}]}`)
	if _, err := NormalizeComment(huge); err == nil {
		t.Fatal("a huge comment was accepted")
	}
}
