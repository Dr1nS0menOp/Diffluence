// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/identity"
)

// userVerificationOf reads what a begin route asks of the authenticator.
func userVerificationOf(t *testing.T, body string, path ...string) string {
	t.Helper()
	var node any
	if err := json.Unmarshal([]byte(body), &node); err != nil {
		t.Fatal(err)
	}
	for _, key := range path {
		m, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("no %s in %s", key, body)
		}
		node = m[key]
	}
	got, _ := node.(string)
	return got
}

// SEC-134: a passkey counts as a second factor, so it must include a local check.
func TestPasskeysRequireUserVerification(t *testing.T) {
	e := newEnv(t)
	u := e.user("anna@example.com", identity.PermUseProduct)

	_, body := e.do(e.client(), "POST", "/api/v1/auth/passkey/begin", "{}")
	if got := userVerificationOf(t, body, "options", "publicKey", "userVerification"); got != "required" {
		t.Errorf("login: %q", got)
	}
	body = u.want(200, "POST", "/api/v1/me/passkeys/begin", "{}")
	if got := userVerificationOf(t, body, "options", "publicKey", "authenticatorSelection", "userVerification"); got != "required" {
		t.Errorf("registration: %q", got)
	}

	// The library enforces the check from what the challenge stored, not from the options.
	var kept int
	err := e.app.Pool().QueryRow(context.Background(), `SELECT count(*) FROM auth_challenges
		WHERE kind IN ('passkey_login', 'passkey_register') AND data->>'userVerification' = 'required'`).Scan(&kept)
	if err != nil || kept != 2 {
		t.Fatalf("challenges that require a check: %d, %v", kept, err)
	}
}

// SEC-135: a stolen session must not be able to add a way back in.
func TestFactorChangesNeedFreshProof(t *testing.T) {
	e := newEnv(t)
	want := map[string]bool{
		"POST /api/v1/me/passkeys/begin":             false,
		"POST /api/v1/me/passkeys/finish":            false,
		"DELETE /api/v1/me/passkeys/{id}":            false,
		"POST /api/v1/me/totp/disable":               false,
		"POST /api/v1/admin/scim-tokens":             false,
		"POST /api/v1/admin/scim-tokens/{id}/rotate": false,
	}
	for _, r := range e.app.Routes() {
		if _, listed := want[r.Pattern]; listed {
			want[r.Pattern] = strings.HasPrefix(r.Access.String(), "sensitive ")
		}
	}
	for route, sensitive := range want {
		if !sensitive {
			t.Errorf("%s needs fresh proof", route)
		}
	}

	u := e.user("anna@example.com", identity.PermUseProduct)
	e.sql("UPDATE sessions SET created_at = now() - interval '11 minutes'")
	if out := u.want(403, "POST", "/api/v1/me/passkeys/begin", "{}"); !strings.Contains(out, "reauth_required") {
		t.Fatal(out)
	}
}

