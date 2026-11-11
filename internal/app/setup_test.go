// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/setup"
)

// firstSetup is a complete request of the setup screen.
func firstSetup(code, email string) map[string]any {
	return map[string]any{"code": code, "organisation": "Acme & Sons", "name": "First Person", "email": email,
		"password": adminPassword, "language": "en", "time_zone": "Europe/Amsterdam"}
}

func (e *env) post(c *http.Client, path string, body any) (int, string) {
	e.t.Helper()
	b, _ := json.Marshal(body)
	resp, out := e.do(c, "POST", path, string(b))
	return resp.StatusCode, out
}

func (e *env) issueCode() string {
	e.t.Helper()
	code, err := e.app.Setup().Issue(context.Background(), "test")
	if err != nil {
		e.t.Fatal(err)
	}
	return code
}

func (e *env) administrators() int {
	e.t.Helper()
	return e.count("SELECT count(*) FROM global_grants WHERE permission = 'administrator'")
}

func TestTheSetupScreenRefusesAnyoneWithoutTheCode(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	e.issueCode()
	for name, code := range map[string]string{"no code": "", "a wrong code": "AAAA-AAAA-AAAA-AAAA"} {
		status, out := e.post(c, "/api/v1/setup", firstSetup(code, "first@example.com"))
		if status != http.StatusForbidden {
			t.Fatalf("%s: got %d %s", name, status, out)
		}
	}
	if e.administrators() != 0 {
		t.Fatal("a refused setup must create nobody")
	}
	if got := e.auditActions("setup.refused"); got != 2 {
		t.Fatalf("every refusal is audited, got %d", got)
	}
	// The code is checked before anything else, so a stranger learns nothing from the form.
	bad := firstSetup("EEEE-EEEE-EEEE-EEEE", "first@example.com")
	bad["organisation"] = "<b>Acme</b>"
	if status, out := e.post(c, "/api/v1/setup", bad); status != http.StatusForbidden {
		t.Fatalf("a wrong code with a wrong form: got %d %s", status, out)
	}
}

func TestTheSetupStaysClosedEvenIfNoAdministratorIsLeft(t *testing.T) {
	e := newEnv(t)
	if status, _ := e.post(e.client(), "/api/v1/setup", firstSetup(e.issueCode(), "first@example.com")); status != http.StatusCreated {
		t.Fatal("setup")
	}
	e.sql("DELETE FROM global_grants WHERE permission = 'administrator'")
	if resp, _ := e.do(e.client(), "GET", "/api/v1/setup", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("a used code closes the setup for good; the way back is the command line: %d", resp.StatusCode)
	}
}

func TestASetupCodeWorksOnceAndThenTheScreenIsGone(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	code := e.issueCode()
	status, out := e.post(c, "/api/v1/setup", firstSetup(code, "first@example.com"))
	if status != http.StatusCreated {
		t.Fatalf("setup: %d %s", status, out)
	}
	if !strings.Contains(out, `"signed_in":true`) {
		t.Fatalf("the new administrator is signed in at once: %s", out)
	}
	if resp, _ := e.do(c, "GET", "/api/v1/me", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("the session works: %d", resp.StatusCode)
	}
	if e.administrators() != 1 {
		t.Fatal("exactly one administrator")
	}
	other := e.client()
	if status, _ := e.post(other, "/api/v1/setup", firstSetup(code, "second@example.com")); status != http.StatusNotFound {
		t.Fatalf("a used code finds nothing: %d", status)
	}
	if resp, _ := e.do(other, "GET", "/api/v1/setup", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("after the setup the route answers as if it does not exist: %d", resp.StatusCode)
	}
	if _, err := e.app.Setup().Issue(context.Background(), "test"); err != setup.ErrClosed {
		t.Fatalf("no new code after the setup: %v", err)
	}
	if e.auditActions("setup.completed") != 1 {
		t.Fatal("the setup is audited")
	}
}

func TestTheSetupStoresTheOrganisationAndTheDefaults(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	if status, out := e.post(c, "/api/v1/setup", firstSetup(e.issueCode(), "first@example.com")); status != http.StatusCreated {
		t.Fatalf("setup: %d %s", status, out)
	}
	resp, out := e.do(c, "GET", "/api/v1/appearance", "")
	look := map[string]any{}
	_ = json.Unmarshal([]byte(out), &look)
	if resp.StatusCode != http.StatusOK || look["organisation"] != "Acme & Sons" {
		t.Fatalf("the search box says Search in Acme & Sons: %s", out)
	}
	if got := e.scalar("SELECT value->>'default_time_zone' FROM settings WHERE key = 'section.general'"); got != "Europe/Amsterdam" {
		t.Fatalf("the time zone of the setup is the default: %q", got)
	}
}

