// SPDX-License-Identifier: Apache-2.0

package leaked_test

import (
	"fmt"
	"testing"

	"github.com/diffluence/diffluence/internal/identity/leaked"
)

func TestAPasswordThatWasAddedIsAlwaysFound(t *testing.T) {
	f := leaked.New(1000, 16, 11)
	for i := range 1000 {
		f.Add(fmt.Sprintf("password-%d", i))
	}
	for i := range 1000 {
		if !f.Contains(fmt.Sprintf("password-%d", i)) {
			t.Fatalf("lost %d", i)
		}
	}
	if !f.Contains("PASSWORD-7") {
		t.Fatal("case must not matter")
	}
}

func TestFalsePositivesStayRare(t *testing.T) {
	f := leaked.New(10_000, 16, 11)
	for i := range 10_000 {
		f.Add(fmt.Sprintf("listed-%d", i))
	}
	wrong := 0
	for i := range 100_000 {
		if f.Contains(fmt.Sprintf("not-listed-%d", i)) {
			wrong++
		}
	}
	if wrong > 300 {
		t.Fatalf("%d of 100000 wrongly found", wrong)
	}
}

func TestAFilterSurvivesTheFileFormat(t *testing.T) {
	f := leaked.New(100, 16, 11)
	f.Add("kept")
	back, err := leaked.Parse(f.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !back.Contains("kept") || back.Contains("something else entirely") {
		t.Fatal("the filter changed")
	}
	if _, err := leaked.Parse([]byte("not a filter at all, sorry")); err == nil {
		t.Fatal("a damaged file must be refused")
	}
	if _, err := leaked.Parse(f.Bytes()[:40]); err == nil {
		t.Fatal("a cut-off file must be refused")
	}
}

func TestTheFilterInTheBinaryKnowsLeakedPasswords(t *testing.T) {
	f, err := leaked.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	for _, password := range []string{"password", "123456", "qwerty", "1qaz2wsx3edc4rfv", "123456789987654321", "MomSanalAdventure"} {
		if !f.Contains(password) {
			t.Errorf("%q is not in the filter", password)
		}
	}
	if f.Contains("correct horse battery staple plus something nobody wrote") {
		t.Fatal("a long passphrase of our own is in the filter")
	}
}
