// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) LookupFunc {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestDefaults(t *testing.T) {
	c, err := Load(env(map[string]string{"DIFFLUENCE_DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Role != RoleAll || !c.Role.RunsAPI() || !c.Role.RunsWorker() {
		t.Errorf("role = %q", c.Role)
	}
	if c.HTTPAddr != ":8080" || !c.MigrateOnStart || c.SessionIdleTimeout != 8*time.Hour {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.SecureCookies() {
		t.Error("http public URL must not use secure cookies")
	}
	if c.PublicOrigin() != "http://localhost:8080" {
		t.Errorf("origin = %q", c.PublicOrigin())
	}
}

func TestFileVariant(t *testing.T) {
	p := filepath.Join(t.TempDir(), "db")
	if err := os.WriteFile(p, []byte("postgres://from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(env(map[string]string{"DIFFLUENCE_DATABASE_URL_FILE": p}))
	if err != nil {
		t.Fatal(err)
	}
	if c.DatabaseURL != "postgres://from-file" {
		t.Errorf("DatabaseURL = %q", c.DatabaseURL)
	}
}

func TestBothVariantsIsAnError(t *testing.T) {
	_, err := Load(env(map[string]string{
		"DIFFLUENCE_DATABASE_URL":      "a",
		"DIFFLUENCE_DATABASE_URL_FILE": "/nonexistent",
	}))
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("want 'not both' error, got %v", err)
	}
}

func TestErrorsAreCollected(t *testing.T) {
	_, err := Load(env(map[string]string{
		"DIFFLUENCE_ROLE":            "api", // also requires NATS_URL
		"DIFFLUENCE_PUBLIC_URL":      "https://wiki.example.com/sub/path",
		"DIFFLUENCE_LOG_LEVEL":       "loud",
		"DIFFLUENCE_TRUSTED_PROXIES": "10.0.0.0/8, nonsense",
	}))
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{"DATABASE_URL", "NATS_URL", "PUBLIC_URL", "LOG_LEVEL", "TRUSTED_PROXIES"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
}

// ADR-040: an upgrade asks for a backup of at most 24 hours unless the operator switches
// the check off on purpose with "off". Zero or a negative age is a mistake, not "off".
func TestTheBackupAgeForAnUpgradeIsADayByDefaultAndOffOnlyOnPurpose(t *testing.T) {
	c, err := Load(env(map[string]string{"DIFFLUENCE_DATABASE_URL": "postgres://x"}))
	if err != nil || c.UpgradeBackupMaxAge != 24*time.Hour {
		t.Fatalf("default %v, err %v", c.UpgradeBackupMaxAge, err)
	}
	c, err = Load(env(map[string]string{"DIFFLUENCE_DATABASE_URL": "postgres://x", "DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE": "26h"}))
	if err != nil || c.UpgradeBackupMaxAge != 26*time.Hour {
		t.Fatalf("set %v, err %v", c.UpgradeBackupMaxAge, err)
	}
	c, err = Load(env(map[string]string{"DIFFLUENCE_DATABASE_URL": "postgres://x", "DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE": "off"}))
	if err != nil || c.UpgradeBackupMaxAge != 0 {
		t.Fatalf("off %v, err %v", c.UpgradeBackupMaxAge, err)
	}
	for _, bad := range []string{"-1h", "0", "0s", "never", "yes"} {
		_, err = Load(env(map[string]string{"DIFFLUENCE_DATABASE_URL": "postgres://x", "DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE": bad}))
		if err == nil || !strings.Contains(err.Error(), "UPGRADE_BACKUP_MAX_AGE") || !strings.Contains(err.Error(), "off") {
			t.Fatalf("%q: want a refusal that names off, got %v", bad, err)
		}
	}
}

func TestHTTPSAndProxies(t *testing.T) {
	c, err := Load(env(map[string]string{
		"DIFFLUENCE_DATABASE_URL":    "postgres://x",
		"DIFFLUENCE_PUBLIC_URL":      "https://wiki.example.com",
		"DIFFLUENCE_TRUSTED_PROXIES": "10.1.2.3/8,::1/128",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !c.SecureCookies() {
		t.Error("https must use secure cookies")
	}
	if len(c.TrustedProxies) != 2 || c.TrustedProxies[0].String() != "10.0.0.0/8" {
		t.Errorf("proxies = %v", c.TrustedProxies)
	}
}

// Cookies and HSTS follow the public address, so a plain address that others reach must be
// said out loud (SEC-257). On this machine alone, plain http is how people try it out.
func TestAPlainAddressIsWeakOnlyWhenOthersReachIt(t *testing.T) {
	for address, weak := range map[string]bool{
		"http://wiki.example.com":   true,
		"http://192.0.2.10:8080":    true,
		"http://[2001:db8::1]:8080": true,
		"https://wiki.example.com":  false,
		"http://localhost:8080":     false,
		"http://127.0.0.1:8080":     false,
		"http://[::1]:8080":         false,
	} {
		c, err := Load(env(map[string]string{"DIFFLUENCE_DATABASE_URL": "postgres://x", "DIFFLUENCE_PUBLIC_URL": address}))
		if err != nil {
			t.Fatalf("%s: %v", address, err)
		}
		if got := c.PlainToOthers(); got != weak {
			t.Errorf("%s: plain to others = %v, want %v", address, got, weak)
		}
	}
}

func TestTheDrawingServiceIsOptionalAndNeverOurOwnOrigin(t *testing.T) {
	load := func(address string) (Config, error) {
		return Load(env(map[string]string{
			"DIFFLUENCE_DATABASE_URL": "postgres://x",
			"DIFFLUENCE_PUBLIC_URL":   "https://wiki.example.com",
			"DIFFLUENCE_DRAWING_URL":  address,
		}))
	}
	c, err := load("")
	if err != nil || c.DrawingOrigin() != "" {
		t.Fatalf("off by default: %q %v", c.DrawingOrigin(), err)
	}
	c, err = load("https://draw.example.com")
	if err != nil || c.DrawingOrigin() != "https://draw.example.com" {
		t.Fatalf("own address: %q %v", c.DrawingOrigin(), err)
	}
	for _, bad := range []string{"https://wiki.example.com", "HTTPS://WIKI.example.com", "ftp://draw.example.com",
		"https://draw.example.com/path", "https://user@draw.example.com", "https://draw.example.com?a=1", "draw.example.com"} {
		if _, err := load(bad); err == nil || !strings.Contains(err.Error(), "DRAWING_URL") {
			t.Errorf("%s: want a DRAWING_URL error, got %v", bad, err)
		}
	}
}
