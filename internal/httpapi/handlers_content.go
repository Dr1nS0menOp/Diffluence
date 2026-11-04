// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/search"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/spaces"
)

// Bodies are small, except documents, which are limited by the document format.
const (
	maxSmallBody    = 64 << 10
	maxDocumentBody = 6 << 20
	uploadDeadline  = 2 * time.Hour
)

func (s *Server) registerContentRoutes() {
	r := s.router
	spacesPath, pagesPath := "/api/v1/spaces", "/api/v1/pages"

	r.HandleFunc("GET "+spacesPath, Object("space.view"), s.handleSpaceList)
	r.HandleFunc("POST "+spacesPath, Permission(identity.PermCreateSpace), s.handleSpaceCreate)
	r.HandleFunc("GET "+spacesPath+"/{id}", Object("space.view"), s.handleSpaceGet)
	r.HandleFunc("GET /api/v1/space-keys/{key}", Object("space.view"), s.handleSpaceByKey)
	r.HandleFunc("PATCH "+spacesPath+"/{id}", Object("space.administer"), s.handleSpaceUpdate)
	r.HandleFunc("POST "+spacesPath+"/{id}/archive", Object("space.administer"), s.handleSpaceArchive)
	r.HandleFunc("POST "+spacesPath+"/{id}/unarchive", Object("space.administer"), s.handleSpaceUnarchive)
	r.HandleFunc("DELETE "+spacesPath+"/{id}", Object("space.administer"), s.handleSpaceDelete)
	r.HandleFunc("GET "+spacesPath+"/{id}/tree", Object("space.view"), s.handleTree)
	r.HandleFunc("GET "+spacesPath+"/{id}/grants", Object("space.administer"), s.handleGrants)
	r.HandleFunc("PUT "+spacesPath+"/{id}/grants", Object("space.administer"), s.handleGrantsSet)
	r.HandleFunc("POST "+spacesPath+"/{id}/preset", Object("space.administer"), s.handlePreset)
	r.HandleFunc("GET "+spacesPath+"/{id}/trash", Object("space.administer"), s.handleTrashList)
	r.HandleFunc("GET "+spacesPath+"/{id}/export.zip", Object("space.export"), s.handleSpaceExport)
	r.HandleFunc("POST "+spacesPath+"/{id}/imports", Object("page.create"), s.handleImport)
	r.HandleFunc("GET /api/v1/jobs/{id}", Object("job.owner"), s.handleJob)

	r.HandleFunc("POST /api/v1/trash/{id}/restore", Object("space.administer"), s.handleTrashRestore)
	r.HandleFunc("DELETE /api/v1/trash/{id}", Object("space.administer"), s.handleTrashPurge)

	r.HandleFunc("POST "+pagesPath, Object("page.create"), s.handlePageCreate)
	r.HandleFunc("GET "+pagesPath+"/{id}", Object("page.view"), s.handlePageGet)
	r.HandleFunc("PATCH "+pagesPath+"/{id}", Object("page.edit"), s.handlePageRename)
	r.HandleFunc("DELETE "+pagesPath+"/{id}", Object("page.delete"), s.handlePageDelete)
	r.HandleFunc("POST "+pagesPath+"/{id}/publish", Object("page.edit"), s.handlePublish)
	r.HandleFunc("POST "+pagesPath+"/{id}/move", Object("page.edit"), s.handleMove)
	r.HandleFunc("POST "+pagesPath+"/{id}/copy", Object("page.create"), s.handleCopy)
	r.HandleFunc("GET "+pagesPath+"/{id}/draft", Object("page.view"), s.handleDraftGet)
	r.HandleFunc("PUT "+pagesPath+"/{id}/draft", Object("page.edit"), s.handleDraftSave)
	r.HandleFunc("DELETE "+pagesPath+"/{id}/draft", Object("page.view"), s.handleDraftDiscard)
	r.HandleFunc("GET "+pagesPath+"/{id}/versions", Object("page.view"), s.handleVersions)
	r.HandleFunc("GET "+pagesPath+"/{id}/versions/{number}", Object("page.view"), s.handleVersion)
	r.HandleFunc("POST "+pagesPath+"/{id}/versions/{number}/restore", Object("page.edit"), s.handleVersionRestore)
	r.HandleFunc("GET "+pagesPath+"/{id}/compare", Object("page.view"), s.handleCompare)
	r.HandleFunc("GET "+pagesPath+"/{id}/restrictions", Object("page.view"), s.handleRestrictions)
	r.HandleFunc("PUT "+pagesPath+"/{id}/restrictions", Object("restriction.manage"), s.handleRestrictionsSet)
	r.HandleFunc("GET "+pagesPath+"/{id}/explain", Object("space.administer"), s.handleExplain)
	r.HandleFunc("GET "+pagesPath+"/{id}/export.md", Object("space.export"), s.handlePageExportMD)
	r.HandleFunc("GET "+pagesPath+"/{id}/export.zip", Object("space.export"), s.handlePageExportZip)
	r.HandleFunc("PUT "+pagesPath+"/{id}/favourite", Object("page.view"), s.handleFavouriteOn)
	r.HandleFunc("DELETE "+pagesPath+"/{id}/favourite", Object("page.view"), s.handleFavouriteOff)
	r.HandleFunc("GET "+pagesPath+"/{id}/attachments", Object("page.view"), s.handleAttachments)
	r.HandleFunc("POST "+pagesPath+"/{id}/attachments", Object("attachment.add"), s.handleAttachmentUpload)
	r.HandleFunc("GET /api/v1/attachments/{id}/content", Object("page.view"), s.handleAttachmentContent)
	r.HandleFunc("DELETE /api/v1/attachments/{id}", Object("attachment.delete"), s.handleAttachmentDelete)

	admin := Permission(identity.PermAdministrator)
	r.HandleFunc("POST /api/v1/admin/spaces/{id}/recover", admin, s.handleRecoverSpace)
	r.HandleFunc("POST /api/v1/admin/pages/{id}/recover", admin, s.handleRecoverPage)
	r.HandleFunc("GET /api/v1/admin/spaces", admin, s.handleAdminSpaces)
	r.HandleFunc("GET /api/v1/admin/trash/spaces", admin, s.handleAdminTrashedSpaces)
	r.HandleFunc("GET /api/v1/admin/settings", admin, s.handleSettingsGet)
	r.HandleFunc("PUT /api/v1/admin/settings", admin, s.handleSettingsSet)

	r.HandleFunc("GET /api/v1/me/recent", Authenticated(), s.handleRecent)
	r.HandleFunc("GET /api/v1/me/favourites", Authenticated(), s.handleFavourites)
	r.HandleFunc("GET /api/v1/search", Object("page.view"), s.handleSearch)
}

