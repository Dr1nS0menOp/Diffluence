// SPDX-License-Identifier: Apache-2.0

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
)

// runContext is what a run knows when it starts. It holds ids only: names are looked
// up for whoever reads the log (GAP-009).
type runContext struct {
	PageID  *uuid.UUID  `json:"page_id,omitempty"`
	SpaceID *uuid.UUID  `json:"space_id,omitempty"`
	ActorID *uuid.UUID  `json:"actor_id,omitempty"`
	Chain   []uuid.UUID `json:"chain,omitempty"`
	Trigger string      `json:"trigger"`
}

var eventsToTriggers = map[string][]string{
	"page.moved": {"page.moved"}, "page.copied": {"page.copied"}, "page.archived": {"page.archived"},
	"page.trashed": {"page.deleted"}, "comment.created": {"page.commented", "blog.commented"},
	"page.labelled": {"page.labelled", "blog.labelled"}, "page.stage_changed": {"page.status_changed"},
	"page.owner_changed": {"page.owner_changed"}, "attachment.uploaded": {"attachment.added"},
	"attachment.trashed": {"attachment.removed"}, "task.done": {"task.ticked"},
	"group.member_added": {"group.member_added"}, "group.member_removed": {"group.member_removed"},
	"space.created": {"space.created"}, "space.archived": {"space.archived"},
}

func hasEntries(payload map[string]any, key string) bool {
	list, ok := payload[key].([]any)
	return ok && len(list) > 0
}

// triggersFor sorts one event to the triggers it can start.
func triggersFor(eventType string, payload map[string]any) []string {
	out := slices.Clone(eventsToTriggers[eventType])
	if eventType == "page.published" {
		if version, ok := payload["version"].(float64); ok && version > 1 {
			out = append(out, "page.edited")
		} else {
			out = append(out, "page.published", "blog.published")
		}
	}
	if eventType == "page.published" || eventType == "comment.created" {
		if hasEntries(payload, "mentions") {
			out = append(out, "person.mentioned")
		}
	}
	if eventType == "page.published" && hasEntries(payload, "assigned") {
		out = append(out, "task.created")
	}
	return out
}

// postOnly and pageOnly keep blog triggers and page triggers apart. The kind is
// looked at when the run starts, because a comment event does not carry it.
func wantsKind(trigger, kind string) bool {
	if len(trigger) > 5 && trigger[:5] == "blog." {
		return kind == "post"
	}
	if len(trigger) > 5 && trigger[:5] == "page." {
		return kind != "post"
	}
	return true
}

func uuidField(payload map[string]any, key string) *uuid.UUID {
	raw, ok := payload[key].(string)
	if !ok {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &id
}

func chainOf(payload map[string]any) []uuid.UUID {
	list, ok := payload["rule_chain"].([]any)
	if !ok {
		return nil
	}
	out := make([]uuid.UUID, 0, len(list))
	for _, v := range list {
		if raw, ok := v.(string); ok {
			if id, err := uuid.Parse(raw); err == nil {
				out = append(out, id)
			}
		}
	}
	return out
}

// Handle is the consumer of the event stream. A redelivered event finds its runs
// already there, so it changes nothing.
func (s *Service) Handle(ctx context.Context, data []byte) error {
	var m outbox.Message
	if err := json.Unmarshal(data, &m); err != nil {
		// A message that cannot be read never will be: redelivery would only repeat it.
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(m.Payload, &payload); err != nil {
		return nil
	}
	for _, trigger := range triggersFor(m.Type, payload) {
		if err := s.matchTrigger(ctx, trigger, m.ID, payload); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) matchTrigger(ctx context.Context, trigger string, event uuid.UUID, payload map[string]any) error {
	space := uuidField(payload, "space_id")
	rows, err := s.Pool.Query(ctx, "SELECT "+ruleColumns+`WHERE r.enabled AND r.paused_at IS NULL AND r.trigger = $1
		AND (r.space_id IS NULL OR r.space_id = $2)`, trigger, space)
	if err != nil {
		return err
	}
	rules, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Rule, error) { return scanRule(row) })
	if err != nil {
		return err
	}
	rc := runContext{PageID: uuidField(payload, "page_id"), SpaceID: space, ActorID: uuidField(payload, "actor_id"),
		Chain: chainOf(payload), Trigger: trigger}
	for _, r := range rules {
		if r.SpaceID != nil && slices.Contains(instanceOnlyTriggers, trigger) {
			continue
		}
		if err := s.start(ctx, r, event, rc); err != nil {
			return err
		}
	}
	return nil
}

// spaceAllowsRules reads the switch of the space. An instance rule follows the
// switch of the space of its page.
func (s *Service) spaceAllowsRules(ctx context.Context, rc runContext) (bool, error) {
	if rc.SpaceID == nil {
		return true, nil
	}
	return s.Features.Enabled(ctx, s.Pool, *rc.SpaceID, "automation")
}

// start records a run, or a skipped one, and queues the job. UNIQUE (rule, event)
// keeps a redelivered event from running a rule twice.
func (s *Service) start(ctx context.Context, r Rule, event uuid.UUID, rc runContext) error {
	allowed, err := s.spaceAllowsRules(ctx, rc)
	if err != nil || !allowed {
		return err
	}
	note := ""
	switch {
	case slices.Contains(rc.Chain, r.ID):
		note = "own_chain"
	case len(rc.Chain) >= maxChain:
		note = "chain_too_long"
	}
	raw, err := json.Marshal(rc)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if note != "" {
			_, err := tx.Exec(ctx, `INSERT INTO automation_runs (id, rule_id, event_id, state, context, report, finished_at)
				VALUES ($1, $2, $3, 'skipped', $4, $5, now()) ON CONFLICT (rule_id, event_id) DO NOTHING`, ids.New(), r.ID, event, raw, note)
			return err
		}
		over, err := s.overLimit(ctx, tx, r)
		if err != nil || over {
			return err
		}
		id := ids.New()
		tag, err := tx.Exec(ctx, `INSERT INTO automation_runs (id, rule_id, event_id, state, context) VALUES ($1, $2, $3, 'queued', $4)
			ON CONFLICT (rule_id, event_id) DO NOTHING`, id, r.ID, event, raw)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		_, err = s.Jobs.Enqueue(ctx, tx, JobRun, map[string]any{"run_id": id}, r.AccountID)
		return err
	})
}

