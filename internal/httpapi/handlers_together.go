// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/comments"
	"github.com/diffluence/diffluence/internal/feed"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/notify"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/sharing"
	"github.com/diffluence/diffluence/internal/trash"
)

func (s *Server) registerTogetherRoutes() {
	r := s.router
	me := Authenticated()
	pagesPath := "/api/v1/pages/{id}"
	commentPath := "/api/v1/comments/{id}"

	r.HandleFunc("GET "+pagesPath+"/comments", Object("page.view"), s.handleComments)
	r.HandleFunc("POST "+pagesPath+"/comments", Object("comment.add"), s.handleCommentCreate)
	r.HandleFunc("PUT "+pagesPath+"/comments/seen", Object("page.view"), s.handleCommentsSeen)
	r.HandleFunc("PATCH "+pagesPath+"/comments-closed", Object("page.edit"), s.handleCommentsClosed)
	r.HandleFunc("PATCH "+commentPath, Object("comment.add"), s.handleCommentEdit)
	r.HandleFunc("DELETE "+commentPath, Object("comment.delete"), s.handleCommentDelete)
	// Personal: only the person who deleted the comment, for a short time.
	r.HandleFunc("POST "+commentPath+"/undo-delete", me, s.handleCommentUndo)
	r.HandleFunc("POST "+commentPath+"/resolve", Object("page.edit"), s.handleCommentResolve)
	r.HandleFunc("POST "+commentPath+"/reopen", Object("page.edit"), s.handleCommentReopen)

	r.HandleFunc("GET "+pagesPath+"/reactions", Object("page.view"), s.handleReactions(comments.TargetPage))
	r.HandleFunc("GET "+commentPath+"/reactions", Object("page.view"), s.handleReactions(comments.TargetComment))
	r.HandleFunc("PUT "+pagesPath+"/reactions/{emoji}", Object("page.view"), s.handleReaction(comments.TargetPage, true))
	r.HandleFunc("DELETE "+pagesPath+"/reactions/{emoji}", Object("page.view"), s.handleReaction(comments.TargetPage, false))
	r.HandleFunc("PUT "+commentPath+"/reactions/{emoji}", Object("page.view"), s.handleReaction(comments.TargetComment, true))
	r.HandleFunc("DELETE "+commentPath+"/reactions/{emoji}", Object("page.view"), s.handleReaction(comments.TargetComment, false))

	r.HandleFunc("GET "+pagesPath+"/mention-candidates", Object("page.view"), s.handleCandidates)
	r.HandleFunc("GET "+pagesPath+"/access", Object("page.view"), s.handleAccess)
	r.HandleFunc("POST "+pagesPath+"/share", Object("page.view"), s.handleShare)
	// The module decides: someone who cannot see the page gets 404, unless the space
	// chose to allow requests.
	r.HandleFunc("POST "+pagesPath+"/access-requests", me, s.handleAccessAsk)
	r.HandleFunc("GET "+pagesPath+"/access-requests", Object("restriction.manage"), s.handleAccessOpen)
	r.HandleFunc("GET /api/v1/access-requests/{id}", Object("restriction.manage"), s.handleAccessDescribe)
	r.HandleFunc("POST /api/v1/access-requests/{id}/allow", Object("restriction.manage"), s.handleAccessAnswer(true))
	r.HandleFunc("POST /api/v1/access-requests/{id}/refuse", Object("restriction.manage"), s.handleAccessAnswer(false))

	r.HandleFunc("GET "+pagesPath+"/watch", Object("page.view"), s.handleWatchState)
	r.HandleFunc("PUT "+pagesPath+"/watch", Object("page.view"), s.handleWatchPage(true))
	r.HandleFunc("DELETE "+pagesPath+"/watch", Object("page.view"), s.handleWatchPage(false))
	r.HandleFunc("GET /api/v1/spaces/{id}/watch", Object("space.view"), s.handleSpaceWatchState)
	r.HandleFunc("PUT /api/v1/spaces/{id}/watch", Object("space.view"), s.handleWatchSpace(true))
	r.HandleFunc("DELETE /api/v1/spaces/{id}/watch", Object("space.view"), s.handleWatchSpace(false))
	r.HandleFunc("GET "+pagesPath+"/watchers", Object("space.administer"), s.handleWatchers)
	r.HandleFunc("PUT "+pagesPath+"/watchers/{user}", Object("space.administer"), s.handleWatcher(true))
	r.HandleFunc("DELETE "+pagesPath+"/watchers/{user}", Object("space.administer"), s.handleWatcher(false))
	r.HandleFunc("GET /api/v1/me/watches", me, s.handleMyWatches)

	r.HandleFunc("GET /api/v1/me/tasks", me, s.handleMyTasks)
	r.HandleFunc("POST /api/v1/tasks/{id}/done", Object("page.edit"), s.handleTaskDone)

	r.HandleFunc("GET /api/v1/me/notifications", me, s.handleNotifications)
	r.HandleFunc("GET /api/v1/me/notifications/count", me, s.handleNotificationCount)
	r.HandleFunc("PUT /api/v1/me/notifications/{id}/read", me, s.handleNotificationRead(true))
	r.HandleFunc("PUT /api/v1/me/notifications/{id}/unread", me, s.handleNotificationRead(false))
	r.HandleFunc("POST /api/v1/me/notifications/read-all", me, s.handleNotificationsReadAll)
	r.HandleFunc("GET /api/v1/me/notification-settings", me, s.handleNotificationSettings)
	r.HandleFunc("PUT /api/v1/me/notification-settings", me, s.handleNotificationSettingsSet)

	// A list: it only holds what the caller may see, which the module decides.
	r.HandleFunc("GET /api/v1/feed", me, s.handleFeed)
	r.HandleFunc("GET /api/v1/me/following", me, s.handleFollowing)
	r.HandleFunc("PUT /api/v1/users/{id}/follow", me, s.handleFollow(true))
	r.HandleFunc("DELETE /api/v1/users/{id}/follow", me, s.handleFollow(false))

	admin := Permission(identity.PermAdministrator)
	r.HandleFunc("GET /api/v1/admin/mail", admin, s.handleMailQueue)
	r.HandleFunc("POST /api/v1/admin/mail/test", admin, s.handleMailTest)
	// RFC 8058: the mail client sends no session, so the sealed token is the proof.
	// It can only reduce mail, and a bad token does nothing.
	r.HandleFunc("POST /api/v1/mail/unsubscribe/{token}", Public, s.handleUnsubscribe)
}

