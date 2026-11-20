// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"bytes"
	"context"
	"crypto/sha256"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// Signatures that came with a bundle (spec 064 wave DC5, DOC-1212, DOC-1421). Trust is local to
// an installation's audit chain, so they are shown as signed in another installation: their
// mark compares the hash they carry with the content of the version here, and they never
// count as a signature or an approval of this installation.

const sha256Size = sha256.Size

// ImportedSignature is one such record with its mark.
type ImportedSignature struct {
	ID             uuid.UUID `json:"id"`
	Version        int       `json:"version"`
	SourceInstance uuid.UUID `json:"source_instance"`
	SignerName     string    `json:"signer_name"`
	Meaning        string    `json:"meaning"`
	MeaningName    string    `json:"meaning_name"`
	Method         string    `json:"method"`
	SignedAt       time.Time `json:"signed_at"`
	ImportedAt     time.Time `json:"imported_at"`
	// Matches says the version here still holds the content that was signed.
	Matches bool `json:"matches"`
}

// maxImportedSignatures bounds the list of one page.
const maxImportedSignatures = 1000

// ImportedSignatures lists the imported signatures of the versions of a page the person may
// see in its history, each verified against the content of its version now.
func (s *Service) ImportedSignatures(ctx context.Context, c identity.Caller, id uuid.UUID) ([]ImportedSignature, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return nil, err
	}
	view, err := s.historyOf(ctx, c, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT id, version_number, source_instance, printed_name, meaning, meaning_name, method, signed_at, imported_at,
			content_hash, content_form
		FROM imported_signatures WHERE page_id = $1 ORDER BY signed_at, id LIMIT $2`, id, maxImportedSignatures)
	if err != nil {
		return nil, err
	}
	type row struct {
		ImportedSignature
		hash []byte
		form int16
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var x row
		err := r.Scan(&x.ID, &x.Version, &x.SourceInstance, &x.SignerName, &x.Meaning, &x.MeaningName, &x.Method, &x.SignedAt, &x.ImportedAt,
			&x.hash, &x.form)
		return x, err
	})
	if err != nil {
		return nil, err
	}
	out := []ImportedSignature{}
	hashes := map[int][]byte{}
	for _, x := range list {
		if !view.shows(x.Version) {
			continue
		}
		hash, known := hashes[x.Version]
		if !known {
			content, err := signedContentOf(ctx, s.pool, id, x.Version)
			if err != nil {
				return nil, err
			}
			hash = content.Hash()
			hashes[x.Version] = hash
		}
		x.Matches = x.form == ContentForm && bytes.Equal(hash, x.hash)
		out = append(out, x.ImportedSignature)
	}
	return out, nil
}
