// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiskRoundTripAndLimits(t *testing.T) {
	root := t.TempDir()
	d, err := NewDisk(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	obj, err := d.Put(ctx, "attachments/abc", strings.NewReader("hello"), 5)
	if err != nil || obj.Size != 5 || len(obj.SHA256) != 32 {
		t.Fatalf("put: %+v %v", obj, err)
	}
	f, err := d.Open(ctx, "attachments/abc")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(f)
	_ = f.Close()
	if string(b) != "hello" {
		t.Fatalf("read %q", b)
	}

	if _, err := d.Put(ctx, "attachments/big", strings.NewReader("too long"), 5); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("over limit: %v", err)
	}
	if _, err := d.Open(ctx, "attachments/big"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a refused upload left a file: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "attachments"))
	if len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v", entries)
	}

	for _, key := range []string{"../escape", "a/../../b", "/abs", "A", "a b", ""} {
		if _, err := d.Put(ctx, key, strings.NewReader("x"), 5); err == nil {
			t.Errorf("key %q accepted", key)
		}
	}
	if err := d.Delete(ctx, "attachments/abc"); err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, "attachments/abc"); err != nil {
		t.Fatalf("deleting twice must not fail: %v", err)
	}
	if err := d.Healthy(ctx); err != nil {
		t.Fatal(err)
	}
}
