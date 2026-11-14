// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/monitoring"
	"github.com/diffluence/diffluence/internal/settings"
)

// Installation is what the console's areas of the running installation read (spec 075, wave
// CS1). The wiring gives each; every one only reads.
type Installation struct {
	// Upgrades reads the nodes, the migrations and the last upgrade marker.
	Upgrades func(ctx context.Context) (any, error)
	// Events reads where the events transport stands, as diffluence events status does.
	Events func(ctx context.Context) (any, error)
	// Audit reads the newest checkpoint, the signing keys and the newest witness report.
	Audit func(ctx context.Context) (any, error)
	// LogLevel reads the log level for a while and the level each node runs at.
	LogLevel func(ctx context.Context) (any, error)
	// StartedWith is the configuration as this process read it at its start.
	StartedWith []config.Setting
	// LastProbe is the last try of the settings used only later, and whether one ran.
	LastProbe func() (time.Time, []config.Warning, bool)
}

// registerInstallationRoutes are the areas that show what runs the installation, and the log
// level for a while (spec 075, wave CS1). Every administrator may read them (CSL-610); a
// quieter log level is a system administrator's, with a fresh login and a second factor.
func (s *Server) registerInstallationRoutes() {
	r := s.router
	admin := SessionOnly(Permission(identity.PermAdministrator))
	r.HandleFunc("GET /api/v1/admin/upgrades", admin, s.readArea(func() func(context.Context) (any, error) { return s.deps.Installation.Upgrades }))
	r.HandleFunc("GET /api/v1/admin/events", admin, s.readArea(func() func(context.Context) (any, error) { return s.deps.Installation.Events }))
	r.HandleFunc("GET /api/v1/admin/audit-overview", admin, s.readArea(func() func(context.Context) (any, error) { return s.deps.Installation.Audit }))
	r.HandleFunc("GET /api/v1/admin/log-level", admin, s.readArea(func() func(context.Context) (any, error) { return s.deps.Installation.LogLevel }))
	r.HandleFunc("PUT /api/v1/admin/log-level", admin, s.handleLogLevelSet)
	r.HandleFunc("PUT /api/v1/admin/log-level/confirmed", Sensitive(Permission(identity.PermSystemAdministrator)), s.handleLogLevelConfirmed)
	r.HandleFunc("DELETE /api/v1/admin/log-level", admin, s.handleLogLevelEnd)
	r.HandleFunc("GET /api/v1/admin/monitoring", admin, s.handleMonitoring)
	r.HandleFunc("GET /api/v1/admin/monitoring/{kind}", admin, s.handleMonitoringFile)
	r.HandleFunc("GET /api/v1/admin/sessions/count", admin, s.handleSessionCount)
}

