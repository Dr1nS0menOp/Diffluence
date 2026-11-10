// SPDX-License-Identifier: Apache-2.0

package app

import (
	"strings"
	"testing"
	"time"
)

func TestTheChecksBeforeAnUpgradeSayWhyTheyRefuse(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	healthy := upgradeFacts{freeBytes: minFreeForUpgrade, freeKnown: true, queueHealthy: true, lastBackup: now.Add(-time.Hour)}
	for _, tc := range []struct {
		name   string
		change func(*upgradeFacts)
		maxAge time.Duration
		want   string // "" = no refusal
	}{
		{"a full data folder", func(f *upgradeFacts) { f.freeBytes = minFreeForUpgrade - 1 }, 0, "511 MiB free and needs 512 MiB"},
		{"the queue is down", func(f *upgradeFacts) { f.queueHealthy = false }, 0, "queue (NATS)"},
		{"no backup while one is asked for", func(f *upgradeFacts) { f.lastBackup = time.Time{} }, 24 * time.Hour, "no backup was noted"},
		{"a backup too old", func(f *upgradeFacts) { f.lastBackup = now.Add(-25 * time.Hour) }, 24 * time.Hour, "2026-11-08T11:00:00Z"},
		{"a backup too old says how to go on", func(f *upgradeFacts) { f.lastBackup = now.Add(-25 * time.Hour) }, 24 * time.Hour, "DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE=off"},
		{"a backup just inside the age", func(f *upgradeFacts) { f.lastBackup = now.Add(-24 * time.Hour) }, 24 * time.Hour, ""},
		{"no backup while none is asked for", func(f *upgradeFacts) { f.lastBackup = time.Time{} }, 0, ""},
		{"free space this system cannot tell", func(f *upgradeFacts) { f.freeBytes, f.freeKnown = 0, false }, 0, ""},
		{"exactly enough room", func(*upgradeFacts) {}, time.Hour, ""},
		{"no pg_trgm on the server", func(f *upgradeFacts) { f.trigrams = extensionMissing }, 0, "pg_trgm is not available"},
		{"pg_trgm that may not be created", func(f *upgradeFacts) { f.trigrams = extensionNotAllowed }, 0, "may not create the extension pg_trgm"},
	} {
		facts := healthy
		tc.change(&facts)
		problems := strings.Join(upgradeProblems(facts, now, tc.maxAge), "; ")
		if tc.want == "" && problems != "" {
			t.Errorf("%s: refused: %s", tc.name, problems)
		}
		if tc.want != "" && !strings.Contains(problems, tc.want) {
			t.Errorf("%s: want a reason with %q, got %q", tc.name, tc.want, problems)
		}
	}
}

func TestEveryProblemIsNamedAtOnce(t *testing.T) {
	problems := upgradeProblems(upgradeFacts{freeKnown: true}, time.Now(), time.Hour)
	if len(problems) != 3 {
		t.Fatalf("want disk, queue and backup, got %q", problems)
	}
}
