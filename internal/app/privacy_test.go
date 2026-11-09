// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/mail"
	"github.com/diffluence/diffluence/internal/permissions"
)

// withQueuedMail makes account mail go through the real queue, into a fake sender.
func withQueuedMail(e *env) (*fakeSender, *mail.Service) {
	f := &fakeSender{}
	e.app.UseMailForTest()
	s := mail.NewService(e.app.Pool(), f, e.app.Logger())
	s.SetRenderer(e.app.RenderMailForTest)
	return f, s
}

func linkIn(t *testing.T, text string) string {
	t.Helper()
	at := strings.Index(text, "http")
	if at < 0 {
		t.Fatalf("no link in %q", text)
	}
	return strings.Fields(text[at:])[0]
}

// Wave C's queue carries the links of wave F. A link holds a secret, so the queue
// keeps it sealed, and nothing of it stays once the mail is gone.
func TestAResetMailGoesThroughTheQueueWithItsLinkSealed(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	f, queue := withQueuedMail(e)
	ctx := context.Background()

	if resp, _ := e.do(e.client(), "POST", "/api/v1/auth/forgot", `{"email":"anna@example.com"}`); resp.StatusCode != 202 {
		t.Fatalf("forgot: %d", resp.StatusCode)
	}
	var sealed []byte
	var kind string
	if err := e.app.Pool().QueryRow(ctx, "SELECT kind, sealed FROM mail_queue").Scan(&kind, &sealed); err != nil {
		t.Fatal(err)
	}
	if kind != mail.KindAccount || bytes.Contains(sealed, []byte("token=")) || bytes.Contains(sealed, []byte(anna.email)) {
		t.Fatalf("%s: the queue can read what it carries", kind)
	}
	if n, err := queue.RunOnce(ctx); n != 1 || err != nil {
		t.Fatalf("sent %d: %v", n, err)
	}
	if len(f.sent) != 1 || f.sent[0].To != anna.email {
		t.Fatalf("sent: %+v", f.sent)
	}
	link := linkIn(t, f.sent[0].Text)
	token := link[strings.Index(link, "token=")+len("token="):]
	body, _ := json.Marshal(map[string]string{"token": token, "password": "a fine new passphrase for me"})
	if resp, out := e.do(e.client(), "POST", "/api/v1/auth/reset", string(body)); resp.StatusCode != 204 {
		t.Fatalf("reset: %d %s", resp.StatusCode, out)
	}
	// The reset itself is a change to the sign-in, and the person hears of it (SEC-127).
	if n, err := queue.RunOnce(ctx); n != 1 || err != nil {
		t.Fatalf("sent %d after the reset: %v", n, err)
	}
	var left int
	if err := e.app.Pool().QueryRow(ctx, "SELECT count(*) FROM mail_queue WHERE sealed IS NOT NULL").Scan(&left); err != nil || left != 0 {
		t.Fatalf("%d sealed mails stayed behind: %v", left, err)
	}
}

// SEC-127: a person hears of every change to a way into their account (ASVS 6.3.7), also
// when an administrator makes it.
func TestAPersonIsToldOfEveryChangeToTheirSignIn(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	root := admin(e)
	f, queue := withQueuedMail(e)
	ctx := context.Background()
	told := func(what string) {
		t.Helper()
		before := len(f.sent)
		if _, err := queue.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
		if len(f.sent) != before+1 || f.sent[len(f.sent)-1].To != anna.email || !strings.Contains(f.sent[len(f.sent)-1].Subject, "sign-in changed") {
			t.Fatalf("%s: the person was not told: %+v", what, f.sent[before:])
		}
	}
	anna.enrolTOTP()
	told("a second factor added")
	anna.ok("POST", "/api/v1/me/totp/disable", map[string]any{"password": adminPassword})
	told("a second factor removed")
	anna.ok("POST", "/api/v1/me/password", map[string]any{"current_password": adminPassword, "new_password": "another fine passphrase here"})
	told("a new password")
	root.ok("POST", "/api/v1/admin/users/"+anna.id+"/reset-password", map[string]any{"password": "a passphrase the admin chose"})
	told("a password set by an administrator")
}

