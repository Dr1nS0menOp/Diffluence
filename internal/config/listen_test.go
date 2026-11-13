// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
)

// Without its own address a worker listens on the database the pool uses (spec 068 D8).
func TestTheListenAddressFallsBackToTheDatabase(t *testing.T) {
	c, err := Load(env(map[string]string{"DIFFLUENCE_DATABASE_URL": "postgres://app@bouncer/db"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenURL() != "postgres://app@bouncer/db" {
		t.Errorf("ListenURL = %q", c.ListenURL())
	}
	c, err = Load(env(map[string]string{"DIFFLUENCE_DATABASE_URL": "postgres://app@bouncer/db",
		"DIFFLUENCE_LISTEN_DATABASE_URL": "postgres://app@primary/db"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.ListenURL() != "postgres://app@primary/db" {
		t.Errorf("ListenURL = %q", c.ListenURL())
	}
}

// The listen address holds the password of the application role, so the report hides it.
func TestTheListenAddressIsShownWithoutItsPassword(t *testing.T) {
	_, settings, err := Inspect(env(map[string]string{"DIFFLUENCE_DATABASE_URL": "postgres://x",
		"DIFFLUENCE_LISTEN_DATABASE_URL": "postgres://app:secret@primary:5432/db"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range settings {
		if s.Name != "LISTEN_DATABASE_URL" {
			continue
		}
		if strings.Contains(s.Value, "secret") || !strings.Contains(s.Value, "primary:5432") {
			t.Fatalf("shown as %q", s.Value)
		}
		return
	}
	t.Fatal("LISTEN_DATABASE_URL is missing from the report")
}
