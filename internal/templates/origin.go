// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/pages"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// Where a page came from, and what a version made (spec 074, wave GT5, TPL-611 to TPL-613).

// maxListedSets bounds the sets of a version one list shows; the count covers them all.
const maxListedSets = 100

// maxSetPages is the most pages a set holds (TPL-102), so the other pages of a set fit in
// one question to the pages module.
const maxSetPages = maxEntries

// Origin is where a page came from, as its Details panel says it (TPL-611). A reader who may
// not see the template learns only that the page was made from one (TPL-504).
type Origin struct {
	// Made says the page was made from a template; pages made before versions have no origin.
	Made   bool      `json:"made"`
	MadeAt time.Time `json:"made_at"`
	// Named says the reader may see the template, so its name and version are given.
	Named      bool       `json:"named"`
	TemplateID *uuid.UUID `json:"template_id,omitempty"`
	Name       string     `json:"name,omitempty"`
	Version    int        `json:"version,omitempty"`
	// Deleted says the template is gone; the page keeps its origin (TPL-601).
	Deleted bool `json:"deleted"`
	// History says the reader may open the template's history.
	History bool `json:"history"`
	// Newer says a newer version is in use now, for one who may edit the page (TPL-612). The
	// page never follows it.
	Newer bool `json:"newer"`
	// Set lists the other pages of the same set that the reader may see.
	Set []pages.PageRef `json:"set"`
}

// Origin tells a reader of a page where it came from.
func (s *Service) Origin(ctx context.Context, c identity.Caller, pageID uuid.UUID) (Origin, error) {
	out := Origin{Set: []pages.PageRef{}}
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return out, err
	}
	var setID uuid.UUID
	var templateID *uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT set_id, template_id, version, created_at FROM page_template_origin WHERE page_id = $1`,
		pageID).Scan(&setID, &templateID, &out.Version, &out.MadeAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Origin{Set: []pages.PageRef{}}, nil
	}
	if err != nil {
		return out, err
	}
	out.Made, out.Deleted = true, templateID == nil
	if err := s.nameOrigin(ctx, c, pageID, templateID, &out); err != nil {
		return out, err
	}
	if !out.Named {
		out.Version = 0
	}
	out.Set, err = s.otherPagesOfSet(ctx, c, setID, pageID)
	return out, err
}

// nameOrigin fills in the template of an origin for a reader who may see it.
func (s *Service) nameOrigin(ctx context.Context, c identity.Caller, pageID uuid.UUID, templateID *uuid.UUID, out *Origin) error {
	if templateID == nil {
		return nil
	}
	t, err := load(ctx, s.pool, c.UserID, *templateID)
	if err != nil {
		return err
	}
	st, err := s.standingOf(ctx, s.pool, c, t)
	if err != nil {
		return err
	}
	seen, err := s.maySee(ctx, s.pool, st)
	if err != nil || !seen {
		return err
	}
	out.Named, out.TemplateID, out.Name, out.History = true, templateID, t.Name, st.mayLook()
	if t.TemplateVersion <= out.Version {
		return nil
	}
	edit, err := s.perms.CheckPage(ctx, s.pool, c.UserID, pageID, permissions.PageEdit, uuid.Nil)
	out.Newer = err == nil && edit.Allowed
	return err
}

// otherPagesOfSet lists the pages a set made beside this one that the reader may see.
func (s *Service) otherPagesOfSet(ctx context.Context, c identity.Caller, setID, pageID uuid.UUID) ([]pages.PageRef, error) {
	rows, err := s.pool.Query(ctx, `SELECT page_id FROM page_template_origin WHERE set_id = $1 AND page_id <> $2 LIMIT $3`,
		setID, pageID, maxSetPages)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil || len(ids) == 0 {
		return []pages.PageRef{}, err
	}
	return s.pages.RefsOf(ctx, c, ids)
}

// SetRow is one set a version made, as its managers see it: its first page only when they may
// see that page (TPL-613).
type SetRow struct {
	ID        uuid.UUID      `json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	Pages     int            `json:"pages"`
	Root      *pages.PageRef `json:"root"`
}

