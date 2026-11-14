// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/templates"
)

// Spec 074, wave GT1: sets of pages from one template, all or nothing.

// guided makes a template of a space whose first page holds text.
func (u *user) guided(space, name, text string) string {
	u.e.t.Helper()
	return u.ok("POST", "/api/v1/spaces/"+space+"/templates", map[string]any{"name": name, "document": para(text)})["id"].(string)
}

// entry adds an entry to a template and gives its key.
func (u *user) entry(tpl string, in map[string]any) string {
	u.e.t.Helper()
	return u.ok("POST", "/api/v1/templates/"+tpl+"/entries", in)["key"].(string)
}

// makeSet uses a template for real.
func (u *user) makeSet(tpl string, in map[string]any) (int, map[string]any) {
	u.e.t.Helper()
	status, out := u.do("POST", "/api/v1/templates/"+tpl+"/sets", in)
	m := map[string]any{}
	_ = json.Unmarshal([]byte(out), &m)
	return status, m
}

// refusedWith gives the code and message of an error answer.
func refusedWith(out map[string]any) (code, message string) {
	e, _ := out["error"].(map[string]any)
	return fmt.Sprint(e["code"]), fmt.Sprint(e["message"])
}

func valueOf(source string) map[string]any {
	return map[string]any{"type": "templateValue", "attrs": map[string]any{"source": source}}
}

func textWith(parts ...map[string]any) map[string]any {
	content := []any{}
	for _, p := range parts {
		content = append(content, p)
	}
	return map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": content}}}
}

func words(text string) map[string]any { return map[string]any{"type": "text", "text": text} }

// trace counts everything a set could leave behind in a space.
type trace struct{ pages, drafts, labels, restrictions, files, origins, sets, audits int }

func (e *env) traceOf(space string) trace {
	e.t.Helper()
	return trace{
		pages:  e.count("SELECT count(*) FROM pages WHERE space_id = $1 AND kind <> 'template' AND status <> 'trashed'", space),
		drafts: e.count("SELECT count(*) FROM drafts d JOIN pages p ON p.id = d.page_id WHERE p.space_id = $1 AND p.kind <> 'template'", space),
		labels: e.count("SELECT count(*) FROM page_labels l JOIN pages p ON p.id = l.page_id WHERE p.space_id = $1 AND p.kind <> 'template'", space),
		restrictions: e.count(`SELECT count(*) FROM page_restrictions r JOIN pages p ON p.id = r.page_id
			WHERE p.space_id = $1 AND p.kind <> 'template'`, space),
		files:   e.count("SELECT count(*) FROM attachments a JOIN pages p ON p.id = a.page_id WHERE p.space_id = $1 AND p.kind <> 'template'", space),
		origins: e.count("SELECT count(*) FROM page_template_origin o JOIN pages p ON p.id = o.page_id WHERE p.space_id = $1", space),
		sets:    e.count("SELECT count(*) FROM template_sets WHERE space_id = $1", space),
		audits:  e.auditActions("template.set_created"),
	}
}

// partOff turns guided templates off, keeping every other part on.
func partOff(root *user) {
	root.e.t.Helper()
	root.want(http.StatusNoContent, "PUT", "/api/v1/admin/experimental", map[string]any{"on": []string{"automation", "bundles", "console_restore", "databases"}})
}

func TestEveryRouteOfGuidedTemplatesIsOffWhileThePartIsOff(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	root := admin(e)
	tpl := owner.guided(space, "Onboarding", "Welcome")
	owner.entry(tpl, map[string]any{"name": "Equipment", "parent": "root", "document": para("A laptop")})
	plain := owner.guided(space, "Plain", "Just one page")
	partOff(root)

	routes := []struct{ method, path string }{
		{"GET", "/api/v1/templates/" + tpl + "/entries"},
		{"POST", "/api/v1/templates/" + tpl + "/entries"},
		{"GET", "/api/v1/templates/" + tpl + "/entries/root"},
		{"PATCH", "/api/v1/templates/" + tpl + "/entries/root"},
		{"DELETE", "/api/v1/templates/" + tpl + "/entries/root"},
		{"PUT", "/api/v1/templates/" + tpl + "/publishing"},
		{"GET", "/api/v1/templates/" + tpl + "/versions"},
		{"POST", "/api/v1/templates/" + tpl + "/sets"},
		{"POST", "/api/v1/templates/" + tpl + "/sets/check"},
	}
	for _, r := range routes {
		out := owner.want(http.StatusNotFound, r.method, r.path, map[string]any{"space_id": space})
		if !strings.Contains(out, "experimental_off") {
			t.Errorf("%s %s: %s", r.method, r.path, out)
		}
	}
	// A guided template is not offered, and its use answers that the part is off; a plain
	// one works exactly as before (TPL-050).
	gallery := editor.ok("GET", "/api/v1/spaces/"+space+"/templates", nil)["templates"].([]any)
	names := []string{}
	for _, g := range gallery {
		names = append(names, g.(map[string]any)["name"].(string))
	}
	if has(names, "Onboarding") || !has(names, "Plain") {
		t.Fatalf("gallery while off: %v", names)
	}
	if out := editor.want(http.StatusNotFound, "POST", "/api/v1/templates/"+tpl+"/use", map[string]any{"space_id": space}); !strings.Contains(out, "experimental_off") {
		t.Fatalf("use while off: %s", out)
	}
	editor.ok("POST", "/api/v1/templates/"+plain+"/use", map[string]any{"space_id": space})
	// Managers still list, change and delete it.
	managed := owner.want(200, "GET", "/api/v1/spaces/"+space+"/manage-templates", nil)
	if !strings.Contains(managed, `"name":"Onboarding"`) || !strings.Contains(managed, `"guided":true`) {
		t.Fatalf("managers must see it, marked: %s", managed)
	}
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Onboarding 2"})
	owner.want(http.StatusNoContent, "DELETE", "/api/v1/templates/"+tpl, nil)
	if n := e.count("SELECT count(*) FROM pages WHERE kind = 'template' AND title = 'Equipment'"); n != 0 {
		t.Fatalf("the hidden page of an entry outlived its template: %d", n)
	}
}

