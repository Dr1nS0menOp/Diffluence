// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
)

// ImportMapping is what a person decides after reading a report.
type ImportMapping struct {
	People      map[uuid.UUID]uuid.UUID `json:"people"`
	Fingerprint string                  `json:"fingerprint"`
	SpaceKey    string                  `json:"space_key"`
}

// StartBundleImport stores a bundle and reads it for the report.
func (s *Service) StartBundleImport(ctx context.Context, c identity.Caller, filename string, body io.Reader) (ImportRun, error) {
	if !isAny(filename, ".zip") {
		return ImportRun{}, apperr.Invalid("Upload a bundle, which is a .zip file.")
	}
	run := ImportRun{ID: ids.New(), Kind: KindBundle, State: importReading, Filename: filename, createdBy: c.UserID}
	return s.storeImport(ctx, c, run, JobKindImportRead, body)
}

// SetMapping stores the choices of the report. A wrong fingerprint is refused here,
// and is checked again before anything is written.
func (s *Service) SetMapping(ctx context.Context, c identity.Caller, id uuid.UUID, m ImportMapping) (ImportRun, error) {
	run, err := s.Import(ctx, c, id)
	if err != nil {
		return run, err
	}
	if run.State != importReady {
		return run, apperr.Conflict("import_not_ready", "This import cannot change now.")
	}
	if m.Fingerprint != "" && !strings.EqualFold(m.Fingerprint, run.Report.Signer) {
		return run, apperr.Invalid("This fingerprint does not match the signer of the bundle.")
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return run, err
	}
	return run, pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE import_runs SET people_map = $2 WHERE id = $1`, id, raw); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("import.mapped", "import", id.String(), "", map[string]any{"people": len(m.People),
			"fingerprint": m.Fingerprint != ""})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "import.mapped", map[string]any{"import_id": id})
		return err
	})
}

func mappingOf(run ImportRun) (ImportMapping, error) {
	var m ImportMapping
	if len(run.peopleMap) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(run.peopleMap, &m); err != nil {
		return m, fmt.Errorf("exchange: read mapping: %v", err)
	}
	return m, nil
}

// checkBundleTrust decides whether a signer may be believed (D13). Our own key is
// known. Any other needs a person to have entered its fingerprint.
func (s *Service) checkBundleTrust(ctx context.Context, run ImportRun) error {
	own, err := s.OwnBundleKey()
	if err != nil {
		return err
	}
	if run.Report.Signer == own.Fingerprint {
		return nil
	}
	m, err := mappingOf(run)
	if err != nil {
		return err
	}
	if m.Fingerprint != "" && strings.EqualFold(m.Fingerprint, run.Report.Signer) {
		return nil
	}
	return apperr.Conflict("unknown_signer", "Enter the fingerprint of the instance this bundle comes from.")
}

// openedBundle is a bundle whose signature and every file have been checked.
type openedBundle struct {
	manifest bundleManifest
	signer   string
	files    map[string]*zip.File
}

func readZipFile(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, apperr.Invalid("This bundle cannot be read.")
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, apperr.Invalid("This bundle cannot be read.")
	}
	if int64(len(data)) > limit {
		return nil, apperr.TooLarge("A part of this bundle is too large.")
	}
	return data, nil
}

// verifyBundle checks the signature first, then every file against the manifest.
// Nothing in the bundle is believed before that, and a wrong signature has no override.
func verifyBundle(zr *zip.Reader) (openedBundle, error) {
	out := openedBundle{files: map[string]*zip.File{}}
	if len(zr.File) > maxPartEntries {
		return out, apperr.TooLarge("This bundle has too many files.")
	}
	for _, f := range zr.File {
		out.files[f.Name] = f
	}
	mf, sf := out.files[manifestFile], out.files[signatureFile]
	if mf == nil || sf == nil {
		return out, apperr.Invalid("This file is not a signed bundle.")
	}
	raw, err := readZipFile(mf, maxBundleJSON)
	if err != nil {
		return out, err
	}
	sig, err := readZipFile(sf, ed25519.SignatureSize+1)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out.manifest); err != nil {
		return out, apperr.Invalid("This bundle cannot be read.")
	}
	public, err := base64.StdEncoding.DecodeString(out.manifest.PublicKey)
	if err != nil || len(public) != ed25519.PublicKeySize || !ed25519.Verify(public, raw, sig) {
		return out, apperr.Invalid("The signature of this bundle is wrong. It was changed after it was made.")
	}
	if out.manifest.Format != bundleFormat ||
		out.manifest.Version < bundleVersionBeforeSplit || out.manifest.Version > bundleVersion {
		return out, apperr.Invalid("This version of a bundle cannot be read here.")
	}
	out.signer = fingerprintOf(public)
	return out, checkFiles(out)
}

func checkFiles(b openedBundle) error {
	listed := map[string]bool{manifestFile: true, signatureFile: true}
	for _, e := range b.manifest.Files {
		listed[e.Path] = true
		f := b.files[e.Path]
		if f == nil {
			return apperr.Invalid("This bundle misses a file its manifest lists.")
		}
		rc, err := f.Open()
		if err != nil {
			return apperr.Invalid("This bundle cannot be read.")
		}
		sum := sha256.New()
		_, err = io.Copy(sum, io.LimitReader(rc, e.Size+1))
		_ = rc.Close()
		if err != nil || hex.EncodeToString(sum.Sum(nil)) != e.SHA256 {
			return apperr.Invalid("A file in this bundle does not match its manifest.")
		}
	}
	for name := range b.files {
		if !listed[name] {
			return apperr.Invalid("This bundle holds a file its manifest does not list.")
		}
	}
	return nil
}

func (b openedBundle) decode(name string, into any) error {
	f := b.files[name]
	if f == nil {
		return apperr.Invalid("This bundle is not complete.")
	}
	raw, err := readZipFile(f, maxBundleJSON)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return apperr.Invalid("This bundle cannot be read.")
	}
	return nil
}

// bundleContent is what a checked bundle holds.
type bundleContent struct {
	space    bundleSpace
	pages    []pages.ImportedPage
	comments []bundleComment
	files    []bundleFile
	people   []BundlePerson
}

func (b openedBundle) content() (bundleContent, error) {
	var out bundleContent
	for name, into := range map[string]any{spaceFile: &out.space, pagesFile: &out.pages, commentsFile: &out.comments,
		filesFile: &out.files, peopleFile: &out.people} {
		if err := b.decode(name, into); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (s *Service) openBundle(ctx context.Context, run ImportRun) (openedBundle, func(), error) {
	src, err := s.store.Open(ctx, run.objectKey)
	if err != nil {
		return openedBundle{}, func() {}, err
	}
	defer func() { _ = src.Close() }()
	tmp, err := os.CreateTemp(s.tempDir, "bundle-in-*")
	if err != nil {
		return openedBundle{}, func() {}, err
	}
	done := func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }
	size, err := io.Copy(tmp, src)
	if err != nil {
		done()
		return openedBundle{}, func() {}, err
	}
	zr, err := zip.NewReader(tmp, size)
	if err != nil {
		done()
		return openedBundle{}, func() {}, apperr.Invalid("This file is not a signed bundle.")
	}
	b, err := verifyBundle(zr)
	if err != nil {
		done()
		return b, func() {}, err
	}
	return b, done, nil
}

func (s *Service) matchPeople(ctx context.Context, people []BundlePerson) ([]BundlePerson, error) {
	out := make([]BundlePerson, 0, len(people))
	for _, p := range people {
		var id uuid.UUID
		err := s.pool.QueryRow(ctx, `SELECT id FROM users WHERE lower(email) = lower($1) AND status = 'active'`, p.Email).Scan(&id)
		switch {
		case err == nil:
			p.Matched = &id
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// readBundle makes the report of a bundle.
func (s *Service) readBundle(ctx context.Context, run ImportRun, result *ImportResult) error {
	b, done, err := s.openBundle(ctx, run)
	defer done()
	if err != nil {
		return err
	}
	content, err := b.content()
	if err != nil {
		return err
	}
	result.Kind, result.Signer = KindBundle, b.signer
	result.Pages, result.Comments, result.Files = len(content.pages), len(content.comments), len(content.files)
	depths := map[uuid.UUID]int{}
	for _, p := range content.pages {
		result.Versions += len(p.Versions)
		depth := 0
		if p.ParentID != nil {
			depth = depths[*p.ParentID] + 1
		}
		depths[p.ID] = depth
		result.Tree = append(result.Tree, ReportPage{Title: p.Title, Depth: depth})
	}
	if result.People, err = s.matchPeople(ctx, content.people); err != nil {
		return err
	}
	for _, g := range content.space.Grants {
		if g.SubjectType == permissions.SubjectAllMembers || g.SubjectType == permissions.SubjectAllGuests {
			result.Warnings = append(result.Warnings, "This space is open to all members or guests. Here, that means other people.")
			break
		}
	}
	return s.noteTaken(ctx, content, result)
}

func (s *Service) noteTaken(ctx context.Context, content bundleContent, result *ImportResult) error {
	taken, err := s.idsTaken(ctx, content)
	if err != nil {
		return err
	}
	if taken {
		result.Warnings = append(result.Warnings, "This space is already here, so it comes in as a copy with new ids.")
	}
	var keyTaken bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM spaces WHERE lower(key) = lower($1))`, content.space.Key).Scan(&keyTaken); err != nil {
		return err
	}
	if keyTaken {
		result.Warnings = append(result.Warnings, fmt.Sprintf("The key %s is in use here. Choose a new one.", content.space.Key))
	}
	return nil
}

