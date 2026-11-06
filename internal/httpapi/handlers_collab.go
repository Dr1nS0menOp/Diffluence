// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/collab"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

const maxCollabBody = 32 << 20

func (s *Server) registerCollabRoutes() {
	r := s.router
	view := Object(permissions.ActionPageView)
	r.HandleFunc("GET /api/v1/pages/{id}/collab", view, s.handleCollabInfo)
	r.HandleFunc("GET /api/v1/pages/{id}/version-authors", view, s.handleVersionAuthors)
	// Public: these are for the collab service, which has no session. The service
	// token is the permission, checked in constant time, and without the service
	// installed every call is a 404.
	r.HandleFunc("POST /collab/authorize", Public, s.collabOnly(s.handleCollabAuthorize))
	r.HandleFunc("GET /collab/documents/{id}", Public, s.collabOnly(s.handleCollabLoad))
	r.HandleFunc("PUT /collab/documents/{id}", Public, s.collabOnly(s.handleCollabStore))
	// Public: a browser's WebSocket to the service. The service asks /collab/authorize
	// with the cookie of the connection before it lets anyone in, and this route only
	// passes on what comes from our own origin.
	r.Handle("GET /collab", Public, s.collabProxy())
}

// collabOnly requires the service token.
func (s *Server) collabOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !found || !s.deps.Collab.TrustsToken(token) {
			writeError(w, http.StatusNotFound, "not_found", "Not found.")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleCollabInfo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	info, err := s.deps.Collab.For(r.Context(), *PrincipalFrom(r.Context()), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleVersionAuthors(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	authors, err := s.deps.Collab.Authors(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authors": authors})
}

// sessionFromHeader reads our session out of the cookies of a connection. An answer
// without a valid session is a plain no.
func (s *Server) sessionFromHeader(r *http.Request, header string) (identity.Principal, error) {
	cookies, err := http.ParseCookie(header)
	if err != nil {
		return identity.Principal{}, identity.ErrNoSession
	}
	for _, c := range cookies {
		if c.Name == s.cookieName {
			return s.deps.Identity.AuthenticateSession(r.Context(), c.Value)
		}
	}
	return identity.Principal{}, identity.ErrNoSession
}

func (s *Server) handleCollabAuthorize(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PageID uuid.UUID `json:"page_id"`
		Cookie string    `json:"cookie"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	p, err := s.sessionFromHeader(r, in.Cookie)
	if errors.Is(err, identity.ErrNoSession) {
		writeJSON(w, http.StatusOK, collab.Person{})
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	person, err := s.deps.Collab.Authorize(r.Context(), p, in.PageID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, person)
}

func (s *Server) handleCollabLoad(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	state, err := s.deps.Collab.State(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if state == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	if _, err := w.Write(state); err != nil {
		s.deps.Log.Error("sending a shared document failed", "error", err, "request_id", RequestIDFrom(r.Context()))
	}
}

func (s *Server) handleCollabStore(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in collab.Stored
	if decodeJSON(w, r, maxCollabBody, &in) != nil {
		badBody(w)
		return
	}
	err := s.deps.Collab.Store(r.Context(), id, in)
	if errors.Is(err, collab.ErrRefused) {
		writeError(w, http.StatusUnprocessableEntity, "document_refused", "This document did not pass the gate.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// collabProxy passes the WebSocket of a browser on to the service (ARC-118). A page
// on another site could open this socket with our cookie, so the origin must be ours.
func (s *Server) collabProxy() http.Handler {
	missing := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
	})
	if !s.deps.Collab.Installed() {
		return missing
	}
	target, err := url.Parse(s.deps.Collab.URL())
	if err != nil {
		s.deps.Log.Error("the address of the collab service is not valid", "error", err)
		return missing
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	origin := s.deps.Config.PublicOrigin()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != origin {
			writeError(w, http.StatusForbidden, "cross_origin_blocked", "Cross-origin requests are not allowed.")
			return
		}
		// The service listens on its root, as it serves nothing else on this socket.
		r.URL.Path = "/"
		proxy.ServeHTTP(w, r)
	})
}

// publishedTogether names who took part in a version.
func (s *Server) publishedTogether(w http.ResponseWriter, r *http.Request, id uuid.UUID, version int) bool {
	if !s.deps.Collab.Installed() {
		return true
	}
	if err := s.deps.Collab.Published(r.Context(), s.caller(r), id, version); err != nil {
		s.fail(w, r, err)
		return false
	}
	return true
}
