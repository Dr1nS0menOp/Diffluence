// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/health"
	"github.com/diffluence/diffluence/internal/storage"
	"github.com/diffluence/diffluence/internal/version"
)

// errDoctorFound means the report shows a problem. It is said once, in the report.
var errDoctorFound = errors.New("doctor found a problem; see the report above")

// stateLabels give each state a fixed width, so the report lines up when pasted.
var stateLabels = map[health.State]string{health.OK: "ok     ", health.Warn: "WARN   ", health.Problem: "PROBLEM"}

// cmdDoctor runs the checks of the health page from the command line, with the
// configuration and the keys, and prints a report an operator can paste in a question
// (OPS-107, spec 051 D8). It prints states, counts, sizes and dates only: no address, no
// secret and no name. Each part runs even when another failed.
func cmdDoctor(ctx context.Context, stdout io.Writer) error {
	built := version.Date
	if built == "" {
		built = "unknown"
	}
	fmt.Fprintf(stdout, "Diffluence doctor\nversion: %s (built %s)\n", version.Version, built)
	cfg, err := config.FromEnvironment()
	if err != nil {
		fmt.Fprintf(stdout, "configuration: PROBLEM: %v\n", err)
		return errDoctorFound
	}
	fmt.Fprintf(stdout, "configuration: ok (role %s)\n", cfg.Role)

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		// The error of a failed connection names the host and the user, so it stays out.
		fmt.Fprintln(stdout, "database: PROBLEM: the database does not answer; diffluence migrate shows why")
		return errDoctorFound
	}
	defer pool.Close()
	migs, err := db.Migrations()
	if err != nil {
		return err
	}
	disk, err := storage.NewDisk(filepath.Join(cfg.DataDir, "objects"))
	if err != nil {
		fmt.Fprintln(stdout, "files: PROBLEM: the folder of the stored files cannot be opened")
		return errDoctorFound
	}
	facts, err := app.ReadHealthFacts(ctx, app.HealthSource{Pool: pool, Store: disk, Migrations: migs,
		MailConfigured: cfg.SMTP.Host != "", OIDCConfigured: cfg.OIDC.Issuer != "", ContractGrace: cfg.ContractGrace}, time.Now())
	if err != nil {
		fmt.Fprintln(stdout, "health: PROBLEM: the health facts cannot be read; is the database migrated?")
		return errDoctorFound
	}
	checks := health.Evaluate(facts)
	for _, c := range checks {
		fmt.Fprintf(stdout, "%s  %-10s  %s\n", stateLabels[c.State], c.Name, health.Describe(c))
	}
	problems, warnings := health.Count(checks)
	if !doctorKeys(ctx, cfg, stdout) {
		problems++
	}
	fmt.Fprintf(stdout, "summary: %d problem(s), %d warning(s)\n", problems, warnings)
	if problems > 0 {
		return errDoctorFound
	}
	return nil
}

// doctorKeys opens every sealed value and names every stored file with the keys on disk,
// as diffluence key check does, and says only how many. It reports whether all opened.
func doctorKeys(ctx context.Context, cfg config.Config, stdout io.Writer) bool {
	tools, err := app.OpenKeyTools(ctx, cfg)
	if err != nil {
		fmt.Fprintln(stdout, "keys: PROBLEM: the key file cannot be read; see docs/operations/backup-restore.md")
		return false
	}
	defer tools.Close()
	rep, err := app.CheckKeys(ctx, tools.Pool, tools.Store, tools.Keyring, false)
	if err != nil {
		fmt.Fprintln(stdout, "keys: PROBLEM: the keys could not be checked; run diffluence key check")
		return false
	}
	values, files := 0, 0
	for _, n := range rep.Values {
		values += n
	}
	for _, n := range rep.Files {
		files += n
	}
	state := "ok"
	if len(rep.Failures) > 0 {
		state = "PROBLEM"
	}
	notes := []string{fmt.Sprintf("%d value(s) and %d file(s) open", values, files)}
	if len(rep.Failures) > 0 {
		notes = append(notes, fmt.Sprintf("%d cannot be opened; run diffluence key check", len(rep.Failures)))
	}
	if rep.PlainFiles > 0 {
		notes = append(notes, fmt.Sprintf("%d file(s) not encrypted yet", rep.PlainFiles))
	}
	fmt.Fprintf(stdout, "keys: %s (%s)\n", state, strings.Join(notes, "; "))
	return len(rep.Failures) == 0
}
