// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/storage"
)

// JobKindExport is the background job that makes an export file.
const JobKindExport = "export"

// The formats a person can export to.
const (
	FormatMarkdown = "markdown"
	FormatText     = "text"
	FormatHTML     = "html"
	FormatWord     = "docx"
	FormatODT      = "odt"
	FormatPDF      = "pdf"
	FormatBundle   = "bundle"
)

// Formats are all the formats, in the order of the dialog.
var Formats = []string{FormatPDF, FormatWord, FormatODT, FormatHTML, FormatMarkdown, FormatText, FormatBundle}

const (
	exportLifetime = 24 * time.Hour
	exportStopped  = "failed"
	exportFinished = "done"
	exportExpired  = "expired"
	uniqueViolated = "23505"
	mebibyte       = 1 << 20
)

// ExportOptions are the choices of the export dialog.
type ExportOptions struct {
	Comments    bool   `json:"comments"`
	Versions    bool   `json:"versions"`
	Attachments bool   `json:"attachments"`
	Restricted  bool   `json:"restricted"`
	Paper       string `json:"paper"`
	Orientation string `json:"orientation"`
	Contents    bool   `json:"contents"`
}

// ExportRequest says what to export. No root and no pages means the whole space.
type ExportRequest struct {
	SpaceID uuid.UUID     `json:"space_id"`
	RootID  *uuid.UUID    `json:"root_id"`
	PageIDs []uuid.UUID   `json:"page_ids"`
	Format  string        `json:"format"`
	Options ExportOptions `json:"options"`
}

// ExportReport says what happened. Nothing in it names a page the person may not
// see: a page restricted a moment ago only counts as skipped.
type ExportReport struct {
	Pages       int      `json:"pages"`
	Skipped     int      `json:"skipped"`
	Restricted  int      `json:"restricted"`
	Drafts      []string `json:"drafts"`
	Failed      []string `json:"failed"`
	OutsideSet  int      `json:"outside_links"`
	Explanation string   `json:"explanation,omitempty"`
}

// ExportRun is an export and its state.
type ExportRun struct {
	ID        uuid.UUID     `json:"id"`
	JobID     uuid.UUID     `json:"job_id"`
	SpaceID   uuid.UUID     `json:"space_id"`
	Format    string        `json:"format"`
	State     string        `json:"state"`
	Filename  string        `json:"filename"`
	Size      int64         `json:"size"`
	Report    ExportReport  `json:"report"`
	Options   ExportOptions `json:"options"`
	CreatedAt time.Time     `json:"created_at"`
	ExpiresAt time.Time     `json:"expires_at"`
	objectKey string
	rootID    *uuid.UUID
	createdBy uuid.UUID
	chosen    []uuid.UUID
}

type exportPayload struct {
	RunID   uuid.UUID   `json:"run_id"`
	PageIDs []uuid.UUID `json:"page_ids"`
}

func validateExport(in *ExportRequest) error {
	if !slices.Contains(Formats, in.Format) || in.Format == FormatBundle {
		return apperr.Invalid("Choose a format.")
	}
	if in.Options.Paper == "" {
		in.Options.Paper = "A4"
	}
	if in.Options.Orientation == "" {
		in.Options.Orientation = "portrait"
	}
	if in.Options.Paper != "A4" && in.Options.Paper != "Letter" {
		return apperr.Invalid("Choose A4 or Letter.")
	}
	if in.Options.Orientation != "portrait" && in.Options.Orientation != "landscape" {
		return apperr.Invalid("Choose portrait or landscape.")
	}
	if len(in.PageIDs) > MaxFiles {
		return apperr.TooLarge("Choose at most 10,000 pages.")
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolated
}

// StartExport checks what it can and queues the job. The job checks everything
// again, because it may run later than this moment.
func (s *Service) StartExport(ctx context.Context, c identity.Caller, in ExportRequest) (ExportRun, error) {
	if err := validateExport(&in); err != nil {
		return ExportRun{}, err
	}
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, in.SpaceID, permissions.SpaceExport); err != nil {
		return ExportRun{}, err
	}
	if in.RootID != nil {
		if err := s.perms.RequirePage(ctx, s.pool, c.UserID, *in.RootID, permissions.ActionPageView, uuid.Nil); err != nil {
			return ExportRun{}, err
		}
	}
	allowed, err := s.mayUseFormat(ctx, c.UserID, in.Format)
	if err != nil {
		return ExportRun{}, err
	}
	if !allowed {
		return ExportRun{}, apperr.Forbidden()
	}
	return s.queueExport(ctx, c, in, JobKindExport)
}

