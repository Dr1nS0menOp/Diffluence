// SPDX-License-Identifier: Apache-2.0

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/notify"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Step is one line of a run. It holds ids and codes: what a page is called is looked
// up for whoever reads it, so the log never tells someone a title they may not see.
type Step struct {
	Key    string     `json:"key"`
	Action string     `json:"action"`
	State  string     `json:"state"`
	Note   string     `json:"note,omitempty"`
	PageID *uuid.UUID `json:"page_id,omitempty"`
}

// The states of a step.
const (
	stepDone    = "done"
	stepSkipped = "skipped"
	stepFailed  = "failed"
	stepWould   = "would"
)

// run is one execution of a rule.
type run struct {
	id      uuid.UUID
	rule    Rule
	context runContext
	steps   []Step
	caller  identity.Caller
	dry     bool
	// actions counts what really ran, against the limit of 50.
	actions int
	found   []uuid.UUID
}

func (r *run) finished(key string) bool {
	for _, st := range r.steps {
		if st.Key == key && st.State == stepDone {
			return true
		}
	}
	return false
}

func (s *Service) runJob(ctx context.Context, j jobs.Job) (any, error) {
	var in struct {
		RunID uuid.UUID `json:"run_id"`
	}
	if err := json.Unmarshal(j.Payload, &in); err != nil {
		return nil, fmt.Errorf("automation: read job: %v", err)
	}
	return nil, s.Execute(ctx, in.RunID)
}

func (s *Service) loadRun(ctx context.Context, id uuid.UUID) (*run, string, error) {
	r := &run{id: id}
	var state string
	var rawContext, rawSteps []byte
	var ruleID uuid.UUID
	err := s.Pool.QueryRow(ctx, "SELECT rule_id, state, context, steps FROM automation_runs WHERE id = $1", id).
		Scan(&ruleID, &state, &rawContext, &rawSteps)
	if err != nil {
		return nil, "", err
	}
	if err := json.Unmarshal(rawContext, &r.context); err != nil {
		return nil, "", err
	}
	if err := json.Unmarshal(rawSteps, &r.steps); err != nil {
		return nil, "", err
	}
	if r.rule, err = s.load(ctx, s.Pool, ruleID); err != nil {
		return nil, "", err
	}
	return r, state, nil
}

func (r *run) callerFor() identity.Caller {
	return identity.Caller{UserID: r.rule.AccountID, Rule: &identity.RuleRef{ID: r.rule.ID, Name: r.rule.Name,
		Chain: append(append([]uuid.UUID{}, r.context.Chain...), r.rule.ID)}}
}

// Execute does one run. It can run again after a crash: steps that finished are stored
// and are not repeated.
func (s *Service) Execute(ctx context.Context, id uuid.UUID) error {
	r, state, err := s.loadRun(ctx, id)
	if err != nil {
		return err
	}
	if state == "done" || state == "skipped" {
		return nil
	}
	r.caller = r.callerFor()
	if _, err := s.Pool.Exec(ctx, "UPDATE automation_runs SET state = 'running' WHERE id = $1", id); err != nil {
		return err
	}
	report, err := s.perform(ctx, r)
	if err == nil {
		return s.finish(ctx, r, report)
	}
	return s.fail(ctx, r, err)
}

func (s *Service) finish(ctx context.Context, r *run, report string) error {
	state := "done"
	if report != "" {
		state = "skipped"
	}
	_, err := s.Pool.Exec(ctx, "UPDATE automation_runs SET state = $2, report = $3, steps = $4, finished_at = now() WHERE id = $1",
		r.id, state, report, stepsJSON(r.steps))
	return err
}

func stepsJSON(steps []Step) []byte {
	if steps == nil {
		return []byte("[]")
	}
	raw, err := json.Marshal(steps)
	if err != nil {
		return []byte("[]") // a Step holds strings and ids, which always marshal
	}
	return raw
}

// fail records what went wrong without its wording, which can hold a title, and tells
// the owner at most once an hour.
func (s *Service) fail(ctx context.Context, r *run, cause error) error {
	code := "failed"
	if e, ok := apperr.As(cause); ok {
		code = e.Code
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE automation_runs SET state = 'failed', report = $2, steps = $3, finished_at = now() WHERE id = $1",
			r.id, code, stepsJSON(r.steps)); err != nil {
			return err
		}
		if r.rule.toldAt != nil && s.now().Sub(*r.rule.toldAt) < failureNoticeSpacer {
			return nil
		}
		if _, err := tx.Exec(ctx, "UPDATE automation_rules SET told_at = $2 WHERE id = $1", r.rule.ID, s.now()); err != nil {
			return err
		}
		return s.tellOwner(ctx, tx, r.rule, "failed", r.context.PageID)
	})
}

