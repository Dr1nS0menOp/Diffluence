// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/pages"
)

const iconBodyLimit = 2 << 20

func (s *Server) registerOrganisingRoutes() {
	r := s.router
	me := Authenticated()
	pagesPath, spacesPath := "/api/v1/pages", "/api/v1/spaces"
	admin := Permission(identity.PermAdministrator)

	r.HandleFunc("PUT "+pagesPath+"/{id}/owner", Object("page.edit"), s.handlePageOwner)
	r.HandleFunc("PUT "+pagesPath+"/{id}/stage", Object("page.edit"), s.handlePageStage)
	r.HandleFunc("PUT "+pagesPath+"/{id}/verification", Object("page.edit"), s.handleVerify)
	r.HandleFunc("DELETE "+pagesPath+"/{id}/verification", Object("page.edit"), s.handleUnverify)
	r.HandleFunc("PUT "+pagesPath+"/{id}/review", Object("page.edit"), s.handleReview)
	r.HandleFunc("GET "+pagesPath+"/{id}/labels", Object("page.view"), s.handlePageLabels)
	r.HandleFunc("POST "+pagesPath+"/{id}/labels", Object("page.edit"), s.handleLabelAdd)
	r.HandleFunc("DELETE "+pagesPath+"/{id}/labels/{label}", Object("page.edit"), s.handleLabelRemove)
	r.HandleFunc("POST "+pagesPath+"/{id}/kind", Object("page.edit"), s.handlePageKind)
	r.HandleFunc("PUT "+pagesPath+"/{id}/schedule", Object("page.edit"), s.handleSchedule)
	r.HandleFunc("DELETE "+pagesPath+"/{id}/schedule", Object("page.edit"), s.handleUnschedule)
	r.HandleFunc("GET "+pagesPath+"/{id}/move-impact", Object("page.edit"), s.handleMoveImpact)
	r.HandleFunc("GET "+pagesPath+"/{id}/copy-impact", Object("page.view"), s.handleCopyImpact)
	r.HandleFunc("POST "+pagesPath+"/{id}/sort-children", Object("page.edit"), s.handleSortChildren)
	r.HandleFunc("GET "+pagesPath+"/{id}/editors", Object("page.edit"), s.handleOtherEditors)
	r.HandleFunc("POST "+pagesPath+"/{id}/archive", Object("page.archive"), s.handleArchive)
	r.HandleFunc("POST "+pagesPath+"/{id}/unarchive", Object("page.archive"), s.handleUnarchive)
	r.HandleFunc("POST "+pagesPath+"/{id}/approvals", Object("page.edit"), s.handleApprovalAsk)
	r.HandleFunc("GET "+pagesPath+"/{id}/approvals", Object("page.view"), s.handleApprovals)
	// Lists and personal rounds: the module checks each page and each person.
	r.HandleFunc("POST "+pagesPath+"/bulk-move", me, s.handleBulkMove)
	r.HandleFunc("POST "+pagesPath+"/reachable", me, s.handleReachable)
	r.HandleFunc("POST /api/v1/markdown/document", me, s.handleMarkdownDocument)
	r.HandleFunc("GET /api/v1/labels", me, s.handleLabels)
	r.HandleFunc("GET /api/v1/labels/{label}/pages", me, s.handleLabelPages)
	r.HandleFunc("GET /api/v1/posts", me, s.handleAllPosts)
	r.HandleFunc("GET /api/v1/same-title", me, s.handleSameTitle)
	r.HandleFunc("GET /api/v1/approvals/{id}", me, s.handleApproval)
	r.HandleFunc("PUT /api/v1/approvals/{id}/answer", me, s.handleApprovalAnswer)
	r.HandleFunc("POST /api/v1/approvals/{id}/close", me, s.handleApprovalClose)
	r.HandleFunc("DELETE /api/v1/approvals/{id}/people/{user}", me, s.handleApprovalRemove)

	r.HandleFunc("GET "+spacesPath+"/{id}/labels", Object("space.view"), s.handleSpaceLabels)
	r.HandleFunc("PUT "+spacesPath+"/{id}/labels", Object("space.labels"), s.handleSpaceLabelsFixed)
	r.HandleFunc("POST "+spacesPath+"/{id}/labels/rename", Object("space.labels"), s.handleLabelRename)
	r.HandleFunc("GET "+spacesPath+"/{id}/statuses", Object("space.view"), s.handleStatuses)
	r.HandleFunc("PUT "+spacesPath+"/{id}/statuses", Object("space.statuses"), s.handleStatusesSet)
	r.HandleFunc("PUT "+spacesPath+"/{id}/review", Object("space.statuses"), s.handleSpaceReview)
	r.HandleFunc("GET "+spacesPath+"/{id}/approvals", Object("space.approvals"), s.handleSpaceApprovalRule)
	r.HandleFunc("PUT "+spacesPath+"/{id}/approvals", Object("space.approvals"), s.handleSpaceApprovals)
	r.HandleFunc("GET "+spacesPath+"/{id}/posts", Object("space.view"), s.handleSpacePosts)
	r.HandleFunc("GET "+spacesPath+"/{id}/archive", Object("space.view"), s.handleSpaceArchiveList)
	r.HandleFunc("GET "+spacesPath+"/{id}/content", Object("space.administer"), s.handleContent)
	r.HandleFunc("POST "+spacesPath+"/{id}/content/preview", Object("space.administer"), s.handleContentPreview)
	r.HandleFunc("POST "+spacesPath+"/{id}/content/run", Object("space.administer"), s.handleContentRun)
	r.HandleFunc("PUT "+spacesPath+"/{id}/key", Object("space.administer"), s.handleSpaceKey)
	r.HandleFunc("GET "+spacesPath+"/{id}/old-keys", Object("space.administer"), s.handleOldKeys)
	r.HandleFunc("DELETE "+spacesPath+"/{id}/old-keys/{alias}", Object("space.administer"), s.handleOldKeyRelease)
	r.HandleFunc("PUT "+spacesPath+"/{id}/owners", Object("space.administer"), s.handleSpaceOwners)
	r.HandleFunc("PUT "+spacesPath+"/{id}/home", Object("space.look"), s.handleSpaceHome)
	r.HandleFunc("PUT "+spacesPath+"/{id}/features", Object("space.administer"), s.handleSpaceFeatures)
	r.HandleFunc("PUT "+spacesPath+"/{id}/title-align", Object("space.look"), s.handleSpaceTitleAlign)
	r.HandleFunc("POST "+spacesPath+"/{id}/icon", Object("space.look"), s.handleSpaceIcon)
	r.HandleFunc("DELETE "+spacesPath+"/{id}/icon", Object("space.look"), s.handleSpaceIconRemove)
	r.HandleFunc("GET "+spacesPath+"/{id}/icon", Object("space.view"), s.handleSpaceIconGet)

	r.HandleFunc("GET /api/v1/admin/features", admin, s.handleInstanceFeatures)
	r.HandleFunc("PUT /api/v1/admin/features", admin, s.handleInstanceFeaturesSet)
	r.HandleFunc("DELETE /api/v1/admin/pages/{id}/versions/{number}", admin, s.handleVersionRemove)
}