func (s *Service) idsTaken(ctx context.Context, content bundleContent) (bool, error) {
	list := make([]uuid.UUID, 0, len(content.pages))
	for _, p := range content.pages {
		list = append(list, p.ID)
	}
	var taken bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM spaces WHERE id = $1) OR EXISTS (SELECT 1 FROM pages WHERE id = ANY($2))`,
		content.space.ID, list).Scan(&taken)
	return taken, err
}

// remap gives everything new ids, for a copy of what is already here.
type remap struct {
	ids      map[uuid.UUID]uuid.UUID
	replacer *strings.Replacer
}

func newRemap(content bundleContent, copied bool) remap {
	r := remap{ids: map[uuid.UUID]uuid.UUID{}}
	if !copied {
		r.replacer = strings.NewReplacer()
		return r
	}
	add := func(id uuid.UUID) { r.ids[id] = ids.New() }
	add(content.space.ID)
	for _, p := range content.pages {
		add(p.ID)
	}
	for _, c := range content.comments {
		add(c.ID)
	}
	for _, f := range content.files {
		add(f.ID)
	}
	pairs := make([]string, 0, len(r.ids)*2)
	for from, to := range r.ids {
		pairs = append(pairs, from.String(), to.String())
	}
	r.replacer = strings.NewReplacer(pairs...)
	return r
}

func (r remap) id(id uuid.UUID) uuid.UUID {
	if to, ok := r.ids[id]; ok {
		return to
	}
	return id
}

func (r remap) optional(id *uuid.UUID) *uuid.UUID {
	if id == nil {
		return nil
	}
	to := r.id(*id)
	return &to
}

func (r remap) raw(in json.RawMessage) json.RawMessage {
	return json.RawMessage(r.replacer.Replace(string(in)))
}

// people answers who someone from the bundle is here. Who has no match becomes the
// person importing, and the history says it is imported (SEC-353).
type peopleLookup struct {
	known    map[uuid.UUID]uuid.UUID
	fallback uuid.UUID
}

func (p peopleLookup) author(id uuid.UUID) uuid.UUID {
	if to, ok := p.known[id]; ok {
		return to
	}
	return p.fallback
}

func (s *Service) lookupPeople(ctx context.Context, c identity.Caller, content bundleContent, m ImportMapping) (peopleLookup, error) {
	out := peopleLookup{known: map[uuid.UUID]uuid.UUID{}, fallback: c.UserID}
	matched, err := s.matchPeople(ctx, content.people)
	if err != nil {
		return out, err
	}
	for _, p := range matched {
		if p.Matched != nil {
			out.known[p.ID] = *p.Matched
		}
	}
	for from, to := range m.People {
		var active bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND status = 'active')`, to).Scan(&active); err != nil {
			return out, err
		}
		if active {
			out.known[from] = to
		}
	}
	return out, nil
}

