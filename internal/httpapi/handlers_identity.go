// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/sso"
)

const oidcCookie = "diffluence_oidc"

func (s *Server) registerIdentityRoutes() {
	r := s.router
	// Public: the login page must know which methods exist before anyone is logged in.
	r.HandleFunc("GET /api/v1/auth/providers", Public, s.handleProviders)
	// Public: the second step of a login. Needs the token from a correct password; rate limited and audited.
	r.HandleFunc("POST /api/v1/auth/login/totp", Public, s.handleLoginTOTP)
	// Public: starting and finishing a login cannot need a session. Checked by state, PKCE and nonce.
	r.HandleFunc("GET /api/v1/auth/oidc/start", Public, s.handleOIDCStart)
	r.HandleFunc("GET /api/v1/auth/oidc/callback", Public, s.handleOIDCCallback)
	// Public: a passkey login is how a session is obtained. Verified by signature and audited.
	r.HandleFunc("POST /api/v1/auth/passkey/begin", Public, s.handlePasskeyLoginBegin)
	r.HandleFunc("POST /api/v1/auth/passkey/finish", Public, s.handlePasskeyLoginFinish)

	me := SessionOnly(Authenticated())
	r.HandleFunc("POST /api/v1/me/password", me, s.handlePasswordChange)
	r.HandleFunc("GET /api/v1/me/security", me, s.handleSecurity)
	r.HandleFunc("GET /api/v1/me/sessions", me, s.handleSessions)
	r.HandleFunc("DELETE /api/v1/me/sessions/{id}", me, s.handleSessionRevoke)
	r.HandleFunc("POST /api/v1/me/sessions/end-others", SessionOnly(me), s.handleEndOtherSessions)
	r.HandleFunc("POST /api/v1/me/totp/begin", me, s.handleTOTPBegin)
	r.HandleFunc("POST /api/v1/me/totp/confirm", me, s.handleTOTPConfirm)
	r.HandleFunc("POST /api/v1/me/totp/disable", Sensitive(me), s.handleTOTPDisable)
	r.HandleFunc("GET /api/v1/me/passkeys", me, s.handlePasskeys)
	r.HandleFunc("POST /api/v1/me/passkeys/begin", Sensitive(me), s.handlePasskeyBegin)
	r.HandleFunc("POST /api/v1/me/passkeys/finish", Sensitive(me), s.handlePasskeyFinish)
	r.HandleFunc("DELETE /api/v1/me/passkeys/{id}", Sensitive(me), s.handlePasskeyRemove)
	r.HandleFunc("GET /api/v1/directory", Permission(identity.PermViewProfiles), s.handleDirectory)

	admin := SessionOnly(Permission(identity.PermAdministrator))
	r.HandleFunc("GET /api/v1/admin/users", admin, s.handleUsers)
	r.HandleFunc("POST /api/v1/admin/users", admin, s.handleUserCreate)
	r.HandleFunc("GET /api/v1/admin/users/{id}", admin, s.handleUser)
	r.HandleFunc("PATCH /api/v1/admin/users/{id}", admin, s.handleUserUpdate)
	r.HandleFunc("POST /api/v1/admin/users/{id}/disable", admin, s.handleUserDisable)
	r.HandleFunc("POST /api/v1/admin/users/{id}/enable", admin, s.handleUserEnable)
	r.HandleFunc("POST /api/v1/admin/users/{id}/reset-password", Sensitive(admin), s.handleUserResetPassword)
	r.HandleFunc("POST /api/v1/admin/users/{id}/reset-mfa", Sensitive(admin), s.handleUserResetMFA)
	r.HandleFunc("POST /api/v1/admin/users/{id}/unlock-password", admin, s.handleUserUnlockPassword)
	r.HandleFunc("PUT /api/v1/admin/users/{id}/permissions", Sensitive(admin), s.handleUserPermissions)
	r.HandleFunc("GET /api/v1/admin/groups", admin, s.handleGroups)
	r.HandleFunc("POST /api/v1/admin/groups", admin, s.handleGroupCreate)
	r.HandleFunc("PATCH /api/v1/admin/groups/{id}", admin, s.handleGroupUpdate)
	r.HandleFunc("DELETE /api/v1/admin/groups/{id}", admin, s.handleGroupDelete)
	r.HandleFunc("GET /api/v1/admin/groups/{id}/members", admin, s.handleGroupMembers)
	r.HandleFunc("PUT /api/v1/admin/groups/{id}/members/{user}", admin, s.handleGroupMemberAdd)
	r.HandleFunc("DELETE /api/v1/admin/groups/{id}/members/{user}", admin, s.handleGroupMemberRemove)
	r.HandleFunc("PUT /api/v1/admin/groups/{id}/permissions", admin, s.handleGroupPermissions)
	r.HandleFunc("GET /api/v1/admin/scim-tokens", admin, s.handleScimTokens)
	r.HandleFunc("POST /api/v1/admin/scim-tokens", Sensitive(admin), s.handleScimTokenCreate)
	r.HandleFunc("POST /api/v1/admin/scim-tokens/{id}/rotate", Sensitive(admin), s.handleScimTokenRotate)
	r.HandleFunc("DELETE /api/v1/admin/scim-tokens/{id}", admin, s.handleScimTokenRevoke)
	r.HandleFunc("GET /api/v1/admin/system", admin, s.handleSystem)
	r.HandleFunc("GET /api/v1/admin/health", admin, s.handleHealth)

	if s.deps.SCIM == nil {
		return
	}
	sc := s.deps.SCIM
	base := "/scim/v2"
	withCaller := func(h func(identity.Caller, http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { h(callerFromSCIM(r), w, r) }
	}
	r.HandleFunc("GET "+base+"/ServiceProviderConfig", ScimToken(), sc.ServiceProviderConfig)
	r.HandleFunc("GET "+base+"/ResourceTypes", ScimToken(), sc.ResourceTypes)
	r.HandleFunc("GET "+base+"/Schemas", ScimToken(), sc.Schemas)
	r.HandleFunc("GET "+base+"/Users", ScimToken(), sc.ListUsers)
	r.HandleFunc("POST "+base+"/Users", ScimToken(), withCaller(sc.CreateUser))
	r.HandleFunc("GET "+base+"/Users/{id}", ScimToken(), sc.GetUser)
	r.HandleFunc("PUT "+base+"/Users/{id}", ScimToken(), withCaller(sc.ReplaceUser))
	r.HandleFunc("PATCH "+base+"/Users/{id}", ScimToken(), withCaller(sc.PatchUser))
	r.HandleFunc("DELETE "+base+"/Users/{id}", ScimToken(), withCaller(sc.DeleteUser))
	r.HandleFunc("GET "+base+"/Groups", ScimToken(), sc.ListGroups)
	r.HandleFunc("POST "+base+"/Groups", ScimToken(), withCaller(sc.CreateGroup))
	r.HandleFunc("GET "+base+"/Groups/{id}", ScimToken(), sc.GetGroup)
	r.HandleFunc("PUT "+base+"/Groups/{id}", ScimToken(), withCaller(sc.ReplaceGroup))
	r.HandleFunc("PATCH "+base+"/Groups/{id}", ScimToken(), withCaller(sc.PatchGroup))
	r.HandleFunc("DELETE "+base+"/Groups/{id}", ScimToken(), withCaller(sc.DeleteGroup))
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	// Secure follows the public address: on for https, off only where a browser would drop it, on plain http.
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
	http.SetCookie(w, &http.Cookie{Name: s.cookieName, Value: token, Path: "/", HttpOnly: true,
		Secure: s.deps.Config.SecureCookies(), SameSite: http.SameSiteStrictMode, MaxAge: int(s.deps.Config.SessionMaxAge.Seconds())})
}

func (s *Server) handleProviders(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{"password": true, "passkey": true}
	if s.deps.SSO != nil {
		out["oidc"] = map[string]any{"name": s.deps.SSO.DisplayName()}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleLoginTOTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"mfa_token"`
		Code  string `json:"code"`
	}
	if decodeJSON(w, r, 4<<10, &in) != nil {
		badBody(w)
		return
	}
	meta := metaFrom(r)
	if !s.loginByIP.allow("ip:" + meta.IP) {
		w.Header().Set("Retry-After", strconv.Itoa(int(loginWindow.Seconds())))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many login attempts. Try again later.")
		return
	}
	token, p, err := s.deps.Identity.CompleteMFA(r.Context(), in.Token, in.Code, meta)
	if errors.Is(err, identity.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "invalid_code", "That code is not correct.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.loginByIP.giveBack("ip:" + meta.IP)
	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(p.User)})
}

func (s *Server) handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	if s.deps.SSO == nil {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
		return
	}
	start := s.deps.SSO.Start
	if r.URL.Query().Get("prompt") == "login" {
		start = s.deps.SSO.StartFresh // step-up: the provider must ask again.
	}
	url, binding, err := start(r.Context())
	if err != nil {
		s.deps.Log.Error("oidc start failed", "error", err)
		http.Redirect(w, r, "/login?error=oidc_unavailable", http.StatusFound)
		return
	}
	// Lax on purpose: this cookie must come back on the redirect from the provider. Secure
	// follows the public address: on for https, off only on plain http.
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
	http.SetCookie(w, &http.Cookie{Name: s.flowCookie, Value: binding, Path: "/api/v1/auth/oidc", HttpOnly: true,
		Secure: s.deps.Config.SecureCookies(), SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if s.deps.SSO == nil {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
		return
	}
	binding := ""
	if c, err := r.Cookie(s.flowCookie); err == nil {
		binding = c.Value
	}
	// Clears the flow cookie; Secure follows the public address, as when it was set.
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
	http.SetCookie(w, &http.Cookie{Name: s.flowCookie, Value: "", Path: "/api/v1/auth/oidc", MaxAge: -1, HttpOnly: true, Secure: s.deps.Config.SecureCookies()})
	q := r.URL.Query()
	if code := q.Get("error"); code != "" {
		if err := s.deps.SSO.Refused(r.Context(), q.Get("state"), code, metaFrom(r)); err != nil {
			s.deps.Log.Error("oidc refusal could not be recorded", "error", err)
		}
		http.Redirect(w, r, "/login?error=oidc_failed", http.StatusFound)
		return
	}
	token, err := s.deps.SSO.Callback(r.Context(), q.Get("state"), q.Get("code"), binding, metaFrom(r))
	switch {
	case errors.Is(err, sso.ErrAccountExists):
		http.Redirect(w, r, "/login?error=oidc_account_exists", http.StatusFound)
	case errors.Is(err, sso.ErrNoAccount):
		http.Redirect(w, r, "/login?error=oidc_no_account", http.StatusFound)
	case errors.Is(err, sso.ErrFailed):
		http.Redirect(w, r, "/login?error=oidc_failed", http.StatusFound)
	case err != nil:
		s.deps.Log.Error("oidc callback failed", "error", err, "request_id", RequestIDFrom(r.Context()))
		http.Redirect(w, r, "/login?error=oidc_failed", http.StatusFound)
	default:
		s.setSessionCookie(w, token)
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func (s *Server) handlePasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	if !s.loginByIP.allow("ip:" + metaFrom(r).IP) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many login attempts. Try again later.")
		return
	}
	options, token, err := s.deps.Identity.BeginPasskeyLogin(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"options": options, "token": token})
}

type passkeyFinish struct {
	Token    string          `json:"token"`
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

func (s *Server) handlePasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	var in passkeyFinish
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	token, p, err := s.deps.Identity.FinishPasskeyLogin(r.Context(), in.Token, in.Response, metaFrom(r))
	if errors.Is(err, identity.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "This passkey could not be used.")
		return
	}
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.loginByIP.giveBack("ip:" + metaFrom(r).IP)
	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(p.User)})
}

func (s *Server) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if decodeJSON(w, r, 4<<10, &in) != nil {
		badBody(w)
		return
	}
	p := PrincipalFrom(r.Context())
	if !s.loginByAcc.allow("pw:" + p.User.ID.String()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Try again later.")
		return
	}
	if err := s.deps.Identity.ChangePassword(r.Context(), s.caller(r), p.SessionID, in.Current, in.New); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSecurity(w http.ResponseWriter, r *http.Request) {
	u, err := s.deps.Identity.GetUser(r.Context(), s.deps.Pool, PrincipalFrom(r.Context()).User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"has_password": u.HasPassword, "totp_enabled": u.TOTPEnabled, "passkeys": u.Passkeys, "source": u.Source})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	p := PrincipalFrom(r.Context())
	list, err := s.deps.Identity.Sessions(r.Context(), p.User.ID, p.SessionID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": list})
}

func (s *Server) handleSessionRevoke(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Identity.RevokeSession(r.Context(), c, id) })
}

type passwordBody struct {
	Password string `json:"password"`
}

func (s *Server) handleTOTPBegin(w http.ResponseWriter, r *http.Request) {
	var in passwordBody
	if decodeJSON(w, r, 4<<10, &in) != nil {
		badBody(w)
		return
	}
	p := PrincipalFrom(r.Context())
	if !s.loginByAcc.allow("pw:" + p.User.ID.String()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Try again later.")
		return
	}
	setup, err := s.deps.Identity.BeginTOTP(r.Context(), s.caller(r), p.User.Email, in.Password)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, setup)
}

func (s *Server) handleTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if decodeJSON(w, r, 4<<10, &in) != nil {
		badBody(w)
		return
	}
	codes, err := s.deps.Identity.ConfirmTOTP(r.Context(), s.caller(r), in.Code)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	var in passwordBody
	if decodeJSON(w, r, 4<<10, &in) != nil {
		badBody(w)
		return
	}
	// The same count as every other route that checks the password, so a stolen session cannot guess it here (SEC-113).
	if !s.loginByAcc.allow("pw:" + PrincipalFrom(r.Context()).User.ID.String()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Try again later.")
		return
	}
	if err := s.deps.Identity.DisableTOTP(r.Context(), s.caller(r), in.Password); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePasskeys(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Identity.Passkeys(r.Context(), PrincipalFrom(r.Context()).User.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"passkeys": list})
}

func (s *Server) handlePasskeyBegin(w http.ResponseWriter, r *http.Request) {
	options, token, err := s.deps.Identity.BeginPasskey(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"options": options, "token": token})
}

func (s *Server) handlePasskeyFinish(w http.ResponseWriter, r *http.Request) {
	var in passkeyFinish
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Identity.FinishPasskey(r.Context(), s.caller(r), in.Token, in.Name, in.Response); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePasskeyRemove(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Identity.RemovePasskey(r.Context(), c, id) })
}

func (s *Server) handleDirectory(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Identity.Directory(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": list})
}

func queryInt(r *http.Request, key string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(key))
	return n
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	group, err := optionalID(q.Get("group_id"))
	if err != nil {
		badBody(w)
		return
	}
	space, err := optionalID(q.Get("space_id"))
	if err != nil {
		badBody(w)
		return
	}
	role := q.Get("role")
	if role != "" && !slices.Contains(identity.FilterRoles, role) {
		badBody(w)
		return
	}
	list, total, err := s.deps.Identity.ListUsers(r.Context(), identity.UserFilter{Text: q.Get("q"), Type: q.Get("type"),
		Status: q.Get("status"), Source: q.Get("source"), GroupID: group, IdleDays: queryInt(r, "idle_days"),
		Role: role, SpaceID: space, Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset")})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": list, "total": total})
}

func (s *Server) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email             string   `json:"email"`
		DisplayName       string   `json:"display_name"`
		Type              string   `json:"type"`
		Password          string   `json:"password"`
		GlobalPermissions []string `json:"global_permissions"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	u, err := s.deps.Identity.CreateUserAdmin(r.Context(), s.caller(r), identity.NewUser{Email: in.Email, DisplayName: in.DisplayName,
		Type: in.Type, Password: in.Password, GlobalPermissions: in.GlobalPermissions})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	u, err := s.deps.Identity.GetUser(r.Context(), s.deps.Pool, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleUserUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		DisplayName string `json:"display_name"`
		Type        string `json:"type"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	u, err := s.deps.Identity.UpdateUserDetails(r.Context(), s.caller(r), id, identity.UpdateUser{DisplayName: in.DisplayName, Type: in.Type})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.deps.Spaces.FollowRename(r.Context(), id, u.DisplayName); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) handleUserDisable(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error {
		return s.deps.Identity.SetUserStatus(r.Context(), c, id, identity.StatusDisabled)
	})
}

func (s *Server) handleUserEnable(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error {
		return s.deps.Identity.SetUserStatus(r.Context(), c, id, identity.StatusActive)
	})
}

func (s *Server) handleUserResetPassword(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	// An administrator never chooses someone else's password: the person gets a
	// reset link, mailed when mail is on, or shown here once to hand over.
	sent, err := s.deps.Identity.SendResetLink(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sent)
}

func (s *Server) handleUserResetMFA(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Identity.ResetMFA(r.Context(), c, id) })
}

func (s *Server) handleUserUnlockPassword(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error {
		return s.deps.Identity.LiftPasswordLock(r.Context(), c, id)
	})
}

func (s *Server) setGlobalPermissions(w http.ResponseWriter, r *http.Request, subjectType string) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Permissions []string `json:"permissions"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Identity.SetGlobalPermissions(r.Context(), s.caller(r), subjectType, id, in.Permissions); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUserPermissions(w http.ResponseWriter, r *http.Request) {
	s.setGlobalPermissions(w, r, "user")
}

func (s *Server) handleGroupPermissions(w http.ResponseWriter, r *http.Request) {
	s.setGlobalPermissions(w, r, "group")
}

func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	list, total, err := s.deps.Identity.ListGroups(r.Context(), identity.GroupFilter{Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset")})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": list, "total": total})
}

type groupBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Server) handleGroupCreate(w http.ResponseWriter, r *http.Request) {
	var in groupBody
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	g, err := s.deps.Identity.CreateGroup(r.Context(), s.caller(r), identity.NewGroup{Name: in.Name, Description: in.Description})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) handleGroupUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in groupBody
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	g, err := s.deps.Identity.UpdateGroup(r.Context(), s.caller(r), id, identity.NewGroup{Name: in.Name, Description: in.Description})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleGroupDelete(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Identity.DeleteGroup(r.Context(), c, id) })
}

