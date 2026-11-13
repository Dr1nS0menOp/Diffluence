// SPDX-License-Identifier: Apache-2.0

package sharing

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/notify"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/settings"
)

const (
	maxInvitationWords  = 500
	maxListedInvitation = 200
	invitationWaiting   = "waiting"
	invitationApproved  = "approved"
	invitationRefused   = "refused"
	invitationObject    = "invitation_request"
)

// InvitationAsk is what someone with the role fills in.
type InvitationAsk struct {
	Email string
	Note  string
}

// InvitationRequest is a request as an administrator sees it. It names the space,
// which is what approval grants, and never the page: an administrator holds no
// content (PRM-004).
type InvitationRequest struct {
	ID            uuid.UUID `json:"id"`
	Email         string    `json:"email"`
	RequesterName string    `json:"requester_name"`
	SpaceID       uuid.UUID `json:"space_id"`
	SpaceName     string    `json:"space_name"`
	Note          string    `json:"note"`
	CreatedAt     time.Time `json:"created_at"`
}

// invitationRow is a request while it is being filed or answered.
type invitationRow struct {
	id        uuid.UUID
	email     string
	note      string
	pageID    uuid.UUID
	spaceID   uuid.UUID
	requester uuid.UUID
}

// requireGlobal refuses, and audits, anyone without perm. The refusal is written on its
// own, because the transaction around it ends with the error.
func (s *Service) requireGlobal(ctx context.Context, c identity.Caller, perm string) error {
	allowed, err := s.identity.HasGlobalPermission(ctx, c.UserID, perm)
	if err != nil || allowed {
		return err
	}
	refused := c.Event("access.denied", invitationObject, "", "", map[string]any{"permission": perm})
	refused.Result = audit.ResultDenied
	if err := audit.RecordStandalone(ctx, s.pool, refused); err != nil {
		return err
	}
	return apperr.Forbidden()
}

func (s *Service) requireInvitationsOn(ctx context.Context, q db.Querier) error {
	values := settings.DefaultFeatures
	if err := s.settings.ReadSectionIn(ctx, q, settings.SectionFeatures, &values); err != nil {
		return err
	}
	if !values.Invitations {
		return apperr.Conflict("invitations_off", "Invitations are turned off.")
	}
	return nil
}

// AskInvitation files a request to invite someone to the space of a page. Nothing goes
// out until an administrator approves it. Someone who cannot see the page gets 404
// whatever their role, so the answer says nothing about the page.
func (s *Service) AskInvitation(ctx context.Context, c identity.Caller, pageID uuid.UUID, in InvitationAsk) (uuid.UUID, error) {
	id := ids.New()
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
			return err
		}
		mayInvite, err := s.identity.HasGlobalPermissionIn(ctx, tx, c.UserID, identity.PermInvitePeople)
		if err != nil {
			return err
		}
		if !mayInvite {
			return errMayNotInvite
		}
		space, _, err := s.perms.LoadPage(ctx, tx, pageID)
		if err != nil {
			return err
		}
		return s.fileInvitation(ctx, tx, c, invitationRow{id: id, email: in.Email, note: in.Note, pageID: pageID, spaceID: space.ID, requester: c.UserID})
	})
	if errors.Is(err, errMayNotInvite) {
		// The refusal is audited once the transaction has let go of its connection.
		return uuid.Nil, s.requireGlobal(ctx, c, identity.PermInvitePeople)
	}
	return id, err
}

// errMayNotInvite ends the transaction of a request from someone who may not invite.
var errMayNotInvite = errors.New("sharing: may not invite people")

// GuestInvitation is the answer to a request for a guest: the link when an administrator
// invited at once, or else the request that waits for an administrator.
type GuestInvitation struct {
	Link    string
	Request uuid.UUID
}

