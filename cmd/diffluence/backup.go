// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/diffluence/diffluence/internal/backups"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/settings"
)

// The backup worker (the backup service of Compose, a Kubernetes CronJob or a systemd
// timer) talks to the console through these commands, with the database access of the
// application role it already has (spec 058, D2). Nothing here reads or prints a key.

// openBackups connects with the application's role and returns the backups module.
func openBackups(ctx context.Context) (*backups.Service, func(), error) {
	cfg, err := config.FromEnvironment()
	if err != nil {
		return nil, nil, err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	// The worker never mails: only a server node holds the key that seals mail.
	return backups.NewService(pool, settings.NewService(pool, nil), nil, ""), pool.Close, nil
}

// cmdBackupNext prints what the worker should do now, as one line it can read in a shell:
//
//	now <reason> <keep>     make a backup now, then keep the newest <keep>
//	wait <seconds> <keep>   ask again after <seconds>
func cmdBackupNext(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("backup next", flag.ContinueOnError)
	interval := fs.Duration("default-interval", 4*time.Hour, "how often to back up when the console did not choose")
	keep := fs.Int("default-keep", 42, "how many backups to keep when the console did not choose")
	key := fs.String("key-fingerprint", "", "the first 16 hexadecimal characters of a SHA-256 of the backup key; empty when there is no key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, done, err := openBackups(ctx)
	if err != nil {
		return err
	}
	defer done()
	d, err := svc.Next(ctx, backups.Defaults{Interval: *interval, Keep: *keep, KeyFingerprint: *key})
	if err != nil {
		return err
	}
	if d.Now {
		fmt.Fprintf(stdout, "now %s %d\n", d.Reason, d.Keep)
		return nil
	}
	fmt.Fprintf(stdout, "wait %d %d\n", int(d.Wait.Seconds()), d.Keep)
	return nil
}

// cmdBackupReport tells the console how a backup went.
func cmdBackupReport(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("backup report", flag.ContinueOnError)
	name := fs.String("name", "", "the folder of the backup, such as diffluence-20261111T071618Z")
	reason := fs.String("reason", backups.ReasonManual, "schedule, request or manual")
	started := fs.String("started", "", "when the backup started, in RFC 3339")
	size := fs.Int64("size", 0, "the size of the finished backup in bytes")
	failed := fs.String("failed", "", "the step that failed; empty when the backup is complete")
	if err := fs.Parse(args); err != nil {
		return err
	}
	at, err := time.Parse(time.RFC3339, *started)
	if err != nil {
		return errors.New("give --started as an RFC 3339 time, such as 2026-11-11T07:16:18Z")
	}
	svc, done, err := openBackups(ctx)
	if err != nil {
		return err
	}
	defer done()
	if err := svc.ReportRun(ctx, backups.Report{Name: *name, Reason: *reason, StartedAt: at,
		OK: *failed == "", SizeBytes: *size, Message: *failed}); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "reported to the console")
	return nil
}

// cmdBackupRestoreChecked tells the console how the restore of a backup into a throw-away
// copy went.
func cmdBackupRestoreChecked(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("backup restore-checked", flag.ContinueOnError)
	name := fs.String("name", "", "the folder of the backup that was restored")
	seconds := fs.Int("seconds", 0, "how long the restore took")
	failed := fs.String("failed", "", "what went wrong; empty when the restore passed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, done, err := openBackups(ctx)
	if err != nil {
		return err
	}
	defer done()
	if err := svc.ReportRestore(ctx, backups.RestoreReport{Name: *name, OK: *failed == "", Seconds: *seconds, Message: *failed}); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "reported to the console")
	return nil
}
