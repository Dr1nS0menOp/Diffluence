// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/workflow"
)

// Document control, wave DC1 of spec 064: workflows of a space, the space workflow, page
// and branch workflows, and the steps of a page through its workflow.
func (s *Server) registerWorkflowRoutes() {
	r := s.router
	pagesPath, spacesPath := "/api/v1/pages", "/api/v1/spaces"
	r.HandleFunc("GET "+spacesPath+"/{id}/workflows", Object(permissions.SpaceView), s.handleWorkflows)
	r.HandleFunc("POST "+spacesPath+"/{id}/workflows", Object(permissions.SpaceWorkflows), s.handleWorkflowCreate)
	r.HandleFunc("PUT "+spacesPath+"/{id}/workflow", Object(permissions.SpaceWorkflows), s.handleSpaceWorkflow)
	// A workflow is found by its own id; the module checks the space it belongs to.
	me := Authenticated()
	r.HandleFunc("GET /api/v1/workflows/{id}", me, s.handleWorkflow)
	r.HandleFunc("PUT /api/v1/workflows/{id}", me, s.handleWorkflowSave)
	r.HandleFunc("POST /api/v1/workflows/{id}/archive", me, s.handleWorkflowArchive)
	r.HandleFunc("GET /api/v1/workflows/{id}/history", me, s.handleWorkflowHistory)
	r.HandleFunc("PUT "+pagesPath+"/{id}/workflow", Object(permissions.PageWorkflow), s.handlePageWorkflow)
	r.HandleFunc("POST "+pagesPath+"/{id}/transitions", Object(permissions.PageEdit), s.handleTransition)
	s.registerReviewRoutes()
}

// registerReviewRoutes is wave DC2: reviewers and their answers, the review panel, the
// editor's validator and the instance templates.
func (s *Server) registerReviewRoutes() {
	r := s.router
	pagesPath := "/api/v1/pages"
	// A reviewer may read a page they cannot view, so these routes are found by the page id
	// and the module decides: approval.read for reviewers, page.edit for authors (DOC-203).
	me := Authenticated()
	r.HandleFunc("GET "+pagesPath+"/{id}/review", me, s.handleReviewPanel)
	r.HandleFunc("POST "+pagesPath+"/{id}/review/answer", me, s.handleReviewAnswer)
	r.HandleFunc("POST "+pagesPath+"/{id}/review/comments", me, s.handleReviewComment)
	r.HandleFunc("POST "+pagesPath+"/{id}/reviewers", me, s.handleReviewerAdd)
	r.HandleFunc("POST "+pagesPath+"/{id}/reviewers/{user}/remove", me, s.handleReviewerRemove)
	// The validator is pure: it reads nothing and writes nothing.
	r.HandleFunc("POST /api/v1/workflows/check", me, s.handleWorkflowCheck)
	r.HandleFunc("GET /api/v1/workflow-templates", me, s.handleWorkflowStarts)
	r.HandleFunc("POST /api/v1/spaces/{id}/workflows/from-template", Object(permissions.SpaceWorkflows), s.handleWorkflowCopy)
	r.HandleFunc("GET /api/v1/workflows/{id}/template-changes", me, s.handleTemplateChanges)
	r.HandleFunc("POST /api/v1/workflows/{id}/take-template", me, s.handleTakeTemplate)
	admin := SessionOnly(Permission(identity.PermAdministrator))
	r.HandleFunc("GET /api/v1/admin/workflow-templates", admin, s.handleAdminWorkflowTemplates)
	r.HandleFunc("POST /api/v1/admin/workflow-templates", admin, s.handleWorkflowTemplateCreate)
	r.HandleFunc("PUT /api/v1/admin/workflow-templates/{id}", admin, s.handleWorkflowTemplateSave)
	r.HandleFunc("POST /api/v1/admin/workflow-templates/{id}/archive", admin, s.handleWorkflowTemplateArchive)
}

func (s *Server) handleReviewPanel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	panel, err := s.deps.Pages.Review(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, panel)
}

func (s *Server) handleReviewAnswer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Answer  string `json:"answer"`
		Comment string `json:"comment"`
		Version int    `json:"version"`
	}
	if !body(w, r, &in) {
		return
	}
	p, err := s.deps.Pages.AnswerReview(r.Context(), s.caller(r), id, pages.ReviewAnswerInput{Answer: in.Answer, Comment: in.Comment, Version: in.Version})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleReviewComment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if !body(w, r, &in) {
		return
	}
	out, err := s.deps.Pages.AddReviewComment(r.Context(), s.caller(r), id, in.Body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleReviewerAdd(w http.ResponseWriter, r *http.Request) {
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
	if err := s.deps.Pages.AddReviewer(r.Context(), s.caller(r), id, in.UserID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReviewerRemove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	user, ok := pathID(w, r, "user")
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !body(w, r, &in) {
		return
	}
	p, err := s.deps.Pages.RemoveReviewer(r.Context(), s.caller(r), id, user, in.Reason)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleWorkflowCheck(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Definition workflow.Definition `json:"definition"`
	}
	if !body(w, r, &in) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"problem": pages.CheckDefinition(in.Definition)})
}

