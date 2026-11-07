// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

const (
	// branchRows is how much of one branch a request brings. A level with more says so
	// in More, and the reader asks for the rest.
	branchRows = 200
	// firstRows bounds the opening of a space: levels are added while they fit, so a
	// small space opens as it always did and a very large one opens at once.
	firstRows = 1000
	// firstLevels matches the levels a tree shows open at first.
	firstLevels = 3
	// maxBranches bounds the branches of one request.
	maxBranches = 50
	// foundRows bounds what a filter or a flat view shows.
	foundRows = 200
)

// Views of a space as a flat list, the way the tree offers them.
const (
	ViewVisited  = "visited"
	ViewUpdated  = "updated"
	ViewAlphabet = "alphabet"
)

// BranchMore says that a level has more rows, and where they start. A nil parent is
// the top level.
type BranchMore struct {
	ParentID *uuid.UUID `json:"parent_id"`
	Offset   int        `json:"offset"`
}

// Branches is a part of the tree: the rows, and the levels that go on.
type Branches struct {
	Pages []Node       `json:"pages"`
	More  []BranchMore `json:"more"`
}

// Found is what a filter or a flat view gives: the rows that match, in order, and the
// rows above them, so a match keeps its place in the tree.
type Found struct {
	Pages   []Node      `json:"pages"`
	Matches []uuid.UUID `json:"matches"`
	Capped  bool        `json:"capped"`
}

// listed says which rows a person gets, without asking perm_listed_pages for each of them:
// that works out the hidden pages again every time, and a level has many rows. It picks
// rows and says whether a branch goes on. What is shown passes the rule itself, in rowsByID.
func listed(alias string) string {
	return alias + `.space_id = @space AND ` + alias + `.kind = ANY(@kinds) AND ` + alias + `.archived_at IS NULL
		AND (` + alias + `.status = 'published' OR (` + alias + `.status = 'draft' AND ` + alias + `.created_by = @user))
		AND ` + alias + `.id NOT IN (SELECT unnest(@hidden::uuid[]))`
}

// topParents are the pages whose children stand at the top of the tree: the home page,
// which has its own row above the tree, and a parent this person gets no row for
// because it is archived, in the trash or someone else's draft. A hidden parent is
// not one of them: what is below it is hidden too.
const topParents = `SELECT s.home_page_id FROM spaces s WHERE s.id = @space
	UNION ALL
	SELECT up.id FROM pages up WHERE up.space_id = @space
		AND (up.archived_at IS NOT NULL OR up.status <> 'published' OR up.kind NOT IN ('page', 'folder', 'link', 'database'))
		AND NOT (up.status = 'draft' AND up.created_by = @user AND up.archived_at IS NULL)`

var (
	topLevel = `SELECT p.id, p.parent_id FROM pages p WHERE ` + listed("p") + `
		AND (p.parent_id IS NULL OR p.parent_id IN (` + topParents + `))
		ORDER BY p.parent_id IS NULL, p.position, p.id OFFSET @offset LIMIT @rows`

	childLevel = `SELECT level.id, level.parent_id FROM (
			SELECT p.id, p.parent_id, row_number() OVER (PARTITION BY p.parent_id ORDER BY p.position, p.id) AS place
			FROM pages p WHERE ` + listed("p") + ` AND p.parent_id = ANY(@parents)
		) level WHERE level.place > @offset AND level.place <= @offset + @rows ORDER BY level.parent_id, level.place LIMIT @most`

	// rowsByID reads what is shown. perm_listed_pages is the rule, and it looks at these
	// rows only because they are named inside the subquery.
	rowsByID = `SELECT p.id, p.parent_id, p.position, p.title, p.status, p.emoji,
			EXISTS (SELECT 1 FROM page_restrictions r WHERE r.page_id = p.id),
			d.page_id IS NOT NULL, p.updated_at, COALESCE(u.display_name, ''), rp.viewed_at,
			p.kind, p.link_target, COALESCE(o.display_name, ''), p.stage_text, p.stage_color,
			EXISTS (SELECT 1 FROM pages c WHERE c.parent_id = p.id AND ` + listed("c") + `)
		FROM pages p
		JOIN (SELECT v.page_id FROM perm_listed_pages(@user) v WHERE v.page_id = ANY(@ids)) v ON v.page_id = p.id
		LEFT JOIN users u ON u.id = p.updated_by
		LEFT JOIN users o ON o.id = COALESCE(p.owner_id, p.created_by)
		LEFT JOIN drafts d ON d.page_id = p.id AND d.user_id = @user
		LEFT JOIN recent_pages rp ON rp.page_id = p.id AND rp.user_id = @user
		WHERE p.space_id = @space AND p.kind = ANY(@kinds)`

	foundRowsSQL = `SELECT p.id FROM pages p
		LEFT JOIN recent_pages rp ON rp.page_id = p.id AND rp.user_id = @user
		WHERE ` + listed("p") + `
		  AND (@text = '' OR position(lower(@text) IN lower(p.title)) > 0)
		  AND (@view <> '` + ViewVisited + `' OR rp.viewed_at IS NOT NULL)
		ORDER BY CASE WHEN @view = '` + ViewVisited + `' THEN rp.viewed_at END DESC NULLS LAST,
			CASE WHEN @view = '` + ViewUpdated + `' THEN p.updated_at END DESC NULLS LAST,
			p.title, p.id LIMIT @rows`
)

