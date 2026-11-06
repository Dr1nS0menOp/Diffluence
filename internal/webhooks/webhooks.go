// SPDX-License-Identifier: Apache-2.0

// Package webhooks tells outside systems what happened (spec 015, H10). What is sent
// is ids, a type and a time, signed as the Standard Webhooks specification says. Page
// text never leaves here, and a title only if an endpoint asked for it.
package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/keys"
	"github.com/diffluence/diffluence/internal/notify"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/outward"
	"github.com/diffluence/diffluence/internal/permissions"
)

const (
	secretContext   = "webhook-secret"
	secretBytes     = 32
	secretPrefix    = "whsec_"
	maxAttempts     = 5
	failuresToOff   = 50
	rotationOverlap = 24 * time.Hour
	logDays         = 30
	maxListed       = 200
	maxBatch        = 50
)

// waits are the pauses after each failed attempt: five attempts over a day.
var waits = []time.Duration{time.Minute, 10 * time.Minute, 2 * time.Hour, 22 * time.Hour}

// Events lists what an endpoint can ask for.
var Events = []string{"page.created", "page.published", "page.renamed", "page.moved", "page.archived", "page.trashed", "page.restored",
	"page.labelled", "page.stage_changed", "page.owner_changed", "comment.created", "attachment.uploaded", "attachment.trashed",
	"space.created", "space.archived"}

// Service keeps endpoints and delivers.
type Service struct {
	pool    *pgxpool.Pool
	perms   *permissions.Service
	id      *identity.Service
	outward *outward.Service
	keyring *keys.Keyring
	notify  *notify.Service
	now     func() time.Time
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, id *identity.Service, out *outward.Service, keyring *keys.Keyring, n *notify.Service) *Service {
	return &Service{pool: pool, perms: perms, id: id, outward: out, keyring: keyring, notify: n, now: time.Now}
}

