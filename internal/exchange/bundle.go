// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"archive/zip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
)

// JobKindBundle is the background job that writes a bundle.
const JobKindBundle = "bundle_export"

const (
	bundleFormat   = "diffluence-bundle"
	bundleVersion  = 1
	manifestFile   = "manifest.json"
	signatureFile  = "manifest.sig"
	spaceFile      = "space.json"
	pagesFile      = "pages.json"
	commentsFile   = "comments.json"
	filesFile      = "attachments.json"
	peopleFile     = "people.json"
	bundleFiles    = "files/"
	maxBundleJSON  = 512 << 20
	maxPartEntries = 1_000_000
)

// bundleEntry is one file in a manifest.
type bundleEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// bundleManifest is signed, and says what a bundle holds (D11).
type bundleManifest struct {
	Format    string        `json:"format"`
	Version   int           `json:"version"`
	Instance  uuid.UUID     `json:"instance_id"`
	SpaceID   uuid.UUID     `json:"space_id"`
	CreatedAt time.Time     `json:"created_at"`
	PublicKey string        `json:"public_key"`
	Files     []bundleEntry `json:"files"`
}

type bundleGrant struct {
	SubjectType string    `json:"subject_type"`
	SubjectID   uuid.UUID `json:"subject_id"`
	Permission  string    `json:"permission"`
}

type bundleRestriction struct {
	PageID      uuid.UUID `json:"page_id"`
	Kind        string    `json:"kind"`
	SubjectType string    `json:"subject_type"`
	SubjectID   uuid.UUID `json:"subject_id"`
}

type bundleLabel struct {
	PageID uuid.UUID `json:"page_id"`
	Label  string    `json:"label"`
}

type bundleGroup struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type bundleSpace struct {
	ID           uuid.UUID           `json:"id"`
	Origin       uuid.UUID           `json:"origin_instance_id"`
	Key          string              `json:"key"`
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Icon         string              `json:"icon"`
	HomePageID   *uuid.UUID          `json:"home_page_id"`
	Grants       []bundleGrant       `json:"grants"`
	Restrictions []bundleRestriction `json:"restrictions"`
	Labels       []bundleLabel       `json:"labels"`
	Groups       []bundleGroup       `json:"groups"`
}

type bundleComment struct {
	ID       uuid.UUID       `json:"id"`
	Origin   uuid.UUID       `json:"origin_instance_id"`
	PageID   uuid.UUID       `json:"page_id"`
	ParentID *uuid.UUID      `json:"parent_id"`
	AuthorID uuid.UUID       `json:"author_id"`
	Body     json.RawMessage `json:"body"`
	Status   string          `json:"status"`
	Editors  bool            `json:"editors_only"`
	Created  time.Time       `json:"created_at"`
}

type bundleFile struct {
	ID       uuid.UUID `json:"id"`
	Origin   uuid.UUID `json:"origin_instance_id"`
	PageID   uuid.UUID `json:"page_id"`
	Filename string    `json:"filename"`
	Author   uuid.UUID `json:"created_by"`
}

// BundlePerson is someone the content refers to, and nothing else about them (G6).
type BundlePerson struct {
	ID      uuid.UUID  `json:"id"`
	Name    string     `json:"name"`
	Email   string     `json:"email"`
	Matched *uuid.UUID `json:"matched,omitempty"`
}

// StartBundle queues a bundle of a space. It holds everything, so it needs both
// administer and export (PRM-095).
func (s *Service) StartBundle(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (ExportRun, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
		return ExportRun{}, err
	}
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceExport); err != nil {
		return ExportRun{}, err
	}
	allowed, err := s.mayUseFormat(ctx, c.UserID, FormatBundle)
	if err != nil {
		return ExportRun{}, err
	}
	if !allowed {
		return ExportRun{}, apperr.Forbidden()
	}
	return s.queueExport(ctx, c, ExportRequest{SpaceID: spaceID, Format: FormatBundle}, JobKindBundle)
}

// BundleKey is this instance's public key, so another one can trust it.
type BundleKey struct {
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
}

