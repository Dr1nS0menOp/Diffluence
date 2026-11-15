// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/tasks"
)

// What starting a draft space from a space with content reads and writes of pages (spec 073,
// wave PB4, PUB-1302). The publishing module decides what is copied and how the documents are
// rewritten; these functions do the work inside its transaction. None checks a permission: the
// caller has.

// StartSources reads every page of a space that is not in the trash, of every kind, through q.
// The caller sorts out what a start copies and what it leaves as it is.
func StartSources(ctx context.Context, q db.Querier, spaceID uuid.UUID) ([]Page, error) {
	rows, err := q.Query(ctx, "SELECT "+pageColumns+" FROM pages WHERE space_id = $1 AND status <> 'trashed' ORDER BY position, id", spaceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Page, error) { return scanPage(r) })
}

// HoldForStart takes the rows of every page of a space for the rest of the transaction, so no
// publish, move or rename lands between reading a page and pairing it with its copy: such a write
// waits, and once the start commits it meets the rule of a published space (PUB-1302).
func HoldForStart(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID) error {
	_, err := tx.Exec(ctx, "SELECT id FROM pages WHERE space_id = $1 ORDER BY id FOR SHARE", spaceID)
	return err
}

// ControlledPagesIn counts the pages of a space that follow a workflow. A published space has
// none (PUB-703), so a space with such pages cannot become one by a start.
func ControlledPagesIn(ctx context.Context, q db.Querier, spaceID uuid.UUID) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM page_control pc JOIN pages p ON p.id = pc.page_id
		WHERE p.space_id = $1 AND p.status <> 'trashed'`, spaceID).Scan(&n)
	return n, err
}

// DropTwinTasks takes away the task rows of a page that becomes a twin: a twin keeps none, as
// its tasks are ticked in the draft space (PUB-817). Its text keeps the tasks.
func DropTwinTasks(ctx context.Context, tx pgx.Tx, pageID, spaceID uuid.UUID) error {
	_, err := tasks.Sync(ctx, tx, pageID, spaceID, uuid.Nil, document.Node{Type: document.TypeDoc})
	return err
}

// StartCopy is the copy of one page in the new draft space, before its first version.
type StartCopy struct {
	Page Page
	// Files maps each file of the source to its copy.
	Files map[uuid.UUID]uuid.UUID
}

// CopyRowForStart makes the copy of a page in a draft space, below parent, with the files,
// restrictions, labels and look of its source but no version yet: the version follows once every
// copy exists, so links can lead from one copy to another (PUB-1310).
func (s *Service) CopyRowForStart(ctx context.Context, tx pgx.Tx, c identity.Caller, src Page, spaceID uuid.UUID, parent *uuid.UUID) (StartCopy, error) {
	n, err := s.insert(ctx, tx, c, newPage{spaceID: spaceID, parent: parent, title: src.Title, author: c.UserID, kind: src.Kind, link: src.LinkTarget})
	if err != nil {
		return StartCopy{}, err
	}
	return s.CopyIntoForStart(ctx, tx, c, src, n)
}

// CopyIntoForStart gives an existing page of a draft space, such as its home page, the files,
// restrictions, labels and look of a source page. The comments, watchers and history of the
// source stay behind (PUB-1302, PUB-1405, PUB-1406).
func (s *Service) CopyIntoForStart(ctx context.Context, tx pgx.Tx, c identity.Caller, src, target Page) (StartCopy, error) {
	files, err := s.attachments.CopyAll(ctx, tx, c, src.ID, target.ID, target.SpaceID)
	if err != nil {
		return StartCopy{}, err
	}
	if err := copyRestrictions(ctx, tx, src.ID, target.ID); err != nil {
		return StartCopy{}, err
	}
	if err := copyLabels(ctx, tx, c, src.ID, target.ID); err != nil {
		return StartCopy{}, err
	}
	look := src.Look
	look.HeaderAttachmentID = nil
	if h := src.HeaderAttachmentID; h != nil {
		if copied, ok := files[*h]; ok {
			look.HeaderAttachmentID = &copied
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE pages SET emoji = $2, full_width = $3, header_attachment_id = $4, header_position = $5, language = $6,
		title_align = $7, hide_children = $8 WHERE id = $1`, target.ID, look.Emoji, look.FullWidth, look.HeaderAttachmentID,
		look.HeaderPosition, look.Language, look.TitleAlign, look.HideChildren); err != nil {
		return StartCopy{}, err
	}
	out, err := load(ctx, tx, target.ID)
	return StartCopy{Page: out, Files: files}, err
}

// WriteStartVersion writes the version of a copy that a start made: the text of its source,
// with links that lead to the copies. It tells nobody: the page is new to the draft space, and
// its readers had the text already (PUB-1302).
func (s *Service) WriteStartVersion(ctx context.Context, tx pgx.Tx, c identity.Caller, pageID uuid.UUID, title string, doc document.Node) (Page, error) {
	p, err := load(ctx, tx, pageID)
	if err != nil {
		return Page{}, err
	}
	return s.appendVersion(ctx, tx, c, p, newVersion{Title: title, Doc: doc, Author: c.UserID, Silent: true})
}
