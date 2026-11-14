// SPDX-License-Identifier: Apache-2.0

package backups

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var started = Defaults{Interval: 4 * time.Hour, Keep: 42}

// A worker that holds nothing yet starts from whatever keeps the most: the console's choice
// or its own defaults. Fewer than its defaults wait for a code, as any other lowering.
func TestAWorkerThatHoldsNothingYetKeepsTheMore(t *testing.T) {
	var h Held
	h.begin(7, 168*time.Hour, started)
	if h.Keep != 42 || h.IntervalSeconds != 4*3600 {
		t.Fatalf("from fewer than its defaults: %+v", h)
	}
	if !h.asksFewer(7, 168*time.Hour) {
		t.Fatal("fewer than its defaults are taken without a code")
	}
	var more Held
	more.begin(100, time.Hour, started)
	if more.Keep != 100 || more.IntervalSeconds != 3600 {
		t.Fatalf("from more than its defaults: %+v", more)
	}
}

// More backups, and more often, are held at once; fewer, or less often, never by follow.
func TestMoreBackupsAreHeldAtOnceAndFewerNever(t *testing.T) {
	h := Held{Keep: 42, IntervalSeconds: 4 * 3600}
	keep, interval := h.follow(1, 168*time.Hour)
	if keep != 42 || interval != 4*time.Hour {
		t.Fatalf("follow took fewer: %d %s", keep, interval)
	}
	keep, interval = h.follow(50, time.Hour)
	if keep != 50 || interval != time.Hour || h.Keep != 50 || h.IntervalSeconds != 3600 {
		t.Fatalf("follow did not take more: %d %s %+v", keep, interval, h)
	}
	if keep, _ = h.follow(42, time.Hour); keep != 50 {
		t.Fatalf("follow went back down after a raise: %d", keep)
	}
	if h.asksFewer(50, time.Hour) || !h.asksFewer(49, time.Hour) || !h.asksFewer(50, 2*time.Hour) {
		t.Fatalf("what counts as fewer: %+v", h)
	}
}

// A file that cannot be read is never a fresh start, which would take whatever the console
// says; a missing one is a worker that holds nothing yet.
func TestADamagedHeldFileIsNeverAFreshStart(t *testing.T) {
	dir := t.TempDir()
	missing, err := LoadHeld(filepath.Join(dir, "none.json"))
	if err != nil || missing.Keep != 0 {
		t.Fatalf("a missing file: %+v %v", missing, err)
	}
	for name, content := range map[string]string{"damaged": "{not json", "negative": `{"keep": -1}`} {
		path := filepath.Join(dir, name+".json")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadHeld(path); err == nil {
			t.Fatalf("a %s file was read", name)
		}
		if _, err := HeldKeep(path); err == nil {
			t.Fatalf("the shell was told a number from a %s file", name)
		}
	}
	path := filepath.Join(dir, "held.json")
	h, err := LoadHeld(path)
	if err != nil {
		t.Fatal(err)
	}
	h.Keep, h.IntervalSeconds = 9, 7200
	if err := h.Save(); err != nil {
		t.Fatal(err)
	}
	if keep, err := HeldKeep(path); err != nil || keep != 9 {
		t.Fatalf("the shell is told %d %v", keep, err)
	}
	if err := (&Held{}).Save(); err == nil {
		t.Fatal("a held state without a file was saved somewhere")
	}
}

// Every code is made from its own random part, the ask and the values, so a code seen once
// confirms nothing else.
func TestTheCodeOfAChallengeCoversTheAskAndTheValues(t *testing.T) {
	base := Challenge{Nonce: []byte("0123456789abcdef"), Keep: 7, IntervalSeconds: 3600}
	other := map[string]Challenge{
		"another random part": {Nonce: []byte("fedcba9876543210"), Keep: 7, IntervalSeconds: 3600},
		"another keep":        {Nonce: base.Nonce, Keep: 8, IntervalSeconds: 3600},
		"another interval":    {Nonce: base.Nonce, Keep: 7, IntervalSeconds: 7200},
	}
	for name, c := range other {
		if string(c.message()) == string(base.message()) {
			t.Fatalf("%s makes the same code", name)
		}
	}
	withAsk := base
	withAsk.Request[0] = 1
	if string(withAsk.message()) == string(base.message()) {
		t.Fatal("another ask makes the same code")
	}
}
