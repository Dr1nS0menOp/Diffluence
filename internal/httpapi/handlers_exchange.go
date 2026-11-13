// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"io"
	"net/http"
	"time"

	"github.com/diffluence/diffluence/internal/exchange"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
)

const maxExchangeBody = 1 << 20

func (s *Server) registerExchangeRoutes() {
	r := s.router
	me := Authenticated()
	admin := Permission(identity.PermAdministrator)

	r.HandleFunc("GET /api/v1/export-formats", me, s.handleExportFormats)
	r.HandleFunc("POST /api/v1/exports", Object("space.export"), s.handleExportStart)
	// An export belongs to the person who made it, and the module decides that:
	// for anyone else it does not exist.
	r.HandleFunc("GET /api/v1/exports/{id}", me, s.handleExportGet)
	r.HandleFunc("GET /api/v1/exports/{id}/file", me, s.handleExportFile)
	r.HandleFunc("GET /api/v1/spaces/{id}/pdf-design", Object("space.administer"), s.handlePDFDesign)
	r.HandleFunc("PUT /api/v1/spaces/{id}/pdf-design", Object("space.administer"), s.handlePDFDesignSet)
	// An import belongs to the person who started it, and the module decides that too.
	r.HandleFunc("GET /api/v1/imports/{id}", me, s.handleImportGet)
	r.HandleFunc("PUT /api/v1/imports/{id}/mapping", me, s.handleImportMapping)
	r.HandleFunc("POST /api/v1/imports/{id}/run", me, s.handleImportRun)
	r.HandleFunc("POST /api/v1/imports/{id}/undo", me, s.handleImportUndo)
	r.HandleFunc("POST /api/v1/pages/{id}/import-word", Object("page.edit"), s.handleImportWord)
	// Bundles between instances are an experimental part (spec 069).
	r.HandleFunc("POST /api/v1/spaces/{id}/bundles", Object("space.bundle"), s.experimental(features.ExperimentalBundles, s.handleBundleStart))
	r.HandleFunc("POST /api/v1/admin/bundles", admin, s.experimental(features.ExperimentalBundles, s.handleBundleImport))
	r.HandleFunc("GET /api/v1/admin/bundles/key", admin, s.experimental(features.ExperimentalBundles, s.handleBundleKey))
	r.HandleFunc("GET /api/v1/admin/export-policy", admin, s.handleExportPolicy)
	r.HandleFunc("PUT /api/v1/admin/export-policy", admin, s.handleExportPolicySet)
}

func (s *Server) handleExportFormats(w http.ResponseWriter, r *http.Request) {
	formats, err := s.deps.Exchange.AllowedFormats(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"formats": formats})
}

func (s *Server) handleExportStart(w http.ResponseWriter, r *http.Request) {
	var in exchange.ExportRequest
	if decodeJSON(w, r, maxExchangeBody, &in) != nil {
		badBody(w)
		return
	}
	run, err := s.deps.Exchange.StartExport(r.Context(), s.caller(r), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) handleExportGet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	run, err := s.deps.Exchange.Export(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleExportFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	run, file, err := s.deps.Exchange.OpenExport(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", attachmentHeader(run.Filename))
	if _, err := io.Copy(w, file); err != nil {
		s.deps.Log.Error("export download interrupted", "error", err, "request_id", RequestIDFrom(r.Context()))
	}
}

func (s *Server) handlePDFDesign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	design, err := s.deps.Exchange.PDFDesignOf(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, design)
}

func (s *Server) handlePDFDesignSet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in exchange.PDFDesign
	if decodeJSON(w, r, maxExchangeBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Exchange.SetPDFDesign(r.Context(), s.caller(r), id, in); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleExportPolicy(w http.ResponseWriter, r *http.Request) {
	policy, err := s.deps.Exchange.Policy(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) handleExportPolicySet(w http.ResponseWriter, r *http.Request) {
	var in exchange.ExportPolicy
	if decodeJSON(w, r, maxExchangeBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Exchange.SetPolicy(r.Context(), s.caller(r), in); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleImportGet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	run, err := s.deps.Exchange.Import(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleImportMapping(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in exchange.ImportMapping
	if decodeJSON(w, r, maxExchangeBody, &in) != nil {
		badBody(w)
		return
	}
	run, err := s.deps.Exchange.SetMapping(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleImportRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	run, err := s.deps.Exchange.RunImport(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) handleImportUndo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Rest bool `json:"rest"`
	}
	if decodeJSON(w, r, maxExchangeBody, &in) != nil {
		badBody(w)
		return
	}
	out, err := s.deps.Exchange.UndoImport(r.Context(), s.caller(r), id, in.Rest)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleImportWord(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadDeadline))
	out, err := s.deps.Exchange.WordIntoDraft(r.Context(), s.caller(r), id, r.Body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleBundleStart(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	run, err := s.deps.Exchange.StartBundle(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) handleBundleImport(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadDeadline))
	run, err := s.deps.Exchange.StartBundleImport(r.Context(), s.caller(r), r.URL.Query().Get("filename"), r.Body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) handleBundleKey(w http.ResponseWriter, r *http.Request) {
	key, err := s.deps.Exchange.OwnBundleKey()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, key)
}
