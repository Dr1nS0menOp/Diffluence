// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/office"
	"github.com/diffluence/diffluence/internal/permissions"

	"github.com/diffluence/diffluence/internal/document"
)

// post uploads a file and returns the answer.
func (u *user) post(path string, body []byte) (int, map[string]any) {
	u.e.t.Helper()
	req, err := http.NewRequest("POST", u.e.srv.URL+path, bytes.NewReader(body))
	if err != nil {
		u.e.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Origin", u.e.srv.URL)
	for _, c := range u.c.Jar.Cookies(req.URL) {
		if strings.Contains(c.Name, "csrf") {
			req.Header.Set("X-CSRF-Token", c.Value)
		}
	}
	resp, err := u.c.Do(req)
	if err != nil {
		u.e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	got := map[string]any{}
	_ = json.Unmarshal(out, &got)
	return resp.StatusCode, got
}

// report uploads a file for a report, and returns the run once it is read.
func (u *user) report(space, name string, body []byte) map[string]any {
	u.e.t.Helper()
	status, got := u.post("/api/v1/spaces/"+space+"/imports?report=1&filename="+url.QueryEscape(name), body)
	if status != 202 {
		u.e.t.Fatalf("report: %d %v", status, got)
	}
	u.e.runJobs()
	return u.ok("GET", "/api/v1/imports/"+got["import_id"].(string), nil)
}

func (u *user) carryOut(id string) map[string]any {
	u.e.t.Helper()
	u.ok("POST", "/api/v1/imports/"+id+"/run", map[string]any{})
	u.e.runJobs()
	return u.ok("GET", "/api/v1/imports/"+id, nil)
}

func pageCount(t *testing.T, e *env, space string) int {
	t.Helper()
	var n int
	if err := e.app.Pool().QueryRow(context.Background(), `SELECT count(*) FROM pages WHERE space_id = $1 AND status = 'published'`, space).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func importSetup(t *testing.T) (*env, *user, string) {
	t.Helper()
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("IMP")
	return e, owner, space
}

func TestAReportChangesNothingUntilConfirmed(t *testing.T) {
	e, owner, space := importSetup(t)
	before := pageCount(t, e, space)
	run := owner.report(space, "a.zip", zipOf(t, map[string]string{"One.md": "# one", "One/Two.md": "# two"}))
	if run["state"] != "ready" {
		t.Fatalf("%+v", run)
	}
	report := run["report"].(map[string]any)
	if report["pages"].(float64) != 2 || len(report["tree"].([]any)) != 2 {
		t.Fatalf("%+v", report)
	}
	if got := pageCount(t, e, space); got != before {
		t.Fatalf("a report made pages: %d", got-before)
	}
	done := owner.carryOut(run["id"].(string))
	if done["state"] != "done" || pageCount(t, e, space) != before+2 {
		t.Fatalf("%+v", done)
	}
}

func TestOnlyTheOwnerSeesAnImport(t *testing.T) {
	e, owner, space := importSetup(t)
	other := e.user("other@example.com", identity.PermUseProduct)
	run := owner.report(space, "a.md", []byte("# a"))
	id := run["id"].(string)
	other.want(404, "GET", "/api/v1/imports/"+id, nil)
	other.want(404, "POST", "/api/v1/imports/"+id+"/run", map[string]any{})
	other.want(404, "POST", "/api/v1/imports/"+id+"/undo", map[string]any{})
}

func TestUndoMovesAnImportToTheTrash(t *testing.T) {
	e, owner, space := importSetup(t)
	before := pageCount(t, e, space)
	run := owner.report(space, "a.zip", zipOf(t, map[string]string{"One.md": "# one", "One/Two.md": "# two"}))
	id := run["id"].(string)
	owner.carryOut(id)
	out := owner.ok("POST", "/api/v1/imports/"+id+"/undo", map[string]any{})
	if out["undone"].(float64) != 2 || pageCount(t, e, space) != before {
		t.Fatalf("%+v", out)
	}
	if e.auditActions("import.undone") != 1 {
		t.Fatal("an undo must be audited")
	}
}

func TestUndoNeverRemovesSomeonesEdit(t *testing.T) {
	e, owner, space := importSetup(t)
	editor := e.user("editor@example.com", identity.PermUseProduct)
	owner.grant(space, "user", editor.id, permissions.TemplateEditor)
	before := pageCount(t, e, space)
	run := owner.report(space, "a.zip", zipOf(t, map[string]string{"One.md": "# one", "Two.md": "# two"}))
	id := run["id"].(string)
	owner.carryOut(id)
	var page string
	if err := e.app.Pool().QueryRow(context.Background(), `SELECT p.id FROM pages p JOIN import_items i ON i.page_id = p.id WHERE p.title = 'One'`).Scan(&page); err != nil {
		t.Fatal(err)
	}
	editor.publish(page, "One", 1, docWith("edited"))
	owner.want(409, "POST", "/api/v1/imports/"+id+"/undo", map[string]any{})
	if got := owner.ok("GET", "/api/v1/imports/"+id, nil)["touched"].([]any); len(got) != 1 || got[0] != "One" {
		t.Fatalf("touched %v", got)
	}
	out := owner.ok("POST", "/api/v1/imports/"+id+"/undo", map[string]any{"rest": true})
	if out["undone"].(float64) != 1 || pageCount(t, e, space) != before+1 {
		t.Fatalf("%+v", out)
	}
}

func docWith(text string) map[string]any {
	return map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph",
		"content": []any{map[string]any{"type": "text", "text": text}}}}}
}

