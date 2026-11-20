// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/attachments"
	"github.com/kenwerk/kenwerk/internal/db"
	"github.com/kenwerk/kenwerk/internal/labels"
	"github.com/kenwerk/kenwerk/internal/workflow"
)

// The conditions of a step (spec 064 wave DC5, DOC-120). They are read only for a workflow
// that has any, so every other page pays nothing.

// conditionFacts reads what the tests of a page look at: its labels, which labels the
// conditions name are still allowed in its space, which parts changed since the approved
// version, and how many files it has.
func conditionFacts(ctx context.Context, q db.Querier, p Page, ctl control, def workflow.Definition) (workflow.Facts, error) {
	f := workflow.Facts{Labels: map[string]bool{}, Known: map[string]bool{}, Changed: map[string]bool{}}
	list, err := labelsOf(ctx, q, p.ID)
	if err != nil {
		return f, err
	}
	for _, l := range list {
		f.Labels[l] = true
	}
	for _, l := range def.LabelsNamed() {
		refused, err := labels.Refused(ctx, q, p.SpaceID, l)
		if err != nil {
			return f, err
		}
		f.Known[l] = !refused
	}
	if !ctl.approved() {
		f.Changed[workflow.PartTitle], f.Changed[workflow.PartBody], f.Changed[workflow.PartFiles] = true, true, true
	} else {
		var title, body bool
		err := q.QueryRow(ctx, `SELECT a.title <> h.title, a.document <> h.document FROM page_versions a, page_versions h
			WHERE a.page_id = $1 AND a.number = $2 AND h.page_id = $1 AND h.number = $3`, p.ID, *ctl.approvedVersion, ctl.headVersion).Scan(&title, &body)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return f, err
		}
		f.Changed[workflow.PartTitle], f.Changed[workflow.PartBody] = title, body
		same, err := attachments.SamePins(ctx, q, p.ID, *ctl.approvedVersion, ctl.headVersion)
		if err != nil {
			return f, err
		}
		f.Changed[workflow.PartFiles] = !same
	}
	err = q.QueryRow(ctx, "SELECT count(*) FROM attachments WHERE page_id = $1 AND status = 'active'", p.ID).Scan(&f.Attachments)
	return f, err
}

// blockedSteps names why each step of the band that its conditions hold back cannot be taken.
func blockedSteps(ctx context.Context, q db.Querier, p Page, ctl control, def workflow.Definition, steps []workflow.Transition) (map[string]string, error) {
	out := map[string]string{}
	if !def.HasConditions() {
		return out, nil
	}
	f, err := conditionFacts(ctx, q, p, ctl, def)
	if err != nil {
		return out, err
	}
	for _, tr := range steps {
		if ok, reason := tr.Allows(f); !ok {
			out[tr.ID] = reason
		}
	}
	return out, nil
}

// requireConditions refuses a step whose conditions do not hold, with the reason.
func requireConditions(ctx context.Context, tx pgx.Tx, p Page, ctl control, def workflow.Definition, tr workflow.Transition) error {
	if tr.Conditions == nil {
		return nil
	}
	f, err := conditionFacts(ctx, tx, p, ctl, def)
	if err != nil {
		return err
	}
	if ok, reason := tr.Allows(f); !ok {
		return apperr.Conflict("condition_failed", "This step cannot be taken yet: "+reasonWords[reason])
	}
	return nil
}

// reasonWords say each reason of a refused condition; the screens translate the code.
var reasonWords = map[string]string{
	workflow.ReasonLabelMissing:  "the page lacks a label the step asks for.",
	workflow.ReasonLabelPresent:  "the page has a label the step does not allow.",
	workflow.ReasonLabelGone:     "a label the step names no longer exists in this space.",
	workflow.ReasonUnchanged:     "the change does not touch the part the step asks for.",
	workflow.ReasonNoAttachment:  "the page has no file.",
	workflow.ReasonNoneHolds:     "none of its conditions holds.",
	workflow.ReasonBadConditions: "its conditions are not valid.",
}
