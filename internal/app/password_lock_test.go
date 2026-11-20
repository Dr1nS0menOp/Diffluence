// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/kenwerk/kenwerk/internal/identity"
)

// lockedState reads the run of wrong passwords and whether password login stopped.
func (e *env) lockedState(userID string) (int, bool) {
	e.t.Helper()
	var run int
	var locked bool
	err := e.app.Pool().QueryRow(context.Background(), `SELECT failed_logins_in_row, password_locked_at IS NOT NULL
		FROM local_credentials WHERE user_id = $1`, userID).Scan(&run, &locked)
	if err != nil {
		e.t.Fatal(err)
	}
	return run, locked
}

// ADR-038: the hundredth wrong password in a row stops password login, the right
// password then gets the same answer as a wrong one, and the person is told by mail.
func TestTheHundredthWrongPasswordStopsPasswordLogin(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	f, queue := withQueuedMail(e)
	e.sql("UPDATE local_credentials SET failed_logins_in_row = $2 WHERE user_id = $1", anna.id, identity.PasswordLockAfter-2)

	if got := e.login(e.client(), anna.email, "wrong password number 99"); got != http.StatusUnauthorized {
		t.Fatalf("wrong password 99: %d", got)
	}
	if run, locked := e.lockedState(anna.id); run != identity.PasswordLockAfter-1 || locked {
		t.Fatalf("after 99 wrong passwords: run %d, locked %v", run, locked)
	}
	if got := e.login(e.client(), anna.email, "wrong password number 100"); got != http.StatusUnauthorized {
		t.Fatalf("wrong password 100: %d", got)
	}
	if _, locked := e.lockedState(anna.id); !locked {
		t.Fatal("100 wrong passwords in a row did not stop password login")
	}
	_, wrong := e.do(e.client(), "POST", "/api/v1/auth/login", `{"email":"anna@example.com","password":"yet another wrong one"}`)
	resp, right := e.do(e.client(), "POST", "/api/v1/auth/login", `{"email":"anna@example.com","password":"`+adminPassword+`"}`)
	if resp.StatusCode != http.StatusUnauthorized || right != wrong {
		t.Fatalf("the right password after the stop: %d %s, want the answer to a wrong one: %s", resp.StatusCode, right, wrong)
	}
	if e.auditActions("auth.password_locked") != 1 {
		t.Fatal("the stop must be audited once")
	}
	if _, err := queue.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 || f.sent[0].To != anna.email || !strings.Contains(f.sent[0].Subject, "Password login") {
		t.Fatalf("the person was not told: %+v", f.sent)
	}
	// The session the person already had goes on: the stop is about guessing, not about them.
	anna.ok("GET", "/api/v1/me", nil)
}

// The right password ends a run, so wrong passwords spread over months never add up.
func TestTheRightPasswordEndsARunOfWrongOnes(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	e.sql("UPDATE local_credentials SET failed_logins_in_row = $2 WHERE user_id = $1", anna.id, identity.PasswordLockAfter-1)
	if got := e.login(e.client(), anna.email, adminPassword); got != http.StatusOK {
		t.Fatalf("the right password: %d", got)
	}
	if run, locked := e.lockedState(anna.id); run != 0 || locked {
		t.Fatalf("after the right password: run %d, locked %v", run, locked)
	}
}

// Only guessing this account's password counts: an unknown address costs nothing, and
// a disabled account does not count up to a stop that would outlive enabling it again.
func TestOnlyWrongPasswordsOnALiveAccountCount(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	e.login(e.client(), "nobody@example.com", "wrong password here")
	root.ok("POST", "/api/v1/admin/users/"+anna.id+"/disable", nil)
	e.login(e.client(), anna.email, "wrong password here")
	if run, _ := e.lockedState(anna.id); run != 0 {
		t.Fatalf("a disabled account counted: run %d", run)
	}
	root.ok("POST", "/api/v1/admin/users/"+anna.id+"/enable", nil)
	e.login(e.client(), anna.email, "wrong password here")
	if run, _ := e.lockedState(anna.id); run != 1 {
		t.Fatalf("a wrong password on a live account: run %d, want 1", run)
	}
}

// Every place that asks a person with a session for their password again counts a wrong one
// as login does, and the right one ends the run as at login.
func TestAWrongPasswordGivenWithASessionCountsAsAtLogin(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	anna.ok("POST", "/api/v1/auth/reauth", map[string]any{"password": adminPassword})
	wrong := "wrong password here"
	checks := []struct {
		name, path string
		body       map[string]any
	}{
		{"confirming who you are", "/api/v1/auth/reauth", map[string]any{"password": wrong}},
		{"starting an authenticator app", "/api/v1/me/totp/begin", map[string]any{"password": wrong}},
		{"turning the authenticator app off", "/api/v1/me/totp/disable", map[string]any{"password": wrong}},
		{"changing the password", "/api/v1/me/password", map[string]any{"current_password": wrong, "new_password": "another fine passphrase here"}},
	}
	for i, check := range checks {
		anna.want(http.StatusConflict, "POST", check.path, check.body)
		if run, locked := e.lockedState(anna.id); run != i+1 || locked {
			t.Fatalf("a wrong password when %s: run %d, locked %v, want run %d", check.name, run, locked, i+1)
		}
	}
	anna.ok("POST", "/api/v1/auth/reauth", map[string]any{"password": adminPassword})
	if run, locked := e.lockedState(anna.id); run != 0 || locked {
		t.Fatalf("after the right password: run %d, locked %v", run, locked)
	}
}

