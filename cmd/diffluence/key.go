// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/keys"
)

const keyUsage = `Usage: diffluence key <step>

Replacing master.key, one step at a time (docs/operations/backup-restore.md):
  add                 write a new key beside master.key; then restart every node
  promote             let the new key seal from now on; then restart every node
  reseal              seal every value and file again with the new key
  check [--deep]      prove that everything opens; --deep reads every byte of every file
        [--require-current]  also fail while anything still uses an older key
  retire              remove the old key, only when check finds nothing that needs it
`

// resealRunBudget is the length of one run of the reseal loop, after which it reports progress.
const resealRunBudget = 2 * time.Minute

// errKeyCheck means a check found values or files that need attention. It is reported, not
// printed twice.
var errKeyCheck = errors.New("the check found problems; see above")

func cmdKey(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(stderr, keyUsage)
		return errors.New("name a step")
	}
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}
	switch args[0] {
	case "add":
		return keyAdd(ctx, cfg, stdout)
	case "promote":
		return keyPromote(ctx, cfg, stdout)
	case "reseal":
		return keyReseal(ctx, cfg, stdout)
	case "check":
		return keyCheck(ctx, cfg, args[1:], stdout)
	case "retire":
		return keyRetire(ctx, cfg, stdout)
	default:
		fmt.Fprint(stderr, keyUsage)
		return fmt.Errorf("unknown step %q", args[0])
	}
}

func keyAdd(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	id, err := keys.AddNext(cfg.KeyFile())
	if err != nil {
		return err
	}
	if err := recordKeyStep(ctx, cfg, "keys.added", id, nil); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "new key %s written to %s%s\n", id, cfg.KeyFile(), keys.NextSuffix)
	fmt.Fprintln(stdout, "Copy it to every node beside master.key, back it up apart from the data, restart every node, then run: diffluence key promote")
	return nil
}

func keyPromote(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	id, err := keys.Promote(cfg.KeyFile())
	if err != nil {
		return err
	}
	if err := recordKeyStep(ctx, cfg, "keys.promoted", id, nil); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "key %s seals from now on; the old key is kept in %s%s\n", id, cfg.KeyFile(), keys.PreviousSuffix)
	fmt.Fprintln(stdout, "Copy master.key and master.key.previous to every node, back them up, restart every node, then run: diffluence key reseal")
	return nil
}

func keyReseal(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	tools, err := app.OpenKeyTools(ctx, cfg)
	if err != nil {
		return err
	}
	defer tools.Close()
	var total app.ResealReport
	for {
		rep, err := app.ResealKeys(ctx, tools.Pool, tools.Store, tools.Keyring, resealRunBudget)
		if err != nil {
			return err
		}
		total.Values += rep.Values
		total.Files += rep.Files
		total.Skipped += rep.Skipped
		fmt.Fprintf(stdout, "sealed again: %d value(s), %d file(s); %d could not be opened\n", total.Values, total.Files, total.Skipped)
		if rep.Done {
			break
		}
	}
	err = app.RecordKeyEvent(ctx, tools.Pool, "keys.resealed", tools.Keyring.KeyID(),
		map[string]any{"values": total.Values, "files": total.Files, "skipped": total.Skipped})
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Done. Now run: diffluence key check --require-current")
	return nil
}

func keyCheck(ctx context.Context, cfg config.Config, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("key check", flag.ContinueOnError)
	deep := fs.Bool("deep", false, "read every byte of every file")
	requireCurrent := fs.Bool("require-current", false, "fail while anything still uses an older key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tools, err := app.OpenKeyTools(ctx, cfg)
	if err != nil {
		return err
	}
	defer tools.Close()
	rep, err := app.CheckKeys(ctx, tools.Pool, tools.Store, tools.Keyring, *deep)
	if err != nil {
		return err
	}
	printReport(stdout, rep)
	switch {
	case len(rep.Failures) > 0:
		return errKeyCheck
	case *requireCurrent && !rep.AllCurrent():
		return errors.New("something still uses an older key or is still plain; run diffluence key reseal first")
	}
	return nil
}

func keyRetire(ctx context.Context, cfg config.Config, stdout io.Writer) error {
	tools, err := app.OpenKeyTools(ctx, cfg)
	if err != nil {
		return err
	}
	defer tools.Close()
	rep, err := app.RetireOldKey(ctx, tools, cfg.KeyFile())
	if errors.Is(err, app.ErrOldKeyStillNeeded) {
		printReport(stdout, rep)
		return errors.New("the old key is still needed; run diffluence key reseal, then try again")
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "the old key is removed here; remove %s%s from every other node and from the backups that are newer than this change\n", cfg.KeyFile(), keys.PreviousSuffix)
	return nil
}

// printReport shows counts per key id and where each failure is. It shows no secret: key ids
// are one-way, and a failure names a row or a file, not its content.
func printReport(w io.Writer, rep app.KeyReport) {
	fmt.Fprintf(w, "current key: %s\n", rep.CurrentKey)
	for _, part := range []struct {
		name   string
		counts map[string]int
	}{{"values", rep.Values}, {"files", rep.Files}} {
		ids := make([]string, 0, len(part.counts))
		for id := range part.counts {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			fmt.Fprintf(w, "%s sealed with %s: %d\n", part.name, id, part.counts[id])
		}
	}
	if rep.PlainFiles > 0 {
		fmt.Fprintf(w, "files not encrypted yet: %d\n", rep.PlainFiles)
	}
	for _, f := range rep.Failures {
		fmt.Fprintf(w, "cannot open %s: %s\n", f.Where, f.Reason)
	}
	fmt.Fprintf(w, "problems: %d\n", len(rep.Failures))
}

func recordKeyStep(ctx context.Context, cfg config.Config, action, id string, details map[string]any) error {
	tools, err := app.OpenKeyTools(ctx, cfg)
	if err != nil {
		return fmt.Errorf("the key file changed, but the audit event could not be written: %w", err)
	}
	defer tools.Close()
	return app.RecordKeyEvent(ctx, tools.Pool, action, id, details)
}
