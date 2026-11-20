// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pquerna/otp/totp"

	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/pages"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// Electronic signatures, wave DC4 of spec 064 (blueprint 09.8). The refusals come first:
// the wrong person, a token, a confirmation reused, expired or made for another step, a
// stale login, a changed version and a removed reviewer. Then what a signature records, how
// it shows and verifies, and how it is kept.

// signingFlow is reviewFlow with the author's own approval signed as approved (DOC-700).
func signingFlow() map[string]any {
	f := reviewFlow()
	for _, t := range f["transitions"].([]any) {
		if step := t.(map[string]any); step["id"] == "approve" {
			step["signature_meaning"] = "approved"
		}
	}
	return f
}

// signedReviewFlow asks the reviewers given, all of them, and every approval is signed.
func signedReviewFlow(reviewers ...string) map[string]any {
	entries := []any{}
	for _, id := range reviewers {
		entries = append(entries, map[string]any{"type": "user", "id": id})
	}
	return map[string]any{
		"states": []any{
			map[string]any{"id": "draft", "name": "Draft", "kind": "draft", "start": true},
			map[string]any{"id": "review", "name": "In review", "kind": "in_review", "review": map[string]any{"reviewers": entries, "rule": "all"}},
			map[string]any{"id": "approved", "name": "Approved", "kind": "approved"},
			map[string]any{"id": "rejected", "name": "Changes asked", "kind": "rejected"},
		},
		"transitions": []any{
			map[string]any{"id": "submit", "name": "Send for review", "from": "draft", "to": "review"},
			map[string]any{"id": "approve", "name": "Approve", "from": "review", "to": "approved", "outcome": "approved", "signature_meaning": "approved"},
			map[string]any{"id": "changes", "name": "Ask for changes", "from": "review", "to": "rejected", "outcome": "changes"},
			map[string]any{"id": "rework", "name": "Work on it again", "from": "rejected", "to": "draft"},
		},
	}
}

// signingWorld is a control world whose author may sign, with an authenticator app, and a
// workflow whose approval the author signs.
type signingWorld struct {
	controlWorld
	root    *user
	secret  string
	signing string
}

func newSigningWorld(e *env, key string) signingWorld {
	e.t.Helper()
	w := signingWorld{controlWorld: newControlWorld(e, key), root: admin(e)}
	w.signing = w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": "Signed", "definition": signingFlow()})["id"].(string)
	w.secret, _ = w.author.enrolTOTP()
	w.root.ok("PUT", "/api/v1/admin/users/"+w.author.id+"/signing", map[string]any{"identified_by": identity.IdentifiedInPerson})
	return w
}

// signedPage is a page under the signing workflow, sent for review, waiting for its signed
// approval.
func (w signingWorld) signedPage(title, text string) string {
	w.e.t.Helper()
	page := w.author.page(w.space, w.home, title, text)
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.signing})
	w.step(page, "submit")
	return page
}

// freshCode gives a code of an authenticator app that has not been used. A code counts once
// per step of 30 seconds, so the test forgets the last step used, as only a test may.
func (e *env) freshCode(u *user, secret string) string {
	e.t.Helper()
	e.sql("UPDATE totp_credentials SET last_step = 0 WHERE user_id = $1", u.id)
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		e.t.Fatal(err)
	}
	return code
}

// begin opens a signing step and answers its prompt.
func begin(u *user, page string, step map[string]any) map[string]any {
	u.e.t.Helper()
	return u.ok("POST", "/api/v1/pages/"+page+"/signing", step)
}

func tokenOf(prompt map[string]any) string { return prompt["start"].(map[string]any)["token"].(string) }

// confirm opens a signing step and confirms it with the password and a code.
func (e *env) confirm(u *user, secret, page string, step map[string]any) string {
	e.t.Helper()
	token := tokenOf(begin(u, page, step))
	if status, out := u.do("POST", "/api/v1/signing/confirm", map[string]any{"token": token, "password": adminPassword,
		"code": e.freshCode(u, secret)}); status != http.StatusNoContent {
		e.t.Fatalf("confirm: %d %s", status, out)
	}
	return token
}

func (w signingWorld) approveStep(page string) map[string]any {
	return map[string]any{"transition": "approve", "version": w.head(page)}
}

// signApproval signs and takes the author's approval of a page.
func (w signingWorld) signApproval(page string) {
	w.e.t.Helper()
	token := w.e.confirm(w.author, w.secret, page, w.approveStep(page))
	w.author.ok("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": w.head(page), "confirmation": token})
}

func wantRefusal(t *testing.T, status int, out string, wantStatus int, code string) {
	t.Helper()
	if status != wantStatus || !strings.Contains(out, code) {
		t.Fatalf("got %d %s, want %d %s", status, out, wantStatus, code)
	}
}

// seal puts every waiting audit event into the chain, as the worker does each second.
func (e *env) seal() {
	e.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { e.app.RunSealerForTest(ctx, 10*time.Millisecond); close(stopped) }()
	deadline := time.Now().Add(10 * time.Second)
	for e.auditWaiting() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-stopped
	if n := e.auditWaiting(); n != 0 {
		e.t.Fatalf("%d audit events still wait", n)
	}
}

