// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/sites"
	"github.com/diffluence/diffluence/internal/spaces"
)

// Space kinds that cannot be in a pair: a personal space has one writer and no audience
// (PUB-1049).
const kindPersonal = "personal"

// maxNamesShown bounds the names a preview shows of those who lose editing (PUB-210).
const maxNamesShown = 50

// SpaceRef names a space, for those who may see it.
type SpaceRef struct {
	ID   uuid.UUID `json:"id"`
	Key  string    `json:"key"`
	Name string    `json:"name"`
}

// LinkInput chooses the published space: an existing empty space, or a new one (PUB-211).
type LinkInput struct {
	PublishedSpaceID *uuid.UUID `json:"published_space_id"`
	NewSpace         *NewSpace  `json:"new_space"`
}

// NewSpace is the name and key of a published space made for the pair.
type NewSpace struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

func (in LinkInput) check() error {
	if (in.PublishedSpaceID == nil) == (in.NewSpace == nil) {
		return apperr.Invalid("Choose an existing empty space, or name a new one.")
	}
	return nil
}

// LosingEditors counts who will no longer edit the published space, by kind, and names them
// up to fifty (PUB-210).
type LosingEditors struct {
	People   int      `json:"people"`
	Groups   int      `json:"groups"`
	Everyone bool     `json:"everyone"`
	Names    []string `json:"names"`
}

// LinkPreview says what linking would change, before anyone confirms it (PUB-210, PUB-610,
// PUB-611).
type LinkPreview struct {
	LosingEditors LosingEditors `json:"losing_editors"`
	// DraftsReadable warns that people who read the published space can read the drafts too.
	DraftsReadable bool `json:"drafts_readable"`
	// Sites and Links are the public sites and links of the draft space that will go.
	Sites    int  `json:"sites"`
	Links    int  `json:"links"`
	NewSpace bool `json:"new_space"`
}

// writePermissions are the permissions whose holders lose editing in a published space.
func writePermissions() []string {
	out := []string{}
	for _, p := range permissions.AllSpacePermissions {
		if permissions.WritesContent(p) {
			out = append(out, p)
		}
	}
	return out
}

// requireLinkRights checks the rights to link a draft space with the chosen published space:
// space.administer in both, or the right to create a space for a new one (PUB-011, PUB-210).
func (s *Service) requireLinkRights(ctx context.Context, q db.Querier, c identity.Caller, draft uuid.UUID, in LinkInput) error {
	// The right first, so nobody learns more than that they may not (deny by default).
	if err := s.perms.RequireSpace(ctx, q, c.UserID, draft, permissions.SpaceAdminister); err != nil {
		return err
	}
	if err := in.check(); err != nil {
		return err
	}
	if in.PublishedSpaceID != nil {
		return s.perms.RequireSpace(ctx, q, c.UserID, *in.PublishedSpaceID, permissions.SpaceAdminister)
	}
	u, err := s.perms.LoadUser(ctx, q, c.UserID)
	if err != nil {
		return err
	}
	if !u.Active || !slices.Contains(u.Global, identity.PermCreateSpace) {
		return apperr.Forbidden()
	}
	return nil
}

// checkSides refuses a pair that would break the model: a second pair on either side, a
// chain, a pair with itself, a personal space, or a space not in use (PUB-200, PUB-1049 to
// PUB-1052).
func checkSides(draft spaces.Space, published *spaces.Space) error {
	if draft.Kind == kindPersonal || (published != nil && published.Kind == kindPersonal) {
		return apperr.Conflict("personal_space", "A personal space cannot be in a pair.")
	}
	if draft.Status != spaces.StatusActive || (published != nil && published.Status != spaces.StatusActive) {
		return apperr.Conflict("space_not_active", "Both spaces must be in use: archived spaces and spaces in the trash cannot be linked.")
	}
	if draft.PairRole != "" {
		return apperr.Conflict("already_paired", "This space is already in a pair. A space is in one pair at most, on one side.")
	}
	if published == nil {
		return nil
	}
	if published.ID == draft.ID {
		return apperr.Conflict("self_pair", "A space cannot be linked to itself.")
	}
	if published.PairRole == permissions.PairPublished {
		return apperr.Conflict("already_published", "That space is already the published space of another draft space. "+
			"Merge the two draft spaces first, or keep two published spaces and link between them.")
	}
	if published.PairRole == permissions.PairDraft {
		return apperr.Conflict("already_paired", "That space is the draft space of another pair. A space is never both draft and published.")
	}
	return nil
}

