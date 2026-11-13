// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

const collabToken = "a-long-service-token"

// ticketHeader carries the ticket from the API to the service (spec 067).
const ticketHeader = "X-Diffluence-Collab-Ticket"

type collabTeam struct {
	e       *env
	editor  *user
	reader  *user
	space   string
	page    string
	checked *atomic.Int32
	// reached holds the headers of the last socket the API passed on to the service.
	reached *atomic.Pointer[http.Header]
}

func newCollabTeam(t *testing.T) collabTeam {
	t.Helper()
	checked := &atomic.Int32{}
	reached := &atomic.Pointer[http.Header]{}
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/recheck" && r.Header.Get("Authorization") == "Bearer "+collabToken {
			checked.Add(1)
		}
		if r.URL.Path == "/" {
			h := r.Header.Clone()
			reached.Store(&h)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(service.Close)
	e := newEnvWith(t, func(c *config.Config) { c.CollabURL, c.CollabToken = service.URL, collabToken })
	editor := e.user("editor@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := editor.space("COL")
	editor.grant(space, "user", reader.id, permissions.TemplateViewer)
	return collabTeam{e: e, editor: editor, reader: reader, space: space, page: editor.page(space, nil, "Shared", "x"),
		checked: checked, reached: reached}
}

func (c collabTeam) service(method, path, body string) (int, string) {
	c.e.t.Helper()
	resp, out := c.e.do(c.e.client(), method, path, body, "Authorization", "Bearer "+collabToken, "Content-Type", "application/json")
	return resp.StatusCode, out
}

// connect opens the socket of a page as the editor in a browser does, and says what the
// service was given. A nil header means the API did not pass the socket on.
func (c collabTeam) connect(client *http.Client, page string, headers ...string) (int, http.Header) {
	c.e.t.Helper()
	c.reached.Store(nil)
	headers = append([]string{"Origin", "http://localhost"}, headers...)
	resp, _ := c.e.do(client, "GET", "/collab?page="+page, "", headers...)
	got := c.reached.Load()
	if got == nil {
		return resp.StatusCode, nil
	}
	return resp.StatusCode, *got
}

// ticketOf is the ticket the API gives a person's socket on the page of the team.
func (c collabTeam) ticketOf(u *user) string {
	c.e.t.Helper()
	status, reached := c.connect(u.c, c.page)
	if reached == nil || reached.Get(ticketHeader) == "" {
		c.e.t.Fatalf("the socket of %s was not passed on with a ticket: %d", u.id, status)
	}
	return reached.Get(ticketHeader)
}

// ask is the service asking about a ticket on a page.
func (c collabTeam) ask(page, ticket string) map[string]any {
	c.e.t.Helper()
	body, err := json.Marshal(map[string]any{"page_id": page, "ticket": ticket})
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

func (c collabTeam) authorize(u *user) map[string]any {
	c.e.t.Helper()
	return c.ask(c.page, c.ticketOf(u))
}

func TestOnlySomeoneWhoMayEditCanConnect(t *testing.T) {
	c := newCollabTeam(t)
	if got := c.authorize(c.editor); got["allowed"] != true || got["user_id"] != c.editor.id {
		t.Fatalf("got %v", got)
	}
	if status, reached := c.connect(c.reader.c, c.page); status != 404 || reached != nil {
		t.Fatalf("a reader's socket got %d, passed on: %v", status, reached != nil)
	}
	if status, reached := c.connect(c.e.client(), c.page); status != 401 || reached != nil {
		t.Fatalf("a socket without a session got %d, passed on: %v", status, reached != nil)
	}
	if got := c.ask(c.page, "wrong"); got["allowed"] != false {
		t.Fatalf("a wrong ticket got %v", got)
	}
}

func TestTheServiceGetsATicketAndNeverTheCookie(t *testing.T) {
	c := newCollabTeam(t)
	status, reached := c.connect(c.editor.c, c.page, ticketHeader, "made-up", "Authorization", "Bearer "+collabToken)
	if status != 202 || reached == nil {
		t.Fatalf("the socket got %d", status)
	}
	if got := reached.Get("Cookie"); got != "" {
		t.Fatalf("the service got the cookie %q", got)
	}
	if got := reached.Get("Authorization"); got != "" {
		t.Fatalf("the service got the browser's Authorization %q", got)
	}
	if got := reached.Get(ticketHeader); got == "" || got == "made-up" {
		t.Fatalf("the ticket is %q", got)
	}
	if got := c.ask(c.page, reached.Get(ticketHeader)); got["allowed"] != true {
		t.Fatalf("the ticket it got was refused: %v", got)
	}
}

func TestATokenDoesNotOpenASocket(t *testing.T) {
	c := newCollabTeam(t)
	ends := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	secret := c.editor.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "script", "can_write": true, "expires_at": ends})["secret"].(string)
	if status, reached := c.connect(c.e.client(), c.page, "Authorization", "Bearer "+secret); status != 404 || reached != nil {
		t.Fatalf("a token's socket got %d, passed on: %v", status, reached != nil)
	}
}

