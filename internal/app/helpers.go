// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/exchange"
)

// collabLivePath is where the editing service says it runs (collab/server.mjs).
const collabLivePath = "/livez"

// HelperState is one optional helper as the console shows it (ARC-072).
type HelperState struct {
	Name      string    `json:"name"`
	OK        bool      `json:"ok"`
	Detail    string    `json:"detail"`
	CheckedAt time.Time `json:"checked_at"`
}

// checkHelpers is the worker's minute task: it asks each configured helper whether it
// answers, and keeps the answer for the console. A helper that is no longer configured
// is forgotten.
func (a *App) checkHelpers(ctx context.Context) error {
	checks := a.exchange.CheckHelpers(ctx)
	if a.cfg.CollabURL != "" {
		checks = append(checks, exchange.ProbeURL(ctx, "collaboration", strings.TrimRight(a.cfg.CollabURL, "/")+collabLivePath))
	}
	names := make([]string, 0, len(checks))
	return pgx.BeginFunc(ctx, a.pool, func(tx pgx.Tx) error {
		for _, c := range checks {
			names = append(names, c.Name)
			if _, err := tx.Exec(ctx, `INSERT INTO helper_health (name, ok, detail, checked_at) VALUES ($1, $2, $3, now())
				ON CONFLICT (name) DO UPDATE SET ok = EXCLUDED.ok, detail = EXCLUDED.detail, checked_at = EXCLUDED.checked_at`,
				c.Name, c.OK, c.Detail); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, "DELETE FROM helper_health WHERE NOT (name = ANY($1))", names)
		return err
	})
}

// HelperStates lists what the last check found.
func (a *App) HelperStates(ctx context.Context) ([]HelperState, error) {
	rows, err := a.pool.Query(ctx, "SELECT name, ok, detail, checked_at FROM helper_health ORDER BY name")
	if err != nil {
		return nil, err
	}
	states, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (HelperState, error) {
		var h HelperState
		err := r.Scan(&h.Name, &h.OK, &h.Detail, &h.CheckedAt)
		return h, err
	})
	if states == nil {
		states = []HelperState{}
	}
	return states, err
}

// HelpersDown names the helpers that did not answer their last check.
func (a *App) HelpersDown(ctx context.Context) (map[string]bool, error) {
	states, err := a.HelperStates(ctx)
	if err != nil {
		return nil, err
	}
	down := map[string]bool{}
	for _, h := range states {
		if !h.OK {
			down[h.Name] = true
		}
	}
	return down, nil
}
