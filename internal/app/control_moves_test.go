// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Document control, wave DC5 of spec 064: moves, copies and imports (DOC-1200 to DOC-1213),
// exempt labels (DOC-402), conditions (DOC-120), the reason in the audit (DOC-1001) and the
// move from today's approvals (DOC-1100 to DOC-1107).

// TestAMoveToAnotherSpaceTakesItsRule is DOC-1200 and DOC-1201: into a controlled space the
// page starts again and readers there see nothing until it is approved; into a space without
// control it is an ordinary page at its approved version; a page workflow of the old space
// never follows the page; within a space the record stays.
func TestAMoveToAnotherSpaceTakesItsRule(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DMA")
	target := newControlWorld(e, "DMB")
	plain := newControlWorld(e, "DMC")
	for _, other := range []controlWorld{target, plain} {
		other.admin.grant(other.space, "user", w.author.id, permissions.TemplateEditor)
		other.admin.grant(other.space, "user", w.reader.id, permissions.TemplateViewer)
	}
	target.admin.ok("PUT", "/api/v1/spaces/"+target.space+"/workflow", map[string]any{"workflow_id": target.workflow, "kinds": []string{"page"}})
	w.admin.grant(w.space, "user", w.author.id, permissions.TemplateSpaceAdmin)

	moved := w.controlled("Moving procedure", "approved there")
	w.approve(moved)
	w.author.ok("POST", "/api/v1/pages/"+moved+"/move", map[string]any{"space_id": target.space, "parent_id": target.home})
	c := w.control(w.author, moved)
	if c["workflow_id"] != target.workflow || c["approved_version"] != nil || c["state_id"] != "draft" {
		t.Fatalf("the moved page kept the approval of another procedure: %v", c)
	}
	if code, _ := w.reader.do("GET", "/api/v1/pages/"+moved, nil); code != 404 {
		t.Fatalf("a reader of the target sees a page its procedure never approved: %d", code)
	}

	out := w.controlled("Leaving control", "approved text")
	w.approve(out)
	w.publishAs(w.author, out, "Leaving control", "working text", false)
	w.author.ok("POST", "/api/v1/pages/"+out+"/move", map[string]any{"space_id": plain.space, "parent_id": plain.home})
	if c := w.control(w.author, out); c != nil {
		t.Fatalf("a page workflow of the old space followed the page: %v", c)
	}
	if got := w.reader.ok("GET", "/api/v1/pages/"+out, nil); !strings.Contains(fmt.Sprint(got["document"]), "approved text") {
		t.Fatalf("a reader of a space without control sees %v", got["document"])
	}

	within := w.controlled("Staying", "text")
	w.approve(within)
	w.author.ok("POST", "/api/v1/pages/"+within+"/move", map[string]any{"parent_id": nil})
	if c := w.control(w.author, within); c == nil || c["state_id"] != "approved" {
		t.Fatalf("a move within the space changed the record: %v", c)
	}
}

// TestACopyCarriesNoApprovalOrSignature is DOC-1202: a copy is a new page without approvals,
// rounds or signatures, and a reader's copy holds the approved text.
func TestACopyCarriesNoApprovalOrSignature(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "DMD")
	page := w.signedPage("Signed original", "approved words")
	w.signApproval(page)
	w.publishAs(w.author, page, "Signed original", "working words", false)
	w.admin.grant(w.space, "user", w.reader.id, permissions.TemplateEditor)
	copied := w.reader.ok("POST", "/api/v1/pages/"+page+"/copy", map[string]any{})["id"].(string)
	if e.count("SELECT count(*) FROM signatures WHERE page_id = $1", copied) != 0 ||
		e.count("SELECT count(*) FROM workflow_rounds WHERE page_id = $1 AND status = 'approved'", copied) != 0 {
		t.Fatal("a copy carried a signature or an approval")
	}
	got := w.reader.ok("GET", "/api/v1/pages/"+copied, nil)
	if !strings.Contains(fmt.Sprint(got["document"]), "approved words") || strings.Contains(fmt.Sprint(got["document"]), "working words") {
		t.Fatalf("a reader's copy holds %v", got["document"])
	}
	if c := w.control(w.author, copied); c != nil && c["approved_version"] != nil {
		t.Fatalf("the copy is approved: %v", c)
	}
}