// checkEmpty refuses a published space with content, and says what is in it (PUB-211).
func checkEmpty(ctx context.Context, q db.Querier, space spaces.Space) error {
	found, err := pages.ContentInTheWay(ctx, q, space.ID, space.HomePageID)
	if err != nil {
		return err
	}
	files, err := attachments.CountInSpace(ctx, q, space.ID)
	if err != nil {
		return err
	}
	if files > 0 {
		found["files"] = files
	}
	if len(found) == 0 {
		return nil
	}
	keys := make([]string, 0, len(found))
	for key := range found {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	words := map[string]string{"pages": "pages", "posts": "blog posts", "other_items": "folders, links or databases",
		"files": "files", "home_versions": "versions of its home page"}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%d %s", found[key], words[key]))
	}
	return apperr.Conflict("published_space_not_empty", "The published space must be empty. It holds "+strings.Join(parts, ", ")+
		". Choose an empty space, or make a new one.")
}

// PreviewLink says what linking would do, and refuses as Link would (PUB-210).
func (s *Service) PreviewLink(ctx context.Context, c identity.Caller, draftID uuid.UUID, in LinkInput) (LinkPreview, error) {
	out := LinkPreview{LosingEditors: LosingEditors{Names: []string{}}}
	if err := s.requireLinkRights(ctx, s.pool, c, draftID, in); err != nil {
		return out, err
	}
	draft, err := spaces.LoadIn(ctx, s.pool, draftID)
	if err != nil {
		return out, err
	}
	var published *spaces.Space
	if in.PublishedSpaceID != nil {
		sp, err := spaces.LoadIn(ctx, s.pool, *in.PublishedSpaceID)
		if err != nil {
			return out, err
		}
		published = &sp
	}
	if err := checkSides(draft, published); err != nil {
		return out, err
	}
	if published == nil {
		out.NewSpace = true
	} else {
		if err := checkEmpty(ctx, s.pool, *published); err != nil {
			return out, err
		}
		holders, err := s.perms.HoldersOf(ctx, s.pool, published.ID, writePermissions())
		if err != nil {
			return out, err
		}
		out.LosingEditors = losing(holders)
	}
	if out.DraftsReadable, err = s.draftsReadable(ctx, s.pool, draft.ID, published); err != nil {
		return out, err
	}
	if out.Sites, out.Links, err = sites.LiveInSpace(ctx, s.pool, draft.ID); err != nil {
		return out, err
	}
	return out, nil
}

func losing(holders []permissions.NamedSubject) LosingEditors {
	out := LosingEditors{Names: []string{}}
	for _, h := range holders {
		switch h.Type {
		case permissions.SubjectUser:
			out.People++
		case permissions.SubjectGroup:
			out.Groups++
		default:
			out.Everyone = true
			continue
		}
		if len(out.Names) < maxNamesShown && h.Name != "" {
			out.Names = append(out.Names, h.Name)
		}
	}
	return out
}

// draftsReadable says whether the readers of the published space can read the draft space
// as well: the draft space is open to all members or all guests, or a group reads both
// (PUB-611). It is a warning, never a refusal.
func (s *Service) draftsReadable(ctx context.Context, q db.Querier, draft uuid.UUID, published *spaces.Space) (bool, error) {
	readers, err := s.perms.HoldersOf(ctx, q, draft, []string{permissions.SpaceView})
	if err != nil {
		return false, err
	}
	var publishedGroups []uuid.UUID
	if published != nil {
		outside, err := s.perms.HoldersOf(ctx, q, published.ID, []string{permissions.SpaceView})
		if err != nil {
			return false, err
		}
		for _, h := range outside {
			if h.Type == permissions.SubjectGroup {
				publishedGroups = append(publishedGroups, h.ID)
			}
		}
	}
	for _, h := range readers {
		switch {
		case h.Type == permissions.SubjectAllMembers, h.Type == permissions.SubjectAllGuests:
			return true, nil
		case h.Type == permissions.SubjectGroup && slices.Contains(publishedGroups, h.ID):
			return true, nil
		}
	}
	return false, nil
}

// Link makes a pair, in one transaction (PUB-210). Linking makes no release (PUB-214): the
// two home pages are paired, and the published space is empty until the first release.
func (s *Service) Link(ctx context.Context, c identity.Caller, draftID uuid.UUID, in LinkInput) (State, error) {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.requireLinkRights(ctx, tx, c, draftID, in); err != nil {
			return err
		}
		var published spaces.Space
		if in.NewSpace != nil {
			var err error
			// A new space gives view to no one but its maker (PUB-211).
			if published, err = s.spaces.CreateIn(ctx, tx, c, spaces.Input{Key: in.NewSpace.Key, Name: in.NewSpace.Name}); err != nil {
				return err
			}
		} else {
			published.ID = *in.PublishedSpaceID
		}
		if err := spaces.LockForPairing(ctx, tx, draftID, published.ID); err != nil {
			return err
		}
		draft, err := spaces.LoadIn(ctx, tx, draftID)
		if err != nil {
			return err
		}
		if published, err = spaces.LoadIn(ctx, tx, published.ID); err != nil {
			return err
		}
		if err := checkSides(draft, &published); err != nil {
			return err
		}
		if err := checkEmpty(ctx, tx, published); err != nil {
			return err
		}
		return s.makePair(ctx, tx, c, draft, published)
	})
	if err != nil {
		return State{}, err
	}
	return s.State(ctx, c, draftID)
}

