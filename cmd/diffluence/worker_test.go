// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/config"
)

// The server of compose.yaml holds only the application role (spec 070). A command that needs
// the owner refuses there in one sentence that names where to run it, instead of trying as
// the application role.
func TestAContainerWithoutTheOwnerSaysWhereToMigrate(t *testing.T) {
	app := "postgres://app@db/diffluence"
	owner := "postgres://owner@db/diffluence"
	_, err := ownerURL(config.Config{DatabaseURL: app, MigrateOnStart: false})
	if !errors.Is(err, errNoOwner) || !strings.Contains(err.Error(), "backup migrate") {
		t.Fatalf("a server without the owner: %v", err)
	}
	for name, c := range map[string]config.Config{
		"the old layout":     {DatabaseURL: app, MigrateDatabaseURL: owner, MigrateOnStart: true},
		"the backup service": {DatabaseURL: app, MigrateDatabaseURL: owner, MigrateOnStart: false},
	} {
		if got, err := ownerURL(c); err != nil || got != owner {
			t.Errorf("%s: %q %v", name, got, err)
		}
	}
	// One role for everything, as in development: it migrates with that role.
	if got, err := ownerURL(config.Config{DatabaseURL: app, MigrateOnStart: true}); err != nil || got != app {
		t.Errorf("a single role: %q %v", got, err)
	}
}
