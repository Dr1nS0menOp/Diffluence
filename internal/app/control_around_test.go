// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/kenwerk/kenwerk/internal/config"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/mail"
	"github.com/kenwerk/kenwerk/internal/notify"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// Document control, wave DC3 of spec 064: the content around the page. A reader held on the
// approved version never sees the working version through a comment, a file, search, an
// export, a public site, the API, a webhook or a notification. Each test starts from the
// leak, and only then shows that whoever may see the working version gets it.

// workingPage is a controlled page approved with approvedText, with a working version that
// holds workingText waiting for review.
func (w controlWorld) workingPage(title, approvedText, workingTitle, workingText string) string {
	w.e.t.Helper()
	page := w.controlled(title, approvedText)
	w.approve(page)
	w.publishAs(w.author, page, workingTitle, workingText, false)
	return page
}

func commentsSeen(u *user, page string) string {
	u.e.t.Helper()
	return fmt.Sprint(u.ok("GET", "/api/v1/pages/"+page+"/comments", nil))
}

// TestCommentsOnTheWorkingVersionStayWithItsAuthors is DOC-500: a comment or a reply that an
// author writes while a working version waits is about that version, so readers see it only
// once a version holding it is approved; a reader's comment is about the approved version
// and everyone sees it. A reviewer who may not edit puts remarks in the review instead.
func TestCommentsOnTheWorkingVersionStayWithItsAuthors(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCM")
	page := w.workingPage("Handling", "approved handling", "Handling", "draft handling quokkaword")

	w.author.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("the draft now says quokkaword")})
	asked := w.reader.comment(page, "is this still right")
	w.author.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("not in the draft: wombatword"), "parent_id": asked})
	w.author.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("about this sentence"),
		"anchor": map[string]any{"kind": "text", "exact": "draft handling quokkaword"}})

	for _, word := range []string{"quokkaword", "wombatword", "about this sentence"} {
		if got := commentsSeen(w.reader, page); strings.Contains(got, word) {
			t.Fatalf("a reader sees a comment on the working version (%s): %s", word, got)
		}
		for _, u := range []*user{w.reader, w.guest} {
			for _, row := range u.ok("GET", "/api/v1/search?q="+strings.Fields(word)[0]+"&kind=comment", nil)["results"].([]any) {
				t.Fatalf("%s finds a comment on the working version: %v", u.email, row)
			}
		}
		if got := commentsSeen(w.author, page); !strings.Contains(got, word) {
			t.Fatalf("the author lost a comment (%s): %s", word, got)
		}
	}
	if got := commentsSeen(w.reader, page); !strings.Contains(got, "is this still right") {
		t.Fatalf("a reader's own comment is gone: %s", got)
	}

	w.approve(page)
	got := commentsSeen(w.reader, page)
	for _, word := range []string{"quokkaword", "wombatword", "about this sentence"} {
		if !strings.Contains(got, word) {
			t.Fatalf("after approval a reader misses %s: %s", word, got)
		}
	}
}

// TestAnInlineCommentOnTextThatNeverGetsApprovedStaysWithTheAuthors is the edge of DOC-500: an
// inline comment anchored to text that changed again stays with its version.
func TestAnInlineCommentOnTextThatNeverGetsApprovedStaysWithTheAuthors(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCN")
	page := w.workingPage("Steps", "approved steps", "Steps", "first draft secretphrase")
	w.author.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("check this"),
		"anchor": map[string]any{"kind": "text", "exact": "first draft secretphrase"}})
	w.publishAs(w.author, page, "Steps", "second draft without it", false)
	w.approve(page)
	if got := commentsSeen(w.reader, page); strings.Contains(got, "secretphrase") || strings.Contains(got, "check this") {
		t.Fatalf("a reader sees a comment on text that was never approved: %s", got)
	}
}

// TestAReviewerWhoMayNotEditPutsRemarksInTheReview is DOC-500 with DOC-251: a reviewer reads
// the working version, and a page comment from them would reach readers, so it is refused.
func TestAReviewerWhoMayNotEditPutsRemarksInTheReview(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DCR")
	flow := w.workflowOf("Any one", reviewedDef(review("any", 0, entry("user", w.r1.id))))
	page := w.author.page(w.space, w.home, "Reviewed", "approved words")
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": flow})
	w.step(page, "submit")
	if code, out := w.answer(w.r1, page, "approve"); code != 200 {
		t.Fatalf("approve: %d %s", code, out)
	}
	w.publishAs(w.author, page, "Reviewed", "working words", false)
	w.step(page, "submit")
	out := w.r1.want(409, "POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("looks fine")})
	if !strings.Contains(out, "comment_in_review") {
		t.Fatalf("a reviewer's page comment: %s", out)
	}
}

// TestResolvingAndDeletingCommentsOnAControlledPageAreAudited is DOC-502, and that a review
// comment has no way to be deleted.
func TestResolvingAndDeletingCommentsOnAControlledPageAreAudited(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCO")
	page := w.controlled("Audited", "text")
	w.approve(page)
	thread := w.reader.comment(page, "a remark")
	w.author.want(204, "POST", "/api/v1/comments/"+thread+"/resolve", nil)
	w.admin.want(204, "DELETE", "/api/v1/comments/"+thread, nil)
	for _, action := range []string{"comment.resolved", "comment.trashed"} {
		if e.count("SELECT count(*) FROM audit_entries WHERE action = $1 AND object_id = $2", action, thread) != 1 {
			t.Fatalf("%s is not audited", action)
		}
	}
	if code, _ := w.admin.do("DELETE", "/api/v1/pages/"+page+"/review/comments/"+thread, nil); code != 404 && code != 405 {
		t.Fatalf("a review comment could be deleted: %d", code)
	}
}

// fileTile is a document that shows one file under some words.
func fileTile(file, name, text string) map[string]any {
	return map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}},
		map[string]any{"type": "file", "attrs": map[string]any{"attachment_id": file, "filename": name}}}}
}

func (w controlWorld) publishDoc(page, title string, doc map[string]any) {
	w.e.t.Helper()
	w.author.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": title, "document": doc, "base_version": w.head(page)})
}

func download(u *user, file string) (int, string) {
	u.e.t.Helper()
	return u.do("GET", "/api/v1/attachments/"+file+"/content", nil)
}