// body reads a small JSON body into v, and answers for itself when it is bad.
func body[T any](w http.ResponseWriter, r *http.Request, v *T) bool {
	if decodeJSON(w, r, maxSmallBody, v) != nil {
		badBody(w)
		return false
	}
	return true
}

func optionalUUID(w http.ResponseWriter, value string) (*uuid.UUID, bool) {
	if value == "" {
		return nil, true
	}
	id, err := uuid.Parse(value)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "A filter is not valid.")
		return nil, false
	}
	return &id, true
}

func (s *Server) handlePageOwner(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		UserID uuid.UUID `json:"user_id"`
	}
	if !body(w, r, &in) {
		return
	}
	p, err := s.deps.Pages.SetOwner(r.Context(), s.caller(r), id, in.UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handlePageStage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in pages.Stage
	if !body(w, r, &in) {
		return
	}
	p, err := s.deps.Pages.SetStage(r.Context(), s.caller(r), id, in.Text, in.Color)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Until string `json:"until"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.Verify(r.Context(), s.caller(r), id, in.Until); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnverify(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Pages.Unverify(r.Context(), c, id) })
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		On string `json:"on"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.SetReview(r.Context(), s.caller(r), id, in.On); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePageLabels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.PageLabels(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"labels": list})
}

func (s *Server) handleLabelAdd(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Label string `json:"label"`
	}
	if !body(w, r, &in) {
		return
	}
	list, err := s.deps.Pages.AddLabel(r.Context(), s.caller(r), id, in.Label)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"labels": list})
}

