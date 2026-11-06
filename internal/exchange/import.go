// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/storage"
)

// The background jobs of an import: reading a file for the report, and carrying it out.
const (
	JobKindImport     = "markdown_import"
	JobKindImportRead = "import_read"
)

// The kinds of file an import reads.
const (
	KindMarkdown = "markdown"
	KindWord     = "word"
	KindHTML     = "html"
	KindBundle   = "bundle"
)

// The states of an import.
const (
	importReading = "reading"
	importReady   = "ready"
	importRunning = "running"
	importDone    = "done"
	importFailed  = "failed"
	importUndone  = "undone"
)

const (
	maxWarnings     = 200
	reportLifetime  = 24 * time.Hour
	holdingPage     = "Imported Word files"
	finishTimeout   = 10 * time.Second
	maxTitleRunes   = 255
	maxReportedName = 80
)

// ReportPage is a line of the tree a report shows.
type ReportPage struct {
	Title string `json:"title"`
	Depth int    `json:"depth"`
}

// ImportResult is what the user sees before and after an import.
type ImportResult struct {
	Kind     string       `json:"kind"`
	Pages    int          `json:"pages"`
	Files    int          `json:"files"`
	Comments int          `json:"comments"`
	Versions int          `json:"versions"`
	Tree     []ReportPage `json:"tree"`
	Warnings []string     `json:"warnings"`
	// People are the authors a bundle names, to match with people here.
	People []BundlePerson `json:"people,omitempty"`
	// Signer is who signed a bundle, as a fingerprint.
	Signer string `json:"signer,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// ImportRun is an import and its state.
type ImportRun struct {
	ID        uuid.UUID    `json:"id"`
	JobID     uuid.UUID    `json:"job_id"`
	SpaceID   *uuid.UUID   `json:"space_id"`
	ParentID  *uuid.UUID   `json:"parent_id"`
	Kind      string       `json:"kind"`
	State     string       `json:"state"`
	Filename  string       `json:"filename"`
	Report    ImportResult `json:"report"`
	Touched   []string     `json:"touched"`
	CreatedAt time.Time    `json:"created_at"`
	objectKey string
	createdBy uuid.UUID
	peopleMap []byte
}

type importPayload struct {
	RunID uuid.UUID `json:"run_id"`
}

func kindOf(filename string) (string, bool) {
	switch strings.ToLower(path.Ext(filename)) {
	case ".md", ".markdown":
		return KindMarkdown, true
	case ".docx":
		return KindWord, true
	case ".doc":
		return KindWord, true
	case ".html", ".htm":
		return KindHTML, true
	case ".zip":
		return "", true
	}
	return "", false
}

// StartImport stores the upload. With report set it stops at the report, and
// nothing changes until the person says so (EXC-033). Without it, it runs at once,
// which is what scripts use.
func (s *Service) StartImport(ctx context.Context, c identity.Caller, spaceID uuid.UUID, parent *uuid.UUID, filename string, report bool, body io.Reader) (ImportRun, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.PageCreate); err != nil {
		return ImportRun{}, err
	}
	if parent != nil {
		if err := s.perms.RequirePage(ctx, s.pool, c.UserID, *parent, permissions.PageCreate, uuid.Nil); err != nil {
			return ImportRun{}, err
		}
	}
	kind, ok := kindOf(filename)
	if !ok {
		return ImportRun{}, apperr.Invalid("Upload a .md, .docx, .html or .zip file.")
	}
	state, job := importRunning, JobKindImport
	if report {
		state, job = importReading, JobKindImportRead
	}
	run := ImportRun{ID: ids.New(), SpaceID: &spaceID, ParentID: parent, Kind: kind, State: state, Filename: filename, createdBy: c.UserID}
	return s.storeImport(ctx, c, run, job, body)
}

func (s *Service) storeImport(ctx context.Context, c identity.Caller, run ImportRun, job string, body io.Reader) (ImportRun, error) {
	limits, err := s.Limits(ctx)
	if err != nil {
		return run, err
	}
	run.objectKey = "imports/" + run.ID.String()
	if _, err := s.store.Put(ctx, run.objectKey, body, int64(limits.MaxImportMiB)*mebibyte); err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			return run, apperr.TooLarge(fmt.Sprintf("The upload is larger than the limit of %d MiB.", limits.MaxImportMiB))
		}
		return run, err
	}
	object := spaceText(run.SpaceID)
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		run.JobID, err = s.jobs.Enqueue(ctx, tx, job, importPayload{RunID: run.ID}, c.UserID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO import_runs (id, space_id, parent_id, kind, state, object_key, filename, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, run.ID, run.SpaceID, run.ParentID, run.Kind, run.State, run.objectKey, run.Filename, c.UserID); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("import.started", "space", object, object,
			map[string]any{"filename": run.Filename, "job": run.JobID, "import": run.ID})); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "import.started", map[string]any{"job_id": run.JobID, "import_id": run.ID, "space_id": run.SpaceID})
		return err
	})
	if err != nil {
		_ = s.store.Delete(context.WithoutCancel(ctx), run.objectKey)
	}
	return run, err
}

const importColumns = `id, space_id, parent_id, kind, state, object_key, filename, report, people_map, created_by, created_at`

func scanImport(row pgx.Row) (ImportRun, error) {
	var r ImportRun
	var report []byte
	err := row.Scan(&r.ID, &r.SpaceID, &r.ParentID, &r.Kind, &r.State, &r.objectKey, &r.Filename, &report, &r.peopleMap, &r.createdBy, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, apperr.NotFound()
	}
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(report, &r.Report); err != nil {
		return r, fmt.Errorf("exchange: read import report: %v", err)
	}
	return r, nil
}

// Import returns a person's own import, with the pages others touched since. For
// anyone else it does not exist.
func (s *Service) Import(ctx context.Context, c identity.Caller, id uuid.UUID) (ImportRun, error) {
	run, err := scanImport(s.pool.QueryRow(ctx, `SELECT `+importColumns+` FROM import_runs WHERE id = $1 AND created_by = $2`, id, c.UserID))
	if err != nil {
		return run, err
	}
	touched, err := s.touched(ctx, c, id)
	if err != nil {
		return run, err
	}
	run.Touched = make([]string, 0, len(touched))
	for _, t := range touched {
		run.Touched = append(run.Touched, t.title)
	}
	return run, nil
}

// RunImport carries out an import after its report.
func (s *Service) RunImport(ctx context.Context, c identity.Caller, id uuid.UUID) (ImportRun, error) {
	run, err := s.Import(ctx, c, id)
	if err != nil {
		return run, err
	}
	if run.Kind == KindBundle {
		if err := s.checkBundleTrust(ctx, run); err != nil {
			return run, err
		}
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE import_runs SET state = $3 WHERE id = $1 AND state = $2`, id, importReady, importRunning)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("import_not_ready", "This import cannot start now.")
		}
		run.JobID, err = s.jobs.Enqueue(ctx, tx, JobKindImport, importPayload{RunID: id}, c.UserID)
		return err
	})
	if err != nil {
		return run, err
	}
	run.State = importRunning
	return run, nil
}