// approvedWithFile makes a controlled page whose approved version shows form.txt.
func (w controlWorld) approvedWithFile(title string) (page, file string) {
	w.e.t.Helper()
	page = w.controlled(title, "approved text")
	file = w.author.file(page, "form.txt", "approved form")
	w.publishDoc(page, title, fileTile(file, "form.txt", "approved text"))
	w.approve(page)
	return page, file
}

// TestReadersDownloadThePinnedVersionOfAFile is DOC-510 and DOC-511: after a new upload a
// reader still gets the version the approved version showed, everywhere a file can be had,
// and the upload starts review again.
func TestReadersDownloadThePinnedVersionOfAFile(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCF")
	page, file := w.approvedWithFile("Form")
	if w.state(page) != "approved" {
		t.Fatalf("state %s", w.state(page))
	}
	w.author.file(page, "form.txt", "changed form")
	if w.state(page) != "draft" {
		t.Fatalf("a new version of a shown file did not start review again: %s", w.state(page))
	}
	for _, u := range []*user{w.reader, w.guest} {
		if code, body := download(u, file); code != 200 || body != "approved form" {
			t.Fatalf("%s downloads %d %q", u.email, code, body)
		}
		if code, body := u.do("GET", "/api/v1/attachments/"+file+"/text", nil); code != 200 || strings.Contains(body, "changed") {
			t.Fatalf("%s previews %d %s", u.email, code, body)
		}
		if code, _ := u.do("GET", "/api/v1/attachments/"+file+"/versions/2/content", nil); code != 404 {
			t.Fatalf("%s reaches the new version by its number: %d", u.email, code)
		}
		versions := u.ok("GET", "/api/v1/attachments/"+file+"/versions", nil)["versions"].([]any)
		if len(versions) != 1 || versions[0].(map[string]any)["number"].(float64) != 1 {
			t.Fatalf("%s sees versions %v", u.email, versions)
		}
		facts := fmt.Sprint(u.ok("POST", "/api/v1/attachments/lookup", map[string]any{"ids": []string{file}}))
		if !strings.Contains(facts, "size:13") {
			t.Fatalf("%s gets the facts of the new version: %s", u.email, facts)
		}
	}
	if code, body := download(w.author, file); code != 200 || body != "changed form" {
		t.Fatalf("the author downloads %d %q", code, body)
	}
	w.approve(page)
	if code, body := download(w.reader, file); code != 200 || body != "changed form" {
		t.Fatalf("after approval the reader downloads %d %q", code, body)
	}
}

// TestAFileOnlyTheWorkingVersionHoldsStaysWithItsAuthors is DOC-512 and the file name of
// DOC-520: a new file, or a new name, does not reach a reader on the page, on the Files
// screen, in its filters, in quick search, in search or in the history of its files.
func TestAFileOnlyTheWorkingVersionHoldsStaysWithItsAuthors(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCG")
	page, file := w.approvedWithFile("Forms")
	secret := w.author.file(page, "platypusplan.txt", "secret plan")
	w.author.ok("PATCH", "/api/v1/attachments/"+file, map[string]any{"filename": "echidnaform.txt", "comment": ""})
	if w.state(page) != "draft" {
		t.Fatalf("a new name of a shown file did not start review again: %s", w.state(page))
	}
	for _, u := range []*user{w.reader, w.guest} {
		views := map[string]string{
			"page files":  fmt.Sprint(u.ok("GET", "/api/v1/pages/"+page+"/attachments", nil)),
			"files":       fmt.Sprint(u.ok("GET", "/api/v1/spaces/"+w.space+"/files", nil)),
			"filters":     fmt.Sprint(u.ok("GET", "/api/v1/spaces/"+w.space+"/files/filters", nil)),
			"quick":       fmt.Sprint(u.ok("GET", "/api/v1/search/quick?q=platypusplan", nil)),
			"quick name":  fmt.Sprint(u.ok("GET", "/api/v1/search/quick?q=echidnaform", nil)),
			"search":      fmt.Sprint(u.ok("GET", "/api/v1/search?q=platypusplan", nil)),
			"search name": fmt.Sprint(u.ok("GET", "/api/v1/search?q=echidnaform", nil)),
			"file events": fmt.Sprint(u.ok("GET", "/api/v1/pages/"+page+"/file-events", nil)),
		}
		for where, got := range views {
			if strings.Contains(got, "platypusplan") || strings.Contains(got, "echidnaform") || strings.Contains(got, secret) {
				t.Fatalf("%s finds the working file on %s: %s", u.email, where, got)
			}
		}
		if code, _ := download(u, secret); code != 404 {
			t.Fatalf("%s downloads a file the approved version does not show: %d", u.email, code)
		}
		if got := fmt.Sprint(u.ok("GET", "/api/v1/pages/"+page+"/attachments", nil)); !strings.Contains(got, "form.txt") {
			t.Fatalf("%s lost the approved file: %s", u.email, got)
		}
	}
	if got := fmt.Sprint(w.author.ok("GET", "/api/v1/pages/"+page+"/attachments", nil)); !strings.Contains(got, "platypusplan") || !strings.Contains(got, "echidnaform") {
		t.Fatalf("the author lost the working files: %s", got)
	}
	if got := fmt.Sprint(w.author.ok("GET", "/api/v1/search/quick?q=platypusplan", nil)); !strings.Contains(got, secret) {
		t.Fatalf("the author does not find the new file: %s", got)
	}
}

// TestAFileTheApprovedVersionShowsCannotBeTakenAway is the edge of DOC-510: deleting, moving
// or removing the pinned version of such a file is refused, until a version without it is
// approved.
func TestAFileTheApprovedVersionShowsCannotBeTakenAway(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCH")
	page, file := w.approvedWithFile("Kept")
	other := w.author.page(w.space, w.home, "Elsewhere", "x")
	w.author.file(page, "form.txt", "second")
	for _, try := range []func() string{
		func() string { return w.admin.want(409, "DELETE", "/api/v1/attachments/"+file, nil) },
		func() string { return w.admin.want(409, "DELETE", "/api/v1/attachments/"+file+"/versions/1", nil) },
		func() string {
			return w.admin.want(409, "POST", "/api/v1/attachments/"+file+"/move", map[string]any{"page_id": other})
		},
	} {
		if out := try(); !strings.Contains(out, "file_approved") {
			t.Fatalf("a pinned file was taken away: %s", out)
		}
	}
	w.publishDoc(page, "Kept", para("no file now"))
	w.approve(page)
	w.admin.want(204, "DELETE", "/api/v1/attachments/"+file, nil)
}

