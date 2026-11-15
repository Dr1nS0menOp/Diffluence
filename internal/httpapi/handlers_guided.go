// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"strconv"

	"github.com/diffluence/diffluence/internal/conditions"
	"github.com/diffluence/diffluence/internal/templates"
)

// The routes of guided templates (spec 074). Each is registered behind the experimental
// switch in registerTemplateRoutes; the templates module checks who may do what.

func (s *Server) handleTemplateEntries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Templates.Entries(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleTemplateEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	e, err := s.deps.Templates.Entry(r.Context(), s.caller(r), id, r.PathValue("key"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// entryInput reads an entry, which may hold a whole document.
func entryInput(w http.ResponseWriter, r *http.Request) (templates.EntryInput, bool) {
	var in templates.EntryInput
	if decodeJSON(w, r, maxDocumentBody, &in) != nil {
		badBody(w)
		return in, false
	}
	return in, true
}

func (s *Server) handleTemplateEntryAdd(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	in, ok := entryInput(w, r)
	if !ok {
		return
	}
	e, err := s.deps.Templates.AddEntry(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) handleTemplateEntryUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	in, ok := entryInput(w, r)
	if !ok {
		return
	}
	e, err := s.deps.Templates.UpdateEntry(r.Context(), s.caller(r), id, r.PathValue("key"), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleTemplateEntryRemove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.deps.Templates.RemoveEntry(r.Context(), s.caller(r), id, r.PathValue("key")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTemplatePublishing(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Publish bool `json:"publish"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Templates.SetPublishing(r.Context(), s.caller(r), id, in.Publish); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTemplateVersions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Templates.Versions(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": list})
}

func (s *Server) handleTemplateVersionChanges(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || number < 1 {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
		return
	}
	changes, err := s.deps.Templates.VersionChanges(r.Context(), s.caller(r), id, number)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, changes)
}

func (s *Server) handleTemplateEntryMove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		To string `json:"to"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Templates.MoveEntry(r.Context(), s.caller(r), id, r.PathValue("key"), in.To); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTemplateEntryCondition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Condition *conditions.Condition `json:"condition"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Templates.SetEntryCondition(r.Context(), s.caller(r), id, r.PathValue("key"), in.Condition); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTemplateRules(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in templates.RulesInput
	if !body(w, r, &in) {
		return
	}
	rules, err := s.deps.Templates.SetLabelRules(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"label_rules": rules})
}

func (s *Server) handleTemplateFieldRename(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in templates.RenameInput
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Templates.RenameField(r.Context(), s.caller(r), id, in); err != nil {
		s.fail(w, r, err)
		return
	}
	form, err := s.deps.Templates.FormOf(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, form)
}

func (s *Server) handleTemplateFields(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in templates.FieldsInput
	if !body(w, r, &in) {
		return
	}
	form, err := s.deps.Templates.SetFields(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, form)
}

func (s *Server) handleTemplateSet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in templates.SetInput
	if !body(w, r, &in) {
		return
	}
	made, err := s.deps.Templates.MakeSet(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeSet(w, made)
}

func (s *Server) handleTemplateSetCheck(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in templates.SetInput
	if !body(w, r, &in) {
		return
	}
	check, err := s.deps.Templates.CheckSet(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, check)
}
