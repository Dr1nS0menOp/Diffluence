// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// newVersion sends a file as the next version of one that exists, as a drop on a tile does.
func (u *user) newVersion(file, droppedName, content string) (int, string) {
	u.e.t.Helper()
	req, _ := http.NewRequest("POST", u.e.srv.URL+"/api/v1/attachments/"+file+"/versions?filename="+url.QueryEscape(droppedName), strings.NewReader(content))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := u.c.Do(req)
	if err != nil {
		u.e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// age makes a file look old enough to be cleared up.
func (e *env) age(file string) {
	e.t.Helper()
	if _, err := e.app.Pool().Exec(context.Background(), "UPDATE attachments SET created_at = now() - interval '2 days' WHERE id = $1", file); err != nil {
		e.t.Fatal(err)
	}
}

func rowsOf(u *user, page string) map[string]map[string]any {
	u.e.t.Helper()
	out := map[string]map[string]any{}
	for _, row := range u.ok("GET", "/api/v1/pages/"+page+"/attachments", nil)["attachments"].([]any) {
		file := row.(map[string]any)
		out[file["filename"].(string)] = file
	}
	return out
}

func fileDoc(file, name string) map[string]any {
	return map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "file", "attrs": map[string]any{"attachment_id": file, "filename": name}}}}
}

func (u *user) republish(page, title string, doc map[string]any) {
	u.e.t.Helper()
	version := u.ok("GET", "/api/v1/pages/"+page, nil)["current_version"]
	u.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": title, "document": doc, "base_version": version})
}

// scalar reads one text value.
func (e *env) scalar(query string, args ...any) string {
	e.t.Helper()
	var out string
	if err := e.app.Pool().QueryRow(context.Background(), query, args...).Scan(&out); err != nil {
		e.t.Fatal(err)
	}
	return out
}

func (u *user) setLimit(space string, mib int) {
	u.e.t.Helper()
	u.ok("PUT", "/api/v1/admin/spaces/"+space+"/storage-limit", map[string]any{"limit_mib": mib})
}

func TestOneVersionOfAFileHasItsOwnAddressThatGivesNoAccess(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	file := owner.file(page, "plan.txt", "first")
	owner.file(page, "plan.txt", "second")
	address := "/api/v1/attachments/" + file + "/versions/1/content"

	resp, body := e.do(viewer.c, http.MethodGet, address, "")
	if resp.StatusCode != 200 || body != "first" {
		t.Fatalf("the old version: %d %q", resp.StatusCode, body)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment") || resp.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("an old version is always a download: %v", resp.Header)
	}
	if e.count("SELECT count(*) FROM audit_events WHERE action = 'attachment.downloaded'") != 1 {
		t.Fatal("the download left no trace")
	}
	if resp, _ := e.do(viewer.c, http.MethodGet, "/api/v1/attachments/"+file+"/versions/9/content", ""); resp.StatusCode != 404 {
		t.Fatalf("a version that does not exist: %d", resp.StatusCode)
	}
	owner.restrict(page, "view", owner.id)
	if resp, _ := e.do(viewer.c, http.MethodGet, address, ""); resp.StatusCode != 404 {
		t.Fatalf("the link gave access to a restricted page: %d", resp.StatusCode)
	}
	if resp, _ := e.do(http.DefaultClient, http.MethodGet, address, ""); resp.StatusCode != 401 {
		t.Fatalf("the link gave access without an account: %d", resp.StatusCode)
	}
	owner.ok("DELETE", "/api/v1/attachments/"+file, nil)
	if resp, _ := e.do(owner.c, http.MethodGet, address, ""); resp.StatusCode != 404 {
		t.Fatalf("a file in the trash still answers: %d", resp.StatusCode)
	}
}

