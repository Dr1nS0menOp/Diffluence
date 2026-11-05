// SPDX-License-Identifier: Apache-2.0

// Package tasks is the index of the tasks in current versions (spec 010, C6). The
// document stays the one truth: this list only makes tasks findable, and it is
// rewritten in the transaction that publishes a version.
package tasks

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
)

// The tabs and filters of My tasks.
const (
	TabAssigned = "assigned"
	TabCreated  = "created"

	StatusOpen = "open"
	StatusDone = "done"
	StatusAll  = "all"
)

const maxListed = 500

// AssignIDs gives every task of a document an id of its own, before the version is
// written. An id on another page is taken: it came in with a paste or a copy.
func AssignIDs(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, doc *document.Node) error {
	rows, err := tx.Query(ctx, "SELECT id FROM page_tasks WHERE page_id <> $1 AND id = ANY($2)", pageID, taskIDs(*doc))
	if err != nil {
		return err
	}
	taken, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	document.AssignTaskIDs(doc, func(id uuid.UUID) bool { return slices.Contains(taken, id) }, ids.New)
	return nil
}

func taskIDs(doc document.Node) []uuid.UUID {
	out := []uuid.UUID{}
	for _, t := range document.Tasks(doc) {
		if t.ID != uuid.Nil {
			out = append(out, t.ID)
		}
	}
	return out
}

type stored struct {
	assignee *uuid.UUID
	due      *string
}

func storedTasks(ctx context.Context, tx pgx.Tx, pageID uuid.UUID) (map[uuid.UUID]stored, error) {
	rows, err := tx.Query(ctx, "SELECT id, assignee_id, due_on::text FROM page_tasks WHERE page_id = $1", pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]stored{}
	for rows.Next() {
		var id uuid.UUID
		var s stored
		if err := rows.Scan(&id, &s.assignee, &s.due); err != nil {
			return nil, err
		}
		out[id] = s
	}
	return out, rows.Err()
}

func sameDue(a *string, b string) bool {
	if a == nil {
		return b == ""
	}
	return *a == b
}

// Sync rewrites the index of a page from its new version. It returns the people
// who got a task in this version, so they can be told once.
func Sync(ctx context.Context, tx pgx.Tx, pageID, spaceID, author uuid.UUID, doc document.Node) ([]uuid.UUID, error) {
	before, err := storedTasks(ctx, tx, pageID)
	if err != nil {
		return nil, err
	}
	list := document.Tasks(doc)
	if _, err := tx.Exec(ctx, "DELETE FROM page_tasks WHERE page_id = $1 AND NOT (id = ANY($2))", pageID, taskIDs(doc)); err != nil {
		return nil, err
	}
	assigned := []uuid.UUID{}
	for position, t := range list {
		var due *string
		if t.Due != "" {
			due = &t.Due
		}
		old, known := before[t.ID]
		// A new date means a new reminder.
		keepsReminder := known && sameDue(old.due, t.Due)
		if _, err := tx.Exec(ctx, `INSERT INTO page_tasks (id, page_id, space_id, text, assignee_id, creator_id, due_on, done, done_at, position)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, CASE WHEN $8 THEN now() END, $9)
			ON CONFLICT (id) DO UPDATE SET text = EXCLUDED.text, assignee_id = EXCLUDED.assignee_id, due_on = EXCLUDED.due_on,
				done = EXCLUDED.done, position = EXCLUDED.position, updated_at = now(),
				done_at = CASE WHEN EXCLUDED.done AND NOT page_tasks.done THEN now() WHEN EXCLUDED.done THEN page_tasks.done_at END,
				due_notified = page_tasks.due_notified AND $10`,
			t.ID, pageID, spaceID, t.Text, t.AssigneeID, author, due, t.Done, position, keepsReminder); err != nil {
			return nil, err
		}
		isNewAssignee := t.AssigneeID != nil && !t.Done && (!known || old.assignee == nil || *old.assignee != *t.AssigneeID)
		if isNewAssignee && !slices.Contains(assigned, *t.AssigneeID) {
			assigned = append(assigned, *t.AssigneeID)
		}
	}
	return assigned, nil
}

