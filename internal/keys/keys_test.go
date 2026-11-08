// SPDX-License-Identifier: Apache-2.0

package keys

import (
	"bytes"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
)

func TestSealOpen(t *testing.T) {
	k, err := LoadOrCreate(filepath.Join(t.TempDir(), "sub", "key"), true)
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("totp seed")
	a, err := k.Seal(secret, "totp:1")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := k.Seal(secret, "totp:1")
	if bytes.Equal(a, b) {
		t.Fatal("two seals of the same secret must differ")
	}
	if bytes.Contains(a, secret) {
		t.Fatal("the secret is readable in the sealed value")
	}
	got, err := k.Open(a, "totp:1")
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("open: %q %v", got, err)
	}
	if _, err := k.Open(a, "totp:2"); err == nil {
		t.Fatal("a value opened under another context")
	}
	a[len(a)-1] ^= 1
	if _, err := k.Open(a, "totp:1"); err == nil {
		t.Fatal("a changed value was accepted")
	}
	if _, err := k.Open([]byte{9, 9}, "x"); err == nil {
		t.Fatal("garbage was accepted")
	}
	other, _ := LoadOrCreate(filepath.Join(t.TempDir(), "key"), true)
	b2, _ := k.Seal(secret, "c")
	if _, err := other.Open(b2, "c"); err == nil {
		t.Fatal("another key opened the value")
	}
}

func TestKeyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	if _, err := LoadOrCreate(path, false); err == nil {
		t.Fatal("a missing key must be an error when create is false")
	}
	k1, err := LoadOrCreate(path, true)
	if err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("key file is readable by others: %v", info.Mode())
	}
	k2, err := LoadOrCreate(path, true)
	if err != nil || !bytes.Equal(k1.current.kek, k2.current.kek) {
		t.Fatal("the key must survive a restart")
	}
	s1, _ := k1.SigningKey()
	s2, _ := k2.SigningKey()
	if !s1.Equal(s2) {
		t.Fatal("the signing key must be stable")
	}
	msg := []byte("checkpoint")
	if !ed25519.Verify(s1.Public().(ed25519.PublicKey), msg, ed25519.Sign(s2, msg)) {
		t.Fatal("signature does not verify")
	}
	if err := os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(path, true); err == nil {
		t.Fatal("a damaged key file must be refused, not replaced")
	}
}
