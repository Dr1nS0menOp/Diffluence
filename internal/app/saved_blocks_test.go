// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
)

const savedPath = "/api/v1/saved-blocks"

func savedDoc(words string) map[string]any {
	return map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": words}}},
	}}
}

func savedNames(u *user) string {
	u.e.t.Helper()
	return u.want(http.StatusOK, "GET", savedPath, nil)
}

func TestASavedBlockStaysPrivateUntilShared(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct)
	other := e.user("other@example.com", identity.PermUseProduct)

	block := owner.ok("POST", savedPath, map[string]any{"name": "Secret intro", "document": savedDoc("Private words")})
	id := block["id"].(string)
	if strings.Contains(savedNames(other), "Secret intro") {
		t.Fatal("another person sees a private block")
	}
	owner.want(http.StatusNoContent, "PATCH", savedPath+"/"+id, map[string]any{"shared": true})
	if !strings.Contains(savedNames(other), "Secret intro") {
		t.Fatal("a shared block does not reach another member")
	}
	owner.want(http.StatusNoContent, "PATCH", savedPath+"/"+id, map[string]any{"shared": false})
	if strings.Contains(savedNames(other), "Secret intro") {
		t.Fatal("a block stays shared after closing it")
	}
}

func TestOnlyTheOwnerChangesASavedBlock(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct)
	other := e.user("other@example.com", identity.PermUseProduct)
	root := admin(e)

	id := owner.ok("POST", savedPath, map[string]any{"name": "Shared intro", "document": savedDoc("Words"), "shared": true})["id"].(string)
	for _, stranger := range []*user{other, root} {
		stranger.want(http.StatusNotFound, "DELETE", savedPath+"/"+id, nil)
		stranger.want(http.StatusNotFound, "PATCH", savedPath+"/"+id, map[string]any{"shared": false})
	}
	if !strings.Contains(savedNames(other), "Shared intro") {
		t.Fatal("a refused change still changed the block")
	}
	owner.want(http.StatusNoContent, "DELETE", savedPath+"/"+id, nil)
	if strings.Contains(savedNames(owner), "Shared intro") {
		t.Fatal("a deleted block is still listed")
	}
	if got := e.count("SELECT count(*) FROM audit_events WHERE action LIKE 'saved_block.%'"); got != 2 {
		t.Fatalf("audit lines = %d, want 2", got)
	}
}

func TestAGuestDoesNotSeeSharedBlocks(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct)
	guest := e.user("guest@example.com", identity.PermUseProduct)
	if _, err := e.app.Pool().Exec(t.Context(), "UPDATE users SET type = 'guest' WHERE id = $1", guest.id); err != nil {
		t.Fatal(err)
	}
	owner.ok("POST", savedPath, map[string]any{"name": "Shared intro", "document": savedDoc("Words"), "shared": true})
	if strings.Contains(savedNames(guest), "Shared intro") {
		t.Fatal("a guest sees what members shared")
	}
}

func TestASavedBlockPassesTheDocumentGate(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct)
	script := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "script"}}}
	owner.want(http.StatusBadRequest, "POST", savedPath, map[string]any{"name": "Bad", "document": script})
	owner.want(http.StatusBadRequest, "POST", savedPath, map[string]any{"name": "  ", "document": savedDoc("Words")})
	owner.want(http.StatusBadRequest, "POST", savedPath, map[string]any{"name": strings.Repeat("x", 101), "document": savedDoc("Words")})
}

func TestAPageLanguageMustLookLikeOne(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	space, _ := root.space("ENG")
	page := root.page(space, nil, "Hello", "Text")
	look := func(language string) map[string]any {
		return map[string]any{"title_align": "start", "language": language}
	}
	for _, bad := range []string{"english", "<b>", "nl_NL", `"><script>`, "NL"} {
		root.want(http.StatusBadRequest, "PATCH", "/api/v1/pages/"+page+"/look", look(bad))
	}
	root.want(http.StatusOK, "PATCH", "/api/v1/pages/"+page+"/look", look("pt-BR"))
	if got := root.ok("GET", "/api/v1/pages/"+page, nil)["language"]; got != "pt-BR" {
		t.Fatalf("language = %v", got)
	}
}