func scanNode(r pgx.CollectableRow) (Node, error) {
	var n Node
	err := r.Scan(&n.ID, &n.ParentID, &n.Position, &n.Title, &n.Status, &n.Emoji,
		&n.Restricted, &n.HasDraft, &n.UpdatedAt, &n.UpdatedByName, &n.VisitedAt,
		&n.Kind, &n.LinkTarget, &n.OwnerName, &n.StageText, &n.StageColor, &n.HasChildren)
	return n, err
}

// treeRead is one request for a part of a tree. It holds what every step uses.
type treeRead struct {
	s    *Service
	args pgx.NamedArgs
}

// startRead checks the space, and works out the hidden pages once for the whole request.
func (s *Service) startRead(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (*treeRead, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceView); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT h.page_id FROM perm_hidden_pages($1) h", c.UserID)
	if err != nil {
		return nil, err
	}
	hidden, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	return &treeRead{s: s, args: pgx.NamedArgs{"user": c.UserID, "space": spaceID, "kinds": treeKinds, "hidden": hidden}}, nil
}

// picked is a row of a level, before it is read in full.
type picked struct {
	ID     uuid.UUID
	Parent *uuid.UUID
}

func (t *treeRead) pick(ctx context.Context, sql string, extra pgx.NamedArgs) ([]picked, error) {
	args := pgx.NamedArgs{}
	for k, v := range t.args {
		args[k] = v
	}
	for k, v := range extra {
		args[k] = v
	}
	rows, err := t.s.pool.Query(ctx, sql, args)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (picked, error) {
		var p picked
		err := r.Scan(&p.ID, &p.Parent)
		return p, err
	})
}

// rows reads picked rows in full, through the rule.
func (t *treeRead) rows(ctx context.Context, ids []uuid.UUID) ([]Node, error) {
	if len(ids) == 0 {
		return []Node{}, nil
	}
	args := pgx.NamedArgs{"ids": ids}
	for k, v := range t.args {
		args[k] = v
	}
	rows, err := t.s.pool.Query(ctx, rowsByID, args)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanNode)
}

// top reads a part of the top level. One row past the end shows whether it goes on.
func (t *treeRead) top(ctx context.Context, offset int) (Branches, error) {
	list, err := t.pick(ctx, topLevel, pgx.NamedArgs{"offset": offset, "rows": branchRows + 1})
	if err != nil {
		return Branches{}, err
	}
	out := Branches{More: []BranchMore{}}
	if len(list) > branchRows {
		list = list[:branchRows]
		out.More = append(out.More, BranchMore{Offset: offset + branchRows})
	}
	out.Pages, err = t.rows(ctx, idsOf(list))
	return out, err
}

func idsOf(list []picked) []uuid.UUID {
	out := make([]uuid.UUID, len(list))
	for i, p := range list {
		out[i] = p.ID
	}
	return out
}

// children reads a part of each branch, and cuts each one at branchRows.
func (t *treeRead) children(ctx context.Context, parents []uuid.UUID, offset int) (Branches, error) {
	list, err := t.pick(ctx, childLevel, pgx.NamedArgs{"parents": parents, "offset": offset, "rows": branchRows + 1, "most": len(parents) * (branchRows + 1)})
	if err != nil {
		return Branches{}, err
	}
	return t.keep(ctx, list, offset)
}

// keep cuts each branch at branchRows, and reads what stays.
func (t *treeRead) keep(ctx context.Context, list []picked, offset int) (Branches, error) {
	out := Branches{More: []BranchMore{}}
	kept := make([]uuid.UUID, 0, len(list))
	seen := map[uuid.UUID]int{}
	for _, row := range list {
		seen[*row.Parent]++
		if seen[*row.Parent] <= branchRows {
			kept = append(kept, row.ID)
			continue
		}
		out.More = append(out.More, BranchMore{ParentID: row.Parent, Offset: offset + branchRows})
	}
	var err error
	out.Pages, err = t.rows(ctx, kept)
	return out, err
}