func TestGuidedTemplatesStayOffWhileAnOlderReleaseRuns(t *testing.T) {
	e := newInstalledEnv(t, func(*config.Config) {})
	root := admin(e)
	e.sql(`INSERT INTO nodes (id, role, version, max_migration, last_seen_at) VALUES (gen_random_uuid(), 'all', 'older', 57, now())`)
	out := root.want(http.StatusConflict, "PUT", "/api/v1/admin/experimental", map[string]any{"on": []string{"guided_templates"}})
	if !strings.Contains(out, "older_release_running") {
		t.Fatalf("refusal: %s", out)
	}
	// Other parts are not held back by it, and a part already on stays on.
	root.want(http.StatusNoContent, "PUT", "/api/v1/admin/experimental", map[string]any{"on": []string{"databases"}})
	e.sql(`UPDATE nodes SET last_seen_at = now() - interval '1 hour' WHERE version = 'older'`)
	root.want(http.StatusNoContent, "PUT", "/api/v1/admin/experimental", map[string]any{"on": []string{"guided_templates"}})
	e.sql(`UPDATE nodes SET last_seen_at = now() WHERE version = 'older'`)
	root.want(http.StatusNoContent, "PUT", "/api/v1/admin/experimental", map[string]any{"on": []string{"guided_templates", "databases"}})
}

func TestASetIsRefusedWholeWhenTheCreatorLacksARightOnPartOfIt(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	workflow := owner.ok("POST", "/api/v1/spaces/"+space+"/workflows", map[string]any{"name": "Review", "definition": reviewFlow()})["id"].(string)
	tpl := owner.guided(space, "Onboarding", "Welcome")
	owner.entry(tpl, map[string]any{"name": "Manager notes", "parent": "root", "document": para("Private"),
		"restrictions": map[string]any{"view": []any{map[string]any{"type": "user", "id": owner.id}}}})
	owner.entry(tpl, map[string]any{"name": "Plan", "parent": "root", "document": para("Steps"), "workflow_id": workflow})
	before := e.traceOf(space)

	// The editor may create pages and choose workflows, but not manage restrictions.
	status, out := editor.makeSet(tpl, map[string]any{"space_id": space})
	if code, message := refusedWith(out); status != http.StatusForbidden || code != "set_refused" || !strings.Contains(message, "Manager notes") {
		t.Fatalf("a restricted entry without restriction.manage: %d %v", status, out)
	}
	// Someone who lacks both is told both at once (TPL-810).
	both := e.user("both@example.com", identity.PermUseProduct)
	owner.ok("PUT", "/api/v1/spaces/"+space+"/grants", map[string]any{"subject": map[string]any{"type": "user", "id": both.id},
		"permissions": []string{permissions.SpaceView, permissions.PageCreate}})
	status, out = both.makeSet(tpl, map[string]any{"space_id": space})
	_, message := refusedWith(out)
	if status != http.StatusForbidden || !strings.Contains(message, "Manager notes") || !strings.Contains(message, "Plan") {
		t.Fatalf("every missing right must be listed: %d %v", status, out)
	}
	// The check step refuses alike (TPL-811).
	if status, check := both.do("POST", "/api/v1/templates/"+tpl+"/sets/check", map[string]any{"space_id": space}); status != http.StatusForbidden || !strings.Contains(check, "Plan") {
		t.Fatalf("the dry run must refuse alike: %d %s", status, check)
	}
	// A reader without page.create is refused before anything else.
	if status, _ := viewer.makeSet(tpl, map[string]any{"space_id": space}); status != http.StatusForbidden {
		t.Fatalf("a viewer: %d", status)
	}
	if after := e.traceOf(space); after != before {
		t.Fatalf("a refused set left a trace: %+v, was %+v", after, before)
	}
	// The owner holds every right, and gets the restricted page restricted from the start.
	status, made := owner.makeSet(tpl, map[string]any{"space_id": space})
	if status != http.StatusCreated {
		t.Fatalf("owner: %d %v", status, made)
	}
	if got := len(made["pages"].([]any)); got != 3 {
		t.Fatalf("pages: %d", got)
	}
	notes := pageOfEntry(t, made, "Manager notes")
	if n := e.count("SELECT count(*) FROM page_restrictions WHERE page_id = $1 AND kind = 'view'", notes); n == 0 {
		t.Fatal("the restricted entry was made without its restriction")
	}
	if n := e.count("SELECT count(*) FROM page_workflows WHERE page_id = $1", pageOfEntry(t, made, "Plan")); n != 1 {
		t.Fatalf("the workflow of an entry: %d", n)
	}
}

// pageOfEntry finds the page a set made from an entry, by the entry's title.
func pageOfEntry(t *testing.T, made map[string]any, title string) string {
	t.Helper()
	for _, p := range made["pages"].([]any) {
		if row := p.(map[string]any); row["title"] == title {
			return row["id"].(string)
		}
	}
	t.Fatalf("no page %q in %v", title, made["pages"])
	return ""
}

func TestARestrictionNamingADeletedGroupRefusesTheSet(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	root := admin(e)
	group := root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Managers"})["id"].(string)
	tpl := owner.guided(space, "Onboarding", "Welcome")
	owner.entry(tpl, map[string]any{"name": "Manager notes", "parent": "root", "document": para("Private"),
		"restrictions": map[string]any{"view": []any{map[string]any{"type": "group", "id": group}}}})
	root.want(http.StatusNoContent, "DELETE", "/api/v1/admin/groups/"+group, nil)
	before := e.traceOf(space)
	status, out := owner.makeSet(tpl, map[string]any{"space_id": space})
	if _, message := refusedWith(out); status != http.StatusConflict || !strings.Contains(message, "Manager notes") {
		t.Fatalf("a deleted group: %d %v", status, out)
	}
	if after := e.traceOf(space); after != before {
		t.Fatalf("a refused set left a trace: %+v", after)
	}
}

