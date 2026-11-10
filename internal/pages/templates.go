// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/labels"
	"github.com/diffluence/diffluence/internal/permissions"
)

// These calls are unchecked on purpose. Evaluate refuses a template to everyone,
// and the templates module holds the rule of who may use one (spec 011, D6).

// NewTemplatePage adds the hidden page behind a template, inside tx.
func (s *Service) NewTemplatePage(ctx context.Context, tx pgx.Tx, c identity.Caller, spaceID uuid.UUID, name string, doc document.Node) (Page, error) {
	p, err := s.insert(ctx, tx, c, newPage{spaceID: spaceID, title: name, author: c.UserID, kind: KindTemplate})
	if err != nil {
		return p, err
	}
	return s.appendVersion(ctx, tx, c, p, newVersion{Title: name, Doc: doc, Author: c.UserID})
}

// NewTemplateVersion saves new text for a template, inside tx.
func (s *Service) NewTemplateVersion(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID, name string, doc document.Node) error {
	p, err := lockPage(ctx, tx, id)
	if err != nil {
		return err
	}
	if p.Kind != KindTemplate {
		return apperr.NotFound()
	}
	_, err = s.appendVersion(ctx, tx, c, p, newVersion{Title: name, Doc: doc, Author: c.UserID})
	return err
}

// TemplateDocument reads the current text of a template.
func (s *Service) TemplateDocument(ctx context.Context, q db.Querier, id uuid.UUID) (document.Node, error) {
	var raw []byte
	err := q.QueryRow(ctx, `SELECT v.document FROM pages p JOIN page_versions v ON v.page_id = p.id AND v.number = p.current_version
		WHERE p.id = $1 AND p.kind = 'template'`, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return document.Node{}, apperr.NotFound()
	}
	if err != nil {
		return document.Node{}, err
	}
	return document.Parse(raw)
}

// UsableParent keeps a template's default parent, unless it is gone or out of reach.
func (s *Service) UsableParent(ctx context.Context, c identity.Caller, parent *uuid.UUID, spaceID uuid.UUID) *uuid.UUID {
	if parent == nil {
		return nil
	}
	if err := s.checkParent(ctx, s.pool, c, placement{spaceID: spaceID, parent: *parent, kind: KindPage}); err != nil {
		return nil
	}
	return parent
}

// TemplateStart is a page to begin from a template.
type TemplateStart struct {
	SpaceID      uuid.UUID
	ParentID     *uuid.UUID
	Title        string
	TemplatePage uuid.UUID
	Document     document.Node
	Labels       []string
}

// StartFromTemplate makes an unpublished page with a draft. It answers with the
// labels a fixed list of the space left out.
func (s *Service) StartFromTemplate(ctx context.Context, c identity.Caller, in TemplateStart) (Page, []string, error) {
	title := in.Title
	if title != "" {
		clean, err := cleanTitle(title)
		if err != nil {
			return Page{}, nil, err
		}
		title = clean
	}
	var out Page
	skipped := []string{}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, in.SpaceID, permissions.PageCreate); err != nil {
			return err
		}
		if err := lockTree(ctx, tx, in.SpaceID); err != nil {
			return err
		}
		if in.ParentID != nil {
			if err := s.checkParent(ctx, tx, c, placement{spaceID: in.SpaceID, parent: *in.ParentID, kind: KindPage}); err != nil {
				return err
			}
		}
		var err error
		if out, err = s.insert(ctx, tx, c, newPage{spaceID: in.SpaceID, parent: in.ParentID, title: title, author: c.UserID}); err != nil {
			return err
		}
		mapping, err := s.attachments.CopyAll(ctx, tx, c, in.TemplatePage, out.ID, in.SpaceID)
		if err != nil {
			return err
		}
		doc := in.Document
		document.RemapAttachments(&doc, mapping)
		raw, err := document.Marshal(doc)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO drafts (page_id, user_id, title, document, schema_version, base_version)
			VALUES ($1, $2, $3, $4, $5, 0)`, out.ID, c.UserID, title, raw, document.SchemaVersion); err != nil {
			return err
		}
		for _, label := range in.Labels {
			refused, err := labels.Refused(ctx, tx, in.SpaceID, label)
			if err != nil {
				return err
			}
			if refused {
				skipped = append(skipped, label)
				continue
			}
			if err := addLabel(ctx, tx, c, out, label); err != nil {
				return err
			}
		}
		return nil
	})
	return out, skipped, err
}

// RequireRoomForFiles refuses at once when the files of a page would not fit in a space,
// so saving a page as a template stops before it makes the template.
func (s *Service) RequireRoomForFiles(ctx context.Context, from, spaceID uuid.UUID) error {
	return attachments.RefuseCopyThatDoesNotFit(ctx, s.pool, from, spaceID)
}

// CopyFilesToTemplate copies a page's files to its template, and points the
// template's text at the copies.
func (s *Service) CopyFilesToTemplate(ctx context.Context, c identity.Caller, from, to, spaceID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		mapping, err := s.attachments.CopyAll(ctx, tx, c, from, to, spaceID)
		if err != nil || len(mapping) == 0 {
			return err
		}
		doc, err := s.TemplateDocument(ctx, tx, to)
		if err != nil {
			return err
		}
		document.RemapAttachments(&doc, mapping)
		p, err := lockPage(ctx, tx, to)
		if err != nil {
			return err
		}
		_, err = s.appendVersion(ctx, tx, c, p, newVersion{Title: p.Title, Doc: doc, Author: c.UserID})
		return err
	})
}

const companyHomeKey = "company_home_page_id"

// CompanyHomeSetting reads the page an administrator picked.
func (s *Service) CompanyHomeSetting(ctx context.Context) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.settings.Read(ctx, companyHomeKey, &id)
	return id, err
}

// CompanyHome is the page everyone starts on, if an administrator picked one and
// this person may see it. Someone who may not gets the personal screen.
func (s *Service) CompanyHome(ctx context.Context, c identity.Caller) (*uuid.UUID, error) {
	id, err := s.CompanyHomeSetting(ctx)
	if err != nil || id == uuid.Nil {
		return nil, err
	}
	d, err := s.perms.CheckPage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil)
	if _, gone := apperr.As(err); gone {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !d.Allowed {
		return nil, nil
	}
	return &id, nil
}

// SetCompanyHome picks that page. The route requires administrator. An
// administrator has no content access, so it need not be a page they can read.
func (s *Service) SetCompanyHome(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	if id != uuid.Nil {
		if _, err := load(ctx, s.pool, id); err != nil {
			return err
		}
	}
	return s.settings.Write(ctx, c, companyHomeKey, id)
}

// DraftRetention reads how many days an unused draft stays.
func (s *Service) DraftRetention(ctx context.Context) (int, error) {
	days := defaultDraftDays
	err := s.settings.Read(ctx, draftRetentionKey, &days)
	return days, err
}

// SetDraftRetention sets how many days an unused draft stays.
func (s *Service) SetDraftRetention(ctx context.Context, c identity.Caller, days int) error {
	if days <= draftWarningDays || days > maxDraftDays {
		return apperr.Invalid("Drafts stay between 15 and 3650 days.")
	}
	return s.settings.Write(ctx, c, draftRetentionKey, days)
}
