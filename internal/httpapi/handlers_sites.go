// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/sites"
)

func (s *Server) registerSiteRoutes() {
	r := s.router
	rule := Object("site.publish")
	admin := Permission(identity.PermAdministrator)

	r.HandleFunc("GET /api/v1/spaces/{id}/sites", rule, s.handleSites)
	r.HandleFunc("POST /api/v1/spaces/{id}/sites", rule, s.handleSiteCreate)
	r.HandleFunc("GET /api/v1/sites/{id}", rule, s.handleSite)
	r.HandleFunc("PATCH /api/v1/sites/{id}", rule, s.handleSiteUpdate)
	r.HandleFunc("DELETE /api/v1/sites/{id}", rule, s.handleSiteDelete)
	r.HandleFunc("GET /api/v1/sites/{id}/pages", rule, s.handleSitePages)
	r.HandleFunc("PUT /api/v1/sites/{id}/pages", rule, s.handleSitePagesSet)
	r.HandleFunc("POST /api/v1/sites/{id}/preview", rule, s.handleSitePreview)
	r.HandleFunc("POST /api/v1/sites/{id}/publish", rule, s.handleSitePublish)
	r.HandleFunc("GET /api/v1/sites/{id}/builds", rule, s.handleSiteBuilds)
	r.HandleFunc("POST /api/v1/sites/{id}/rollback", rule, s.handleSiteRollback)
	r.HandleFunc("POST /api/v1/sites/{id}/withdraw", rule, s.handleSiteWithdraw)
	r.HandleFunc("GET /api/v1/sites/{id}/download", rule, s.handleSiteDownload)
	r.HandleFunc("GET /api/v1/pages/{id}/public-state", Object("page.view"), s.handlePublicState)
	r.HandleFunc("GET /api/v1/pages/{id}/public-link", rule, s.handleLink)
	r.HandleFunc("PUT /api/v1/pages/{id}/public-link", rule, s.handleLinkMake)
	r.HandleFunc("DELETE /api/v1/pages/{id}/public-link", rule, s.handleLinkWithdraw)
	r.HandleFunc("GET /api/v1/admin/public-links", admin, s.handleLinks)
}

// siteAnswer runs something that needs an id and gives JSON.
func siteAnswer[T any](s *Server, w http.ResponseWriter, r *http.Request, status int, run func(identity.Caller, uuid.UUID) (T, error)) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := run(s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, status, out)
}

func (s *Server) siteAction(w http.ResponseWriter, r *http.Request, run func(identity.Caller, uuid.UUID) error) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := run(s.caller(r), id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	siteAnswer(s, w, r, http.StatusOK, func(c identity.Caller, id uuid.UUID) (map[string]any, error) {
		list, err := s.deps.Sites.List(r.Context(), c, id)
		return map[string]any{"sites": list}, err
	})
}

func (s *Server) handleSiteCreate(w http.ResponseWriter, r *http.Request) {
	var in sites.Input
	if decodeJSON(w, r, maxExchangeBody, &in) != nil {
		badBody(w)
		return
	}
	siteAnswer(s, w, r, http.StatusCreated, func(c identity.Caller, id uuid.UUID) (sites.Site, error) {
		return s.deps.Sites.Create(r.Context(), c, id, in)
	})
}

func (s *Server) handleSite(w http.ResponseWriter, r *http.Request) {
	siteAnswer(s, w, r, http.StatusOK, func(c identity.Caller, id uuid.UUID) (sites.Site, error) {
		return s.deps.Sites.Get(r.Context(), c, id)
	})
}

func (s *Server) handleSiteUpdate(w http.ResponseWriter, r *http.Request) {
	var in sites.Input
	if decodeJSON(w, r, maxExchangeBody, &in) != nil {
		badBody(w)
		return
	}
	siteAnswer(s, w, r, http.StatusOK, func(c identity.Caller, id uuid.UUID) (sites.Site, error) {
		return s.deps.Sites.Update(r.Context(), c, id, in)
	})
}

func (s *Server) handleSiteDelete(w http.ResponseWriter, r *http.Request) {
	s.siteAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Sites.Delete(r.Context(), c, id) })
}

