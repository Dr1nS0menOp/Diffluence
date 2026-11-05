// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/search"
	"github.com/diffluence/diffluence/internal/spaces"
)

// quickGroupSize is how many rows each group of quick search shows.
const quickGroupSize = 5

func (s *Server) registerReadingRoutes() {
	r := s.router
	me := Authenticated()

	r.HandleFunc("GET /api/v1/me/preferences", me, s.handlePreferences)
	r.HandleFunc("PUT /api/v1/me/preferences", me, s.handlePreferencesSet)
	r.HandleFunc("PUT /api/v1/me/favourites/order", me, s.handleFavouritesOrder)
	r.HandleFunc("GET /api/v1/me/spaces", me, s.handleMySpaces)
	// Lists: both only hold what the caller may see, which the module decides.
	r.HandleFunc("GET /api/v1/space-directory", me, s.handleSpaceDirectory)
	r.HandleFunc("GET /api/v1/search/quick", me, s.handleQuickSearch)
	r.HandleFunc("GET /api/v1/space-key-check", Permission(identity.PermCreateSpace), s.handleKeyCheck)

	r.HandleFunc("PUT /api/v1/spaces/{id}/favourite", Object("space.view"), s.handleSpaceFavouriteOn)
	r.HandleFunc("DELETE /api/v1/spaces/{id}/favourite", Object("space.view"), s.handleSpaceFavouriteOff)
	r.HandleFunc("GET /api/v1/spaces/{id}/overview", Object("space.view"), s.handleSpaceOverview)
	r.HandleFunc("GET /api/v1/spaces/{id}/shortcuts", Object("space.view"), s.handleShortcuts)
	r.HandleFunc("PUT /api/v1/spaces/{id}/shortcuts", Object("space.administer"), s.handleShortcutsSet)

	r.HandleFunc("PATCH /api/v1/pages/{id}/look", Object("page.edit"), s.handlePageLook)
	r.HandleFunc("GET /api/v1/pages/{id}/details", Object("page.view"), s.handlePageDetails)
	r.HandleFunc("GET /api/v1/pages/{id}/summary", Object("page.view"), s.handlePageSummary)
	r.HandleFunc("GET /api/v1/pages/{id}/impact", Object("page.delete"), s.handlePageImpact)
	r.HandleFunc("POST /api/v1/admin/example-space", Permission(identity.PermAdministrator), s.handleExampleSpace)
	// Personal: only the person who deleted the page can use this, for a short time.
	r.HandleFunc("POST /api/v1/pages/{id}/undo-delete", me, s.handleUndoDelete)
}

func (s *Server) handleExampleSpace(w http.ResponseWriter, r *http.Request) {
	sp, err := s.deps.Example.Create(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"key": sp.Key})
}

func (s *Server) handlePreferences(w http.ResponseWriter, r *http.Request) {
	p, err := s.deps.Identity.Preferences(r.Context(), s.caller(r).UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handlePreferencesSet(w http.ResponseWriter, r *http.Request) {
	var in identity.Preferences
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Identity.SetPreferences(r.Context(), s.caller(r).UserID, in); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

func (s *Server) handleFavouritesOrder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PageIDs []uuid.UUID `json:"page_ids"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Pages.OrderFavourites(r.Context(), s.caller(r), in.PageIDs); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMySpaces(w http.ResponseWriter, r *http.Request) {
	mine, err := s.deps.Spaces.Mine(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mine)
}

func (s *Server) handleSpaceDirectory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, total, err := s.deps.Spaces.Directory(r.Context(), s.caller(r), spaces.DirectoryQuery{
		Text: q.Get("q"), Kind: q.Get("kind"), Sort: q.Get("sort"),
		Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset"),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"spaces": list, "total": total})
}

func (s *Server) handleKeyCheck(w http.ResponseWriter, r *http.Request) {
	free, err := s.deps.Spaces.KeyFree(r.Context(), r.URL.Query().Get("key"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"free": free})
}

func (s *Server) setSpaceFavourite(w http.ResponseWriter, r *http.Request, on bool) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Spaces.SetFavourite(r.Context(), c, id, on) })
}

func (s *Server) handleSpaceFavouriteOn(w http.ResponseWriter, r *http.Request) {
	s.setSpaceFavourite(w, r, true)
}

func (s *Server) handleSpaceFavouriteOff(w http.ResponseWriter, r *http.Request) {
	s.setSpaceFavourite(w, r, false)
}

func (s *Server) handleSpaceOverview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	o, err := s.deps.Spaces.Overview(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) handleShortcuts(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Spaces.Shortcuts(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shortcuts": list})
}

func (s *Server) handleShortcutsSet(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Shortcuts []spaces.ShortcutInput `json:"shortcuts"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Spaces.SetShortcuts(r.Context(), s.caller(r), id, in.Shortcuts); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePageLook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in pages.Look
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	p, err := s.deps.Pages.SetLook(r.Context(), s.caller(r), id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handlePageDetails(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	d, err := s.deps.Pages.PageDetails(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handlePageSummary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	summary, err := s.deps.Pages.PageSummary(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) handlePageImpact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	impact, err := s.deps.Pages.DeleteImpact(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, impact)
}

func (s *Server) handleUndoDelete(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Trash.UndoDelete(r.Context(), c, id) })
}

// quickPerson leaves the e-mail address out: quick search only needs a name.
type quickPerson struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type quickResults struct {
	Pages  []search.Result     `json:"pages"`
	Spaces []spaces.Space      `json:"spaces"`
	Files  []attachments.Found `json:"files"`
	People []quickPerson       `json:"people"`
}

// people finds people for quick search. Without "view profiles" the group stays
// empty: the server does not send it at all, whatever the client asks for.
func (s *Server) quickPeople(r *http.Request, c identity.Caller, text string) ([]quickPerson, error) {
	out := []quickPerson{}
	allowed, err := s.deps.Identity.HasGlobalPermission(r.Context(), c.UserID, identity.PermViewProfiles)
	if err != nil || !allowed {
		return out, err
	}
	entries, err := s.deps.Identity.Directory(r.Context(), text)
	if err != nil {
		return out, err
	}
	for _, e := range entries {
		if e.Type == "user" && len(out) < quickGroupSize {
			out = append(out, quickPerson{ID: e.ID, Name: e.Name})
		}
	}
	return out, nil
}

func (s *Server) handleQuickSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	spaceID, err := optionalID(q.Get("space_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "A search filter is not valid.")
		return
	}
	text, c := q.Get("q"), s.caller(r)
	out := quickResults{Pages: []search.Result{}, Spaces: []spaces.Space{}, Files: []attachments.Found{}, People: []quickPerson{}}
	if text == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	out.Pages, err = s.deps.Search.Search(r.Context(), c.UserID, search.Query{Text: text, SpaceID: spaceID, Limit: quickGroupSize, ActiveSpacesOnly: true})
	if err == nil {
		out.Files, err = s.deps.Attachments.Find(r.Context(), c, text, spaceID, quickGroupSize)
	}
	if err == nil && spaceID == uuid.Nil {
		out.Spaces, err = s.deps.Spaces.Find(r.Context(), c, text, quickGroupSize)
	}
	if err == nil && spaceID == uuid.Nil {
		out.People, err = s.quickPeople(r, c, text)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
