// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// idRoute says who may use a route that names an object, by a path parameter or by ids
// in its body, while its route rule is not an object rule, so the module behind it
// decides who may (spec 067). Every such route is listed here: a new one fails the walk
// until it is declared, so no route that takes an id can be added without saying whose
// object it reaches.
type idRoute struct {
	// owner names who the module lets through.
	owner string
	// open marks a route any member may use on any object by design, such as a profile:
	// the walk then only checks that no hidden content comes back.
	open bool
}

// idRoutes is the one table of routes that take an id and decide access in the module.
var idRoutes = map[string]idRoute{
	"DELETE /api/v1/approvals/{id}/people/{user}":   {owner: "the one who asked for the round"},
	"GET /api/v1/approvals/{id}":                    {owner: "whoever may view the page of the round"},
	"PUT /api/v1/approvals/{id}/answer":             {owner: "an approver of the round"},
	"POST /api/v1/approvals/{id}/close":             {owner: "the one who asked for the round"},
	"DELETE /api/v1/me/passkeys/{id}":               {owner: "the caller's own passkey"},
	"DELETE /api/v1/me/sessions/{id}":               {owner: "the caller's own session"},
	"DELETE /api/v1/me/tokens/{id}":                 {owner: "the caller's own token"},
	"PUT /api/v1/me/notifications/{id}/read":        {owner: "the caller's own notification"},
	"PUT /api/v1/me/notifications/{id}/unread":      {owner: "the caller's own notification"},
	"DELETE /api/v1/saved-blocks/{id}":              {owner: "the maker of the block"},
	"PATCH /api/v1/saved-blocks/{id}":               {owner: "the maker of the block"},
	"GET /api/v1/templates/{id}":                    {owner: "whoever may view the template's space"},
	"PATCH /api/v1/templates/{id}":                  {owner: "a manager of the template's space"},
	"DELETE /api/v1/templates/{id}":                 {owner: "a manager of the template's space"},
	"POST /api/v1/templates/{id}/use":               {owner: "whoever may view the template's space"},
	"GET /api/v1/templates/{id}/export":             {owner: "whoever may view the template's space"},
	"GET /api/v1/templates/{id}/files/{file}":       {owner: "whoever may view the template's space"},
	"PUT /api/v1/templates/{id}/favourite":          {owner: "whoever may view the template's space"},
	"DELETE /api/v1/templates/{id}/favourite":       {owner: "whoever may view the template's space"},
	"GET /api/v1/exports/{id}":                      {owner: "the maker of the export"},
	"GET /api/v1/exports/{id}/file":                 {owner: "the maker of the export"},
	"GET /api/v1/imports/{id}":                      {owner: "the maker of the import"},
	"PUT /api/v1/imports/{id}/mapping":              {owner: "the maker of the import"},
	"POST /api/v1/imports/{id}/run":                 {owner: "the maker of the import"},
	"POST /api/v1/imports/{id}/undo":                {owner: "the maker of the import"},
	"GET /api/v1/workflows/{id}":                    {owner: "whoever may view the workflow's space"},
	"GET /api/v1/workflows/{id}/history":            {owner: "whoever may view the workflow's space"},
	"PUT /api/v1/workflows/{id}":                    {owner: "a holder of space.workflows"},
	"POST /api/v1/workflows/{id}/archive":           {owner: "a holder of space.workflows"},
	"POST /api/v1/comments/{id}/undo-delete":        {owner: "the one who deleted the comment"},
	"POST /api/v1/pages/{id}/undo-delete":           {owner: "the one who deleted the page"},
	"POST /api/v1/pages/{id}/access-requests":       {owner: "whoever may view the page"},
	"POST /api/v1/invitation-requests/{id}/approve": {owner: "an administrator"},
	"POST /api/v1/invitation-requests/{id}/refuse":  {owner: "an administrator"},
	"GET /api/v1/labels/{label}/pages":              {owner: "lists only pages the caller may view", open: true},
	"GET /api/v1/people/{id}":                       {owner: "any member: profiles are shared", open: true},
	"GET /api/v1/people/{id}/picture":               {owner: "any member: profiles are shared", open: true},
	"GET /api/v1/people/{id}/follows":               {owner: "any member: profiles are shared", open: true},
	"GET /api/v1/people/{id}/pages":                 {owner: "lists only pages the caller may view", open: true},
	"GET /api/v1/people/{id}/mentions":              {owner: "lists only pages the caller may view", open: true},
	"PUT /api/v1/users/{id}/follow":                 {owner: "any member follows any member", open: true},
	"DELETE /api/v1/users/{id}/follow":              {owner: "any member follows any member", open: true},
	"POST /api/v1/blocks/{kind}":                    {owner: "answers only about pages the caller may view", open: true},
	"POST /api/v1/attachments/lookup":               {owner: "answers only about files the caller may view", open: true},
	"POST /api/v1/pages/bulk-move":                  {owner: "moves only pages the caller may move, and reports the rest as not found", open: true},
	"POST /api/v1/pages/link-facts":                 {owner: "answers only about pages the caller may view", open: true},
	"POST /api/v1/pages/reachable":                  {owner: "answers only about pages the caller may view", open: true},
}