func (s *Service) makePair(ctx context.Context, tx pgx.Tx, c identity.Caller, draft, published spaces.Space) error {
	id := ids.New()
	settings := defaultSettings()
	_, err := tx.Exec(ctx, `INSERT INTO space_pairs (id, draft_space_id, published_space_id, settings, created_by)
		VALUES ($1, $2, $3, $4, $5)`, id, draft.ID, published.ID, settings, c.UserID)
	if db.IsUniqueViolation(err) {
		return apperr.Conflict("already_paired", "One of the two spaces was linked a moment ago.")
	}
	if err != nil {
		return err
	}
	if err := spaces.SetPairRole(ctx, tx, draft.ID, permissions.PairDraft); err != nil {
		return err
	}
	if err := spaces.SetPairRole(ctx, tx, published.ID, permissions.PairPublished); err != nil {
		return err
	}
	// The two home pages pair now, and the published one gets its content at the first
	// release (PUB-213).
	if draft.HomePageID != nil && published.HomePageID != nil {
		// A home page in an ended pair of this draft space starts again in this one.
		if _, err := tx.Exec(ctx, `INSERT INTO page_pairs (draft_page_id, pair_id, twin_page_id) VALUES ($1, $2, $3)
			ON CONFLICT (draft_page_id) DO UPDATE SET pair_id = EXCLUDED.pair_id, twin_page_id = EXCLUDED.twin_page_id,
				released_version = 0, twin_version = 0, released_at = NULL, released_by = NULL, run_id = NULL, released_title = '',
				released_parent_id = NULL, released_look = '{}', released_labels = '{}', released_hash = '', text_links = '{}',
				changed = '{}', changed_since = NULL`,
			*draft.HomePageID, id, *published.HomePageID); err != nil {
			return err
		}
	}
	// The draft space holds the record with both sides; the published space learns that it
	// is managed, without the draft's name or id (PUB-900, PUB-901).
	if _, err := audit.Record(ctx, tx, c.Event("space.paired", "space", draft.ID.String(), draft.ID.String(),
		map[string]any{"pair_id": id, "draft_space_id": draft.ID, "published_space_id": published.ID})); err != nil {
		return err
	}
	if _, err := audit.Record(ctx, tx, c.Event("space.managed", "space", published.ID.String(), published.ID.String(),
		map[string]any{"pair_id": id})); err != nil {
		return err
	}
	// Public sites and links of the draft space are taken down by the rule of public sites,
	// which this event wakes (PUB-610).
	_, err = outbox.Enqueue(ctx, tx, "space.paired", map[string]any{"space_id": draft.ID, "pair_id": id, "published_space_id": published.ID})
	return err
}

// End ends a pair: the published space becomes an ordinary space its editors can change
// again, and the draft space stays as it is (PUB-243). It needs space.administer in both
// spaces, and stays open while the experimental part is off (PUB-1304).
func (s *Service) End(ctx context.Context, c identity.Caller, spaceID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
			return err
		}
		p, err := activePairOf(ctx, tx, spaceID, true)
		if err != nil {
			return err
		}
		for _, side := range []uuid.UUID{p.Draft, p.Published} {
			if err := s.perms.RequireSpace(ctx, tx, c.UserID, side, permissions.SpaceAdminister); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, "UPDATE space_pairs SET ended_at = now(), ended_by = $2 WHERE id = $1", p.ID, c.UserID); err != nil {
			return err
		}
		for _, side := range []uuid.UUID{p.Draft, p.Published} {
			if err := spaces.SetPairRole(ctx, tx, side, ""); err != nil {
				return err
			}
		}
		if _, err := audit.Record(ctx, tx, c.Event("space.unpaired", "space", p.Draft.String(), p.Draft.String(),
			map[string]any{"pair_id": p.ID, "draft_space_id": p.Draft, "published_space_id": p.Published})); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("space.unmanaged", "space", p.Published.String(), p.Published.String(),
			map[string]any{"pair_id": p.ID})); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "space.unpaired", map[string]any{"space_id": p.Draft, "pair_id": p.ID})
		return err
	})
}

