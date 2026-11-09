// SPDX-License-Identifier: Apache-2.0

package automation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/comments"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/notify"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/outward"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/templates"
	"github.com/diffluence/diffluence/internal/watches"
)

const (
	// JobRun is the job kind of one run.
	JobRun = "automation.run"

	settingsKey         = "automation"
	defaultRunsPerHour  = 100
	maxRunsPerHour      = 10000
	maxLookupPages      = 100
	maxChain            = 5
	runLogDays          = 30
	maxNameRunes        = 200
	hookTokenBytes      = 32
	maxRulesListed      = 500
	maxRuleSpaces       = 500
	failureNoticeSpacer = time.Hour
)

// Deps are the modules a rule acts through.
type Deps struct {
	Pool      *pgxpool.Pool
	Perms     *permissions.Service
	Identity  *identity.Service
	Pages     *pages.Service
	Comments  *comments.Service
	Watches   *watches.Service
	Templates *templates.Service
	Notify    *notify.Service
	Jobs      *jobs.Service
	Features  *features.Service
	Outward   *outward.Service
	Origin    string
}

// Service keeps rules and runs them.
type Service struct {
	Deps
	now func() time.Time
}

// NewService creates the service, and registers the job that runs a rule.
func NewService(d Deps) *Service {
	s := &Service{Deps: d, now: time.Now}
	d.Jobs.Register(JobRun, s.runJob)
	return s
}

// Rule is one rule.
type Rule struct {
	ID         uuid.UUID  `json:"id"`
	SpaceID    *uuid.UUID `json:"space_id"`
	Name       string     `json:"name"`
	Enabled    bool       `json:"enabled"`
	Definition Definition `json:"definition"`
	OwnerID    uuid.UUID  `json:"owner_id"`
	OwnerName  string     `json:"owner_name"`
	AccountID  uuid.UUID  `json:"account_id"`
	PausedAt   *time.Time `json:"paused_at"`
	LastRunAt  *time.Time `json:"last_run_at"`
	LastState  string     `json:"last_state"`
	HasHook    bool       `json:"has_hook"`
	Problems   []Problem  `json:"problems"`
	nextRunAt  *time.Time
	toldAt     *time.Time
}

// Input is what a person sends to make or change a rule.
type Input struct {
	Name       string     `json:"name"`
	Enabled    bool       `json:"enabled"`
	Definition Definition `json:"definition"`
	AccountID  *uuid.UUID `json:"account_id"`
}

const ruleColumns = `r.id, r.space_id, r.name, r.enabled, r.definition, r.owner_id, COALESCE(o.display_name, ''), r.account_id,
	r.paused_at, r.next_run_at, r.told_at,
	(SELECT max(x.created_at) FROM automation_runs x WHERE x.rule_id = r.id),
	COALESCE((SELECT x.state FROM automation_runs x WHERE x.rule_id = r.id ORDER BY x.created_at DESC LIMIT 1), ''),
	EXISTS (SELECT 1 FROM automation_hooks h WHERE h.rule_id = r.id)
	FROM automation_rules r LEFT JOIN users o ON o.id = r.owner_id `

func scanRule(row pgx.Row) (Rule, error) {
	var r Rule
	var definition []byte
	err := row.Scan(&r.ID, &r.SpaceID, &r.Name, &r.Enabled, &definition, &r.OwnerID, &r.OwnerName, &r.AccountID,
		&r.PausedAt, &r.nextRunAt, &r.toldAt, &r.LastRunAt, &r.LastState, &r.HasHook)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, apperr.NotFound()
	}
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(definition, &r.Definition)
}

func (s *Service) load(ctx context.Context, q db.Querier, id uuid.UUID) (Rule, error) {
	return scanRule(q.QueryRow(ctx, "SELECT "+ruleColumns+"WHERE r.id = $1", id))
}

