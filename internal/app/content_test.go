// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/permissions"
)

// user is a logged-in test user.
type user struct {
	e     *env
	c     *http.Client
	email string
	id    string
}

func (e *env) user(email string, perms ...string) *user {
	e.t.Helper()
	return e.userOfType(email, identity.TypeMember, perms...)
}

func (e *env) userOfType(email, typ string, perms ...string) *user {
	e.t.Helper()
	u, err := e.app.Identity().CreateLocalUser(context.Background(), identity.NewUser{
		Email: email, DisplayName: "Test " + email, Type: typ, Password: adminPassword, GlobalPermissions: perms,
	}, identity.SystemActor("test"), identity.RequestMeta{})
	if err != nil {
		e.t.Fatal(err)
	}
	c := e.client()
	if got := e.login(c, email, adminPassword); got != 200 {
		e.t.Fatalf("login %s: %d", email, got)
	}
	return &user{e: e, c: c, email: email, id: u.ID.String()}
}

func (u *user) do(method, path string, body any) (int, string) {
	u.e.t.Helper()
	s := ""
	switch v := body.(type) {
	case nil:
	case string:
		s = v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			u.e.t.Fatal(err)
		}
		s = string(b)
	}
	resp, out := u.e.do(u.c, method, path, s)
	return resp.StatusCode, out
}

// ok calls an endpoint that must succeed and decodes the answer.
func (u *user) ok(method, path string, body any) map[string]any {
	u.e.t.Helper()
	status, out := u.do(method, path, body)
	if status < 200 || status > 299 {
		u.e.t.Fatalf("%s %s as %s: %d %s", method, path, u.email, status, out)
	}
	m := map[string]any{}
	_ = json.Unmarshal([]byte(out), &m)
	return m
}

func (u *user) want(status int, method, path string, body any) string {
	u.e.t.Helper()
	got, out := u.do(method, path, body)
	if got != status {
		u.e.t.Fatalf("%s %s as %s: got %d, want %d: %s", method, path, u.email, got, status, out)
	}
	return out
}

func para(text string) map[string]any {
	return map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}}}}
}

func (u *user) space(key string) (id, home string) {
	u.e.t.Helper()
	m := u.ok("POST", "/api/v1/spaces", map[string]any{"key": key, "name": "Space " + key})
	return m["id"].(string), m["home_page_id"].(string)
}

// page creates and publishes a page.
func (u *user) page(space string, parent any, title, text string) string {
	u.e.t.Helper()
	m := u.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "parent_id": parent, "title": title})
	id := m["id"].(string)
	u.ok("POST", "/api/v1/pages/"+id+"/publish", map[string]any{"title": title, "document": para(text), "base_version": 0})
	return id
}

func (u *user) grant(space, subjectType, subjectID, template string) {
	u.e.t.Helper()
	if subjectID == "" {
		subjectID = "00000000-0000-0000-0000-000000000000"
	}
	u.ok("PUT", "/api/v1/spaces/"+space+"/grants", map[string]any{
		"subject":     map[string]any{"type": subjectType, "id": subjectID},
		"permissions": permissions.Templates[template]})
}

func (u *user) restrict(page, kind string, userIDs ...string) {
	u.e.t.Helper()
	subjects := []any{}
	for _, id := range userIDs {
		subjects = append(subjects, map[string]any{"type": "user", "id": id})
	}
	u.ok("PUT", "/api/v1/pages/"+page+"/restrictions", map[string]any{"kind": kind, "subjects": subjects})
}

func (e *env) auditActions(action string) int {
	e.t.Helper()
	evs, err := audit.List(context.Background(), e.app.Pool(), audit.Filter{Action: action, Limit: 500})
	if err != nil {
		e.t.Fatal(err)
	}
	return len(evs)
}

