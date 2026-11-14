// SPDX-License-Identifier: Apache-2.0

// Package features says what is turned on for a space (spec 011, D25). Turning
// something off hides it and refuses new content, and never deletes anything.
package features

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/settings"
)

// The features of a space.
const (
	Blog           = "blog"
	Comments       = "comments"
	Reactions      = "reactions"
	AccessRequests = "access_requests"
	Stages         = "stages"
	Analytics      = "analytics"
	Automation     = "automation"
	Calendar       = "calendar"
)

// All lists every feature.
var All = []string{Blog, Comments, Reactions, AccessRequests, Stages, Analytics, Automation, Calendar}

// offByDefault waits until a space turns it on: a way to ask for access says that a
// hidden page exists, so a space chooses that itself.
var offByDefault = []string{AccessRequests}

const instanceKey = "features"

// Instance is what an administrator sets for all spaces.
type Instance struct {
	// Off is turned off everywhere.
	Off []string `json:"off"`
	// NewOff starts off in new spaces, and can be turned on there.
	NewOff []string `json:"new_off"`
}

// Service reads and sets features.
type Service struct {
	pool     *pgxpool.Pool
	settings *settings.Service
	// olderReleaseRuns says whether a node of a previous release is still alive.
	olderReleaseRuns func(context.Context) (bool, error)
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, st *settings.Service) *Service {
	return &Service{pool: pool, settings: st}
}

// Instance reads the settings for all spaces.
func (s *Service) Instance(ctx context.Context) (Instance, error) {
	return s.instanceIn(ctx, s.pool)
}

// instanceIn reads the settings for all spaces through q. Code that holds a
// transaction reads through it, so it never waits for a second connection (spec 050).
func (s *Service) instanceIn(ctx context.Context, q db.Querier) (Instance, error) {
	v := Instance{Off: []string{}, NewOff: []string{}}
	err := s.settings.ReadIn(ctx, q, instanceKey, &v)
	return v, err
}

func known(list []string) bool {
	for _, name := range list {
		if !slices.Contains(All, name) {
			return false
		}
	}
	return true
}

// SetInstance stores the settings for all spaces. The route requires administrator.
func (s *Service) SetInstance(ctx context.Context, c identity.Caller, v Instance) error {
	if !known(v.Off) || !known(v.NewOff) {
		return apperr.Invalid("That is not a feature.")
	}
	return s.settings.Write(ctx, c, instanceKey, v)
}

// Enabled reports whether a feature is on in a space.
func (s *Service) Enabled(ctx context.Context, q db.Querier, spaceID uuid.UUID, feature string) (bool, error) {
	all, err := s.ForSpace(ctx, q, spaceID)
	if err != nil {
		return false, err
	}
	return all[feature], nil
}

// ForSpace lists every feature of a space. What a space never set has its default,
// and what an administrator turned off for all spaces stays off.
func (s *Service) ForSpace(ctx context.Context, q db.Querier, spaceID uuid.UUID) (map[string]bool, error) {
	v, err := s.instanceIn(ctx, q)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, "SELECT feature, enabled FROM space_features WHERE space_id = $1", spaceID)
	if err != nil {
		return nil, err
	}
	stored := map[string]bool{}
	var feature string
	var on bool
	if _, err := pgx.ForEachRow(rows, []any{&feature, &on}, func() error {
		stored[feature] = on
		return nil
	}); err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(All))
	for _, name := range All {
		value, set := stored[name]
		if !set {
			value = !slices.Contains(offByDefault, name)
		}
		out[name] = value && !slices.Contains(v.Off, name)
	}
	return out, nil
}

// Set stores the features of a space, inside tx. The caller has checked the rights.
func (s *Service) Set(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID, values map[string]bool) error {
	for feature, on := range values {
		if !slices.Contains(All, feature) {
			return apperr.Invalid("That is not a feature.")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO space_features (space_id, feature, enabled) VALUES ($1, $2, $3)
			ON CONFLICT (space_id, feature) DO UPDATE SET enabled = EXCLUDED.enabled`, spaceID, feature, on); err != nil {
			return err
		}
	}
	return nil
}

// Start gives a new space the defaults of the instance.
func (s *Service) Start(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID) error {
	v, err := s.instanceIn(ctx, tx)
	if err != nil {
		return err
	}
	off := map[string]bool{}
	for _, feature := range v.NewOff {
		off[feature] = false
	}
	return s.Set(ctx, tx, spaceID, off)
}
