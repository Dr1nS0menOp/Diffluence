// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/sso"
)

// Electronic signatures, wave DC4 of spec 064: the signing step, its confirmation, the
// signatures of a version, the meanings of a space, the right to sign, the control history
// and the auditor's export.
func (s *Server) registerSigningRoutes() {
	r := s.router
	// A signature is a person's act in a session of their own: no token reaches the steps
	// that confirm one (DOC-733). The module refuses tokens too, at the signature itself.
	person := SessionOnly(Authenticated())
	// Found by the page id; the module checks what the step itself checks (DOC-760).
	r.HandleFunc("POST /api/v1/pages/{id}/signing", person, s.handleSigningBegin)
	r.HandleFunc("POST /api/v1/signing/confirm", person, s.handleSigningConfirm)
	r.HandleFunc("POST /api/v1/signing/passkey", person, s.handleSigningPasskey)
	// Under the path of the flow cookie, which must come back from the provider.
	r.HandleFunc("GET /api/v1/auth/oidc/sign", person, s.handleSigningAtProvider)
	r.HandleFunc("GET /api/v1/pages/{id}/versions/{number}/signatures", Object("page.view"), s.handleSignatures)
	// The module asks space.audit of the page's space.
	r.HandleFunc("GET /api/v1/pages/{id}/control-history", Authenticated(), s.handleControlHistory)
	r.HandleFunc("GET /api/v1/spaces/{id}/signature-meanings", Object(permissions.SpaceView), s.handleMeanings)
	r.HandleFunc("POST /api/v1/spaces/{id}/signature-meanings", Object(permissions.SpaceWorkflows), s.handleMeaningAdd)
	r.HandleFunc("GET /api/v1/spaces/{id}/auditor-export", Object(permissions.SpaceAudit), s.handleAuditorExport)
	admin := SessionOnly(Permission(identity.PermAdministrator))
	r.HandleFunc("GET /api/v1/admin/users/{id}/signing", admin, s.handleSigningRight)
	r.HandleFunc("PUT /api/v1/admin/users/{id}/signing", Sensitive(admin), s.handleSigningEnable)
	r.HandleFunc("DELETE /api/v1/admin/users/{id}/signing", admin, s.handleSigningDisable)
}

// signatureFrom is the signature a step carries: the session comes from the login, never
// from the request, so a token, which has no session, can never sign (DOC-733).
func (s *Server) signatureFrom(r *http.Request, confirmation string) pages.SignatureInput {
	p := PrincipalFrom(r.Context())
	in := pages.SignatureInput{Confirmation: confirmation}
	if p != nil && !p.ViaToken {
		in.SessionID = p.SessionID
	}
	return in
}

