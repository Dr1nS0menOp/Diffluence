// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/version"
)

const (
	heartbeatEvery = 15 * time.Second
	// A node that was not seen for this long counts as gone.
	nodeLiveWindow = time.Minute
	// How long every live node must have known a contract migration before it
	// runs: leaves room for a rollback in the first minutes after an upgrade.
	contractGrace = 10 * time.Minute
	// How often the head of the audit chain is signed. It bounds what an owner of the
	// database can remove from the tail before the witness holds a copy (ADR-039).
	checkpointEvery = 5 * time.Minute
)

// alwaysEnabled passes every record to its handler, whatever level that was set to, so
// a quieter log level cannot drop the witness of the audit log.
type alwaysEnabled struct{ slog.Handler }

func (alwaysEnabled) Enabled(context.Context, slog.Level) bool { return true }

func (a *App) maxMigration() int {
	if n := len(a.migrations); n > 0 {
		return a.migrations[n-1].Version
	}
	return 0
}

// RegisterNode records this process in the node registry.
func (a *App) RegisterNode(ctx context.Context) error {
	if a.nodeID == [16]byte{} {
		a.nodeID = ids.New()
	}
	_, err := a.pool.Exec(ctx, `INSERT INTO nodes (id, role, version, max_migration) VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET last_seen_at = now()`, a.nodeID, string(a.cfg.Role), version.Version, a.maxMigration())
	return err
}

func (a *App) heartbeat(ctx context.Context) {
	for {
		if err := a.RegisterNode(ctx); err != nil && ctx.Err() == nil {
			a.log.Warn("node heartbeat failed", "error", err)
		}
		select {
		case <-ctx.Done():
			// Leave the registry at once, so a contract migration need not wait for us.
			leave, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			defer cancel()
			_, _ = a.pool.Exec(leave, "DELETE FROM nodes WHERE id = $1", a.nodeID)
			return
		case <-time.After(heartbeatEvery):
		}
	}
}

// RunContractMigrations applies pending contract migrations once every live node
// knows them, so no node still needs what they remove (docs/blueprint/07). It is
// exported for tests.
func (a *App) RunContractMigrations(ctx context.Context) (int, error) {
	pending, err := db.PendingContract(ctx, a.pool, a.migrations)
	if err != nil || len(pending) == 0 {
		return 0, err
	}
	var oldest int
	var youngestStart *time.Time
	err = a.pool.QueryRow(ctx, `SELECT COALESCE(min(max_migration), 0), max(started_at) FROM nodes
		WHERE last_seen_at > now() - make_interval(secs => $1)`, nodeLiveWindow.Seconds()).Scan(&oldest, &youngestStart)
	if err != nil {
		return 0, err
	}
	if oldest < pending[len(pending)-1].Version {
		return 0, nil // an older node is still alive, or no node reported yet
	}
	if youngestStart != nil && time.Since(*youngestStart) < contractGrace {
		return 0, nil
	}
	if err := a.migrate(ctx, db.Options{IncludeContract: true, Log: a.log}); err != nil {
		return 0, err
	}
	for _, m := range pending {
		if err := audit.RecordStandalone(ctx, a.pool, audit.Event{ActorType: audit.ActorSystem, ActorID: "worker",
			Action: "system.contract_migrated", ObjectType: "migration", ObjectID: fmt.Sprintf("%04d_%s", m.Version, m.Name),
			Result: audit.ResultSuccess}); err != nil {
			return len(pending), err
		}
	}
	return len(pending), nil
}

func (a *App) runContractMigrations(ctx context.Context) error {
	_, err := a.RunContractMigrations(ctx)
	return err
}

// CheckAudit verifies the chain and the signatures on its checkpoints.
func (a *App) CheckAudit(ctx context.Context) (audit.VerifyResult, error) {
	key, err := a.keyring.SigningKey()
	if err != nil {
		return audit.VerifyResult{}, err
	}
	public, ok := key.Public().(ed25519.PublicKey)
	if !ok {
		return audit.VerifyResult{}, errors.New("app: unexpected key type")
	}
	return audit.Check(ctx, a.pool, public)
}