func (s *Server) caller(r *http.Request) identity.Caller {
	return identity.CallerFor(*PrincipalFrom(r.Context()), metaFrom(r))
}

// fail answers with the right status for an expected error, and audits refusals.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	e, ok := apperr.As(err)
	if !ok {
		s.internalError(w, r, err)
		return
	}
	if e.Denied != nil {
		if p := PrincipalFrom(r.Context()); p != nil {
			if err := s.deps.Identity.RecordObjectDenied(r.Context(), *p, e.Denied.Action, e.Denied.ObjectType, e.Denied.ObjectID, e.Denied.SpaceID, metaFrom(r)); err != nil {
				s.internalError(w, r, err)
				return
			}
		}
	}
	status := http.StatusBadRequest
	switch e.Kind {
	case apperr.KindNotFound:
		status = http.StatusNotFound
	case apperr.KindForbidden:
		status = http.StatusForbidden
	case apperr.KindConflict:
		status = http.StatusConflict
	case apperr.KindTooLarge:
		status = http.StatusRequestEntityTooLarge
	}
	writeError(w, status, e.Code, e.Message)
}

// pathID reads a UUID from the path. A malformed id is simply "not found".
func pathID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
		return uuid.Nil, false
	}
	return id, true
}

func badBody(w http.ResponseWriter) {
	writeError(w, http.StatusBadRequest, "bad_request", "The request body is not valid.")
}

func optionalID(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}
	return uuid.Parse(s)
}

