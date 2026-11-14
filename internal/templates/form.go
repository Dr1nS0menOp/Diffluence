// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/templates/fields"
)

// The form of a template (spec 074, wave GT2): what each question asks, checked when the
// template is saved and again, with every answer, when it is used.

const maxStepTitleRunes = 100

// reservedNames are the names of the values a template knows without asking (TPL-230).
var reservedNames = []string{document.ValueDate, document.ValueSpace, document.ValueCreator, document.ValueSet,
	document.ValueTemplate}

// specOf is the question a field of a text asks: its settings in the manifest when it has
// some, and otherwise what the text itself says, as every question said before GT2.
func (m Manifest) specOf(f document.Field) fields.Spec {
	for _, s := range m.Fields {
		if s.Name == f.Name {
			return s
		}
	}
	return fields.Spec{Name: f.Name, Kind: fields.Kind(f.Kind), Required: f.Required, Options: f.Options}
}

// formOf lists the questions of a version once each, in the order the set asks them: by
// entry from the root down, then as each text holds them.
func formOf(m Manifest, texts map[string]document.Node) []fields.Spec {
	out := []fields.Spec{}
	seen := map[string]bool{}
	for _, e := range m.ordered() {
		for _, f := range document.Fields(texts[e.Key]) {
			if seen[f.Name] {
				continue
			}
			seen[f.Name] = true
			out = append(out, m.specOf(f))
		}
	}
	return out
}

func fieldProblem(name string, p *fields.Problem) apperr.FieldProblem {
	return apperr.FieldProblem{Field: name, Problem: p.Code, Params: p.Params, Message: p.Message()}
}

// checkSpecs refuses settings that no longer fit, before a version is saved (TPL-316): a
// question no text asks, a reserved name, a setting its kind lacks, a default that is no
// answer, a suggested person who left. Every problem is named at once.
func (s *Service) checkSpecs(ctx context.Context, q db.Querier, m Manifest, texts map[string]document.Node, today string) error {
	if len(m.Fields) == 0 {
		return nil
	}
	asked := map[string]bool{}
	for _, doc := range texts {
		for _, f := range document.Fields(doc) {
			asked[f.Name] = true
		}
	}
	var problems []apperr.FieldProblem
	seen := map[string]bool{}
	for _, spec := range m.Fields {
		var p *fields.Problem
		switch {
		case !asked[spec.Name] || seen[spec.Name]:
			p = &fields.Problem{Code: "not_asked"}
		case slices.Contains(reservedNames, strings.ToLower(spec.Name)):
			p = &fields.Problem{Code: "reserved_name"}
		default:
			p = spec.Check(today)
		}
		seen[spec.Name] = true
		if p == nil && spec.Default != nil && spec.Default.Value != "" && (spec.Kind == fields.KindPerson || spec.Kind == fields.KindPeople) {
			gone, err := s.inactivePeople(ctx, q, strings.Split(spec.Default.Value, "\n"))
			if err != nil {
				return err
			}
			if len(gone) > 0 {
				p = &fields.Problem{Code: "default_inactive"}
			}
		}
		if p != nil {
			problems = append(problems, fieldProblem(spec.Name, p))
		}
	}
	if len(problems) > 0 {
		return apperr.InvalidFields("invalid_fields", problems)
	}
	return nil
}

// inactivePeople lists the ids of people who no longer exist or are no longer active.
func (s *Service) inactivePeople(ctx context.Context, q db.Querier, values []string) ([]string, error) {
	var gone []string
	for _, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			gone = append(gone, value)
			continue
		}
		facts, err := s.perms.LoadUser(ctx, q, id)
		if err != nil {
			return nil, err
		}
		if !facts.Active || facts.Type == identity.TypeService {
			gone = append(gone, value)
		}
	}
	return gone, nil
}

// withGoneDefaults marks a suggested person who is no longer active, so the form shows the
// field empty with a note instead of a person nobody can choose (TPL-316).
func (s *Service) withGoneDefaults(ctx context.Context, q db.Querier, form []fields.Spec) ([]fields.Spec, error) {
	for i, spec := range form {
		if spec.Default == nil || spec.Default.Value == "" || (spec.Kind != fields.KindPerson && spec.Kind != fields.KindPeople) {
			continue
		}
		gone, err := s.inactivePeople(ctx, q, strings.Split(spec.Default.Value, "\n"))
		if err != nil {
			return nil, err
		}
		if len(gone) > 0 {
			form[i].Default, form[i].DefaultGone = nil, true
		}
	}
	return form, nil
}

