// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"

	"github.com/diffluence/diffluence/internal/features"
)

// experimental serves next only while an experimental part is on (spec 069). Access is
// checked first, so a caller without a session still gets 401 and one without the
// right still gets what the access rule says; only then does an off part answer 404
// with the code experimental_off.
func (s *Server) experimental(part string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.deps.Features.RequireExperimental(r.Context(), s.deps.Pool, part); err != nil {
			s.fail(w, r, err)
			return
		}
		next(w, r)
	}
}

func (s *Server) handleExperimental(w http.ResponseWriter, r *http.Request) {
	v, err := s.deps.Features.Experimental(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"on": v.On, "all": features.ExperimentalParts})
}

func (s *Server) handleExperimentalSet(w http.ResponseWriter, r *http.Request) {
	var in features.Experimental
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Features.SetExperimental(r.Context(), s.caller(r), in); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
