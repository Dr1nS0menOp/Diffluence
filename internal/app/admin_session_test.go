// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/identity"
)

// pathParameter matches one parameter of a route pattern, such as {id}.
var pathParameter = regexp.MustCompile(`\{[^}]+\}`)

// TestEveryAdministrationChangeNeedsASession walks the real route table: every route below
// /api/v1/admin/ that is not a read refuses a token, even one with every scope owned by an
// administrator, so a route added later is covered the moment it exists (spec 085).
func TestEveryAdministrationChangeNeedsASession(t *testing.T) {
	e := newEnv(t)
	root := e.user("root@example.com", identity.AllGlobalPermissions...)
	token := root.ok("POST", "/api/v1/me/tokens", map[string]any{
		"name": "script", "scopes": []string{identity.ScopeRead, identity.ScopeWrite, identity.ScopeExport, identity.ScopeAdmin},
	})["secret"].(string)

	checked := 0
	for _, r := range e.app.Routes() {
		method, path, found := strings.Cut(r.Pattern, " ")
		if !found {
			method, path = "", r.Pattern
		}
		if !strings.HasPrefix(path, "/api/v1/admin/") || method == http.MethodGet || method == http.MethodHead {
			continue
		}
		checked++
		// A pattern without a method matches every method; POST stands for a change.
		if method == "" {
			method = http.MethodPost
		}
		path = pathParameter.ReplaceAllStringFunc(path, func(string) string { return uuid.NewString() })
		if status, out := e.bearer(method, path, token, "{}"); status != http.StatusForbidden || !strings.Contains(out, "session_only") {
			t.Errorf("%s with an administrator's token: %d %s", r.Pattern, status, out)
		}
	}
	if checked < 80 {
		t.Fatalf("only %d administration changes were walked", checked)
	}

	if status, out := e.bearer("GET", "/api/v1/admin/features", token, ""); status != http.StatusOK {
		t.Fatalf("a token with the admin scope no longer reads Administration: %d %s", status, out)
	}
	root.ok("PUT", "/api/v1/admin/features", map[string]any{"off": []string{"blog"}, "new_off": []string{"reactions"}})
}
