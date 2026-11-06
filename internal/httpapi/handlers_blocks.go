// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/blocks"
)

func (s *Server) registerBlockRoutes() {
	r := s.router
	// A list: the answer only holds what the caller may see, which the module decides.
	r.HandleFunc("POST /api/v1/blocks/{kind}", Authenticated(), s.handleBlock)
	r.HandleFunc("GET /api/v1/pages/{id}/used-by", Object("page.view"), s.handleUsedBy)
}

func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PageID   *uuid.UUID     `json:"page_id"`
		Settings map[string]any `json:"settings"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	request := blocks.Request{Kind: r.PathValue("kind"), Settings: in.Settings}
	if in.PageID != nil {
		request.PageID = *in.PageID
	}
	answer, err := s.deps.Blocks.Answer(r.Context(), s.caller(r), request)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, answer)
}

func (s *Server) handleUsedBy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.UsedBy(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": list})
}
