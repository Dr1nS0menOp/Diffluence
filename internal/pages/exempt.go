// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/labels"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Exempt labels (spec 064 wave DC5, DOC-402). A space workflow can leave out pages that carry
// one of its exempt labels; those are ordinary pages. Putting such a label on a page or taking
// it off is a decision about control, so it needs space.workflows, never page.edit alone, and
// it is audited. Taking it off puts the page under the workflow again, in its start state.

// maxExemptLabels bounds the labels one space workflow exempts.
const maxExemptLabels = 20

// settleExempt is why a page is settled again after an exempt label changed.
const settleExempt = "exempt_label"

// cleanExemptLabels checks the labels a space workflow exempts.
func cleanExemptLabels(raw []string) ([]string, error) {
	if len(raw) > maxExemptLabels {
		return nil, apperr.Invalid("A space workflow exempts at most 20 labels.")
	}
	out := []string{}
	for _, r := range raw {
		l, err := labels.Clean(r)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out, nil
}

// exemptLabelsOf reads the labels the space workflow of a space exempts.
func exemptLabelsOf(ctx context.Context, q db.Querier, spaceID uuid.UUID) ([]string, error) {
	var list []string
	err := q.QueryRow(ctx, "SELECT exempt_labels FROM space_workflows WHERE space_id = $1", spaceID).Scan(&list)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return list, err
}

// requireExemptRight says whether a label is exempt in a space, and refuses a person who may
// not touch it there.
func (s *Service) requireExemptRight(ctx context.Context, tx pgx.Tx, c identity.Caller, spaceID uuid.UUID, label string) (bool, error) {
	exempt, err := s.requireExemptRights(ctx, tx, c, spaceID, []string{label})
	return len(exempt) > 0, err
}

// requireExemptRights is the one rule for every writer of labels: of the labels a change puts
// on or takes off, it answers those that are exempt in the space, and refuses the change when
// there are any and the caller lacks space.workflows there (DOC-402).
func (s *Service) requireExemptRights(ctx context.Context, tx pgx.Tx, c identity.Caller, spaceID uuid.UUID, changed []string) ([]string, error) {
	list, err := exemptLabelsOf(ctx, tx, spaceID)
	if err != nil {
		return nil, err
	}
	var exempt []string
	for _, label := range changed {
		if slices.Contains(list, label) {
			exempt = append(exempt, label)
		}
	}
	if len(exempt) == 0 {
		return nil, nil
	}
	d, err := s.perms.CheckSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceWorkflows)
	if err != nil {
		return exempt, err
	}
	if !d.Allowed {
		return exempt, apperr.Forbidden()
	}
	return exempt, nil
}

// followExemption settles a page after an exempt label went on or came off, and audits it.
func (s *Service) followExemption(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, label string, added bool) error {
	if _, err := s.settle(ctx, tx, c, []uuid.UUID{p.ID}, settleExempt); err != nil {
		return err
	}
	action := "page.exemption_ended"
	if added {
		action = "page.exempted"
	}
	return recordAudit(ctx, tx, c, action, "page", p.ID, p.SpaceID, map[string]any{"label": label})
}

// dropExemptLabels takes the exempt labels of a space off a page that someone without
// space.workflows brought there, such as a copy, so nobody escapes review by copying.
func (s *Service) dropExemptLabels(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page) error {
	list, err := exemptLabelsOf(ctx, tx, p.SpaceID)
	if err != nil || len(list) == 0 {
		return err
	}
	d, err := s.perms.CheckSpace(ctx, tx, c.UserID, p.SpaceID, permissions.SpaceWorkflows)
	if err != nil || d.Allowed {
		return err
	}
	_, err = tx.Exec(ctx, "DELETE FROM page_labels WHERE page_id = $1 AND label = ANY($2)", p.ID, list)
	return err
}
