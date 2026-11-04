// SPDX-License-Identifier: Apache-2.0

// Package permissions is the single place where access decisions are made
// (docs/blueprint/05, spec 002). Evaluate is a pure function over facts, so it
// can be tested exhaustively; Service loads those facts from the database.
package permissions

import (
	"slices"

	"github.com/google/uuid"
)

// Global permissions a user needs before anything else counts.
const globalUseProduct = "use_product"

// Space permissions (layer 2).
const (
	SpaceView       = "space.view"
	SpaceExport     = "space.export"
	SpaceAdminister = "space.administer"

	PageCreate    = "page.create"
	PageEdit      = "page.edit"
	PageArchive   = "page.archive"
	PageDeleteOwn = "page.delete_own"
	PageDeleteAny = "page.delete_any"

	CommentAdd       = "comment.add"
	CommentDeleteOwn = "comment.delete_own"
	CommentDeleteAny = "comment.delete_any"

	AttachmentAdd       = "attachment.add"
	AttachmentDeleteOwn = "attachment.delete_own"
	AttachmentDeleteAny = "attachment.delete_any"

	RestrictionManage = "restriction.manage"
)

// AllSpacePermissions lists every space permission.
var AllSpacePermissions = []string{
	SpaceView, SpaceExport, SpaceAdminister,
	PageCreate, PageEdit, PageArchive, PageDeleteOwn, PageDeleteAny,
	CommentAdd, CommentDeleteOwn, CommentDeleteAny,
	AttachmentAdd, AttachmentDeleteOwn, AttachmentDeleteAny,
	RestrictionManage,
}

// Actions that are not permissions themselves.
const (
	ActionPageView         = "page.view"
	ActionPageDelete       = "page.delete"       // own or any
	ActionAttachmentDelete = "attachment.delete" // own or any
)

// Subject types.
const (
	SubjectUser       = "user"
	SubjectGroup      = "group"
	SubjectAllMembers = "all_members"
	SubjectAllGuests  = "all_guests"
)

// Subject is who a grant or restriction applies to. ID is uuid.Nil for
// all_members and all_guests.
type Subject struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
}

// Grant is one space permission held by a subject.
type Grant struct {
	Subject    Subject
	Permission string
}

// UserFacts is what evaluation needs to know about a user.
type UserFacts struct {
	ID     uuid.UUID
	Type   string // member, guest, service
	Active bool
	Groups []uuid.UUID
	Global []string
}

// SpaceFacts is what evaluation needs to know about a space.
type SpaceFacts struct {
	ID     uuid.UUID
	Status string // active, archived, trashed
	Grants []Grant
}

// RestrictionLevel is a page in the ancestor chain that has a view restriction.
type RestrictionLevel struct {
	PageID   uuid.UUID
	Subjects []Subject
}

// PageFacts is what evaluation needs to know about a page.
type PageFacts struct {
	ID        uuid.UUID
	Status    string // draft, published, trashed
	CreatedBy uuid.UUID
	// ViewChain holds the page and its ancestors that carry a view restriction.
	ViewChain []RestrictionLevel
	// EditSubjects is the edit restriction on the page itself (empty = none).
	EditSubjects []Subject
}

// Input is one question: may this user do this?
type Input struct {
	User   UserFacts
	Space  SpaceFacts
	Page   *PageFacts // nil for space-level actions
	Action string
	Owner  uuid.UUID // creator of the object, for "delete own"
}

// Check is one step of an evaluation, for the "why can or can't I?" view.
type Check struct {
	Layer  string `json:"layer"`
	Rule   string `json:"rule"`
	Passed bool   `json:"passed"`
}

// Layers of the model, as shown in explanations.
const (
	LayerUser        = "user"
	LayerGlobal      = "global"
	LayerSpace       = "space"
	LayerPage        = "page"
	LayerRestriction = "restriction"
)

