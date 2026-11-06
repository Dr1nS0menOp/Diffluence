// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"

	"github.com/google/uuid"
)

func (s *Server) registerLinkRoutes() {
	r := s.router
	// A list: the answer only holds what the caller may see, which the module decides.
	r.HandleFunc("POST /api/v1/pages/link-facts", Authenticated(), s.handleLinkFacts)
	r.HandleFunc("GET /api/v1/pages/{id}/targets", Object("page.view"), s.handleLinkTargets)
	r.HandleFunc("GET /api/v1/pages/{id}/placeholders", Object("page.view"), s.handlePlaceholders)
	// Viewing the page is the rule here. The module also asks for page.create on it.
	r.HandleFunc("POST /api/v1/pages/{id}/placeholders", Object("page.view"), s.handleFollowPlaceholder)
	r.HandleFunc("GET /api/v1/spaces/{id}/placeholders", Object("space.administer"), s.handleOpenPlaceholders)
}

func (s *Server) handleLinkFacts(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []uuid.UUID `json:"ids"`
	}
	if !body(w, r, &in) {
		return
	}
	found, err := s.deps.Pages.LinkFacts(r.Context(), s.caller(r), in.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": found})
}

func (s *Server) handleLinkTargets(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	targets, err := s.deps.Pages.Targets(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, targets)
}

func (s *Server) handlePlaceholders(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.Followed(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"placeholders": list})
}

func (s *Server) handleFollowPlaceholder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Title string `json:"title"`
	}
	if !body(w, r, &in) {
		return
	}
	page, err := s.deps.Pages.Follow(r.Context(), s.caller(r), id, in.Title)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"page_id": page})
}

func (s *Server) handleOpenPlaceholders(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.OpenPlaceholders(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"placeholders": list})
}