// TestAPageMadeControlledKeepsItsFilesForReaders is DOC-510 for a page that comes under a
// workflow already approved: readers keep the file they had.
func TestAPageMadeControlledKeepsItsFilesForReaders(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCI")
	page := w.author.page(w.space, w.home, "Before", "x")
	file := w.author.file(page, "form.txt", "first")
	w.author.republish(page, "Before", fileTile(file, "form.txt", "x"))
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.workflow})
	w.approve(page)
	w.author.file(page, "form.txt", "second")
	if code, body := download(w.reader, file); code != 200 || body != "first" {
		t.Fatalf("the reader downloads %d %q", code, body)
	}
}

func decode(t *testing.T, body string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	return out
}

// searchFinds says whether a search for a word lists a page, and gives its row.
func searchFinds(u *user, page, query string) (bool, map[string]any) {
	u.e.t.Helper()
	for _, row := range u.ok("GET", "/api/v1/search?q="+query, nil)["results"].([]any) {
		if r := row.(map[string]any); r["page_id"] == page {
			return true, r
		}
	}
	return false, nil
}

func quickFinds(u *user, page, query string) bool {
	u.e.t.Helper()
	return strings.Contains(fmt.Sprint(u.ok("GET", "/api/v1/search/quick?q="+query, nil)), page)
}

// TestAWordOnlyInTheWorkingVersionIsNeverFoundByAReader is DOC-520 and DOC-1302: the body, the
// title and a file name of a working version are found by its authors, with its title and
// excerpt, and never by a reader or a guest. The second entry lives only while the draft
// differs from the approved version.
func TestAWordOnlyInTheWorkingVersionIsNeverFoundByAReader(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCS")
	page := w.workingPage("Calibration", "approved calibration text", "Calibration capybaratitle", "draft calibration okapibody")
	file := w.author.file(page, "narwhalfile.txt", "x")
	w.publishDoc(page, "Calibration capybaratitle", fileTile(file, "narwhalfile.txt", "draft calibration okapibody"))
	for _, u := range []*user{w.reader, w.guest} {
		for _, word := range []string{"okapibody", "capybaratitle", "narwhalfile"} {
			if found, row := searchFinds(u, page, word); found {
				t.Fatalf("%s finds the page by %s: %v", u.email, word, row)
			}
			if quickFinds(u, page, word) {
				t.Fatalf("%s finds the page by %s in the box", u.email, word)
			}
		}
		found, row := searchFinds(u, page, "approved")
		if !found || row["title"] != "Calibration" || strings.Contains(fmt.Sprint(row["snippet"]), "okapibody") {
			t.Fatalf("%s: the approved version is not what search shows: %v", u.email, row)
		}
	}
	for _, word := range []string{"okapibody", "capybaratitle", "narwhalfile"} {
		found, row := searchFinds(w.author, page, word)
		if !found || row["title"] != "Calibration capybaratitle" {
			t.Fatalf("the author does not find the working version by %s: %v", word, row)
		}
	}
	if _, row := searchFinds(w.author, page, "okapibody"); !strings.Contains(fmt.Sprint(row["snippet"]), "okapibody") {
		t.Fatalf("the author's excerpt is not from the working version: %v", row)
	}
	if !quickFinds(w.author, page, "capybaratitle") {
		t.Fatal("the author does not find the working title in the box")
	}
	if n := e.count("SELECT count(*) FROM search_working WHERE page_id = $1", page); n != 1 {
		t.Fatalf("working entries while the draft waits: %d", n)
	}
	w.approve(page)
	if n := e.count("SELECT count(*) FROM search_working WHERE page_id = $1", page); n != 0 {
		t.Fatalf("a working entry outlived its approval: %d", n)
	}
	for _, word := range []string{"okapibody", "capybaratitle", "narwhalfile"} {
		if found, _ := searchFinds(w.reader, page, word); !found {
			t.Fatalf("after approval a reader does not find %s", word)
		}
	}
	never := w.controlled("Never approved", "aardvarkword")
	if found, _ := searchFinds(w.reader, never, "aardvarkword"); found {
		t.Fatal("a reader finds a page that was never approved (DOC-521)")
	}
	if n := e.count("SELECT count(*) FROM search_working WHERE page_id = $1", never); n != 0 {
		t.Fatalf("a page that was never approved has a second entry: %d", n)
	}
	if got := fmt.Sprint(w.reader.ok("GET", "/api/v1/search?q=aardvarkwrd", nil)["suggestion"]); strings.Contains(got, "aardvark") {
		t.Fatalf("spelling suggests a word from a page a reader may not see: %s", got)
	}
}

// TestUntilTheIndexHoldsAnApprovalReadersFindTheVersionBefore is DOC-1253: indexing the new
// approved version is part of the approval, and until it is done a reader finds the version
// approved before, never the working one.
func TestUntilTheIndexHoldsAnApprovalReadersFindTheVersionBefore(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCT")
	page := w.workingPage("Indexed", "oldapprovedword", "Indexed", "newworkingword")
	w.step(page, "submit")
	head := w.head(page)
	ctx := context.Background()
	hold, err := e.app.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// A failure below must not leave the approval waiting on the lock for ever.
	defer func() { _ = hold.Rollback(ctx) }()
	if _, err := hold.Exec(ctx, "SELECT 1 FROM search_documents WHERE page_id = $1 FOR UPDATE", page); err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() {
		code, _ := w.author.do("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": head})
		done <- code
	}()
	waitFor(t, "the approval to wait for the index", func() bool {
		return e.count("SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND query LIKE '%search_documents%'") > 0
	})
	if found, _ := searchFinds(w.reader, page, "newworkingword"); found {
		t.Fatal("while indexing waits, a reader finds the working version")
	}
	if found, _ := searchFinds(w.reader, page, "oldapprovedword"); !found {
		t.Fatal("while indexing waits, a reader loses the version approved before")
	}
	if err := hold.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if code := <-done; code != 200 {
		t.Fatalf("approval: %d", code)
	}
	if found, _ := searchFinds(w.reader, page, "newworkingword"); !found {
		t.Fatal("after the approval a reader does not find the new version")
	}
}

