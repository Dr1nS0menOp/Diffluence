// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/mail"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
)

// excerptRunes keeps an excerpt short when an instance allows them at all.
const excerptRunes = 200

// sentences are all the words of a mail. They sit together, so a person's language
// can follow later without touching the code around them (wave F).
var sentences = map[string]string{
	TypeMention:          "%s mentioned you on %s",
	TypeTaskAssigned:     "%s gave you a task on %s",
	TypeTaskDue:          "A task on %[2]s is due",
	TypeShare:            "%s shared %s with you",
	TypeComment:          "%s commented on %s",
	TypeReply:            "%s replied to you on %s",
	TypeReaction:         "%s reacted on %s",
	TypePageChanged:      "%s changed %s",
	TypeAccessRequest:    "%s asks for access to %s",
	TypeAccessAnswer:     "%s answered your request for %s",
	TypeRule:             "%s sent a message about %s",
	TypeAccessRecovered:  "%s took access to %s as an administrator",
	TypeDraftExpiring:    "Your draft of %[2]s will be removed in two weeks",
	TypeInvitationAnswer: "%s answered your invitation from %s",
	TypeReviewAsked:      "You are asked to review %[2]s",
	TypeReviewAnswered:   "%s answered the review of %s",
	TypeReviewEnded:      "The review of %[2]s has ended",
	TypeReviewerRemoved:  "%s took a reviewer off the review of %s",
	TypePagesReleased:    "%s released pages you watch in the space of %s",
	TypeReleaseFinished:  "Your release from %[2]s has ended",
	TypeVersionApproved:  "A new version of %[2]s was approved",
	TypeApprovalRelease:  "The release of the approved %[2]s has ended",
	TypeReviewReminder:   "The review of %[2]s waits for your answer",
	TypeReadRequested:    "Please confirm you read %[2]s",
	TypePeriodicReview:   "%[2]s comes up for its periodic review",
	TypeTemplateProposed: "%s asks you to approve the template %s",
	TypeTemplateDecided:  "%s decided on the template %s",
	TypeTemplateSet:      "%s added %s and pages below it",
}

// templateDecisions are the words of TypeTemplateDecided, by what was decided (TPL-722). A
// reason is not in them: the author reads it in the template's history.
var templateDecisions = map[string]string{
	"approved":  "%s approved the template %s",
	"sent_back": "%s sent the template %s back, and said why",
	"withdrawn": "%s withdrew the template %s",
}

const (
	setMorePages = " and %d more pages of the same set"
	setReady     = "The pages from a template below %[2]s are ready"
)

const (
	someone        = "Someone"
	footerStop     = "Stop this kind of mail"
	lineStop       = "Stop watching"
	footerSettings = "Your notification settings"
	changeAdded    = "Added"
	changeRemoved  = "Removed"
	changeMore     = "And {count} more changes on the page"
	testSubject    = "Test mail from %s"
	testBody       = "If you can read this, mail from %s works."
	digestSubject  = "%d things happened in %s"
)

var mailPage = template.Must(template.New("mail").Parse(`<!DOCTYPE html>
<html><body>
{{range .Spaces}}<h2>{{.Name}}</h2><ul>{{range .Lines}}<li><a href="{{.Link}}">{{.Words}}</a>{{if .Excerpt}}<br>{{.Excerpt}}{{end}}{{if .ChangesHTML}}<div>{{.ChangesHTML}}</div>{{end}}{{if .Stop}}<br><a href="{{.Stop}}">{{.StopLabel}}</a>{{end}}</li>{{end}}</ul>{{end}}
<p><a href="{{.Settings}}">{{.SettingsLabel}}</a> | <a href="{{.Stop}}">{{.StopLabel}}</a></p>
</body></html>`))

type mailLine struct {
	Words     string
	Link      string
	Excerpt   string
	Stop      string // only when a watch brought this line
	StopLabel string

	Changes     string
	ChangesHTML template.HTML
}

type mailSpace struct {
	Name  string
	Lines []mailLine
}

type mailView struct {
	Spaces        []mailSpace
	Settings      string
	SettingsLabel string
	Stop          string
	StopLabel     string
}

