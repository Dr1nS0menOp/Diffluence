// SPDX-License-Identifier: Apache-2.0

package storage_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/diffluence/diffluence/internal/keys"
	"github.com/diffluence/diffluence/internal/storage"
)

func keyringAt(t *testing.T, path string) *keys.Keyring {
	t.Helper()
	k, err := keys.LoadOrCreate(path, true)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestAFileIsSealedAgainUnderTheNewKeyAndStaysTheSame(t *testing.T) {
	ctx := context.Background()
	keyPath := filepath.Join(t.TempDir(), "master.key")
	disk, err := storage.NewDisk(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	before := storage.NewEncrypted(disk, keyringAt(t, keyPath), notDone)
	body := randomBytes(t, 3*chunk+17)
	if _, err := before.Put(ctx, "attachments/a", bytes.NewReader(body), int64(len(body))); err != nil {
		t.Fatal(err)
	}
	oldID := keyringAt(t, keyPath).KeyID()

	if _, err := keys.AddNext(keyPath); err != nil {
		t.Fatal(err)
	}
	newID, err := keys.Promote(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	after := storage.NewEncrypted(disk, keyringAt(t, keyPath), notDone)
	wrapped, err := after.WrappedKey(ctx, "attachments/a")
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := keys.IDOf(wrapped); id != oldID {
		t.Fatalf("before resealing the file names the old key, got %s", id)
	}
	if err := after.Reseal(ctx, "attachments/a"); err != nil {
		t.Fatal(err)
	}
	wrapped, _ = after.WrappedKey(ctx, "attachments/a")
	if id, _ := keys.IDOf(wrapped); id != newID {
		t.Fatalf("after resealing the file must name the new key, got %s", id)
	}
	if err := keys.RetirePrevious(keyPath); err != nil {
		t.Fatal(err)
	}
	retired := storage.NewEncrypted(disk, keyringAt(t, keyPath), notDone)
	got, err := readAll(t, retired, "attachments/a")
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("the file must read the same with only the new key: %v", err)
	}
}

func TestAFileOfARetiredKeyDoesNotOpenAndIsNotResealed(t *testing.T) {
	ctx := context.Background()
	keyPath := filepath.Join(t.TempDir(), "master.key")
	disk, root, _ := newStores(t)
	enc := storage.NewEncrypted(disk, keyringAt(t, keyPath), notDone)
	if _, err := enc.Put(ctx, "attachments/a", bytes.NewReader([]byte("x")), 1); err != nil {
		t.Fatal(err)
	}
	stored, _ := os.ReadFile(filepath.Join(root, "attachments", "a"))
	if _, err := keys.AddNext(keyPath); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Promote(keyPath); err != nil {
		t.Fatal(err)
	}
	if err := keys.RetirePrevious(keyPath); err != nil {
		t.Fatal(err)
	}
	only := storage.NewEncrypted(disk, keyringAt(t, keyPath), notDone)
	if _, err := only.Open(ctx, "attachments/a"); !errors.Is(err, storage.ErrUnreadable) {
		t.Fatalf("a file of a removed key must be unreadable, got %v", err)
	}
	if err := only.Reseal(ctx, "attachments/a"); !errors.Is(err, storage.ErrUnreadable) {
		t.Fatalf("resealing a file that does not open must fail, got %v", err)
	}
	if now, _ := os.ReadFile(filepath.Join(root, "attachments", "a")); !bytes.Equal(now, stored) {
		t.Fatal("a file that does not open must be left as it was")
	}
}

func TestTheHeaderOfAPlainOrDamagedFileIsReportedAsSuch(t *testing.T) {
	ctx := context.Background()
	disk, root, enc := newStores(t)
	if _, err := disk.Put(ctx, "attachments/plain", bytes.NewReader([]byte("plain text")), 100); err != nil {
		t.Fatal(err)
	}
	if _, err := enc.WrappedKey(ctx, "attachments/plain"); !errors.Is(err, storage.ErrNotEncrypted) {
		t.Fatalf("a plain file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "attachments", "cut"), []byte("DFE1\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := enc.WrappedKey(ctx, "attachments/cut"); !errors.Is(err, storage.ErrUnreadable) {
		t.Fatalf("a damaged header: %v", err)
	}
}