// exported makes a file of one page in a format and gives its text: the file itself, the
// files inside a zip, or for a PDF what the helper was sent.
func exported(u *user, w controlWorld, page, format string, form *string, options map[string]any) string {
	u.e.t.Helper()
	run := u.export(map[string]any{"space_id": w.space, "format": format, "page_ids": []string{page}, "options": options})
	if run["state"] != "done" {
		u.e.t.Fatalf("%s export as %s: %v", format, u.email, run)
	}
	data := u.exportFile(run["id"].(string))
	if format == "pdf" {
		return *form
	}
	if !strings.HasPrefix(string(data), "PK") {
		return string(data)
	}
	return fmt.Sprint(zipNames(u.e.t, data))
}

// TestEveryExportHoldsTheVersionThePersonMaySee is DOC-530 and DOC-531: as a reader each format
// holds the approved version and its control block; as an author the working version, marked
// not approved, on every page of a PDF. A reader's HTML holds no old version of the page, and
// its files are the pinned ones.
func TestEveryExportHoldsTheVersionThePersonMaySee(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCX")
	page, file := w.approvedWithFile("Exported")
	w.publishDoc(page, "Exported", fileTile(file, "form.txt", "workingexport words"))
	w.author.file(page, "form.txt", "working form")

	var form string
	helper := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		form = string(b)
		_, _ = rw.Write([]byte("%PDF-fake"))
	}))
	defer helper.Close()
	e.app.Exchange().SetPDFHelper(helper.URL)
	defer e.app.Exchange().SetPDFHelper("")

	for _, format := range []string{"text", "html", "markdown", "docx", "odt", "pdf"} {
		got := exported(w.reader, w, page, format, &form, map[string]any{"attachments": true, "versions": true})
		if strings.Contains(got, "workingexport") || strings.Contains(got, "working form") || strings.Contains(got, notApprovedLine) {
			t.Fatalf("a reader's %s holds the working version: %s", format, got)
		}
		if !strings.Contains(got, "approved text") {
			t.Fatalf("a reader's %s lacks the approved version: %s", format, got)
		}
		if format != "markdown" && !strings.Contains(got, "Controlled document. State: Approved. Version") {
			t.Fatalf("a reader's %s has no control block: %s", format, got)
		}
		if format == "html" && strings.Contains(got, "Old versions") {
			t.Fatalf("a reader's HTML holds old versions: %s", got)
		}
		mine := exported(w.author, w, page, format, &form, map[string]any{"attachments": true})
		if !strings.Contains(mine, "workingexport") {
			t.Fatalf("the author's %s lacks the working version: %s", format, mine)
		}
		if format != "markdown" && !strings.Contains(mine, notApprovedLine) {
			t.Fatalf("the author's %s does not say it is not approved: %s", format, mine)
		}
	}
	// The header of a PDF is on every page of it.
	exported(w.author, w, page, "pdf", &form, nil)
	if !strings.Contains(form, `filename="header.html"`) || !strings.Contains(form, "Not approved") {
		t.Fatalf("the PDF of a working version has no header saying so: %s", form)
	}
	exported(w.reader, w, page, "pdf", &form, nil)
	if strings.Contains(form, `filename="header.html"`) {
		t.Fatalf("a reader's PDF says not approved: %s", form)
	}
	if code, body := w.reader.do("GET", "/api/v1/pages/"+page+"/export.md", nil); code != 200 || strings.Contains(body, "workingexport") {
		t.Fatalf("a reader's Markdown: %d %s", code, body)
	}
}

const notApprovedLine = "Not approved. State:"

