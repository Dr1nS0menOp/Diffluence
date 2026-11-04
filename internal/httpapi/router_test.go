// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestRouteWithoutAccessRulePanics(t *testing.T) {
	r := newRouter(func(_ Access, h http.Handler) http.Handler { return h })
	defer func() {
		if recover() == nil {
			t.Fatal("registering a route without an access rule must panic")
		}
	}()
	r.Handle("GET /x", Access{}, http.NotFoundHandler())
}

func TestPermissionNeedsAName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Permission(\"\") must panic")
		}
	}()
	Permission("")
}

func TestClientIPTrustsOnlyConfiguredProxies(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	var got string
	h := withClientIP(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = ClientIPFrom(r.Context()) }))

	cases := []struct{ remote, xff, want string }{
		{"203.0.113.9:5000", "1.2.3.4", "203.0.113.9"},              // untrusted peer: header ignored
		{"10.0.0.5:5000", "198.51.100.7", "198.51.100.7"},           // via trusted proxy
		{"10.0.0.5:5000", "6.6.6.6, 198.51.100.7", "198.51.100.7"},  // spoofed left-most entry ignored
		{"10.0.0.5:5000", "198.51.100.7, 10.0.0.9", "198.51.100.7"}, // two trusted hops
		{"10.0.0.5:5000", "", "10.0.0.5"},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = c.remote
		if c.xff != "" {
			req.Header.Set("X-Forwarded-For", c.xff)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
		if got != c.want {
			t.Errorf("remote=%s xff=%q: got %s, want %s", c.remote, c.xff, got, c.want)
		}
	}
}

func TestLimiter(t *testing.T) {
	l := newLimiter(2, time.Minute)
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }
	first, second, third := l.allow("k"), l.allow("k"), l.allow("k")
	if !first || !second || third {
		t.Fatalf("attempts allowed = %v %v %v, want true true false", first, second, third)
	}
	now = now.Add(time.Minute)
	if !l.allow("k") {
		t.Fatal("a new window must allow again")
	}
	l.allow("k")
	l.reset("k")
	if !l.allow("k") {
		t.Fatal("reset must clear the counter")
	}
}