// TestDeleteRestoreAndArchiveKeepTheRecord is DOC-1203 and DOC-1204.
func TestDeleteRestoreAndArchiveKeepTheRecord(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DME")
	w.admin.grant(w.space, "user", w.author.id, permissions.TemplateSpaceAdmin)
	page := w.controlled("Kept", "text")
	w.approve(page)
	w.author.ok("POST", "/api/v1/pages/"+page+"/archive", map[string]any{})
	if code, _ := w.reader.do("GET", "/api/v1/pages/"+page, nil); code == 200 {
		if got := w.reader.ok("GET", "/api/v1/pages/"+page, nil); got["archived_at"] == nil {
			t.Fatal("an archived page is shown to a reader as current")
		}
	}
	w.author.ok("POST", "/api/v1/pages/"+page+"/unarchive", map[string]any{})
	if w.state(page) != "approved" {
		t.Fatal("unarchiving changed the state")
	}
	w.author.want(204, "DELETE", "/api/v1/pages/"+page, nil)
	item := e.scalar("SELECT id::text FROM trash_items WHERE object_id = $1", page)
	w.author.ok("POST", "/api/v1/trash/"+item+"/restore", map[string]any{})
	if w.state(page) != "approved" || e.count("SELECT count(*) FROM workflow_answers WHERE page_id = $1", page) < 2 {
		t.Fatal("a restore from the trash lost the state or the record")
	}
}

// TestAnImportNeverBringsReadersToAWorkingVersionNorForeignReviewers is DOC-1212 and DOC-1213
// with the reviewer ids of DC3: a bundle whose readers' version is not the approved one is
// refused, the workflows it brings name no reviewers of another installation, and an import of
// Markdown into a controlled space makes working versions.
func TestAnImportNeverBringsReadersToAWorkingVersionNorForeignReviewers(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DMF")
	root := w.root
	flow := dueDef(review("any", 0, entry("user", w.r1.id)), 3, 0, "", entry("group", w.group))
	flow["states"].([]any)[2].(map[string]any)["read_and_understood"] = map[string]any{"groups": []any{w.group}}
	id := w.workflowOf("Named", flow)
	page := w.reviewedPage(id, "Imported procedure")
	w.answer(w.r1, page, "approve")
	w.publishAs(w.author, page, "Imported procedure", "working text", false)

	importAs := func(key string, data []byte) map[string]any {
		status, got := root.post("/api/v1/admin/bundles?filename=b.zip", data)
		if status != 202 {
			t.Fatalf("%d %v", status, got)
		}
		e.runJobs()
		run := got["id"].(string)
		root.ok("PUT", "/api/v1/imports/"+run+"/mapping", map[string]any{"space_key": key})
		return root.carryOut(run)
	}
	if done := importAs("DMFCOPY", bundleOf(t, w.admin, w.space)); done["state"] != "done" {
		t.Fatalf("%+v", done)
	}
	def := e.scalar(`SELECT v.definition::text FROM workflow_versions v JOIN workflows f ON f.id = v.workflow_id
		JOIN spaces s ON s.id = f.space_id WHERE s.key = 'DMFCOPY'`)
	for _, foreign := range []string{w.r1.id, w.group} {
		if strings.Contains(def, foreign) {
			t.Fatalf("an imported workflow names someone of the bundle's installation: %s", def)
		}
	}

	// A bundle whose page shows its readers the working version is refused.
	e.sql("UPDATE pages SET current_version = (SELECT head_version FROM page_control WHERE page_id = $1) WHERE id = $1", page)
	if done := importAs("DMFBAD", bundleOf(t, w.admin, w.space)); done["state"] == "done" {
		copied := e.scalar(`SELECT p.id::text FROM pages p JOIN spaces s ON s.id = p.space_id WHERE s.key = 'DMFBAD' AND p.title = 'Imported procedure'`)
		if got := w.reader.ok("GET", "/api/v1/pages/"+copied, nil); strings.Contains(fmt.Sprint(got["document"]), "working text") {
			t.Fatal("an import showed readers a working version")
		}
	}

	t.Run("an import into a controlled space makes working versions", func(t *testing.T) {
		w.admin.ok("PUT", "/api/v1/spaces/"+w.space+"/workflow", map[string]any{"workflow_id": w.workflow, "kinds": []string{"page"}})
		before := e.count("SELECT count(*) FROM pages WHERE space_id = $1", w.space)
		w.admin.importFile(w.space, "Imported note.md", []byte("# Imported note\n\nimported words\n"))
		if e.count("SELECT count(*) FROM pages WHERE space_id = $1", w.space) != before+1 {
			t.Fatal("nothing was imported")
		}
		note := e.scalar("SELECT id::text FROM pages WHERE space_id = $1 AND title = 'Imported note'", w.space)
		if code, _ := w.reader.do("GET", "/api/v1/pages/"+note, nil); code != 404 {
			t.Fatalf("a reader sees an imported page that was never approved: %d", code)
		}
	})
}

