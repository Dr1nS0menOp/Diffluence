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
	// SpaceManageAccess changes who has access. Administer implies it, and someone
	// who only holds this cannot touch administer (spec 013, D3 and D4).
	SpaceManageAccess = "space.manage_access"

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

	// AnalyticsView shows what is read in a space (spec 015, D5).
	AnalyticsView = "analytics.view"

	// SitePublish publishes public sites of a space and makes public links to its
	// pages (spec 036).
	SitePublish = "site.publish"
	// SpaceAutomation looks after the automation rules and webhooks of a space.
	SpaceAutomation = "space.automation"
	// SpaceTemplates makes and changes the templates of a space.
	SpaceTemplates = "space.templates"
	// SpaceAudit reads the audit of a space.
	SpaceAudit = "space.audit"

	// SpaceLabels fixes the labels of a space and renames a label in it (spec 040).
	SpaceLabels = "space.labels"
	// SpaceStatuses sets the page statuses and the review period of a space.
	SpaceStatuses = "space.statuses"
	// SpaceApprovals sets the approval rule of a space and its fixed approvers.
	SpaceApprovals = "space.approvals"
	// SpaceBundle makes a bundle of the whole space, which still needs export too.
	SpaceBundle = "space.bundle"
	// SpaceLook sets the shortcuts, the picture, the title alignment and the home page.
	SpaceLook = "space.look"
	// PageWatchers shows who watches a page, and adds or removes someone else.
	PageWatchers = "page.watchers"
	// PageMoveOut moves a page and the pages below it to another space.
	PageMoveOut = "page.move_out"
	// SpaceInviteGuests asks for a guest in the space. Like manage access, administer
	// implies it; the instance-wide invite_people still decides who may ask at all.
	SpaceInviteGuests = "space.invite_guests"

	// PostCreate writes new blog posts, and turns an unpublished page into one (spec 045).
	PostCreate = "post.create"
	// DatabaseCreate makes new databases in the space, and copies or moves them in (spec 045).
	DatabaseCreate = "database.create"
	// DatabaseEdit changes databases that are already there: their rows, columns and views,
	// and everything page.edit allows on any other item (ADR-040, spec 049). On a database,
	// asking for page.edit asks for this.
	DatabaseEdit = "database.edit"

	// SpaceWorkflows makes, changes and archives the workflows of a space and sets the space
	// workflow (spec 064).
	SpaceWorkflows = "space.workflows"
	// PageWorkflow sets a workflow of the space on a page or a branch, and removes it.
	PageWorkflow = "page.workflow"
)

// Kinds of item that have a create permission of their own (ADR-038). The pages package
// names every kind; these two are spelled here so the rule needs nothing from it.
const (
	kindPost     = "post"
	kindDatabase = "database"
)

// CreatePermission is the permission that makes an item of a kind: blog posts and
// databases have their own, and pages, folders and link entries are made with page.create.
func CreatePermission(kind string) string {
	switch kind {
	case kindPost:
		return PostCreate
	case kindDatabase:
		return DatabaseCreate
	}
	return PageCreate
}

// AllSpacePermissions lists every space permission.
var AllSpacePermissions = []string{
	SpaceView, SpaceExport, SpaceAdminister, SpaceManageAccess,
	PageCreate, PageEdit, PageArchive, PageDeleteOwn, PageDeleteAny,
	CommentAdd, CommentDeleteOwn, CommentDeleteAny,
	AttachmentAdd, AttachmentDeleteOwn, AttachmentDeleteAny,
	RestrictionManage, AnalyticsView,
	SitePublish, SpaceAutomation, SpaceTemplates, SpaceAudit,
	SpaceLabels, SpaceStatuses, SpaceApprovals, SpaceBundle, SpaceLook, PageWatchers,
	PageMoveOut, SpaceInviteGuests, PostCreate, DatabaseCreate, DatabaseEdit,
	SpaceWorkflows, PageWorkflow,
}

// SplitFrom names, for each permission split off a broader one (specs 036 and 040), that
// broader permission. Whoever held the broader one before holds the finer one too, so the
// split moves no access; it only lets a role hold one without the other.
var SplitFrom = map[string]string{
	SitePublish:       SpaceAdminister,
	SpaceAutomation:   SpaceAdminister,
	SpaceTemplates:    SpaceAdminister,
	SpaceAudit:        SpaceAdminister,
	SpaceLabels:       SpaceAdminister,
	SpaceStatuses:     SpaceAdminister,
	SpaceApprovals:    SpaceAdminister,
	SpaceBundle:       SpaceAdminister,
	SpaceLook:         SpaceAdminister,
	PageWatchers:      SpaceAdminister,
	PageMoveOut:       PageEdit,
	SpaceInviteGuests: SpaceManageAccess,
	PostCreate:        PageCreate,
	DatabaseCreate:    PageCreate,
	DatabaseEdit:      PageEdit,
	SpaceWorkflows:    SpaceAdminister,
	PageWorkflow:      PageEdit,
}

// splitOffAdminister reports whether a permission was split off space administration, so
// it works wherever administer worked.
func splitOffAdminister(action string) bool {
	return SplitFrom[action] == SpaceAdminister
}