func (s *Server) handleLabelRemove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.RemoveLabel(r.Context(), s.caller(r), id, r.PathValue("label"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"labels": list})
}

func (s *Server) handleLabels(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	spaceID, ok := optionalUUID(w, q.Get("space_id"))
	if !ok {
		return
	}
	list, err := s.deps.Pages.Labels(r.Context(), s.caller(r), pages.LabelQuery{Text: q.Get("q"), SpaceID: spaceID, ByName: q.Get("sort") == "name"})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"labels": list})
}

func (s *Server) handleLabelPages(w http.ResponseWriter, r *http.Request) {
	spaceID, ok := optionalUUID(w, r.URL.Query().Get("space_id"))
	if !ok {
		return
	}
	labels := strings.Split(r.PathValue("label"), "+")
	list, err := s.deps.Pages.LabelPages(r.Context(), s.caller(r), labels, spaceID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": list})
}

func (s *Server) handleSpaceLabels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	v, err := s.deps.Pages.SpaceLabelList(r.Context(), s.caller(r), id, r.URL.Query().Get("sort") == "name")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleSpaceLabelsFixed(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Fixed  bool     `json:"fixed"`
		Labels []string `json:"labels"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.SetFixedLabels(r.Context(), s.caller(r), id, in.Fixed, in.Labels); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLabelRename(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.RenameLabel(r.Context(), s.caller(r), id, in.From, in.To); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleStatuses(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, free, err := s.deps.Pages.SpaceStages(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"statuses": list, "free": free})
}

func (s *Server) handleStatusesSet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Free     bool          `json:"free"`
		Statuses []pages.Stage `json:"statuses"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.SetStages(r.Context(), s.caller(r), id, in.Statuses, in.Free); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpaceReview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Months int `json:"months"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.SetReviewDefault(r.Context(), s.caller(r), id, in.Months); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpaceApprovalRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	v, err := s.deps.Pages.ApprovalRuleOf(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleSpaceApprovals(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Required  bool        `json:"required"`
		Approvers []uuid.UUID `json:"approvers"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.SetApprovalRule(r.Context(), s.caller(r), id, in.Required, in.Approvers); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePageKind(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Kind string `json:"kind"`
	}
	if !body(w, r, &in) {
		return
	}
	p, err := s.deps.Pages.SetKind(r.Context(), s.caller(r), id, in.Kind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) writePosts(w http.ResponseWriter, r *http.Request, spaceID *uuid.UUID) {
	list, total, err := s.deps.Pages.Posts(r.Context(), s.caller(r), spaceID, queryInt(r, "limit"), queryInt(r, "offset"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"posts": list, "total": total})
}

func (s *Server) handleAllPosts(w http.ResponseWriter, r *http.Request) { s.writePosts(w, r, nil) }

func (s *Server) handleSpacePosts(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	s.writePosts(w, r, &id)
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		At     time.Time `json:"at"`
		Silent bool      `json:"silent"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.Schedule(r.Context(), s.caller(r), id, in.At, in.Silent); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnschedule(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Pages.Unschedule(r.Context(), c, id) })
}

func (s *Server) handleCopyImpact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	q := r.URL.Query()
	spaceID, ok := optionalUUID(w, q.Get("space_id"))
	if !ok {
		return
	}
	parent, ok := optionalUUID(w, q.Get("parent_id"))
	if !ok {
		return
	}
	v, err := s.deps.Pages.CopyImpact(r.Context(), s.caller(r), id, spaceID, parent)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleMoveImpact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	q := r.URL.Query()
	spaceID, ok := optionalUUID(w, q.Get("space_id"))
	if !ok {
		return
	}
	parent, ok := optionalUUID(w, q.Get("parent_id"))
	if !ok {
		return
	}
	v, err := s.deps.Pages.MoveImpact(r.Context(), s.caller(r), id, spaceID, parent)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleBulkMove(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PageIDs  []uuid.UUID `json:"page_ids"`
		SpaceID  *uuid.UUID  `json:"space_id"`
		ParentID *uuid.UUID  `json:"parent_id"`
	}
	if !body(w, r, &in) {
		return
	}
	moved, failed, err := s.deps.Pages.BulkMove(r.Context(), s.caller(r), in.PageIDs, pages.MoveInput{SpaceID: in.SpaceID, ParentID: in.ParentID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"moved": moved, "failed": failed})
}

func (s *Server) handleReachable(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []uuid.UUID `json:"ids"`
	}
	if !body(w, r, &in) {
		return
	}
	found, err := s.deps.Pages.Reachable(r.Context(), s.caller(r), in.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": found})
}

// maxPastedMarkdown is far more than anyone pastes. It keeps the parser's work in a request
// to seconds at worst, as requests never do slow work (ARC-014).
const maxPastedMarkdown = 256 << 10

// markdownSlots is how many pastes are converted at once, so nobody can fill every core.
const markdownSlots = 2

// handleMarkdownDocument turns pasted Markdown into a document. Nothing is stored, and
// the result went through the same gate as everything that is saved.
func (s *Server) handleMarkdownDocument(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Markdown string `json:"markdown"`
	}
	if decodeJSON(w, r, maxDocumentBody, &in) != nil {
		badBody(w)
		return
	}
	if len(in.Markdown) > maxPastedMarkdown {
		writeError(w, http.StatusBadRequest, "too_large", "That is too much text to convert at once.")
		return
	}
	select {
	case s.markdownBusy <- struct{}{}:
		defer func() { <-s.markdownBusy }()
	default:
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too much text is being converted right now. Try again shortly.")
		return
	}
	parsed, _ := document.FromMarkdown([]byte(in.Markdown), nil)
	raw, err := json.Marshal(parsed)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	checked, err := document.Normalize(raw)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"document": checked})
}

func (s *Server) handleSortChildren(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		By string `json:"by"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.SortChildren(r.Context(), s.caller(r), id, in.By); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOtherEditors(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.OtherEditors(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"editors": list})
}

func (s *Server) handleSameTitle(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	spaceID, err1 := uuid.Parse(q.Get("space_id"))
	except, err2 := optionalID(q.Get("except"))
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "A filter is not valid.")
		return
	}
	page, err := s.deps.Pages.SameTitle(r.Context(), s.caller(r), spaceID, except, q.Get("title"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"page": page})
}