// TestImportedSignaturesVerifyAndNeverApprove is DOC-1212 and DOC-1421.
func TestImportedSignaturesVerifyAndNeverApprove(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "DMG")
	page := w.signedPage("Signed bundled", "signed words")
	w.signApproval(page)
	data := bundleOf(t, w.admin, w.space)
	status, got := w.root.post("/api/v1/admin/bundles?filename=b.zip", data)
	if status != 202 {
		t.Fatalf("%d %v", status, got)
	}
	e.runJobs()
	run := got["id"].(string)
	w.root.ok("PUT", "/api/v1/imports/"+run+"/mapping", map[string]any{"space_key": "DMGCOPY"})
	if done := w.root.carryOut(run); done["state"] != "done" {
		t.Fatalf("%+v", done)
	}
	copied := e.scalar(`SELECT p.id::text FROM pages p JOIN spaces s ON s.id = p.space_id WHERE s.key = 'DMGCOPY' AND p.title = 'Signed bundled'`)
	if e.count("SELECT count(*) FROM signatures WHERE page_id = $1", copied) != 0 {
		t.Fatal("an imported signature became a signature of this installation")
	}
	if e.count("SELECT count(*) FROM imported_signatures WHERE page_id = $1", copied) != 1 {
		t.Fatal("the imported signature is not on record")
	}
	history := w.root.ok("GET", "/api/v1/pages/"+copied+"/imported-signatures", nil)
	list := history["signatures"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["matches"] != true || list[0].(map[string]any)["source_instance"] == nil {
		t.Fatalf("the imported signature does not verify: %v", history)
	}
	// Tampering with the imported version turns its mark.
	e.sql("ALTER TABLE page_versions DISABLE TRIGGER USER")
	e.sql("UPDATE page_versions SET title = 'Changed' WHERE page_id = $1", copied)
	e.sql("ALTER TABLE page_versions ENABLE TRIGGER USER")
	if again := w.root.ok("GET", "/api/v1/pages/"+copied+"/imported-signatures", nil)["signatures"].([]any); again[0].(map[string]any)["matches"] != false {
		t.Fatal("a changed version still verifies")
	}
}

// TestOnlyThoseWhoLookAfterWorkflowsTouchAnExemptLabel is DOC-402: page.edit alone cannot add
// or remove an exempt label; the change is audited; removing it puts the page in the start
// state.
func TestOnlyThoseWhoLookAfterWorkflowsTouchAnExemptLabel(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DMH")
	w.admin.ok("PUT", "/api/v1/spaces/"+w.space+"/workflow", map[string]any{"workflow_id": w.workflow, "kinds": []string{"page"},
		"exempt_labels": []string{"working-notes"}})
	page := w.author.page(w.space, w.home, "Notes", "text")
	if w.state(page) != "draft" {
		t.Fatal("the page is not under the space workflow")
	}
	if code, _ := w.author.do("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "working-notes"}); code != 403 {
		t.Fatalf("an author escaped review with a label: %d", code)
	}
	w.admin.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "working-notes"})
	if c := w.control(w.author, page); c != nil {
		t.Fatalf("an exempt page is still controlled: %v", c)
	}
	if code, _ := w.author.do("DELETE", "/api/v1/pages/"+page+"/labels/working-notes", nil); code != 403 {
		t.Fatalf("an author removed the exempt label: %d", code)
	}
	w.admin.ok("DELETE", "/api/v1/pages/"+page+"/labels/working-notes", nil)
	if w.state(page) != "draft" {
		t.Fatal("removing the exempt label did not put the page in the start state")
	}
	if e.auditActions("page.exempted") != 1 || e.auditActions("page.exemption_ended") != 1 {
		t.Fatal("the exemption is not audited")
	}
}

