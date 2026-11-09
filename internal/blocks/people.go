// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/people"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Person is one card: only what the directory gives this reader.
type Person struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	JobTitle   string    `json:"job_title,omitempty"`
	Department string    `json:"department,omitempty"`
	Location   string    `json:"location,omitempty"`
	Email      string    `json:"email,omitempty"`
	HasPicture bool      `json:"has_picture"`
}

// Contributions a writer can choose.
const (
	contributionVersions = "versions"
	contributionComments = "comments"
	contributionWatchers = "watchers"
)

// contributionKinds keeps what this reader may see. Who watches a page follows the same
// rule as the list of watchers: a block never shows more than that route.
func (r reader) contributionKinds(ctx context.Context, page uuid.UUID) ([]string, error) {
	kinds := r.list("kinds")
	if len(kinds) == 0 {
		return []string{contributionVersions}, nil
	}
	if !slices.Contains(kinds, contributionWatchers) {
		return kinds, nil
	}
	d, err := r.s.perms.CheckPage(ctx, r.s.pool, r.userID, page, permissions.PageWatchers, uuid.Nil)
	if err != nil {
		return nil, err
	}
	if d.Allowed {
		return kinds, nil
	}
	return slices.DeleteFunc(kinds, func(kind string) bool { return kind == contributionWatchers }), nil
}

// contributors lists who wrote, commented on, labelled or watch a page. A comment for
// editors, or one in the trash, is not something every reader may know about.
func contributors(ctx context.Context, r reader) (Answer, error) {
	id := r.target()
	if ok, err := r.visible(ctx, id); !ok {
		return Answer{}, err
	}
	kinds, err := r.contributionKinds(ctx, id)
	if err != nil {
		return Answer{}, err
	}
	rows, err := r.s.pool.Query(ctx, `WITH acts AS (
			SELECT author_id AS person, created_at AS at FROM page_versions WHERE page_id = $1 AND 'versions' = ANY($3)
			UNION ALL SELECT author_id, created_at FROM comments
				WHERE page_id = $1 AND NOT editors_only AND NOT purged AND trash_item_id IS NULL AND 'comments' = ANY($3)
			UNION ALL SELECT created_by, created_at FROM page_labels WHERE page_id = $1 AND 'labels' = ANY($3)
			UNION ALL SELECT user_id, created_at FROM watches WHERE target_type = 'page' AND target_id = $1 AND 'watchers' = ANY($3))
		SELECT a.person, COALESCE(u.display_name, ''), count(*), max(a.at) FROM acts a LEFT JOIN users u ON u.id = a.person
		GROUP BY a.person, u.display_name ORDER BY count(*) DESC, a.person LIMIT $2`, id, r.max(defaultRows), kinds)
	out, err := scanRows(rows, err, func(row pgx.CollectableRow, v *Row) error {
		var at time.Time
		err := row.Scan(&v.ID, &v.Title, &v.Count, &at)
		v.At = &at
		return err
	})
	return Answer{Rows: out}, err
}

// contributionActs counts versions and comments for a page and all below it that the
// reader may see. The rest of the query groups them.
const contributionActs = `WITH seen AS (SELECT t.descendant_id AS page FROM page_tree t
		JOIN perm_visible_pages($1) v ON v.page_id = t.descendant_id
		JOIN pages p ON p.id = t.descendant_id WHERE t.ancestor_id = $2 AND p.status = 'published'),
	acts AS (SELECT v.page_id AS page, v.author_id AS person, 1 AS versions, 0 AS comments
			FROM page_versions v JOIN seen s ON s.page = v.page_id
		UNION ALL SELECT c.page_id, c.author_id, 0, 1 FROM comments c JOIN seen s ON s.page = c.page_id
			WHERE NOT c.editors_only AND NOT c.purged AND c.trash_item_id IS NULL)`