func (s *Server) handleWorkflowStarts(w http.ResponseWriter, r *http.Request) {
	templates, examples, err := s.deps.Pages.WorkflowStarts(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": templates, "examples": examples})
}

func (s *Server) handleWorkflowCopy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		TemplateID uuid.UUID `json:"template_id"`
		Name       string    `json:"name"`
	}
	if !body(w, r, &in) {
		return
	}
	wf, err := s.deps.Pages.CopyWorkflowTemplate(r.Context(), s.caller(r), id, in.TemplateID, in.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, wf)
}

func (s *Server) handleTemplateChanges(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	changes, version, err := s.deps.Pages.TemplateChanges(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": changes, "template_version": version})
}

func (s *Server) handleTakeTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		BaseVersion int `json:"base_version"`
	}
	if !body(w, r, &in) {
		return
	}
	wf, err := s.deps.Pages.TakeTemplate(r.Context(), s.caller(r), id, in.BaseVersion)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wf)
}

func (s *Server) handleAdminWorkflowTemplates(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Pages.AdminWorkflowTemplates(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": list})
}

func (s *Server) handleWorkflowTemplateCreate(w http.ResponseWriter, r *http.Request) {
	var in workflowBody
	if !body(w, r, &in) {
		return
	}
	t, err := s.deps.Pages.CreateWorkflowTemplate(r.Context(), s.caller(r), in.input())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleWorkflowTemplateSave(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in workflowBody
	if !body(w, r, &in) {
		return
	}
	t, err := s.deps.Pages.SaveWorkflowTemplate(r.Context(), s.caller(r), id, in.input())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleWorkflowTemplateArchive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.deps.Pages.ArchiveWorkflowTemplate(r.Context(), s.caller(r), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type workflowBody struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Definition  workflow.Definition `json:"definition"`
	BaseVersion int                 `json:"base_version"`
}

func (b workflowBody) input() pages.WorkflowInput {
	return pages.WorkflowInput{Name: b.Name, Description: b.Description, Definition: b.Definition, BaseVersion: b.BaseVersion}
}

func (s *Server) handleWorkflows(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.Workflows(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflows": list})
}

func (s *Server) handleWorkflowCreate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in workflowBody
	if !body(w, r, &in) {
		return
	}
	wf, err := s.deps.Pages.CreateWorkflow(r.Context(), s.caller(r), id, in.input())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, wf)
}

func (s *Server) handleWorkflow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	wf, err := s.deps.Pages.Workflow(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wf)
}

func (s *Server) handleWorkflowSave(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in workflowBody
	if !body(w, r, &in) {
		return
	}
	wf, err := s.deps.Pages.SaveWorkflow(r.Context(), s.caller(r), id, in.input())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wf)
}

func (s *Server) handleWorkflowArchive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.deps.Pages.ArchiveWorkflow(r.Context(), s.caller(r), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWorkflowHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.WorkflowHistory(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": list})
}

func (s *Server) handleSpaceWorkflow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		WorkflowID *uuid.UUID `json:"workflow_id"`
		Kinds      []string   `json:"kinds"`
		Preview    bool       `json:"preview"`
	}
	if !body(w, r, &in) {
		return
	}
	land, err := s.deps.Pages.SetSpaceWorkflow(r.Context(), s.caller(r), id, pages.SpaceWorkflowInput{
		WorkflowID: in.WorkflowID, Kinds: in.Kinds, Preview: in.Preview})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, land)
}

func (s *Server) handlePageWorkflow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		WorkflowID      *uuid.UUID `json:"workflow_id"`
		Branch          bool       `json:"branch"`
		ReplaceChildren bool       `json:"replace_children"`
		Preview         bool       `json:"preview"`
	}
	if !body(w, r, &in) {
		return
	}
	land, err := s.deps.Pages.SetPageWorkflow(r.Context(), s.caller(r), id, pages.PageWorkflowInput{
		WorkflowID: in.WorkflowID, Branch: in.Branch, ReplaceChildren: in.ReplaceChildren, Preview: in.Preview})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, land)
}

func (s *Server) handleTransition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Transition string `json:"transition"`
		Version    int    `json:"version"`
		Comment    string `json:"comment"`
	}
	if !body(w, r, &in) {
		return
	}
	p, err := s.deps.Pages.Transition(r.Context(), s.caller(r), id, pages.TransitionInput{
		Transition: in.Transition, Version: in.Version, Comment: in.Comment})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