func TestADroppedFileBecomesTheNextVersionUnderTheOldName(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	file := owner.file(page, "plan.txt", "first")

	if status, body := viewer.newVersion(file, "new.txt", "nope"); status != 403 {
		t.Fatalf("view only: %d %s", status, body)
	}
	status, body := owner.newVersion(file, "other-name.txt", "second")
	if status != 201 {
		t.Fatalf("%d %s", status, body)
	}
	var got map[string]any
	_ = json.Unmarshal([]byte(body), &got)
	if got["id"] != file || got["filename"] != "plan.txt" || got["version"] != float64(2) {
		t.Fatalf("new version: %v", got)
	}
	if resp, text := e.do(owner.c, http.MethodGet, "/api/v1/attachments/"+file+"/content?download=1", ""); resp.StatusCode != 200 || text != "second" {
		t.Fatalf("the tile shows: %d %q", resp.StatusCode, text)
	}
}

func TestADroppedFileCannotComeInUnderTheNameOfAnAllowedOne(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	file := owner.file(page, "plan.txt", "first")
	blockFileTypes(t, e, "exe", "html")

	if status, body := owner.newVersion(file, "setup.exe", "plain"); status != 400 {
		t.Fatalf("a blocked name: %d %s", status, body)
	}
	if status, body := owner.newVersion(file, "plan.txt", "<!DOCTYPE html><html></html>"); status != 400 {
		t.Fatalf("blocked content: %d %s", status, body)
	}
	if e.count("SELECT count(*) FROM attachment_versions") != 1 {
		t.Fatal("a refused file left a version")
	}
}

func TestAnArchiveStaysOneFile(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	archive := "PK\x03\x04\x14\x00\x00\x00\x00\x00 not really unpacked"
	file := owner.file(page, "bundle.zip", archive)
	if rows := rowsOf(owner, page); len(rows) != 1 || rows["bundle.zip"]["id"] != file {
		t.Fatalf("files: %v", rows)
	}
	if resp, text := e.do(owner.c, http.MethodGet, "/api/v1/attachments/"+file+"/content?download=1", ""); resp.StatusCode != 200 || text != archive {
		t.Fatalf("the archive changed: %d", resp.StatusCode)
	}
}

func TestARowSaysWhoAddedAFileAndWhetherThePageShowsIt(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	shown := editor.file(page, "shown.txt", "x")
	owner.file(page, "beside.txt", "x")
	owner.republish(page, "P", fileDoc(shown, "shown.txt"))

	rows := rowsOf(viewer, page)
	if rows["shown.txt"]["in_use"] != true || rows["beside.txt"]["in_use"] != false {
		t.Fatalf("in use: %v", rows)
	}
	if rows["shown.txt"]["added_by"] != editor.ok("GET", "/api/v1/me", nil)["user"].(map[string]any)["display_name"] {
		t.Fatalf("added by: %v", rows["shown.txt"])
	}
	if rows["shown.txt"]["created_at"] == "" {
		t.Fatalf("added when: %v", rows["shown.txt"])
	}
	owner.republish(page, "P", para("gone from the page"))
	if rowsOf(viewer, page)["shown.txt"]["in_use"] != false {
		t.Fatal("the current version no longer shows the file")
	}
}

func TestRenamingKeepsTheCommentThatWasSaved(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	file := owner.file(page, "plan.txt", "x")
	owner.ok("PATCH", "/api/v1/attachments/"+file, map[string]any{"filename": "plan.txt", "comment": "for the board"})
	if got := rowsOf(owner, page)["plan.txt"]["comment"]; got != "for the board" {
		t.Fatalf("the form opens with: %v", got)
	}
}