// mayManage is the rule on rules: automation on the space (spec 036), or administrator
// for an instance rule (spec 015, H5).
func (s *Service) mayManage(ctx context.Context, c identity.Caller, spaceID *uuid.UUID) error {
	if spaceID != nil {
		return s.Perms.RequireSpace(ctx, s.Pool, c.UserID, *spaceID, permissions.SpaceAutomation)
	}
	allowed, err := s.Identity.HasGlobalPermission(ctx, c.UserID, identity.PermAdministrator)
	if err != nil {
		return err
	}
	if !allowed {
		return apperr.Forbidden()
	}
	return nil
}

func (s *Service) withProblems(ctx context.Context, r Rule) (Rule, error) {
	r.Problems = r.Definition.Problems()
	gone, err := s.goneThings(ctx, r)
	r.Problems = append(r.Problems, gone...)
	return r, err
}

// goneThings finds groups and hosts a rule still names, so it shows on the rule and
// stops it from running (H5).
func (s *Service) goneThings(ctx context.Context, r Rule) ([]Problem, error) {
	out := []Problem{}
	for i, a := range r.Definition.Actions {
		if a.Type == ActWebhook || a.Type == ActChat {
			if _, err := s.Outward.Target(ctx, a.Address); err != nil {
				e, ok := apperr.As(err)
				if !ok {
					return nil, err
				}
				out = append(out, Problem{Step: i, Message: e.Message})
			}
		}
		if a.To != ToGroup && a.Type != ActRestrict || a.TargetID == nil {
			continue
		}
		var found bool
		if err := s.Pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM groups WHERE id = $1)", *a.TargetID).Scan(&found); err != nil {
			return nil, err
		}
		if !found {
			out = append(out, Problem{Step: i, Message: "This group no longer exists."})
		}
	}
	return out, nil
}

// Get returns a rule for someone who may manage it.
func (s *Service) Get(ctx context.Context, c identity.Caller, id uuid.UUID) (Rule, error) {
	r, err := s.load(ctx, s.Pool, id)
	if err != nil {
		return r, err
	}
	if err := s.mayManage(ctx, c, r.SpaceID); err != nil {
		return r, err
	}
	return s.withProblems(ctx, r)
}

// List gives the rules of a space, or of the instance when spaceID is nil.
func (s *Service) List(ctx context.Context, c identity.Caller, spaceID *uuid.UUID) ([]Rule, error) {
	if err := s.mayManage(ctx, c, spaceID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, "SELECT "+ruleColumns+`WHERE r.space_id IS NOT DISTINCT FROM $1 ORDER BY r.name LIMIT $2`, spaceID, maxRulesListed)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Rule, error) { return scanRule(row) })
	if err != nil {
		return nil, err
	}
	out := make([]Rule, 0, len(list))
	for _, r := range list {
		v, err := s.withProblems(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// Manual lists the rules someone can start by hand on a page, for the More menu.
func (s *Service) Manual(ctx context.Context, c identity.Caller, pageID uuid.UUID) ([]Rule, error) {
	if err := s.Perms.RequirePage(ctx, s.Pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, "SELECT "+ruleColumns+`WHERE r.enabled AND r.trigger = $1 AND r.paused_at IS NULL
		AND (r.space_id IS NULL OR r.space_id = (SELECT space_id FROM pages WHERE id = $2)) ORDER BY r.name`, TriggerManual, pageID)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Rule, error) { return scanRule(row) })
	if err != nil {
		return nil, err
	}
	var space uuid.UUID
	if err := s.Pool.QueryRow(ctx, "SELECT space_id FROM pages WHERE id = $1", pageID).Scan(&space); err != nil {
		return nil, err
	}
	out := []Rule{}
	for _, r := range list {
		if r.Definition.reaches(&space) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Service) account(ctx context.Context, c identity.Caller, in Input) (uuid.UUID, error) {
	if in.AccountID == nil || *in.AccountID == c.UserID {
		return c.UserID, nil
	}
	// Choosing another account lends its rights to this rule, so only a service
	// account, and only by someone who runs them.
	isAdmin, err := s.Identity.HasGlobalPermission(ctx, c.UserID, identity.PermAdministrator)
	if err != nil {
		return uuid.Nil, err
	}
	var kind string
	err = s.Pool.QueryRow(ctx, "SELECT type FROM users WHERE id = $1", *in.AccountID).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (kind != "service" || !isAdmin)) {
		return uuid.Nil, apperr.Invalid("A rule acts as you, or as a service account an administrator chose.")
	}
	return *in.AccountID, err
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > maxNameRunes {
		return "", apperr.Invalid("A rule needs a name of at most 200 characters.")
	}
	return name, nil
}