func (s *Service) queueExport(ctx context.Context, c identity.Caller, in ExportRequest, kind string) (ExportRun, error) {
	run := ExportRun{ID: ids.New(), SpaceID: in.SpaceID, Format: in.Format, State: "running", Options: in.Options}
	options, err := json.Marshal(in.Options)
	if err != nil {
		return run, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// A worker that died leaves a run behind: it must not block the next one.
		if _, err := tx.Exec(ctx, `UPDATE export_runs SET state = $2 WHERE created_by = $1 AND state = 'running'
			AND job_id IN (SELECT id FROM jobs WHERE status IN ($3, $4))`, c.UserID, exportStopped, jobs.StatusDone, jobs.StatusFailed); err != nil {
			return err
		}
		var err error
		run.JobID, err = s.jobs.Enqueue(ctx, tx, kind, exportPayload{RunID: run.ID, PageIDs: in.PageIDs}, c.UserID)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `INSERT INTO export_runs (id, job_id, space_id, root_id, format, options, created_by, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, now() + $8::interval) RETURNING created_at, expires_at`,
			run.ID, run.JobID, in.SpaceID, in.RootID, in.Format, options, c.UserID, exportLifetime.String()).Scan(&run.CreatedAt, &run.ExpiresAt)
		if err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("export.created", "space", in.SpaceID.String(), in.SpaceID.String(),
			map[string]any{"format": in.Format, "root": in.RootID, "chosen": len(in.PageIDs), "export": run.ID})); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "export.created", map[string]any{"export_id": run.ID, "space_id": in.SpaceID})
		return err
	})
	if isUniqueViolation(err) {
		return run, apperr.Conflict("export_running", "You already have an export running. Wait for it to end.")
	}
	return run, err
}

func scanRun(row pgx.Row) (ExportRun, error) {
	var r ExportRun
	var report, options []byte
	err := row.Scan(&r.ID, &r.JobID, &r.SpaceID, &r.rootID, &r.Format, &r.State, &r.objectKey, &r.Filename, &r.Size, &report, &options,
		&r.createdBy, &r.CreatedAt, &r.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, apperr.NotFound()
	}
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(report, &r.Report); err != nil {
		return r, fmt.Errorf("exchange: read export report: %v", err)
	}
	if err := json.Unmarshal(options, &r.Options); err != nil {
		return r, fmt.Errorf("exchange: read export options: %v", err)
	}
	return r, nil
}

const runColumns = `id, job_id, space_id, root_id, format, state, object_key, filename, size, report, options, created_by, created_at, expires_at`

// Export returns a person's own export. For anyone else it does not exist.
func (s *Service) Export(ctx context.Context, c identity.Caller, id uuid.UUID) (ExportRun, error) {
	return scanRun(s.pool.QueryRow(ctx, `SELECT `+runColumns+` FROM export_runs WHERE id = $1 AND created_by = $2`, id, c.UserID))
}

