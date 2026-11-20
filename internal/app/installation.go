// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/kenwerk/kenwerk/internal/audit"
	"github.com/kenwerk/kenwerk/internal/db"
	"github.com/kenwerk/kenwerk/internal/events"
	"github.com/kenwerk/kenwerk/internal/health"
	"github.com/kenwerk/kenwerk/internal/settings"
	"github.com/kenwerk/kenwerk/internal/version"
)

// The areas of the console that show what runs the installation (spec 075, wave CS1). Each
// only reads: the upgrade page, the events, the signatures of the audit log, and where the
// log level stands. Every read is a few small statements, made while a page is open.

// UpgradeSource is what the upgrade page reads besides the database.
type UpgradeSource struct {
	Migrations []db.Migration
	// Grace is KENWERK_CONTRACT_GRACE, and GraceSource where its value came from.
	Grace       time.Duration
	GraceSource string
}

// WaitingContract is a contract migration of this release that has not run.
type WaitingContract struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
	// NodesBehind are the live nodes that do not know it yet, by id.
	NodesBehind []string `json:"nodes_behind"`
	// GraceEnds is when the grace after the youngest live node's start ends; nil while no
	// node is live.
	GraceEnds *time.Time `json:"grace_ends"`
}

// UpgradeMarker is the newest system.upgrade_started of the audit log.
type UpgradeMarker struct {
	At            time.Time `json:"at"`
	Version       any       `json:"version"`
	FromMigration any       `json:"from_migration"`
	ToMigration   any       `json:"to_migration"`
	BackupNotedAt any       `json:"backup_noted_at"`
}

// Upgrades is the upgrade page (CSL-560 to CSL-562).
type Upgrades struct {
	Release   string       `json:"release"`
	BuildDate string       `json:"build_date"`
	Postgres  string       `json:"postgres"`
	Nodes     []NodeStatus `json:"nodes"`
	// Newest is the newest migration applied; Known the newest this release knows.
	Newest int `json:"newest"`
	Known  int `json:"known"`
	// ExpandWaiting counts expand migrations of this release not applied yet: where another
	// process migrates, such as the backup service in Compose, they wait for it.
	ExpandWaiting int               `json:"expand_waiting"`
	Contract      []WaitingContract `json:"contract"`
	Grace         ShownSetting      `json:"grace"`
	Marker        *UpgradeMarker    `json:"marker"`
}

// ShownSetting is a start-up setting with where its value came from.
type ShownSetting struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// ReadUpgrades reads the upgrade page.
func ReadUpgrades(ctx context.Context, q db.Querier, src UpgradeSource) (Upgrades, error) {
	out := Upgrades{Release: version.Version, BuildDate: version.Date, Contract: []WaitingContract{},
		Grace: ShownSetting{Name: "CONTRACT_GRACE", Value: src.Grace.String(), Source: src.GraceSource}}
	if n := len(src.Migrations); n > 0 {
		out.Known = src.Migrations[n-1].Version
	}
	var err error
	if out.Nodes, err = listNodes(ctx, q); err != nil {
		return out, err
	}
	if err := q.QueryRow(ctx, "SHOW server_version").Scan(&out.Postgres); err != nil {
		return out, fmt.Errorf("app: read the PostgreSQL version: %w", err)
	}
	st, err := db.Status(ctx, q, src.Migrations)
	if err != nil {
		return out, fmt.Errorf("app: read the migration state: %w", err)
	}
	out.Newest, out.ExpandWaiting = st.Newest, st.PendingExpand
	pending, err := db.PendingContract(ctx, q, src.Migrations)
	if err != nil {
		return out, err
	}
	graceEnds := youngestLiveStart(out.Nodes)
	if graceEnds != nil {
		ends := graceEnds.Add(src.Grace)
		graceEnds = &ends
	}
	for _, m := range pending {
		w := WaitingContract{Version: m.Version, Name: m.Name, NodesBehind: []string{}, GraceEnds: graceEnds}
		for _, n := range out.Nodes {
			if n.Live && n.MaxMigration < m.Version {
				w.NodesBehind = append(w.NodesBehind, n.ID)
			}
		}
		out.Contract = append(out.Contract, w)
	}
	out.Marker, err = upgradeMarker(ctx, q)
	return out, err
}

// youngestLiveStart is when the newest live node started, from which the grace of a contract
// migration runs (contractDue); nil while no node is live.
func youngestLiveStart(nodes []NodeStatus) *time.Time {
	var youngest *time.Time
	for _, n := range nodes {
		if n.Live && (youngest == nil || n.StartedAt.After(*youngest)) {
			started := n.StartedAt
			youngest = &started
		}
	}
	return youngest
}