func TestOnlyFilesNothingHoldsAreOfferedForClearingUp(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	neighbour := owner.page(space, nil, "N", "x")
	old := owner.file(page, "old-version.txt", "x")
	draft := owner.file(page, "in-draft.txt", "x")
	comment := owner.file(page, "in-comment.png", pngHeader)
	header := owner.file(page, "header.png", pngHeader)
	moved := owner.file(page, "moved.txt", "x")
	left := owner.file(page, "left-over.txt", "x")
	owner.file(page, "young.txt", "x")

	owner.republish(page, "P", fileDoc(old, "old-version.txt"))
	owner.republish(page, "P", fileDoc(moved, "moved.txt"))
	owner.republish(page, "P", para("nothing here now"))
	owner.ok("POST", "/api/v1/attachments/"+moved+"/move", map[string]any{"page_id": neighbour})
	owner.ok("PUT", "/api/v1/pages/"+page+"/draft", map[string]any{"title": "P", "document": fileDoc(draft, "in-draft.txt"), "base_version": 3})
	owner.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "image", "attrs": map[string]any{"attachment_id": comment, "alt": "x"}}}}})
	owner.ok("PATCH", "/api/v1/pages/"+page+"/look", map[string]any{"header_attachment_id": header, "header_position": 50, "title_align": "start"})
	for _, file := range []string{old, draft, comment, header, moved, left} {
		e.age(file)
	}

	rows := rowsOf(owner, page)
	for name, row := range rows {
		if want := name == "left-over.txt"; row["clearable"] != want {
			t.Fatalf("%s: clearable is %v", name, row["clearable"])
		}
	}
	if got := rowsOf(owner, neighbour)["moved.txt"]["clearable"]; got != false {
		t.Fatalf("a moved file that its old page still shows: %v", got)
	}
	out := owner.ok("POST", "/api/v1/pages/"+page+"/attachments/clear-up", nil)
	if out["deleted"] != float64(1) || out["skipped"] != float64(0) {
		t.Fatalf("clear up: %v", out)
	}
	rows = rowsOf(owner, page)
	if _, there := rows["left-over.txt"]; there {
		t.Fatal("the left-over file stayed")
	}
	if _, there := rows["young.txt"]; !there {
		t.Fatal("a file that may still be on its way was cleared up")
	}
	if e.count("SELECT count(*) FROM trash_items WHERE object_id = $1", left) != 1 {
		t.Fatal("clearing up must be undoable")
	}
}

func TestClearingUpOnlyTakesWhatThePersonMayDelete(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	theirs := owner.file(page, "theirs.txt", "x")
	mine := editor.file(page, "mine.txt", "x")
	e.age(theirs)
	e.age(mine)

	out := viewer.ok("POST", "/api/v1/pages/"+page+"/attachments/clear-up", nil)
	if out["deleted"] != float64(0) || out["skipped"] != float64(2) {
		t.Fatalf("view only: %v", out)
	}
	out = editor.ok("POST", "/api/v1/pages/"+page+"/attachments/clear-up", nil)
	if out["deleted"] != float64(1) || out["skipped"] != float64(1) {
		t.Fatalf("an editor deletes their own: %v", out)
	}
	owner.restrict(page, "view", owner.id)
	viewer.want(404, "POST", "/api/v1/pages/"+page+"/attachments/clear-up", nil)
}

func TestEarlierVersionsAreKnownAfterTheMigrationFilledThem(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	file := owner.file(page, "plan.txt", "x")
	owner.republish(page, "P", fileDoc(file, "plan.txt"))
	if e.count("SELECT count(*) FROM attachment_refs WHERE attachment_id = $1 AND page_id = $2 AND version = 2", file, page) != 1 {
		t.Fatal("publishing wrote no reference")
	}
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	owner.republish(page, "P", para("later"))
	admin.ok("DELETE", "/api/v1/admin/pages/"+page+"/versions/2", map[string]any{"reason": "a leaked secret"})
	if e.count("SELECT count(*) FROM attachment_refs WHERE attachment_id = $1", file) != 0 {
		t.Fatal("a removed version left its reference")
	}
}