// Actions that are not permissions themselves.
const (
	ActionPageView = "page.view"
	// ActionApprovalRead is view for someone named in an approval round. It is the
	// rule without the part on unpublished pages, because they were asked.
	ActionApprovalRead     = "approval.read"
	ActionPageDelete       = "page.delete"       // own or any
	ActionAttachmentDelete = "attachment.delete" // own or any
	ActionCommentDelete    = "comment.delete"    // own or any
	ActionReact            = "reaction.set"      // needs view, and changes what others see
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
	Kind      string // page, post, folder, link, database, template
	Archived  bool
	// ViewChain holds the page and its ancestors that carry a view restriction.
	ViewChain []RestrictionLevel
	// EditSubjects is the edit restriction on the page itself (empty = none).
	EditSubjects []Subject
	// ControlHidden marks a controlled page that was never approved or was withdrawn: only
	// its authors may see it (spec 064, DOC-300 to DOC-322).
	ControlHidden bool
	// Reviewer says the person is asked to review the page now: an active reviewer of the
	// open round, in the stage the page is in. It is loaded for approval.read only, where it
	// opens this page past its restrictions and past "never approved" (DOC-203).
	Reviewer bool
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
	case PageWorkflow:
		return true
	case PageEdit, PageArchive, ActionPageDelete, AttachmentAdd, ActionAttachmentDelete, PageMoveOut, DatabaseEdit:
		return true
	}
	return false
}

// spaceEditPermission is the space permission that edits an item: database.edit for a
// database, page.edit for every other kind (ADR-040). Modules keep asking for page.edit,
// so no route can forget the database.
func spaceEditPermission(p *PageFacts) string {
	if p != nil && p.Kind == kindDatabase {
		return DatabaseEdit
	}
	return PageEdit
}

// KindTemplate marks a hidden page that holds a template. It is never a page to
// anyone: the templates module reaches it through its own unchecked calls.
const KindTemplate = "template"

// worksOnArchivedPage lists what still works on a page in the archive: reading,
// export and delete, restoring it, and who may see it. What spec 036 split off
// administer works where administer does.
func worksOnArchivedPage(action string) bool {
	switch action {
	case ActionPageView, ActionApprovalRead, SpaceExport, SpaceAdminister, ActionPageDelete, PageArchive, RestrictionManage, AnalyticsView:
		return true
	}
	return splitOffAdminister(action)
}

// worksWhenArchived lists what still works in an archived (read-only) space.
func worksWhenArchived(action string) bool {
	switch action {
	case SpaceView, ActionPageView, ActionApprovalRead, SpaceExport, SpaceAdminister, SpaceManageAccess, AnalyticsView,
		SpaceInviteGuests:
		return true
	}
	return splitOffAdminister(action)
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
	case SpaceManageAccess:
		return SpaceManageAccess, holds(in, SpaceManageAccess) || holds(in, SpaceAdminister)
	case SpaceInviteGuests:
		return SpaceInviteGuests, holds(in, SpaceInviteGuests) || holds(in, SpaceAdminister)
	case ActionCommentDelete:
		return CommentDeleteAny, holds(in, CommentDeleteAny) || (in.Owner == in.User.ID && holds(in, CommentDeleteOwn))
	case PageEdit:
		perm := spaceEditPermission(in.Page)
		return perm, holds(in, perm)
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
	if in.Action != SpaceView && in.Action != ActionPageView && in.Action != ActionApprovalRead && in.Action != ActionReact {
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
	check(LayerPage, "not_a_template", p.Kind != KindTemplate, true)
	check(LayerPage, "page_not_trashed", p.Status != "trashed", true)
	if in.Action != ActionApprovalRead {
		check(LayerPage, "published_or_own_draft", p.Status != "draft" || p.CreatedBy == in.User.ID, true)
	}
	if p.Archived && !worksOnArchivedPage(in.Action) {
		check(LayerPage, "page_not_archived", false, false)
	}
	reviewing := reviewsNow(in)
	for _, level := range p.ViewChain {
		check(LayerRestriction, "view:"+level.PageID.String(), reviewing || in.User.matchesAny(level.Subjects), true)
	}
	if isEdit(in.Action) && len(p.EditSubjects) > 0 {
		check(LayerRestriction, "edit:"+p.ID.String(), in.User.matchesAny(p.EditSubjects), false)
	}
	if p.ControlHidden {
		check(LayerPage, "control_author", reviewing || mayAuthor(in), true)
	}
	return d
}

// reviewsNow says a reviewer of the round reads the page they were asked about. Guests never
// review (DOC-1422), and the grant is approval.read on this page alone: never a child page,
// never viewing it in lists (DOC-203). perm_review_pages is the same rule in SQL.
func reviewsNow(in Input) bool {
	return in.Action == ActionApprovalRead && in.Page.Reviewer && in.User.Type == "member"
}

// mayAuthor says whether the person is an author of the page: they hold the edit permission
// of its kind and no edit restriction keeps them out. Archiving does not count, so the
// authors of an archived, never-approved page still see it. perm_control_hidden_pages is
// the same rule in SQL.
func mayAuthor(in Input) bool {
	if !holds(in, spaceEditPermission(in.Page)) {
		return false
	}
	return len(in.Page.EditSubjects) == 0 || in.User.matchesAny(in.Page.EditSubjects)
}

// Global permissions that decide who may see a person.
const globalViewProfiles = "view_profiles"

// PersonFacts is what deciding about a person needs to know about them.
type PersonFacts struct {
	ID   uuid.UUID
	Type string // member, guest, service
	// SharedSpace says the viewer and this person both view a space through a
	// direct or group grant. "All members" does not count (spec 013, D5).
	SharedSpace bool
}

// EvaluatePerson says whether a viewer may see a person. perm_visible_people is
// the same rule for lists, and the matrix test compares the two.
func EvaluatePerson(viewer UserFacts, target PersonFacts) bool {
	if !viewer.Active || !slices.Contains(viewer.Global, globalUseProduct) || target.Type == "service" {
		return false
	}
	return target.ID == viewer.ID || slices.Contains(viewer.Global, globalViewProfiles) || target.SharedSpace
}
