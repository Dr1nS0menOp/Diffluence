// SPDX-License-Identifier: Apache-2.0

package permissions

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
)

// Templates: named sets of space permissions (spec 002).
const (
	TemplateViewer     = "viewer"
	TemplateEditor     = "editor"
	TemplateManager    = "manager"
	TemplateSpaceAdmin = "space_admin"
)

// Templates maps a template to its permissions.
var Templates = map[string][]string{
	TemplateViewer: {SpaceView, SpaceExport, CommentAdd, CommentDeleteOwn},
	TemplateEditor: {SpaceView, SpaceExport, PageCreate, PageEdit, PageArchive, PageDeleteOwn,
		CommentAdd, CommentDeleteOwn, AttachmentAdd, AttachmentDeleteOwn},
	// Manager runs the people and the content of a space, but not the space itself.
	TemplateManager: {SpaceView, SpaceExport, PageCreate, PageEdit, PageArchive, PageDeleteOwn, PageDeleteAny,
		CommentAdd, CommentDeleteOwn, CommentDeleteAny, AttachmentAdd, AttachmentDeleteOwn, AttachmentDeleteAny,
		RestrictionManage, SpaceManageAccess},
	TemplateSpaceAdmin: AllSpacePermissions,
}

// Whole-space presets: which subject gets which template.
const (
	PresetTeamSpace = "team_space"
	PresetReadOnly  = "read_only_kb"
)

func validSpaceSubject(s Subject) bool {
	switch s.Type {
	case SubjectUser, SubjectGroup:
		return s.ID != uuid.Nil
	case SubjectAllMembers, SubjectAllGuests:
		return s.ID == uuid.Nil
	}
	return false
}