// filesUnder counts the stored files below a folder.
func filesUnder(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

var errInjected = errors.New("injected fault")

func TestAFailureAtAnyStepOfASetLeavesNoTrace(t *testing.T) {
	var dataDir string
	e := newEnvWith(t, func(c *config.Config) { dataDir = c.DataDir })
	owner, _, _, space := e.team("ENG")
	source := owner.page(space, nil, "Source", "with a file")
	owner.upload(source, "plan.txt", "the plan")
	tpl := owner.ok("POST", "/api/v1/pages/"+source+"/save-as-template", map[string]any{"name": "Project"})["id"].(string)
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Project", "labels": []string{"project"}})
	owner.entry(tpl, map[string]any{"name": "Plan", "parent": "root", "document": para("Steps"), "labels": []string{"plan"}})
	owner.entry(tpl, map[string]any{"name": "Notes", "parent": "root", "document": para("Private"),
		"restrictions": map[string]any{"edit": []any{map[string]any{"type": "user", "id": owner.id}}}})
	service := e.app.Templates()
	defer service.SetHooks(templates.SetHooks{})
	before := e.traceOf(space)
	storedBefore := filesUnder(t, dataDir)

	cases := []struct {
		step  string
		entry int
	}{{"page", 0}, {"page", 1}, {"page", 2}, {"file", 0}, {"label", 1}, {"restriction", 2}, {"origin", 0}, {"audit", -1}}
	for _, c := range cases {
		service.SetHooks(templates.SetHooks{Step: func(_ context.Context, step string, entry int) error {
			if step == c.step && entry == c.entry {
				return errInjected
			}
			return nil
		}})
		if status, out := owner.makeSet(tpl, map[string]any{"space_id": space}); status != http.StatusInternalServerError {
			t.Fatalf("fault at %s %d: %d %v", c.step, c.entry, status, out)
		}
		if after := e.traceOf(space); after != before {
			t.Fatalf("fault at %s %d left a trace: %+v, was %+v", c.step, c.entry, after, before)
		}
	}
	// The copies of the file were stored before the rollback; the task of TPL-821 hands
	// them to the storage deletions once they are an hour old.
	if e.count("SELECT count(*) FROM pending_file_copies") == 0 {
		t.Fatal("a copy that rolled back must be remembered")
	}
	e.runTask("collect-pending-copies")
	e.runTask("drain-storage")
	if got := filesUnder(t, dataDir); got == storedBefore {
		t.Fatal("a young copy must wait, in case its transaction is still running")
	}
	e.sql("UPDATE pending_file_copies SET queued_at = now() - interval '2 hours'")
	e.runTask("collect-pending-copies")
	e.runTask("drain-storage")
	if got := filesUnder(t, dataDir); got != storedBefore {
		t.Fatalf("stored files: %d, before %d", got, storedBefore)
	}
	// A set that commits keeps its copy, and the pending row is gone with the commit.
	service.SetHooks(templates.SetHooks{})
	if status, out := owner.makeSet(tpl, map[string]any{"space_id": space}); status != http.StatusCreated {
		t.Fatalf("make: %d %v", status, out)
	}
	if n := e.count("SELECT count(*) FROM pending_file_copies"); n != 0 {
		t.Fatalf("a committed copy is no longer pending: %d", n)
	}
	e.sql("UPDATE pending_file_copies SET queued_at = now() - interval '2 hours'")
	e.runTask("collect-pending-copies")
	e.runTask("drain-storage")
	if got := filesUnder(t, dataDir); got != storedBefore+1 {
		t.Fatalf("the committed copy must stay: %d, before %d", got, storedBefore)
	}
}

func TestASetIsInvisibleFromAnotherConnectionUntilItCommits(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	tpl := owner.guided(space, "Onboarding", "Welcome")
	owner.entry(tpl, map[string]any{"name": "Manager notes", "parent": "root", "document": para("Private"),
		"restrictions": map[string]any{"view": []any{map[string]any{"type": "user", "id": owner.id}}}})
	owner.entry(tpl, map[string]any{"name": "Plan", "parent": "root", "document": para("Steps")})
	service := e.app.Templates()
	defer service.SetHooks(templates.SetHooks{})
	seen := -1
	service.SetHooks(templates.SetHooks{Step: func(ctx context.Context, step string, entry int) error {
		if step == "page" && entry == 2 {
			// Another connection, as a second node or a reader would have.
			return e.app.Pool().QueryRow(ctx, `SELECT count(*) FROM pages WHERE space_id = $1 AND kind <> 'template'
				AND title IN ('Manager notes', 'Plan', 'Onboarding')`, space).Scan(&seen)
		}
		return nil
	}})
	if status, out := owner.makeSet(tpl, map[string]any{"space_id": space, "title": "Onboarding"}); status != http.StatusCreated {
		t.Fatalf("make: %d %v", status, out)
	}
	if seen != 0 {
		t.Fatalf("another connection saw %d pages of a set still being made", seen)
	}
}

func TestABackgroundSetKilledHalfwayLeavesNothingAndCompletesOnRetry(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	tpl := owner.guided(space, "Planning round", "Overview")
	for i := range 26 {
		owner.entry(tpl, map[string]any{"name": fmt.Sprintf("Team %02d", i), "parent": "root", "document": para("Goals")})
	}
	before := e.traceOf(space)
	status, out := owner.makeSet(tpl, map[string]any{"space_id": space, "answers": map[string]string{}})
	if status != http.StatusAccepted || out["job_id"] == nil {
		t.Fatalf("a set of 27 pages runs in the background: %d %v", status, out)
	}
	job := out["job_id"].(string)
	if raw := e.scalar("SELECT payload::text FROM jobs WHERE id = $1", job); !strings.Contains(raw, tpl) {
		t.Fatalf("payload: %s", raw)
	}
	service := e.app.Templates()
	defer service.SetHooks(templates.SetHooks{})
	// The worker stops halfway: its context ends while the set is being made.
	worker, stop := context.WithCancel(context.Background())
	service.SetHooks(templates.SetHooks{Step: func(ctx context.Context, step string, entry int) error {
		if step == "page" && entry == 13 {
			stop()
			return ctx.Err()
		}
		return nil
	}})
	if _, err := e.app.Jobs().RunOnce(worker); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if got := owner.ok("GET", "/api/v1/jobs/"+job, nil); got["status"] != "failed" {
		t.Fatalf("job: %v", got)
	}
	if after := e.traceOf(space); after != before {
		t.Fatalf("a killed set left a trace: %+v", after)
	}
	if e.auditActions("template.set_failed") != 1 {
		t.Fatal("a failed background set is audited after its rollback")
	}
	service.SetHooks(templates.SetHooks{})
	e.sql("UPDATE jobs SET status = 'queued', started_at = NULL, finished_at = NULL WHERE id = $1", job)
	e.runJobs()
	if got := owner.ok("GET", "/api/v1/jobs/"+job, nil); got["status"] != "done" {
		t.Fatalf("retry: %v", got)
	}
	if after := e.traceOf(space); after.pages != before.pages+27 || after.sets != before.sets+1 {
		t.Fatalf("the retry makes one whole set: %+v", after)
	}
	// The answers leave with the job (TPL-1409).
	if raw := e.scalar("SELECT payload::text FROM jobs WHERE id = $1", job); strings.Contains(raw, "answers") {
		t.Fatalf("the job kept the answers: %s", raw)
	}
}

func TestADoubleSubmitMakesOneSet(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	tpl := owner.guided(space, "Onboarding", "Welcome")
	owner.entry(tpl, map[string]any{"name": "Plan", "parent": "root", "document": para("Steps")})
	before := e.traceOf(space)
	body := map[string]any{"space_id": space, "request_key": "form-1"}
	var wg sync.WaitGroup
	results := make([]map[string]any, 2)
	for i := range 2 {
		wg.Go(func() {
			status, out := owner.makeSet(tpl, body)
			if status != http.StatusCreated && status != http.StatusOK {
				t.Errorf("submit %d: %d %v", i, status, out)
			}
			results[i] = out
		})
	}
	wg.Wait()
	if results[0]["set_id"] != results[1]["set_id"] {
		t.Fatalf("two sets: %v %v", results[0]["set_id"], results[1]["set_id"])
	}
	if after := e.traceOf(space); after.pages != before.pages+2 || after.sets != before.sets+1 {
		t.Fatalf("one set: %+v", after)
	}
	// The same answers without the key, or with a new one, make a second set (TPL-123).
	if status, _ := owner.makeSet(tpl, map[string]any{"space_id": space, "request_key": "form-2"}); status != http.StatusCreated {
		t.Fatal("a new form makes a new set")
	}
}

func TestLinksBetweenEntriesResolveInOnePass(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	tpl := owner.guided(space, "Project", "Start")
	b := owner.entry(tpl, map[string]any{"name": "Plan", "parent": "root", "document": para("later")})
	c := owner.entry(tpl, map[string]any{"name": "Risks", "parent": b, "document": textWith(words("Back to "), valueOf("entry:root"))})
	owner.ok("PATCH", "/api/v1/templates/"+tpl+"/entries/"+b, map[string]any{"name": "Plan", "parent": "root",
		"document": textWith(words("See "), valueOf("entry:"+c), words(" in "), valueOf("set"))})
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Project", "document": textWith(words("Plan: "), valueOf("entry:"+b),
		words(" by "), valueOf("creator"), words(" in "), valueOf("space"), words(" from "), valueOf("template"), words(" on "), valueOf("date"))})

	// Drafts by default: one draft each, no version, and a reader sees nothing (TPL-150).
	status, made := owner.makeSet(tpl, map[string]any{"space_id": space, "title": "Apollo"})
	if status != http.StatusCreated {
		t.Fatalf("make: %d %v", status, made)
	}
	ids := map[string]string{}
	for _, title := range []string{"Apollo", "Plan", "Risks"} {
		ids[title] = pageOfEntry(t, made, title)
		if n := e.count("SELECT count(*) FROM page_versions WHERE page_id = $1", ids[title]); n != 0 {
			t.Fatalf("%s has %d versions", title, n)
		}
		if n := e.count("SELECT count(*) FROM drafts WHERE page_id = $1", ids[title]); n != 1 {
			t.Fatalf("%s has %d drafts", title, n)
		}
		viewer.want(http.StatusNotFound, "GET", "/api/v1/pages/"+ids[title], nil)
	}
	draft := func(title string) string {
		return e.scalar("SELECT document::text FROM drafts WHERE page_id = $1", ids[title])
	}
	if d := draft("Apollo"); !strings.Contains(d, ids["Plan"]) || !strings.Contains(d, "Space ENG") || !strings.Contains(d, "Project") ||
		!strings.Contains(d, owner.id) || !strings.Contains(d, `"type": "date"`) || strings.Contains(d, "templateValue") {
		t.Fatalf("the root: %s", d)
	}
	if d := draft("Plan"); !strings.Contains(d, ids["Risks"]) || !strings.Contains(d, ids["Apollo"]) {
		t.Fatalf("the plan links to the risks and the set: %s", d)
	}
	if d := draft("Risks"); !strings.Contains(d, ids["Apollo"]) {
		t.Fatalf("the risks link back: %s", d)
	}
	if got := e.scalar("SELECT parent_id::text FROM pages WHERE id = $1", ids["Risks"]); got != ids["Plan"] {
		t.Fatalf("the risks sit below the plan: %s", got)
	}
	if n := e.count("SELECT count(*) FROM page_template_origin WHERE set_id = $1", made["set_id"]); n != 3 {
		t.Fatalf("origins: %d", n)
	}

	// Published at once: one version each, readable, and the links already work (TPL-111, TPL-151).
	owner.want(http.StatusNoContent, "PUT", "/api/v1/templates/"+tpl+"/publishing", map[string]any{"publish": true})
	status, made = owner.makeSet(tpl, map[string]any{"space_id": space, "title": "Gemini"})
	if status != http.StatusCreated {
		t.Fatalf("publish at once: %d %v", status, made)
	}
	for _, title := range []string{"Gemini", "Plan", "Risks"} {
		id := pageOfEntry(t, made, title)
		if n := e.count("SELECT count(*) FROM page_versions WHERE page_id = $1", id); n != 1 {
			t.Fatalf("%s has %d versions", title, n)
		}
		viewer.ok("GET", "/api/v1/pages/"+id, nil)
	}
	links := e.count("SELECT count(*) FROM page_links WHERE source_id = $1", pageOfEntry(t, made, "Plan"))
	if links != 2 {
		t.Fatalf("the links of the plan are known at once: %d", links)
	}
}

