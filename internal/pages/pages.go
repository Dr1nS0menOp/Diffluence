// SPDX-License-Identifier: Apache-2.0

// Package pages owns pages, the page tree, versions and drafts (spec 001).
// Every method takes the Caller and checks permissions itself.
package pages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/comments"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/search"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/tasks"
)

// Page statuses.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusTrashed   = "trashed"
)

// Kinds of page. They share versions, drafts, restrictions and search, so a blog
// post is a page that does not sit in the tree (spec 011, D1).
const (
	KindPage     = "page"
	KindPost     = "post"
	KindFolder   = "folder"
	KindLink     = "link"
	KindDatabase = "database"
	KindTemplate = permissions.KindTemplate
)

const maxTitleRunes = 255

// Page is the metadata of a page.
type Page struct {
	ID             uuid.UUID  `json:"id"`
	SpaceID        uuid.UUID  `json:"space_id"`
	ParentID       *uuid.UUID `json:"parent_id"`
	Position       int        `json:"position"`
	Title          string     `json:"title"`
	Status         string     `json:"status"`
	CurrentVersion int        `json:"current_version"`
	CreatedBy      uuid.UUID  `json:"created_by"`
	UpdatedBy      uuid.UUID  `json:"updated_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Kind           string     `json:"kind"`
	OwnerID        uuid.UUID  `json:"owner_id"`
	StageText      string     `json:"stage_text"`
	StageColor     string     `json:"stage_color"`
	ReviewOn       string     `json:"review_on"`
	ArchivedAt     *time.Time `json:"archived_at"`
	ArchivedBy     *uuid.UUID `json:"archived_by"`
	PublishedAt    *time.Time `json:"published_at"`
	LinkTarget     string     `json:"link_target"`
	CommentsClosed bool       `json:"comments_closed"`
	Look
}

// Crumb is one step of the breadcrumb trail.
type Crumb struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
	Emoji string    `json:"emoji"`
}

// Detail is a page with everything the reading view needs.
type Detail struct {
	Page
	Document      json.RawMessage `json:"document"`
	SchemaVersion int             `json:"schema_version"`
	CreatedByName string          `json:"created_by_name"`
	UpdatedByName string          `json:"updated_by_name"`
	Breadcrumbs   []Crumb         `json:"breadcrumbs"`
	HasDraft      bool            `json:"has_draft"`
	Favourite     bool            `json:"favourite"`
	Restricted    bool            `json:"restricted"`
	Can           map[string]bool `json:"can"`
	OwnerName     string          `json:"owner_name"`
	OwnerDisabled bool            `json:"owner_disabled"`
	Labels        []string        `json:"labels"`
	Verified      *Verified       `json:"verified"`
	// SeenVersion is the version this person read last time, or 0 on a first visit.
	SeenVersion int        `json:"seen_version"`
	Scheduled   *Scheduled `json:"scheduled"`
	Approval    *Approval  `json:"approval"`

	Features map[string]bool `json:"features"`
}

// Service manages pages.
type Service struct {
	pool        *pgxpool.Pool
	perms       *permissions.Service
	attachments *attachments.Service
	settings    *settings.Service
	features    *features.Service
	jobs        *jobs.Service
	instanceID  uuid.UUID
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, att *attachments.Service, st *settings.Service,
	ft *features.Service, jb *jobs.Service, instanceID uuid.UUID) *Service {
	s := &Service{pool: pool, perms: perms, attachments: att, settings: st, features: ft, jobs: jb, instanceID: instanceID}
	jb.Register(jobCopy, s.runCopyJob)
	jb.Register(jobContent, s.runContentJob)
	return s
}

func cleanTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", apperr.Invalid("A page needs a title.")
	}
	if !utf8.ValidString(title) || utf8.RuneCountInString(title) > maxTitleRunes {
		return "", apperr.Invalid("A title can have at most 255 characters.")
	}
	if strings.ContainsAny(title, "\r\n\x00") {
		return "", apperr.Invalid("A title must be one line.")
	}
	return title, nil
}

const pageColumns = `id, space_id, parent_id, position, title, status, current_version, created_by, updated_by, created_at, updated_at,
	comments_closed, emoji, full_width, title_align, hide_children, header_attachment_id, header_position,
	kind, COALESCE(owner_id, created_by), stage_text, stage_color, COALESCE(to_char(review_on, 'YYYY-MM-DD'), ''),
	archived_at, archived_by, published_at, link_target`

func scanPage(r pgx.Row) (Page, error) {
	var p Page
	err := r.Scan(&p.ID, &p.SpaceID, &p.ParentID, &p.Position, &p.Title, &p.Status, &p.CurrentVersion,
		&p.CreatedBy, &p.UpdatedBy, &p.CreatedAt, &p.UpdatedAt, &p.CommentsClosed,
		&p.Emoji, &p.FullWidth, &p.TitleAlign, &p.HideChildren, &p.HeaderAttachmentID, &p.HeaderPosition,
		&p.Kind, &p.OwnerID, &p.StageText, &p.StageColor, &p.ReviewOn, &p.ArchivedAt, &p.ArchivedBy, &p.PublishedAt, &p.LinkTarget)
	return p, err
}

func load(ctx context.Context, q db.Querier, id uuid.UUID) (Page, error) {
	p, err := scanPage(q.QueryRow(ctx, "SELECT "+pageColumns+" FROM pages WHERE id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, apperr.NotFound()
	}
	return p, err
}

// lockTree makes tree changes within a space take turns, so two concurrent
// moves cannot create a loop. NO KEY keeps foreign-key checks unblocked.
func lockTree(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID) error {
	var id uuid.UUID
	err := tx.QueryRow(ctx, "SELECT id FROM spaces WHERE id = $1 FOR NO KEY UPDATE", spaceID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.NotFound()
	}
	return err
}

func record(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, p Page, details map[string]any) error {
	return recordEvent(ctx, tx, c, action, p, details, nil)
}

// recordEvent is record with more in the event for other modules. Events hold ids,
// never a title or a sentence.
func recordEvent(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, p Page, details, payload map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	details["title"] = p.Title
	if _, err := audit.Record(ctx, tx, c.Event(action, "page", p.ID.String(), p.SpaceID.String(), details)); err != nil {
		return err
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["page_id"], payload["space_id"], payload["version"], payload["kind"] = p.ID, p.SpaceID, p.CurrentVersion, p.Kind
	if _, set := payload["actor_id"]; !set {
		payload["actor_id"] = c.UserID
	}
	// Those who send notifications must know who an event is for.
	for _, key := range []string{"owner_id", "user_id"} {
		if v, ok := details[key]; ok {
			payload[key] = v
		}
	}
	c.Stamp(payload)
	_, err := outbox.Enqueue(ctx, tx, action, payload)
	return err
}

// newPage is a row to add.
type newPage struct {
	spaceID uuid.UUID
	parent  *uuid.UUID
	title   string
	author  uuid.UUID
	kind    string
	link    string
}

// insert adds a page row and its closure rows, at the end of its siblings. Kinds
// outside the tree take no position, or they would leave gaps between pages.
func (s *Service) insert(ctx context.Context, tx pgx.Tx, c identity.Caller, in newPage) (Page, error) {
	id := ids.New()
	if in.kind == "" {
		in.kind = KindPage
	}
	var pos int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(position) + 1, 0) FROM pages
		WHERE space_id = $1 AND parent_id IS NOT DISTINCT FROM $2 AND kind = ANY($3)`, in.spaceID, in.parent, treeKinds).Scan(&pos); err != nil {
		return Page{}, err
	}
	p, err := scanPage(tx.QueryRow(ctx, `INSERT INTO pages (id, origin_instance_id, space_id, parent_id, position, title, status,
			created_by, updated_by, owner_id, kind, link_target, title_align)
		VALUES ($1, $2, $3, $4, $5, $6, 'draft', $7, $7, $7, $8, $9, (SELECT title_align FROM spaces WHERE id = $3)) RETURNING `+pageColumns,
		id, s.instanceID, in.spaceID, in.parent, pos, in.title, in.author, in.kind, in.link))
	if err != nil {
		return p, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO page_tree (ancestor_id, descendant_id, depth)
		SELECT ancestor_id, $2::uuid, depth + 1 FROM page_tree WHERE descendant_id = $1
		UNION ALL SELECT $2::uuid, $2::uuid, 0`, in.parent, id); err != nil {
		return p, err
	}
	return p, record(ctx, tx, c, "page.created", p, map[string]any{"parent_id": in.parent})
}

// NewPage is the input for Create.
type NewPage struct {
	SpaceID  uuid.UUID
	ParentID *uuid.UUID
	Title    string
	// Untitled starts a page whose title comes later. It cannot be published
	// until it has one (PAG-003).
	Untitled bool
	Kind     string
	Link     string
}

// checkParent makes sure a page may go under parent.
func (s *Service) checkParent(ctx context.Context, q db.Querier, c identity.Caller, spaceID uuid.UUID, parent uuid.UUID) error {
	if err := s.perms.RequirePage(ctx, q, c.UserID, parent, permissions.PageCreate, uuid.Nil); err != nil {
		return err
	}
	pp, err := load(ctx, q, parent)
	if err != nil {
		return err
	}
	if pp.SpaceID != spaceID {
		return apperr.Invalid("The parent page is in another space.")
	}
	if pp.Status != StatusPublished {
		return apperr.Conflict("parent_unpublished", "Publish the parent page first.")
	}
	return nil
}

// Create starts a new unpublished page, visible only to its creator.
func (s *Service) Create(ctx context.Context, c identity.Caller, in NewPage) (Page, error) {
	title, err := in.cleanedTitle()
	if err != nil {
		return Page{}, err
	}
	link, err := in.cleanedLink()
	if err != nil {
		return Page{}, err
	}
	var p Page
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, in.SpaceID, permissions.PageCreate); err != nil {
			return err
		}
		if in.Kind == KindPost {
			if err := s.requireFeature(ctx, tx, in.SpaceID, features.Blog); err != nil {
				return err
			}
		}
		if err := lockTree(ctx, tx, in.SpaceID); err != nil {
			return err
		}
		if in.ParentID != nil {
			if err := s.checkParent(ctx, tx, c, in.SpaceID, *in.ParentID); err != nil {
				return err
			}
		}
		p, err = s.insert(ctx, tx, c, newPage{spaceID: in.SpaceID, parent: in.ParentID, title: title, author: c.UserID, kind: in.Kind, link: link})
		if err != nil || (in.Kind != KindLink && in.Kind != KindFolder && in.Kind != KindDatabase) {
			return err
		}
		// A link entry and a folder have nothing to write, so they are published at once.
		// A database starts as an empty sheet, so it has its grid from the first moment.
		doc := document.Empty()
		if in.Kind == KindDatabase {
			doc = document.EmptySheet(firstColumnName, secondColumnName)
		}
		p, err = s.appendVersion(ctx, tx, c, p, newVersion{Title: title, Doc: doc, Author: c.UserID})
		return err
	})
	return p, err
}

func (in NewPage) cleanedTitle() (string, error) {
	switch in.Kind {
	case "", KindPage, KindPost, KindFolder, KindLink, KindDatabase:
	default:
		return "", apperr.Invalid("That is not a kind of page.")
	}
	if in.Kind == KindPost && in.ParentID != nil {
		return "", apperr.Invalid("A blog post has no place in the tree.")
	}
	if in.Untitled && strings.TrimSpace(in.Title) == "" {
		return "", nil
	}
	return cleanTitle(in.Title)
}

func (in NewPage) cleanedLink() (string, error) {
	if in.Kind != KindLink {
		return "", nil
	}
	link, err := document.SafeHref(in.Link)
	if err != nil {
		return "", apperr.Invalid("A link entry goes to an address in this instance, or to an http or https address.")
	}
	return link, nil
}

// CreatePublished adds a published page inside tx. The caller has already
// checked permissions. It is for home pages, copies and imports.
func (s *Service) CreatePublished(ctx context.Context, tx pgx.Tx, c identity.Caller, spaceID uuid.UUID, parent *uuid.UUID, title string, doc document.Node, author uuid.UUID) (Page, error) {
	p, err := s.insert(ctx, tx, c, newPage{spaceID: spaceID, parent: parent, title: title, author: author})
	if err != nil {
		return p, err
	}
	return s.appendVersion(ctx, tx, c, p, newVersion{Title: title, Doc: doc, Author: author})
}

// newVersion is what the next version of a page holds.
type newVersion struct {
	Title        string
	Doc          document.Node
	Author       uuid.UUID
	Summary      string
	RestoredFrom *int
	Silent       bool // reaches no watcher, and the history still shows it (PAG-025)
}

// syncMentions keeps the list of people and groups a page mentions, and returns those
// that are new in this version, so nobody is told again on every later edit.
func syncMentions(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, doc document.Node) (people, groups []uuid.UUID, err error) {
	rows, err := tx.Query(ctx, "SELECT user_id FROM page_mentions WHERE page_id = $1", pageID)
	if err != nil {
		return nil, nil, err
	}
	before, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, nil, err
	}
	isNew := func(list []uuid.UUID) []uuid.UUID {
		return slices.DeleteFunc(slices.Clone(list), func(id uuid.UUID) bool { return slices.Contains(before, id) })
	}
	current, currentGroups := document.MentionedUsers(doc), document.MentionedGroups(doc)
	if _, err := tx.Exec(ctx, "DELETE FROM page_mentions WHERE page_id = $1", pageID); err != nil {
		return nil, nil, err
	}
	_, err = tx.Exec(ctx, "INSERT INTO page_mentions (page_id, user_id) SELECT $1, person FROM unnest($2::uuid[]) AS person",
		pageID, append(slices.Clone(current), currentGroups...))
	return isNew(current), isNew(currentGroups), err
}

// appendVersion writes the next version and updates the page and search index.
func (s *Service) appendVersion(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, in newVersion) (Page, error) {
	title, doc, author := in.Title, in.Doc, in.Author
	if err := checkDocumentKind(p.Kind, doc); err != nil {
		return Page{}, err
	}
	if err := tasks.AssignIDs(ctx, tx, p.ID, &doc); err != nil {
		return p, err
	}
	raw, err := document.Marshal(doc)
	if err != nil {
		return p, err
	}
	first := p.CurrentVersion == 0
	number := p.CurrentVersion + 1
	if _, err := tx.Exec(ctx, `INSERT INTO page_versions (id, origin_instance_id, page_id, number, title, document, schema_version, author_id, summary, restored_from, silent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		ids.New(), s.instanceID, p.ID, number, title, raw, document.SchemaVersion, author, in.Summary, in.RestoredFrom, in.Silent); err != nil {
		return p, err
	}
	if err := attachments.RecordUse(ctx, tx, p.ID, number, doc); err != nil {
		return p, err
	}
	if p.PublishedAt == nil {
		if _, err := tx.Exec(ctx, `UPDATE pages p SET review_on = (now() + make_interval(months => s.review_months))::date
			FROM spaces s WHERE p.id = $1 AND s.id = p.space_id AND p.review_on IS NULL AND s.review_months > 0`, p.ID); err != nil {
			return p, err
		}
	}
	p, err = scanPage(tx.QueryRow(ctx, `UPDATE pages SET current_version = $2, title = $3, status = 'published', updated_by = $4, updated_at = now(),
			published_at = COALESCE(published_at, now())
		WHERE id = $1 RETURNING `+pageColumns, p.ID, number, title, author))
	if err != nil {
		return p, err
	}
	if p.Kind == KindTemplate {
		return p, nil // a template is never found, and has no page to link from
	}
	if err := search.Index(ctx, tx, p.ID, title, document.Text(doc)); err != nil {
		return p, err
	}
	if err := syncLinks(ctx, tx, p.ID, doc); err != nil {
		return p, err
	}
	if err := syncBlocks(ctx, tx, p.ID, doc); err != nil {
		return p, err
	}
	mentions, mentionedGroups, err := syncMentions(ctx, tx, p.ID, doc)
	if err != nil {
		return p, err
	}
	assigned, err := tasks.Sync(ctx, tx, p.ID, p.SpaceID, author, doc)
	if err != nil {
		return p, err
	}
	if err := comments.ReleaseEditorsOnly(ctx, tx, p.ID, doc); err != nil {
		return p, err
	}
	// Attachment names were not indexed while the page had no search row yet.
	rows, err := tx.Query(ctx, "SELECT filename FROM attachments WHERE page_id = $1 AND status = 'active'", p.ID)
	if err != nil {
		return p, err
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return p, err
	}
	if err := search.SetAttachmentNames(ctx, tx, p.ID, names); err != nil {
		return p, err
	}
	details := map[string]any{"version": number}
	if in.RestoredFrom != nil {
		details["restored_from"] = *in.RestoredFrom
	}
	if in.Silent {
		details["silent"] = true
	}
	return p, recordEvent(ctx, tx, c, "page.published", p, details, map[string]any{"actor_id": author, "silent": in.Silent, "first": first, "version": number,
		"mentions": mentions, "mentioned_groups": mentionedGroups, "assigned": assigned})
}