// TestAStepWithConditionsSaysWhyItCannotBeTaken is DOC-120 and DOC-121: the band names the
// reason, the step is refused with it, through a token too, and it opens once the condition
// holds.
func TestAStepWithConditionsSaysWhyItCannotBeTaken(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DMI")
	d := reviewFlow()
	d["transitions"].([]any)[0].(map[string]any)["conditions"] = map[string]any{"match": "all",
		"tests": []any{map[string]any{"kind": "label_present", "label": "sop"}}}
	id := w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": "Labelled", "definition": d})["id"].(string)
	page := w.author.page(w.space, w.home, "Conditional", "text")
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": id})
	if blocked, _ := w.control(w.author, page)["blocked"].(map[string]any); blocked["submit"] != "label_missing" {
		t.Fatalf("the band does not say why: %v", blocked)
	}
	out := w.author.want(409, "POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "submit", "version": w.head(page)})
	if !strings.Contains(out, "condition_failed") {
		t.Fatal(out)
	}
	secret := w.author.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "script", "scopes": []string{"read", "write"}})["secret"].(string)
	if code, _ := e.bearer("POST", "/api/v1/pages/"+page+"/transitions", secret, fmt.Sprintf(`{"transition":"submit","version":%d}`, w.head(page))); code != 409 {
		t.Fatalf("a token stepped past a condition: %d", code)
	}
	w.author.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "sop"})
	w.step(page, "submit")
	if w.state(page) != "review" {
		t.Fatal("the step did not open when its condition held")
	}
}