// DOC-733, DOC-751, DOC-014, DOC-1422: only a person who may sign signs, in a session of
// their own. A role, the space's administration, a token or a guest never signs.
func TestASignatureIsRefusedToAnyoneButItsSigner(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "WHO")
	page := w.signedPage("Cleaning procedure", "first text")

	// Without a confirmation the step is refused, and nothing moves.
	status, out := w.author.do("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": w.head(page)})
	wantRefusal(t, status, out, http.StatusForbidden, "signature_required")

	// The space's administrator may take the step, but signing is never part of a role.
	w.admin.enrolTOTP()
	status, out = w.admin.do("POST", "/api/v1/pages/"+page+"/signing", w.approveStep(page))
	wantRefusal(t, status, out, http.StatusForbidden, "signing_not_enabled")

	// A reader with the right to sign still may not take an author's step (DOC-760).
	w.root.ok("PUT", "/api/v1/admin/users/"+w.reader.id+"/signing", map[string]any{"identified_by": identity.IdentifiedByVideo})
	if status, _ := w.reader.do("POST", "/api/v1/pages/"+page+"/signing", w.approveStep(page)); status != 403 && status != 404 {
		t.Fatalf("a reader opened a signing step: %d", status)
	}

	// A guest can never be given the right to sign, and an administrator never gives it to
	// themselves.
	status, out = w.root.do("PUT", "/api/v1/admin/users/"+w.guest.id+"/signing", map[string]any{"identified_by": identity.IdentifiedInPerson})
	wantRefusal(t, status, out, http.StatusConflict, "cannot_sign")
	status, out = w.root.do("PUT", "/api/v1/admin/users/"+w.root.id+"/signing", map[string]any{"identified_by": identity.IdentifiedInPerson})
	wantRefusal(t, status, out, http.StatusConflict, "own_signing")
	status, out = w.root.do("PUT", "/api/v1/admin/users/"+w.reader.id+"/signing", map[string]any{"identified_by": "a passport, number 123"})
	wantRefusal(t, status, out, http.StatusBadRequest, "")

	// A token reaches neither the signing step nor the signature, even with a confirmation
	// the person made in the browser.
	token := e.confirm(w.author, w.secret, page, w.approveStep(page))
	secret := w.author.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "script", "scopes": []string{"read", "write"}})["secret"].(string)
	status, out = e.bearer("POST", "/api/v1/pages/"+page+"/signing", secret, fmt.Sprintf(`{"transition":"approve","version":%d}`, w.head(page)))
	wantRefusal(t, status, out, http.StatusForbidden, "session_only")
	status, out = e.bearer("POST", "/api/v1/pages/"+page+"/transitions", secret,
		fmt.Sprintf(`{"transition":"approve","version":%d,"confirmation":%q}`, w.head(page), token))
	wantRefusal(t, status, out, http.StatusForbidden, "signing_needs_person")

	// Another person's confirmation is not yours.
	other := e.user("who-other@example.com", identity.PermUseProduct)
	w.admin.grant(w.space, "user", other.id, permissions.TemplateEditor)
	otherSecret, _ := other.enrolTOTP()
	w.root.ok("PUT", "/api/v1/admin/users/"+other.id+"/signing", map[string]any{"identified_by": identity.IdentifiedByHRRecords})
	theirs := e.confirm(other, otherSecret, page, w.approveStep(page))
	status, out = w.author.do("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": w.head(page), "confirmation": theirs})
	wantRefusal(t, status, out, http.StatusForbidden, "signing_confirmation_invalid")

	if n := e.count("SELECT count(*) FROM signatures"); n != 0 {
		t.Fatalf("%d signatures were made by refused steps", n)
	}
	if w.state(page) != "review" {
		t.Fatalf("a refused signature moved the page to %s", w.state(page))
	}
	// The confirmation the token could not use is still the author's, for two minutes.
	w.author.ok("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": w.head(page), "confirmation": token})
	if w.state(page) != "approved" {
		t.Fatalf("the signed approval did not approve: %s", w.state(page))
	}
}

// DOC-730, DOC-1416: a confirmation counts for exactly one signature, of the step and version
// it was made for, within two minutes; a password needs a code beside it, and a login or a
// confirmation for other sensitive work does not count.
func TestAConfirmationCountsOnceForOneSignatureWithinTwoMinutes(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "ONCE")
	first := w.signedPage("Procedure A", "text a")
	second := w.signedPage("Procedure B", "text b")
	take := func(page, token string) (int, string) {
		return w.author.do("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": w.head(page), "confirmation": token})
	}

	// A stale login: confirming who you are for sensitive work is not a signature's confirmation.
	w.author.ok("POST", "/api/v1/auth/reauth", map[string]any{"password": adminPassword})
	status, out := take(first, "")
	wantRefusal(t, status, out, http.StatusForbidden, "signature_required")

	// The step must be confirmed before it signs.
	unconfirmed := tokenOf(begin(w.author, first, w.approveStep(first)))
	status, out = take(first, unconfirmed)
	wantRefusal(t, status, out, http.StatusForbidden, "signing_confirmation_invalid")

	// A password alone is refused, and so is a wrong code; the answer never says which part was wrong.
	token := tokenOf(begin(w.author, first, w.approveStep(first)))
	status, out = w.author.do("POST", "/api/v1/signing/confirm", map[string]any{"token": token, "password": adminPassword, "code": ""})
	wantRefusal(t, status, out, http.StatusForbidden, "wrong_confirmation")
	status, out = w.author.do("POST", "/api/v1/signing/confirm", map[string]any{"token": token, "password": "not the password", "code": e.freshCode(w.author, w.secret)})
	wantRefusal(t, status, out, http.StatusForbidden, "wrong_confirmation")

	// A confirmation for one page does not sign another.
	forFirst := e.confirm(w.author, w.secret, first, w.approveStep(first))
	status, out = take(second, forFirst)
	wantRefusal(t, status, out, http.StatusForbidden, "signing_confirmation_invalid")

	// An expired confirmation is refused.
	e.sql("UPDATE signing_confirmations SET expires_at = now() - interval '1 second'")
	status, out = take(first, forFirst)
	wantRefusal(t, status, out, http.StatusForbidden, "signing_confirmation")

	// A fresh one signs, once: replaying it is refused.
	fresh := e.confirm(w.author, w.secret, first, w.approveStep(first))
	if status, out := take(first, fresh); status != 200 {
		t.Fatalf("a fresh confirmation did not sign: %d %s", status, out)
	}
	w.publishAs(w.author, first, "Procedure A", "text a, changed", false)
	w.step(first, "submit")
	status, out = take(first, fresh)
	wantRefusal(t, status, out, http.StatusForbidden, "signing_confirmation_invalid")
	if n := e.count("SELECT count(*) FROM signatures"); n != 1 {
		t.Fatalf("%d signatures, want 1", n)
	}

	// A confirmation belongs to the session it was made in: the same person in another
	// session cannot use it.
	elsewhere := &user{e: e, c: e.client(), email: w.author.email, id: w.author.id}
	login := e.loginBody(elsewhere.c, w.author.email)
	if got := e.secondStep(elsewhere.c, login["mfa_token"].(string), e.freshCode(w.author, w.secret)); got != 200 {
		t.Fatalf("a second login: %d", got)
	}
	inFirstSession := e.confirm(w.author, w.secret, second, w.approveStep(second))
	status, out = elsewhere.do("POST", "/api/v1/pages/"+second+"/transitions", map[string]any{"transition": "approve",
		"version": w.head(second), "confirmation": inFirstSession})
	wantRefusal(t, status, out, http.StatusForbidden, "signing_confirmation_invalid")

	// A person without a second factor cannot sign with a password alone: the dialog says so.
	plain := e.user("once-plain@example.com", identity.PermUseProduct)
	w.admin.grant(w.space, "user", plain.id, permissions.TemplateEditor)
	w.root.ok("PUT", "/api/v1/admin/users/"+plain.id+"/signing", map[string]any{"identified_by": identity.IdentifiedAsKnownPerson})
	status, out = plain.do("POST", "/api/v1/pages/"+second+"/signing", w.approveStep(second))
	wantRefusal(t, status, out, http.StatusForbidden, "second_factor_required")
}