// ApplyTemplateTx replaces a subject's permissions in a space, inside tx, with no
// checks. It is for setting up a new space; everything else uses SetGrants.
func ApplyTemplateTx(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID, sub Subject, template string) error {
	if _, err := tx.Exec(ctx, "DELETE FROM space_grants WHERE space_id = $1 AND subject_type = $2 AND subject_id = $3",
		spaceID, sub.Type, sub.ID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO space_grants (space_id, subject_type, subject_id, permission)
		SELECT $1::uuid, $2::text, $3::uuid, unnest($4::text[])`, spaceID, sub.Type, sub.ID, Templates[template])
	return err
}

// GrantEntry is one subject's permissions in a space, with a name for display.
type GrantEntry struct {
	Subject     Subject  `json:"subject"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

// Grants lists who holds what in a space, for those who manage access.
func (s *Service) Grants(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]GrantEntry, error) {
	if err := s.RequireSpace(ctx, s.pool, c.UserID, spaceID, SpaceManageAccess); err != nil {
		return nil, err
	}
	return s.grants(ctx, s.pool, spaceID)
}

func (s *Service) grants(ctx context.Context, q db.Querier, spaceID uuid.UUID) ([]GrantEntry, error) {
	rows, err := q.Query(ctx, `SELECT g.subject_type, g.subject_id,
			COALESCE(u.display_name, gr.name, ''), array_agg(g.permission ORDER BY g.permission)
		FROM space_grants g
		LEFT JOIN users u ON g.subject_type = 'user' AND u.id = g.subject_id
		LEFT JOIN groups gr ON g.subject_type = 'group' AND gr.id = g.subject_id
		WHERE g.space_id = $1
		GROUP BY g.subject_type, g.subject_id, u.display_name, gr.name
		ORDER BY g.subject_type, 3`, spaceID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (GrantEntry, error) {
		var e GrantEntry
		err := r.Scan(&e.Subject.Type, &e.Subject.ID, &e.Name, &e.Permissions)
		return e, err
	})
	if out == nil {
		out = []GrantEntry{}
	}
	return out, err
}

func (s *Service) keepsAnAdmin(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID) error {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM space_grants
		WHERE space_id = $1 AND permission = 'space.administer' AND subject_type IN ('user', 'group'))`, spaceID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Conflict("last_admin", "A space must keep at least one administrator.")
	}
	return nil
}

func (s *Service) subjectExists(ctx context.Context, q db.Querier, sub Subject) error {
	var ok bool
	var err error
	switch sub.Type {
	case SubjectUser:
		err = q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)", sub.ID).Scan(&ok)
	case SubjectGroup:
		err = q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM groups WHERE id = $1)", sub.ID).Scan(&ok)
	default:
		return nil
	}
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Invalid("That user or group does not exist.")
	}
	return nil
}

func recordChange(ctx context.Context, tx pgx.Tx, c identity.Caller, action, objType, objID, spaceID string, details map[string]any) error {
	if _, err := audit.Record(ctx, tx, c.Event(action, objType, objID, spaceID, details)); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"object_id": objID, "space_id": spaceID, "actor_id": c.UserID})
	return err
}

// SetGrants replaces one subject's permissions in a space. An empty list
// removes the subject.
func (s *Service) SetGrants(ctx context.Context, c identity.Caller, spaceID uuid.UUID, sub Subject, perms []string) error {
	if !validSpaceSubject(sub) {
		return apperr.Invalid("Choose a user, a group, all members or all guests.")
	}
	for _, p := range perms {
		if !slices.Contains(AllSpacePermissions, p) {
			return apperr.Invalid("Unknown permission.")
		}
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.RequireSpace(ctx, tx, c.UserID, spaceID, SpaceManageAccess); err != nil {
			return err
		}
		if err := s.subjectExists(ctx, tx, sub); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT permission FROM space_grants
			WHERE space_id = $1 AND subject_type = $2 AND subject_id = $3`, spaceID, sub.Type, sub.ID)
		if err != nil {
			return err
		}
		before, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if err := s.mayChangeSubject(ctx, tx, c, spaceID, sub, before, perms); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM space_grants WHERE space_id = $1 AND subject_type = $2 AND subject_id = $3",
			spaceID, sub.Type, sub.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO space_grants (space_id, subject_type, subject_id, permission)
			SELECT $1::uuid, $2::text, $3::uuid, unnest($4::text[]) ON CONFLICT DO NOTHING`, spaceID, sub.Type, sub.ID, perms); err != nil {
			return err
		}
		if err := s.keepsAnAdmin(ctx, tx, spaceID); err != nil {
			return err
		}
		for _, p := range perms {
			if !slices.Contains(before, p) {
				if err := recordChange(ctx, tx, c, "permission.space.granted", sub.Type, sub.ID.String(), spaceID.String(), map[string]any{"permission": p}); err != nil {
					return err
				}
			}
		}
		for _, p := range before {
			if !slices.Contains(perms, p) {
				if err := recordChange(ctx, tx, c, "permission.space.revoked", sub.Type, sub.ID.String(), spaceID.String(), map[string]any{"permission": p}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// mayChangeSubject holds the limits of managing access. Someone who only manages
// access cannot reach administer, or Manager would be administrator with one extra
// step. The owner of a personal space keeps their direct grant.
func (s *Service) mayChangeSubject(ctx context.Context, tx pgx.Tx, c identity.Caller, spaceID uuid.UUID, sub Subject, before, after []string) error {
	d, err := s.CheckSpace(ctx, tx, c.UserID, spaceID, SpaceAdminister)
	if err != nil {
		return err
	}
	touchesAdminister := slices.Contains(before, SpaceAdminister) || slices.Contains(after, SpaceAdminister)
	if !d.Allowed && touchesAdminister {
		return apperr.Conflict("needs_administer", "Only a space administrator can change an administrator.")
	}
	var owner *uuid.UUID
	if err := tx.QueryRow(ctx, "SELECT owner_id FROM spaces WHERE id = $1 AND kind = 'personal'", spaceID).Scan(&owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if owner != nil && sub.Type == SubjectUser && sub.ID == *owner && !slices.Contains(after, SpaceAdminister) {
		return apperr.Conflict("owner_grant", "The owner of a personal space keeps their access.")
	}
	return nil
}

// ApplyPreset sets up a space with one of the whole-space presets. It keeps
// directly listed users and groups, and replaces only the "all members" entry.
func (s *Service) ApplyPreset(ctx context.Context, c identity.Caller, spaceID uuid.UUID, preset string) error {
	template := ""
	switch preset {
	case PresetTeamSpace:
		template = TemplateEditor
	case PresetReadOnly:
		template = TemplateViewer
	default:
		return apperr.Invalid("Unknown preset.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.RequireSpace(ctx, tx, c.UserID, spaceID, SpaceAdminister); err != nil {
			return err
		}
		if err := ApplyTemplateTx(ctx, tx, spaceID, Subject{Type: SubjectAllMembers}, template); err != nil {
			return err
		}
		return recordChange(ctx, tx, c, "permission.template.applied", "space", spaceID.String(), spaceID.String(), map[string]any{"preset": preset})
	})
}

// Restrictions is the current state of a page's own restrictions.
type Restrictions struct {
	View      []NamedSubject `json:"view"`
	Edit      []NamedSubject `json:"edit"`
	Inherited []Inherited    `json:"inherited"`
}

// NamedSubject is a subject with a name for display.
type NamedSubject struct {
	Subject
	Name string `json:"name"`
}

// Inherited is a view restriction that comes from an ancestor.
type Inherited struct {
	PageID uuid.UUID `json:"page_id"`
	Title  string    `json:"title"`
}

// Restrictions returns what is set on a page. Anyone who can view it may see
// this: it explains why colleagues might not find the page.
func (s *Service) Restrictions(ctx context.Context, c identity.Caller, pageID uuid.UUID) (Restrictions, error) {
	if err := s.RequirePage(ctx, s.pool, c.UserID, pageID, ActionPageView, uuid.Nil); err != nil {
		return Restrictions{}, err
	}
	out := Restrictions{View: []NamedSubject{}, Edit: []NamedSubject{}, Inherited: []Inherited{}}
	rows, err := s.pool.Query(ctx, `SELECT r.kind, r.subject_type, r.subject_id, COALESCE(u.display_name, g.name, '')
		FROM page_restrictions r
		LEFT JOIN users u ON r.subject_type = 'user' AND u.id = r.subject_id
		LEFT JOIN groups g ON r.subject_type = 'group' AND g.id = r.subject_id
		WHERE r.page_id = $1 ORDER BY 4`, pageID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var n NamedSubject
		if err := rows.Scan(&kind, &n.Type, &n.ID, &n.Name); err != nil {
			return out, err
		}
		if kind == "view" {
			out.View = append(out.View, n)
		} else {
			out.Edit = append(out.Edit, n)
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	up, err := s.pool.Query(ctx, `SELECT DISTINCT t.depth, p.id, p.title FROM page_tree t
		JOIN page_restrictions r ON r.page_id = t.ancestor_id AND r.kind = 'view'
		JOIN pages p ON p.id = t.ancestor_id
		WHERE t.descendant_id = $1 AND t.depth > 0 ORDER BY t.depth`, pageID)
	if err != nil {
		return out, err
	}
	defer up.Close()
	for up.Next() {
		var depth int
		var i Inherited
		if err := up.Scan(&depth, &i.PageID, &i.Title); err != nil {
			return out, err
		}
		out.Inherited = append(out.Inherited, i)
	}
	return out, up.Err()
}

// BlockingLevel is one restriction that keeps a person out of a page.
type BlockingLevel struct {
	PageID uuid.UUID
	Kind   string
}

// LevelsBlocking lists every restriction that keeps a person out of a page. Each one
// must change before they can get in, and one on a page above opens that whole branch.
func (s *Service) LevelsBlocking(ctx context.Context, q db.Querier, pageID, user uuid.UUID, edit bool) ([]BlockingLevel, error) {
	_, page, err := s.LoadPage(ctx, q, pageID)
	if err != nil {
		return nil, err
	}
	facts, err := s.LoadUser(ctx, q, user)
	if err != nil {
		return nil, err
	}
	out := []BlockingLevel{}
	for _, level := range page.ViewChain {
		if !facts.matchesAny(level.Subjects) {
			out = append(out, BlockingLevel{PageID: level.PageID, Kind: "view"})
		}
	}
	if edit && len(page.EditSubjects) > 0 && !facts.matchesAny(page.EditSubjects) {
		out = append(out, BlockingLevel{PageID: pageID, Kind: "edit"})
	}
	return out, nil
}

// OpenRestrictionsFor adds one person to the restrictions that keep them out of a page.
// It only opens what the caller may manage, so it cannot widen anything quietly.
func (s *Service) OpenRestrictionsFor(ctx context.Context, tx pgx.Tx, c identity.Caller, pageID, user uuid.UUID, edit bool) error {
	levels, err := s.LevelsBlocking(ctx, tx, pageID, user, edit)
	if err != nil {
		return err
	}
	me := Subject{Type: SubjectUser, ID: user}
	for _, level := range levels {
		if err := s.addRestriction(ctx, tx, c, level.PageID, level.Kind, me); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) addRestriction(ctx context.Context, tx pgx.Tx, c identity.Caller, pageID uuid.UUID, kind string, sub Subject) error {
	if err := s.RequirePage(ctx, tx, c.UserID, pageID, RestrictionManage, uuid.Nil); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO page_restrictions (page_id, kind, subject_type, subject_id)
		VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, pageID, kind, sub.Type, sub.ID); err != nil {
		return err
	}
	space, err := spaceOfPage(ctx, tx, pageID)
	if err != nil {
		return err
	}
	return recordChange(ctx, tx, c, "permission.restriction.added", "page", pageID.String(), space, map[string]any{"kind": kind, "subject": sub})
}

// GrantEditor makes someone an editor of a whole space, for a request a space
// administrator allowed. The caller's right to do so is checked here as well.
func (s *Service) GrantEditor(ctx context.Context, tx pgx.Tx, c identity.Caller, spaceID, user uuid.UUID) error {
	if err := s.RequireSpace(ctx, tx, c.UserID, spaceID, SpaceAdminister); err != nil {
		return err
	}
	sub := Subject{Type: SubjectUser, ID: user}
	if err := ApplyTemplateTx(ctx, tx, spaceID, sub, TemplateEditor); err != nil {
		return err
	}
	return recordChange(ctx, tx, c, "permission.space.granted", sub.Type, sub.ID.String(), spaceID.String(), map[string]any{"template": TemplateEditor})
}

func spaceOfPage(ctx context.Context, q db.Querier, pageID uuid.UUID) (string, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx, "SELECT space_id FROM pages WHERE id = $1", pageID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.NotFound()
	}
	return id.String(), err
}

// SetRestrictions replaces the view or edit restriction of a page. An empty list
// lifts it. Users cannot lock themselves out with a view restriction: whoever
// sets one must stay able to view the page.
func (s *Service) SetRestrictions(ctx context.Context, c identity.Caller, pageID uuid.UUID, kind string, subjects []Subject) error {
	if kind != "view" && kind != "edit" {
		return apperr.Invalid("A restriction is view or edit.")
	}
	for _, sub := range subjects {
		if (sub.Type != SubjectUser && sub.Type != SubjectGroup) || sub.ID == uuid.Nil {
			return apperr.Invalid("Only users and groups can be listed on a restriction.")
		}
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.RequirePage(ctx, tx, c.UserID, pageID, RestrictionManage, uuid.Nil); err != nil {
			return err
		}
		for _, sub := range subjects {
			if err := s.subjectExists(ctx, tx, sub); err != nil {
				return err
			}
		}
		spaceID, err := spaceOfPage(ctx, tx, pageID)
		if err != nil {
			return err
		}
		if subjects, err = s.withCaller(ctx, tx, c, subjects); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM page_restrictions WHERE page_id = $1 AND kind = $2", pageID, kind); err != nil {
			return err
		}
		names := make([]string, 0, len(subjects))
		for _, sub := range subjects {
			if _, err := tx.Exec(ctx, `INSERT INTO page_restrictions (page_id, kind, subject_type, subject_id)
				VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, pageID, kind, sub.Type, sub.ID); err != nil {
				return err
			}
			names = append(names, sub.Type+":"+sub.ID.String())
		}
		d, err := s.CheckPage(ctx, tx, c.UserID, pageID, ActionPageView, uuid.Nil)
		if err != nil {
			return err
		}
		if !d.Allowed {
			return apperr.Conflict("locks_you_out", "Add yourself to the list, or you will lose access to this page.")
		}
		action := "restriction.added"
		if len(subjects) == 0 {
			action = "restriction.removed"
		}
		return recordChange(ctx, tx, c, action, "page", pageID.String(), spaceID, map[string]any{"kind": kind, "subjects": strings.Join(names, ",")})
	})
}

