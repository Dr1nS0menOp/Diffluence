// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/identity"
)

func (s *Server) registerFileRoutes() {
	r := s.router
	files := "/api/v1/attachments"
	r.HandleFunc("GET "+files+"/{id}/versions", Object("page.view"), s.handleFileVersions)
	r.HandleFunc("POST "+files+"/{id}/versions/{number}/current", Object("attachment.add"), s.handleFileMakeCurrent)
	r.HandleFunc("DELETE "+files+"/{id}/versions/{number}", Object("attachment.delete_any"), s.handleFileVersionDelete)
	r.HandleFunc("PATCH "+files+"/{id}", Object("attachment.add"), s.handleFileUpdate)
	r.HandleFunc("GET "+files+"/{id}/text", Object("page.view"), s.handleFileText)
	// A list: hidden and unknown ids answer the same, and the module decides.
	r.HandleFunc("POST "+files+"/lookup", Authenticated(), s.handleFileLookup)
	r.HandleFunc("GET /api/v1/pages/{id}/attachments.zip", Object("page.view"), s.handleFilesZip)
	r.HandleFunc("GET /api/v1/spaces/{id}/files", Object("space.view"), s.handleSpaceFiles)
	r.HandleFunc("GET /api/v1/spaces/{id}/files/usage", Object("space.administer"), s.handleSpaceFileUsage)
}

func (s *Server) handleFileVersions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Attachments.Versions(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": list})
}

func (s *Server) fileVersionAction(w http.ResponseWriter, r *http.Request, act func(identity.Caller, uuid.UUID, int) error) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	n, ok := versionNumber(w, r)
	if !ok {
		return
	}
	if err := act(s.caller(r), id, n); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFileMakeCurrent(w http.ResponseWriter, r *http.Request) {
	s.fileVersionAction(w, r, func(c identity.Caller, id uuid.UUID, n int) error {
		return s.deps.Attachments.MakeCurrent(r.Context(), c, id, n)
	})
}

func (s *Server) handleFileVersionDelete(w http.ResponseWriter, r *http.Request) {
	s.fileVersionAction(w, r, func(c identity.Caller, id uuid.UUID, n int) error {
		return s.deps.Attachments.DeleteVersion(r.Context(), c, id, n)
	})
}

func (s *Server) handleFileUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in attachments.Change
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	a, err := s.deps.Attachments.Update(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleFileText(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	text, err := s.deps.Attachments.PreviewText(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, text)
}

func (s *Server) handleFileLookup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []uuid.UUID `json:"ids"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	list, err := s.deps.Attachments.Lookup(r.Context(), s.caller(r), in.IDs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": list})
}

func (s *Server) handleFilesZip(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(uploadDeadline))
	z := &zipResponse{ResponseWriter: w, name: "attachments.zip"}
	if err := s.deps.Attachments.WriteZip(r.Context(), s.caller(r), id, z); err != nil {
		if z.started {
			s.deps.Log.Error("zip of files interrupted", "error", err, "request_id", RequestIDFrom(r.Context()))
			return
		}
		s.fail(w, r, err)
	}
}

func (s *Server) handleSpaceFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Attachments.SpaceFiles(r.Context(), s.caller(r), id, r.URL.Query().Get("q"), queryInt(r, "offset"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": list})
}

func (s *Server) handleSpaceFileUsage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	usage, err := s.deps.Attachments.SpaceUsage(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, usage)
}
