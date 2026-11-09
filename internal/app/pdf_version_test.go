// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func (u *user) uploadVersion(file, name, content string) (int, string) {
	u.e.t.Helper()
	req, _ := http.NewRequest("POST", u.e.srv.URL+"/api/v1/attachments/"+file+"/versions?filename="+url.QueryEscape(name), strings.NewReader(content))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := u.c.Do(req)
	if err != nil {
		u.e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func mayAddVersion(t *testing.T, u *user, file string) bool {
	t.Helper()
	found := u.ok("POST", "/api/v1/attachments/lookup", map[string]any{"ids": []string{file}})["files"].([]any)
	if len(found) != 1 {
		t.Fatalf("lookup: %v", found)
	}
	return found[0].(map[string]any)["may_add_version"] == true
}

// A viewer may open a PDF but never store a changed one; an editor may (ARC-099). The
// viewer only offers saving to people the upload would accept.
func TestOnlySomeoneWhoMayAddFilesIsOfferedToSaveAChangedPDF(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	file := owner.file(page, "plan.pdf", "%PDF-1.4 x")

	if mayAddVersion(t, viewer, file) {
		t.Fatal("a viewer must not be offered to save a new version")
	}
	if code, _ := viewer.uploadVersion(file, "plan.pdf", "%PDF-1.4 changed"); code == 201 || code == 200 {
		t.Fatalf("a viewer stored a version: %d", code)
	}
	if !mayAddVersion(t, editor, file) {
		t.Fatal("an editor may save a new version")
	}
	if code, body := editor.uploadVersion(file, "plan.pdf", "%PDF-1.4 changed"); code != 201 && code != 200 {
		t.Fatalf("an editor could not store a version: %d %s", code, body)
	}
}