// TestABundleKeepsTheRecordAndShowsReadersOnlyWhatWasApproved is DOC-532: a bundle carries the
// workflow, where each page stands and its rounds with their answers; an import keeps that
// record, marked as imported, and readers of the new space see only approved versions.
func TestABundleKeepsTheRecordAndShowsReadersOnlyWhatWasApproved(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCB")
	root := e.user("dcb-root@example.com", identity.AllGlobalPermissions...)
	page := w.workingPage("Bundled procedure", "approved bundled text", "Bundled procedure", "working bundled text")
	never := w.controlled("Never bundled", "never approved text")
	data := bundleOf(t, w.admin, w.space)
	if !strings.Contains(zipNames(t, data)["control.json"], page) {
		t.Fatal("the bundle carries no record of document control")
	}
	status, got := root.post("/api/v1/admin/bundles?filename=b.zip", data)
	if status != 202 {
		t.Fatalf("%d %v", status, got)
	}
	e.runJobs()
	id := got["id"].(string)
	root.ok("PUT", "/api/v1/imports/"+id+"/mapping", map[string]any{"space_key": "DCBCOPY"})
	if done := root.carryOut(id); done["state"] != "done" {
		t.Fatalf("%+v", done)
	}
	copyOf := func(title string) string {
		return e.scalar(`SELECT p.id::text FROM pages p JOIN spaces s ON s.id = p.space_id WHERE s.key = 'DCBCOPY' AND p.title = $1`, title)
	}
	copied, copiedNever := copyOf("Bundled procedure"), copyOf("Never bundled")
	if copied == page || copiedNever == never {
		t.Fatal("the import kept the ids of pages that are here")
	}
	got2 := w.reader.ok("GET", "/api/v1/pages/"+copied, nil)
	if !strings.Contains(fmt.Sprint(got2["document"]), "approved bundled text") {
		t.Fatalf("a reader of the copy sees %v", got2["document"])
	}
	if found, _ := searchFinds(w.reader, copied, "working"); found {
		t.Fatal("a reader of the copy finds the working version")
	}
	w.reader.want(404, "GET", "/api/v1/pages/"+copiedNever, nil)
	if c := w.control(w.author, copied); c["head_version"].(float64) != 2 || c["approved_version"].(float64) != 1 {
		t.Fatalf("the copy's band: %v", c)
	}
	if n := e.count("SELECT count(*) FROM workflow_rounds WHERE page_id = $1 AND imported_from IS NOT NULL", copied); n < 2 {
		t.Fatalf("imported rounds: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM workflow_answers a JOIN workflow_rounds r ON r.id = a.round_id
		WHERE r.page_id = $1 AND r.imported_from IS NOT NULL AND a.transition_id = 'approve'`, copied); n != 1 {
		t.Fatalf("the approval of the bundled page is not on record: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM workflows w JOIN spaces s ON s.id = w.space_id WHERE s.key = 'DCBCOPY'`); n != 1 {
		t.Fatalf("workflows in the copy: %d", n)
	}
}

// TestTheAPIGivesEachCallerTheVersionTheyMaySee is DOC-550 and DOC-551: through a token a reader
// gets the approved version and its number, and nothing of the working one; an author gets the
// working version and both numbers. A token takes steps, answers a review and reads its record
// with the same checks as the screens. Signing comes with wave DC4, which refuses it to a token.
func TestTheAPIGivesEachCallerTheVersionTheyMaySee(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DCK")
	flow := w.workflowOf("Any one", reviewedDef(review("any", 0, entry("user", w.r1.id))))
	page := w.author.page(w.space, w.home, "Through the API", "approved api text")
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": flow})
	w.step(page, "submit")
	if code, out := w.answer(w.r1, page, "approve"); code != 200 {
		t.Fatalf("approve: %d %s", code, out)
	}
	w.publishAs(w.author, page, "Through the API", "working api text", false)
	token := func(u *user) string {
		return u.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "script", "scopes": []string{"read", "write"}})["secret"].(string)
	}
	readerToken, authorToken, reviewerToken := token(w.reader), token(w.author), token(w.r1)

	code, out := e.bearer("GET", "/api/v1/pages/"+page, readerToken, "")
	if code != 200 || strings.Contains(out, "working api text") || !strings.Contains(out, "approved api text") {
		t.Fatalf("a reader's token reads %d %s", code, out)
	}
	read := decode(t, out)
	if c := read["control"].(map[string]any); c["head_version"].(float64) != 1 || c["approved_version"].(float64) != 1 || c["shows_working"] != false {
		t.Fatalf("a reader's token learns of the working version: %v", c)
	}
	code, out = e.bearer("GET", "/api/v1/pages/"+page, authorToken, "")
	if code != 200 || !strings.Contains(out, "working api text") {
		t.Fatalf("an author's token reads %d %s", code, out)
	}
	if c := decode(t, out)["control"].(map[string]any); c["head_version"].(float64) != 2 || c["approved_version"].(float64) != 1 {
		t.Fatalf("an author's token: %v", c)
	}
	if code, _ := e.bearer("POST", "/api/v1/pages/"+page+"/transitions", readerToken, `{"transition":"submit","version":2}`); code != 403 && code != 404 {
		t.Fatalf("a reader's token took a step: %d", code)
	}
	if code, out := e.bearer("POST", "/api/v1/pages/"+page+"/transitions", authorToken, `{"transition":"submit","version":2}`); code != 200 {
		t.Fatalf("an author's token could not take a step: %d %s", code, out)
	}
	if code, out := e.bearer("GET", "/api/v1/pages/"+page+"/review", reviewerToken, ""); code != 200 || !strings.Contains(out, "working api text") {
		t.Fatalf("a reviewer's token reads the review: %d %s", code, out)
	}
	if code, _ := e.bearer("GET", "/api/v1/pages/"+page+"/review", readerToken, ""); code != 403 && code != 404 {
		t.Fatalf("a reader's token read the review: %d", code)
	}
	if code, out := e.bearer("POST", "/api/v1/pages/"+page+"/review/answer", reviewerToken,
		`{"answer":"approve","comment":"fine","version":2}`); code != 200 {
		t.Fatalf("a reviewer's token could not answer: %d %s", code, out)
	}
	if code, out := e.bearer("GET", "/api/v1/pages/"+page, readerToken, ""); code != 200 || !strings.Contains(out, "working api text") {
		t.Fatalf("after the answer a reader's token reads %d %s", code, out)
	}
}

// TestWebhooksTellOfWorkingChangesOnlyToThoseWhoMayEdit is DOC-552: an endpoint of someone who
// may only read hears that a new version was approved, and never that a working version
// changed; an endpoint of an author hears both.
func TestWebhooksTellOfWorkingChangesOnlyToThoseWhoMayEdit(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) { c.OutwardAllowPrivate = true })
	w := newControlWorld(e, "DCW")
	r := newReceiver(t)
	root := e.user("dcw-root@example.com", identity.AllGlobalPermissions...)
	root.ok("PUT", "/api/v1/admin/outgoing-hosts", map[string]any{"hosts": []string{"127.0.0.1"}})
	events := []string{"page.version_approved", "page.working_changed", "page.published"}
	w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/webhooks", map[string]any{"address": r.URL + "/authors", "events": events})
	readers := w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/webhooks", map[string]any{"address": r.URL + "/readers", "events": events})
	e.sql("UPDATE webhook_endpoints SET owner_id = $1 WHERE id = $2", w.reader.id, readers["webhook"].(map[string]any)["id"])
	page := w.workingPage("Hooked", "approved", "Hooked", "working")
	e.sendHooks()
	heard := func(path, event string) int {
		n := 0
		for i, req := range r.requests {
			if req.URL.Path == path && strings.Contains(r.bodies[i], `"type":"`+event+`"`) && strings.Contains(r.bodies[i], page) {
				n++
			}
		}
		return n
	}
	if n := heard("/readers", "page.working_changed"); n != 0 {
		t.Fatalf("a reader's endpoint heard of a working change %d times", n)
	}
	if n := heard("/authors", "page.working_changed"); n != 1 {
		t.Fatalf("an author's endpoint heard of the working change %d times", n)
	}
	w.approve(page)
	e.sendHooks()
	for _, path := range []string{"/readers", "/authors"} {
		if n := heard(path, "page.version_approved"); n != 2 {
			t.Fatalf("%s heard of %d approvals, want 2", path, n)
		}
	}
}

// notificationsAbout lists the types of a person's notifications about a page.
func notificationsAbout(u *user, page string) []string {
	u.e.t.Helper()
	out := []string{}
	for _, n := range u.notifications() {
		if n["page_id"] == page {
			out = append(out, n["type"].(string))
		}
	}
	return out
}