func (s *Server) handleSitePages(w http.ResponseWriter, r *http.Request) {
	siteAnswer(s, w, r, http.StatusOK, func(c identity.Caller, id uuid.UUID) (map[string]any, error) {
		list, err := s.deps.Sites.Choices(r.Context(), c, id)
		return map[string]any{"pages": list}, err
	})
}

func (s *Server) handleSitePagesSet(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PageIDs []uuid.UUID `json:"page_ids"`
	}
	if decodeJSON(w, r, maxExchangeBody, &in) != nil {
		badBody(w)
		return
	}
	s.siteAction(w, r, func(c identity.Caller, id uuid.UUID) error {
		return s.deps.Sites.SetPages(r.Context(), c, id, in.PageIDs)
	})
}

func (s *Server) handleSitePreview(w http.ResponseWriter, r *http.Request) {
	siteAnswer(s, w, r, http.StatusOK, func(c identity.Caller, id uuid.UUID) (sites.Report, error) {
		return s.deps.Sites.Preview(r.Context(), c, id)
	})
}

func (s *Server) handleSitePublish(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm string `json:"confirm"`
	}
	if decodeJSON(w, r, maxExchangeBody, &in) != nil {
		badBody(w)
		return
	}
	siteAnswer(s, w, r, http.StatusAccepted, func(c identity.Caller, id uuid.UUID) (map[string]any, error) {
		job, err := s.deps.Sites.Publish(r.Context(), c, id, in.Confirm)
		return map[string]any{"job_id": job}, err
	})
}

func (s *Server) handleSiteBuilds(w http.ResponseWriter, r *http.Request) {
	siteAnswer(s, w, r, http.StatusOK, func(c identity.Caller, id uuid.UUID) (map[string]any, error) {
		list, err := s.deps.Sites.Builds(r.Context(), c, id)
		return map[string]any{"builds": list}, err
	})
}

func (s *Server) handleSiteRollback(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Number int `json:"number"`
	}
	if decodeJSON(w, r, maxExchangeBody, &in) != nil {
		badBody(w)
		return
	}
	s.siteAction(w, r, func(c identity.Caller, id uuid.UUID) error {
		return s.deps.Sites.Rollback(r.Context(), c, id, in.Number)
	})
}

func (s *Server) handleSiteWithdraw(w http.ResponseWriter, r *http.Request) {
	s.siteAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Sites.Withdraw(r.Context(), c, id) })
}

func (s *Server) handleSiteDownload(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(uploadDeadline))
	z := &zipResponse{ResponseWriter: w, name: "public-site.zip"}
	if err := s.deps.Sites.WriteZip(r.Context(), s.caller(r), id, z); err != nil {
		if z.started {
			s.deps.Log.Error("site download interrupted", "error", err, "request_id", RequestIDFrom(r.Context()))
			return
		}
		s.fail(w, r, err)
	}
}

func (s *Server) handlePublicState(w http.ResponseWriter, r *http.Request) {
	siteAnswer(s, w, r, http.StatusOK, func(c identity.Caller, id uuid.UUID) (sites.PublicState, error) {
		return s.deps.Sites.StateOf(r.Context(), c, id)
	})
}

func (s *Server) handleLink(w http.ResponseWriter, r *http.Request) {
	siteAnswer(s, w, r, http.StatusOK, func(c identity.Caller, id uuid.UUID) (sites.Link, error) {
		return s.deps.Sites.LinkOf(r.Context(), c, id)
	})
}

func (s *Server) handleLinkMake(w http.ResponseWriter, r *http.Request) {
	siteAnswer(s, w, r, http.StatusOK, func(c identity.Caller, id uuid.UUID) (sites.Link, error) {
		return s.deps.Sites.MakeLink(r.Context(), c, id)
	})
}

func (s *Server) handleLinkWithdraw(w http.ResponseWriter, r *http.Request) {
	s.siteAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Sites.WithdrawLink(r.Context(), c, id) })
}

func (s *Server) handleLinks(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Sites.Links(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": list})
}
