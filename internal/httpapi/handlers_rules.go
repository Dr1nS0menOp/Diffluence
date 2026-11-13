// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/automation"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/webhooks"
)

func (s *Server) registerRuleRoutes() {
	r := s.router
	admin := Permission(identity.PermAdministrator)
	manage := Object(permissions.SpaceAutomation)

	// Automation is an experimental part (spec 069): while it is off, every route of it
	// answers 404 experimental_off, except the list of a page, which is then empty.
	on := func(next http.HandlerFunc) http.HandlerFunc {
		return s.experimental(features.ExperimentalAutomation, next)
	}
	r.HandleFunc("GET /api/v1/spaces/{id}/rules", manage, on(s.handleSpaceRules))
	r.HandleFunc("POST /api/v1/spaces/{id}/rules", manage, on(s.handleSpaceRuleCreate))
	r.HandleFunc("GET /api/v1/rules/{id}", manage, on(s.handleRule))
	r.HandleFunc("PUT /api/v1/rules/{id}", manage, on(s.handleRuleUpdate))
	r.HandleFunc("DELETE /api/v1/rules/{id}", manage, on(s.handleRuleDelete))
	r.HandleFunc("POST /api/v1/rules/{id}/test", manage, on(s.handleRuleTest))
	r.HandleFunc("GET /api/v1/rules/{id}/runs", manage, on(s.handleRuleRuns))
	r.HandleFunc("POST /api/v1/rules/{id}/hook", SessionOnly(manage), on(s.handleRuleHook))
	// The module decides: page.view for a rule with a manual trigger, otherwise manage.
	r.HandleFunc("POST /api/v1/rules/{id}/run", Object(permissions.ActionPageView), on(s.handleRuleRun))
	r.HandleFunc("GET /api/v1/pages/{id}/rules", Object(permissions.ActionPageView), s.handlePageRules)
	r.HandleFunc("GET /api/v1/admin/rules", admin, on(s.handleInstanceRules))
	r.HandleFunc("POST /api/v1/admin/rules", admin, on(s.handleInstanceRuleCreate))
	r.HandleFunc("GET /api/v1/admin/automation", admin, on(s.handleAutomationLimit))
	r.HandleFunc("PUT /api/v1/admin/automation", SessionOnly(admin), on(s.handleAutomationLimitSet))
	// Public: an outside system has no session here. The 256-bit token in the address
	// is the permission, it is stored hashed, an unknown token is a plain 404, and the
	// rule still acts only as its account, within its limit of runs per hour.
	r.HandleFunc("POST /hooks/{token}", Public, on(s.handleIncomingHook))

	r.HandleFunc("GET /api/v1/spaces/{id}/webhooks", manage, s.handleSpaceWebhooks)
	r.HandleFunc("POST /api/v1/spaces/{id}/webhooks", manage, s.handleSpaceWebhookCreate)
	r.HandleFunc("GET /api/v1/webhooks/{id}", manage, s.handleWebhook)
	r.HandleFunc("PATCH /api/v1/webhooks/{id}", manage, s.handleWebhookUpdate)
	r.HandleFunc("DELETE /api/v1/webhooks/{id}", manage, s.handleWebhookDelete)
	r.HandleFunc("GET /api/v1/webhooks/{id}/deliveries", manage, s.handleWebhookDeliveries)
	r.HandleFunc("POST /api/v1/webhooks/{id}/deliveries/{delivery}/resend", manage, s.handleWebhookResend)
	r.HandleFunc("POST /api/v1/webhooks/{id}/rotate", SessionOnly(manage), s.handleWebhookRotate)
	r.HandleFunc("GET /api/v1/admin/webhooks", admin, s.handleInstanceWebhooks)
	r.HandleFunc("POST /api/v1/admin/webhooks", admin, s.handleInstanceWebhookCreate)
}

func (s *Server) writeRules(w http.ResponseWriter, r *http.Request, spaceID *uuid.UUID) {
	list, err := s.deps.Automation.List(r.Context(), s.caller(r), spaceID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": list})
}

func (s *Server) createRule(w http.ResponseWriter, r *http.Request, spaceID *uuid.UUID) {
	var in automation.Input
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	rule, err := s.deps.Automation.Create(r.Context(), s.caller(r), spaceID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, rule)
}

func (s *Server) handleSpaceRules(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		s.writeRules(w, r, &id)
	}
}