func mailAt(u *user, types ...string) {
	u.e.t.Helper()
	settings := u.ok("GET", "/api/v1/me/notification-settings", nil)
	for _, kind := range types {
		settings["channels"].(map[string]any)[kind] = "app_mail"
	}
	u.ok("PUT", "/api/v1/me/notification-settings", settings)
}

// sendMail runs the mail queue now and gives what left, per address.
func sendMail(e *env) map[string]string {
	e.t.Helper()
	f := &fakeSender{}
	queue := e.mailer(f)
	e.dueNow()
	if _, err := queue.RunOnce(context.Background()); err != nil {
		e.t.Fatal(err)
	}
	out := map[string]string{}
	for _, m := range f.sent {
		out[m.To] += m.Subject + "\n" + m.Text + "\n" + m.HTML
	}
	return out
}

// TestWatchersWhoAreReadersHearOnlyOfApprovedVersions is DOC-603 and DOC-604: a reader who
// watches hears of a new approved version, with the change since the version approved before,
// and never of a working change; an author who watches hears of both, until they lose the
// right to edit, which takes the working notices away at once.
func TestWatchersWhoAreReadersHearOnlyOfApprovedVersions(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCQ")
	root := e.user("dcq-root@example.com", identity.AllGlobalPermissions...)
	values := root.ok("GET", "/api/v1/admin/settings", nil)
	values["mail_excerpts"], values["mail_enabled"] = true, true
	root.ok("PUT", "/api/v1/admin/settings", values)
	second := e.user("dcq-second@example.com", identity.PermUseProduct)
	w.admin.grant(w.space, "user", second.id, permissions.TemplateEditor)
	page := w.controlled("Watched", "approved first words")
	w.approve(page)
	e.deliver()
	for _, u := range []*user{w.reader, second} {
		u.ok("PUT", "/api/v1/pages/"+page+"/watch", nil)
		mailAt(u, "page_changed", "version_approved", "mention")
	}
	e.deliver()
	sendMail(e)

	w.publishAs(w.author, page, "Watched", "never approved middle words", false)
	if got := notificationsAbout(w.reader, page); len(got) != 0 {
		t.Fatalf("a reader who watches heard of a working change: %v", got)
	}
	if got := notificationsAbout(second, page); len(got) != 1 || got[0] != "page_changed" {
		t.Fatalf("an author who watches: %v", got)
	}
	w.publishAs(w.author, page, "Watched", "approved last words", false)
	w.approve(page)
	w.publishAs(w.author, page, "Watched", "working after words", false)
	e.deliver()
	mails := sendMail(e)
	reader := mails[w.reader.email]
	if !strings.Contains(reader, "was approved") || !strings.Contains(reader, "{+last+}") {
		t.Fatalf("the reader's mail misses the approval: %s", reader)
	}
	for _, leak := range []string{"never approved middle words", "working after words"} {
		if strings.Contains(reader, leak) {
			t.Fatalf("the reader's mail holds a version nobody approved (%s): %s", leak, reader)
		}
	}
	if got := notificationsAbout(w.reader, page); len(got) != 1 || got[0] != "version_approved" {
		t.Fatalf("the reader's notifications: %v", got)
	}

	w.admin.grant(w.space, "user", second.id, permissions.TemplateViewer)
	for _, kind := range notificationsAbout(second, page) {
		if kind == "page_changed" {
			t.Fatal("a watcher who lost the right to edit still sees a working change")
		}
	}
}

// TestAMentionInAWorkingVersionWaitsForTheApproval is DOC-606: an author mentioned in a working
// version hears of it at once; a reader hears when it is approved, once.
func TestAMentionInAWorkingVersionWaitsForTheApproval(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCU")
	second := e.user("dcu-second@example.com", identity.PermUseProduct)
	w.admin.grant(w.space, "user", second.id, permissions.TemplateEditor)
	page := w.controlled("Mentions", "approved")
	w.approve(page)
	w.author.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Mentions", "base_version": w.head(page),
		"document": commentBody("ask ", mention(w.reader.id, "Reader"), mention(second.id, "Second"))})
	if got := notificationsAbout(w.reader, page); len(got) != 0 {
		t.Fatalf("a reader heard of a mention in a working version: %v", got)
	}
	if got := notificationsAbout(second, page); len(got) != 1 || got[0] != "mention" {
		t.Fatalf("an author mentioned in a working version: %v", got)
	}
	w.approve(page)
	if got := notificationsAbout(w.reader, page); len(got) != 1 || got[0] != "mention" {
		t.Fatalf("after approval the reader: %v", got)
	}
	// The list groups a page's mentions on one line, so the line counts them.
	for _, n := range second.notifications() {
		if n["page_id"] == page && n["count"].(float64) != 1 {
			t.Fatalf("the author heard of the mention again: %v", n)
		}
	}
}

