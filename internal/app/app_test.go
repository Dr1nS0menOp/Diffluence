// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/httpapi"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/testutil"
)

const adminPassword = "correct horse battery staple"

type env struct {
	t   *testing.T
	app *app.App
	srv *httptest.Server
}

func newEnv(t *testing.T) *env {
	t.Helper()
	return newEnvWith(t, func(*config.Config) {})
}

func newEnvWith(t *testing.T, change func(*config.Config)) *env {
	t.Helper()
	return newEnvLogging(t, change, slog.New(slog.DiscardHandler))
}

// newEnvLogging is newEnvWith with a logger of the test's choosing.
func newEnvLogging(t *testing.T, change func(*config.Config), log *slog.Logger) *env {
	t.Helper()
	return newEnvOn(t, testutil.Database(t), change, log)
}

// newInstalledEnv runs on a database exactly as a new installation starts, with every
// experimental part off (spec 069).
func newInstalledEnv(t *testing.T, change func(*config.Config)) *env {
	t.Helper()
	return newEnvOn(t, testutil.InstalledDatabase(t), change, slog.New(slog.DiscardHandler))
}

func newEnvOn(t *testing.T, tdb testutil.TestDB, change func(*config.Config), log *slog.Logger) *env {
	t.Helper()
	u, _ := url.Parse("http://localhost")
	cfg := config.Config{
		Role: config.RoleAll, HTTPAddr: "127.0.0.1:0", PublicURL: u, InstanceName: "Test",
		DatabaseURL: tdb.URL, DataDir: t.TempDir(), MigrateOnStart: true,
		SessionIdleTimeout: time.Hour, SessionMaxAge: 24 * time.Hour,
	}
	change(&cfg)
	a, err := app.New(context.Background(), cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() { srv.Close(); a.Close() })
	return &env{t: t, app: a, srv: srv}
}

func (e *env) createUser(email string, perms ...string) {
	e.t.Helper()
	_, err := e.app.Identity().CreateLocalUser(context.Background(), identity.NewUser{
		Email: email, DisplayName: "Test " + email, Password: adminPassword, GlobalPermissions: perms,
	}, identity.SystemActor("test"), identity.RequestMeta{})
	if err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func (e *env) do(c *http.Client, method, path, body string, headers ...string) (*http.Response, string) {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, string(b)
}

func (e *env) login(c *http.Client, email, password string) int {
	e.t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email, "password": password})
	resp, _ := e.do(c, "POST", "/api/v1/auth/login", string(b))
	return resp.StatusCode
}

func TestEveryNonPublicRouteRequiresASession(t *testing.T) {
	e := newEnv(t)
	routes := e.app.Routes()
	if len(routes) < 5 {
		t.Fatalf("only %d routes registered", len(routes))
	}
	c := e.client()
	for _, r := range routes {
		if r.Access.IsPublic() {
			continue
		}
		method, path, ok := strings.Cut(r.Pattern, " ")
		if !ok {
			t.Fatalf("route %q has no method", r.Pattern)
		}
		for _, wildcard := range []string{"{id}", "{key}", "{number}", "{user}", "{emoji}", "{section}", "{label}", "{alias}", "{file}", "{kind}", "{delivery}"} {
			path = strings.ReplaceAll(path, wildcard, "00000000-0000-0000-0000-000000000001")
		}
		if strings.Contains(path, "{") {
			t.Fatalf("route %q: extend this test for this wildcard", r.Pattern)
		}
		body := ""
		if method == "POST" || method == "PUT" || method == "PATCH" {
			body = "{}"
		}
		resp, _ := e.do(c, method, path, body)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s without session: status %d, want 401", r.Pattern, resp.StatusCode)
		}
	}
}

