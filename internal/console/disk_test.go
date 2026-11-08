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
		percent, full := DiskNearlyFull(c.free, c.total)
		if percent != c.percent || full != c.full {
			t.Fatalf("DiskNearlyFull(%d, %d) = %d, %v; want %d, %v", c.free, c.total, percent, full, c.percent, c.full)
		}
	}
}
