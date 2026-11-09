// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/templates"
)

func (s *Server) registerTemplateRoutes() {
	r := s.router
	me := Authenticated()
	pagesPath, spacesPath := "/api/v1/pages", "/api/v1/spaces"
	admin := Permission(identity.PermAdministrator)

	r.HandleFunc("POST "+spacesPath+"/{id}/trash/bulk", Object("space.administer"), s.handleTrashBulk)

	r.HandleFunc("GET "+spacesPath+"/{id}/templates", Object("page.create"), s.handleGallery)
	r.HandleFunc("GET "+spacesPath+"/{id}/manage-templates", Object("space.templates"), s.handleSpaceTemplateList)
	r.HandleFunc("POST "+spacesPath+"/{id}/templates", Object("space.templates"), s.handleSpaceTemplateCreate)
	r.HandleFunc("POST "+spacesPath+"/{id}/templates/import", Object("space.templates"), s.handleSpaceTemplateImport)
	r.HandleFunc("POST "+pagesPath+"/{id}/save-as-template", Object("space.templates"), s.handleSaveAsTemplate)
	// The module decides per template: it may belong to a space or to everyone.
	r.HandleFunc("GET /api/v1/templates/{id}", me, s.handleTemplate)
	r.HandleFunc("PATCH /api/v1/templates/{id}", me, s.handleTemplateUpdate)
	r.HandleFunc("DELETE /api/v1/templates/{id}", me, s.handleTemplateDelete)
	r.HandleFunc("POST /api/v1/templates/{id}/use", me, s.handleTemplateUse)
	r.HandleFunc("GET /api/v1/templates/{id}/export", me, s.handleTemplateExport)
	r.HandleFunc("GET /api/v1/templates/{id}/files/{file}", me, s.handleTemplateFile)
	r.HandleFunc("PUT /api/v1/templates/{id}/favourite", me, s.handleTemplateFavouriteOn)
	r.HandleFunc("DELETE /api/v1/templates/{id}/favourite", me, s.handleTemplateFavouriteOff)
	r.HandleFunc("GET /api/v1/space-templates", me, s.handleSpaceTemplates)
	r.HandleFunc("GET /api/v1/start", me, s.handleStart)

	r.HandleFunc("GET /api/v1/admin/templates", admin, s.handleInstanceTemplates)
	r.HandleFunc("POST /api/v1/admin/templates", admin, s.handleInstanceTemplateCreate)
	r.HandleFunc("POST /api/v1/admin/templates/import", admin, s.handleInstanceTemplateImport)
	r.HandleFunc("POST /api/v1/admin/space-templates", admin, s.handleSpaceTemplateSave)
	r.HandleFunc("GET /api/v1/admin/organising", admin, s.handleOrganising)
	r.HandleFunc("PUT /api/v1/admin/organising", admin, s.handleOrganisingSet)
}

func (s *Server) handleTemplateFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	file, ok := pathID(w, r, "file")
	if !ok {
		return
	}
	a, f, err := s.deps.Templates.File(r.Context(), s.caller(r), id, file)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer func() { _ = f.Close() }()
	serveFile(w, r, a, f, r.URL.Query().Get("download") == "1")
}

func (s *Server) handleTrashBulk(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Action string      `json:"action"`
		IDs    []uuid.UUID `json:"ids"`
	}
	if !body(w, r, &in) {
		return
	}
	done, failed, err := s.deps.Trash.Bulk(r.Context(), s.caller(r), id, in.Action, in.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"done": done, "failed": failed})
}