// PageOf says which page holds a task.
func PageOf(ctx context.Context, q db.Querier, id uuid.UUID) (uuid.UUID, error) {
	var page uuid.UUID
	err := q.QueryRow(ctx, "SELECT page_id FROM page_tasks WHERE id = $1", id).Scan(&page)
	if errors.Is(err, pgx.ErrNoRows) {
		return page, apperr.NotFound()
	}
	return page, err
}

// Item is one row of My tasks.
type Item struct {
	ID           uuid.UUID  `json:"id"`
	Text         string     `json:"text"`
	Done         bool       `json:"done"`
	Due          *string    `json:"due"`
	PageID       uuid.UUID  `json:"page_id"`
	PageTitle    string     `json:"page_title"`
	SpaceName    string     `json:"space_name"`
	AssigneeID   *uuid.UUID `json:"assignee_id"`
	AssigneeName string     `json:"assignee_name"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Service lists tasks.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Mine lists tasks of one person. It goes through perm_visible_pages, so a task on
// a page in the trash or out of reach disappears.
func (s *Service) Mine(ctx context.Context, c identity.Caller, tab, status string) ([]Item, error) {
	if tab == "" {
		tab = TabAssigned
	}
	if status == "" {
		status = StatusOpen
	}
	if !slices.Contains([]string{TabAssigned, TabCreated}, tab) || !slices.Contains([]string{StatusOpen, StatusDone, StatusAll}, status) {
		return nil, apperr.Invalid("That is not a list of tasks.")
	}
	rows, err := s.pool.Query(ctx, `SELECT t.id, t.text, t.done, t.due_on::text, t.page_id, p.title, sp.name,
			t.assignee_id, COALESCE(u.display_name, ''), t.updated_at
		FROM page_tasks t
		JOIN perm_visible_pages($1) v ON v.page_id = t.page_id
		JOIN pages p ON p.id = t.page_id
		JOIN spaces sp ON sp.id = t.space_id
		LEFT JOIN users u ON u.id = t.assignee_id
		WHERE CASE $2 WHEN 'assigned' THEN t.assignee_id = $1 ELSE t.creator_id = $1 END
		  AND CASE $3 WHEN 'open' THEN NOT t.done WHEN 'done' THEN t.done ELSE true END
		ORDER BY t.due_on NULLS LAST, t.updated_at DESC, t.id LIMIT $4`, c.UserID, tab, status, maxListed)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Item, error) {
		var v Item
		err := r.Scan(&v.ID, &v.Text, &v.Done, &v.Due, &v.PageID, &v.PageTitle, &v.SpaceName, &v.AssigneeID, &v.AssigneeName, &v.UpdatedAt)
		return v, err
	})
	if out == nil {
		out = []Item{}
	}
	return out, err
}

// Due is a task whose person must be reminded.
type Due struct {
	ID       uuid.UUID
	PageID   uuid.UUID
	SpaceID  uuid.UUID
	Assignee uuid.UUID
}

// TakeDue claims open tasks that reached their date, and marks them as told, so a
// reminder goes out once.
func TakeDue(ctx context.Context, tx pgx.Tx, today string) ([]Due, error) {
	rows, err := tx.Query(ctx, `UPDATE page_tasks SET due_notified = true
		WHERE id IN (SELECT id FROM page_tasks
			WHERE NOT done AND NOT due_notified AND assignee_id IS NOT NULL AND due_on <= $1::date
			ORDER BY due_on LIMIT 500 FOR UPDATE SKIP LOCKED)
		RETURNING id, page_id, space_id, assignee_id`, today)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Due, error) {
		var d Due
		err := r.Scan(&d.ID, &d.PageID, &d.SpaceID, &d.Assignee)
		return d, err
	})
}