// withCaller adds the person who sets a restriction, unless a group already covers
// them, so nobody locks themselves out by forgetting their own name (ACC-065).
func (s *Service) withCaller(ctx context.Context, q db.Querier, c identity.Caller, subjects []Subject) ([]Subject, error) {
	if len(subjects) == 0 {
		return subjects, nil
	}
	u, err := s.LoadUser(ctx, q, c.UserID)
	if err != nil {
		return nil, err
	}
	if u.matchesAny(subjects) {
		return subjects, nil
	}
	return append(subjects, Subject{Type: SubjectUser, ID: c.UserID}), nil
}

// Explanation is the answer to "why can or can't this user?".
type Explanation struct {
	UserID uuid.UUID `json:"user_id"`
	Action string    `json:"action"`
	Decision
}

// Explain evaluates an action for another user, through the same function as
// the real check. Who may ask: space admins, and administrators.
func (s *Service) Explain(ctx context.Context, c identity.Caller, pageID, userID uuid.UUID, action string, isAdministrator bool) (Explanation, error) {
	valid := slices.Contains(AllSpacePermissions, action) || action == ActionPageView || action == ActionPageDelete || action == ActionAttachmentDelete
	if !valid {
		return Explanation{}, apperr.Invalid("Unknown action.")
	}
	if !isAdministrator {
		if err := s.RequirePage(ctx, s.pool, c.UserID, pageID, SpaceAdminister, uuid.Nil); err != nil {
			return Explanation{}, err
		}
	}
	d, err := s.CheckPage(ctx, s.pool, userID, pageID, action, uuid.Nil)
	if err != nil {
		return Explanation{}, err
	}
	return Explanation{UserID: userID, Action: action, Decision: d}, nil
}

