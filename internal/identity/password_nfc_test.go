// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"testing"
)

// The same password typed on two systems can arrive composed or decomposed (SEC-107).
const (
	composedPassword   = "Ångström and the café of fifteen"
	decomposedPassword = "Ångström and the café of fifteen"
)

func TestAPasswordIsTheSameComposedOrDecomposed(t *testing.T) {
	if composedPassword == decomposedPassword {
		t.Fatal("the two forms must differ as bytes")
	}
	h, err := HashPassword(context.Background(), decomposedPassword, fastParams)
	if err != nil {
		t.Fatal(err)
	}
	ok, _, err := VerifyPassword(context.Background(), h, composedPassword, fastParams)
	if err != nil || !ok {
		t.Fatalf("the composed form was refused: ok=%v err=%v", ok, err)
	}
	ok, _, err = VerifyPassword(context.Background(), h, decomposedPassword, fastParams)
	if err != nil || !ok {
		t.Fatalf("the decomposed form was refused: ok=%v err=%v", ok, err)
	}
}

func TestAHashMadeBeforeNormalisingStillOpensAndAsksForARehash(t *testing.T) {
	// A hash from before this change holds the bytes as they were typed.
	salt := make([]byte, fastParams.SaltLen)
	key, err := hashKey(context.Background(), decomposedPassword, salt, fastParams, fastParams.KeyLen)
	if err != nil {
		t.Fatal(err)
	}
	old := encodeHash(fastParams, salt, key)
	ok, rehash, err := VerifyPassword(context.Background(), old, decomposedPassword, fastParams)
	if err != nil || !ok || !rehash {
		t.Fatalf("an old hash: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	if ok, _, _ := VerifyPassword(context.Background(), old, "something else entirely here", fastParams); ok {
		t.Fatal("a wrong password opened an old hash")
	}
}