// SEC-820: people sometimes type a password in the address field, and audit rows stay.
func TestAFailedLoginStoresNoTypedAddress(t *testing.T) {
	e := newEnv(t)
	e.user("anna@example.com", identity.PermUseProduct)
	typed := "MyVerySecretPassword@example.com"
	for range 2 {
		if got := e.login(e.client(), typed, "whatever it is"); got == 200 {
			t.Fatal("logged in")
		}
	}
	if got := e.login(e.client(), "anna@example.com", "the wrong password"); got == 200 {
		t.Fatal("logged in")
	}

	rows, err := e.app.Pool().Query(context.Background(), `SELECT object_id, payload FROM audit_events
		WHERE action = 'auth.login' AND result = 'failure' ORDER BY seq`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var hashes []string
	var known string
	for rows.Next() {
		var object, payload string
		if err := rows.Scan(&object, &payload); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(payload), "myverysecretpassword") || strings.Contains(payload, "anna@example.com") {
			t.Fatalf("the typed address is in the log: %s", payload)
		}
		var event struct {
			Details map[string]any `json:"details"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			t.Fatal(err)
		}
		if hash, ok := event.Details["address_hash"].(string); ok {
			hashes = append(hashes, hash)
		}
		if object != "" {
			known = object
		}
	}
	if len(hashes) != 2 || hashes[0] == "" || hashes[0] != hashes[1] {
		t.Fatalf("an unknown address needs a stable hash, to show credential stuffing: %v", hashes)
	}
	if known == "" {
		t.Fatal("a known account gets its id")
	}
}

// SEC-814: the signatures are the one defence against rewriting the chain end to end.
func TestABadCheckpointReachesAPerson(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	root := admin(e)
	if err := e.app.SignCheckpointForTest(ctx); err != nil {
		t.Fatal(err)
	}
	if out := root.ok("GET", "/api/v1/admin/audit-events/verify", nil); out["ok"] != true || out["checkpoints"] != float64(1) {
		t.Fatalf("a good chain: %v", out)
	}
	e.sql("UPDATE audit_checkpoints SET signature = decode(repeat('00', 64), 'hex')")

	if out := root.ok("GET", "/api/v1/admin/audit-events/verify", nil); out["ok"] != false || out["reason"] == "" {
		t.Fatalf("the Verify button passed a forged checkpoint: %v", out)
	}
	if err := e.app.VerifyAuditForTest(ctx); err != nil {
		t.Fatal(err)
	}
	attention := root.want(200, "GET", "/api/v1/admin/attention", nil)
	if !strings.Contains(attention, "audit_damaged") {
		t.Fatalf("no person hears of it: %s", attention)
	}
	// Pressing the button afterwards must not take the warning away.
	root.ok("GET", "/api/v1/admin/audit-events/verify", nil)
	if attention := root.want(200, "GET", "/api/v1/admin/attention", nil); !strings.Contains(attention, "audit_damaged") {
		t.Fatalf("the warning went away: %s", attention)
	}
}

// SEC-126: an attacker with a session should not outlive the reset of the account.
func TestAnAdministratorsResetEndsSessions(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	anna.enrolTOTP()
	root.ok("POST", "/api/v1/admin/users/"+anna.id+"/reset-mfa", nil)
	anna.want(401, "GET", "/api/v1/me", nil)
}

func TestAPersonCanEndOtherSessionsAfterRemovingAFactor(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	other := e.client()
	if got := e.login(other, anna.email, adminPassword); got != 200 {
		t.Fatalf("second login: %d", got)
	}
	anna.enrolTOTP()
	anna.ok("POST", "/api/v1/me/totp/disable", map[string]any{"password": adminPassword})
	if resp, _ := e.do(other, "GET", "/api/v1/me", ""); resp.StatusCode != 200 {
		t.Fatal("removing a factor only offers this, it does not do it by itself")
	}
	anna.ok("POST", "/api/v1/me/sessions/end-others", nil)
	if resp, _ := e.do(other, "GET", "/api/v1/me", ""); resp.StatusCode != 401 {
		t.Fatalf("the other session goes on: %d", resp.StatusCode)
	}
	anna.ok("GET", "/api/v1/me", nil)
	if e.auditActions("auth.sessions_ended") != 1 {
		t.Fatal("ending sessions must be audited")
	}
}

// SEC-156: a bearer token needs a limited lifetime the provider can check.
func TestSCIMTokensEndAndCanBeRotatedWithAnOverlap(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	old, id := newScim(t, e, root)

	var tokens struct {
		Tokens []struct {
			ExpiresAt time.Time `json:"expires_at"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal([]byte(root.want(200, "GET", "/api/v1/admin/scim-tokens", nil)), &tokens); err != nil {
		t.Fatal(err)
	}
	left := time.Until(tokens.Tokens[0].ExpiresAt)
	if left < 360*24*time.Hour || left > 366*24*time.Hour {
		t.Fatalf("the default is one year, not %v", left)
	}
	root.want(400, "POST", "/api/v1/admin/scim-tokens", map[string]any{"name": "x", "expires_at": time.Now().Add(-time.Hour)})
	root.want(400, "POST", "/api/v1/admin/scim-tokens", map[string]any{"name": "x", "expires_at": time.Now().AddDate(2, 0, 0)})

	rotated := root.ok("POST", "/api/v1/admin/scim-tokens/"+id+"/rotate", nil)
	fresh := &scimClient{e: e, token: rotated["secret"].(string)}
	old.want(200, "GET", "/Users", nil)
	fresh.want(200, "GET", "/Users", nil)
	if e.auditActions("scim.token_rotated") != 1 {
		t.Fatal("a rotation must be audited")
	}
	if attention := root.want(200, "GET", "/api/v1/admin/attention", nil); !strings.Contains(attention, "tokens_expiring") {
		t.Fatalf("the old token ends within days, and the console says nothing: %s", attention)
	}

	e.sql("UPDATE scim_tokens SET expires_at = now() - interval '1 second' WHERE id = $1", id)
	old.want(401, "GET", "/Users", nil)
	fresh.want(200, "GET", "/Users", nil)
	root.want(404, "POST", "/api/v1/admin/scim-tokens/"+id+"/rotate", nil)
}

func TestATokenWithoutADateLastsAYear(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	made := anna.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "script"})
	end, err := time.Parse(time.RFC3339, made["token"].(map[string]any)["expires_at"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if left := time.Until(end); left < 360*24*time.Hour || left > 366*24*time.Hour {
		t.Fatalf("%v", left)
	}
}
