// SPDX-License-Identifier: Apache-2.0

// Package collab is the side of editing together that stays in the application (spec
// 015, part 5). The service moves changes between browsers and has no business logic:
// it asks here who may connect, and every state it stores passes the gate here.
package collab

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
)

const recheckTimeout = 5 * time.Second

// Options say where the service is. An empty URL means it is not installed, and every
// page keeps its private drafts (ARC-120).
type Options struct {
	URL   string
	Token string
}

// Service answers the collab service.
type Service struct {
	pool     *pgxpool.Pool
	perms    *permissions.Service
	identity *identity.Service
	options  Options
	client   *http.Client
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, id *identity.Service, o Options) *Service {
	return &Service{pool: pool, perms: perms, identity: id, options: o, client: &http.Client{Timeout: recheckTimeout}}
}

// Installed says whether editing together is on for this instance.
func (s *Service) Installed() bool { return s.options.URL != "" && s.options.Token != "" }

// URL is the address of the service, for the proxy.
func (s *Service) URL() string { return s.options.URL }

// TrustsToken compares in constant time: this token opens every document.
func (s *Service) TrustsToken(token string) bool {
	return s.Installed() && subtle.ConstantTimeCompare([]byte(token), []byte(s.options.Token)) == 1
}

// Person is who the service may let in.
type Person struct {
	Allowed bool      `json:"allowed"`
	UserID  uuid.UUID `json:"user_id"`
	Name    string    `json:"name"`
}

// Authorize decides about a session and a page. Edit is needed: a person with view
// rights only cannot connect. Every doubt is a no.
func (s *Service) Authorize(ctx context.Context, p identity.Principal, pageID uuid.UUID) (Person, error) {
	d, err := s.perms.CheckPage(ctx, s.pool, p.User.ID, pageID, permissions.PageEdit, uuid.Nil)
	if err != nil || !d.Allowed {
		return Person{}, err
	}
	return Person{Allowed: true, UserID: p.User.ID, Name: p.User.DisplayName}, nil
}

// Info is what the editor needs to know before it opens.
type Info struct {
	Enabled bool      `json:"enabled"`
	UserID  uuid.UUID `json:"user_id"`
	Name    string    `json:"name"`
}

// For says whether this person edits this page together with others.
func (s *Service) For(ctx context.Context, p identity.Principal, pageID uuid.UUID) (Info, error) {
	if err := s.perms.RequirePage(ctx, s.pool, p.User.ID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return Info{}, err
	}
	if !s.Installed() {
		return Info{}, nil
	}
	person, err := s.Authorize(ctx, p, pageID)
	return Info{Enabled: person.Allowed, UserID: person.UserID, Name: person.Name}, err
}

// State reads the shared document. Nil means none exists yet.
func (s *Service) State(ctx context.Context, pageID uuid.UUID) ([]byte, error) {
	var state []byte
	err := s.pool.QueryRow(ctx, "SELECT state FROM collab_documents WHERE page_id = $1", pageID).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return state, err
}

// Stored is what the service sends after people typed.
type Stored struct {
	State    []byte          `json:"state"`
	Title    string          `json:"title"`
	Document json.RawMessage `json:"document"`
	Authors  []uuid.UUID     `json:"authors"`
}

// ErrRefused says a shared document did not pass the gate.
var ErrRefused = errors.New("collab: the document did not pass the gate")

// Store keeps a shared document, if it passes the same gate as a published page: a
// hostile client must not be able to leave something behind (SEC-546). The service
// does not write versions, so this changes no page, and a keystroke is not audited.
func (s *Service) Store(ctx context.Context, pageID uuid.UUID, in Stored) error {
	if _, err := document.Normalize(in.Document); err != nil {
		return fmt.Errorf("%w: %v", ErrRefused, err)
	}
	if in.Authors == nil {
		in.Authors = []uuid.UUID{}
	}
	tag, err := s.pool.Exec(ctx, `INSERT INTO collab_documents (page_id, state, title, authors)
		SELECT id, $2, $3, $4 FROM pages WHERE id = $1
		ON CONFLICT (page_id) DO UPDATE SET state = EXCLUDED.state, title = EXCLUDED.title, updated_at = now(),
			authors = (SELECT array_agg(DISTINCT a) FROM unnest(collab_documents.authors || EXCLUDED.authors) AS a)`,
		pageID, in.State, in.Title, in.Authors)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.NotFound()
	}
	return nil
}

// Published names everyone who took part in a version, and starts counting anew
// (COL-092). The person who published took part, also without typing.
func (s *Service) Published(ctx context.Context, c identity.Caller, pageID uuid.UUID, version int) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO page_version_authors (page_id, number, user_id)
			SELECT $1, $2, a FROM unnest(
				COALESCE((SELECT authors FROM collab_documents WHERE page_id = $1), '{}') || $3::uuid) AS a
			ON CONFLICT DO NOTHING`, pageID, version, c.UserID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE collab_documents SET authors = '{}' WHERE page_id = $1", pageID)
		return err
	})
}

// Authors lists who took part in the versions of a page, for someone who can see it.
func (s *Service) Authors(ctx context.Context, c identity.Caller, pageID uuid.UUID) (map[int][]string, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT a.number, COALESCE(u.display_name, '') FROM page_version_authors a
		LEFT JOIN users u ON u.id = a.user_id WHERE a.page_id = $1 ORDER BY a.number, 2`, pageID)
	if err != nil {
		return nil, err
	}
	out := map[int][]string{}
	var number int
	var name string
	_, err = pgx.ForEachRow(rows, []any{&number, &name}, func() error {
		out[number] = append(out[number], name)
		return nil
	})
	return out, err
}

// accessEvents change who may edit, as do all events of the permissions module, so
// connections are checked again at once (SEC-542).
var accessEvents = map[string]bool{"group.member_removed": true, "group.deleted": true, "user.disabled": true, "user.expired": true,
	"user.anonymised": true, "page.trashed": true, "page.archived": true, "space.archived": true, "space.trashed": true}

func changesAccess(eventType string) bool {
	return accessEvents[eventType] || strings.HasPrefix(eventType, "permission.") || strings.HasPrefix(eventType, "restriction.")
}

// Handle is the consumer of the event stream. A failed call is returned, so the
// stream delivers again: an access that ended must not wait for the next minute.
func (s *Service) Handle(ctx context.Context, data []byte) error {
	var m outbox.Message
	if err := json.Unmarshal(data, &m); err != nil {
		// A message that cannot be read never will be: redelivery would only repeat it.
		return nil
	}
	if !s.Installed() || !changesAccess(m.Type) {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.options.URL+"/recheck", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.options.Token)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("collab: recheck answered %d", resp.StatusCode)
	}
	return nil
}
