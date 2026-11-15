// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/templates"
)

// The routes of rights per template and approval (spec 074, wave GT4). Each is behind the
// experimental switch of guided templates; the templates module decides per template who may.
func (s *Server) registerTemplateApprovalRoutes(guided func(http.HandlerFunc) http.HandlerFunc) {
	r := s.router
	me := Authenticated()
	admin := Permission(identity.PermAdministrator)
	r.HandleFunc("GET /api/v1/templates/{id}/rights", me, guided(s.handleTemplateRights))
	r.HandleFunc("PUT /api/v1/templates/{id}/rights", me, guided(s.handleTemplateRightsSet))
	r.HandleFunc("POST /api/v1/templates/{id}/versions/{number}/propose", me, guided(s.handleTemplatePropose))
	r.HandleFunc("POST /api/v1/templates/{id}/versions/{number}/approve", me, guided(s.handleTemplateApprove))
	r.HandleFunc("POST /api/v1/templates/{id}/versions/{number}/send-back", me, guided(s.handleTemplateSendBack))
	r.HandleFunc("POST /api/v1/templates/{id}/withdraw", me, guided(s.handleTemplateWithdraw))
	r.HandleFunc("GET /api/v1/spaces/{id}/my-templates", Object("template.propose"), guided(s.handleMyTemplates))
	r.HandleFunc("POST /api/v1/spaces/{id}/my-templates", Object("template.propose"), guided(s.handleMyTemplateCreate))
	r.HandleFunc("GET /api/v1/spaces/{id}/template-review", Object("space.templates"), guided(s.handleSpaceFourEyes))
	r.HandleFunc("PUT /api/v1/spaces/{id}/template-review", Object("space.administer"), guided(s.handleSpaceFourEyesSet))
	r.HandleFunc("GET /api/v1/admin/template-review", admin, guided(s.handleInstanceFourEyes))
	r.HandleFunc("PUT /api/v1/admin/template-review", admin, guided(s.handleInstanceFourEyesSet))
}

func (s *Server) handleTemplateNewest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	t, err := s.deps.Templates.Newest(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleTemplateRights(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	view, err := s.deps.Templates.RightsOf(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleTemplateRightsSet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in templates.Rights
	if !body(w, r, &in) {
		return
	}
	view, err := s.deps.Templates.SetRights(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// versionPath reads the template and the version a route names; a number that is no
// version is not found.
func versionPath(w http.ResponseWriter, r *http.Request) (uuid.UUID, int, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return id, 0, false
	}
	number, err := strconv.Atoi(r.PathValue("number"))
	if err != nil || number < 1 {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
		return id, 0, false
	}
	return id, number, true
}

// decisionBody is the words of a decision: a comment, or a reason.
type decisionBody struct {
	Comment string `json:"comment"`
	Reason  string `json:"reason"`
}

func (s *Server) writeHistory(w http.ResponseWriter, r *http.Request, history templates.History, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, history)
}

func (s *Server) handleTemplatePropose(w http.ResponseWriter, r *http.Request) {
	id, number, ok := versionPath(w, r)
	if !ok {
		return
	}
	history, err := s.deps.Templates.Propose(r.Context(), s.caller(r), id, number)
	s.writeHistory(w, r, history, err)
}

func (s *Server) handleTemplateApprove(w http.ResponseWriter, r *http.Request) {
	id, number, ok := versionPath(w, r)
	if !ok {
		return
	}
	var in decisionBody
	if !body(w, r, &in) {
		return
	}
	history, err := s.deps.Templates.Approve(r.Context(), s.caller(r), id, number, in.Comment)
	s.writeHistory(w, r, history, err)
}

func (s *Server) handleTemplateSendBack(w http.ResponseWriter, r *http.Request) {
	id, number, ok := versionPath(w, r)
	if !ok {
		return
	}
	var in decisionBody
	if !body(w, r, &in) {
		return
	}
	history, err := s.deps.Templates.SendBack(r.Context(), s.caller(r), id, number, in.Reason)
	s.writeHistory(w, r, history, err)
}

func (s *Server) handleTemplateWithdraw(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in decisionBody
	if !body(w, r, &in) {
		return
	}
	history, err := s.deps.Templates.Withdraw(r.Context(), s.caller(r), id, in.Reason)
	s.writeHistory(w, r, history, err)
}

func (s *Server) handleMyTemplates(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Templates.Mine(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": list})
}

func (s *Server) handleMyTemplateCreate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in templates.Input
	if decodeJSON(w, r, maxDocumentBody, &in) != nil {
		badBody(w)
		return
	}
	t, err := s.deps.Templates.CreateDraft(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// fourEyesBody is the setting "every template change needs a second person".
type fourEyesBody struct {
	On bool `json:"on"`
}

func (s *Server) writeFourEyes(w http.ResponseWriter, r *http.Request, v templates.FourEyes, err error) {
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleSpaceFourEyes(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	v, err := s.deps.Templates.FourEyesOf(r.Context(), s.caller(r), &id)
	s.writeFourEyes(w, r, v, err)
}

func (s *Server) handleSpaceFourEyesSet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in fourEyesBody
	if !body(w, r, &in) {
		return
	}
	v, err := s.deps.Templates.SetFourEyes(r.Context(), s.caller(r), &id, in.On)
	s.writeFourEyes(w, r, v, err)
}

func (s *Server) handleInstanceFourEyes(w http.ResponseWriter, r *http.Request) {
	v, err := s.deps.Templates.FourEyesOf(r.Context(), s.caller(r), nil)
	s.writeFourEyes(w, r, v, err)
}

func (s *Server) handleInstanceFourEyesSet(w http.ResponseWriter, r *http.Request) {
	var in fourEyesBody
	if !body(w, r, &in) {
		return
	}
	v, err := s.deps.Templates.SetFourEyes(r.Context(), s.caller(r), nil, in.On)
	s.writeFourEyes(w, r, v, err)
}
