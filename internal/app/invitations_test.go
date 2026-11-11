// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

const outsider = "outsider@example.org"

var goodReason = map[string]any{"reason": "We work with them"}

// inviteSetup is a page with someone who may ask for an invitation, and an ordinary
// member who may not. Both read the page.
type inviteSetup struct {
	root, inviter, member *user
	space, page           string
}

func newInviteSetup(e *env) inviteSetup {
	e.t.Helper()
	root := admin(e)
	inviter := e.user("inviter@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermInvitePeople)
	member := e.user("member@example.com", identity.PermUseProduct)
	space, _ := inviter.space("ENG")
	inviter.grant(space, "user", member.id, permissions.TemplateViewer)
	return inviteSetup{root: root, inviter: inviter, member: member, space: space, page: inviter.page(space, nil, "Merger with Initech", "Secret plans")}
}

func (s inviteSetup) ask(email string) string {
	s.inviter.e.t.Helper()
	return s.inviter.ok("POST", "/api/v1/pages/"+s.page+"/invitation-requests", map[string]any{"email": email, "note": "Our auditor, please"})["id"].(string)
}

func (e *env) openInvitations() int {
	e.t.Helper()
	return e.count("SELECT count(*) FROM auth_challenges WHERE kind = 'invitation'")
}

// TestInvitationMatrix is who may do what, line by line (ADR-033). A line is a person
// and an action, in the order that leaves each line its own request to act on.
func TestInvitationMatrix(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	grouped := e.user("grouped@example.com", identity.PermUseProduct)
	s.inviter.grant(s.space, "user", grouped.id, permissions.TemplateViewer)
	role := s.root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Inviters"})["id"].(string)
	s.root.ok("PUT", "/api/v1/admin/groups/"+role+"/members/"+grouped.id, nil)
	s.root.ok("PUT", "/api/v1/admin/groups/"+role+"/permissions", map[string]any{"permissions": []string{identity.PermInvitePeople}})
	stranger := e.user("stranger@example.com", identity.PermUseProduct, identity.PermInvitePeople)

	request := s.ask("first@example.org")
	ask := "/api/v1/pages/" + s.page + "/invitation-requests"
	lines := []struct {
		name   string
		who    *user
		method string
		path   string
		body   any
		want   int
	}{
		{"a member without the permission cannot ask", s.member, "POST", ask, map[string]any{"email": outsider}, 403},
		{"an administrator without the page cannot ask", s.root, "POST", ask, map[string]any{"email": outsider}, 404},
		{"someone with the permission cannot ask about a page they cannot see", stranger, "POST", ask, map[string]any{"email": outsider}, 404},
		{"a role given through a group can ask", grouped, "POST", ask, map[string]any{"email": "second@example.org"}, 202},
		{"a member cannot list requests", s.member, "GET", "/api/v1/invitation-requests", nil, 403},
		{"the one who asked cannot list requests", s.inviter, "GET", "/api/v1/invitation-requests", nil, 403},
		{"a member cannot approve", s.member, "POST", "/api/v1/invitation-requests/" + request + "/approve", goodReason, 403},
		{"the one who asked cannot approve", s.inviter, "POST", "/api/v1/invitation-requests/" + request + "/approve", goodReason, 403},
		{"the one who asked cannot refuse", s.inviter, "POST", "/api/v1/invitation-requests/" + request + "/refuse", goodReason, 403},
		{"an administrator can list requests", s.root, "GET", "/api/v1/invitation-requests", nil, 200},
		{"an administrator can approve", s.root, "POST", "/api/v1/invitation-requests/" + request + "/approve", goodReason, 200},
	}
	for _, line := range lines {
		if got, out := line.who.do(line.method, line.path, line.body); got != line.want {
			t.Errorf("%s: got %d, want %d: %s", line.name, got, line.want, out)
		}
	}
	if got := e.count("SELECT count(*) FROM audit_entries WHERE action = 'access.denied' AND payload LIKE '%' || $1 || '%'", identity.PermInvitePeople); got != 1 {
		t.Errorf("%d refusals of the permission in the audit log", got)
	}
	if got := e.count("SELECT count(*) FROM audit_entries WHERE action = 'access.denied' AND payload LIKE '%' || $1 || '%'", identity.PermAdministrator); got != 5 {
		t.Errorf("%d refusals of the administrator in the audit log", got)
	}
}

func TestAskingForAnInvitationSendsNothing(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	f, queue := withQueuedMail(e)
	s.ask(outsider)
	if n, err := queue.RunOnce(context.Background()); n != 0 || err != nil {
		t.Fatalf("sent %d: %v", n, err)
	}
	if len(f.sent) != 0 || e.openInvitations() != 0 {
		t.Fatalf("mail %d, invitations %d", len(f.sent), e.openInvitations())
	}
	if e.auditActions("invitation.requested") != 1 || e.outboxCount("invitation.requested") != 1 {
		t.Fatal("asking leaves a line in the audit log and an event")
	}
}

func TestTheSameAddressCannotWaitTwiceForOneSpace(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	s.ask(outsider)
	s.inviter.want(409, "POST", "/api/v1/pages/"+s.page+"/invitation-requests", map[string]any{"email": "OUTSIDER@example.org"})
}

func TestARequestNeedsAnAddress(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	s.inviter.want(400, "POST", "/api/v1/pages/"+s.page+"/invitation-requests", map[string]any{"email": "not an address"})
	s.inviter.want(400, "POST", "/api/v1/pages/"+s.page+"/invitation-requests", map[string]any{"email": outsider, "note": strings.Repeat("x", 501)})
}

func TestRequestsWaitOnlyWhileInvitationsAreOn(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	request := s.ask(outsider)
	features := s.root.ok("GET", "/api/v1/admin/settings/features", nil)
	features["invitations"] = false
	s.root.ok("PUT", "/api/v1/admin/settings/features", features)
	s.inviter.want(409, "POST", "/api/v1/pages/"+s.page+"/invitation-requests", map[string]any{"email": "other@example.org"})
	s.root.want(409, "POST", "/api/v1/invitation-requests/"+request+"/approve", goodReason)
	if e.openInvitations() != 0 {
		t.Fatal("an invitation went out while they are off")
	}
}

func TestASpaceManagerOnlyAsksForAGuest(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	f, queue := withQueuedMail(e)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, home := owner.space("ENG")
	guests := "/api/v1/spaces/" + space + "/guests"

	owner.want(403, "POST", guests, map[string]any{"email": outsider})
	root.ok("PUT", "/api/v1/admin/users/"+owner.id+"/permissions", map[string]any{"permissions": []string{
		identity.PermUseProduct, identity.PermCreateSpace, identity.PermInvitePeople}})
	owner.want(202, "POST", guests, map[string]any{"email": outsider})
	if n, err := queue.RunOnce(context.Background()); n != 0 || err != nil || e.openInvitations() != 0 {
		t.Fatalf("sent %d, invitations %d: %v", n, e.openInvitations(), err)
	}
	if got := e.count("SELECT count(*) FROM invitation_requests WHERE page_id = $1 AND space_id = $2 AND status = 'waiting'", home, space); got != 1 {
		t.Fatalf("%d requests wait", got)
	}
	if len(f.sent) != 0 {
		t.Fatalf("sent: %+v", f.sent)
	}
}

func TestAnAdministratorInvitesAGuestDirectly(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	space, _ := root.space("OPS")
	root.want(201, "POST", "/api/v1/spaces/"+space+"/guests", map[string]any{"email": outsider})
	if e.openInvitations() != 1 {
		t.Fatal("an administrator is the one who approves")
	}
}

func TestAnAnswerNeedsAReason(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	request := s.ask(outsider)
	for _, answer := range []string{"approve", "refuse"} {
		path := "/api/v1/invitation-requests/" + request + "/" + answer
		s.root.want(400, "POST", path, map[string]any{"reason": "   "})
		s.root.want(400, "POST", path, map[string]any{"reason": strings.Repeat("x", 501)})
	}
	if e.openInvitations() != 0 || e.count("SELECT count(*) FROM invitation_requests WHERE status = 'waiting'") != 1 {
		t.Fatal("an answer without a reason changed something")
	}
}

func TestAnApprovedInvitationGoesThroughTheQueueWithoutTheContent(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	f, queue := withQueuedMail(e)
	other, _ := s.inviter.space("OTHER")
	hidden := s.inviter.page(s.space, nil, "Salaries", "Secret numbers")
	s.inviter.restrict(hidden, "view", s.inviter.id)

	request := s.ask(outsider)
	answer := s.root.ok("POST", "/api/v1/invitation-requests/"+request+"/approve", goodReason)
	if answer["link"] != "" {
		t.Fatal("with mail the link goes by mail, and does not show")
	}
	if n, err := queue.RunOnce(context.Background()); n != 1 || err != nil {
		t.Fatalf("sent %d: %v", n, err)
	}
	if len(f.sent) != 1 || f.sent[0].To != outsider {
		t.Fatalf("sent: %+v", f.sent)
	}
	for _, secret := range []string{"Merger", "Secret plans", "Our auditor", "We work with them", s.page} {
		if strings.Contains(f.sent[0].Text+f.sent[0].HTML+f.sent[0].Subject, secret) {
			t.Fatalf("the invitation holds %q: %+v", secret, f.sent[0])
		}
	}
	if e.auditActions("invitation.approved") != 1 || e.auditActions("invitation.created") != 1 || e.outboxCount("invitation.approved") != 1 {
		t.Fatal("an approval leaves lines in the audit log and an event")
	}

	link, err := url.Parse(linkIn(t, f.sent[0].Text))
	if err != nil {
		t.Fatal(err)
	}
	accept := map[string]any{"token": link.Query().Get("token"), "display_name": "Outsider", "password": "a long enough passphrase"}
	if status, out := publicPost(e, "/api/v1/auth/invitation", accept); status != 204 {
		t.Fatalf("accept: %d %s", status, out)
	}
	c := e.client()
	if got := e.login(c, outsider, "a long enough passphrase"); got != 200 {
		t.Fatalf("login: %d", got)
	}
	guest := &user{e: e, c: c, email: outsider}
	guest.want(200, "GET", "/api/v1/pages/"+s.page, nil)
	guest.want(404, "GET", "/api/v1/pages/"+hidden, nil)
	guest.want(404, "GET", "/api/v1/spaces/"+other, nil)
	if got := e.count("SELECT count(*) FROM users WHERE email = $1 AND type = 'guest'", outsider); got != 1 {
		t.Fatal("the recipient is a guest")
	}
}

func TestWithoutMailTheAdministratorGetsTheLink(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	answer := s.root.ok("POST", "/api/v1/invitation-requests/"+s.ask(outsider)+"/approve", goodReason)
	if !strings.Contains(answer["link"].(string), "/invitation?token=") {
		t.Fatalf("answer: %v", answer)
	}
}

func TestARefusalSendsNothingAndStays(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	f, queue := withQueuedMail(e)
	request := s.ask(outsider)
	s.root.ok("POST", "/api/v1/invitation-requests/"+request+"/refuse", goodReason)
	s.root.want(409, "POST", "/api/v1/invitation-requests/"+request+"/approve", goodReason)
	s.root.want(409, "POST", "/api/v1/invitation-requests/"+request+"/refuse", goodReason)
	if n, err := queue.RunOnce(context.Background()); n != 0 || err != nil || len(f.sent) != 0 || e.openInvitations() != 0 {
		t.Fatalf("sent %d, invitations %d: %v", n, e.openInvitations(), err)
	}
	if e.auditActions("invitation.refused") != 1 || e.outboxCount("invitation.refused") != 1 {
		t.Fatal("a refusal leaves a line in the audit log and an event")
	}
	if list := s.root.ok("GET", "/api/v1/invitation-requests", nil)["requests"].([]any); len(list) != 0 {
		t.Fatalf("still waiting: %v", list)
	}
}

func TestAnApprovalCountsOnce(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	request := s.ask(outsider)
	s.root.ok("POST", "/api/v1/invitation-requests/"+request+"/approve", goodReason)
	s.root.want(409, "POST", "/api/v1/invitation-requests/"+request+"/approve", goodReason)
	if e.auditActions("invitation.created") != 1 {
		t.Fatal("a second approval made a second invitation")
	}
}

func TestAnUnknownRequestIsNotFound(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	root.want(404, "POST", "/api/v1/invitation-requests/00000000-0000-0000-0000-000000000000/approve", goodReason)
}

func TestApprovalFailsWhenTheRequesterLostTheRole(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	request := s.ask(outsider)
	s.root.ok("PUT", "/api/v1/admin/users/"+s.inviter.id+"/permissions", map[string]any{"permissions": []string{identity.PermUseProduct}})
	s.root.want(409, "POST", "/api/v1/invitation-requests/"+request+"/approve", goodReason)
	if e.openInvitations() != 0 {
		t.Fatal("an invitation went out for someone who lost the role")
	}
	s.root.ok("POST", "/api/v1/invitation-requests/"+request+"/refuse", goodReason)
}

// D8: asking does not tell a requester who has an account.
func TestAnAddressWithAnAccountFailsOnlyAtApproval(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	request := s.ask("member@example.com")
	s.root.want(409, "POST", "/api/v1/invitation-requests/"+request+"/approve", goodReason)
	if got := e.count("SELECT count(*) FROM invitation_requests WHERE status = 'waiting'"); got != 1 {
		t.Fatal("a failed approval must leave the request waiting")
	}
}

func TestTheRequesterHearsTheAnswerAndTheReason(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	s.root.ok("POST", "/api/v1/invitation-requests/"+s.ask("one@example.org")+"/approve", map[string]any{"reason": "Known to us"})
	s.root.ok("POST", "/api/v1/invitation-requests/"+s.ask("two@example.org")+"/refuse", map[string]any{"reason": "Not this one"})
	list := s.inviter.ok("GET", "/api/v1/me/notifications?type=invitation_answer", nil)["notifications"].([]any)
	if len(list) != 2 {
		t.Fatalf("each answer has its own line: %v", list)
	}
	answers := map[string]map[string]any{}
	for _, item := range list {
		data := item.(map[string]any)["data"].(map[string]any)
		answers[data["email"].(string)] = data
	}
	if got := answers["one@example.org"]; got["answer"] != "approved" || got["reason"] != "Known to us" {
		t.Fatalf("approved: %v", got)
	}
	if got := answers["two@example.org"]; got["answer"] != "refused" || got["reason"] != "Not this one" {
		t.Fatalf("refused: %v", got)
	}
}

func TestTheConsoleShowsTheSpaceAndNeverThePage(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	s.ask(outsider)
	out := s.root.want(200, "GET", "/api/v1/invitation-requests", nil)
	if strings.Contains(out, "Merger") {
		t.Fatalf("an administrator holds no content: %s", out)
	}
	for _, want := range []string{outsider, "Space ENG", "Our auditor, please", "Test inviter@example.com"} {
		if !strings.Contains(out, want) {
			t.Fatalf("%q is missing: %s", want, out)
		}
	}
	if !strings.Contains(s.root.want(200, "GET", "/api/v1/admin/attention", nil), "invitations_waiting") {
		t.Fatal("the summary says that requests wait")
	}
}

func TestTheAddressOfAnOutsiderIsNotKept(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	s.root.ok("POST", "/api/v1/invitation-requests/"+s.ask("one@example.org")+"/approve", goodReason)
	s.root.ok("POST", "/api/v1/invitation-requests/"+s.ask("two@example.org")+"/refuse", goodReason)
	if got := e.count("SELECT count(*) FROM invitation_requests WHERE email <> '' OR note <> ''"); got != 0 {
		t.Fatalf("%d answered requests keep the address or the note", got)
	}
	if got := e.count("SELECT count(*) FROM audit_entries WHERE payload LIKE '%example.org%'"); got != 0 {
		t.Fatalf("%d audit lines hold an address", got)
	}
}

func TestAnonymisingARequesterRemovesTheirRequests(t *testing.T) {
	e := newEnv(t)
	s := newInviteSetup(e)
	s.ask(outsider)
	s.root.ok("POST", "/api/v1/admin/users/"+s.inviter.id+"/anonymise", map[string]any{"reason": "Request by mail"})
	if got := e.count("SELECT count(*) FROM invitation_requests"); got != 0 {
		t.Fatalf("%d requests stayed", got)
	}
}
