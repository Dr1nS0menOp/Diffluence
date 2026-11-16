// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"

	"github.com/diffluence/diffluence/internal/permissions"
)

// The routes of where pages came from (spec 074, wave GT5): the origin in a page's Details,
// and the sets a version made. Each is behind the experimental switch of guided templates;
// the templates module decides who may see what.
func (s *Server) registerTemplateHistoryRoutes(guided func(http.HandlerFunc) http.HandlerFunc) {
	r := s.router
	r.HandleFunc("GET /api/v1/pages/{id}/template-origin", Object(permissions.ActionPageView), guided(s.handlePageTemplateOrigin))
	r.HandleFunc("GET /api/v1/templates/{id}/versions/{number}/sets", Authenticated(), guided(s.handleTemplateVersionSets))
}

func (s *Server) handlePageTemplateOrigin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	origin, err := s.deps.Templates.Origin(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, origin)
}

func (s *Server) handleTemplateVersionSets(w http.ResponseWriter, r *http.Request) {
	id, number, ok := versionPath(w, r)
	if !ok {
		return
	}
	sets, err := s.deps.Templates.SetsOf(r.Context(), s.caller(r), id, number)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sets)
}