// entry is one file of an import.
type entry struct {
	name string // cleaned path, forward slashes, never used on disk
	open func() (io.ReadCloser, error)
	size uint64
}

// cleanEntryName normalises a path from a zip. A path that tries to climb out
// is refused. Paths are only used as labels, but a label should not lie.
func cleanEntryName(name string) (string, bool) {
	name = strings.ReplaceAll(name, `\`, "/")
	clean := path.Clean("/" + name)[1:]
	if clean == "" || strings.Contains(name, "\x00") {
		return "", false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", false
		}
	}
	for _, part := range strings.Split(clean, "/") {
		if strings.HasPrefix(part, ".") || part == "__MACOSX" {
			return "", false
		}
	}
	return clean, true
}

// zipEntries checks the limits a zip claims. The real size is enforced again
// while reading, because headers can lie.
func zipEntries(zr *zip.Reader, result *ImportResult, maxUnpacked int64) ([]entry, error) {
	if len(zr.File) > MaxFiles {
		return nil, apperr.TooLarge("The archive has more than 10,000 files.")
	}
	var total uint64
	var out []entry
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		clean, ok := cleanEntryName(f.Name)
		if !ok {
			if strings.Contains(f.Name, "..") && len(result.Warnings) < 100 {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Skipped the unsafe path %q.", truncate(f.Name, maxReportedName)))
			}
			continue
		}
		total += f.UncompressedSize64
		if total > uint64(maxUnpacked) {
			return nil, apperr.TooLarge("The archive unpacks to more than the limit.")
		}
		out = append(out, entry{name: clean, open: f.Open, size: f.UncompressedSize64})
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return strings.ToValidUTF8(s[:n], "")
	}
	return s
}

// openEntries gives the files of an upload. The caller calls done when finished.
func (s *Service) openEntries(ctx context.Context, run ImportRun, result *ImportResult, maxUnpacked int64) ([]entry, func(), error) {
	src, err := s.store.Open(ctx, run.objectKey)
	if err != nil {
		return nil, func() {}, err
	}
	if !strings.HasSuffix(strings.ToLower(run.Filename), ".zip") {
		name := path.Base(strings.ReplaceAll(run.Filename, `\`, "/"))
		spool, err := s.spool(src)
		_ = src.Close()
		if err != nil {
			return nil, func() {}, err
		}
		open := func() (io.ReadCloser, error) { return os.Open(spool) }
		return []entry{{name: name, open: open}}, func() { _ = os.Remove(spool) }, nil
	}
	tmp, err := os.CreateTemp(s.tempDir, "import-*.zip")
	if err != nil {
		_ = src.Close()
		return nil, func() {}, err
	}
	done := func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }
	size, err := io.Copy(tmp, src)
	_ = src.Close()
	if err != nil {
		done()
		return nil, func() {}, err
	}
	zr, err := zip.NewReader(tmp, size)
	if err != nil {
		done()
		return nil, func() {}, apperr.Invalid("The file is not a valid zip archive.")
	}
	entries, err := zipEntries(zr, result, maxUnpacked)
	if err != nil {
		done()
		return nil, func() {}, err
	}
	return entries, done, nil
}

func (s *Service) spool(src io.Reader) (string, error) {
	tmp, err := os.CreateTemp(s.tempDir, "import-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = tmp.Close() }()
	if _, err := io.Copy(tmp, src); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// sink is where a plan goes. The report uses one that writes nothing, so what it
// says comes from the same code that does the work (D8).
type sink interface {
	create(ctx context.Context, parent *uuid.UUID, title string) (uuid.UUID, error)
	publish(ctx context.Context, id uuid.UUID, title string, doc document.Node, base int) (int, error)
	upload(ctx context.Context, pageID uuid.UUID, name string, data []byte) (uuid.UUID, error)
	label(ctx context.Context, id uuid.UUID, label string) error
	stage(ctx context.Context, id uuid.UUID, text string) error
	closeToImporter(ctx context.Context, id uuid.UUID) error
}

func (s *Service) maxUnpacked(ctx context.Context) (int64, error) {
	limits, err := s.Limits(ctx)
	return int64(limits.MaxImportMiB) * mebibyte * unpackFactor, err
}

func spaceText(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func emptyResult() *ImportResult { return &ImportResult{Warnings: []string{}, Tree: []ReportPage{}} }

func callerOf(user uuid.UUID, j jobs.Job) identity.Caller {
	return identity.Caller{UserID: user, Meta: identity.RequestMeta{RequestID: "job-" + j.ID.String()}}
}

// readImport makes the report.
func (s *Service) readImport(ctx context.Context, j jobs.Job) (any, error) {
	var in importPayload
	if err := json.Unmarshal(j.Payload, &in); err != nil {
		return nil, err
	}
	run, err := scanImport(s.pool.QueryRow(ctx, `SELECT `+importColumns+` FROM import_runs WHERE id = $1`, in.RunID))
	if err != nil {
		return nil, err
	}
	result := emptyResult()
	if run.Kind == KindBundle {
		err = s.readBundle(ctx, run, result)
	} else {
		err = s.plan(ctx, callerOf(run.createdBy, j), run, &dryRun{result: result}, result, nil)
	}
	if err != nil {
		s.endImport(ctx, callerOf(run.createdBy, j), run, result, importFailed, err)
		// Nobody can carry this out any more, so the upload has no reason to stay.
		if delErr := s.store.Delete(context.WithoutCancel(ctx), run.objectKey); delErr != nil && !errors.Is(delErr, storage.ErrNotFound) {
			s.log.Error("removing a refused upload failed", "error", delErr, "import", run.ID)
		}
		return result, err
	}
	s.endImport(ctx, callerOf(run.createdBy, j), run, result, importReady, nil)
	return result, nil
}

func (s *Service) runImport(ctx context.Context, j jobs.Job) (any, error) {
	var in importPayload
	if err := json.Unmarshal(j.Payload, &in); err != nil {
		return nil, err
	}
	run, err := scanImport(s.pool.QueryRow(ctx, `SELECT `+importColumns+` FROM import_runs WHERE id = $1`, in.RunID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.store.Delete(context.WithoutCancel(ctx), run.objectKey) }()
	c := callerOf(run.createdBy, j)
	result := emptyResult()
	if run.Kind == KindBundle {
		err = s.writeBundle(ctx, c, &run, result)
	} else {
		items := &itemLog{}
		err = s.plan(ctx, c, run, &liveSink{s: s, c: c, spaceID: *run.SpaceID}, result, items)
		if saveErr := s.saveItems(context.WithoutCancel(ctx), run.ID, items); saveErr != nil && err == nil {
			err = saveErr
		}
	}
	state := importDone
	if err != nil {
		state = importFailed
	}
	s.endImport(ctx, c, run, result, state, err)
	return result, err
}

func (s *Service) saveItems(ctx context.Context, runID uuid.UUID, items *itemLog) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, item := range items.list {
			if _, err := tx.Exec(ctx, `INSERT INTO import_items (run_id, page_id, source, version) VALUES ($1, $2, $3, $4)
				ON CONFLICT (run_id, page_id) DO UPDATE SET version = EXCLUDED.version`, runID, item.page, item.source, item.version); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) endImport(ctx context.Context, c identity.Caller, run ImportRun, result *ImportResult, state string, cause error) {
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
	defer cancel()
	if a, ok := apperr.As(cause); ok {
		result.Reason = a.Message
	}
	raw, err := json.Marshal(result)
	if err != nil {
		s.log.Error("recording an import failed", "error", err, "import", run.ID)
		return
	}
	object := spaceText(run.SpaceID)
	err = pgx.BeginFunc(finish, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(finish, `UPDATE import_runs SET state = $2, report = $3, finished_at = now() WHERE id = $1`, run.ID, state, raw); err != nil {
			return err
		}
		action := "import.finished"
		if state == importReady {
			action = "import.read"
		}
		ev := c.Event(action, "space", object, object, map[string]any{"import": run.ID, "pages": result.Pages, "files": result.Files, "ok": cause == nil})
		if _, err := audit.Record(finish, tx, ev); err != nil {
			return err
		}
		_, err := outbox.Enqueue(finish, tx, action, map[string]any{"import_id": run.ID, "space_id": run.SpaceID})
		return err
	})
	if err != nil {
		s.log.Error("recording an import failed", "error", err, "import", run.ID)
	}
}

// PurgeImports removes uploads that were read and never carried out.
func (s *Service) PurgeImports(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT `+importColumns+` FROM import_runs WHERE state = $1 AND finished_at < now() - $2::interval`,
		importReady, reportLifetime.String())
	if err != nil {
		return err
	}
	due, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ImportRun, error) { return scanImport(r) })
	if err != nil {
		return err
	}
	for _, run := range due {
		if err := s.store.Delete(ctx, run.objectKey); err != nil && !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		if _, err := s.pool.Exec(ctx, `UPDATE import_runs SET state = $2, object_key = '' WHERE id = $1`, run.ID, importFailed); err != nil {
			return err
		}
	}
	return nil
}