func TestAnInvitationGoesThroughTheQueueToSomeoneWithoutAnAccount(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	f, queue := withQueuedMail(e)
	made := root.ok("POST", "/api/v1/admin/invitations", map[string]any{"email": "new@example.com", "type": "member", "groups": []string{}})
	if made["link"] != "" {
		t.Fatal("with mail the link goes by mail, and does not show")
	}
	if n, err := queue.RunOnce(context.Background()); n != 1 || err != nil {
		t.Fatalf("sent %d: %v", n, err)
	}
	if len(f.sent) != 1 || f.sent[0].To != "new@example.com" || !strings.Contains(f.sent[0].Text, "/invitation?token=") {
		t.Fatalf("sent: %+v", f.sent)
	}
}

func TestMailWaitsDuringMaintenance(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	e.app.UseMailForTest()
	root.ok("POST", "/api/v1/admin/invitations", map[string]any{"email": "new@example.com", "type": "member", "groups": []string{}})
	root.ok("PUT", "/api/v1/admin/maintenance", map[string]any{"on": true, "message": ""})
	f := &fakeSender{}
	queue := mail.NewService(e.app.Pool(), f, e.app.Logger())
	queue.SetRenderer(e.app.RenderMailForTest)
	queue.HoldWhile(e.app.InMaintenanceForTest)
	if n, err := queue.RunOnce(context.Background()); n != 0 || err != nil {
		t.Fatalf("sent %d during maintenance: %v", n, err)
	}
}

// idColumns says why an id may stay after anonymising (PRM-176). A column that still
// holds the id fails the walk until someone decides here.
var idColumns = map[string]string{
	"users.id":                        "the account stays, under a neutral name",
	"pages.created_by":                "authorship resolves to the neutral name",
	"pages.updated_by":                "authorship resolves to the neutral name",
	"page_versions.author_id":         "authorship resolves to the neutral name",
	"spaces.created_by":               "authorship resolves to the neutral name",
	"spaces.owner_id":                 "the space stays until it is handed over",
	"attachments.created_by":          "authorship resolves to the neutral name",
	"attachment_versions.uploaded_by": "authorship resolves to the neutral name",
	"comments.author_id":              "authorship resolves to the neutral name",
	"comments.resolved_by":            "authorship resolves to the neutral name",
	"comments.edited_by":              "authorship resolves to the neutral name",
	"reactions.user_id":               "authorship resolves to the neutral name",
	"page_mentions.user_id":           "a mention resolves to the neutral name",
	"page_tasks.assignee_id":          "a task stays with the neutral name",
	"page_tasks.creator_id":           "authorship resolves to the neutral name",
	"trash_items.deleted_by":          "authorship resolves to the neutral name",
	"jobs.created_by":                 "authorship resolves to the neutral name",
	"scim_tokens.created_by":          "authorship resolves to the neutral name",
	"space_grants.subject_id":         "the rule stays: taking it away could open a page",
	"global_grants.subject_id":        "the rule stays, and the account cannot log in",
	"role_assignments.subject_id":     "the roles stay with the grants they add up to, for the same reason",
	"page_restrictions.subject_id":    "the rule stays: an empty restriction would open a page",
	"access_requests.asked_id":        "someone else's request keeps who it went to",
	"access_requests.answered_by":     "someone else's request keeps who answered",
	"invitation_requests.decided_by":  "someone else's request keeps who answered",
}

func uuidColumns(t *testing.T, e *env) []string {
	t.Helper()
	rows, err := e.app.Pool().Query(context.Background(), `SELECT table_name || '.' || column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND udt_name IN ('uuid', '_uuid') AND table_name NOT IN ('audit_events', 'schema_migrations')`)
	if err != nil {
		t.Fatal(err)
	}
	columns, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return columns
}