// groupsHere matches groups by name.
func (s *Service) groupsHere(ctx context.Context, groups []bundleGroup) (map[uuid.UUID]uuid.UUID, error) {
	out := map[uuid.UUID]uuid.UUID{}
	for _, g := range groups {
		var id uuid.UUID
		err := s.pool.QueryRow(ctx, `SELECT id FROM groups WHERE lower(name) = lower($1)`, g.Name).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[g.ID] = id
	}
	return out, nil
}

// bundleImport writes one bundle in one transaction.
type bundleImport struct {
	s       *Service
	c       identity.Caller
	b       openedBundle
	content bundleContent
	ids     remap
	people  peopleLookup
	groups  map[uuid.UUID]uuid.UUID
	spaceID uuid.UUID
	result  *ImportResult
	keys    []string
}

func (s *Service) writeBundle(ctx context.Context, c identity.Caller, run *ImportRun, result *ImportResult) error {
	b, done, err := s.openBundle(ctx, *run)
	defer done()
	if err != nil {
		return err
	}
	run.Report.Signer = b.signer
	if err := s.checkBundleTrust(ctx, *run); err != nil {
		return err
	}
	result.Kind, result.Signer = KindBundle, b.signer
	content, err := b.content()
	if err != nil {
		return err
	}
	m, err := mappingOf(*run)
	if err != nil {
		return err
	}
	taken, err := s.idsTaken(ctx, content)
	if err != nil {
		return err
	}
	in := &bundleImport{s: s, c: c, b: b, content: content, ids: newRemap(content, taken), result: result}
	if in.people, err = s.lookupPeople(ctx, c, content, m); err != nil {
		return err
	}
	if in.groups, err = s.groupsHere(ctx, content.space.Groups); err != nil {
		return err
	}
	in.spaceID = in.ids.id(content.space.ID)
	run.SpaceID = &in.spaceID
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return in.write(ctx, tx, m) })
	if err != nil {
		for _, key := range in.keys {
			_ = s.store.Delete(context.WithoutCancel(ctx), key)
		}
		run.SpaceID = nil
		if db := "spaces_key_key"; isUniqueViolation(err) && strings.Contains(err.Error(), db) {
			return apperr.Conflict("key_taken", "This key is already in use.")
		}
	}
	return err
}

