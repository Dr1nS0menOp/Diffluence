// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"time"

	"github.com/diffluence/diffluence/internal/identity"
)

// fontCacheSeconds can be long: a replaced font gets a new versioned address.
const fontCacheSeconds = "31536000"

func (s *Server) registerAppearanceRoutes() {
	r := s.router
	// Public: the login page shows the installation's font before anyone is logged
	// in. It reveals only which font an administrator imported.
	r.HandleFunc("GET /api/v1/appearance", Public, s.handleAppearance)
	// Public: same reason; a font file holds no data of the installation.
	r.HandleFunc("GET /api/v1/appearance/font", Public, s.handleAppearanceFont)

	admin := Permission(identity.PermAdministrator)
	r.HandleFunc("PUT /api/v1/admin/appearance/font", admin, s.handleFontImport)
	r.HandleFunc("DELETE /api/v1/admin/appearance/font", admin, s.handleFontRemove)
}

type fontJSON struct {
	Name    string `json:"name"`
	Format  string `json:"format"`
	Version string `json:"version"`
}

func (s *Server) handleAppearance(w http.ResponseWriter, r *http.Request) {
	font, found, err := s.deps.Settings.CurrentFont(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !found {
		writeJSON(w, http.StatusOK, map[string]any{"font": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"font": fontJSON{Name: font.Name, Format: font.Format, Version: font.Version}})
}

func (s *Server) handleAppearanceFont(w http.ResponseWriter, r *http.Request) {
	font, file, err := s.deps.Settings.OpenFont(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer func() { _ = file.Close() }()
	h := w.Header()
	h.Set("Content-Type", "font/"+font.Format)
	h.Set("X-Content-Type-Options", "nosniff")
	if r.URL.Query().Get("v") == font.Version {
		h.Set("Cache-Control", "public, max-age="+fontCacheSeconds+", immutable")
	} else {
		h.Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, "", time.Time{}, file)
}

func (s *Server) handleFontImport(w http.ResponseWriter, r *http.Request) {
	font, err := s.deps.Settings.ImportFont(r.Context(), s.caller(r), r.URL.Query().Get("filename"), r.Body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, fontJSON{Name: font.Name, Format: font.Format, Version: font.Version})
}

func (s *Server) handleFontRemove(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Settings.RemoveFont(r.Context(), s.caller(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