func sentence(i Item) string {
	actor := someone
	if len(i.Actors) > 0 {
		actor = strings.Join(i.Actors, ", ")
	}
	words := sentences[i.Type]
	switch i.Type {
	case TypeTemplateDecided:
		decision, _ := i.Data["decision"].(string)
		if w, ok := templateDecisions[decision]; ok {
			words = w
		}
	case TypeTemplateSet:
		if ready, _ := i.Data["ready"].(bool); ready {
			words = setReady
		}
	}
	out := fmt.Sprintf(words, actor, i.PageTitle)
	// One mention for a whole set names how many more of its pages mention the person (TPL-721).
	if pages, _ := i.Data["pages"].([]any); i.Type == TypeMention && len(pages) > 1 {
		out += fmt.Sprintf(setMorePages, len(pages)-1)
	}
	return out
}

func (s *Service) link(i Item) string {
	if id, ok := i.Data["template_id"].(string); ok && (i.Type == TypeTemplateProposed || i.Type == TypeTemplateDecided) {
		return fmt.Sprintf("%s/templates/%s/settings?section=versions", s.options.PublicOrigin, id)
	}
	address := fmt.Sprintf("%s/pages/%s", s.options.PublicOrigin, i.PageID)
	if i.CommentID != nil {
		address += "#comment-" + i.CommentID.String()
	}
	return address
}

// mailRows reads what a mail may still say. Rows the person lost access to do not come
// back. A digest also leaves out what was read in the meantime.
func (s *Service) mailRows(ctx context.Context, j mail.Job) ([]stored, error) {
	rows, err := s.pool.Query(ctx, visibleRows+`AND n.id = ANY($2) AND ($3 = 'instant' OR n.read_at IS NULL)
		ORDER BY n.created_at DESC`, j.UserID, j.NotificationIDs, j.Kind)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanStored)
}

// excerpt is the start of a comment, through the same rule as the page: a thread kept for
// editors gives nothing to someone who may not edit (spec 064, DOC-604).
func (s *Service) excerpt(ctx context.Context, user uuid.UUID, i Item, allowed bool) (string, error) {
	if !allowed || i.CommentID == nil {
		return "", nil
	}
	editor, err := s.perms.CheckPage(ctx, s.pool, user, i.PageID, permissions.PageEdit, uuid.Nil)
	if err != nil {
		return "", err
	}
	var raw []byte
	err = s.pool.QueryRow(ctx, "SELECT body FROM comments WHERE id = $1 AND trash_item_id IS NULL AND (NOT editors_only OR $2)", *i.CommentID, editor.Allowed).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil // a removed comment simply has no excerpt
	}
	if err != nil {
		return "", err
	}
	n, err := document.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("notify: read comment: %v", err)
	}
	if err := s.fillNames(ctx, &n); err != nil {
		return "", err
	}
	words := []rune(strings.TrimSpace(document.BlockText(n)))
	return string(words[:min(len(words), excerptRunes)]), nil
}

// fillNames puts today's names into a document, as stored versions hold only ids.
func (s *Service) fillNames(ctx context.Context, n *document.Node) error {
	names, err := identity.DisplayNames(ctx, s.pool, append(document.MentionedUsers(*n), document.MentionedGroups(*n)...))
	if err != nil {
		return err
	}
	document.FillMentionNames(n, names)
	return nil
}