func (in *bundleImport) write(ctx context.Context, tx pgx.Tx, m ImportMapping) error {
	sp := in.content.space
	key := sp.Key
	if m.SpaceKey != "" {
		key = m.SpaceKey
	}
	if _, err := tx.Exec(ctx, `INSERT INTO spaces (id, origin_instance_id, key, name, description, icon, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, 'active', $7)`, in.spaceID, sp.Origin, key, sp.Name, sp.Description, sp.Icon, in.c.UserID); err != nil {
		return err
	}
	if err := permissions.ApplyTemplateTx(ctx, tx, in.spaceID, permissions.Subject{Type: permissions.SubjectUser, ID: in.c.UserID}, permissions.TemplateSpaceAdmin); err != nil {
		return err
	}
	if err := in.grants(ctx, tx); err != nil {
		return err
	}
	if err := in.pages(ctx, tx); err != nil {
		return err
	}
	if sp.HomePageID != nil {
		if _, err := tx.Exec(ctx, `UPDATE spaces SET home_page_id = $2 WHERE id = $1`, in.spaceID, in.ids.optional(sp.HomePageID)); err != nil {
			return err
		}
	}
	if err := in.restrictions(ctx, tx); err != nil {
		return err
	}
	if err := in.labels(ctx, tx); err != nil {
		return err
	}
	if err := in.files(ctx, tx); err != nil {
		return err
	}
	if err := in.comments(ctx, tx); err != nil {
		return err
	}
	return recordBundle(ctx, tx, in.c, in.b.manifest, in.spaceID, in.b.signer, in.result)
}

func (in *bundleImport) grants(ctx context.Context, tx pgx.Tx) error {
	for _, g := range in.content.space.Grants {
		subject, ok := in.subject(g.SubjectType, g.SubjectID)
		if !ok {
			continue // nobody here: the grant goes, and the space stays closed
		}
		perms := permissionsOfGrant(in.b.manifest.Version, g.Permission)
		// Grants are what roles add up to, so a grant comes in as part of a role (spec 036).
		if _, err := tx.Exec(ctx, "SELECT rbac_add_permissions($1, $2, $3, $4::text[])",
			in.spaceID, g.SubjectType, subject, perms); err != nil {
			return err
		}
	}
	return nil
}

// heldSince is the first bundle version that holds a permission split off a broader one.
// A bundle older than that knew only the broader one.
var heldSince = map[string]int{
	permissions.SitePublish:       2,
	permissions.SpaceAutomation:   2,
	permissions.SpaceTemplates:    2,
	permissions.SpaceAudit:        2,
	permissions.SpaceLabels:       3,
	permissions.SpaceStatuses:     3,
	permissions.SpaceApprovals:    3,
	permissions.SpaceBundle:       3,
	permissions.SpaceLook:         3,
	permissions.PageWatchers:      3,
	permissions.PageMoveOut:       3,
	permissions.SpaceInviteGuests: 3,
}

// permissionsOfGrant is what one grant in a bundle stands for here. In a bundle from before
// a split a broader permission brings the finer ones split off it, as the migration of
// that split did.
func permissionsOfGrant(version int, permission string) []string {
	perms := []string{permission}
	for finer, broad := range permissions.SplitFrom {
		if broad == permission && version < heldSince[finer] {
			perms = append(perms, finer)
		}
	}
	slices.Sort(perms)
	return perms
}

