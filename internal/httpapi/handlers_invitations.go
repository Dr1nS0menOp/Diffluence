// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"

	"github.com/diffluence/diffluence/internal/sharing"
)

func (s *Server) handleInvitationAsk(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Email string `json:"email"`
		Note  string `json:"note"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	request, err := s.deps.Sharing.AskInvitation(r.Context(), s.caller(r), id, sharing.InvitationAsk{Email: in.Email, Note: in.Note})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"id": request, "waiting": true})
}

func (s *Server) handleInvitationRequests(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Sharing.WaitingInvitations(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": list})
}

func (s *Server) handleInvitationApprove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	reason, ok := answerReason(w, r)
	if !ok {
		return
	}
	link, err := s.deps.Sharing.ApproveInvitation(r.Context(), s.caller(r), id, reason)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"link": link})
}

func (s *Server) handleInvitationRefuse(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	reason, ok := answerReason(w, r)
	if !ok {
		return
	}
	if err := s.deps.Sharing.RefuseInvitation(r.Context(), s.caller(r), id, reason); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func answerReason(w http.ResponseWriter, r *http.Request) (string, bool) {
	var in struct {
		Reason string `json:"reason"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return "", false
	}
	return in.Reason, true
}
