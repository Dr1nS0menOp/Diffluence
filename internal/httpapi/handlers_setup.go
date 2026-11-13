// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/setup"
)

// setupWindow and setupTriesPerAddress limit how often one address may try a code,
// on top of the limit per code (setup.MaxWrongTries).
const (
	setupWindow          = 15 * time.Minute
	setupTriesPerAddress = 10
)

func (s *Server) registerSetupRoutes() {
	// Public: the first setup happens before any account exists. Both routes answer as
	// if they did not exist once an administrator exists, need the one-time code from the
	// server's log to change anything, are rate limited per address and audited (spec 038).
	s.router.HandleFunc("GET /api/v1/setup", Public, s.handleSetupState)
	s.router.HandleFunc("POST /api/v1/setup", Public, s.handleSetup)
}

// setupOpen reports whether the setup screen may be shown, and answers 404 otherwise.
func (s *Server) setupOpen(w http.ResponseWriter, r *http.Request) bool {
	open, err := s.deps.Setup.Open(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return false
	}
	if !open {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
	}
	return open
}

func (s *Server) handleSetupState(w http.ResponseWriter, r *http.Request) {
	if !s.setupOpen(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"open": true})
}

type setupInput struct {
	Code         string `json:"code"`
	Organisation string `json:"organisation"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	Language     string `json:"language"`
	TimeZone     string `json:"time_zone"`
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if !s.setupOpen(w, r) {
		return
	}
	var in setupInput
	if err := decodeJSON(w, r, 16<<10, &in); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Send JSON with the setup code and the first administrator.")
		return
	}
	meta := metaFrom(r)
	if !s.setupTries.allow(addressKey(meta.IP)) {
		w.Header().Set("Retry-After", strconv.Itoa(int(setupWindow.Seconds())))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many tries. Try again later.")
		return
	}
	from := setup.Attempt{IP: meta.IP, UserAgent: meta.UserAgent, RequestID: meta.RequestID}
	// The code comes first, so a stranger learns nothing about the other fields.
	if err := s.deps.Setup.Check(r.Context(), in.Code, from); err != nil {
		s.setupRefused(w, r, err)
		return
	}
	if err := settings.CheckFirstSetup(in.Organisation, in.Language, in.TimeZone); err != nil {
		s.fail(w, r, err)
		return
	}
	user := identity.NewUser{Email: in.Email, DisplayName: in.Name, Type: identity.TypeMember,
		Password: in.Password, GlobalPermissions: identity.AllGlobalPermissions}
	actor := identity.Actor{Type: audit.ActorAnonymous}
	var created identity.User
	err := s.deps.Setup.Complete(r.Context(), in.Code, from, func(tx pgx.Tx) error {
		var err error
		created, err = s.deps.Identity.CreateLocalUserIn(r.Context(), tx, user, actor, meta)
		return err
	})
	if errors.Is(err, setup.ErrClosed) || errors.Is(err, setup.ErrRefused) {
		s.setupRefused(w, r, err)
		return
	}
	if err != nil {
		s.fail(w, r, identity.Explain(err))
		return
	}
	s.setupTries.reset(addressKey(meta.IP))
	caller := identity.SystemCaller("setup")
	if err := s.deps.Settings.ApplyFirstSetup(r.Context(), caller, in.Organisation, in.Language, in.TimeZone); err != nil {
		// The administrator exists; the name and defaults can be set under Look and Settings.
		s.deps.Log.Error("first setup: storing the organisation and defaults failed", "error", err)
	}
	s.signInAfterSetup(r.Context(), w, created, in.Password, meta)
}

// signInAfterSetup opens a session for the new administrator, so the next screen can
// ask for a passkey at once. If that fails, the person signs in on the login page.
func (s *Server) signInAfterSetup(ctx context.Context, w http.ResponseWriter, u identity.User, password string, meta identity.RequestMeta) {
	token, _, err := s.deps.Identity.Login(ctx, u.Email, password, meta)
	if err != nil {
		writeJSON(w, http.StatusCreated, map[string]any{"user": toUserJSON(u), "signed_in": false})
		return
	}
	s.setSessionCookie(w, token)
	writeJSON(w, http.StatusCreated, map[string]any{"user": toUserJSON(u), "signed_in": true})
}

func (s *Server) setupRefused(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, setup.ErrClosed):
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
	case errors.Is(err, setup.ErrRefused):
		writeError(w, http.StatusForbidden, "setup_code_refused",
			"That setup code does not work. Run `diffluence setup-code` on the server for a new one.")
	default:
		s.internalError(w, r, err)
	}
}
