// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/search"
)

// What a release of draft and published spaces reads and writes of pages (spec 073). The
// publishing module decides; these functions do the work inside its transaction, so no other
// module names the tables of pages. None of them checks a permission: the caller has.

// ReleaseSource is a draft page as a release reads it: its row, its labels and its
// ancestors, nearest first.
type ReleaseSource struct {
	Page
	Labels    []string
	Ancestors []uuid.UUID
}

// SourceForRelease reads a page for a release, through q.
func SourceForRelease(ctx context.Context, q db.Querier, id uuid.UUID) (ReleaseSource, error) {
	p, err := load(ctx, q, id)
	if err != nil {
		return ReleaseSource{}, err
	}
	out := ReleaseSource{Page: p}
	if err := q.QueryRow(ctx, `SELECT COALESCE(array_agg(label ORDER BY label), '{}') FROM page_labels WHERE page_id = $1`, id).
		Scan(&out.Labels); err != nil {
		return out, err
	}
	rows, err := q.Query(ctx, `SELECT ancestor_id FROM page_tree WHERE descendant_id = $1 AND depth > 0 ORDER BY depth`, id)
	if err != nil {
		return out, err
	}
	out.Ancestors, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	return out, err
}

// ReleasedVersion is one version of a page as a release takes it.
type ReleasedVersion struct {
	Title string
	Doc   document.Node
	// Summary is what its author wrote about the version. It stays in the draft space
	// (PUB-1408).
	Summary string
	// Hash is the content hash of the stored document (page_pair_content_hash in SQL).
	Hash string
}

// VersionForRelease reads one version of a page, through q.
func VersionForRelease(ctx context.Context, q db.Querier, pageID uuid.UUID, number int) (ReleasedVersion, error) {
	var v ReleasedVersion
	var raw []byte
	err := q.QueryRow(ctx, `SELECT title, document, page_pair_content_hash(document), summary FROM page_versions WHERE page_id = $1 AND number = $2`,
		pageID, number).Scan(&v.Title, &raw, &v.Hash, &v.Summary)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, apperr.NotFound()
	}
	if err != nil {
		return v, err
	}
	v.Doc, err = document.Parse(raw)
	return v, err
}

// LinkTarget is what a release needs to know of a page a document links to.
type LinkTarget struct {
	SpaceID uuid.UUID
	Title   string
	Kind    string
}

