// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// freshLogin walks a step-up through the provider and returns where the browser ends up.
func (e *env) freshLogin(p *idp, claims map[string]any) (string, url.Values) {
	e.t.Helper()
	c := noRedirect()
	resp, _ := e.do(c, "GET", "/api/v1/auth/oidc/start?prompt=login", "")
	if resp.StatusCode != http.StatusFound {
		e.t.Fatalf("start: %d", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		e.t.Fatal(err)
	}
	q := loc.Query()
	code := fmt.Sprintf("code-%d", time.Now().UnixNano())
	p.mu.Lock()
	p.codes[code] = claims
	p.nonces[code] = q.Get("nonce")
	p.challenges[code] = q.Get("code_challenge")
	p.mu.Unlock()
	resp, _ = e.do(c, "GET", "/api/v1/auth/oidc/callback?state="+url.QueryEscape(q.Get("state"))+"&code="+code, "")
	return resp.Header.Get("Location"), q
}

// A provider may ignore prompt=login. The step-up only counts when the provider says the
// person logged in again after it started (SEC-150, OpenID Connect auth_time).
func TestAStepUpThroughTheProviderNeedsALoginAfterItStarted(t *testing.T) {
	p := newIDP(t)
	e := newEnvWithOIDC(t, p)
	who := map[string]any{"sub": "fresh-1", "email": "fresh@example.com", "name": "Fresh Person", "email_verified": true}
	with := func(extra map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range who {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}

	to, q := e.freshLogin(p, with(map[string]any{"auth_time": time.Now().Add(-time.Hour).Unix()}))
	if q.Get("max_age") != "0" || q.Get("prompt") != "login" {
		t.Fatalf("the provider was not asked for a fresh login: %v", q)
	}
	if !strings.HasPrefix(to, "/login?error=") {
		t.Fatalf("an old login counted as fresh: %s", to)
	}
	if to, _ := e.freshLogin(p, who); !strings.HasPrefix(to, "/login?error=") {
		t.Fatalf("a login without auth_time counted as fresh: %s", to)
	}
	if to, _ := e.freshLogin(p, with(map[string]any{"auth_time": time.Now().Unix()})); to != "/" {
		t.Fatalf("a fresh login was refused: %s", to)
	}
	if e.auditActions("auth.login") < 2 {
		t.Fatal("the refused step-ups were not recorded")
	}
}