// DOC-330, DOC-711: a confirmation is for one version. A version published after the step
// was confirmed is not the one the person confirmed, and the signature is refused.
func TestASignatureIsRefusedForAVersionThatChangedAfterItWasConfirmed(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "CHANGED")
	page := w.signedPage("Procedure", "what was read")
	before := w.head(page)
	token := e.confirm(w.author, w.secret, page, w.approveStep(page))
	w.publishAs(w.author, page, "Procedure", "something else", false)
	w.step(page, "submit")
	status, out := w.author.do("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": before, "confirmation": token})
	wantRefusal(t, status, out, http.StatusConflict, "superseded")
	status, out = w.author.do("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": w.head(page), "confirmation": token})
	wantRefusal(t, status, out, http.StatusForbidden, "signing_confirmation_invalid")
	if n := e.count("SELECT count(*) FROM signatures"); n != 0 {
		t.Fatalf("%d signatures of a changed version", n)
	}
}

// DOC-700, DOC-202: every reviewer who approves signs; a reviewer taken off the round cannot
// sign with a confirmation made before, and a signed approval stands.
func TestEveryApprovingReviewerSignsAndARemovedOneCannot(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "REVIEW")
	people := map[string]*user{}
	secrets := map[string]string{}
	for _, name := range []string{"ann", "bob", "cy"} {
		u := e.user("review-"+name+"@example.com", identity.PermUseProduct)
		w.admin.grant(w.space, "user", u.id, permissions.TemplateViewer)
		secrets[name], _ = u.enrolTOTP()
		w.root.ok("PUT", "/api/v1/admin/users/"+u.id+"/signing", map[string]any{"identified_by": identity.IdentifiedInPerson})
		people[name] = u
	}
	flow := w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": "Reviewed and signed",
		"definition": signedReviewFlow(people["ann"].id, people["bob"].id, people["cy"].id)})["id"].(string)
	page := w.author.page(w.space, w.home, "Release note", "reviewed text")
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": flow})
	w.step(page, "submit")
	head := w.head(page)
	answer := func(u *user, token string) (int, string) {
		return u.do("POST", "/api/v1/pages/"+page+"/review/answer", map[string]any{"answer": "approve", "version": head, "confirmation": token})
	}

	status, out := answer(people["ann"], "")
	wantRefusal(t, status, out, http.StatusForbidden, "signature_required")
	approval := map[string]any{"version": head}
	// Cy confirms, then is taken off the round: the confirmation signs nothing.
	cyToken := e.confirm(people["cy"], secrets["cy"], page, approval)
	w.admin.ok("POST", "/api/v1/pages/"+page+"/reviewers/"+people["cy"].id+"/remove", map[string]any{"reason": "on leave"})
	if status, _ := answer(people["cy"], cyToken); status != http.StatusForbidden && status != http.StatusNotFound {
		t.Fatalf("a removed reviewer signed: %d", status)
	}
	if status, _ := people["cy"].do("POST", "/api/v1/pages/"+page+"/signing", approval); status != 403 && status != 404 && status != 409 {
		t.Fatalf("a removed reviewer opened a signing step: %d", status)
	}

	if status, out := answer(people["ann"], e.confirm(people["ann"], secrets["ann"], page, approval)); status != 200 {
		t.Fatalf("ann: %d %s", status, out)
	}
	// A signed approval cannot become a request for changes.
	status, out = people["ann"].do("POST", "/api/v1/pages/"+page+"/review/answer", map[string]any{"answer": "changes", "comment": "after all", "version": head})
	wantRefusal(t, status, out, http.StatusConflict, "signed_answer_final")
	if w.state(page) != "review" {
		t.Fatal("one of two approvals passed the round")
	}
	if status, out := answer(people["bob"], e.confirm(people["bob"], secrets["bob"], page, approval)); status != 200 {
		t.Fatalf("bob: %d %s", status, out)
	}
	if w.state(page) != "approved" {
		t.Fatalf("both signed approvals did not approve: %s", w.state(page))
	}
	signed := w.control(w.reader, page)["signatures"].([]any)
	if len(signed) != 2 {
		t.Fatalf("a reader sees %d signatures, want 2: %v", len(signed), signed)
	}
	if e.count("SELECT count(*) FROM signatures WHERE round_id IS NOT NULL AND stage = 1") != 2 {
		t.Fatal("the signatures do not name their round and stage")
	}
}

// DOC-712: the signature and its step are one transaction. A step that fails leaves no
// signature, and the confirmation stays the person's.
func TestAFailedStepLeavesNoSignature(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "FAIL")
	// The signed step leads into a review state that names nobody, which refuses the step.
	flow := signingFlow()
	for _, s := range flow["states"].([]any) {
		if st := s.(map[string]any); st["id"] == "review" {
			st["review"] = map[string]any{"reviewers": []any{}, "rule": "all"}
		}
	}
	flow["transitions"] = []any{
		map[string]any{"id": "submit", "name": "Send for review", "from": "draft", "to": "review", "signature_meaning": "reviewed"},
		map[string]any{"id": "approve", "name": "Approve", "from": "review", "to": "approved", "outcome": "approved"},
		map[string]any{"id": "reject", "name": "Reject", "from": "review", "to": "rejected", "outcome": "changes"},
		map[string]any{"id": "rework", "name": "Rework", "from": "rejected", "to": "draft"},
		map[string]any{"id": "withdraw", "name": "Withdraw", "from": "approved", "to": "withdrawn"},
	}
	id := w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": "Nobody", "definition": flow})["id"].(string)
	page := w.author.page(w.space, w.home, "Procedure", "text")
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": id})
	step := map[string]any{"transition": "submit", "version": w.head(page)}
	token := e.confirm(w.author, w.secret, page, step)
	status, out := w.author.do("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "submit", "version": w.head(page), "confirmation": token})
	wantRefusal(t, status, out, http.StatusConflict, "no_reviewers")
	if e.count("SELECT count(*) FROM signatures") != 0 || e.auditActions("page.signed") != 0 {
		t.Fatal("a failed step left a signature")
	}
	if e.count("SELECT count(*) FROM signing_confirmations WHERE confirmed_at IS NOT NULL") != 1 {
		t.Fatal("a failed step used up the confirmation")
	}
}