func (s *Server) handleComments(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	q := r.URL.Query()
	out, err := s.deps.Comments.List(r.Context(), s.caller(r), id, comments.ListOptions{
		Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset"), Order: q.Get("order")})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCommentCreate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Body        json.RawMessage  `json:"body"`
		ParentID    *uuid.UUID       `json:"parent_id"`
		Anchor      *comments.Anchor `json:"anchor"`
		EditorsOnly bool             `json:"editors_only"`
	}
	if decodeJSON(w, r, maxDocumentBody, &in) != nil {
		badBody(w)
		return
	}
	c, err := s.deps.Comments.Create(r.Context(), s.caller(r), id, comments.NewComment{Body: in.Body, ParentID: in.ParentID, Anchor: in.Anchor, EditorsOnly: in.EditorsOnly})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleCommentEdit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Body json.RawMessage `json:"body"`
	}
	if decodeJSON(w, r, maxDocumentBody, &in) != nil {
		badBody(w)
		return
	}
	c, err := s.deps.Comments.Edit(r.Context(), s.caller(r), id, in.Body)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleCommentDelete(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Comments.Delete(r.Context(), c, id) })
}

func (s *Server) handleCommentUndo(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error {
		return s.deps.Trash.UndoDelete(r.Context(), c, trash.TypeComment, id)
	})
}

func (s *Server) handleCommentResolve(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Comments.Resolve(r.Context(), c, id) })
}

func (s *Server) handleCommentReopen(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Comments.Reopen(r.Context(), c, id) })
}

func (s *Server) handleCommentsSeen(w http.ResponseWriter, r *http.Request) {
	s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Comments.MarkSeen(r.Context(), c, id) })
}

