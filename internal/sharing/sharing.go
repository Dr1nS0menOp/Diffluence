// SPDX-License-Identifier: Apache-2.0

// Package sharing tells people about a page and handles requests for access (spec
// 010, C7 and C8). Sharing only notifies: it never grants anything by itself.
package sharing

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/notify"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/settings"
)

const (
	maxSharePeople   = 30
	maxMessageRunes  = 500
	maxListedAccess  = 50
	maxCandidates    = 20
	maxGroupMembers  = 50
	refusedCoolDown  = 7 * 24 * time.Hour
	kindView         = "view"
	kindEdit         = "edit"
	statusOpen       = "open"
	statusAllowed    = "allowed"
	statusRefused    = "refused"
	EffectNone       = "none"
	EffectRestrict   = "restriction"
	EffectSpaceGrant = "space_editor"
)

// Service shares pages and answers requests.
type Service struct {
	pool     *pgxpool.Pool
	perms    *permissions.Service
	identity *identity.Service
	settings *settings.Service
	notify   *notify.Service
	features *features.Service
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, id *identity.Service, st *settings.Service, n *notify.Service, f *features.Service) *Service {
	return &Service{pool: pool, perms: perms, identity: id, settings: st, notify: n, features: f}
}

// Entry is someone who has access.
type Entry struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Access says who can see a page.
type Access struct {
	Restricted bool    `json:"restricted"`
	Entries    []Entry `json:"entries"`
	Total      int     `json:"total"`
}

func subjectsOfRestriction(ctx context.Context, q db.Querier, pageID uuid.UUID) ([]Entry, error) {
	rows, err := q.Query(ctx, `SELECT r.subject_type, r.subject_id, COALESCE(u.display_name, g.name, '')
		FROM page_restrictions r
		LEFT JOIN users u ON r.subject_type = 'user' AND u.id = r.subject_id
		LEFT JOIN groups g ON r.subject_type = 'group' AND g.id = r.subject_id
		WHERE r.page_id = $1 AND r.kind = 'view' ORDER BY 3`, pageID)
	return collectEntries(rows, err)
}

func subjectsOfSpace(ctx context.Context, q db.Querier, spaceID uuid.UUID) ([]Entry, error) {
	rows, err := q.Query(ctx, `SELECT g.subject_type, g.subject_id, COALESCE(u.display_name, gr.name, '')
		FROM space_grants g
		LEFT JOIN users u ON g.subject_type = 'user' AND u.id = g.subject_id
		LEFT JOIN groups gr ON g.subject_type = 'group' AND gr.id = g.subject_id
		WHERE g.space_id = $1 AND g.permission = $2 ORDER BY 1, 3`, spaceID, permissions.SpaceView)
	return collectEntries(rows, err)
}

func collectEntries(rows pgx.Rows, err error) ([]Entry, error) {
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Entry, error) {
		var e Entry
		err := r.Scan(&e.Type, &e.ID, &e.Name)
		return e, err
	})
	if out == nil {
		out = []Entry{}
	}
	return out, err
}

// PageAccess lists who can see a page: the nearest restriction, or else the space.
func (s *Service) PageAccess(ctx context.Context, c identity.Caller, pageID uuid.UUID) (Access, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return Access{}, err
	}
	space, page, err := s.perms.LoadPage(ctx, s.pool, pageID)
	if err != nil {
		return Access{}, err
	}
	var entries []Entry
	if n := len(page.ViewChain); n > 0 {
		entries, err = subjectsOfRestriction(ctx, s.pool, page.ViewChain[n-1].PageID)
	} else {
		entries, err = subjectsOfSpace(ctx, s.pool, space.ID)
	}
	if err != nil {
		return Access{}, err
	}
	return Access{Restricted: len(page.ViewChain) > 0, Entries: entries[:min(len(entries), maxListedAccess)], Total: len(entries)}, nil
}

// Candidate is someone who can be mentioned or shared with.
type Candidate struct {
	Kind    string    `json:"kind"`
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Members int       `json:"members,omitempty"`
	CanView bool      `json:"can_view"`
}

