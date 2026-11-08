// SPDX-License-Identifier: Apache-2.0

package identity

import "testing"

func TestANewPasswordCannotHoldTheNameOfTheProduct(t *testing.T) {
	s := &Service{}
	if err := s.validateNewPassword("my Diffluence account 2026", "sam@example.org", "Sam Rivera"); err == nil {
		t.Fatal("a password with the product's name was accepted")
	}
	if err := s.validateNewPassword("a quiet river through the hills", "sam@example.org", "Sam Rivera"); err != nil {
		t.Fatalf("an ordinary password was refused: %v", err)
	}
}