// SetSettings changes the settings of a pair, from the draft space (PUB-240).
func (s *Service) SetSettings(ctx context.Context, c identity.Caller, spaceID uuid.UUID, in Settings) (Settings, error) {
	in, err := in.clean()
	if err != nil {
		return in, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
			return err
		}
		p, err := activePairOf(ctx, tx, spaceID, true)
		if err != nil {
			return err
		}
		if p.Draft != spaceID {
			return apperr.Conflict("not_the_draft_space", "The settings of a pair are changed in its draft space.")
		}
		if _, err := tx.Exec(ctx, "UPDATE space_pairs SET settings = $2 WHERE id = $1", p.ID, in); err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, c.Event("space.pair_settings_changed", "space", spaceID.String(), spaceID.String(),
			map[string]any{"pair_id": p.ID, "old": p.Settings, "new": in}))
		return err
	})
	return in, err
}

// State is what the settings section shows of a space and its pair (PUB-1200). Each side is
// named only to those who may view it.
type State struct {
	Side string     `json:"side"`
	Pair *PairState `json:"pair"`
	Can  StateCan   `json:"can"`
}

// StateCan says which actions the section offers this person.
type StateCan struct {
	Link     bool `json:"link"`
	End      bool `json:"end"`
	Settings bool `json:"settings"`
	Release  bool `json:"release"`
}

// PairState is a pair as one person may see it.
type PairState struct {
	ID        uuid.UUID                  `json:"id"`
	CreatedAt time.Time                  `json:"created_at"`
	Draft     *SpaceRef                  `json:"draft"`
	Published *SpaceRef                  `json:"published"`
	Settings  *Settings                  `json:"settings"`
	Releasers []permissions.NamedSubject `json:"releasers"`
	Health    *Health                    `json:"health"`
}

// Health is the health line of a pair in its draft space (PUB-241).
type Health struct {
	DraftsReadable bool       `json:"drafts_readable"`
	OutOfStep      int        `json:"out_of_step"`
	Since          *time.Time `json:"since"`
}

// State reads a space's side of a pair, for anyone who may view the space.
func (s *Service) State(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (State, error) {
	out := State{}
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceView); err != nil {
		return out, err
	}
	may := func(space uuid.UUID, action string) (bool, error) {
		d, err := s.perms.CheckSpace(ctx, s.pool, c.UserID, space, action)
		if apperr.IsNotFound(err) {
			return false, nil
		}
		return d.Allowed, err
	}
	administers, err := may(spaceID, permissions.SpaceAdminister)
	if err != nil {
		return out, err
	}
	p, err := activePairOf(ctx, s.pool, spaceID, false)
	if hasCode(err, codeNotPaired) {
		out.Can.Link = administers
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Side = permissions.PairDraft
	if p.Published == spaceID {
		out.Side = permissions.PairPublished
	}
	view := &PairState{ID: p.ID, CreatedAt: p.CreatedAt}
	for _, side := range []struct {
		id  uuid.UUID
		ref **SpaceRef
	}{{p.Draft, &view.Draft}, {p.Published, &view.Published}} {
		seen, err := may(side.id, permissions.SpaceView)
		if err != nil {
			return out, err
		}
		if seen {
			sp, err := spaces.LoadIn(ctx, s.pool, side.id)
			if err != nil {
				return out, err
			}
			*side.ref = &SpaceRef{ID: sp.ID, Key: sp.Key, Name: sp.Name}
		}
	}
	otherAdmin, err := may(otherSide(p, spaceID), permissions.SpaceAdminister)
	if err != nil {
		return out, err
	}
	out.Can.End = administers && otherAdmin
	if out.Side == permissions.PairDraft {
		if out.Can.Release, err = may(spaceID, permissions.SpaceRelease); err != nil {
			return out, err
		}
		if administers {
			out.Can.Settings = true
			settings := p.Settings
			view.Settings = &settings
			if view.Releasers, err = s.perms.HoldersOf(ctx, s.pool, p.Draft, []string{permissions.SpaceRelease}); err != nil {
				return out, err
			}
		}
		if administers || out.Can.Release {
			if view.Health, err = s.health(ctx, p); err != nil {
				return out, err
			}
		}
	}
	out.Pair = view
	return out, nil
}

func otherSide(p pair, spaceID uuid.UUID) uuid.UUID {
	if p.Draft == spaceID {
		return p.Published
	}
	return p.Draft
}

func (s *Service) health(ctx context.Context, p pair) (*Health, error) {
	h := &Health{}
	published, err := spaces.LoadIn(ctx, s.pool, p.Published)
	if err != nil {
		return nil, err
	}
	if h.DraftsReadable, err = s.draftsReadable(ctx, s.pool, p.Draft, &published); err != nil {
		return nil, err
	}
	err = s.pool.QueryRow(ctx, `SELECT count(*), min(changed_since) FROM page_pairs WHERE pair_id = $1 AND cardinality(changed) > 0`, p.ID).
		Scan(&h.OutOfStep, &h.Since)
	if errors.Is(err, pgx.ErrNoRows) {
		return h, nil
	}
	return h, err
}
