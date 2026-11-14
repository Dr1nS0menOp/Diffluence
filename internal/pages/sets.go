// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/labels"
	"github.com/diffluence/diffluence/internal/permissions"
)

// The parts of making a set of pages from a template that belong to pages (spec 074).
// The templates module runs them inside one transaction of its own, so the whole set is
// made or nothing is (TPL-120). Rights are checked by the caller, before, for the whole
// set at once (TPL-810); the parent is checked here again under the tree lock.

// LockTree takes the tree lock of a space for the rest of tx.
func (s *Service) LockTree(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID) error {
	return lockTree(ctx, tx, spaceID)
}

// CheckParentForSet makes sure the root of a set may go below parent: the caller may
// create pages there, and parent is a published page of the space.
func (s *Service) CheckParentForSet(ctx context.Context, q db.Querier, c identity.Caller, spaceID, parent uuid.UUID) error {
	return s.checkParent(ctx, q, c, placement{spaceID: spaceID, parent: parent, kind: KindPage})
}

// TemplateText is one version of the text of a template page, as a manifest pins it.
type TemplateText struct {
	PageID uuid.UUID
	Number int
}

// TemplateDocumentsAt reads several pinned texts of templates in one query, with the
// stored size of each. A text that is not there is not found.
func (s *Service) TemplateDocumentsAt(ctx context.Context, q db.Querier, want []TemplateText) (map[TemplateText]document.Node, map[TemplateText]int, error) {
	ids, numbers := make([]uuid.UUID, len(want)), make([]int, len(want))
	for i, w := range want {
		ids[i], numbers[i] = w.PageID, w.Number
	}
	rows, err := q.Query(ctx, `SELECT v.page_id, v.number, v.document FROM unnest($1::uuid[], $2::int[]) AS w (page_id, number)
		JOIN pages p ON p.id = w.page_id AND p.kind = 'template'
		JOIN page_versions v ON v.page_id = w.page_id AND v.number = w.number`, ids, numbers)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	docs, sizes := map[TemplateText]document.Node{}, map[TemplateText]int{}
	for rows.Next() {
		var key TemplateText
		var raw []byte
		if err := rows.Scan(&key.PageID, &key.Number, &raw); err != nil {
			return nil, nil, err
		}
		doc, err := document.Parse(raw)
		if err != nil {
			return nil, nil, err
		}
		docs[key], sizes[key] = doc, len(raw)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(docs) != len(want) {
		return nil, nil, apperr.NotFound()
	}
	return docs, sizes, nil
}

// TemplateHeads gives the current version of the text of each template page.
func (s *Service) TemplateHeads(ctx context.Context, q db.Querier, ids []uuid.UUID) (map[uuid.UUID]int, error) {
	rows, err := q.Query(ctx, "SELECT id, current_version FROM pages WHERE id = ANY($1) AND kind = 'template'", ids)
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID]int{}
	for rows.Next() {
		var id uuid.UUID
		var number int
		if err := rows.Scan(&id, &number); err != nil {
			rows.Close()
			return nil, err
		}
		out[id] = number
	}
	return out, rows.Err()
}

// DeleteTemplatePages removes hidden pages of a template, with their versions and files.
func (s *Service) DeleteTemplatePages(ctx context.Context, tx pgx.Tx, ids []uuid.UUID) error {
	_, err := tx.Exec(ctx, "DELETE FROM pages WHERE id = ANY($1) AND kind = 'template'", ids)
	return err
}

