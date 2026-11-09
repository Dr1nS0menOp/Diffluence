// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"

	"github.com/diffluence/diffluence/internal/setup"
)

// announceSetup writes a fresh setup code to the log while no administrator exists, so
// the person who installed the server can open the setup screen (spec 038). A new code
// replaces any earlier one, so only the newest line in the log works.
func (a *App) announceSetup(ctx context.Context) {
	open, err := a.setup.Open(ctx)
	if err != nil {
		a.log.Error("first setup: checking whether it is open failed", "error", err)
		return
	}
	if !open {
		return
	}
	code, err := a.setup.Issue(ctx, "server")
	if errors.Is(err, setup.ErrClosed) {
		return
	}
	if err != nil {
		a.log.Error("first setup: making a setup code failed", "error", err)
		return
	}
	// The code belongs in this log on purpose: only whoever runs the server reads it.
	a.log.Warn(setup.Announcement(a.cfg.PublicOrigin(), code))
}