// Endpoint is one address. Secrets never leave this package, except once at creation.
type Endpoint struct {
	ID         uuid.UUID  `json:"id"`
	SpaceID    *uuid.UUID `json:"space_id"`
	Address    string     `json:"address"`
	Events     []string   `json:"events"`
	WithTitles bool       `json:"with_titles"`
	Enabled    bool       `json:"enabled"`
	Failures   int        `json:"failures"`
	OwnerID    uuid.UUID  `json:"owner_id"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Input is what a person sends.
type Input struct {
	Address    string   `json:"address"`
	Events     []string `json:"events"`
	WithTitles bool     `json:"with_titles"`
	Enabled    *bool    `json:"enabled"`
}

const endpointColumns = "id, space_id, address, events, with_titles, enabled, failures, owner_id, created_at FROM webhook_endpoints "

func scanEndpoint(row pgx.Row) (Endpoint, error) {
	var e Endpoint
	err := row.Scan(&e.ID, &e.SpaceID, &e.Address, &e.Events, &e.WithTitles, &e.Enabled, &e.Failures, &e.OwnerID, &e.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, apperr.NotFound()
	}
	return e, err
}

func (s *Service) mayManage(ctx context.Context, c identity.Caller, spaceID *uuid.UUID) error {
	if spaceID != nil {
		return s.perms.RequireSpace(ctx, s.pool, c.UserID, *spaceID, permissions.SpaceAdminister)
	}
	allowed, err := s.id.HasGlobalPermission(ctx, c.UserID, identity.PermAdministrator)
	if err != nil {
		return err
	}
	if !allowed {
		return apperr.Forbidden()
	}
	return nil
}

// Get returns an endpoint for someone who may manage it.
func (s *Service) Get(ctx context.Context, c identity.Caller, id uuid.UUID) (Endpoint, error) {
	e, err := scanEndpoint(s.pool.QueryRow(ctx, "SELECT "+endpointColumns+"WHERE id = $1", id))
	if err != nil {
		return e, err
	}
	return e, s.mayManage(ctx, c, e.SpaceID)
}

// List gives the endpoints of a space, or of the instance when spaceID is nil.
func (s *Service) List(ctx context.Context, c identity.Caller, spaceID *uuid.UUID) ([]Endpoint, error) {
	if err := s.mayManage(ctx, c, spaceID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT "+endpointColumns+"WHERE space_id IS NOT DISTINCT FROM $1 ORDER BY created_at LIMIT $2", spaceID, maxListed)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Endpoint, error) { return scanEndpoint(r) })
	if list == nil {
		list = []Endpoint{}
	}
	return list, err
}

func (s *Service) check(ctx context.Context, in Input) error {
	if _, err := s.outward.Target(ctx, in.Address); err != nil {
		return err
	}
	if len(in.Events) == 0 {
		return apperr.Invalid("Choose at least one event.")
	}
	for _, e := range in.Events {
		if !slices.Contains(Events, e) {
			return apperr.Invalid("That is not an event.")
		}
	}
	return nil
}

func (s *Service) newSecret() (string, []byte, error) {
	raw := make([]byte, secretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	sealed, err := s.keyring.Seal(raw, secretContext)
	return secretPrefix + base64.StdEncoding.EncodeToString(raw), sealed, err
}

func record(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, e Endpoint) error {
	space := ""
	if e.SpaceID != nil {
		space = e.SpaceID.String()
	}
	if _, err := audit.Record(ctx, tx, c.Event(action, "webhook", e.ID.String(), space, map[string]any{"events": e.Events, "with_titles": e.WithTitles})); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"webhook_id": e.ID, "space_id": e.SpaceID})
	return err
}

// Create adds an endpoint, and answers with its secret, shown this once.
func (s *Service) Create(ctx context.Context, c identity.Caller, spaceID *uuid.UUID, in Input) (Endpoint, string, error) {
	e := Endpoint{ID: ids.New(), SpaceID: spaceID, Address: in.Address, Events: in.Events, WithTitles: in.WithTitles, Enabled: true, OwnerID: c.UserID}
	if err := s.mayManage(ctx, c, spaceID); err != nil {
		return e, "", err
	}
	if err := s.check(ctx, in); err != nil {
		return e, "", err
	}
	secret, sealed, err := s.newSecret()
	if err != nil {
		return e, "", err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO webhook_endpoints (id, space_id, address, events, with_titles, secret, owner_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, e.ID, spaceID, e.Address, e.Events, e.WithTitles, sealed, e.OwnerID); err != nil {
			return err
		}
		return record(ctx, tx, c, "webhook.created", e)
	})
	return e, secret, err
}

// Update changes an endpoint. Turning it on again starts its count of failures anew.
func (s *Service) Update(ctx context.Context, c identity.Caller, id uuid.UUID, in Input) (Endpoint, error) {
	e, err := s.Get(ctx, c, id)
	if err != nil {
		return e, err
	}
	if err := s.check(ctx, in); err != nil {
		return e, err
	}
	e.Address, e.Events, e.WithTitles = in.Address, in.Events, in.WithTitles
	if in.Enabled != nil {
		e.Enabled = *in.Enabled
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE webhook_endpoints SET address = $2, events = $3, with_titles = $4, enabled = $5,
			failures = CASE WHEN $5 THEN 0 ELSE failures END WHERE id = $1`, id, e.Address, e.Events, e.WithTitles, e.Enabled); err != nil {
			return err
		}
		return record(ctx, tx, c, "webhook.changed", e)
	})
	return e, err
}

// Delete removes an endpoint with its log.
func (s *Service) Delete(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	e, err := s.Get(ctx, c, id)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM webhook_endpoints WHERE id = $1", id); err != nil {
			return err
		}
		return record(ctx, tx, c, "webhook.deleted", e)
	})
}

// Rotate makes a new secret. The old one keeps being signed with for a day, so a
// receiver can change over without losing anything.
func (s *Service) Rotate(ctx context.Context, c identity.Caller, id uuid.UUID) (string, error) {
	e, err := s.Get(ctx, c, id)
	if err != nil {
		return "", err
	}
	secret, sealed, err := s.newSecret()
	if err != nil {
		return "", err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE webhook_endpoints SET old_secret = secret, old_until = $2, secret = $3 WHERE id = $1",
			id, s.now().Add(rotationOverlap), sealed); err != nil {
			return err
		}
		return record(ctx, tx, c, "webhook.rotated", e)
	})
	return secret, err
}