// changes shows what a page change was, when the instance allows content in mail. It
// runs after mailRows, so the person may still see this page at the moment of sending.
func (s *Service) changes(ctx context.Context, user uuid.UUID, i Item, allowed bool) (string, template.HTML, error) {
	newest, from := versionOf(i), i.firstVersion-1
	if i.Type == TypeVersionApproved {
		// Between two approved versions only: the versions in between were never approved,
		// and a reader may not see them (spec 064, DOC-604).
		from = previousOf(i)
	}
	if !allowed || (i.Type != TypePageChanged && i.Type != TypeVersionApproved) || from < 1 {
		return "", "", nil
	}
	reader, current, err := s.readsApprovedOnly(ctx, user, i.PageID)
	if err != nil || (reader && (i.Type != TypeVersionApproved || newest > current)) {
		// Whatever the row says, a reader of a controlled page gets no change but one between
		// approved versions: every mail goes through the rule of DOC-300, not each kind.
		return "", "", err
	}
	before, after, found, err := pages.VersionsForMail(ctx, s.pool, i.PageID, from, newest)
	if err != nil || !found {
		return "", "", err
	}
	if err := s.fillNames(ctx, &before); err != nil {
		return "", "", err
	}
	if err := s.fillNames(ctx, &after); err != nil {
		return "", "", err
	}
	blocks := document.Compare(before, after)
	labels := document.ChangeLabels{Added: changeAdded, Removed: changeRemoved, More: changeMore}
	//nolint:gosec // G203: ChangesHTML escapes every word that comes from a page.
	return document.ChangesText(blocks, labels), template.HTML(document.ChangesHTML(blocks, labels)), nil
}

// readsApprovedOnly says a person reads only the approved version of a controlled page, and
// which version that is.
func (s *Service) readsApprovedOnly(ctx context.Context, user, pageID uuid.UUID) (bool, int, error) {
	controlled, current, err := pages.Controlled(ctx, s.pool, pageID)
	if err != nil || !controlled {
		return false, current, err
	}
	working, err := s.perms.ReadsWorking(ctx, s.pool, user, pageID)
	return !working, current, err
}

func (s *Service) stopLink(user uuid.UUID, i Item) (string, error) {
	if i.Reason != ReasonWatching {
		return "", nil
	}
	token, err := s.StopWatchingToken(user, i.PageID, i.SpaceID)
	if err != nil {
		return "", err
	}
	return s.options.PublicOrigin + "/unsubscribe/" + token + "?what=watch", nil
}

func (s *Service) mailSpaces(ctx context.Context, user uuid.UUID, items []Item, excerpts bool) ([]mailSpace, error) {
	out := []mailSpace{}
	for _, i := range items {
		text, err := s.excerpt(ctx, user, i, excerpts)
		if err != nil {
			return nil, err
		}
		stop, err := s.stopLink(user, i)
		if err != nil {
			return nil, err
		}
		changes, changesHTML, err := s.changes(ctx, user, i, excerpts)
		if err != nil {
			return nil, err
		}
		line := mailLine{Words: sentence(i), Link: s.link(i), Excerpt: text, Stop: stop, StopLabel: lineStop,
			Changes: changes, ChangesHTML: changesHTML}
		at := slices.IndexFunc(out, func(m mailSpace) bool { return m.Name == i.SpaceName })
		if at < 0 {
			out = append(out, mailSpace{Name: i.SpaceName})
			at = len(out) - 1
		}
		out[at].Lines = append(out[at].Lines, line)
	}
	return out, nil
}

func plainText(v mailView) string {
	var b strings.Builder
	for _, space := range v.Spaces {
		b.WriteString(space.Name + "\n\n")
		for _, line := range space.Lines {
			b.WriteString("- " + line.Words + "\n  " + line.Link + "\n")
			if line.Excerpt != "" {
				b.WriteString("  " + line.Excerpt + "\n")
			}
			if line.Changes != "" {
				b.WriteString("  " + strings.ReplaceAll(strings.TrimSpace(line.Changes), "\n", "\n  ") + "\n")
			}
			if line.Stop != "" {
				b.WriteString("  " + line.StopLabel + ": " + line.Stop + "\n")
			}
		}
		b.WriteString("\n")
	}
	b.WriteString(v.SettingsLabel + ": " + v.Settings + "\n" + v.StopLabel + ": " + v.Stop + "\n")
	return b.String()
}

func (s *Service) address(ctx context.Context, user uuid.UUID) (string, error) {
	var email string
	var active bool
	err := s.pool.QueryRow(ctx, "SELECT email, status = 'active' FROM users WHERE id = $1", user).Scan(&email, &active)
	if err != nil {
		return "", err
	}
	if !active {
		return "", nil
	}
	return email, nil
}