func TestTheTitlesOfASet(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	tpl := owner.guided(space, "Onboarding", "Welcome")
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Onboarding", "title_pattern": "Onboarding: {Name}",
		"document": textWith(words("Hello "), map[string]any{"type": "templateField", "attrs": map[string]any{"name": "Name", "kind": "line"}})})
	owner.entry(tpl, map[string]any{"name": "Equipment", "parent": "root", "document": para("x")})
	owner.entry(tpl, map[string]any{"name": "Long", "parent": "root", "title_pattern": strings.Repeat("word ", 40) + "{Name}", "document": para("x")})
	owner.entry(tpl, map[string]any{"name": "Empty", "parent": "root", "title_pattern": "{Nickname}",
		"document": textWith(map[string]any{"type": "templateField", "attrs": map[string]any{"name": "Nickname", "kind": "line"}})})
	owner.page(space, nil, "Equipment", "already here")

	check := owner.ok("POST", "/api/v1/templates/"+tpl+"/sets/check", map[string]any{"space_id": space, "answers": map[string]string{"Name": "{space} " + strings.Repeat("x", 100)}})
	titles := map[string]map[string]any{}
	for _, p := range check["pages"].([]any) {
		row := p.(map[string]any)
		titles[row["entry_name"].(string)] = row
	}
	if got := titles["Equipment"]; got["title"] != "Equipment" || got["exists"] != true {
		t.Fatalf("an empty pattern takes the entry's name, and an existing title is marked: %v", got)
	}
	if got := titles["Empty"]; got["title"] != "Empty" {
		t.Fatalf("a title that comes out empty takes the entry's name: %v", got)
	}
	long := titles["Long"]["title"].(string)
	if len([]rune(long)) > 255 || !strings.HasSuffix(long, "…") || strings.HasSuffix(strings.TrimSuffix(long, "…"), " ") {
		t.Fatalf("a long title is cut at a word with an ellipsis: %q", long)
	}
	if got := titles[""]["title"].(string); !strings.HasPrefix(got, "Onboarding: {space} xxx") {
		t.Fatalf("an answer is filled in once, as text: %q", got)
	}
	if e.traceOf(space).sets != 0 {
		t.Fatal("the check step made something")
	}
}

