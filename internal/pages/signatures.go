// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/audit"
	"github.com/kenwerk/kenwerk/internal/db"
	"github.com/kenwerk/kenwerk/internal/identity"
)

// Showing and verifying signatures (DOC-740, DOC-741). On display a signature is compared with
// the hash its version keeps, which is a comparison and no hash of the page (DOC-1303). A full
// verification recomputes the hash from the content and checks the signature's audit entry
// in the chain; the page offers it on demand, and the auditor's export runs it.

// The marks of a signature.
const (
	MarkValid    = "valid"
	MarkMismatch = "does_not_match"
)

// Why a signature does not match.
const (
	// reasonRecordDiffers: the record names another hash than its version keeps, so it was
	// moved, copied or edited.
	reasonRecordDiffers = "record_differs"
	// reasonContentChanged: the version no longer gives the hash it keeps.
	reasonContentChanged = "content_changed"
	reasonVersionMissing = "version_missing"
	reasonUnknownForm    = "unknown_form"
	reasonAuditMissing   = "audit_entry_missing"
	reasonAuditDiffers   = "audit_entry_differs"
	reasonChainBroken    = "audit_chain_broken"
)

// SignatureView is a signature as the page and its history show it (DOC-740): the printed
// name of the moment of signing, the meaning, the time and the mark.
type SignatureView struct {
	ID          uuid.UUID `json:"id"`
	Version     int       `json:"version"`
	SignerName  string    `json:"signer_name"`
	Meaning     string    `json:"meaning"`
	MeaningName string    `json:"meaning_name"`
	SignedAt    time.Time `json:"signed_at"`
	Method      string    `json:"method"`
	Mark        string    `json:"mark"`
	Reason      string    `json:"reason,omitempty"`
}

// signatureViewColumns reads a signature with the quick mark of display: does the record
// name the hash its version keeps?
const signatureViewColumns = `s.id, s.version_number, s.printed_name, s.meaning, s.meaning_name, s.signed_at, s.method,
	s.content_hash IS NOT DISTINCT FROM v.content_hash AND s.content_form IS NOT DISTINCT FROM v.content_form`

func scanSignatureView(r pgx.Row) (SignatureView, error) {
	var v SignatureView
	var matches bool
	err := r.Scan(&v.ID, &v.Version, &v.SignerName, &v.Meaning, &v.MeaningName, &v.SignedAt, &v.Method, &matches)
	v.Mark = MarkValid
	if !matches {
		v.Mark, v.Reason = MarkMismatch, reasonRecordDiffers
	}
	return v, err
}

// signaturesOf lists the signatures of some versions of a page, oldest first.
func signaturesOf(ctx context.Context, q db.Querier, pageID uuid.UUID, versions []int) ([]SignatureView, error) {
	rows, err := q.Query(ctx, `SELECT `+signatureViewColumns+` FROM signatures s
		LEFT JOIN version_hashes v ON v.page_id = s.page_id AND v.number = s.version_number
		WHERE s.page_id = $1 AND s.version_number = ANY($2) ORDER BY s.signed_at, s.id LIMIT 2000`, pageID, versions)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (SignatureView, error) { return scanSignatureView(r) })
	if out == nil {
		out = []SignatureView{}
	}
	return out, err
}

// SignatureRecord is the whole record of a signature (DOC-710), for the auditor's export and
// a full verification.
type SignatureRecord struct {
	SignatureView
	PageID             uuid.UUID  `json:"page_id"`
	SpaceID            uuid.UUID  `json:"space_id"`
	SignerID           uuid.UUID  `json:"signer_id"`
	MeaningDescription string     `json:"meaning_description"`
	WorkflowID         uuid.UUID  `json:"workflow_id"`
	WorkflowVersion    int        `json:"workflow_version"`
	Transition         string     `json:"transition"`
	RoundID            *uuid.UUID `json:"round_id"`
	Stage              *int       `json:"stage"`
	ContentHash        string     `json:"content_hash"`
	ContentForm        int        `json:"content_form"`
	AuditEventID       uuid.UUID  `json:"audit_event_id"`
}

const signatureRecordColumns = signatureViewColumns + `, s.page_id, s.space_id, s.signer_id, s.meaning_description, s.workflow_id,
	s.workflow_version, s.transition_id, s.round_id, s.stage, encode(s.content_hash, 'hex'), s.content_form, s.audit_event_id`

func scanSignatureRecord(r pgx.Row) (SignatureRecord, error) {
	var rec SignatureRecord
	var matches bool
	err := r.Scan(&rec.ID, &rec.Version, &rec.SignerName, &rec.Meaning, &rec.MeaningName, &rec.SignedAt, &rec.Method, &matches,
		&rec.PageID, &rec.SpaceID, &rec.SignerID, &rec.MeaningDescription, &rec.WorkflowID, &rec.WorkflowVersion, &rec.Transition,
		&rec.RoundID, &rec.Stage, &rec.ContentHash, &rec.ContentForm, &rec.AuditEventID)
	rec.Mark = MarkValid
	if !matches {
		rec.Mark, rec.Reason = MarkMismatch, reasonRecordDiffers
	}
	return rec, err
}

// Verification is the outcome of a full verification of one signature (DOC-741).
type Verification struct {
	Mark   string `json:"mark"`
	Reason string `json:"reason,omitempty"`
	// VersionHash is the hash the version keeps; Recomputed is the hash of its content now.
	VersionHash string `json:"version_hash"`
	Recomputed  string `json:"recomputed_hash"`
	// AuditSeq is the position of the signature's audit entry in the chain, 0 while it waits
	// for the sealer.
	AuditSeq int64 `json:"audit_seq"`
}

func mismatch(v Verification, reason string) Verification {
	v.Mark, v.Reason = MarkMismatch, reason
	return v
}