func (s *Service) testMail(to string) mail.Rendered {
	name := s.options.InstanceName
	body := fmt.Sprintf(testBody, name)
	return mail.Rendered{Message: mail.Message{To: to, Subject: fmt.Sprintf(testSubject, name), Text: body,
		HTML: template.HTMLEscapeString(body), Reason: "test"}}
}

func subject(items []Item, instance string) string {
	if len(items) == 1 {
		return sentence(items[0])
	}
	return fmt.Sprintf(digestSubject, len(items), instance)
}

// Render writes a mail at the moment it is sent. Access is checked here again, so a
// person who lost access in the meantime gets nothing.
func (s *Service) Render(ctx context.Context, j mail.Job) (mail.Rendered, error) {
	to, err := s.address(ctx, j.UserID)
	if err != nil {
		return mail.Rendered{}, err
	}
	if to == "" {
		return mail.Rendered{Drop: true}, nil
	}
	if j.Kind == mail.KindTest {
		return s.testMail(to), nil
	}
	values, err := s.settings.Get(ctx)
	if err != nil {
		return mail.Rendered{}, err
	}
	choices, err := load(ctx, s.pool, j.UserID)
	if err != nil {
		return mail.Rendered{}, err
	}
	now := s.now()
	if !values.MailEnabled || choices.paused(now) {
		return mail.Rendered{Drop: true}, nil
	}
	if until := choices.quietUntil(now); !until.IsZero() {
		return mail.Rendered{Hold: until}, nil
	}
	rows, err := s.mailRows(ctx, j)
	if err != nil {
		return mail.Rendered{}, err
	}
	if len(rows) == 0 {
		return mail.Rendered{Drop: true}, nil
	}
	return s.compose(ctx, to, j, grouped(rows), values.MailExcerpts)
}

func (s *Service) compose(ctx context.Context, to string, j mail.Job, items []Item, excerpts bool) (mail.Rendered, error) {
	spaces, err := s.mailSpaces(ctx, j.UserID, items, excerpts)
	if err != nil {
		return mail.Rendered{}, err
	}
	scope := ScopeAll
	if j.Kind == mail.KindDigest {
		scope = ScopeDigest
	}
	token, err := s.UnsubscribeToken(j.UserID, scope)
	if err != nil {
		return mail.Rendered{}, err
	}
	view := mailView{Spaces: spaces, Settings: s.options.PublicOrigin + "/me/notifications", SettingsLabel: footerSettings,
		Stop: s.options.PublicOrigin + "/unsubscribe/" + token, StopLabel: footerStop}
	var html bytes.Buffer
	if err := mailPage.Execute(&html, view); err != nil {
		return mail.Rendered{}, err
	}
	return mail.Rendered{Message: mail.Message{To: to, Subject: subject(items, s.options.InstanceName), Text: plainText(view), HTML: html.String(),
		Reason: items[0].Reason, Unsubscribe: s.options.PublicOrigin + "/api/v1/mail/unsubscribe/" + token}}, nil
}

// SendDigests queues the digests that are due in this hour. A digest with nothing new
// is left out, and a person gets at most one per period.
func (s *Service) SendDigests(ctx context.Context) error {
	values, err := s.settings.Get(ctx)
	if err != nil || !values.MailEnabled {
		return err
	}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT user_id FROM notifications
		WHERE digested_at IS NULL AND read_at IS NULL AND hidden_at IS NULL`)
	if err != nil {
		return err
	}
	people, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	now := s.now()
	for _, person := range people {
		choices, err := load(ctx, s.pool, person)
		if err != nil {
			return err
		}
		if !choices.digestDue(now) || choices.paused(now) {
			continue
		}
		if err := s.queueDigest(ctx, person); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) queueDigest(ctx context.Context, person uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE notifications SET digested_at = now()
			WHERE user_id = $1 AND digested_at IS NULL AND read_at IS NULL AND hidden_at IS NULL RETURNING id`, person)
		if err != nil {
			return err
		}
		list, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil || len(list) == 0 {
			return err
		}
		return mail.EnqueueDigest(ctx, tx, person, list)
	})
}
