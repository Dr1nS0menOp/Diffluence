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
	// Public: a browser's WebSocket to the service. The proxy checks the session and the
	// right to edit itself, passes on only what comes from our own origin, and hands the
	// service a ticket in place of the cookie (spec 067).
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

// handleCollabAuthorize answers the service about the ticket of a connection. The service
// never holds a session: it has the ticket the API gave the socket (spec 067).
func (s *Server) handleCollabAuthorize(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PageID uuid.UUID `json:"page_id"`
		Ticket string    `json:"ticket"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	person, err := s.deps.Collab.Recheck(r.Context(), in.Ticket, in.PageID)
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
// The API decides who may connect: the service gets a ticket for one person and one
// page, and never the cookies of the browser (spec 067).
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
		// The origin stops a page on another site in a browser. A client that is not a
		// browser can send any origin; it still needs a session and the right to edit the
		// page, and the caps below bound what it can hold.
		if r.Header.Get("Origin") != origin {
			writeError(w, http.StatusForbidden, "cross_origin_blocked", "Cross-origin requests are not allowed.")
			return
		}
		p := PrincipalFrom(r.Context())
		if p == nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "Please log in.")
			return
		}
		pageID, err := uuid.Parse(r.URL.Query().Get("page"))
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "Not found.")
			return
		}
		ticket, err := s.deps.Collab.Ticket(r.Context(), *p, pageID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		// What proves the person to us stays here. A header of the same name from the
		// browser is replaced, never passed on.
		r.Header.Del("Cookie")
		r.Header.Del("Authorization")
		r.Header.Set(collab.TicketHeader, ticket)
		// A socket stays open as long as its editor, outside admission (spec 066), so it
		// has caps of its own: per node, and per login, by the id of the session.
		key := p.SessionID.String()
		if !s.sockets.acquire(key) {
			w.Header().Set("Retry-After", "5")
			writeError(w, http.StatusServiceUnavailable, "busy", busyMessage)
			return
		}
		defer s.sockets.release(key)
		// The service listens on its root, as it serves nothing else on this socket.
		r.URL.Path = "/"
		r.URL.RawQuery = ""
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
