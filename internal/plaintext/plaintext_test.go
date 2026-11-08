// SPDX-License-Identifier: Apache-2.0

package plaintext

import "testing"

func TestDirectionMarksLeaveANameAndJoinersStay(t *testing.T) {
	cases := map[string]string{
		"invoice\u202efdp.exe":      "invoicefdp.exe",
		"\u2067plan\u2069 \u200fv2": "plan v2",
		"family 👨\u200d👩\u200d👧":    "family 👨\u200d👩\u200d👧",
		"שלום world":                "שלום world",
	}
	for in, want := range cases {
		if got := WithoutDirectionMarks(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
