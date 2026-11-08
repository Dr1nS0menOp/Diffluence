// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/identity"
)

// storedFiles returns every file below the object store.
func storedFiles(t *testing.T, dataDir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(filepath.Join(dataDir, "objects"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && !strings.HasPrefix(d.Name(), ".") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAFileIsStoredEncryptedAndComesBackWhole(t *testing.T) {
	var dataDir string
	e := newEnvWith(t, func(c *config.Config) { dataDir = c.DataDir })
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Page", "x")
	secret := "a line nobody may read on the disk " + strings.Repeat("x", 200)
	status, body := owner.upload(page, "notes.txt", secret)
	if status != 201 {
		t.Fatalf("upload: %d %s", status, body)
	}
	for _, path := range storedFiles(t, dataDir) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte("nobody may read")) {
			t.Fatalf("plain text on the disk in %s", path)
		}
	}
	var att map[string]any
	_ = json.Unmarshal([]byte(body), &att)
	address := "/api/v1/attachments/" + att["id"].(string) + "/content"
	if resp, got := e.do(owner.c, "GET", address, ""); resp.StatusCode != 200 || got != secret {
		t.Fatalf("download: %d, %d bytes", resp.StatusCode, len(got))
	}
	resp, got := e.do(owner.c, "GET", address, "", "Range", "bytes=2-5")
	if resp.StatusCode != 206 || got != secret[2:6] {
		t.Fatalf("range: %d %q", resp.StatusCode, got)
	}
}

func TestOldPlainFilesAreEncryptedThenPlainIsRefused(t *testing.T) {
	var dataDir string
	e := newEnvWith(t, func(c *config.Config) { dataDir = c.DataDir })
	ctx := context.Background()
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Page", "x")
	_, body := owner.upload(page, "old.txt", "written before encryption existed")
	var att map[string]any
	_ = json.Unmarshal([]byte(body), &att)
	address := "/api/v1/attachments/" + att["id"].(string) + "/content"

	// Make the stored file look as it did before this release: plain bytes.
	files := storedFiles(t, dataDir)
	if len(files) != 1 {
		t.Fatalf("want one stored file, got %v", files)
	}
	plain := []byte("written before encryption existed")
	if err := os.WriteFile(files[0], plain, 0o600); err != nil {
		t.Fatal(err)
	}
	if resp, got := e.do(owner.c, "GET", address, ""); resp.StatusCode != 200 || got != string(plain) {
		t.Fatalf("an old plain file must still be served: %d %q", resp.StatusCode, got)
	}

	if err := e.app.EncryptStoredFilesForTest(ctx); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(files[0])
	if bytes.Contains(raw, plain) {
		t.Fatal("the job left the file plain")
	}
	if resp, got := e.do(owner.c, "GET", address, ""); resp.StatusCode != 200 || got != string(plain) {
		t.Fatalf("after the job: %d %q", resp.StatusCode, got)
	}

	// A pass that finds nothing plain records that every file is encrypted.
	if err := e.app.EncryptStoredFilesForTest(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[0], plain, 0o600); err != nil {
		t.Fatal(err)
	}
	if resp, got := e.do(owner.c, "GET", address, ""); resp.StatusCode == 200 || strings.Contains(got, string(plain)) {
		t.Fatalf("a plain file swapped in afterwards must be refused: %d %q", resp.StatusCode, got)
	}
}