// DOC-732: a wrong password or code of a signing confirmation counts toward the stop after
// wrong passwords, and the hundredth stops password login.
func TestFailedSigningConfirmationsCountTowardTheLockout(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "LOCK")
	page := w.signedPage("Procedure", "text")
	token := tokenOf(begin(w.author, page, w.approveStep(page)))
	w.author.do("POST", "/api/v1/signing/confirm", map[string]any{"token": token, "password": "a wrong guess", "code": "000000"})
	if n, locked := e.lockedState(w.author.id); n != 1 || locked {
		t.Fatalf("after a wrong password: %d in a row, locked %v", n, locked)
	}
	w.author.do("POST", "/api/v1/signing/confirm", map[string]any{"token": token, "password": adminPassword, "code": "000000"})
	if n, _ := e.lockedState(w.author.id); n != 2 {
		t.Fatalf("a wrong code did not count: %d", n)
	}
	e.sql("UPDATE local_credentials SET failed_logins_in_row = $2 WHERE user_id = $1", w.author.id, identity.PasswordLockAfter-1)
	w.author.do("POST", "/api/v1/signing/confirm", map[string]any{"token": token, "password": "another guess", "code": "000000"})
	if _, locked := e.lockedState(w.author.id); !locked {
		t.Fatal("the hundredth wrong confirmation did not stop password login")
	}
	if e.auditActions("auth.password_locked") != 1 || e.count("SELECT count(*) FROM audit_pending WHERE action = 'auth.signing_failed'") < 3 {
		t.Fatal("the failures and the stop are not in the audit log")
	}
	// A stopped password signs nothing, even when it is right.
	status, out := w.author.do("POST", "/api/v1/signing/confirm", map[string]any{"token": token, "password": adminPassword, "code": e.freshCode(w.author, w.secret)})
	wantRefusal(t, status, out, http.StatusForbidden, "wrong_confirmation")
}

// softKey is a passkey on a device made in the test: a P-256 key and its counter.
type softKey struct {
	id    []byte
	key   *ecdsa.PrivateKey
	count uint32
}

// addSoftPasskey registers a passkey for a person directly, as only a test does.
func (e *env) addSoftPasskey(u *user) *softKey {
	e.t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		e.t.Fatal(err)
	}
	k := &softKey{id: make([]byte, 16), key: key}
	_, _ = rand.Read(k.id)
	// The public key in COSE, as an authenticator gives it: {1: 2, 3: -7, -1: 1, -2: x, -3: y}.
	point, err := key.PublicKey.Bytes() // 0x04, then x and y of 32 bytes each
	if err != nil {
		e.t.Fatal(err)
	}
	cose := []byte{0xa5, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21, 0x58, 0x20}
	cose = append(cose, point[1:33]...)
	cose = append(cose, 0x22, 0x58, 0x20)
	cose = append(cose, point[33:65]...)
	body, err := json.Marshal(webauthn.Credential{ID: k.id, PublicKey: cose, AttestationType: "none",
		Flags: webauthn.CredentialFlags{UserPresent: true, UserVerified: true}})
	if err != nil {
		e.t.Fatal(err)
	}
	e.sql("INSERT INTO webauthn_credentials (id, user_id, credential_id, credential, name) VALUES (gen_random_uuid(), $1, $2, $3, 'test key')",
		u.id, k.id, body)
	return k
}

// assert answers the request of a passkey step. verified says whether the device checked a
// PIN or a fingerprint, the user verification a signature needs.
func (k *softKey) assert(t *testing.T, options map[string]any, verified bool) json.RawMessage {
	t.Helper()
	request := options["publicKey"].(map[string]any)
	client, err := json.Marshal(map[string]any{"type": "webauthn.get", "challenge": request["challenge"], "origin": "http://localhost", "crossOrigin": false})
	if err != nil {
		t.Fatal(err)
	}
	rp := sha256.Sum256([]byte(request["rpId"].(string)))
	flags := byte(0x01)
	if verified {
		flags |= 0x04
	}
	k.count++
	auth := append(rp[:], flags, byte(k.count>>24), byte(k.count>>16), byte(k.count>>8), byte(k.count))
	clientHash := sha256.Sum256(client)
	digest := sha256.Sum256(append(append([]byte{}, auth...), clientHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, k.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	b := base64.RawURLEncoding.EncodeToString
	out, err := json.Marshal(map[string]any{"id": b(k.id), "rawId": b(k.id), "type": "public-key",
		"response": map[string]any{"authenticatorData": b(auth), "clientDataJSON": b(client), "signature": b(sig)}})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// DOC-730: a passkey signs only with user verification, and the record names the method.
func TestAPasskeySignsOnlyWithUserVerification(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "PASSKEY")
	key := e.addSoftPasskey(w.author)
	page := w.signedPage("Procedure", "text")
	prompt := begin(w.author, page, w.approveStep(page))
	start := prompt["start"].(map[string]any)
	if methods := fmt.Sprint(start["methods"]); !strings.Contains(methods, identity.SigningByPasskey) {
		t.Fatalf("methods: %s", methods)
	}
	options := start["passkey"].(map[string]any)
	token := tokenOf(prompt)
	status, out := w.author.do("POST", "/api/v1/signing/passkey", map[string]any{"token": token, "response": key.assert(t, options, false)})
	wantRefusal(t, status, out, http.StatusForbidden, "passkey_refused")
	status, out = w.author.do("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": w.head(page), "confirmation": token})
	wantRefusal(t, status, out, http.StatusForbidden, "signing_confirmation_invalid")

	prompt = begin(w.author, page, w.approveStep(page))
	token = tokenOf(prompt)
	if status, out := w.author.do("POST", "/api/v1/signing/passkey", map[string]any{"token": token,
		"response": key.assert(t, prompt["start"].(map[string]any)["passkey"].(map[string]any), true)}); status != 204 {
		t.Fatalf("a verified passkey was refused: %d %s", status, out)
	}
	w.author.ok("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": w.head(page), "confirmation": token})
	if got := e.scalar("SELECT method FROM signatures"); got != identity.SigningByPasskey {
		t.Fatalf("method %q", got)
	}
}

