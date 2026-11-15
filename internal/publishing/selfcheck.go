// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/settings"
)

// The consistency check by itself (spec 073, wave PB4, PUB-343, PUB-1305): the worker runs it
// once a week and once after the part was turned on again, only looking; its result is a line of
// the health page, whose button fixes what it found.

// checkKey holds the result of the last check of every pair, for the health page.
const checkKey = "publishing_check"

// SelfCheckEvery is how often the check runs by itself.
const SelfCheckEvery = 7 * 24 * time.Hour

// CheckRecord is what the last check of every pair found.
type CheckRecord struct {
	At    time.Time `json:"at"`
	Pairs int       `json:"pairs"`
	Found int       `json:"found"`
	Fixed bool      `json:"fixed"`
}

// LastCheck reads the result of the last check, through q; ok is false when none ran.
func LastCheck(ctx context.Context, q db.Querier) (CheckRecord, bool, error) {
	var r CheckRecord
	err := settings.Load(ctx, q, checkKey, &r)
	return r, !r.At.IsZero(), err
}

// SelfCheck runs the check of every pair when it is due: the part is on, and no check ran since it
// was last turned on or for a week. The worker asks every hour. It never fixes: a person decides
// that on the health page.
func (s *Service) SelfCheck(ctx context.Context) error {
	on, err := features.ExperimentalIn(ctx, s.pool)
	if err != nil || !slices.Contains(on.On, features.ExperimentalPublishing) {
		return err
	}
	last, ran, err := LastCheck(ctx, s.pool)
	if err != nil {
		return err
	}
	since := on.OnSince[features.ExperimentalPublishing]
	if ran && last.At.After(since) && time.Since(last.At) < SelfCheckEvery {
		return nil
	}
	_, err = s.CheckEverything(ctx, identity.Caller{System: "worker"}, false)
	return err
}

// CheckEverything runs the check of every pair as c, fixes what it finds when asked, keeps the
// result for the health page and audits it. The route of the health page requires administrator.
func (s *Service) CheckEverything(ctx context.Context, c identity.Caller, fix bool) (CheckRecord, error) {
	results, err := checkAllAs(ctx, s.pool, c, fix)
	if err != nil {
		return CheckRecord{}, err
	}
	rec := CheckRecord{At: time.Now().UTC(), Pairs: len(results), Fixed: fix}
	for _, r := range results {
		rec.Found += len(r.Found)
	}
	// What a fix found is fixed now, so the health page keeps nothing of it.
	kept := rec
	if fix {
		kept.Found = 0
	}
	raw, err := json.Marshal(kept)
	if err != nil {
		return rec, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := settings.Store(ctx, tx, checkKey, raw); err != nil {
			return err
		}
		_, err := audit.Record(ctx, tx, c.Event("publishing.checked", "settings", checkKey, "",
			map[string]any{"pairs": rec.Pairs, "found": rec.Found, "fixed": fix}))
		return err
	})
	if err == nil && s.afterCheck != nil {
		s.afterCheck()
	}
	return rec, err
}

// SetAfterCheck gives what a check must tell when it is done: the wiring drops the health facts it
// keeps, so the health page shows the new result at once.
func (s *Service) SetAfterCheck(f func()) { s.afterCheck = f }