func TestAWorkflowEntryNeedsTheRightToChooseOne(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	workflow := owner.ok("POST", "/api/v1/spaces/"+space+"/workflows", map[string]any{"name": "Review", "definition": reviewFlow()})["id"].(string)
	tpl := owner.guided(space, "Policy", "Rules")
	owner.ok("PATCH", "/api/v1/templates/"+tpl+"/entries/root", map[string]any{"name": "", "workflow_id": workflow})
	writer := e.user("writer@example.com", identity.PermUseProduct)
	owner.ok("PUT", "/api/v1/spaces/"+space+"/grants", map[string]any{"subject": map[string]any{"type": "user", "id": writer.id},
		"permissions": []string{permissions.SpaceView, permissions.PageCreate}})
	before := e.traceOf(space)
	if status, out := writer.makeSet(tpl, map[string]any{"space_id": space, "title": "Policy"}); status != http.StatusForbidden {
		t.Fatalf("a workflow without page.workflow: %d %v", status, out)
	}
	if e.traceOf(space) != before {
		t.Fatal("a refused set left a trace")
	}
	// A space workflow wins, so the entry's own needs no right then (DOC-420).
	owner.ok("PUT", "/api/v1/spaces/"+space+"/workflow", map[string]any{"workflow_id": workflow, "kinds": []string{"page"}})
	if status, out := writer.makeSet(tpl, map[string]any{"space_id": space, "title": "Policy"}); status != http.StatusCreated {
		t.Fatalf("under a space workflow: %d %v", status, out)
	}
}

func TestMissingPlacesRefuseTheSetAndKeepNothing(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	other, _ := owner.space("OPS")
	tpl := owner.guided(space, "Onboarding", "Welcome")
	owner.entry(tpl, map[string]any{"name": "Plan", "parent": "root", "document": para("x")})

	// A template of one space is not found in another (TPL-804).
	owner.want(http.StatusNotFound, "POST", "/api/v1/templates/"+tpl+"/sets", map[string]any{"space_id": other})
	// A space that is gone, or one the person may not add to, is one message (TPL-800).
	outsider := e.user("outsider@example.com", identity.PermUseProduct)
	status, out := outsider.makeSet(tpl, map[string]any{"space_id": space})
	if status != http.StatusNotFound && status != http.StatusForbidden {
		t.Fatalf("an outsider: %d %v", status, out)
	}

	// The chosen parent: deleted, in another space, or restricted against the creator (TPL-803).
	gone := owner.page(space, nil, "Gone", "x")
	owner.want(http.StatusNoContent, "DELETE", "/api/v1/pages/"+gone, nil)
	elsewhere := owner.page(other, nil, "Elsewhere", "x")
	closed := owner.page(space, nil, "Closed", "x")
	owner.restrict(closed, "view", owner.id)
	before := e.traceOf(space)
	for name, parent := range map[string]string{"deleted": gone, "another space": elsewhere, "restricted": closed} {
		if status, out := editor.makeSet(tpl, map[string]any{"space_id": space, "parent_id": parent}); status < 400 {
			t.Fatalf("a %s parent: %d %v", name, status, out)
		}
	}
	if e.traceOf(space) != before {
		t.Fatal("a refused set left a trace")
	}

	// A default parent that is gone puts the root at the top, and the check step says so (TPL-802).
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Onboarding", "default_parent": gone})
	check := editor.ok("POST", "/api/v1/templates/"+tpl+"/sets/check", map[string]any{"space_id": space})
	if check["parent_id"] != nil || check["default_parent_unusable"] != true {
		t.Fatalf("check: %v", check)
	}
	status, made := editor.makeSet(tpl, map[string]any{"space_id": space})
	if status != http.StatusCreated {
		t.Fatalf("make: %d %v", status, made)
	}
	if got := e.scalar("SELECT COALESCE(parent_id::text, 'top') FROM pages WHERE id = $1", made["page"].(map[string]any)["id"]); got != "top" {
		t.Fatalf("the root lands at the top: %s", got)
	}
}

func TestTheLimitsOfATemplate(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")

	// 100 entries, 101 refused (TPL-102). The template has its first entry already.
	wide := owner.guided(space, "Wide", "x")
	for i := range 99 {
		owner.entry(wide, map[string]any{"name": fmt.Sprintf("Page %d", i), "parent": "root"})
	}
	out := owner.want(http.StatusBadRequest, "POST", "/api/v1/templates/"+wide+"/entries", map[string]any{"name": "One too many", "parent": "root"})
	if !strings.Contains(out, "100") {
		t.Fatalf("the 101st entry: %s", out)
	}

	// Six levels below the first entry, seven refused.
	deep := owner.guided(space, "Deep", "x")
	parent := "root"
	for i := range 6 {
		parent = owner.entry(deep, map[string]any{"name": fmt.Sprintf("Level %d", i+1), "parent": parent})
	}
	if out := owner.want(http.StatusBadRequest, "POST", "/api/v1/templates/"+deep+"/entries", map[string]any{"name": "Level 7", "parent": parent}); !strings.Contains(out, "6") {
		t.Fatalf("the 7th level: %s", out)
	}

	// 40 fields across the entries, 41 refused (TPL-825).
	fields := func(from, n int) map[string]any {
		parts := []map[string]any{}
		for i := range n {
			parts = append(parts, map[string]any{"type": "templateField", "attrs": map[string]any{"name": fmt.Sprintf("Field %d", from+i), "kind": "line"}})
		}
		return textWith(parts...)
	}
	many := owner.guided(space, "Many", "x")
	owner.ok("PATCH", "/api/v1/templates/"+many, map[string]any{"name": "Many", "document": fields(0, 20)})
	owner.entry(many, map[string]any{"name": "Second", "parent": "root", "document": fields(20, 20)})
	if out := owner.want(http.StatusBadRequest, "POST", "/api/v1/templates/"+many+"/entries", map[string]any{"name": "Third", "parent": "root", "document": fields(40, 1)}); !strings.Contains(out, "40") {
		t.Fatalf("the 41st field: %s", out)
	}
	// A field that repeats a name is the same field, so it counts once.
	owner.entry(many, map[string]any{"name": "Third", "parent": "root", "document": fields(0, 1)})

	// The text of all entries together stays under 5 MiB.
	big := owner.guided(space, "Big", "x")
	chunk := strings.Repeat("a", 1<<20+1000)
	for i := range 4 {
		owner.entry(big, map[string]any{"name": fmt.Sprintf("Chunk %d", i), "parent": "root", "document": para(chunk)})
	}
	if out := owner.want(http.StatusRequestEntityTooLarge, "POST", "/api/v1/templates/"+big+"/entries", map[string]any{"name": "Chunk 5", "parent": "root", "document": para(chunk)}); !strings.Contains(out, "5 MiB") {
		t.Fatalf("over 5 MiB: %s", out)
	}
}