func TestASocketNeedsAPageItMayEdit(t *testing.T) {
	c := newCollabTeam(t)
	for _, page := range []string{"", "not-a-uuid", "00000000-0000-4000-8000-000000000000"} {
		if status, reached := c.connect(c.editor.c, page); status != 404 || reached != nil {
			t.Errorf("page %q got %d, passed on: %v", page, status, reached != nil)
		}
	}
}

func TestATicketOpensOnlyItsOwnPage(t *testing.T) {
	c := newCollabTeam(t)
	other := c.editor.page(c.space, nil, "Other", "y")
	if got := c.ask(other, c.ticketOf(c.editor)); got["allowed"] != false {
		t.Fatalf("a ticket for one page opened another: %v", got)
	}
}

func TestAForgedTicketIsRefused(t *testing.T) {
	c := newCollabTeam(t)
	good := c.ticketOf(c.editor)
	payload, signature, _ := strings.Cut(good, ".")
	readerTicket := strings.Replace(mustDecode(t, payload), c.editor.id, c.reader.id, 1)
	forged := base64.RawURLEncoding.EncodeToString([]byte(readerTicket)) + "." + signature
	if got := c.ask(c.page, forged); got["allowed"] != false {
		t.Fatalf("a ticket with another person in it was taken: %v", got)
	}
	if got := c.ask(c.page, payload+"."+signature[1:]); got["allowed"] != false {
		t.Fatalf("a damaged signature was taken: %v", got)
	}
}

func mustDecode(t *testing.T, s string) string {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAnOpenEditorDoesNotKeepTheSessionAlive(t *testing.T) {
	c := newCollabTeam(t)
	ticket := c.ticketOf(c.editor)
	c.e.sql("UPDATE sessions SET last_seen_at = now() - interval '50 minutes' WHERE user_id = $1", c.editor.id)
	got := c.ask(c.page, ticket)
	if got["allowed"] != true {
		t.Fatalf("a live session was refused: %v", got)
	}
	if c.e.count("SELECT count(*) FROM sessions WHERE user_id = $1 AND last_seen_at < now() - interval '49 minutes'", c.editor.id) != 1 {
		t.Fatal("asking about a ticket counted as activity")
	}
	renewed, _ := got["ticket"].(string)
	if renewed == "" {
		t.Fatalf("no renewed ticket: %v", got)
	}
	// Past the idle limit of an hour, the session ends while the editor is still open.
	c.e.sql("UPDATE sessions SET last_seen_at = now() - interval '61 minutes' WHERE user_id = $1", c.editor.id)
	if got := c.ask(c.page, renewed); got["allowed"] != false {
		t.Fatalf("an idle session was kept: %v", got)
	}
}

func TestATicketEndsWithItsSession(t *testing.T) {
	c := newCollabTeam(t)
	ticket := c.ticketOf(c.editor)
	c.editor.ok("POST", "/api/v1/auth/logout", nil)
	if got := c.ask(c.page, ticket); got["allowed"] != false {
		t.Fatalf("a ticket outlived its session: %v", got)
	}
}

func TestATicketEndsWhenTheRightToEditEnds(t *testing.T) {
	c := newCollabTeam(t)
	writer := c.e.user("writer@example.com", identity.PermUseProduct)
	c.editor.grant(c.space, "user", writer.id, permissions.TemplateEditor)
	ticket := c.ticketOf(writer)
	if got := c.ask(c.page, ticket); got["allowed"] != true {
		t.Fatalf("a writer was refused before the change: %v", got)
	}
	c.editor.restrict(c.page, "edit", c.editor.id)
	if got := c.ask(c.page, ticket); got["allowed"] != false {
		t.Fatalf("a ticket outlived the right to edit: %v", got)
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
