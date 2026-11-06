// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"fmt"
	"net/http"
	"sort"
)

type accessKind int

const (
	accessUnset accessKind = iota
	accessPublic
	accessAuthenticated
	accessPermission
	accessObject
	accessSCIM
)

// Access is the rule a route enforces before its handler runs. The zero value
// is invalid on purpose: forgetting to choose a rule makes registration panic.
type Access struct {
	kind       accessKind
	permission string
	// sessionOnly refuses personal access tokens: a leaked script token must not
	// reach passwords, tokens or the console.
	sessionOnly bool
	// sensitive also asks for proof of who you are, at most 10 minutes old.
	sensitive bool
}

// Public routes need no session. Every use must carry a comment explaining why.
var Public = Access{kind: accessPublic}

// Authenticated requires a valid session.
func Authenticated() Access { return Access{kind: accessAuthenticated} }

// Permission requires a valid session and a global permission.
func Permission(p string) Access {
	if p == "" {
		panic("httpapi: Permission needs a permission name")
	}
	return Access{kind: accessPermission, permission: p}
}

// Object requires a valid session and declares the permission the route needs on
// the object it touches. The module behind the route makes the actual decision
// (permissions.Require...), because only it knows the object; this declaration
// keeps every route's rule visible in one list and in the tests.
func Object(action string) Access {
	if action == "" {
		panic("httpapi: Object needs an action name")
	}
	return Access{kind: accessObject, permission: action}
}

// SessionOnly refuses tokens on top of a rule.
func SessionOnly(a Access) Access {
	a.sessionOnly = true
	return a
}

// Sensitive asks a person to prove who they are, on top of a rule. It implies
// SessionOnly: a token cannot prove anything.
func Sensitive(a Access) Access {
	a.sessionOnly, a.sensitive = true, true
	return a
}

// ScimToken requires a valid SCIM bearer token. There is no session: identity
// providers call these routes.
func ScimToken() Access { return Access{kind: accessSCIM} }

func (a Access) String() string {
	switch {
	case a.sensitive:
		return "sensitive " + a.base()
	case a.sessionOnly:
		return "session-only " + a.base()
	}
	return a.base()
}

func (a Access) base() string {
	switch a.kind {
	case accessPublic:
		return "public"
	case accessAuthenticated:
		return "authenticated"
	case accessPermission:
		return "permission:" + a.permission
	case accessObject:
		return "object:" + a.permission
	case accessSCIM:
		return "scim-token"
	default:
		return "unset"
	}
}

// IsPublic reports whether the rule lets anonymous requests through.
func (a Access) IsPublic() bool { return a.kind == accessPublic }

// Route describes one registered route, for tests and documentation.
type Route struct {
	Pattern string
	Access  Access
}

// Router wraps http.ServeMux and refuses routes without an access rule.
type Router struct {
	mux     *http.ServeMux
	routes  []Route
	enforce func(Access, http.Handler) http.Handler
}

func newRouter(enforce func(Access, http.Handler) http.Handler) *Router {
	return &Router{mux: http.NewServeMux(), enforce: enforce}
}

// Handle registers pattern (e.g. "GET /api/v1/me") with an access rule.
func (r *Router) Handle(pattern string, a Access, h http.Handler) {
	if a.kind == accessUnset {
		panic(fmt.Sprintf("httpapi: route %q registered without an access rule", pattern))
	}
	r.routes = append(r.routes, Route{Pattern: pattern, Access: a})
	r.mux.Handle(pattern, r.enforce(a, h))
}

// HandleFunc is Handle for plain functions.
func (r *Router) HandleFunc(pattern string, a Access, h http.HandlerFunc) { r.Handle(pattern, a, h) }

// Routes returns all registered routes, sorted by pattern.
func (r *Router) Routes() []Route {
	out := append([]Route(nil), r.routes...)
	sort.Slice(out, func(i, j int) bool { return out[i].Pattern < out[j].Pattern })
	return out
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) { r.mux.ServeHTTP(w, req) }