func TestForbiddenContentIsRefusedOnSaveAndAnswersStayText(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.guided(space, "Note", "x")
	// A link to an entry that does not exist (TPL-840).
	owner.want(http.StatusBadRequest, "PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Note", "document": textWith(valueOf("entry:nowhere"))})
	owner.want(http.StatusBadRequest, "POST", "/api/v1/templates/"+tpl+"/entries", map[string]any{"name": "Bad", "parent": "root", "document": textWith(valueOf("entry:nowhere"))})
	// An unknown block, and a link of another scheme.
	owner.want(http.StatusBadRequest, "POST", "/api/v1/templates/"+tpl+"/entries", map[string]any{"name": "Bad", "parent": "root",
		"document": map[string]any{"type": "doc", "content": []any{map[string]any{"type": "script"}}}})
	link := map[string]any{"type": "text", "text": "click", "marks": []any{map[string]any{"type": "link", "attrs": map[string]any{"href": "javascript:alert(1)"}}}}
	owner.want(http.StatusBadRequest, "POST", "/api/v1/templates/"+tpl+"/entries", map[string]any{"name": "Links", "parent": "root", "document": textWith(link)})
	// An import goes through the same checks.
	owner.want(http.StatusBadRequest, "POST", "/api/v1/spaces/"+space+"/templates/import", map[string]any{"name": "Imported", "document": textWith(valueOf("entry:nowhere"))})
	// A link to an entry cannot be removed from under the link.
	key := owner.entry(tpl, map[string]any{"name": "Target", "parent": "root"})
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Note", "document": textWith(valueOf("entry:" + key))})
	if out := owner.want(http.StatusConflict, "DELETE", "/api/v1/templates/"+tpl+"/entries/"+key, nil); !strings.Contains(out, "links") {
		t.Fatalf("removing a linked entry: %s", out)
	}

	// Answers never carry markup (TPL-842).
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Note", "title_pattern": "{Topic}",
		"document": textWith(map[string]any{"type": "templateField", "attrs": map[string]any{"name": "Topic", "kind": "line"}}, valueOf("entry:"+key))})
	answer := `{"type":"image","attrs":{"src":"x"}} {space} javascript:alert(1)`
	status, made := editor.makeSet(tpl, map[string]any{"space_id": space, "answers": map[string]string{"Topic": answer}})
	if status != http.StatusCreated {
		t.Fatalf("make: %d %v", status, made)
	}
	root := made["page"].(map[string]any)
	if root["title"] != answer {
		t.Fatalf("the title holds the answer as it was typed: %v", root["title"])
	}
	d := e.scalar("SELECT document::text FROM drafts WHERE page_id = $1", root["id"])
	if strings.Contains(d, `"type": "image"`) || strings.Contains(d, `"href"`) || !strings.Contains(d, "{space}") {
		t.Fatalf("an answer stays text: %s", d)
	}
}

func TestVersionsOfATemplateAndAFormBoundToOne(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.guided(space, "Note", "First text")
	versions := func() int { return len(owner.ok("GET", "/api/v1/templates/"+tpl+"/versions", nil)["versions"].([]any)) }
	if got := versions(); got != 1 {
		t.Fatalf("a new template has version 1: %d", got)
	}
	// The name changes how it is found, not what it makes (TPL-600).
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Renamed"})
	if got := versions(); got != 1 {
		t.Fatalf("a new name made a version: %d", got)
	}
	opened := editor.ok("GET", "/api/v1/templates/"+tpl, nil)
	if opened["template_version"] != float64(1) {
		t.Fatalf("the form learns its version: %v", opened["template_version"])
	}
	steps := []map[string]any{
		{"name": "Renamed", "document": para("Second text")},
		{"name": "Renamed", "title_pattern": "{date}"},
		{"name": "Renamed", "title_pattern": "{date}", "labels": []string{"x"}},
	}
	for i, change := range steps {
		owner.ok("PATCH", "/api/v1/templates/"+tpl, change)
		if got := versions(); got != i+2 {
			t.Fatalf("change %d: %d versions", i, got)
		}
	}
	owner.entry(tpl, map[string]any{"name": "Child", "parent": "root"})
	owner.want(http.StatusNoContent, "PUT", "/api/v1/templates/"+tpl+"/publishing", map[string]any{"publish": true})
	if got := versions(); got != 6 {
		t.Fatalf("entries and publishing make versions: %d", got)
	}
	// A form opened on version 1 makes version 1, with its old text and one page (TPL-603).
	status, made := editor.makeSet(tpl, map[string]any{"space_id": space, "version": 1, "title": "Old"})
	if status != http.StatusCreated || len(made["pages"].([]any)) != 1 {
		t.Fatalf("version 1: %d %v", status, made)
	}
	if d := e.scalar("SELECT document::text FROM drafts WHERE page_id = $1", made["page"].(map[string]any)["id"]); !strings.Contains(d, "First text") {
		t.Fatalf("the old text: %s", d)
	}
	editor.want(http.StatusNotFound, "POST", "/api/v1/templates/"+tpl+"/sets", map[string]any{"space_id": space, "version": 99})
	// Today's route uses the newest version, now a set of two published pages.
	status, out := editor.do("POST", "/api/v1/templates/"+tpl+"/use", map[string]any{"space_id": space, "title": "New"})
	if status != http.StatusCreated || !strings.Contains(out, `"status":"published"`) {
		t.Fatalf("use: %d %s", status, out)
	}
}

