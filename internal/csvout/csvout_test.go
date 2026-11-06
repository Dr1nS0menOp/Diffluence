// SPDX-License-Identifier: Apache-2.0

package csvout

import (
	"bytes"
	"strings"
	"testing"
)

func TestEveryDangerousStartIsGuarded(t *testing.T) {
	for _, s := range []string{"=1+1", "+1", "-1", "@SUM(A1)", "\tx", "\rx", "\nx", "＝1", "＋1", "－1", "＠x"} {
		if got := Cell(s); got != "'"+s {
			t.Errorf("%q became %q", s, got)
		}
	}
}

func TestHarmlessCellsStayAsTheyAre(t *testing.T) {
	for _, s := range []string{"", "Plan", "1+1", "a=b"} {
		if got := Cell(s); got != s {
			t.Errorf("%q became %q", s, got)
		}
	}
}

func TestRowsAreGuardedAndQuoted(t *testing.T) {
	var b bytes.Buffer
	w, err := New(&b, "title", "views")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Row(`=HYPERLINK("x")`, "3"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"'=HYPERLINK(""x"")",3`) {
		t.Fatalf("got %q", b.String())
	}
}