func TestAnExpiredSetupCodeIsRefused(t *testing.T) {
	e := newEnv(t)
	past := func() time.Time { return time.Now().Add(-2 * setup.CodeLifetime) }
	code, err := e.app.Setup().WithClock(past).Issue(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := e.post(e.client(), "/api/v1/setup", firstSetup(code, "first@example.com")); status != http.StatusForbidden {
		t.Fatalf("an expired code: %d", status)
	}
}

func TestANewSetupCodeReplacesTheOldOne(t *testing.T) {
	e := newEnv(t)
	old := e.issueCode()
	e.issueCode()
	if status, _ := e.post(e.client(), "/api/v1/setup", firstSetup(old, "first@example.com")); status != http.StatusForbidden {
		t.Fatalf("the old code: %d", status)
	}
}

func TestASetupCodeEndsAfterTooManyWrongTries(t *testing.T) {
	e := newEnv(t)
	code := e.issueCode()
	for range setup.MaxWrongTries {
		if err := e.app.Setup().Check(context.Background(), "BBBB-BBBB-BBBB-BBBB", setup.Attempt{}); err != setup.ErrRefused {
			t.Fatalf("a wrong code: %v", err)
		}
	}
	if status, _ := e.post(e.client(), "/api/v1/setup", firstSetup(code, "first@example.com")); status != http.StatusForbidden {
		t.Fatalf("after too many wrong tries the right code no longer works: %d", status)
	}
}

func TestOneAddressCannotKeepGuessingSetupCodes(t *testing.T) {
	e := newEnv(t)
	e.issueCode()
	c := e.client()
	last := 0
	for range 11 {
		last, _ = e.post(c, "/api/v1/setup", firstSetup("CCCC-CCCC-CCCC-CCCC", "first@example.com"))
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("the eleventh try from one address is refused: %d", last)
	}
}

func TestTheSetupIsGoneOnceAnAdministratorExists(t *testing.T) {
	e := newEnv(t)
	code := e.issueCode()
	e.createUser("admin@example.com", identity.AllGlobalPermissions...)
	c := e.client()
	if resp, _ := e.do(c, "GET", "/api/v1/setup", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("state: %d", resp.StatusCode)
	}
	if status, _ := e.post(c, "/api/v1/setup", firstSetup(code, "first@example.com")); status != http.StatusNotFound {
		t.Fatalf("even a valid code: %d", status)
	}
	if e.administrators() != 1 {
		t.Fatal("no second administrator")
	}
}

func TestTwoPeopleAtOnceMakeOneAdministrator(t *testing.T) {
	e := newEnv(t)
	code := e.issueCode()
	statuses := make([]int, 2)
	var wg sync.WaitGroup
	for i, email := range []string{"one@example.com", "two@example.com"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i], _ = e.post(e.client(), "/api/v1/setup", firstSetup(code, email))
		}()
	}
	wg.Wait()
	created := 0
	for _, s := range statuses {
		if s == http.StatusCreated {
			created++
		}
	}
	if created != 1 || e.administrators() != 1 {
		t.Fatalf("exactly one wins: %v, %d administrators", statuses, e.administrators())
	}
}

func TestAWrongFieldLeavesTheSetupCodeUsable(t *testing.T) {
	e := newEnv(t)
	code := e.issueCode()
	c := e.client()
	bad := firstSetup(code, "first@example.com")
	bad["organisation"] = "<b>Acme</b>"
	if status, _ := e.post(c, "/api/v1/setup", bad); status != http.StatusBadRequest {
		t.Fatalf("markup in the name: %d", status)
	}
	weak := firstSetup(code, "first@example.com")
	weak["password"] = "short"
	if status, _ := e.post(c, "/api/v1/setup", weak); status != http.StatusBadRequest {
		t.Fatalf("a short password: %d", status)
	}
	if e.administrators() != 0 {
		t.Fatal("nothing is created by a refused form")
	}
	if status, out := e.post(c, "/api/v1/setup", firstSetup(code, "first@example.com")); status != http.StatusCreated {
		t.Fatalf("the code still works after a mistake in the form: %d %s", status, out)
	}
}

func TestNoSetupCodeIsKeptOrAudited(t *testing.T) {
	e := newEnv(t)
	code := e.issueCode()
	e.post(e.client(), "/api/v1/setup", firstSetup("DDDD-DDDD-DDDD-DDDD", "first@example.com"))
	if status, _ := e.post(e.client(), "/api/v1/setup", firstSetup(code, "first@example.com")); status != http.StatusCreated {
		t.Fatal("setup")
	}
	plain := strings.ReplaceAll(code, "-", "")
	for _, needle := range []string{code, plain, "DDDD-DDDD", "DDDDDDDD"} {
		if n := e.count("SELECT count(*) FROM audit_entries WHERE payload::text LIKE '%' || $1 || '%'", needle); n != 0 {
			t.Fatalf("a code appears in the audit log %d times", n)
		}
	}
	if n := e.count("SELECT count(*) FROM setup_codes WHERE encode(code_hash, 'escape') LIKE '%' || $1 || '%'", plain); n != 0 {
		t.Fatal("only a hash of the code is stored")
	}
}
