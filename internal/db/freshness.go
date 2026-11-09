// SPDX-License-Identifier: Apache-2.0

package db

import "context"

// Freshness says whether a read must see the latest commit, or may come from a copy of
// the database that lags (ARC-228). There is no such copy yet: every read goes to the
// primary. The marker is the groundwork, and /metrics counts both kinds, so the share
// of reads that could move is measured before a standby is added.
type Freshness int

const (
	// Current is the default: permission checks, sessions and a person's own
	// just-saved work.
	Current Freshness = iota
	// MayLag is for lists, reports, feeds, search and analytics.
	MayLag
)

type freshnessKey struct{}

// WithMayLag marks the reads made with ctx as allowed to lag.
func WithMayLag(ctx context.Context) context.Context {
	return context.WithValue(ctx, freshnessKey{}, MayLag)
}

// MustBeCurrent marks the reads made with ctx as needing the latest commit, also inside
// a request that may lag otherwise. Permission facts and sessions always use it.
func MustBeCurrent(ctx context.Context) context.Context {
	if FreshnessOf(ctx) == Current {
		return ctx
	}
	return context.WithValue(ctx, freshnessKey{}, Current)
}

// FreshnessOf returns what ctx asks for. Unmarked means Current.
func FreshnessOf(ctx context.Context) Freshness {
	f, _ := ctx.Value(freshnessKey{}).(Freshness)
	return f
}
