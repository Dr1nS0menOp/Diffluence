// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"github.com/google/uuid"
)

// Time, periodic review and read and understood (blueprint 09.3.4, 09.9, spec 064 wave DC5).

// The outcomes the system takes on time. Like the outcomes of a review, nobody takes them by
// hand (DOC-013).
const (
	// OutcomeOverdue leaves a review state when its due date passes (DOC-231). It never leads
	// into an approved state: a time-out is never an approval (DOC-105, DOC-1401).
	OutcomeOverdue Outcome = "overdue"
	// OutcomeReviewDue leaves a timed approved state for a review state when the next review
	// date comes (DOC-802). Readers keep the approved version while that review runs.
	OutcomeReviewDue Outcome = "review_due"
)

// Bounds of time and of the people a state names for it.
const (
	// MaxDueDays is the longest due period, in working days.
	MaxDueDays = 60
	// MaxReviewMonths is the longest review period of a timed approved state.
	MaxReviewMonths = 120
	// DefaultReviewMonths is the period of a timed state that names none, as states saved
	// before wave DC5 do.
	DefaultReviewMonths = 12
	// MaxAcknowledgeGroups is how many groups one approved state asks to confirm reading.
	MaxAcknowledgeGroups = 10
)

// Due is how long the reviewers of a state have, and who is asked when that runs out
// (DOC-230, DOC-231). What happens when it passes is the state's overdue step if it has one;
// otherwise the escalation targets are added, once; otherwise the reviewers are reminded again.
type Due struct {
	// WorkingDays is the due period, counted in the working week of the instance (DOC-1242).
	WorkingDays int `json:"working_days"`
	// RemindBefore is how many working days before the due date the reviewers are reminded;
	// zero reminds them on the day only.
	RemindBefore int `json:"remind_before"`
	// Escalate names who is added as a reviewer when the date passes, or when the round can
	// no longer pass because people left (DOC-242).
	Escalate []Reviewer `json:"escalate,omitempty"`
}

// ReadAndUnderstood asks the members of groups to confirm they read a version approved into
// the state (DOC-820).
type ReadAndUnderstood struct {
	Groups []uuid.UUID `json:"groups"`
}

// Months is the review period of a timed approved state.
func (s State) Months() int {
	if s.ReviewMonths > 0 {
		return s.ReviewMonths
	}
	return DefaultReviewMonths
}

// Escalation is who a state escalates to, or nothing.
func (s State) Escalation() []Reviewer {
	if s.Review == nil || s.Review.Due == nil {
		return nil
	}
	return s.Review.Due.Escalate
}

// checkTime checks the time and the reading a state asks for.
func checkTime(s State) *Problem {
	if s.ReviewMonths != 0 && (!s.Timed || s.ReviewMonths < 0 || s.ReviewMonths > MaxReviewMonths) {
		return problem("bad_review_period", s.ID, "Only a timed approved state has a review period, of 1 to %d months, and %q is not one.", MaxReviewMonths, s.Name)
	}
	if ack := s.ReadAndUnderstood; ack != nil {
		if s.Kind != KindApproved {
			return problem("acknowledge_not_approved", s.ID, "Only an approved state can ask people to confirm they read it, and %q is not one.", s.Name)
		}
		if len(ack.Groups) == 0 || len(ack.Groups) > MaxAcknowledgeGroups {
			return problem("bad_acknowledge", s.ID, "The state %q asks 1 to %d groups to confirm they read it.", s.Name, MaxAcknowledgeGroups)
		}
		seen := map[uuid.UUID]bool{}
		for _, g := range ack.Groups {
			if g == uuid.Nil || seen[g] {
				return problem("bad_acknowledge", s.ID, "A group that %q asks to confirm reading is missing or named twice.", s.Name)
			}
			seen[g] = true
		}
	}
	if s.Review == nil || s.Review.Due == nil {
		return nil
	}
	due := s.Review.Due
	if due.WorkingDays < 1 || due.WorkingDays > MaxDueDays {
		return problem("bad_due", s.ID, "The due period of %q is 1 to %d working days.", s.Name, MaxDueDays)
	}
	if due.RemindBefore < 0 || due.RemindBefore >= due.WorkingDays {
		return problem("bad_due", s.ID, "The reminder of %q comes before the due date, within its period.", s.Name)
	}
	if len(due.Escalate) > MaxReviewerEntries {
		return problem("bad_escalation", s.ID, "The state %q escalates to more than %d people, groups and roles.", s.Name, MaxReviewerEntries)
	}
	seen := map[Reviewer]bool{}
	for _, r := range due.Escalate {
		if !validReviewer(r) || seen[r] {
			return problem("bad_escalation", s.ID, "Someone %q escalates to is not a person, group or role, or is named twice.", s.Name)
		}
		seen[r] = true
	}
	return nil
}

// checkTimedOutcome checks a step the system takes on time. It answers false when the step
// is not one of those.
func checkTimedOutcome(tr Transition, from, to State) (*Problem, bool) {
	switch tr.Outcome {
	case OutcomeOverdue:
		if from.Review == nil || from.Review.Due == nil {
			return problem("overdue_without_due", tr.ID, "The step %q follows a due date, but %q has none.", tr.Name, from.Name), true
		}
		// The rule of DOC-105 and DOC-1401: a time-out is never an approval.
		if to.Kind == KindApproved {
			return problem("timeout_never_approves", tr.ID, "The step %q is taken when time runs out, so it cannot lead to approval.", tr.Name), true
		}
		return nil, true
	case OutcomeReviewDue:
		if from.Kind != KindApproved || !from.Timed {
			return problem("review_due_not_timed", tr.ID, "The step %q follows a review date, but %q is not a timed approved state.", tr.Name, from.Name), true
		}
		if to.Review == nil {
			return problem("review_due_target", tr.ID, "The step %q starts a periodic review, so it leads to a state with reviewers.", tr.Name), true
		}
		return nil, true
	}
	return nil, false
}

// checkTimedCounts allows at most one step of each timed outcome per state.
func checkTimedCounts(d Definition) *Problem {
	for _, s := range d.States {
		overdue, reviewDue := 0, 0
		for _, tr := range d.From(s.ID) {
			switch tr.Outcome {
			case OutcomeOverdue:
				overdue++
			case OutcomeReviewDue:
				reviewDue++
			}
		}
		if overdue > 1 || reviewDue > 1 {
			return problem("timed_outcomes", s.ID, "The state %q has more than one step for when its time comes.", s.Name)
		}
	}
	return nil
}
