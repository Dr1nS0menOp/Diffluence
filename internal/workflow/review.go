// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"github.com/google/uuid"
)

// Reviewers, rules to pass and independence (blueprint 09.3, spec 064 wave DC2).

// Rule is how many reviewers must approve for a round to pass (DOC-210).
type Rule string

// The rules to pass.
const (
	RuleAll    Rule = "all"
	RuleQuorum Rule = "quorum"
	RuleAny    Rule = "any"
)

// Kinds of reviewer entry: a person, a group, or a role of the space (DOC-200). A guest is
// never one, whichever entry names them (DOC-1422).
const (
	ReviewerUser  = "user"
	ReviewerGroup = "group"
	ReviewerRole  = "role"
)

// Limits on who is asked.
const (
	// MaxReviewersPerRound is the most people a round under "all" or a quorum asks, because
	// nobody can review with hundreds of sign-offs (DOC-1408).
	MaxReviewersPerRound = 50
	// MaxAnyReviewers bounds a round under "any one", where a large group is sensible.
	MaxAnyReviewers = 500
	// MaxReviewerEntries is how many people, groups and roles one state may name.
	MaxReviewerEntries = 20
)

// Reviewer is one entry of the list of a review state: resolved to people when a round
// starts, and stored on the round, so a later change of a group rewrites nothing (DOC-201).
type Reviewer struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
}

// Review is what a state of kind in review asks: who, and how many must agree.
type Review struct {
	Reviewers []Reviewer `json:"reviewers"`
	Rule      Rule       `json:"rule"`
	Quorum    int        `json:"quorum,omitempty"`
	// AuthorsMayReview switches independence off. It is off by default, so an author of the
	// change never counts as its reviewer (DOC-220, DOC-1403); only the workflow, as a new
	// version, can turn it on (DOC-221).
	AuthorsMayReview bool `json:"authors_may_review"`
	// AuthorsMayAdd lets the authors add reviewers to a running round (DOC-202).
	AuthorsMayAdd bool `json:"authors_may_add"`
}

// Outcome is where a round stands, and which transition the system takes for it.
type Outcome string

// The outcomes of a round. A transition with an outcome is taken by the system when the
// round reaches it, never by a person (DOC-013, DOC-213).
const (
	OutcomePending  Outcome = "pending"
	OutcomeApproved Outcome = "approved"
	OutcomeChanges  Outcome = "changes"
)

// Needed is how many approvals pass a round that asked this many people.
func (r Review) Needed(asked int) int {
	switch r.Rule {
	case RuleQuorum:
		return r.Quorum
	case RuleAny:
		return 1
	}
	return asked
}

// Count decides a round from its answers. One "changes" ends it under every rule
// (DOC-211, DOC-1402); otherwise it passes once enough approved.
func (r Review) Count(asked, approved, changes int) Outcome {
	if changes > 0 {
		return OutcomeChanges
	}
	if asked > 0 && approved >= r.Needed(asked) {
		return OutcomeApproved
	}
	return OutcomePending
}

// MaxResolved is the most people a round under this rule may ask.
func (r Review) MaxResolved() int {
	if r.Rule == RuleAny {
		return MaxAnyReviewers
	}
	return MaxReviewersPerRound
}

// Outcome finds the transition the system takes from a review state for an outcome.
func (d Definition) Outcome(state string, o Outcome) (Transition, bool) {
	for _, t := range d.Transitions {
		if t.From == state && t.Outcome == o {
			return t, true
		}
	}
	return Transition{}, false
}

func validReviewer(r Reviewer) bool {
	switch r.Type {
	case ReviewerUser, ReviewerGroup, ReviewerRole:
		return r.ID != uuid.Nil
	}
	return false
}

// checkReview checks the review of one state.
func checkReview(s State) *Problem {
	rv := s.Review
	if s.Kind != KindInReview {
		return problem("review_not_in_review", s.ID, "Only a state of kind in review can name reviewers, and %q is not one.", s.Name)
	}
	switch rv.Rule {
	case RuleAll, RuleAny:
		if rv.Quorum != 0 {
			return problem("bad_quorum", s.ID, "Only the rule \"a number of them\" takes a number, in %q.", s.Name)
		}
	case RuleQuorum:
		if rv.Quorum < 1 || rv.Quorum > MaxReviewersPerRound {
			return problem("bad_quorum", s.ID, "The number who must approve in %q is 1 to %d.", s.Name, MaxReviewersPerRound)
		}
	default:
		return problem("bad_rule", s.ID, "The state %q needs a rule: all, a number of them, or any one.", s.Name)
	}
	if len(rv.Reviewers) > MaxReviewerEntries {
		return problem("too_many_reviewers", s.ID, "The state %q names more than %d people, groups and roles.", s.Name, MaxReviewerEntries)
	}
	seen := map[Reviewer]bool{}
	for _, r := range rv.Reviewers {
		if !validReviewer(r) || seen[r] {
			return problem("bad_reviewer", s.ID, "A reviewer of %q is not a person, group or role, or is named twice.", s.Name)
		}
		seen[r] = true
	}
	return nil
}

// checkOutcomes checks the transitions out of review states: each review state has one
// approving outcome into review or approval and one "changes" outcome back to draft or
// rejected, and no author can step from it straight into approval (DOC-013, DOC-211).
func checkOutcomes(d Definition, states map[string]State) *Problem {
	for _, tr := range d.Transitions {
		from, to := states[tr.From], states[tr.To]
		switch tr.Outcome {
		case "":
			if from.Review != nil && to.Kind == KindApproved {
				return problem("approval_needs_review", tr.ID, "The step %q approves without the reviewers of %q.", tr.Name, from.Name)
			}
			continue
		case OutcomeApproved:
			if to.Kind != KindApproved && to.Kind != KindInReview {
				return problem("approval_target", tr.ID, "The step %q follows approval, so it leads to review or approval.", tr.Name)
			}
		case OutcomeChanges:
			if to.Kind != KindDraft && to.Kind != KindRejected {
				return problem("changes_target", tr.ID, "The step %q follows a request for changes, so it leads to draft or rejected.", tr.Name)
			}
		default:
			return problem("bad_outcome", tr.ID, "The step %q has an outcome that does not exist.", tr.Name)
		}
		if from.Review == nil {
			return problem("outcome_without_review", tr.ID, "The step %q follows a review, but %q asks no reviewers.", tr.Name, from.Name)
		}
	}
	for _, s := range d.States {
		if s.Review == nil {
			continue
		}
		approving, changes := 0, 0
		for _, tr := range d.From(s.ID) {
			switch tr.Outcome {
			case OutcomeApproved:
				approving++
			case OutcomeChanges:
				changes++
			}
		}
		if approving != 1 || changes != 1 {
			return problem("review_outcomes", s.ID, "The state %q needs one step for when it is approved and one for when changes are asked.", s.Name)
		}
	}
	return nil
}