// touchedPage is an imported page that someone else has changed since.
type touchedPage struct {
	id    uuid.UUID
	title string
}

// touched lists imported pages someone worked on since: a newer version, a comment
// by another person, or a page of theirs below it (D10).
func (s *Service) touched(ctx context.Context, c identity.Caller, runID uuid.UUID) ([]touchedPage, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.title FROM import_items i JOIN pages p ON p.id = i.page_id
		WHERE i.run_id = $1 AND p.status <> 'trashed' AND (
			p.current_version <> i.version
			OR EXISTS (SELECT 1 FROM comments m WHERE m.page_id = p.id AND m.author_id <> $2 AND m.trash_item_id IS NULL)
			OR EXISTS (SELECT 1 FROM page_tree t JOIN pages d ON d.id = t.descendant_id
				WHERE t.ancestor_id = p.id AND t.depth > 0 AND d.status <> 'trashed'
				AND d.id NOT IN (SELECT page_id FROM import_items WHERE run_id = $1)))
		ORDER BY p.title`, runID, c.UserID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (touchedPage, error) {
		var t touchedPage
		err := r.Scan(&t.id, &t.title)
		return t, err
	})
}

func (s *Service) keptByUndo(ctx context.Context, runID uuid.UUID, touched []touchedPage) (map[uuid.UUID]bool, error) {
	kept := map[uuid.UUID]bool{}
	list := make([]uuid.UUID, 0, len(touched))
	for _, t := range touched {
		list = append(list, t.id)
	}
	// A page above one that stays must stay too, or the trash would take it along.
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT t.ancestor_id FROM page_tree t
		JOIN import_items i ON i.page_id = t.ancestor_id AND i.run_id = $1 WHERE t.descendant_id = ANY($2)`, runID, list)
	if err != nil {
		return nil, err
	}
	above, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	for _, id := range above {
		kept[id] = true
	}
	return kept, nil
}