// TestEveryNotificationKindForAReaderHoldsNoWorkingText is DOC-604: whatever a row about a
// controlled page says, the mail built for a reader holds no word of a working version.
func TestEveryNotificationKindForAReaderHoldsNoWorkingText(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCV")
	root := e.user("dcv-root@example.com", identity.AllGlobalPermissions...)
	values := root.ok("GET", "/api/v1/admin/settings", nil)
	values["mail_excerpts"], values["mail_enabled"] = true, true
	root.ok("PUT", "/api/v1/admin/settings", values)
	page := w.workingPage("Rendered", "approved rendered", "Rendered", "secret working rendered")
	comment := e.scalar(`INSERT INTO comments (id, origin_instance_id, space_id, page_id, author_id, body, editors_only)
		VALUES (gen_random_uuid(), gen_random_uuid(), $1, $2, $3, $4, true) RETURNING id::text`,
		w.space, page, w.author.id, `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"secret comment"}]}]}`)
	var ids []uuid.UUID
	for _, kind := range notify.Types {
		var id uuid.UUID
		if err := e.app.Pool().QueryRow(context.Background(), `INSERT INTO notifications (id, user_id, type, reason, page_id, space_id, comment_id,
				group_key, source_event, data) VALUES (gen_random_uuid(), $1, $2, 'watching', $3::uuid, $4, $5, $2::text || ':' || $3::text, gen_random_uuid(),
				'{"version": 2, "previous": 1}') RETURNING id`, w.reader.id, kind, page, w.space, comment).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rendered, err := e.app.Notify().Render(context.Background(), mail.Job{UserID: uuid.MustParse(w.reader.id), Kind: mail.KindDigest, NotificationIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	all := rendered.Message.Subject + rendered.Message.Text + rendered.Message.HTML
	if rendered.Drop || !strings.Contains(all, "Rendered") {
		t.Fatalf("the digest was not written: %+v", rendered)
	}
	for _, leak := range []string{"secret working", "secret comment"} {
		if strings.Contains(all, leak) {
			t.Fatalf("a mail to a reader holds %q: %s", leak, all)
		}
	}
}

// TestADigestHoldsOneLinePerPageWithTheNewestApprovedVersion is DOC-605.
func TestADigestHoldsOneLinePerPageWithTheNewestApprovedVersion(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCY")
	page := w.controlled("Digested", "one")
	w.approve(page)
	e.deliver()
	w.reader.ok("PUT", "/api/v1/pages/"+page+"/watch", nil)
	for _, text := range []string{"two", "three"} {
		w.publishAs(w.author, page, "Digested", text, false)
		w.approve(page)
	}
	e.deliver()
	lines := 0
	for _, n := range w.reader.notifications() {
		if n["page_id"] == page {
			lines++
			if n["type"] != "version_approved" || n["count"].(float64) != 2 || n["data"].(map[string]any)["version"].(float64) != 3 {
				t.Fatalf("the line of the digest: %v", n)
			}
		}
	}
	if lines != 1 {
		t.Fatalf("%d lines for one page", lines)
	}
}

// TestTheApprovalsDigestTellsOfEveryApprovalInASpace is DOC-607 and DOC-609: a person who asks
// hears of every approval in the space, on pages they may see, through the usual channels.
func TestTheApprovalsDigestTellsOfEveryApprovalInASpace(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCZ")
	stranger := e.user("dcz-stranger@example.com", identity.PermUseProduct)
	stranger.want(404, "PUT", "/api/v1/spaces/"+w.space+"/approvals-digest", nil)
	if got := w.reader.ok("PUT", "/api/v1/spaces/"+w.space+"/approvals-digest", nil); got["on"] != true {
		t.Fatalf("%v", got)
	}
	if got := w.reader.ok("GET", "/api/v1/spaces/"+w.space+"/approvals-digest", nil); got["on"] != true {
		t.Fatalf("%v", got)
	}
	open := w.controlled("Open procedure", "x")
	closed := w.controlled("Closed procedure", "y")
	w.admin.restrict(closed, "view", w.admin.id, w.author.id)
	w.approve(open)
	w.approve(closed)
	if got := notificationsAbout(w.reader, open); len(got) != 1 || got[0] != "version_approved" {
		t.Fatalf("the approvals digest: %v", got)
	}
	if got := notificationsAbout(w.reader, closed); len(got) != 0 {
		t.Fatalf("an approval of a page the reader may not see: %v", got)
	}
	if e.auditActions("notify.approvals_digest_started") != 1 {
		t.Fatal("asking for the digest is not audited")
	}
	w.reader.ok("DELETE", "/api/v1/spaces/"+w.space+"/approvals-digest", nil)
	w.publishAs(w.author, open, "Open procedure", "z", false)
	w.approve(open)
	if got := notificationsAbout(w.reader, open); len(got) != 1 {
		t.Fatalf("after stopping the digest: %v", got)
	}
}

// liveHolds says whether any file of the live site holds the words.
func (s *siteEnv) liveHolds(words string) bool {
	s.t.Helper()
	var target string
	if err := s.app.Pool().QueryRow(context.Background(), `SELECT target FROM public_sites WHERE id = $1`, s.site).Scan(&target); err != nil {
		s.t.Fatal(err)
	}
	found := false
	// The live folder is a link to the build, which a walk only enters with a separator after it.
	_ = filepath.WalkDir(filepath.Join(s.dir, target, "live")+string(os.PathSeparator), func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if data, _ := os.ReadFile(path); strings.Contains(string(data), words) {
				found = true
			}
		}
		return nil
	})
	return found
}

// TestAPublicSiteNeverGetsAWorkingVersion is DOC-540: a site shows the approved version of a
// controlled page with its pinned files; a working version never reaches it, a page never
// approved is not published, and a new approval goes out with the next build.
func TestAPublicSiteNeverGetsAWorkingVersion(t *testing.T) {
	s := siteSetup(t)
	flow := s.owner.ok("POST", "/api/v1/spaces/"+s.space+"/workflows", map[string]any{"name": "Review", "definition": reviewFlow()})["id"].(string)
	page := s.owner.page(s.space, s.home, "Public procedure", "approved public words")
	file := s.owner.file(page, "form.txt", "approved public form")
	s.owner.republish(page, "Public procedure", fileTile(file, "form.txt", "approved public words"))
	s.owner.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": flow})
	never := s.owner.page(s.space, s.home, "Never public", "never approved words")
	s.owner.ok("PUT", "/api/v1/pages/"+never+"/workflow", map[string]any{"workflow_id": flow})
	step := func(transition string) {
		head := s.owner.ok("GET", "/api/v1/pages/"+page, nil)["control"].(map[string]any)["head_version"]
		s.owner.ok("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": transition, "version": head})
	}
	step("submit")
	step("approve")
	s.owner.want(409, "PUT", "/api/v1/sites/"+s.site+"/pages", map[string]any{"page_ids": []string{page, never}})
	s.choose(page)
	head := s.owner.ok("GET", "/api/v1/pages/"+page, nil)["control"].(map[string]any)["head_version"]
	s.owner.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Public procedure", "base_version": head,
		"document": fileTile(file, "form.txt", "working public words")})
	s.owner.file(page, "form.txt", "working public form")
	s.deliver()
	s.publish()
	if !s.liveHolds("approved public words") || !s.liveHolds("approved public form") {
		t.Fatalf("the approved version is not on the site: %v %v %s", s.liveHolds("approved public words"), s.liveHolds("approved public form"), s.live("public-procedure.html"))
	}
	for _, leak := range []string{"working public words", "working public form", "never approved words"} {
		if s.liveHolds(leak) {
			t.Fatalf("the site holds %q", leak)
		}
	}
	step("submit")
	step("approve")
	s.deliver()
	s.publish()
	if !s.liveHolds("working public words") || !s.liveHolds("working public form") {
		t.Fatal("the new approval did not reach the site")
	}
}

