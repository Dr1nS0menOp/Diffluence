// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"

	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/publishing"
)

// Draft and published spaces, wave PB1 of spec 073: linking a pair, its settings, the status
// of a page and releasing one page. The part is experimental (PUB-1303); every route answers
// 404 experimental_off while it is off, except ending a pair, which stays open so a published
// space can always be given back to its editors (PUB-1304).
func (s *Server) registerPublishingRoutes() {
	r := s.router
	on := func(next http.HandlerFunc) http.HandlerFunc {
		return s.experimental(features.ExperimentalPublishing, next)
	}
	spacesPath, pagesPath := "/api/v1/spaces/{id}/publishing", "/api/v1/pages/{id}/release"
	r.HandleFunc("GET "+spacesPath, Object(permissions.SpaceView), on(s.handlePublishingState))
	r.HandleFunc("POST "+spacesPath+"/link-preview", Object(permissions.SpaceAdminister), on(s.handlePublishingLinkPreview))
	r.HandleFunc("POST "+spacesPath+"/link", Object(permissions.SpaceAdminister), on(s.handlePublishingLink))
	r.HandleFunc("PUT "+spacesPath+"/settings", Object(permissions.SpaceAdminister), on(s.handlePublishingSettings))
	r.HandleFunc("POST "+spacesPath+"/end", Object(permissions.SpaceAdminister), s.handlePublishingEnd)
	// Wave PB4: a draft space started from a space with content (PUB-1302), and undone before
	// its first release (PUB-1311), which is a way of ending and stays open as ending does.
	r.HandleFunc("POST "+spacesPath+"/start-preview", Object(permissions.SpaceAdminister), on(s.handlePublishingStartPreview))
	r.HandleFunc("POST "+spacesPath+"/start", Object(permissions.SpaceAdminister), on(s.handlePublishingStart))
	r.HandleFunc("POST "+spacesPath+"/undo-start", Object(permissions.SpaceAdminister), s.handlePublishingUndoStart)
	r.HandleFunc("GET "+pagesPath, Object(permissions.ActionPageView), on(s.handleReleaseStatus))
	r.HandleFunc("POST "+pagesPath+"/preview", Object(permissions.SpaceRelease), on(s.handleReleasePreview))
	r.HandleFunc("POST "+pagesPath, Object(permissions.SpaceRelease), on(s.handleRelease))
	r.HandleFunc("POST "+pagesPath+"/verify", Object(permissions.SpaceAudit), on(s.handleReleaseVerify))
	// The button of the health page: the consistency check of every pair, fixing when asked
	// (PUB-343).
	r.HandleFunc("POST /api/v1/admin/publishing/check", SessionOnly(Permission(identity.PermAdministrator)), on(s.handlePublishingCheckAll))
}

func (s *Server) handlePublishingCheckAll(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Fix bool `json:"fix"`
	}
	if !body(w, r, &in) {
		return
	}
	out, err := s.deps.Publishing.CheckEverything(r.Context(), s.caller(r), in.Fix)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleReleaseVerify checks the twin of a draft page against the draft version it came from
// (spec 073, PUB-732).
func (s *Server) handleReleaseVerify(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.deps.Publishing.Verify(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePublishingState(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	state, err := s.deps.Publishing.State(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handlePublishingLinkPreview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in publishing.LinkInput
	if !body(w, r, &in) {
		return
	}
	preview, err := s.deps.Publishing.PreviewLink(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handlePublishingLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in publishing.LinkInput
	if !body(w, r, &in) {
		return
	}
	state, err := s.deps.Publishing.Link(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) handlePublishingSettings(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in publishing.Settings
	if !body(w, r, &in) {
		return
	}
	settings, err := s.deps.Publishing.SetSettings(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handlePublishingEnd(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.deps.Publishing.End(r.Context(), s.caller(r), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePublishingStartPreview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in publishing.StartInput
	if !body(w, r, &in) {
		return
	}
	preview, err := s.deps.Publishing.PreviewStart(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handlePublishingStart(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in publishing.StartInput
	if !body(w, r, &in) {
		return
	}
	job, err := s.deps.Publishing.Start(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handlePublishingUndoStart(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.deps.Publishing.UndoStart(r.Context(), s.caller(r), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReleaseStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	status, err := s.deps.Publishing.Status(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleReleasePreview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	preview, err := s.deps.Publishing.Preview(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleRelease(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in publishing.RunInput
	if !body(w, r, &in) {
		return
	}
	report, err := s.deps.Publishing.Release(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