func (e *env) outboxCount(eventType string) int {
	e.t.Helper()
	var n int
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM outbox_events WHERE type = $1", eventType).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// TestAuditAndOutboxPerStateChange walks through the life of a page and checks
// that every change left both an audit event and an outbox event.
func TestAuditAndOutboxPerStateChange(t *testing.T) {
	e := newEnv(t)
	a := e.user("a@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := a.space("ENG")
	id := a.page(space, nil, "Page", "x")
	a.ok("PATCH", "/api/v1/pages/"+id, map[string]any{"title": "New"})
	a.ok("POST", "/api/v1/pages/"+id+"/versions/1/restore", nil)
	other := a.page(space, nil, "Other", "x")
	a.ok("POST", "/api/v1/pages/"+id+"/move", map[string]any{"parent_id": other})
	a.ok("POST", "/api/v1/pages/"+id+"/copy", map[string]any{})
	a.upload(id, "a.txt", "x")
	a.ok("DELETE", "/api/v1/pages/"+other, nil)
	item := a.ok("GET", "/api/v1/spaces/"+space+"/trash", nil)["items"].([]any)[0].(map[string]any)["id"].(string)
	a.ok("POST", "/api/v1/trash/"+item+"/restore", nil)
	a.ok("PATCH", "/api/v1/spaces/"+space, map[string]any{"name": "Renamed"})
	a.ok("POST", "/api/v1/spaces/"+space+"/archive", nil)
	a.ok("POST", "/api/v1/spaces/"+space+"/unarchive", nil)

	for _, action := range []string{
		"space.created", "space.updated", "space.archived", "space.unarchived",
		"page.created", "page.published", "page.renamed", "page.version_restored", "page.moved", "page.copied",
		"page.trashed", "page.restored", "attachment.uploaded",
	} {
		if e.auditActions(action) == 0 {
			t.Errorf("%s: no audit event", action)
		}
		if e.outboxCount(action) == 0 {
			t.Errorf("%s: no outbox event", action)
		}
	}
	res, err := audit.Verify(context.Background(), e.app.Pool())
	if err != nil || !res.OK {
		t.Fatalf("audit chain: %+v %v", res, err)
	}
}

// A failed change must leave nothing behind in the audit log or the outbox.
func TestRefusedChangeLeavesNoEvents(t *testing.T) {
	e := newEnv(t)
	a := e.user("a@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := a.space("ENG")
	id := a.page(space, nil, "Page", "x")
	before := e.auditActions("page.published")
	a.want(409, "POST", "/api/v1/pages/"+id+"/publish", map[string]any{"title": "Page", "document": para("y"), "base_version": 99})
	if got := e.auditActions("page.published"); got != before {
		t.Fatalf("a refused publish wrote an audit event: %d to %d", before, got)
	}
}

func TestSpaceCreationAndKeys(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)

	id, home := owner.space("ENG")
	owner.want(409, "POST", "/api/v1/spaces", map[string]any{"key": "eng", "name": "x"}) // ignoring case
	for _, key := range []string{"", "a", "has space", "../x", strings.Repeat("a", 33)} {
		owner.want(400, "POST", "/api/v1/spaces", map[string]any{"key": key, "name": "x"})
	}
	reader.want(403, "POST", "/api/v1/spaces", map[string]any{"key": "NOPE", "name": "x"})

	m := owner.ok("GET", "/api/v1/spaces/"+id, nil)
	if !m["can"].(map[string]any)[permissions.SpaceAdminister].(bool) {
		t.Fatal("the creator must be space admin")
	}
	p := owner.ok("GET", "/api/v1/pages/"+home, nil)
	if p["status"] != "published" || p["current_version"] != float64(1) {
		t.Fatalf("home page: %+v", p)
	}
	if e.auditActions("space.created") != 1 || e.outboxCount("space.created") != 1 {
		t.Fatal("space.created must be audited and published")
	}
}

func TestUnpublishedPageIsPrivate(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	peer := e.user("peer@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", peer.id, permissions.TemplateEditor)

	m := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Secret plan"})
	id := m["id"].(string)
	owner.ok("GET", "/api/v1/pages/"+id, nil)
	peer.want(404, "GET", "/api/v1/pages/"+id, nil)
	if out := peer.want(200, "GET", "/api/v1/spaces/"+space+"/tree", nil); strings.Contains(out, "Secret plan") {
		t.Fatal("an unpublished page is visible in the tree of someone else")
	}
	if out := owner.want(200, "GET", "/api/v1/spaces/"+space+"/tree", nil); !strings.Contains(out, "Secret plan") {
		t.Fatal("the creator must see their own unpublished page")
	}
	owner.want(409, "POST", "/api/v1/pages", map[string]any{"space_id": space, "parent_id": id, "title": "Child"})

	owner.ok("POST", "/api/v1/pages/"+id+"/publish", map[string]any{"title": "Secret plan", "document": para("now public"), "base_version": 0})
	peer.ok("GET", "/api/v1/pages/"+id, nil)
}

func TestVersionsConflictRestoreCompare(t *testing.T) {
	e := newEnv(t)
	a := e.user("a@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	b := e.user("b@example.com", identity.PermUseProduct)
	space, _ := a.space("ENG")
	a.grant(space, "user", b.id, permissions.TemplateEditor)

	id := a.page(space, nil, "Page", "first text")
	path := "/api/v1/pages/" + id

	a.ok("POST", path+"/publish", map[string]any{"title": "Page", "document": para("second text"), "base_version": 1})
	// B edited version 1 while A published version 2.
	out := b.want(409, "POST", path+"/publish", map[string]any{"title": "Page", "document": para("from b"), "base_version": 1})
	if !strings.Contains(out, "version_conflict") {
		t.Fatalf("conflict: %s", out)
	}
	b.ok("POST", path+"/publish", map[string]any{"title": "Page", "document": para("from b"), "base_version": 1, "overwrite": true})

	cmp := a.ok("GET", path+"/compare?from=1&to=2", nil)
	blocks := cmp["blocks"].([]any)
	if len(blocks) != 1 || blocks[0].(map[string]any)["op"] != "changed" {
		t.Fatalf("compare: %+v", blocks)
	}

	a.ok("POST", path+"/versions/1/restore", nil)
	p := a.ok("GET", path, nil)
	if p["current_version"] != float64(4) || !strings.Contains(fmt.Sprint(p["document"]), "first text") {
		t.Fatalf("restore: %+v", p)
	}
	list := a.ok("GET", path+"/versions", nil)["versions"].([]any)
	if len(list) != 4 || list[0].(map[string]any)["restored_from"] != float64(1) {
		t.Fatalf("versions: %+v", list)
	}
	if e.auditActions("page.version_restored") != 1 {
		t.Fatal("restore not audited")
	}

	if _, err := e.app.Pool().Exec(context.Background(), "UPDATE page_versions SET title = 'tampered'"); err == nil {
		t.Fatal("versions must be immutable")
	}
}

func TestDraftsArePrivateAndCleared(t *testing.T) {
	e := newEnv(t)
	a := e.user("a@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	b := e.user("b@example.com", identity.PermUseProduct)
	space, _ := a.space("ENG")
	a.grant(space, "user", b.id, permissions.TemplateEditor)
	id := a.page(space, nil, "Page", "text")
	path := "/api/v1/pages/" + id

	a.ok("PUT", path+"/draft", map[string]any{"title": "Page", "document": para("work in progress"), "base_version": 1})
	a.ok("PUT", path+"/draft", map[string]any{"title": "Page", "document": para("work in progress 2"), "base_version": 1})
	if m := a.ok("GET", path+"/draft", nil); !strings.Contains(fmt.Sprint(m["document"]), "progress 2") {
		t.Fatalf("draft: %+v", m)
	}
	b.want(404, "GET", path+"/draft", nil)
	if p := a.ok("GET", path, nil); p["has_draft"] != true || strings.Contains(fmt.Sprint(p["document"]), "progress") {
		t.Fatalf("a draft must not change the page: %+v", p)
	}
	a.ok("POST", path+"/publish", map[string]any{"title": "Page", "document": para("done"), "base_version": 1})
	a.want(404, "GET", path+"/draft", nil)
}

func TestDocumentGateAtTheAPI(t *testing.T) {
	e := newEnv(t)
	a := e.user("a@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := a.space("ENG")
	id := a.page(space, nil, "Page", "text")
	path := "/api/v1/pages/" + id

	evil := []string{
		`{"type":"doc","content":[{"type":"script"}]}`,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}]}`,
		`{"type":"doc","content":[{"type":"image","attrs":{"src":"https://evil.example/p.gif"}}]}`,
	}
	for _, doc := range evil {
		a.want(400, "POST", path+"/publish", `{"title":"Page","base_version":1,"document":`+doc+`}`)
		a.want(400, "PUT", path+"/draft", `{"title":"Page","base_version":1,"document":`+doc+`}`)
	}
	a.ok("POST", path+"/publish", `{"title":"Page","base_version":1,"document":{"type":"doc","content":[{"type":"paragraph","attrs":{"onclick":"x"}}]}}`)
	if strings.Contains(a.want(200, "GET", path, nil), "onclick") {
		t.Fatal("an unknown attribute was stored")
	}
}

func TestTreeMoveAndStableURL(t *testing.T) {
	e := newEnv(t)
	a := e.user("a@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := a.space("ENG")
	p1 := a.page(space, nil, "One", "x")
	p2 := a.page(space, p1, "Two", "x")
	p3 := a.page(space, p2, "Three", "x")
	p4 := a.page(space, nil, "Four", "x")

	a.want(409, "POST", "/api/v1/pages/"+p1+"/move", map[string]any{"parent_id": p3})
	a.want(409, "POST", "/api/v1/pages/"+p1+"/move", map[string]any{"parent_id": p1})

	a.ok("POST", "/api/v1/pages/"+p2+"/move", map[string]any{"parent_id": p4})
	m := a.ok("GET", "/api/v1/pages/"+p3, nil)
	crumbs := m["breadcrumbs"].([]any)
	if len(crumbs) != 2 || crumbs[0].(map[string]any)["title"] != "Four" || crumbs[1].(map[string]any)["title"] != "Two" {
		t.Fatalf("breadcrumbs after move: %+v", crumbs)
	}

	a.ok("PATCH", "/api/v1/pages/"+p3, map[string]any{"title": "Renamed"})
	if m := a.ok("GET", "/api/v1/pages/"+p3, nil); m["title"] != "Renamed" {
		t.Fatalf("after rename: %+v", m)
	}

	// Reorder: put Four before One at the top level.
	a.ok("POST", "/api/v1/pages/"+p4+"/move", map[string]any{"before_id": p1})
	tree := a.ok("GET", "/api/v1/spaces/"+space+"/tree", nil)["pages"].([]any)
	var top []string
	for _, n := range tree {
		if m := n.(map[string]any); m["parent_id"] == nil {
			top = append(top, m["title"].(string))
		}
	}
	if strings.Join(top, ",") != "Space ENG,Four,One" {
		t.Fatalf("order: %v", top)
	}
}

// Two people moving pages into each other at the same moment must never create
// a loop, whichever wins.
func TestConcurrentMovesCannotCreateALoop(t *testing.T) {
	e := newEnv(t)
	a := e.user("a@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := a.space("ENG")
	for round := range 5 {
		p := a.page(space, nil, fmt.Sprintf("P%d", round), "x")
		q := a.page(space, nil, fmt.Sprintf("Q%d", round), "x")
		var wg sync.WaitGroup
		codes := make([]int, 2)
		for i, ids := range [][2]string{{p, q}, {q, p}} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				b, _ := json.Marshal(map[string]any{"parent_id": ids[1]})
				req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/pages/"+ids[0]+"/move", bytes.NewReader(b))
				req.Header.Set("Content-Type", "application/json")
				resp, err := a.c.Do(req)
				if err != nil {
					t.Error(err)
					return
				}
				_ = resp.Body.Close()
				codes[i] = resp.StatusCode
			}()
		}
		wg.Wait()
		if codes[0] == 200 && codes[1] == 200 {
			t.Fatalf("both moves succeeded: a loop exists")
		}
		var loops int
		if err := e.app.Pool().QueryRow(context.Background(),
			"SELECT count(*) FROM page_tree a JOIN page_tree b ON a.ancestor_id = b.descendant_id AND a.descendant_id = b.ancestor_id WHERE a.depth > 0").Scan(&loops); err != nil {
			t.Fatal(err)
		}
		if loops != 0 {
			t.Fatalf("closure table has a loop")
		}
	}
}

func TestRestrictionsAcrossTreeSearchFavouritesAndExport(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	viewer := e.user("viewer@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", viewer.id, permissions.TemplateViewer)

	parent := owner.page(space, nil, "Payroll", "salaries zebrafish")
	child := owner.page(space, parent, "Bonus", "bonus zebrafish")

	viewer.ok("PUT", "/api/v1/pages/"+child+"/favourite", nil)
	viewer.ok("GET", "/api/v1/pages/"+child, nil)
	if out := viewer.want(200, "GET", "/api/v1/search?q=zebrafish", nil); !strings.Contains(out, "Bonus") {
		t.Fatalf("search before restricting: %s", out)
	}

	owner.restrict(parent, "view", owner.id)

	for _, p := range []string{parent, child} {
		viewer.want(404, "GET", "/api/v1/pages/"+p, nil)
		viewer.want(404, "GET", "/api/v1/pages/"+p+"/versions", nil)
		viewer.want(404, "GET", "/api/v1/pages/"+p+"/attachments", nil)
		viewer.want(404, "GET", "/api/v1/pages/"+p+"/export.md", nil)
	}
	checks := map[string]string{
		"tree":       "/api/v1/spaces/" + space + "/tree",
		"search":     "/api/v1/search?q=zebrafish",
		"favourites": "/api/v1/me/favourites",
		"recent":     "/api/v1/me/recent",
	}
	for name, path := range checks {
		if out := viewer.want(200, "GET", path, nil); strings.Contains(out, "Bonus") || strings.Contains(out, "Payroll") {
			t.Errorf("%s leaks restricted pages: %s", name, out)
		}
	}
	resp, body := e.do(viewer.c, "GET", "/api/v1/spaces/"+space+"/export.zip", "")
	if resp.StatusCode != 200 || bytes.Contains([]byte(body), []byte("Bonus")) {
		t.Fatalf("export: %d", resp.StatusCode)
	}
	if e.auditActions("access.denied") == 0 {
		t.Fatal("refused access must be audited")
	}

	// Being on a restriction gives nothing without a space grant.
	outsider := e.user("outsider@example.com", identity.PermUseProduct)
	owner.restrict(parent, "view", owner.id, outsider.id)
	outsider.want(404, "GET", "/api/v1/pages/"+parent, nil)

	// Lifting the restriction brings everything back.
	owner.restrict(parent, "view")
	viewer.ok("GET", "/api/v1/pages/"+child, nil)
}

func TestEditRestrictionDoesNotInherit(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	editor := e.user("editor@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", editor.id, permissions.TemplateEditor)
	parent := owner.page(space, nil, "Parent", "x")
	child := owner.page(space, parent, "Child", "x")
	owner.restrict(parent, "edit", owner.id)

	editor.want(403, "POST", "/api/v1/pages/"+parent+"/publish", map[string]any{"title": "Parent", "document": para("y"), "base_version": 1})
	editor.ok("GET", "/api/v1/pages/"+parent, nil)
	editor.ok("POST", "/api/v1/pages/"+child+"/publish", map[string]any{"title": "Child", "document": para("y"), "base_version": 1})
}

func TestMoveCannotQuietlyDropARestriction(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	editor := e.user("editor@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", editor.id, permissions.TemplateEditor)
	secret := owner.page(space, nil, "Secret", "x")
	child := owner.page(space, secret, "Child", "x")
	owner.restrict(secret, "view", owner.id, editor.id)

	editor.want(403, "POST", "/api/v1/pages/"+child+"/move", map[string]any{"parent_id": nil})
	owner.ok("POST", "/api/v1/pages/"+child+"/move", map[string]any{"parent_id": nil})
}

func TestAdministratorsHaveNoContentAccessUntilRecovery(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Private", "x")
	owner.restrict(page, "view", owner.id)

	admin.want(404, "GET", "/api/v1/pages/"+page, nil)
	admin.want(404, "GET", "/api/v1/spaces/"+space, nil)
	admin.want(400, "POST", "/api/v1/admin/spaces/"+space+"/recover", map[string]any{"reason": ""})
	admin.ok("POST", "/api/v1/admin/spaces/"+space+"/recover", map[string]any{"reason": "owner left the company"})
	admin.ok("GET", "/api/v1/spaces/"+space, nil)
	admin.want(404, "GET", "/api/v1/pages/"+page, nil) // the restriction still holds
	admin.ok("POST", "/api/v1/admin/pages/"+page+"/recover", map[string]any{"reason": "owner left the company"})
	admin.ok("GET", "/api/v1/pages/"+page, nil)
	if e.auditActions("access.recovered") != 2 {
		t.Fatal("recover access must be audited")
	}

	// A normal user can never use recovery.
	owner.want(403, "POST", "/api/v1/admin/spaces/"+space+"/recover", map[string]any{"reason": "x"})
}

func TestSpaceKeepsAnAdministratorAndGuestsAreNotMembers(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	member := e.user("member@example.com", identity.PermUseProduct)
	guest := e.userOfType("guest@example.com", identity.TypeGuest, identity.PermUseProduct)
	space, home := owner.space("ENG")

	out := owner.want(409, "PUT", "/api/v1/spaces/"+space+"/grants", map[string]any{
		"subject": map[string]any{"type": "user", "id": owner.id}, "permissions": []string{}})
	if !strings.Contains(out, "last_admin") {
		t.Fatal(out)
	}

	owner.ok("POST", "/api/v1/spaces/"+space+"/preset", map[string]any{"preset": "read_only_kb"})
	member.ok("GET", "/api/v1/pages/"+home, nil)
	guest.want(404, "GET", "/api/v1/pages/"+home, nil)

	owner.grant(space, "all_guests", "", permissions.TemplateViewer)
	guest.ok("GET", "/api/v1/pages/"+home, nil)
}

func TestWhyCanOrCantI(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	viewer := e.user("viewer@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", viewer.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")
	owner.restrict(page, "view", owner.id)

	m := owner.ok("GET", "/api/v1/pages/"+page+"/explain?action=page.view&user_id="+viewer.id, nil)
	if m["allowed"] != false {
		t.Fatalf("explain: %+v", m)
	}
	found := false
	for _, c := range m["checks"].([]any) {
		check := c.(map[string]any)
		if check["layer"] == permissions.LayerRestriction && check["passed"] == false {
			found = true
		}
	}
	if !found {
		t.Fatalf("the explanation must name the restriction: %+v", m)
	}
	viewer.want(404, "GET", "/api/v1/pages/"+page+"/explain?action=page.view&user_id="+owner.id, nil)
}

func TestDeleteRestoreAndPurge(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	editor := e.user("editor@example.com", identity.PermUseProduct)
	space, home := owner.space("ENG")
	owner.grant(space, "user", editor.id, permissions.TemplateEditor)

	owner.want(409, "DELETE", "/api/v1/pages/"+home, nil)

	// Refused when something below cannot be deleted.
	parent := editor.page(space, nil, "Parent", "findable kiwifruit")
	hidden := owner.page(space, parent, "Hidden", "x")
	owner.restrict(hidden, "view", owner.id)
	if out := editor.want(409, "DELETE", "/api/v1/pages/"+parent, nil); !strings.Contains(out, "contains_protected_pages") {
		t.Fatal(out)
	}

	child := owner.page(space, parent, "Child", "x")
	owner.ok("DELETE", "/api/v1/pages/"+child, nil)
	owner.ok("DELETE", "/api/v1/pages/"+parent, nil)
	owner.want(404, "GET", "/api/v1/pages/"+parent, nil)
	owner.want(404, "GET", "/api/v1/pages/"+hidden, nil)
	if out := owner.want(200, "GET", "/api/v1/search?q=kiwifruit", nil); strings.Contains(out, "Parent") {
		t.Fatal("a trashed page is searchable")
	}

	editor.want(403, "GET", "/api/v1/spaces/"+space+"/trash", nil)
	items := owner.ok("GET", "/api/v1/spaces/"+space+"/trash", nil)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("trash: %+v", items)
	}
	ids := map[string]string{}
	for _, it := range items {
		m := it.(map[string]any)
		ids[m["title"].(string)] = m["id"].(string)
	}

	// A page cannot come back while its parent is in the trash: it would lose
	// restrictions it inherited.
	if out := owner.want(409, "POST", "/api/v1/trash/"+ids["Child"]+"/restore", nil); !strings.Contains(out, "parent_in_trash") {
		t.Fatal(out)
	}
	owner.ok("POST", "/api/v1/trash/"+ids["Parent"]+"/restore", nil)
	owner.ok("GET", "/api/v1/pages/"+parent, nil)
	owner.ok("GET", "/api/v1/pages/"+hidden, nil) // came back with its parent
	owner.want(404, "GET", "/api/v1/pages/"+child, nil)
	owner.ok("POST", "/api/v1/trash/"+ids["Child"]+"/restore", nil)
	owner.ok("GET", "/api/v1/pages/"+child, nil)

	// Purge: everything is gone, including the search row and stored file.
	owner.upload(parent, "notes.txt", "hello")
	owner.ok("DELETE", "/api/v1/pages/"+parent, nil)
	items = owner.ok("GET", "/api/v1/spaces/"+space+"/trash", nil)["items"].([]any)
	owner.ok("DELETE", "/api/v1/trash/"+items[0].(map[string]any)["id"].(string), nil)
	var pagesLeft, versions, searchRows, files, queued int
	err := e.app.Pool().QueryRow(context.Background(), `SELECT
		(SELECT count(*) FROM pages WHERE id = $1),
		(SELECT count(*) FROM page_versions WHERE page_id = $1),
		(SELECT count(*) FROM search_documents WHERE page_id = $1),
		(SELECT count(*) FROM attachments WHERE page_id = $1),
		(SELECT count(*) FROM storage_deletions)`, parent).Scan(&pagesLeft, &versions, &searchRows, &files, &queued)
	if err != nil {
		t.Fatal(err)
	}
	if pagesLeft+versions+searchRows+files != 0 || queued != 1 {
		t.Fatalf("after purge: pages=%d versions=%d search=%d files=%d queued=%d", pagesLeft, versions, searchRows, files, queued)
	}
	if n, err := e.app.Attachments().DrainDeletions(context.Background()); err != nil || n != 1 {
		t.Fatalf("drain: %d %v", n, err)
	}
	if e.auditActions("page.purged") != 1 || e.auditActions("page.trashed") < 2 {
		t.Fatal("trash actions must be audited")
	}
}

func TestRetentionJobHonoursTheSetting(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	space, _ := owner.space("ENG")
	admin.ok("PUT", "/api/v1/admin/settings", map[string]any{"trash_retention_days": 7, "max_upload_mib": 100})
	page := owner.page(space, nil, "Old", "x")
	owner.ok("DELETE", "/api/v1/pages/"+page, nil)

	ctx := context.Background()
	var days float64
	if err := e.app.Pool().QueryRow(ctx, "SELECT extract(epoch FROM purge_after - deleted_at) / 86400 FROM trash_items").Scan(&days); err != nil {
		t.Fatal(err)
	}
	if days < 6.99 || days > 7.01 {
		t.Fatalf("retention is %v days", days)
	}
	if n, _ := e.app.Trash().PurgeExpired(ctx); n != 0 {
		t.Fatalf("a fresh item was purged")
	}
	if _, err := e.app.Pool().Exec(ctx, "UPDATE trash_items SET purge_after = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	if n, err := e.app.Trash().PurgeExpired(ctx); n != 1 || err != nil {
		t.Fatalf("purge: %d %v", n, err)
	}
	var left int
	_ = e.app.Pool().QueryRow(ctx, "SELECT count(*) FROM pages WHERE id = $1", page).Scan(&left)
	if left != 0 {
		t.Fatal("the expired page still exists")
	}
}

func (u *user) upload(page, name, content string) (int, string) {
	u.e.t.Helper()
	req, _ := http.NewRequest("POST", u.e.srv.URL+"/api/v1/pages/"+page+"/attachments?filename="+url.QueryEscape(name), strings.NewReader(content))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := u.c.Do(req)
	if err != nil {
		u.e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

const pngHeader = "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01"

func TestAttachmentsNeverTrustAFile(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	viewer := e.user("viewer@example.com", identity.PermUseProduct)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", viewer.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")

	// HTML disguised as an image.
	status, body := owner.upload(page, "photo.png", "<html><script>alert(1)</script></html>")
	if status != 201 {
		t.Fatalf("upload: %d %s", status, body)
	}
	var fake map[string]any
	_ = json.Unmarshal([]byte(body), &fake)
	resp, _ := e.do(viewer.c, "GET", "/api/v1/attachments/"+fake["id"].(string)+"/content", "")
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment") ||
		resp.Header.Get("Content-Type") != "application/octet-stream" || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("headers: %+v", resp.Header)
	}

	// A real image is shown inline.
	_, body = owner.upload(page, "real.png", pngHeader)
	var real map[string]any
	_ = json.Unmarshal([]byte(body), &real)
	resp, _ = e.do(viewer.c, "GET", "/api/v1/attachments/"+real["id"].(string)+"/content", "")
	if resp.Header.Get("Content-Type") != "image/png" || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "inline") {
		t.Fatalf("image headers: %+v", resp.Header)
	}

	// SVG can carry scripts, so it is never inline.
	_, body = owner.upload(page, "x.svg", `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	var svg map[string]any
	_ = json.Unmarshal([]byte(body), &svg)
	resp, _ = e.do(viewer.c, "GET", "/api/v1/attachments/"+svg["id"].(string)+"/content", "")
	if !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("svg headers: %+v", resp.Header)
	}

	// Same name: a new version.
	_, body = owner.upload(page, "REAL.png", pngHeader+"more")
	var v2 map[string]any
	_ = json.Unmarshal([]byte(body), &v2)
	if v2["id"] != real["id"] || v2["version"] != float64(2) {
		t.Fatalf("new version: %+v", v2)
	}

	// Access follows the page.
	viewer.want(403, "DELETE", "/api/v1/attachments/"+real["id"].(string), nil)
	if status, _ := viewer.upload(page, "nope.txt", "x"); status != 403 {
		t.Fatalf("a viewer may not upload: %d", status)
	}
	owner.restrict(page, "view", owner.id)
	viewer.want(404, "GET", "/api/v1/attachments/"+real["id"].(string)+"/content", nil)
	owner.restrict(page, "view")

	// Over the limit: refused, and no file stays behind.
	admin.ok("PUT", "/api/v1/admin/settings", map[string]any{"trash_retention_days": 30, "max_upload_mib": 1})
	if status, body := owner.upload(page, "big.bin", strings.Repeat("x", 1<<20+1)); status != http.StatusRequestEntityTooLarge {
		t.Fatalf("over limit: %d %s", status, body)
	}
	var n int
	_ = e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM attachments WHERE filename = 'big.bin'").Scan(&n)
	if n != 0 {
		t.Fatal("an oversized upload left a row")
	}

	// A download is audited, an inline view is not.
	before := e.auditActions("attachment.downloaded")
	viewer.want(200, "GET", "/api/v1/attachments/"+real["id"].(string)+"/content?download=1", nil)
	if e.auditActions("attachment.downloaded") != before+1 {
		t.Fatal("a download must be audited")
	}

	// Attachment names can be found.
	if out := owner.want(200, "GET", "/api/v1/search?q=photo", nil); !strings.Contains(out, "Page") {
		t.Fatalf("attachment name not found: %s", out)
	}
	owner.ok("DELETE", "/api/v1/attachments/"+real["id"].(string), nil)
	owner.want(404, "GET", "/api/v1/attachments/"+real["id"].(string)+"/content", nil)
}

func TestSearch(t *testing.T) {
	e := newEnv(t)
	a := e.user("a@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := a.space("ENG")
	other, _ := a.space("OPS")
	a.page(space, nil, "Deployment guide", "How we ship the fietsenstalling service")
	a.page(other, nil, "Runbook", "Restart the fietsenstalling at night")

	if out := a.want(200, "GET", "/api/v1/search?q=fietsenst", nil); !strings.Contains(out, "Deployment guide") || !strings.Contains(out, "Runbook") {
		t.Fatalf("prefix search: %s", out)
	}
	if out := a.want(200, "GET", "/api/v1/search?q=fietsenstalling&space_id="+other, nil); strings.Contains(out, "Deployment") {
		t.Fatalf("space filter: %s", out)
	}
	if out := a.want(200, "GET", "/api/v1/search?q=deployment", nil); !strings.Contains(out, "Deployment guide") {
		t.Fatalf("title search: %s", out)
	}
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if out := a.want(200, "GET", "/api/v1/search?q=fietsenstalling&since="+future, nil); strings.Contains(out, "Runbook") {
		t.Fatalf("date filter: %s", out)
	}
	if out := a.want(200, "GET", "/api/v1/search?q=fietsenstalling&author_id=00000000-0000-0000-0000-000000000001", nil); strings.Contains(out, "Runbook") {
		t.Fatalf("author filter: %s", out)
	}
	for _, q := range []string{"'", "\\", "a & | !", "(", ":*", "%27%3B+DROP+TABLE+pages"} {
		a.want(200, "GET", "/api/v1/search?q="+url.QueryEscape(q), nil)
	}
	out := a.want(200, "GET", "/api/v1/search?q=fietsenstalling", nil)
	if !strings.Contains(out, "[[[fietsenstalling]]]") {
		t.Fatalf("snippet: %s", out)
	}
}

func (u *user) importFile(space, name string, body []byte) map[string]any {
	u.e.t.Helper()
	req, _ := http.NewRequest("POST", u.e.srv.URL+"/api/v1/spaces/"+space+"/imports?filename="+url.QueryEscape(name), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := u.c.Do(req)
	if err != nil {
		u.e.t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 202 {
		u.e.t.Fatalf("import: %d %s", resp.StatusCode, b)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if _, err := u.e.app.Jobs().RunOnce(context.Background()); err != nil {
		u.e.t.Fatal(err)
	}
	return u.ok("GET", "/api/v1/jobs/"+m["job_id"].(string), nil)
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte(content))
	}
	_ = w.Close()
	return b.Bytes()
}

func TestMarkdownExportImportRoundTrip(t *testing.T) {
	e := newEnv(t)
	a := e.user("a@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := a.space("ENG")
	parent := a.page(space, nil, "Guide", "Parent text")
	child := a.page(space, parent, "Details", "Child text")
	if status, _ := a.upload(child, "pixel.png", pngHeader); status != 201 {
		t.Fatal("upload failed")
	}

	resp, md := e.do(a.c, "GET", "/api/v1/pages/"+parent+"/export.md", "")
	if resp.StatusCode != 200 || !strings.Contains(md, "Parent text") || !strings.Contains(resp.Header.Get("Content-Disposition"), "Guide.md") {
		t.Fatalf("export md: %d %s %v", resp.StatusCode, md, resp.Header)
	}
	resp, archive := e.do(a.c, "GET", "/api/v1/spaces/"+space+"/export.zip", "")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("export zip: %d %v", resp.StatusCode, resp.Header)
	}
	if e.auditActions("space.exported") != 1 {
		t.Fatal("exports must be audited")
	}

	target, _ := a.space("COPY")
	job := a.importFile(target, "export.zip", []byte(archive))
	if job["status"] != jobs.StatusDone {
		t.Fatalf("import: %+v", job)
	}
	tree := a.ok("GET", "/api/v1/spaces/"+target+"/tree", nil)["pages"].([]any)
	byTitle := map[string]map[string]any{}
	for _, n := range tree {
		m := n.(map[string]any)
		byTitle[m["title"].(string)] = m
	}
	if byTitle["Guide"] == nil || byTitle["Details"] == nil || byTitle["Details"]["parent_id"] != byTitle["Guide"]["id"] {
		t.Fatalf("imported tree: %+v", tree)
	}
	p := a.ok("GET", "/api/v1/pages/"+byTitle["Details"]["id"].(string), nil)
	if !strings.Contains(fmt.Sprint(p["document"]), "Child text") {
		t.Fatalf("imported content: %+v", p)
	}

	// A single file works too, and images inside a zip become attachments.
	job = a.importFile(target, "Note.md", []byte("# Hello\n\nworld\n"))
	if job["status"] != jobs.StatusDone {
		t.Fatalf("md import: %+v", job)
	}
	job = a.importFile(target, "pics.zip", zipOf(t, map[string]string{"Pics.md": "![a](img/a.png)\n", "img/a.png": pngHeader}))
	result := job["result"].(map[string]any)
	if job["status"] != jobs.StatusDone || result["files"] != float64(1) {
		t.Fatalf("zip import: %+v", job)
	}
}

func TestFileInAPageSurvivesCopyExportAndImport(t *testing.T) {
	e := newEnv(t)
	a := e.user("a@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := a.space("ENG")
	page := a.page(space, nil, "Guide", "Intro")
	status, out := a.upload(page, "plan.txt", "the plan")
	if status != 201 {
		t.Fatalf("upload: %d %s", status, out)
	}
	var file struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &file); err != nil {
		t.Fatal(err)
	}
	withFile := map[string]any{"type": "doc", "content": []any{map[string]any{
		"type": "file", "attrs": map[string]any{"attachment_id": file.ID, "filename": "plan.txt"}}}}
	a.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Guide", "document": withFile, "base_version": 1})

	copied := a.ok("POST", "/api/v1/pages/"+page+"/copy", map[string]any{})
	duplicate := fmt.Sprint(a.ok("GET", "/api/v1/pages/"+copied["id"].(string), nil)["document"])
	if !strings.Contains(duplicate, "plan.txt") || strings.Contains(duplicate, file.ID) {
		t.Fatalf("a copy must point at its own file: %s", duplicate)
	}

	resp, md := e.do(a.c, "GET", "/api/v1/pages/"+page+"/export.md", "")
	if resp.StatusCode != 200 || !strings.Contains(md, "[plan.txt](plan.txt)") {
		t.Fatalf("export md: %d %s", resp.StatusCode, md)
	}

	resp, archive := e.do(a.c, "GET", "/api/v1/spaces/"+space+"/export.zip", "")
	if resp.StatusCode != 200 {
		t.Fatalf("export zip: %d", resp.StatusCode)
	}
	target, _ := a.space("COPY")
	if job := a.importFile(target, "export.zip", []byte(archive)); job["status"] != jobs.StatusDone {
		t.Fatalf("import: %+v", job)
	}
	var imported string
	for _, n := range a.ok("GET", "/api/v1/spaces/"+target+"/tree", nil)["pages"].([]any) {
		if m := n.(map[string]any); m["title"] == "Guide" {
			imported = m["id"].(string)
		}
	}
	if got := fmt.Sprint(a.ok("GET", "/api/v1/pages/"+imported, nil)["document"]); !strings.Contains(got, "type:file") {
		t.Fatalf("the file left the page: %s", got)
	}
	files := a.ok("GET", "/api/v1/pages/"+imported+"/attachments", nil)["attachments"].([]any)
	if len(files) != 1 || files[0].(map[string]any)["filename"] != "plan.txt" {
		t.Fatalf("imported attachments: %+v", files)
	}
}

func TestImportKeepsLinksBetweenPagesAsLinks(t *testing.T) {
	e := newEnv(t)
	a := e.user("a@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := a.space("ENG")
	job := a.importFile(space, "pages.zip", zipOf(t, map[string]string{"A.md": "[to b](B.md)\n", "B.md": "text\n"}))
	if result := job["result"].(map[string]any); job["status"] != jobs.StatusDone || result["files"] != float64(0) {
		t.Fatalf("a page must not become an attachment of another page: %+v", job)
	}
}

func TestImportRefusesHostileArchives(t *testing.T) {
	e := newEnv(t)
	a := e.user("a@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	viewer := e.user("viewer@example.com", identity.PermUseProduct)
	space, _ := a.space("ENG")
	a.grant(space, "user", viewer.id, permissions.TemplateViewer)

	// Paths that climb out are skipped and reported; they never reach a disk.
	job := a.importFile(space, "evil.zip", zipOf(t, map[string]string{"../../etc/passwd.md": "evil", "ok.md": "fine"}))
	if job["status"] != jobs.StatusDone || job["result"].(map[string]any)["pages"] != float64(1) {
		t.Fatalf("traversal: %+v", job)
	}
	if !strings.Contains(fmt.Sprint(job["result"]), "unsafe path") {
		t.Fatalf("the skipped path must be reported: %+v", job)
	}

	// A zip bomb: tiny archive, huge content. Refused while reading.
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, _ := w.Create("bomb.md")
	zeros := make([]byte, 1<<20)
	for range 25 { // 25 MiB of zeros, over the limit for one Markdown file
		_, _ = f.Write(zeros)
	}
	_ = w.Close()
	if b.Len() > 1<<20 {
		t.Fatalf("test archive is not a bomb: %d bytes", b.Len())
	}
	job = a.importFile(space, "bomb.zip", b.Bytes())
	if job["status"] != jobs.StatusFailed {
		t.Fatalf("bomb: %+v", job)
	}

	job = a.importFile(space, "broken.zip", []byte("this is not a zip"))
	if job["status"] != jobs.StatusFailed {
		t.Fatalf("broken: %+v", job)
	}

	// Permissions apply to imports like everything else.
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/spaces/"+space+"/imports?filename=a.md", strings.NewReader("x"))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := viewer.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("a viewer may not import: %d", resp.StatusCode)
	}
}

func TestPeriodicTasksRunOnceAcrossWorkers(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	var mu sync.Mutex
	runs := 0
	task := jobs.Task{Name: "test-task", Every: time.Hour, Run: func(context.Context) error {
		mu.Lock()
		runs++
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		return nil
	}}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.app.Jobs().Tick(ctx, []jobs.Task{task})
		}()
	}
	wg.Wait()
	if runs != 1 {
		t.Fatalf("task ran %d times, want 1", runs)
	}
	e.app.Jobs().Tick(ctx, []jobs.Task{task})
	if runs != 1 {
		t.Fatal("a task must wait for its interval")
	}
}

// objectRoute fills in a route pattern with real ids.
func objectRoute(pattern string, ids map[string]string) (method, path string) {
	method, path, _ = strings.Cut(pattern, " ")
	id := ids["page"]
	switch {
	case strings.Contains(path, "/spaces/"):
		id = ids["space"]
	case strings.Contains(path, "/rules/"):
		id = ids["rule"]
	case strings.Contains(path, "/webhooks/"):
		id = ids["webhook"]
	case strings.Contains(path, "/sites/"):
		id = ids["site"]
	case strings.Contains(path, "/attachments/"):
		id = ids["attachment"]
	case strings.Contains(path, "/jobs/"):
		id = ids["job"]
	case strings.Contains(path, "/trash/"):
		id = ids["trash"]
	case strings.Contains(path, "/comments/{id}"):
		id = ids["comment"]
	case strings.Contains(path, "/tasks/"):
		id = ids["task"]
	case strings.Contains(path, "/access-requests/{id}"):
		id = ids["request"]
	}
	path = strings.ReplaceAll(path, "{id}", id)
	path = strings.ReplaceAll(path, "{key}", "ENG")
	path = strings.ReplaceAll(path, "{number}", "1")
	path = strings.ReplaceAll(path, "{label}", "x")
	path = strings.ReplaceAll(path, "{alias}", "x")
	path = strings.ReplaceAll(path, "{emoji}", url.PathEscape("👍"))
	path = strings.ReplaceAll(path, "{user}", ids["owner"])
	return method, path
}

// validRequest makes a request the server would accept from someone with rights,
// so that a refusal can only come from the permission check.
func validRequest(pattern, path string, ids map[string]string) (string, string) {
	doc := `{"type":"doc","content":[{"type":"paragraph"}]}`
	subject := `{"type":"user","id":"` + ids["owner"] + `"}`
	switch {
	case strings.HasPrefix(pattern, "POST /api/v1/sites/"):
		return path, "{}"
	case pattern == "POST /api/v1/exports":
		return path, `{"space_id":"` + ids["space"] + `","format":"text"}`
	case strings.HasSuffix(pattern, "/compare"):
		return path + "?from=1&to=1", ""
	case strings.HasSuffix(pattern, "/resend"):
		return strings.Replace(path, "{delivery}", ids["owner"], 1), ""
	case strings.HasSuffix(pattern, "/presence"):
		return path, `{"mode":"reading"}`
	case pattern == "POST /api/v1/rules/{id}/run":
		return path, `{"page_id":"` + ids["page"] + `"}`
	case pattern == "PUT /api/v1/rules/{id}":
		return path, `{"name":"Walk","definition":{"trigger":{"kind":"incoming"},"actions":[{"type":"find_pages"}]}}`
	case pattern == "POST /api/v1/spaces/{id}/rules":
		return path, `{"name":"Walk","definition":{"trigger":{"kind":"incoming"},"actions":[{"type":"find_pages"}]}}`
	case strings.HasSuffix(pattern, "/explain"):
		return path + "?action=page.view&user_id=" + ids["owner"], ""
	case strings.HasSuffix(pattern, "/imports"):
		return path + "?filename=a.md", "# hello"
	case pattern == "POST /api/v1/pages/{id}/attachments":
		return path + "?filename=b.txt", "hello"
	case pattern == "POST /api/v1/pages":
		return path, `{"space_id":"` + ids["space"] + `","title":"x"}`
	case strings.HasSuffix(pattern, "/publish"):
		return path, `{"title":"x","base_version":1,"document":` + doc + `}`
	case strings.HasSuffix(pattern, "/draft") && strings.HasPrefix(pattern, "PUT"):
		return path, `{"title":"x","base_version":1,"document":` + doc + `}`
	case pattern == "PATCH /api/v1/pages/{id}":
		return path, `{"title":"x"}`
	case pattern == "PATCH /api/v1/spaces/{id}":
		return path, `{"name":"x"}`
	case pattern == "POST /api/v1/pages/{id}/approvals":
		return path, `{"user_ids":["` + ids["owner"] + `"]}`
	case strings.HasSuffix(pattern, "/kind"):
		return path, `{"kind":"folder"}`
	case pattern == "POST /api/v1/pages/{id}/labels":
		return path, `{"label":"x"}`
	case strings.HasSuffix(pattern, "/sort-children"):
		return path, `{"by":"title"}`
	case strings.Contains(pattern, "/content/"):
		return path, `{"action":"archive","page_ids":["` + ids["page"] + `"]}`
	case strings.HasSuffix(pattern, "/icon") && strings.HasPrefix(pattern, "POST"):
		return path, pngHeader
	case strings.HasSuffix(pattern, "/labels/rename"):
		return path, `{"from":"a","to":"b"}`
	case strings.HasSuffix(pattern, "/schedule") && strings.HasPrefix(pattern, "PUT"):
		return path, `{"at":"2099-01-01T00:00:00Z"}`
	case strings.HasSuffix(pattern, "/verification") && strings.HasPrefix(pattern, "PUT"):
		return path, `{"until":"2099-01-01"}`
	case strings.HasSuffix(pattern, "/key"):
		return path, `{"key":"FREEKEY"}`
	case strings.HasSuffix(pattern, "/title-align"):
		return path, `{"align":"center"}`
	case strings.HasSuffix(pattern, "/trash/bulk"):
		return path, `{"action":"restore","ids":["` + ids["trash"] + `"]}`
	case pattern == "POST /api/v1/spaces/{id}/templates" || strings.HasSuffix(pattern, "/templates/import"):
		return path, `{"name":"x"}`
	case pattern == "POST /api/v1/pages/{id}/comments" || pattern == "PATCH /api/v1/comments/{id}":
		return path, `{"body":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x"}]}]}}`
	case pattern == "PATCH /api/v1/attachments/{id}":
		return path, `{"filename":"renamed.txt"}`
	case strings.HasSuffix(pattern, "/look"):
		return path, `{"title_align":"start","header_position":50}`
	case strings.HasSuffix(pattern, "/shortcuts") && strings.HasPrefix(pattern, "PUT"):
		return path, `{"shortcuts":[]}`
	case strings.HasSuffix(pattern, "/grants") && strings.HasPrefix(pattern, "PUT"):
		return path, `{"subject":` + subject + `,"permissions":["space.view"]}`
	case strings.HasSuffix(pattern, "/preset"):
		return path, `{"preset":"team_space"}`
	case strings.HasSuffix(pattern, "/restrictions") && strings.HasPrefix(pattern, "PUT"):
		return path, `{"kind":"edit","subjects":[]}`
	}
	if m, _, _ := strings.Cut(pattern, " "); m == "POST" || m == "PUT" || m == "PATCH" {
		return path, "{}"
	}
	return path, ""
}

func contentTypeFor(pattern string) string {
	if strings.HasSuffix(pattern, "/attachments") || strings.HasSuffix(pattern, "/imports") {
		return "application/octet-stream"
	}
	return "application/json"
}

// TestEveryObjectRouteIsProtected walks the real route table, so a new route is
// covered the moment it is added: a stranger gets 404 and a viewer gets 403 on
// everything that changes something.
func TestEveryObjectRouteIsProtected(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	viewer := e.user("viewer@example.com", identity.PermUseProduct)
	stranger := e.user("stranger@example.com", identity.PermUseProduct)

	space, _ := owner.space("ENG")
	owner.grant(space, "user", viewer.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")
	gone := owner.page(space, nil, "Gone", "x")
	_, body := owner.upload(page, "a.txt", "hello")
	var att map[string]any
	_ = json.Unmarshal([]byte(body), &att)
	owner.ok("DELETE", "/api/v1/pages/"+gone, nil)
	trash := owner.ok("GET", "/api/v1/spaces/"+space+"/trash", nil)["items"].([]any)[0].(map[string]any)["id"].(string)
	job := owner.importFile(space, "a.md", []byte("x"))

	comment := owner.comment(page, "x")
	tasked := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Tasks"})["id"].(string)
	owner.publish(tasked, "Tasks", 0, taskDoc(map[string]any{"checked": false}, "Do it"))
	task := tasksOf(owner, "tab=created")[0]["id"].(string)
	viewer.ok("POST", "/api/v1/pages/"+page+"/access-requests", map[string]any{"kind": "edit"})
	var request string
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT id FROM access_requests").Scan(&request); err != nil {
		t.Fatal(err)
	}

	rule := owner.rule(space, "Walk", map[string]any{"trigger": map[string]any{"kind": "incoming"}, "actions": []map[string]any{{"type": "find_pages"}}})
	webhook := uuid.NewString()
	e.sql(`INSERT INTO webhook_endpoints (id, space_id, address, events, secret, owner_id) VALUES ($1, $2, $3, $4, $5, $6)`,
		webhook, space, "https://hooks.example.com/x", []string{"page.created"}, []byte{1}, owner.id)

	// Sites are off here, and the route must refuse before it says so.
	site := "018f0000-0000-7000-8000-00000000abcd"
	if _, err := e.app.Pool().Exec(context.Background(), `INSERT INTO public_sites (id, space_id, name, target, created_by) VALUES ($1, $2, 'x', 'x', $3)`, site, space, owner.id); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{"space": space, "page": page, "attachment": att["id"].(string), "job": job["id"].(string), "trash": trash, "owner": owner.id,
		"comment": comment, "task": task, "request": request, "site": site, "rule": rule, "webhook": webhook}

	// Routes that are personal or list only what the caller may see.
	listOnly := map[string]bool{"GET /api/v1/spaces": true, "GET /api/v1/search": true}
	// A viewer can do these by design.
	viewerMay := map[string]bool{
		"space.view": true, "page.view": true, "space.export": true, "comment.add": true,
		// Group owners are decided in the module: everyone else gets "not found".
		"group.manage_members": true,
	}
	checked := 0
	for _, r := range e.app.Routes() {
		rule := r.Access.String()
		if !strings.HasPrefix(rule, "object:") || listOnly[r.Pattern] {
			continue
		}
		method, path := objectRoute(r.Pattern, ids)
		path, body := validRequest(r.Pattern, path, ids)
		checked++
		resp, out := e.do(stranger.c, method, path, body, "Content-Type", contentTypeFor(r.Pattern))
		if resp.StatusCode != 404 {
			t.Errorf("%s as a stranger: %d, want 404: %s", r.Pattern, resp.StatusCode, out)
		}
		action := strings.TrimPrefix(rule, "object:")
		if viewerMay[action] || action == "job.owner" {
			continue
		}
		resp, out = e.do(viewer.c, method, path, body, "Content-Type", contentTypeFor(r.Pattern))
		if resp.StatusCode != 403 {
			t.Errorf("%s as a viewer: %d, want 403: %s", r.Pattern, resp.StatusCode, out)
		}
	}
	if checked < 30 {
		t.Fatalf("only %d routes were walked", checked)
	}
	// The space list and search must simply not show anything to a stranger.
	if out := stranger.want(200, "GET", "/api/v1/spaces", nil); strings.Contains(out, "ENG") {
		t.Fatalf("stranger sees the space: %s", out)
	}
}

func (u *user) putFont(name string, body []byte) (int, string) {
	u.e.t.Helper()
	req, _ := http.NewRequest("PUT", u.e.srv.URL+"/api/v1/admin/appearance/font?filename="+url.QueryEscape(name), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := u.c.Do(req)
	if err != nil {
		u.e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}

// The system font is the default. An administrator can import a house font, and
// the login page must be able to load it before anyone is logged in.
func TestAdministratorImportsReplacesAndRemovesAFont(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	member := e.user("member@example.com", identity.PermUseProduct)
	anonymous := e.client()

	if resp, body := e.do(anonymous, "GET", "/api/v1/appearance", ""); resp.StatusCode != 200 || !strings.Contains(body, `"font":null`) {
		t.Fatalf("default: %d %s", resp.StatusCode, body)
	}
	if resp, _ := e.do(anonymous, "GET", "/api/v1/appearance/font", ""); resp.StatusCode != 404 {
		t.Fatalf("no font yet: %d", resp.StatusCode)
	}

	font := append([]byte("wOF2"), bytes.Repeat([]byte{1}, 512)...)
	if status, _ := member.putFont("house.woff2", font); status != http.StatusForbidden {
		t.Fatalf("a member may not import a font: %d", status)
	}
	if status, _ := admin.putFont("trick.woff2", []byte("<html><script>alert(1)</script>")); status != http.StatusBadRequest {
		t.Fatalf("a file that is not a font: %d", status)
	}
	if status, _ := admin.putFont("huge.woff2", append([]byte("wOF2"), make([]byte, 2<<20)...)); status != http.StatusRequestEntityTooLarge {
		t.Fatalf("a font over the limit: %d", status)
	}
	if status, body := admin.putFont("../../House Sans.woff2", font); status != 200 || !strings.Contains(body, `"name":"House Sans.woff2"`) {
		t.Fatalf("import: %d %s", status, body)
	}

	resp, body := e.do(anonymous, "GET", "/api/v1/appearance/font", "")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "font/woff2" || resp.Header.Get("X-Content-Type-Options") != "nosniff" || body != string(font) {
		t.Fatalf("serving: %d %v", resp.StatusCode, resp.Header)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("an address without the version must not be cached for long: %q", got)
	}
	resp, body = e.do(anonymous, "GET", "/api/v1/appearance", "")
	if resp.StatusCode != 200 || strings.Contains(body, "object_key") || !strings.Contains(body, "House Sans") {
		t.Fatalf("appearance: %s", body)
	}
	var appearance struct {
		Font struct {
			Version string `json:"version"`
		} `json:"font"`
	}
	if err := json.Unmarshal([]byte(body), &appearance); err != nil {
		t.Fatal(err)
	}
	if resp, _ := e.do(anonymous, "GET", "/api/v1/appearance/font?v="+appearance.Font.Version, ""); !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("the versioned address can be kept: %v", resp.Header)
	}

	// A second font replaces the first, whose file goes away.
	if status, _ := admin.putFont("Other.woff", append([]byte("wOFF"), 1, 2, 3)); status != 200 {
		t.Fatalf("replace: %d", status)
	}
	var queued int
	_ = e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM storage_deletions").Scan(&queued)
	if queued != 1 {
		t.Fatalf("old font file queued: %d", queued)
	}

	admin.want(204, "DELETE", "/api/v1/admin/appearance/font", nil)
	if resp, body := e.do(anonymous, "GET", "/api/v1/appearance", ""); resp.StatusCode != 200 || !strings.Contains(body, `"font":null`) {
		t.Fatalf("after removing: %s", body)
	}
	if e.auditActions("settings.font_changed") != 3 || e.outboxCount("settings.font_changed") != 3 {
		t.Fatal("font changes must be audited and published")
	}
}

func TestPageNamesItsAuthors(t *testing.T) {
	e := newEnv(t)
	a := e.user("a@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := a.space("ENG")
	id := a.page(space, nil, "Page", "x")
	p := a.ok("GET", "/api/v1/pages/"+id, nil)
	if p["created_by_name"] != "Test a@example.com" || p["updated_by_name"] != "Test a@example.com" {
		t.Fatalf("names: %+v", p)
	}
}
