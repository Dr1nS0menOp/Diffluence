// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// registerRoleRoutes holds the routes for roles an administrator makes (spec 036).
// The permissions module checks every rule itself; the access rules here only stop
// the obvious cases early.
func (s *Server) registerRoleRoutes() {
	r := s.router
	admin := Permission(identity.PermAdministrator)
	r.HandleFunc("GET /api/v1/roles", Authenticated(), s.handleRoles)
	r.HandleFunc("POST /api/v1/admin/roles", SessionOnly(admin), s.handleRoleCreate)
	r.HandleFunc("PUT /api/v1/admin/roles/{id}", SessionOnly(admin), s.handleRoleUpdate)
	r.HandleFunc("POST /api/v1/admin/roles/{id}/preview", SessionOnly(admin), s.handleRolePreview)
	r.HandleFunc("DELETE /api/v1/admin/roles/{id}", SessionOnly(admin), s.handleRoleDelete)
	r.HandleFunc("GET /api/v1/admin/roles/{id}/holders", admin, s.handleRoleHolders)
	r.HandleFunc("GET /api/v1/admin/role-assignments", SessionOnly(admin), s.handleInstanceAssignments)
	r.HandleFunc("PUT /api/v1/admin/role-assignments", SessionOnly(admin), s.handleInstanceAssign)
	r.HandleFunc("GET /api/v1/spaces/{id}/role-assignments", Object(permissions.SpaceManageAccess), s.handleSpaceAssignments)
	r.HandleFunc("PUT /api/v1/spaces/{id}/role-assignments", Object(permissions.SpaceManageAccess), s.handleSpaceAssign)
	r.HandleFunc("POST /api/v1/spaces/{id}/role-assignments/preview", Object(permissions.SpaceManageAccess), s.handleSpaceAssignPreview)
}

func (s *Server) handleRoles(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = permissions.ScopeSpace
	}
	list, err := s.deps.Permissions.Roles(r.Context(), s.caller(r), scope)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": list, "permissions": rolePermissions(scope)})
}

func (s *Server) handleRoleHolders(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, total, err := s.deps.Permissions.RoleHolders(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"holders": list, "total": total})
}

func rolePermissions(scope string) []string {
	if scope == permissions.ScopeInstance {
		return identity.AllGlobalPermissions
	}
	return permissions.AllSpacePermissions
}

func (s *Server) handleRoleCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Scope string `json:"scope"`
		permissions.RoleInput
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	role, err := s.deps.Permissions.CreateRole(r.Context(), s.caller(r), in.Scope, in.RoleInput)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, role)
}

func (s *Server) handleRoleUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		permissions.RoleInput
		Confirm bool `json:"confirm"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	role, err := s.deps.Permissions.UpdateRole(r.Context(), s.caller(r), id, in.RoleInput, in.Confirm)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (s *Server) handleRolePreview(w http.ResponseWriter, r *http.Request) {
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
	impact, err := s.deps.Permissions.PreviewRole(r.Context(), s.caller(r), id, in.Permissions)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, impact)
}

func (s *Server) handleRoleDelete(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error {
		return s.deps.Permissions.DeleteRole(r.Context(), c, id)
	})
}

type assignmentBody struct {
	Subject permissions.Subject `json:"subject"`
	Roles   []uuid.UUID         `json:"roles"`
}

func (s *Server) handleInstanceAssignments(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Permissions.InstanceAssignments(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assignments": list})
}

func (s *Server) handleInstanceAssign(w http.ResponseWriter, r *http.Request) {
	var in assignmentBody
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Permissions.SetInstanceRoles(r.Context(), s.caller(r), in.Subject, in.Roles); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpaceAssignments(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Permissions.SpaceAssignments(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assignments": list})
}

func (s *Server) handleSpaceAssign(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in assignmentBody
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Permissions.SetSpaceRoles(r.Context(), s.caller(r), id, in.Subject, in.Roles); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpaceAssignPreview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in assignmentBody
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	preview, err := s.deps.Permissions.PreviewSpaceRoles(r.Context(), s.caller(r), id, in.Subject, in.Roles)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, preview)
}
