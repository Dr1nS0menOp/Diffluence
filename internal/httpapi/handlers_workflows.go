// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"

	"github.com/google/uuid"

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
