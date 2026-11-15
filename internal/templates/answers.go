// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/conditions"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/labels"
	"github.com/diffluence/diffluence/internal/templates/fields"
)

// answered holds every answer of a set once it is checked, in the form each place needs.
// Answers are never stored apart from the pages they fill (TPL-1409).
type answered struct {
	// fills is what each answered question puts in a page; a question missing here stays a
	// field, for the writer to fill in.
	fills map[string]document.Filling
	// titles is each answer as a title shows it: names, not ids (TPL-140).
	titles map[string]string
	// asked names every question of the set once, in the order the form asks them.
	asked []string
	// facts are what the conditions of the set's pages, sections and rules are decided on:
	// every answer as it lands, a hidden question as empty (TPL-421, TPL-430).
	facts conditions.Facts
}

// checkAnswers checks every answer of a set on the server, whatever the browser said
// (TPL-320): the rules of its kind from the shared table, and what only the database
// knows, such as whether the creator may see a chosen page or person. A question left
// out takes its default (TPL-316). Every refused answer is named at once (TPL-324).
//
// The server decides again which questions were shown, whatever the browser said (TPL-1406):
// a question whose step or own condition does not hold on the answers before it is hidden,
// and an answer sent for it is dropped unread (TPL-421, TPL-1407).
func (s *Service) checkAnswers(ctx context.Context, c identity.Caller, p *plan, given map[string]string) (answered, error) {
	m := p.version.Manifest
	order := formOrder(m, p.texts)
	form := make([]fields.Spec, len(order))
	for i, a := range order {
		form[i] = a.spec
	}
	form, err := s.withGoneDefaults(ctx, s.pool, form)
	if err != nil {
		return answered{}, err
	}
	facts, err := s.baseFacts(ctx, c, p)
	if err != nil {
		return answered{}, err
	}
	out := answered{fills: map[string]document.Filling{}, titles: map[string]string{}}
	stepShown := map[int]bool{}
	var problems []apperr.FieldProblem
	for i, spec := range form {
		out.asked = append(out.asked, spec.Name)
		step := order[i].step
		shown, decided := stepShown[step]
		if !decided {
			shown = step >= len(m.Steps) || conditions.Holds(m.Steps[step].Condition, facts)
			stepShown[step] = shown
		}
		kind := conditions.Kind(spec.Kind)
		if !shown || !conditions.Holds(spec.Condition, facts) {
			// Never seen, so never answered: empty everywhere it is used.
			out.fills[spec.Name], out.titles[spec.Name] = document.Filling{}, ""
			facts.Answers[spec.Name] = conditions.Answer{Kind: kind}
			continue
		}
		raw, present := given[spec.Name]
		if !present && spec.Default != nil {
			raw, present = spec.DefaultValue(p.today, c.UserID.String()), true
		}
		value, problem := fields.Answer(spec, raw, p.today)
		title := value
		if problem == nil && value != "" {
			title, problem, err = s.lookUp(ctx, c, p, spec, value)
			if err != nil {
				return answered{}, err
			}
		}
		if problem != nil {
			problems = append(problems, fieldProblem(spec.Name, problem))
			facts.Answers[spec.Name] = conditions.Answer{Kind: kind}
			continue
		}
		facts.Answers[spec.Name] = conditions.Answer{Kind: kind, Value: value}
		// A page published at once holds no question left open (TPL-151).
		if present || m.Publish {
			out.fills[spec.Name] = fields.Fill(spec, value)
			out.titles[spec.Name] = titleWords(spec, value, title)
		}
	}
	if len(problems) > 0 {
		return answered{}, apperr.InvalidFields("invalid_answers", problems)
	}
	out.facts = facts.Prepared()
	return out, nil
}

// lookUp checks what only the database knows about an answer, and gives the words a title
// shows for it. A page or a person the creator may not see is "not found", never named.
func (s *Service) lookUp(ctx context.Context, c identity.Caller, p *plan, spec fields.Spec, value string) (string, *fields.Problem, error) {
	switch spec.Kind {
	case fields.KindPage:
		page, err := s.pages.ChosenPage(ctx, c, uuid.MustParse(value))
		if apperr.IsNotFound(err) {
			return "", &fields.Problem{Code: "page_not_found"}, nil
		}
		if err != nil {
			return "", nil, err
		}
		if spec.SpaceID != "" && page.SpaceID.String() != spec.SpaceID {
			return "", &fields.Problem{Code: "page_other_space"}, nil
		}
		return page.Title, nil, nil
	case fields.KindPerson, fields.KindPeople:
		return s.lookUpPeople(ctx, c, strings.Split(value, "\n"))
	case fields.KindLabel:
		refused, err := labels.Refused(ctx, s.pool, p.spaceID, value)
		if err != nil || !refused {
			return value, nil, err
		}
		return "", &fields.Problem{Code: "label_not_allowed"}, nil
	}
	return value, nil, nil
}

// lookUpPeople accepts people the creator may see under the access rule, guests included
// where the rule shows them, and only while they are active (TPL-307, TPL-308).
func (s *Service) lookUpPeople(ctx context.Context, c identity.Caller, values []string) (string, *fields.Problem, error) {
	ids := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		id := uuid.MustParse(value)
		visible, err := s.perms.CheckPerson(ctx, s.pool, c.UserID, id)
		if err != nil {
			return "", nil, err
		}
		if !visible {
			return "", &fields.Problem{Code: "person_not_found"}, nil
		}
		gone, err := s.inactivePeople(ctx, s.pool, []string{value})
		if err != nil {
			return "", nil, err
		}
		if len(gone) > 0 {
			return "", &fields.Problem{Code: "person_inactive"}, nil
		}
		ids = append(ids, id)
	}
	names, err := identity.DisplayNames(ctx, s.pool, ids)
	if err != nil {
		return "", nil, err
	}
	words := make([]string, 0, len(ids))
	for _, id := range ids {
		words = append(words, names[id])
	}
	return strings.Join(words, ", "), nil, nil
}

// titleWords is an answer as a title pattern shows it (TPL-140): the names of people and
// the title of a page, options joined with commas, yes or no in words.
func titleWords(spec fields.Spec, value, looked string) string {
	switch spec.Kind {
	case fields.KindPage, fields.KindPerson, fields.KindPeople:
		return looked
	case fields.KindChoices:
		return strings.Join(strings.Split(value, "\n"), ", ")
	case fields.KindYesNo:
		if value == "yes" {
			return "Yes"
		}
		return "No"
	case fields.KindText:
		return strings.Join(strings.Fields(value), " ")
	}
	return value
}