// overLimit pauses a rule at its limit and tells the owner. Runs that were skipped
// did nothing, so they do not count.
func (s *Service) overLimit(ctx context.Context, tx pgx.Tx, r Rule) (bool, error) {
	limit, err := runsPerHour(ctx, tx)
	if err != nil {
		return false, err
	}
	var runs int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM automation_runs
		WHERE rule_id = $1 AND created_at > now() - interval '1 hour' AND state <> 'skipped'`, r.ID).Scan(&runs); err != nil {
		return false, err
	}
	if runs < limit {
		return false, nil
	}
	if _, err := tx.Exec(ctx, "UPDATE automation_rules SET paused_at = now() WHERE id = $1", r.ID); err != nil {
		return true, err
	}
	if err := s.recordSystem(ctx, tx, "rule.paused", r, map[string]any{"limit": limit}); err != nil {
		return true, err
	}
	return true, s.tellOwner(ctx, tx, r, "paused", nil)
}

func (s *Service) recordSystem(ctx context.Context, tx pgx.Tx, action string, r Rule, details map[string]any) error {
	space := ""
	if r.SpaceID != nil {
		space = r.SpaceID.String()
	}
	details["name"] = r.Name
	if _, err := audit.Record(ctx, tx, identity.SystemCaller("automation").Event(action, "rule", r.ID.String(), space, details)); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"rule_id": r.ID, "space_id": r.SpaceID})
	return err
}

// RunByHand starts a rule with a manual trigger, for someone who can see the page.
func (s *Service) RunByHand(ctx context.Context, c identity.Caller, id, pageID uuid.UUID) error {
	r, err := s.load(ctx, s.Pool, id)
	if err != nil {
		return err
	}
	if err := s.Perms.RequirePage(ctx, s.Pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return err
	}
	var space uuid.UUID
	if err := s.Pool.QueryRow(ctx, "SELECT space_id FROM pages WHERE id = $1", pageID).Scan(&space); err != nil {
		return err
	}
	if r.Definition.Trigger.Kind != TriggerManual {
		// Any other rule is for those who manage it.
		if err := s.mayManage(ctx, c, r.SpaceID); err != nil {
			return err
		}
	}
	if r.SpaceID != nil && *r.SpaceID != space {
		return apperr.NotFound()
	}
	if !r.Enabled || r.PausedAt != nil {
		return apperr.Conflict("rule_off", "This rule is off.")
	}
	return s.start(ctx, r, ids.New(), runContext{PageID: &pageID, SpaceID: &space, ActorID: &c.UserID, Trigger: TriggerManual})
}

// Incoming starts the rule that owns a token. The token is the permission, so an
// unknown one looks exactly like an address that does not exist.
func (s *Service) Incoming(ctx context.Context, token string) error {
	var id uuid.UUID
	err := s.Pool.QueryRow(ctx, "SELECT rule_id FROM automation_hooks WHERE token_hash = $1", hashToken(token)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.NotFound()
	}
	if err != nil {
		return err
	}
	r, err := s.load(ctx, s.Pool, id)
	if err != nil {
		return err
	}
	if !r.Enabled || r.PausedAt != nil {
		return apperr.NotFound()
	}
	return s.start(ctx, r, ids.New(), runContext{SpaceID: r.SpaceID, Trigger: TriggerIncoming})
}

// RunSchedules starts what is due. The event id follows from the rule and the moment,
// so two workers can never start the same moment twice.
func (s *Service) RunSchedules(ctx context.Context) error {
	rows, err := s.Pool.Query(ctx, "SELECT "+ruleColumns+`WHERE r.enabled AND r.paused_at IS NULL AND r.trigger = $1 AND r.next_run_at <= $2`,
		TriggerSchedule, s.now())
	if err != nil {
		return err
	}
	due, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Rule, error) { return scanRule(row) })
	if err != nil {
		return err
	}
	for _, r := range due {
		moment := r.nextRunAt.UTC().Format(time.RFC3339)
		event := uuid.NewSHA1(r.ID, []byte(moment))
		if err := s.start(ctx, r, event, runContext{SpaceID: r.SpaceID, Trigger: TriggerSchedule}); err != nil {
			return err
		}
		if _, err := s.Pool.Exec(ctx, "UPDATE automation_rules SET next_run_at = $2 WHERE id = $1", r.ID, nextRun(r.Definition.Trigger, s.now())); err != nil {
			return err
		}
	}
	return nil
}
