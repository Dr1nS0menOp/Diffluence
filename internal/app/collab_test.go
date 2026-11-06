// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

const collabToken = "a-long-service-token"

type collabTeam struct {
	e       *env
	editor  *user
	reader  *user
	space   string
	page    string
	checked *atomic.Int32
}

func newCollabTeam(t *testing.T) collabTeam {
	t.Helper()
	checked := &atomic.Int32{}
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/recheck" && r.Header.Get("Authorization") == "Bearer "+collabToken {
			checked.Add(1)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(service.Close)
	e := newEnvWith(t, func(c *config.Config) { c.CollabURL, c.CollabToken = service.URL, collabToken })
	editor := e.user("editor@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := editor.space("COL")
	editor.grant(space, "user", reader.id, permissions.TemplateViewer)
	return collabTeam{e: e, editor: editor, reader: reader, space: space, page: editor.page(space, nil, "Shared", "x"), checked: checked}
}

func (c collabTeam) service(method, path, body string) (int, string) {
	c.e.t.Helper()
	resp, out := c.e.do(c.e.client(), method, path, body, "Authorization", "Bearer "+collabToken, "Content-Type", "application/json")
	return resp.StatusCode, out
}

func (u *user) sessionCookie() string {
	u.e.t.Helper()
	target, err := http.NewRequest("GET", u.e.srv.URL, nil)
	if err != nil {
		u.e.t.Fatal(err)
	}
	parts := []string{}
	for _, c := range u.c.Jar.Cookies(target.URL) {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

func (c collabTeam) authorize(u *user) map[string]any {
	c.e.t.Helper()
	body, err := json.Marshal(map[string]any{"page_id": c.page, "cookie": u.sessionCookie()})
	if err != nil {
		c.e.t.Fatal(err)
	}
	status, out := c.service("POST", "/collab/authorize", string(body))
	if status != 200 {
		c.e.t.Fatalf("status %d: %s", status, out)
	}
	var answer map[string]any
	if err := json.Unmarshal([]byte(out), &answer); err != nil {
		c.e.t.Fatal(err)
	}
	return answer
}

func TestOnlySomeoneWhoMayEditCanConnect(t *testing.T) {
	c := newCollabTeam(t)
	if got := c.authorize(c.editor); got["allowed"] != true || got["user_id"] != c.editor.id {
		t.Fatalf("got %v", got)
	}
	if got := c.authorize(c.reader); got["allowed"] != false {
		t.Fatalf("a reader was let in: %v", got)
	}
	status, out := c.service("POST", "/collab/authorize", `{"page_id":"`+c.page+`","cookie":"diffluence_session=wrong"}`)
	if status != 200 || strings.Contains(out, `"allowed":true`) {
		t.Fatalf("a wrong session got %d %s", status, out)
	}
}

func TestTheServiceRoutesNeedTheToken(t *testing.T) {
	c := newCollabTeam(t)
	resp, _ := c.e.do(c.e.client(), "GET", "/collab/documents/"+c.page, "", "Authorization", "Bearer wrong")
	if resp.StatusCode != 404 {
		t.Fatalf("a wrong token got %d", resp.StatusCode)
	}
	resp, _ = c.e.do(c.e.client(), "PUT", "/collab/documents/"+c.page, `{}`)
	if resp.StatusCode != 404 {
		t.Fatalf("no token got %d", resp.StatusCode)
	}
}

func TestWithoutTheServiceEveryPageKeepsPrivateDrafts(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("PRV")
	page := owner.page(space, nil, "Mine", "x")
	if got := owner.ok("GET", "/api/v1/pages/"+page+"/collab", nil); got["enabled"] != false {
		t.Fatalf("got %v", got)
	}
	resp, _ := e.do(e.client(), "POST", "/collab/authorize", `{}`, "Authorization", "Bearer ")
	if resp.StatusCode != 404 {
		t.Fatalf("an empty token got %d", resp.StatusCode)
	}
}

func TestAReaderIsToldNotToJoin(t *testing.T) {
	c := newCollabTeam(t)
	if got := c.reader.ok("GET", "/api/v1/pages/"+c.page+"/collab", nil); got["enabled"] != false {
		t.Fatalf("got %v", got)
	}
	if got := c.editor.ok("GET", "/api/v1/pages/"+c.page+"/collab", nil); got["enabled"] != true {
		t.Fatalf("got %v", got)
	}
}

func TestAHostileDocumentIsNotStored(t *testing.T) {
	c := newCollabTeam(t)
	hostile := `{"state":"AAA=","title":"x","document":{"type":"doc","content":[{"type":"script"}]},"authors":[]}`
	if status, out := c.service("PUT", "/collab/documents/"+c.page, hostile); status != 422 {
		t.Fatalf("status %d: %s", status, out)
	}
	if c.e.count("SELECT count(*) FROM collab_documents") != 0 {
		t.Fatal("a refused state was kept")
	}
}

func TestAGoodDocumentComesBackAsItWent(t *testing.T) {
	c := newCollabTeam(t)
	if status, _ := c.service("GET", "/collab/documents/"+c.page, ""); status != 204 {
		t.Fatalf("a new document got %d", status)
	}
	good := `{"state":"AQID","title":"x","document":{"type":"doc","content":[{"type":"paragraph"}]},"authors":["` + c.reader.id + `"]}`
	if status, out := c.service("PUT", "/collab/documents/"+c.page, good); status != 204 {
		t.Fatalf("status %d: %s", status, out)
	}
	status, out := c.service("GET", "/collab/documents/"+c.page, "")
	if status != 200 || out != "\x01\x02\x03" {
		t.Fatalf("status %d: %q", status, out)
	}
}

func TestAVersionNamesEveryoneWhoTookPart(t *testing.T) {
	c := newCollabTeam(t)
	body := `{"state":"AQID","title":"x","document":{"type":"doc","content":[{"type":"paragraph"}]},"authors":["` + c.reader.id + `"]}`
	if status, out := c.service("PUT", "/collab/documents/"+c.page, body); status != 204 {
		t.Fatalf("status %d: %s", status, out)
	}
	c.editor.publish(c.page, "Shared", 1, map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph"}}})
	authors := c.editor.ok("GET", "/api/v1/pages/"+c.page+"/version-authors", nil)["authors"].(map[string]any)
	if got := authors["2"].([]any); len(got) != 2 {
		t.Fatalf("authors %v", got)
	}
	if c.e.count("SELECT cardinality(authors) FROM collab_documents WHERE page_id = $1", c.page) != 0 {
		t.Fatal("the count of authors did not start anew")
	}
}

func TestAChangeOfRightsMakesTheServiceCheckAgain(t *testing.T) {
	c := newCollabTeam(t)
	c.e.settle()
	before := c.checked.Load()
	c.editor.restrict(c.page, "view", c.editor.id)
	c.e.settle()
	if c.checked.Load() <= before {
		t.Fatal("the service was not told")
	}
}

func TestASocketFromAnotherSiteIsRefused(t *testing.T) {
	c := newCollabTeam(t)
	resp, _ := c.e.do(c.e.client(), "GET", "/collab", "", "Origin", "https://evil.example")
	if resp.StatusCode != 403 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
