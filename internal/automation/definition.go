// SPDX-License-Identifier: Apache-2.0

// Package automation runs rules: a trigger, conditions and actions (spec 015, part 2).
// A rule acts as its account through the same module methods as a person, so it can
// never do more than that account may do. Rule code changes no content table itself.
package automation

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// The triggers that do not come from an event.
const (
	TriggerSchedule = "schedule"
	TriggerManual   = "manual"
	TriggerIncoming = "incoming"
)

// eventTriggers are the triggers that follow from events (PLT-025).
var eventTriggers = []string{
	"page.published", "page.edited", "page.moved", "page.copied", "page.archived", "page.deleted", "page.commented",
	"page.labelled", "page.status_changed", "page.owner_changed", "blog.published", "blog.commented", "blog.labelled",
	"attachment.added", "attachment.removed", "task.created", "task.ticked", "person.mentioned",
	"group.member_added", "group.member_removed", "space.created", "space.archived",
}

// instanceOnlyTriggers have no space, so a rule of a space never hears them.
var instanceOnlyTriggers = []string{"group.member_added", "group.member_removed", "space.created"}

// The ways a schedule repeats (D14). Times are in UTC, and the form says so.
const (
	EveryHour  = "hourly"
	EveryDay   = "daily"
	EveryWeek  = "weekly"
	EveryMonth = "monthly"
)

// Trigger says when a rule runs.
type Trigger struct {
	Kind  string `json:"kind"`
	Every string `json:"every,omitempty"`
	At    string `json:"at,omitempty"`
}

// The kinds of condition. All, any and not group others.
const (
	CondAll      = "all"
	CondAny      = "any"
	CondNot      = "not"
	CondSpace    = "in_space"
	CondBelow    = "below_page"
	CondLabel    = "has_label"
	CondStatus   = "has_status"
	CondTitle    = "title_contains"
	CondText     = "text_contains"
	CondAuthor   = "created_by"
	CondOlder    = "older_than"
	CondInGroup  = "member_of"
	maxCondDepth = 5
	maxCondCount = 50
)

// Condition is one test, or a group of them.
type Condition struct {
	Kind  string      `json:"kind"`
	Value string      `json:"value,omitempty"`
	Days  int         `json:"days,omitempty"`
	Items []Condition `json:"items,omitempty"`
}

// The kinds of action.
const (
	ActFind      = "find_pages"
	ActTemplate  = "new_from_template"
	ActCopy      = "copy"
	ActMove      = "move"
	ActArchive   = "archive"
	ActDelete    = "delete"
	ActRestrict  = "restrict"
	ActAddLabel  = "add_label"
	ActDropLabel = "remove_label"
	ActStatus    = "set_status"
	ActOwner     = "set_owner"
	ActComment   = "add_comment"
	ActWatch     = "add_watcher"
	ActUnwatch   = "remove_watcher"
	ActNotify    = "notify"
	ActMail      = "email"
	ActTask      = "assign_task"
	ActWebhook   = "webhook"
	ActChat      = "chat"
)

// Who a message or watcher can be.
const (
	ToOwner  = "owner"
	ToAuthor = "author"
	ToUser   = "user"
	ToGroup  = "group"
)

// Action is one step. Which fields count depends on the type, and validate says so.
type Action struct {
	Type       string     `json:"type"`
	Text       string     `json:"text,omitempty"`
	Label      string     `json:"label,omitempty"`
	Color      string     `json:"color,omitempty"`
	To         string     `json:"to,omitempty"`
	TargetID   *uuid.UUID `json:"target_id,omitempty"`
	SpaceID    *uuid.UUID `json:"space_id,omitempty"`
	ParentID   *uuid.UUID `json:"parent_id,omitempty"`
	Title      string     `json:"title,omitempty"`
	OlderDays  int        `json:"older_days,omitempty"`
	Address    string     `json:"address,omitempty"`
	Body       string     `json:"body,omitempty"`
	WithText   bool       `json:"with_text,omitempty"`
	Restricted string     `json:"restricted,omitempty"`
}

// Definition is what a rule does.
type Definition struct {
	Trigger    Trigger    `json:"trigger"`
	Conditions *Condition `json:"conditions,omitempty"`
	Actions    []Action   `json:"actions"`
	// Spaces narrows a rule of the instance to these spaces; empty means every space.
	// A rule of a space never has any (spec 033).
	Spaces []uuid.UUID `json:"spaces,omitempty"`
}

// reaches says whether the rule acts on something in this space. An event without a
// space, such as a new group member, is not narrowed by spaces.
func (d Definition) reaches(space *uuid.UUID) bool {
	return len(d.Spaces) == 0 || space == nil || slices.Contains(d.Spaces, *space)
}