func (s *Service) directory(ctx context.Context, c identity.Caller, spaceID uuid.UUID, text string) ([]identity.DirectoryEntry, error) {
	mayBrowse, err := s.identity.HasGlobalPermission(ctx, c.UserID, identity.PermViewProfiles)
	if err != nil {
		return nil, err
	}
	if mayBrowse {
		return s.identity.Directory(ctx, text)
	}
	// Without "view profiles", only people who have a grant in this space (NAV-059).
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT 'user', u.id, u.display_name, ''
		FROM space_grants g JOIN users u ON g.subject_type = 'user' AND u.id = g.subject_id
		WHERE g.space_id = $1 AND u.status = 'active'
		  AND ($2 = '' OR position(lower($2) IN lower(u.display_name)) > 0) LIMIT $3`, spaceID, text, maxCandidates)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (identity.DirectoryEntry, error) {
		var e identity.DirectoryEntry
		err := r.Scan(&e.Type, &e.ID, &e.Name, &e.Detail)
		return e, err
	})
}

func (s *Service) groupSize(ctx context.Context, group uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, "SELECT count(*) FROM group_members WHERE group_id = $1", group).Scan(&n)
	return n, err
}

// Candidates lists people for the @ list and for Share, those who can see the page
// first. forShare keeps groups in the list: a group mention has its own rule.
func (s *Service) Candidates(ctx context.Context, c identity.Caller, pageID uuid.UUID, text string, forShare bool) ([]Candidate, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return nil, err
	}
	space, _, err := s.perms.LoadPage(ctx, s.pool, pageID)
	if err != nil {
		return nil, err
	}
	found, err := s.directory(ctx, c, space.ID, strings.TrimSpace(text))
	if err != nil {
		return nil, err
	}
	values, err := s.settings.Get(ctx)
	if err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, e := range found {
		candidate := Candidate{Kind: e.Type, ID: e.ID, Name: e.Name, CanView: true}
		if e.Type == "group" {
			if candidate.Members, err = s.groupSize(ctx, e.ID); err != nil {
				return nil, err
			}
			if !forShare && (!values.GroupMentions || candidate.Members > maxGroupMembers) {
				continue
			}
		} else {
			d, err := s.perms.CheckPage(ctx, s.pool, e.ID, pageID, permissions.ActionPageView, uuid.Nil)
			if err != nil {
				return nil, err
			}
			candidate.CanView = d.Allowed
		}
		out = append(out, candidate)
	}
	slices.SortStableFunc(out, func(a, b Candidate) int {
		switch {
		case a.CanView == b.CanView:
			return 0
		case a.CanView:
			return -1
		}
		return 1
	})
	return out, nil
}

// ShareInput is who to tell, and what to say.
type ShareInput struct {
	People  []permissions.Subject
	Message string
	Notify  bool
}

// Person names someone in the answer to a share.
type Person struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// ShareResult says who was told, and who could not be.
type ShareResult struct {
	Notified int      `json:"notified"`
	NoAccess []Person `json:"no_access"`
}

func (s *Service) expand(ctx context.Context, q db.Querier, people []permissions.Subject) ([]uuid.UUID, error) {
	out := []uuid.UUID{}
	add := func(id uuid.UUID) {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	for _, sub := range people {
		switch sub.Type {
		case permissions.SubjectUser:
			add(sub.ID)
		case permissions.SubjectGroup:
			rows, err := q.Query(ctx, "SELECT user_id FROM group_members WHERE group_id = $1", sub.ID)
			if err != nil {
				return nil, err
			}
			members, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
			if err != nil {
				return nil, err
			}
			for _, m := range members {
				add(m)
			}
		default:
			return nil, apperr.Invalid("Share with people or groups.")
		}
	}
	if len(out) > maxSharePeople {
		return nil, apperr.Invalid("A page can be shared with at most 30 people at once.")
	}
	return out, nil
}

// Share notifies people about a page. Someone who cannot see it gets nothing: a
// notification would reveal its title (spec 010, D19). Nothing is granted here.
func (s *Service) Share(ctx context.Context, c identity.Caller, pageID uuid.UUID, in ShareInput) (ShareResult, error) {
	message := strings.TrimSpace(in.Message)
	if !utf8.ValidString(message) || utf8.RuneCountInString(message) > maxMessageRunes {
		return ShareResult{}, apperr.Invalid("A message can have at most 500 characters.")
	}
	out := ShareResult{NoAccess: []Person{}}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
			return err
		}
		space, _, err := s.perms.LoadPage(ctx, tx, pageID)
		if err != nil {
			return err
		}
		people, err := s.expand(ctx, tx, in.People)
		if err != nil {
			return err
		}
		event := ids.New()
		var missing []uuid.UUID
		for _, person := range people {
			d, err := s.perms.CheckPage(ctx, tx, person, pageID, permissions.ActionPageView, uuid.Nil)
			if err != nil {
				return err
			}
			if !d.Allowed {
				missing = append(missing, person)
				continue
			}
			out.Notified++
			if !in.Notify {
				continue
			}
			row := notify.Row{UserID: person, Type: notify.TypeShare, Reason: notify.ReasonShared, ActorID: c.UserID, PageID: pageID,
				SpaceID: space.ID, Event: event, Data: map[string]any{"message": message}}
			if err := s.notify.Deliver(ctx, tx, row); err != nil {
				return err
			}
		}
		names, err := identity.DisplayNames(ctx, tx, missing)
		if err != nil {
			return err
		}
		for _, id := range missing {
			out.NoAccess = append(out.NoAccess, Person{ID: id, Name: names[id]})
		}
		// The message stays out of the audit log: it is between people.
		if _, err := audit.Record(ctx, tx, c.Event("page.shared", "page", pageID.String(), space.ID.String(),
			map[string]any{"people": len(people), "notified": out.Notified})); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "page.shared", map[string]any{"page_id": pageID, "space_id": space.ID, "actor_id": c.UserID})
		return err
	})
	return out, err
}

// Request is a request for access, as a person who may answer sees it.
type Request struct {
	ID            uuid.UUID `json:"id"`
	PageID        uuid.UUID `json:"page_id"`
	RequesterName string    `json:"requester_name"`
	Kind          string    `json:"kind"`
	Status        string    `json:"status"`
	Effect        string    `json:"effect"`
	Levels        []Level   `json:"levels"`
	CanAllow      bool      `json:"can_allow"`
	CreatedAt     time.Time `json:"created_at"`
}

// Level is a restriction that allowing would change, as the person answering may know
// it: of one they cannot manage, they learn that it is there and nothing else.
type Level struct {
	Title     string `json:"title"`
	Kind      string `json:"kind"`
	Pages     int    `json:"pages"`
	CanManage bool   `json:"can_manage"`
}

type request struct {
	Request
	spaceID   uuid.UUID
	requester uuid.UUID
	asked     uuid.UUID
}

func loadRequest(ctx context.Context, q db.Querier, id uuid.UUID) (request, error) {
	var r request
	err := q.QueryRow(ctx, `SELECT a.id, a.page_id, a.space_id, a.requester_id, a.asked_id, a.kind, a.status, a.created_at, COALESCE(u.display_name, '')
		FROM access_requests a LEFT JOIN users u ON u.id = a.requester_id WHERE a.id = $1`, id).
		Scan(&r.ID, &r.PageID, &r.spaceID, &r.requester, &r.asked, &r.Kind, &r.Status, &r.CreatedAt, &r.RequesterName)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, apperr.NotFound()
	}
	return r, err
}

func actionFor(kind string) string {
	if kind == kindEdit {
		return permissions.PageEdit
	}
	return permissions.ActionPageView
}

// effectOf says what allowing would change. A restriction only narrows, so a page
// cannot give more than the space does (spec 010, D18).
func (s *Service) effectOf(ctx context.Context, q db.Querier, user, pageID uuid.UUID, kind string) (string, error) {
	u, err := s.perms.LoadUser(ctx, q, user)
	if err != nil {
		return "", err
	}
	space, page, err := s.perms.LoadPage(ctx, q, pageID)
	if err != nil {
		return "", err
	}
	action := actionFor(kind)
	if permissions.Evaluate(permissions.Input{User: u, Space: space, Page: &page, Action: action}).Allowed {
		return EffectNone, nil
	}
	open := permissions.PageFacts{ID: page.ID, Status: page.Status, CreatedBy: page.CreatedBy}
	if permissions.Evaluate(permissions.Input{User: u, Space: space, Page: &open, Action: action}).Allowed {
		return EffectRestrict, nil
	}
	return EffectSpaceGrant, nil
}

func grantAction(effect string) string {
	if effect == EffectSpaceGrant {
		return permissions.SpaceAdminister
	}
	return permissions.RestrictionManage
}

// levels lists what allowing would change, and says what each restriction opens.
func (s *Service) levels(ctx context.Context, q db.Querier, c identity.Caller, r request) ([]Level, error) {
	blocking, err := s.perms.LevelsBlocking(ctx, q, r.PageID, r.requester, r.Kind == kindEdit)
	if err != nil {
		return nil, err
	}
	out := make([]Level, 0, len(blocking))
	for _, b := range blocking {
		d, err := s.perms.CheckPage(ctx, q, c.UserID, b.PageID, permissions.RestrictionManage, uuid.Nil)
		if err != nil {
			return nil, err
		}
		level := Level{Kind: b.Kind, CanManage: d.Allowed}
		if d.Allowed {
			// An edit restriction sits on one page. A view restriction covers the branch,
			// counted as far as the person answering can see it.
			err = q.QueryRow(ctx, `SELECT p.title, CASE WHEN $3 THEN 1 ELSE (SELECT count(*) FROM page_tree t
					JOIN perm_visible_pages($2) v ON v.page_id = t.descendant_id WHERE t.ancestor_id = p.id) END
				FROM pages p WHERE p.id = $1`, b.PageID, c.UserID, b.Kind == "edit").Scan(&level.Title, &level.Pages)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, level)
	}
	return out, nil
}

func canOpenAll(levels []Level) bool {
	return !slices.ContainsFunc(levels, func(l Level) bool { return !l.CanManage })
}

// whoToAsk finds the first person who may grant it: the creator, the last editor, and
// then the space administrators (COL-051).
func (s *Service) whoToAsk(ctx context.Context, q db.Querier, pageID uuid.UUID, effect string) (uuid.UUID, error) {
	var creator, editor, spaceID uuid.UUID
	err := q.QueryRow(ctx, "SELECT created_by, updated_by, space_id FROM pages WHERE id = $1", pageID).Scan(&creator, &editor, &spaceID)
	if err != nil {
		return uuid.Nil, err
	}
	rows, err := q.Query(ctx, `SELECT subject_id FROM space_grants
		WHERE space_id = $1 AND permission = $2 AND subject_type = 'user' ORDER BY created_at`, spaceID, permissions.SpaceAdminister)
	if err != nil {
		return uuid.Nil, err
	}
	admins, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return uuid.Nil, err
	}
	for _, person := range append([]uuid.UUID{creator, editor}, admins...) {
		d, err := s.perms.CheckPage(ctx, q, person, pageID, grantAction(effect), uuid.Nil)
		if err != nil {
			return uuid.Nil, err
		}
		if d.Allowed {
			return person, nil
		}
	}
	return uuid.Nil, apperr.Conflict("nobody_to_ask", "Nobody can grant this at the moment.")
}

// MayAsk reports whether a person who cannot see a page may be offered a request. It
// stays false unless the space chose to allow that, so "not found" reveals nothing.
func (s *Service) MayAsk(ctx context.Context, c identity.Caller, pageID uuid.UUID) (bool, error) {
	space, page, err := s.perms.LoadPage(ctx, s.pool, pageID)
	if e, known := apperr.As(err); known && e.Kind == apperr.KindNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if page.Status != "published" {
		return false, nil
	}
	return s.features.Enabled(ctx, s.pool, space.ID, features.AccessRequests)
}

func (s *Service) checkMayRequest(ctx context.Context, q db.Querier, c identity.Caller, pageID uuid.UUID, kind string) error {
	if kind == kindEdit {
		return s.perms.RequirePage(ctx, q, c.UserID, pageID, permissions.ActionPageView, uuid.Nil)
	}
	allowed, err := s.MayAsk(ctx, c, pageID)
	if err != nil {
		return err
	}
	if !allowed {
		return apperr.NotFound()
	}
	return nil
}

func (s *Service) checkNotRefused(ctx context.Context, q db.Querier, c identity.Caller, pageID uuid.UUID, kind string) error {
	var recently bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM access_requests
		WHERE page_id = $1 AND requester_id = $2 AND kind = $3 AND status = 'refused'
		  AND answered_at > now() - make_interval(secs => $4))`, pageID, c.UserID, kind, refusedCoolDown.Seconds()).Scan(&recently)
	if err != nil {
		return err
	}
	if recently {
		return apperr.Conflict("recently_refused", "This was refused less than a week ago.")
	}
	return nil
}