func (s *Server) handleCommentsClosed(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Closed bool `json:"closed"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Comments.Closed(r.Context(), s.caller(r), id, in.Closed); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReactions(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		c := s.caller(r)
		counts, err := s.deps.Comments.Reactions(r.Context(), c, kind, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		who, err := s.deps.Comments.Reactors(r.Context(), c, kind, id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"reactions": counts, "who": who})
	}
}

func (s *Server) handleReaction(kind string, on bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error {
			return s.deps.Comments.SetReaction(r.Context(), c, kind, id, r.PathValue("emoji"), on)
		})
	}
}

func (s *Server) handleCandidates(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	q := r.URL.Query()
	list, err := s.deps.Sharing.Candidates(r.Context(), s.caller(r), id, q.Get("q"), q.Get("for") == "share")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": list})
}

func (s *Server) handleAccess(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.deps.Sharing.PageAccess(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		People  []permissions.Subject `json:"people"`
		Message string                `json:"message"`
		Notify  bool                  `json:"notify"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	out, err := s.deps.Sharing.Share(r.Context(), s.caller(r), id, sharing.ShareInput{People: in.People, Message: in.Message, Notify: in.Notify})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAccessAsk(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Kind string `json:"kind"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Sharing.Ask(r.Context(), s.caller(r), id, in.Kind); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAccessOpen(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Sharing.Open(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": list})
}

func (s *Server) handleAccessDescribe(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.deps.Sharing.Describe(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAccessAnswer(allow bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		var in struct {
			Note string `json:"note"`
		}
		if decodeJSON(w, r, maxSmallBody, &in) != nil {
			badBody(w)
			return
		}
		if err := s.deps.Sharing.Answer(r.Context(), s.caller(r), id, allow, in.Note); err != nil {
			s.fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) handleWatchState(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	out, err := s.deps.Watches.PageState(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleWatchPage(on bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error {
			return s.deps.Watches.WatchPage(r.Context(), c, id, r.URL.Query().Get("scope"), on)
		})
	}
}

func (s *Server) handleSpaceWatchState(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	on, err := s.deps.Watches.SpaceWatched(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"watching": on})
}

func (s *Server) handleWatchSpace(on bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error {
			return s.deps.Watches.WatchSpace(r.Context(), c, id, on)
		})
	}
}

func (s *Server) handleWatchers(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	list, err := s.deps.Watches.Watchers(r.Context(), s.caller(r), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"watchers": list})
}

func (s *Server) handleWatcher(on bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := pathID(w, r, "user")
		if !ok {
			return
		}
		s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error {
			return s.deps.Watches.SetWatcher(r.Context(), c, id, user, on)
		})
	}
}

func (s *Server) handleMyWatches(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Watches.Mine(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"watches": list})
}

func (s *Server) handleMyTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	list, err := s.deps.Tasks.Mine(r.Context(), s.caller(r), q.Get("tab"), q.Get("status"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": list})
}

func (s *Server) handleTaskDone(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var in struct {
		Done bool `json:"done"`
	}
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if _, err := s.deps.Pages.SetTaskDone(r.Context(), s.caller(r), id, in.Done); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func queryTime(w http.ResponseWriter, r *http.Request, key string) (time.Time, bool) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return time.Time{}, true
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "That is not a time.")
		return t, false
	}
	return t, true
}

func querySpace(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := optionalID(r.URL.Query().Get("space_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "That is not a space.")
		return id, false
	}
	return id, true
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	before, ok := queryTime(w, r, "before")
	if !ok {
		return
	}
	space, ok := querySpace(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	unreadOnly, _ := strconv.ParseBool(q.Get("unread"))
	list, err := s.deps.Notify.List(r.Context(), s.caller(r), notify.Query{UnreadOnly: unreadOnly, Type: q.Get("type"),
		SpaceID: space, Before: before, Limit: queryInt(r, "limit")})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": list})
}

func (s *Server) handleNotificationCount(w http.ResponseWriter, r *http.Request) {
	n, err := s.deps.Notify.Count(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unread": n})
}

func (s *Server) handleNotificationRead(read bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error {
			return s.deps.Notify.SetRead(r.Context(), c, id, read)
		})
	}
}

func (s *Server) handleNotificationsReadAll(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Notify.ReadAll(r.Context(), s.caller(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleNotificationSettings(w http.ResponseWriter, r *http.Request) {
	out, err := s.deps.Notify.Settings(r.Context(), s.caller(r).UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleNotificationSettingsSet(w http.ResponseWriter, r *http.Request) {
	var in notify.Settings
	if decodeJSON(w, r, maxSmallBody, &in) != nil {
		badBody(w)
		return
	}
	if err := s.deps.Notify.SetSettings(r.Context(), s.caller(r).UserID, in); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	before, ok := queryTime(w, r, "before")
	if !ok {
		return
	}
	space, ok := querySpace(w, r)
	if !ok {
		return
	}
	list, err := s.deps.Feed.List(r.Context(), s.caller(r), feed.Query{Tab: r.URL.Query().Get("tab"), SpaceID: space,
		Before: before, Limit: queryInt(r, "limit")})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) handleFollowing(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Watches.Following(r.Context(), s.caller(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"following": list})
}

func (s *Server) handleFollow(on bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.spaceAction(w, r, func(c identity.Caller, id uuid.UUID) error { return s.deps.Watches.Follow(r.Context(), c, id, on) })
	}
}

func (s *Server) handleMailQueue(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Mail.List(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"configured": s.deps.Mail.Configured(), "queue": list})
}

func (s *Server) handleMailTest(w http.ResponseWriter, r *http.Request) {
	if !s.deps.Mail.Configured() {
		s.fail(w, r, apperr.Conflict("mail_off", "No mail server is set."))
		return
	}
	if err := s.deps.Mail.EnqueueTest(r.Context(), s.caller(r).UserID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	// RFC 8058 forbids a redirect. A bad token gets the same plain answer as a good one,
	// so the route tells nothing about who exists.
	if err := s.deps.Notify.Unsubscribe(r.Context(), r.PathValue("token")); err != nil {
		if _, expected := apperr.As(err); !expected {
			s.internalError(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"done": true})
}
