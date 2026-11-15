// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// The auditor's export (DOC-1020, DOC-1021, DOC-742): the record of document control of one
// page, a branch or a whole space for a period, as JSON Lines, one object per line. It holds
// every version with its content hash, the rounds with their reviewers, answers and comments,
// every signature with its record and verification, the workflow versions that applied, and
// the audit entries of all of these with their place in the chain. For each signed version
// it holds the content its hash covers, so an auditor can verify the signatures outside
// Diffluence with docs/operations/regulated-use.md.

// AuditorExportFormat is the version of the layout of the export.
const AuditorExportFormat = 1

// maxExportPages bounds an export made while the request waits. A larger scope is narrowed by
// a branch or a period.
const maxExportPages = 1000

// AuditorExportInput is what to export. No page means the whole space; Branch adds the
// pages below the page. Since and until may be zero.
type AuditorExportInput struct {
	PageID *uuid.UUID
	Branch bool
	Since  time.Time
	Until  time.Time
}

// AuditorExport is an export that was checked and recorded, ready to be written.
type AuditorExport struct {
	s       *Service
	c       identity.Caller
	spaceID uuid.UUID
	in      AuditorExportInput
	pages   []uuid.UUID
	scope   string
}

// PrepareAuditorExport checks an export and records it in the audit log, with its scope and
// period, before any of it is written (DOC-1021).
func (s *Service) PrepareAuditorExport(ctx context.Context, c identity.Caller, spaceID uuid.UUID, in AuditorExportInput) (*AuditorExport, error) {
	if !in.Since.IsZero() && !in.Until.IsZero() && !in.Since.Before(in.Until) {
		return nil, apperr.Invalid("The period ends before it starts.")
	}
	e := &AuditorExport{s: s, c: c, spaceID: spaceID, in: in, scope: "space"}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceAudit); err != nil {
			return err
		}
		var err error
		switch {
		case in.PageID != nil && in.Branch:
			e.scope = "branch"
			e.pages, err = collectIDs(tx.Query(ctx, `SELECT t.descendant_id FROM page_tree t JOIN pages p ON p.id = t.descendant_id
				WHERE t.ancestor_id = $1 AND p.space_id = $2 ORDER BY t.depth, t.descendant_id LIMIT $3`, *in.PageID, spaceID, maxExportPages+1))
		case in.PageID != nil:
			e.scope = "page"
			e.pages, err = collectIDs(tx.Query(ctx, "SELECT id FROM pages WHERE id = $1 AND space_id = $2", *in.PageID, spaceID))
		default:
			// The pages of the space that are or were under document control.
			e.pages, err = collectIDs(tx.Query(ctx, `SELECT p.id FROM pages p WHERE p.space_id = $1
				AND (EXISTS (SELECT 1 FROM page_control pc WHERE pc.page_id = p.id)
				  OR EXISTS (SELECT 1 FROM workflow_rounds r WHERE r.page_id = p.id)
				  OR EXISTS (SELECT 1 FROM signatures sg WHERE sg.page_id = p.id))
				ORDER BY p.id LIMIT $2`, spaceID, maxExportPages+1))
		}
		if err != nil {
			return err
		}
		if in.PageID != nil && len(e.pages) == 0 {
			return apperr.NotFound()
		}
		if len(e.pages) > maxExportPages {
			return apperr.Conflict("export_too_large", "This export holds more than 1000 pages. Choose a branch or a page.")
		}
		return recordAudit(ctx, tx, c, "space.auditor_export", "space", spaceID, spaceID, map[string]any{
			"scope": e.scope, "page": in.PageID, "since": optionalTime(in.Since), "until": optionalTime(in.Until), "pages": len(e.pages)})
	})
	return e, err
}