func TestFileLabelsFollowTheRulesOfLabels(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	file := owner.file(page, "plan.txt", "x")
	address := "/api/v1/attachments/" + file + "/labels"

	viewer.want(403, "POST", address, map[string]any{"label": "contract"})
	editor.want(400, "POST", address, map[string]any{"label": "not/valid"})
	got := editor.ok("POST", address, map[string]any{"label": "Signed Contract"})
	if labels := strs(got["labels"]); len(labels) != 1 || labels[0] != "signed-contract" {
		t.Fatalf("labels: %v", got)
	}
	if labels := strs(rowsOf(viewer, page)["plan.txt"]["labels"]); len(labels) != 1 {
		t.Fatalf("a reader sees: %v", labels)
	}
	viewer.want(403, "DELETE", address+"/signed-contract", nil)
	owner.ok("PUT", "/api/v1/spaces/"+space+"/labels", map[string]any{"fixed": true, "labels": []string{"allowed"}})
	editor.want(400, "POST", address, map[string]any{"label": "other"})
	editor.ok("POST", address, map[string]any{"label": "allowed"})
	got = editor.ok("DELETE", address+"/signed-contract", nil)
	if labels := strs(got["labels"]); len(labels) != 1 || labels[0] != "allowed" {
		t.Fatalf("after removing: %v", got)
	}
	owner.restrict(page, "view", owner.id)
	viewer.want(404, "POST", address, map[string]any{"label": "allowed"})
}

func TestAFileCarriesAtMostFiftyLabels(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	file := owner.file(owner.page(space, nil, "P", "x"), "plan.txt", "x")
	address := "/api/v1/attachments/" + file + "/labels"
	for i := range 50 {
		owner.ok("POST", address, map[string]any{"label": fmt.Sprintf("label-%d", i)})
	}
	owner.want(400, "POST", address, map[string]any{"label": "one-too-many"})
}

func TestSearchFindsAPageByTheLabelOfItsFileAndOnlyWhatMayBeSeen(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	open := owner.page(space, nil, "Open", "x")
	hidden := owner.page(space, nil, "Hidden", "x")
	binned := owner.page(space, nil, "Binned", "x")
	shown := owner.file(open, "a.txt", "x")
	secret := owner.file(hidden, "b.txt", "x")
	gone := owner.file(binned, "c.txt", "x")
	for _, file := range []string{shown, secret, gone} {
		owner.ok("POST", "/api/v1/attachments/"+file+"/labels", map[string]any{"label": "contract"})
	}
	owner.restrict(hidden, "view", owner.id)
	owner.ok("DELETE", "/api/v1/attachments/"+gone, nil)

	if got := titlesIn(viewer.ok("GET", "/api/v1/search?label=contract", nil), "results"); len(got) != 1 || got[0] != "Open" {
		t.Fatalf("found: %v", got)
	}
}

func TestTheFilesScreenFiltersAndOffersOnlyWhatMayBeSeen(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	open := owner.page(space, nil, "Open", "x")
	hidden := owner.page(space, nil, "Hidden", "x")
	text := owner.file(open, "notes.txt", "words")
	image := editor.file(open, "picture.png", pngHeader)
	owner.file(open, "report.docx", "PK\x03\x04 office")
	secret := owner.file(hidden, "secret.txt", "x")
	owner.ok("POST", "/api/v1/attachments/"+text+"/labels", map[string]any{"label": "minutes"})
	owner.ok("POST", "/api/v1/attachments/"+secret+"/labels", map[string]any{"label": "layoffs"})
	owner.restrict(hidden, "view", owner.id)

	names := func(query string) []string {
		var out []string
		for _, row := range viewer.ok("GET", "/api/v1/spaces/"+space+"/files?"+query, nil)["files"].([]any) {
			out = append(out, row.(map[string]any)["filename"].(string))
		}
		slices.Sort(out)
		return out
	}
	for query, want := range map[string][]string{
		"":                      {"notes.txt", "picture.png", "report.docx"},
		"kind=image":            {"picture.png"},
		"kind=text":             {"notes.txt"},
		"kind=office":           {"report.docx"},
		"label=minutes":         {"notes.txt"},
		"label=layoffs":         nil,
		"uploader=" + editor.id: {"picture.png"},
		"q=note&kind=image":     nil,
	} {
		if got := names(query); !slices.Equal(got, want) {
			t.Fatalf("%q: %v", query, got)
		}
	}
	viewer.want(400, "GET", "/api/v1/spaces/"+space+"/files?kind=nonsense", nil)
	viewer.want(400, "GET", "/api/v1/spaces/"+space+"/files?uploader=nonsense", nil)
	_ = image

	filters := viewer.want(200, "GET", "/api/v1/spaces/"+space+"/files/filters", nil)
	if strings.Contains(filters, "layoffs") || !strings.Contains(filters, "minutes") {
		t.Fatalf("filters tell about a hidden page: %s", filters)
	}
}