func fingerprintOf(public ed25519.PublicKey) string {
	sum := sha256.Sum256(public)
	return hex.EncodeToString(sum[:])
}

// OwnBundleKey gives the key this instance signs bundles with.
func (s *Service) OwnBundleKey() (BundleKey, error) {
	private, err := s.keyring.SigningKey()
	if err != nil {
		return BundleKey{}, err
	}
	public, ok := private.Public().(ed25519.PublicKey)
	if !ok {
		return BundleKey{}, errors.New("exchange: signing key is not ed25519")
	}
	return BundleKey{PublicKey: base64.StdEncoding.EncodeToString(public), Fingerprint: fingerprintOf(public)}, nil
}

// bundleWriter adds files to a zip and keeps the sums for the manifest.
type bundleWriter struct {
	zw      *zip.Writer
	entries []bundleEntry
}

func (b *bundleWriter) add(name string, src io.Reader) error {
	f, err := b.zw.Create(name)
	if err != nil {
		return err
	}
	sum := sha256.New()
	size, err := io.Copy(io.MultiWriter(f, sum), src)
	if err != nil {
		return err
	}
	b.entries = append(b.entries, bundleEntry{Path: name, SHA256: hex.EncodeToString(sum.Sum(nil)), Size: size})
	return nil
}

func (b *bundleWriter) addJSON(name string, v any) error {
	f, err := b.zw.Create(name)
	if err != nil {
		return err
	}
	sum := sha256.New()
	counter := &countingWriter{}
	if err := json.NewEncoder(io.MultiWriter(f, sum, counter)).Encode(v); err != nil {
		return err
	}
	b.entries = append(b.entries, bundleEntry{Path: name, SHA256: hex.EncodeToString(sum.Sum(nil)), Size: counter.n})
	return nil
}

type countingWriter struct{ n int64 }

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return len(p), nil
}

func (s *Service) runBundle(ctx context.Context, j jobs.Job) (any, error) {
	var in exportPayload
	if err := json.Unmarshal(j.Payload, &in); err != nil {
		return nil, err
	}
	run, err := scanRun(s.pool.QueryRow(ctx, `SELECT `+runColumns+` FROM export_runs WHERE id = $1`, in.RunID))
	if err != nil {
		return nil, err
	}
	c := identity.Caller{UserID: run.createdBy, Meta: identity.RequestMeta{RequestID: "job-" + j.ID.String()}}
	report := &ExportReport{Drafts: []string{}, Failed: []string{}}
	if err := s.buildBundle(ctx, c, &run, report); err != nil {
		s.failExport(ctx, c, run, report, err)
		return report, err
	}
	return report, nil
}