func TestAPlainTemplateSavedByTheOlderReleaseGetsItsNextVersionOnFirstUse(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.guided(space, "Note", "x")
	// The older release changes the row and writes no version (TPL-1103).
	e.sql("UPDATE templates SET title_pattern = 'Old release {date}', updated_at = now() + interval '1 second' WHERE id = $1", tpl)
	status, made := editor.makeSet(tpl, map[string]any{"space_id": space})
	if status != http.StatusCreated || !strings.HasPrefix(made["page"].(map[string]any)["title"].(string), "Old release") {
		t.Fatalf("make: %d %v", status, made)
	}
	if n := e.count("SELECT count(*) FROM template_versions WHERE template_id = $1", tpl); n != 2 {
		t.Fatalf("versions: %d", n)
	}
}

func TestTheEmptyPageMakesWayInTheSameTransaction(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.guided(space, "Note", "x")
	empty := editor.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "untitled": true, "title": ""})["id"].(string)
	service := e.app.Templates()
	defer service.SetHooks(templates.SetHooks{})
	service.SetHooks(templates.SetHooks{Step: func(_ context.Context, step string, _ int) error {
		if step == "audit" {
			return errInjected
		}
		return nil
	}})
	if status, _ := editor.makeSet(tpl, map[string]any{"space_id": space, "replaces": empty}); status != http.StatusInternalServerError {
		t.Fatalf("fault: %d", status)
	}
	if got := e.scalar("SELECT status FROM pages WHERE id = $1", empty); got == "trashed" {
		t.Fatal("a refused set removed the empty page")
	}
	service.SetHooks(templates.SetHooks{})
	if status, out := editor.makeSet(tpl, map[string]any{"space_id": space, "replaces": empty}); status != http.StatusCreated {
		t.Fatalf("make: %d %v", status, out)
	}
	if got := e.scalar("SELECT status FROM pages WHERE id = $1", empty); got != "trashed" {
		t.Fatalf("the empty page stays: %s", got)
	}
	// Today's route takes the same field (TPL-006).
	empty = editor.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "untitled": true, "title": ""})["id"].(string)
	editor.ok("POST", "/api/v1/templates/"+tpl+"/use", map[string]any{"space_id": space, "replaces": empty})
	if got := e.scalar("SELECT status FROM pages WHERE id = $1", empty); got != "trashed" {
		t.Fatalf("use: the empty page stays: %s", got)
	}
	// Only an empty page of one's own makes way.
	written := editor.page(space, nil, "Written", "words")
	editor.want(http.StatusConflict, "POST", "/api/v1/templates/"+tpl+"/sets", map[string]any{"space_id": space, "replaces": written})
	others := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "untitled": true, "title": ""})["id"].(string)
	if status, _ := editor.makeSet(tpl, map[string]any{"space_id": space, "replaces": others}); status < 400 {
		t.Fatal("someone else's empty page made way")
	}
}

func TestTwoSetsInOneSpaceAtOnceAreBothWhole(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.guided(space, "Onboarding", "Welcome")
	for i := range 5 {
		owner.entry(tpl, map[string]any{"name": fmt.Sprintf("Part %d", i), "parent": "root"})
	}
	var wg sync.WaitGroup
	for _, u := range []*user{owner, editor} {
		wg.Go(func() {
			if status, out := u.makeSet(tpl, map[string]any{"space_id": space}); status != http.StatusCreated {
				t.Errorf("%s: %d %v", u.email, status, out)
			}
		})
	}
	wg.Wait()
	if n := e.count("SELECT count(*) FROM page_template_origin o JOIN pages p ON p.id = o.page_id WHERE p.space_id = $1", space); n != 12 {
		t.Fatalf("pages of the two sets: %d", n)
	}
	positions := e.count(`SELECT count(*) FROM (SELECT parent_id, position FROM pages WHERE space_id = $1 AND kind <> 'template'
		GROUP BY parent_id, position HAVING count(*) > 1) twice`, space)
	if positions != 0 {
		t.Fatalf("two pages share a place: %d", positions)
	}
}

