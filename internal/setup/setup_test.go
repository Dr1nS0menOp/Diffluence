// SPDX-License-Identifier: Apache-2.0

package setup

import (
	"strings"
	"testing"
)

func TestACodeIsReadTheWayPeopleTypeIt(t *testing.T) {
	want := "ABCD-EFGH-2345-67AB"
	for _, typed := range []string{"ABCD-EFGH-2345-67AB", "abcd efgh 2345 67ab", " abcdEFGH234567ab\n", "AB-CD-EF-GH-23-45-67-AB"} {
		if got := format(typed); got != want {
			t.Fatalf("%q read as %q, want %q", typed, got, want)
		}
	}
	if format("") != "" {
		t.Fatal("an empty code stays empty")
	}
}

func TestDifferentCodesHaveDifferentDigests(t *testing.T) {
	if string(digest("ABCD-EFGH")) == string(digest("ABCD-EFGJ")) {
		t.Fatal("digests must differ")
	}
	if len(digest("")) != len(digest("ABCD-EFGH-2345-67AB")) {
		t.Fatal("every digest has the same length, so comparing them takes the same time")
	}
}

func TestTheAnnouncementCarriesTheCodeAndTheAddress(t *testing.T) {
	line := Announcement("https://wiki.example.com/", "ABCD-EFGH")
	if !strings.Contains(line, "https://wiki.example.com/setup") || !strings.Contains(line, "ABCD-EFGH") {
		t.Fatalf("announcement: %q", line)
	}
}