func (s *Server) handleSigningBegin(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Transition string `json:"transition"`
		Version    int    `json:"version"`
	}
	if !body(w, r, &in) {
		return
	}
	prompt, err := s.deps.Pages.BeginSignature(r.Context(), s.caller(r), id, pages.SignatureStep{Transition: in.Transition,
		Version: in.Version, SessionID: s.signatureFrom(r, "").SessionID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if s.deps.SSO == nil {
		prompt.Start.Methods = withoutMethod(prompt.Start.Methods, identity.SigningByProvider)
	}
	writeJSON(w, http.StatusOK, prompt)
}

func withoutMethod(methods []string, drop string) []string {
	out := make([]string, 0, len(methods))
	for _, m := range methods {
		if m != drop {
			out = append(out, m)
		}
	}
	return out
}

// allowSigningTry limits confirmations per account as the password of a login is limited, so
// guessing a password through the signing dialog is no faster than at the login (DOC-732).
func (s *Server) allowSigningTry(w http.ResponseWriter, r *http.Request) bool {
	p := PrincipalFrom(r.Context())
	if !s.loginByAcc.allow("pw:" + p.User.ID.String()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Try again later.")
		return false
	}
	return true
}

func (s *Server) handleSigningConfirm(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !body(w, r, &in) || !s.allowSigningTry(w, r) {
		return
	}
	p := PrincipalFrom(r.Context())
	if err := s.deps.Identity.ConfirmSigningWithPassword(r.Context(), s.caller(r), p.SessionID, in.Token, in.Password, in.Code); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSigningPasskey(w http.ResponseWriter, r *http.Request) {
	var in passkeyFinish
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if !s.allowSigningTry(w, r) {
		return
	}
	p := PrincipalFrom(r.Context())
	if err := s.deps.Identity.ConfirmSigningWithPasskey(r.Context(), s.caller(r), p.SessionID, in.Token, in.Response); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSigningAtProvider sends the browser to the identity provider for a fresh login that
// confirms a signing step (DOC-731).
func (s *Server) handleSigningAtProvider(w http.ResponseWriter, r *http.Request) {
	if s.deps.SSO == nil {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
		return
	}
	token := r.URL.Query().Get("token")
	p := PrincipalFrom(r.Context())
	step, err := s.deps.Identity.BeginSigningAtProvider(r.Context(), s.caller(r), p.SessionID, token)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	to, binding, err := s.deps.SSO.StartSigning(r.Context(), token, step.BeganAt)
	if err != nil {
		s.deps.Log.Error("oidc start for signing failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "oidc_unavailable", "The identity provider cannot be reached. Try again shortly.")
		return
	}
	// As for a login: Lax, so it comes back on the redirect from the provider.
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
	http.SetCookie(w, &http.Cookie{Name: s.flowCookie, Value: binding, Path: "/api/v1/auth/oidc", HttpOnly: true,
		Secure: s.deps.Config.SecureCookies(), SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, to, http.StatusFound)
}

// finishProviderSigning sends the browser back to the page of the signing step, which says
// whether the login confirmed it. The page comes from the step itself, never the request.
func (s *Server) finishProviderSigning(w http.ResponseWriter, r *http.Request, back string, err error) {
	if back == "" || !strings.HasPrefix(back, "/pages/") {
		back = "/"
	}
	outcome := "confirmed"
	if err != nil {
		outcome = "failed"
		if !isExpectedSigningFailure(err) {
			s.deps.Log.Error("oidc signing failed", "error", err, "request_id", RequestIDFrom(r.Context()))
		}
	}
	http.Redirect(w, r, back+"?"+url.Values{"signing": {outcome}}.Encode(), http.StatusFound)
}

// isExpectedSigningFailure says the provider's login was refused for a reason that was
// recorded already, which needs no line in the log.
func isExpectedSigningFailure(err error) bool {
	_, refused := apperr.As(err)
	return refused || errors.Is(err, sso.ErrFailed)
}

func (s *Server) handleSignatures(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || number < 1 {
		writeError(w, http.StatusBadRequest, "invalid", "A version is a number from 1.")
		return
	}
	list, err := s.deps.Pages.VerifySignatures(r.Context(), s.caller(r), id, number)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"signatures": list})
}

func (s *Server) handleControlHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	h, err := s.deps.Pages.ControlHistory(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) handleMeanings(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.SignatureMeanings(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"meanings": list})
}

func (s *Server) handleMeaningAdd(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !body(w, r, &in) {
		return
	}
	m, err := s.deps.Pages.AddSignatureMeaning(r.Context(), s.caller(r), id, pages.MeaningInput{Name: in.Name, Description: in.Description})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) handleAuditorExport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	q := r.URL.Query()
	in := pages.AuditorExportInput{Branch: q.Get("branch") == "true"}
	if v := q.Get("page"); v != "" {
		page, err := uuid.Parse(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid", "The page is not an id.")
			return
		}
		in.PageID = &page
	}
	for name, at := range map[string]*time.Time{"since": &in.Since, "until": &in.Until} {
		if v := q.Get(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid", "A time is written as 2026-01-31T00:00:00Z.")
				return
			}
			*at = t
		}
	}
	export, err := s.deps.Pages.PrepareAuditorExport(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="auditor-export.jsonl"`)
	if err := export.Write(r.Context(), w); err != nil {
		// The header is gone, so the file ends short; the last line of a whole export says
		// how many lines it has, which tells a reader it is incomplete.
		s.deps.Log.Error("auditor export failed", "error", err, "request_id", RequestIDFrom(r.Context()))
	}
}

func (s *Server) handleSigningRight(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	right, err := s.deps.Identity.SigningRightOf(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"right": right, "methods": identity.IdentificationMethods})
}

func (s *Server) handleSigningEnable(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		IdentifiedBy string `json:"identified_by"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Identity.EnableSigning(r.Context(), s.caller(r), id, in.IdentifiedBy); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSigningDisable(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.deps.Identity.DisableSigning(r.Context(), s.caller(r), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