// bundlePages reads all pages. One the person cannot see stops the bundle, because
// a bundle that silently misses pages would be worse (G6).
func (s *Service) bundlePages(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]pages.ImportedPage, error) {
	var hidden int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM pages p WHERE p.space_id = $1 AND p.status = 'published'
		AND p.id NOT IN (SELECT page_id FROM perm_visible_pages($2))`, spaceID, c.UserID).Scan(&hidden); err != nil {
		return nil, err
	}
	if hidden > 0 {
		return nil, apperr.Conflict("bundle_hidden_pages", "This space holds pages you cannot see, so a bundle would be incomplete. Ask for access first.")
	}
	var out []pages.ImportedPage
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT p.id, p.origin_instance_id, p.parent_id, p.position, p.title, p.kind, p.created_by, p.created_at
			FROM pages p WHERE p.space_id = $1 AND p.status = 'published'
			ORDER BY (SELECT count(*) FROM page_tree a WHERE a.descendant_id = p.id), p.position, p.id`, spaceID)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (pages.ImportedPage, error) {
			var p pages.ImportedPage
			err := r.Scan(&p.ID, &p.Origin, &p.ParentID, &p.Position, &p.Title, &p.Kind, &p.CreatedBy, &p.CreatedAt)
			return p, err
		})
		if err != nil {
			return err
		}
		for i := range out {
			if out[i].Versions, err = s.pages.BundleVersions(ctx, tx, out[i].ID); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

func (s *Service) bundleSpaceOf(ctx context.Context, spaceID uuid.UUID) (bundleSpace, error) {
	var sp bundleSpace
	if err := s.pool.QueryRow(ctx, `SELECT id, origin_instance_id, key, name, description, icon, home_page_id FROM spaces WHERE id = $1`, spaceID).
		Scan(&sp.ID, &sp.Origin, &sp.Key, &sp.Name, &sp.Description, &sp.Icon, &sp.HomePageID); err != nil {
		return sp, err
	}
	rows, err := s.pool.Query(ctx, `SELECT subject_type, subject_id, permission FROM space_grants WHERE space_id = $1`, spaceID)
	if err != nil {
		return sp, err
	}
	if sp.Grants, err = pgx.CollectRows(rows, pgx.RowToStructByPos[bundleGrant]); err != nil {
		return sp, err
	}
	rows, err = s.pool.Query(ctx, `SELECT r.page_id, r.kind, r.subject_type, r.subject_id FROM page_restrictions r
		JOIN pages p ON p.id = r.page_id WHERE p.space_id = $1 AND p.status = 'published'`, spaceID)
	if err != nil {
		return sp, err
	}
	if sp.Restrictions, err = pgx.CollectRows(rows, pgx.RowToStructByPos[bundleRestriction]); err != nil {
		return sp, err
	}
	rows, err = s.pool.Query(ctx, `SELECT l.page_id, l.label FROM page_labels l JOIN pages p ON p.id = l.page_id
		WHERE p.space_id = $1 AND p.status = 'published'`, spaceID)
	if err != nil {
		return sp, err
	}
	if sp.Labels, err = pgx.CollectRows(rows, pgx.RowToStructByPos[bundleLabel]); err != nil {
		return sp, err
	}
	rows, err = s.pool.Query(ctx, `SELECT g.id, g.name FROM groups g WHERE g.id IN (
			SELECT subject_id FROM space_grants WHERE space_id = $1 AND subject_type = 'group'
			UNION SELECT r.subject_id FROM page_restrictions r JOIN pages p ON p.id = r.page_id
				WHERE p.space_id = $1 AND r.subject_type = 'group')`, spaceID)
	if err != nil {
		return sp, err
	}
	sp.Groups, err = pgx.CollectRows(rows, pgx.RowToStructByPos[bundleGroup])
	return sp, err
}

func (s *Service) bundleComments(ctx context.Context, spaceID uuid.UUID) ([]bundleComment, error) {
	rows, err := s.pool.Query(ctx, `SELECT c.id, c.origin_instance_id, c.page_id, c.parent_id, c.author_id, c.body, c.status, c.editors_only, c.created_at
		FROM comments c JOIN pages p ON p.id = c.page_id
		WHERE c.space_id = $1 AND p.status = 'published' AND c.trash_item_id IS NULL AND NOT c.purged ORDER BY c.created_at, c.id`, spaceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[bundleComment])
}

// bundlePeople lists names and addresses of those the content refers to.
func (s *Service) bundlePeople(ctx context.Context, list []uuid.UUID) ([]BundlePerson, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, display_name, email FROM users WHERE id = ANY($1)`, list)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (BundlePerson, error) {
		var p BundlePerson
		err := r.Scan(&p.ID, &p.Name, &p.Email)
		return p, err
	})
}

type peopleSet map[uuid.UUID]bool

func (p peopleSet) list() []uuid.UUID {
	out := make([]uuid.UUID, 0, len(p))
	for id := range p {
		out = append(out, id)
	}
	return out
}

func peopleIn(sp bundleSpace, list []pages.ImportedPage, comments []bundleComment, files []bundleFile) peopleSet {
	people := peopleSet{}
	for _, p := range list {
		people[p.CreatedBy] = true
		for _, v := range p.Versions {
			people[v.AuthorID] = true
		}
	}
	for _, c := range comments {
		people[c.AuthorID] = true
	}
	for _, f := range files {
		people[f.Author] = true
	}
	for _, g := range sp.Grants {
		if g.SubjectType == permissions.SubjectUser {
			people[g.SubjectID] = true
		}
	}
	for _, r := range sp.Restrictions {
		if r.SubjectType == permissions.SubjectUser {
			people[r.SubjectID] = true
		}
	}
	return people
}

func (s *Service) buildBundle(ctx context.Context, c identity.Caller, run *ExportRun, report *ExportReport) error {
	list, err := s.bundlePages(ctx, c, run.SpaceID)
	if err != nil {
		return err
	}
	sp, err := s.bundleSpaceOf(ctx, run.SpaceID)
	if err != nil {
		return err
	}
	comments, err := s.bundleComments(ctx, run.SpaceID)
	if err != nil {
		return err
	}
	limits, err := s.Limits(ctx)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.tempDir, "bundle-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()); _ = tmp.Close() }()
	maxBytes := int64(limits.MaxExportMiB) * mebibyte
	zw := zip.NewWriter(&limitedWriter{w: tmp, left: maxBytes})
	b := &bundleWriter{zw: zw}
	files, err := s.addBundleFiles(ctx, b, list)
	if err != nil {
		return err
	}
	people, err := s.bundlePeople(ctx, peopleIn(sp, list, comments, files).list())
	if err != nil {
		return err
	}
	for name, v := range map[string]any{spaceFile: sp, pagesFile: list, commentsFile: comments, filesFile: files, peopleFile: people} {
		if err := b.addJSON(name, v); err != nil {
			return err
		}
	}
	if err := s.signBundle(b, run.SpaceID); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	size, err := tmp.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	run.objectKey = "exports/" + run.ID.String()
	if _, err := s.store.Put(ctx, run.objectKey, tmp, maxBytes); err != nil {
		return err
	}
	run.Filename, run.Size = safeName(sp.Name)+".bundle.zip", size
	items := make([]exportItem, 0, len(list))
	for _, p := range list {
		items = append(items, exportItem{page: pages.ExportPage{ID: p.ID}})
	}
	if len(items) == 0 {
		return apperr.Invalid("This space has no published pages.")
	}
	report.Pages = len(items)
	return s.finishExport(ctx, c, *run, items, report)
}

func (s *Service) addBundleFiles(ctx context.Context, b *bundleWriter, list []pages.ImportedPage) ([]bundleFile, error) {
	files := []bundleFile{}
	for _, p := range list {
		stored, err := s.attachments.ListForPage(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		for _, a := range stored {
			if err := s.addBundleFile(ctx, b, a); err != nil {
				return nil, err
			}
			files = append(files, bundleFile{ID: a.ID, Origin: p.Origin, PageID: p.ID, Filename: a.Filename, Author: a.CreatedBy})
		}
	}
	return files, nil
}

func (s *Service) addBundleFile(ctx context.Context, b *bundleWriter, a attachments.Attachment) error {
	src, err := s.attachments.OpenUnchecked(ctx, a)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	return b.add(bundleFiles+a.ID.String(), src)
}

func (s *Service) signBundle(b *bundleWriter, spaceID uuid.UUID) error {
	private, err := s.keyring.SigningKey()
	if err != nil {
		return err
	}
	key, err := s.OwnBundleKey()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(bundleManifest{Format: bundleFormat, Version: bundleVersion, Instance: s.instanceID, SpaceID: spaceID,
		CreatedAt: time.Now().UTC(), PublicKey: key.PublicKey, Files: b.entries})
	if err != nil {
		return err
	}
	f, err := b.zw.Create(manifestFile)
	if err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		return err
	}
	sig, err := b.zw.Create(signatureFile)
	if err != nil {
		return err
	}
	_, err = sig.Write(ed25519.Sign(private, raw))
	return err
}

// recordBundle audits the import of a bundle, with who signed it.
func recordBundle(ctx context.Context, tx pgx.Tx, c identity.Caller, m bundleManifest, spaceID uuid.UUID, signer string, result *ImportResult) error {
	if _, err := audit.Record(ctx, tx, c.Event("bundle.imported", "space", spaceID.String(), spaceID.String(), map[string]any{
		"signer": signer, "source_instance": m.Instance, "pages": result.Pages, "versions": result.Versions,
		"comments": result.Comments, "files": result.Files, "imported_history": true})); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, "bundle.imported", map[string]any{"space_id": spaceID})
	return err
}