// verifyAudit raises the alarm when the audit log does not hold. The console shows the
// result of this job, so a failure reaches a person and not only the log.
func (a *App) verifyAudit(ctx context.Context) error {
	res, err := a.CheckAudit(ctx)
	if err != nil {
		return err
	}
	if !res.OK {
		a.log.Error("AUDIT LOG TAMPERING DETECTED", "first_bad_seq", res.FirstBadSeq, "reason", res.Reason)
	}
	return audit.RecordStandalone(ctx, a.pool, audit.Event{ActorType: audit.ActorSystem, ActorID: "worker",
		Action: "audit.verified", ObjectType: "audit_log", Result: resultOf(res.OK),
		Details: map[string]any{"ok": res.OK, "checked": res.Checked, "first_bad_seq": res.FirstBadSeq}})
}

func resultOf(ok bool) string {
	if ok {
		return audit.ResultSuccess
	}
	return audit.ResultFailure
}

// signCheckpoint signs the current head of the audit chain. Someone who rewrites
// the whole chain cannot forge these without the key. Each checkpoint also goes to the
// application log once it is committed, as the witness outside the database (ADR-039):
// a line for a checkpoint that was rolled back would look like a removed tail.
func (a *App) signCheckpoint(ctx context.Context) error {
	key, err := a.keyring.SigningKey()
	if err != nil {
		return err
	}
	cp, signed, err := audit.SignCheckpoint(ctx, a.pool, key)
	if err != nil || !signed {
		return err
	}
	a.witness.Info(audit.WitnessMessage, cp.WitnessAttrs()...)
	return nil
}

// NodeStatus is one row of the admin console's node list.
type NodeStatus struct {
	ID           string    `json:"id"`
	Role         string    `json:"role"`
	Version      string    `json:"version"`
	MaxMigration int       `json:"max_migration"`
	StartedAt    time.Time `json:"started_at"`
	LastSeenAt   time.Time `json:"last_seen_at"`
	Live         bool      `json:"live"`
}

// SystemStatus is what the admin console shows under system health.
func (a *App) SystemStatus(ctx context.Context) (any, error) {
	rows, err := a.pool.Query(ctx, `SELECT id::text, role, version, max_migration, started_at, last_seen_at,
			last_seen_at > now() - make_interval(secs => $1)
		FROM nodes ORDER BY started_at`, nodeLiveWindow.Seconds())
	if err != nil {
		return nil, err
	}
	nodes, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (NodeStatus, error) {
		var n NodeStatus
		err := r.Scan(&n.ID, &n.Role, &n.Version, &n.MaxMigration, &n.StartedAt, &n.LastSeenAt, &n.Live)
		return n, err
	})
	if err != nil {
		return nil, err
	}
	if nodes == nil {
		nodes = []NodeStatus{}
	}
	var unpublished, failedJobs, queuedJobs, storageQueue, trashItems int
	var applied int
	err = a.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM outbox_events WHERE published_at IS NULL),
		(SELECT count(*) FROM jobs WHERE status = 'failed' AND created_at > now() - interval '7 days'),
		(SELECT count(*) FROM jobs WHERE status = 'queued'),
		(SELECT count(*) FROM storage_deletions),
		(SELECT count(*) FROM trash_items),
		(SELECT COALESCE(max(version), 0) FROM schema_migrations)`).
		Scan(&unpublished, &failedJobs, &queuedJobs, &storageQueue, &trashItems, &applied)
	if err != nil {
		return nil, err
	}
	pending, err := db.PendingContract(ctx, a.pool, a.migrations)
	if err != nil {
		return nil, err
	}
	storageState, ready := "ok", "ready"
	if err := a.store.Healthy(ctx); err != nil {
		storageState = "error"
	}
	if err := a.Ready(ctx); err != nil {
		ready = err.Error()
	}
	helpers, err := a.HelperStates(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"helpers": helpers,
		"version": version.Version, "build_date": version.Date, "ready": ready, "storage": storageState, "queue_healthy": a.queue.Healthy(),
		"nodes": nodes, "migrations_applied": applied, "contract_pending": len(pending),
		"outbox_unpublished": unpublished, "jobs_queued": queuedJobs, "jobs_failed_7d": failedJobs,
		"storage_queue": storageQueue, "trash_items": trashItems,
		"oidc_configured": a.sso != nil,
	}, nil
}
