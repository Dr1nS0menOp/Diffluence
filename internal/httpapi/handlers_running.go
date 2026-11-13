// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"os"
	"strings"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/backups"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/restore"
)

// registerRunningRoutes are the parts of the console that run the installation (spec 058):
// backups, the settings the server started with, and logging people out.
func (s *Server) registerRunningRoutes() {
	r := s.router
	admin := SessionOnly(Permission(identity.PermAdministrator))
	r.HandleFunc("GET /api/v1/admin/backups", admin, s.handleBackups)
	r.HandleFunc("POST /api/v1/admin/backups/request", admin, s.handleBackupRequest)
	// A restore over the live installation is the most dangerous action there is: it asks
	// for a fresh login and, on top of it, a second factor or a passkey (spec 059). It
	// brings back every account and grant of the backup, so it is for system
	// administrators only (spec 066).
	system := SessionOnly(Permission(identity.PermSystemAdministrator))
	// Restore from the console is an experimental part (spec 069). Cancelling stays open,
	// so a restore asked for before the part was turned off can still be stopped.
	r.HandleFunc("POST /api/v1/admin/backups/restore", Sensitive(system), s.experimental(features.ExperimentalConsoleRestore, s.handleRestoreRequest))
	// The backup service goes on only with the code it wrote to its own log (spec 070): the
	// code is the proof, and the request before it already asked for a fresh login. Like
	// cancelling, it stays open with the part off, for a request asked for before.
	r.HandleFunc("POST /api/v1/admin/backups/restore/code", system, s.handleRestoreCode)
	r.HandleFunc("DELETE /api/v1/admin/backups/restore", admin, s.handleRestoreCancel)
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
	latest, err := restore.Latest(r.Context(), s.deps.Pool)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		backups.Overview
		Restore          *restore.Restore `json:"restore"`
		InstallationName string           `json:"installation_name"`
		RestoresPerDay   int              `json:"restores_per_day"`
		RestoreCodeTries int              `json:"restore_code_tries"`
	}{overview, latest, s.deps.Config.InstanceName, restore.PerDay, restore.CodeTries})
}

// handleRestoreRequest asks the backup worker to restore a backup over the installation.
// The name is checked before the second factor, so a typing error costs no code.
func (s *Server) handleRestoreRequest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name    string `json:"name"`
		Confirm string `json:"confirm"`
		Code    string `json:"code"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	installation := s.deps.Config.InstanceName
	if strings.TrimSpace(in.Confirm) != installation {
		s.fail(w, r, apperr.Invalid("Type the name of this installation, "+installation+", to confirm."))
		return
	}
	p := PrincipalFrom(r.Context())
	c := s.caller(r)
	// A code is six digits: without a limit per account, a stolen session that passed
	// the password could keep guessing.
	if strings.TrimSpace(in.Code) != "" && !s.loginByAcc.allow("2fa:"+p.User.ID.String()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Try again later.")
		return
	}
	if err := s.deps.Identity.ConfirmSecondFactor(r.Context(), c, p.SessionID, strings.TrimSpace(in.Code)); err != nil {
		s.fail(w, r, err)
		return
	}
	rs, err := restore.Request(r.Context(), s.deps.Pool, restore.Ask{BackupName: in.Name, Confirm: in.Confirm,
		Installation: installation, Caller: c, CallerName: p.User.DisplayName})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, rs)
}

// handleRestoreCode passes the code from the backup service's log on to it.
func (s *Server) handleRestoreCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := restore.EnterCode(r.Context(), s.deps.Pool, s.caller(r), in.Code); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRestoreCancel(w http.ResponseWriter, r *http.Request) {
	if err := restore.Cancel(r.Context(), s.deps.Pool, s.caller(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