// TitlesTaken lists which of the given titles a page of a space already has (TPL-141).
func (s *Service) TitlesTaken(ctx context.Context, q db.Querier, spaceID uuid.UUID, titles []string) (map[string]bool, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT title FROM pages WHERE space_id = $1 AND title = ANY($2)
		AND kind <> 'template' AND status <> 'trashed'`, spaceID, titles)
	if err != nil {
		return nil, err
	}
	taken, err := pgx.CollectRows(rows, pgx.RowTo[string])
	out := map[string]bool{}
	for _, title := range taken {
		out[title] = true
	}
	return out, err
}

// SpaceHasWorkflow says whether a whole space follows a workflow, which wins over the
// workflow an entry of a template names (DOC-420, TPL-152).
func (s *Service) SpaceHasWorkflow(ctx context.Context, q db.Querier, spaceID uuid.UUID) (bool, error) {
	var has bool
	err := q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM space_workflows WHERE space_id = $1)", spaceID).Scan(&has)
	return has, err
}

// SetPage is one page of a set, with its id made beforehand so that every link of the
// set could point to it before any page existed (TPL-111).
type SetPage struct {
	ID           uuid.UUID
	SpaceID      uuid.UUID
	ParentID     *uuid.UUID
	Title        string
	Folder       bool
	TemplatePage *uuid.UUID
	Document     document.Node
	Labels       []string
	// Publish makes the first version at once (TPL-151); otherwise the page is an
	// unpublished page with a draft of its creator, as one page from a template always was.
	Publish bool
	// WorkflowID puts the page under a workflow of the space, unless the space has one.
	WorkflowID *uuid.UUID
	// Before runs before each step (file, label), so a test can break a set there.
	Before func(step string) error
}

func (in SetPage) before(step string) error {
	if in.Before == nil {
		return nil
	}
	return in.Before(step)
}

// AddSetPage makes one page of a set inside tx, which holds the tree lock of the space.
// It answers with the labels a fixed list of the space left out.
func (s *Service) AddSetPage(ctx context.Context, tx pgx.Tx, c identity.Caller, in SetPage) (Page, []string, error) {
	kind := KindPage
	if in.Folder {
		kind = KindFolder
	}
	p, err := s.insert(ctx, tx, c, newPage{id: in.ID, spaceID: in.SpaceID, parent: in.ParentID, title: in.Title, author: c.UserID, kind: kind})
	if err != nil {
		return p, nil, err
	}
	doc := in.Document
	if in.TemplatePage != nil {
		if err := in.before("file"); err != nil {
			return p, nil, err
		}
		mapping, err := s.attachments.CopyAll(ctx, tx, c, *in.TemplatePage, p.ID, in.SpaceID)
		if err != nil {
			return p, nil, err
		}
		document.RemapAttachments(&doc, mapping)
	}
	if p, err = s.writeSetText(ctx, tx, c, p, doc, in); err != nil {
		return p, nil, err
	}
	if err := in.before("label"); err != nil {
		return p, nil, err
	}
	skipped := []string{}
	for _, label := range in.Labels {
		refused, err := labels.Refused(ctx, tx, in.SpaceID, label)
		if err != nil {
			return p, nil, err
		}
		if refused {
			skipped = append(skipped, label)
			continue
		}
		if err := addLabel(ctx, tx, c, p, label); err != nil {
			return p, nil, err
		}
	}
	if in.WorkflowID != nil {
		if err := s.putNewPageUnderWorkflow(ctx, tx, c, p, *in.WorkflowID); err != nil {
			return p, nil, err
		}
	}
	return p, skipped, nil
}

// writeSetText gives a new page of a set its text: a folder and a page published at once
// get their first version, any other page a draft of its creator.
func (s *Service) writeSetText(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, doc document.Node, in SetPage) (Page, error) {
	if in.Folder {
		return s.appendVersion(ctx, tx, c, p, newVersion{Title: in.Title, Doc: document.Empty(), Author: c.UserID})
	}
	if in.Publish {
		ready, err := document.ForPublish(doc)
		if err != nil {
			return p, err
		}
		if strings.TrimSpace(in.Title) == "" {
			return p, apperr.Invalid("A page published at once needs a title.")
		}
		return s.appendVersion(ctx, tx, c, p, newVersion{Title: in.Title, Doc: ready, Author: c.UserID})
	}
	if err := checkDocumentKind(p.Kind, doc); err != nil {
		return p, err
	}
	raw, err := document.Marshal(doc)
	if err != nil {
		return p, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO drafts (page_id, user_id, title, document, schema_version, base_version)
		VALUES ($1, $2, $3, $4, $5, 0)`, p.ID, c.UserID, in.Title, raw, document.SchemaVersion)
	return p, err
}

// putNewPageUnderWorkflow gives a new page the workflow its entry names (TPL-152). A
// space workflow wins, and then the page already follows that one.
func (s *Service) putNewPageUnderWorkflow(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, workflowID uuid.UUID) error {
	spaceWide, err := s.SpaceHasWorkflow(ctx, tx, p.SpaceID)
	if err != nil || spaceWide {
		return err
	}
	if err := workflowOfSpace(ctx, tx, workflowID, p.SpaceID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO page_workflows (page_id, workflow_id, branch, set_by) VALUES ($1, $2, false, $3)`,
		p.ID, workflowID, c.UserID); err != nil {
		return err
	}
	_, err = s.settle(ctx, tx, c, []uuid.UUID{p.ID}, settleAssigned)
	return err
}

// TrashEmptyPage removes the empty page that was waiting for a template, inside the
// transaction that makes the set (TPL-122). Only an empty, unpublished page of the
// caller's own makes way.
func (s *Service) TrashEmptyPage(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID) error {
	p, err := load(ctx, tx, id)
	if err != nil {
		return err
	}
	if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return err
	}
	var children int
	var draftTitle string
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM pages WHERE parent_id = $1 AND status <> 'trashed'),
			COALESCE(d.title, ''), d.document
		FROM (SELECT 1) one LEFT JOIN drafts d ON d.page_id = $1 AND d.user_id = $2`, id, c.UserID).Scan(&children, &draftTitle, &raw); err != nil {
		return err
	}
	empty := p.CreatedBy == c.UserID && p.Status == StatusDraft && p.CurrentVersion == 0 && children == 0 &&
		strings.TrimSpace(p.Title) == "" && strings.TrimSpace(draftTitle) == ""
	if empty && raw != nil {
		doc, err := document.Parse(raw)
		if err != nil {
			return err
		}
		empty = strings.TrimSpace(document.Text(doc)) == ""
	}
	if !empty {
		return apperr.Conflict("not_empty", "Only an empty page of your own makes way for a template.")
	}
	retention, err := s.settings.TrashRetention(ctx)
	if err != nil {
		return err
	}
	return s.trashTree(ctx, tx, c, p, retention)
}
