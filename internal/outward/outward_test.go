// SPDX-License-Identifier: Apache-2.0

package outward

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestOnlyPublicAddressesPass(t *testing.T) {
	refused := []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.64.0.1",
		"0.0.0.0", "::1", "fe80::1", "fc00::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1", "224.0.0.1", "198.18.0.1", "240.0.0.1"}
	for _, s := range refused {
		if isPublic(netip.MustParseAddr(s)) {
			t.Errorf("%s passed", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !isPublic(netip.MustParseAddr(s)) {
			t.Errorf("%s was refused", s)
		}
	}
}

func TestAConnectionToAPrivateAddressIsRefusedAtConnectTime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("the call got through") }))
	defer srv.Close()
	c := newClient(Options{})
	resp, err := c.Get(srv.URL)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("a private address was reached")
	}
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestARedirectIsNotFollowed(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer srv.Close()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := newClient(Options{AllowPrivate: true}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound || reached {
		t.Fatalf("status %d, reached %v", resp.StatusCode, reached)
	}
}

func TestHostsAreNamesOnly(t *testing.T) {
	for _, bad := range []string{"", "a/b", "user@host", "host:8080", "*.example.com", "a b"} {
		if _, err := cleanHost(bad); err == nil {
			t.Errorf("%q passed", bad)
		}
	}
	if got, err := cleanHost(" Hooks.Example.COM "); err != nil || got != "hooks.example.com" {
		t.Fatalf("%q %v", got, err)
	}
}
