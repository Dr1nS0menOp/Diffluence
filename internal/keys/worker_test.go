// SPDX-License-Identifier: Apache-2.0

package keys

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeBackupKey(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backup.key")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A missing, empty or short backup key is refused with a reason, never used (spec 070).
func TestAWorkerKeyThatIsMissingOrShortIsRefused(t *testing.T) {
	if _, err := LoadWorker(filepath.Join(t.TempDir(), "none")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a missing key: %v", err)
	}
	for _, content := range []string{"", "   \n", "short"} {
		if _, err := LoadWorker(writeBackupKey(t, content)); err == nil {
			t.Fatalf("a key of %q was taken", content)
		}
	}
}

// The checkpoint key and the restore MACs come from the backup key, each with its own
// purpose, the same on every load, and different for another backup key.
func TestTheWorkerKeysAreStableAndApartPerPurpose(t *testing.T) {
	path := writeBackupKey(t, "c2VjcmV0IHRoYXQgb25seSB0aGUgYmFja3VwIHNlcnZpY2UgaG9sZHM=\n")
	a, err := LoadWorker(path)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := LoadWorker(path)
	ka, err := a.CheckpointKey()
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := b.CheckpointKey()
	if !ka.Equal(kb) {
		t.Fatal("the checkpoint key changes between loads")
	}
	msg := []byte("restore 1")
	if !bytes.Equal(a.MAC("restore code", msg), b.MAC("restore code", msg)) {
		t.Fatal("a MAC changes between loads")
	}
	if bytes.Equal(a.MAC("restore code", msg), a.MAC("restore seal", msg)) {
		t.Fatal("two purposes give the same MAC")
	}
	if bytes.Equal(a.MAC("restore code", msg), a.MAC("restore code", []byte("restore 2"))) {
		t.Fatal("two messages give the same MAC")
	}
	other, _ := LoadWorker(writeBackupKey(t, "another backup key, long enough to be one\n"))
	ko, _ := other.CheckpointKey()
	if ko.Equal(ka) || bytes.Equal(other.MAC("restore code", msg), a.MAC("restore code", msg)) {
		t.Fatal("another backup key gives the same keys")
	}
	root, _ := New(bytes.Repeat([]byte{7}, kekSize))
	installation, _ := root.SigningKey()
	if installation.Equal(ka) {
		t.Fatal("the worker's checkpoint key is the installation's")
	}
}
