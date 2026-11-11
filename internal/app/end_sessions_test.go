// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
)

// During an incident an operator ends sessions without disabling anyone (SEC-178).
func TestAnOperatorEndsTheSessionsOfOnePersonOrOfEveryone(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com")
	bob := e.user("bob@example.com")
	ctx := context.Background()
	operator := identity.SystemActor("cli")

	ended, err := e.app.Identity().EndSessions(ctx, operator, "ANNA@example.com", identity.RequestMeta{})
	if err != nil || ended != 1 {
		t.Fatalf("one person: %d %v", ended, err)
	}
	anna.want(401, "GET", "/api/v1/me", nil)
	bob.ok("GET", "/api/v1/me", nil)
	if _, err := e.app.Identity().EndSessions(ctx, operator, "nobody@example.com", identity.RequestMeta{}); err == nil {
		t.Fatal("an unknown address ended nothing and said nothing")
	}

	if _, err := e.app.Identity().EndSessions(ctx, operator, "", identity.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	bob.want(401, "GET", "/api/v1/me", nil)
	if code := e.login(anna.c, "anna@example.com", adminPassword); code != 200 {
		t.Fatalf("ending sessions must not disable anyone: %d", code)
	}
	var audited int
	if err := e.app.Pool().QueryRow(ctx, "SELECT count(*) FROM audit_entries WHERE action = 'sessions.ended'").Scan(&audited); err != nil || audited != 2 {
		t.Fatalf("audited %d %v", audited, err)
	}
}