func (s *Server) handleArchive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		IncludeChildren bool `json:"include_children"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.Archive(r.Context(), s.caller(r), id, in.IncludeChildren); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUnarchive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		WithParents bool `json:"with_parents"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.Unarchive(r.Context(), s.caller(r), id, in.WithParents); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpaceArchiveList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.Archived(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": list})
}

func (s *Server) handleApprovalAsk(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		UserIDs []uuid.UUID `json:"user_ids"`
		Message string      `json:"message"`
		DueOn   string      `json:"due_on"`
	}
	if !body(w, r, &in) {
		return
	}
	round, cannotSee, err := s.deps.Pages.AskApproval(r.Context(), s.caller(r), id, pages.RoundInput{UserIDs: in.UserIDs, Message: in.Message, DueOn: in.DueOn})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"round_id": round, "cannot_see": cannotSee})
}

func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.ApprovalRounds(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rounds": list})
}

func (s *Server) handleApproval(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	round, err := s.deps.Pages.ApprovalRoundText(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, round)
}

func (s *Server) handleApprovalAnswer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Answer  string `json:"answer"`
		Comment string `json:"comment"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.AnswerApproval(r.Context(), s.caller(r), id, in.Answer, in.Comment); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleApprovalClose(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Pages.CloseApproval(r.Context(), c, id) })
}

func (s *Server) handleApprovalRemove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	user, ok := pathID(w, r, "user")
	if !ok {
		return
	}
	if err := s.deps.Pages.RemoveApprover(r.Context(), s.caller(r), id, user); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func contentQuery(w http.ResponseWriter, r *http.Request) (pages.ContentQuery, bool) {
	q := r.URL.Query()
	owner, ok := optionalUUID(w, q.Get("owner_id"))
	if !ok {
		return pages.ContentQuery{}, false
	}
	contributor, ok := optionalUUID(w, q.Get("contributor_id"))
	if !ok {
		return pages.ContentQuery{}, false
	}
	days := map[string]*time.Time{}
	for _, key := range []string{"changed_from", "changed_until", "created_from", "created_until"} {
		raw := q.Get(key)
		if raw == "" {
			continue
		}
		day, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "A date is written as 2026-11-07.")
			return pages.ContentQuery{}, false
		}
		days[key] = &day
	}
	months, _ := strconv.Atoi(q.Get("inactive_months"))
	on := func(key string) bool { return q.Get(key) == "1" }
	return pages.ContentQuery{OwnerID: owner, OwnerDisabled: on("owner_disabled"), Stage: q.Get("stage"), Label: q.Get("label"),
		ContributorID: contributor, InactiveMonths: months, WithParents: on("with_parents"), Restricted: on("restricted"),
		BrokenLinks: on("broken_links"), Orphaned: on("orphaned"), PastReview: on("past_review"), Sort: q.Get("sort"),
		ChangedFrom: days["changed_from"], ChangedUntil: days["changed_until"],
		CreatedFrom: days["created_from"], CreatedUntil: days["created_until"],
		Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset")}, true
}

func (s *Server) handleContent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	q, ok := contentQuery(w, r)
	if !ok {
		return
	}
	list, total, err := s.deps.Pages.Content(r.Context(), s.caller(r), id, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": list, "total": total})
}

func (s *Server) handleContentPreview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in pages.ContentAction
	if !body(w, r, &in) {
		return
	}
	plan, err := s.deps.Pages.PreviewContent(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleContentRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in pages.ContentAction
	if !body(w, r, &in) {
		return
	}
	job, err := s.deps.Pages.StartContent(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job})
}

func (s *Server) handleSpaceKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Key string `json:"key"`
	}
	if !body(w, r, &in) {
		return
	}
	sp, err := s.deps.Spaces.ChangeKey(r.Context(), s.caller(r), id, in.Key)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sp)
}

func (s *Server) handleOldKeys(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Spaces.OldKeys(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": list})
}

func (s *Server) handleOldKeyRelease(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error {
		return s.deps.Spaces.ReleaseKey(r.Context(), c, id, r.PathValue("alias"))
	})
}

func (s *Server) handleSpaceOwners(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		UserIDs []uuid.UUID `json:"user_ids"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Spaces.SetOwners(r.Context(), s.caller(r), id, in.UserIDs); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpaceHome(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		PageID uuid.UUID `json:"page_id"`
	}
	if !body(w, r, &in) {
		return
	}
	sp, err := s.deps.Spaces.SetHome(r.Context(), s.caller(r), id, in.PageID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sp)
}