// readArea answers with what an area's reader gives. The reader is looked up per request, so
// a test that builds the server alone needs none of them.
func (s *Server) readArea(reader func() func(context.Context) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		read := reader()
		if read == nil {
			s.fail(w, r, apperr.NotFound())
			return
		}
		v, err := read(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func (s *Server) handleLogLevelSet(w http.ResponseWriter, r *http.Request) {
	var in settings.LogRequest
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	o, err := s.deps.Settings.SetLogOverride(r.Context(), s.caller(r), in, s.deps.Config.LogLevel)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// handleLogLevelConfirmed stores a quieter log level for a system administrator who logged in
// again (route rule) and gives a second factor now. The request is checked first, so a wrong
// level costs no code.
func (s *Server) handleLogLevelConfirmed(w http.ResponseWriter, r *http.Request) {
	var in struct {
		settings.LogRequest
		Code string `json:"code"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if !slices.Contains(settings.LogLevels, in.Level) || !slices.Contains(settings.LogHours, in.Hours) {
		s.fail(w, r, apperr.Invalid("A log level is debug, info, warn or error, for 1, 4 or 24 hours."))
		return
	}
	if !s.confirmSecondFactor(w, r, in.Code) {
		return
	}
	o, err := s.deps.Settings.SetLogOverrideConfirmed(r.Context(), s.caller(r), in.LogRequest, s.deps.Config.LogLevel)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) handleLogLevelEnd(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Settings.EndLogOverride(r.Context(), s.caller(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleMonitoring lists the monitoring files, and whether /metrics is on: whether a token is
// set, never the token (CSL-587).
func (s *Server) handleMonitoring(w http.ResponseWriter, _ *http.Request) {
	type file struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	}
	files := []file{}
	for _, f := range monitoring.Files() {
		files = append(files, file{Kind: f.Kind, Name: f.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"metrics_on": s.deps.Config.MetricsToken != "", "files": files})
}

// handleMonitoringFile sends a monitoring file of this release as a download (CSL-585).
func (s *Server) handleMonitoringFile(w http.ResponseWriter, r *http.Request) {
	f, ok := monitoring.ByKind(r.PathValue("kind"))
	if !ok {
		s.fail(w, r, apperr.NotFound())
		return
	}
	h := w.Header()
	h.Set("Content-Type", f.ContentType)
	h.Set("Content-Disposition", `attachment; filename="`+f.Name+`"`)
	h.Set("Content-Length", strconv.Itoa(len(f.Body)))
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	// A failed write is a client that went away; there is nobody left to answer.
	_, _ = w.Write(f.Body)
}

// handleSessionCount says how many sessions of how many people logging everyone out ends
// (CSL-541).
func (s *Server) handleSessionCount(w http.ResponseWriter, r *http.Request) {
	c, err := s.deps.Identity.CountSessions(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// confirmSecondFactor asks the person behind the request for a second factor now: a code from
// the authenticator app, a recovery code, or a passkey confirmed in this session just before.
// Wrong codes count against the same limit per account as a restore. It reports whether the
// request may go on; when not, it has answered.
func (s *Server) confirmSecondFactor(w http.ResponseWriter, r *http.Request, code string) bool {
	p := PrincipalFrom(r.Context())
	code = strings.TrimSpace(code)
	// A stolen session that passed the password must not keep guessing.
	if code != "" && !s.loginByAcc.allow("2fa:"+p.User.ID.String()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Try again later.")
		return false
	}
	if err := s.deps.Identity.ConfirmSecondFactor(r.Context(), s.caller(r), p.SessionID, code); err != nil {
		s.fail(w, r, err)
		return false
	}
	return true
}

// configurationRow is one setting of the Configuration page.
type configurationRow struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Source string `json:"source"`
	// ChangesAtRestart marks a value or source that differs from what the process started
	// with; Running is that, when the process read the setting at its start.
	ChangesAtRestart bool               `json:"changes_at_restart"`
	Running          *configurationSeen `json:"running"`
}

type configurationSeen struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

// handleConfiguration shows the settings the server started with and where each came
// from, as diffluence config check prints them: secrets as (set), database addresses
// without their password. It reads the environment and the file again, so it shows what
// a restart would take, and marks what differs from the start (CSL-601). Problems are
// listed by key and line, never with a value. Beside them: the variables of the trace
// exporter (CSL-604) and the last try of the settings used only later (CSL-608).
func (s *Server) handleConfiguration(w http.ResponseWriter, _ *http.Request) {
	_, list, err := config.Inspect(os.LookupEnv)
	problems := []string{}
	if err != nil {
		problems = strings.Split(err.Error(), "\n")
	}
	out := []configurationRow{}
	for _, row := range config.CompareWithStart(s.deps.Installation.StartedWith, list) {
		shown := configurationRow{Name: row.Name, Value: row.Value, Source: row.Source, ChangesAtRestart: row.ChangesAtRestart}
		if row.Running != nil {
			shown.Running = &configurationSeen{Value: row.Running.Value, Source: row.Running.Source}
		}
		out = append(out, shown)
	}
	tracing := []configurationRow{}
	for _, v := range config.TracingVariables(os.Environ()) {
		tracing = append(tracing, configurationRow{Name: v.Name, Value: v.Value, Source: v.Source})
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": out, "problems": problems, "tracing": tracing,
		"probe": s.lastProbe()})
}

// probeShown is the last try of the settings used only later.
type probeShown struct {
	Done     bool          `json:"done"`
	At       *time.Time    `json:"at"`
	Warnings []probeWarned `json:"warnings"`
}

type probeWarned struct {
	Setting string `json:"setting"`
	Message string `json:"message"`
}

func (s *Server) lastProbe() probeShown {
	out := probeShown{Warnings: []probeWarned{}}
	if s.deps.Installation.LastProbe == nil {
		return out
	}
	at, warnings, done := s.deps.Installation.LastProbe()
	if !done {
		return out
	}
	out.Done, out.At = true, &at
	for _, w := range warnings {
		out.Warnings = append(out.Warnings, probeWarned{Setting: w.Setting, Message: w.Message})
	}
	return out
}
