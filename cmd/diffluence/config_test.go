// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// configFile writes a configuration file readable by its owner alone and names it.
func configFile(t *testing.T, body string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "diffluence.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DIFFLUENCE_CONFIG_FILE", p)
}

func TestConfigCheckPrintsEverySettingWithItsSourceAndNoSecret(t *testing.T) {
	configFile(t, "database_url: postgres://owner:dbpass@db:5432/wiki\nmetrics_token: s3cret-token\nlog_level: debug\n")
	t.Setenv("DIFFLUENCE_LOG_LEVEL", "warn")
	var out, errOut bytes.Buffer
	if code := run([]string{"config", "check", "--offline"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("config check: %d\n%s\n%s", code, out.String(), errOut.String())
	}
	report := out.String()
	for _, want := range []string{"LOG_LEVEL", "warn", "environment, over the file", "METRICS_TOKEN", "(set)",
		"file, line 2", "HTTP_ADDR", "default", "configuration: ok"} {
		if !strings.Contains(report, want) {
			t.Errorf("the report lacks %q:\n%s", want, report)
		}
	}
	for _, secret := range []string{"dbpass", "s3cret-token"} {
		if strings.Contains(report, secret) {
			t.Fatalf("the report shows %q:\n%s", secret, report)
		}
	}
}

func TestConfigCheckExitsOneOnABadFile(t *testing.T) {
	configFile(t, "database_url: postgres://x\nlog_levle: debug\n")
	var out, errOut bytes.Buffer
	code := run([]string{"config", "check", "--offline"}, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Fatalf("config check of a file with a typing mistake: %d\n%s", code, out.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "log_levle") {
		t.Errorf("the mistake is not named:\n%s\n%s", out.String(), errOut.String())
	}
}

// A setting that does not work yet is a warning, not a refusal.
func TestConfigCheckWarnsAboutAMailServerThatDoesNotAnswer(t *testing.T) {
	configFile(t, "database_url: postgres://x\nsmtp:\n  host: mail.diffluence.invalid\n  from: w@example.com\n")
	var out, errOut bytes.Buffer
	code := run([]string{"config", "check"}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("a warning must not fail the check: %d\n%s\n%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "WARN") || !strings.Contains(out.String(), "SMTP_HOST") {
		t.Errorf("no warning for the mail server:\n%s", out.String())
	}
}

func TestConfigWithoutCheckIsAUsageError(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"config"}, strings.NewReader(""), &out, &errOut); code != 2 {
		t.Fatalf("config alone: %d", code)
	}
}