// GAP-018: what wave C keeps about a person goes as well, and ids count too.
func TestAnonymisingLeavesTheIdOnlyWhereThatIsDecided(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	anna := e.user("anna@example.com", identity.PermUseProduct, identity.PermViewProfiles, identity.PermPersonalSpace)
	bob := e.user("bob@example.com", identity.PermUseProduct, identity.PermViewProfiles)
	space, _ := root.space("WORK")
	root.grant(space, "user", anna.id, permissions.TemplateEditor)
	root.grant(space, "user", bob.id, permissions.TemplateEditor)
	page := root.page(space, nil, "Shared", "text")
	anna.ok("PUT", "/api/v1/pages/"+page+"/favourite", nil)
	anna.ok("PUT", "/api/v1/spaces/"+space+"/favourite", nil)
	anna.ok("PUT", "/api/v1/pages/"+page+"/watch", nil)
	anna.ok("PUT", "/api/v1/pages/"+page+"/comments/seen", nil)
	anna.ok("PUT", "/api/v1/users/"+bob.id+"/follow", nil)
	bob.ok("PUT", "/api/v1/users/"+anna.id+"/follow", nil)
	anna.ok("GET", "/api/v1/pages/"+page, nil)
	e.sql(`INSERT INTO notifications (id, user_id, actor_id, type, reason, group_key, source_event) VALUES
		(gen_random_uuid(), $1, $2, 'mention', 'mention', 'a', gen_random_uuid()),
		(gen_random_uuid(), $2, $1, 'mention', 'mention', 'b', gen_random_uuid())`, anna.id, bob.id)
	e.sql("INSERT INTO mail_queue (id, user_id, kind) VALUES (gen_random_uuid(), $1, 'digest')", anna.id)
	e.sql("INSERT INTO recent_pages (user_id, page_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", anna.id, page)

	root.ok("POST", "/api/v1/admin/users/"+anna.id+"/anonymise", map[string]any{"reason": "Request by mail"})

	ctx := context.Background()
	walked := 0
	for _, column := range uuidColumns(t, e) {
		table, name, _ := strings.Cut(column, ".")
		var n int
		query := fmt.Sprintf("SELECT count(*) FROM %s WHERE %s::text LIKE '%%' || $1 || '%%'",
			pgx.Identifier{table}.Sanitize(), pgx.Identifier{name}.Sanitize())
		if err := e.app.Pool().QueryRow(ctx, query, anna.id).Scan(&n); err != nil {
			t.Fatalf("%s: %v", column, err)
		}
		walked++
		if _, decided := idColumns[column]; n > 0 && !decided {
			t.Errorf("the id is still in %s, in %d rows", column, n)
		}
	}
	if walked < 40 {
		t.Fatalf("only %d columns walked", walked)
	}
}

// ADR-029: the id, the words "(Censored title)", and never the title.
func TestAnAdministratorSeesACensoredTitleOfAPageTheyCannotRead(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("SECRET")
	page := owner.page(space, nil, "Merger with Initech", "text")
	open, _ := root.space("OPEN")
	mine := root.page(open, nil, "A title I can read", "text")
	owner.want(204, "DELETE", "/api/v1/pages/"+page, nil)

	list := root.want(200, "GET", "/api/v1/admin/audit-events?limit=200", nil)
	if strings.Contains(list, "Initech") {
		t.Fatalf("the screen shows a title: %s", list)
	}
	if !strings.Contains(list, page) || !strings.Contains(list, audit.CensoredTitle) {
		t.Fatalf("the id and the words must show: %s", list)
	}
	if !strings.Contains(list, "A title I can read") {
		t.Fatal("a page the administrator can read keeps its title")
	}
	export := root.want(200, "GET", "/api/v1/admin/audit-events/export", nil)
	if strings.Contains(export, "Initech") || !strings.Contains(export, audit.CensoredTitle) {
		t.Fatalf("the export shows a title: %s", export)
	}
	byID := root.want(200, "GET", "/api/v1/admin/audit-events?object_id="+page, nil)
	if !strings.Contains(byID, page) || strings.Contains(byID, mine) {
		t.Fatalf("events by id: %s", byID)
	}
	note := root.ok("GET", "/api/v1/admin/pages/"+page, nil)
	if note["title"] != audit.CensoredTitle || note["space_name"] == "" {
		t.Fatalf("the note: %v", note)
	}
	if e.auditActions("page.looked_up") != 1 {
		t.Fatal("looking up a page must be audited")
	}
	if own := root.ok("GET", "/api/v1/admin/pages/"+mine, nil); own["title"] != "A title I can read" {
		t.Fatalf("own page: %v", own)
	}
	owner.want(403, "GET", "/api/v1/admin/pages/"+page, nil)
}

// SEC-164: a new instance gives its administrators time, and then asks.
func TestAdministratorsNeedASecondFactorAfterTheGracePeriod(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	member := e.user("anna@example.com", identity.PermUseProduct)
	if me := root.ok("GET", "/api/v1/me", nil); me["needs_second_factor"] != false {
		t.Fatal("an instance of today is in its grace period")
	}
	e.sql("UPDATE instance SET created_at = now() - interval '15 days'")

	c := e.client()
	if got := e.login(c, root.email, adminPassword); got != 200 {
		t.Fatalf("login: %d", got)
	}
	again := &user{e: e, c: c, email: root.email, id: root.id}
	if me := again.ok("GET", "/api/v1/me", nil); me["needs_second_factor"] != true {
		t.Fatalf("an administrator past the grace period: %v", me["needs_second_factor"])
	}
	again.want(403, "GET", "/api/v1/admin/summary", nil)

	c = e.client()
	if got := e.login(c, member.email, adminPassword); got != 200 {
		t.Fatalf("login: %d", got)
	}
	plain := &user{e: e, c: c, email: member.email, id: member.id}
	if me := plain.ok("GET", "/api/v1/me", nil); me["needs_second_factor"] != false {
		t.Fatal("the rule is for administrators")
	}
}

