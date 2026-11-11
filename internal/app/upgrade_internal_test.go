// SPDX-License-Identifier: Apache-2.0

package app

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func problemTexts(problems []UpgradeProblem) string {
	texts := make([]string, len(problems))
	for i, p := range problems {
		texts[i] = p.Text
	}
	return strings.Join(texts, "; ")
}

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
		{"no backup after an earlier upgrade without one", func(f *upgradeFacts) { f.lastBackup = time.Time{} }, 24 * time.Hour, "no backup was noted"},
		{"no backup says to run the backup service", func(f *upgradeFacts) { f.lastBackup = time.Time{} }, 24 * time.Hour, "docker compose run --rm backup once"},
		{"no backup says how to go on once", func(f *upgradeFacts) { f.lastBackup = time.Time{} }, 24 * time.Hour, "diffluence migrate --without-backup"},
		{"a backup too old", func(f *upgradeFacts) { f.lastBackup = now.Add(-25 * time.Hour) }, 24 * time.Hour, "2026-11-08T11:00:00Z"},
		{"a backup too old says how to go on", func(f *upgradeFacts) { f.lastBackup = now.Add(-25 * time.Hour) }, 24 * time.Hour, "DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE=off"},
		{"a backup just inside the age", func(f *upgradeFacts) { f.lastBackup = now.Add(-24 * time.Hour) }, 24 * time.Hour, ""},
		{"no backup while none is asked for", func(f *upgradeFacts) { f.lastBackup = time.Time{} }, 0, ""},
		{"the first upgrade that never knew the rule", func(f *upgradeFacts) { f.lastBackup, f.firstWithoutBackup = time.Time{}, true }, 24 * time.Hour, ""},
		{"free space this system cannot tell", func(f *upgradeFacts) { f.freeBytes, f.freeKnown = 0, false }, 0, ""},
		{"exactly enough room", func(*upgradeFacts) {}, time.Hour, ""},
		{"no pg_trgm on the server", func(f *upgradeFacts) { f.trigrams = extensionMissing }, 0, "pg_trgm is not available"},
		{"pg_trgm that may not be created", func(f *upgradeFacts) { f.trigrams = extensionNotAllowed }, 0, "may not create the extension pg_trgm"},
	} {
		facts := healthy
		tc.change(&facts)
		problems := problemTexts(upgradeProblems(facts, now, tc.maxAge))
		if tc.want == "" && problems != "" {
			t.Errorf("%s: refused: %s", tc.name, problems)
		}
		if tc.want != "" && !strings.Contains(problems, tc.want) {
			t.Errorf("%s: want a reason with %q, got %q", tc.name, tc.want, problems)
		}
	}
}

// The waiver of ADR-042 is for the backup alone: a first upgrade with a full disk still waits.
func TestTheFirstUpgradeWaivesOnlyTheBackup(t *testing.T) {
	f := upgradeFacts{freeKnown: true, firstWithoutBackup: true}
	problems := upgradeProblems(f, time.Now(), 24*time.Hour)
	if got := (&UpgradeRefused{Problems: problems}).Kinds(); !slices.Equal(got, []ProblemKind{ProblemDisk, ProblemQueue}) {
		t.Fatalf("want disk and queue only, got %v", got)
	}
	if !waivesFirstUpgrade(f, 24*time.Hour) {
		t.Fatal("the waiver is not recorded")
	}
	if waivesFirstUpgrade(f, 0) {
		t.Fatal("a check switched off is no waiver to record")
	}
	f.lastBackup = time.Now()
	if waivesFirstUpgrade(f, 24*time.Hour) {
		t.Fatal("an upgrade with a backup is recorded as without one")
	}
}

func TestEveryProblemIsNamedAtOnce(t *testing.T) {
	problems := upgradeProblems(upgradeFacts{freeKnown: true}, time.Now(), time.Hour)
	kinds := (&UpgradeRefused{Problems: problems}).Kinds()
	if !slices.Equal(kinds, []ProblemKind{ProblemDisk, ProblemQueue, ProblemBackup}) {
		t.Fatalf("want disk, queue and backup, got %v", kinds)
	}
}

func TestARefusalNamesEachKindOnce(t *testing.T) {
	r := &UpgradeRefused{Problems: []UpgradeProblem{
		{ProblemExtension, "a"}, {ProblemBackup, "b"}, {ProblemExtension, "c"},
	}}
	if got := r.Kinds(); !slices.Equal(got, []ProblemKind{ProblemExtension, ProblemBackup}) {
		t.Fatalf("got %v", got)
	}
	if !strings.HasPrefix(r.Error(), "upgrade refused before migrating: a; b; c") {
		t.Fatalf("got %q", r.Error())
	}
}
