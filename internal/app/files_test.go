// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

func (u *user) file(page, name, content string) string {
	u.e.t.Helper()
	status, body := u.upload(page, name, content)
	if status != 201 {
		u.e.t.Fatalf("upload: %d %s", status, body)
	}
	var a map[string]any
	_ = json.Unmarshal([]byte(body), &a)
	return a["id"].(string)
}

func TestFileVersionsCanBeRestoredAndRemovedByThoseWhoMay(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	editor := e.user("editor@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", editor.id, permissions.TemplateEditor)
	page := owner.page(space, nil, "P", "x")
	id := owner.file(page, "a.txt", "first")
	owner.file(page, "a.txt", "second")

	list := owner.ok("GET", "/api/v1/attachments/"+id+"/versions", nil)["versions"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["current"] != true {
		t.Fatalf("versions: %v", list)
	}
	owner.want(204, "POST", "/api/v1/attachments/"+id+"/versions/1/current", map[string]any{})
	if got := owner.ok("GET", "/api/v1/attachments/"+id+"/text", nil); got["text"] != "first" {
		t.Fatalf("make current: %v", got)
	}
	owner.want(409, "DELETE", "/api/v1/attachments/"+id+"/versions/3", nil)
	editor.want(403, "DELETE", "/api/v1/attachments/"+id+"/versions/2", nil)
	owner.want(204, "DELETE", "/api/v1/attachments/"+id+"/versions/2", nil)
	// Version 3 shares the stored file of version 1, so removing 1 must leave it.
	owner.want(204, "DELETE", "/api/v1/attachments/"+id+"/versions/1", nil)
	if got := owner.ok("GET", "/api/v1/attachments/"+id+"/text", nil); got["text"] != "first" {
		t.Fatalf("a shared file went: %v", got)
	}
	if e.auditActions("attachment.version_removed") != 2 {
		t.Fatal("a removed version must be audited")
	}
}

func TestRenameKeepsTheFileAndRefusesATakenName(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "P", "x")
	id := owner.file(page, "a.txt", "x")
	owner.file(page, "b.txt", "x")
	owner.want(409, "PATCH", "/api/v1/attachments/"+id, map[string]any{"filename": "B.txt"})
	got := owner.ok("PATCH", "/api/v1/attachments/"+id, map[string]any{"filename": "c.txt", "comment": "note"})
	if got["filename"] != "c.txt" {
		t.Fatalf("rename: %v", got)
	}
}

func TestFilesNeverLeakAcrossARestriction(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	open := owner.page(space, nil, "Open", "x")
	hidden := owner.page(space, nil, "Hidden", "x")
	visibleFile := owner.file(open, "open.txt", "x")
	hiddenFile := owner.file(hidden, "secret.txt", "x")
	owner.restrict(hidden, "view", owner.id)

	got := reader.want(200, "GET", "/api/v1/spaces/"+space+"/files", nil)
	if strings.Contains(got, "secret.txt") || !strings.Contains(got, "open.txt") {
		t.Fatalf("files screen: %s", got)
	}
	reader.want(403, "GET", "/api/v1/spaces/"+space+"/files/usage", nil)
	if usage := owner.ok("GET", "/api/v1/spaces/"+space+"/files/usage", nil); usage["bytes"].(float64) != 2 {
		t.Fatalf("usage: %v", usage)
	}
	found := reader.ok("POST", "/api/v1/attachments/lookup", map[string]any{"ids": []string{visibleFile, hiddenFile}})["files"].([]any)
	if len(found) != 1 {
		t.Fatalf("lookup showed a hidden file: %v", found)
	}
	reader.want(404, "GET", "/api/v1/attachments/"+hiddenFile+"/text", nil)
	reader.want(404, "GET", "/api/v1/attachments/"+hiddenFile+"/versions", nil)
}

func TestAnHTMLFileNamedMp4StaysADownload(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "P", "x")
	id := owner.file(page, "film.mp4", "<html><script>alert(1)</script></html>")
	resp, _ := e.do(owner.c, http.MethodGet, "/api/v1/attachments/"+id+"/content", "")
	if got := resp.Header.Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("content type: %s", got)
	}
	mp4 := "\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"
	real := owner.file(page, "real.mp4", mp4)
	resp, _ = e.do(owner.c, http.MethodGet, "/api/v1/attachments/"+real+"/content", "")
	if got := resp.Header.Get("Content-Type"); got != "video/mp4" {
		t.Fatalf("a real video: %s", got)
	}
	if got := resp.Header.Get("Content-Security-Policy"); !strings.Contains(got, "sandbox") {
		t.Fatalf("policy: %s", got)
	}
}

func TestATextPreviewIsOnlyForText(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "P", "x")
	image := owner.file(page, "i.png", pngHeader)
	owner.want(409, "GET", "/api/v1/attachments/"+image+"/text", nil)
	resp, body := e.do(owner.c, http.MethodGet, "/api/v1/pages/"+page+"/attachments.zip", "")
	if resp.StatusCode != 200 || !strings.HasPrefix(body, "PK") {
		t.Fatalf("zip: %d", resp.StatusCode)
	}
	if status, _ := owner.do("GET", "/api/v1/pages/"+owner.page(space, nil, "Empty", "x")+"/attachments.zip", nil); status != 404 {
		t.Fatalf("an empty zip: %d", status)
	}
}
