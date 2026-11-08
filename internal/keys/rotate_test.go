// SPDX-License-Identifier: Apache-2.0

package keys

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func newKeyFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "master.key")
	if _, err := LoadOrCreate(path, true); err != nil {
		t.Fatal(err)
	}
	return path
}

func load(t *testing.T, path string) *Keyring {
	t.Helper()
	k, err := LoadOrCreate(path, false)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestAValueNamingAKeyThatIsNotLoadedFailsClearly(t *testing.T) {
	path := newKeyFile(t)
	sealed, err := load(t, path).Seal([]byte("seed"), "totp:1")
	if err != nil {
		t.Fatal(err)
	}
	other := load(t, newKeyFile(t))
	if _, err := other.Open(sealed, "totp:1"); !errors.Is(err, ErrNoKey) {
		t.Fatalf("a value of an unknown key must fail with ErrNoKey, got %v", err)
	}
}

func TestAChangedKeyIDDoesNotOpenAValue(t *testing.T) {
	k := load(t, newKeyFile(t))
	sealed, _ := k.Seal([]byte("seed"), "c")
	sealed[1] ^= 1
	if _, err := k.Open(sealed, "c"); err == nil {
		t.Fatal("a value with a changed key id opened")
	}
}

func TestAKeyFileOfTheWrongLengthIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "master.key")
	for _, body := range [][]byte{make([]byte, 31), make([]byte, 33), append([]byte("DFK2"), make([]byte, 63)...)} {
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadOrCreate(path, false); err == nil {
			t.Fatalf("a key file of %d bytes was accepted", len(body))
		}
	}
}

func TestADamagedNextKeyStopsTheStartInsteadOfBeingIgnored(t *testing.T) {
	path := newKeyFile(t)
	if err := os.WriteFile(path+NextSuffix, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreate(path, false); err == nil {
		t.Fatal("a damaged next key must be an error")
	}
}

func TestValuesFromBeforeKeyIDsStillOpen(t *testing.T) {
	path := newKeyFile(t)
	k := load(t, path)
	// A value in the first layout: format 1, the length of the wrapped key, then the rest.
	legacy, err := sealLegacy(k.current.kek, []byte("seed"), "totp:1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := IDOf(legacy); ok {
		t.Fatal("a value from before key ids must have no id")
	}
	got, err := k.Open(legacy, "totp:1")
	if err != nil || string(got) != "seed" {
		t.Fatalf("legacy value: %q %v", got, err)
	}
	if k.SealedWithCurrent(legacy) {
		t.Fatal("a legacy value must count as needing a new seal")
	}
}

func sealLegacy(kek, plaintext []byte, context string) ([]byte, error) {
	dek := make([]byte, kekSize)
	wrapped, err := seal(kek, dek, []byte(context))
	if err != nil {
		return nil, err
	}
	body, err := seal(dek, plaintext, []byte(context))
	if err != nil {
		return nil, err
	}
	out := append([]byte{legacyFormat, byte(len(wrapped))}, wrapped...)
	return append(out, body...), nil
}

func TestTheKeyCanBeReplacedWhileEveryValueStaysReadable(t *testing.T) {
	path := newKeyFile(t)
	before := load(t, path)
	oldValue, _ := before.Seal([]byte("old"), "c")
	fingerprint, _ := before.Fingerprint("login address", []byte("a@example.com"))
	signing, _ := before.SigningKey()

	nextID, err := AddNext(path)
	if err != nil {
		t.Fatal(err)
	}
	during := load(t, path)
	if during.KeyID() != before.KeyID() {
		t.Fatal("a waiting key must not seal yet")
	}
	if len(during.KeyIDs()) != 2 || during.KeyIDs()[1] != nextID {
		t.Fatalf("the waiting key must be loaded to open: %v", during.KeyIDs())
	}

	promoted, err := Promote(path)
	if err != nil || promoted != nextID {
		t.Fatalf("promote: %s %v", promoted, err)
	}
	after := load(t, path)
	if after.KeyID() != nextID {
		t.Fatal("the new key must seal after promote")
	}
	if got, err := after.Open(oldValue, "c"); err != nil || string(got) != "old" {
		t.Fatalf("a value of the old key must still open: %q %v", got, err)
	}
	newValue, _ := after.Seal([]byte("new"), "c")
	if !after.SealedWithCurrent(newValue) || after.SealedWithCurrent(oldValue) {
		t.Fatal("SealedWithCurrent must tell the two keys apart")
	}
	again, _ := after.Fingerprint("login address", []byte("a@example.com"))
	if !bytes.Equal(fingerprint, again) {
		t.Fatal("fingerprints must survive a new key")
	}
	if s, _ := after.SigningKey(); !s.Equal(signing) {
		t.Fatal("the signing key must survive a new key")
	}

	if err := RetirePrevious(path); err != nil {
		t.Fatal(err)
	}
	retired := load(t, path)
	if _, err := retired.Open(oldValue, "c"); !errors.Is(err, ErrNoKey) {
		t.Fatalf("after retiring, the old key must be gone: %v", err)
	}
	if got, err := retired.Open(newValue, "c"); err != nil || string(got) != "new" {
		t.Fatal("a value of the new key must open after retiring")
	}
}

func TestPromoteRunAgainAfterACrashFinishesTheJob(t *testing.T) {
	path := newKeyFile(t)
	value, _ := load(t, path).Seal([]byte("v"), "c")
	if _, err := AddNext(path); err != nil {
		t.Fatal(err)
	}
	// A crash after the old key was saved but before the key file changed.
	old, _ := os.ReadFile(path)
	if err := os.WriteFile(path+PreviousSuffix, old, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := load(t, path).Open(value, "c"); err != nil || string(got) != "v" {
		t.Fatal("halfway, every value must still open")
	}
	if _, err := Promote(path); err != nil {
		t.Fatalf("promote again: %v", err)
	}
	// A crash after the key file changed but before next was removed.
	if _, err := Promote(path); !errors.Is(err, ErrNoNext) {
		t.Fatalf("a finished promote leaves nothing waiting: %v", err)
	}
	if got, err := load(t, path).Open(value, "c"); err != nil || string(got) != "v" {
		t.Fatal("after promote, the old value must open")
	}
}

func TestANewChangeWaitsUntilTheLastOneIsFinished(t *testing.T) {
	path := newKeyFile(t)
	if _, err := AddNext(path); err != nil {
		t.Fatal(err)
	}
	if _, err := AddNext(path); err == nil {
		t.Fatal("a second waiting key must be refused")
	}
	if _, err := Promote(path); err != nil {
		t.Fatal(err)
	}
	if _, err := AddNext(path); !errors.Is(err, ErrPreviousPresent) {
		t.Fatalf("a new key while the old one is still in use must be refused: %v", err)
	}
	if _, err := Promote(path); !errors.Is(err, ErrNoNext) {
		t.Fatalf("promote without a waiting key: %v", err)
	}
}

func TestPromoteRefusesToOverwriteAnOlderKeyStillInUse(t *testing.T) {
	path := newKeyFile(t)
	if err := os.WriteFile(path+PreviousSuffix, make([]byte, kekSize), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+NextSuffix, bytes.Repeat([]byte{7}, kekSize), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Promote(path); !errors.Is(err, ErrPreviousPresent) {
		t.Fatalf("promote must not overwrite another old key: %v", err)
	}
	if prev, _ := os.ReadFile(path + PreviousSuffix); !bytes.Equal(prev, make([]byte, kekSize)) {
		t.Fatal("the older key must be left as it was")
	}
}
