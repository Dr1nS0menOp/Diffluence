// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/setup"
)

// cmdSetupCode prints a new one-time code for the setup screen of a new install. It
// replaces every earlier code, and refuses once an administrator exists (spec 038).
func cmdSetupCode(ctx context.Context, stdout io.Writer) error {
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}
	migs, err := db.Migrations()
	if err != nil {
		return err
	}
	// The server may still be starting, so this does not rely on it having migrated.
	if _, err := migrateDatabase(ctx, cfg, migs, db.Options{}); err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	code, err := setup.New(pool).Issue(ctx, "cli")
	if errors.Is(err, setup.ErrClosed) {
		return errors.New("an administrator exists, so the setup is over; sign in instead")
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, setup.Announcement(cfg.PublicOrigin(), code))
	return nil
}
