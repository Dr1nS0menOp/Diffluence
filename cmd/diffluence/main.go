// SPDX-License-Identifier: Apache-2.0

// Command diffluence is the single Diffluence binary.
package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/instance"
	"github.com/diffluence/diffluence/internal/keys"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/version"
)

const usage = `Usage: diffluence <command> [flags]

Commands:
  serve                     run the server (role from DIFFLUENCE_ROLE)
  migrate [--contract]      apply pending migrations
  user create-admin         create a local administrator (password on stdin)
  audit verify              verify the audit log hash chain
  audit prune               remove audit rows past their retention, as the owner of the tables
  healthcheck               ask a running server on this machine whether it is ready
  version                   print the version
`

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, version.Version)
		return 0
	case "serve":
		err = cmdServe(ctx)
	case "migrate":
		err = cmdMigrate(ctx, args[1:], stdout)
	case "user":
		if len(args) < 2 || args[1] != "create-admin" {
			fmt.Fprint(stderr, usage)
			return 2
		}
		err = cmdCreateAdmin(ctx, args[2:], stdin, stdout)
	case "audit":
		switch {
		case len(args) >= 2 && args[1] == "verify":
			err = cmdAuditVerify(ctx, stdout)
		case len(args) >= 2 && args[1] == "prune":
			err = cmdAuditPrune(ctx, stdout)
		default:
			fmt.Fprint(stderr, usage)
			return 2
		}
	case "healthcheck":
		err = cmdHealthcheck(ctx)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

func cmdHealthcheck(ctx context.Context) error {
	addr := os.Getenv("DIFFLUENCE_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" {
		host = "127.0.0.1"
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/readyz", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("not ready: status %d", resp.StatusCode)
	}
	return nil
}

func cmdServe(ctx context.Context) error {
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}
	log := app.NewLogger(cfg.LogLevel)
	a, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer a.Close()
	return a.Run(ctx)
}

func cmdMigrate(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	contract := fs.Bool("contract", false, "also apply contract migrations (only after every node runs this version)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}
	migs, err := db.Migrations()
	if err != nil {
		return err
	}
	rep, err := migrateDatabase(ctx, cfg, migs, db.Options{IncludeContract: *contract, Log: app.NewLogger(cfg.LogLevel)})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "applied %d migration(s); %d contract migration(s) pending\n", len(rep.Applied), len(rep.PendingContract))
	return nil
}

// migrateDatabase applies migrations, as the owner of the tables when one is
// configured, and gives the application role its rights. Safe to run next to a
// starting server: an advisory lock makes them take turns.
func migrateDatabase(ctx context.Context, cfg config.Config, migs []db.Migration, opts db.Options) (db.Report, error) {
	url := cfg.DatabaseURL
	if cfg.MigrateDatabaseURL != "" {
		url = cfg.MigrateDatabaseURL
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		return db.Report{}, err
	}
	defer pool.Close()
	rep, err := db.Migrate(ctx, pool, migs, opts)
	if err != nil || cfg.MigrateDatabaseURL == "" {
		return rep, err
	}
	role, err := db.RoleOf(cfg.DatabaseURL)
	if err != nil {
		return rep, err
	}
	return rep, db.GrantApp(ctx, pool, role)
}

func cmdCreateAdmin(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("user create-admin", flag.ContinueOnError)
	email := fs.String("email", "", "email address (required)")
	name := fs.String("name", "", "display name (required)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *email == "" || *name == "" {
		return errors.New("--email and --name are required")
	}
	fmt.Fprintf(stdout, "Password (at least %d characters), then Enter: ", identity.MinPasswordLength)
	line, err := bufio.NewReader(io.LimitReader(stdin, identity.MaxPasswordBytes+2)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	fmt.Fprintln(stdout)
	password := strings.TrimRight(line, "\r\n")

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
	instID, err := instance.Ensure(ctx, pool, cfg.InstanceName)
	if err != nil {
		return err
	}
	svc := identity.NewService(pool, instID, identity.Options{SessionIdleTimeout: cfg.SessionIdleTimeout, SessionMaxAge: cfg.SessionMaxAge})
	u, err := svc.CreateLocalUser(ctx, identity.NewUser{
		Email: *email, DisplayName: *name, Type: identity.TypeMember, Password: password,
		GlobalPermissions: identity.AllGlobalPermissions,
	}, identity.SystemActor("cli"), identity.RequestMeta{})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "administrator %s created (id %s)\n", u.Email, u.ID)
	return nil
}

// signingKey loads the key of this installation, without starting anything else: this
// runs beside a server, also in restore.sh.
func signingKey(cfg config.Config) (ed25519.PrivateKey, error) {
	keyring, err := keys.LoadOrCreate(cfg.KeyFile(), false)
	if err != nil {
		return nil, err
	}
	return keyring.SigningKey()
}

// cmdAuditPrune removes rows past the retention set in the console. It needs the owner
// of the tables: the application role has no right to do this (spec 013, D11).
func cmdAuditPrune(ctx context.Context, stdout io.Writer) error {
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}
	url := cfg.MigrateDatabaseURL
	if url == "" {
		url = cfg.DatabaseURL
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	days, err := settings.AuditDays(ctx, pool)
	if err != nil {
		return err
	}
	if days == 0 {
		return errors.New("audit retention is not set in the console, so nothing may be removed")
	}
	key, err := signingKey(cfg)
	if err != nil {
		return err
	}
	res, err := audit.Prune(ctx, pool, time.Now().AddDate(0, 0, -days), key)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "removed %d audit rows, up to %d\n", res.Removed, res.AnchorSeq)
	return nil
}

func cmdAuditVerify(ctx context.Context, stdout io.Writer) error {
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}
	key, err := signingKey(cfg)
	if err != nil {
		return err
	}
	public, ok := key.Public().(ed25519.PublicKey)
	if !ok {
		return errors.New("unexpected key type")
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	res, err := audit.Check(ctx, pool, public)
	if err != nil {
		return err
	}
	result := audit.ResultSuccess
	if !res.OK {
		result = audit.ResultFailure
	}
	if err := audit.RecordStandalone(ctx, pool, audit.Event{ActorType: audit.ActorSystem, ActorID: "cli",
		Action: "audit.verified", ObjectType: "audit_log", Result: result,
		Details: map[string]any{"ok": res.OK, "checked": res.Checked, "checkpoints": res.Checkpoints}}); err != nil {
		return err
	}
	if !res.OK {
		return fmt.Errorf("audit chain BROKEN at seq %d: %s (checked %d events)", res.FirstBadSeq, res.Reason, res.Checked)
	}
	fmt.Fprintf(stdout, "audit chain OK (%d events and %d signed checkpoints checked)\n", res.Checked, res.Checkpoints)
	return nil
}