// tellOwner leaves a notification with the owner of the rule. It needs a page to hang
// on: the given one, or the home page of the space. An instance rule without either
// shows its state on its own screen only.
func (s *Service) tellOwner(ctx context.Context, tx pgx.Tx, r Rule, what string, pageID *uuid.UUID) error {
	row := notify.Row{UserID: r.OwnerID, Type: notify.TypeRule, Reason: what, NoCheck: true,
		Event: uuid.NewSHA1(r.ID, []byte(what+s.now().Format(time.RFC3339))), Data: map[string]any{"rule": r.Name, "what": what}}
	if pageID == nil && r.SpaceID != nil {
		var home *uuid.UUID
		if err := tx.QueryRow(ctx, "SELECT home_page_id FROM spaces WHERE id = $1", *r.SpaceID).Scan(&home); err != nil {
			return err
		}
		pageID = home
	}
	if pageID == nil {
		return nil
	}
	row.PageID = *pageID
	if err := tx.QueryRow(ctx, "SELECT space_id FROM pages WHERE id = $1", *pageID).Scan(&row.SpaceID); errors.Is(err, pgx.ErrNoRows) {
		return nil // the page went with the run
	} else if err != nil {
		return err
	}
	return s.Notify.Deliver(ctx, tx, row)
}

// perform checks what must hold before anything happens, and answers with a reason
// when the run does nothing.
func (s *Service) perform(ctx context.Context, r *run) (string, error) {
	if !r.rule.Enabled {
		return "rule_off", nil
	}
	checked, err := s.withProblems(ctx, r.rule)
	if err != nil {
		return "", err
	}
	if len(checked.Problems) > 0 {
		return "rule_has_problems", nil
	}
	active, err := s.accountActive(ctx, r.rule)
	if err != nil {
		return "", err
	}
	if !active {
		return "account_disabled", s.stopForDisabledOwner(ctx, r.rule)
	}
	if r.context.PageID != nil {
		f, reason, err := s.factsFor(ctx, r, *r.context.PageID)
		if err != nil || reason != "" {
			return reason, err
		}
		if !r.rule.Definition.Conditions.holds(f, s.now()) {
			return "conditions_not_met", nil
		}
	}
	return "", s.runSteps(ctx, r)
}

func (s *Service) accountActive(ctx context.Context, rule Rule) (bool, error) {
	var active bool
	err := s.Pool.QueryRow(ctx, `SELECT count(*) = 2 FROM users WHERE id = ANY($1) AND status = 'active'`,
		[]uuid.UUID{rule.OwnerID, rule.AccountID}).Scan(&active)
	if rule.OwnerID == rule.AccountID {
		err = s.Pool.QueryRow(ctx, "SELECT status = 'active' FROM users WHERE id = $1", rule.OwnerID).Scan(&active)
	}
	return active, err
}

// stopForDisabledOwner turns the rule off, and tells those who run the space (H5).
func (s *Service) stopForDisabledOwner(ctx context.Context, rule Rule) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE automation_rules SET enabled = false, updated_at = now() WHERE id = $1", rule.ID); err != nil {
			return err
		}
		if err := s.recordSystem(ctx, tx, "rule.stopped", rule, map[string]any{"reason": "account_disabled"}); err != nil {
			return err
		}
		if rule.SpaceID == nil {
			return nil
		}
		rows, err := tx.Query(ctx, `SELECT DISTINCT g.subject_id FROM space_grants g JOIN users u ON u.id = g.subject_id
			WHERE g.space_id = $1 AND g.permission = $2 AND g.subject_type = 'user' AND u.status = 'active'`, *rule.SpaceID, permissions.SpaceAdminister)
		if err != nil {
			return err
		}
		admins, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		for _, admin := range admins {
			told := rule
			told.OwnerID = admin
			if err := s.tellOwner(ctx, tx, told, "stopped", nil); err != nil {
				return err
			}
		}
		return nil
	})
}

