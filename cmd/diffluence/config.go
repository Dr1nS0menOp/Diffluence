// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/diffluence/diffluence/internal/config"
)

// errConfigRefused means the configuration would refuse a start. The reasons are printed
// once, in the report.
var errConfigRefused = errors.New("the configuration would refuse a start; see the report above")

// cmdConfigCheck loads the configuration as a start would, without starting anything, and
// prints every setting with the value in effect and where it came from, secrets hidden.
// Then it tries the settings that are otherwise only used later (OPS-100, OPS-101).
func cmdConfigCheck(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("config check", flag.ContinueOnError)
	offline := fs.Bool("offline", false, "do not try the mail server, the identity provider or the publish folder")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if path := os.Getenv("DIFFLUENCE_CONFIG_FILE"); path != "" {
		fmt.Fprintf(stdout, "config file: %s\n", path)
	} else {
		fmt.Fprintln(stdout, "config file: none (DIFFLUENCE_CONFIG_FILE is not set)")
	}
	cfg, settings, err := config.Inspect(os.LookupEnv)
	w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	for _, s := range settings {
		fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, s.Value, s.Source)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if err != nil {
		fmt.Fprintf(stdout, "configuration: PROBLEM\n%v\n", err)
		return errConfigRefused
	}
	fmt.Fprintln(stdout, "configuration: ok")
	if *offline {
		return nil
	}
	printWarnings(stdout, config.Probe(ctx, cfg))
	return nil
}

// printWarnings prints the settings that load but do not work yet.
func printWarnings(stdout io.Writer, warnings []config.Warning) {
	for _, warning := range warnings {
		fmt.Fprintf(stdout, "WARN  %s: %s\n", warning.Setting, warning.Message)
	}
}