// VersionSets are the sets one version made: how many in all, the ones whose first page the
// manager may see, and how many others there are, as a number only (TPL-613).
type VersionSets struct {
	Version int      `json:"version"`
	Count   int      `json:"count"`
	Sets    []SetRow `json:"sets"`
	Hidden  int      `json:"hidden"`
}

// SetsOf lists the sets a version of a template made, for those who manage it.
func (s *Service) SetsOf(ctx context.Context, c identity.Caller, id uuid.UUID, number int) (VersionSets, error) {
	out := VersionSets{Version: number, Sets: []SetRow{}}
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return out, err
	}
	if _, err := s.requireTo(ctx, s.pool, c, t, standing.manages); err != nil {
		return out, err
	}
	if _, err := versionAt(ctx, s.pool, id, number); err != nil {
		return out, err
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM template_sets WHERE template_id = $1 AND version = $2", id, number).
		Scan(&out.Count); err != nil {
		return out, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id, created_at, page_count, root_page_id FROM template_sets
		WHERE template_id = $1 AND version = $2 ORDER BY created_at DESC, id DESC LIMIT $3`, id, number, maxListedSets)
	if err != nil {
		return out, err
	}
	type found struct {
		row  SetRow
		root *uuid.UUID
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (found, error) {
		var f found
		err := r.Scan(&f.row.ID, &f.row.CreatedAt, &f.row.Pages, &f.root)
		return f, err
	})
	if err != nil {
		return out, err
	}
	roots := []uuid.UUID{}
	for _, f := range list {
		if f.root != nil {
			roots = append(roots, *f.root)
		}
	}
	refs, err := s.pages.RefsOf(ctx, c, roots)
	if err != nil {
		return out, err
	}
	seen := map[uuid.UUID]pages.PageRef{}
	for _, ref := range refs {
		seen[ref.ID] = ref
	}
	for _, f := range list {
		ref, ok := pages.PageRef{}, false
		if f.root != nil {
			ref, ok = seen[*f.root]
		}
		if !ok {
			continue
		}
		f.row.Root = &ref
		out.Sets = append(out.Sets, f.row)
	}
	// Every set not listed is only counted: its first page is closed to the manager, gone, or
	// past the length of the list.
	out.Hidden = out.Count - len(out.Sets)
	return out, nil
}

// maxNamedTemplates bounds the templates one search filter stands for.
const maxNamedTemplates = 50

// Named gives the templates whose name is one of names, ignoring capitals, that the reader may
// see: the filter template: of search finds pages made from them (TPL-703). A name the reader
// may not see stands for nothing, so the filter tells nothing about it (TPL-504).
func (s *Service) Named(ctx context.Context, c identity.Caller, names []string) ([]uuid.UUID, error) {
	lower := make([]string, 0, len(names))
	for _, n := range names {
		lower = append(lower, strings.ToLower(strings.TrimSpace(n)))
	}
	rows, err := s.pool.Query(ctx, "SELECT "+columns+" "+joins+` WHERE lower(t.name) = ANY($2) ORDER BY t.id LIMIT $3`,
		c.UserID, lower, maxNamedTemplates)
	list, err := collect(rows, err)
	if err != nil {
		return nil, err
	}
	out := []uuid.UUID{}
	for _, t := range list {
		st, err := s.standingOf(ctx, s.pool, c, t)
		if err != nil {
			return nil, err
		}
		seen, err := s.maySee(ctx, s.pool, st)
		if err != nil {
			return nil, err
		}
		if seen {
			out = append(out, t.ID)
		}
	}
	return out, nil
}

// setCounts counts the sets each version of a template made, for its managers (TPL-613).
func (s *Service) setCounts(ctx context.Context, id uuid.UUID) (map[int]int, error) {
	rows, err := s.pool.Query(ctx, "SELECT version, count(*) FROM template_sets WHERE template_id = $1 GROUP BY version", id)
	if err != nil {
		return nil, err
	}
	out := map[int]int{}
	for rows.Next() {
		var version, n int
		if err := rows.Scan(&version, &n); err != nil {
			rows.Close()
			return nil, err
		}
		out[version] = n
	}
	rows.Close()
	return out, rows.Err()
}
