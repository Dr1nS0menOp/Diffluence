// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
	"time"
)

// The OpenTelemetry variables decide where traces go; they are shown with the source
// "environment", and anything that can hold a credential only as set (spec 075, CSL-604).
func TestTheTracingVariablesShowWithoutCredentials(t *testing.T) {
	environ := []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT=https://user:hunter2-secret@collector.example.com:4318/v1?api_key=hunter2-secret",
		"OTEL_EXPORTER_OTLP_HEADERS=authorization=Bearer hunter2-secret",
		"OTEL_EXPORTER_OTLP_TRACES_HEADERS=x-token=hunter2-secret",
		"OTEL_EXPORTER_OTLP_CLIENT_KEY=/run/secrets/hunter2-secret.key",
		"OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf",
		"OTEL_SERVICE_NAME=diffluence",
		"OTEL_RESOURCE_ATTRIBUTES=deployment.environment=production",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=not a url hunter2-secret",
		"OTEL_SDK_DISABLED=false",
		"HOME=/root",
		"DIFFLUENCE_TRACING=true",
	}
	got := map[string]Setting{}
	for _, s := range TracingVariables(environ) {
		got[s.Name] = s
		if strings.Contains(s.Value, "hunter2") {
			t.Errorf("%s shows a credential: %q", s.Name, s.Value)
		}
		if s.Source != sourceEnvironment {
			t.Errorf("%s: source %q", s.Name, s.Source)
		}
	}
	want := map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT":        "https://collector.example.com:4318/v1",
		"OTEL_EXPORTER_OTLP_HEADERS":         "(set)",
		"OTEL_EXPORTER_OTLP_TRACES_HEADERS":  "(set)",
		"OTEL_EXPORTER_OTLP_CLIENT_KEY":      "(set)",
		"OTEL_EXPORTER_OTLP_PROTOCOL":        "http/protobuf",
		"OTEL_SERVICE_NAME":                  "diffluence",
		"OTEL_RESOURCE_ATTRIBUTES":           "(set)",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "(set)",
	}
	for name, value := range want {
		if got[name].Value != value {
			t.Errorf("%s = %q, want %q", name, got[name].Value, value)
		}
	}
	if _, ok := got["HOME"]; ok || len(got) != len(want) {
		t.Fatalf("only the exporter's variables: %+v", got)
	}
}

// A setting whose value or source differs from what the process started with changes at
// the next restart, and the page shows both (CSL-601).
func TestASettingChangedAfterTheStartIsMarked(t *testing.T) {
	started := []Setting{
		{Name: "PUBLIC_URL", Value: "https://wiki.example.com", Source: "environment"},
		{Name: "LOG_LEVEL", Value: "info", Source: "default"},
		{Name: "SMTP_PASSWORD", Value: "(not set)", Source: "default"},
		{Name: "GONE", Value: "x", Source: "environment"},
	}
	now := []Setting{
		{Name: "PUBLIC_URL", Value: "https://wiki.example.com", Source: "environment"},
		{Name: "LOG_LEVEL", Value: "debug", Source: "file, line 3"},
		{Name: "SMTP_PASSWORD", Value: "(set)", Source: "environment"},
		{Name: "NEW", Value: "y", Source: "environment"},
	}
	rows := CompareWithStart(started, now)
	byName := map[string]Row{}
	for _, r := range rows {
		byName[r.Name] = r
	}
	if byName["PUBLIC_URL"].ChangesAtRestart {
		t.Fatal("an unchanged setting is marked")
	}
	level := byName["LOG_LEVEL"]
	if !level.ChangesAtRestart || level.Running == nil || level.Running.Value != "info" || level.Value != "debug" {
		t.Fatalf("a changed level: %+v", level)
	}
	if !byName["SMTP_PASSWORD"].ChangesAtRestart || byName["SMTP_PASSWORD"].Running.Value != "(not set)" {
		t.Fatalf("a secret set after the start: %+v", byName["SMTP_PASSWORD"])
	}
	if !byName["NEW"].ChangesAtRestart || byName["NEW"].Running != nil {
		t.Fatalf("a setting only a restart reads: %+v", byName["NEW"])
	}
	if len(rows) != len(now) {
		t.Fatalf("one row per setting a restart reads: %+v", rows)
	}
	if got := CompareWithStart(nil, now); got[0].ChangesAtRestart {
		t.Fatal("without a list from the start nothing can be compared, so nothing is marked")
	}
}

// The last try of the settings used only later is kept with its time, so the console shows
// it without reaching outside systems on every look (CSL-608).
func TestTheLastProbeIsKeptWithItsTime(t *testing.T) {
	var p ProbeRecord
	if _, _, done := p.Last(); done {
		t.Fatal("no probe ran yet")
	}
	at := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	p.Keep(at, []Warning{{Setting: "SMTP_HOST", Message: "the mail server does not accept a connection on port 25"}})
	when, warnings, done := p.Last()
	if !done || !when.Equal(at) || len(warnings) != 1 || warnings[0].Setting != "SMTP_HOST" {
		t.Fatalf("kept: %v %+v %v", when, warnings, done)
	}
	warnings[0].Setting = "changed by a reader"
	if _, again, _ := p.Last(); again[0].Setting != "SMTP_HOST" {
		t.Fatal("a reader changed what was kept")
	}
}
