// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kenwerk/kenwerk/internal/db"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// Spec 074, wave GT4: what a set and an approval tell people, and what the gallery costs.

// mentionOf is a mention of a person in a template's text.
func mentionOf(u *user) map[string]any {
	return map[string]any{"type": "mention", "attrs": map[string]any{"id": u.id, "kind": "user", "label": "Someone"}}
}

func TestAPublishedSetTellsEachWatcherOnceOfThePagesTheyMaySee(t *testing.T) {
	tt := newTemplateTeam(t)
	watcher := tt.colleague
	tt.e.deliver() // what happened before the watch is not the set's
	watcher.ok("PUT", "/api/v1/spaces/"+tt.space+"/watch", nil)
	tpl := tt.owner.guided(tt.space, "Project", "Overview")
	tt.owner.entry(tpl, map[string]any{"name": "Plan", "parent": "root", "document": para("Plan")})
	tt.owner.entry(tpl, map[string]any{"name": "Budget", "parent": "root", "document": para("Money"),
		"restrictions": map[string]any{"view": []any{userSubject(tt.owner)}}})
	tt.owner.want(http.StatusNoContent, "PUT", "/api/v1/templates/"+tpl+"/publishing", map[string]any{"publish": true})
	if status, out := tt.owner.makeSet(tpl, map[string]any{"space_id": tt.space, "title": "Apollo"}); status != http.StatusCreated {
		t.Fatalf("set: %d %v", status, out)
	}
	tt.e.deliver()
	rows := tt.e.count("SELECT count(*) FROM notifications WHERE user_id = $1", watcher.id)
	sets := tt.e.count("SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'template_set' AND (data->>'pages')::int = 2", watcher.id)
	if rows != 1 || sets != 1 {
		t.Fatalf("a watcher hears of a set of three, one hidden from them, once with two pages: %d rows, %d right", rows, sets)
	}
	// A watcher who may see none of it is not told.
	stranger := tt.e.user("stranger@example.com", identity.PermUseProduct)
	grantExactly(tt.owner, tt.space, stranger.id, permissions.SpaceView)
	stranger.ok("PUT", "/api/v1/spaces/"+tt.space+"/watch", nil)
	hidden := tt.owner.guided(tt.space, "Secret", "x")
	tt.owner.ok("PATCH", "/api/v1/templates/"+hidden+"/entries/root", map[string]any{"name": "", "kind": "page", "parent": "",
		"restrictions": map[string]any{"view": []any{userSubject(tt.owner)}}})
	tt.owner.want(http.StatusNoContent, "PUT", "/api/v1/templates/"+hidden+"/publishing", map[string]any{"publish": true})
	if status, out := tt.owner.makeSet(hidden, map[string]any{"space_id": tt.space, "title": "Hidden"}); status != http.StatusCreated {
		t.Fatalf("hidden set: %d %v", status, out)
	}
	tt.e.deliver()
	if n := tt.e.count("SELECT count(*) FROM notifications WHERE user_id = $1", stranger.id); n != 0 {
		t.Fatalf("a watcher who may see nothing of a set was told: %d", n)
	}
	// Drafts tell nobody but their creator.
	draft := tt.owner.guided(tt.space, "Quiet", "x")
	tt.owner.entry(draft, map[string]any{"name": "Folder", "kind": "folder", "parent": "root"})
	before := tt.e.count("SELECT count(*) FROM notifications WHERE user_id = $1", watcher.id)
	tt.owner.makeSet(draft, map[string]any{"space_id": tt.space})
	tt.e.deliver()
	if after := tt.e.count("SELECT count(*) FROM notifications WHERE user_id = $1", watcher.id); after != before {
		t.Fatalf("a set of drafts told a watcher: %d to %d", before, after)
	}
}

func TestAPersonMentionedInThreePagesOfASetIsToldOnce(t *testing.T) {
	tt := newTemplateTeam(t)
	named := tt.colleague
	tpl := tt.owner.ok("POST", "/api/v1/spaces/"+tt.space+"/templates", map[string]any{"name": "Kick-off",
		"document": textWith(words("Lead: "), mentionOf(named))})["id"].(string)
	for _, name := range []string{"Risks", "Plan"} {
		tt.owner.entry(tpl, map[string]any{"name": name, "parent": "root", "document": textWith(words("Owner: "), mentionOf(named))})
	}
	tt.owner.want(http.StatusNoContent, "PUT", "/api/v1/templates/"+tpl+"/publishing", map[string]any{"publish": true})
	if status, out := tt.owner.makeSet(tpl, map[string]any{"space_id": tt.space, "title": "Mercury"}); status != http.StatusCreated {
		t.Fatalf("set: %d %v", status, out)
	}
	tt.e.deliver()
	if n := tt.e.count("SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'mention'", named.id); n != 1 {
		t.Fatalf("mentions of one set: %d", n)
	}
	if n := tt.e.count(`SELECT jsonb_array_length(data->'pages') FROM notifications WHERE user_id = $1 AND type = 'mention'`, named.id); n != 3 {
		t.Fatalf("the one mention lists the pages: %d", n)
	}
	if n := tt.e.count("SELECT count(*) FROM mail_queue WHERE user_id = $1", named.id); n > 1 {
		t.Fatalf("mails for one set: %d", n)
	}
}

