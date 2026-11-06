// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"encoding/base32"
	"strings"
	"testing"
)

// SEC-124: under 112 bits a plain hash is not enough, so a code holds 128.
func TestARecoveryCodeHolds128Bits(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		code := newRecoveryCode()
		if seen[code] {
			t.Fatalf("%s came twice", code)
		}
		seen[code] = true
		plain := strings.ToUpper(normalizeRecoveryCode(code))
		raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(plain)
		if err != nil {
			t.Fatalf("%s: %v", code, err)
		}
		if len(raw)*8 < 128 {
			t.Fatalf("%s holds %d bits", code, len(raw)*8)
		}
	}
}
