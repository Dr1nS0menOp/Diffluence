// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"testing"

	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/identity"
)

func newEnvBehindProxy(t *testing.T) *env {
	t.Helper()
	e := newEnvWith(t, func(c *config.Config) {
		c.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("::1/128")}
	})
	_, err := e.app.Identity().CreateLocalUser(context.Background(), identity.NewUser{
		Email: "anna@example.com", DisplayName: "Anna", Password: adminPassword,
	}, identity.SystemActor("test"), identity.RequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) loginFrom(address, password string) int {
	e.t.Helper()
	body, _ := json.Marshal(map[string]string{"email": "anna@example.com", "password": password})
	resp, _ := e.do(e.client(), "POST", "/api/v1/auth/login", string(body), "X-Forwarded-For", address)
	return resp.StatusCode
}

func TestWrongPasswordsFromOneAddressDoNotLockOutThePerson(t *testing.T) {
	e := newEnvBehindProxy(t)
	for range 12 {
		e.loginFrom("198.51.100.1", "wrong password here")
	}
	if got := e.loginFrom("198.51.100.1", adminPassword); got != http.StatusTooManyRequests {
		t.Fatalf("the address that guessed: %d, want 429", got)
	}
	if got := e.loginFrom("192.0.2.7", adminPassword); got != http.StatusOK {
		t.Fatalf("the person from their own address: %d, want 200", got)
	}
}

func TestGuessesFromManyAddressesStillStopAtALimit(t *testing.T) {
	e := newEnvBehindProxy(t)
	last := 0
	for i := range 101 {
		last = e.loginFrom(fmt.Sprintf("198.51.%d.%d", i/200, i%200+1), "wrong password here")
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("guess 101 from a new address: %d, want 429", last)
	}
}