type spaceBody struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

func (b spaceBody) input() spaces.Input {
	return spaces.Input{Key: b.Key, Name: b.Name, Description: b.Description, Icon: b.Icon}
}

func (s *Server) handleSpaceList(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Spaces.List(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"spaces": list})
}

func (s *Server) handleSpaceCreate(w http.ResponseWriter, r *http.Request) {
	var in spaceBody
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	sp, err := s.deps.Spaces.Create(r.Context(), s.caller(r), in.input())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, sp)
}

func (s *Server) handleSpaceGet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	sp, err := s.deps.Spaces.Get(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sp)
}

func (s *Server) handleSpaceByKey(w http.ResponseWriter, r *http.Request) {
	sp, err := s.deps.Spaces.GetByKey(r.Context(), s.caller(r), r.PathValue("key"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sp)
}

func (s *Server) handleSpaceUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in spaceBody
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	sp, err := s.deps.Spaces.Update(r.Context(), s.caller(r), id, in.input())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sp)
}

func (s *Server) spaceAction(w http.ResponseWriter, r *http.Request, act func(identity.Caller, uuid.UUID) error) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := act(s.caller(r), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSpaceArchive(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Spaces.Archive(r.Context(), c, id) })
}

func (s *Server) handleSpaceUnarchive(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Spaces.Unarchive(r.Context(), c, id) })
}

func (s *Server) handleSpaceDelete(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Spaces.Delete(r.Context(), c, id) })
}

func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	tree, err := s.deps.Pages.Tree(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": tree})
}

func (s *Server) handleGrants(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Permissions.Grants(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"grants": list, "all_permissions": permissions.AllSpacePermissions, "templates": permissions.Templates})
}