func TestBulkDeleteTakesOnlyWhatTheAdministratorMayReachInThatSpace(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	other, _ := owner.space("OPS")
	page := owner.page(space, nil, "P", "x")
	hiddenPage := owner.page(space, nil, "Hidden", "x")
	lockedPage := owner.page(space, nil, "Locked", "x")
	foreignPage := owner.page(other, nil, "F", "x")
	plain := owner.file(page, "a.txt", "x")
	hidden := owner.file(hiddenPage, "b.txt", "x")
	locked := owner.file(lockedPage, "c.txt", "x")
	foreign := owner.file(foreignPage, "d.txt", "x")
	owner.restrict(hiddenPage, "view", owner.id)
	owner.restrict(lockedPage, "edit", owner.id)
	owner.grant(space, "user", editor.id, permissions.TemplateSpaceAdmin)
	owner.grant(other, "user", editor.id, permissions.TemplateSpaceAdmin)
	address := "/api/v1/spaces/" + space + "/files/delete"

	viewer.want(403, "POST", address, map[string]any{"ids": []string{plain}})
	out := editor.ok("POST", address, map[string]any{"ids": []string{plain, hidden, locked, foreign, "00000000-0000-0000-0000-000000000001"}})
	if out["deleted"] != float64(1) || out["skipped"] != float64(4) {
		t.Fatalf("answer: %v", out)
	}
	for file, want := range map[string]string{plain: "trashed", hidden: "active", locked: "active", foreign: "active"} {
		if e.count("SELECT count(*) FROM attachments WHERE id = $1 AND status = $2", file, want) != 1 {
			t.Fatalf("%s should be %s", file, want)
		}
	}
	ids := make([]string, 201)
	for i := range ids {
		ids[i] = plain
	}
	owner.want(400, "POST", address, map[string]any{"ids": ids})
}

func TestOnlyAnAdministratorSetsTheLimitOfASpace(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	owner.want(403, "PUT", "/api/v1/admin/spaces/"+space+"/storage-limit", map[string]any{"limit_mib": 5})
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	admin.want(400, "PUT", "/api/v1/admin/spaces/"+space+"/storage-limit", map[string]any{"limit_mib": -1})
	admin.want(404, "PUT", "/api/v1/admin/spaces/00000000-0000-0000-0000-000000000001/storage-limit", map[string]any{"limit_mib": 5})
	admin.setLimit(space, 5)
	if e.count("SELECT count(*) FROM audit_events WHERE action = 'space.storage_limit_set'") != 1 {
		t.Fatal("the limit left no trace")
	}
	if got := owner.ok("GET", "/api/v1/spaces/"+space+"/files/usage", nil)["limit_bytes"]; got != float64(5<<20) {
		t.Fatalf("usage: %v", got)
	}
}