// TestAStepThatNeedsAReasonKeepsItInTheAudit is DOC-1001.
func TestAStepThatNeedsAReasonKeepsItInTheAudit(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DMJ")
	page := w.controlled("Reasoned", "text")
	w.step(page, "submit")
	w.author.want(400, "POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "reject", "version": w.head(page)})
	w.author.ok("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "reject", "version": w.head(page), "comment": "the scope is wrong"})
	if e.audited("action = 'page.transition' AND payload::text LIKE '%the scope is wrong%'") != 1 {
		t.Fatal("the reason is not in the audit entry")
	}
}

// TestPruningKeepsTheRecordsOfDocumentControl is DOC-1002: the prune of the audit log stops at
// the oldest record of document control, never at an ordinary page before it.
func TestPruningKeepsTheRecordsOfDocumentControl(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DMN")
	w.author.page(w.space, w.home, "Ordinary first", "text")
	e.seal()
	page := w.controlled("Controlled later", "text")
	w.approve(page)
	e.seal()
	kept, err := pages.OldestControlRecord(context.Background(), e.app.Pool())
	if err != nil || kept == nil {
		t.Fatalf("%v %v", kept, err)
	}
	first := e.scalar("SELECT to_char(min(occurred_at), 'YYYY-MM-DD HH24:MI:SS.US') FROM audit_events WHERE object_id = $1", page)
	if got := kept.UTC().Format("2006-01-02 15:04:05.000000"); got > first {
		t.Fatalf("the prune would remove control entries: keeps from %s, the page's first entry is %s", got, first)
	}
	ordinary := e.scalar("SELECT to_char(min(occurred_at), 'YYYY-MM-DD HH24:MI:SS.US') FROM audit_events")
	if got := kept.UTC().Format("2006-01-02 15:04:05.000000"); got <= ordinary {
		t.Fatalf("the prune keeps everything, also what came before any control: %s", got)
	}
}

// TestTodaysApprovalsMoveToAWorkflowByChoice is DOC-1100 to DOC-1107: today's rule keeps
// working, the generated workflow gives today's outcome, the three landing cases, no
// signature for a carried-over approval, the old rule switched off and kept, and undo before
// the first round, refused after it.
func TestTodaysApprovalsMoveToAWorkflowByChoice(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DMK")
	approvedNow := w.author.page(w.space, w.home, "Approved now", "today's approved text")
	round := w.author.ok("POST", "/api/v1/pages/"+approvedNow+"/approvals", map[string]any{"user_ids": []string{w.r1.id, w.r2.id}})["round_id"].(string)
	w.r1.ok("PUT", "/api/v1/approvals/"+round+"/answer", map[string]any{"answer": "approve"})
	w.r2.ok("PUT", "/api/v1/approvals/"+round+"/answer", map[string]any{"answer": "approve"})
	older := w.author.page(w.space, w.home, "Approved before", "older approved text")
	e.sql(`INSERT INTO approval_rounds (id, page_id, asked_by, title, document, base_version, approved_version, status)
		SELECT gen_random_uuid(), id, created_by, title, '{}', 1, 1, 'approved' FROM pages WHERE id = $1`, older)
	w.author.ok("POST", "/api/v1/pages/"+older+"/publish", map[string]any{"title": "Approved before", "document": para("newer text"), "base_version": 1})
	never := w.author.page(w.space, w.home, "Never approved", "never approved text")
	w.admin.ok("PUT", "/api/v1/spaces/"+w.space+"/approvals", map[string]any{"required": true, "approvers": []string{w.r1.id, w.r2.id}})

	// A space that does not require approval is offered nothing (DOC-1104).
	plain := newControlWorld(e, "DML")
	plain.admin.want(409, "GET", "/api/v1/spaces/"+plain.space+"/workflow/from-approval-rule", nil)

	offer := w.admin.ok("GET", "/api/v1/spaces/"+w.space+"/workflow/from-approval-rule", nil)
	landing := offer["landing"].(map[string]any)
	if landing["approved"].(float64) < 1 || landing["kept"].(float64) != 1 || landing["start"].(float64) < 1 {
		t.Fatalf("the preview: %v", landing)
	}
	if w.author.do2("POST", "/api/v1/spaces/"+w.space+"/workflow/from-approval-rule") != 403 {
		t.Fatal("an author moved the space")
	}
	w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/workflow/from-approval-rule", map[string]any{})
	if w.state(approvedNow) != "approved" || w.control(w.reader, approvedNow)["carried_over"] != true {
		t.Fatalf("the page approved today: %v", w.control(w.reader, approvedNow))
	}
	if got := w.reader.ok("GET", "/api/v1/pages/"+older, nil); !strings.Contains(fmt.Sprint(got["document"]), "older approved text") {
		t.Fatal("readers do not keep the older approved version")
	}
	if code, _ := w.reader.do("GET", "/api/v1/pages/"+never, nil); code != 404 {
		t.Fatalf("a never approved page is shown to readers: %d", code)
	}
	if e.count("SELECT count(*) FROM signatures s JOIN pages p ON p.id = s.page_id WHERE p.space_id = $1", w.space) != 0 {
		t.Fatal("a carried-over approval became a signature")
	}
	rule := w.admin.ok("GET", "/api/v1/spaces/"+w.space+"/approvals", nil)
	if rule["moved_to_workflow"] != true || len(rule["approvers"].([]any)) != 2 {
		t.Fatalf("the old rule is not kept as history: %v", rule)
	}
	// The generated workflow gives today's outcome: all fixed approvers must approve.
	w.publishAs(w.author, approvedNow, "Approved now", "a change", false)
	w.step(approvedNow, w.firstStep(approvedNow))
	w.answer(w.r1, approvedNow, "approve")
	if w.state(approvedNow) == "approved" {
		t.Fatal("one of two fixed approvers approved alone")
	}
	// After the first round, undo is refused (DOC-1107).
	if code, out := w.admin.do("POST", "/api/v1/spaces/"+w.space+"/workflow/from-approval-rule/undo", map[string]any{}); code != 409 {
		t.Fatalf("undo after a round: %d %s", code, out)
	}

	t.Run("undo before the first round brings today's rule back", func(t *testing.T) {
		x := newReviewWorld(e, "DMM")
		x.author.page(x.space, x.home, "Some page", "text")
		x.admin.ok("PUT", "/api/v1/spaces/"+x.space+"/approvals", map[string]any{"required": true, "approvers": []string{x.r1.id}})
		x.admin.ok("POST", "/api/v1/spaces/"+x.space+"/workflow/from-approval-rule", map[string]any{})
		x.admin.ok("POST", "/api/v1/spaces/"+x.space+"/workflow/from-approval-rule/undo", map[string]any{})
		if e.count("SELECT count(*) FROM space_workflows WHERE space_id = $1", x.space) != 0 {
			t.Fatal("undo left the workflow")
		}
		if rule := x.admin.ok("GET", "/api/v1/spaces/"+x.space+"/approvals", nil); rule["required"] != true || rule["moved_to_workflow"] == true {
			t.Fatalf("today's rule did not come back: %v", rule)
		}
	})
}

// firstStep is the one step a page's band offers its author now.
func (w reviewWorld) firstStep(page string) string {
	w.e.t.Helper()
	steps := w.control(w.author, page)["transitions"].([]any)
	if len(steps) == 0 {
		w.e.t.Fatal("no step to take")
	}
	return steps[0].(map[string]any)["id"].(string)
}

func (u *user) do2(method, path string) int {
	u.e.t.Helper()
	code, _ := u.do(method, path, map[string]any{})
	return code
}
