// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
)

// starter is a template shipped in the binary, in our own words (spec 011, D31).
type starter struct {
	key, name, description, category, pattern string
	labels                                    []string
	build                                     func() document.Node
}

func text(s string) document.Node { return document.Node{Type: document.TypeText, Text: s} }

func para(nodes ...document.Node) document.Node {
	return document.Node{Type: document.TypeParagraph, Content: nodes}
}

func heading(level int, s string) document.Node {
	return document.Node{Type: document.TypeHeading, Attrs: map[string]any{"level": level}, Content: []document.Node{text(s)}}
}

func field(name, kind string) document.Node {
	return document.Node{Type: document.TypeTemplateField, Attrs: map[string]any{"name": name, "kind": kind}}
}

func hint(s string) document.Node {
	return document.Node{Type: document.TypeInstruction, Attrs: map[string]any{"text": s}}
}

func page(blocks ...document.Node) func() document.Node {
	return func() document.Node { return document.Node{Type: document.TypeDoc, Content: blocks} }
}

// section is a heading with a line that says what belongs below it.
func section(title, advice string) []document.Node {
	return []document.Node{heading(2, title), para(hint(advice))}
}

func sections(lead []document.Node, parts ...[]document.Node) func() document.Node {
	blocks := lead
	for _, part := range parts {
		blocks = append(blocks, part...)
	}
	return page(blocks...)
}

var starters = []starter{
	{"meeting-notes", "Meeting notes", "Who was there, what was decided, and who does what next.", "Meetings", "{date} {Topic}", []string{"meeting"},
		sections([]document.Node{para(text("Topic: "), field("Topic", "line")), para(text("Date: "), field("Date", "line"))},
			section("People", "Name who took part."), section("Agenda", "List what you want to talk about."),
			section("Decisions", "Write down what was decided."), section("Actions", "Who does what, and by when."))},
	{"decision-record", "Decision record", "A choice, why it was made, and what follows from it.", "Decisions", "Decision: {Subject}", []string{"decision"},
		sections([]document.Node{para(text("Subject: "), field("Subject", "line"))},
			section("Situation", "What is going on, and why does it need a choice?"), section("Options", "What could you do? Name the good and the bad of each."),
			section("Decision", "What was chosen, by whom, and when?"), section("What follows", "What changes now, and what becomes harder?"))},
	{"project-plan", "Project plan", "The goal, the people and the steps of a piece of work.", "Projects", "Plan: {Project}", []string{"project"},
		sections([]document.Node{para(text("Project: "), field("Project", "line"))},
			section("Goal", "What is different when this is done?"), section("People", "Who does the work, and who decides?"),
			section("Steps", "What happens first, and what depends on what?"), section("Risks", "What could go wrong, and what will you do then?"))},
	{"how-to", "How-to guide", "Steps someone can follow without asking you.", "Guides", "How to {Task}", []string{"how-to"},
		sections([]document.Node{para(text("This explains how to "), field("Task", "line"), text("."))},
			section("Before you start", "What does a reader need first?"), section("Steps", "One step per line, in the right order."),
			section("If it goes wrong", "Name what people often run into, and the way out."))},
	{"incident-review", "Incident review", "What happened, why, and how to keep it from happening again. Blames nobody.", "Reviews", "Incident {date}", []string{"incident"},
		sections(nil, section("What happened", "Tell it plainly, in order, with times."), section("Effect", "Who noticed what, and for how long?"),
			section("Why it happened", "Look for causes in the system, not in people."), section("What we change", "Each change has a name and a date."))},
	{"retrospective", "Retrospective", "What went well, what did not, and what to try next.", "Reviews", "Retrospective {date}", []string{"retrospective"},
		sections(nil, section("What went well", "Keep doing these."), section("What did not", "Be specific."), section("What we try next", "Pick a few, and name who starts."))},
	{"requirements", "Requirements", "What something must do, and how you will know it does.", "Projects", "Requirements: {Subject}", []string{"requirements"},
		sections([]document.Node{para(text("Subject: "), field("Subject", "line"))},
			section("Problem", "Who has which problem today?"), section("Must have", "What is needed for this to count as done?"),
			section("Left out", "What is not part of this, on purpose?"), section("How we test it", "How will you see that it works?"))},
	{"onboarding-plan", "Onboarding plan", "The first weeks of someone new.", "People", "Onboarding {Name}", []string{"onboarding"},
		sections([]document.Node{para(text("For: "), field("Name", "line"))},
			section("First day", "Accounts, a place to sit, and a person to ask."), section("First week", "Who should they meet, and what should they read?"),
			section("First month", "What should they be able to do by themselves by now?"))},
	{"team-home", "Team home", "Who a team is, what it does, and where things are.", "Teams", "{Team}", []string{"team"},
		sections([]document.Node{para(text("Welcome to "), field("Team", "line"), text("."))},
			section("What we do", "Say it so someone from outside understands."), section("Who we are", "Names, and what each person is there for."),
			section("How to reach us", "Where do people ask a question?"), section("Where things are", "Links to what people look for most."))},
	{"weekly-update", "Weekly update", "What happened, what comes next, and where help is needed.", "Updates", "Week of {date}", []string{"update"},
		sections(nil, section("Done", "What was finished this week?"), section("Next", "What comes next?"), section("Help needed", "Where are you stuck?"))},
}

// seed adds the starter set the first time someone looks. A unique key keeps two
// nodes from adding it twice, and an edited starter is left alone.
func (s *Service) seed(ctx context.Context) error {
	if s.seeded.Load() {
		return nil
	}
	c := identity.SystemCaller("templates")
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, st := range starters {
			var known bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM templates WHERE starter_key = $1)", st.key).Scan(&known); err != nil {
				return err
			}
			if known {
				continue
			}
			key := st.key
			in := Input{Name: st.name, Description: st.description, Category: st.category, Labels: st.labels, TitlePattern: st.pattern, Steps: []Step{}}
			if _, err := s.insert(ctx, tx, c, nil, in, st.build(), &key); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.seeded.Store(true)
	return nil
}

// SpaceTemplate is a way to start a space.
type SpaceTemplate struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Starter     bool      `json:"starter"`
}

// BundlePage is one page of a space template. Parent is the index of an earlier page.
type BundlePage struct {
	Title    string          `json:"title"`
	Parent   int             `json:"parent"`
	Labels   []string        `json:"labels"`
	Document json.RawMessage `json:"document"`
}

type starterSpace struct {
	key, name, description string
	pages                  []string
}

var starterSpaces = []starterSpace{
	{"team", "Team space", "A home for a team: how it works, meetings and decisions.", []string{"How we work", "Meetings", "Decisions"}},
	{"project", "Project space", "A place for one piece of work: plan, requirements and reviews.", []string{"Plan", "Requirements", "Reviews"}},
}
