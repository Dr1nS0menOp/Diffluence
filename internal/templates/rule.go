// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/templates/fields"
)

// An automation rule's action "new page from a template" (spec 074, wave GT5, TPL-505,
// TPL-604, TPL-1105). The rule acts with the rights of its owner, through the same checks a
// person meets, and its history keeps the code of why a run made nothing.

var (
	// errRuleNotOffered answers a rule whose owner may not use the template there: a users
	// list leaves them out, its space is not chosen, or it is gone. Like errNoTemplate it does
	// not say which (TPL-504).
	errRuleNotOffered = &apperr.Error{Kind: apperr.KindNotFound, Code: "template_not_offered",
		Message: "This template does not exist, or it is not offered to the owner of this rule here, so the rule made nothing."}
	// errRuleNothingApproved answers a rule whose template has no version in use: a rule only
	// ever uses the approved version (TPL-604).
	errRuleNothingApproved = &apperr.Error{Kind: apperr.KindConflict, Code: "template_not_approved",
		Message: "This template has no approved version in use, so the rule made nothing."}
)

// errRuleAsks answers a rule whose template asks a question nobody answers for it (TPL-1105).
func errRuleAsks(name string) error {
	return &apperr.Error{Kind: apperr.KindInvalid, Code: "template_asks",
		Message: "This template asks for " + name + ", which a rule cannot answer."}
}

// UseForRule makes the pages a rule asks for, from the version approved at the moment it runs
// (TPL-604). A rule answers no question, so a template that requires an answer without a
// suggestion refuses it, naming the question (TPL-1105).
func (s *Service) UseForRule(ctx context.Context, c identity.Caller, id uuid.UUID, in UseInput) (SetResult, error) {
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return SetResult{}, ruleRefusal(err)
	}
	st, err := s.mayUseIn(ctx, s.pool, c, t, in.SpaceID)
	if err != nil {
		return SetResult{}, ruleRefusal(err)
	}
	if !st.offeredToReader() && !st.mayLook() {
		return SetResult{}, errRuleNotOffered
	}
	if t.TemplateVersion == 0 {
		return SetResult{}, errRuleNothingApproved
	}
	asked, err := s.unanswerable(ctx, s.pool, t.ID, t.TemplateVersion)
	if err != nil {
		return SetResult{}, err
	}
	if len(asked) > 0 {
		return SetResult{}, errRuleAsks(asked[0])
	}
	made, err := s.MakeSet(ctx, c, id, SetInput{SpaceID: in.SpaceID, ParentID: in.ParentID, Title: in.Title,
		Version: t.TemplateVersion})
	return made, ruleRefusal(err)
}

// ruleRefusal tells a rule's history that the template is not for its owner, in the words of a
// rule, whatever check found it.
func ruleRefusal(err error) error {
	if errors.Is(err, errNoTemplate) {
		return errRuleNotOffered
	}
	// Other refusals keep their own code, such as a space that is gone or a part that is off.
	if e, ok := apperr.As(err); ok && e.Code == apperr.NotFound().Code {
		return errRuleNotOffered
	}
	return err
}

// unanswerable lists the questions of a version that a rule cannot answer: required, with no
// suggestion that still stands, and asked whatever was answered. A question a condition shows
// may stay hidden, so it does not count; the run says so when it shows after all.
func (s *Service) unanswerable(ctx context.Context, q db.Querier, templateID uuid.UUID, number int) ([]string, error) {
	v, err := versionAt(ctx, q, templateID, number)
	if err != nil {
		return nil, err
	}
	texts, _, err := s.textsOf(ctx, q, v.Manifest)
	if err != nil {
		return nil, err
	}
	form, err := s.withGoneDefaults(ctx, q, formOf(v.Manifest, texts))
	if err != nil {
		return nil, err
	}
	return ruleQuestions(form, v.Manifest.Steps), nil
}

// ruleQuestions picks the questions a rule cannot answer from a form.
func ruleQuestions(form []fields.Spec, steps []Step) []string {
	shownByStep := map[string]bool{}
	for _, st := range steps {
		for _, name := range st.Fields {
			shownByStep[name] = st.Condition == nil
		}
	}
	out := []string{}
	for _, f := range form {
		always := f.Condition == nil && (shownByStep[f.Name] || !inAnyStep(steps, f.Name))
		if f.Required && f.Default == nil && always {
			out = append(out, f.Name)
		}
	}
	return out
}

func inAnyStep(steps []Step, name string) bool {
	for _, st := range steps {
		for _, n := range st.Fields {
			if n == name {
				return true
			}
		}
	}
	return false
}

// RuleQuestions lists the questions of the version in use that a rule cannot answer, for the
// rule's test run, which warns about them before the rule is turned on (TPL-1105).
func (s *Service) RuleQuestions(ctx context.Context, c identity.Caller, id uuid.UUID) ([]string, error) {
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return nil, ruleRefusal(err)
	}
	st, err := s.standingOf(ctx, s.pool, c, t)
	if err != nil {
		return nil, err
	}
	if seen, err := s.maySee(ctx, s.pool, st); err != nil || !seen {
		return nil, errOr(err, errRuleNotOffered)
	}
	if t.TemplateVersion == 0 {
		return nil, errRuleNothingApproved
	}
	return s.unanswerable(ctx, s.pool, t.ID, t.TemplateVersion)
}