// Get returns a page for reading, and remembers it in the user's recent list.
func (s *Service) Get(ctx context.Context, c identity.Caller, id uuid.UUID) (Detail, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return Detail{}, err
	}
	p, err := load(ctx, s.pool, id)
	if err != nil {
		return Detail{}, err
	}
	d := Detail{Page: p}
	if d.Features, err = s.features.ForSpace(ctx, s.pool, p.SpaceID); err != nil {
		return d, err
	}
	if p.CurrentVersion > 0 {
		err = s.pool.QueryRow(ctx, "SELECT document, schema_version FROM page_versions WHERE page_id = $1 AND number = $2", id, p.CurrentVersion).
			Scan(&d.Document, &d.SchemaVersion)
		if err != nil {
			return d, err
		}
		if d.Document, err = s.withNames(ctx, d.Document); err != nil {
			return d, err
		}
	} else {
		d.Document, _ = document.Marshal(document.Empty())
		d.SchemaVersion = document.SchemaVersion
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji FROM page_tree t JOIN pages p ON p.id = t.ancestor_id
		WHERE t.descendant_id = $1 AND t.depth > 0 ORDER BY t.depth DESC`, id)
	if err != nil {
		return d, err
	}
	d.Breadcrumbs, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Crumb, error) {
		var b Crumb
		err := r.Scan(&b.ID, &b.Title, &b.Emoji)
		return b, err
	})
	if err != nil {
		return d, err
	}
	if d.Breadcrumbs == nil {
		d.Breadcrumbs = []Crumb{}
	}
	err = s.pool.QueryRow(ctx, `SELECT
			EXISTS (SELECT 1 FROM drafts WHERE page_id = $1 AND user_id = $2),
			EXISTS (SELECT 1 FROM favourites WHERE page_id = $1 AND user_id = $2),
			EXISTS (SELECT 1 FROM page_tree t JOIN page_restrictions r ON r.page_id = t.ancestor_id
			        WHERE t.descendant_id = $1 AND (r.kind = 'view' OR t.depth = 0))`, id, c.UserID).
		Scan(&d.HasDraft, &d.Favourite, &d.Restricted)
	if err != nil {
		return d, err
	}
	if d.CreatedByName, d.UpdatedByName, err = s.authorNames(ctx, p); err != nil {
		return d, err
	}
	if d.Can, err = s.capabilities(ctx, c, p); err != nil {
		return d, err
	}
	if err := s.fillOrganising(ctx, c, &d); err != nil {
		return d, err
	}
	if d.HeaderAttachmentID != nil {
		// A file in the trash shows no header, and comes back with a restore.
		shown, err := s.attachments.IsImageOf(ctx, s.pool, *d.HeaderAttachmentID, id)
		if err != nil {
			return d, err
		}
		if !shown {
			d.HeaderAttachmentID = nil
		}
	}
	s.touchRecent(ctx, c.UserID, p)
	return d, nil
}

// readable parses a stored document and fills in today's names. A stored document holds
// ids only, so a mention follows a rename and survives anonymising (spec 010, D6).
func (s *Service) readable(ctx context.Context, raw json.RawMessage) (document.Node, error) {
	n, err := document.Parse(raw)
	if err != nil {
		return n, fmt.Errorf("pages: read document: %v", err)
	}
	names, err := identity.DisplayNames(ctx, s.pool, append(document.MentionedUsers(n), document.MentionedGroups(n)...))
	if err != nil {
		return n, err
	}
	document.FillMentionNames(&n, names)
	return n, nil
}

// withNames is readable for a document on its way out as JSON.
func (s *Service) withNames(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	if !document.HasMentions(raw) {
		return raw, nil
	}
	n, err := s.readable(ctx, raw)
	if err != nil {
		return nil, err
	}
	return document.Marshal(n)
}

func (s *Service) authorNames(ctx context.Context, p Page) (created, updated string, err error) {
	err = s.pool.QueryRow(ctx, `SELECT
			COALESCE((SELECT display_name FROM users WHERE id = $1), ''),
			COALESCE((SELECT display_name FROM users WHERE id = $2), '')`, p.CreatedBy, p.UpdatedBy).
		Scan(&created, &updated)
	if err != nil {
		return "", "", fmt.Errorf("pages: read author names: %v", err)
	}
	return created, updated, nil
}

// capabilities tells the UI which buttons to show. The server still checks
// every action; this only avoids offering what would be refused.
func (s *Service) capabilities(ctx context.Context, c identity.Caller, p Page) (map[string]bool, error) {
	u, err := s.perms.LoadUser(ctx, s.pool, c.UserID)
	if err != nil {
		return nil, err
	}
	space, page, err := s.perms.LoadPage(ctx, s.pool, p.ID)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, action := range []string{permissions.PageEdit, permissions.PageCreate, permissions.PageArchive, permissions.ActionPageDelete,
		permissions.AttachmentAdd, permissions.RestrictionManage, permissions.SpaceExport, permissions.SpaceAdminister,
		permissions.CommentAdd, permissions.ActionReact} {
		d := permissions.Evaluate(permissions.Input{User: u, Space: space, Page: &page, Action: action, Owner: page.CreatedBy})
		out[action] = d.Allowed
	}
	out[capabilityVerify] = out[permissions.PageEdit] && (p.OwnerID == c.UserID || out[permissions.SpaceAdminister])
	return out, nil
}

// touchRecent is a convenience list, so a failure must never fail the read.
func (s *Service) touchRecent(ctx context.Context, userID uuid.UUID, p Page) {
	_, _ = s.pool.Exec(ctx, `INSERT INTO recent_pages (user_id, page_id, seen_version) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, page_id) DO UPDATE SET viewed_at = now(), seen_version = EXCLUDED.seen_version
		WHERE recent_pages.viewed_at < now() - interval '1 minute'
			OR recent_pages.seen_version IS DISTINCT FROM EXCLUDED.seen_version
			OR EXISTS (SELECT 1 FROM recent_pages newer WHERE newer.user_id = $1 AND newer.viewed_at > recent_pages.viewed_at)`, userID, p.ID, p.CurrentVersion)
}

// recentPagesKept is how long a person's recent list may get.
const recentPagesKept = 50

// TrimRecent keeps each person's recent list short. It is a task, so a view of a
// page costs one small write at most (ARC-198).
func (s *Service) TrimRecent(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM recent_pages r USING
		(SELECT user_id, page_id, row_number() OVER (PARTITION BY user_id ORDER BY viewed_at DESC) AS position FROM recent_pages) ranked
		WHERE r.user_id = ranked.user_id AND r.page_id = ranked.page_id AND ranked.position > $1`, recentPagesKept)
	return err
}

