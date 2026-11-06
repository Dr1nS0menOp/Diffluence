// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

func TestFeaturesHaveOneStore(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "Open", "x")
	body := func() map[string]any { return map[string]any{"body": commentBody("Hello")} }
	owner.ok("POST", "/api/v1/pages/"+page+"/comments", body())

	// A value left in the old column must not count any more.
	ctx := context.Background()
	if _, err := e.app.Pool().Exec(ctx, `UPDATE spaces SET features = '{"comments": false}' WHERE id = $1`, space); err != nil {
		t.Fatal(err)
	}
	owner.ok("POST", "/api/v1/pages/"+page+"/comments", body())

	owner.ok("PUT", "/api/v1/spaces/"+space+"/features", map[string]any{"comments": false})
	owner.want(409, "POST", "/api/v1/pages/"+page+"/comments", body())
	if got := owner.ok("GET", "/api/v1/pages/"+page, nil)["features"].(map[string]any); got["comments"] != false || got["reactions"] != true {
		t.Fatalf("a page reads the same store: %v", got)
	}
	got := owner.ok("GET", "/api/v1/spaces/"+space, nil)["features"].(map[string]any)
	if got["comments"] != false || got["access_requests"] != false || got["blog"] != true {
		t.Fatalf("a space reads the same store, and asking for access starts off: %v", got)
	}
}

