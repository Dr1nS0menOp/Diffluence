// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"bytes"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/console"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/settings"
)

const maxLogoBytes = 1 << 20

func (s *Server) registerConsoleRoutes() {
	r := s.router
	admin := Permission(identity.PermAdministrator)
	sensitive := Sensitive(admin)

	// Public: the login page shows the logo before anyone is logged in. It is an
	// image an administrator chose to show.
	r.HandleFunc("GET /api/v1/appearance/logo", Public, s.handleLogo)

	r.HandleFunc("GET /api/v1/admin/summary", SessionOnly(admin), s.handleSummary)
	r.HandleFunc("GET /api/v1/admin/attention", SessionOnly(admin), s.handleAttention)
	r.HandleFunc("GET /api/v1/admin/invitations", SessionOnly(admin), s.handleInvitations)
	r.HandleFunc("POST /api/v1/admin/invitations", SessionOnly(admin), s.handleInvitationCreate)
	r.HandleFunc("DELETE /api/v1/admin/invitations/{id}", SessionOnly(admin), s.handleInvitationCancel)
	r.HandleFunc("POST /api/v1/admin/users/bulk", SessionOnly(admin), s.handleUsersBulk)
	r.HandleFunc("POST /api/v1/admin/users/import", SessionOnly(admin), s.handleUsersImport)
	r.HandleFunc("POST /api/v1/admin/users/{id}/anonymise", sensitive, s.handleAnonymise)
	r.HandleFunc("GET /api/v1/admin/users/{id}/type-impact", SessionOnly(admin), s.handleTypeImpact)
	r.HandleFunc("GET /api/v1/admin/users/{id}/reach", SessionOnly(admin), s.handleUserReach)
	r.HandleFunc("PUT /api/v1/admin/users/{id}/expiry", SessionOnly(admin), s.handleExpiry)
	r.HandleFunc("POST /api/v1/admin/service-accounts", SessionOnly(admin), s.handleServiceAccount)
	r.HandleFunc("POST /api/v1/admin/service-accounts/{id}/tokens", sensitive, s.handleServiceToken)
	r.HandleFunc("GET /api/v1/admin/global-permissions", SessionOnly(admin), s.handleGlobalHolders)
	r.HandleFunc("GET /api/v1/admin/groups/{id}/impact", SessionOnly(admin), s.handleGroupImpact)
	r.HandleFunc("PUT /api/v1/admin/groups/{id}/owners", SessionOnly(admin), s.handleGroupOwners)
	r.HandleFunc("POST /api/v1/admin/spaces/{id}/handover", SessionOnly(admin), s.handleSpaceHandover)
	r.HandleFunc("GET /api/v1/admin/pages/{id}", SessionOnly(admin), s.handlePageNote)
	r.HandleFunc("GET /api/v1/admin/permission-sets", SessionOnly(admin), s.handlePermissionSets)
	r.HandleFunc("PUT /api/v1/admin/permission-sets", SessionOnly(admin), s.handlePermissionSetSave)
	r.HandleFunc("DELETE /api/v1/admin/permission-sets/{id}", SessionOnly(admin), s.handlePermissionSetDelete)
	r.HandleFunc("GET /api/v1/categories", Authenticated(), s.handleCategories)
	r.HandleFunc("POST /api/v1/admin/categories", SessionOnly(admin), s.handleCategoryAdd)
	r.HandleFunc("DELETE /api/v1/admin/categories/{id}", SessionOnly(admin), s.handleCategoryDelete)
	r.HandleFunc("GET /api/v1/admin/settings/{section}", SessionOnly(admin), s.handleSectionGet)
	r.HandleFunc("PUT /api/v1/admin/settings/{section}", SessionOnly(admin), s.handleSectionSet)
	r.HandleFunc("PUT /api/v1/admin/appearance/logo", SessionOnly(admin), s.handleLogoSet)
	r.HandleFunc("DELETE /api/v1/admin/appearance/logo", SessionOnly(admin), s.handleLogoRemove)
	r.HandleFunc("POST /api/v1/admin/appearance/reset", SessionOnly(admin), s.handleLookReset)
	r.HandleFunc("POST /api/v1/admin/jobs/{id}/retry", SessionOnly(admin), s.handleJobRetry)
	r.HandleFunc("POST /api/v1/admin/jobs/{id}/cancel", SessionOnly(admin), s.handleJobCancel)
	r.HandleFunc("GET /api/v1/admin/jobs", SessionOnly(admin), s.handleJobs)
	r.HandleFunc("GET /api/v1/admin/event-failures", SessionOnly(admin), s.handleEventFailures)
	r.HandleFunc("POST /api/v1/admin/event-failures/{id}/replay", SessionOnly(admin), s.handleEventReplay)
	r.HandleFunc("GET /api/v1/admin/reports/largest", SessionOnly(admin), s.handleLargest)
	r.HandleFunc("GET /api/v1/admin/storage/unused", SessionOnly(admin), s.handleUnused)
	r.HandleFunc("POST /api/v1/admin/storage/unused", SessionOnly(admin), s.handleUnusedRemove)
	r.HandleFunc("PUT /api/v1/admin/maintenance", Sensitive(Permission(identity.PermSystemAdministrator)), s.handleMaintenance)
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := s.deps.Console.Summary(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handleAttention(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Console.Attention(r.Context(), s.deps.Identity.MailReady())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// Administrators get the strongest push: their account reaches everything.
	if s.deps.SSO == nil {
		list = append(list, console.Item{Kind: "no_identity_provider"})
	}
	// A proxy with TLS in front and a forgotten address leaves cookies weak without a sound (SEC-257).
	if s.deps.Config.PlainToOthers() {
		list = append(list, console.Item{Kind: "public_address_plain"})
	}
	me, err := s.deps.Identity.GetUser(r.Context(), s.deps.Pool, s.caller(r).UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if me.Passkeys == 0 {
		list = append(list, console.Item{Kind: "no_passkey"})
	}
	unmanaged, err := s.deps.Spaces.WithoutActiveAdministrator(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	orphaned, err := s.deps.Console.OrphanedPages(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "unmanaged_spaces": unmanaged, "orphaned_pages": orphaned})
}

func (s *Server) handleInvitations(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Identity.Invitations(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invitations": list})
}

func (s *Server) handleInvitationCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email  string      `json:"email"`
		Type   string      `json:"type"`
		Groups []uuid.UUID `json:"groups"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	features, err := s.features(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !features.Invitations {
		s.fail(w, r, apperr.Conflict("invitations_off", "Invitations are turned off."))
		return
	}
	link, err := s.deps.Identity.Invite(r.Context(), s.caller(r), identity.NewInvitation{Email: in.Email, Type: in.Type, Groups: in.Groups})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"link": link})
}

func (s *Server) handleInvitationCancel(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Identity.CancelInvitation(r.Context(), s.caller(r), r.PathValue("id")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUsersBulk(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action  string      `json:"action"`
		Users   []uuid.UUID `json:"users"`
		GroupID uuid.UUID   `json:"group_id"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	c := s.caller(r)
	done, failed := 0, 0
	for _, id := range in.Users {
		var err error
		switch in.Action {
		case "disable":
			err = s.deps.Identity.SetUserStatus(r.Context(), c, id, identity.StatusDisabled)
		case "add_to_group":
			err = s.deps.Identity.AddGroupMember(r.Context(), c, in.GroupID, id)
		default:
			s.fail(w, r, apperr.Invalid("Choose disable or add to group."))
			return
		}
		if err != nil {
			failed++
			continue
		}
		done++
	}
	writeJSON(w, http.StatusOK, map[string]any{"done": done, "failed": failed})
}

// handleUsersImport shows what a file would do with ?preview=1, and does it without.
func (s *Server) handleUsersImport(w http.ResponseWriter, r *http.Request) {
	rows, err := identity.ParseImport(http.MaxBytesReader(w, r.Body, maxSmallBody))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if r.URL.Query().Get("preview") != "" {
		rows, err = s.deps.Identity.PreviewImport(r.Context(), rows)
	} else {
		rows, err = s.deps.Identity.Import(r.Context(), s.caller(r), rows)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

func (s *Server) handleAnonymise(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	// In this order: a refused request must leave the profile alone.
	if err := s.deps.Identity.Anonymise(r.Context(), s.caller(r), id, in.Reason); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.deps.People.Forget(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTypeImpact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	impact, err := s.deps.Identity.ChangeImpact(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, impact)
}

func (s *Server) handleUserReach(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	reach, err := s.deps.Identity.UserReach(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reach)
}

func (s *Server) handleExpiry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Identity.SetExpiry(r.Context(), s.caller(r), id, in.ExpiresAt); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleServiceAccount(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	u, err := s.deps.Identity.CreateServiceAccount(r.Context(), s.caller(r), in.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) handleServiceToken(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in tokenBody
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	t, secret, err := s.deps.Identity.CreateServiceToken(r.Context(), s.caller(r), id, in.token())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": t, "secret": secret})
}

func (s *Server) handleGlobalHolders(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Permissions.GlobalHolders(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"holders": list})
}

func (s *Server) handleGroupImpact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	impact, err := s.deps.Permissions.GroupImpact(r.Context(), s.deps.Pool, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, impact)
}

func (s *Server) handleGroupOwners(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Owners []uuid.UUID `json:"owners"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Identity.SetGroupOwners(r.Context(), s.caller(r), id, in.Owners); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpaceHandover(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		To uuid.UUID `json:"to"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Spaces.Handover(r.Context(), s.caller(r), id, in.To); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePageNote answers what an administrator may know of a page they found in the
// audit log by its id (ADR-029).
func (s *Server) handlePageNote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	note, err := s.deps.Pages.Note(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, note)
}

func (s *Server) handlePermissionSets(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Permissions.PermissionSets(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sets": list})
}

func (s *Server) handlePermissionSetSave(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Permissions.SavePermissionSet(r.Context(), s.caller(r), in.Name, in.Permissions); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePermissionSetDelete(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error {
		return s.deps.Permissions.DeletePermissionSet(r.Context(), c, id)
	})
}

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Spaces.Categories(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": list})
}

func (s *Server) handleCategoryAdd(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	c, err := s.deps.Spaces.AddCategory(r.Context(), s.caller(r), in.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleCategoryDelete(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Spaces.DeleteCategory(r.Context(), c, id) })
}

func (s *Server) handleSectionGet(w http.ResponseWriter, r *http.Request) {
	v, err := s.deps.Settings.Section(r.Context(), r.PathValue("section"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleSectionSet(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(w, r, maxSmallBody)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v, err := s.deps.Settings.SetSection(r.Context(), s.caller(r), r.PathValue("section"), bytes.NewReader(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleLogo(w http.ResponseWriter, r *http.Request) {
	logo, f, err := s.deps.Settings.OpenLogo(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer func() { _ = f.Close() }()
	h := w.Header()
	h.Set("Content-Type", logo.ContentType)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "", time.Time{}, f)
}

func (s *Server) handleLogoSet(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Settings.SetLogo(r.Context(), s.caller(r), http.MaxBytesReader(w, r.Body, maxLogoBytes)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLogoRemove(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Settings.RemoveLogo(r.Context(), s.caller(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleLookReset(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Settings.ResetLook(r.Context(), s.caller(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Jobs.Recent(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": list})
}

func (s *Server) handleJobRetry(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Jobs.Retry(r.Context(), c, id) })
}

func (s *Server) handleEventFailures(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Failures.List(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"failures": list})
}

func (s *Server) handleEventReplay(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error {
		return s.deps.Failures.RequestReplay(r.Context(), c, id)
	})
}

func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Jobs.Cancel(r.Context(), c, id) })
}

func (s *Server) handleLargest(w http.ResponseWriter, r *http.Request) {
	report, err := s.deps.Console.Largest(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) handleUnused(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Console.UnusedFiles(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": list})
}

func (s *Server) handleUnusedRemove(w http.ResponseWriter, r *http.Request) {
	removed, err := s.deps.Console.RemoveUnusedFiles(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": removed})
}

func (s *Server) handleMaintenance(w http.ResponseWriter, r *http.Request) {
	var in settings.Maintenance
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Settings.SetMaintenance(r.Context(), s.caller(r), in); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}
