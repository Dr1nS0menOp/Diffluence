// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
)

// An administrator runs accounts, but an account that holds system administration
// is only acted on by someone who holds it too.
func TestOnlyASystemAdministratorActsOnASystemAdministrator(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	other := e.user("other-root@example.com", identity.AllGlobalPermissions...)
	clerk := e.user("clerk@example.com", identity.PermUseProduct, identity.PermAdministrator)
	plain := e.user("plain@example.com", identity.PermUseProduct)

	refused := func(method, path string, body any) {
		t.Helper()
		if out := clerk.want(http.StatusConflict, method, path, body); !strings.Contains(out, "needs_system_administrator") {
			t.Fatalf("%s %s: %s", method, path, out)
		}
	}
	at := "/api/v1/admin/users/" + other.id
	refused("POST", at+"/reset-password", nil)
	refused("POST", at+"/reset-mfa", nil)
	refused("POST", at+"/disable", nil)
	refused("POST", at+"/anonymise", map[string]any{"reason": "a request to be forgotten"})
	refused("PUT", at+"/expiry", map[string]any{"expires_at": "2030-01-01T00:00:00Z"})
	refused("PATCH", at, map[string]any{"display_name": "Someone else", "type": "member"})
	refused("POST", at+"/unlock-password", nil)

	// An ordinary account stays the administrator's to run.
	clerk.ok("POST", "/api/v1/admin/users/"+plain.id+"/reset-password", nil)
	clerk.ok("PUT", "/api/v1/admin/users/"+plain.id+"/expiry", map[string]any{"expires_at": "2030-01-01T00:00:00Z"})
	clerk.ok("POST", "/api/v1/admin/users/"+plain.id+"/disable", nil)

	// Someone who holds system administration may.
	root.ok("PUT", at+"/expiry", map[string]any{"expires_at": "2030-01-01T00:00:00Z"})
	root.ok("POST", at+"/disable", nil)
}

// A group that holds system administration hands it to its members, so changing
// who is in it is for a system administrator too.
func TestOnlyASystemAdministratorChangesASystemAdministrationGroup(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	clerk := e.user("clerk@example.com", identity.PermUseProduct, identity.PermAdministrator)
	plain := e.user("plain@example.com", identity.PermUseProduct)
	group := root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Operators"})
	gid, _ := group["id"].(string)
	root.ok("PUT", "/api/v1/admin/groups/"+gid+"/permissions", map[string]any{"permissions": []string{identity.PermSystemAdministrator}})

	members := "/api/v1/admin/groups/" + gid + "/members/"
	if out := clerk.want(http.StatusConflict, "PUT", members+clerk.id, nil); !strings.Contains(out, "needs_system_administrator") {
		t.Fatalf("an administrator made themselves a system administrator: %s", out)
	}
	clerk.want(http.StatusConflict, "PUT", members+plain.id, nil)
	root.ok("PUT", members+plain.id, nil)
	clerk.want(http.StatusConflict, "DELETE", members+plain.id, nil)
	clerk.want(http.StatusConflict, "DELETE", "/api/v1/admin/groups/"+gid, nil)
	root.ok("DELETE", members+plain.id, nil)

	// An ordinary group stays open to every administrator, but taking a system
	// administrator out of one is acting on their account.
	team := root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Writers"})
	tid, _ := team["id"].(string)
	clerk.ok("PUT", "/api/v1/admin/groups/"+tid+"/members/"+plain.id, nil)
	clerk.ok("PUT", "/api/v1/admin/groups/"+tid+"/members/"+root.id, nil)
	clerk.ok("DELETE", "/api/v1/admin/groups/"+tid+"/members/"+plain.id, nil)
	if out := clerk.want(http.StatusConflict, "DELETE", "/api/v1/admin/groups/"+tid+"/members/"+root.id, nil); !strings.Contains(out, "needs_system_administrator") {
		t.Fatalf("an administrator took a system administrator out of a group: %s", out)
	}
}

// An administrator sends a link; nobody types in someone else's password.
func TestAnAdministratorSendsAResetLinkAndNeverChoosesThePassword(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	anna := e.user("anna@example.com", identity.PermUseProduct)

	// Without mail the link comes back once, to pass on, and the handing out is audited.
	sent := root.ok("POST", "/api/v1/admin/users/"+anna.id+"/reset-password", map[string]any{"password": "ignored and never stored"})
	link, _ := sent["link"].(string)
	if sent["mailed"] != false || !strings.Contains(link, "/reset?token=") {
		t.Fatalf("without mail the link must come back: %v", sent)
	}
	if e.auditActions("auth.reset_link_sent") != 1 {
		t.Fatal("handing out a reset link must be audited")
	}
	if got := e.login(e.client(), anna.email, "ignored and never stored"); got == http.StatusOK {
		t.Fatal("a password in the request was stored")
	}
	if got := e.login(e.client(), anna.email, adminPassword); got != http.StatusOK {
		t.Fatalf("sending a link changed the password already: %d", got)
	}
	token := link[strings.Index(link, "token=")+len("token="):]
	next := "a sturdy passphrase of her own"
	if status, out := publicPost(e, "/api/v1/auth/reset", map[string]any{"token": token, "password": next}); status != http.StatusNoContent {
		t.Fatalf("reset: %d %s", status, out)
	}
	if got := e.login(e.client(), anna.email, next); got != http.StatusOK {
		t.Fatalf("the chosen password: %d", got)
	}

	// A second link ends the first.
	first, _ := root.ok("POST", "/api/v1/admin/users/"+anna.id+"/reset-password", nil)["link"].(string)
	root.ok("POST", "/api/v1/admin/users/"+anna.id+"/reset-password", nil)
	firstToken := first[strings.Index(first, "token=")+len("token="):]
	if status, _ := publicPost(e, "/api/v1/auth/reset", map[string]any{"token": firstToken, "password": next + " again"}); status != http.StatusConflict {
		t.Fatalf("an older link still worked: %d", status)
	}
}

// An account from an identity provider has no password here to reset.
func TestAResetLinkIsOnlyForLocalAccounts(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	sso := e.user("sso@example.com", identity.PermUseProduct)
	e.sql("UPDATE users SET source = $2, external_id = 'ext-1' WHERE id = $1", sso.id, identity.SourceOIDC)
	if out := root.want(http.StatusConflict, "POST", "/api/v1/admin/users/"+sso.id+"/reset-password", nil); !strings.Contains(out, "not_local") {
		t.Fatalf("a provider account got a reset link: %s", out)
	}
	if e.auditActions("auth.reset_link_sent") != 0 {
		t.Fatal("a refused link was recorded as sent")
	}
}