// InviteGuest lets someone into one space as a guest. Only someone who may invite guests to
// the space asks, and only while an administrator allows guests. An administrator invites
// at once; anyone else files a request that an administrator answers (ADR-033).
func (s *Service) InviteGuest(ctx context.Context, c identity.Caller, spaceID uuid.UUID, email string) (GuestInvitation, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceInviteGuests); err != nil {
		return GuestInvitation{}, err
	}
	access := settings.DefaultAccess
	if err := s.settings.ReadSection(ctx, settings.SectionAccess, &access); err != nil {
		return GuestInvitation{}, err
	}
	if !access.GuestInvites {
		return GuestInvitation{}, apperr.Conflict("guest_invites_off", "An administrator turned guest invitations off.")
	}
	administrator, err := s.identity.HasGlobalPermission(ctx, c.UserID, identity.PermAdministrator)
	if err != nil {
		return GuestInvitation{}, err
	}
	if administrator {
		link, err := s.identity.Invite(ctx, c, identity.NewInvitation{Email: email, Type: identity.TypeGuest, SpaceID: &spaceID})
		return GuestInvitation{Link: link}, err
	}
	request, err := s.askGuestInvitation(ctx, c, spaceID, InvitationAsk{Email: email})
	return GuestInvitation{Request: request}, err
}

// askGuestInvitation files the request of InviteGuest. The answer hangs on the home page,
// because a notification hangs on a page.
func (s *Service) askGuestInvitation(ctx context.Context, c identity.Caller, spaceID uuid.UUID, in InvitationAsk) (uuid.UUID, error) {
	if err := s.requireGlobal(ctx, c, identity.PermInvitePeople); err != nil {
		return uuid.Nil, err
	}
	id := ids.New()
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var home *uuid.UUID
		if err := tx.QueryRow(ctx, "SELECT home_page_id FROM spaces WHERE id = $1", spaceID).Scan(&home); err != nil {
			return err
		}
		if home == nil {
			return apperr.Conflict("no_home_page", "This space has no home page yet.")
		}
		return s.fileInvitation(ctx, tx, c, invitationRow{id: id, email: in.Email, note: in.Note, pageID: *home, spaceID: spaceID, requester: c.UserID})
	})
	return id, err
}

func (s *Service) fileInvitation(ctx context.Context, tx pgx.Tx, c identity.Caller, r invitationRow) error {
	if err := s.requireInvitationsOn(ctx, tx); err != nil {
		return err
	}
	address, err := identity.InvitationAddress(r.email)
	if err != nil {
		return err
	}
	note := strings.TrimSpace(r.note)
	if !utf8.ValidString(note) || utf8.RuneCountInString(note) > maxInvitationWords {
		return apperr.Invalid("A note can have at most 500 characters.")
	}
	_, err = tx.Exec(ctx, `INSERT INTO invitation_requests (id, email, page_id, space_id, requester_id, note) VALUES ($1, $2, $3, $4, $5, $6)`,
		r.id, address, r.pageID, r.spaceID, r.requester, note)
	if db.IsUniqueViolation(err) {
		return apperr.Conflict("invitation_waiting", "An invitation for this address is already waiting for an administrator.")
	}
	if err != nil {
		return err
	}
	return recordInvitation(ctx, tx, c, "invitation.requested", r)
}

// recordInvitation writes the audit line and the event. Both hold ids: the address
// belongs to someone who was never asked.
func recordInvitation(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, r invitationRow) error {
	if _, err := audit.Record(ctx, tx, c.Event(action, invitationObject, r.id.String(), r.spaceID.String(),
		map[string]any{"page_id": r.pageID, "requester_id": r.requester})); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"request_id": r.id, "page_id": r.pageID, "space_id": r.spaceID,
		"actor_id": c.UserID, "requester_id": r.requester})
	return err
}