// DOC-731: someone who signs in only through the identity provider confirms with a fresh login
// there, after the signing step began. An old authentication time, or another account at the
// provider, confirms nothing; the login opens no session of its own.
func TestTheIdentityProviderMustConfirmAfterTheSigningStepBegan(t *testing.T) {
	p := newIDP(t)
	e := newEnvWithOIDC(t, p)
	root := admin(e)
	owner := e.user("idp-owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, home := owner.space("IDP")
	flow := owner.ok("POST", "/api/v1/spaces/"+space+"/workflows", map[string]any{"name": "Signed", "definition": signingFlow()})["id"].(string)

	c := noRedirect()
	who := map[string]any{"sub": "signer-1", "email": "signer@example.com", "name": "Signer At Provider", "email_verified": true}
	if to := e.oidcLogin(p, c, who); to != "/" {
		t.Fatalf("login at the provider: %s", to)
	}
	signer := &user{e: e, c: c, email: "signer@example.com", id: e.scalar("SELECT id FROM users WHERE email = 'signer@example.com'")}
	owner.grant(space, "user", signer.id, permissions.TemplateEditor)
	root.ok("PUT", "/api/v1/admin/users/"+signer.id+"/signing", map[string]any{"identified_by": identity.IdentifiedInPerson})
	page := signer.page(space, home, "Procedure", "text")
	signer.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": flow})
	head := func() int {
		return int(signer.ok("GET", "/api/v1/pages/"+page, nil)["control"].(map[string]any)["head_version"].(float64))
	}
	signer.ok("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "submit", "version": head()})

	confirmAt := func(claims map[string]any) (string, url.Values) {
		t.Helper()
		prompt := begin(signer, page, map[string]any{"transition": "approve", "version": head()})
		if methods := fmt.Sprint(prompt["start"].(map[string]any)["methods"]); methods != "[identity_provider]" {
			t.Fatalf("methods of someone who signs in at the provider only: %s", methods)
		}
		token := tokenOf(prompt)
		resp, _ := e.do(c, "GET", "/api/v1/auth/oidc/sign?token="+url.QueryEscape(token), "")
		if resp.StatusCode != http.StatusFound {
			t.Fatalf("start: %d", resp.StatusCode)
		}
		loc, _ := url.Parse(resp.Header.Get("Location"))
		q := loc.Query()
		code := fmt.Sprintf("code-%d", time.Now().UnixNano())
		p.mu.Lock()
		p.codes[code], p.nonces[code], p.challenges[code] = claims, q.Get("nonce"), q.Get("code_challenge")
		p.mu.Unlock()
		resp, _ = e.do(c, "GET", "/api/v1/auth/oidc/callback?state="+url.QueryEscape(q.Get("state"))+"&code="+code, "")
		if q.Get("prompt") != "login" || q.Get("max_age") != "0" {
			t.Fatalf("the provider was not asked for a fresh login: %v", q)
		}
		return token, url.Values{"to": {resp.Header.Get("Location")}}
	}
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
	sessions := e.count("SELECT count(*) FROM sessions WHERE user_id = $1", signer.id)

	token, back := confirmAt(with(map[string]any{"auth_time": time.Now().Add(-time.Hour).Unix()}))
	if back.Get("to") != "/pages/"+page+"?signing=failed" {
		t.Fatalf("an old login at the provider: %s", back.Get("to"))
	}
	status, out := signer.do("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": head(), "confirmation": token})
	wantRefusal(t, status, out, http.StatusForbidden, "signing_confirmation_invalid")

	_, back = confirmAt(map[string]any{"sub": "someone-else", "email": "else@example.com", "auth_time": time.Now().Unix(), "email_verified": true})
	if back.Get("to") != "/pages/"+page+"?signing=failed" {
		t.Fatalf("another account at the provider: %s", back.Get("to"))
	}

	token, back = confirmAt(with(map[string]any{"auth_time": time.Now().Unix()}))
	if back.Get("to") != "/pages/"+page+"?signing=confirmed" {
		t.Fatalf("a fresh login at the provider: %s", back.Get("to"))
	}
	signer.ok("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": head(), "confirmation": token})
	if got := e.scalar("SELECT method FROM signatures"); got != identity.SigningByProvider {
		t.Fatalf("method %q", got)
	}
	if n := e.count("SELECT count(*) FROM sessions WHERE user_id = $1", signer.id); n != sessions {
		t.Fatalf("a login for a signature opened a session: %d, before %d", n, sessions)
	}
	if e.count("SELECT count(*) FROM users WHERE email = 'else@example.com'") != 0 {
		t.Fatal("a login for a signature made an account")
	}
}

