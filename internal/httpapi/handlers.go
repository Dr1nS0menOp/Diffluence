// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/version"
	"github.com/diffluence/diffluence/internal/webui"
)

const loginWindow = 15 * time.Minute

func (s *Server) registerRoutes() {
	r := s.router
	// Public: probes from Docker/Kubernetes/load balancers carry no session.
	r.HandleFunc("GET /livez", Public, handleLivez)
	r.HandleFunc("GET /readyz", Public, readyzHandler(s.ready, s.deps.ReadyNotes, s.deps.Log))
	// Public because a scraper has no session: the handler checks its own token.
	r.HandleFunc("GET /metrics", Public, s.handleMetrics)
	// Public: this is how a session is obtained. Rate limited and audited.
	r.HandleFunc("POST /api/v1/auth/login", Public, s.handleLogin)
	// Public: the web app itself is static files with no data in them. The login
	// page has to load before anyone is logged in. Unknown API paths get a JSON 404.
	r.Handle("/", Public, webui.Handler(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
	}, s.deps.Config.DrawingOrigin(), s.deps.Config.PublicOrigin()))

	r.HandleFunc("POST /api/v1/auth/logout", Authenticated(), s.handleLogout)
	r.HandleFunc("GET /api/v1/me", Authenticated(), s.handleMe)
	r.HandleFunc("GET /api/v1/system/info", Permission(identity.PermUseProduct), s.handleSystemInfo)
	r.HandleFunc("GET /api/v1/system/capabilities", Permission(identity.PermUseProduct), s.handleCapabilities)

	auditor := SessionOnly(Permission(identity.PermAdministrator))
	r.HandleFunc("GET /api/v1/admin/audit-events", auditor, s.handleAuditList)
	r.HandleFunc("GET /api/v1/admin/audit-events/export", auditor, s.handleAuditExport)
	r.HandleFunc("GET /api/v1/admin/audit-events/verify", auditor, s.handleAuditVerify)

	s.registerContentRoutes()
	s.registerReadingRoutes()
	s.registerBlockRoutes()
	s.registerLinkRoutes()
	s.registerFileRoutes()
	s.registerOrganisingRoutes()
	s.registerWorkflowRoutes()
	s.registerTemplateRoutes()
	s.registerTogetherRoutes()
	s.registerSavedBlockRoutes()
	s.registerIdentityRoutes()
	s.registerAppearanceRoutes()
	s.registerPeopleRoutes()
	s.registerExchangeRoutes()
	s.registerSiteRoutes()
	s.registerConsoleRoutes()
	s.registerRoleRoutes()
	s.registerInsightRoutes()
	s.registerRuleRoutes()
	s.registerCollabRoutes()
	s.registerSetupRoutes()
	s.registerRunningRoutes()
	s.registerPublishingRoutes()
}

type userJSON struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Type        string    `json:"type"`
	CreatedAt   time.Time `json:"created_at"`
}

func toUserJSON(u identity.User) userJSON {
	return userJSON{ID: u.ID.String(), Email: u.Email, DisplayName: u.DisplayName, Type: u.Type, CreatedAt: u.CreatedAt}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, 16<<10, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Send JSON with email and password.")
		return
	}
	meta := metaFrom(r)
	// An account alone is not the key: anyone could then lock a person out by guessing
	// wrong on purpose (spec 025). The wide limit stops guesses from many addresses.
	allAddressesKey := "acc:" + identityKey(in.Email)
	accKey := allAddressesKey + "|" + addressKey(meta.IP)
	if !s.loginByIP.allow(addressKey(meta.IP)) || !s.loginByAcc.allow(accKey) || !s.loginByAllIPs.allow(allAddressesKey) {
		w.Header().Set("Retry-After", strconv.Itoa(int(loginWindow.Seconds())))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many login attempts. Try again later.")
		return
	}
	token, p, err := s.deps.Identity.Login(r.Context(), in.Email, in.Password, meta)
	if errors.Is(err, identity.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Email or password is incorrect.")
		return
	}
	var mfa *identity.MFARequiredError
	if errors.As(err, &mfa) {
		s.loginByIP.giveBack(addressKey(meta.IP))
		writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true, "mfa_token": mfa.Token, "mfa_methods": mfa.Methods})
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.loginByIP.giveBack(addressKey(meta.IP))
	s.loginByAllIPs.giveBack(allAddressesKey)
	s.loginByAcc.reset(accKey)
	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(p.User)})
}

