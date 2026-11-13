// SPDX-License-Identifier: Apache-2.0

// Package workflow is the model of a document-control workflow: its states, their kinds and
// the transitions between them (blueprint 09.2, spec 064). It is pure, so the rules a
// workflow must meet can be tested exhaustively; the pages module stores and applies it.
package workflow

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Kind is what a state means to the rest of Diffluence. Names differ by organisation and
// language; readers, search and exports decide by kind only (DOC-101).
type Kind string

// The kinds of state.
const (
	KindDraft    Kind = "draft"
	KindInReview Kind = "in_review"
	KindApproved Kind = "approved"
	KindRejected Kind = "rejected"
	KindObsolete Kind = "obsolete"
)

// Kinds lists every kind, in the order a person meets them.
var Kinds = []Kind{KindDraft, KindInReview, KindApproved, KindRejected, KindObsolete}

func (k Kind) valid() bool {
	for _, known := range Kinds {
		if k == known {
			return true
		}
	}
	return false
}

// State is one step a document can be in.
type State struct {
	// ID never changes, so history and audit keep pointing at the right state when it is
	// renamed (DOC-101).
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Kind        Kind   `json:"kind"`
	// Start marks the one state a page enters a workflow in, and returns to on a change.
	Start bool `json:"start"`
	// Timed marks an approved state whose documents come up for periodic review (DOC-103).
	Timed bool `json:"timed"`
}

// Transition is a named step from one state to another (DOC-104).
type Transition struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	From            string `json:"from"`
	To              string `json:"to"`
	RequiresComment bool   `json:"requires_comment"`
}

// Definition is one version of a workflow: its states and transitions.
type Definition struct {
	States      []State      `json:"states"`
	Transitions []Transition `json:"transitions"`
}

