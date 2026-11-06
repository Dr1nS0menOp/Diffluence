// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/comments"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/tasks"
)

// Draft is a user's private work in progress on a page.
type Draft struct {
	PageID        uuid.UUID       `json:"page_id"`
	Title         string          `json:"title"`
	Document      json.RawMessage `json:"document"`
	SchemaVersion int             `json:"schema_version"`
	BaseVersion   int             `json:"base_version"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// DraftInput is what autosave sends.
type DraftInput struct {
	Title       string
	Document    json.RawMessage
	BaseVersion int
}

// SaveDraft overwrites the user's draft. Drafts are private and autosaved, so
// they are not audited.
func (s *Service) SaveDraft(ctx context.Context, c identity.Caller, id uuid.UUID, in DraftInput) (Draft, error) {
	doc, err := document.Normalize(in.Document)
	if err != nil {
		return Draft{}, err
	}
	raw, err := document.Marshal(doc)
	if err != nil {
		return Draft{}, err
	}
	title := strings.TrimSpace(in.Title)
	if !utf8.ValidString(title) || utf8.RuneCountInString(title) > maxTitleRunes || strings.ContainsAny(title, "\r\n\x00") {
		return Draft{}, apperr.Invalid("A title must be one line of at most 255 characters.")
	}
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
		return Draft{}, err
	}
	d := Draft{PageID: id, Title: title, Document: raw, SchemaVersion: document.SchemaVersion, BaseVersion: in.BaseVersion}
	err = s.pool.QueryRow(ctx, `INSERT INTO drafts (page_id, user_id, title, document, schema_version, base_version)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (page_id, user_id) DO UPDATE SET title = EXCLUDED.title, document = EXCLUDED.document,
			schema_version = EXCLUDED.schema_version, base_version = EXCLUDED.base_version, updated_at = now(), warned_at = NULL
		RETURNING updated_at`, id, c.UserID, title, raw, document.SchemaVersion, in.BaseVersion).Scan(&d.UpdatedAt)
	return d, err
}

// GetDraft returns the user's own draft, if any.
func (s *Service) GetDraft(ctx context.Context, c identity.Caller, id uuid.UUID) (Draft, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return Draft{}, err
	}
	d := Draft{PageID: id}
	err := s.pool.QueryRow(ctx, `SELECT title, document, schema_version, base_version, updated_at FROM drafts
		WHERE page_id = $1 AND user_id = $2`, id, c.UserID).
		Scan(&d.Title, &d.Document, &d.SchemaVersion, &d.BaseVersion, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, apperr.NotFound()
	}
	if err != nil {
		return d, err
	}
	d.Document, err = s.withNames(ctx, d.Document)
	return d, err
}

// DiscardDraft removes the user's draft.
func (s *Service) DiscardDraft(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, "DELETE FROM drafts WHERE page_id = $1 AND user_id = $2", id, c.UserID)
	return err
}

// PublishInput is a request to publish.
type PublishInput struct {
	Title       string
	Document    json.RawMessage
	BaseVersion int
	Summary     string
	Overwrite   bool // publish even though someone else published in between
	Silent      bool // tell no watcher: a small fix (PAG-025)
	// StageText and StageColor apply when SetStage is true (PAG-057).
	SetStage   bool
	StageText  string
	StageColor string
}

func checkSummary(summary string) (string, error) {
	summary = strings.TrimSpace(summary)
	if !utf8.ValidString(summary) || utf8.RuneCountInString(summary) > 500 {
		return "", apperr.Invalid("A summary can have at most 500 characters.")
	}
	return summary, nil
}

// lockPage takes the row lock that makes publishing one at a time per page.
func lockPage(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Page, error) {
	p, err := scanPage(tx.QueryRow(ctx, "SELECT "+pageColumns+" FROM pages WHERE id = $1 FOR NO KEY UPDATE", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, apperr.NotFound()
	}
	return p, err
}

// Publish creates a new version from the given content.
func (s *Service) Publish(ctx context.Context, c identity.Caller, id uuid.UUID, in PublishInput) (Page, error) {
	doc, err := document.Normalize(in.Document)
	if err != nil {
		return Page{}, err
	}
	if doc, err = document.ForPublish(doc); err != nil {
		return Page{}, err
	}
	title, err := cleanTitle(in.Title)
	if err != nil {
		return Page{}, err
	}
	summary, err := checkSummary(in.Summary)
	if err != nil {
		return Page{}, err
	}
	var out Page
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		p, err := lockPage(ctx, tx, id)
		if err != nil {
			return err
		}
		if p.CurrentVersion != in.BaseVersion && !in.Overwrite {
			return apperr.Conflict("version_conflict", "Someone else published this page while you were editing.")
		}
		if err := s.requireApproved(ctx, tx, p, title, doc); err != nil {
			return err
		}
		if in.SetStage {
			if err := s.applyStage(ctx, tx, c, p, in.StageText, in.StageColor); err != nil {
				return err
			}
		}
		if out, err = s.appendVersion(ctx, tx, c, p, newVersion{Title: title, Doc: doc, Author: c.UserID, Summary: summary, Silent: in.Silent}); err != nil {
			return err
		}
		if err := endRoundsOnPublish(ctx, tx, id, out.CurrentVersion, title, doc); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM scheduled_publications WHERE page_id = $1", id); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "DELETE FROM drafts WHERE page_id = $1 AND user_id = $2", id, c.UserID)
		return err
	})
	return out, err
}

func currentDocument(ctx context.Context, tx pgx.Tx, p Page) (document.Node, error) {
	var raw []byte
	err := tx.QueryRow(ctx, "SELECT document FROM page_versions WHERE page_id = $1 AND number = $2", p.ID, p.CurrentVersion).Scan(&raw)
	if err != nil {
		return document.Node{}, err
	}
	return document.Parse(raw)
}

// Rename changes the title. It creates a version, so history shows who did it.
func (s *Service) Rename(ctx context.Context, c identity.Caller, id uuid.UUID, title string) (Page, error) {
	title, err := cleanTitle(title)
	if err != nil {
		return Page{}, err
	}
	var out Page
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		p, err := lockPage(ctx, tx, id)
		if err != nil {
			return err
		}
		if p.CurrentVersion == 0 {
			// Not published yet: there is no version to add to.
			out, err = scanPage(tx.QueryRow(ctx, "UPDATE pages SET title = $2, updated_at = now() WHERE id = $1 RETURNING "+pageColumns, id, title))
			if err != nil {
				return err
			}
			return record(ctx, tx, c, "page.renamed", out, nil)
		}
		doc, err := currentDocument(ctx, tx, p)
		if err != nil {
			return err
		}
		if out, err = s.appendVersion(ctx, tx, c, p, newVersion{Title: title, Doc: doc, Author: c.UserID}); err != nil {
			return err
		}
		return record(ctx, tx, c, "page.renamed", out, map[string]any{"previous_title": p.Title})
	})
	return out, err
}

// Version is one entry of the history.
type Version struct {
	Number       int       `json:"number"`
	Title        string    `json:"title"`
	AuthorID     uuid.UUID `json:"author_id"`
	AuthorName   string    `json:"author_name"`
	Summary      string    `json:"summary"`
	RestoredFrom *int      `json:"restored_from"`
	CreatedAt    time.Time `json:"created_at"`
	// Imported says the author and date came from elsewhere, so nobody here vouches for them.
	Imported bool            `json:"imported"`
	Document json.RawMessage `json:"document,omitempty"`
	// Removed marks a version an administrator took out, whose number stays.
	Removed bool `json:"removed"`
}

const versionColumns = `v.number, v.title, v.author_id, COALESCE(u.display_name, ''), v.summary, v.restored_from, v.created_at, v.imported`

// Versions lists the history of a page, newest first.
func (s *Service) Versions(ctx context.Context, c identity.Caller, id uuid.UUID) ([]Version, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT "+versionColumns+` FROM page_versions v
		LEFT JOIN users u ON u.id = v.author_id WHERE v.page_id = $1 ORDER BY v.number DESC LIMIT 1000`, id)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Version, error) {
		var v Version
		err := r.Scan(&v.Number, &v.Title, &v.AuthorID, &v.AuthorName, &v.Summary, &v.RestoredFrom, &v.CreatedAt, &v.Imported)
		return v, err
	})
	if err != nil {
		return nil, err
	}
	removed, err := s.pool.Query(ctx, "SELECT number, removed_at FROM removed_versions WHERE page_id = $1", id)
	if err != nil {
		return nil, err
	}
	gone, err := pgx.CollectRows(removed, func(r pgx.CollectableRow) (Version, error) {
		v := Version{Removed: true}
		err := r.Scan(&v.Number, &v.CreatedAt)
		return v, err
	})
	if err != nil {
		return nil, err
	}
	out = append(out, gone...)
	slices.SortFunc(out, func(a, b Version) int { return b.Number - a.Number })
	if out == nil {
		out = []Version{}
	}
	return out, nil
}

func (s *Service) version(ctx context.Context, id uuid.UUID, number int) (Version, error) {
	var v Version
	err := s.pool.QueryRow(ctx, "SELECT "+versionColumns+`, v.document FROM page_versions v
		LEFT JOIN users u ON u.id = v.author_id WHERE v.page_id = $1 AND v.number = $2`, id, number).
		Scan(&v.Number, &v.Title, &v.AuthorID, &v.AuthorName, &v.Summary, &v.RestoredFrom, &v.CreatedAt, &v.Imported, &v.Document)
	if errors.Is(err, pgx.ErrNoRows) {
		var removed bool
		if err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM removed_versions WHERE page_id = $1 AND number = $2)", id, number).Scan(&removed); err != nil {
			return v, err
		}
		if removed {
			return v, apperr.Conflict("version_removed", "An administrator removed this version.")
		}
		return v, apperr.NotFound()
	}
	return v, err
}

// RemoveVersion takes one version out, for a leaked secret. The route requires
// administrator. Versions are immutable, so this deletes the row and leaves a
// marker: the number stays, and the audit log keeps who, when and why (PAG-096).
func (s *Service) RemoveVersion(ctx context.Context, c identity.Caller, id uuid.UUID, number int, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || utf8.RuneCountInString(reason) > 500 {
		return apperr.Invalid("Give a reason of at most 500 characters.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := lockPage(ctx, tx, id)
		if err != nil {
			return err
		}
		if number == p.CurrentVersion {
			return apperr.Conflict("current_version", "The current version cannot be removed. Publish a new one first.")
		}
		var raw []byte
		err = tx.QueryRow(ctx, "DELETE FROM page_versions WHERE page_id = $1 AND number = $2 RETURNING document", id, number).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound()
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO removed_versions (page_id, number, removed_by, reason) VALUES ($1, $2, $3, $4)", id, number, c.UserID, reason); err != nil {
			return err
		}
		removed, err := document.Parse(raw)
		if err != nil {
			return err
		}
		current, err := currentDocument(ctx, tx, p)
		if err != nil {
			return err
		}
		// A quote copies words from the page, so they would outlive the version (SEC-407).
		if err := comments.BlankQuotes(ctx, tx, c, id, removed, current); err != nil {
			return err
		}
		return record(ctx, tx, c, "page.version_removed", p, map[string]any{"version": number, "reason": reason})
	})
}

// GetVersion returns one version with its content.
func (s *Service) GetVersion(ctx context.Context, c identity.Caller, id uuid.UUID, number int) (Version, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return Version{}, err
	}
	v, err := s.version(ctx, id, number)
	if err != nil {
		return v, err
	}
	v.Document, err = s.withNames(ctx, v.Document)
	return v, err
}

// Comparison is the difference between two versions.
type Comparison struct {
	From   Version              `json:"from"`
	To     Version              `json:"to"`
	Blocks []document.DiffBlock `json:"blocks"`
}

// Compare shows what changed between two versions.
func (s *Service) Compare(ctx context.Context, c identity.Caller, id uuid.UUID, from, to int) (Comparison, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return Comparison{}, err
	}
	a, err := s.version(ctx, id, from)
	if err != nil {
		return Comparison{}, err
	}
	b, err := s.version(ctx, id, to)
	if err != nil {
		return Comparison{}, err
	}
	da, err := s.readable(ctx, a.Document)
	if err != nil {
		return Comparison{}, err
	}
	db, err := s.readable(ctx, b.Document)
	if err != nil {
		return Comparison{}, err
	}
	a.Document, b.Document = nil, nil
	return Comparison{From: a, To: b, Blocks: document.Compare(da, db)}, nil
}

// VersionsForMail reads two versions of a page. It checks no access: the caller has
// just done that for the person the mail goes to. Found is false when one of them is
// gone, which happens when a version was removed.
func VersionsForMail(ctx context.Context, q db.Querier, pageID uuid.UUID, from, to int) (before, after document.Node, found bool, err error) {
	read := func(number int) (document.Node, bool, error) {
		var raw []byte
		err := q.QueryRow(ctx, "SELECT document FROM page_versions WHERE page_id = $1 AND number = $2", pageID, number).Scan(&raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return document.Node{}, false, nil
		}
		if err != nil {
			return document.Node{}, false, err
		}
		n, err := document.Parse(raw)
		return n, err == nil, err
	}
	if before, found, err = read(from); !found || err != nil {
		return before, after, false, err
	}
	after, found, err = read(to)
	return before, after, found, err
}

// RestoreVersion publishes a new version with the content of an old one.
func (s *Service) RestoreVersion(ctx context.Context, c identity.Caller, id uuid.UUID, number int) (Page, error) {
	var out Page
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		p, err := lockPage(ctx, tx, id)
		if err != nil {
			return err
		}
		var raw []byte
		var title string
		err = tx.QueryRow(ctx, "SELECT title, document FROM page_versions WHERE page_id = $1 AND number = $2", id, number).Scan(&title, &raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound()
		}
		if err != nil {
			return err
		}
		doc, err := document.Parse(raw)
		if err != nil {
			return err
		}
		if out, err = s.appendVersion(ctx, tx, c, p, newVersion{Title: title, Doc: doc, Author: c.UserID, RestoredFrom: &number}); err != nil {
			return err
		}
		return record(ctx, tx, c, "page.version_restored", out, map[string]any{"restored_from": number})
	})
	return out, err
}

// SetTaskDone ticks a task. The document is the truth, so a tick publishes a silent
// version: it is a change, and it is not news (spec 010, D8).
func (s *Service) SetTaskDone(ctx context.Context, c identity.Caller, taskID uuid.UUID, done bool) (Page, error) {
	pageID, err := tasks.PageOf(ctx, s.pool, taskID)
	if err != nil {
		return Page{}, err
	}
	var out Page
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, pageID, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		p, err := lockPage(ctx, tx, pageID)
		if err != nil {
			return err
		}
		doc, err := currentDocument(ctx, tx, p)
		if err != nil {
			return err
		}
		if !document.SetTaskDone(&doc, taskID, done) {
			return apperr.NotFound()
		}
		out, err = s.appendVersion(ctx, tx, c, p, newVersion{Title: p.Title, Doc: doc, Author: c.UserID, Silent: true})
		if err != nil {
			return err
		}
		action := "task.reopened"
		if done {
			action = "task.done"
		}
		return recordEvent(ctx, tx, c, action, out, map[string]any{"task_id": taskID}, map[string]any{"task_id": taskID})
	})
	return out, err
}
