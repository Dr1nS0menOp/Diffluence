// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/identity"
)

const (
	defaultListed = 50
	maxListed     = 100
)

// Query asks for a part of a person's notifications.
type Query struct {
	UnreadOnly bool
	Type       string
	SpaceID    uuid.UUID
	Limit      int
	Before     time.Time
	// BeforeID is the id of the last item seen; with Before it makes the cursor exact.
	BeforeID uuid.UUID
}

// Item is one line in the panel: one notification, or a group of them.
type Item struct {
	ID        uuid.UUID      `json:"id"`
	Type      string         `json:"type"`
	Reason    string         `json:"reason"`
	Actors    []string       `json:"actors"`
	Count     int            `json:"count"`
	PageID    uuid.UUID      `json:"page_id"`
	PageTitle string         `json:"page_title"`
	SpaceID   uuid.UUID      `json:"space_id"`
	SpaceName string         `json:"space_name"`
	CommentID *uuid.UUID     `json:"comment_id"`
	Read      bool           `json:"read"`
	At        time.Time      `json:"at"`
	Data      map[string]any `json:"data"`

	// firstVersion is the oldest version in a group, so a mail can show all of it.
	firstVersion int
}

type stored struct {
	Item
	groupKey string
	actor    string
}

// visibleRows reads through perm_visible_pages: a row about a page the person may no
// longer see simply does not come back, and returns if access does (COL-072).
const visibleRows = `SELECT n.id, n.type, n.reason, COALESCE(u.display_name, ''), n.page_id, p.title, n.space_id, sp.name,
		n.comment_id, n.read_at IS NOT NULL, n.created_at, n.data, n.group_key
	FROM notifications n
	JOIN perm_visible_pages($1) v ON v.page_id = n.page_id
	JOIN pages p ON p.id = n.page_id
	JOIN spaces sp ON sp.id = n.space_id
	LEFT JOIN users u ON u.id = n.actor_id
	WHERE n.user_id = $1 AND n.hidden_at IS NULL `

func scanStored(r pgx.CollectableRow) (stored, error) {
	var v stored
	var data []byte
	err := r.Scan(&v.ID, &v.Type, &v.Reason, &v.actor, &v.PageID, &v.PageTitle, &v.SpaceID, &v.SpaceName,
		&v.CommentID, &v.Read, &v.At, &data, &v.groupKey)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(data, &v.Data); err != nil {
		return v, fmt.Errorf("notify: read data: %v", err)
	}
	return v, nil
}

// versionOf reads the version a change made. JSON numbers arrive as floats.
func versionOf(i Item) int {
	n, _ := i.Data["version"].(float64)
	return int(n)
}

// grouped makes one line of unread rows of the same type on the same page. Newest
// comes first, so the first row of a group gives it its time and link.
func grouped(rows []stored) []Item {
	out := []Item{}
	at := map[string]int{}
	for _, r := range rows {
		i, ok := at[r.groupKey]
		if ok && !r.Read {
			if r.actor != "" && !slices.Contains(out[i].Actors, r.actor) {
				out[i].Actors = append(out[i].Actors, r.actor)
			}
			out[i].Count++
			out[i].firstVersion = versionOf(r.Item)
			continue
		}
		item := r.Item
		item.Count = 1
		item.firstVersion = versionOf(r.Item)
		item.Actors = []string{}
		if r.actor != "" {
			item.Actors = []string{r.actor}
		}
		if !r.Read {
			at[r.groupKey] = len(out)
		}
		out = append(out, item)
	}
	return out
}

// List returns a person's notifications, newest first.
func (s *Service) List(ctx context.Context, c identity.Caller, q Query) ([]Item, error) {
	limit := q.Limit
	if limit <= 0 || limit > maxListed {
		limit = defaultListed
	}
	var space *uuid.UUID
	if q.SpaceID != uuid.Nil {
		space = &q.SpaceID
	}
	var before *time.Time
	if !q.Before.IsZero() {
		before = &q.Before
	}
	var beforeID *uuid.UUID
	if q.BeforeID != uuid.Nil {
		beforeID = &q.BeforeID
	}
	if q.Type != "" && !slices.Contains(Types, q.Type) {
		return nil, apperr.Invalid("That is not a type of notification.")
	}
	rows, err := s.pool.Query(ctx, visibleRows+`
		AND (NOT $2 OR n.read_at IS NULL)
		AND ($3 = '' OR n.type = $3)
		AND ($4::uuid IS NULL OR n.space_id = $4)
		AND ($5::timestamptz IS NULL OR n.created_at < $5 OR (n.created_at = $5 AND $7::uuid IS NOT NULL AND n.id > $7))
		ORDER BY n.created_at DESC, n.id LIMIT $6`, c.UserID, q.UnreadOnly, q.Type, space, before, limit*2, beforeID)
	if err != nil {
		return nil, err
	}
	found, err := pgx.CollectRows(rows, scanStored)
	if err != nil {
		return nil, err
	}
	items := grouped(found)
	return items[:min(len(items), limit)], nil
}

// Count says how many lines are unread, by the same rule as the list.
func (s *Service) Count(ctx context.Context, c identity.Caller) (int, error) {
	var n int
	// The bell asks often, so the rule is asked only about the pages of unread rows: a
	// few, where perm_visible_pages over every page cost tens of milliseconds (spec 050).
	err := s.pool.QueryRow(ctx, `WITH unread AS MATERIALIZED (
			SELECT n.group_key, n.page_id FROM notifications n
			WHERE n.user_id = $1 AND n.read_at IS NULL AND n.hidden_at IS NULL)
		SELECT count(DISTINCT r.group_key) FROM unread r
		WHERE r.page_id IN (SELECT v.page_id FROM perm_visible_pages($1) v
			WHERE v.page_id = ANY (ARRAY(SELECT DISTINCT page_id FROM unread WHERE page_id IS NOT NULL)))`, c.UserID).Scan(&n)
	return n, err
}

// SetRead marks a line as read or unread. A group goes together. Only a person's own
// rows are touched, whatever id comes in.
func (s *Service) SetRead(ctx context.Context, c identity.Caller, id uuid.UUID, read bool) error {
	var at *time.Time
	if read {
		now := s.now()
		at = &now
	}
	tag, err := s.pool.Exec(ctx, `UPDATE notifications SET read_at = $3
		WHERE user_id = $1 AND (id = $2 OR (group_key = (SELECT group_key FROM notifications WHERE id = $2 AND user_id = $1)
			AND (read_at IS NULL) = $4))`, c.UserID, id, at, read)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound()
	}
	return nil
}

// ReadAll marks everything as read.
func (s *Service) ReadAll(ctx context.Context, c identity.Caller) error {
	_, err := s.pool.Exec(ctx, "UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL", c.UserID)
	return err
}