// RecoverSpace gives an administrator space admin rights, for a space that lost
// its people. The route requires administrator. It is always audited.
func (s *Service) RecoverSpace(ctx context.Context, c identity.Caller, spaceID uuid.UUID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 500 {
		return apperr.Invalid("Give a reason of at most 500 characters.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := s.LoadSpace(ctx, tx, spaceID); err != nil {
			return err
		}
		if err := ApplyTemplateTx(ctx, tx, spaceID, Subject{Type: SubjectUser, ID: c.UserID}, TemplateSpaceAdmin); err != nil {
			return err
		}
		return recordChange(ctx, tx, c, "access.recovered", "space", spaceID.String(), spaceID.String(), map[string]any{"reason": reason})
	})
}

// RecoverPage adds an administrator to every view restriction on a page and its
// ancestors, and to the edit restriction on the page. The route requires
// administrator. It is always audited.
func (s *Service) RecoverPage(ctx context.Context, c identity.Caller, pageID uuid.UUID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 500 {
		return apperr.Invalid("Give a reason of at most 500 characters.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		spaceID, err := spaceOfPage(ctx, tx, pageID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO page_restrictions (page_id, kind, subject_type, subject_id)
			SELECT DISTINCT r.page_id, r.kind, 'user', $2::uuid
			FROM page_tree t JOIN page_restrictions r ON r.page_id = t.ancestor_id
			WHERE t.descendant_id = $1 AND (r.kind = 'view' OR t.depth = 0)
			ON CONFLICT DO NOTHING`, pageID, c.UserID); err != nil {
			return err
		}
		return recordChange(ctx, tx, c, "access.recovered", "page", pageID.String(), spaceID, map[string]any{"reason": reason})
	})
}