// Decision is the result of an evaluation.
type Decision struct {
	Allowed bool `json:"allowed"`
	// Visible says whether the user may see the object at all. When false the
	// API answers "not found", so restricted objects do not reveal themselves.
	Visible bool    `json:"visible"`
	Checks  []Check `json:"checks"`
}

// isEdit reports whether an edit restriction on the page applies to action.
func isEdit(action string) bool {
	switch action {
	case PageEdit, PageArchive, ActionPageDelete, AttachmentAdd, ActionAttachmentDelete:
		return true
	}
	return false
}

// worksWhenArchived lists what still works in an archived (read-only) space.
func worksWhenArchived(action string) bool {
	switch action {
	case SpaceView, ActionPageView, SpaceExport, SpaceAdminister:
		return true
	}
	return false
}

func (u UserFacts) matches(s Subject) bool {
	switch s.Type {
	case SubjectUser:
		return s.ID == u.ID
	case SubjectGroup:
		return slices.Contains(u.Groups, s.ID)
	case SubjectAllMembers:
		return u.Type == "member"
	case SubjectAllGuests:
		return u.Type == "guest"
	}
	return false
}

func (u UserFacts) matchesAny(subjects []Subject) bool {
	return slices.ContainsFunc(subjects, u.matches)
}

func holds(in Input, permission string) bool {
	return slices.ContainsFunc(in.Space.Grants, func(g Grant) bool {
		return g.Permission == permission && in.User.matches(g.Subject)
	})
}

// actionAllowedBySpace applies layer 2 for one action.
func actionAllowedBySpace(in Input) (string, bool) {
	switch in.Action {
	case ActionPageView:
		return SpaceView, holds(in, SpaceView)
	case ActionPageDelete:
		return PageDeleteAny, holds(in, PageDeleteAny) || (in.Owner == in.User.ID && holds(in, PageDeleteOwn))
	case ActionAttachmentDelete:
		return AttachmentDeleteAny, holds(in, AttachmentDeleteAny) || (in.Owner == in.User.ID && holds(in, AttachmentDeleteOwn))
	}
	return in.Action, holds(in, in.Action)
}

// Evaluate applies the rule from spec 002. Every layer is always evaluated, so
// an explanation shows the complete picture and not only the first refusal.
func Evaluate(in Input) Decision {
	d := Decision{Allowed: true, Visible: true}
	check := func(layer, rule string, passed, affectsVisibility bool) {
		d.Checks = append(d.Checks, Check{Layer: layer, Rule: rule, Passed: passed})
		if !passed {
			d.Allowed = false
			if affectsVisibility {
				d.Visible = false
			}
		}
	}

	check(LayerUser, "account_active", in.User.Active, true)
	check(LayerGlobal, globalUseProduct, slices.Contains(in.User.Global, globalUseProduct), true)
	check(LayerSpace, "space_not_trashed", in.Space.Status != "trashed", true)
	check(LayerSpace, SpaceView, holds(in, SpaceView), true)
	if in.Action != SpaceView && in.Action != ActionPageView {
		perm, ok := actionAllowedBySpace(in)
		check(LayerSpace, perm, ok, false)
	}
	if !worksWhenArchived(in.Action) {
		check(LayerSpace, "space_not_archived", in.Space.Status != "archived", false)
	}

	if in.Page == nil {
		return d
	}
	p := in.Page
	check(LayerPage, "page_not_trashed", p.Status != "trashed", true)
	check(LayerPage, "published_or_own_draft", p.Status != "draft" || p.CreatedBy == in.User.ID, true)
	for _, level := range p.ViewChain {
		check(LayerRestriction, "view:"+level.PageID.String(), in.User.matchesAny(level.Subjects), true)
	}
	if isEdit(in.Action) && len(p.EditSubjects) > 0 {
		check(LayerRestriction, "edit:"+p.ID.String(), in.User.matchesAny(p.EditSubjects), false)
	}
	return d
}
