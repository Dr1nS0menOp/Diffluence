// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"encoding/json"
	"net/http"
)

// Personal routes: a block belongs to one person, and the module checks that.
func (s *Server) registerSavedBlockRoutes() {
	r := s.router
	me := Authenticated()
	r.HandleFunc("GET /api/v1/saved-blocks", me, s.handleSavedBlocks)
	r.HandleFunc("POST /api/v1/saved-blocks", me, s.handleSavedBlockCreate)
	r.HandleFunc("PATCH /api/v1/saved-blocks/{id}", me, s.handleSavedBlockShare)
	r.HandleFunc("DELETE /api/v1/saved-blocks/{id}", me, s.handleSavedBlockDelete)
}

func (s *Server) handleSavedBlocks(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.SavedBlocks.List(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"blocks": list})
}

func (s *Server) handleSavedBlockCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string          `json:"name"`
		Document json.RawMessage `json:"document"`
		Shared   bool            `json:"shared"`
	}
	if decodeJSON(w, r, maxDocumentBody, &in) != nil {
		badBody(w)
		return
	}
	block, err := s.deps.SavedBlocks.Create(r.Context(), s.caller(r), in.Name, in.Document, in.Shared)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, block)
}

func (s *Server) handleSavedBlockShare(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Shared bool `json:"shared"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.SavedBlocks.SetShared(r.Context(), s.caller(r), id, in.Shared); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSavedBlockDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.deps.SavedBlocks.Delete(r.Context(), s.caller(r), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