// DOC-710, DOC-712, DOC-1225: the record holds who signed under the name of that moment, the
// meaning, the time, the page, version, workflow version, step and method, and the hash, and
// its audit entry holds the hash too. A person renamed later keeps the printed name.
func TestASignatureRecordHoldsEveryField(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "FIELDS")
	page := w.signedPage("Cleaning procedure B", "text")
	version := w.head(page)
	w.signApproval(page)
	var rec struct {
		signer, printed, meaning, meaningName, desc, method, transition, workflow, hash string
		version, workflowVersion, form                                                  int
		signedAt                                                                        time.Time
	}
	if err := e.app.Pool().QueryRow(context.Background(), `SELECT signer_id::text, printed_name, meaning, meaning_name, meaning_description,
			method, transition_id, workflow_id::text, encode(content_hash, 'hex'), version_number, workflow_version, content_form, signed_at
		FROM signatures WHERE page_id = $1`, page).Scan(&rec.signer, &rec.printed, &rec.meaning, &rec.meaningName, &rec.desc, &rec.method,
		&rec.transition, &rec.workflow, &rec.hash, &rec.version, &rec.workflowVersion, &rec.form, &rec.signedAt); err != nil {
		t.Fatal(err)
	}
	if rec.signer != w.author.id || rec.printed != "Test "+w.author.email || rec.meaning != "approved" || rec.meaningName != "Approved" ||
		rec.desc == "" || rec.method != identity.SigningByPasswordAndCode || rec.transition != "approve" || rec.workflow != w.signing ||
		rec.version != version || rec.workflowVersion != 1 || rec.form != 1 || len(rec.hash) != 64 || time.Since(rec.signedAt) > time.Minute {
		t.Fatalf("record: %+v", rec)
	}
	if got := e.scalar("SELECT encode(content_hash, 'hex') FROM version_hashes WHERE page_id = $1 AND number = $2", page, version); got != rec.hash {
		t.Fatalf("the version keeps %s, the record %s", got, rec.hash)
	}
	if e.count(`SELECT count(*) FROM audit_pending WHERE action = 'page.signed' AND payload LIKE '%' || $1 || '%'`, rec.hash) != 1 {
		t.Fatal("the audit entry of the signature does not hold its hash")
	}
	w.root.ok("PATCH", "/api/v1/admin/users/"+w.author.id, map[string]any{"display_name": "A New Name", "type": "member"})
	shown := w.control(w.reader, page)["signatures"].([]any)[0].(map[string]any)
	if shown["signer_name"] != "Test "+w.author.email || shown["meaning_name"] != "Approved" || shown["mark"] != "valid" {
		t.Fatalf("shown: %v", shown)
	}
	// Every human-readable copy names the signature too (Part 11 §11.50(b)).
	var form string
	if text := exported(w.reader, w.controlWorld, page, "text", &form, map[string]any{}); !strings.Contains(text, "Signed as Approved by Test "+w.author.email) {
		t.Fatalf("the export of a signed version: %s", text)
	}
	history := w.reader.ok("GET", "/api/v1/pages/"+page+"/versions", nil)
	if !strings.Contains(fmt.Sprint(history), "Test "+w.author.email) {
		t.Fatalf("the history does not show the signature: %v", history)
	}
}

// DOC-741: tampering with the database turns the mark to "does not match", with the reason:
// the content of a version, a record moved to another version, and its audit entry.
func TestTamperingInTheDatabaseTurnsTheMarkToDoesNotMatch(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "TAMPER")
	content, moved, entry := w.signedPage("Content", "text one"), w.signedPage("Moved", "text two"), w.signedPage("Entry", "text three")
	for _, page := range []string{content, moved, entry} {
		w.signApproval(page)
	}
	e.seal()
	mark := func(page string, version int) (string, string) {
		t.Helper()
		list := w.author.ok("GET", fmt.Sprintf("/api/v1/pages/%s/versions/%d/signatures", page, version), nil)["signatures"].([]any)
		if len(list) == 0 {
			return "", "none"
		}
		s := list[0].(map[string]any)
		reason, _ := s["reason"].(string)
		return s["mark"].(string), reason
	}
	versionOf := func(page string) int {
		return e.count("SELECT version_number FROM signatures WHERE page_id = $1", page)
	}
	for _, page := range []string{content, moved, entry} {
		if m, r := mark(page, versionOf(page)); m != "valid" {
			t.Fatalf("before tampering: %s %s", m, r)
		}
	}
	// The check of every signature, which a restore drill runs (DOC-772).
	if res, err := pages.VerifyAllSignatures(context.Background(), e.app.Pool()); err != nil || res.Checked != 3 || len(res.Mismatches) != 0 {
		t.Fatalf("before tampering: %+v %v", res, err)
	}

	e.sql("ALTER TABLE page_versions DISABLE TRIGGER page_versions_no_update")
	e.sql(`UPDATE page_versions SET document = jsonb_set(document, '{content,0,content,0,text}', '"tampered"') WHERE page_id = $1 AND number = $2`,
		content, versionOf(content))
	if m, r := mark(content, versionOf(content)); m != "does_not_match" || r != "content_changed" {
		t.Fatalf("changed content: %s %s", m, r)
	}

	// A record copied onto another version of its page does not verify there.
	e.sql("ALTER TABLE signatures DISABLE TRIGGER signatures_immutable")
	signedVersion := versionOf(moved)
	w.publishAs(w.author, moved, "Moved", "a later text", false)
	other := signedVersion + 1
	e.sql("UPDATE signatures SET version_number = $2 WHERE page_id = $1", moved, other)
	if m, r := mark(moved, other); m != "does_not_match" || r != "record_differs" {
		t.Fatalf("a moved record: %s %s", m, r)
	}
	// Moving content and hash together still leaves the audit entry saying otherwise.
	e.sql("ALTER TABLE version_hashes DISABLE TRIGGER version_hashes_immutable")
	e.sql(`INSERT INTO version_hashes (page_id, number, content_hash, content_form)
		SELECT page_id, $3, content_hash, content_form FROM version_hashes WHERE page_id = $1 AND number = $2`, moved, signedVersion, other)
	if m, _ := mark(moved, other); m != "does_not_match" {
		t.Fatalf("a moved record with its hash: %s", m)
	}

	e.sql("ALTER TABLE audit_events DISABLE TRIGGER USER")
	e.sql(`UPDATE audit_events SET payload = replace(payload, '"meaning":"approved"', '"meaning":"reviewed"')
		WHERE action = 'page.signed' AND object_id = $1`, entry)
	if m, r := mark(entry, versionOf(entry)); m != "does_not_match" || r != "audit_entry_differs" {
		t.Fatalf("a changed audit entry: %s %s", m, r)
	}
	e.sql(`UPDATE audit_events SET payload = replace(payload, '"meaning":"reviewed"', '"meaning":"approved"'),
		hash = sha256('x'::bytea) WHERE action = 'page.signed' AND object_id = $1`, entry)
	if m, r := mark(entry, versionOf(entry)); m != "does_not_match" || r != "audit_chain_broken" {
		t.Fatalf("a broken chain: %s %s", m, r)
	}
	if res, err := pages.VerifyAllSignatures(context.Background(), e.app.Pool()); err != nil || res.Checked != 3 || len(res.Mismatches) != 3 {
		t.Fatalf("after tampering: %+v %v", res, err)
	}
}