func TestAMoveTakesEveryRowOfTheBranchAlong(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("SRC")
	target, _ := owner.space("DST")
	top := owner.page(space, nil, "Traveller", "x")
	child := owner.page(space, top, "Luggage", "x")
	ctx := context.Background()
	pool := e.app.Pool()

	stays := owner.comment(child, "Stays")
	gone := owner.comment(child, "In the trash")
	owner.ok("DELETE", "/api/v1/comments/"+gone, nil)
	status, body := owner.upload(child, "gone.txt", "words")
	file := fileID(t, status, body)
	owner.ok("DELETE", "/api/v1/attachments/"+file, nil)

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO page_tasks (id, page_id, space_id, text, creator_id, position) VALUES ($1, $2, $3, 'x', $4, 0)`,
		uuid.New(), child, space, owner.id)
	exec(`INSERT INTO notifications (id, user_id, type, reason, page_id, space_id, group_key, source_event)
		VALUES ($1, $2, 'x', 'x', $3, $4, 'x', $5)`, uuid.New(), owner.id, child, space, uuid.New())
	exec(`INSERT INTO access_requests (id, page_id, space_id, requester_id, kind, asked_id) VALUES ($1, $2, $3, $4, 'view', $5)`,
		uuid.New(), child, space, editor.id, owner.id)

	owner.ok("POST", "/api/v1/pages/"+top+"/move", map[string]any{"space_id": target})

	// Every table that has both columns: a new one cannot be forgotten.
	rows, err := pool.Query(ctx, `SELECT a.table_name FROM information_schema.columns a
		JOIN information_schema.columns b ON b.table_name = a.table_name AND b.table_schema = a.table_schema
		JOIN information_schema.tables t ON t.table_name = a.table_name AND t.table_schema = a.table_schema
		WHERE a.table_schema = 'public' AND a.column_name = 'space_id' AND b.column_name = 'page_id' AND t.table_type = 'BASE TABLE'`)
	if err != nil {
		t.Fatal(err)
	}
	tables := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	for _, want := range []string{"comments", "page_tasks", "notifications", "access_requests", "attachments"} {
		if !has(tables, want) {
			t.Fatalf("%s should be in the walk: %v", want, tables)
		}
		var moved int
		if err := pool.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s WHERE page_id = $1", want), child).Scan(&moved); err != nil {
			t.Fatal(err)
		}
		if moved == 0 {
			t.Fatalf("%s has no row to compare", want)
		}
	}
	for _, table := range tables {
		var wrong int
		err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM %s x JOIN pages p ON p.id = x.page_id
			WHERE x.space_id IS NOT NULL AND x.space_id <> p.space_id`, table)).Scan(&wrong)
		if err != nil {
			t.Fatal(err)
		}
		if wrong != 0 {
			t.Errorf("%s: %d rows still say the old space", table, wrong)
		}
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM trash_items WHERE space_id = $1 AND object_id = ANY($2)`,
		space, []string{gone, file}).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatal("what is in the trash follows its page")
	}
	if got := owner.ok("GET", "/api/v1/pages/"+child+"/comments", nil); !strings.Contains(mustJSON(t, got), stays) {
		t.Fatalf("the comment came along: %v", got)
	}
}

func searchComments(u *user, word string) []any {
	u.e.t.Helper()
	return u.ok("GET", "/api/v1/search?kind=comment&q="+word, nil)["results"].([]any)
}

func TestCommentSearchFollowsTheRuleOfAComment(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	page := owner.page(space, nil, "Open page", "x")
	secret := editor.page(space, nil, "Hidden page", "x")
	owner.comment(page, "A plain zebra")
	drafted := owner.comment(page, "An editors giraffe")
	removed := owner.comment(page, "A removed walrus")
	owner.ok("DELETE", "/api/v1/comments/"+removed, nil)
	editor.comment(secret, "A hidden pelican")
	e.hideFromAllBut(secret, editor)
	if _, err := e.app.Pool().Exec(context.Background(), "UPDATE comments SET editors_only = true WHERE id = $1", drafted); err != nil {
		t.Fatal(err)
	}

	found := searchComments(viewer, "zebra")
	if len(found) != 1 {
		t.Fatalf("a comment is found: %v", found)
	}
	hit := found[0].(map[string]any)
	if hit["page_id"] != page || hit["comment_id"] == nil || !strings.Contains(hit["snippet"].(string), "zebra") {
		t.Fatalf("a result links to the comment: %v", hit)
	}
	for _, u := range []*user{viewer, stranger} {
		if got := searchComments(u, "giraffe"); len(got) != 0 {
			t.Fatalf("a thread for editors stayed out of reach of %s: %v", u.email, got)
		}
	}
	if got := searchComments(stranger, "zebra"); len(got) != 0 {
		t.Fatalf("a comment follows its page: %v", got)
	}
	if got := searchComments(editor, "giraffe"); len(got) != 1 {
		t.Fatalf("an editor finds it: %v", got)
	}
	if got := searchComments(owner, "walrus"); len(got) != 0 {
		t.Fatalf("a comment in the trash is gone: %v", got)
	}
	if got := searchComments(owner, "pelican"); len(got) != 0 {
		t.Fatalf("a comment on a hidden page is hidden: %v", got)
	}
	if got := searchComments(editor, "pelican"); len(got) != 1 {
		t.Fatalf("whoever sees the page finds it: %v", got)
	}
}

// The SQL function and the check on one comment are the same rule (PRM-077).
func TestCommentVisibilityAgreesWithTheCheckOnOneComment(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "Open page", "x")
	plain := owner.comment(page, "Plain")
	drafted := owner.comment(page, "For editors")
	ctx := context.Background()
	if _, err := e.app.Pool().Exec(ctx, "UPDATE comments SET editors_only = true WHERE id = $1", drafted); err != nil {
		t.Fatal(err)
	}
	listed := func(u *user, comment string) bool {
		t.Helper()
		var found bool
		err := e.app.Pool().QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM perm_visible_comments($1) v WHERE v.comment_id = $2)", u.id, comment).Scan(&found)
		if err != nil {
			t.Fatal(err)
		}
		return found
	}
	checkAll := func(when string) {
		t.Helper()
		for _, u := range []*user{owner, editor, viewer} {
			for _, comment := range []string{plain, drafted} {
				status, _ := u.do("GET", "/api/v1/comments/"+comment+"/reactions", nil)
				if listed(u, comment) != (status == 200) {
					t.Errorf("%s: %s and comment %s: SQL says %v, the route says %d", when, u.email, comment, listed(u, comment), status)
				}
			}
		}
	}
	checkAll("open")
	owner.restrict(page, "edit", owner.id)
	checkAll("with an edit restriction")
	owner.ok("POST", "/api/v1/pages/"+page+"/archive", map[string]any{"include_children": true})
	checkAll("archived")
}

func TestAReactionChangesNothingOnAnArchivedPageOrInAnArchivedSpace(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	page := owner.page(space, nil, "Quiet", "x")
	other := owner.page(space, nil, "Open", "x")
	react := "/api/v1/pages/" + page + "/reactions/%F0%9F%91%8D"
	editor.ok("PUT", react, nil)
	if got := editor.ok("GET", "/api/v1/pages/"+other, nil)["can"].(map[string]any); got[permissions.ActionReact] != true {
		t.Fatalf("a reaction is allowed on an open page: %v", got)
	}
	owner.ok("POST", "/api/v1/pages/"+page+"/archive", map[string]any{"include_children": true})
	editor.want(403, "DELETE", react, nil)
	if got := editor.ok("GET", "/api/v1/pages/"+page, nil)["can"].(map[string]any); got[permissions.ActionReact] != false {
		t.Fatalf("the screen must not offer it: %v", got)
	}
	owner.ok("POST", "/api/v1/spaces/"+space+"/archive", nil)
	editor.want(403, "PUT", "/api/v1/pages/"+other+"/reactions/%F0%9F%91%8D", nil)
}

func TestRemovingAVersionBlanksTheQuotesThatHoldIt(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	owner, _, _, space := e.team("ENG")
	owner.grant(space, "user", admin.id, permissions.TemplateViewer)
	secret := "the password is swordfish"
	page := owner.page(space, nil, "Leak", secret+" and more")
	thread := owner.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{
		"body":   para("This should go"),
		"anchor": map[string]any{"kind": "text", "exact": secret, "prefix": "", "suffix": " and more", "offset": 0, "version": 1},
	})["id"].(string)
	kept := owner.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{
		"body":   para("This may stay"),
		"anchor": map[string]any{"kind": "text", "exact": "and more", "prefix": "", "suffix": "", "offset": 0, "version": 1},
	})["id"].(string)
	owner.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Leak", "document": para("Fixed, and more"), "base_version": 1})

	admin.ok("DELETE", "/api/v1/admin/pages/"+page+"/versions/1", map[string]any{"reason": "A leaked password"})

	listing := owner.want(200, "GET", "/api/v1/pages/"+page+"/comments", nil)
	if strings.Contains(listing, "swordfish") {
		t.Fatalf("the words live on in a comment: %s", listing)
	}
	if !strings.Contains(listing, thread) || !strings.Contains(listing, kept) || !strings.Contains(listing, "and more") {
		t.Fatalf("the threads stay, and words still on the page keep their mark: %s", listing)
	}
	if e.outboxCount("comment.quote_blanked") != 1 {
		t.Fatal("blanking is a change of its own")
	}
}