// checkSteps holds the steps of a form to their rules (ORG-061, TPL-330): at most ten, each
// with a title, and each question in one step at most. A question that no text asks any
// longer may stay named in a step: it shows nowhere until a text asks it again.
func checkSteps(steps []Step) error {
	if len(steps) > maxSteps {
		return apperr.Invalid("A template can have at most ten steps.")
	}
	placed := map[string]bool{}
	for _, st := range steps {
		title := strings.TrimSpace(st.Title)
		if title == "" || utf8.RuneCountInString(title) > maxStepTitleRunes {
			return apperr.Invalid("Each step needs a title of at most 100 characters.")
		}
		for _, name := range st.Fields {
			if placed[name] {
				return apperr.Invalid("The question " + name + " is in two steps. Put it in one.")
			}
			placed[name] = true
		}
	}
	return nil
}

// Form is what a person fills in for a template, as one version asks it.
type Form struct {
	Version int           `json:"version"`
	Fields  []fields.Spec `json:"fields"`
	Steps   []Step        `json:"steps"`
	// Pages is how many pages the version makes.
	Pages int `json:"pages"`
}

// FieldsInput is the settings of every question of a template, from the Form section.
type FieldsInput struct {
	Fields []fields.Spec `json:"fields"`
	Steps  []Step        `json:"steps"`
}

// SetFields saves the settings of the questions and the steps of a template as its next
// version (TPL-1012). A question left out asks as its text says.
func (s *Service) SetFields(ctx context.Context, c identity.Caller, id uuid.UUID, in FieldsInput) (Form, error) {
	if err := checkSteps(in.Steps); err != nil {
		return Form{}, err
	}
	_, err := s.editTemplate(ctx, c, id, func(tx pgx.Tx, t Template, m Manifest) (Manifest, error) {
		texts, _, err := s.textsOf(ctx, tx, m)
		if err != nil {
			return m, err
		}
		if m.Fields, err = beyondTexts(in.Fields, texts); err != nil {
			return m, err
		}
		m.Steps = slices.Clone(in.Steps)
		if m.Steps == nil {
			m.Steps = []Step{}
		}
		// The row keeps the steps too, for the previous release and the gallery (syncRow).
		raw, err := json.Marshal(m.Steps)
		if err != nil {
			return m, err
		}
		_, err = tx.Exec(ctx, "UPDATE templates SET steps = $2 WHERE id = $1", t.ID, raw)
		return m, err
	})
	if err != nil {
		return Form{}, err
	}
	return s.FormOf(ctx, c, id)
}

// beyondTexts keeps the settings that say more than the texts already do. A question whose
// settings are just what its text says needs none, so saving the Form section never turns
// a plain template into a guided one by itself (TPL-050).
func beyondTexts(specs []fields.Spec, texts map[string]document.Node) ([]fields.Spec, error) {
	plain := map[string]fields.Spec{}
	for _, doc := range texts {
		for _, f := range document.Fields(doc) {
			plain[f.Name] = Manifest{}.specOf(f)
		}
	}
	var kept []fields.Spec
	for _, spec := range specs {
		spec.DefaultGone = false
		if own, asked := plain[spec.Name]; asked {
			a, err := json.Marshal(own)
			if err != nil {
				return nil, err
			}
			b, err := json.Marshal(spec)
			if err != nil {
				return nil, err
			}
			if string(a) == string(b) {
				continue
			}
		}
		kept = append(kept, spec)
	}
	return kept, nil
}

// FormOf gives the form a template shows now, for its managers.
func (s *Service) FormOf(ctx context.Context, c identity.Caller, id uuid.UUID) (Form, error) {
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return Form{}, err
	}
	if err := s.requireManager(ctx, s.pool, c, t.SpaceID); err != nil {
		return Form{}, err
	}
	return s.formAt(ctx, c, t, 0)
}

// formAt gives the form of one version of a template, or of the one it offers.
func (s *Service) formAt(ctx context.Context, c identity.Caller, t Template, number int) (Form, error) {
	v, err := s.versionFor(ctx, c, t, number)
	if err != nil {
		return Form{}, err
	}
	texts, _, err := s.textsOf(ctx, s.pool, v.Manifest)
	if err != nil {
		return Form{}, err
	}
	form, err := s.withGoneDefaults(ctx, s.pool, formOf(v.Manifest, texts))
	return Form{Version: v.Number, Fields: form, Steps: v.Manifest.Steps, Pages: len(v.Manifest.Entries)}, err
}
