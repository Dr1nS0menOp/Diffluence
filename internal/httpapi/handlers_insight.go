// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/analytics"
	"github.com/diffluence/diffluence/internal/csvout"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/presence"
)

const popularPagesShown = 5

func (s *Server) registerInsightRoutes() {
	r := s.router
	admin := Permission(identity.PermAdministrator)
	view := Object(permissions.ActionPageView)
	insights := Object(permissions.AnalyticsView)

	r.HandleFunc("POST /api/v1/pages/{id}/views", view, s.handleViewOpened)
	r.HandleFunc("PATCH /api/v1/pages/{id}/views", view, s.handleViewEngaged)
	r.HandleFunc("PUT /api/v1/pages/{id}/presence", view, s.handlePresence)
	r.HandleFunc("GET /api/v1/pages/{id}/insights", insights, s.handlePageInsights)
	r.HandleFunc("GET /api/v1/spaces/{id}/popular", Object(permissions.SpaceView), s.handleSpacePopular)
	r.HandleFunc("GET /api/v1/spaces/{id}/insights", insights, s.handleSpaceInsights)
	r.HandleFunc("GET /api/v1/spaces/{id}/insights/content", insights, s.handleSpaceContent)
	r.HandleFunc("GET /api/v1/spaces/{id}/insights/people", insights, s.handleSpaceReaders)
	r.HandleFunc("GET /api/v1/admin/insights", admin, s.handleInstanceInsights)
	r.HandleFunc("GET /api/v1/admin/insights/search", admin, s.handleSearchInsights)
	r.HandleFunc("GET /api/v1/admin/analytics", admin, s.handleAnalyticsSettings)
	r.HandleFunc("PUT /api/v1/admin/analytics", SessionOnly(admin), s.handleAnalyticsSettingsSet)
	r.HandleFunc("DELETE /api/v1/admin/analytics/data", SessionOnly(admin), s.handleAnalyticsDelete)
	r.HandleFunc("GET /api/v1/openapi.json", Authenticated(), s.handleOpenAPI)
	r.HandleFunc("GET /api/v1/admin/outgoing-hosts", admin, s.handleOutgoingHosts)
	r.HandleFunc("PUT /api/v1/admin/outgoing-hosts", SessionOnly(admin), s.handleOutgoingHostsSet)
}

func (s *Server) handleViewOpened(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		From *uuid.UUID `json:"from"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	c := s.caller(r)
	if err := s.deps.Analytics.Record(r.Context(), c, analytics.Visit{PageID: id, From: in.From}); err != nil {
		s.fail(w, r, err)
		return
	}
	views, mayKnow, err := s.deps.Analytics.ViewCount(r.Context(), c, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !mayKnow {
		writeJSON(w, http.StatusOK, map[string]any{"views": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"views": views})
}

func (s *Server) handleViewEngaged(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.deps.Analytics.Record(r.Context(), s.caller(r), analytics.Visit{PageID: id, Engaged: true}); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePresence(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Mode string `json:"mode"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	answer, err := s.deps.Presence.Beat(r.Context(), s.caller(r), presence.Beat{PageID: id, Mode: in.Mode})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, answer)
}

func (s *Server) period(w http.ResponseWriter, r *http.Request) (analytics.Period, bool) {
	p, err := s.deps.Analytics.PeriodOf(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		s.fail(w, r, err)
		return p, false
	}
	return p, true
}

func (s *Server) handlePageInsights(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	p, ok := s.period(w, r)
	if !ok {
		return
	}
	out, err := s.deps.Analytics.PageReport(r.Context(), s.caller(r), id, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSpacePopular(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	c := s.caller(r)
	if err := s.deps.Permissions.RequireSpace(r.Context(), s.deps.Pool, c.UserID, id, permissions.SpaceView); err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.deps.Analytics.Popular(r.Context(), c, id, popularPagesShown)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// The overview is for everyone in the space, and numbers need the permission.
	for i := range list {
		list[i].Count = 0
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": list})
}

func (s *Server) handleSpaceInsights(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	c := s.caller(r)
	stale, err := s.deps.Analytics.Stale(r.Context(), c, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	popular, err := s.deps.Analytics.Popular(r.Context(), c, id, popularPagesShown)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	st, err := s.deps.Analytics.Settings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": st.Enabled, "mode": st.Mode, "popular": popular, "stale": stale})
}

func startCSV(w http.ResponseWriter, name string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.csv"`)
}

func (s *Server) handleSpaceContent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	p, ok := s.period(w, r)
	if !ok {
		return
	}
	c := s.caller(r)
	list, err := s.deps.Analytics.SpaceReport(r.Context(), c, id, p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if r.URL.Query().Get("format") != "csv" {
		writeJSON(w, http.StatusOK, map[string]any{"pages": list})
		return
	}
	startCSV(w, "space-content")
	if err := analytics.WriteContentCSV(w, list); err != nil {
		s.deps.Log.Error("insights export interrupted", "error", err, "request_id", RequestIDFrom(r.Context()))
	}
}

func (s *Server) handleSpaceReaders(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Analytics.SpacePeople(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"people": list})
}

func (s *Server) handleInstanceInsights(w http.ResponseWriter, r *http.Request) {
	p, ok := s.period(w, r)
	if !ok {
		return
	}
	list, err := s.deps.Analytics.InstanceReport(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if r.URL.Query().Get("format") != "csv" {
		writeJSON(w, http.StatusOK, map[string]any{"days": list})
		return
	}
	startCSV(w, "instance-insights")
	if err := analytics.WriteInstanceCSV(w, list); err != nil {
		s.deps.Log.Error("insights export interrupted", "error", err, "request_id", RequestIDFrom(r.Context()))
	}
}

func (s *Server) handleSearchInsights(w http.ResponseWriter, r *http.Request) {
	p, ok := s.period(w, r)
	if !ok {
		return
	}
	list, err := s.deps.Analytics.Terms(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if r.URL.Query().Get("format") != "csv" {
		writeJSON(w, http.StatusOK, map[string]any{"terms": list})
		return
	}
	startCSV(w, "search-insights")
	if err := writeTermsCSV(w, list); err != nil {
		s.deps.Log.Error("insights export interrupted", "error", err, "request_id", RequestIDFrom(r.Context()))
	}
}

func writeTermsCSV(w http.ResponseWriter, list []analytics.Term) error {
	out, err := csvout.New(w, "term", "searches", "without results")
	if err != nil {
		return err
	}
	for _, t := range list {
		if err := out.Row(t.Term, strconv.Itoa(t.Searches), strconv.Itoa(t.Empty)); err != nil {
			return err
		}
	}
	return out.Close()
}

func (s *Server) handleAnalyticsSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.deps.Analytics.Settings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleAnalyticsSettingsSet(w http.ResponseWriter, r *http.Request) {
	var in analytics.Settings
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Analytics.SetSettings(r.Context(), s.caller(r), in); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

func (s *Server) handleAnalyticsDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Analytics.DeleteData(r.Context(), s.caller(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOutgoingHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.deps.Outward.Hosts(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hosts": hosts})
}

func (s *Server) handleOutgoingHostsSet(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Hosts []string `json:"hosts"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Outward.SetHosts(r.Context(), s.caller(r), in.Hosts); err != nil {
		s.fail(w, r, err)
		return
	}
	s.handleOutgoingHosts(w, r)
}