// Ask opens a request for access.
func (s *Service) Ask(ctx context.Context, c identity.Caller, pageID uuid.UUID, kind string) error {
	if kind != kindView && kind != kindEdit {
		return apperr.Invalid("Ask to view or to edit.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.checkMayRequest(ctx, tx, c, pageID, kind); err != nil {
			return err
		}
		effect, err := s.effectOf(ctx, tx, c.UserID, pageID, kind)
		if err != nil {
			return err
		}
		if effect == EffectNone {
			return apperr.Conflict("already_allowed", "You can already do this.")
		}
		if err := s.checkNotRefused(ctx, tx, c, pageID, kind); err != nil {
			return err
		}
		asked, err := s.whoToAsk(ctx, tx, pageID, effect)
		if err != nil {
			return err
		}
		space, _, err := s.perms.LoadPage(ctx, tx, pageID)
		if err != nil {
			return err
		}
		id := ids.New()
		_, err = tx.Exec(ctx, `INSERT INTO access_requests (id, page_id, space_id, requester_id, kind, asked_id) VALUES ($1, $2, $3, $4, $5, $6)`,
			id, pageID, space.ID, c.UserID, kind, asked)
		if db.IsUniqueViolation(err) {
			return apperr.Conflict("already_asked", "You already asked for this.")
		}
		if err != nil {
			return err
		}
		return recordRequest(ctx, tx, c, "access.requested", request{Request: Request{ID: id, PageID: pageID, Kind: kind}, spaceID: space.ID, requester: c.UserID, asked: asked})
	})
}