func TestNoNotificationOfASetHoldsAnAnswer(t *testing.T) {
	tt := newTemplateTeam(t)
	tt.e.deliver()
	tt.colleague.ok("PUT", "/api/v1/spaces/"+tt.space+"/watch", nil)
	secret := "SALARY-87000"
	tpl := tt.owner.ok("POST", "/api/v1/spaces/"+tt.space+"/templates", map[string]any{"name": "Offer",
		"title_pattern": "Offer", "document": fieldDocument("Pay", true)})["id"].(string)
	tt.owner.entry(tpl, map[string]any{"name": "Details", "parent": "root", "document": textWith(words("For "), mentionOf(tt.colleague))})
	tt.owner.want(http.StatusNoContent, "PUT", "/api/v1/templates/"+tpl+"/publishing", map[string]any{"publish": true})
	if status, out := tt.owner.makeSet(tpl, map[string]any{"space_id": tt.space, "answers": map[string]string{"Pay": secret}}); status != http.StatusCreated {
		t.Fatalf("set: %d %v", status, out)
	}
	tt.editor.ok("POST", "/api/v1/spaces/"+tt.space+"/my-templates", map[string]any{"name": "Proposal"})
	tt.e.deliver()
	if n := tt.e.count("SELECT count(*) FROM notifications WHERE data::text LIKE '%' || $1 || '%'", secret); n != 0 {
		t.Fatalf("%d notifications hold an answer", n)
	}
	if n := tt.e.count("SELECT count(*) FROM outbox_events WHERE payload::text LIKE '%' || $1 || '%'", secret); n != 0 {
		t.Fatalf("%d events hold an answer", n)
	}
	// What a notification says in the bell and in a mail is made from ids when shown.
	for _, n := range tt.colleague.notifications() {
		if strings.Contains(fmt.Sprint(n), secret) {
			t.Fatalf("the bell says an answer: %v", n)
		}
	}
}

// galleryWith makes n templates in a space, of which listed have a list of who may use them.
func galleryWith(tt templateTeam, n, listed int) {
	tt.e.t.Helper()
	for i := range n {
		tpl := tt.owner.guided(tt.space, fmt.Sprintf("Template %03d", i), "x")
		if i < listed {
			tt.owner.ok("PUT", "/api/v1/templates/"+tpl+"/rights", map[string]any{
				"use": []any{userSubject(tt.editor), userSubject(tt.owner)}, "edit": []any{userSubject(tt.owner)}})
		}
	}
}

// TestTheGalleryCostsTheSameWithListsAsWithout counts the queries of the gallery: the lists are
// joined in its one query, so a hundred templates cost as many queries with ten lists as with
// none (TPL-900, ARC-239).
func TestTheGalleryCostsTheSameWithListsAsWithout(t *testing.T) {
	tt := newTemplateTeam(t)
	galleryWith(tt, 100, 0)
	path := "/api/v1/spaces/" + tt.space + "/templates"
	without := queriesOf(t, tt.e, tt.editor, path)
	tt.e.sql(`INSERT INTO template_rights (template_id, kind, subject_type, subject_id)
		SELECT id, 'use', 'user', $2 FROM templates WHERE space_id = $1 ORDER BY name LIMIT 10`, tt.space, tt.editor.id)
	with := queriesOf(t, tt.e, tt.editor, path)
	if with != without {
		t.Fatalf("the gallery cost %d queries without lists and %d with them", without, with)
	}
	t.Logf("the gallery of 100 templates costs %d queries, with or without lists", with)
}

// TestTheLoadBudgetOfTheGallery measures TPL-900 on generated data: the gallery of a space
// with 100 templates, 10 of them with lists, within 30 ms, and the plan it runs. It runs only
// when asked, outside the race detector.
func TestTheLoadBudgetOfTheGallery(t *testing.T) {
	if os.Getenv("KENWERK_LOAD_BUDGET") == "" {
		t.Skip("KENWERK_LOAD_BUDGET is not set")
	}
	tt := newTemplateTeam(t)
	galleryWith(tt, 100, 10)
	path := "/api/v1/spaces/" + tt.space + "/templates"
	took := make([]time.Duration, 0, 31)
	for range 36 {
		started := time.Now()
		tt.editor.ok("GET", path, nil)
		took = append(took, time.Since(started))
	}
	took = took[5:]
	t.Logf("the gallery of 100 templates, 10 with lists: p95 %v", p95(took).Round(10*time.Microsecond))
	rows, err := tt.e.app.Pool().Query(db.MustBeCurrent(context.Background()), `EXPLAIN (ANALYZE, COSTS OFF)
		SELECT t.id FROM templates t JOIN pages p ON p.id = t.page_id
		LEFT JOIN template_favourites f ON f.template_id = t.id AND f.user_id = $1
		LEFT JOIN template_uses u ON u.template_id = t.id AND u.user_id = $1 `+galleryFilterForPlan, tt.editor.id, tt.space, true, []string{})
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, line)
	}
	t.Logf("plan:\n%s", strings.Join(plan, "\n"))
}

// galleryFilterForPlan is the filter of the gallery as the templates module writes it, for
// EXPLAIN only; the query-count test above runs the real one.
const galleryFilterForPlan = `WHERE NOT t.hidden
	AND (t.space_id = $2 OR (t.space_id IS NULL AND (NOT t.offered_in_chosen
		OR EXISTS (SELECT 1 FROM template_spaces ts WHERE ts.template_id = t.id AND ts.space_id = $2))))
	AND (NOT (t.guided OR t.offered_in_chosen OR EXISTS (SELECT 1 FROM template_rights r WHERE r.template_id = t.id)) OR $3)
	AND (t.approved_version IS NOT NULL OR (COALESCE(t.owner_id, t.created_by) = $1 AND t.state IN ('draft', 'sent_back')))
	AND (NOT EXISTS (SELECT 1 FROM template_rights r WHERE r.template_id = t.id AND r.kind = 'use')
		OR EXISTS (SELECT 1 FROM template_rights r WHERE r.template_id = t.id AND r.kind = 'use'
			AND ((r.subject_type = 'user' AND r.subject_id = $1) OR (r.subject_type = 'group' AND r.subject_id = ANY ($4::uuid[])))))`