// Sign gives one signature value of the Standard Webhooks format.
func Sign(secret []byte, id string, at time.Time, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(id + "." + strconv.FormatInt(at.Unix(), 10) + "."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// Handle is the consumer of the event stream: it queues a delivery per endpoint. The
// unique pair of endpoint and event keeps a redelivered event from sending twice.
func (s *Service) Handle(ctx context.Context, data []byte) error {
	var m outbox.Message
	if err := json.Unmarshal(data, &m); err != nil {
		// A message that cannot be read never will be: redelivery would only repeat it.
		return nil
	}
	if !slices.Contains(Events, m.Type) {
		return nil
	}
	var payload struct {
		PageID  *uuid.UUID `json:"page_id"`
		SpaceID *uuid.UUID `json:"space_id"`
	}
	if err := json.Unmarshal(m.Payload, &payload); err != nil {
		return nil
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO webhook_deliveries (id, endpoint_id, event_id, event_type, event_time, page_id, space_id, state)
		SELECT gen_random_uuid(), e.id, $1, $2, $3, $4, $5, 'waiting' FROM webhook_endpoints e
		WHERE e.enabled AND $2 = ANY(e.events) AND (e.space_id IS NULL OR e.space_id = $5)
		ON CONFLICT (endpoint_id, event_id) DO NOTHING`, m.ID, m.Type, m.CreatedAt, payload.PageID, payload.SpaceID)
	return err
}

// delivery is one row to send.
type delivery struct {
	id, endpoint, event uuid.UUID
	eventType           string
	at                  time.Time
	page, space         *uuid.UUID
	attempts            int
}

// DeliverDue sends what is waiting. A person's wait for a slow receiver is a worker's,
// so this runs from the periodic tasks.
func (s *Service) DeliverDue(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT id, endpoint_id, event_id, event_type, event_time, page_id, space_id, attempts
		FROM webhook_deliveries WHERE state = 'waiting' AND next_attempt <= $1 ORDER BY next_attempt LIMIT $2`, s.now(), maxBatch)
	if err != nil {
		return err
	}
	due, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (delivery, error) {
		var d delivery
		err := r.Scan(&d.id, &d.endpoint, &d.event, &d.eventType, &d.at, &d.page, &d.space, &d.attempts)
		return d, err
	})
	if err != nil {
		return err
	}
	for _, d := range due {
		if err := s.deliver(ctx, d); err != nil {
			return err
		}
	}
	return nil
}

type target struct {
	Endpoint
	secret, oldSecret []byte
	oldUntil          *time.Time
}

func (s *Service) loadTarget(ctx context.Context, id uuid.UUID) (target, error) {
	var t target
	err := s.pool.QueryRow(ctx, `SELECT id, space_id, address, events, with_titles, enabled, failures, owner_id, created_at, secret, old_secret, old_until
		FROM webhook_endpoints WHERE id = $1`, id).Scan(&t.ID, &t.SpaceID, &t.Address, &t.Events, &t.WithTitles, &t.Enabled, &t.Failures, &t.OwnerID, &t.CreatedAt,
		&t.secret, &t.oldSecret, &t.oldUntil)
	return t, err
}

// body builds what is sent, and says no when the owner may not hear about the page.
// The check happens at delivery: access can have ended since the event.
func (s *Service) body(ctx context.Context, t target, d delivery) ([]byte, bool, error) {
	out := map[string]any{"type": d.eventType, "timestamp": d.at.UTC().Format(time.RFC3339), "id": d.event}
	data := map[string]any{}
	if d.space != nil {
		data["space_id"] = d.space
	}
	if d.page != nil {
		allowed, err := s.perms.CheckPage(ctx, s.pool, t.OwnerID, *d.page, permissions.ActionPageView, uuid.Nil)
		if err != nil {
			return nil, false, err
		}
		if !allowed.Allowed {
			return nil, false, nil
		}
		data["page_id"] = d.page
		if t.WithTitles {
			var title string
			if err := s.pool.QueryRow(ctx, "SELECT title FROM pages WHERE id = $1", *d.page).Scan(&title); err != nil {
				return nil, false, err
			}
			data["title"] = title
		}
	}
	out["data"] = data
	raw, err := json.Marshal(out)
	return raw, true, err
}

func (s *Service) signatures(t target, id string, at time.Time, body []byte) (string, error) {
	raw, err := s.keyring.Open(t.secret, secretContext)
	if err != nil {
		return "", err
	}
	out := []string{Sign(raw, id, at, body)}
	if t.oldSecret != nil && t.oldUntil != nil && s.now().Before(*t.oldUntil) {
		old, err := s.keyring.Open(t.oldSecret, secretContext)
		if err != nil {
			return "", err
		}
		out = append(out, Sign(old, id, at, body))
	}
	return strings.Join(out, " "), nil
}

func (s *Service) deliver(ctx context.Context, d delivery) error {
	t, err := s.loadTarget(ctx, d.endpoint)
	if err != nil {
		return err
	}
	if !t.Enabled {
		return s.settle(ctx, d, "dropped", 0)
	}
	body, allowed, err := s.body(ctx, t, d)
	if err != nil {
		return err
	}
	if !allowed {
		return s.settle(ctx, d, "dropped", 0)
	}
	at := s.now()
	id := d.event.String()
	signature, err := s.signatures(t, id, at, body)
	if err != nil {
		return err
	}
	answer, err := s.outward.Post(ctx, t.Address, body, map[string]string{"webhook-id": id,
		"webhook-timestamp": strconv.FormatInt(at.Unix(), 10), "webhook-signature": signature})
	if err == nil && answer.Status >= 200 && answer.Status < 300 {
		return s.settle(ctx, d, "delivered", answer.Status)
	}
	return s.failed(ctx, t, d, answer.Status)
}

func (s *Service) settle(ctx context.Context, d delivery, state string, status int) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE webhook_deliveries SET state = $2, attempts = attempts + 1, last_status = $3 WHERE id = $1", d.id, state, status); err != nil {
			return err
		}
		if state != "delivered" {
			return nil
		}
		_, err := tx.Exec(ctx, "UPDATE webhook_endpoints SET failures = 0 WHERE id = $1", d.endpoint)
		return err
	})
}

