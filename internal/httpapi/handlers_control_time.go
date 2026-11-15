// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/csvout"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Document control, wave DC5 of spec 064: delegation and absence, read and understood, the
// review period of a page, the reports, imported signatures and the move from today's
// approvals.
func (s *Server) registerControlTimeRoutes() {
	r := s.router
	me := Authenticated()
	// A reviewer may act on a page they cannot view, so these are found by the page id and
	// the module decides, as for the other review routes (DOC-203).
	r.HandleFunc("POST /api/v1/pages/{id}/review/delegate", me, s.handleReviewDelegate)
	r.HandleFunc("GET /api/v1/me/absence", SessionOnly(me), s.handleAbsence)
	r.HandleFunc("PUT /api/v1/me/absence", SessionOnly(me), s.handleAbsenceSet)
	r.HandleFunc("DELETE /api/v1/me/absence", SessionOnly(me), s.handleAbsenceEnd)
	// A confirmation of reading needs the session that opened the version (DOC-824).
	r.HandleFunc("POST /api/v1/pages/{id}/read-confirmation", SessionOnly(me), s.handleReadConfirm)
	r.HandleFunc("GET /api/v1/pages/{id}/read-confirmations", me, s.handleReadList)
	r.HandleFunc("PUT /api/v1/pages/{id}/review-period", me, s.handleReviewPeriod)
	r.HandleFunc("GET /api/v1/pages/{id}/imported-signatures", Object("page.view"), s.handleImportedSignatures)
	r.HandleFunc("GET /api/v1/spaces/{id}/control-reports/{kind}", Object(permissions.SpaceAudit), s.handleSpaceControlReport)
	r.HandleFunc("GET /api/v1/control-reports/{kind}", me, s.handleControlReport)
	r.HandleFunc("GET /api/v1/spaces/{id}/workflow/from-approval-rule", Object(permissions.SpaceWorkflows), s.handleRuleMoveOffer)
	r.HandleFunc("POST /api/v1/spaces/{id}/workflow/from-approval-rule", Object(permissions.SpaceWorkflows), s.handleRuleMoveAccept)
	r.HandleFunc("POST /api/v1/spaces/{id}/workflow/from-approval-rule/undo", Object(permissions.SpaceWorkflows), s.handleRuleMoveUndo)
}

func (s *Server) handleReviewDelegate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		From   *uuid.UUID `json:"from"`
		To     uuid.UUID  `json:"to"`
		Reason string     `json:"reason"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.Delegate(r.Context(), s.caller(r), id, pages.DelegateInput{From: in.From, To: in.To, Reason: in.Reason}); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"delegated": true})
}

func (s *Server) handleAbsence(w http.ResponseWriter, r *http.Request) {
	a, err := s.deps.Pages.AbsenceOf(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"absence": a})
}

func (s *Server) handleAbsenceSet(w http.ResponseWriter, r *http.Request) {
	var in pages.Absence
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.SetAbsence(r.Context(), s.caller(r), in); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"absence": in})
}

func (s *Server) handleAbsenceEnd(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Pages.EndAbsence(r.Context(), s.caller(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sessionOf is the login session of a request; a token has none.
func sessionOf(r *http.Request) uuid.UUID {
	p := PrincipalFrom(r.Context())
	if p == nil || p.ViaToken {
		return uuid.Nil
	}
	return p.SessionID
}

// noteReadOpened records that a person opened the version they are asked to confirm, when the
// detail shows them that version, and lets the band offer the confirmation (DOC-824).
func (s *Server) noteReadOpened(r *http.Request, d *pages.Detail) error {
	ci := d.Control
	if ci == nil || ci.ReadRequest == nil || ci.ReadRequest.Confirmed || ci.ReadRequest.Version != d.CurrentVersion {
		return nil
	}
	session := sessionOf(r)
	if session == uuid.Nil {
		return nil
	}
	if err := s.deps.Pages.NoteOpened(r.Context(), s.caller(r).UserID, d.ID, d.CurrentVersion, session); err != nil {
		return err
	}
	ci.ReadRequest.CanConfirm = true
	return nil
}

func (s *Server) handleReadConfirm(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Version int `json:"version"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.ConfirmRead(r.Context(), s.caller(r), id, in.Version, sessionOf(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"confirmed": true, "version": in.Version})
}

func (s *Server) handleReadList(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.ReadList(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if r.URL.Query().Get("format") != "csv" {
		writeJSON(w, http.StatusOK, map[string]any{"people": list})
		return
	}
	startCSV(w, "read-and-understood")
	out, err := csvout.New(w, "name", "email", "version", "asked", "late", "confirmed")
	if err == nil {
		for _, p := range list {
			confirmed := ""
			if p.ConfirmedAt != nil {
				confirmed = p.ConfirmedAt.UTC().Format(time.RFC3339)
			}
			if err = out.Row(p.Name, p.Email, fmt.Sprint(p.Version), p.AskedAt.UTC().Format(time.RFC3339), fmt.Sprint(p.Late), confirmed); err != nil {
				break
			}
		}
	}
	if err == nil {
		err = out.Close()
	}
	if err != nil {
		s.deps.Log.Error("read list export interrupted", "error", err, "request_id", RequestIDFrom(r.Context()))
	}
}

func (s *Server) handleReviewPeriod(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Months *int `json:"months"`
	}
	if !body(w, r, &in) {
		return
	}
	if err := s.deps.Pages.SetReviewPeriod(r.Context(), s.caller(r), id, in.Months); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"months": in.Months})
}

func (s *Server) handleImportedSignatures(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Pages.ImportedSignatures(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"signatures": list})
}

func (s *Server) handleSpaceControlReport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	s.writeControlReport(w, r, &id)
}

func (s *Server) handleControlReport(w http.ResponseWriter, r *http.Request) {
	s.writeControlReport(w, r, nil)
}

// writeControlReport answers a report as JSON, or as CSV with ?format=csv (DOC-1030).
func (s *Server) writeControlReport(w http.ResponseWriter, r *http.Request, spaceID *uuid.UUID) {
	in := pages.ReportInput{Kind: r.PathValue("kind"), SpaceID: spaceID}
	for name, into := range map[string]**time.Time{"from": &in.From, "to": &in.To} {
		if v := r.URL.Query().Get(name); v != "" {
			t, err := time.Parse(time.DateOnly, v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "bad_request", "A date is written as 2026-01-31.")
				return
			}
			*into = &t
		}
	}
	report, err := s.deps.Pages.ControlReport(r.Context(), s.caller(r), in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if r.URL.Query().Get("format") != "csv" {
		writeJSON(w, http.StatusOK, report)
		return
	}
	startCSV(w, "document-control-"+report.Kind)
	out, err := csvout.New(w, report.Columns...)
	for i := 0; err == nil && i < len(report.Rows); i++ {
		cells := make([]string, len(report.Columns))
		for j, col := range report.Columns {
			cells[j] = reportCell(report.Rows[i][col])
		}
		err = out.Row(cells...)
	}
	if err == nil {
		err = out.Close()
	}
	if err != nil {
		s.deps.Log.Error("report export interrupted", "error", err, "request_id", RequestIDFrom(r.Context()))
	}
}

func reportCell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(raw)
}

func (s *Server) handleRuleMoveOffer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	offer, err := s.deps.Pages.OfferApprovalRuleMove(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, offer)
}

func (s *Server) handleRuleMoveAccept(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	done, err := s.deps.Pages.AcceptApprovalRuleMove(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, done)
}

func (s *Server) handleRuleMoveUndo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.deps.Pages.UndoApprovalRuleMove(r.Context(), s.caller(r), id); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"restored": true})
}
