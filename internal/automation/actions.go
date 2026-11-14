// SPDX-License-Identifier: Apache-2.0

package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/comments"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/notify"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/templates"
)

// values are the placeholders of a step, read as the account sees the page. Nothing
// here is an expression: a person picks them from a list (D13).
func (s *Service) values(ctx context.Context, r *run, a Action, page *uuid.UUID) (map[string]string, error) {
	out := map[string]string{"rule.name": r.rule.Name}
	if page == nil {
		return out, nil
	}
	d, err := s.Pages.Get(ctx, r.caller, *page)
	if err != nil {
		return nil, err
	}
	out["page.title"], out["found.title"] = d.Title, d.Title
	out["page.url"] = s.Origin + "/pages/" + page.String()
	out["page.author"], out["page.owner"] = d.CreatedByName, d.OwnerName
	if a.WithText {
		if out["page.text"], err = plainText(d.Document); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func comment(text string) (json.RawMessage, error) {
	return json.Marshal(map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph",
		"content": []any{map[string]any{"type": "text", "text": text}}}}})
}

// act does one action, or says what it would do in a test run. The answer is a note
// for the log: on a real run a note means the step was skipped. Every change goes
// through the method a person would use, with the account as caller (D10).
func (s *Service) act(ctx context.Context, r *run, a Action, page *uuid.UUID) (string, error) {
	values, err := s.values(ctx, r, a, page)
	if err != nil {
		return "", err
	}
	if r.dry {
		return s.preview(ctx, r, a, page)
	}
	switch a.Type {
	case ActWebhook, ActChat:
		return "", s.callOut(ctx, a, values)
	case ActNotify, ActMail:
		return s.message(ctx, r, a, page, values)
	case ActTemplate:
		return "", s.fromTemplate(ctx, r, a, values)
	}
	if page == nil {
		return "no_page", nil
	}
	return s.onPage(ctx, r, a, *page, values)
}

func (s *Service) onPage(ctx context.Context, r *run, a Action, page uuid.UUID, values map[string]string) (string, error) {
	c := r.caller
	var err error
	switch a.Type {
	case ActCopy:
		_, err = s.Pages.Copy(ctx, c, page, pages.CopyInput{SpaceID: a.SpaceID, ParentID: a.ParentID})
	case ActMove:
		_, err = s.Pages.Move(ctx, c, page, pages.MoveInput{SpaceID: a.SpaceID, ParentID: a.ParentID})
	case ActArchive:
		err = s.Pages.Archive(ctx, c, page, false)
	case ActDelete:
		err = s.Pages.Delete(ctx, c, page)
	case ActRestrict:
		err = s.Perms.SetRestrictions(ctx, c, page, a.Restricted, []permissions.Subject{{Type: permissions.SubjectGroup, ID: *a.TargetID}})
	case ActAddLabel:
		_, err = s.Pages.AddLabel(ctx, c, page, expand(a.Label, values))
	case ActDropLabel:
		_, err = s.Pages.RemoveLabel(ctx, c, page, strings.ToLower(expand(a.Label, values)))
	case ActStatus:
		_, err = s.Pages.SetStage(ctx, c, page, expand(a.Text, values), a.Color)
	case ActOwner:
		_, err = s.Pages.SetOwner(ctx, c, page, *a.TargetID)
	case ActComment:
		err = s.addComment(ctx, r, page, expand(a.Text, values))
	case ActWatch, ActUnwatch:
		return s.watchers(ctx, r, a, page)
	case ActTask:
		return s.assignTask(ctx, r, a, page, values)
	default:
		return "unknown_action", nil
	}
	return "", err
}

func (s *Service) addComment(ctx context.Context, r *run, page uuid.UUID, text string) error {
	body, err := comment(text)
	if err != nil {
		return err
	}
	_, err = s.Comments.Create(ctx, r.caller, page, comments.NewComment{Body: body})
	return err
}

func (s *Service) fromTemplate(ctx context.Context, r *run, a Action, values map[string]string) error {
	space := r.rule.SpaceID
	if a.SpaceID != nil {
		space = a.SpaceID
	}
	if space == nil {
		return apperr.Invalid("Choose the space of the new page.")
	}
	_, err := s.Templates.Use(ctx, r.caller, *a.TargetID, templates.UseInput{SpaceID: *space, ParentID: a.ParentID, Title: expand(a.Title, values)})
	return err
}

// recipients resolves who a step is for.
func (s *Service) recipients(ctx context.Context, a Action, page *uuid.UUID) ([]uuid.UUID, error) {
	switch a.To {
	case ToUser:
		return []uuid.UUID{*a.TargetID}, nil
	case ToGroup:
		rows, err := s.Pool.Query(ctx, "SELECT user_id FROM group_members WHERE group_id = $1", *a.TargetID)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	}
	if page == nil {
		return nil, nil
	}
	var owner, author uuid.UUID
	if err := s.Pool.QueryRow(ctx, "SELECT COALESCE(owner_id, created_by), created_by FROM pages WHERE id = $1", *page).Scan(&owner, &author); err != nil {
		return nil, err
	}
	if a.To == ToAuthor {
		return []uuid.UUID{author}, nil
	}
	return []uuid.UUID{owner}, nil
}