func (s *Server) handleSpaceFeatures(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	in := map[string]bool{}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Spaces.SetFeatures(r.Context(), s.caller(r), id, in); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpaceTitleAlign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Align string `json:"align"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Spaces.SetTitleAlign(r.Context(), s.caller(r), id, in.Align); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpaceIcon(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.deps.Spaces.SetIcon(r.Context(), s.caller(r), id, http.MaxBytesReader(w, r.Body, iconBodyLimit)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpaceIconRemove(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Spaces.RemoveIcon(r.Context(), c, id) })
}

// handleSpaceIconGet serves an image with the same care as an attachment: the
// type comes from the bytes, and nothing in it can run.
func (s *Server) handleSpaceIconGet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	f, contentType, err := s.deps.Spaces.Icon(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer func() { _ = f.Close() }()
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, max-age=300")
	h.Set("Content-Type", contentType)
	http.ServeContent(w, r, "", time.Time{}, f)
}

func (s *Server) handleInstanceFeatures(w http.ResponseWriter, r *http.Request) {
	v, err := s.deps.Features.Instance(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"off": v.Off, "new_off": v.NewOff, "all": features.All})
}

func (s *Server) handleInstanceFeaturesSet(w http.ResponseWriter, r *http.Request) {
	var in features.Instance
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Features.SetInstance(r.Context(), s.caller(r), in); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleVersionRemove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	n, ok := versionNumber(w, r)
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.RemoveVersion(r.Context(), s.caller(r), id, n, in.Reason); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