// TestRouteTableIsDocumented keeps docs/api/routes.txt equal to the real routes, so
// every new route and every changed access rule shows up as a line in the diff.
// Regenerate with: UPDATE_ROUTES=1 go test ./internal/app/ -run RouteTable
func TestTheAPIDescriptionMatchesTheRoutes(t *testing.T) {
	e := newEnv(t)
	raw, err := httpapi.OpenAPI(e.app.Routes())
	if err != nil {
		t.Fatal(err)
	}
	const path = "../../docs/api/openapi.json"
	if os.Getenv("UPDATE_ROUTES") != "" {
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(raw) {
		t.Fatal("docs/api/openapi.json differs from the routes. Regenerate with: UPDATE_ROUTES=1 go test ./internal/app/ -run 'RouteTable|APIDescription'")
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Access string `json:"x-access"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	entries := 0
	for _, methods := range doc.Paths {
		entries += len(methods)
	}
	routes := 0
	for _, r := range e.app.Routes() {
		method, path, found := strings.Cut(r.Pattern, " ")
		if !found {
			continue
		}
		routes++
		if doc.Paths[path][strings.ToLower(method)].Access != r.Access.String() {
			t.Errorf("%s has no entry with its rule", r.Pattern)
		}
	}
	if entries != routes {
		t.Fatalf("%d entries for %d routes", entries, routes)
	}
}

func TestRouteTableIsDocumented(t *testing.T) {
	e := newEnv(t)
	var b strings.Builder
	b.WriteString("# Every route and its access rule. Generated by TestRouteTableIsDocumented.\n")
	for _, r := range e.app.Routes() {
		fmt.Fprintf(&b, "%-62s %s\n", r.Pattern, r.Access)
	}
	const path = "../../docs/api/routes.txt"
	if os.Getenv("UPDATE_ROUTES") != "" {
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != b.String() {
		t.Fatalf("the routes changed: run UPDATE_ROUTES=1 go test ./internal/app/ -run RouteTable and review the diff")
	}
}

func TestLoginSessionAndAuditTrail(t *testing.T) {
	e := newEnv(t)
	e.createUser("admin@example.com", identity.AllGlobalPermissions...)
	c := e.client()

	if got := e.login(c, "admin@example.com", "wrong password here"); got != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", got)
	}
	if got := e.login(c, "nobody@example.com", adminPassword); got != http.StatusUnauthorized {
		t.Fatalf("unknown user: %d", got)
	}
	if got := e.login(c, "ADMIN@example.com", adminPassword); got != http.StatusOK {
		t.Fatalf("login (case-insensitive email): %d", got)
	}

	resp, body := e.do(c, "GET", "/api/v1/me", "")
	if resp.StatusCode != 200 || !strings.Contains(body, `"administrator"`) {
		t.Fatalf("me: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(c, "GET", "/api/v1/system/info", "")
	if resp.StatusCode != 200 || !strings.Contains(body, `"instance_id"`) {
		t.Fatalf("system info: %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(c, "GET", "/api/v1/admin/audit-events?action=auth.login", "")
	if resp.StatusCode != 200 {
		t.Fatalf("audit list: %d %s", resp.StatusCode, body)
	}
	var list struct {
		Events []struct {
			Action  string         `json:"action"`
			Result  string         `json:"result"`
			Details map[string]any `json:"details"`
		} `json:"events"`
	}
	_ = json.Unmarshal([]byte(body), &list)
	if len(list.Events) != 3 {
		t.Fatalf("want 3 login events (2 failures, 1 success), got %d: %s", len(list.Events), body)
	}
	if list.Events[0].Result != audit.ResultSuccess || list.Events[2].Details["reason"] != "bad_password" {
		t.Fatalf("unexpected login events: %+v", list.Events)
	}
	if strings.Contains(body, adminPassword) {
		t.Fatal("password leaked into the audit log")
	}

	resp, body = e.do(c, "GET", "/api/v1/admin/audit-events/export", "")
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/csv") ||
		!strings.HasPrefix(body, "seq,id,occurred_at") || !strings.Contains(body, "\n"+audit.CSVComplete+",") {
		t.Fatalf("export: %d %s", resp.StatusCode, body[:min(len(body), 200)])
	}
	resp, body = e.do(c, "GET", "/api/v1/admin/audit-events/verify", "")
	if resp.StatusCode != 200 || !strings.Contains(body, `"ok":true`) {
		t.Fatalf("verify: %d %s", resp.StatusCode, body)
	}

	resp, _ = e.do(c, "POST", "/api/v1/auth/logout", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: %d", resp.StatusCode)
	}
	resp, _ = e.do(c, "GET", "/api/v1/me", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout: %d", resp.StatusCode)
	}
}

func TestNonAdminIsDeniedAndAudited(t *testing.T) {
	e := newEnv(t)
	e.createUser("admin@example.com", identity.AllGlobalPermissions...)
	e.createUser("reader@example.com", identity.PermUseProduct)
	c := e.client()
	if e.login(c, "reader@example.com", adminPassword) != 200 {
		t.Fatal("login failed")
	}
	resp, _ := e.do(c, "GET", "/api/v1/admin/audit-events", "")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin got %d, want 403", resp.StatusCode)
	}
	evs, err := audit.List(context.Background(), e.app.Pool(), audit.Filter{Action: "access.denied"})
	if err != nil || len(evs) != 1 || evs[0].Details["permission"] != identity.PermAdministrator {
		t.Fatalf("denied access not audited: %+v %v", evs, err)
	}
}

func TestSecurityHeadersAndCrossOriginProtection(t *testing.T) {
	e := newEnv(t)
	e.createUser("admin@example.com", identity.AllGlobalPermissions...)
	c := e.client()

	resp, _ := e.do(c, "GET", "/livez", "")
	for _, h := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "X-Request-Id"} {
		if resp.Header.Get(h) == "" {
			t.Errorf("missing header %s", h)
		}
	}

	b := `{"email":"admin@example.com","password":"` + adminPassword + `"}`
	resp, _ = e.do(c, "POST", "/api/v1/auth/login", b, "Sec-Fetch-Site", "cross-site")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site login: %d, want 403", resp.StatusCode)
	}
	resp, _ = e.do(c, "POST", "/api/v1/auth/login", b, "Origin", "https://evil.example")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin login: %d, want 403", resp.StatusCode)
	}
	resp, _ = e.do(c, "POST", "/api/v1/auth/login", b, "Sec-Fetch-Site", "same-origin")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("same-origin login: %d", resp.StatusCode)
	}
	cookie := resp.Header.Get("Set-Cookie")
	for _, want := range []string{"HttpOnly", "SameSite=Strict", "Path=/"} {
		if !strings.Contains(cookie, want) {
			t.Errorf("cookie %q lacks %s", cookie, want)
		}
	}
}

func TestBadInputAndBruteForce(t *testing.T) {
	e := newEnv(t)
	e.createUser("admin@example.com", identity.AllGlobalPermissions...)
	c := e.client()

	for _, body := range []string{`not json`, `{"email":"a","password":"b","extra":1}`, `{"email":"a"}{"x":1}`} {
		resp, _ := e.do(c, "POST", "/api/v1/auth/login", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("body %q: %d, want 400", body, resp.StatusCode)
		}
	}
	var last int
	for range 11 {
		last = e.login(c, "admin@example.com", "wrong password here")
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("11th attempt: %d, want 429", last)
	}
	// Even the right password is refused while the account is rate limited.
	if got := e.login(c, "admin@example.com", adminPassword); got != http.StatusTooManyRequests {
		t.Fatalf("correct password while limited: %d, want 429", got)
	}
}

// An office shares one address, and everyone logs in around nine. Only failed
// attempts may count against an address.
func TestSuccessfulLoginsAreNotRateLimited(t *testing.T) {
	e := newEnv(t)
	e.createUser("a@example.com", identity.PermUseProduct)
	for i := range 60 {
		if got := e.login(e.client(), "a@example.com", adminPassword); got != http.StatusOK {
			t.Fatalf("login %d: %d", i+1, got)
		}
	}
	// Failures from that same address are still limited.
	var last int
	for i := range 60 {
		last = e.login(e.client(), fmt.Sprintf("nobody%d@example.com", i), "wrong password here")
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("failed logins from one address: %d, want 429", last)
	}
}

func TestProbesAndNotFound(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	if resp, body := e.do(c, "GET", "/readyz", ""); resp.StatusCode != 200 || !strings.Contains(body, "ready") {
		t.Fatalf("readyz: %d %s", resp.StatusCode, body)
	}
	if resp, body := e.do(c, "GET", "/api/v1/nope", ""); resp.StatusCode != 404 || !strings.Contains(body, "not_found") {
		t.Fatalf("404: %d %s", resp.StatusCode, body)
	}
	if resp, body := e.do(c, "POST", "/nope", "{}"); resp.StatusCode != 404 || !strings.Contains(body, "not_found") {
		t.Fatalf("POST to the app: %d %s", resp.StatusCode, body)
	}
	if resp, body := e.do(c, "GET", "/assets/missing.js", ""); resp.StatusCode != 404 || !strings.Contains(body, "not_found") {
		t.Fatalf("a missing asset must not become the app page: %d %s", resp.StatusCode, body)
	}
	resp, _ := e.do(c, "GET", "/pages/some-client-route", "")
	policy := resp.Header.Get("Content-Security-Policy")
	for _, want := range []string{"script-src 'self'", "media-src 'self'", "require-trusted-types-for 'script'", "frame-ancestors 'none'", "base-uri 'none'"} {
		if !strings.Contains(policy, want) {
			t.Errorf("the app's policy lacks %q: %s", want, policy)
		}
	}
	if strings.Contains(policy, "script-src 'unsafe") {
		t.Errorf("inline scripts must stay forbidden: %s", policy)
	}
	if strings.Contains(policy, "frame-src 'self'") || strings.Contains(policy, "wasm-unsafe-eval") {
		t.Errorf("only the viewers may load in a frame, and only they run WebAssembly: %s", policy)
	}
}