// factsFor loads a page as the account sees it (D11). A page the account cannot see
// ends the run, so a rule can never learn about a page through a condition.
func (s *Service) factsFor(ctx context.Context, r *run, pageID uuid.UUID) (facts, string, error) {
	var f facts
	d, err := s.Pages.Get(ctx, r.caller, pageID)
	if apperr.IsNotFound(err) {
		var exists bool
		if err := s.Pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pages WHERE id = $1 AND status <> 'trashed')", pageID).Scan(&exists); err != nil {
			return f, "", err
		}
		if !exists {
			return f, "gone", nil
		}
		return f, "not_visible", nil
	}
	if e, ok := apperr.As(err); ok && e.Code == apperr.Forbidden().Code {
		return f, "not_visible", nil
	}
	if err != nil {
		return f, "", err
	}
	if !wantsKind(r.context.Trigger, d.Kind) {
		return f, "other_kind", nil
	}
	f = facts{spaceID: d.SpaceID, labels: d.Labels, status: d.StageText, title: d.Title, authorID: d.CreatedBy, createdAt: d.CreatedAt}
	f.text, err = plainText(d.Document)
	if err != nil {
		return f, "", err
	}
	if err := s.relations(ctx, &f, pageID); err != nil {
		return f, "", err
	}
	return f, "", nil
}

func (s *Service) relations(ctx context.Context, f *facts, pageID uuid.UUID) error {
	rows, err := s.Pool.Query(ctx, "SELECT ancestor_id FROM page_tree WHERE descendant_id = $1 AND depth > 0", pageID)
	if err != nil {
		return err
	}
	if f.ancestors, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID]); err != nil {
		return err
	}
	rows, err = s.Pool.Query(ctx, "SELECT group_id FROM group_members WHERE user_id = $1", f.authorID)
	if err != nil {
		return err
	}
	f.groups, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	return err
}

func plainText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	n, err := document.Parse(raw)
	if err != nil {
		return "", err
	}
	text := ""
	document.Walk(n, func(node document.Node) { text += node.Text + " " })
	return text, nil
}

