// SPDX-License-Identifier: Apache-2.0

package settings

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/apperr"
)

// The disk thresholds have bounds, and the refuse level always stays below the warn level
// (spec 075, CSL-581, D13). Zero is the default of each.
func TestTheDiskThresholdsKeepToTheirBounds(t *testing.T) {
	for _, c := range []struct {
		warn, refuse int
		ok           bool
	}{
		{0, 0, true},
		{5, 1, true},
		{50, 10, true},
		{4, 1, false},
		{51, 2, false},
		{10, 11, false},
		{30, 11, false},
		{30, 10, true},
		{50, 0, true},
		{10, -1, false},
		{-1, 2, false},
		{5, 5, false},
		{6, 5, true},
		{10, 10, false},
		{0, 10, false},
		{0, 9, true},
		{5, 0, true},
		{2, 0, false},
	} {
		err := Health{DiskWarnPercent: c.warn, DiskRefusePercent: c.refuse}.validate()
		if (err == nil) != c.ok {
			t.Errorf("warn %d, refuse %d: %v, want ok %v", c.warn, c.refuse, err, c.ok)
		}
	}
}

// Warning later or refusing files later lets a disk fill further, so it is a confirmed
// change; warning or refusing sooner stays an administrator's (spec 076, spec 075 chapter 06).
func TestLowerDiskThresholdsNeedAConfirmedChange(t *testing.T) {
	stored := &Health{}
	for _, lower := range []Health{{DiskWarnPercent: 9}, {DiskRefusePercent: 1}, {DiskWarnPercent: 5, DiskRefusePercent: 1}} {
		_, err := lower.guard(Change{}, stored)
		var refused *apperr.Error
		if !errors.As(err, &refused) || refused.Code != CodeConfirmationRequired {
			t.Errorf("%+v from the defaults without confirmation: %v", lower, err)
		}
		if _, err := lower.guard(Change{Confirmed: true}, stored); err != nil {
			t.Errorf("%+v confirmed: %v", lower, err)
		}
	}
	for _, higher := range []Health{{}, {DiskWarnPercent: 10}, {DiskWarnPercent: 20}, {DiskRefusePercent: 5}, {DiskWarnPercent: 30, DiskRefusePercent: 9}} {
		if _, err := higher.guard(Change{}, stored); err != nil {
			t.Errorf("%+v is no lower than the defaults: %v", higher, err)
		}
	}
	// Back to the defaults from higher levels is lower.
	if _, err := (Health{}).guard(Change{}, &Health{DiskWarnPercent: 20}); err == nil {
		t.Error("the default warn level below a stored 20% passed as an ordinary change")
	}
}

func TestTheLevelsInForceFillInTheDefaults(t *testing.T) {
	if w, r := (Health{}).Levels(); w != 10 || r != 2 {
		t.Fatalf("defaults %d %d", w, r)
	}
	if w, r := (Health{DiskWarnPercent: 20, DiskRefusePercent: 5}).Levels(); w != 20 || r != 5 {
		t.Fatalf("set %d %d", w, r)
	}
}

// The log level changes for 1, 4 or 24 hours, to one of four levels, for every node or for
// one (CSL-570, D12).
func TestALogOverrideKeepsToItsLevelsAndHours(t *testing.T) {
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		in LogRequest
		ok bool
	}{
		{LogRequest{Level: "debug", Hours: 1}, true},
		{LogRequest{Level: "info", Hours: 4}, true},
		{LogRequest{Level: "warn", Hours: 24}, true},
		{LogRequest{Level: "error", Hours: 1, Node: "0195b4f2-8d40-7000-8000-000000000001"}, true},
		{LogRequest{Level: "trace", Hours: 1}, false},
		{LogRequest{Level: "DEBUG", Hours: 1}, false},
		{LogRequest{Level: "debug", Hours: 0}, false},
		{LogRequest{Level: "debug", Hours: 2}, false},
		{LogRequest{Level: "debug", Hours: 25}, false},
		{LogRequest{Level: "debug", Hours: 48}, false},
		{LogRequest{Level: "debug", Hours: 1, Node: "node-1"}, false},
	} {
		o, err := c.in.override(now)
		if (err == nil) != c.ok {
			t.Errorf("%+v: %v, want ok %v", c.in, err, c.ok)
			continue
		}
		if err == nil && !o.Until.Equal(now.Add(time.Duration(c.in.Hours)*time.Hour)) {
			t.Errorf("%+v ends %v", c.in, o.Until)
		}
	}
}

// A level quieter than the configured one hides warnings, so it is a confirmed change; a
// louder one is an administrator's (decided on the product owner's behalf, spec 075 D26).
func TestAQuieterLogLevelNeedsAConfirmedChange(t *testing.T) {
	now := time.Now()
	for _, level := range []string{"warn", "error"} {
		_, err := guardLog(LogRequest{Level: level, Hours: 1}, slog.LevelInfo, Change{At: now})
		var refused *apperr.Error
		if !errors.As(err, &refused) || refused.Code != CodeConfirmationRequired {
			t.Errorf("%s over info without confirmation: %v", level, err)
		}
		if _, err := guardLog(LogRequest{Level: level, Hours: 1}, slog.LevelInfo, Change{Confirmed: true, At: now}); err != nil {
			t.Errorf("%s confirmed: %v", level, err)
		}
	}
	for _, level := range []string{"debug", "info"} {
		if _, err := guardLog(LogRequest{Level: level, Hours: 24}, slog.LevelInfo, Change{At: now}); err != nil {
			t.Errorf("%s over info: %v", level, err)
		}
	}
}

// The override holds for its nodes until its time, and then the configured level returns
// without anyone asking (CSL-571).
func TestALogOverrideEndsByItsTimeAndHoldsOnlyForItsNodes(t *testing.T) {
	start := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	all, err := LogRequest{Level: "debug", Hours: 1}.override(start)
	if err != nil {
		t.Fatal(err)
	}
	if level, ok := all.LevelFor("any", start.Add(59*time.Minute)); !ok || level != slog.LevelDebug {
		t.Fatalf("within the hour: %v %v", level, ok)
	}
	if _, ok := all.LevelFor("any", start.Add(time.Hour)); ok {
		t.Fatal("the override outlived its hour")
	}
	one, err := LogRequest{Level: "error", Hours: 4, Node: "0195b4f2-8d40-7000-8000-000000000001"}.override(start)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := one.LevelFor("0195b4f2-8d40-7000-8000-000000000002", start); ok {
		t.Fatal("an override for one node held for another")
	}
	if level, ok := one.LevelFor("0195b4f2-8d40-7000-8000-000000000001", start); !ok || level != slog.LevelError {
		t.Fatalf("for its node: %v %v", level, ok)
	}
	var none *LogOverride
	if _, ok := none.LevelFor("any", start); ok {
		t.Fatal("no override held")
	}
	if !strings.Contains(LevelName(slog.LevelWarn), "warn") {
		t.Fatalf("level name %q", LevelName(slog.LevelWarn))
	}
}
