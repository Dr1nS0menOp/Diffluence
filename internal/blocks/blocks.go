// SPDX-License-Identifier: Apache-2.0

// Package blocks answers dynamic blocks (spec 012). A document stores settings, and
// every reader gets their own answer here, so a block can never show more than a
// search by that reader. Everything goes through perm_visible_pages.
package blocks

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
)

const (
	defaultRows = 15
	maxRows     = 500
	maxDepth    = 5
)

// Row is one line of an answer.
type Row struct {
	ID        uuid.UUID  `json:"id"`
	ParentID  *uuid.UUID `json:"parent_id,omitempty"`
	Title     string     `json:"title"`
	Emoji     string     `json:"emoji,omitempty"`
	SpaceKey  string     `json:"space_key,omitempty"`
	SpaceName string     `json:"space_name,omitempty"`
	At        *time.Time `json:"at,omitempty"`
	By        string     `json:"by,omitempty"`
	Detail    string     `json:"detail,omitempty"`
	Count     int        `json:"count,omitempty"`
	Cells     []string   `json:"cells,omitempty"`
	Done      *bool      `json:"done,omitempty"`
}

// Answer is what one block shows to one reader.
type Answer struct {
	Rows     []Row           `json:"rows"`
	Columns  []string        `json:"columns,omitempty"`
	Document json.RawMessage `json:"document,omitempty"`
	Source   *Row            `json:"source,omitempty"`
}

// Request asks for one block. PageID is the page it stands on, if any.
type Request struct {
	Kind     string
	PageID   uuid.UUID
	Settings map[string]any
}

// Service answers blocks.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// reader is one request with its clean settings.
type reader struct {
	s        *Service
	userID   uuid.UUID
	pageID   uuid.UUID
	spaceID  uuid.UUID
	settings map[string]any
}

func (r reader) text(key string) string {
	v, _ := r.settings[key].(string)
	return v
}

func (r reader) on(key string) bool {
	v, _ := r.settings[key].(bool)
	return v
}

func (r reader) list(key string) []string {
	items, _ := r.settings[key].([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func (r reader) max(fallback int) int {
	if n, ok := r.settings["max"].(int); ok {
		return min(n, maxRows)
	}
	return fallback
}

// target is the page a block is about: its setting, or the page it stands on.
func (r reader) target() uuid.UUID {
	if id, err := uuid.Parse(r.text("page")); err == nil {
		return id
	}
	return r.pageID
}

func (r reader) space() *uuid.UUID {
	if !r.on("this_space") || r.spaceID == uuid.Nil {
		return nil
	}
	return &r.spaceID
}

// visible is the one question every page in a setting must pass. A hidden page and a
// missing page give the same answer.
func (r reader) visible(ctx context.Context, id uuid.UUID) (bool, error) {
	var ok bool
	err := r.s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM perm_visible_pages($1) v
		JOIN pages p ON p.id = v.page_id WHERE v.page_id = $2 AND p.status = 'published')`, r.userID, id).Scan(&ok)
	return ok, err
}

type handler func(ctx context.Context, r reader) (Answer, error)

var handlers map[string]handler

func init() {
	handlers = map[string]handler{
		"child_pages": childPages, "page_tree": pageTree, "space_index": spaceIndex,
		"spaces_list": spacesList, "breadcrumbs": breadcrumbs, "recent_changes": recentChanges,
		"my_favourites": myFavourites, "my_drafts": myDrafts, "my_tasks": myTasks,
		"filter_list": filterList, "report_table": reportTable, "labels_list": labelsList,
		"facts_report": factsReport, "decision_report": decisionReport, "task_report": taskReport,
		document.KindShowExcerpt: showInclude, document.KindEmbeddedPage: showInclude,
		"contributors": contributors, "history_here": historyHere, "attachments_here": attachmentsHere,
		"blog_posts": blogPosts, "link_cards": linkCards,
	}
}

// Kinds lists the kinds this module answers. The others run in the browser.
func Kinds() []string {
	out := make([]string, 0, len(handlers))
	for kind := range handlers {
		out = append(out, kind)
	}
	return out
}

// Answer gives the answer of one block for this reader.
func (s *Service) Answer(ctx context.Context, c identity.Caller, in Request) (Answer, error) {
	run, ok := handlers[in.Kind]
	if !ok {
		return Answer{}, apperr.Invalid("That kind of block has no answer here.")
	}
	r := reader{s: s, userID: c.UserID, pageID: in.PageID, settings: document.CleanSettings(in.Kind, in.Settings)}
	if in.PageID != uuid.Nil {
		// The page it stands on sets the space. A page the reader may not see sets nothing.
		err := s.pool.QueryRow(ctx, `SELECT p.space_id FROM pages p JOIN perm_visible_pages($1) v ON v.page_id = p.id
			WHERE p.id = $2`, c.UserID, in.PageID).Scan(&r.spaceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return Answer{Rows: []Row{}}, nil
		}
		if err != nil {
			return Answer{}, err
		}
	}
	out, err := run(ctx, r)
	if out.Rows == nil {
		out.Rows = []Row{}
	}
	return out, err
}

func scanRows(rows pgx.Rows, err error, scan func(pgx.CollectableRow, *Row) error) ([]Row, error) {
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Row, error) {
		var v Row
		err := scan(row, &v)
		return v, err
	})
	if out == nil {
		out = []Row{}
	}
	return out, err
}

// pageColumns and pageRow read the same page fields in every list.
const pageColumns = `p.id, p.parent_id, p.title, p.emoji, s.key, s.name, p.updated_at, COALESCE(u.display_name, '')`

func pageRow(r pgx.CollectableRow, v *Row) error {
	var at time.Time
	err := r.Scan(&v.ID, &v.ParentID, &v.Title, &v.Emoji, &v.SpaceKey, &v.SpaceName, &at, &v.By)
	v.At = &at
	return err
}
