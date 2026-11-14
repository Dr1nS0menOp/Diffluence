// SPDX-License-Identifier: Apache-2.0

package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/health"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
)

// Two settings every node rereads every few seconds, beside maintenance mode (spec 075,
// CSL-570 and CSL-581): a log level for a while, and the levels of free disk space.

// SectionHealth holds the disk thresholds.
const SectionHealth = "health"

// keyLogOverride holds the log level for a while.
const keyLogOverride = "log_override"

// The bounds of the disk thresholds (CSL-1213): warn between 5 and 50 percent free, refuse
// files between 1 and 10, and refuse always below warn.
const (
	minDiskWarnPercent   = 5
	maxDiskWarnPercent   = 50
	minDiskRefusePercent = 1
	maxDiskRefusePercent = 10
)

// Health holds the levels of free disk space. Zero is the default of each.
type Health struct {
	DiskWarnPercent   int `json:"disk_warn_percent"`
	DiskRefusePercent int `json:"disk_refuse_percent"`
}

// Levels are the warn and refuse levels in force, with the defaults filled in.
func (h Health) Levels() (warn, refuse int) {
	warn, refuse = health.DiskWarnPercent, health.DiskRefusePercent
	if h.DiskWarnPercent > 0 {
		warn = h.DiskWarnPercent
	}
	if h.DiskRefusePercent > 0 {
		refuse = h.DiskRefusePercent
	}
	return warn, refuse
}

// Thresholds are the levels as the health rules take them.
func (h Health) Thresholds() health.Thresholds {
	warn, refuse := h.Levels()
	return health.Thresholds{DiskWarnPercent: warn, DiskRefusePercent: refuse}
}

func (h Health) validate() error {
	if h.DiskWarnPercent != 0 && (h.DiskWarnPercent < minDiskWarnPercent || h.DiskWarnPercent > maxDiskWarnPercent) {
		return apperr.Invalid("Administrators are warned below 5 to 50 percent free.")
	}
	if h.DiskRefusePercent != 0 && (h.DiskRefusePercent < minDiskRefusePercent || h.DiskRefusePercent > maxDiskRefusePercent) {
		return apperr.Invalid("New files are refused below 1 to 10 percent free.")
	}
	if warn, refuse := h.Levels(); refuse >= warn {
		return apperr.Invalid("New files are refused only at a level below the warning.")
	}
	return nil
}

// guard lets a lower level, which lets the disk fill further before anyone hears of it or
// before files stop, through only as a confirmed change (spec 076).
func (h Health) guard(c Change, stored section) (section, error) {
	warn, refuse := h.Levels()
	storedWarn, storedRefuse := stored.(*Health).Levels()
	if (warn < storedWarn || refuse < storedRefuse) && !c.Confirmed {
		return nil, ConfirmationRequired("Lower disk thresholds need a system administrator, a fresh login and a second factor.")
	}
	return &h, nil
}

// LogLevels are the levels an override may set, by name, from loudest to quietest.
var LogLevels = []string{"debug", "info", "warn", "error"}

// LogHours are the lengths an override may last (CSL-1212).
var LogHours = []int{1, 4, 24}

// LogRequest is what an administrator asks: a level, for how long, and for which node; an
// empty node is every node.
type LogRequest struct {
	Level string `json:"level"`
	Hours int    `json:"hours"`
	Node  string `json:"node"`
}

// LogOverride is a log level that holds until its time.
type LogOverride struct {
	Level string    `json:"level"`
	Node  string    `json:"node"`
	Until time.Time `json:"until"`
}

// override checks the request and gives the override it asks for from now.
func (r LogRequest) override(now time.Time) (LogOverride, error) {
	if !slices.Contains(LogLevels, r.Level) {
		return LogOverride{}, apperr.Invalid("A log level is debug, info, warn or error.")
	}
	if !slices.Contains(LogHours, r.Hours) {
		return LogOverride{}, apperr.Invalid("A log level holds for 1, 4 or 24 hours.")
	}
	if r.Node != "" {
		if _, err := uuid.Parse(r.Node); err != nil {
			return LogOverride{}, apperr.Invalid("A node is named by its id.")
		}
	}
	return LogOverride{Level: r.Level, Node: r.Node, Until: now.Add(time.Duration(r.Hours) * time.Hour)}, nil
}

// levelOf is the slog level of a name from LogLevels.
func levelOf(name string) slog.Level {
	var level slog.Level
	// A name outside LogLevels never reaches here; the zero level is info.
	_ = level.UnmarshalText([]byte(name))
	return level
}