func identityKey(email string) string {
	b := []byte(email)
	if len(b) > 254 {
		b = b[:254]
	}
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Identity.Logout(r.Context(), *PrincipalFrom(r.Context()), metaFrom(r)); err != nil {
		s.internalError(w, r, err)
		return
	}
	// Secure follows the public address: on for https, off only where a browser would drop it, on plain http.
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
	http.SetCookie(w, &http.Cookie{Name: s.cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true,
		Secure: s.deps.Config.SecureCookies(), SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	perms, err := s.deps.Identity.GlobalPermissions(r.Context(), p.User.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if perms == nil {
		perms = []string{}
	}
	preferences, err := s.deps.Identity.Preferences(r.Context(), p.User.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	mine, err := s.deps.Spaces.PersonalOf(r.Context(), p.User.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	var personal string
	if mine != nil {
		personal = mine.Key
	}
	signIn, err := s.deps.Identity.SignInStatus(r.Context(), p.User.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	access := settings.DefaultAccess
	if err := s.deps.Settings.ReadSection(r.Context(), settings.SectionAccess, &access); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(p.User), "global_permissions": perms, "preferences": preferences,
		"personal_space": personal, "needs_second_factor": p.NeedsSecondFactor, "sign_in": signIn, "idle_minutes": access.IdleMinutes, "drawing_origin": s.deps.Config.DrawingOrigin(),
		"office_preview": s.deps.Exchange.OfficeAvailable()})
}

// handleCapabilities says in one answer which optional parts are on (ARC-071), so a screen
// never offers what would fail. It says only whether a part is on, never where it runs.
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	down := map[string]bool{}
	if s.deps.HelpersDown != nil {
		found, err := s.deps.HelpersDown(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		down = found
	}
	experimental, err := s.deps.Features.Experimental(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// A helper that is set up but did not answer its last check is not offered either,
	// because the screen would offer what fails (ARC-071, ARC-072). An experimental part
	// that is off is not offered at all (spec 069).
	writeJSON(w, http.StatusOK, map[string]any{
		"pdf_export":       s.deps.Exchange.PDFAvailable() && !down["pdf"],
		"office_preview":   s.deps.Exchange.OfficeAvailable() && !down["office"],
		"drawing":          s.deps.Config.DrawingOrigin() != "",
		"collaboration":    s.deps.Config.CollabURL != "" && !down["collaboration"],
		"automation":       slices.Contains(experimental.On, features.ExperimentalAutomation),
		"databases":        slices.Contains(experimental.On, features.ExperimentalDatabases),
		"bundles":          slices.Contains(experimental.On, features.ExperimentalBundles),
		"console_restore":  slices.Contains(experimental.On, features.ExperimentalConsoleRestore),
		"publishing":       slices.Contains(experimental.On, features.ExperimentalPublishing),
		"guided_templates": slices.Contains(experimental.On, features.ExperimentalGuidedTemplates),
	})
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":        s.deps.Config.InstanceName,
		"instance_id": s.deps.InstanceID.String(),
		"version":     version.Version,
	})
}

// auditAdmin records a read-only admin action (viewing, exporting, verifying).
func (s *Server) auditAdmin(r *http.Request, action string, details map[string]any) error {
	p := PrincipalFrom(r.Context())
	m := metaFrom(r)
	return audit.RecordStandalone(r.Context(), s.deps.Pool, audit.Event{
		ActorType: audit.ActorUser, ActorID: p.User.ID.String(), Action: action, ObjectType: "audit_log",
		Result: audit.ResultSuccess, IP: m.IP, UserAgent: m.UserAgent, RequestID: m.RequestID, Details: details,
	})
}

func parseAuditFilter(r *http.Request) (audit.Filter, error) {
	q := r.URL.Query()
	f := audit.Filter{ActorID: q.Get("actor_id"), Action: q.Get("action"), ObjectType: q.Get("object_type"),
		ObjectID: q.Get("object_id"), SpaceID: q.Get("space_id"), Result: q.Get("result"), Limit: 100}
	var err error
	parseTime := func(key string) time.Time {
		v := q.Get(key)
		if v == "" || err != nil {
			return time.Time{}
		}
		t, perr := time.Parse(time.RFC3339, v)
		if perr != nil {
			err = fmt.Errorf("%s must be an RFC 3339 time like 2026-11-03T09:00:00Z", key)
		}
		return t
	}
	f.Since, f.Until = parseTime("since"), parseTime("until")
	if v := q.Get("before_seq"); v != "" && err == nil {
		f.BeforeSeq, err = strconv.ParseInt(v, 10, 64)
		if err != nil || f.BeforeSeq < 0 {
			err = errors.New("before_seq must be a positive number")
		}
	}
	if v := q.Get("limit"); v != "" && err == nil {
		f.Limit, err = strconv.Atoi(v)
		if err != nil || f.Limit < 1 || f.Limit > 500 {
			err = errors.New("limit must be between 1 and 500")
		}
	}
	return f, err
}

type auditEventJSON struct {
	// Seq is zero, and Sealed false, while the event waits for the sealer (ADR-041).
	Seq    int64 `json:"seq"`
	Sealed bool  `json:"sealed"`
	audit.Event
}

func (s *Server) handleAuditList(w http.ResponseWriter, r *http.Request) {
	f, err := parseAuditFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if f.Censor, err = s.deps.Pages.AuditCensor(r.Context(), s.caller(r)); err != nil {
		s.internalError(w, r, err)
		return
	}
	events, err := audit.List(r.Context(), s.deps.Pool, f)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.auditAdmin(r, "audit.viewed", map[string]any{"filter_actor_id": f.ActorID, "filter_action": f.Action}); err != nil {
		s.internalError(w, r, err)
		return
	}
	out := make([]auditEventJSON, 0, len(events))
	for _, e := range events {
		out = append(out, auditEventJSON{Seq: e.Seq, Sealed: e.Sealed, Event: e.Event})
	}
	// The next page starts below the last event of this one. A full page that ends on a
	// waiting event has no position yet; it is sealed within seconds, so asking again then
	// gives one.
	var next any
	if len(events) > 0 && len(events) == f.Limit {
		last := events[len(events)-1]
		if last.Sealed {
			next = last.Seq
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": out, "next_before_seq": next})
}

const maxExportRows = 1_000_000

func (s *Server) handleAuditExport(w http.ResponseWriter, r *http.Request) {
	f, err := parseAuditFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if f.Censor, err = s.deps.Pages.AuditCensor(r.Context(), s.caller(r)); err != nil {
		s.internalError(w, r, err)
		return
	}
	// Audit first: an export is recorded even if the download is interrupted.
	if err := s.auditAdmin(r, "audit.exported", map[string]any{"format": "csv", "filter_actor_id": f.ActorID, "filter_action": f.Action}); err != nil {
		s.internalError(w, r, err)
		return
	}
	name := "diffluence-audit-" + time.Now().UTC().Format("20060102-150405") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	// A long log takes longer to send than the server's write timeout. Each query of the
	// export reads one page, so the limit on one query of a read still holds.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(uploadDeadline))
	if _, err := audit.ExportCSV(r.Context(), s.deps.Pool, f, maxExportRows, w); err != nil {
		// Headers are already sent: the file ends with a line that says it stopped.
		s.deps.Log.Error("audit export interrupted", "error", err, "request_id", RequestIDFrom(r.Context()))
	}
}

// auditWalkLimit is how long the check from the console may walk the chain in its one
// query: long enough for millions of events, and still an end. The nightly check and
// `diffluence audit verify` have no limit. Only auditors ask, and the class of search and
// reports bounds how many walks run at once.
const auditWalkLimit = 10 * time.Minute

func (s *Server) handleAuditVerify(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(auditWalkLimit + time.Minute))
	res, err := s.deps.CheckAudit(db.WithQueryLimit(r.Context(), auditWalkLimit))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if err := s.auditAdmin(r, "audit.verified", map[string]any{"ok": res.OK, "checked": res.Checked, "checkpoints": res.Checkpoints}); err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
