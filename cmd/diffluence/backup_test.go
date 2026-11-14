// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The shell that removes old backups asks how many the backup service holds to, and removes
// nothing when the answer cannot be read (spec 076).
func TestTheShellIsToldHowManyBackupsTheServiceHoldsTo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".diffluence-held.json")
	var out bytes.Buffer
	if err := cmdBackupHeld([]string{"--held", path}, &out); err != nil || strings.TrimSpace(out.String()) != "0" {
		t.Fatalf("a service that holds nothing yet: %q %v", out.String(), err)
	}
	if err := os.WriteFile(path, []byte(`{"keep": 42, "interval_seconds": 14400}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := cmdBackupHeld([]string{"--held", path}, &out); err != nil || strings.TrimSpace(out.String()) != "42" {
		t.Fatalf("a service that holds 42: %q %v", out.String(), err)
	}
	if err := os.WriteFile(path, []byte(`{"keep": `), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := cmdBackupHeld([]string{"--held", path}, &out); err == nil || out.Len() != 0 {
		t.Fatalf("a damaged file gave a number: %q", out.String())
	}
}