// WaitingInvitations lists what waits for an administrator, oldest first.
func (s *Service) WaitingInvitations(ctx context.Context, c identity.Caller) ([]InvitationRequest, error) {
	if err := s.requireGlobal(ctx, c, identity.PermAdministrator); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT r.id, r.email, u.display_name, r.space_id, sp.name, r.note, r.created_at
		FROM invitation_requests r
		JOIN users u ON u.id = r.requester_id
		JOIN spaces sp ON sp.id = r.space_id
		WHERE r.status = $1 ORDER BY r.created_at LIMIT $2`, invitationWaiting, maxListedInvitation)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (InvitationRequest, error) {
		var v InvitationRequest
		err := row.Scan(&v.ID, &v.Email, &v.RequesterName, &v.SpaceID, &v.SpaceName, &v.Note, &v.CreatedAt)
		return v, err
	})
	if out == nil {
		out = []InvitationRequest{}
	}
	return out, err
}

// ApproveInvitation sends the invitation. It returns the link when there is no mail.
// All of it is one transaction: if the mail cannot be queued, the request still waits.
func (s *Service) ApproveInvitation(ctx context.Context, c identity.Caller, id uuid.UUID, reason string) (string, error) {
	reason, err := s.checkAnswer(ctx, c, reason)
	if err != nil {
		return "", err
	}
	var link string
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		r, err := lockWaitingInvitation(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.requireInvitationsOn(ctx, tx); err != nil {
			return err
		}
		mayInvite, err := s.identity.HasGlobalPermissionIn(ctx, tx, r.requester, identity.PermInvitePeople)
		if err != nil {
			return err
		}
		if !mayInvite {
			return apperr.Conflict("requester_lost_role", "The person who asked may no longer invite anyone. Refuse this request.")
		}
		if err := s.answerInvitation(ctx, tx, c, r, invitationApproved, reason); err != nil {
			return err
		}
		link, err = s.identity.InviteWithin(ctx, tx, c, identity.NewInvitation{Email: r.email, Type: identity.TypeGuest, SpaceID: &r.spaceID})
		return err
	})
	return link, err
}

// RefuseInvitation ends a request. Nothing goes out, and it cannot be approved after all.
func (s *Service) RefuseInvitation(ctx context.Context, c identity.Caller, id uuid.UUID, reason string) error {
	reason, err := s.checkAnswer(ctx, c, reason)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		r, err := lockWaitingInvitation(ctx, tx, id)
		if err != nil {
			return err
		}
		return s.answerInvitation(ctx, tx, c, r, invitationRefused, reason)
	})
}

// checkAnswer holds what both answers need: an administrator, and a reason.
func (s *Service) checkAnswer(ctx context.Context, c identity.Caller, reason string) (string, error) {
	if err := s.requireGlobal(ctx, c, identity.PermAdministrator); err != nil {
		return "", err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || !utf8.ValidString(reason) || utf8.RuneCountInString(reason) > maxInvitationWords {
		return "", apperr.Invalid("An answer needs a reason of at most 500 characters.")
	}
	return reason, nil
}

// lockWaitingInvitation holds the row, so two answers at once become one answer and
// one conflict.
func lockWaitingInvitation(ctx context.Context, tx pgx.Tx, id uuid.UUID) (invitationRow, error) {
	r := invitationRow{id: id}
	var status string
	err := tx.QueryRow(ctx, `SELECT email, page_id, space_id, requester_id, status FROM invitation_requests WHERE id = $1 FOR UPDATE`, id).
		Scan(&r.email, &r.pageID, &r.spaceID, &r.requester, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, apperr.NotFound()
	}
	if err != nil {
		return r, err
	}
	if status != invitationWaiting {
		return r, apperr.Conflict("already_answered", "This request already has an answer.")
	}
	return r, nil
}

// answerInvitation closes the row and tells the requester. The notification carries the
// address, because the row no longer does.
func (s *Service) answerInvitation(ctx context.Context, tx pgx.Tx, c identity.Caller, r invitationRow, status, reason string) error {
	_, err := tx.Exec(ctx, `UPDATE invitation_requests SET status = $2, decided_by = $3, decided_at = now(), reason = $4, email = '', note = ''
		WHERE id = $1`, r.id, status, c.UserID, reason)
	if err != nil {
		return err
	}
	if err := recordInvitation(ctx, tx, c, "invitation."+status, r); err != nil {
		return err
	}
	return s.notify.Deliver(ctx, tx, notify.Row{UserID: r.requester, Type: notify.TypeInvitationAnswer, Reason: notify.ReasonAsked, ActorID: c.UserID,
		PageID: r.pageID, SpaceID: r.spaceID, Event: ids.New(), NoCheck: true,
		Data: map[string]any{"email": r.email, "answer": status, "reason": reason}})
}