// contributionTable shows those numbers by person or by page.
func contributionTable(ctx context.Context, r reader) (Answer, error) {
	id := r.target()
	if ok, err := r.visible(ctx, id); !ok {
		return Answer{}, err
	}
	query := contributionActs + ` SELECT a.person, COALESCE(u.display_name, ''), sum(a.versions)::int, sum(a.comments)::int
		FROM acts a LEFT JOIN users u ON u.id = a.person GROUP BY a.person, u.display_name
		ORDER BY sum(a.versions) + sum(a.comments) DESC, a.person LIMIT $3`
	if r.text("by") == "page" {
		query = contributionActs + ` SELECT a.page, p.title, sum(a.versions)::int, sum(a.comments)::int
			FROM acts a JOIN pages p ON p.id = a.page GROUP BY a.page, p.title
			ORDER BY sum(a.versions) + sum(a.comments) DESC, a.page LIMIT $3`
	}
	rows, err := r.s.pool.Query(ctx, query, r.userID, id, r.max(defaultRows))
	out, err := scanRows(rows, err, func(row pgx.CollectableRow, v *Row) error {
		var versions, comments int
		err := row.Scan(&v.ID, &v.Title, &versions, &comments)
		v.Cells = []string{strconv.Itoa(versions), strconv.Itoa(comments)}
		return err
	})
	return Answer{Rows: out, ColumnKeys: []string{contributionVersions, contributionComments}}, err
}

// personNamed finds who a setting means. Whether the reader may see them is for the
// directory to say.
func (r reader) personNamed(ctx context.Context, text string) (uuid.UUID, bool, error) {
	var id uuid.UUID
	err := r.s.pool.QueryRow(ctx, `SELECT id FROM users WHERE status = 'active' AND anonymised_at IS NULL
		AND (id::text = lower($1) OR lower(email) = lower($1) OR lower(display_name) = lower($1)) ORDER BY id LIMIT 1`, text).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	return id, err == nil, err
}

// card is one person as this reader may see them. Someone they may not see gives the
// same answer as someone who does not exist.
func (r reader) card(ctx context.Context, id uuid.UUID) (Person, bool, error) {
	p, err := r.s.people.Get(ctx, r.caller, id)
	var refused *apperr.Error
	if errors.As(err, &refused) && refused.Kind == apperr.KindNotFound {
		return Person{}, false, nil
	}
	if err != nil {
		return Person{}, false, err
	}
	return Person{ID: p.ID, Name: p.Name, JobTitle: p.JobTitle, Department: p.Department, Location: p.Location,
		Email: p.Email, HasPicture: p.HasPicture}, true, nil
}

func personCard(ctx context.Context, r reader) (Answer, error) {
	id, ok, err := r.personNamed(ctx, r.text("person"))
	if err != nil || !ok {
		return Answer{}, err
	}
	person, ok, err := r.card(ctx, id)
	if err != nil || !ok {
		return Answer{}, err
	}
	return Answer{People: []Person{person}}, nil
}

// peopleList shows people chosen by hand, or a group, through the directory.
func peopleList(ctx context.Context, r reader) (Answer, error) {
	out := []Person{}
	for _, text := range r.list("people") {
		id, ok, err := r.personNamed(ctx, text)
		if err != nil {
			return Answer{}, err
		}
		if !ok {
			continue
		}
		person, ok, err := r.card(ctx, id)
		if err != nil {
			return Answer{}, err
		}
		if ok {
			out = append(out, person)
		}
	}
	group, ok, err := r.groupNamed(ctx, r.text("group"))
	if err != nil || !ok {
		return Answer{People: out}, err
	}
	members, _, err := r.s.people.List(ctx, r.caller, people.Query{GroupID: group, Limit: r.max(defaultPeople)})
	if err != nil {
		return Answer{}, err
	}
	for _, m := range members {
		out = append(out, Person{ID: m.ID, Name: m.Name, JobTitle: m.JobTitle, Department: m.Department, HasPicture: m.HasPicture})
	}
	return Answer{People: out}, nil
}

func (r reader) groupNamed(ctx context.Context, name string) (uuid.UUID, bool, error) {
	if name == "" {
		return uuid.Nil, false, nil
	}
	var id uuid.UUID
	err := r.s.pool.QueryRow(ctx, "SELECT id FROM groups WHERE lower(name) = lower($1) ORDER BY id LIMIT 1", name).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	return id, err == nil, err
}