func TestAFullSpaceRefusesEveryWayInAndLeavesNothingBehind(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	other, _ := owner.space("OPS")
	page := owner.page(space, nil, "P", "x")
	elsewhere := owner.page(other, nil, "E", "x")
	megabyte := strings.Repeat("x", 1<<20)
	first := owner.file(page, "first.txt", megabyte)
	big := owner.file(elsewhere, "big.txt", megabyte)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	admin.setLimit(space, 1)
	versions := e.count("SELECT count(*) FROM attachment_versions")

	if status, body := owner.upload(page, "more.txt", "x"); status != 413 {
		t.Fatalf("upload: %d %s", status, body)
	}
	if status, body := owner.newVersion(first, "first.txt", "x"); status != 413 {
		t.Fatalf("new version: %d %s", status, body)
	}
	owner.want(413, "POST", "/api/v1/attachments/"+big+"/copy", map[string]any{"page_id": page})
	owner.want(413, "POST", "/api/v1/attachments/"+big+"/move", map[string]any{"page_id": page})
	if e.count("SELECT count(*) FROM attachment_versions") != versions {
		t.Fatal("a refused file left a version")
	}

	admin.setLimit(space, 3)
	if status, body := owner.upload(page, "fits.txt", megabyte); status != 201 {
		t.Fatalf("within the limit: %d %s", status, body)
	}
	if status, body := owner.upload(page, "too-big.txt", strings.Repeat(megabyte, 2)); status != 413 {
		t.Fatalf("a file larger than the room left: %d %s", status, body)
	}
	owner.ok("POST", "/api/v1/attachments/"+big+"/move", map[string]any{"page_id": page})
	admin.setLimit(space, 0)
	if status, body := owner.upload(page, "free.txt", megabyte); status != 201 {
		t.Fatalf("no limit: %d %s", status, body)
	}
}

func TestThePageHistoryTellsWhatHappenedToItsFiles(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	secret := owner.page(space, nil, "Merger plans", "x")
	owner.restrict(secret, "view", owner.id)
	file := owner.file(page, "plan.txt", "one")
	owner.file(page, "plan.txt", "two")
	moved := owner.file(page, "moved.txt", "x")
	owner.ok("POST", "/api/v1/attachments/"+moved+"/move", map[string]any{"page_id": secret})
	owner.ok("DELETE", "/api/v1/attachments/"+file, nil)
	item := e.scalar("SELECT id::text FROM trash_items WHERE object_id = $1", file)
	owner.ok("POST", "/api/v1/trash/"+item+"/restore", nil)

	raw := viewer.want(200, "GET", "/api/v1/pages/"+page+"/file-events", nil)
	if strings.Contains(raw, "Merger plans") || strings.Contains(raw, secret) {
		t.Fatalf("the history names a hidden page: %s", raw)
	}
	var got struct {
		Events []struct {
			Filename string `json:"filename"`
			Action   string `json:"action"`
			Actor    string `json:"actor"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, event := range got.Events {
		actions = append(actions, event.Filename+":"+event.Action)
		if event.Actor == "" {
			t.Fatalf("who did it: %s", raw)
		}
	}
	want := []string{"plan.txt:restored", "plan.txt:removed", "moved.txt:moved_away", "moved.txt:added", "plan.txt:version", "plan.txt:added"}
	if !slices.Equal(actions, want) {
		t.Fatalf("history: %v", actions)
	}
	if list := owner.ok("GET", "/api/v1/pages/"+secret+"/file-events", nil)["events"].([]any); len(list) != 1 {
		t.Fatalf("the new page: %v", list)
	}
	owner.restrict(page, "view", owner.id)
	viewer.want(404, "GET", "/api/v1/pages/"+page+"/file-events", nil)
}

func TestPurgingAFileTakesItsHistoryAlong(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "P", "x")
	file := owner.file(page, "leaked.txt", "x")
	owner.ok("DELETE", "/api/v1/attachments/"+file, nil)
	item := e.scalar("SELECT id::text FROM trash_items WHERE object_id = $1", file)
	owner.ok("DELETE", "/api/v1/trash/"+item, nil)
	if list := owner.ok("GET", "/api/v1/pages/"+page+"/file-events", nil)["events"].([]any); len(list) != 0 {
		t.Fatalf("the name lives on: %v", list)
	}
}
