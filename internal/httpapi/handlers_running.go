// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"os"
	"strings"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/identity"
)

// registerRunningRoutes are the parts of the console that run the installation (spec 058):
// backups, the settings the server started with, and logging people out.
func (s *Server) registerRunningRoutes() {
	r := s.router
	admin := SessionOnly(Permission(identity.PermAdministrator))
	r.HandleFunc("GET /api/v1/admin/backups", admin, s.handleBackups)
	r.HandleFunc("POST /api/v1/admin/backups/request", admin, s.handleBackupRequest)
	r.HandleFunc("GET /api/v1/admin/configuration", admin, s.handleConfiguration)
	// Logging everyone out is an incident action; asking for the password again first
	// keeps a stolen session from using it as a weapon.
	r.HandleFunc("POST /api/v1/admin/sessions/end", Sensitive(admin), s.handleEndSessions)
}

func (s *Server) handleBackups(w http.ResponseWriter, r *http.Request) {
	overview, err := s.deps.Backups.Overview(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (s *Server) handleBackupRequest(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Backups.RequestNow(r.Context(), s.caller(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleConfiguration shows the settings the server started with and where each came
// from, as diffluence config check prints them: secrets as (set), database addresses
// without their password. It reads the environment and the file again, so it shows what
// a restart would take; problems are listed by key and line, never with a value.
func (s *Server) handleConfiguration(w http.ResponseWriter, _ *http.Request) {
	_, list, err := config.Inspect(os.LookupEnv)
	problems := []string{}
	if err != nil {
		problems = strings.Split(err.Error(), "\n")
	}
	type setting struct {
		Name   string `json:"name"`
		Value  string `json:"value"`
		Source string `json:"source"`
	}
	out := make([]setting, 0, len(list))
	for _, item := range list {
		out = append(out, setting{Name: item.Name, Value: item.Value, Source: item.Source})
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": out, "problems": problems})
}

func (s *Server) handleEndSessions(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
		All   bool   `json:"all"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	// Everyone needs all, so a missing address can never log everyone out by mistake.
	if (strings.TrimSpace(in.Email) == "") == !in.All {
		s.fail(w, r, apperr.Invalid("Give either one address, or all."))
		return
	}
	c := s.caller(r)
	ended, err := s.deps.Identity.EndSessions(r.Context(), c.Actor(), strings.TrimSpace(in.Email), c.Meta)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ended": ended})
}