// DOC-1020, DOC-1021, DOC-742: the auditor's export needs space.audit, is audited with its
// scope and period, and the documented procedure, run by its script, verifies it outside
// Kenwerk. A changed export fails the script.
func TestTheAuditorsExportVerifiesOutsideKenwerk(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed here; the development toolbox has it")
	}
	e := newEnv(t)
	w := newSigningWorld(e, "EXPORT")
	first, second := w.signedPage("First", "first text"), w.signedPage("Second", "second text")
	w.signApproval(first)
	w.signApproval(second)
	e.seal()

	if status, _ := w.reader.do("GET", "/api/v1/spaces/"+w.space+"/auditor-export", nil); status != 403 && status != 404 {
		t.Fatalf("a reader made an auditor's export: %d", status)
	}
	status, export := w.admin.do("GET", "/api/v1/spaces/"+w.space+"/auditor-export?since="+url.QueryEscape(time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)), nil)
	if status != 200 {
		t.Fatalf("export: %d %s", status, export)
	}
	if e.auditActions("space.auditor_export") != 1 || e.count(`SELECT count(*) FROM audit_pending WHERE action = 'space.auditor_export' AND payload LIKE '%"scope":"space"%' AND payload LIKE '%"since"%'`) != 1 {
		t.Fatal("the export is not audited with its scope and period")
	}
	for _, kind := range []string{`"type":"export"`, `"type":"page"`, `"type":"version"`, `"type":"round"`, `"type":"answer"`,
		`"type":"signature"`, `"type":"workflow_version"`, `"type":"audit_entry"`, `"type":"end"`} {
		if !strings.Contains(export, kind) {
			t.Errorf("the export lacks %s", kind)
		}
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "export.jsonl")
	if err := os.WriteFile(file, []byte(export), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join("..", "..", "scripts", "verify-signatures.mjs")
	out, err := exec.Command(node, script, file).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "2 signatures verified") {
		t.Fatalf("the script did not verify the export: %v %s", err, out)
	}
	changed := strings.Replace(export, "first text", "first test", 1)
	if changed == export {
		t.Fatal("the export holds no signed text")
	}
	if err := os.WriteFile(file, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(node, script, file).CombinedOutput(); err == nil {
		t.Fatalf("a changed export verified: %s", out)
	}
	// One page alone, and its control history, tell the same.
	if status, one := w.admin.do("GET", "/api/v1/spaces/"+w.space+"/auditor-export?page="+first, nil); status != 200 || strings.Contains(one, second) {
		t.Fatalf("the export of one page: %d", status)
	}
	history := w.admin.ok("GET", "/api/v1/pages/"+first+"/control-history", nil)
	if !strings.Contains(fmt.Sprint(history), "page.signed") || !strings.Contains(fmt.Sprint(history), "valid") {
		t.Fatalf("the control history: %v", history)
	}
	if status, _ := w.author.do("GET", "/api/v1/pages/"+first+"/control-history", nil); status != 403 && status != 404 {
		t.Fatalf("an author without the audit read the control history: %d", status)
	}
}

// DOC-750, DOC-751: the right to sign is given per person, with how they were identified on
// record; taken away, or with the account disabled, the signatures made still verify.
func TestSignaturesOfADisabledSignerStillVerify(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "GONE")
	page := w.signedPage("Procedure", "text")
	w.signApproval(page)
	e.seal()
	if e.count(`SELECT count(*) FROM audit_events WHERE action = 'user.signing_enabled' AND actor_id = $1 AND payload LIKE '%"identified_by":"in_person"%'`, w.root.id) != 1 {
		t.Fatal("the audit does not name who enabled signing and how they identified the person")
	}
	w.root.want(204, "DELETE", "/api/v1/admin/users/"+w.author.id+"/signing", nil)
	w.root.ok("POST", "/api/v1/admin/users/"+w.author.id+"/disable", nil)
	version := e.count("SELECT version_number FROM signatures")
	list := w.reader.ok("GET", fmt.Sprintf("/api/v1/pages/%s/versions/%d/signatures", page, version), nil)["signatures"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["mark"] != "valid" {
		t.Fatalf("a disabled signer's signature: %v", list)
	}
}

// DOC-770: nobody deletes or edits a signature or the version it signs, administrators
// included; a signed page goes to the trash as before, and purging keeps it.
func TestSignedVersionsSurviveDeletePurgeAndRemoval(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "KEEP")
	page := w.signedPage("Procedure", "text")
	w.signApproval(page)
	version := e.count("SELECT version_number FROM signatures")
	w.publishAs(w.author, page, "Procedure", "a newer text", false)
	w.step(page, "submit")
	w.signApproval(page)

	status, out := w.root.do("DELETE", fmt.Sprintf("/api/v1/admin/pages/%s/versions/%d", page, version), map[string]any{"reason": "a leaked secret"})
	wantRefusal(t, status, out, http.StatusConflict, "signed_version")

	if _, err := e.app.Pool().Exec(context.Background(), "DELETE FROM signatures"); err == nil {
		t.Fatal("a signature was deleted in the database")
	}
	if _, err := e.app.Pool().Exec(context.Background(), "UPDATE signatures SET printed_name = 'Someone else'"); err == nil {
		t.Fatal("a signature was changed in the database")
	}
	if _, err := e.app.Pool().Exec(context.Background(), "UPDATE version_hashes SET content_form = 2"); err == nil {
		t.Fatal("the hash of a signed version was changed in the database")
	}

	w.author.want(204, "DELETE", "/api/v1/pages/"+page, nil)
	item := e.scalar("SELECT id FROM trash_items WHERE object_id = $1", page)
	status, out = w.admin.do("DELETE", "/api/v1/trash/"+item, nil)
	wantRefusal(t, status, out, http.StatusConflict, "signed_records")
	e.sql("UPDATE trash_items SET purge_after = now() - interval '1 day'")
	if n, err := e.app.Trash().PurgeExpired(context.Background()); n != 0 || err != nil {
		t.Fatalf("purged %d: %v", n, err)
	}
	if e.count("SELECT count(*) FROM pages WHERE id = $1", page) != 1 || e.count("SELECT count(*) FROM signatures") != 2 {
		t.Fatal("purging the trash took a signed page")
	}
	w.admin.ok("POST", "/api/v1/trash/"+item+"/restore", nil)
	if w.state(page) == "" {
		t.Fatal("the restored page lost its control")
	}
}

