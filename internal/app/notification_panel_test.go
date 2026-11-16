// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// listed asks for the notifications of one query, after the worker has run.
func (u *user) listed(query string) []map[string]any {
	u.e.t.Helper()
	u.e.deliver()
	out := []map[string]any{}
	for _, n := range u.ok("GET", "/api/v1/me/notifications?"+query, nil)["notifications"].([]any) {
		out = append(out, n.(map[string]any))
	}
	return out
}

func typesOf(items []map[string]any) []string {
	out := []string{}
	for _, n := range items {
		out = append(out, n["type"].(string))
	}
	return out
}

func TestAnUnknownTabOfNotificationsIsRefused(t *testing.T) {
	e := newEnv(t)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	reader.want(400, "GET", "/api/v1/me/notifications?tab=everyone", nil)
	reader.want(400, "POST", "/api/v1/me/notifications/read-all?tab=everyone", nil)
}

func TestTheTabsSplitWhatIsAimedAtYouFromWhatYouWatch(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")
	owner.publish(page, "Page", 1, para("first"))
	reader.ok("PUT", "/api/v1/pages/"+page+"/watch?scope=page", nil)
	owner.publish(page, "Page", 2, para("second"))
	owner.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("look ", mention(reader.id, "R"))})

	forMe := typesOf(reader.listed("tab=for_me"))
	watched := typesOf(reader.listed("tab=watched"))
	if len(forMe) != 1 || forMe[0] != "mention" {
		t.Fatalf("for me holds what is aimed at the person: %v", forMe)
	}
	for _, kind := range watched {
		if kind == "mention" {
			t.Fatalf("a mention is never only watched: %v", watched)
		}
	}
	if len(watched) == 0 {
		t.Fatal("the change to a watched page belongs to Watched")
	}
	if all := reader.listed(""); len(all) != len(forMe)+len(watched) {
		t.Fatalf("without a tab every notification shows: %v", typesOf(all))
	}

	// A comment on the person's own page is aimed at them, though they also watch it (COL-115).
	reader.comment(page, "a question for the owner")
	ownerForMe := typesOf(owner.listed("tab=for_me"))
	if !slices.Contains(ownerForMe, "comment") || slices.Contains(typesOf(owner.listed("tab=watched")), "comment") {
		t.Fatalf("a comment on your own page belongs to For me: %v", ownerForMe)
	}

	reader.ok("POST", "/api/v1/me/notifications/read-all?tab=watched", nil)
	if got := reader.listed("tab=watched&unread=true"); len(got) != 0 {
		t.Fatalf("mark all as read on Watched: %v", typesOf(got))
	}
	if got := reader.listed("tab=for_me&unread=true"); len(got) != 1 {
		t.Fatalf("mark all as read on Watched leaves For me unread: %v", typesOf(got))
	}
	reader.ok("POST", "/api/v1/me/notifications/read-all", nil)
	if reader.unread() != 0 {
		t.Fatal("without a tab, mark all as read reads everything")
	}
}

func TestThePanelShowsTheStartOfACommentOnlyToWhoMayReadIt(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")
	owner.publish(page, "Page", 1, para("first"))
	comment := owner.ok("POST", "/api/v1/pages/"+page+"/comments",
		map[string]any{"body": commentBody("please read ", mention(reader.id, "R"))})["id"].(string)

	plain := reader.listed("")
	if len(plain) != 1 || plain[0]["excerpt"] != nil {
		t.Fatalf("an excerpt only when asked for: %v", plain)
	}
	if plain[0]["page_kind"] != "page" {
		t.Fatalf("each item names the kind of its content: %v", plain[0]["page_kind"])
	}
	got := reader.listed("excerpts=true")
	if excerpt, _ := got[0]["excerpt"].(string); len(got) != 1 || !strings.HasPrefix(excerpt, "please read ") {
		t.Fatalf("the start of the comment: %v", got)
	}

	if _, err := e.app.Pool().Exec(context.Background(), "UPDATE comments SET editors_only = true WHERE id = $1", comment); err != nil {
		t.Fatal(err)
	}
	for _, n := range reader.listed("excerpts=true") {
		if n["excerpt"] != nil {
			t.Fatalf("a thread kept for editors gives a reader nothing: %v", n)
		}
	}
	if _, err := e.app.Pool().Exec(context.Background(), "UPDATE comments SET editors_only = false WHERE id = $1", comment); err != nil {
		t.Fatal(err)
	}
	owner.ok("DELETE", "/api/v1/comments/"+comment, nil)
	for _, n := range reader.listed("excerpts=true") {
		if n["excerpt"] != nil {
			t.Fatalf("a removed comment gives nothing: %v", n)
		}
	}
}
