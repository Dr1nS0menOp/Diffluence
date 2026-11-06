// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/document"
)

func TestTitlePatternsFillInDateSpaceAndFields(t *testing.T) {
	today := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	got := ExpandTitle("{date} {Topic} in {space}", "Handbook", map[string]string{"Topic": "Plans"}, today)
	if got != "2026-11-05 Plans in Handbook" {
		t.Fatalf("got %q", got)
	}
	if got := ExpandTitle("{Unknown}", "x", nil, today); got != "{Unknown}" {
		t.Fatalf("a field without an answer stays: %q", got)
	}
}

func TestEveryStarterIsAValidDocumentInOurOwnWords(t *testing.T) {
	if len(starters) != 10 {
		t.Fatalf("starters: %d", len(starters))
	}
	seen := map[string]bool{}
	for _, st := range starters {
		if seen[st.key] {
			t.Fatalf("key used twice: %s", st.key)
		}
		seen[st.key] = true
		raw, err := document.Marshal(st.build())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := document.Normalize(raw); err != nil {
			t.Fatalf("%s: %v", st.key, err)
		}
	}
}
