// SPDX-License-Identifier: Apache-2.0

// Package notify tells people what happened (spec 010, C10 to C13). The worker fans
// events out into rows. A row holds ids, never a title or a sentence, and access is
// checked three times: at fan-out, when shown, and when mailed (spec 010, D11).
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/keys"
	"github.com/diffluence/diffluence/internal/mail"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/watches"
)

// keepFor is how long a notification stays (COL-069).
const keepFor = 90 * 24 * time.Hour

// maxGroupMembers is the largest group a mention can reach (spec 010, D20).
const maxGroupMembers = 50

// Why someone got a notification.
const (
	ReasonMentioned = "mentioned"
	ReasonAssigned  = "assigned"
	ReasonWatching  = "watching"
	ReasonShared    = "shared"
	ReasonExport    = "export"
	ReasonAuthor    = "author"
	ReasonThread    = "thread"
	ReasonAsked     = "asked"
)

// Options are what the module needs to write mail.
type Options struct {
	InstanceName string
	PublicOrigin string
	Keyring      *keys.Keyring
}

// Service fans out, lists and mails notifications.
type Service struct {
	pool     *pgxpool.Pool
	perms    *permissions.Service
	settings *settings.Service
	watches  *watches.Service
	options  Options
	now      func() time.Time
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, st *settings.Service, w *watches.Service, o Options) *Service {
	return &Service{pool: pool, perms: perms, settings: st, watches: w, options: o, now: time.Now}
}

// Row is one notification on its way to a person.
type Row struct {
	UserID    uuid.UUID
	Type      string
	Reason    string
	ActorID   uuid.UUID
	PageID    uuid.UUID
	SpaceID   uuid.UUID
	CommentID *uuid.UUID
	Event     uuid.UUID // the same event never reaches a person twice
	Data      map[string]any
	NeedsEdit bool // a thread for editors only reaches editors
	NoCheck   bool // the answer to someone who asked for access
	ForceMail bool // a rule sends an e-mail: mail unless the person turned this type off
}

func groupKey(r Row) string { return fmt.Sprintf("%s:%s", r.Type, r.PageID) }

func (s *Service) mayHear(ctx context.Context, q db.Querier, r Row) (bool, error) {
	if r.NoCheck {
		return true, nil
	}
	action := permissions.ActionPageView
	if r.NeedsEdit {
		action = permissions.PageEdit
	}
	d, err := s.perms.CheckPage(ctx, q, r.UserID, r.PageID, action, uuid.Nil)
	return d.Allowed, err
}

// Deliver stores one notification and queues its mail. It can run twice for the same
// event without harm, and skips anyone who may not hear about the page.
func (s *Service) Deliver(ctx context.Context, q db.Querier, r Row) error {
	choices, err := load(ctx, q, r.UserID)
	if err != nil {
		return err
	}
	channel := choices.channel(r.Type)
	if channel == ChannelOff || (r.UserID == r.ActorID && !choices.NotifyOwn) {
		return nil
	}
	if r.ForceMail {
		channel = ChannelMail
	}
	allowed, err := s.mayHear(ctx, q, r)
	if err != nil || !allowed {
		return err
	}
	data, err := json.Marshal(r.Data)
	if err != nil {
		return err
	}
	if r.Data == nil {
		data = []byte("{}")
	}
	actor := &r.ActorID
	if r.ActorID == uuid.Nil {
		actor = nil
	}
	id := ids.New()
	// Anything but the digest channel is marked as already counted for the digest.
	tag, err := q.Exec(ctx, `INSERT INTO notifications (id, user_id, type, reason, actor_id, page_id, space_id, comment_id, group_key, source_event, data, digested_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, CASE WHEN $12 THEN NULL ELSE now() END)
		ON CONFLICT (user_id, source_event) DO NOTHING`,
		id, r.UserID, r.Type, r.Reason, actor, r.PageID, r.SpaceID, r.CommentID, groupKey(r), r.Event, data, channel == ChannelDigest)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	if channel != ChannelMail {
		return nil
	}
	values, err := s.settings.Get(ctx)
	if err != nil || !values.MailEnabled {
		return err
	}
	return mail.EnqueueInstant(ctx, q, r.UserID, id)
}

// Purge removes what is older than the period people can see.
func (s *Service) Purge(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM notifications WHERE created_at < now() - make_interval(secs => $1)", keepFor.Seconds())
	return err
}
