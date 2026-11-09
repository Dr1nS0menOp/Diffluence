// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/people"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/sharing"
	"github.com/diffluence/diffluence/internal/watches"
)

// forgotAnswerTime makes every answer on the forgot-password route take as long,
// so the time cannot tell whether an address has an account.
const forgotAnswerTime = 400 * time.Millisecond

func (s *Server) registerPeopleRoutes() {
	r := s.router
	me := Authenticated()

	// Public: someone who forgot a password has no session. The answer is the same
	// for every address, and both routes are rate limited.
	r.HandleFunc("POST /api/v1/auth/forgot", Public, s.handleForgot)
	r.HandleFunc("POST /api/v1/auth/reset", Public, s.handleReset)
	// Public: the token in the link is the proof, and it works once.
	r.HandleFunc("GET /api/v1/auth/invitation", Public, s.handleInvitationGet)
	r.HandleFunc("POST /api/v1/auth/invitation", Public, s.handleInvitationAccept)
	// Public: off by default, and the answer never says whether an address is known.
	r.HandleFunc("POST /api/v1/auth/signup", Public, s.handleSignup)
	r.HandleFunc("POST /api/v1/auth/signup/confirm", Public, s.handleSignupConfirm)
	r.HandleFunc("POST /api/v1/auth/reauth", SessionOnly(me), s.handleReauth)
	r.HandleFunc("POST /api/v1/auth/reauth/passkey/begin", SessionOnly(me), s.handleReauthPasskeyBegin)
	r.HandleFunc("POST /api/v1/auth/reauth/passkey/finish", SessionOnly(me), s.handleReauthPasskeyFinish)

	// The module decides who sees whom: a person they may not see is "not found".
	r.HandleFunc("GET /api/v1/people", me, s.handlePeople)
	r.HandleFunc("GET /api/v1/people/{id}", me, s.handlePerson)
	r.HandleFunc("GET /api/v1/people/{id}/picture", me, s.handlePersonPicture)
	r.HandleFunc("GET /api/v1/people/{id}/pages", me, s.handlePersonPages)
	r.HandleFunc("GET /api/v1/people/{id}/mentions", me, s.handlePersonMentions)
	r.HandleFunc("GET /api/v1/people/{id}/follows", me, s.handlePersonFollows)

	r.HandleFunc("PUT /api/v1/me/profile", SessionOnly(me), s.handleProfileSet)
	r.HandleFunc("PUT /api/v1/me/picture", SessionOnly(me), s.handlePictureSet)
	r.HandleFunc("DELETE /api/v1/me/picture", SessionOnly(me), s.handlePictureRemove)
	r.HandleFunc("GET /api/v1/me/logins", SessionOnly(me), s.handleLogins)
	r.HandleFunc("GET /api/v1/me/tokens", SessionOnly(me), s.handleTokens)
	r.HandleFunc("POST /api/v1/me/tokens", SessionOnly(me), s.handleTokenCreate)
	r.HandleFunc("DELETE /api/v1/me/tokens/{id}", SessionOnly(me), s.handleTokenRevoke)
	r.HandleFunc("POST /api/v1/me/export", SessionOnly(me), s.handleMyData)
	r.HandleFunc("POST /api/v1/me/removal-request", SessionOnly(me), s.handleRemovalRequest)
	r.HandleFunc("POST /api/v1/me/personal-space", Permission(identity.PermPersonalSpace), s.handlePersonalSpace)
	r.HandleFunc("POST /api/v1/me/sign-in-prompt/later", SessionOnly(me), s.handleSignInLater)

	r.HandleFunc("GET /api/v1/me/groups", me, s.handleMyGroups)
	r.HandleFunc("GET /api/v1/groups/{id}", Object("group.manage_members"), s.handleOwnedGroup)
	r.HandleFunc("PUT /api/v1/groups/{id}/members/{user}", Object("group.manage_members"), s.handleOwnedGroupAdd)
	r.HandleFunc("DELETE /api/v1/groups/{id}/members/{user}", Object("group.manage_members"), s.handleOwnedGroupRemove)

	r.HandleFunc("GET /api/v1/spaces/{id}/people", Object("space.view"), s.handleSpacePeople)
	r.HandleFunc("GET /api/v1/spaces/{id}/access", Object("space.manage_access"), s.handleSpaceAccess)
	r.HandleFunc("GET /api/v1/spaces/{id}/access.csv", Object("space.manage_access"), s.handleSpaceAccessCSV)
	r.HandleFunc("POST /api/v1/spaces/{id}/grants/preview", Object("space.manage_access"), s.handleGrantPreview)
	r.HandleFunc("POST /api/v1/spaces/{id}/guests", Object("space.manage_access"), s.handleInviteGuest)
	r.HandleFunc("GET /api/v1/spaces/{id}/audit", Object("space.audit"), s.handleSpaceAudit)
	r.HandleFunc("GET /api/v1/spaces/{id}/categories", Object("space.view"), s.handleSpaceCategoriesGet)
	r.HandleFunc("PUT /api/v1/spaces/{id}/categories", Object("space.administer"), s.handleSpaceCategories)
}