func upgradeMarker(ctx context.Context, q db.Querier) (*UpgradeMarker, error) {
	e, err := audit.LatestOf(ctx, q, ActionUpgradeStarted)
	if err != nil || e == nil {
		return nil, err
	}
	return &UpgradeMarker{At: e.OccurredAt, Version: e.Details["version"], FromMigration: e.Details["from_migration"],
		ToMigration: e.Details["to_migration"], BackupNotedAt: e.Details["backup_noted_at"]}, nil
}

// UpgradeStatus is the upgrade page of this installation.
func (a *App) UpgradeStatus(ctx context.Context) (any, error) {
	return ReadUpgrades(ctx, a.pool, UpgradeSource{Migrations: a.migrations, Grace: a.cfg.ContractGrace,
		GraceSource: a.startedSource("CONTRACT_GRACE")})
}

// startedSource is where a setting this process started with came from.
func (a *App) startedSource(name string) string {
	for _, s := range a.startedWith {
		if s.Name == name {
			return s.Source
		}
	}
	return ""
}

// EventsState is what kenwerk events status prints (CSL-590).
type EventsState struct {
	Transport  events.Transport `json:"transport"`
	ChangedAt  time.Time        `json:"changed_at"`
	DrainedAt  *time.Time       `json:"drained_at"`
	OlderNodes []OlderNode      `json:"older_nodes"`
	// Problem says in words why the release without NATS may not be installed yet; empty when
	// it may.
	Problem string `json:"problem"`
}

// OlderNode is a running node of a release from before the events switch.
type OlderNode struct {
	Role         string `json:"role"`
	Version      string `json:"version"`
	MaxMigration int    `json:"max_migration"`
}

// EventsStatus reads where the events transport stands.
func (a *App) EventsStatus(ctx context.Context) (any, error) {
	s, err := events.ReadState(ctx, a.pool)
	if err != nil {
		return nil, err
	}
	problem, err := events.NATSRemovalProblem(ctx, a.pool)
	if err != nil {
		return nil, err
	}
	out := EventsState{Transport: s.Transport, ChangedAt: s.ChangedAt, OlderNodes: []OlderNode{}, Problem: problem}
	for _, n := range s.OlderNodes {
		out.OlderNodes = append(out.OlderNodes, OlderNode{Role: n.Role, Version: n.Version, MaxMigration: n.MaxMigration})
	}
	if !s.DrainedAt.IsZero() {
		out.DrainedAt = &s.DrainedAt
	}
	return out, nil
}

// AuditArea is the audit area beside the check of the chain (CSL-530 to CSL-533).
type AuditArea struct {
	audit.Overview
	// Witness is the newest reported comparison with the witness, or nil.
	Witness *audit.WitnessCheck `json:"witness"`
	// WitnessWarnAfterDays is how long the audit line waits for the next report.
	WitnessWarnAfterDays int `json:"witness_warn_after_days"`
}

// AuditOverview reads the newest checkpoint, the keys and the newest witness report.
func (a *App) AuditOverview(ctx context.Context) (any, error) {
	key, err := a.keyring.SigningKey()
	if err != nil {
		return nil, err
	}
	public, ok := key.Public().(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("app: unexpected key type")
	}
	overview, err := audit.ReadOverview(ctx, a.pool, public)
	if err != nil {
		return nil, err
	}
	witness, err := audit.LatestWitnessCheck(ctx, a.pool)
	if err != nil {
		return nil, err
	}
	return AuditArea{Overview: overview, Witness: witness, WitnessWarnAfterDays: int(health.WitnessWarnAfter.Hours() / 24)}, nil
}

// LogLevelArea is where the log level stands on every node (CSL-570 to CSL-572).
type LogLevelArea struct {
	// Configured is the level this node was started with.
	Configured string                `json:"configured"`
	Override   *settings.LogOverride `json:"override"`
	Nodes      []NodeStatus          `json:"nodes"`
	Levels     []string              `json:"levels"`
	Hours      []int                 `json:"hours"`
}

// LogLevelState reads the log level for a while and the level each node reports.
func (a *App) LogLevelState(ctx context.Context) (any, error) {
	override, err := a.settings.LogOverrideNow(ctx)
	if err != nil {
		return nil, err
	}
	nodes, err := listNodes(ctx, a.pool)
	if err != nil {
		return nil, err
	}
	return LogLevelArea{Configured: settings.LevelName(a.cfg.LogLevel), Override: override, Nodes: nodes,
		Levels: settings.LogLevels, Hours: settings.LogHours}, nil
}