// subject finds who a grant or restriction means here.
func (in *bundleImport) subject(kind string, id uuid.UUID) (uuid.UUID, bool) {
	switch kind {
	case permissions.SubjectUser:
		to, ok := in.people.known[id]
		return to, ok
	case permissions.SubjectGroup:
		to, ok := in.groups[id]
		return to, ok
	}
	return id, true
}

func (in *bundleImport) pages(ctx context.Context, tx pgx.Tx) error {
	for _, p := range in.content.pages {
		out := p
		out.ID, out.ParentID, out.CreatedBy = in.ids.id(p.ID), in.ids.optional(p.ParentID), in.people.author(p.CreatedBy)
		out.Versions = make([]pages.ImportedVersion, 0, len(p.Versions))
		for _, v := range p.Versions {
			v.Document, v.AuthorID = in.ids.raw(v.Document), in.people.author(v.AuthorID)
			out.Versions = append(out.Versions, v)
		}
		if err := in.s.pages.InsertImported(ctx, tx, in.spaceID, out); err != nil {
			return fmt.Errorf("%s: %w", truncate(p.Title, maxReportedName), err)
		}
		in.result.Pages++
		in.result.Versions += len(p.Versions)
		in.result.Tree = append(in.result.Tree, ReportPage{Title: p.Title})
	}
	return nil
}

// restrictions keep a page at least as closed as it was. When someone on a list has
// no match, the person importing takes their place, so the list never empties and
// the page never opens up (GAP-011).
func (in *bundleImport) restrictions(ctx context.Context, tx pgx.Tx) error {
	for _, r := range in.content.space.Restrictions {
		page := in.ids.id(r.PageID)
		subjectType := r.SubjectType
		subject, ok := in.subject(r.SubjectType, r.SubjectID)
		if !ok {
			subjectType, subject = permissions.SubjectUser, in.c.UserID
			in.warnOnce("A restriction could not be matched, so the page stays closed to you alone.")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO page_restrictions (page_id, kind, subject_type, subject_id) VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING`, page, r.Kind, subjectType, subject); err != nil {
			return err
		}
	}
	return nil
}

func (in *bundleImport) warnOnce(text string) {
	if !slices.Contains(in.result.Warnings, text) {
		in.result.Warnings = append(in.result.Warnings, text)
	}
}

func (in *bundleImport) labels(ctx context.Context, tx pgx.Tx) error {
	for _, l := range in.content.space.Labels {
		if _, err := tx.Exec(ctx, `INSERT INTO page_labels (page_id, label, created_by) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
			in.ids.id(l.PageID), l.Label, in.c.UserID); err != nil {
			return err
		}
	}
	return nil
}

func (in *bundleImport) files(ctx context.Context, tx pgx.Tx) error {
	for _, f := range in.content.files {
		zf := in.b.files[bundleFiles+f.ID.String()]
		if zf == nil {
			return apperr.Invalid("This bundle misses a file.")
		}
		rc, err := zf.Open()
		if err != nil {
			return apperr.Invalid("This bundle cannot be read.")
		}
		key, err := in.s.attachments.AddImported(ctx, tx, attachments.ImportedFile{ID: in.ids.id(f.ID), Origin: f.Origin, SpaceID: in.spaceID,
			PageID: in.ids.id(f.PageID), Filename: f.Filename, Author: in.people.author(f.Author)}, rc)
		_ = rc.Close()
		if key != "" {
			in.keys = append(in.keys, key)
		}
		if err != nil {
			return err
		}
		in.result.Files++
	}
	return nil
}

func (in *bundleImport) comments(ctx context.Context, tx pgx.Tx) error {
	for _, c := range in.content.comments {
		doc, err := document.Normalize(bytes.Clone(in.ids.raw(c.Body)))
		if err != nil {
			continue // a body the gate refuses stays out, and the report counts what came in
		}
		raw, err := document.Marshal(doc)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO comments (id, origin_instance_id, space_id, page_id, parent_id, author_id, body, status, editors_only, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`, in.ids.id(c.ID), c.Origin, in.spaceID, in.ids.id(c.PageID), in.ids.optional(c.ParentID),
			in.people.author(c.AuthorID), raw, commentStatus(c.Status), c.Editors, c.Created); err != nil {
			return err
		}
		in.result.Comments++
	}
	return nil
}

func commentStatus(status string) string {
	if status == "resolved" {
		return status
	}
	return "open"
}