func (s *Server) features(r *http.Request) (settings.Features, error) {
	f := settings.DefaultFeatures
	return f, s.deps.Settings.ReadSection(r.Context(), settings.SectionFeatures, &f)
}

// limitAccountLinks stops anyone from trying links or addresses at speed.
func (s *Server) limitAccountLinks(w http.ResponseWriter, r *http.Request) bool {
	if s.loginByIP.allow("ip:" + metaFrom(r).IP) {
		return true
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(loginWindow.Seconds())))
	writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Try again later.")
	return false
}

func (s *Server) handleForgot(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if decodeJSON(w, r, 4<<10, &in) != nil {
		badBody(w)
		return
	}
	if !s.limitAccountLinks(w, r) {
		return
	}
	ready := time.Now().Add(forgotAnswerTime)
	if err := s.deps.Identity.RequestReset(r.Context(), in.Email, metaFrom(r)); err != nil {
		s.deps.Log.Error("password reset failed", "error", err, "request_id", RequestIDFrom(r.Context()))
	}
	time.Sleep(time.Until(ready))
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if decodeJSON(w, r, 4<<10, &in) != nil {
		badBody(w)
		return
	}
	if !s.limitAccountLinks(w, r) {
		return
	}
	if err := s.deps.Identity.ResetWithLink(r.Context(), in.Token, in.Password, metaFrom(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleInvitationGet(w http.ResponseWriter, r *http.Request) {
	if !s.limitAccountLinks(w, r) {
		return
	}
	email, err := s.deps.Identity.InvitationFor(r.Context(), r.URL.Query().Get("token"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": email})
}

type newAccountBody struct {
	Token       string `json:"token"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

func (b newAccountBody) account() identity.NewAccount {
	return identity.NewAccount{Token: b.Token, DisplayName: b.DisplayName, Password: b.Password}
}

func (s *Server) handleInvitationAccept(w http.ResponseWriter, r *http.Request) {
	var in newAccountBody
	if decodeJSON(w, r, 4<<10, &in) != nil {
		badBody(w)
		return
	}
	if !s.limitAccountLinks(w, r) {
		return
	}
	err := s.deps.Identity.AcceptInvitation(r.Context(), in.account(), metaFrom(r), func(tx pgx.Tx, a identity.Accepted) error {
		if a.SpaceID == nil {
			return nil
		}
		return permissions.GrantGuest(r.Context(), tx, *a.SpaceID, a.UserID)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if decodeJSON(w, r, 4<<10, &in) != nil {
		badBody(w)
		return
	}
	if !s.limitAccountLinks(w, r) {
		return
	}
	features, err := s.features(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !features.SelfSignup {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
		return
	}
	access := settings.DefaultAccess
	if err := s.deps.Settings.ReadSection(r.Context(), settings.SectionAccess, &access); err != nil {
		s.fail(w, r, err)
		return
	}
	ready := time.Now().Add(forgotAnswerTime)
	if err := s.deps.Identity.RequestSignup(r.Context(), in.Email, access.SignupDomains); err != nil {
		s.deps.Log.Error("sign-up failed", "error", err, "request_id", RequestIDFrom(r.Context()))
	}
	time.Sleep(time.Until(ready))
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleSignupConfirm(w http.ResponseWriter, r *http.Request) {
	var in newAccountBody
	if decodeJSON(w, r, 4<<10, &in) != nil {
		badBody(w)
		return
	}
	if !s.limitAccountLinks(w, r) {
		return
	}
	if err := s.deps.Identity.ConfirmSignup(r.Context(), in.account(), metaFrom(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReauth(w http.ResponseWriter, r *http.Request) {
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
	if err := s.deps.Identity.Reauthenticate(r.Context(), s.caller(r), p.SessionID, in.Password); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReauthPasskeyBegin(w http.ResponseWriter, r *http.Request) {
	options, token, err := s.deps.Identity.BeginPasskeyReauth(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"options": options, "token": token})
}

func (s *Server) handleReauthPasskeyFinish(w http.ResponseWriter, r *http.Request) {
	var in passkeyFinish
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	p := PrincipalFrom(r.Context())
	if !s.loginByAcc.allow("pw:" + p.User.ID.String()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts. Try again later.")
		return
	}
	err := s.deps.Identity.FinishPasskeyReauth(r.Context(), s.caller(r), p.SessionID, in.Token, in.Response)
	if errors.Is(err, identity.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "This passkey could not be used.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpaceCategoriesGet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Spaces.CategoriesOf(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": list})
}

func (s *Server) handleMyGroups(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Identity.GroupsOf(r.Context(), s.caller(r).UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": list})
}

func (s *Server) handlePeople(w http.ResponseWriter, r *http.Request) {
	group, err := optionalID(r.URL.Query().Get("group_id"))
	if err != nil {
		badBody(w)
		return
	}
	list, total, err := s.deps.People.List(r.Context(), s.caller(r), people.Query{Text: r.URL.Query().Get("q"),
		GroupID: group, Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset")})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": list, "total": total})
}

func (s *Server) handlePerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	p, err := s.deps.People.Get(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handlePersonPages(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	// Who may see a person decides first: not found stays not found here as well.
	if _, err := s.deps.People.Get(r.Context(), s.caller(r), id); err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.deps.Pages.ByPerson(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": list})
}

func (s *Server) handlePersonMentions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if _, err := s.deps.People.Get(r.Context(), s.caller(r), id); err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.deps.Pages.MentionsOf(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": list})
}

// handlePersonFollows answers with empty lists when the person hid them, so a screen
// has nothing to treat apart. They still see their own.
func (s *Server) handlePersonFollows(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	profile, err := s.deps.People.Get(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	choices, err := s.deps.Identity.Preferences(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if choices.HideFollows && !profile.IsSelf {
		writeJSON(w, http.StatusOK, watches.Follows{Followers: []watches.FollowEntry{}, Following: []watches.FollowEntry{}})
		return
	}
	follows, err := s.deps.Watches.FollowsOf(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, follows)
}

func (s *Server) handlePersonPicture(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	f, changed, err := s.deps.People.OpenPicture(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer func() { _ = f.Close() }()
	h := w.Header()
	h.Set("Content-Type", "image/png")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "private, no-cache")
	http.ServeContent(w, r, "", changed, f)
}

func (s *Server) handleProfileSet(w http.ResponseWriter, r *http.Request) {
	var in people.Input
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.People.SetOwn(r.Context(), s.caller(r), in); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePictureSet(w http.ResponseWriter, r *http.Request) {
	crop := people.Crop{X: queryInt(r, "x"), Y: queryInt(r, "y"), Size: queryInt(r, "size")}
	if err := s.deps.People.SetPicture(r.Context(), s.caller(r), r.Body, crop); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePictureRemove(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.People.RemovePicture(r.Context(), s.caller(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLogins(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Identity.LoginHistory(r.Context(), s.caller(r).UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logins": list})
}

func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Identity.Tokens(r.Context(), s.caller(r).UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": list})
}

type tokenBody struct {
	Name      string    `json:"name"`
	CanWrite  bool      `json:"can_write"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (b tokenBody) token() identity.NewToken {
	return identity.NewToken{Name: b.Name, CanWrite: b.CanWrite, Scopes: b.Scopes, ExpiresAt: b.ExpiresAt}
}

func (s *Server) handleTokenCreate(w http.ResponseWriter, r *http.Request) {
	var in tokenBody
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	features, err := s.features(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !features.Tokens {
		s.fail(w, r, apperr.Conflict("tokens_off", "An administrator turned tokens off."))
		return
	}
	t, secret, err := s.deps.Identity.CreateToken(r.Context(), s.caller(r), in.token())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": t, "secret": secret})
}

func (s *Server) handleTokenRevoke(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Identity.RevokeToken(r.Context(), c, id) })
}

func (s *Server) handleMyData(w http.ResponseWriter, r *http.Request) {
	c := s.caller(r)
	data, err := s.deps.Identity.ExportPersonalData(r.Context(), c.UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pages, err := s.deps.Pages.OwnTitles(r.Context(), c)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Disposition", attachmentHeader("diffluence-my-data.json"))
	writeJSON(w, http.StatusOK, map[string]any{"personal_data": data, "pages": pages})
}

func (s *Server) handleRemovalRequest(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Identity.RequestRemoval(r.Context(), s.caller(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSignInLater(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Identity.PostponeSignInPrompt(r.Context(), s.caller(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePersonalSpace(w http.ResponseWriter, r *http.Request) {
	features, err := s.features(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !features.PersonalSpaces {
		s.fail(w, r, apperr.Conflict("personal_spaces_off", "An administrator turned personal spaces off."))
		return
	}
	sp, err := s.deps.Spaces.CreatePersonal(r.Context(), s.caller(r), PrincipalFrom(r.Context()).User.DisplayName)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, sp)
}

func (s *Server) handleOwnedGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	g, members, err := s.deps.Identity.OwnedGroup(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"group": g, "members": members})
}

func (s *Server) handleOwnedGroupAdd(w http.ResponseWriter, r *http.Request) {
	s.groupMember(w, r, func(c identity.Caller, g, u uuid.UUID) error {
		return s.deps.Identity.OwnerAddMember(r.Context(), c, g, u)
	})
}

func (s *Server) handleOwnedGroupRemove(w http.ResponseWriter, r *http.Request) {
	s.groupMember(w, r, func(c identity.Caller, g, u uuid.UUID) error {
		return s.deps.Identity.OwnerRemoveMember(r.Context(), c, g, u)
	})
}

func (s *Server) handleSpacePeople(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Permissions.Holders(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": list})
}

func (s *Server) handleSpaceAccess(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Permissions.AccessDetail(r.Context(), s.caller(r), id, r.URL.Query().Get("q"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": list})
}

func (s *Server) handleSpaceAccessCSV(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	// A refusal must stay a normal JSON error, so the headers wait for the first byte.
	z := &csvResponse{ResponseWriter: w}
	if err := s.deps.Permissions.WriteAccessCSV(r.Context(), s.caller(r), id, z); err != nil {
		if z.started {
			s.deps.Log.Error("access export interrupted", "error", err, "request_id", RequestIDFrom(r.Context()))
			return
		}
		s.fail(w, r, err)
	}
}

type csvResponse struct {
	http.ResponseWriter
	started bool
}

func (c *csvResponse) Write(b []byte) (int, error) {
	if !c.started {
		c.started = true
		c.Header().Set("Content-Type", "text/csv; charset=utf-8")
		c.Header().Set("Content-Disposition", attachmentHeader("diffluence-access.csv"))
	}
	return c.ResponseWriter.Write(b)
}

func (s *Server) handleGrantPreview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Subject     permissions.Subject `json:"subject"`
		Permissions []string            `json:"permissions"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	p, err := s.deps.Permissions.PreviewGrant(r.Context(), s.caller(r), id, in.Subject, in.Permissions)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleInviteGuest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Email string `json:"email"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	c := s.caller(r)
	if err := s.deps.Permissions.RequireSpace(r.Context(), s.deps.Pool, c.UserID, id, permissions.SpaceManageAccess); err != nil {
		s.fail(w, r, err)
		return
	}
	access := settings.DefaultAccess
	if err := s.deps.Settings.ReadSection(r.Context(), settings.SectionAccess, &access); err != nil {
		s.fail(w, r, err)
		return
	}
	if !access.GuestInvites {
		s.fail(w, r, apperr.Conflict("guest_invites_off", "An administrator turned guest invitations off."))
		return
	}
	administrator, err := s.deps.Identity.HasGlobalPermission(r.Context(), c.UserID, identity.PermAdministrator)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// Only an administrator lets someone in. Anyone else asks one (ADR-033).
	if !administrator {
		request, err := s.deps.Sharing.AskGuestInvitation(r.Context(), c, id, sharing.InvitationAsk{Email: in.Email})
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"id": request, "waiting": true})
		return
	}
	link, err := s.deps.Identity.Invite(r.Context(), c, identity.NewInvitation{Email: in.Email, Type: identity.TypeGuest, SpaceID: &id})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"link": link})
}

func (s *Server) handleSpaceCategories(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Categories []uuid.UUID `json:"categories"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Spaces.SetCategories(r.Context(), s.caller(r), id, in.Categories); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpaceAudit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.SpaceAudit(r.Context(), s.caller(r), id, int64(queryInt(r, "before_seq")))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": list})
}

// readBody reads a body with a limit, for handlers that hand it on as it is.
func readBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, apperr.TooLarge("This is too large.")
		}
		return nil, apperr.Invalid("The request body is not valid.")
	}
	if !json.Valid(body) {
		return nil, apperr.Invalid("The request body is not valid.")
	}
	return body, nil
}