func (s *Service) runSteps(ctx context.Context, r *run) error {
	page := r.context.PageID
	for i, a := range r.rule.Definition.Actions {
		if a.Type == ActFind {
			if err := s.find(ctx, r, a); err != nil {
				return err
			}
			r.note(Step{Key: strconv.Itoa(i), Action: a.Type, State: stepDone, Note: strconv.Itoa(len(r.found)) + " found"})
			continue
		}
		targets := []*uuid.UUID{page}
		if r.found != nil && needsPage(a.Type) {
			targets = targets[:0]
			for k := range r.found {
				targets = append(targets, &r.found[k])
			}
		}
		for _, target := range targets {
			if err := s.oneStep(ctx, r, i, a, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *run) note(st Step) {
	if r.finished(st.Key) {
		return
	}
	r.steps = append(r.steps, st)
}

func (s *Service) saveSteps(ctx context.Context, r *run) error {
	if r.dry {
		return nil
	}
	_, err := s.Pool.Exec(ctx, "UPDATE automation_runs SET steps = $2 WHERE id = $1", r.id, stepsJSON(r.steps))
	return err
}

func stepKey(index int, page *uuid.UUID) string {
	if page == nil {
		return strconv.Itoa(index)
	}
	return strconv.Itoa(index) + ":" + page.String()
}

// oneStep does an action on one page, and stores that it did before going on, so a
// retry knows where it was.
func (s *Service) oneStep(ctx context.Context, r *run, index int, a Action, page *uuid.UUID) error {
	key := stepKey(index, page)
	if r.finished(key) {
		return nil
	}
	r.actions++
	if r.actions > maxActions {
		r.note(Step{Key: key, Action: a.Type, State: stepSkipped, Note: "limit_of_actions", PageID: page})
		return s.saveSteps(ctx, r)
	}
	st := Step{Key: key, Action: a.Type, State: stepDone, PageID: page}
	note, err := s.act(ctx, r, a, page)
	switch {
	case err == nil && r.dry:
		st.State, st.Note = stepWould, note
	case err == nil && note != "":
		st.State, st.Note = stepSkipped, note
	case err != nil:
		st.State = stepFailed
		if e, ok := apperr.As(err); ok {
			st.Note = e.Code
		}
	}
	r.steps = append(r.steps, st)
	if saveErr := s.saveSteps(ctx, r); saveErr != nil {
		return saveErr
	}
	return err
}

// find runs a lookup as the account: pages it cannot see do not exist for it.
func (s *Service) find(ctx context.Context, r *run, a Action) error {
	var before *time.Time
	if a.OlderDays > 0 {
		at := s.now().AddDate(0, 0, -a.OlderDays)
		before = &at
	}
	rows, err := s.Pool.Query(ctx, `SELECT p.id FROM pages p JOIN perm_visible_pages($1) v ON v.page_id = p.id
		WHERE p.status = 'published' AND ($2::uuid IS NULL OR p.space_id = $2)
			AND ($3 = '' OR EXISTS (SELECT 1 FROM page_labels l WHERE l.page_id = p.id AND l.label = lower($3)))
			AND ($4 = '' OR p.title ILIKE '%' || $4 || '%')
			AND ($5::timestamptz IS NULL OR p.updated_at < $5)
		ORDER BY p.updated_at, p.id LIMIT $6`, r.rule.AccountID, r.rule.SpaceID, a.Label, a.Title, before, maxLookupPages)
	if err != nil {
		return err
	}
	r.found, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if r.found == nil {
		r.found = []uuid.UUID{}
	}
	return err
}

// TestResult is what a test run would do.
type TestResult struct {
	Outcome string `json:"outcome"`
	Steps   []Step `json:"steps"`
	// Warnings say what a person should think about, such as a rule that writes into
	// a page others can read (GAP-016).
	Warnings []string `json:"warnings"`
}

// Test says what a rule would do for a page, and changes nothing.
func (s *Service) Test(ctx context.Context, c identity.Caller, id uuid.UUID, pageID *uuid.UUID) (TestResult, error) {
	rule, err := s.Get(ctx, c, id)
	out := TestResult{Steps: []Step{}, Warnings: []string{}}
	if err != nil {
		return out, err
	}
	if len(rule.Problems) > 0 {
		out.Outcome = "rule_has_problems"
		return out, nil
	}
	r := &run{rule: rule, dry: true, context: runContext{PageID: pageID, Chain: nil, Trigger: rule.Definition.Trigger.Kind}}
	r.caller = r.callerFor()
	if pageID != nil {
		f, reason, err := s.factsFor(ctx, r, *pageID)
		if err != nil {
			return out, err
		}
		if reason == "" && !rule.Definition.Conditions.holds(f, s.now()) {
			reason = "conditions_not_met"
		}
		if reason != "" {
			out.Outcome = reason
			return out, nil
		}
	}
	if err := s.runSteps(ctx, r); err != nil {
		return out, err
	}
	out.Outcome, out.Steps = "would_run", r.steps
	for _, a := range rule.Definition.Actions {
		if a.Type == ActComment || a.Type == ActTask || a.Type == ActTemplate {
			out.Warnings = append(out.Warnings, "writes_into_page")
			break
		}
	}
	return out, nil
}

// RunRow is one line of the log, as the person looking may see it.
type RunRow struct {
	ID        uuid.UUID  `json:"id"`
	State     string     `json:"state"`
	Report    string     `json:"report"`
	Trigger   string     `json:"trigger"`
	PageTitle *string    `json:"page_title"`
	Steps     []Step     `json:"steps"`
	CreatedAt time.Time  `json:"created_at"`
	Finished  *time.Time `json:"finished_at"`
}

// Runs reads the log. Ids turn into titles here, for the person looking (GAP-009).
func (s *Service) Runs(ctx context.Context, c identity.Caller, id uuid.UUID) ([]RunRow, error) {
	if _, err := s.Get(ctx, c, id); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT x.id, x.state, x.report, x.context, x.steps, x.created_at, x.finished_at
		FROM automation_runs x WHERE x.rule_id = $1 ORDER BY x.created_at DESC LIMIT 100`, id)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (RunRow, error) {
		var v RunRow
		var rawContext, rawSteps []byte
		var rc runContext
		if err := row.Scan(&v.ID, &v.State, &v.Report, &rawContext, &rawSteps, &v.CreatedAt, &v.Finished); err != nil {
			return v, err
		}
		if err := json.Unmarshal(rawContext, &rc); err != nil {
			return v, err
		}
		v.Trigger = rc.Trigger
		if rc.PageID != nil {
			v.PageTitle = s.titleFor(ctx, c, *rc.PageID)
		}
		return v, json.Unmarshal(rawSteps, &v.Steps)
	})
	if list == nil {
		list = []RunRow{}
	}
	return list, err
}

// titleFor answers nil for a page this person may not see, or that is gone.
func (s *Service) titleFor(ctx context.Context, c identity.Caller, pageID uuid.UUID) *string {
	d, err := s.Perms.CheckPage(ctx, s.Pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil)
	if err != nil || !d.Allowed {
		return nil
	}
	var title string
	if err := s.Pool.QueryRow(ctx, "SELECT title FROM pages WHERE id = $1", pageID).Scan(&title); err != nil {
		return nil
	}
	return &title
}