// hiddenInRun counts the pages of a file the person can no longer see (GAP-008).
func (s *Service) hiddenInRun(ctx context.Context, c identity.Caller, id uuid.UUID) (int, error) {
	var hidden int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM export_run_pages e
		WHERE e.run_id = $1 AND e.page_id NOT IN (SELECT page_id FROM perm_visible_pages($2))`, id, c.UserID).Scan(&hidden)
	return hidden, err
}

// OpenExport gives the file to the person who made it, after asking again whether
// they may have each page in it (D5). The caller closes the file.
func (s *Service) OpenExport(ctx context.Context, c identity.Caller, id uuid.UUID) (ExportRun, io.ReadCloser, error) {
	run, err := s.Export(ctx, c, id)
	if err != nil {
		return run, nil, err
	}
	if run.State != exportFinished || !time.Now().Before(run.ExpiresAt) {
		return run, nil, apperr.Conflict("export_expired", "This export is no longer available. Make it again.")
	}
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, run.SpaceID, permissions.SpaceExport); err != nil {
		return run, nil, err
	}
	allowed, err := s.mayUseFormat(ctx, c.UserID, run.Format)
	if err != nil {
		return run, nil, err
	}
	if !allowed {
		return run, nil, apperr.Forbidden()
	}
	hidden, err := s.hiddenInRun(ctx, c, id)
	if err != nil {
		return run, nil, err
	}
	if hidden > 0 {
		return run, nil, apperr.Conflict("export_outdated", "You no longer have access to everything in this file. Make a new export.")
	}
	src, err := s.store.Open(ctx, run.objectKey)
	if err != nil {
		return run, nil, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := audit.Record(ctx, tx, c.Event("export.downloaded", "space", run.SpaceID.String(), run.SpaceID.String(),
			map[string]any{"format": run.Format, "export": run.ID})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "export.downloaded", map[string]any{"export_id": run.ID, "space_id": run.SpaceID})
		return err
	})
	if err != nil {
		_ = src.Close()
		return run, nil, err
	}
	return run, src, nil
}

// PurgeExports removes files past their end date.
func (s *Service) PurgeExports(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT `+runColumns+` FROM export_runs WHERE state = $1 AND expires_at <= now()`, exportFinished)
	if err != nil {
		return err
	}
	due, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ExportRun, error) { return scanRun(r) })
	if err != nil {
		return err
	}
	for _, run := range due {
		if err := s.store.Delete(ctx, run.objectKey); err != nil && !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		c := identity.Caller{UserID: run.createdBy, Meta: identity.RequestMeta{RequestID: "task-purge-exports"}}
		err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE export_runs SET state = $2, object_key = '' WHERE id = $1`, run.ID, exportExpired); err != nil {
				return err
			}
			if _, err := audit.Record(ctx, tx, c.Event("export.expired", "space", run.SpaceID.String(), run.SpaceID.String(),
				map[string]any{"export": run.ID})); err != nil {
				return err
			}
			_, err := outbox.Enqueue(ctx, tx, "export.expired", map[string]any{"export_id": run.ID, "space_id": run.SpaceID})
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// exportItem is a page on its way into a file.
type exportItem struct {
	page  pages.ExportPage
	doc   document.Node
	depth int
	name  string // file name without extension, unique in the export
	files map[uuid.UUID]attachments.Attachment
}

func (s *Service) restrictedAmong(ctx context.Context, list []pages.ExportPage) (map[uuid.UUID]bool, error) {
	idList := make([]uuid.UUID, 0, len(list))
	for _, p := range list {
		idList = append(idList, p.ID)
	}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT t.descendant_id FROM page_tree t
		JOIN page_restrictions r ON r.page_id = t.ancestor_id AND r.kind = 'view'
		WHERE t.descendant_id = ANY($1)`, idList)
	if err != nil {
		return nil, err
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]bool, len(found))
	for _, id := range found {
		out[id] = true
	}
	return out, nil
}