func (s *Server) handleGallery(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Templates.Gallery(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": list})
}

func (s *Server) writeTemplates(w http.ResponseWriter, r *http.Request, spaceID *uuid.UUID) {
	list, err := s.deps.Templates.Manage(r.Context(), s.caller(r), spaceID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": list})
}

func (s *Server) handleSpaceTemplateList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	s.writeTemplates(w, r, &id)
}

func (s *Server) handleInstanceTemplates(w http.ResponseWriter, r *http.Request) {
	s.writeTemplates(w, r, nil)
}

func (s *Server) createTemplate(w http.ResponseWriter, r *http.Request, spaceID *uuid.UUID) {
	var in templates.Input
	if decodeJSON(w, r, maxDocumentBody, &in) != nil {
		badBody(w)
		return
	}
	t, err := s.deps.Templates.Create(r.Context(), s.caller(r), spaceID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleSpaceTemplateCreate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	s.createTemplate(w, r, &id)
}

func (s *Server) handleInstanceTemplateCreate(w http.ResponseWriter, r *http.Request) {
	s.createTemplate(w, r, nil)
}

func (s *Server) importTemplate(w http.ResponseWriter, r *http.Request, spaceID *uuid.UUID) {
	var in templates.Bundle
	if decodeJSON(w, r, maxDocumentBody, &in) != nil {
		badBody(w)
		return
	}
	t, err := s.deps.Templates.Import(r.Context(), s.caller(r), spaceID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleSpaceTemplateImport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	s.importTemplate(w, r, &id)
}

func (s *Server) handleInstanceTemplateImport(w http.ResponseWriter, r *http.Request) {
	s.importTemplate(w, r, nil)
}

func (s *Server) handleSaveAsTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !body(w, r, &in) {
		return
	}
	t, err := s.deps.Templates.FromPage(r.Context(), s.caller(r), id, in.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	t, err := s.deps.Templates.Get(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleTemplateUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in templates.Input
	if decodeJSON(w, r, maxDocumentBody, &in) != nil {
		badBody(w)
		return
	}
	t, err := s.deps.Templates.Update(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleTemplateDelete(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Templates.Delete(r.Context(), c, id) })
}

func (s *Server) handleTemplateUse(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in templates.UseInput
	if !body(w, r, &in) {
		return
	}
	p, skipped, err := s.deps.Templates.Use(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"page": p, "labels_left_out": skipped})
}

func (s *Server) handleTemplateExport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	b, err := s.deps.Templates.Export(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Disposition", attachmentHeader("template.json"))
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) setTemplateFavourite(w http.ResponseWriter, r *http.Request, on bool) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error {
		return s.deps.Templates.SetFavourite(r.Context(), c, id, on)
	})
}

func (s *Server) handleTemplateFavouriteOn(w http.ResponseWriter, r *http.Request) {
	s.setTemplateFavourite(w, r, true)
}

func (s *Server) handleTemplateFavouriteOff(w http.ResponseWriter, r *http.Request) {
	s.setTemplateFavourite(w, r, false)
}

func (s *Server) handleSpaceTemplates(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Templates.SpaceTemplates(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	preset, err := s.deps.Spaces.DefaultPreset(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": list, "default_preset": preset})
}

func (s *Server) handleSpaceTemplateSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SpaceID     uuid.UUID `json:"space_id"`
		Name        string    `json:"name"`
		Description string    `json:"description"`
	}
	if !body(w, r, &in) {
		return
	}
	t, err := s.deps.Templates.SaveSpace(r.Context(), s.caller(r), in.SpaceID, in.Name, in.Description)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	id, err := s.deps.Pages.CompanyHome(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"page_id": id})
}

type organisingSettings struct {
	CompanyHome   uuid.UUID `json:"company_home"`
	DraftDays     int       `json:"draft_days"`
	DefaultPreset string    `json:"default_preset"`
}

func (s *Server) handleOrganising(w http.ResponseWriter, r *http.Request) {
	var out organisingSettings
	var err error
	if out.CompanyHome, err = s.deps.Pages.CompanyHomeSetting(r.Context()); err != nil {
		s.fail(w, r, err)
		return
	}
	if out.DraftDays, err = s.deps.Pages.DraftRetention(r.Context()); err != nil {
		s.fail(w, r, err)
		return
	}
	if out.DefaultPreset, err = s.deps.Spaces.DefaultPreset(r.Context()); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleOrganisingSet(w http.ResponseWriter, r *http.Request) {
	var in organisingSettings
	if !body(w, r, &in) {
		return
	}
	c := s.caller(r)
	err := s.deps.Pages.SetCompanyHome(r.Context(), c, in.CompanyHome)
	if err == nil {
		err = s.deps.Pages.SetDraftRetention(r.Context(), c, in.DraftDays)
	}
	if err == nil {
		err = s.deps.Spaces.SetDefaultPreset(r.Context(), c, in.DefaultPreset)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}
