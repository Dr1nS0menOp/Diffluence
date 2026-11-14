// SPDX-License-Identifier: Apache-2.0

package features

import (
	"context"
	"slices"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
)

// The parts that are experimental for 1.0 (ADR-043, spec 069). Each is off on a new
// installation until an administrator turns it on. Public sites and analytics are
// experimental too, but keep the switches they already had, which start off.
const (
	ExperimentalAutomation     = "automation"
	ExperimentalDatabases      = "databases"
	ExperimentalBundles        = "bundles"
	ExperimentalConsoleRestore = "console_restore"
	// ExperimentalPublishing is draft and published spaces (spec 073, PUB-1415).
	ExperimentalPublishing      = "publishing"
	ExperimentalGuidedTemplates = "guided_templates"
)

// ExperimentalParts lists every part that the experimental switch covers.
var ExperimentalParts = []string{ExperimentalAutomation, ExperimentalDatabases, ExperimentalBundles, ExperimentalConsoleRestore,
	ExperimentalPublishing, ExperimentalGuidedTemplates}

// partsHeldForOlderReleases are parts that a previous release, still running beside this
// one during an upgrade, would damage: it drops what it does not know of a guided template
// on save (spec 074, TPL-1103). They cannot be turned on while such a release runs.
var partsHeldForOlderReleases = []string{ExperimentalGuidedTemplates}

// experimentalKey holds the parts an administrator turned on. Migration 0054 writes it
// on upgrade from what the installation already used; without it, every part is off.
const experimentalKey = "experimental"

// Experimental is what an administrator turned on.
type Experimental struct {
	On []string `json:"on"`
}

// experimentalOff answers a route of a part that is off. It is a 404, because for this
// installation the part does not exist, with a code and a message that say why.
func experimentalOff() *apperr.Error {
	return &apperr.Error{Kind: apperr.KindNotFound, Code: "experimental_off",
		Message: "This experimental part is off on this installation. An administrator can turn it on in the console."}
}

// Experimental reads which experimental parts are on.
func (s *Service) Experimental(ctx context.Context) (Experimental, error) {
	return s.experimentalIn(ctx, s.pool)
}

func (s *Service) experimentalIn(ctx context.Context, q db.Querier) (Experimental, error) {
	v := Experimental{On: []string{}}
	err := s.settings.ReadIn(ctx, q, experimentalKey, &v)
	return v, err
}

// SetExperimental stores which experimental parts are on. The route requires administrator.
func (s *Service) SetExperimental(ctx context.Context, c identity.Caller, v Experimental) error {
	on := []string{}
	for _, part := range v.On {
		if !slices.Contains(ExperimentalParts, part) {
			return apperr.Invalid("That is not an experimental part.")
		}
		if !slices.Contains(on, part) {
			on = append(on, part)
		}
	}
	if err := s.refuseWhileOlderReleaseRuns(ctx, on); err != nil {
		return err
	}
	return s.settings.Write(ctx, c, experimentalKey, Experimental{On: on})
}

// refuseWhileOlderReleaseRuns refuses to turn on a part held for older releases while a
// node of one is alive. A part that is on already stays on.
func (s *Service) refuseWhileOlderReleaseRuns(ctx context.Context, on []string) error {
	if s.olderReleaseRuns == nil {
		return nil
	}
	before, err := s.Experimental(ctx)
	if err != nil {
		return err
	}
	for _, part := range on {
		if !slices.Contains(partsHeldForOlderReleases, part) || slices.Contains(before.On, part) {
			continue
		}
		older, err := s.olderReleaseRuns(ctx)
		if err != nil {
			return err
		}
		if older {
			return apperr.Conflict("older_release_running", "A node of this installation still runs the previous release. "+
				"Turn this part on once every node runs this release; a node that stopped counts after a minute.")
		}
	}
	return nil
}

// HoldWhileOlderReleaseRuns gives the check that tells whether a node of an older release
// is still alive. The wiring sets it, because the nodes are the installation's own table.
func (s *Service) HoldWhileOlderReleaseRuns(check func(context.Context) (bool, error)) {
	s.olderReleaseRuns = check
}

// ExperimentalOn reports whether an experimental part is on, reading through q.
func (s *Service) ExperimentalOn(ctx context.Context, q db.Querier, part string) (bool, error) {
	v, err := s.experimentalIn(ctx, q)
	if err != nil {
		return false, err
	}
	return slices.Contains(v.On, part), nil
}

// RequireExperimental refuses with a 404 that says why when part is off.
func (s *Service) RequireExperimental(ctx context.Context, q db.Querier, part string) error {
	on, err := s.ExperimentalOn(ctx, q, part)
	if err != nil {
		return err
	}
	if !on {
		return experimentalOff()
	}
	return nil
}