// UndoResult says what an undo did.
type UndoResult struct {
	Undone int      `json:"undone"`
	Kept   []string `json:"kept"`
}

// UndoImport moves the pages of an import to the trash (D10). When someone else
// worked on some of them it refuses, unless the person asked to undo the rest.
func (s *Service) UndoImport(ctx context.Context, c identity.Caller, id uuid.UUID, rest bool) (UndoResult, error) {
	run, err := s.Import(ctx, c, id)
	if err != nil {
		return UndoResult{}, err
	}
	if run.Kind == KindBundle || (run.State != importDone && run.State != importFailed) {
		return UndoResult{}, apperr.Conflict("import_not_done", "This import cannot be undone now.")
	}
	touched, err := s.touched(ctx, c, id)
	if err != nil {
		return UndoResult{}, err
	}
	if len(touched) > 0 && !rest {
		return UndoResult{}, apperr.Conflict("import_touched", "Others worked on some of these pages.")
	}
	kept, err := s.keptByUndo(ctx, id, touched)
	if err != nil {
		return UndoResult{}, err
	}
	// The deepest pages first, so each page leaves on its own.
	rows, err := s.pool.Query(ctx, `SELECT p.id FROM import_items i JOIN pages p ON p.id = i.page_id
		WHERE i.run_id = $1 AND p.status <> 'trashed'
		ORDER BY (SELECT count(*) FROM page_tree a WHERE a.descendant_id = p.id) DESC`, id)
	if err != nil {
		return UndoResult{}, err
	}
	list, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return UndoResult{}, err
	}
	out := UndoResult{Kept: run.Touched}
	for _, page := range list {
		if kept[page] {
			continue
		}
		if err := s.pages.Delete(ctx, c, page); err != nil {
			return out, err
		}
		out.Undone++
	}
	object := spaceText(run.SpaceID)
	return out, pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE import_runs SET state = $2 WHERE id = $1`, id, importUndone); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("import.undone", "space", object, object,
			map[string]any{"import": id, "pages": out.Undone, "kept": len(touched)})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "import.undone", map[string]any{"import_id": id, "space_id": run.SpaceID})
		return err
	})
}

// sortShallowFirst puts a parent before its children.
func sortShallowFirst(list []planPage) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := strings.Count(list[i].key, "/"), strings.Count(list[j].key, "/")
		if a != b {
			return a < b
		}
		return list[i].key < list[j].key
	})
}

func isAny(name string, extensions ...string) bool {
	return slices.Contains(extensions, strings.ToLower(path.Ext(name)))
}
