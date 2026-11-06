// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
)

func TestAFileMovesToAnotherPageAndKeepsItsID(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	from := owner.page(space, nil, "From", "x")
	to := owner.page(space, nil, "To", "x")
	file := owner.file(from, "plan.txt", "hello")

	got := owner.ok("POST", "/api/v1/attachments/"+file+"/move", map[string]any{"page_id": to})
	if got["id"] != file || got["page_id"] != to {
		t.Fatalf("moved: %v", got)
	}
	if list := owner.ok("GET", "/api/v1/pages/"+from+"/attachments", nil)["attachments"].([]any); len(list) != 0 {
		t.Fatalf("the file stayed behind: %v", list)
	}
	if list := owner.ok("GET", "/api/v1/pages/"+to+"/attachments", nil)["attachments"].([]any); len(list) != 1 {
		t.Fatalf("the file did not arrive: %v", list)
	}
	if e.count("SELECT count(*) FROM audit_events WHERE action = 'attachment.moved'") != 1 {
		t.Fatal("the move left no trace in the audit log")
	}
}

func TestAFileMovesAcrossSpacesAndFollowsTheRulesOfItsNewPage(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	other, _ := owner.space("OPS")
	from := owner.page(space, nil, "From", "x")
	to := owner.page(other, nil, "To", "x")
	file := owner.file(from, "plan.txt", "hello")

	if resp, _ := e.do(viewer.c, http.MethodGet, "/api/v1/attachments/"+file+"/content", ""); resp.StatusCode != 200 {
		t.Fatalf("before: %d", resp.StatusCode)
	}
	got := owner.ok("POST", "/api/v1/attachments/"+file+"/move", map[string]any{"page_id": to})
	if got["space_id"] != other {
		t.Fatalf("space: %v", got)
	}
	if resp, _ := e.do(viewer.c, http.MethodGet, "/api/v1/attachments/"+file+"/content", ""); resp.StatusCode != 404 {
		t.Fatalf("a reader outside the new space still gets the file: %d", resp.StatusCode)
	}
}

func TestMovingAFileNeedsDeleteOnTheSourceAndAddOnTheTarget(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	from := owner.page(space, nil, "From", "x")
	to := owner.page(space, nil, "To", "x")
	locked := owner.page(space, nil, "Locked", "x")
	hidden := owner.page(space, nil, "Hidden", "x")
	owner.restrict(locked, "edit", owner.id)
	owner.restrict(hidden, "view", owner.id)
	theirs := owner.file(from, "theirs.txt", "x")
	mine := editor.file(from, "mine.txt", "x")

	viewer.want(403, "POST", "/api/v1/attachments/"+theirs+"/move", map[string]any{"page_id": to})
	editor.want(403, "POST", "/api/v1/attachments/"+theirs+"/move", map[string]any{"page_id": to})
	editor.want(403, "POST", "/api/v1/attachments/"+mine+"/move", map[string]any{"page_id": locked})
	editor.want(404, "POST", "/api/v1/attachments/"+mine+"/move", map[string]any{"page_id": hidden})
	editor.ok("POST", "/api/v1/attachments/"+mine+"/move", map[string]any{"page_id": to})
}

func TestAMoveDoesNotOverwriteAFileWithTheSameName(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	from := owner.page(space, nil, "From", "x")
	to := owner.page(space, nil, "To", "x")
	file := owner.file(from, "plan.txt", "one")
	owner.file(to, "PLAN.txt", "two")
	out := owner.want(409, "POST", "/api/v1/attachments/"+file+"/move", map[string]any{"page_id": to})
	if !strings.Contains(out, "name_taken") {
		t.Fatal(out)
	}
	owner.want(400, "POST", "/api/v1/attachments/"+file+"/move", map[string]any{"page_id": from})
}

func TestAFileCopiesToAnotherPageAndBothLiveOn(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	from := owner.page(space, nil, "From", "x")
	to := owner.page(space, nil, "To", "x")
	hidden := owner.page(space, nil, "Hidden", "x")
	locked := owner.page(space, nil, "Locked", "x")
	secret := owner.page(space, nil, "Secret", "x")
	owner.restrict(hidden, "view", owner.id)
	owner.restrict(locked, "edit", owner.id)
	file := owner.file(from, "plan.txt", "hello")
	secretFile := owner.file(secret, "secret.txt", "hush")
	owner.restrict(secret, "view", owner.id)

	copied := editor.ok("POST", "/api/v1/attachments/"+file+"/copy", map[string]any{"page_id": to})
	if copied["id"] == file || copied["page_id"] != to || copied["filename"] != "plan.txt" {
		t.Fatalf("copy: %v", copied)
	}
	owner.ok("DELETE", "/api/v1/attachments/"+file, nil)
	resp, body := e.do(owner.c, http.MethodGet, "/api/v1/attachments/"+copied["id"].(string)+"/content?download=1", "")
	if resp.StatusCode != 200 || body != "hello" {
		t.Fatalf("the copy lost its content: %d %q", resp.StatusCode, body)
	}

	other := owner.file(from, "other.txt", "x")
	viewer.want(403, "POST", "/api/v1/attachments/"+other+"/copy", map[string]any{"page_id": to})
	editor.want(403, "POST", "/api/v1/attachments/"+other+"/copy", map[string]any{"page_id": locked})
	editor.want(404, "POST", "/api/v1/attachments/"+other+"/copy", map[string]any{"page_id": hidden})
	editor.want(404, "POST", "/api/v1/attachments/"+secretFile+"/copy", map[string]any{"page_id": to})
	owner.ok("POST", "/api/v1/attachments/"+other+"/copy", map[string]any{"page_id": to})
	owner.want(409, "POST", "/api/v1/attachments/"+other+"/copy", map[string]any{"page_id": to})
}

