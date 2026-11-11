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
  setup-code                print a new one-time code for the setup screen of a new install
  audit verify [--witness FILE]
                            verify the audit log hash chain, and compare it with the
                            checkpoints in an application log (FILE, or - for stdin)
  audit prune               remove audit rows past their retention, as the owner of the tables
  backup note               record that a complete backup was just made (scripts/backup.sh runs it)
  key <step>                replace master.key without downtime; see diffluence key
  session end --email ADDR  log one person out everywhere, without disabling them
  session end --all         log everyone out, for an incident
  healthcheck               ask a running server on this machine whether it is ready
  doctor                    check disk, database, backup, migrations, work, mail, login,
                            release, configuration and keys, and print a report that
                            holds no secret; exits 1 when something needs a person now
  client <command>          use the API of a server with a token; see diffluence client
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
	case "client":
		return cmdClient(ctx, args[1:], os.Getenv, stdout, stderr)
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
			err = cmdAuditVerify(ctx, args[2:], stdin, stdout)
		case len(args) >= 2 && args[1] == "prune":
			err = cmdAuditPrune(ctx, stdout)
		default:
			fmt.Fprint(stderr, usage)
			return 2
		}
	case "backup":
		if len(args) < 2 || args[1] != "note" {
			fmt.Fprint(stderr, usage)
			return 2
		}
		err = cmdBackupNote(ctx, stdout)
	case "setup-code":
		err = cmdSetupCode(ctx, stdout)
	case "key":
		err = cmdKey(ctx, args[1:], stdout, stderr)
	case "session":
		if len(args) < 2 || args[1] != "end" {
			fmt.Fprint(stderr, usage)
			return 2
		}
		err = cmdEndSessions(ctx, args[2:], stdout)
	case "healthcheck":
		err = cmdHealthcheck(ctx)
	case "doctor":
		err = cmdDoctor(ctx, stdout)
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
	organisation := fs.String("organisation", "", "name of the company or person this instance is for (optional)")
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
	if *organisation == "" {
		return nil
	}
	if err := settings.NewService(pool, nil).SetOrganisationName(ctx, identity.SystemCaller("cli"), *organisation); err != nil {
		return fmt.Errorf("the administrator exists, but the organisation name was refused: %w", err)
	}
	fmt.Fprintf(stdout, "organisation: %s\n", *organisation)
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

// cmdBackupNote tells the checks before the next upgrade that a backup exists. It does not
// check the backup: scripts/backup.sh runs it only after every part was written.
func cmdBackupNote(ctx context.Context, stdout io.Writer) error {
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := app.NoteBackup(ctx, pool); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "backup noted in the audit log")
	return nil
}

func cmdAuditVerify(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("audit verify", flag.ContinueOnError)
	witnessPath := fs.String("witness", "", "an application log holding audit checkpoints, or - for standard input")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var witness []audit.Checkpoint
	if *witnessPath != "" {
		var err error
		if witness, err = readWitness(*witnessPath, stdin); err != nil {
			return err
		}
	}
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
	compared := audit.WitnessResult{OK: true}
	if res.OK && witness != nil {
		if compared, err = audit.CompareWitness(ctx, pool, public, witness); err != nil {
			return err
		}
	}
	whole := res.OK && compared.OK
	details := map[string]any{"ok": whole, "checked": res.Checked, "checkpoints": res.Checkpoints}
	if witness != nil {
		details["witness_compared"] = compared.Compared
	}
	result := audit.ResultSuccess
	if !whole {
		result = audit.ResultFailure
	}
	if err := audit.RecordStandalone(ctx, pool, audit.Event{ActorType: audit.ActorSystem, ActorID: "cli",
		Action: "audit.verified", ObjectType: "audit_log", Result: result, Details: details}); err != nil {
		return err
	}
	if !res.OK {
		return fmt.Errorf("audit chain BROKEN at seq %d: %s (checked %d events)", res.FirstBadSeq, res.Reason, res.Checked)
	}
	if !compared.OK {
		return fmt.Errorf("audit chain does not match the witness log at seq %d: %s", compared.FirstBadSeq, compared.Reason)
	}
	fmt.Fprintf(stdout, "audit chain OK (%d events and %d signed checkpoints checked)\n", res.Checked, res.Checkpoints)
	if witness != nil {
		fmt.Fprintf(stdout, "witness log OK (%d checkpoints match the chain, %d from before the retention period)\n",
			compared.Compared, compared.Pruned)
	}
	return nil
}

// readWitness reads the checkpoints from an application log. A log without any is an
// error: checkpoints are made every few minutes, so none means the wrong file, or a
// collector that kept nothing.
func readWitness(path string, stdin io.Reader) ([]audit.Checkpoint, error) {
	in := stdin
	if path != "-" {
		f, err := os.Open(path) // #nosec G304 -- the operator names the log to read.
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		in = f
	}
	witness, err := audit.ReadWitness(in)
	if err != nil {
		return nil, err
	}
	if len(witness) == 0 {
		return nil, errors.New("the witness log holds no audit checkpoints")
	}
	return witness, nil
}
