// SPDX-License-Identifier: Apache-2.0

package storage_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/keys"
	"github.com/diffluence/diffluence/internal/storage"
)

const chunk = 64 << 10

func keyring(t *testing.T) *keys.Keyring {
	t.Helper()
	kek := make([]byte, 32)
	if _, err := rand.Read(kek); err != nil {
		t.Fatal(err)
	}
	k, err := keys.New(kek)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func notDone(context.Context) (bool, error) { return false, nil }

func newStores(t *testing.T) (*storage.Disk, string, *storage.Encrypted) {
	t.Helper()
	root := t.TempDir()
	disk, err := storage.NewDisk(root)
	if err != nil {
		t.Fatal(err)
	}
	return disk, root, storage.NewEncrypted(disk, keyring(t), notDone)
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func readAll(t *testing.T, s storage.Store, key string) ([]byte, error) {
	t.Helper()
	f, err := s.Open(context.Background(), key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

func TestAWrongKeyFailsBeforeAnyByteIsRead(t *testing.T) {
	disk, _, enc := newStores(t)
	ctx := context.Background()
	if _, err := enc.Put(ctx, "attachments/a", bytes.NewReader(randomBytes(t, 3*chunk)), 1<<20); err != nil {
		t.Fatal(err)
	}
	other := storage.NewEncrypted(disk, keyring(t), notDone)
	if _, err := other.Open(ctx, "attachments/a"); !errors.Is(err, storage.ErrUnreadable) {
		t.Fatalf("a wrong key must fail at open, got %v", err)
	}
}

func TestAFileCutShortOrChangedNeverOpensOrReads(t *testing.T) {
	_, root, enc := newStores(t)
	ctx := context.Background()
	if _, err := enc.Put(ctx, "attachments/a", bytes.NewReader(randomBytes(t, 3*chunk+10)), 1<<20); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "attachments", "a")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Cut at a chunk border: the chunk left last was not sealed as the last.
	if err := os.WriteFile(path, original[:len(original)-(10+16)], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := enc.Open(ctx, "attachments/a"); !errors.Is(err, storage.ErrUnreadable) {
		t.Fatalf("a file cut at a chunk border must not open, got %v", err)
	}

	// One changed byte in the first chunk: open works, reading that chunk does not.
	changed := append([]byte(nil), original...)
	changed[len(changed)-chunk*2] ^= 1
	if err := os.WriteFile(path, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readAll(t, enc, "attachments/a"); !errors.Is(err, storage.ErrUnreadable) {
		t.Fatalf("a changed byte must fail, read %d bytes and %v", len(got), err)
	}
}

func TestAFileMovedUnderAnotherNameDoesNotOpen(t *testing.T) {
	_, root, enc := newStores(t)
	ctx := context.Background()
	if _, err := enc.Put(ctx, "attachments/a", strings.NewReader("secret"), 100); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "attachments", "a"), filepath.Join(root, "attachments", "b")); err != nil {
		t.Fatal(err)
	}
	if _, err := enc.Open(ctx, "attachments/b"); !errors.Is(err, storage.ErrUnreadable) {
		t.Fatalf("a file under another name must not open, got %v", err)
	}
}

func TestATooLargeFileLeavesNothing(t *testing.T) {
	disk, _, enc := newStores(t)
	ctx := context.Background()
	if _, err := enc.Put(ctx, "attachments/a", bytes.NewReader(randomBytes(t, chunk+1)), chunk); !errors.Is(err, storage.ErrTooLarge) {
		t.Fatalf("want too large, got %v", err)
	}
	if _, err := disk.Open(ctx, "attachments/a"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("nothing may remain, got %v", err)
	}
}

func TestPlainFilesAreReadUntilEveryFileIsEncrypted(t *testing.T) {
	disk, _, _ := newStores(t)
	ctx := context.Background()
	if _, err := disk.Put(ctx, "attachments/old", strings.NewReader("written before"), 100); err != nil {
		t.Fatal(err)
	}
	done := false
	enc := storage.NewEncrypted(disk, keyring(t), func(context.Context) (bool, error) { return done, nil })
	if got, err := readAll(t, enc, "attachments/old"); err != nil || string(got) != "written before" {
		t.Fatalf("a plain file must still be read: %q %v", got, err)
	}
	enc.MarkEveryFileEncrypted()
	if _, err := enc.Open(ctx, "attachments/old"); !errors.Is(err, storage.ErrNotEncrypted) {
		t.Fatalf("after the job, a plain file must be refused, got %v", err)
	}
}

func TestEncryptingInPlaceKeepsTheBytes(t *testing.T) {
	disk, root, enc := newStores(t)
	ctx := context.Background()
	plain := randomBytes(t, 2*chunk+5)
	if _, err := disk.Put(ctx, "attachments/old", bytes.NewReader(plain), 1<<20); err != nil {
		t.Fatal(err)
	}
	if encrypted, err := enc.IsEncrypted(ctx, "attachments/old"); err != nil || encrypted {
		t.Fatalf("starts plain: %v %v", encrypted, err)
	}
	if err := enc.EncryptInPlace(ctx, "attachments/old"); err != nil {
		t.Fatal(err)
	}
	if encrypted, err := enc.IsEncrypted(ctx, "attachments/old"); err != nil || !encrypted {
		t.Fatalf("now encrypted: %v %v", encrypted, err)
	}
	stored, err := os.ReadFile(filepath.Join(root, "attachments", "old"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, plain[:64]) {
		t.Fatal("the stored file still holds plain bytes")
	}
	if got, err := readAll(t, enc, "attachments/old"); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("bytes changed: %v", err)
	}
}

func TestSizesAtTheChunkBordersRoundTrip(t *testing.T) {
	_, _, enc := newStores(t)
	ctx := context.Background()
	for _, n := range []int{0, 1, chunk - 1, chunk, chunk + 1, 3 * chunk} {
		plain := randomBytes(t, n)
		obj, err := enc.Put(ctx, "attachments/x", bytes.NewReader(plain), int64(n))
		sum := sha256.Sum256(plain)
		if err != nil || obj.Size != int64(n) || !bytes.Equal(obj.SHA256, sum[:]) {
			t.Fatalf("size %d: %+v %v", n, obj, err)
		}
		got, err := readAll(t, enc, "attachments/x")
		if err != nil || !bytes.Equal(got, plain) {
			t.Fatalf("size %d did not come back: %d bytes, %v", n, len(got), err)
		}
	}
}

func TestRangeReadsSeekInsideTheFile(t *testing.T) {
	_, _, enc := newStores(t)
	ctx := context.Background()
	plain := randomBytes(t, 3*chunk+100)
	if _, err := enc.Put(ctx, "attachments/r", bytes.NewReader(plain), 1<<20); err != nil {
		t.Fatal(err)
	}
	f, err := enc.Open(ctx, "attachments/r")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil || size != int64(len(plain)) {
		t.Fatalf("size %d %v", size, err)
	}
	for _, start := range []int64{0, chunk - 3, 2*chunk + 7, size - 5} {
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, 10)
		n, err := io.ReadFull(f, got)
		want := plain[start:min(start+10, size)]
		if !bytes.Equal(got[:n], want) || (err != nil && !errors.Is(err, io.ErrUnexpectedEOF)) {
			t.Fatalf("range at %d: %v", start, err)
		}
	}
}

// BenchmarkEncryptedRoundTrip measures storing and reading 8 MiB through the encrypted store.
func BenchmarkEncryptedRoundTrip(b *testing.B) {
	root := b.TempDir()
	disk, err := storage.NewDisk(root)
	if err != nil {
		b.Fatal(err)
	}
	kek := make([]byte, 32)
	k, _ := keys.New(kek)
	for _, name := range []string{"plain", "encrypted"} {
		var s storage.Store = disk
		if name == "encrypted" {
			s = storage.NewEncrypted(disk, k, notDone)
		}
		plain := make([]byte, 8<<20)
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(plain)))
			for b.Loop() {
				if _, err := s.Put(context.Background(), "attachments/bench", bytes.NewReader(plain), int64(len(plain))); err != nil {
					b.Fatal(err)
				}
				f, err := s.Open(context.Background(), "attachments/bench")
				if err != nil {
					b.Fatal(err)
				}
				if _, err := io.Copy(io.Discard, f); err != nil {
					b.Fatal(err)
				}
				_ = f.Close()
			}
		})
	}
}