func TestACopyDoesNotTellWhatAPageItMayNotChangeHolds(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	from := owner.page(space, nil, "From", "x")
	locked := owner.page(space, nil, "Locked", "x")
	file := owner.file(from, "plan.txt", "hello")
	owner.file(locked, "plan.txt", "other")
	owner.restrict(locked, "edit", owner.id)
	editor.want(403, "POST", "/api/v1/attachments/"+file+"/copy", map[string]any{"page_id": locked})
}

func blockFileTypes(t *testing.T, e *env, types ...string) {
	t.Helper()
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	current := admin.ok("GET", "/api/v1/admin/settings", nil)
	current["blocked_file_types"] = types
	admin.ok("PUT", "/api/v1/admin/settings", current)
}

func TestABlockedFileTypeIsRefusedByNameAndByContent(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	blockFileTypes(t, e, ".EXE", "html ")

	for name, content := range map[string]string{
		"setup.exe":       "plain",
		"SETUP.EXE":       "plain",
		"holiday.jpg":     "MZ\x90\x00 program",
		"notes.txt":       "<html><script>alert(1)</script></html>",
		"folder/page.htm": "<!DOCTYPE html><html></html>",
	} {
		if status, body := owner.upload(page, name, content); status != 400 {
			t.Fatalf("%s: %d %s", name, status, body)
		}
	}
	if status, body := owner.upload(page, "fine.txt", "words"); status != 201 {
		t.Fatalf("an allowed file: %d %s", status, body)
	}
}

func TestRenamingCannotGetAroundABlockedType(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	file := owner.file(page, "fine.txt", "words")
	blockFileTypes(t, e, "exe")
	owner.want(400, "PATCH", "/api/v1/attachments/"+file, map[string]any{"filename": "fine.exe"})
}

func TestBlockedTypesAreCheckedWhenSaved(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	current := admin.ok("GET", "/api/v1/admin/settings", nil)
	if list, ok := current["blocked_file_types"].([]any); !ok || len(list) != 0 {
		t.Fatalf("defaults: %v", current["blocked_file_types"])
	}
	current["blocked_file_types"] = []string{"not a type!"}
	admin.want(400, "PUT", "/api/v1/admin/settings", current)
}

func TestAnOfficeFileOpensAsPDFThroughTheHelper(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	hidden := owner.page(space, nil, "Hidden", "x")
	report := owner.file(page, "report.docx", "word content")
	secret := owner.file(hidden, "secret.docx", "word content")
	notes := owner.file(page, "notes.txt", "words")
	owner.restrict(hidden, "view", owner.id)
	address := func(id string) string { return "/api/v1/attachments/" + id + "/preview.pdf" }

	if resp, _ := e.do(owner.c, http.MethodGet, address(report), ""); resp.StatusCode != 409 {
		t.Fatalf("without a helper: %d", resp.StatusCode)
	}
	var path, form string
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		form = string(b)
		_, _ = w.Write([]byte("%PDF-fake"))
	}))
	defer helper.Close()
	e.app.Exchange().SetPDFHelper(helper.URL)
	defer e.app.Exchange().SetPDFHelper("")

	resp, body := e.do(viewer.c, http.MethodGet, address(report), "")
	if resp.StatusCode != 200 || body != "%PDF-fake" {
		t.Fatalf("%d %q", resp.StatusCode, body)
	}
	if path != "/forms/libreoffice/convert" || !strings.Contains(form, "word content") || !strings.Contains(form, ".docx") {
		t.Fatalf("what the helper got: %s %q", path, form)
	}
	if strings.Contains(form, "report") {
		t.Fatalf("the helper needs the type, not the name: %q", form)
	}
	if got := resp.Header.Get("Content-Security-Policy"); got != "default-src 'none'; sandbox" {
		t.Fatalf("csp: %q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") || !strings.Contains(got, "report.docx.pdf") {
		t.Fatalf("disposition: %q", got)
	}
	if resp, _ := e.do(viewer.c, http.MethodGet, address(secret), ""); resp.StatusCode != 404 {
		t.Fatalf("a hidden file: %d", resp.StatusCode)
	}
	if resp, _ := e.do(owner.c, http.MethodGet, address(notes), ""); resp.StatusCode != 409 {
		t.Fatalf("a text file: %d", resp.StatusCode)
	}
}

func TestAHelperThatFailsGivesAClearAnswer(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	report := owner.file(page, "report.docx", "word content")
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer helper.Close()
	e.app.Exchange().SetPDFHelper(helper.URL)
	defer e.app.Exchange().SetPDFHelper("")
	if resp, _ := e.do(owner.c, http.MethodGet, "/api/v1/attachments/"+report+"/preview.pdf", ""); resp.StatusCode != 400 {
		t.Fatalf("%d", resp.StatusCode)
	}
}