// The hundredth wrong password when confirming who you are stops password login as at login:
// the person is told, and the right password then gets the answer to a wrong one, here and
// wherever the password is asked again. The session goes on.
func TestTheHundredthWrongPasswordWhenConfirmingWhoYouAreStopsPasswordLogin(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	f, queue := withQueuedMail(e)
	e.sql("UPDATE local_credentials SET failed_logins_in_row = $2 WHERE user_id = $1", anna.id, identity.PasswordLockAfter-2)

	anna.want(http.StatusConflict, "POST", "/api/v1/auth/reauth", map[string]any{"password": "wrong password number 99"})
	if run, locked := e.lockedState(anna.id); run != identity.PasswordLockAfter-1 || locked {
		t.Fatalf("after 99 wrong passwords: run %d, locked %v", run, locked)
	}
	wrong := anna.want(http.StatusConflict, "POST", "/api/v1/auth/reauth", map[string]any{"password": "wrong password number 100"})
	if _, locked := e.lockedState(anna.id); !locked {
		t.Fatal("100 wrong passwords in a row did not stop password login")
	}
	if right := anna.want(http.StatusConflict, "POST", "/api/v1/auth/reauth", map[string]any{"password": adminPassword}); right != wrong {
		t.Fatalf("the right password after the stop: %s, want the answer to a wrong one: %s", right, wrong)
	}
	anna.want(http.StatusConflict, "POST", "/api/v1/me/password",
		map[string]any{"current_password": adminPassword, "new_password": "another fine passphrase here"})
	if got := e.login(e.client(), anna.email, adminPassword); got != http.StatusUnauthorized {
		t.Fatalf("password login after the stop: %d", got)
	}
	if e.auditActions("auth.password_locked") != 1 {
		t.Fatal("the stop must be audited once")
	}
	if _, err := queue.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 || f.sent[0].To != anna.email || !strings.Contains(f.sent[0].Subject, "Password login") {
		t.Fatalf("the person was not told: %+v", f.sent)
	}
	anna.ok("GET", "/api/v1/me", nil)
}

// Wrong passwords at login and when confirming who you are add up to one run and one stop.
func TestWrongPasswordsAtLoginAndWhenConfirmingWhoYouAreAddUpToOneStop(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	e.sql("UPDATE local_credentials SET failed_logins_in_row = $2 WHERE user_id = $1", anna.id, identity.PasswordLockAfter-4)

	e.login(e.client(), anna.email, "a wrong guess at login")
	anna.want(http.StatusConflict, "POST", "/api/v1/auth/reauth", map[string]any{"password": "a wrong guess with a session"})
	e.login(e.client(), anna.email, "another wrong guess at login")
	if run, locked := e.lockedState(anna.id); run != identity.PasswordLockAfter-1 || locked {
		t.Fatalf("after three more wrong passwords: run %d, locked %v", run, locked)
	}
	anna.want(http.StatusConflict, "POST", "/api/v1/auth/reauth", map[string]any{"password": "the last wrong guess"})
	if _, locked := e.lockedState(anna.id); !locked {
		t.Fatal("wrong passwords at login and with a session did not add up to the stop")
	}
	if got := e.login(e.client(), anna.email, adminPassword); got != http.StatusUnauthorized {
		t.Fatalf("password login after the stop: %d", got)
	}
	if e.auditActions("auth.password_locked") != 1 {
		t.Fatal("one stop must be audited once")
	}
}

// An administrator lifts the stop, and it is audited; a new password lifts it too.
func TestAnAdministratorLiftsTheStopAndANewPasswordDoesToo(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	lock := func() {
		t.Helper()
		e.sql("UPDATE local_credentials SET failed_logins_in_row = $2, password_locked_at = now() WHERE user_id = $1", anna.id, identity.PasswordLockAfter)
	}
	lock()
	if detail := root.ok("GET", "/api/v1/admin/users/"+anna.id, nil); detail["password_locked"] != true {
		t.Fatalf("the console does not show the stop: %v", detail)
	}
	anna.want(http.StatusForbidden, "POST", "/api/v1/admin/users/"+anna.id+"/unlock-password", nil)
	root.ok("POST", "/api/v1/admin/users/"+anna.id+"/unlock-password", nil)
	if run, locked := e.lockedState(anna.id); run != 0 || locked {
		t.Fatalf("after lifting: run %d, locked %v", run, locked)
	}
	if e.auditActions("auth.password_unlocked") != 1 {
		t.Fatal("lifting the stop must be audited")
	}
	if got := e.login(e.client(), anna.email, adminPassword); got != http.StatusOK {
		t.Fatalf("password login after lifting: %d", got)
	}
	root.want(http.StatusConflict, "POST", "/api/v1/admin/users/"+anna.id+"/unlock-password", nil)

	lock()
	sent := root.ok("POST", "/api/v1/admin/users/"+anna.id+"/reset-password", nil)
	link, _ := sent["link"].(string)
	token := link[strings.Index(link, "token=")+len("token="):]
	if status, out := publicPost(e, "/api/v1/auth/reset", map[string]any{"token": token, "password": "a sturdy passphrase of her own"}); status != http.StatusNoContent {
		t.Fatalf("reset through the link: %d %s", status, out)
	}
	if _, locked := e.lockedState(anna.id); locked {
		t.Fatal("a new password left the stop in place")
	}
}
