// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// referenceFeed is the feed as it was written before spec 050: every visible page first,
// then its newest version that was not silent. The new query must give the same rows in
// the same order; this one stays here only to prove that.
const referenceFeed = `WITH visible AS (SELECT page_id FROM perm_visible_pages($1)),
	changes AS (
		SELECT DISTINCT ON (v.page_id) v.page_id, v.author_id, v.created_at AS at,
			CASE WHEN v.number = 1 THEN 'created' ELSE 'changed' END AS kind
		FROM page_versions v JOIN visible ON visible.page_id = v.page_id
		WHERE NOT v.silent
		ORDER BY v.page_id, v.number DESC
	)
	SELECT p.title, c.kind
	FROM changes c
	JOIN pages p ON p.id = c.page_id AND p.status = 'published'
	JOIN spaces s ON s.id = p.space_id AND s.status = 'active'
	WHERE (NOT $2 OR (c.author_id <> $1 AND (
		c.author_id IN (SELECT followed_id FROM follows WHERE user_id = $1)
		OR EXISTS (SELECT 1 FROM watches w WHERE w.user_id = $1 AND (
			(w.target_type = 'page' AND w.target_id = c.page_id)
			OR (w.target_type = 'space' AND w.target_id = p.space_id)
			OR (w.target_type = 'branch' AND w.target_id IN
				(SELECT t.ancestor_id FROM page_tree t WHERE t.descendant_id = c.page_id)))))))
	ORDER BY c.at DESC, c.page_id`

// referencePopular is the popular tab as it was written before spec 050: a score for every
// visible page.
const referencePopular = `WITH visible AS (SELECT page_id FROM perm_visible_pages($1)),
	scores AS (
		SELECT v.page_id,
			(SELECT count(*) FROM reactions r WHERE r.target_type = 'page' AND r.target_id = v.page_id
				AND r.created_at > now() - make_interval(days => 7)) AS reactions,
			(SELECT count(*) FROM comments m WHERE m.page_id = v.page_id AND m.trash_item_id IS NULL AND NOT m.purged
				AND NOT m.editors_only AND m.created_at > now() - make_interval(days => 7)) AS comments
		FROM visible v
	)
	SELECT p.title, 'popular'
	FROM scores c
	JOIN pages p ON p.id = c.page_id AND p.status = 'published'
	JOIN spaces s ON s.id = p.space_id AND s.status = 'active'
	WHERE c.reactions + c.comments > 0
	ORDER BY c.reactions + c.comments DESC, p.updated_at DESC, c.page_id`

// feedRows reads a whole feed through the API, a few items at a time, so the cursor
// is part of what is compared.
func feedRows(u *user, tab string) []string {
	u.e.t.Helper()
	out := []string{}
	query := url.Values{"tab": {tab}, "limit": {"3"}}
	if tab == "popular" {
		// Popular has no cursor: one part holds it all.
		query.Set("limit", "50")
	}
	for range 50 {
		items, _ := u.ok("GET", "/api/v1/feed?"+query.Encode(), nil)["items"].([]any)
		if len(items) == 0 {
			return out
		}
		for _, item := range items {
			row := item.(map[string]any)
			out = append(out, fmt.Sprintf("%s/%s", row["title"], row["kind"]))
		}
		if tab == "popular" {
			return out
		}
		last := items[len(items)-1].(map[string]any)
		query.Set("before", last["at"].(string))
		query.Set("before_id", last["page_id"].(string))
	}
	u.e.t.Fatal("the feed did not end")
	return nil
}

func TestTheFeedGivesWhatTheFormerQueryGave(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	outsider := e.user("outsider@example.com", identity.PermUseProduct)
	eng, _ := owner.space("ENG")
	ops, _ := owner.space("OPS")
	owner.grant(eng, "user", reader.id, permissions.TemplateEditor)

	top := owner.page(eng, nil, "Top", "x")
	owner.page(eng, top, "Child", "x")
	secret := owner.page(eng, nil, "Secret", "x")
	owner.page(eng, secret, "Under secret", "x")
	owner.restrict(secret, "view", owner.id)
	quiet := owner.page(eng, nil, "Quiet", "x")
	owner.ok("POST", "/api/v1/pages/"+quiet+"/publish", map[string]any{"title": "Quiet", "base_version": 1, "silent": true, "document": para("small")})
	busy := reader.page(eng, nil, "Busy", "x")
	for version := 1; version <= 2; version++ {
		reader.ok("POST", "/api/v1/pages/"+busy+"/publish", map[string]any{"title": "Busy", "base_version": version, "document": para(fmt.Sprint("v", version))})
	}
	reader.ok("POST", "/api/v1/pages", map[string]any{"space_id": eng, "title": "Draft only"})
	owner.page(ops, nil, "Elsewhere", "x")
	reader.ok("PUT", "/api/v1/pages/"+top+"/watch", nil)
	// Every way into "for you" (spec 061): a followed person, a watched space and a
	// watched branch, besides the watched page above.
	if _, err := e.app.Pool().Exec(context.Background(),
		"INSERT INTO follows (user_id, followed_id) VALUES ($1, $2)", reader.id, owner.id); err != nil {
		t.Fatal(err)
	}
	owner.ok("PUT", "/api/v1/spaces/"+eng+"/watch", nil)
	// Someone who reaches pages only through a watched branch, and may see them.
	brancher := e.user("brancher@example.com", identity.PermUseProduct)
	owner.grant(eng, "user", brancher.id, permissions.TemplateViewer)
	if _, err := e.app.Pool().Exec(context.Background(),
		"INSERT INTO watches (user_id, target_type, target_id) VALUES ($1, 'branch', $2)", brancher.id, top); err != nil {
		t.Fatal(err)
	}
	reader.ok("PUT", "/api/v1/pages/"+top+"/reactions/"+url.PathEscape("👍"), nil)
	owner.ok("PUT", "/api/v1/pages/"+busy+"/reactions/"+url.PathEscape("👍"), nil)
	owner.comment(busy, "nice")
	owner.comment(secret, "only for me")

	for _, u := range []*user{owner, reader, outsider, brancher} {
		for _, tab := range []string{"recent", "for_you", "popular"} {
			reference, args := referenceFeed, []any{u.id, tab == "for_you"}
			if tab == "popular" {
				reference, args = referencePopular, []any{u.id}
			}
			rows, err := e.app.Pool().Query(context.Background(), reference, args...)
			if err != nil {
				t.Fatal(err)
			}
			want, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (string, error) {
				var title, kind string
				err := r.Scan(&title, &kind)
				return title + "/" + kind, err
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := feedRows(u, tab); fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("%s, %s:\n got  %v\n want %v", u.email, tab, got, want)
			}
		}
	}
	// The case that matters most: what the reader may not see stays out.
	if got := fmt.Sprint(feedRows(reader, "recent")); !strings.Contains(got, "Top") || strings.Contains(got, "Secret") || strings.Contains(got, "Under secret") || strings.Contains(got, "Elsewhere") {
		t.Errorf("reader sees %s", got)
	}
	if got := fmt.Sprint(feedRows(reader, "popular")); !strings.Contains(got, "Busy") || strings.Contains(got, "Secret") {
		t.Errorf("reader sees as popular %s", got)
	}
}