func (s *Server) handleGroupMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Identity.GroupMembers(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": list})
}

func (s *Server) groupMember(w http.ResponseWriter, r *http.Request, act func(identity.Caller, uuid.UUID, uuid.UUID) error) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	user, ok := pathID(w, r, "user")
	if !ok {
		return
	}
	if err := act(s.caller(r), id, user); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGroupMemberAdd(w http.ResponseWriter, r *http.Request) {
	s.groupMember(w, r, func(c identity.Caller, g, u uuid.UUID) error {
		return s.deps.Identity.AddGroupMember(r.Context(), c, g, u)
	})
}

func (s *Server) handleGroupMemberRemove(w http.ResponseWriter, r *http.Request) {
	s.groupMember(w, r, func(c identity.Caller, g, u uuid.UUID) error {
		return s.deps.Identity.RemoveGroupMember(r.Context(), c, g, u)
	})
}

func (s *Server) handleScimTokens(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.SCIM.Tokens(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": list})
}

func (s *Server) handleScimTokenCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name      string    `json:"name"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	t, secret, err := s.deps.SCIM.CreateToken(r.Context(), s.caller(r), in.Name, in.ExpiresAt)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": t, "secret": secret})
}

func (s *Server) handleScimTokenRotate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	t, secret, err := s.deps.SCIM.RotateToken(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": t, "secret": secret})
}

func (s *Server) handleEndOtherSessions(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Identity.EndOtherSessions(r.Context(), s.caller(r), PrincipalFrom(r.Context()).SessionID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleScimTokenRevoke(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.SCIM.RevokeToken(r.Context(), c, id) })
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	status, err := s.deps.SystemStatus(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleHealth answers the checks of the health page, which every page of the console
// also reads for its band about the disk (spec 051). The answer is at most 30 seconds old.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	checks, err := s.deps.HealthChecks(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"checks": checks})
}