func TestAnOperatorCanChooseAnotherRuleForASecondFactor(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	e.sql("UPDATE instance SET created_at = now() - interval '15 days'")
	access := root.ok("GET", "/api/v1/admin/settings/access", nil)
	access["second_factor"] = "none"
	root.ok("PUT", "/api/v1/admin/settings/access", access)
	c := e.client()
	if got := e.login(c, root.email, adminPassword); got != 200 {
		t.Fatalf("login: %d", got)
	}
	again := &user{e: e, c: c, email: root.email, id: root.id}
	if me := again.ok("GET", "/api/v1/me", nil); me["needs_second_factor"] != false {
		t.Fatal("the choice of an operator counts")
	}
}

// SEC-821: what identifies a person sits apart, and the chain only holds its digest.
func TestPersonalDataInTheAuditLogCanBeErasedWhileTheChainHolds(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	ctx := context.Background()
	anna := e.user("annika.visserius@example.com", identity.PermUseProduct)
	root.ok("PATCH", "/api/v1/admin/users/"+anna.id, map[string]any{"display_name": "Annika Visserius", "type": "member"})

	var inChain int
	if err := e.app.Pool().QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE payload ILIKE '%visserius%'").Scan(&inChain); err != nil {
		t.Fatal(err)
	}
	if inChain != 0 {
		t.Fatalf("%d events hold the name inside the chain", inChain)
	}
	shown := root.want(200, "GET", "/api/v1/admin/audit-events?object_id="+anna.id, nil)
	if !strings.Contains(strings.ToLower(shown), "visserius") {
		t.Fatalf("an administrator still reads what happened: %s", shown)
	}

	root.ok("POST", "/api/v1/admin/users/"+anna.id+"/anonymise", map[string]any{"reason": "Request by mail"})
	var left int
	if err := e.app.Pool().QueryRow(ctx, "SELECT count(*) FROM audit_personal WHERE data ILIKE '%visserius%'").Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d personal parts stayed", left)
	}
	if res, err := e.app.CheckAudit(ctx); err != nil || !res.OK {
		t.Fatalf("the chain must hold after erasing: %+v %v", res, err)
	}
	if out := root.want(200, "GET", "/api/v1/admin/audit-events?object_id="+anna.id, nil); strings.Contains(strings.ToLower(out), "visserius") {
		t.Fatalf("the name still shows: %s", out)
	}
}

func TestChangingAPersonalPartOfTheAuditLogIsNoticed(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	ctx := context.Background()
	e.user("anna@example.com", identity.PermUseProduct)
	root.ok("GET", "/api/v1/me", nil)
	e.sql("UPDATE audit_personal SET data = replace(data, 'anna', 'somebody')")
	res, err := audit.Verify(ctx, e.app.Pool())
	if err != nil || res.OK {
		t.Fatalf("a changed part passed: %+v %v", res, err)
	}
}

// SEC-831: 12 months, and longer only with a stated reason.
func TestTheAuditLogStaysAYearByDefault(t *testing.T) {
	e := newEnv(t)
	root := admin(e)
	retention := root.ok("GET", "/api/v1/admin/settings/retention", nil)
	if retention["audit_days"] != float64(365) {
		t.Fatalf("default: %v", retention)
	}
	root.want(400, "PUT", "/api/v1/admin/settings/retention", map[string]any{"audit_days": 800})
	root.want(400, "PUT", "/api/v1/admin/settings/retention", map[string]any{"audit_days": 0})
	root.ok("PUT", "/api/v1/admin/settings/retention", map[string]any{"audit_days": 800, "longer_because": "A contract asks for it"})
	root.ok("PUT", "/api/v1/admin/settings/retention", map[string]any{"audit_days": 90})
}

func TestAnErrorFromTheRendererOfAccountMailReachesNobody(t *testing.T) {
	e := newEnv(t)
	e.app.UseMailForTest()
	_, err := e.app.RenderMailForTest(context.Background(), mail.Job{Kind: mail.KindAccount, Sealed: []byte("not sealed")})
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("a broken mail must fail: %v", err)
	}
}