func collectIDs(rows pgx.Rows, err error) ([]uuid.UUID, error) {
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// exportLine is one object of the export, with its type first.
type exportLine map[string]any

// Write writes the export. Its last line says how many lines came before it, so a reader
// knows an export that ended early.
func (e *AuditorExport) Write(ctx context.Context, w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	lines := 0
	put := func(kind string, v any) error {
		lines++
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		line := exportLine{}
		if err := json.Unmarshal(raw, &line); err != nil {
			return err
		}
		line["type"] = kind
		return enc.Encode(line)
	}
	if err := put("export", map[string]any{"format": AuditorExportFormat, "content_forms": []int{ContentForm},
		"installation": e.s.instanceID, "space_id": e.spaceID, "scope": e.scope, "page_id": e.in.PageID,
		"since": optionalTime(e.in.Since), "until": optionalTime(e.in.Until), "generated_at": time.Now().UTC(), "pages": len(e.pages)}); err != nil {
		return err
	}
	workflows := map[workflowRef]bool{}
	for _, id := range e.pages {
		if err := e.writePage(ctx, id, put, workflows); err != nil {
			return err
		}
	}
	if err := e.writeWorkflows(ctx, workflows, put); err != nil {
		return err
	}
	ids := make([]string, len(e.pages))
	for i, id := range e.pages {
		ids[i] = id.String()
	}
	if err := audit.PageEntries(ctx, e.s.pool, ids, e.in.Since, e.in.Until, func(entry audit.Entry) error {
		return put("audit_entry", entry)
	}); err != nil {
		return err
	}
	return put("end", map[string]any{"lines": lines})
}

// writePage writes what the export holds of one page. A page the exporter may not read gives
// its record without its title and text: reading the audit of a space is not reading every
// page of it.
func (e *AuditorExport) writePage(ctx context.Context, id uuid.UUID, put func(string, any) error, workflows map[workflowRef]bool) error {
	q := e.s.pool
	d, err := e.s.perms.CheckPage(ctx, q, e.c.UserID, id, permissions.ActionPageView, uuid.Nil)
	if err != nil {
		return err
	}
	readable := d.Allowed
	var title string
	page := map[string]any{"page_id": id, "readable": readable}
	var ctl struct {
		workflow        *uuid.UUID
		workflowVersion *int
		state, kind     *string
		approved, head  *int
		approvedAt      *time.Time
		approvedBy      *uuid.UUID
	}
	if err := q.QueryRow(ctx, `SELECT p.title, pc.workflow_id, pc.workflow_version, pc.state_id, pc.state_kind, pc.approved_version,
			pc.approved_at, pc.approved_by, pc.head_version
		FROM pages p LEFT JOIN page_control pc ON pc.page_id = p.id WHERE p.id = $1`, id).
		Scan(&title, &ctl.workflow, &ctl.workflowVersion, &ctl.state, &ctl.kind, &ctl.approved, &ctl.approvedAt, &ctl.approvedBy, &ctl.head); err != nil {
		return err
	}
	if !readable {
		title = audit.CensoredTitle
	}
	page["title"], page["workflow_id"], page["workflow_version"], page["state_id"], page["state_kind"] = title, ctl.workflow, ctl.workflowVersion, ctl.state, ctl.kind
	page["approved_version"], page["approved_at"], page["approved_by"], page["head_version"] = ctl.approved, ctl.approvedAt, ctl.approvedBy, ctl.head
	if ctl.workflow != nil {
		workflows[workflowRef{*ctl.workflow, *ctl.workflowVersion}] = true
	}
	if err := put("page", page); err != nil {
		return err
	}
	records, err := signatureRecordsOf(ctx, q, []uuid.UUID{id}, e.in.Since, e.in.Until)
	if err != nil {
		return err
	}
	signed := map[int]bool{}
	for _, rec := range records {
		signed[rec.Version] = true
	}
	if err := e.writeVersions(ctx, id, readable, signed, put); err != nil {
		return err
	}
	if err := e.writeRounds(ctx, id, readable, put, workflows); err != nil {
		return err
	}
	for _, rec := range records {
		v, err := verifySignature(ctx, q, rec)
		if err != nil {
			return err
		}
		rec.Mark, rec.Reason = v.Mark, v.Reason
		if err := put("signature", map[string]any{"record": rec, "verification": v}); err != nil {
			return err
		}
		workflows[workflowRef{rec.WorkflowID, rec.WorkflowVersion}] = true
	}
	return nil
}

// writeVersions writes every version of the period with its content hash: the one it keeps
// once signed, or the one its content gives now. A signed version carries its content.
func (e *AuditorExport) writeVersions(ctx context.Context, id uuid.UUID, readable bool, signed map[int]bool, put func(string, any) error) error {
	rows, err := e.s.pool.Query(ctx, `SELECT v.number, v.created_at, v.author_id, COALESCE(u.display_name, ''),
			COALESCE(encode(h.content_hash, 'hex'), ''), h.content_form::int
		FROM page_versions v LEFT JOIN version_hashes h ON h.page_id = v.page_id AND h.number = v.number
		LEFT JOIN users u ON u.id = v.author_id
		WHERE v.page_id = $1 AND ($2::timestamptz IS NULL OR v.created_at >= $2) AND ($3::timestamptz IS NULL OR v.created_at < $3)
		ORDER BY v.number`, id, optionalTime(e.in.Since), optionalTime(e.in.Until))
	if err != nil {
		return err
	}
	type version struct {
		Number      int       `json:"number"`
		CreatedAt   time.Time `json:"created_at"`
		AuthorID    uuid.UUID `json:"author_id"`
		AuthorName  string    `json:"author_name"`
		ContentHash string    `json:"content_hash"`
		ContentForm *int      `json:"content_form"`
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[version])
	if err != nil {
		return err
	}
	for _, v := range list {
		line := map[string]any{"page_id": id, "number": v.Number, "created_at": v.CreatedAt, "author_id": v.AuthorID,
			"author_name": v.AuthorName, "content_hash": v.ContentHash, "content_form": v.ContentForm, "hash_kept": v.ContentHash != "",
			"signed": signed[v.Number]}
		content, err := signedContentOf(ctx, e.s.pool, id, v.Number)
		if err != nil {
			return err
		}
		if v.ContentHash == "" {
			line["content_hash"], line["content_form"] = hexHash(content.Hash()), ContentForm
		}
		if signed[v.Number] && readable {
			line["content"] = content
		}
		if err := put("version", line); err != nil {
			return err
		}
	}
	return nil
}

// writeRounds writes the rounds of the period with their answers, reviewers and comments. What a
// person wrote in a round is text of the page, so a page the exporter may not read gives none.
func (e *AuditorExport) writeRounds(ctx context.Context, id uuid.UUID, readable bool, put func(string, any) error, workflows map[workflowRef]bool) error {
	q := e.s.pool
	since, until := optionalTime(e.in.Since), optionalTime(e.in.Until)
	rows, err := q.Query(ctx, `SELECT id, workflow_id, workflow_version, version_number, status, ended_reason, started_at, ended_at, review_stage,
			imported_from IS NOT NULL
		FROM workflow_rounds WHERE page_id = $1 AND ($2::timestamptz IS NULL OR started_at >= $2) AND ($3::timestamptz IS NULL OR started_at < $3)
		ORDER BY started_at, id`, id, since, until)
	if err != nil {
		return err
	}
	type round struct {
		ID              uuid.UUID  `json:"id"`
		WorkflowID      uuid.UUID  `json:"workflow_id"`
		WorkflowVersion int        `json:"workflow_version"`
		Version         int        `json:"version"`
		Status          string     `json:"status"`
		EndedReason     string     `json:"ended_reason"`
		StartedAt       time.Time  `json:"started_at"`
		EndedAt         *time.Time `json:"ended_at"`
		Stages          int        `json:"stages"`
		Imported        bool       `json:"imported"`
	}
	rounds, err := pgx.CollectRows(rows, pgx.RowToStructByPos[round])
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, len(rounds))
	for i, r := range rounds {
		ids[i] = r.ID
		workflows[workflowRef{r.WorkflowID, r.WorkflowVersion}] = true
		if err := put("round", map[string]any{"page_id": id, "round": r}); err != nil {
			return err
		}
	}
	// Answers of the period, the system's moves without a round among them.
	if err := e.writeJSONRows(ctx, "answer", `SELECT json_build_object('id', a.id, 'round_id', a.round_id, 'page_id', a.page_id,
			'user_id', a.user_id, 'transition', a.transition_id, 'from', a.from_state, 'to', a.to_state, 'version', a.version_number,
			'comment', CASE WHEN $4 THEN a.comment ELSE '' END, 'cause', a.cause, 'at', a.answered_at)
		FROM workflow_answers a WHERE a.page_id = $1
		  AND ($2::timestamptz IS NULL OR a.answered_at >= $2) AND ($3::timestamptz IS NULL OR a.answered_at < $3) ORDER BY a.answered_at, a.id`,
		put, id, since, until, readable); err != nil {
		return err
	}
	// Who the rounds of the period asked, and what they said.
	if err := e.writeJSONRows(ctx, "reviewer", `SELECT json_build_object('round_id', r.round_id, 'stage', r.stage, 'state', r.state_id,
			'user_id', r.user_id, 'source', r.source_type, 'asked_at', r.asked_at, 'answer', r.answer,
			'comment', CASE WHEN $2 THEN r.comment ELSE '' END,
			'answered_at', r.answered_at, 'removed_at', r.removed_at, 'removed_by', r.removed_by, 'removed_reason', r.removed_reason,
			'signature_id', r.signature_id)
		FROM workflow_reviewers r WHERE r.round_id = ANY($1) ORDER BY r.round_id, r.stage, r.asked_at, r.user_id`, put, ids, readable); err != nil {
		return err
	}
	if !readable {
		return nil
	}
	return e.writeJSONRows(ctx, "review_comment", `SELECT json_build_object('id', c.id, 'round_id', c.round_id, 'user_id', c.user_id,
			'body', c.body, 'at', c.created_at)
		FROM workflow_review_comments c WHERE c.round_id = ANY($1) ORDER BY c.created_at, c.id`, put, ids)
}
func (e *AuditorExport) writeJSONRows(ctx context.Context, kind, sql string, put func(string, any) error, args ...any) error {
	rows, err := e.s.pool.Query(ctx, sql, args...)
	if err != nil {
		return err
	}
	list, err := pgx.CollectRows(rows, pgx.RowTo[map[string]any])
	if err != nil {
		return err
	}
	for _, v := range list {
		if err := put(kind, v); err != nil {
			return err
		}
	}
	return nil
}

// workflowRef names one version of a workflow.
type workflowRef struct {
	id      uuid.UUID
	version int
}

// writeWorkflows writes each workflow version the pages followed or were signed under.
func (e *AuditorExport) writeWorkflows(ctx context.Context, used map[workflowRef]bool, put func(string, any) error) error {
	refs := make([]workflowRef, 0, len(used))
	for ref := range used {
		refs = append(refs, ref)
	}
	slices.SortFunc(refs, func(a, b workflowRef) int {
		if c := strings.Compare(a.id.String(), b.id.String()); c != 0 {
			return c
		}
		return a.version - b.version
	})
	for _, ref := range refs {
		var name string
		var def json.RawMessage
		var saved time.Time
		if err := e.s.pool.QueryRow(ctx, `SELECT w.name, v.definition, v.saved_at FROM workflow_versions v JOIN workflows w ON w.id = v.workflow_id
			WHERE v.workflow_id = $1 AND v.version = $2`, ref.id, ref.version).Scan(&name, &def, &saved); err != nil {
			return err
		}
		if err := put("workflow_version", map[string]any{"workflow_id": ref.id, "version": ref.version, "name": name, "definition": def,
			"saved_at": saved}); err != nil {
			return err
		}
	}
	return nil
}
