// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/permissions"
)

// runJobs works off the queue, as the worker does.
func (e *env) runJobs() {
	e.t.Helper()
	for {
		ran, err := e.app.Jobs().RunOnce(context.Background())
		if err != nil {
			e.t.Fatal(err)
		}
		if !ran {
			return
		}
	}
}

// export starts an export, runs it, and returns the run.
func (u *user) export(in map[string]any) map[string]any {
	u.e.t.Helper()
	run := u.ok("POST", "/api/v1/exports", in)
	u.e.runJobs()
	return u.ok("GET", "/api/v1/exports/"+run["id"].(string), nil)
}

func (u *user) exportFile(id string) []byte {
	u.e.t.Helper()
	resp, body := u.e.do(u.c, "GET", "/api/v1/exports/"+id+"/file", "")
	if resp.StatusCode != 200 {
		u.e.t.Fatalf("download: %d %s", resp.StatusCode, body)
	}
	return []byte(body)
}

func zipNames(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		_ = rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

func exportSetup(t *testing.T) (*env, *user, string, string, string) {
	t.Helper()
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("EXP")
	parent := owner.page(space, nil, "Parent", "parent words")
	child := owner.page(space, &parent, "Child", "child words")
	return e, owner, space, parent, child
}

func TestEveryFormatExportsABranchInTheBackground(t *testing.T) {
	e, owner, space, parent, _ := exportSetup(t)
	for _, format := range []string{"text", "markdown", "html", "docx", "odt"} {
		run := owner.export(map[string]any{"space_id": space, "root_id": parent, "format": format})
		if run["state"] != "done" {
			t.Fatalf("%s: %+v", format, run)
		}
		if report := run["report"].(map[string]any); report["pages"].(float64) != 2 {
			t.Fatalf("%s: %+v", format, report)
		}
		data := owner.exportFile(run["id"].(string))
		if len(data) == 0 {
			t.Fatalf("%s: no file", format)
		}
	}
	if e.auditActions("export.created") != 5 || e.auditActions("export.downloaded") != 5 {
		t.Fatal("exports must be audited")
	}
}

func TestHTMLExportWorksOfflineWithRelativeLinksAndNoScript(t *testing.T) {
	_, owner, space, parent, _ := exportSetup(t)
	run := owner.export(map[string]any{"space_id": space, "root_id": parent, "format": "html"})
	files := zipNames(t, owner.exportFile(run["id"].(string)))
	for _, want := range []string{"index.html", "style.css", "Parent.html", "Child.html"} {
		if _, ok := files[want]; !ok {
			t.Fatalf("missing %s in %v", want, files)
		}
	}
	for name, body := range files {
		if strings.Contains(body, "<script") {
			t.Errorf("%s has a script", name)
		}
	}
	if !strings.Contains(files["index.html"], `href="Child.html"`) {
		t.Fatalf("index: %s", files["index.html"])
	}
}

func TestChosenPagesOnlyAndDraftIsNamed(t *testing.T) {
	_, owner, space, parent, child := exportSetup(t)
	draft := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Unfinished"})["id"].(string)
	run := owner.export(map[string]any{"space_id": space, "format": "text", "page_ids": []string{child, draft}})
	report := run["report"].(map[string]any)
	if report["pages"].(float64) != 1 || len(report["drafts"].([]any)) != 1 || report["drafts"].([]any)[0] != "Unfinished" {
		t.Fatalf("%+v", report)
	}
	text := string(owner.exportFile(run["id"].(string)))
	if !strings.Contains(text, "child words") || strings.Contains(text, "parent words") {
		t.Fatalf("got %q", text)
	}
	_ = parent
}

func TestExportLeavesOutWhatThePersonCannotSee(t *testing.T) {
	e, owner, space, parent, child := exportSetup(t)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	owner.restrict(child, "view", owner.id)

	run := reader.export(map[string]any{"space_id": space, "format": "text", "root_id": parent})
	text := string(reader.exportFile(run["id"].(string)))
	if strings.Contains(text, "child words") {
		t.Fatalf("a hidden page is in the file: %q", text)
	}
	// The list itself leaves it out, so nothing later has anything to skip, and the
	// report does not even hint that the page exists.
	report := run["report"].(map[string]any)
	if report["pages"].(float64) != 1 || report["skipped"].(float64) != 0 || report["restricted"].(float64) != 0 {
		t.Fatalf("%+v", run)
	}
}

func TestRestrictedPagesStayOutUntilChosen(t *testing.T) {
	_, owner, space, parent, child := exportSetup(t)
	owner.restrict(child, "view", owner.id)
	without := owner.export(map[string]any{"space_id": space, "format": "text", "root_id": parent})
	report := without["report"].(map[string]any)
	if report["pages"].(float64) != 1 || report["restricted"].(float64) != 1 {
		t.Fatalf("%+v", report)
	}
	with := owner.export(map[string]any{"space_id": space, "format": "text", "root_id": parent, "options": map[string]any{"restricted": true}})
	if with["report"].(map[string]any)["pages"].(float64) != 2 {
		t.Fatalf("%+v", with)
	}
}

func TestOnlyTheOwnerDownloadsAnExport(t *testing.T) {
	e, owner, space, parent, _ := exportSetup(t)
	other := e.user("other@example.com", identity.PermUseProduct)
	owner.grant(space, "user", other.id, permissions.TemplateViewer)
	run := owner.export(map[string]any{"space_id": space, "format": "text", "root_id": parent})
	id := run["id"].(string)
	other.want(404, "GET", "/api/v1/exports/"+id, nil)
	other.want(404, "GET", "/api/v1/exports/"+id+"/file", nil)
}

func TestAFileIsRefusedOnceAPageInItIsHidden(t *testing.T) {
	e, owner, space, parent, child := exportSetup(t)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	run := reader.export(map[string]any{"space_id": space, "format": "text", "root_id": parent})
	id := run["id"].(string)
	reader.exportFile(id)
	owner.restrict(child, "view", owner.id)
	reader.want(409, "GET", "/api/v1/exports/"+id+"/file", nil)
}

func TestAnExpiredFileIsGone(t *testing.T) {
	e, owner, space, parent, _ := exportSetup(t)
	run := owner.export(map[string]any{"space_id": space, "format": "text", "root_id": parent})
	id := run["id"].(string)
	ctx := context.Background()
	if _, err := e.app.Pool().Exec(ctx, `UPDATE export_runs SET expires_at = now() - interval '1 minute' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	owner.want(409, "GET", "/api/v1/exports/"+id+"/file", nil)
	if err := e.app.Exchange().PurgeExports(ctx); err != nil {
		t.Fatal(err)
	}
	if got := owner.ok("GET", "/api/v1/exports/"+id, nil); got["state"] != "expired" {
		t.Fatalf("%+v", got)
	}
	var left int
	if err := e.app.Pool().QueryRow(ctx, `SELECT count(*) FROM export_runs WHERE object_key <> '' AND id = $1`, id).Scan(&left); err != nil || left != 0 {
		t.Fatalf("left %d %v", left, err)
	}
}

func TestOneExportAtATimePerPerson(t *testing.T) {
	_, owner, space, parent, _ := exportSetup(t)
	owner.ok("POST", "/api/v1/exports", map[string]any{"space_id": space, "format": "text", "root_id": parent})
	owner.want(409, "POST", "/api/v1/exports", map[string]any{"space_id": space, "format": "text", "root_id": parent})
}

func TestAFormatCanBeTurnedOffOrLimitedToAGroup(t *testing.T) {
	e, owner, space, parent, _ := exportSetup(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	limits := map[string]any{"max_export_mib": 100, "max_import_mib": 100}
	admin.want(204, "PUT", "/api/v1/admin/export-policy", map[string]any{"limits": limits,
		"rules": []map[string]any{{"format": "docx", "enabled": false, "group_ids": []string{}}}})
	owner.want(403, "POST", "/api/v1/exports", map[string]any{"space_id": space, "format": "docx", "root_id": parent})
	formats := owner.ok("GET", "/api/v1/export-formats", nil)["formats"].([]any)
	for _, f := range formats {
		if f == "docx" {
			t.Fatal("the dialog offers a format that is off")
		}
	}
	group := admin.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Writers"})["id"].(string)
	admin.want(204, "PUT", "/api/v1/admin/export-policy", map[string]any{"limits": limits,
		"rules": []map[string]any{{"format": "docx", "enabled": true, "group_ids": []string{group}}}})
	owner.want(403, "POST", "/api/v1/exports", map[string]any{"space_id": space, "format": "docx", "root_id": parent})
	admin.want(204, "PUT", "/api/v1/admin/groups/"+group+"/members/"+owner.id, nil)
	owner.ok("POST", "/api/v1/exports", map[string]any{"space_id": space, "format": "docx", "root_id": parent})
}

func TestAFailingPageIsNamedAndTheRestArrives(t *testing.T) {
	e, owner, space, parent, child := exportSetup(t)
	ctx := context.Background()
	// A document only the database can hold: this is what a bug elsewhere would leave.
	if _, err := e.app.Pool().Exec(ctx, `INSERT INTO page_versions (id, origin_instance_id, page_id, number, title, document, schema_version, author_id)
		SELECT gen_random_uuid(), origin_instance_id, page_id, 99, title, '[1]', schema_version, author_id FROM page_versions WHERE page_id = $1 LIMIT 1`, child); err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Pool().Exec(ctx, `UPDATE pages SET current_version = 99 WHERE id = $1`, child); err != nil {
		t.Fatal(err)
	}
	run := owner.export(map[string]any{"space_id": space, "format": "html", "root_id": parent})
	report := run["report"].(map[string]any)
	if run["state"] != "done" || len(report["failed"].([]any)) != 1 {
		t.Fatalf("%+v", run)
	}
	if files := zipNames(t, owner.exportFile(run["id"].(string))); !strings.Contains(files["missing.txt"], "Child") {
		t.Fatalf("no note: %v", files)
	}
}

func TestAFinishedExportReachesThePersonAsANotification(t *testing.T) {
	e, owner, space, parent, _ := exportSetup(t)
	owner.export(map[string]any{"space_id": space, "format": "text", "root_id": parent})
	if err := e.app.DeliverEvents(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := e.app.Pool().QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE type = 'export_ready'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("notifications %d %v", count, err)
	}
}

func TestPDFGoesThroughTheHelperAndIsAbsentWithoutIt(t *testing.T) {
	e, owner, space, parent, _ := exportSetup(t)
	owner.want(403, "POST", "/api/v1/exports", map[string]any{"space_id": space, "format": "pdf", "root_id": parent})

	var form string
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		form = string(b)
		_, _ = w.Write([]byte("%PDF-fake"))
	}))
	defer helper.Close()
	e.app.Exchange().SetPDFHelper(helper.URL)
	defer e.app.Exchange().SetPDFHelper("")

	run := owner.export(map[string]any{"space_id": space, "format": "pdf", "root_id": parent,
		"options": map[string]any{"contents": true, "paper": "Letter", "orientation": "landscape"}})
	if run["state"] != "done" {
		t.Fatalf("%+v", run)
	}
	if got := owner.exportFile(run["id"].(string)); string(got) != "%PDF-fake" {
		t.Fatalf("got %q", got)
	}
	for _, want := range []string{"parent words", "child words", "Contents", "generateTaggedPdf", `name="landscape"`} {
		if !strings.Contains(form, want) {
			t.Errorf("the helper did not get %s", want)
		}
	}
}

func TestAFailingHelperFailsTheExportWithASentence(t *testing.T) {
	e, owner, space, parent, _ := exportSetup(t)
	helper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer helper.Close()
	e.app.Exchange().SetPDFHelper(helper.URL)
	defer e.app.Exchange().SetPDFHelper("")
	run := owner.ok("POST", "/api/v1/exports", map[string]any{"space_id": space, "format": "pdf", "root_id": parent})
	if _, err := e.app.Jobs().RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := owner.ok("GET", "/api/v1/exports/"+run["id"].(string), nil)
	if got["state"] != "failed" || got["report"].(map[string]any)["explanation"] == "" {
		t.Fatalf("%+v", got)
	}
	_ = jobs.StatusFailed
}

func TestAViewerCanExportButAStrangerCannot(t *testing.T) {
	e, _, space, parent, _ := exportSetup(t)
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	stranger.want(404, "POST", "/api/v1/exports", map[string]any{"space_id": space, "format": "text", "root_id": parent})
}