// TestAPageMadeFromATemplateEntersTheWorkflowItNames is DOC-560: a template of a space may name
// a workflow, and a page made from it enters it in its start state; a space workflow wins. A
// template of all spaces names none, and a workflow of another space is refused.
func TestAPageMadeFromATemplateEntersTheWorkflowItNames(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCL")
	root := e.user("dcl-root@example.com", identity.AllGlobalPermissions...)
	other := newControlWorld(e, "DCL2")
	w.admin.want(400, "POST", "/api/v1/spaces/"+w.space+"/templates", map[string]any{"name": "Wrong", "workflow_id": other.workflow})
	root.want(400, "POST", "/api/v1/admin/templates", map[string]any{"name": "Everywhere", "workflow_id": w.workflow})
	made := w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/templates", map[string]any{"name": "Procedure", "workflow_id": w.workflow,
		"document": para("procedure text")})
	if made["workflow_id"] != w.workflow {
		t.Fatalf("the template lost its workflow: %v", made)
	}
	page := w.author.ok("POST", "/api/v1/templates/"+made["id"].(string)+"/use", map[string]any{"space_id": w.space, "parent_id": w.home,
		"title": "From the template"})["page"].(map[string]any)["id"].(string)
	w.author.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "From the template", "document": para("procedure text"), "base_version": 0})
	c := w.control(w.author, page)
	if c == nil || c["workflow_id"] != w.workflow || c["state_id"] != "draft" || c["governed_by"] != "page" {
		t.Fatalf("the page did not enter the template's workflow: %v", c)
	}
	w.reader.want(404, "GET", "/api/v1/pages/"+page, nil)

	space := w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": "Space flow", "definition": reviewFlow()})["id"].(string)
	w.admin.ok("PUT", "/api/v1/spaces/"+w.space+"/workflow", map[string]any{"workflow_id": space, "kinds": []string{"page"}})
	second := w.author.ok("POST", "/api/v1/templates/"+made["id"].(string)+"/use", map[string]any{"space_id": w.space, "parent_id": w.home,
		"title": "Under the space"})["page"].(map[string]any)["id"].(string)
	w.author.ok("POST", "/api/v1/pages/"+second+"/publish", map[string]any{"title": "Under the space", "document": para("x"), "base_version": 0})
	if c := w.control(w.author, second); c["workflow_id"] != space || c["governed_by"] != "space" {
		t.Fatalf("the space workflow did not win: %v", c)
	}
}

// TestTheFeedTellsReadersOfApprovalsAndNeverOfWorkingChanges is DOC-570: a reader's feed has a
// controlled page as approved, at the time of its approval, and nothing of a working change.
func TestTheFeedTellsReadersOfApprovalsAndNeverOfWorkingChanges(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCE")
	page := w.controlled("Fed", "approved")
	w.approve(page)
	feedItem := func(u *user, tab string) map[string]any {
		for _, item := range u.ok("GET", "/api/v1/feed?tab="+tab, nil)["items"].([]any) {
			if it := item.(map[string]any); it["page_id"] == page {
				return it
			}
		}
		return nil
	}
	w.reader.ok("PUT", "/api/v1/pages/"+page+"/watch", nil)
	for _, tab := range []string{"recent", "for_you"} {
		if it := feedItem(w.reader, tab); it == nil || it["kind"] != "approved" {
			t.Fatalf("the %s feed of a reader: %v", tab, it)
		}
	}
	before := feedItem(w.reader, "recent")["at"]
	w.publishAs(w.author, page, "Fed changed", "working words", false)
	for _, tab := range []string{"recent", "for_you"} {
		it := feedItem(w.reader, tab)
		if it == nil || it["kind"] != "approved" || it["title"] != "Fed" || it["at"] != before || strings.Contains(fmt.Sprint(it), "working") {
			t.Fatalf("a working change reached the %s feed of a reader: %v", tab, it)
		}
	}
	w.approve(page)
	if it := feedItem(w.reader, "recent"); it["title"] != "Fed changed" || it["at"] == before {
		t.Fatalf("the new approval is not in the feed: %v", it)
	}
}

// TestAnalyticsCountTheVersionThePersonRead is DOC-571: a reader's read counts the approved
// version, an author's the working one.
func TestAnalyticsCountTheVersionThePersonRead(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCA2")
	root := e.user("dca2-root@example.com", identity.AllGlobalPermissions...)
	root.ok("PUT", "/api/v1/admin/analytics", map[string]any{"enabled": true, "mode": "names", "retention_days": 365})
	page := w.workingPage("Counted", "approved", "Counted", "working")
	second := e.user("dca2-second@example.com", identity.PermUseProduct)
	w.admin.grant(w.space, "user", second.id, permissions.TemplateEditor)
	for _, u := range []*user{w.reader, second} {
		u.ok("POST", "/api/v1/pages/"+page+"/views", map[string]any{})
	}
	if v := e.count("SELECT last_version FROM page_view_people WHERE page_id = $1 AND user_id = $2", page, w.reader.id); v != 1 {
		t.Fatalf("a reader's read counts version %d", v)
	}
	if v := e.count("SELECT last_version FROM page_view_people WHERE page_id = $1 AND user_id = $2", page, second.id); v != 2 {
		t.Fatalf("an author's read counts version %d", v)
	}
}

// TestTheTreeShowsReadersTheApprovedTitle is DOC-572.
func TestTheTreeShowsReadersTheApprovedTitle(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCJ")
	page := w.workingPage("Approved title", "x", "Working title", "y")
	for _, row := range w.reader.ok("GET", "/api/v1/spaces/"+w.space+"/tree", nil)["pages"].([]any) {
		if r := row.(map[string]any); r["id"] == page && r["title"] != "Approved title" {
			t.Fatalf("the tree shows a reader %v", r["title"])
		}
	}
	if got := fmt.Sprint(w.reader.ok("GET", "/api/v1/spaces/"+w.space+"/tree", nil)); strings.Contains(got, "Working title") {
		t.Fatalf("the tree shows a reader the working title: %s", got)
	}
}