// bodyIDRoutes take the ids of the objects they reach in their body, not in the path.
var bodyIDRoutes = map[string]bool{
	"POST /api/v1/attachments/lookup": true, "POST /api/v1/pages/bulk-move": true,
	"POST /api/v1/pages/link-facts": true, "POST /api/v1/pages/reachable": true,
}

// secretMark is in the name of everything the owner makes, so an answer that carries
// any of it to the stranger is caught whatever its shape.
const secretMark = "Secretwalk"

// TestEveryRouteThatTakesAnIDIsProtected walks every route that names an object, by
// path or by body, whose rule is not an object rule: the module decides there, and a
// stranger must get "not found" or a refusal, or a list without anything hidden, never
// the owner's data, and nothing of the owner's may change.
func TestEveryRouteThatTakesAnIDIsProtected(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.AllGlobalPermissions...)
	colleague := e.user("colleague@example.com", identity.PermUseProduct)
	stranger := e.user("stranger@example.com", identity.PermUseProduct)

	space, _ := owner.space("WALK")
	owner.grant(space, "user", colleague.id, permissions.TemplateEditor)
	page := owner.page(space, nil, secretMark+" page", "words of "+secretMark)
	owner.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "secretwalk"})
	_, uploaded := owner.upload(page, secretMark+".txt", "hello")
	var file map[string]any
	_ = json.Unmarshal([]byte(uploaded), &file)
	gone := owner.page(space, nil, secretMark+" gone", "x")
	owner.ok("DELETE", "/api/v1/pages/"+gone, nil)
	comment := owner.comment(page, secretMark+" comment")
	owner.ok("DELETE", "/api/v1/comments/"+comment, nil)

	round := owner.ok("POST", "/api/v1/pages/"+page+"/approvals", map[string]any{"user_ids": []string{colleague.id}})["round_id"].(string)
	template := owner.ok("POST", "/api/v1/spaces/"+space+"/templates", map[string]any{"name": secretMark + " template", "document": para("words")})["id"].(string)
	workflow := owner.ok("POST", "/api/v1/spaces/"+space+"/workflows", map[string]any{"name": secretMark + " flow", "definition": reviewFlow()})["id"].(string)
	block := owner.ok("POST", "/api/v1/saved-blocks", map[string]any{"name": secretMark + " block", "document": para("words")})["id"].(string)
	export := owner.ok("POST", "/api/v1/exports", map[string]any{"space_id": space, "format": "text", "root_id": page})["id"].(string)
	imported := owner.importFile(space, secretMark+".md", []byte("# "+secretMark))["id"].(string)
	owner.ok("POST", "/api/v1/me/tokens", map[string]any{"name": secretMark})
	token := owner.ok("GET", "/api/v1/me/tokens", nil)["tokens"].([]any)[0].(map[string]any)["id"].(string)
	session := owner.ok("GET", "/api/v1/me/sessions", nil)["sessions"].([]any)[0].(map[string]any)["id"].(string)
	notification := uuid.NewString()
	_ = e.app.Pool().QueryRow(context.Background(), "SELECT id::text FROM notifications WHERE user_id = $1 LIMIT 1", owner.id).Scan(&notification)

	byPrefix := map[string]string{
		"/api/v1/approvals/": round, "/api/v1/templates/": template, "/api/v1/workflows/": workflow,
		"/api/v1/saved-blocks/": block, "/api/v1/exports/": export, "/api/v1/imports/": imported,
		"/api/v1/me/tokens/": token, "/api/v1/me/sessions/": session, "/api/v1/me/notifications/": notification,
		"/api/v1/comments/": comment, "/api/v1/people/": owner.id, "/api/v1/users/": owner.id,
		// No passkey can be made without a browser; a foreign id must still be refused.
		"/api/v1/me/passkeys/": uuid.NewString(), "/api/v1/invitation-requests/": uuid.NewString(),
	}
	bodies := map[string]any{
		"POST /api/v1/attachments/lookup": map[string]any{"ids": []string{file["id"].(string)}},
		"POST /api/v1/pages/bulk-move":    map[string]any{"page_ids": []string{page}, "space_id": space},
		"POST /api/v1/pages/link-facts":   map[string]any{"ids": []string{page}},
		"POST /api/v1/pages/reachable":    map[string]any{"ids": []string{page}},
	}
	pathFor := func(pattern string) string {
		_, path, _ := strings.Cut(pattern, " ")
		switch {
		case strings.HasPrefix(path, "/api/v1/pages/{id}/undo-delete"):
			return strings.Replace(path, "{id}", gone, 1)
		case strings.HasPrefix(path, "/api/v1/pages/{id}"):
			return strings.Replace(path, "{id}", page, 1)
		case strings.HasPrefix(path, "/api/v1/labels/"):
			return strings.Replace(path, "{label}", "secretwalk", 1)
		case strings.HasPrefix(path, "/api/v1/blocks/"):
			return strings.Replace(path, "{kind}", "children", 1)
		}
		for prefix, id := range byPrefix {
			if strings.HasPrefix(path, prefix) {
				path = strings.Replace(path, "{id}", id, 1)
			}
		}
		return strings.NewReplacer("{user}", colleague.id, "{file}", "x").Replace(path)
	}

	walked := map[string]bool{}
	for _, r := range e.app.Routes() {
		rule := r.Access.String()
		if !strings.Contains(rule, "authenticated") {
			continue
		}
		if !strings.Contains(r.Pattern, "{") && !bodyIDRoutes[r.Pattern] {
			continue
		}
		declared, ok := idRoutes[r.Pattern]
		if !ok {
			t.Errorf("%s takes an id and decides access in its module: declare its owner in idRoutes", r.Pattern)
			continue
		}
		walked[r.Pattern] = true
		method, _, _ := strings.Cut(r.Pattern, " ")
		var body any = map[string]any{}
		if b, ok := bodies[r.Pattern]; ok {
			body = b
		} else if r.Pattern == "POST /api/v1/blocks/{kind}" {
			body = map[string]any{"page_id": page}
		}
		raw, _ := json.Marshal(body)
		in := ""
		if method != http.MethodGet && method != http.MethodDelete {
			in = string(raw)
		}
		resp, out := e.do(stranger.c, method, pathFor(r.Pattern), in, "Content-Type", "application/json")
		if strings.Contains(out, secretMark) {
			t.Errorf("%s gave a stranger the owner's data (%d): %s", r.Pattern, resp.StatusCode, out)
			continue
		}
		if declared.open {
			continue
		}
		if resp.StatusCode < 400 {
			t.Errorf("%s as a stranger: %d, want a refusal (owner: %s): %s", r.Pattern, resp.StatusCode, declared.owner, out)
		}
	}
	for pattern := range idRoutes {
		if !walked[pattern] {
			t.Errorf("%s is declared in idRoutes but no longer takes an id or no longer exists", pattern)
		}
	}
	if len(walked) < 40 {
		t.Fatalf("only %d routes were walked", len(walked))
	}

	// Nothing of the owner's changed: the page is where it was, and the owner is still in.
	got := owner.ok("GET", "/api/v1/pages/"+page, nil)
	if got["space_id"] != space || got["title"] != secretMark+" page" {
		t.Fatalf("the owner's page changed: %v", got)
	}
	owner.ok("GET", "/api/v1/templates/"+template, nil)
	owner.ok("GET", "/api/v1/workflows/"+workflow, nil)
	if blocks := owner.ok("GET", "/api/v1/saved-blocks", nil); !strings.Contains(mustJSON(t, blocks), secretMark+" block") {
		t.Fatalf("the owner's saved block is gone: %v", blocks)
	}
	owner.ok("GET", "/api/v1/me", nil)
}
