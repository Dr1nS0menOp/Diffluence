// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
)

func TestTheScopeOfARequestComesFromItsRoute(t *testing.T) {
	cases := []struct {
		method, pattern, want string
	}{
		{"GET", "", identity.ScopeAdmin},
		{"GET", "GET /api/v1/admin/analytics", identity.ScopeAdmin},
		{"GET", "GET /api/v1/admin/export-policy", identity.ScopeAdmin},
		{"GET", "GET /api/v1/spaces/{id}/export.zip", identity.ScopeExport},
		{"POST", "POST /api/v1/exports", identity.ScopeExport},
		{"GET", "GET /api/v1/labels/{label}/pages", identity.ScopeRead},
		{"HEAD", "GET /api/v1/pages/{id}", identity.ScopeRead},
		{"PATCH", "PATCH /api/v1/pages/{id}", identity.ScopeWrite},
	}
	for _, c := range cases {
		r := &http.Request{Method: c.method, Pattern: c.pattern}
		if got := scopeFor(r); got != c.want {
			t.Errorf("%s %q: scope %s, want %s", c.method, c.pattern, got, c.want)
		}
	}
}
