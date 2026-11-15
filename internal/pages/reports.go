// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Reports of document control (spec 064 wave DC5, DOC-804, DOC-823, DOC-1030, DOC-1031): for a
// space with space.audit, and across every space where the person holds it. Each runs on the
// read path under the read limit of the screens (DOC-1305) and returns at most maxReportRows
// rows. A page the person may not read is listed without its title, as in the auditor's export.

// The kinds of report.
const (
	ReportStates    = "states"
	ReportOverdue   = "overdue"
	ReportPeriodic  = "periodic"
	ReportRead      = "read"
	ReportApprovals = "approvals"
)

// maxReportRows bounds every report; a review date within 30 days counts as coming (DOC-804).
const maxReportRows = 5000

// Report is one report: its columns in order and its rows.
type Report struct {
	Kind    string           `json:"kind"`
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

// ReportInput chooses the report, the space (nil for every space the person audits), and for
// approvals a period.
type ReportInput struct {
	Kind    string
	SpaceID *uuid.UUID
	From    *time.Time
	To      *time.Time
}

type reportQuery struct {
	columns []string
	sql     string
}

// The spaces of a report are bound as $1 (the person) and $2 (the spaces); the titles of pages
// the person may not read stay empty.
var reportQueries = map[string]reportQuery{
	ReportStates: {[]string{"space_id", "page_id", "title", "workflow", "state", "state_kind", "approved_version", "head_version", "approved_at"},
		`SELECT p.space_id, p.id, CASE WHEN v.page_id IS NULL THEN '' ELSE p.title END, w.name, pc.state_id, pc.state_kind,
			pc.approved_version, pc.head_version, pc.approved_at
		FROM page_control pc JOIN pages p ON p.id = pc.page_id JOIN workflows w ON w.id = pc.workflow_id
		LEFT JOIN perm_visible_pages($1) v ON v.page_id = p.id
		WHERE p.space_id = ANY($2) AND p.status = 'published' ORDER BY p.space_id, pc.state_kind, p.title LIMIT $3`},
	ReportOverdue: {[]string{"space_id", "page_id", "title", "due_at", "stage", "reminders", "escalated"},
		`SELECT p.space_id, p.id, CASE WHEN v.page_id IS NULL THEN '' ELSE p.title END, d.due_at, d.stage, d.reminders, d.escalated_at IS NOT NULL
		FROM review_deadlines d JOIN workflow_rounds r ON r.id = d.round_id AND r.status = 'open' AND r.review_stage = d.stage
		JOIN pages p ON p.id = d.page_id LEFT JOIN perm_visible_pages($1) v ON v.page_id = p.id
		WHERE p.space_id = ANY($2) AND d.due_at <= now() ORDER BY d.due_at LIMIT $3`},
	ReportPeriodic: {[]string{"space_id", "page_id", "title", "due_at", "status", "owner_gone"},
		`SELECT p.space_id, p.id, CASE WHEN v.page_id IS NULL THEN '' ELSE p.title END, pr.due_at,
			CASE WHEN pr.due_at <= now() THEN 'overdue' WHEN pr.due_at <= now() + interval '30 days' THEN 'due_soon' ELSE 'current' END,
			NOT EXISTS (SELECT 1 FROM users o WHERE o.id = COALESCE(p.owner_id, p.created_by) AND o.status = 'active')
		FROM periodic_reviews pr JOIN pages p ON p.id = pr.page_id LEFT JOIN perm_visible_pages($1) v ON v.page_id = p.id
		WHERE p.space_id = ANY($2) ORDER BY pr.due_at LIMIT $3`},
	ReportRead: {[]string{"space_id", "page_id", "title", "version", "asked", "confirmed"},
		`SELECT p.space_id, p.id, CASE WHEN v.page_id IS NULL THEN '' ELSE p.title END, rq.version_number,
			(SELECT count(*) FROM read_confirmations rc WHERE rc.page_id = rq.page_id AND rc.version_number = rq.version_number),
			(SELECT count(*) FROM read_confirmations rc WHERE rc.page_id = rq.page_id AND rc.version_number = rq.version_number AND rc.confirmed_at IS NOT NULL)
		FROM read_requests rq JOIN pages p ON p.id = rq.page_id LEFT JOIN perm_visible_pages($1) v ON v.page_id = p.id
		WHERE p.space_id = ANY($2) ORDER BY p.space_id, p.title, rq.version_number DESC LIMIT $3`},
	ReportApprovals: {[]string{"space_id", "user_id", "name", "approvals"},
		`SELECT p.space_id, wr.user_id, COALESCE(u.display_name, ''), count(*)
		FROM workflow_reviewers wr JOIN workflow_rounds r ON r.id = wr.round_id JOIN pages p ON p.id = r.page_id
		LEFT JOIN users u ON u.id = wr.user_id
		WHERE p.space_id = ANY($2) AND wr.answer = 'approve' AND r.imported_from IS NULL
		  AND wr.answered_at >= COALESCE($4::timestamptz, '-infinity') AND wr.answered_at < COALESCE($5::timestamptz, 'infinity') AND $1::uuid IS NOT NULL
		GROUP BY p.space_id, wr.user_id, u.display_name ORDER BY p.space_id, count(*) DESC LIMIT $3`},
}

// ControlReport makes one report.
func (s *Service) ControlReport(ctx context.Context, c identity.Caller, in ReportInput) (Report, error) {
	q, ok := reportQueries[in.Kind]
	if !ok {
		return Report{}, apperr.NotFound()
	}
	var spaces []uuid.UUID
	if in.SpaceID != nil {
		if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, *in.SpaceID, permissions.SpaceAudit); err != nil {
			return Report{}, err
		}
		spaces = []uuid.UUID{*in.SpaceID}
	} else {
		rows, err := s.pool.Query(ctx, "SELECT space_id FROM perm_spaces($1, 'space.audit')", c.UserID)
		if err != nil {
			return Report{}, err
		}
		if spaces, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID]); err != nil {
			return Report{}, err
		}
	}
	args := []any{c.UserID, spaces, maxReportRows}
	if in.Kind == ReportApprovals {
		args = append(args, in.From, in.To)
	}
	rows, err := s.pool.Query(ctx, q.sql, args...)
	if err != nil {
		return Report{}, err
	}
	out := Report{Kind: in.Kind, Columns: q.columns, Rows: []map[string]any{}}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			rows.Close()
			return Report{}, err
		}
		row := make(map[string]any, len(q.columns))
		for i, col := range q.columns {
			row[col] = reportValue(values[i])
		}
		out.Rows = append(out.Rows, row)
	}
	return out, rows.Err()
}

// reportValue makes a database value plain for JSON and CSV.
func reportValue(v any) any {
	switch x := v.(type) {
	case [16]byte:
		return uuid.UUID(x)
	case time.Time:
		return x.UTC()
	}
	return v
}
