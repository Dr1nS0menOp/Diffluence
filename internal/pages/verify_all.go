// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/db"
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

// OldestSignature is when the first signature that stays was made, or nil when there is
// none. The audit entries from then on hold signatures in the chain, so pruning the audit
// log stops before it (DOC-770, Part 11 §11.10(c)).
func OldestSignature(ctx context.Context, q db.Querier) (*time.Time, error) {
	var at *time.Time
	err := q.QueryRow(ctx, "SELECT min(signed_at) FROM signatures").Scan(&at)
	return at, err
}