func (s *Service) watchers(ctx context.Context, r *run, a Action, page uuid.UUID) (string, error) {
	people, err := s.recipients(ctx, a, &page)
	if err != nil {
		return "", err
	}
	for _, person := range people {
		if err := s.Watches.SetWatcher(ctx, r.caller, page, person, a.Type == ActWatch); err != nil {
			return "", err
		}
	}
	return "", nil
}

// mayReceive holds a message back from anyone who cannot see every page it refers
// to (GAP-016): the page of the step and the page that started the run.
func (s *Service) mayReceive(ctx context.Context, r *run, person uuid.UUID, page *uuid.UUID) (bool, error) {
	for _, id := range []*uuid.UUID{page, r.context.PageID} {
		if id == nil {
			continue
		}
		d, err := s.Perms.CheckPage(ctx, s.Pool, person, *id, permissions.ActionPageView, uuid.Nil)
		if err != nil || !d.Allowed {
			return false, err
		}
	}
	return true, nil
}

// message sends a notification or a mail. E-mail only ever goes to people on the
// instance, through the mail queue, and a message always hangs on a page.
func (s *Service) message(ctx context.Context, r *run, a Action, page *uuid.UUID, values map[string]string) (string, error) {
	if page == nil {
		return "no_page", nil
	}
	people, err := s.recipients(ctx, a, page)
	if err != nil {
		return "", err
	}
	var space uuid.UUID
	if err := s.Pool.QueryRow(ctx, "SELECT space_id FROM pages WHERE id = $1", *page).Scan(&space); err != nil {
		return "", err
	}
	sent := 0
	for _, person := range people {
		allowed, err := s.mayReceive(ctx, r, person, page)
		if err != nil {
			return "", err
		}
		if !allowed {
			continue
		}
		row := notify.Row{UserID: person, Type: notify.TypeRule, Reason: "rule", ActorID: r.rule.AccountID, PageID: *page, SpaceID: space,
			Event: uuid.NewSHA1(r.id, []byte(fmt.Sprintf("%s:%s:%s", a.Type, page, person))), ForceMail: a.Type == ActMail,
			Data: map[string]any{"rule": r.rule.Name, "text": expand(a.Text, values)}}
		if err := s.Notify.Deliver(ctx, s.Pool, row); err != nil {
			return "", err
		}
		sent++
	}
	if sent == 0 {
		return "nobody_may_hear", nil
	}
	return "", nil
}

// assignTask adds a task to the page, through publishing like a person would.
func (s *Service) assignTask(ctx context.Context, r *run, a Action, page uuid.UUID, values map[string]string) (string, error) {
	people, err := s.recipients(ctx, a, &page)
	if err != nil || len(people) == 0 {
		return "nobody_may_hear", err
	}
	d, err := s.Pages.Get(ctx, r.caller, page)
	if err != nil {
		return "", err
	}
	var doc map[string]any
	if err := json.Unmarshal(d.Document, &doc); err != nil {
		return "", err
	}
	content, _ := doc["content"].([]any)
	mention := map[string]any{"type": "mention", "attrs": map[string]any{"id": people[0].String(), "kind": document.MentionUser}}
	item := map[string]any{"type": "taskItem", "attrs": map[string]any{"checked": false},
		"content": []any{map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": expand(a.Text, values) + " "}, mention}}}}
	doc["content"] = append(content, map[string]any{"type": "taskList", "content": []any{item}})
	raw, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	_, err = s.Pages.Publish(ctx, r.caller, page, pages.PublishInput{Title: d.Title, Document: raw, BaseVersion: d.CurrentVersion, Summary: "Task from a rule"})
	return "", err
}

// callOut sends through the one outward client. The host must be on the list when the
// call is made, not only when the rule was saved.
func (s *Service) callOut(ctx context.Context, a Action, values map[string]string) error {
	body := a.Body
	if a.Type == ActChat {
		raw, err := json.Marshal(map[string]string{"text": expand(a.Text, values)})
		if err != nil {
			return err
		}
		body = string(raw)
	} else {
		var err error
		if body, err = expandJSON(body, values); err != nil {
			return apperr.Invalid("The body of this call is not JSON.")
		}
	}
	answer, err := s.Outward.Post(ctx, a.Address, []byte(body), nil)
	if err != nil {
		return err
	}
	if answer.Status < 200 || answer.Status >= 300 {
		return apperr.Conflict("call_refused", "The other server said no.")
	}
	return nil
}

// preview says what a step would do, using the same permission rule the real step
// meets, and changes nothing.
func (s *Service) preview(ctx context.Context, r *run, a Action, page *uuid.UUID) (string, error) {
	if page == nil {
		return "would_" + a.Type, nil
	}
	action := previewPermission(a.Type)
	if action == "" {
		return "would_" + a.Type, nil
	}
	d, err := s.Perms.CheckPage(ctx, s.Pool, r.rule.AccountID, *page, action, uuid.Nil)
	if err != nil {
		return "", err
	}
	if !d.Allowed {
		return "account_may_not", nil
	}
	return "would_" + a.Type, nil
}

func previewPermission(kind string) string {
	switch kind {
	case ActMove, ActAddLabel, ActDropLabel, ActStatus, ActOwner, ActTask:
		return permissions.PageEdit
	case ActArchive:
		return permissions.PageArchive
	case ActDelete:
		return permissions.ActionPageDelete
	case ActRestrict:
		return permissions.RestrictionManage
	case ActComment:
		return permissions.CommentAdd
	}
	return ""
}
