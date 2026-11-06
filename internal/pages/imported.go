// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/search"
	"github.com/diffluence/diffluence/internal/tasks"
)

// ImportedVersion is a version from a bundle. Its author and date come from
// another instance, and nobody here can vouch for them (SEC-353).
type ImportedVersion struct {
	Number    int             `json:"number"`
	Title     string          `json:"title"`
	Document  json.RawMessage `json:"document"`
	AuthorID  uuid.UUID       `json:"author_id"`
	Summary   string          `json:"summary"`
	CreatedAt time.Time       `json:"created_at"`
}

// ImportedPage is a page from a bundle, parents first.
type ImportedPage struct {
	ID        uuid.UUID         `json:"id"`
	Origin    uuid.UUID         `json:"origin_instance_id"`
	ParentID  *uuid.UUID        `json:"parent_id"`
	Position  int               `json:"position"`
	Title     string            `json:"title"`
	Kind      string            `json:"kind"`
	CreatedBy uuid.UUID         `json:"created_by"`
	CreatedAt time.Time         `json:"created_at"`
	Versions  []ImportedVersion `json:"versions"`
}

// BundleVersions reads every version of a page, for a bundle.
func (s *Service) BundleVersions(ctx context.Context, q pgx.Tx, id uuid.UUID) ([]ImportedVersion, error) {
	rows, err := q.Query(ctx, `SELECT number, title, document, author_id, summary, created_at FROM page_versions WHERE page_id = $1 ORDER BY number`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ImportedVersion, error) {
		var v ImportedVersion
		err := r.Scan(&v.Number, &v.Title, &v.Document, &v.AuthorID, &v.Summary, &v.CreatedAt)
		return v, err
	})
}

// InsertImported writes a page with its history, within the transaction of the
// import. Every version passes the same gate as a publish, because a signed bundle
// only proves who made it, not that it is harmless.
func (s *Service) InsertImported(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID, in ImportedPage) error {
	if len(in.Versions) == 0 {
		return apperr.Invalid("A page in the bundle has no versions.")
	}
	last := in.Versions[len(in.Versions)-1]
	kind := in.Kind
	if !isKnownKind(kind) {
		kind = KindPage
	}
	if _, err := tx.Exec(ctx, `INSERT INTO pages (id, origin_instance_id, space_id, parent_id, position, title, status, current_version,
			created_by, updated_by, owner_id, kind, created_at, updated_at, published_at)
		VALUES ($1, $2, $3, $4, $5, $6, 'published', $7, $8, $9, $8, $10, $11, $12, $11)`,
		in.ID, in.Origin, spaceID, in.ParentID, in.Position, last.Title, last.Number, in.CreatedBy, last.AuthorID, kind, in.CreatedAt, last.CreatedAt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO page_tree (ancestor_id, descendant_id, depth)
		SELECT ancestor_id, $2::uuid, depth + 1 FROM page_tree WHERE descendant_id = $1
		UNION ALL SELECT $2::uuid, $2::uuid, 0`, in.ParentID, in.ID); err != nil {
		return err
	}
	var current document.Node
	for _, v := range in.Versions {
		doc, err := document.Normalize(v.Document)
		if err != nil {
			return err
		}
		current = doc
		if v.Number == last.Number {
			if err := tasks.AssignIDs(ctx, tx, in.ID, &current); err != nil {
				return err
			}
		}
		raw, err := document.Marshal(current)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO page_versions (id, origin_instance_id, page_id, number, title, document, schema_version,
				author_id, summary, created_at, imported)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, true)`, ids.New(), in.Origin, in.ID, v.Number, v.Title, raw,
			document.SchemaVersion, v.AuthorID, v.Summary, v.CreatedAt); err != nil {
			return err
		}
	}
	if kind == KindTemplate {
		return nil
	}
	if err := search.Index(ctx, tx, in.ID, last.Title, document.Text(current)); err != nil {
		return err
	}
	if err := syncLinks(ctx, tx, in.ID, current); err != nil {
		return err
	}
	if _, _, err := syncMentions(ctx, tx, in.ID, current); err != nil {
		return err
	}
	_, err := tasks.Sync(ctx, tx, in.ID, spaceID, last.AuthorID, current)
	return err
}

func isKnownKind(kind string) bool {
	switch kind {
	case KindPage, KindPost, KindFolder, KindLink, KindTemplate:
		return true
	}
	return false
}