func documentOf(t *testing.T, e *env, title string) string {
	t.Helper()
	var raw string
	err := e.app.Pool().QueryRow(context.Background(), `SELECT v.document::text FROM pages p
		JOIN page_versions v ON v.page_id = p.id AND v.number = p.current_version WHERE p.title = $1`, title).Scan(&raw)
	if err != nil {
		t.Fatalf("%s: %v", title, err)
	}
	return raw
}

func pageID(t *testing.T, e *env, title string) string {
	t.Helper()
	var id string
	if err := e.app.Pool().QueryRow(context.Background(), `SELECT id FROM pages WHERE title = $1`, title).Scan(&id); err != nil {
		t.Fatalf("%s: %v", title, err)
	}
	return id
}

func TestLinksBetweenMarkdownFilesLeadToTheNewPages(t *testing.T) {
	e, owner, space := importSetup(t)
	job := owner.importFile(space, "a.zip", zipOf(t, map[string]string{
		"Alpha.md": "See [beta](Beta.md#part) and [[Gamma]] and [[Gamma|that one]] and [gone](Nope.md).",
		"Beta.md":  "Back to [alpha](Alpha.md).",
		"Gamma.md": "# gamma",
	}))
	if job["status"] != "done" {
		t.Fatalf("%+v", job)
	}
	alpha := documentOf(t, e, "Alpha")
	for _, want := range []string{"/pages/" + pageID(t, e, "Beta") + "#part", "/pages/" + pageID(t, e, "Gamma"), "that one"} {
		if !strings.Contains(alpha, want) {
			t.Errorf("missing %s in %s", want, alpha)
		}
	}
	if strings.Contains(alpha, "Nope.md") {
		t.Errorf("a link that leads nowhere stayed: %s", alpha)
	}
	if !strings.Contains(documentOf(t, e, "Beta"), "/pages/"+pageID(t, e, "Alpha")) {
		t.Error("a link back is gone")
	}
}

func TestFrontMatterGivesTitleAndLabelsAndWarnsAboutTheRest(t *testing.T) {
	e, owner, space := importSetup(t)
	run := owner.report(space, "a.md", []byte("---\ntitle: Real title\nlabels: [one, two]\nweird: yes\n---\n# body\n"))
	warnings := run["report"].(map[string]any)["warnings"].([]any)
	if len(warnings) != 1 || !strings.Contains(warnings[0].(string), "weird") {
		t.Fatalf("%v", warnings)
	}
	owner.carryOut(run["id"].(string))
	var labels int
	id := pageID(t, e, "Real title")
	if err := e.app.Pool().QueryRow(context.Background(), `SELECT count(*) FROM page_labels WHERE page_id = $1`, id).Scan(&labels); err != nil || labels != 2 {
		t.Fatalf("labels %d %v", labels, err)
	}
}

