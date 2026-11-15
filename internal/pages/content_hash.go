// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/db"
)

// The content a signature is bound to (DOC-711): SHA-256 over a canonical form of a
// version's title, its document and the files it pins. The form is versioned, so a later
// change of the document format can still verify the signatures made before it. Its exact
// shape is written down in docs/operations/regulated-use.md, where an auditor rebuilds it
// outside Diffluence (DOC-742); change it only as a new form.

// ContentForm is the version of the canonical form new signatures use.
const ContentForm = 1

// SignedFile is one file a signed version shows, as its canonical form holds it.
type SignedFile struct {
	ID       uuid.UUID `json:"id"`
	Version  int       `json:"version"`
	Filename string    `json:"filename"`
	SHA256   string    `json:"sha256"`
}

// SignedContent is what a signature covers of one version of a page.
type SignedContent struct {
	Title string `json:"title"`
	// SchemaVersion is the document format the version was written in.
	SchemaVersion int `json:"schema_version"`
	// Document is the version's document as canonical JSON: keys sorted, no spaces, numbers
	// as stored, no escaping beyond what JSON needs.
	Document string       `json:"document"`
	Files    []SignedFile `json:"files"`
}

// CanonicalForm is the exact bytes the hash covers, in form 1: a header line, then one line
// per part, each value with its length in bytes so no value can pose as another part.
func (c SignedContent) CanonicalForm() []byte {
	var b bytes.Buffer
	b.WriteString("diffluence-signed-content\n1\n")
	field := func(name, value string) {
		b.WriteString(name)
		b.WriteByte(' ')
		b.WriteString(strconv.Itoa(len(value)))
		b.WriteByte('\n')
		b.WriteString(value)
		b.WriteByte('\n')
	}
	field("title", c.Title)
	field("schema", strconv.Itoa(c.SchemaVersion))
	field("document", c.Document)
	files := slices.Clone(c.Files)
	slices.SortFunc(files, func(x, y SignedFile) int { return strings.Compare(x.ID.String(), y.ID.String()) })
	field("files", strconv.Itoa(len(files)))
	for _, f := range files {
		field("file", f.ID.String())
		field("version", strconv.Itoa(f.Version))
		field("name", f.Filename)
		field("sha256", f.SHA256)
	}
	return b.Bytes()
}

// Hash is the SHA-256 of the canonical form.
func (c SignedContent) Hash() []byte {
	sum := sha256.Sum256(c.CanonicalForm())
	return sum[:]
}

// canonicalJSON writes a document as canonical JSON. Numbers keep the digits they were
// stored with, so nothing is lost or rounded on the way.
func canonicalJSON(raw []byte) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return "", fmt.Errorf("pages: read document for its hash: %v", err)
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", fmt.Errorf("pages: write document for its hash: %v", err)
	}
	return strings.TrimSuffix(out.String(), "\n"), nil
}

// signedContentOf reads one version as its signature covers it: its title, document and the
// files it pins, at their pinned version and name with the hash of that version's bytes.
func signedContentOf(ctx context.Context, q db.Querier, pageID uuid.UUID, number int) (SignedContent, error) {
	var c SignedContent
	var raw []byte
	err := q.QueryRow(ctx, "SELECT title, document, schema_version FROM page_versions WHERE page_id = $1 AND number = $2",
		pageID, number).Scan(&c.Title, &raw, &c.SchemaVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, apperr.NotFound()
	}
	if err != nil {
		return c, err
	}
	if c.Document, err = canonicalJSON(raw); err != nil {
		return c, err
	}
	pinned, err := attachments.PinnedFiles(ctx, q, pageID, number)
	if err != nil {
		return c, err
	}
	c.Files = make([]SignedFile, len(pinned))
	for i, f := range pinned {
		c.Files[i] = SignedFile{ID: f.ID, Version: f.Version, Filename: f.Filename, SHA256: f.SHA256}
	}
	return c, nil
}

// hexHash is how a hash reads in an audit entry, an export and the screens.
func hexHash(h []byte) string { return hex.EncodeToString(h) }
