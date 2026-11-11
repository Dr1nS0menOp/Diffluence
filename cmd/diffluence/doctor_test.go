// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/keys"
)

// doctorEnv is a migrated installation with its key, as a running server leaves it.
func doctorEnv(t *testing.T) string {
	t.Helper()
	url := commandEnv(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"migrate"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("migrate: %d %s", code, errOut.String())
	}
	if _, err := keys.LoadOrCreate(filepath.Join(os.Getenv("DIFFLUENCE_DATA_DIR"), "master.key"), true); err != nil {
		t.Fatal(err)
	}
	return url
}

func TestDoctorReportsEveryCheckAndNoSecret(t *testing.T) {
	url := doctorEnv(t)
	var out, errOut bytes.Buffer
	code := run([]string{"doctor"}, strings.NewReader(""), &out, &errOut)
	report := out.String()
	// A fresh installation has no backup and no mail server: warnings, not problems.
	if code != 0 {
		t.Fatalf("doctor on a fresh installation: %d\n%s\n%s", code, report, errOut.String())
	}
	for _, want := range []string{"configuration: ok", "disk", "database", "backup", "migrations", "work",
		"mail", "login", "version", "No backup was ever noted", "keys: ok", "0 problem(s)"} {
		if !strings.Contains(report, want) {
			t.Errorf("the report lacks %q:\n%s", want, report)
		}
	}
	for _, secret := range []string{url, "postgres://", "password="} {
		if strings.Contains(report, secret) {
			t.Fatalf("the report shows %q:\n%s", secret, report)
		}
	}
}

func TestDoctorFailsWhenTheKeyIsMissing(t *testing.T) {
	commandEnv(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"migrate"}, strings.NewReader(""), &out, &errOut); code != 0 {
		t.Fatalf("migrate: %d %s", code, errOut.String())
	}
	out.Reset()
	code := run([]string{"doctor"}, strings.NewReader(""), &out, &errOut)
	if code == 0 {
		t.Fatalf("a missing key is a problem:\n%s", out.String())
	}
	// The other checks still run and are reported.
	if !strings.Contains(out.String(), "keys: PROBLEM") || !strings.Contains(out.String(), "backup") {
		t.Fatalf("the report:\n%s", out.String())
	}
}

func TestDoctorFailsOnAConfigurationItCannotUse(t *testing.T) {
	commandEnv(t)
	t.Setenv("DIFFLUENCE_ROLE", "nonsense")
	var out, errOut bytes.Buffer
	if code := run([]string{"doctor"}, strings.NewReader(""), &out, &errOut); code == 0 {
		t.Fatalf("a wrong role is a problem:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "configuration: PROBLEM") {
		t.Fatalf("the report:\n%s", out.String())
	}
}
