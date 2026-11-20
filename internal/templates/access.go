// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/db"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// The states of a template and of its versions (spec 074, TPL-510, TPL-602).
const (
	StateDraft      = "draft"
	StateWaiting    = "waiting"
	StateApproved   = "approved"
	StateSentBack   = "sent_back"
	StateWithdrawn  = "withdrawn"
	StateSuperseded = "superseded"
)

// The two audience lists of a template (TPL-500).
const (
	RightUse  = "use"
	RightEdit = "edit"
)

// standing is what one person is to one template: every fact a decision about it needs, read
// once. The lists only narrow: being on one gives nothing that a right of the space does not
// give already (TPL-501, TPL-1415).
type standing struct {
	t        Template
	userID   uuid.UUID
	groups   []uuid.UUID
	reader   bool // may read its space, or anyone for a template of all spaces
	manager  bool // space.templates in its space, or administrator for a template of all spaces
	proposer bool // template.propose in its space
	// administer holds space.administer in its space, who may always clear the lists (TPL-501).
	administer bool
	owner      bool
	// editListed and useListed: on the list, or the list is empty.
	editListed, useListed bool
	// rights are the lists as stored.
	rights Rights
}

// manages says the person looks after this template: they manage its scope and its editors
// list lets them, or they are an administrator and it is of all spaces, which no list can
// lock away (TPL-501).
func (st standing) manages() bool {
	return st.manager && (st.editListed || st.t.SpaceID == nil)
}

// mayEdit: whoever manages it, and its owner while they may still propose templates of its
// space and its editors list does not leave them out (TPL-061, TPL-512).
func (st standing) mayEdit() bool {
	return st.manages() || (st.owner && st.proposer && st.editListed && st.t.SpaceID != nil)
}

// mayLook is reading what a template holds beyond what using it shows: its pages, its
// questions and its history. An owner who may no longer read its space reads none of it.
func (st standing) mayLook() bool { return st.mayEdit() || (st.owner && st.reader) }

// seenByManagers: a draft that was never proposed is its owner's own, until the owner leaves
// (TPL-510, TPL-853).
func (st standing) seenByManagers(proposed, ownerActive bool) bool {
	return st.t.TemplateVersion > 0 || st.t.State != StateDraft || proposed || !ownerActive
}

// offeredToReader says a person who does not look after the template sees it: it has an
// approved version, and they may read where it lives and are on its users list (TPL-504).
func (st standing) offeredToReader() bool {
	return st.t.TemplateVersion > 0 && st.reader && st.useListed
}

// matches says a person is one of subjects, by themselves or through a group.
func matches(userID uuid.UUID, groups []uuid.UUID, subjects []permissions.Subject) bool {
	return slices.ContainsFunc(subjects, func(sub permissions.Subject) bool {
		return (sub.Type == permissions.SubjectUser && sub.ID == userID) ||
			(sub.Type == permissions.SubjectGroup && slices.Contains(groups, sub.ID))
	})
}

// standingOf reads what a person is to a template.
func (s *Service) standingOf(ctx context.Context, q db.Querier, c identity.Caller, t Template) (standing, error) {
	st := standing{t: t, userID: c.UserID, owner: t.ownerID == c.UserID}
	user, err := s.perms.LoadUser(ctx, q, c.UserID)
	if err != nil {
		return st, err
	}
	st.groups = user.Groups
	if t.SpaceID == nil {
		st.reader = user.Active
		if st.manager, err = s.identity.HasGlobalPermissionIn(ctx, q, c.UserID, identity.PermAdministrator); err != nil {
			return st, err
		}
	} else {
		space, err := s.perms.LoadSpace(ctx, q, *t.SpaceID)
		if err != nil && !apperr.IsNotFound(err) {
			return st, err
		}
		holds := func(action string) bool {
			return err == nil && permissions.Evaluate(permissions.Input{User: user, Space: space, Action: action}).Allowed
		}
		st.reader, st.manager = holds(permissions.SpaceView), holds(permissions.SpaceTemplates)
		st.proposer, st.administer = holds(permissions.TemplatePropose), holds(permissions.SpaceAdminister)
	}
	if st.rights, err = rightsOf(ctx, q, t.ID); err != nil {
		return st, err
	}
	st.editListed = len(st.rights.Edit) == 0 || matches(c.UserID, user.Groups, st.rights.Edit)
	st.useListed = len(st.rights.Use) == 0 || matches(c.UserID, user.Groups, st.rights.Use)
	return st, nil
}

// maySee says whether a person may know the template exists at all. Who may not is told it
// is not found, never that it is forbidden, because a name can say too much (TPL-504).
func (s *Service) maySee(ctx context.Context, q db.Querier, st standing) (bool, error) {
	if (st.owner && st.reader) || st.offeredToReader() {
		return true, nil
	}
	if !st.manages() {
		return false, nil
	}
	var proposed bool
	err := q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM template_reviews WHERE template_id = $1 AND decision = 'proposed')",
		st.t.ID).Scan(&proposed)
	if err != nil {
		return false, err
	}
	owner, err := s.perms.LoadUser(ctx, q, st.t.ownerID)
	if err != nil {
		return false, err
	}
	return st.seenByManagers(proposed, owner.Active), nil
}

// refuse answers a person who may not do what they asked: forbidden when they may see the
// template, and not found when they may not (TPL-504).
func (s *Service) refuse(ctx context.Context, q db.Querier, st standing) error {
	seen, err := s.maySee(ctx, q, st)
	if err != nil {
		return err
	}
	if seen {
		return apperr.Forbidden()
	}
	return apperr.NotFound()
}

// requireTo loads what a person is to a template and refuses unless allowed says they may.
func (s *Service) requireTo(ctx context.Context, q db.Querier, c identity.Caller, t Template, allowed func(standing) bool) (standing, error) {
	st, err := s.standingOf(ctx, q, c, t)
	if err != nil {
		return st, err
	}
	if !allowed(st) {
		return st, s.refuse(ctx, q, st)
	}
	return st, nil
}

// fourEyesOn says whether every template change in a scope needs a second person: a space,
// or the templates of all spaces when spaceID is nil (TPL-511).
func fourEyesOn(ctx context.Context, q db.Querier, spaceID *uuid.UUID) (bool, error) {
	var on bool
	err := q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM template_four_eyes WHERE space_id IS NOT DISTINCT FROM $1)", spaceID).Scan(&on)
	return on, err
}

// publishesDirectly says a version this person saves is approved at once: they manage the
// template, and its scope does not ask for a second person (TPL-511, TPL-1411).
func publishesDirectly(ctx context.Context, q db.Querier, st standing) (bool, error) {
	if !st.manages() {
		return false, nil
	}
	on, err := fourEyesOn(ctx, q, st.t.SpaceID)
	return !on, err
}