func wordFile(t *testing.T, text string) []byte {
	t.Helper()
	var b bytes.Buffer
	doc := document.Node{Type: document.TypeDoc, Content: []document.Node{{Type: document.TypeHeading, Attrs: map[string]any{"level": 2},
		Content: []document.Node{{Type: document.TypeText, Text: text}}}}}
	if err := office.WriteDOCX(&b, []office.Section{{Title: "File", Document: doc}}, office.Options{}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestOneWordFileBecomesAPage(t *testing.T) {
	e, owner, space := importSetup(t)
	job := owner.importFile(space, "Plan.docx", wordFile(t, "Inside"))
	if job["status"] != "done" {
		t.Fatalf("%+v", job)
	}
	if got := documentOf(t, e, "Plan"); !strings.Contains(got, "Inside") || !strings.Contains(got, `"heading"`) {
		t.Fatalf("%s", got)
	}
}

func TestSeveralWordFilesWaitUnderARestrictedPage(t *testing.T) {
	e, owner, space := importSetup(t)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, name := range []string{"One.docx", "Two.docx"} {
		f, _ := zw.Create(name)
		_, _ = f.Write(wordFile(t, name))
	}
	_ = zw.Close()
	if job := owner.importFile(space, "words.zip", b.Bytes()); job["status"] != "done" {
		t.Fatalf("%+v", job)
	}
	reader.want(404, "GET", "/api/v1/pages/"+pageID(t, e, "One"), nil)
	reader.want(404, "GET", "/api/v1/pages/"+pageID(t, e, "Imported Word files"), nil)
}

func TestAnOldWordFileSaysWhatToDo(t *testing.T) {
	_, owner, space := importSetup(t)
	job := owner.importFile(space, "old.doc", []byte{0xD0, 0xCF, 0x11, 0xE0})
	if job["status"] != "failed" {
		t.Fatalf("%+v", job)
	}
}

func TestWordGoesIntoADraftWithoutPublishing(t *testing.T) {
	e, owner, space := importSetup(t)
	page := owner.page(space, nil, "Target", "x")
	status, got := owner.post("/api/v1/pages/"+page+"/import-word", wordFile(t, "Pasted"))
	if status != 200 || !strings.Contains(mustJSON(t, got), "Pasted") {
		t.Fatalf("%d %v", status, got)
	}
	if strings.Contains(documentOf(t, e, "Target"), "Pasted") {
		t.Fatal("the published page changed")
	}
}

func TestHTMLThroughTheAllowListKeepsNoScript(t *testing.T) {
	e, owner, space := importSetup(t)
	page := `<html><head><title>Hello</title><script>alert(1)</script></head><body onload="x()">
		<h2 onclick="y()">Part</h2><p>Text <b>bold</b> <a href="javascript:evil()">bad</a> <a href="other.html">next</a></p>
		<img src="https://evil.example.com/a.png" alt="outside"><img src="pic.png" alt="inside">
		<iframe src="https://evil.example.com"></iframe><script>alert(2)</script><table><tr><th>H</th></tr><tr><td>C</td></tr></table></body></html>`
	job := owner.importFile(space, "site.zip", zipOf(t, map[string]string{"page.html": page,
		"other.html": "<title>Other</title><p>x</p>", "pic.png": pngHeader}))
	if job["status"] != "done" {
		t.Fatalf("%+v", job)
	}
	got := documentOf(t, e, "Hello")
	for _, bad := range []string{"alert", "javascript", "onclick", "iframe"} {
		if strings.Contains(got, bad) {
			t.Errorf("%s survived: %s", bad, got)
		}
	}
	for _, want := range []string{`"table"`, `"bold"`, "/pages/" + pageID(t, e, "Other"), `"attachment_id"`, "https://evil.example.com/a.png"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s: %s", want, got)
		}
	}
	if strings.Contains(got, `"src"`) {
		t.Errorf("an outside image is shown: %s", got)
	}
}

func TestHTMLFoldersMakeATree(t *testing.T) {
	e, owner, space := importSetup(t)
	job := owner.importFile(space, "site.zip", zipOf(t, map[string]string{
		"guide/index.html": "<title>Guide</title><p>top</p>", "guide/step.html": "<title>Step</title><p>x</p>"}))
	if job["status"] != "done" {
		t.Fatalf("%+v", job)
	}
	var parent string
	if err := e.app.Pool().QueryRow(context.Background(), `SELECT parent.title FROM pages p JOIN pages parent ON parent.id = p.parent_id WHERE p.title = 'Step'`).Scan(&parent); err != nil || parent != "Guide" {
		t.Fatalf("parent %q %v", parent, err)
	}
}

func TestAnImportStoppedHalfwayCanBeUndone(t *testing.T) {
	e, owner, space := importSetup(t)
	before := pageCount(t, e, space)
	huge := strings.Repeat("x", 21<<20)
	run := owner.report(space, "a.zip", zipOf(t, map[string]string{"A.md": "# ok"}))
	if run["state"] != "ready" {
		t.Fatalf("%+v", run)
	}
	failed := owner.importFile(space, "b.zip", zipOf(t, map[string]string{"A.md": "# ok", "B.md": huge}))
	if failed["status"] != "failed" {
		t.Fatalf("%+v", failed)
	}
	var id string
	if err := e.app.Pool().QueryRow(context.Background(), `SELECT id FROM import_runs WHERE state = 'failed'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	owner.ok("POST", "/api/v1/imports/"+id+"/undo", map[string]any{})
	if got := pageCount(t, e, space); got != before {
		t.Fatalf("left %d pages", got-before)
	}
}

// bundleOf makes a bundle of a space on the same instance.
func bundleOf(t *testing.T, owner *user, space string) []byte {
	t.Helper()
	run := owner.ok("POST", "/api/v1/spaces/"+space+"/bundles", map[string]any{})
	owner.e.runJobs()
	got := owner.ok("GET", "/api/v1/exports/"+run["id"].(string), nil)
	if got["state"] != "done" {
		t.Fatalf("%+v", got)
	}
	return owner.exportFile(run["id"].(string))
}

func bundleSetup(t *testing.T) (*env, *user, *user, string) {
	t.Helper()
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("SRC")
	page := owner.page(space, nil, "Bundled", "first")
	owner.publish(page, "Bundled", 1, docWith("second"))
	owner.comment(page, "a remark")
	owner.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "kept"})
	owner.upload(page, "a.txt", "hello")
	return e, admin, owner, space
}

func TestABundleMovesASpaceWithItsHistory(t *testing.T) {
	e, admin, owner, space := bundleSetup(t)
	data := bundleOf(t, owner, space)
	files := zipNames(t, data)
	for _, name := range []string{"manifest.json", "manifest.sig", "pages.json"} {
		if _, ok := files[name]; !ok {
			t.Fatalf("missing %s", name)
		}
	}
	// The permission to read a space's audit is a grant like any other, not audit itself.
	held := strings.ReplaceAll(files["people.json"]+files["space.json"], `"`+permissions.SpaceAudit+`"`, "")
	for _, secret := range []string{"password", "session", "audit"} {
		if strings.Contains(strings.ToLower(held), secret) {
			t.Fatalf("a bundle holds %s", secret)
		}
	}

	status, got := admin.post("/api/v1/admin/bundles?filename=b.zip", data)
	if status != 202 {
		t.Fatalf("%d %v", status, got)
	}
	e.runJobs()
	id := got["id"].(string)
	run := admin.ok("GET", "/api/v1/imports/"+id, nil)
	report := run["report"].(map[string]any)
	if run["state"] != "ready" || report["versions"].(float64) < 2 || report["comments"].(float64) != 1 {
		t.Fatalf("%+v", run)
	}
	admin.ok("PUT", "/api/v1/imports/"+id+"/mapping", map[string]any{"space_key": "COPY"})
	done := admin.carryOut(id)
	if done["state"] != "done" {
		t.Fatalf("%+v", done)
	}
	ctx := context.Background()
	var copyPage, copySpace string
	if err := e.app.Pool().QueryRow(ctx, `SELECT p.id, p.space_id FROM pages p JOIN spaces s ON s.id = p.space_id WHERE s.key = 'COPY' AND p.title = 'Bundled'`).Scan(&copyPage, &copySpace); err != nil {
		t.Fatal(err)
	}
	var versions, comments, labels, files2, imported int
	err := e.app.Pool().QueryRow(ctx, `SELECT (SELECT count(*) FROM page_versions WHERE page_id = $1), (SELECT count(*) FROM comments WHERE page_id = $1),
		(SELECT count(*) FROM page_labels WHERE page_id = $1), (SELECT count(*) FROM attachments WHERE page_id = $1),
		(SELECT count(*) FROM page_versions WHERE page_id = $1 AND imported)`, copyPage).Scan(&versions, &comments, &labels, &files2, &imported)
	if err != nil || versions != 2 || comments != 1 || labels != 1 || files2 != 1 || imported != 2 {
		t.Fatalf("%d %d %d %d %d %v", versions, comments, labels, files2, imported, err)
	}
	if e.auditActions("bundle.imported") != 1 {
		t.Fatal("a bundle must be audited")
	}
	history := admin.ok("GET", "/api/v1/pages/"+copyPage+"/versions", nil)
	if !strings.Contains(mustJSON(t, history), `"imported":true`) {
		t.Fatalf("the history does not say it is imported: %v", history)
	}
}

func changeOneByte(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, f := range zr.File {
		rc, _ := f.Open()
		body, _ := io.ReadAll(rc)
		_ = rc.Close()
		if f.Name == name {
			body[len(body)/2] ^= 1
		}
		w, _ := zw.Create(f.Name)
		_, _ = w.Write(body)
	}
	_ = zw.Close()
	return b.Bytes()
}

func TestOneChangedByteStopsABundle(t *testing.T) {
	e, admin, owner, space := bundleSetup(t)
	data := bundleOf(t, owner, space)
	for _, name := range []string{"pages.json", "manifest.json", "manifest.sig"} {
		_, got := admin.post("/api/v1/admin/bundles?filename=b.zip", changeOneByte(t, data, name))
		e.runJobs()
		run := admin.ok("GET", "/api/v1/imports/"+got["id"].(string), nil)
		if run["state"] != "failed" {
			t.Fatalf("%s: %+v", name, run)
		}
		// The byte can also break the JSON, and then it would fail without the hash ever counting.
		if name == "pages.json" && !strings.Contains(mustJSON(t, run["report"]), "does not match its manifest") {
			t.Fatalf("%s failed for another reason: %+v", name, run)
		}
	}
}

func TestAnUnknownSignerNeedsAFingerprint(t *testing.T) {
	e, admin, owner, space := bundleSetup(t)
	data := bundleOf(t, owner, space)
	_, got := admin.post("/api/v1/admin/bundles?filename=b.zip", data)
	e.runJobs()
	id := got["id"].(string)
	signer := admin.ok("GET", "/api/v1/imports/"+id, nil)["report"].(map[string]any)["signer"].(string)
	// The same key would be trusted as our own, so this makes it a stranger's.
	if _, err := e.app.Pool().Exec(context.Background(), `UPDATE import_runs SET report = jsonb_set(report, '{signer}', '"00"') WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	admin.want(409, "POST", "/api/v1/imports/"+id+"/run", map[string]any{})
	admin.want(400, "PUT", "/api/v1/imports/"+id+"/mapping", map[string]any{"fingerprint": signer})
	admin.ok("PUT", "/api/v1/imports/"+id+"/mapping", map[string]any{"fingerprint": "00", "space_key": "NEW"})
}

func TestAnUnmatchedRestrictionStaysClosed(t *testing.T) {
	e, admin, owner, space := bundleSetup(t)
	page := pageID(t, e, "Bundled")
	other := e.user("gone@example.com", identity.PermUseProduct)
	owner.grant(space, "user", other.id, permissions.TemplateViewer)
	owner.restrict(page, "view", other.id, owner.id)
	data := bundleOf(t, owner, space)
	ctx := context.Background()
	// Nobody with those addresses exists at the other end.
	if _, err := e.app.Pool().Exec(ctx, `UPDATE users SET email = email || '.moved' WHERE id = ANY($1::uuid[])`, []string{other.id, owner.id}); err != nil {
		t.Fatal(err)
	}
	_, got := admin.post("/api/v1/admin/bundles?filename=b.zip", data)
	e.runJobs()
	id := got["id"].(string)
	admin.ok("PUT", "/api/v1/imports/"+id+"/mapping", map[string]any{"space_key": "CLOSED"})
	admin.carryOut(id)
	var count int
	err := e.app.Pool().QueryRow(ctx, `SELECT count(*) FROM page_restrictions r JOIN pages p ON p.id = r.page_id JOIN spaces s ON s.id = p.space_id
		WHERE s.key = 'CLOSED' AND r.kind = 'view' AND r.subject_id = $1`, admin.id).Scan(&count)
	if err != nil || count == 0 {
		t.Fatalf("the page opened up: %d %v", count, err)
	}
}

func TestABundleStopsOnAPageThePersonCannotSee(t *testing.T) {
	e, _, owner, space := bundleSetup(t)
	manager := e.user("manager@example.com", identity.PermUseProduct)
	owner.grant(space, "user", manager.id, permissions.TemplateSpaceAdmin)
	owner.restrict(pageID(t, e, "Bundled"), "view", owner.id)
	run := manager.ok("POST", "/api/v1/spaces/"+space+"/bundles", map[string]any{})
	e.runJobs()
	if got := manager.ok("GET", "/api/v1/exports/"+run["id"].(string), nil); got["state"] != "failed" {
		t.Fatalf("%+v", got)
	}
}

func TestAGrantToAllMembersIsShownInTheReport(t *testing.T) {
	e, admin, owner, space := bundleSetup(t)
	owner.grant(space, "all_members", "00000000-0000-0000-0000-000000000000", permissions.TemplateViewer)
	data := bundleOf(t, owner, space)
	_, got := admin.post("/api/v1/admin/bundles?filename=b.zip", data)
	e.runJobs()
	run := admin.ok("GET", "/api/v1/imports/"+got["id"].(string), nil)
	if !strings.Contains(mustJSON(t, run["report"]), "all members") {
		t.Fatalf("%+v", run)
	}
}
