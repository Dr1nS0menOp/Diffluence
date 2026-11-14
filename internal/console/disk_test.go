// SPDX-License-Identifier: Apache-2.0

package console

import "testing"

func TestDiskNearlyFullWarnsBelowATenthAndNeverOnAnUnknownSize(t *testing.T) {
	cases := []struct {
		free, total uint64
		percent     int
		full        bool
	}{
		{free: 0, total: 0, percent: 0, full: false},
		{free: 5, total: 100, percent: 5, full: true},
		{free: 9, total: 100, percent: 9, full: true},
		{free: 10, total: 100, percent: 10, full: false},
		{free: 80, total: 100, percent: 80, full: false},
	}
	for _, c := range cases {
		percent, full := DiskNearlyFull(c.free, c.total, 0)
		if percent != c.percent || full != c.full {
			t.Fatalf("DiskNearlyFull(%d, %d, 0) = %d, %v; want %d, %v", c.free, c.total, percent, full, c.percent, c.full)
		}
	}
}

// The attention list names the disk at the level an administrator set in the health
// section, not at the default (CSL-581).
func TestDiskNearlyFullFollowsTheWarnLevelOfTheHealthSettings(t *testing.T) {
	cases := []struct {
		free, warn int
		full       bool
	}{
		{free: 15, warn: 20, full: true},
		{free: 19, warn: 20, full: true},
		{free: 20, warn: 20, full: false},
		{free: 6, warn: 5, full: false},
		{free: 4, warn: 5, full: true},
	}
	for _, c := range cases {
		percent, full := DiskNearlyFull(uint64(c.free), 100, c.warn)
		if percent != c.free || full != c.full {
			t.Fatalf("DiskNearlyFull(%d, 100, %d) = %d, %v; want %d, %v", c.free, c.warn, percent, full, c.free, c.full)
		}
	}
}