// LevelName is the name of a level as LogLevels writes it.
func LevelName(level slog.Level) string {
	switch {
	case level <= slog.LevelDebug:
		return "debug"
	case level <= slog.LevelInfo:
		return "info"
	case level <= slog.LevelWarn:
		return "warn"
	}
	return "error"
}

// LevelFor is the level the override sets on node at now, and whether it holds there then.
func (o *LogOverride) LevelFor(node string, now time.Time) (slog.Level, bool) {
	if o == nil || !now.Before(o.Until) || (o.Node != "" && o.Node != node) {
		return 0, false
	}
	return levelOf(o.Level), true
}

// guardLog checks a request against the level a node was configured with: a quieter level
// hides warnings, so it is a confirmed change (spec 075, D26).
func guardLog(r LogRequest, configured slog.Level, c Change) (LogOverride, error) {
	o, err := r.override(c.At)
	if err != nil {
		return o, err
	}
	if levelOf(o.Level) > configured && !c.Confirmed {
		return o, ConfirmationRequired("A quieter log level needs a system administrator, a fresh login and a second factor.")
	}
	return o, nil
}

// SetLogOverride stores a log level for a while. configured is the level the answering node
// was started with. Only an administrator gets here (route rule); a quieter level needs
// SetLogOverrideConfirmed.
func (s *Service) SetLogOverride(ctx context.Context, c identity.Caller, r LogRequest, configured slog.Level) (LogOverride, error) {
	return s.setLogOverride(ctx, c, r, configured, Change{At: time.Now()})
}

// SetLogOverrideConfirmed is SetLogOverride for a system administrator who logged in again
// and gave a second factor just now; the route and its handler check both.
func (s *Service) SetLogOverrideConfirmed(ctx context.Context, c identity.Caller, r LogRequest, configured slog.Level) (LogOverride, error) {
	return s.setLogOverride(ctx, c, r, configured, Change{Confirmed: true, At: time.Now()})
}

func (s *Service) setLogOverride(ctx context.Context, c identity.Caller, r LogRequest, configured slog.Level, change Change) (LogOverride, error) {
	o, err := guardLog(r, configured, change)
	if err != nil {
		return o, err
	}
	raw, err := json.Marshal(o)
	if err != nil {
		return o, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := Store(ctx, tx, keyLogOverride, raw); err != nil {
			return err
		}
		details := map[string]any{"level": o.Level, "hours": r.Hours, "node": o.Node}
		if change.Confirmed {
			details["confirmed"] = true
		}
		if _, err := audit.Record(ctx, tx, c.Event("settings.log_level_set", "settings", keyLogOverride, "", details)); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "settings.updated", map[string]any{"key": keyLogOverride})
		return err
	})
	return o, err
}

// EndLogOverride returns every node to its configured level now. Any administrator may: the
// configured level is what the operator chose.
func (s *Service) EndLogOverride(ctx context.Context, c identity.Caller) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM settings WHERE key = $1", keyLogOverride)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("no_log_level", "Every node logs at its configured level already.")
		}
		if _, err := audit.Record(ctx, tx, c.Event("settings.log_level_ended", "settings", keyLogOverride, "", nil)); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "settings.updated", map[string]any{"key": keyLogOverride})
		return err
	})
}

// Runtime is what a node rereads every few seconds.
type Runtime struct {
	// Log is the log level for a while, or nil.
	Log *LogOverride
	// Health holds the disk thresholds.
	Health Health
}

// ReadRuntime reads the log level and the disk thresholds in one statement. A damaged value
// is an error the caller logs; it then keeps what it read before.
func ReadRuntime(ctx context.Context, q db.Querier) (Runtime, error) {
	var out Runtime
	rows, err := q.Query(ctx, "SELECT key, value FROM settings WHERE key = ANY($1)",
		[]string{keyLogOverride, sectionKeyPrefix + SectionHealth})
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return out, err
		}
		if key == keyLogOverride {
			out.Log = &LogOverride{}
			err = json.Unmarshal(raw, out.Log)
		} else {
			err = json.Unmarshal(raw, &out.Health)
		}
		if err != nil {
			return Runtime{}, fmt.Errorf("settings: read %s: %v", key, err)
		}
	}
	return out, rows.Err()
}

// LogOverrideNow is the stored log level for a while, or nil, for the console.
func (s *Service) LogOverrideNow(ctx context.Context) (*LogOverride, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, "SELECT value FROM settings WHERE key = $1", keyLogOverride).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var o LogOverride
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, fmt.Errorf("settings: read %s: %v", keyLogOverride, err)
	}
	if !time.Now().Before(o.Until) {
		return nil, nil
	}
	return &o, nil
}