// nextLevel reads the level below these rows, if all of it fits in room.
func (t *treeRead) nextLevel(ctx context.Context, parents []uuid.UUID, room int) (Branches, bool, error) {
	list, err := t.pick(ctx, childLevel, pgx.NamedArgs{"parents": parents, "offset": 0, "rows": branchRows + 1, "most": room + 1})
	if err != nil || len(list) > room {
		return Branches{}, false, err
	}
	next, err := t.keep(ctx, list, 0)
	return next, err == nil, err
}

func parentsOf(list []Node) []uuid.UUID {
	out := []uuid.UUID{}
	for _, n := range list {
		if n.HasChildren {
			out = append(out, n.ID)
		}
	}
	return out
}

// TreeStart is the opening of a space: the top level, and the levels below it as long
// as they fit in firstRows. A level that does not fit stays closed until it is asked for.
func (s *Service) TreeStart(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (Branches, error) {
	t, err := s.startRead(ctx, c, spaceID)
	if err != nil {
		return Branches{}, err
	}
	out, err := t.top(ctx, 0)
	if err != nil {
		return Branches{}, err
	}
	level := out.Pages
	for depth := 1; depth < firstLevels && len(out.More) == 0; depth++ {
		parents := parentsOf(level)
		if len(parents) == 0 || len(out.Pages)+len(parents) > firstRows {
			break
		}
		next, fits, err := t.nextLevel(ctx, parents, firstRows-len(out.Pages))
		if err != nil {
			return Branches{}, err
		}
		if !fits {
			break
		}
		out.Pages = append(out.Pages, next.Pages...)
		out.More = append(out.More, next.More...)
		level = next.Pages
	}
	return out, nil
}

// TreeBranches reads the children of branches that were opened. No parent asks for the
// rest of the top level.
func (s *Service) TreeBranches(ctx context.Context, c identity.Caller, spaceID uuid.UUID, parents []uuid.UUID, offset int) (Branches, error) {
	t, err := s.startRead(ctx, c, spaceID)
	if err != nil {
		return Branches{}, err
	}
	if len(parents) == 0 {
		return t.top(ctx, offset)
	}
	return t.children(ctx, parents[:min(len(parents), maxBranches)], offset)
}

// TreeAround gives what it takes to show one page: the way to it, and the level of
// each step. A page this person may not see gives nothing.
func (s *Service) TreeAround(ctx context.Context, c identity.Caller, spaceID, pageID uuid.UUID) (Branches, error) {
	empty := Branches{Pages: []Node{}, More: []BranchMore{}}
	t, err := s.startRead(ctx, c, spaceID)
	if err != nil {
		return empty, err
	}
	way, err := s.ancestors(ctx, []uuid.UUID{pageID})
	if err != nil {
		return empty, err
	}
	steps, err := t.rows(ctx, append(way, pageID))
	if err != nil {
		return empty, err
	}
	if !slices.ContainsFunc(steps, func(n Node) bool { return n.ID == pageID }) {
		return empty, nil
	}
	out, err := t.children(ctx, append(way, pageID), 0)
	if err != nil {
		return empty, err
	}
	out.Pages = append(out.Pages, steps...)
	return out, nil
}

// ancestors gives the pages above these.
func (s *Service) ancestors(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ancestor_id FROM page_tree WHERE descendant_id = ANY($1) AND depth > 0`, ids)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// TreeFind filters the pages of a space on their title, or lists them in another
// order. Matches are worked out here because the tree may hold only a part of the
// space. A hidden title neither shows nor counts.
func (s *Service) TreeFind(ctx context.Context, c identity.Caller, spaceID uuid.UUID, text, view string) (Found, error) {
	t, err := s.startRead(ctx, c, spaceID)
	if err != nil {
		return Found{}, err
	}
	args := pgx.NamedArgs{"text": text, "view": view, "rows": foundRows + 1}
	for k, v := range t.args {
		args[k] = v
	}
	rows, err := s.pool.Query(ctx, foundRowsSQL, args)
	if err != nil {
		return Found{}, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return Found{}, err
	}
	out := Found{Pages: []Node{}, Matches: ids, Capped: len(ids) > foundRows}
	if out.Capped {
		out.Matches = ids[:foundRows]
	}
	way, err := s.ancestors(ctx, out.Matches)
	if err != nil {
		return out, err
	}
	out.Pages, err = t.rows(ctx, append(way, out.Matches...))
	return out, err
}
