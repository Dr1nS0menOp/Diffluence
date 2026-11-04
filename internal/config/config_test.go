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
