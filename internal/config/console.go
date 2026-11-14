// SPDX-License-Identifier: Apache-2.0

package config

import (
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// What the console's Configuration page adds to config check (spec 075, chapter 08): the
// variables of the trace exporter, a mark on what changes at the next restart, and the last
// try of the settings used only later.

// tracingPrefix starts the variables of the OpenTelemetry exporter.
const tracingPrefix = "OTEL_EXPORTER_OTLP_"

// tracingNames are the other OpenTelemetry variables the exporter reads.
var tracingNames = []string{"OTEL_SERVICE_NAME", "OTEL_RESOURCE_ATTRIBUTES"}

// TracingVariables lists the OpenTelemetry variables of environ, as os.Environ gives it,
// with the source "environment". A header or a key is only (set); an endpoint shows without
// what may hold a credential (a user, a password, a query); resource attributes, which are
// free text, show only as (set).
func TracingVariables(environ []string) []Setting {
	var out []Setting
	for _, entry := range environ {
		name, value, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(name, tracingPrefix) && !slices.Contains(tracingNames, name) {
			continue
		}
		out = append(out, Setting{Name: name, Value: tracingShown(name, value), Source: sourceEnvironment})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// tracingShown is the value of a tracing variable as the page may show it.
func tracingShown(name, value string) string {
	switch {
	case value == "":
		return "(not set)"
	case name == "OTEL_SERVICE_NAME" || strings.HasSuffix(name, "_PROTOCOL") ||
		strings.HasSuffix(name, "_TIMEOUT") || strings.HasSuffix(name, "_COMPRESSION") || strings.HasSuffix(name, "_INSECURE"):
		return value
	case strings.HasSuffix(name, "_ENDPOINT"):
		return endpointShown(value)
	}
	return "(set)"
}

// endpointShown is an endpoint without its user, password, query or fragment, or (set)
// when it is no address.
func endpointShown(value string) string {
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "(set)"
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}).String()
}

// Row is a setting as the console shows it: what a restart would take, and, where that
// differs from what the process started with, the value it runs with.
type Row struct {
	Setting
	// ChangesAtRestart marks a value or source that differs from the start.
	ChangesAtRestart bool
	// Running is the setting as the process started with it, when it changes at the next
	// restart; nil when it was not read at the start.
	Running *Setting
}

// CompareWithStart marks each setting of now that differs from started. Without a list
// from the start (nil), nothing is marked: nothing can be compared.
func CompareWithStart(started, now []Setting) []Row {
	byName := make(map[string]Setting, len(started))
	for _, s := range started {
		byName[s.Name] = s
	}
	out := make([]Row, 0, len(now))
	for _, s := range now {
		row := Row{Setting: s}
		if started != nil {
			was, found := byName[s.Name]
			if !found || was != s {
				row.ChangesAtRestart = true
			}
			if found && was != s {
				row.Running = &was
			}
		}
		out = append(out, row)
	}
	return out
}

// ProbeRecord keeps the last try of the settings used only later (Probe), so a page can
// show it without reaching outside systems again. The zero value is ready to use.
type ProbeRecord struct {
	mu       sync.Mutex
	at       time.Time
	done     bool
	warnings []Warning
}

// Keep stores the warnings of a try that ended at at.
func (p *ProbeRecord) Keep(at time.Time, warnings []Warning) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.at, p.done, p.warnings = at, true, slices.Clone(warnings)
}

// Last gives the last try: when it ended, its warnings, and whether one ran at all.
func (p *ProbeRecord) Last() (time.Time, []Warning, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.at, slices.Clone(p.warnings), p.done
}