// treeKinds are the kinds that have a place in the tree.
var treeKinds = []string{KindPage, KindFolder, KindLink, KindDatabase}

// Node is one entry of the page tree, with what its row shows. HasDraft and
// VisitedAt are about the person asking.
type Node struct {
	Kind          string     `json:"kind"`
	LinkTarget    string     `json:"link_target"`
	OwnerName     string     `json:"owner_name"`
	StageText     string     `json:"stage_text"`
	StageColor    string     `json:"stage_color"`
	ID            uuid.UUID  `json:"id"`
	ParentID      *uuid.UUID `json:"parent_id"`
	Position      int        `json:"position"`
	Title         string     `json:"title"`
	Status        string     `json:"status"`
	Emoji         string     `json:"emoji"`
	Restricted    bool       `json:"restricted"`
	HasDraft      bool       `json:"has_draft"`
	UpdatedAt     time.Time  `json:"updated_at"`
	UpdatedByName string     `json:"updated_by_name"`
	VisitedAt     *time.Time `json:"visited_at"`
}

// Tree returns the pages of a space the user may see, in order.
func (s *Service) Tree(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]Node, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceView); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.parent_id, p.position, p.title, p.status, p.emoji,
			EXISTS (SELECT 1 FROM page_restrictions r WHERE r.page_id = p.id),
			d.page_id IS NOT NULL, p.updated_at, COALESCE(u.display_name, ''), rp.viewed_at,
			p.kind, p.link_target, COALESCE(o.display_name, ''), p.stage_text, p.stage_color
		FROM pages p JOIN perm_listed_pages($1) v ON v.page_id = p.id
		LEFT JOIN users u ON u.id = p.updated_by
		LEFT JOIN users o ON o.id = COALESCE(p.owner_id, p.created_by)
		LEFT JOIN drafts d ON d.page_id = p.id AND d.user_id = $1
		LEFT JOIN recent_pages rp ON rp.page_id = p.id AND rp.user_id = $1
		WHERE p.space_id = $2 AND p.kind = ANY($3) ORDER BY p.parent_id NULLS FIRST, p.position, p.id`, c.UserID, spaceID, treeKinds)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Node, error) {
		var n Node
		err := r.Scan(&n.ID, &n.ParentID, &n.Position, &n.Title, &n.Status, &n.Emoji,
			&n.Restricted, &n.HasDraft, &n.UpdatedAt, &n.UpdatedByName, &n.VisitedAt,
			&n.Kind, &n.LinkTarget, &n.OwnerName, &n.StageText, &n.StageColor)
		return n, err
	})
	if out == nil {
		out = []Node{}
	}
	return out, err
}
