// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/health"
	"github.com/diffluence/diffluence/internal/settings"
)

// runtimeEvery is how often a node rereads the log level for a while and the disk
// thresholds, as it rereads maintenance mode (spec 075, CSL-571 and CSL-702).
const runtimeEvery = 5 * time.Second

// runtimeState is what the last reread found, and the level this node runs at.
type runtimeState struct {
	// level is the logger's level, when the process gave it to the app.
	level atomic.Pointer[slog.LevelVar]
	// running is the level this node runs at now, as it reports it.
	running atomic.Int64
	// thresholds are the disk levels in force.
	thresholds atomic.Pointer[health.Thresholds]
	// probes is the last try of the settings used only later, when the process kept it.
	probes atomic.Pointer[config.ProbeRecord]
}

// ControlLogLevel hands the app the level of its logger, so a log level for a while can change
// it. Without it the app still reports the level it would run at.
func (a *App) ControlLogLevel(level *slog.LevelVar) {
	level.Set(slog.Level(a.runtime.running.Load()))
	a.runtime.level.Store(level)
}

// UseProbes hands the app the record of the last try of the settings used only later, which
// the Configuration page shows (CSL-608).
func (a *App) UseProbes(p *config.ProbeRecord) { a.runtime.probes.Store(p) }

// lastProbe is the last try, and whether one was kept at all.
func (a *App) lastProbe() (time.Time, []config.Warning, bool) {
	p := a.runtime.probes.Load()
	if p == nil {
		return time.Time{}, nil, false
	}
	return p.Last()
}

// thresholds are the disk levels in force on this node.
func (a *App) thresholds() health.Thresholds {
	if t := a.runtime.thresholds.Load(); t != nil {
		return *t
	}
	return settings.Health{}.Thresholds()
}

// runningLevel is the log level this node runs at.
func (a *App) runningLevel() slog.Level { return slog.Level(a.runtime.running.Load()) }

// ApplyRuntime rereads the log level and the disk thresholds, and applies them as they hold
// at now: a log level whose time ended gives way to the configured one without anyone
// asking. It is exported for tests, which pass their own clock.
func (a *App) ApplyRuntime(ctx context.Context, now time.Time) error {
	rt, err := settings.ReadRuntime(ctx, a.pool)
	if err != nil {
		return err
	}
	level := a.cfg.LogLevel
	if override, ok := rt.Log.LevelFor(a.nodeID.String(), now); ok {
		level = override
	}
	a.runtime.running.Store(int64(level))
	if v := a.runtime.level.Load(); v != nil {
		v.Set(level)
	}
	t := rt.Health.Thresholds()
	a.runtime.thresholds.Store(&t)
	return nil
}

// runRuntime applies the settings every runtimeEvery until ctx ends. A failed read keeps what
// the node read before, and is logged.
func (a *App) runRuntime(ctx context.Context) {
	for {
		if err := a.ApplyRuntime(ctx, time.Now()); err != nil && ctx.Err() == nil {
			a.log.Warn("rereading the log level and the disk thresholds failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(runtimeEvery):
		}
	}
}
