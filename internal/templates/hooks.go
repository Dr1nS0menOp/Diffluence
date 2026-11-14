// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"time"
)

// SetHooks lets a test break or slow down the making of a set at a chosen step, to prove
// that a failure anywhere leaves nothing behind (TPL-120, TPL-820). Production code never
// sets them.
type SetHooks struct {
	// Step runs inside the transaction before each step of a set made for real. The steps
	// are page, file, label, restriction and origin, each with the index of its entry, and
	// audit with -1. An error it returns fails the set at that step.
	Step func(ctx context.Context, step string, entry int) error
	// Deadline replaces the hard stop of TPL-826 when it is above zero.
	Deadline time.Duration
}

// SetHooks replaces the hooks; the zero value removes them.
func (s *Service) SetHooks(h SetHooks) { s.hooks.Store(&h) }
