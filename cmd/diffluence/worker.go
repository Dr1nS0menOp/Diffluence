// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/keys"
	"github.com/diffluence/diffluence/internal/restore"
)

// The duties of the backup service that need the owner of the tables or its backup key, which
// the server no longer holds in Compose (spec 070). Its loop runs both every round.

// checkpointEvery is how often the backup service signs the head of the audit chain, as the
// server's job did (ADR-039).
const checkpointEvery = 5 * time.Minute

// defaultBackupKey is where the backup service of compose.yaml finds its key.
const defaultBackupKey = "/key/backup.key"

// errNoOwner says that this process holds only the application role.
var errNoOwner = errors.New("this process has no owner of the tables (DIFFLUENCE_MIGRATE_DATABASE_URL); " +
	"in Compose the backup service holds it: docker compose run --rm --entrypoint diffluence backup migrate")

// ownerURL is the address of the owner of the tables, or errNoOwner in a process that
// leaves migrations to another. A single-role setup migrates with its one address.
func ownerURL(cfg config.Config) (string, error) {
	switch {
	case cfg.MigrateDatabaseURL != "":
		return cfg.MigrateDatabaseURL, nil
	case cfg.LeavesMigrations():
		return "", errNoOwner
	}
	return cfg.DatabaseURL, nil
}

// workerGate is the gate of the backup service, from its backup key. Without a key that can be
// read it is the zero gate, which confirms no restore and says why in the restore's step.
func workerGate(path string, stderr io.Writer) restore.Gate {
	w, err := keys.LoadWorker(path)
	if err != nil {
		fmt.Fprintln(stderr, "no restore can be confirmed:", err)
		return restore.Gate{}
	}
	return restore.NewGate(w.MAC)
}

func cmdWorker(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: diffluence worker migrate | checkpoint [--key FILE]")
	}
	switch args[0] {
	case "migrate":
		return cmdWorkerMigrate(ctx, stdout)
	case "checkpoint":
		return cmdWorkerCheckpoint(ctx, args[1:], stdout, stderr)
	}
	return fmt.Errorf("unknown worker step %q: migrate or checkpoint", args[0])
}

// cmdWorkerMigrate applies the migrations of this release and, after the grace period, its
// contract migrations. It prints a line only when it did something.
func cmdWorkerMigrate(ctx context.Context, stdout io.Writer) error {
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}
	if cfg.MigrateDatabaseURL == "" {
		return errors.New("the backup service needs DIFFLUENCE_MIGRATE_DATABASE_URL_FILE, the owner of the tables, to migrate")
	}
	migs, err := db.Migrations()
	if err != nil {
		return err
	}
	role, err := db.RoleOf(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	owner, err := db.Open(ctx, cfg.MigrateDatabaseURL)
	if err != nil {
		return err
	}
	defer owner.Close()
	rep, err := app.TendSchema(ctx, owner, migs, app.TendOptions{AppRole: role, BackupMaxAge: cfg.UpgradeBackupMaxAge,
		ContractGrace: cfg.ContractGrace, Log: app.NewLogger(cfg.LogLevel)})
	if err != nil {
		return err
	}
	if rep.Expand > 0 || rep.Contract > 0 {
		fmt.Fprintf(stdout, "applied %d migration(s) and %d contract migration(s)\n", rep.Expand, rep.Contract)
	}
	return nil
}

// cmdWorkerCheckpoint brings the backup service's key in force, endorsed by the key of the
// installation, and signs the head of the chain every five minutes. Each checkpoint goes to
// this process's log as a witness line, whatever the log level.
func cmdWorkerCheckpoint(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("worker checkpoint", flag.ContinueOnError)
	fs.SetOutput(stderr)
	keyPath := fs.String("key", defaultBackupKey, "the backup key, from which the checkpoint key is derived")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}
	worker, err := keys.LoadWorker(*keyPath)
	if err != nil {
		return err
	}
	workerKey, err := worker.CheckpointKey()
	if err != nil {
		return err
	}
	// The server makes master.key on its first start; until then there is nothing to endorse with.
	installation, err := keys.LoadOrCreate(cfg.KeyFile(), false)
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("master.key does not exist yet: the server makes it on its first start")
	}
	if err != nil {
		return err
	}
	installationKey, err := installation.SigningKey()
	if err != nil {
		return err
	}
	url, err := ownerURL(cfg)
	if err != nil {
		return err
	}
	owner, err := db.Open(ctx, url)
	if err != nil {
		return err
	}
	defer owner.Close()
	cp, signed, err := audit.WorkerCheckpoint(ctx, owner, installationKey, workerKey, checkpointEvery)
	if err != nil || !signed {
		return err
	}
	witness := slog.New(slog.NewJSONHandler(stdout, nil))
	witness.Info(audit.WitnessMessage, cp.WitnessAttrs()...)
	return nil
}
