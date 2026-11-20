// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/kenwerk/kenwerk/internal/features"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/permissions"
	"github.com/kenwerk/kenwerk/internal/publishing"
)

// Draft and published spaces, wave PB2 of spec 073: the release screen of a draft space, runs
// of a branch, the whole space or chosen pages, taking pages off, leaving pages out, the
// history and its export, and the consistency check. Like every route of the part they answer
// 404 experimental_off while it is off (PUB-1303).
func (s *Server) registerReleaseRoutes() {
	r := s.router
	on := func(next http.HandlerFunc) http.HandlerFunc {
		return s.experimental(features.ExperimentalPublishing, next)
	}
	base := "/api/v1/spaces/{id}/releases"
	r.HandleFunc("GET "+base, Object(permissions.SpaceRelease), on(s.handleReleaseScreen))
	r.HandleFunc("POST "+base+"/preview", Object(permissions.SpaceRelease), on(s.handleRunPreview))
	r.HandleFunc("GET "+base+"/history", Object(permissions.SpaceRelease), on(s.handleRunHistory))
	r.HandleFunc("GET "+base+"/as-of", Object(permissions.SpaceRelease), on(s.handleReleasedAsOf))
	r.HandleFunc("GET "+base+"/export", Object(permissions.SpaceAudit), on(s.handleReleaseExport))
	r.HandleFunc("POST "+base+"/check", Object(permissions.SpaceAdminister), on(s.handleReleaseCheck))
	r.HandleFunc("POST "+base+"/verify", Object(permissions.SpaceAudit), on(s.handleReleaseVerifySpace))
	r.HandleFunc("GET "+base+"/{run}", Object(permissions.SpaceRelease), on(s.handleRunView))
	r.HandleFunc("POST "+base+"/{run}/start", Object(permissions.SpaceRelease), on(s.handleRunStart))
	r.HandleFunc("POST "+base+"/{run}/cancel", Object(permissions.SpaceRelease), on(s.handleRunCancel))
	r.HandleFunc("POST "+base+"/{run}/resume", Object(permissions.SpaceRelease), on(s.handleRunResume))
	r.HandleFunc("GET /api/v1/spaces/{id}/publishing/linking-in", Object(permissions.SpaceRelease), on(s.handleLinkingIn))
	r.HandleFunc("GET /api/v1/spaces/{id}/publishing/end-preview", Object(permissions.SpaceAdminister), on(s.handleEndPreview))
	r.HandleFunc("PUT /api/v1/pages/{id}/leave-out", Object(permissions.SpaceRelease), on(s.handleLeaveOut))
	r.HandleFunc("DELETE /api/v1/pages/{id}/leave-out", Object(permissions.SpaceRelease), on(s.handleInclude))
}

func (s *Server) handleReleaseScreen(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = publishing.TabToRelease
	}
	screen, err := s.deps.Publishing.Screen(r.Context(), s.caller(r), id, tab, queryInt(r, "offset"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, screen)
}

func (s *Server) handleRunPreview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in publishing.RunScope
	if !body(w, r, &in) {
		return
	}
	preview, err := s.deps.Publishing.PreviewRun(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

func (s *Server) handleRunHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	runs, err := s.deps.Publishing.History(r.Context(), s.caller(r), id, queryInt(r, "offset"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

// releaseTime reads a moment from the query, or now when it is not given.
func releaseTime(w http.ResponseWriter, r *http.Request, key string, fallback time.Time) (time.Time, bool) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback, true
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "A moment is written as 2026-11-14T09:00:00Z.")
		return t, false
	}
	return t, true
}

func (s *Server) handleReleasedAsOf(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	at, ok := releaseTime(w, r, "at", time.Now())
	if !ok {
		return
	}
	var page *uuid.UUID
	if v := r.URL.Query().Get("page_id"); v != "" {
		parsed, err := uuid.Parse(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "That is not the id of a page.")
			return
		}
		page = &parsed
	}
	list, err := s.deps.Publishing.AsOf(r.Context(), s.caller(r), id, at, page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"at": at, "pages": list})
}

func (s *Server) handleReleaseExport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	to, ok := releaseTime(w, r, "to", time.Now())
	if !ok {
		return
	}
	from, ok := releaseTime(w, r, "from", to.AddDate(-1, 0, 0))
	if !ok {
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = publishing.FormatCSV
	}
	// The whole file is made first, so a refusal is an ordinary answer and not half a file.
	var buf bytes.Buffer
	if err := s.deps.Publishing.ExportHistory(r.Context(), s.caller(r), id, from, to, format, &buf); err != nil {
		s.fail(w, r, err)
		return
	}
	contentType := "text/csv; charset=utf-8"
	if format == publishing.FormatLines {
		contentType = "application/jsonl; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+publishing.ExportName(format, from, to)+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes()) //nolint:errcheck // the answer has begun; a client that went away cannot be told
}

func (s *Server) handleReleaseCheck(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Fix bool `json:"fix"`
	}
	if !body(w, r, &in) {
		return
	}
	res, err := s.deps.Publishing.Check(r.Context(), s.caller(r), id, in.Fix)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleReleaseVerifySpace queues the verification of every twin of a pair (spec 073, PUB-920);
// the report is the result of its job.
func (s *Server) handleReleaseVerifySpace(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	job, err := s.deps.Publishing.VerifySpace(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleRunView(w http.ResponseWriter, r *http.Request) {
	s.withRun(w, r, s.deps.Publishing.RunView)
}

func (s *Server) handleRunCancel(w http.ResponseWriter, r *http.Request) {
	s.withRun(w, r, s.deps.Publishing.Cancel)
}

func (s *Server) handleRunResume(w http.ResponseWriter, r *http.Request) {
	s.withRun(w, r, s.deps.Publishing.Resume)
}

// withRun reads the space and the run of a route and answers with the run.
func (s *Server) withRun(w http.ResponseWriter, r *http.Request, do func(context.Context, identity.Caller, uuid.UUID, uuid.UUID) (publishing.Run, error)) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	run, ok := pathID(w, r, "run")
	if !ok {
		return
	}
	out, err := do(r.Context(), s.caller(r), id, run)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRunStart(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	run, ok := pathID(w, r, "run")
	if !ok {
		return
	}
	var in publishing.RunInput
	if !body(w, r, &in) {
		return
	}
	out, err := s.deps.Publishing.StartRun(r.Context(), s.caller(r), id, run, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

func (s *Server) handleLinkingIn(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Publishing.LinkingIn(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": list})
}

func (s *Server) handleEndPreview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.deps.Publishing.PreviewEnd(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleLeaveOut(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in publishing.LeaveOutInput
	if !body(w, r, &in) {
		return
	}
	out, err := s.deps.Publishing.LeaveOut(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleInclude(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.deps.Publishing.Include(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