func (s *Server) handleSpaceRuleCreate(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		s.createRule(w, r, &id)
	}
}

func (s *Server) handleInstanceRules(w http.ResponseWriter, r *http.Request) { s.writeRules(w, r, nil) }

func (s *Server) handleInstanceRuleCreate(w http.ResponseWriter, r *http.Request) {
	s.createRule(w, r, nil)
}

func (s *Server) handleRule(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	rule, err := s.deps.Automation.Get(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleRuleUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in automation.Input
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	rule, err := s.deps.Automation.Update(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleRuleDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.deps.Automation.Delete(r.Context(), s.caller(r), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRuleTest(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		PageID *uuid.UUID `json:"page_id"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	out, err := s.deps.Automation.Test(r.Context(), s.caller(r), id, in.PageID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRuleRuns(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Automation.Runs(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": list})
}

func (s *Server) handleRuleHook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	token, err := s.deps.Automation.NewHook(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"address": s.deps.Config.PublicOrigin() + "/hooks/" + token})
}

func (s *Server) handleRuleRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		PageID uuid.UUID `json:"page_id"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Automation.RunByHand(r.Context(), s.caller(r), id, in.PageID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handlePageRules(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	// Every page asks this, so while automation is off the answer is an empty list,
	// which is the truth: no rule can run.
	on, err := s.deps.Features.ExperimentalOn(r.Context(), s.deps.Pool, features.ExperimentalAutomation)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names := []map[string]any{}
	if !on {
		writeJSON(w, http.StatusOK, map[string]any{"rules": names})
		return
	}
	list, err := s.deps.Automation.Manual(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, rule := range list {
		names = append(names, map[string]any{"id": rule.ID, "name": rule.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": names})
}

func (s *Server) handleAutomationLimit(w http.ResponseWriter, r *http.Request) {
	limit, err := s.deps.Automation.RunsPerHour(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs_per_hour": limit})
}

func (s *Server) handleAutomationLimitSet(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RunsPerHour int `json:"runs_per_hour"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Automation.SetRunsPerHour(r.Context(), s.caller(r), in.RunsPerHour); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

func (s *Server) handleIncomingHook(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Automation.Incoming(r.Context(), r.PathValue("token")); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) writeWebhooks(w http.ResponseWriter, r *http.Request, spaceID *uuid.UUID) {
	list, err := s.deps.Webhooks.List(r.Context(), s.caller(r), spaceID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhooks": list, "events": webhooks.Events})
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request, spaceID *uuid.UUID) {
	var in webhooks.Input
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	endpoint, secret, err := s.deps.Webhooks.Create(r.Context(), s.caller(r), spaceID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"webhook": endpoint, "secret": secret})
}

func (s *Server) handleSpaceWebhooks(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		s.writeWebhooks(w, r, &id)
	}
}

func (s *Server) handleSpaceWebhookCreate(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "id"); ok {
		s.createWebhook(w, r, &id)
	}
}

func (s *Server) handleInstanceWebhooks(w http.ResponseWriter, r *http.Request) {
	s.writeWebhooks(w, r, nil)
}

func (s *Server) handleInstanceWebhookCreate(w http.ResponseWriter, r *http.Request) {
	s.createWebhook(w, r, nil)
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	endpoint, err := s.deps.Webhooks.Get(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, endpoint)
}

func (s *Server) handleWebhookUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in webhooks.Input
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	endpoint, err := s.deps.Webhooks.Update(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, endpoint)
}

func (s *Server) handleWebhookDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.deps.Webhooks.Delete(r.Context(), s.caller(r), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Webhooks.Deliveries(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": list})
}

func (s *Server) handleWebhookResend(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	delivery, ok := pathID(w, r, "delivery")
	if !ok {
		return
	}
	if err := s.deps.Webhooks.Resend(r.Context(), s.caller(r), id, delivery); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleWebhookRotate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	secret, err := s.deps.Webhooks.Rotate(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"secret": secret})
}