// Limits keep a workflow something a person can read and an auditor can follow.
const (
	MaxStates      = 30
	MaxTransitions = 100
	maxNameRunes   = 80
	maxDescRunes   = 500
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

// Problem is a rule a definition breaks, with the state or transition it is about, so the
// message can name both (DOC-102).
type Problem struct {
	Rule    string `json:"rule"`
	Subject string `json:"subject,omitempty"`
	Message string `json:"message"`
}

func (p *Problem) Error() string { return p.Message }

func problem(rule, subject, format string, args ...any) *Problem {
	return &Problem{Rule: rule, Subject: subject, Message: fmt.Sprintf(format, args...)}
}

func checkText(field, subject, value string, required bool, limit int) *Problem {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return problem("name_missing", subject, "Every %s needs a name.", field)
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > limit || strings.ContainsAny(value, "\x00") {
		return problem("text_too_long", subject, "The %s of %q is too long or not valid text.", field, subject)
	}
	return nil
}

// Validate checks every rule of DOC-101 to DOC-104. A definition that passes has one start
// state of kind draft, at least one approved state, every state reachable from the start,
// and a way to approval from every state that is not approved or obsolete.
func Validate(d Definition) error {
	if len(d.States) == 0 {
		return problem("no_states", "", "A workflow needs states.")
	}
	if len(d.States) > MaxStates {
		return problem("too_many_states", "", "A workflow has at most %d states.", MaxStates)
	}
	if len(d.Transitions) > MaxTransitions {
		return problem("too_many_transitions", "", "A workflow has at most %d transitions.", MaxTransitions)
	}
	states := map[string]State{}
	names := map[string]bool{}
	starts, approved := 0, 0
	for _, s := range d.States {
		if !idPattern.MatchString(s.ID) {
			return problem("bad_id", s.ID, "The state id %q must be 1 to 40 lower-case letters, digits, - or _.", s.ID)
		}
		if _, dup := states[s.ID]; dup {
			return problem("duplicate_state", s.ID, "Two states have the id %q.", s.ID)
		}
		if p := checkText("state", s.ID, s.Name, true, maxNameRunes); p != nil {
			return p
		}
		if p := checkText("description", s.ID, s.Description, false, maxDescRunes); p != nil {
			return p
		}
		key := strings.ToLower(strings.TrimSpace(s.Name))
		if names[key] {
			return problem("duplicate_name", s.ID, "Two states are called %q.", s.Name)
		}
		names[key] = true
		if !s.Kind.valid() {
			return problem("bad_kind", s.ID, "The state %q has no known kind.", s.Name)
		}
		if s.Timed && s.Kind != KindApproved {
			return problem("timed_not_approved", s.ID, "Only an approved state can come up for periodic review.")
		}
		if s.Start {
			starts++
			if s.Kind != KindDraft {
				return problem("start_not_draft", s.ID, "The start state %q must be of kind draft.", s.Name)
			}
		}
		if s.Kind == KindApproved {
			approved++
		}
		states[s.ID] = s
	}
	if starts != 1 {
		return problem("one_start", "", "A workflow has exactly one start state.")
	}
	if approved == 0 {
		return problem("no_approved", "", "A workflow needs at least one approved state.")
	}
	ids := map[string]bool{}
	pairs := map[string]bool{}
	next := map[string][]string{}
	for _, tr := range d.Transitions {
		if !idPattern.MatchString(tr.ID) {
			return problem("bad_id", tr.ID, "The transition id %q must be 1 to 40 lower-case letters, digits, - or _.", tr.ID)
		}
		if ids[tr.ID] {
			return problem("duplicate_transition", tr.ID, "Two transitions have the id %q.", tr.ID)
		}
		ids[tr.ID] = true
		if p := checkText("transition", tr.ID, tr.Name, true, maxNameRunes); p != nil {
			return p
		}
		if p := checkText("description", tr.ID, tr.Description, false, maxDescRunes); p != nil {
			return p
		}
		if _, ok := states[tr.From]; !ok {
			return problem("unknown_state", tr.ID, "The transition %q starts in a state that does not exist.", tr.Name)
		}
		if _, ok := states[tr.To]; !ok {
			return problem("unknown_state", tr.ID, "The transition %q goes to a state that does not exist.", tr.Name)
		}
		if tr.From == tr.To {
			return problem("same_state", tr.ID, "The transition %q goes nowhere: it ends where it starts.", tr.Name)
		}
		pair := tr.From + "\x00" + tr.To + "\x00" + strings.ToLower(strings.TrimSpace(tr.Name))
		if pairs[pair] {
			return problem("duplicate_name", tr.ID, "Two transitions between the same states are called %q.", tr.Name)
		}
		pairs[pair] = true
		next[tr.From] = append(next[tr.From], tr.To)
	}
	start := d.StartState()
	reached := reach(start.ID, next)
	for _, s := range d.States {
		if !reached[s.ID] {
			return problem("unreachable", s.ID, "The state %q cannot be reached from the start.", s.Name)
		}
	}
	for _, s := range d.States {
		if s.Kind == KindApproved || s.Kind == KindObsolete {
			continue
		}
		if !leadsToApproval(s.ID, next, states) {
			return problem("no_way_to_approval", s.ID, "From the state %q there is no way to an approved state.", s.Name)
		}
	}
	return nil
}

func reach(from string, next map[string][]string) map[string]bool {
	seen := map[string]bool{from: true}
	queue := []string{from}
	for len(queue) > 0 {
		at := queue[0]
		queue = queue[1:]
		for _, to := range next[at] {
			if !seen[to] {
				seen[to] = true
				queue = append(queue, to)
			}
		}
	}
	return seen
}

func leadsToApproval(from string, next map[string][]string, states map[string]State) bool {
	for id := range reach(from, next) {
		if states[id].Kind == KindApproved {
			return true
		}
	}
	return false
}

// StartState is the state a page enters in. It is the zero State when there is none, which
// Validate refuses.
func (d Definition) StartState() State {
	for _, s := range d.States {
		if s.Start {
			return s
		}
	}
	return State{}
}

// FirstApproved is the first approved state in the order of the definition: where a page
// that is already approved lands when it joins (DOC-140).
func (d Definition) FirstApproved() State {
	for _, s := range d.States {
		if s.Kind == KindApproved {
			return s
		}
	}
	return State{}
}

// State finds a state by id.
func (d Definition) State(id string) (State, bool) {
	for _, s := range d.States {
		if s.ID == id {
			return s, true
		}
	}
	return State{}, false
}

// Transition finds a transition by id.
func (d Definition) Transition(id string) (Transition, bool) {
	for _, t := range d.Transitions {
		if t.ID == id {
			return t, true
		}
	}
	return Transition{}, false
}

// From lists the transitions that leave a state, in the order of the definition.
func (d Definition) From(state string) []Transition {
	var out []Transition
	for _, t := range d.Transitions {
		if t.From == state {
			out = append(out, t)
		}
	}
	return out
}

// Change is one difference between two versions of a workflow, for its history (DOC-112).
type Change struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// The kinds of change.
const (
	StateAdded        = "state_added"
	StateRemoved      = "state_removed"
	StateRenamed      = "state_renamed"
	StateChanged      = "state_changed"
	TransitionAdded   = "transition_added"
	TransitionRemoved = "transition_removed"
	TransitionChanged = "transition_changed"
)

// Diff lists what changed from one version to the next, in a stable order.
func Diff(before, after Definition) []Change {
	var out []Change
	for _, s := range before.States {
		n, ok := after.State(s.ID)
		switch {
		case !ok:
			out = append(out, Change{Kind: StateRemoved, ID: s.ID, From: s.Name})
		case n.Name != s.Name:
			out = append(out, Change{Kind: StateRenamed, ID: s.ID, From: s.Name, To: n.Name})
		case n != s:
			out = append(out, Change{Kind: StateChanged, ID: s.ID, From: s.Name, To: n.Name})
		}
	}
	for _, s := range after.States {
		if _, ok := before.State(s.ID); !ok {
			out = append(out, Change{Kind: StateAdded, ID: s.ID, To: s.Name})
		}
	}
	for _, t := range before.Transitions {
		n, ok := after.Transition(t.ID)
		switch {
		case !ok:
			out = append(out, Change{Kind: TransitionRemoved, ID: t.ID, From: t.Name})
		case n != t:
			out = append(out, Change{Kind: TransitionChanged, ID: t.ID, From: t.Name, To: n.Name})
		}
	}
	for _, t := range after.Transitions {
		if _, ok := before.Transition(t.ID); !ok {
			out = append(out, Change{Kind: TransitionAdded, ID: t.ID, To: t.Name})
		}
	}
	if out == nil {
		out = []Change{}
	}
	return out
}
