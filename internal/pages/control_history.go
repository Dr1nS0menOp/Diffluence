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

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// The control history of a page (DOC-1010): its versions, rounds, answers, signatures and
// transitions in one time line, each with its audit entry and whether that entry holds its
// place in the chain. It is for whoever reads the audit of the space, and shows no text or
// title of the page: who reads the audit does not read every page.

// maxHistoryEvents bounds one control history; the auditor's export has no such bound.
const maxHistoryEvents = 2000

// historyDetails are the details of an audit entry the history shows. Others, such as a
// title, stay in the audit log.
var historyDetails = []string{"transition", "from", "to", "version", "cause", "state", "answer", "before", "outcome", "round", "stage",
	"signature", "content_hash", "content_form", "meaning", "method", "user_id", "reason", "workflow", "workflow_version",
	"restored_from", "silent", "people"}

// HistoryItem is one entry of the control history.
type HistoryItem struct {
	At        time.Time      `json:"at"`
	Action    string         `json:"action"`
	ActorName string         `json:"actor_name"`
	System    bool           `json:"system"`
	Details   map[string]any `json:"details"`
	// Seq is the position of the entry in the chain, 0 while it waits for the sealer.
	Seq    int64 `json:"seq"`
	Linked bool  `json:"linked"`
	// Signature is the full verification of the signature a page.signed entry records.
	Signature *CheckedSignature `json:"signature,omitempty"`
}

// HistoryVersion is one version with the hash it keeps, once signed.
type HistoryVersion struct {
	Number      int       `json:"number"`
	CreatedAt   time.Time `json:"created_at"`
	AuthorName  string    `json:"author_name"`
	ContentHash string    `json:"content_hash"`
}

// ControlHistory is the time line of a controlled page.
type ControlHistory struct {
	PageID   uuid.UUID        `json:"page_id"`
	Versions []HistoryVersion `json:"versions"`
	Items    []HistoryItem    `json:"items"`
	// Truncated says only the newest entries are shown; the auditor's export has them all.
	Truncated bool `json:"truncated"`
}

// chainedEvent is what an audit payload says, as far as the history reads it.
type chainedEvent struct {
	ActorType string         `json:"actor_type"`
	ActorID   string         `json:"actor_id"`
	Details   map[string]any `json:"details"`
}

// ControlHistory reads the control history of a page, for whoever reads the audit of its
// space (DOC-1010).
func (s *Service) ControlHistory(ctx context.Context, c identity.Caller, id uuid.UUID) (ControlHistory, error) {
	out := ControlHistory{PageID: id, Versions: []HistoryVersion{}, Items: []HistoryItem{}}
	var spaceID uuid.UUID
	err := s.pool.QueryRow(ctx, "SELECT space_id FROM pages WHERE id = $1", id).Scan(&spaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperr.NotFound()
	}
	if err != nil {
		return out, err
	}
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAudit); err != nil {
		return out, err
	}
	rows, err := s.pool.Query(ctx, `SELECT pv.number, pv.created_at, COALESCE(u.display_name, ''), COALESCE(encode(v.content_hash, 'hex'), '')
		FROM page_versions pv LEFT JOIN version_hashes v ON v.page_id = pv.page_id AND v.number = pv.number LEFT JOIN users u ON u.id = pv.author_id WHERE pv.page_id = $1 ORDER BY pv.number LIMIT 1000`, id)
	if err != nil {
		return out, err
	}
	if out.Versions, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (HistoryVersion, error) {
		var v HistoryVersion
		err := r.Scan(&v.Number, &v.CreatedAt, &v.AuthorName, &v.ContentHash)
		return v, err
	}); err != nil {
		return out, err
	}
	if out.Versions == nil {
		out.Versions = []HistoryVersion{}
	}
	var entries []audit.Entry
	if err := audit.PageEntries(ctx, s.pool, []string{id.String()}, time.Time{}, time.Time{}, func(e audit.Entry) error {
		entries = append(entries, e)
		return nil
	}); err != nil {
		return out, err
	}
	if len(entries) > maxHistoryEvents {
		entries, out.Truncated = entries[len(entries)-maxHistoryEvents:], true
	}
	checked, err := s.checkedSignaturesOf(ctx, s.pool, id)
	if err != nil {
		return out, err
	}
	actors := map[string]uuid.UUID{}
	events := make([]chainedEvent, len(entries))
	for i, e := range entries {
		if err := json.Unmarshal([]byte(e.Payload), &events[i]); err != nil {
			return out, fmt.Errorf("pages: read audit entry %s: %v", e.ID, err)
		}
		if events[i].ActorType == audit.ActorUser {
			if u, err := uuid.Parse(events[i].ActorID); err == nil {
				actors[events[i].ActorID] = u
			}
		}
	}
	names, err := identity.DisplayNames(ctx, s.pool, mapValues(actors))
	if err != nil {
		return out, err
	}
	for i, e := range entries {
		ev := events[i]
		item := HistoryItem{At: e.At, Action: e.Action, Seq: e.Seq, Linked: e.Linked, System: ev.ActorType == audit.ActorSystem,
			Details: map[string]any{}}
		if u, ok := actors[ev.ActorID]; ok {
			item.ActorName = names[u]
		}
		for _, key := range historyDetails {
			if v, ok := ev.Details[key]; ok {
				item.Details[key] = v
			}
		}
		if sigID, ok := ev.Details["signature"].(string); ok && e.Action == "page.signed" {
			if cs, found := checked[sigID]; found {
				item.Signature = &cs
			}
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// checkedSignaturesOf runs the full verification of every signature of a page, by id.
func (s *Service) checkedSignaturesOf(ctx context.Context, q db.Querier, pageID uuid.UUID) (map[string]CheckedSignature, error) {
	records, err := signatureRecordsOf(ctx, q, []uuid.UUID{pageID}, time.Time{}, time.Time{})
	if err != nil {
		return nil, err
	}
	out := map[string]CheckedSignature{}
	for _, rec := range records {
		v, err := verifySignature(ctx, q, rec)
		if err != nil {
			return nil, err
		}
		view := rec.SignatureView
		view.Mark, view.Reason = v.Mark, v.Reason
		out[rec.ID.String()] = CheckedSignature{SignatureView: view, Verification: v}
	}
	return out, nil
}

// signatureRecordsOf reads the whole records of the signatures of some pages within a period.
func signatureRecordsOf(ctx context.Context, q db.Querier, pageIDs []uuid.UUID, since, until time.Time) ([]SignatureRecord, error) {
	rows, err := q.Query(ctx, `SELECT `+signatureRecordColumns+` FROM signatures s
		LEFT JOIN version_hashes v ON v.page_id = s.page_id AND v.number = s.version_number
		WHERE s.page_id = ANY($1) AND ($2::timestamptz IS NULL OR s.signed_at >= $2) AND ($3::timestamptz IS NULL OR s.signed_at < $3)
		ORDER BY s.signed_at, s.id`, pageIDs, optionalTime(since), optionalTime(until))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (SignatureRecord, error) { return scanSignatureRecord(r) })
}

func optionalTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func mapValues(m map[string]uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
