// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"time"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/tracing"
	"github.com/diffluence/diffluence/internal/version"
)

// flushTracesFor is how long a stopping process waits to send the spans it still holds.
const flushTracesFor = 5 * time.Second

// startTracing switches tracing on when DIFFLUENCE_TRACING asks for it (spec 060), and
// returns what flushes it on exit. Off, both do nothing.
func startTracing(ctx context.Context, cfg config.Config) (func(), error) {
	stop, err := tracing.Start(ctx, tracing.Options{Enabled: cfg.Tracing, Sample: cfg.TracingSample,
		Version: version.Version, Role: string(cfg.Role), Log: app.NewLogger(cfg.LogLevel)})
	if err != nil {
		return nil, err
	}
	return func() {
		flush, cancel := context.WithTimeout(context.WithoutCancel(ctx), flushTracesFor)
		defer cancel()
		if err := stop(flush); err != nil {
			app.NewLogger(cfg.LogLevel).Warn("tracing: the last spans could not be sent", "error", err)
		}
	}, nil
}

// traced runs a short command of the backup worker in a span of its own, and flushes the
// span before the process ends (spec 060 D10).
func traced(ctx context.Context, name string, run func(context.Context) error) error {
	cfg, err := config.FromEnvironment()
	if err != nil {
		// The command reads the configuration itself and reports what is wrong with it.
		return run(ctx)
	}
	stop, err := startTracing(ctx, cfg)
	if err != nil {
		return err
	}
	defer stop()
	ctx, end := tracing.Span(ctx, name, "")
	err = run(ctx)
	end(err)
	return err
}