func recordRequest(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, r request) error {
	if _, err := audit.Record(ctx, tx, c.Event(action, "page", r.PageID.String(), r.spaceID.String(),
		map[string]any{"request_id": r.ID, "kind": r.Kind, "requester_id": r.requester})); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"request_id": r.ID, "page_id": r.PageID, "space_id": r.spaceID,
		"actor_id": c.UserID, "asked_id": r.asked, "requester_id": r.requester})
	return err
}

// Describe shows a request to someone who may answer it, with what allowing changes.
func (s *Service) Describe(ctx context.Context, c identity.Caller, id uuid.UUID) (Request, error) {
	r, err := loadRequest(ctx, s.pool, id)
	if err != nil {
		return Request{}, err
	}
	effect, err := s.effectOf(ctx, s.pool, r.requester, r.PageID, r.Kind)
	if err != nil {
		return Request{}, err
	}
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, r.PageID, grantAction(effect), uuid.Nil); err != nil {
		return Request{}, err
	}
	r.Effect = effect
	if r.Levels, err = s.levels(ctx, s.pool, c, r); err != nil {
		return Request{}, err
	}
	r.CanAllow = canOpenAll(r.Levels)
	return r.Request, nil
}

// Open lists the open requests of a page, for those who may answer them.
func (s *Service) Open(ctx context.Context, c identity.Caller, pageID uuid.UUID) ([]Request, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.RestrictionManage, uuid.Nil); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT id FROM access_requests WHERE page_id = $1 AND status = 'open' ORDER BY created_at", pageID)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	out := []Request{}
	for _, id := range list {
		r, err := s.Describe(ctx, c, id)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// Answer allows or refuses a request. Allowing adds the person to every restriction
// that blocks them, so it is refused for someone who cannot manage them all. When the
// space blocks them, only a space administrator can allow it, and that gives Editor in
// the whole space (spec 010, D18).
func (s *Service) Answer(ctx context.Context, c identity.Caller, id uuid.UUID, allow bool, note string) error {
	if utf8.RuneCountInString(note) > maxMessageRunes {
		return apperr.Invalid("A note can have at most 500 characters.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		r, err := loadRequest(ctx, tx, id)
		if err != nil {
			return err
		}
		effect, err := s.effectOf(ctx, tx, r.requester, r.PageID, r.Kind)
		if err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, r.PageID, grantAction(effect), uuid.Nil); err != nil {
			return err
		}
		if r.Status != statusOpen {
			return apperr.Conflict("already_answered", "This request was already answered.")
		}
		status, action := statusRefused, "access.refused"
		if allow {
			status, action = statusAllowed, "access.allowed"
			levels, err := s.levels(ctx, tx, c, r)
			if err != nil {
				return err
			}
			if !canOpenAll(levels) {
				return apperr.Conflict("restriction_out_of_reach", "This needs someone who can manage every restriction on the way to this page.")
			}
			if err := s.grant(ctx, tx, c, r, effect); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE access_requests SET status = $2, answered_by = $3, note = $4, answered_at = now() WHERE id = $1`,
			id, status, c.UserID, strings.TrimSpace(note)); err != nil {
			return err
		}
		return recordRequest(ctx, tx, c, action, r)
	})
}

func (s *Service) grant(ctx context.Context, tx pgx.Tx, c identity.Caller, r request, effect string) error {
	if effect == EffectSpaceGrant {
		if err := s.perms.GrantEditor(ctx, tx, c, r.spaceID, r.requester); err != nil {
			return err
		}
	}
	return s.perms.OpenRestrictionsFor(ctx, tx, c, r.PageID, r.requester, r.Kind == kindEdit)
}
