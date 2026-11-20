// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/db"
)

// Checks of every signature of an installation, for the command line: after a restore
// (DOC-772) and before audit entries are pruned (DOC-770).

// SignatureCheck is the outcome of a full verification of every signature.
type SignatureCheck struct {
	Checked    int               `json:"checked"`
	Mismatches []SignatureRecord `json:"mismatches"`
}

// VerifyAllSignatures runs the full verification of every signature, oldest first.
func VerifyAllSignatures(ctx context.Context, q db.Querier) (SignatureCheck, error) {
	out := SignatureCheck{Mismatches: []SignatureRecord{}}
	rows, err := q.Query(ctx, `SELECT `+signatureRecordColumns+` FROM signatures s
		LEFT JOIN version_hashes v ON v.page_id = s.page_id AND v.number = s.version_number ORDER BY s.signed_at, s.id`)
	if err != nil {
		return out, err
	}
	records, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (SignatureRecord, error) { return scanSignatureRecord(r) })
	if err != nil {
		return out, err
	}
	for _, rec := range records {
		v, err := verifySignature(ctx, q, rec)
		if err != nil {
			return out, err
		}
		out.Checked++
		if v.Mark != MarkValid {
			rec.Mark, rec.Reason = v.Mark, v.Reason
			out.Mismatches = append(out.Mismatches, rec)
		}
	}
	return out, nil
}

// OldestControlRecord is the moment of the oldest record of document control, or nil when
// there is none: a signature, or an audit entry about a page that is or was controlled, or
// about workflows. Pruning the audit log stops before it, so audit retention never removes
// the trail of a controlled document earlier than the document's own records (DOC-1002).
func OldestControlRecord(ctx context.Context, q db.Querier) (*time.Time, error) {
	var at *time.Time
	err := q.QueryRow(ctx, `SELECT min(t) FROM (
			SELECT min(signed_at) AS t FROM signatures
			UNION ALL SELECT min(e.occurred_at) FROM audit_events e
				WHERE e.object_type = 'page' AND e.object_id IN (SELECT r.page_id::text FROM workflow_rounds r UNION SELECT c.page_id::text FROM page_control c)
			UNION ALL SELECT min(e.occurred_at) FROM audit_events e
				WHERE e.action LIKE 'workflow.%' OR e.action LIKE 'space.workflow%' OR e.action LIKE 'space.approval_rule%'
		) x`).Scan(&at)
	return at, err
}
