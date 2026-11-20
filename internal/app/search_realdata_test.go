// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"net/url"
	"testing"
	"time"

	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// realDataSearchBudget is what spec 080 allows one search to take on the seeded data, as the
// median of seven through the whole application, for a reader of the largest space with a
// tenth of the pages under control.
const realDataSearchBudget = 100 * time.Millisecond

// swapLastLetters makes a typo of a word, so the near tiers have something to rank.
func swapLastLetters(word string) string {
	r := []rune(word)
	if len(r) < 2 {
		return word
	}
	r[len(r)-1], r[len(r)-2] = r[len(r)-2], r[len(r)-1]
	return string(r)
}

// TestSearchOnRealData measures the searches of spec 080 on a restored dump of the seeded data
// (scripts/stress/real-data.sh): the words a reader types, the filters of wave SR1, and the
// box, each through the whole application.
func TestSearchOnRealData(t *testing.T) {
	e := openRealData(t)
	markControlled(t, e)
	stamp := time.Now().Format("150405")
	reader := e.user("sr1-reader-"+stamp+"@example.com", identity.PermUseProduct, identity.PermViewProfiles)
	space := e.scalar(`SELECT space_id::text FROM pages GROUP BY space_id ORDER BY count(*) DESC LIMIT 1`)
	e.sql("SELECT rbac_set_exact($1::uuid, 'user', $2::uuid, $3::text[])", space, reader.id, permissions.Templates[permissions.TemplateViewer])

	common := e.scalar(`SELECT word FROM ts_stat($$SELECT words FROM search_pages$$) WHERE length(word) >= 5 ORDER BY ndoc DESC LIMIT 1`)
	rare := e.scalar(`SELECT word FROM ts_stat($$SELECT words FROM search_pages$$) WHERE length(word) >= 5 ORDER BY ndoc, word LIMIT 1`)
	below := e.scalar(`SELECT t.ancestor_id::text FROM page_tree t JOIN pages p ON p.id = t.ancestor_id
		WHERE p.space_id = $1 AND t.depth > 0 AND p.id <> (SELECT home_page_id FROM spaces WHERE id = $1)
		GROUP BY t.ancestor_id ORDER BY count(*) DESC LIMIT 1`, space)
	// The labels of the seeded data may sit in other spaces than the largest: a label alone is
	// then a search for a word the reader finds nowhere, which is the costly case of a filter.
	label := e.scalar(`SELECT l.label FROM page_labels l GROUP BY l.label ORDER BY count(*) DESC, l.label LIMIT 1`)
	people := e.scalar(`SELECT string_agg(a, '&contributor=') FROM (SELECT v.author_id::text AS a FROM page_versions v
		JOIN pages p ON p.id = v.page_id WHERE p.space_id = $1 GROUP BY v.author_id ORDER BY count(*) DESC LIMIT 2) x`, space)
	t.Logf("common %q, rare %q, typo %q, label %q", common, rare, swapLastLetters(common), label)
	// Spec 086: tasks by the most common word of their text, and people by the start of the most
	// common first word of a name.
	taskWord := e.scalar(`SELECT COALESCE((SELECT word FROM ts_stat($$SELECT to_tsvector('simple', text) FROM page_tasks$$)
		WHERE length(word) >= 4 ORDER BY ndoc DESC, word LIMIT 1), 'plan')`)
	nameStart := e.scalar(`SELECT COALESCE((SELECT left(lower(split_part(display_name, ' ', 1)), 3) FROM users
		WHERE type <> 'service' GROUP BY 1 ORDER BY count(*) DESC, 1 LIMIT 1), 'a')`)
	t.Logf("tasks %s, people %s, groups %s; task word %q, name start %q", e.scalar(`SELECT count(*)::text FROM page_tasks`),
		e.scalar(`SELECT count(*)::text FROM users`), e.scalar(`SELECT count(*)::text FROM groups`), taskWord, nameStart)

	searches := []struct{ name, path string }{
		{"a common word", "/api/v1/search?q=" + url.QueryEscape(common)},
		{"a common word, newest first", "/api/v1/search?sort=date&q=" + url.QueryEscape(common)},
		{"a rare word", "/api/v1/search?q=" + url.QueryEscape(rare)},
		{"a typo", "/api/v1/search?q=" + url.QueryEscape(swapLastLetters(common))},
		{"nothing found", "/api/v1/search?q=zqxjvwkq"},
		{"a label alone", "/api/v1/search?label=" + url.QueryEscape(label)},
		{"below a page", "/api/v1/search?q=" + url.QueryEscape(common) + "&below=" + below},
		{"two people", "/api/v1/search?q=" + url.QueryEscape(common) + "&contributor=" + people},
		{"the box", "/api/v1/search/quick?q=" + url.QueryEscape(common)},
		{"the box, a typo", "/api/v1/search/quick?q=" + url.QueryEscape(swapLastLetters(common))},
		{"tasks", "/api/v1/search?kind=task&q=" + url.QueryEscape(taskWord)},
		{"a field for people", "/api/v1/directory?q=" + url.QueryEscape(nameStart)},
		{"the People tab", "/api/v1/people?q=" + url.QueryEscape(nameStart)},
	}
	for _, s := range searches {
		took := medianOf(reader, "GET", s.path, nil)
		t.Logf("%-28s %v", s.name, took.Round(time.Microsecond*100))
		if took > realDataSearchBudget {
			t.Errorf("%s takes %v, budget %v", s.name, took, realDataSearchBudget)
		}
	}
}