// checkSpaces refuses spaces on a rule of a space, and spaces that do not exist, so a
// rule of the instance never quietly reaches nowhere.
func (s *Service) checkSpaces(ctx context.Context, spaceID *uuid.UUID, d *Definition) error {
	if len(d.Spaces) == 0 {
		d.Spaces = nil
		return nil
	}
	if spaceID != nil {
		return apperr.Invalid("Only a rule of the whole instance chooses spaces.")
	}
	slices.SortFunc(d.Spaces, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	d.Spaces = slices.Compact(d.Spaces)
	if len(d.Spaces) > maxRuleSpaces {
		return apperr.Invalid("A rule chooses at most 500 spaces.")
	}
	var found int
	if err := s.Pool.QueryRow(ctx, "SELECT count(*) FROM spaces WHERE id = ANY($1)", d.Spaces).Scan(&found); err != nil {
		return err
	}
	if found != len(d.Spaces) {
		return apperr.Invalid("One of the chosen spaces does not exist.")
	}
	return nil
}

func (s *Service) mayEnable(ctx context.Context, r Rule) error {
	checked, err := s.withProblems(ctx, r)
	if err != nil {
		return err
	}
	if len(checked.Problems) > 0 {
		return apperr.Invalid("Fix what the form says before turning this rule on.")
	}
	return nil
}

func (s *Service) recordChange(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, r Rule) error {
	space := ""
	if r.SpaceID != nil {
		space = r.SpaceID.String()
	}
	if _, err := audit.Record(ctx, tx, c.Event(action, "rule", r.ID.String(), space, map[string]any{"name": r.Name, "enabled": r.Enabled})); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"rule_id": r.ID, "space_id": r.SpaceID})
	return err
}

func (s *Service) scheduleFrom(d Definition) *time.Time {
	if d.Trigger.Kind != TriggerSchedule || len(d.Trigger.problems()) > 0 {
		return nil
	}
	next := nextRun(d.Trigger, s.now())
	return &next
}

// Create adds a rule. A rule with open problems is stored, and stays off.
func (s *Service) Create(ctx context.Context, c identity.Caller, spaceID *uuid.UUID, in Input) (Rule, error) {
	if err := s.mayManage(ctx, c, spaceID); err != nil {
		return Rule{}, err
	}
	name, err := cleanName(in.Name)
	if err != nil {
		return Rule{}, err
	}
	account, err := s.account(ctx, c, in)
	if err != nil {
		return Rule{}, err
	}
	if err := s.checkSpaces(ctx, spaceID, &in.Definition); err != nil {
		return Rule{}, err
	}
	r := Rule{ID: ids.New(), SpaceID: spaceID, Name: name, Enabled: in.Enabled, Definition: in.Definition, OwnerID: c.UserID, AccountID: account}
	if r.Enabled {
		if err := s.mayEnable(ctx, r); err != nil {
			return r, err
		}
	}
	raw, err := json.Marshal(in.Definition)
	if err != nil {
		return r, err
	}
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO automation_rules (id, space_id, name, enabled, definition, trigger, owner_id, account_id, next_run_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, r.ID, r.SpaceID, r.Name, r.Enabled, raw, in.Definition.Trigger.Kind,
			r.OwnerID, r.AccountID, s.scheduleFrom(in.Definition)); err != nil {
			return err
		}
		return s.recordChange(ctx, tx, c, "rule.created", r)
	})
	if err != nil {
		return r, err
	}
	return s.Get(ctx, c, r.ID)
}