// DOC-771, DOC-1417: erasing a person who signed keeps their signature records, with the name
// they signed under; the rest of them goes as usual. Someone who never signed loses all.
func TestErasureKeepsSignaturesAndErasesTheRest(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "ERASE")
	page := w.signedPage("Procedure", "text")
	w.signApproval(page)
	w.root.ok("POST", "/api/v1/admin/users/"+w.author.id+"/anonymise", map[string]any{"reason": "request by mail"})
	if got := e.scalar("SELECT printed_name FROM signatures WHERE signer_id = $1", w.author.id); got != "Test "+w.author.email {
		t.Fatalf("the printed name after erasure: %q", got)
	}
	if e.count("SELECT count(*) FROM signing_rights WHERE user_id = $1", w.author.id) != 0 ||
		e.count("SELECT count(*) FROM totp_credentials WHERE user_id = $1", w.author.id) != 0 {
		t.Fatal("erasure kept the person's own data")
	}
	if got := e.scalar("SELECT display_name FROM users WHERE id = $1", w.author.id); strings.Contains(got, w.author.email) {
		t.Fatalf("the account kept its name: %s", got)
	}
	w.root.ok("POST", "/api/v1/admin/users/"+w.reader.id+"/anonymise", map[string]any{"reason": "request by mail"})
	if e.count("SELECT count(*) FROM signing_rights WHERE user_id = $1", w.reader.id) != 0 ||
		e.count("SELECT count(*) FROM signatures WHERE signer_id = $1", w.reader.id) != 0 {
		t.Fatal("someone who never signed kept something")
	}
}

// DOC-721: a space adds meanings to the four every space has; a workflow names only meanings
// its space has, and a request for changes is never signed.
func TestASpaceAddsMeaningsAndAWorkflowNamesOnlyThose(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "MEAN")
	flow := signingFlow()
	for _, s := range flow["transitions"].([]any) {
		if st := s.(map[string]any); st["id"] == "approve" {
			st["signature_meaning"] = "checked-twice"
		}
	}
	status, out := w.admin.do("POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": "Own meaning", "definition": flow})
	wantRefusal(t, status, out, http.StatusConflict, "workflow_bad_meaning")
	if status, _ := w.author.do("POST", "/api/v1/spaces/"+w.space+"/signature-meanings", map[string]any{"name": "Checked twice", "description": "x"}); status != 403 {
		t.Fatalf("an author added a meaning: %d", status)
	}
	m := w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/signature-meanings", map[string]any{"name": "Checked twice", "description": "You checked it twice."})
	if m["key"] != "checked-twice" {
		t.Fatalf("key: %v", m)
	}
	w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": "Own meaning", "definition": flow})
	list := w.reader.ok("GET", "/api/v1/spaces/"+w.space+"/signature-meanings", nil)["meanings"].([]any)
	if len(list) != 5 {
		t.Fatalf("meanings: %v", list)
	}
	if status, _ := w.admin.do("POST", "/api/v1/spaces/"+w.space+"/signature-meanings", map[string]any{"name": "approved", "description": "again"}); status != 409 {
		t.Fatalf("a meaning took the name of one every space has: %d", status)
	}
}

// PUB-731: the published copy of a signed version shows its signatures to readers of the
// published space, with the printed name, time and meaning, whatever the setting of
// approvers; the records stay in the draft space and nothing of them is copied.
func TestThePublishedCopyOfASignedVersionShowsItsSignatures(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "PUBSIGN", true)
	root := e.user("pubsign-root@example.com", identity.AllGlobalPermissions...)
	secret, _ := w.writer.enrolTOTP()
	root.ok("PUT", "/api/v1/admin/users/"+w.writer.id+"/signing", map[string]any{"identified_by": identity.IdentifiedInPerson})
	flow := markedFlow(true)
	for _, s := range flow["transitions"].([]any) {
		if step := s.(map[string]any); step["id"] == "approve" {
			step["signature_meaning"] = "released"
		}
	}
	signed := w.admin.ok("POST", "/api/v1/spaces/"+w.draft+"/workflows", map[string]any{"name": "Signed release", "definition": flow})["id"].(string)
	page := w.page(w.writer, w.draftHome, "Safety rules", "wear gloves")
	w.writer.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": signed})
	w.step(page, "submit")
	step := map[string]any{"transition": "approve", "version": w.head(page)}
	token := e.confirm(w.writer, secret, page, step)
	w.writer.ok("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": w.head(page), "confirmation": token})
	w.settle()
	twin := w.twin(page)
	approval, _ := w.status(w.reader, twin)["approval"].(map[string]any)
	list, _ := approval["signatures"].([]any)
	if len(list) != 1 {
		t.Fatalf("the published copy shows %d signatures: %v", len(list), approval)
	}
	shown := list[0].(map[string]any)
	if shown["signer_name"] != "Test "+w.writer.email || shown["meaning"] != "released" || shown["signed_at"] == nil || shown["mark"] != "valid" {
		t.Fatalf("the signature on the published copy: %v", shown)
	}
	if approval["approved_by"] != nil {
		t.Fatal("the approver is named although the pair does not show approvers")
	}
	if e.count("SELECT count(*) FROM signatures WHERE page_id = $1", twin) != 0 || e.count("SELECT count(*) FROM signatures") != 1 {
		t.Fatal("a signature record was copied to the published space")
	}
}

// DOC-730, replay: a confirmation that signed once is refused the second time, even for the same
// step of the same version, which the page reaches again without a change of text.
func TestAConfirmationThatSignedOnceIsRefusedForTheSameStepAgain(t *testing.T) {
	e := newEnv(t)
	w := newSigningWorld(e, "REPLAY")
	flow := reviewFlow()
	for _, s := range flow["transitions"].([]any) {
		if step := s.(map[string]any); step["id"] == "submit" {
			step["signature_meaning"] = "reviewed"
		}
	}
	id := w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": "Signed submit", "definition": flow})["id"].(string)
	page := w.author.page(w.space, w.home, "Procedure", "text")
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": id})
	submit := map[string]any{"transition": "submit", "version": w.head(page)}
	token := e.confirm(w.author, w.secret, page, submit)
	w.author.ok("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "submit", "version": w.head(page), "confirmation": token})
	w.author.ok("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "reject", "version": w.head(page), "comment": "not yet"})
	w.step(page, "rework")
	status, out := w.author.do("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "submit", "version": w.head(page), "confirmation": token})
	wantRefusal(t, status, out, http.StatusForbidden, "signing_confirmation_invalid")
	if n := e.count("SELECT count(*) FROM signatures"); n != 1 {
		t.Fatalf("%d signatures, want 1", n)
	}
}