// failed plans the next attempt, and switches off an endpoint that keeps failing.
func (s *Service) failed(ctx context.Context, t target, d delivery, status int) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		attempts := d.attempts + 1
		state, next := "waiting", s.now()
		if attempts >= maxAttempts {
			state = "failed"
		} else {
			next = next.Add(waits[attempts-1])
		}
		if _, err := tx.Exec(ctx, "UPDATE webhook_deliveries SET state = $2, attempts = $3, next_attempt = $4, last_status = $5 WHERE id = $1",
			d.id, state, attempts, next, status); err != nil {
			return err
		}
		var failures int
		if err := tx.QueryRow(ctx, "UPDATE webhook_endpoints SET failures = failures + 1 WHERE id = $1 RETURNING failures", t.ID).Scan(&failures); err != nil {
			return err
		}
		if failures < failuresToOff {
			return nil
		}
		return s.switchOff(ctx, tx, t, d)
	})
}

func (s *Service) switchOff(ctx context.Context, tx pgx.Tx, t target, d delivery) error {
	if _, err := tx.Exec(ctx, "UPDATE webhook_endpoints SET enabled = false WHERE id = $1", t.ID); err != nil {
		return err
	}
	if err := record(ctx, tx, identity.SystemCaller("webhooks"), "webhook.switched_off", t.Endpoint); err != nil {
		return err
	}
	if d.page == nil || d.space == nil {
		return nil // the endpoint shows that it is off on its own screen
	}
	return s.notify.Deliver(ctx, tx, notify.Row{UserID: t.OwnerID, Type: notify.TypeRule, Reason: "webhook_off", PageID: *d.page, SpaceID: *d.space,
		Event: uuid.NewSHA1(t.ID, []byte(s.now().Format(time.RFC3339))), Data: map[string]any{"what": "webhook_off", "address": t.Address}})
}

// Delivery is one line of the log. Bodies are not kept.
type Delivery struct {
	ID        uuid.UUID `json:"id"`
	EventType string    `json:"event_type"`
	State     string    `json:"state"`
	Attempts  int       `json:"attempts"`
	Status    int       `json:"last_status"`
	CreatedAt time.Time `json:"created_at"`
}

// Deliveries reads the log.
func (s *Service) Deliveries(ctx context.Context, c identity.Caller, id uuid.UUID) ([]Delivery, error) {
	if _, err := s.Get(ctx, c, id); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id, event_type, state, attempts, last_status, created_at FROM webhook_deliveries
		WHERE endpoint_id = $1 ORDER BY created_at DESC LIMIT $2`, id, maxListed)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Delivery, error) {
		var d Delivery
		err := r.Scan(&d.ID, &d.EventType, &d.State, &d.Attempts, &d.Status, &d.CreatedAt)
		return d, err
	})
	if list == nil {
		list = []Delivery{}
	}
	return list, err
}

// Resend queues a delivery again, with the same id, so a receiver can drop a double.
func (s *Service) Resend(ctx context.Context, c identity.Caller, id, delivery uuid.UUID) error {
	e, err := s.Get(ctx, c, id)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE webhook_deliveries SET state = 'waiting', attempts = 0, next_attempt = now() WHERE id = $1 AND endpoint_id = $2", delivery, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound()
		}
		return record(ctx, tx, c, "webhook.resent", e)
	})
}

// PurgeLog removes deliveries past their 30 days, and forgets old secrets.
func (s *Service) PurgeLog(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, "DELETE FROM webhook_deliveries WHERE created_at < now() - make_interval(days => $1)", logDays); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, "UPDATE webhook_endpoints SET old_secret = NULL, old_until = NULL WHERE old_until < now()")
	return err
}
