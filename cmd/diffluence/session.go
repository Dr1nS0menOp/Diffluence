// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/instance"
)

// cmdEndSessions logs one person or everyone out, for an incident (SEC-178). Ending every
// session needs --all, so a missing address can never log everyone out by mistake.
func cmdEndSessions(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("session end", flag.ContinueOnError)
	email := fs.String("email", "", "the address of the person to log out")
	all := fs.Bool("all", false, "log everyone out")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if (*email == "") == !*all {
		return errors.New("give either --email or --all")
	}
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	instID, err := instance.Ensure(ctx, pool, cfg.InstanceName)
	if err != nil {
		return err
	}
	svc := identity.NewService(pool, instID, identity.Options{SessionIdleTimeout: cfg.SessionIdleTimeout, SessionMaxAge: cfg.SessionMaxAge})
	ended, err := svc.EndSessions(ctx, identity.SystemActor("cli"), *email, identity.RequestMeta{})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%d sessions ended. Nobody was disabled.\n", ended)
	return nil
}
