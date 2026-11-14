// SPDX-License-Identifier: Apache-2.0

package workflow

// Example is a workflow Diffluence ships, so a space can start at once (DOC-131). Its
// names are written for Diffluence; the screens translate them by Key and the ids of its
// states and transitions, and a copy keeps whatever words the person saves.
type Example struct {
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Definition  Definition `json:"definition"`
}

// Examples returns fresh copies of the two examples. Their review states name nobody yet:
// a space fills in its own reviewers after copying.
func Examples() []Example {
	return []Example{reviewThenApproveExample(), twoStepExample()}
}

func independentReview() *Review {
	return &Review{Reviewers: []Reviewer{}, Rule: RuleAll}
}

func reviewThenApproveExample() Example {
	return Example{
		Key:         "review-then-approve",
		Name:        "Review then approve",
		Description: "One round of review. A request for changes sends the page back to its authors.",
		Definition: Definition{
			States: []State{
				{ID: "draft", Name: "Draft", Kind: KindDraft, Start: true},
				{ID: "in-review", Name: "In review", Kind: KindInReview, Review: independentReview()},
				{ID: "approved", Name: "Approved", Kind: KindApproved},
				{ID: "rejected", Name: "Changes asked", Kind: KindRejected},
			},
			Transitions: []Transition{
				{ID: "submit", Name: "Send for review", From: "draft", To: "in-review"},
				{ID: "approve", Name: "Approve", From: "in-review", To: "approved", Outcome: OutcomeApproved},
				{ID: "ask-changes", Name: "Ask for changes", From: "in-review", To: "rejected", Outcome: OutcomeChanges},
				{ID: "rework", Name: "Work on it again", From: "rejected", To: "draft"},
			},
		},
	}
}

// twoStepExample is a technical review followed by a quality approval. The signature its
// study names comes with signatures, in wave DC4.
func twoStepExample() Example {
	return Example{
		Key:         "two-step",
		Name:        "Two-step review",
		Description: "A technical review, then a quality approval, before the page becomes effective.",
		Definition: Definition{
			States: []State{
				{ID: "draft", Name: "Draft", Kind: KindDraft, Start: true},
				{ID: "technical-review", Name: "Technical review", Kind: KindInReview, Review: independentReview()},
				{ID: "quality-approval", Name: "Quality approval", Kind: KindInReview, Review: independentReview()},
				{ID: "effective", Name: "Effective", Kind: KindApproved},
			},
			Transitions: []Transition{
				{ID: "submit", Name: "Send for technical review", From: "draft", To: "technical-review"},
				{ID: "technical-ok", Name: "Technically correct", From: "technical-review", To: "quality-approval", Outcome: OutcomeApproved},
				{ID: "technical-changes", Name: "Ask for technical changes", From: "technical-review", To: "draft", Outcome: OutcomeChanges},
				{ID: "quality-ok", Name: "Approve as effective", From: "quality-approval", To: "effective", Outcome: OutcomeApproved},
				{ID: "quality-changes", Name: "Ask for changes", From: "quality-approval", To: "draft", Outcome: OutcomeChanges},
			},
		},
	}
}