func (s *Service) draftTitles(ctx context.Context, c identity.Caller, chosen []uuid.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT title FROM pages WHERE id = ANY($1) AND status = 'draft' AND created_by = $2 ORDER BY title`, chosen, c.UserID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// choosePages decides what goes in the file. It starts from the list the person may
// see now, so a page restricted a second ago is simply not there.
func (s *Service) choosePages(ctx context.Context, c identity.Caller, run ExportRun, report *ExportReport) ([]pages.ExportPage, error) {
	list, err := s.pages.ForExport(ctx, c, run.SpaceID, run.rootID)
	if err != nil {
		return nil, err
	}
	if len(run.chosen) > 0 {
		kept := list[:0:0]
		for _, p := range list {
			if slices.Contains(run.chosen, p.ID) {
				kept = append(kept, p)
			}
		}
		report.Skipped += len(run.chosen) - len(kept)
		list = kept
		drafts, err := s.draftTitles(ctx, c, run.chosen)
		if err != nil {
			return nil, err
		}
		report.Drafts = drafts
		report.Skipped -= len(drafts)
	}
	if run.Options.Restricted {
		return list, nil
	}
	restricted, err := s.restrictedAmong(ctx, list)
	if err != nil {
		return nil, err
	}
	kept := list[:0:0]
	for _, p := range list {
		if restricted[p.ID] {
			report.Restricted++
			continue
		}
		kept = append(kept, p)
	}
	return kept, nil
}

// prepare parses the pages, checking each one again at the moment it is used (D6).
// A page that fails is named, and the rest goes on.
func (s *Service) prepare(ctx context.Context, c identity.Caller, list []pages.ExportPage, report *ExportReport) ([]exportItem, error) {
	depths := map[uuid.UUID]int{}
	used := map[string]bool{}
	items := make([]exportItem, 0, len(list))
	for _, p := range list {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		allowed, err := s.perms.CheckPage(ctx, s.pool, c.UserID, p.ID, permissions.ActionPageView, uuid.Nil)
		if err != nil {
			return nil, err
		}
		if !allowed.Allowed {
			report.Skipped++
			continue
		}
		doc, err := document.Parse(p.Document)
		if err != nil {
			report.Failed = append(report.Failed, p.Title)
			continue
		}
		list, err := s.attachments.ListForPage(ctx, p.ID)
		if err != nil {
			report.Failed = append(report.Failed, p.Title)
			continue
		}
		files := make(map[uuid.UUID]attachments.Attachment, len(list))
		for _, a := range list {
			files[a.ID] = a
		}
		depth := 0
		if p.ParentID != nil {
			if parent, ok := depths[*p.ParentID]; ok {
				depth = parent + 1
			}
		}
		depths[p.ID] = depth
		items = append(items, exportItem{page: p, doc: doc, depth: depth, name: unique(used, safeName(p.Title)), files: files})
	}
	report.Pages = len(items)
	return items, nil
}

// limitedWriter stops a file at the size an administrator allows.
type limitedWriter struct {
	w    io.Writer
	left int64
}

var errExportTooLarge = apperr.TooLarge("The export is larger than the limit an administrator set.")

func (l *limitedWriter) Write(b []byte) (int, error) {
	if int64(len(b)) > l.left {
		return 0, errExportTooLarge
	}
	n, err := l.w.Write(b)
	l.left -= int64(n)
	return n, err
}

func (s *Service) exportTitle(ctx context.Context, run ExportRun, items []exportItem) (string, error) {
	if run.rootID != nil || len(items) == 1 {
		return items[0].page.Title, nil
	}
	var name string
	err := s.pool.QueryRow(ctx, "SELECT name FROM spaces WHERE id = $1", run.SpaceID).Scan(&name)
	return name, err
}

func (s *Service) runExport(ctx context.Context, j jobs.Job) (any, error) {
	var in exportPayload
	if err := json.Unmarshal(j.Payload, &in); err != nil {
		return nil, err
	}
	run, err := scanRun(s.pool.QueryRow(ctx, `SELECT `+runColumns+` FROM export_runs WHERE id = $1`, in.RunID))
	if err != nil {
		return nil, err
	}
	run.chosen = in.PageIDs
	c := identity.Caller{UserID: run.createdBy, Meta: identity.RequestMeta{RequestID: "job-" + j.ID.String()}}
	report := &ExportReport{Drafts: []string{}, Failed: []string{}}
	ok, err := s.buildExport(ctx, c, &run, report)
	if err != nil || !ok {
		if err == nil {
			err = apperr.Invalid("Nothing could be exported.")
		}
		s.failExport(ctx, c, run, report, err)
		return report, err
	}
	return report, nil
}

// failExport records the end of a run. It uses a context of its own, because the
// job's may be the reason for the failure.
func (s *Service) failExport(ctx context.Context, c identity.Caller, run ExportRun, report *ExportReport, cause error) {
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if a, ok := apperr.As(cause); ok {
		report.Explanation = a.Message
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return
	}
	txErr := pgx.BeginFunc(finish, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(finish, `UPDATE export_runs SET state = $2, report = $3 WHERE id = $1`, run.ID, exportStopped, raw); err != nil {
			return err
		}
		if _, err := audit.Record(finish, tx, c.Event("export.failed", "space", run.SpaceID.String(), run.SpaceID.String(),
			map[string]any{"export": run.ID})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(finish, tx, "export.failed", map[string]any{"export_id": run.ID, "space_id": run.SpaceID, "user_id": c.UserID})
		return err
	})
	if txErr != nil {
		s.log.Error("recording a failed export failed", "error", txErr, "export", run.ID)
	}
}

func extensionOf(format string, items []exportItem) string {
	switch format {
	case FormatWord, FormatODT, FormatPDF:
		return "." + format
	case FormatText:
		if len(items) == 1 {
			return ".txt"
		}
	}
	return ".zip"
}

func (s *Service) buildExport(ctx context.Context, c identity.Caller, run *ExportRun, report *ExportReport) (bool, error) {
	list, err := s.choosePages(ctx, c, *run, report)
	if err != nil {
		return false, err
	}
	items, err := s.prepare(ctx, c, list, report)
	if err != nil || len(items) == 0 {
		return false, err
	}
	allowed, err := s.mayUseFormat(ctx, c.UserID, run.Format)
	if err != nil {
		return false, err
	}
	if !allowed {
		return false, apperr.Forbidden()
	}
	limits, err := s.Limits(ctx)
	if err != nil {
		return false, err
	}
	title, err := s.exportTitle(ctx, *run, items)
	if err != nil {
		return false, err
	}
	tmp, err := os.CreateTemp(s.tempDir, "export-*")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.Remove(tmp.Name()); _ = tmp.Close() }()
	maxBytes := int64(limits.MaxExportMiB) * mebibyte
	w := &exportWriter{s: s, c: c, run: *run, items: items, report: report, title: title,
		out: &limitedWriter{w: tmp, left: maxBytes}}
	if err := w.write(ctx); err != nil {
		return false, err
	}
	size, err := tmp.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	run.objectKey = "exports/" + run.ID.String()
	if _, err := s.store.Put(ctx, run.objectKey, tmp, maxBytes); err != nil {
		return false, err
	}
	run.Filename, run.Size = safeName(title)+extensionOf(run.Format, items), size
	return true, s.finishExport(ctx, c, *run, items, report)
}

func (s *Service) finishExport(ctx context.Context, c identity.Caller, run ExportRun, items []exportItem, report *ExportReport) error {
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE export_runs SET state = $2, object_key = $3, filename = $4, size = $5, report = $6,
			expires_at = now() + $7::interval WHERE id = $1`,
			run.ID, exportFinished, run.objectKey, run.Filename, run.Size, raw, exportLifetime.String()); err != nil {
			return err
		}
		for _, item := range items {
			if _, err := tx.Exec(ctx, `INSERT INTO export_run_pages (run_id, page_id) VALUES ($1, $2)`, run.ID, item.page.ID); err != nil {
				return err
			}
		}
		if _, err := audit.Record(ctx, tx, c.Event("export.finished", "space", run.SpaceID.String(), run.SpaceID.String(),
			map[string]any{"format": run.Format, "pages": len(items), "skipped": report.Skipped + report.Restricted, "export": run.ID})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "export.finished", map[string]any{"export_id": run.ID, "space_id": run.SpaceID, "user_id": c.UserID,
			"page_id": items[0].page.ID, "filename": run.Filename})
		return err
	})
}

// missingNote is the list a file carries of what it lacks (G2).
func missingNote(report *ExportReport) string {
	if len(report.Failed) == 0 {
		return ""
	}
	return "These pages could not be exported:\n\n" + strings.Join(report.Failed, "\n") + "\n"
}