// Problem is something a person must fix before a rule can be turned on. Step is the
// number of the action, or -1 for the trigger and -2 for the conditions.
type Problem struct {
	Step    int    `json:"step"`
	Message string `json:"message"`
}

const (
	stepTrigger    = -1
	stepConditions = -2
	maxActions     = 50
	maxTextRunes   = 2000
)

func needsPage(kind string) bool {
	return !slices.Contains([]string{ActFind, ActTemplate, ActWebhook, ActChat}, kind)
}

// timeOfDay reads "09:30".
func timeOfDay(at string) (hour, minute int, ok bool) {
	t, err := time.Parse("15:04", at)
	if err != nil {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}

func (t Trigger) problems() []Problem {
	wrong := func(message string) []Problem { return []Problem{{Step: stepTrigger, Message: message}} }
	switch {
	case t.Kind == TriggerManual, t.Kind == TriggerIncoming, slices.Contains(eventTriggers, t.Kind):
		return nil
	case t.Kind != TriggerSchedule:
		return wrong("Choose when this rule runs.")
	case !slices.Contains([]string{EveryHour, EveryDay, EveryWeek, EveryMonth}, t.Every):
		return wrong("Choose how often: hourly, daily, weekly or monthly.")
	}
	if _, _, ok := timeOfDay(t.At); t.Every != EveryHour && !ok {
		return wrong("Give the time as 09:30.")
	}
	return nil
}

func (c Condition) problem(depth int, count *int) string {
	*count++
	if depth > maxCondDepth || *count > maxCondCount {
		return "These conditions go too deep."
	}
	switch c.Kind {
	case CondAll, CondAny, CondNot:
		if len(c.Items) == 0 {
			return "A group needs a condition."
		}
		if c.Kind == CondNot && len(c.Items) != 1 {
			return "Not takes one condition."
		}
		for _, item := range c.Items {
			if p := item.problem(depth+1, count); p != "" {
				return p
			}
		}
		return ""
	case CondOlder:
		if c.Days < 1 {
			return "Give the number of days."
		}
		return ""
	case CondSpace, CondBelow, CondAuthor, CondInGroup:
		if _, err := uuid.Parse(c.Value); err != nil {
			return "Choose who or what this is about."
		}
		return ""
	case CondLabel, CondStatus, CondTitle, CondText:
		if strings.TrimSpace(c.Value) == "" {
			return "This condition needs a value."
		}
		return ""
	}
	return "That is not a condition."
}

func (a Action) needsRecipient() bool {
	return slices.Contains([]string{ActNotify, ActMail, ActWatch, ActUnwatch, ActTask}, a.Type)
}

func (a Action) problem() string {
	if len([]rune(a.Text)) > maxTextRunes || len([]rune(a.Body)) > maxTextRunes {
		return "This text is too long."
	}
	if a.needsRecipient() {
		if p := a.recipientProblem(); p != "" {
			return p
		}
	}
	switch a.Type {
	case ActCopy, ActArchive, ActDelete, ActWatch, ActUnwatch, ActFind:
		return ""
	case ActMove:
		if a.ParentID == nil && a.SpaceID == nil {
			return "Choose where the page goes."
		}
	case ActTemplate:
		if a.TargetID == nil {
			return "Choose a template."
		}
		if strings.TrimSpace(a.Title) == "" {
			return "A new page needs a title."
		}
	case ActRestrict:
		if a.Restricted != "view" && a.Restricted != "edit" {
			return "Choose whether viewing or editing is restricted."
		}
		if a.TargetID == nil {
			return "Choose the group that keeps access."
		}
	case ActAddLabel, ActDropLabel:
		if strings.TrimSpace(a.Label) == "" {
			return "Give the label."
		}
	case ActStatus:
		if strings.TrimSpace(a.Text) == "" {
			return "Give the status."
		}
	case ActOwner:
		if a.TargetID == nil {
			return "Choose the new owner."
		}
	case ActComment, ActNotify, ActMail, ActTask, ActChat:
		if strings.TrimSpace(a.Text) == "" {
			return "Write the text."
		}
		if a.Type == ActChat {
			return a.addressProblem()
		}
	case ActWebhook:
		return a.addressProblem()
	default:
		return "Choose what this step does."
	}
	return ""
}

func (a Action) recipientProblem() string {
	switch a.To {
	case ToOwner, ToAuthor:
		return ""
	case ToUser, ToGroup:
		if a.TargetID == nil {
			return "Choose who this is for."
		}
		return ""
	}
	return "Choose who this is for."
}

func (a Action) addressProblem() string {
	if a.Address == "" {
		return "Choose an allowed host."
	}
	return ""
}

// Problems lists what is wrong with a definition, so each message can sit beside its
// field, and a rule cannot be turned on while one is open.
func (d Definition) Problems() []Problem {
	out := append([]Problem{}, d.Trigger.problems()...)
	if d.Conditions != nil {
		count := 0
		if p := d.Conditions.problem(0, &count); p != "" {
			out = append(out, Problem{Step: stepConditions, Message: p})
		}
	}
	if len(d.Actions) == 0 {
		out = append(out, Problem{Step: 0, Message: "A rule needs at least one action."})
	}
	if len(d.Actions) > maxActions {
		out = append(out, Problem{Step: maxActions, Message: "A rule holds at most 50 actions."})
	}
	hasPage := d.Trigger.bringsPage()
	for i, a := range d.Actions {
		if p := a.problem(); p != "" {
			out = append(out, Problem{Step: i, Message: p})
			continue
		}
		hasPage = hasPage || a.Type == ActFind
		if needsPage(a.Type) && !hasPage {
			out = append(out, Problem{Step: i, Message: "This step needs a page. Add a Find pages step before it."})
		}
	}
	return out
}

// bringsPage says whether a run starts with a page.
func (t Trigger) bringsPage() bool {
	return t.Kind == TriggerManual || (slices.Contains(eventTriggers, t.Kind) && !strings.HasPrefix(t.Kind, "group.") && !strings.HasPrefix(t.Kind, "space."))
}

// expand fills in {name} from values, and leaves what it does not know alone. There
// is no expression language, so a value can never become an instruction (D13).
func expand(text string, values map[string]string) string {
	pairs := make([]string, 0, len(values))
	for name, value := range values {
		pairs = append(pairs, "{"+name+"}", value)
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

// expandJSON fills in a JSON body: each value is escaped, so a title cannot change the
// shape of what is sent.
func expandJSON(body string, values map[string]string) (string, error) {
	escaped := make(map[string]string, len(values))
	for name, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		escaped[name] = string(raw[1 : len(raw)-1])
	}
	out := expand(body, escaped)
	if !json.Valid([]byte(out)) {
		return "", fmt.Errorf("the body is not JSON")
	}
	return out, nil
}

// nextRun gives the first moment after now for a schedule.
func nextRun(t Trigger, now time.Time) time.Time {
	now = now.UTC()
	if t.Every == EveryHour {
		return now.Truncate(time.Hour).Add(time.Hour)
	}
	hour, minute, _ := timeOfDay(t.At)
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	step := func(at time.Time) time.Time {
		switch t.Every {
		case EveryWeek:
			return at.AddDate(0, 0, 7)
		case EveryMonth:
			return at.AddDate(0, 1, 0)
		}
		return at.AddDate(0, 0, 1)
	}
	for !next.After(now) {
		next = step(next)
	}
	return next
}

// facts are what conditions look at, loaded as the account sees them.
type facts struct {
	spaceID   uuid.UUID
	ancestors []uuid.UUID
	labels    []string
	status    string
	title     string
	text      string
	authorID  uuid.UUID
	createdAt time.Time
	groups    []uuid.UUID // of the author
}

func containsFold(text, part string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(part))
}

func hasID(list []uuid.UUID, value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && slices.Contains(list, id)
}

// holds evaluates a condition. Nil means no conditions, so everything passes.
func (c *Condition) holds(f facts, now time.Time) bool {
	if c == nil {
		return true
	}
	switch c.Kind {
	case CondAll:
		return !slices.ContainsFunc(c.Items, func(i Condition) bool { return !i.holds(f, now) })
	case CondAny:
		return slices.ContainsFunc(c.Items, func(i Condition) bool { return i.holds(f, now) })
	case CondNot:
		return len(c.Items) == 1 && !c.Items[0].holds(f, now)
	case CondSpace:
		return c.Value == f.spaceID.String()
	case CondBelow:
		return hasID(f.ancestors, c.Value)
	case CondLabel:
		return slices.Contains(f.labels, strings.ToLower(strings.TrimSpace(c.Value)))
	case CondStatus:
		return strings.EqualFold(c.Value, f.status)
	case CondTitle:
		return containsFold(f.title, c.Value)
	case CondText:
		return containsFold(f.text, c.Value)
	case CondAuthor:
		return c.Value == f.authorID.String()
	case CondOlder:
		return now.Sub(f.createdAt) > time.Duration(c.Days)*24*time.Hour
	case CondInGroup:
		return hasID(f.groups, c.Value)
	}
	return false
}