// ReleaseTargets reads the pages among list that exist, through q.
func ReleaseTargets(ctx context.Context, q db.Querier, list []uuid.UUID) (map[uuid.UUID]LinkTarget, error) {
	out := map[uuid.UUID]LinkTarget{}
	if len(list) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT id, space_id, title, kind FROM pages WHERE id = ANY($1)`, list)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var t LinkTarget
		if err := rows.Scan(&id, &t.SpaceID, &t.Title, &t.Kind); err != nil {
			return nil, err
		}
		out[id] = t
	}
	return out, rows.Err()
}

// ContentInTheWay says what of a space stands in the way of it becoming a published space,
// which must hold nothing but a home page with one version (PUB-211), as counts by kind.
func ContentInTheWay(ctx context.Context, q db.Querier, spaceID uuid.UUID, home *uuid.UUID) (map[string]int, error) {
	var pages, posts, others, homeVersions int
	err := q.QueryRow(ctx, `SELECT
			count(*) FILTER (WHERE kind = 'page' AND id IS DISTINCT FROM $2),
			count(*) FILTER (WHERE kind = 'post'),
			count(*) FILTER (WHERE kind NOT IN ('page', 'post', 'template')),
			COALESCE((SELECT current_version FROM pages WHERE id = $2), 0)
		FROM pages WHERE space_id = $1 AND status <> 'trashed'`, spaceID, home).Scan(&pages, &posts, &others, &homeVersions)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for key, n := range map[string]int{"pages": pages, "posts": posts, "other_items": others} {
		if n > 0 {
			out[key] = n
		}
	}
	if homeVersions > 1 {
		out["home_versions"] = homeVersions
	}
	return out, nil
}

// MakeTwin adds the row of a twin in a published space, with no version yet: a page with no
// version is seen by nobody but its creator until the release writes one in the same
// transaction (PUB-410). The caller has marked its transaction as a release.
func (s *Service) MakeTwin(ctx context.Context, tx pgx.Tx, c identity.Caller, spaceID uuid.UUID, parent *uuid.UUID, src Page) (Page, error) {
	if err := lockTree(ctx, tx, spaceID); err != nil {
		return Page{}, err
	}
	return s.insert(ctx, tx, c, newPage{spaceID: spaceID, parent: parent, title: src.Title, author: c.UserID, kind: src.Kind, link: src.LinkTarget})
}

// TwinVersion is the version a release writes on a twin: the releaser is its author and the
// release note its summary, never the draft's (PUB-332, PUB-1408).
type TwinVersion struct {
	Title   string
	Doc     document.Node
	Summary string
	Silent  bool
}

// TwinWrite is everything a release changes on a twin.
type TwinWrite struct {
	ID     uuid.UUID
	Parent *uuid.UUID
	// Version is nil when only the place, the look or the labels changed: they make no
	// version (PUB-411).
	Version *TwinVersion
	Look    Look
	Labels  []string
}

// WriteTwin writes a release onto a twin. The caller has marked its transaction as a release
// and checked the rights; the twin is a page of the published space.
func (s *Service) WriteTwin(ctx context.Context, tx pgx.Tx, c identity.Caller, in TwinWrite) (Page, error) {
	p, err := load(ctx, tx, in.ID)
	if err != nil {
		return Page{}, err
	}
	if err := lockTree(ctx, tx, p.SpaceID); err != nil {
		return Page{}, err
	}
	if !sameParent(p.ParentID, in.Parent) {
		if in.Parent != nil {
			tree, err := subtree(ctx, tx, p.ID)
			if err != nil {
				return Page{}, err
			}
			// A twin's parent is always the twin of a draft ancestor, so this cannot happen;
			// it is checked anyway, as a cycle would break the tree for every reader (PUB-1006).
			if slices.Contains(tree, *in.Parent) {
				return Page{}, errors.New("pages: a release would put a twin below itself")
			}
		}
		if err := relink(ctx, tx, p.ID, in.Parent); err != nil {
			return Page{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE pages SET position = (SELECT COALESCE(max(position) + 1, 0) FROM pages
			WHERE space_id = $2 AND parent_id IS NOT DISTINCT FROM $3 AND id <> $1 AND kind = ANY($4)) WHERE id = $1`,
			p.ID, p.SpaceID, in.Parent, treeKinds); err != nil {
			return Page{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE pages SET emoji = $2, full_width = $3, header_attachment_id = $4, header_position = $5, language = $6
		WHERE id = $1`, p.ID, in.Look.Emoji, in.Look.FullWidth, in.Look.HeaderAttachmentID, in.Look.HeaderPosition, in.Look.Language); err != nil {
		return Page{}, err
	}
	if err := setReleasedLabels(ctx, tx, c, p.ID, in.Labels); err != nil {
		return Page{}, err
	}
	if in.Version != nil {
		v := in.Version
		if p, err = s.appendVersion(ctx, tx, c, p, newVersion{Title: v.Title, Doc: v.Doc, Author: c.UserID, Summary: v.Summary, Silent: v.Silent}); err != nil {
			return Page{}, err
		}
	}
	return load(ctx, tx, in.ID)
}

func sameParent(a, b *uuid.UUID) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// setReleasedLabels gives a twin exactly the labels of its draft (PUB-304, PUB-1407).
func setReleasedLabels(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID, labels []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM page_labels WHERE page_id = $1 AND NOT (label = ANY($2))`, id, labels); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO page_labels (page_id, label, created_by) SELECT $1, l, $3 FROM unnest($2::text[]) AS l
		ON CONFLICT DO NOTHING`, id, labels, c.UserID); err != nil {
		return err
	}
	return search.RefreshNames(ctx, tx, id)
}
