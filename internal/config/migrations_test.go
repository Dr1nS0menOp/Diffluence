// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

// A process that does not migrate on start and holds no owner of the tables leaves every
// migration to another, as the server of compose.yaml does since spec 070. Holding the owner,
// or migrating on start, keeps the migrations here.
func TestOnlyAProcessWithoutTheOwnerThatDoesNotMigrateLeavesMigrations(t *testing.T) {
	for name, c := range map[string]struct {
		cfg    Config
		leaves bool
	}{
		"the server of compose.yaml": {Config{MigrateOnStart: false}, true},
		"an old compose.yaml":        {Config{MigrateOnStart: true, MigrateDatabaseURL: "postgres://owner@db/d"}, false},
		"the backup service":         {Config{MigrateOnStart: false, MigrateDatabaseURL: "postgres://owner@db/d"}, false},
		"one role for everything":    {Config{MigrateOnStart: true}, false},
	} {
		if got := c.cfg.LeavesMigrations(); got != c.leaves {
			t.Errorf("%s: leaves migrations %v, want %v", name, got, c.leaves)
		}
	}
}