func (s *Server) handleGrantsSet(w http.ResponseWriter, r *http.Request) {
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
	if err := s.deps.Permissions.SetGrants(r.Context(), s.caller(r), id, in.Subject, in.Permissions); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePreset(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Preset string `json:"preset"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Permissions.ApplyPreset(r.Context(), s.caller(r), id, in.Preset); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTrashList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	items, err := s.deps.Trash.ListSpace(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleTrashRestore(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Trash.Restore(r.Context(), c, id) })
}

func (s *Server) handleTrashPurge(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Trash.Purge(r.Context(), c, id) })
}

func (s *Server) handlePageCreate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SpaceID  uuid.UUID  `json:"space_id"`
		ParentID *uuid.UUID `json:"parent_id"`
		Title    string     `json:"title"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	p, err := s.deps.Pages.Create(r.Context(), s.caller(r), pages.NewPage{SpaceID: in.SpaceID, ParentID: in.ParentID, Title: in.Title})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handlePageGet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	d, err := s.deps.Pages.Get(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handlePageRename(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Title string `json:"title"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	p, err := s.deps.Pages.Rename(r.Context(), s.caller(r), id, in.Title)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handlePageDelete(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Pages.Delete(r.Context(), c, id) })
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Title       string          `json:"title"`
		Document    json.RawMessage `json:"document"`
		BaseVersion int             `json:"base_version"`
		Summary     string          `json:"summary"`
		Overwrite   bool            `json:"overwrite"`
	}
	if decodeJSON(w, r, maxDocumentBody, &in) != nil {
		badBody(w)
		return
	}
	p, err := s.deps.Pages.Publish(r.Context(), s.caller(r), id, pages.PublishInput{Title: in.Title, Document: in.Document,
		BaseVersion: in.BaseVersion, Summary: in.Summary, Overwrite: in.Overwrite})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleMove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		ParentID *uuid.UUID `json:"parent_id"`
		BeforeID *uuid.UUID `json:"before_id"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	p, err := s.deps.Pages.Move(r.Context(), s.caller(r), id, pages.MoveInput{ParentID: in.ParentID, BeforeID: in.BeforeID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleCopy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		ParentID        *uuid.UUID `json:"parent_id"`
		IncludeChildren bool       `json:"include_children"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	p, err := s.deps.Pages.Copy(r.Context(), s.caller(r), id, pages.CopyInput{ParentID: in.ParentID, IncludeChildren: in.IncludeChildren})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleDraftGet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	d, err := s.deps.Pages.GetDraft(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDraftSave(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Title       string          `json:"title"`
		Document    json.RawMessage `json:"document"`
		BaseVersion int             `json:"base_version"`
	}
	if decodeJSON(w, r, maxDocumentBody, &in) != nil {
		badBody(w)
		return
	}
	d, err := s.deps.Pages.SaveDraft(r.Context(), s.caller(r), id, pages.DraftInput{Title: in.Title, Document: in.Document, BaseVersion: in.BaseVersion})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDraftDiscard(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Pages.DiscardDraft(r.Context(), c, id) })
}

func (s *Server) handleVersions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.Versions(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": list})
}

func versionNumber(w http.ResponseWriter, r *http.Request) (int, bool) {
	n, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || n < 1 {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
		return 0, false
	}
	return n, true
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	n, ok := versionNumber(w, r)
	if !ok {
		return
	}
	v, err := s.deps.Pages.GetVersion(r.Context(), s.caller(r), id, n)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleVersionRestore(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	n, ok := versionNumber(w, r)
	if !ok {
		return
	}
	p, err := s.deps.Pages.RestoreVersion(r.Context(), s.caller(r), id, n)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleCompare(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	from, err1 := strconv.Atoi(r.URL.Query().Get("from"))
	to, err2 := strconv.Atoi(r.URL.Query().Get("to"))
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Give two version numbers: from and to.")
		return
	}
	c, err := s.deps.Pages.Compare(r.Context(), s.caller(r), id, from, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleRestrictions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	v, err := s.deps.Permissions.Restrictions(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleRestrictionsSet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Kind     string                `json:"kind"`
		Subjects []permissions.Subject `json:"subjects"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Permissions.SetRestrictions(r.Context(), s.caller(r), id, in.Kind, in.Subjects); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleExplain(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	user, err := uuid.Parse(r.URL.Query().Get("user_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Give a user_id.")
		return
	}
	c := s.caller(r)
	admin, err := s.deps.Identity.HasGlobalPermission(r.Context(), c.UserID, identity.PermAdministrator)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	e, err := s.deps.Permissions.Explain(r.Context(), c, id, user, r.URL.Query().Get("action"), admin)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func attachmentHeader(name string) string {
	return mime.FormatMediaType("attachment", map[string]string{"filename": name})
}

func (s *Server) handlePageExportMD(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	name, body, err := s.deps.Exchange.PageMarkdown(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", attachmentHeader(name))
	_, _ = w.Write(body)
}

// zipResponse delays the headers until the first byte, so a refusal can still
// become a normal JSON error.
type zipResponse struct {
	http.ResponseWriter
	name    string
	started bool
}

func (z *zipResponse) Write(b []byte) (int, error) {
	if !z.started {
		z.started = true
		z.Header().Set("Content-Type", "application/zip")
		z.Header().Set("Content-Disposition", attachmentHeader(z.name))
	}
	return z.ResponseWriter.Write(b)
}

func (s *Server) exportZip(w http.ResponseWriter, r *http.Request, spaceID uuid.UUID, root *uuid.UUID, name string) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(uploadDeadline))
	z := &zipResponse{ResponseWriter: w, name: name}
	if err := s.deps.Exchange.Archive(r.Context(), s.caller(r), spaceID, root, z); err != nil {
		if z.started {
			s.deps.Log.Error("export interrupted", "error", err, "request_id", RequestIDFrom(r.Context()))
			return
		}
		s.fail(w, r, err)
	}
}

func (s *Server) handleSpaceExport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	s.exportZip(w, r, id, nil, "diffluence-space.zip")
}

func (s *Server) handlePageExportZip(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	spaceID, err := s.deps.Pages.SpaceOf(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.exportZip(w, r, spaceID, &id, "diffluence-pages.zip")
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	parent, err := optionalID(r.URL.Query().Get("parent_id"))
	if err != nil {
		badBody(w)
		return
	}
	var parentID *uuid.UUID
	if parent != uuid.Nil {
		parentID = &parent
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadDeadline))
	job, err := s.deps.Exchange.StartImport(r.Context(), s.caller(r), id, parentID, r.URL.Query().Get("filename"), r.Body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": job})
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	j, err := s.deps.Jobs.Get(r.Context(), s.caller(r).UserID, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) setFavourite(w http.ResponseWriter, r *http.Request, on bool) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Pages.SetFavourite(r.Context(), c, id, on) })
}

func (s *Server) handleFavouriteOn(w http.ResponseWriter, r *http.Request) {
	s.setFavourite(w, r, true)
}
func (s *Server) handleFavouriteOff(w http.ResponseWriter, r *http.Request) {
	s.setFavourite(w, r, false)
}

func (s *Server) handleAttachments(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Attachments.List(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"attachments": list})
}

func (s *Server) handleAttachmentUpload(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(uploadDeadline))
	a, err := s.deps.Attachments.Upload(r.Context(), s.caller(r), id, r.URL.Query().Get("filename"), r.Body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// handleAttachmentContent serves a file with everything locked down. Only known
// image types are shown inline; all else is a download. The sandbox policy makes
// sure that even a file a browser decides to render can run nothing.
func (s *Server) handleAttachmentContent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	download := r.URL.Query().Get("download") == "1"
	a, f, err := s.deps.Attachments.Open(r.Context(), s.caller(r), id, download)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer func() { _ = f.Close() }()
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, no-store")
	if !download && attachments.IsInline(a.ContentType) {
		h.Set("Content-Type", a.ContentType)
		h.Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": a.Filename}))
	} else {
		h.Set("Content-Type", "application/octet-stream")
		h.Set("Content-Disposition", attachmentHeader(a.Filename))
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(uploadDeadline))
	http.ServeContent(w, r, "", a.UpdatedAt, f)
}

func (s *Server) handleAttachmentDelete(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Attachments.Delete(r.Context(), c, id) })
}

func (s *Server) recover(w http.ResponseWriter, r *http.Request, act func(identity.Caller, uuid.UUID, string) error) {
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
	if err := act(s.caller(r), id, in.Reason); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRecoverSpace(w http.ResponseWriter, r *http.Request) {
	s.recover(w, r, func(c identity.Caller, id uuid.UUID, reason string) error {
		return s.deps.Permissions.RecoverSpace(r.Context(), c, id, reason)
	})
}

func (s *Server) handleRecoverPage(w http.ResponseWriter, r *http.Request) {
	s.recover(w, r, func(c identity.Caller, id uuid.UUID, reason string) error {
		return s.deps.Permissions.RecoverPage(r.Context(), c, id, reason)
	})
}

func (s *Server) handleAdminSpaces(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Spaces.ListAll(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"spaces": list})
}

func (s *Server) handleAdminTrashedSpaces(w http.ResponseWriter, r *http.Request) {
	items, err := s.deps.Trash.ListSpaces(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	v, err := s.deps.Settings.Get(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleSettingsSet(w http.ResponseWriter, r *http.Request) {
	var in settings.Values
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Settings.Update(r.Context(), s.caller(r), in); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

func (s *Server) handleRecent(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Pages.Recent(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": list})
}

func (s *Server) handleFavourites(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Pages.Favourites(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": list})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := search.Query{Text: q.Get("q")}
	var bad error
	if query.SpaceID, bad = optionalID(q.Get("space_id")); bad == nil {
		query.AuthorID, bad = optionalID(q.Get("author_id"))
	}
	parse := func(key string) time.Time {
		if v := q.Get(key); v != "" && bad == nil {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				bad = errors.New("bad time")
			}
			return t
		}
		return time.Time{}
	}
	query.Since, query.Until = parse("since"), parse("until")
	if v := q.Get("limit"); v != "" && bad == nil {
		query.Limit, bad = strconv.Atoi(v)
	}
	if v := q.Get("offset"); v != "" && bad == nil {
		query.Offset, bad = strconv.Atoi(v)
	}
	if bad != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "A search filter is not valid.")
		return
	}
	results, err := s.deps.Search.Search(r.Context(), s.caller(r).UserID, query)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
