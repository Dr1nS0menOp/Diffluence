// SPDX-License-Identifier: Apache-2.0

package db

import (
	"errors"
	"strings"
	"testing"
)

func TestPostgresBelowTheMinimumIsRefused(t *testing.T) {
	for _, tc := range []struct {
		number  int
		text    string
		refused bool
	}{
		{150012, "15.12", true},
		{159999, "15.99", true},
		{160000, "16.0", false},
		{160004, "16.4 (Debian 16.4-1)", false},
		{180001, "18.1", false},
	} {
		err := checkPostgresVersion(tc.number, tc.text)
		if refused := errors.Is(err, ErrPostgresTooOld); refused != tc.refused {
			t.Errorf("%d: refused %v, want %v (%v)", tc.number, refused, tc.refused, err)
		}
		if tc.refused && !strings.Contains(err.Error(), tc.text) {
			t.Errorf("%d: the reason does not name the version that runs: %v", tc.number, err)
		}
	}
}
