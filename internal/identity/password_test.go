// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"strings"
	"testing"
)

// fastParams keep unit tests quick; production uses DefaultParams.
var fastParams = Params{Memory: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

func TestHashAndVerify(t *testing.T) {
	h, err := HashPassword("correct horse battery staple", fastParams)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Fatalf("unexpected encoding %q", h)
	}
	ok, rehash, err := VerifyPassword(h, "correct horse battery staple", fastParams)
	if err != nil || !ok || rehash {
		t.Fatalf("ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	ok, _, err = VerifyPassword(h, "wrong horse battery staple", fastParams)
	if err != nil || ok {
		t.Fatalf("wrong password accepted: ok=%v err=%v", ok, err)
	}
}

func TestSaltIsRandom(t *testing.T) {
	a, _ := HashPassword("same password here", fastParams)
	b, _ := HashPassword("same password here", fastParams)
	if a == b {
		t.Fatal("two hashes of the same password are identical")
	}
}

func TestNeedsRehashWhenParamsIncrease(t *testing.T) {
	h, _ := HashPassword("correct horse battery staple", fastParams)
	stronger := fastParams
	stronger.Time = 2
	ok, rehash, err := VerifyPassword(h, "correct horse battery staple", stronger)
	if err != nil || !ok || !rehash {
		t.Fatalf("ok=%v rehash=%v err=%v", ok, rehash, err)
	}
}

func TestMalformedAndHostileHashes(t *testing.T) {
	for _, h := range []string{
		"",
		"plaintext",
		"$argon2i$v=19$m=64,t=1,p=1$c2FsdHNhbHRzYWx0$a2V5a2V5a2V5a2V5a2V5a2V5",
		"$argon2id$v=19$m=99999999,t=1,p=1$c2FsdHNhbHRzYWx0$a2V5a2V5a2V5a2V5a2V5a2V5", // memory bomb
		"$argon2id$v=19$m=64,t=999,p=1$c2FsdHNhbHRzYWx0$a2V5a2V5a2V5a2V5a2V5a2V5",     // cpu bomb
		"$argon2id$v=19$m=64,t=1,p=1$!!!$a2V5a2V5a2V5a2V5a2V5a2V5",
	} {
		if ok, _, err := VerifyPassword(h, "anything", fastParams); err == nil || ok {
			t.Errorf("hash %q: ok=%v err=%v, want error", h, ok, err)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	if err := ValidatePassword("short"); err != ErrPasswordTooShort {
		t.Errorf("short: %v", err)
	}
	if err := ValidatePassword(strings.Repeat("a", MaxPasswordBytes+1)); err != ErrPasswordTooLong {
		t.Errorf("long: %v", err)
	}
	if err := ValidatePassword("fifteen letters"); err != nil {
		t.Errorf("valid: %v", err)
	}
	// Too few characters but enough bytes: counting must be in characters.
	if err := ValidatePassword("ééééééééééééé"); err != ErrPasswordTooShort {
		t.Errorf("13 multibyte chars: %v", err)
	}
}