// signedEvent is what the audit entry of a signature says, as the chain hashed it.
type signedEvent struct {
	Action   string `json:"action"`
	ObjectID string `json:"object_id"`
	ActorID  string `json:"actor_id"`
	Details  struct {
		Signature   uuid.UUID `json:"signature"`
		Version     int       `json:"version"`
		ContentHash string    `json:"content_hash"`
		ContentForm int       `json:"content_form"`
		Meaning     string    `json:"meaning"`
	} `json:"details"`
}

// verifySignature recomputes the hash of the signed version and checks it against the version
// and the record, then checks that the signature's audit entry says the same and holds its
// place in the chain.
func verifySignature(ctx context.Context, q db.Querier, rec SignatureRecord) (Verification, error) {
	var v Verification
	if rec.ContentForm != ContentForm {
		return mismatch(v, reasonUnknownForm), nil
	}
	content, err := signedContentOf(ctx, q, rec.PageID, rec.Version)
	if apperr.IsNotFound(err) {
		return mismatch(v, reasonVersionMissing), nil
	}
	if err != nil {
		return v, err
	}
	v.Recomputed = hexHash(content.Hash())
	err = q.QueryRow(ctx, "SELECT encode(content_hash, 'hex') FROM version_hashes WHERE page_id = $1 AND number = $2",
		rec.PageID, rec.Version).Scan(&v.VersionHash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return v, err
	}
	entry, found, err := audit.EntryByID(ctx, q, rec.AuditEventID)
	if err != nil {
		return v, err
	}
	v.AuditSeq = entry.Seq
	switch {
	case rec.ContentHash != v.VersionHash:
		return mismatch(v, reasonRecordDiffers), nil
	case v.Recomputed != v.VersionHash:
		return mismatch(v, reasonContentChanged), nil
	case !found:
		return mismatch(v, reasonAuditMissing), nil
	}
	var said signedEvent
	if err := json.Unmarshal([]byte(entry.Payload), &said); err != nil {
		return mismatch(v, reasonAuditDiffers), nil
	}
	d := said.Details
	if said.Action != "page.signed" || said.ObjectID != rec.PageID.String() || said.ActorID != rec.SignerID.String() ||
		d.Signature != rec.ID || d.Version != rec.Version || d.ContentHash != rec.ContentHash || d.ContentForm != rec.ContentForm ||
		d.Meaning != rec.Meaning {
		return mismatch(v, reasonAuditDiffers), nil
	}
	if !entry.Linked {
		return mismatch(v, reasonChainBroken), nil
	}
	v.Mark = MarkValid
	return v, nil
}

// CheckedSignature is a signature with its full verification.
type CheckedSignature struct {
	SignatureView
	Verification Verification `json:"verification"`
}

// VerifySignatures runs the full verification of the signatures of one version, for whoever
// may see that version in the history (DOC-741, DOC-320).
func (s *Service) VerifySignatures(ctx context.Context, c identity.Caller, id uuid.UUID, number int) ([]CheckedSignature, error) {
	if _, err := s.GetVersion(ctx, c, id, number); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+signatureRecordColumns+` FROM signatures s
		LEFT JOIN version_hashes v ON v.page_id = s.page_id AND v.number = s.version_number
		WHERE s.page_id = $1 AND s.version_number = $2 ORDER BY s.signed_at, s.id LIMIT 500`, id, number)
	if err != nil {
		return nil, err
	}
	records, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (SignatureRecord, error) { return scanSignatureRecord(r) })
	if err != nil {
		return nil, err
	}
	out := make([]CheckedSignature, 0, len(records))
	for _, rec := range records {
		v, err := verifySignature(ctx, s.pool, rec)
		if err != nil {
			return nil, err
		}
		view := rec.SignatureView
		view.Mark, view.Reason = v.Mark, v.Reason
		out = append(out, CheckedSignature{SignatureView: view, Verification: v})
	}
	return out, nil
}

// signedVersionGuard refuses to take away a signed version: retention wins, also over an
// administrator (DOC-770).
func signedVersionGuard(ctx context.Context, q db.Querier, pageID uuid.UUID, number int) error {
	var signed bool
	if err := q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM signatures WHERE page_id = $1 AND version_number = $2)", pageID, number).
		Scan(&signed); err != nil {
		return err
	}
	if signed {
		return apperr.Conflict("signed_version", "This version was signed. Signed versions are kept for the retention of signed records.")
	}
	return nil
}

// signatureRow is a signature as the band's query aggregates it.
type signatureRow struct {
	SignatureView
	Matches bool `json:"matches"`
}

// readSignatureViews reads the signatures the band's query aggregated, with their marks.
func readSignatureViews(raw []byte) ([]SignatureView, error) {
	var rows []signatureRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("pages: read signatures: %v", err)
	}
	out := make([]SignatureView, 0, len(rows))
	for _, r := range rows {
		v := r.SignatureView
		v.Mark = MarkValid
		if !r.Matches {
			v.Mark, v.Reason = MarkMismatch, reasonRecordDiffers
		}
		out = append(out, v)
	}
	return out, nil
}

// keepSignaturesOf keeps the signatures of the version the band shows.
func (ci *ControlInfo) keepSignaturesOf(version int) {
	ci.Signatures = []SignatureView{}
	for _, v := range ci.signed {
		if v.Version == version {
			ci.Signatures = append(ci.Signatures, v)
		}
	}
}

// SignaturesOfVersion lists the signatures of one version of a page with their quick marks,
// for the published copy of a signed version, which shows them and copies no record (PUB-731).
func SignaturesOfVersion(ctx context.Context, q db.Querier, pageID uuid.UUID, version int) ([]SignatureView, error) {
	return signaturesOf(ctx, q, pageID, []int{version})
}