func TestBackgroundSetsTakeTurnsPerPersonAndPerSpace(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	tpl := owner.guided(space, "Planning round", "Overview")
	for i := range 26 {
		owner.entry(tpl, map[string]any{"name": fmt.Sprintf("Team %02d", i), "parent": "root"})
	}
	jobOf := func() string {
		status, out := owner.makeSet(tpl, map[string]any{"space_id": space})
		if status != http.StatusAccepted {
			t.Fatalf("background: %d %v", status, out)
		}
		return out["job_id"].(string)
	}
	first, second := jobOf(), jobOf()
	service := e.app.Templates()
	defer service.SetHooks(templates.SetHooks{})
	holding, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	service.SetHooks(templates.SetHooks{Step: func(ctx context.Context, step string, entry int) error {
		if step == "page" && entry == 1 {
			once.Do(func() { close(holding) })
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}})
	done := make(chan error, 1)
	go func() {
		_, err := e.app.Jobs().RunOnce(context.Background())
		done <- err
	}()
	<-holding
	// While the first set of this person runs, the second waits in the queue (TPL-827).
	if _, err := e.app.Jobs().RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := e.scalar("SELECT status FROM jobs WHERE id = $1", second); got != "queued" {
		t.Fatalf("the second set of one person must wait: %s", got)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	service.SetHooks(templates.SetHooks{})
	time.Sleep(1100 * time.Millisecond) // a job sent back rests a second before it is taken again
	e.runJobs()
	for _, job := range []string{first, second} {
		if got := e.scalar("SELECT status FROM jobs WHERE id = $1", job); got != "done" {
			t.Fatalf("job %s: %s", job, got)
		}
	}
}

func TestASetThatRunsTooLongIsStopped(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	tpl := owner.guided(space, "Note", "x")
	owner.entry(tpl, map[string]any{"name": "Child", "parent": "root"})
	service := e.app.Templates()
	defer service.SetHooks(templates.SetHooks{})
	service.SetHooks(templates.SetHooks{Deadline: 200 * time.Millisecond, Step: func(ctx context.Context, step string, _ int) error {
		if step != "page" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
			return nil
		}
	}})
	before := e.traceOf(space)
	if status, out := owner.makeSet(tpl, map[string]any{"space_id": space}); status < 500 {
		t.Fatalf("a set past its deadline: %d %v", status, out)
	}
	if e.traceOf(space) != before {
		t.Fatal("a stopped set left a trace")
	}
}

func TestMovingKeepsTheOriginAndCopyingDoesNot(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	other, _ := owner.space("OPS")
	tpl := owner.guided(space, "Note", "x")
	owner.want(http.StatusNoContent, "PUT", "/api/v1/templates/"+tpl+"/publishing", map[string]any{"publish": true})
	_, made := owner.makeSet(tpl, map[string]any{"space_id": space, "title": "Made"})
	page := made["page"].(map[string]any)["id"].(string)
	owner.ok("POST", "/api/v1/pages/"+page+"/move", map[string]any{"space_id": other, "parent_id": nil})
	if n := e.count("SELECT count(*) FROM page_template_origin WHERE page_id = $1", page); n != 1 {
		t.Fatal("a move lost the origin (TPL-850)")
	}
	copied := owner.ok("POST", "/api/v1/pages/"+page+"/copy", map[string]any{})["id"].(string)
	if n := e.count("SELECT count(*) FROM page_template_origin WHERE page_id = $1", copied); n != 0 {
		t.Fatal("a copy was not made from a template (TPL-851)")
	}
	// Deleting the template keeps the record (TPL-601, TPL-612).
	owner.want(http.StatusNoContent, "DELETE", "/api/v1/templates/"+tpl, nil)
	if n := e.count("SELECT count(*) FROM page_template_origin WHERE page_id = $1 AND template_id IS NULL", page); n != 1 {
		t.Fatal("the origin of a deleted template")
	}
}

func TestTheMigrationGivesEveryTemplateVersionOneAndKeepsEveryGallery(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	other, _ := owner.space("OPS")
	root := admin(e)
	owner.guided(space, "Note", "x")
	owner.ok("POST", "/api/v1/spaces/"+other+"/templates", map[string]any{"name": "Elsewhere", "title_pattern": "{date}", "labels": []string{"ops"}})
	root.ok("POST", "/api/v1/admin/templates", map[string]any{"name": "Everywhere", "document": para("x")})
	gallery := func(u *user, space string) string {
		var names []string
		for _, x := range u.ok("GET", "/api/v1/spaces/"+space+"/templates", nil)["templates"].([]any) {
			names = append(names, x.(map[string]any)["id"].(string))
		}
		return strings.Join(names, ",")
	}
	before := map[string]string{"eng": gallery(editor, space), "ops": gallery(owner, other)}

	// Back to the state before the upgrade: no version, no owner. Then the migration again,
	// which may run twice (TPL-1100, TPL-1101).
	e.sql("DELETE FROM template_versions")
	e.sql("UPDATE templates SET owner_id = NULL, approved_version = NULL")
	migs, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range migs {
		if m.Name == "guided_templates" {
			e.sql(m.SQL)
		}
	}
	if n := e.count(`SELECT count(*) FROM templates t WHERE t.owner_id IS DISTINCT FROM t.created_by OR t.approved_version IS DISTINCT FROM 1
		OR t.state <> 'approved' OR NOT EXISTS (SELECT 1 FROM template_versions v WHERE v.template_id = t.id AND v.number = 1 AND v.state = 'approved')`); n != 0 {
		t.Fatalf("%d templates without an approved version 1", n)
	}
	after := map[string]string{"eng": gallery(editor, space), "ops": gallery(owner, other)}
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatalf("the galleries changed:\n%v\n%v", before, after)
	}
	// Version 1 makes what the template made before.
	tpl := strings.Split(before["ops"], ",")
	for _, id := range tpl {
		if e.scalar("SELECT name FROM templates WHERE id = $1", id) == "Elsewhere" {
			status, made := owner.makeSet(id, map[string]any{"space_id": other})
			if status != http.StatusCreated || made["page"].(map[string]any)["title"] != e.scalar("SELECT to_char(now(), 'YYYY-MM-DD')") {
				t.Fatalf("version 1: %d %v", status, made)
			}
		}
	}
}

func TestTheTemplateActionsFollowThePermissionMatrix(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	root := admin(e)
	guest := e.userOfType("guest@example.com", identity.TypeGuest, identity.PermUseProduct)
	nobody := e.user("nobody@example.com", identity.PermUseProduct)
	tpl := owner.guided(space, "Note", "x")
	instance := root.ok("POST", "/api/v1/admin/templates", map[string]any{"name": "Everywhere", "document": para("x")})["id"].(string)
	type want struct{ see, use, edit int }
	matrix := map[*user]want{
		owner:  {200, 201, 201},
		editor: {403, 201, 403},
		viewer: {403, 403, 403},
		// Who may not see the space is told it is not there (design standard, States).
		guest:  {404, 404, 404},
		nobody: {404, 404, 404},
		root:   {404, 404, 404},
	}
	for u, w := range matrix {
		if got, out := u.do("GET", "/api/v1/templates/"+tpl+"/entries", nil); got != w.see {
			t.Errorf("%s sees the entries: %d %s", u.email, got, out)
		}
		if got, out := u.do("POST", "/api/v1/templates/"+tpl+"/sets", map[string]any{"space_id": space}); got != w.use {
			t.Errorf("%s uses: %d, want %d: %s", u.email, got, w.use, out)
		}
		if got, out := u.do("POST", "/api/v1/templates/"+tpl+"/entries", map[string]any{"name": "By " + u.email, "parent": "root"}); got != w.edit {
			t.Errorf("%s edits: %d %s", u.email, got, out)
		}
	}
	// A template of all spaces is edited by administrators only, and used where page.create holds.
	owner.want(http.StatusForbidden, "POST", "/api/v1/templates/"+instance+"/entries", map[string]any{"name": "x", "parent": "root"})
	root.ok("POST", "/api/v1/templates/"+instance+"/entries", map[string]any{"name": "x", "parent": "root"})
	if status, _ := editor.makeSet(instance, map[string]any{"space_id": space}); status != http.StatusCreated {
		t.Fatalf("an editor uses a template of all spaces: %d", status)
	}
	if status, _ := guest.makeSet(instance, map[string]any{"space_id": space}); status != http.StatusForbidden && status != http.StatusNotFound {
		t.Fatalf("a guest without page.create: %d", status)
	}
}