// Update changes a rule. Saving a rule ends a pause: the person looked at it.
func (s *Service) Update(ctx context.Context, c identity.Caller, id uuid.UUID, in Input) (Rule, error) {
	r, err := s.Get(ctx, c, id)
	if err != nil {
		return r, err
	}
	if r.Name, err = cleanName(in.Name); err != nil {
		return r, err
	}
	account, err := s.account(ctx, c, in)
	if err != nil {
		return r, err
	}
	if in.AccountID == nil {
		account = r.AccountID
	}
	if err := s.checkSpaces(ctx, r.SpaceID, &in.Definition); err != nil {
		return r, err
	}
	r.Enabled, r.Definition, r.AccountID = in.Enabled, in.Definition, account
	if r.Enabled {
		if err := s.mayEnable(ctx, r); err != nil {
			return r, err
		}
	}
	raw, err := json.Marshal(in.Definition)
	if err != nil {
		return r, err
	}
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE automation_rules SET name = $2, enabled = $3, definition = $4, trigger = $5, account_id = $6,
			next_run_at = $7, paused_at = NULL, updated_at = now() WHERE id = $1`, id, r.Name, r.Enabled, raw, in.Definition.Trigger.Kind,
			r.AccountID, s.scheduleFrom(in.Definition)); err != nil {
			return err
		}
		return s.recordChange(ctx, tx, c, "rule.changed", r)
	})
	if err != nil {
		return r, err
	}
	return s.Get(ctx, c, id)
}

// Delete removes a rule with its log.
func (s *Service) Delete(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	r, err := s.Get(ctx, c, id)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM automation_rules WHERE id = $1", id); err != nil {
			return err
		}
		return s.recordChange(ctx, tx, c, "rule.deleted", r)
	})
}

// NewHook makes the secret address of a rule, and ends the old one. The token shows
// once and is stored hashed, like a personal token.
func (s *Service) NewHook(ctx context.Context, c identity.Caller, id uuid.UUID) (string, error) {
	r, err := s.Get(ctx, c, id)
	if err != nil {
		return "", err
	}
	if r.Definition.Trigger.Kind != TriggerIncoming {
		return "", apperr.Invalid("Only a rule with an incoming webhook has an address.")
	}
	secret := make([]byte, hookTokenBytes)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO automation_hooks (rule_id, token_hash) VALUES ($1, $2)
			ON CONFLICT (rule_id) DO UPDATE SET token_hash = EXCLUDED.token_hash`, id, hashToken(token)); err != nil {
			return err
		}
		return s.recordChange(ctx, tx, c, "rule.hook_changed", r)
	})
	return token, err
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// RunsPerHour reads the limit of the administrator.
func (s *Service) RunsPerHour(ctx context.Context) (int, error) {
	return runsPerHour(ctx, s.Pool)
}

func runsPerHour(ctx context.Context, q db.Querier) (int, error) {
	var v struct {
		RunsPerHour int `json:"runs_per_hour"`
	}
	v.RunsPerHour = defaultRunsPerHour
	var raw []byte
	err := q.QueryRow(ctx, "SELECT value FROM settings WHERE key = $1", settingsKey).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return v.RunsPerHour, nil
	}
	if err != nil {
		return 0, err
	}
	return v.RunsPerHour, json.Unmarshal(raw, &v)
}

// SetRunsPerHour stores the limit. The route requires administrator.
func (s *Service) SetRunsPerHour(ctx context.Context, c identity.Caller, limit int) error {
	if limit < 1 || limit > maxRunsPerHour {
		return apperr.Invalid("A rule runs 1 to 10000 times an hour.")
	}
	raw, err := json.Marshal(map[string]int{"runs_per_hour": limit})
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if err := settings.Store(ctx, tx, settingsKey, raw); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("rule.limit_changed", "settings", "", "", map[string]any{"runs_per_hour": limit})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "rule.limit_changed", map[string]any{"runs_per_hour": limit})
		return err
	})
}

// PurgeRuns removes the log past its 30 days (PLT-031).
func (s *Service) PurgeRuns(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, "DELETE FROM automation_runs WHERE created_at < now() - make_interval(days => $1)", runLogDays)
	return err
}
