// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/audit"
	"github.com/kenwerk/kenwerk/internal/csvout"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// The export of the release history of a pair for a period (PUB-913): each run and each page,
// both sides' versions and the content hashes, as CSV for people and JSON Lines for tools.

// Export formats.
const (
	FormatCSV   = "csv"
	FormatLines = "jsonl"
)

// maxExportRows bounds one export; a longer history is exported by shorter periods.
const maxExportRows = 200000

// ExportRow is one page of one run in the export.
type ExportRow struct {
	RunID        uuid.UUID  `json:"run_id"`
	Scope        string     `json:"scope"`
	Actor        string     `json:"actor"`
	FinishedAt   time.Time  `json:"finished_at"`
	Note         string     `json:"note"`
	DraftPageID  uuid.UUID  `json:"draft_page_id"`
	DraftVersion int        `json:"draft_version"`
	ContentHash  string     `json:"content_hash"`
	TwinPageID   *uuid.UUID `json:"twin_page_id"`
	TwinVersion  *int       `json:"twin_version"`
	TwinHash     string     `json:"twin_hash"`
	Action       string     `json:"action"`
	Outcome      string     `json:"outcome"`
}

// ExportHistory writes the release history of the pair of a draft space between two moments.
// It needs space.audit in the draft space, and making it is audited (PUB-913).
func (s *Service) ExportHistory(ctx context.Context, c identity.Caller, spaceID uuid.UUID, from, to time.Time, format string, w io.Writer) error {
	if format != FormatCSV && format != FormatLines {
		return apperr.Invalid("The release history is exported as csv or jsonl.")
	}
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAudit); err != nil {
		return err
	}
	p, err := activePairOf(ctx, s.pool, spaceID, false)
	if err != nil {
		return err
	}
	if p.Draft != spaceID {
		return apperr.Conflict("not_the_draft_space", "The release history is kept in the draft space.")
	}
	rows, err := s.pool.Query(ctx, `SELECT r.id, r.scope, COALESCE(u.display_name, ''), r.finished_at, r.note, rp.draft_page_id, rp.draft_version,
			rp.content_hash, rp.twin_page_id, rp.twin_version, COALESCE(page_pair_content_hash(v.document), ''), rp.action, rp.outcome
		FROM release_runs r JOIN release_run_pages rp ON rp.run_id = r.id
		LEFT JOIN users u ON u.id = r.actor_id
		LEFT JOIN page_versions v ON v.page_id = rp.twin_page_id AND v.number = rp.twin_version
		WHERE r.pair_id = $1 AND r.finished_at >= $2 AND r.finished_at < $3
		ORDER BY r.finished_at, r.id, rp.ord, rp.draft_page_id LIMIT $4`, p.ID, from, to, maxExportRows)
	if err != nil {
		return err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ExportRow, error) {
		var e ExportRow
		err := r.Scan(&e.RunID, &e.Scope, &e.Actor, &e.FinishedAt, &e.Note, &e.DraftPageID, &e.DraftVersion, &e.ContentHash,
			&e.TwinPageID, &e.TwinVersion, &e.TwinHash, &e.Action, &e.Outcome)
		return e, err
	})
	if err != nil {
		return err
	}
	if err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := audit.Record(ctx, tx, c.Event("release.history_exported", "space", p.Draft.String(), p.Draft.String(),
			map[string]any{"pair_id": p.ID, "from": from, "to": to, "format": format, "rows": len(list)}))
		return err
	}); err != nil {
		return err
	}
	if format == FormatLines {
		enc := json.NewEncoder(w)
		for _, e := range list {
			if err := enc.Encode(e); err != nil {
				return err
			}
		}
		return nil
	}
	out, err := csvout.New(w, "run", "scope", "released by", "finished", "note", "draft page", "draft version", "content hash",
		"published page", "published version", "published content hash", "action", "outcome")
	if err != nil {
		return err
	}
	for _, e := range list {
		twin, version := "", ""
		if e.TwinPageID != nil {
			twin = e.TwinPageID.String()
		}
		if e.TwinVersion != nil {
			version = strconv.Itoa(*e.TwinVersion)
		}
		if err := out.Row(e.RunID.String(), e.Scope, e.Actor, e.FinishedAt.UTC().Format(time.RFC3339), e.Note, e.DraftPageID.String(),
			strconv.Itoa(e.DraftVersion), e.ContentHash, twin, version, e.TwinHash, e.Action, e.Outcome); err != nil {
			return err
		}
	}
	return out.Close()
}

// ExportName is the file name of an export of the release history.
func ExportName(format string, from, to time.Time) string {
	return fmt.Sprintf("release-history-%s-%s.%s", from.UTC().Format("20060102"), to.UTC().Format("20060102"), format)
}
