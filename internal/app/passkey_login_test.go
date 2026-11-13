// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"
)

// givePasskey stores a passkey for an account, as registering one does. Its key is never
// used here: a full login with a passkey is tested in the browser (login e2e).
func givePasskey(t *testing.T, e *env, userID string) {
	t.Helper()
	credential, err := json.Marshal(map[string]any{"id": []byte("key-" + userID), "publicKey": []byte("not-a-key")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.app.Pool().Exec(context.Background(), `INSERT INTO webauthn_credentials (id, user_id, credential_id, credential, name)
		VALUES ($1, $2, $3, $4, 'Laptop')`, uuid.New(), userID, []byte("key-"+userID), credential); err != nil {
		t.Fatal(err)
	}
}

type passwordStep struct {
	MFARequired bool     `json:"mfa_required"`
	MFAToken    string   `json:"mfa_token"`
	MFAMethods  []string `json:"mfa_methods"`
}

func passwordLogin(t *testing.T, e *env, c *http.Client, email string) (*http.Response, passwordStep) {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email, "password": adminPassword})
	resp, body := e.do(c, "POST", "/api/v1/auth/login", string(b))
	var step passwordStep
	_ = json.Unmarshal([]byte(body), &step)
	return resp, step
}

// An account whose only second factor is a passkey is asked for it after the password,
// and the password alone opens no session.
func TestAPasswordLoginAsksForThePasskeyOfAnAccountThatHasOnlyThat(t *testing.T) {
	e := newEnv(t)
	u := e.user("keyed@example.com")
	givePasskey(t, e, u.id)
	c := e.client()
	resp, step := passwordLogin(t, e, c, u.email)
	if resp.StatusCode != http.StatusOK || !step.MFARequired || step.MFAToken == "" {
		t.Fatalf("the password alone was enough: %d %+v", resp.StatusCode, step)
	}
	if !slices.Equal(step.MFAMethods, []string{"passkey"}) {
		t.Fatalf("methods offered: %v", step.MFAMethods)
	}
	if status, _ := e.do(c, "GET", "/api/v1/me", ""); status.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a session after the password alone: %d", status.StatusCode)
	}
	// The code route cannot finish it: there is no code.
	b, _ := json.Marshal(map[string]string{"mfa_token": step.MFAToken, "code": "123456"})
	if resp, _ := e.do(c, "POST", "/api/v1/auth/login/totp", string(b)); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a code finished a passkey login: %d", resp.StatusCode)
	}
}

func TestThePasskeyStepOfALoginNeedsItsOwnChallengeAndAnAssertion(t *testing.T) {
	e := newEnv(t)
	u := e.user("keyed@example.com")
	givePasskey(t, e, u.id)
	c := e.client()
	_, step := passwordLogin(t, e, c, u.email)

	begin := func(token string) (int, map[string]any) {
		b, _ := json.Marshal(map[string]string{"mfa_token": token})
		resp, body := e.do(c, "POST", "/api/v1/auth/login/passkey/begin", string(b))
		out := map[string]any{}
		_ = json.Unmarshal([]byte(body), &out)
		return resp.StatusCode, out
	}
	if status, _ := begin("made-up"); status != http.StatusUnauthorized {
		t.Fatalf("a made-up challenge began a passkey step: %d", status)
	}
	status, started := begin(step.MFAToken)
	if status != http.StatusOK || started["token"] == nil || started["options"] == nil {
		t.Fatalf("begin: %d %v", status, started)
	}
	finish := map[string]any{"mfa_token": step.MFAToken, "token": started["token"], "response": json.RawMessage(`{}`)}
	b, _ := json.Marshal(finish)
	if resp, _ := e.do(c, "POST", "/api/v1/auth/login/passkey/finish", string(b)); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a login finished without an assertion: %d", resp.StatusCode)
	}
	if e.auditActions("auth.login_mfa") != 1 {
		t.Fatal("the failed passkey step is not in the audit log")
	}
	if status, _ := e.do(c, "GET", "/api/v1/me", ""); status.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a session without the passkey: %d", status.StatusCode)
	}
}

// A passkey of another account does not finish the login of this one.
func TestThePasskeyStepIsBoundToTheAccountOfThePassword(t *testing.T) {
	e := newEnv(t)
	first := e.user("first@example.com")
	second := e.user("second@example.com")
	givePasskey(t, e, first.id)
	givePasskey(t, e, second.id)
	c := e.client()
	_, one := passwordLogin(t, e, c, first.email)
	_, two := passwordLogin(t, e, c, second.email)
	b, _ := json.Marshal(map[string]string{"mfa_token": one.MFAToken})
	_, body := e.do(c, "POST", "/api/v1/auth/login/passkey/begin", string(b))
	started := map[string]any{}
	_ = json.Unmarshal([]byte(body), &started)
	// The passkey step of the first account, offered with the challenge of the second.
	b, _ = json.Marshal(map[string]any{"mfa_token": two.MFAToken, "token": started["token"], "response": json.RawMessage(`{}`)})
	if resp, _ := e.do(c, "POST", "/api/v1/auth/login/passkey/finish", string(b)); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("a passkey step crossed accounts: %d", resp.StatusCode)
	}
}

// An account with an authenticator app is offered both, and one without any second
// factor still logs in with the password alone.
func TestALoginOffersTheSecondFactorsTheAccountHas(t *testing.T) {
	e := newEnv(t)
	both := e.user("both@example.com")
	both.enrolTOTP()
	givePasskey(t, e, both.id)
	_, step := passwordLogin(t, e, e.client(), both.email)
	if !slices.Equal(step.MFAMethods, []string{"code", "passkey"}) {
		t.Fatalf("methods for an account with both: %v", step.MFAMethods)
	}
	plain := e.user("plain@example.com")
	resp, step := passwordLogin(t, e, e.client(), plain.email)
	if resp.StatusCode != http.StatusOK || step.MFARequired {
		t.Fatalf("an account without a second factor: %d %+v", resp.StatusCode, step)
	}
	coded := e.user("coded@example.com")
	coded.enrolTOTP()
	if _, step := passwordLogin(t, e, e.client(), coded.email); !slices.Equal(step.MFAMethods, []string{"code"}) {
		t.Fatalf("methods for an account with a code: %v", step.MFAMethods)
	}
}
